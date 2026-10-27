package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azmainadel/kan-do/internal/home"
)

func TestHosts(t *testing.T) {
	local := &guard{port: 4242}
	for _, host := range []string{"localhost", "localhost:4242", "127.0.0.1:4242", "[::1]:4242", "LOCALHOST:4242"} {
		if !local.allows(host) {
			t.Errorf("local refuses %s", host)
		}
	}
	for _, host := range []string{"evil.com", "evil.com:4242", "localhost:80", "192.168.1.5:4242", "", "pi.local:4242"} {
		if local.allows(host) {
			t.Errorf("local allows %s", host)
		}
	}
	lan := &guard{port: 4242, lan: true, names: []string{"pi", "pi.local"}}
	for _, host := range []string{"localhost:4242", "192.168.1.5:4242", "[fe80::1]:4242", "pi.local:4242", "pi:4242", "PI.LOCAL."} {
		if !lan.allows(host) {
			t.Errorf("lan refuses %s", host)
		}
	}
	for _, host := range []string{"evil.com:4242", "pi.evil.com:4242", "192.168.1.5:80"} {
		if lan.allows(host) {
			t.Errorf("lan allows %s", host)
		}
	}
}

// testBoard is a test client for a board in a temporary data folder.
type testBoard struct {
	t    *testing.T
	home *home.Home
	srv  *httptest.Server
}

func newBoard(t *testing.T) *testBoard {
	t.Helper()
	h, err := home.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The host guard only answers on the configured port.
	srv := httptest.NewUnstartedServer(nil)
	h.Config.Port = uint16(srv.Listener.Addr().(*net.TCPAddr).Port)
	srv.Config.Handler = Handler(h)
	srv.Start()
	t.Cleanup(srv.Close)
	return &testBoard{t: t, home: h, srv: srv}
}

// do sends a request with an optional JSON body and decodes the JSON reply into out.
func (b *testBoard) do(method, path string, body, out any) int {
	b.t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, b.srv.URL+path, r)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			b.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return res.StatusCode
}

type apiError struct {
	Error string `json:"error"`
}

func TestAPI(t *testing.T) {
	b := newBoard(t)
	var p struct {
		Slug  string   `json:"slug"`
		Repos []string `json:"repos"`
		Color *string  `json:"color"`
		Name  string   `json:"name"`
	}
	if code := b.do("POST", "/api/projects", map[string]any{"slug": "API", "repo": "Me/Api"}, &p); code != 200 || p.Slug != "api" || p.Repos[0] != "me/api" {
		t.Fatalf("create project: %d %+v", code, p)
	}
	var e apiError
	if code := b.do("POST", "/api/projects", map[string]any{"slug": "api"}, &e); code != 400 || !strings.Contains(e.Error, "already exists") {
		t.Fatalf("duplicate project: %d %q", code, e.Error)
	}
	if b.do("PATCH", "/api/projects/api", map[string]any{"name": "API server", "color": "teal"}, &p); p.Name != "API server" || *p.Color != "teal" {
		t.Fatalf("update project: %+v", p)
	}
	if b.do("POST", "/api/projects/api/repos", map[string]any{"repo": "me/docs"}, &p); len(p.Repos) != 2 {
		t.Fatalf("link repo: %+v", p)
	}
	if b.do("DELETE", "/api/projects/api/repos", map[string]any{"repo": "me/docs"}, &p); len(p.Repos) != 1 {
		t.Fatalf("unlink repo: %+v", p)
	}

	type task struct {
		ID       int64    `json:"id"`
		Title    string   `json:"title"`
		Status   string   `json:"status"`
		Tags     []string `json:"tags"`
		Human    bool     `json:"human"`
		Comments int      `json:"comments"`
	}
	var a, c task
	b.do("POST", "/api/tasks", map[string]any{"project": "api", "title": " a "}, &a)
	b.do("POST", "/api/tasks", map[string]any{"project": "api", "title": "c"}, &c)
	if a.Title != "a" || a.Status != "todo" {
		t.Fatalf("create task: %+v", a)
	}
	if code := b.do("POST", "/api/tasks", map[string]any{"project": "nope", "title": "x"}, &e); code != 404 {
		t.Fatalf("task in unknown project: %d", code)
	}
	b.do("PATCH", "/api/tasks/1", map[string]any{"tags": "Bug, ui", "human": true}, &a)
	if strings.Join(a.Tags, ",") != "bug,ui" || !a.Human {
		t.Fatalf("patch task: %+v", a)
	}
	if code := b.do("PATCH", "/api/tasks/1", map[string]any{"status": "nope"}, &e); code != 400 {
		t.Fatalf("bad status: %d", code)
	}
	b.do("POST", "/api/tasks/2/move", map[string]any{"status": "todo", "before": 1}, &c)

	var board struct {
		Changes    string   `json:"changes"`
		DoneHidden int      `json:"done_hidden"`
		Statuses   []string `json:"statuses"`
		Tasks      []task   `json:"tasks"`
		Version    string   `json:"version"`
	}
	b.do("GET", "/api/board?project=api", nil, &board)
	if len(board.Tasks) != 2 || board.Tasks[0].Title != "c" || board.Changes == "" || board.Version == "" || len(board.Statuses) != 4 {
		t.Fatalf("board: %+v", board)
	}
	if code := b.do("GET", "/api/board?project=nope", nil, &e); code != 404 {
		t.Fatalf("board of unknown project: %d", code)
	}

	var found []task
	if b.do("GET", "/api/search?q=c", nil, &found); len(found) != 1 || found[0].Title != "c" {
		t.Fatalf("search: %+v", found)
	}

	var comment struct {
		ID     int64  `json:"id"`
		Author string `json:"author"`
	}
	b.do("POST", "/api/tasks/1/comments", map[string]any{"body": "hello"}, &comment)
	if comment.Author != "me" {
		t.Fatalf("comment: %+v", comment)
	}
	if code := b.do("POST", "/api/tasks/1/comments", map[string]any{"body": " "}, &e); code != 400 {
		t.Fatalf("empty comment: %d", code)
	}
	var detail struct {
		Task     task  `json:"task"`
		Comments []any `json:"comments"`
		Images   []any `json:"images"`
	}
	if b.do("GET", "/api/tasks/1", nil, &detail); detail.Task.Comments != 1 || len(detail.Comments) != 1 || detail.Images == nil {
		t.Fatalf("task detail: %+v", detail)
	}
	b.do("DELETE", "/api/comments/1", nil, &comment)
	if code := b.do("DELETE", "/api/tasks/2", nil, &c); code != 200 || c.Title != "c" {
		t.Fatalf("delete task: %d %+v", code, c)
	}
	if code := b.do("GET", "/api/tasks/2", nil, &e); code != 404 {
		t.Fatalf("deleted task: %d", code)
	}
	if code := b.do("GET", "/api/tasks/x", nil, &e); code != 400 {
		t.Fatalf("bad id: %d", code)
	}
}

