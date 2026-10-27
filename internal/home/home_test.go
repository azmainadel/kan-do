package home

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/azmainadel/kan-do/internal/store"
)

func TestFirstOpenCreatesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	h, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"config.toml", ".gitignore", "kando.db"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	c := h.Config
	if c.Port != DefaultPort || !slices.Equal(c.Statuses, []string{"todo", "doing", "review", "done"}) ||
		!c.MCPHTTP || !c.Listen.IsLoopback() {
		t.Fatalf("config %+v", c)
	}
}

func TestRejectsBadConfig(t *testing.T) {
	for config, want := range map[string]string{
		`statuses = ["todo", "todo"]`:  "listed twice",
		`statuses = ["todo"]`:          "at least two",
		`statuses = ["To Do", "done"]`: "lowercase",
		`colour = "red"`:               "unknown setting colour",
		`listen = "not an address"`:    "not valid",
		`port = "4242"`:                "not valid",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Open(dir)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error containing %q", config, err, want)
		}
	}
}

func TestReadsConfig(t *testing.T) {
	dir := t.TempDir()
	config := "port = 5000\nlisten = \"0.0.0.0\"\nstatuses = [\"backlog\", \"doing\", \"shipped\"]\nmcp_http = false\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := h.Config
	if c.Port != 5000 || c.Listen.String() != "0.0.0.0" || c.MCPHTTP || c.Statuses[2] != "shipped" {
		t.Fatalf("config %+v", c)
	}
}

func TestSyncLockMakesStoreBusy(t *testing.T) {
	h, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lock, err := h.LockExclusive()
	if err != nil || lock == nil {
		t.Skipf("no file locks here: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		lock.Close()
		close(released)
	}()
	// Waits for the lock instead of failing.
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	<-released
	s.Close()
}

func TestDeletingImagesKeepsFilesStillInUse(t *testing.T) {
	h, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := os.MkdirAll(h.ImagesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	shared, own := "0123456789abcdef.png", "fedcba9876543210.png"
	for _, f := range []string{shared, own} {
		if err := os.WriteFile(filepath.Join(h.ImagesDir(), f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := s.CreateProject("api", "", "")
	a, _ := s.CreateTask(p.ID, "a", "")
	b, _ := s.CreateTask(p.ID, "b", "")
	s.AddImage(a.ID, shared)
	s.AddImage(a.ID, own)
	s.AddImage(b.ID, shared)

	if _, err := DeletingImages(h, s, store.OfTask, a.ID, func() (*store.Task, error) { return s.DeleteTask(a.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h.ImagesDir(), own)); !os.IsNotExist(err) {
		t.Errorf("%s should be deleted", own)
	}
	if _, err := os.Stat(filepath.Join(h.ImagesDir(), shared)); err != nil {
		t.Errorf("%s is still used by task b: %v", shared, err)
	}
}
