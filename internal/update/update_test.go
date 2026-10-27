package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestVersions(t *testing.T) {
	if !Newer("99.0.0") || Newer("0.0.1") || Newer("not a version") {
		t.Fatal("Newer")
	}
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.1.9", 1},
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "1.0.0-dev", 0},
		{"0.1.0", "0.1.1", -1},
	} {
		if got := compare(c.a, c.b); got != c.want {
			t.Errorf("compare(%s, %s) = %d", c.a, c.b, got)
		}
	}
}

func TestRestartsWithoutOpeningATab(t *testing.T) {
	if got := RestartArgs([]string{"--port", "4300"}); !slices.Equal(got, []string{"--port", "4300", "--no-open"}) {
		t.Fatal(got)
	}
	if got := RestartArgs([]string{"--no-open"}); !slices.Equal(got, []string{"--no-open"}) {
		t.Fatal(got)
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// releaseServer serves a GitHub-like latest release with an archive for this machine.
func releaseServer(t *testing.T, archive []byte, sums string) {
	t.Helper()
	name := Archive("9.9.9")
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[{"name":%q,"browser_download_url":"%s/a"},{"name":"checksums.txt","browser_download_url":"%s/sums"}]}`, name, srv.URL, srv.URL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(sums)) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("KANDO_RELEASES_URL", srv.URL)
}

func TestLatestAndInstallPieces(t *testing.T) {
	archive := tarGz(t, map[string]string{"README.md": "hi", "kando": "new program"})
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  " + Archive("9.9.9") + "\n"
	releaseServer(t, archive, sums)

	if v := Latest(true); v != "9.9.9" {
		t.Fatalf("latest %q", v)
	}
	if err := verify(archive, []byte(sums), Archive("9.9.9")); err != nil {
		t.Fatal(err)
	}
	if err := verify(append(archive, 0), []byte(sums), Archive("9.9.9")); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered archive: %v", err)
	}
	if err := verify(archive, []byte(sums), "other.tar.gz"); err == nil {
		t.Fatal("missing checksum line accepted")
	}
	bin, err := extract(archive, "kando")
	if err != nil || string(bin) != "new program" {
		t.Fatalf("extract: %q %v", bin, err)
	}

	exe := filepath.Join(t.TempDir(), "kando")
	os.WriteFile(exe, []byte("old program"), 0o755)
	if err := replace(exe, bin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	info, _ := os.Stat(exe)
	if string(got) != "new program" || info.Mode().Perm() != 0o755 {
		t.Fatalf("replaced with %q, mode %v", got, info.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".kando-update-*")); len(left) > 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestLatestOffline(t *testing.T) {
	t.Setenv("KANDO_RELEASES_URL", "http://127.0.0.1:1")
	if v := Latest(true); v != "" {
		t.Fatalf("offline latest %q", v)
	}
}
