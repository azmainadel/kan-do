package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/store"
	"github.com/azmainadel/kan-do/internal/version"
)

const (
	// doneShown is how many done tasks the board gets unless it asks for all: the
	// newest, which are all it shows. The done column only grows.
	doneShown = 50
	// imageLimit is the largest image the board takes from a paste or drop.
	imageLimit  = 25 << 20
	searchLimit = 30
)

// api serves the JSON API behind the board.
type api struct {
	home *home.Home
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func statusOf(err error) int {
	switch store.KindOf(err) {
	case store.Invalid:
		return http.StatusBadRequest
	case store.NotFound:
		return http.StatusNotFound
	case store.Busy:
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// handle runs f with a fresh connection and answers with its result as JSON.
func (a *api) handle(f func(r *http.Request, s *store.Store) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := a.home.Store()
		if err != nil {
			writeError(w, statusOf(err), err.Error())
			return
		}
		defer s.Close()
		v, err := f(r, s)
		if err != nil {
			writeError(w, statusOf(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

// decode reads a JSON body into v. Unknown fields are ignored.
func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return &store.Error{Kind: store.Invalid, Msg: "invalid JSON: " + err.Error()}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, &store.Error{Kind: store.Invalid, Msg: fmt.Sprintf("%q is not an id", r.PathValue("id"))}
	}
	return id, nil
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "kan-do")
	})
	mux.HandleFunc("GET /api/board", a.handle(a.board))
	mux.HandleFunc("GET /api/search", a.handle(func(r *http.Request, s *store.Store) (any, error) {
		return s.Search(r.URL.Query().Get("q"), searchLimit)
	}))
	mux.HandleFunc("POST /api/projects", a.handle(a.createProject))
	mux.HandleFunc("PATCH /api/projects/{slug}", a.handle(a.updateProject))
	mux.HandleFunc("DELETE /api/projects/{slug}", a.handle(a.deleteProject))
	mux.HandleFunc("POST /api/projects/{slug}/repos", a.handle(a.linkRepo))
	mux.HandleFunc("DELETE /api/projects/{slug}/repos", a.handle(a.unlinkRepo))
	mux.HandleFunc("POST /api/tasks", a.handle(a.createTask))
	mux.HandleFunc("GET /api/tasks/{id}", a.handle(a.getTask))
	mux.HandleFunc("PATCH /api/tasks/{id}", a.handle(a.updateTask))
	mux.HandleFunc("DELETE /api/tasks/{id}", a.handle(a.deleteTask))
	mux.HandleFunc("POST /api/tasks/{id}/move", a.handle(a.moveTask))
	mux.HandleFunc("POST /api/tasks/{id}/comments", a.handle(a.addComment))
	mux.HandleFunc("POST /api/tasks/{id}/images", a.handle(a.attachImage))
	mux.HandleFunc("DELETE /api/comments/{id}", a.handle(a.deleteComment))
	mux.HandleFunc("DELETE /api/images/{id}", a.handle(a.deleteImage))
	mux.HandleFunc("POST /api/images", a.saveImage)
	mux.HandleFunc("GET /images/{file}", a.imageFile)
	a.systemRoutes(mux)
}

type board struct {
	Changes    string          `json:"changes"`
	DoneHidden int             `json:"done_hidden"`
	Projects   []store.Project `json:"projects"`
	Statuses   []string        `json:"statuses"`
	Tasks      []store.Task    `json:"tasks"`
	Version    string          `json:"version"`
}

func (a *api) board(r *http.Request, s *store.Store) (any, error) {
	q := r.URL.Query()
	var projectID int64
	if slug := q.Get("project"); slug != "" && slug != "all" {
		p, err := s.FindProject(slug)
		if err != nil {
			return nil, err
		}
		projectID = p.ID
	}
	keep := doneShown
	if q.Get("done") == "all" {
		keep = store.KeepAll
	}
	tasks, hidden, err := s.ListTasksKeepingDone(projectID, "", keep)
	if err != nil {
		return nil, err
	}
	projects, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	n, err := s.Changes()
	if err != nil {
		return nil, err
	}
	return board{
		Changes:    fmt.Sprintf("%d-%d", n, a.home.DBIdentity()),
		DoneHidden: hidden,
		Projects:   projects,
		Statuses:   s.Statuses(),
		Tasks:      tasks,
		Version:    version.Version,
	}, nil
}

func (a *api) createProject(r *http.Request, s *store.Store) (any, error) {
	var body struct {
		Slug string  `json:"slug"`
		Name string  `json:"name"`
		Repo *string `json:"repo"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	repo := ""
	if body.Repo != nil {
		repo = *body.Repo
	}
	return s.CreateProject(body.Slug, body.Name, repo)
}

func (a *api) updateProject(r *http.Request, s *store.Store) (any, error) {
	var body struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	slug := r.PathValue("slug")
	if body.Name != nil {
		if _, err := s.RenameProject(slug, *body.Name); err != nil {
			return nil, err
		}
	}
	if body.Color != nil {
		if _, err := s.SetProjectColor(slug, *body.Color); err != nil {
			return nil, err
		}
	}
	return s.FindProject(slug)
}

func (a *api) deleteProject(r *http.Request, s *store.Store) (any, error) {
	p, err := s.FindProject(r.PathValue("slug"))
	if err != nil {
		return nil, err
	}
	return home.DeletingImages(a.home, s, store.OfProject, p.ID, func() (*store.Project, error) {
		return s.DeleteProject(p.Slug)
	})
}

type repoBody struct {
	Repo string `json:"repo"`
}

func (a *api) linkRepo(r *http.Request, s *store.Store) (any, error) {
	var body repoBody
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	return s.LinkRepo(r.PathValue("slug"), body.Repo)
}

func (a *api) unlinkRepo(r *http.Request, s *store.Store) (any, error) {
	var body repoBody
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	return s.UnlinkRepo(r.PathValue("slug"), body.Repo)
}

func (a *api) createTask(r *http.Request, s *store.Store) (any, error) {
	var body struct {
		Project     string `json:"project"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	p, err := s.FindProject(body.Project)
	if err != nil {
		return nil, err
	}
	return s.CreateTask(p.ID, body.Title, body.Description)
}

func (a *api) getTask(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	task, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	comments, err := s.ListComments(id)
	if err != nil {
		return nil, err
	}
	images, err := s.ListImages(id)
	if err != nil {
		return nil, err
	}
	return struct {
		Comments []store.Comment `json:"comments"`
		Images   []store.Image   `json:"images"`
		Task     *store.Task     `json:"task"`
	}{comments, images, task}, nil
}

func (a *api) updateTask(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Project     *string `json:"project"`
		Tags        *string `json:"tags"`
		Human       *bool   `json:"human"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	patch := store.TaskPatch{Title: body.Title, Description: body.Description, Status: body.Status, Tags: body.Tags, Human: body.Human}
	if body.Project != nil {
		p, err := s.FindProject(*body.Project)
		if err != nil {
			return nil, err
		}
		patch.ProjectID = &p.ID
	}
	return s.UpdateTask(id, patch)
}

func (a *api) deleteTask(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return home.DeletingImages(a.home, s, store.OfTask, id, func() (*store.Task, error) { return s.DeleteTask(id) })
}

func (a *api) moveTask(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Status string `json:"status"`
		Before *int64 `json:"before"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	var before int64
	if body.Before != nil {
		before = *body.Before
	}
	return s.MoveTask(id, body.Status, before)
}

func (a *api) addComment(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Body   string   `json:"body"`
		Images []string `json:"images"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	for _, f := range body.Images {
		if err := a.imageSaved(f); err != nil {
			return nil, err
		}
	}
	return s.AddComment(id, "me", body.Body, body.Images)
}

func (a *api) attachImage(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		File string `json:"file"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if err := a.imageSaved(body.File); err != nil {
		return nil, err
	}
	return s.AddImage(id, body.File)
}

func (a *api) deleteComment(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return home.DeletingImages(a.home, s, store.OfComment, id, func() (*store.Comment, error) { return s.DeleteComment(id) })
}

func (a *api) deleteImage(r *http.Request, s *store.Store) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return home.DeletingImages(a.home, s, store.OfImage, id, func() (*store.Image, error) { return s.DeleteImage(id) })
}

// imageSaved fails unless file is an image saved by POST /api/images.
func (a *api) imageSaved(file string) error {
	if store.ValidImageFile(file) {
		if info, err := os.Stat(filepath.Join(a.home.ImagesDir(), file)); err == nil && info.Mode().IsRegular() {
			return nil
		}
	}
	return &store.Error{Kind: store.Invalid, Msg: fmt.Sprintf("no saved image %q; paste it again", file)}
}

var imageTypes = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}

// saveImage saves a pasted or dropped image in images/ and answers with its file name
// and path; the board then attaches it to a task or a comment. Files are named by a
// hash of their content, so the same image saved twice is stored once.
func (a *api) saveImage(w http.ResponseWriter, r *http.Request) {
	ext, ok := imageTypes[r.Header.Get("Content-Type")]
	if !ok {
		writeError(w, http.StatusBadRequest, "paste a PNG, JPEG, GIF or WebP image")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imageLimit))
	if mbe, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the image is larger than %d MB", mbe.Limit>>20))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read the image: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "the image is empty")
		return
	}
	sum := sha256.Sum256(body)
	file := hex.EncodeToString(sum[:8]) + "." + ext
	dir := a.home.ImagesDir()
	path := filepath.Join(dir, file)
	if _, err := os.Stat(path); err != nil {
		if err := writeAtomic(dir, path, body); err != nil {
			writeError(w, http.StatusInternalServerError, "cannot save the image: "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"file": file, "path": path})
}

// writeAtomic writes to a temporary file first, so a half-written image never has the real name.
func writeAtomic(dir, path string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// imageFile serves an attached image. Its name is its content's hash, so it never
// changes and can be cached for good.
func (a *api) imageFile(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if !store.ValidImageFile(file) {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(filepath.Join(a.home.ImagesDir(), file))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, ext, _ := strings.Cut(file, ".")
	for mime, e := range imageTypes {
		if e == ext {
			w.Header().Set("Content-Type", mime)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(data)
}