func TestImages(t *testing.T) {
	b := newBoard(t)
	b.do("POST", "/api/projects", map[string]any{"slug": "api"}, nil)
	b.do("POST", "/api/tasks", map[string]any{"project": "api", "title": "a"}, nil)

	upload := func(contentType string, data []byte) (int, map[string]string) {
		res, err := http.Post(b.srv.URL+"/api/images", contentType, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]string
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if code, _ := upload("image/svg+xml", []byte("<svg/>")); code != 400 {
		t.Fatalf("svg: %d", code)
	}
	if code, _ := upload("image/png", nil); code != 400 {
		t.Fatalf("empty: %d", code)
	}
	code, saved := upload("image/png", []byte("png bytes"))
	file := saved["file"]
	if code != 200 || !strings.HasSuffix(file, ".png") || len(file) != 20 {
		t.Fatalf("upload: %d %v", code, saved)
	}
	if _, again := upload("image/png", []byte("png bytes")); again["file"] != file {
		t.Fatal("the same image saved twice gets another name")
	}

	res, err := http.Get(b.srv.URL + "/images/" + file)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(data) != "png bytes" || res.Header.Get("Content-Type") != "image/png" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("serve image: %q %v", data, res.Header)
	}
	if res, _ := http.Get(b.srv.URL + "/images/..%2fkan-do.db"); res.StatusCode != 404 {
		t.Fatalf("path traversal: %d", res.StatusCode)
	}

	var e apiError
	if code := b.do("POST", "/api/tasks/1/images", map[string]any{"file": "0000000000000000.png"}, &e); code != 400 {
		t.Fatalf("attach unsaved image: %d", code)
	}
	if code := b.do("POST", "/api/tasks/1/images", map[string]any{"file": file}, nil); code != 200 {
		t.Fatalf("attach: %d", code)
	}
	// Deleting the project deletes its image files too.
	b.do("DELETE", "/api/projects/api", nil, nil)
	if _, err := os.Stat(filepath.Join(b.home.ImagesDir(), file)); !os.IsNotExist(err) {
		t.Fatalf("image file left behind: %v", err)
	}
}

func TestGuardOnRequests(t *testing.T) {
	b := newBoard(t)
	for _, tc := range []struct {
		host, origin string
		code         int
	}{
		{"", "", 200},
		{"evil.com", "", 403},
		{"", "http://evil.com", 403},
	} {
		req, _ := http.NewRequest("GET", b.srv.URL+"/api/ping", nil)
		if tc.host != "" {
			req.Host = tc.host
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		} else {
			req.Header.Set("Origin", "http://"+req.URL.Host)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Errorf("host %q origin %q: %d, want %d", tc.host, tc.origin, res.StatusCode, tc.code)
		}
		if res.Header.Get("Content-Security-Policy") == "" {
			t.Error("no CSP header")
		}
	}
}

func TestEvents(t *testing.T) {
	h, err := home.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", newHub(h, 10*time.Millisecond).events)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	lines := bufio.NewScanner(res.Body)
	next := func() string {
		t.Helper()
		for lines.Scan() {
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				return data
			}
		}
		t.Fatal("stream ended")
		return ""
	}
	first := next()

	// A write from another process, like `kan-do mcp`, shows up as a new token.
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	s.CreateProject("api", "", "")
	s.Close()
	if second := next(); second == first {
		t.Fatalf("token unchanged after a write: %s", second)
	}
}

func TestStaticUI(t *testing.T) {
	b := newBoard(t)
	res, err := http.Get(b.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), "kan-do") {
		t.Fatalf("index: %d %s", res.StatusCode, body)
	}
}
