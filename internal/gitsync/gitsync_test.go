package gitsync

import (
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/azmainadel/kan-do/internal/home"
)

func TestCandidates(t *testing.T) {
	ssh := "git@github.com:you/kando-data.git"
	https := "https://github.com/you/kando-data.git"
	for url, want := range map[string][]string{
		"you/kando-data":                    {ssh, https},
		"github.com/you/kando-data.git":     {ssh, https},
		"https://github.com/you/kando-data": {"https://github.com/you/kando-data"},
		ssh:                                 {ssh},
		"/srv/git/kan-do.git":               {"/srv/git/kan-do.git"},
		"../kan-do.git":                     {"../kan-do.git"},
	} {
		if got := candidates(url); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", url, got, want)
		}
	}
}

func TestReasons(t *testing.T) {
	missing := "git ls-remote failed:\nERROR: Repository not found.\nfatal: Could not read from remote repository.\n\n" +
		"Please make sure you have the correct access rights\nand the repository exists."
	for msg, want := range map[string]string{
		missing: "Repository not found.",
		"git ls-remote failed:\nfatal: '/x' does not appear to be a git repository": "'/x' does not appear to be a git repository",
	} {
		if got := reason(msg); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if !strings.Contains(reason("fatal: could not read Username for 'https://github.com': terminal prompts disabled"), "gh auth login") {
		t.Error("a sign-in prompt should say how to sign in")
	}
}

// machine is a data folder on one of two machines that sync through the same repo.
func machine(t *testing.T) *home.Home {
	t.Helper()
	h, err := home.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func project(t *testing.T, h *home.Home, slug string) {
	t.Helper()
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateProject(slug, "", ""); err != nil {
		t.Fatal(err)
	}
}

func slugs(t *testing.T, h *home.Home) []string {
	t.Helper()
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range projects {
		out = append(out, p.Slug)
	}
	return out
}

func syncs(t *testing.T, h *home.Home, keep Keep, want string) {
	t.Helper()
	msg, err := Sync(h, keep, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, want) {
		t.Fatalf("got %q, want it to contain %q", msg, want)
	}
}

func TestSyncBetweenTwoMachines(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := filepath.Join(t.TempDir(), "board.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", "--initial-branch", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	laptop, desktop := machine(t), machine(t)

	if _, err := Sync(laptop, KeepNone, false); err == nil || !strings.Contains(err.Error(), "isn't set up") {
		t.Fatalf("sync before setup: %v", err)
	}
	if _, err := SetRemote(laptop, filepath.Join(t.TempDir(), "nope.git"), false); err == nil || !strings.Contains(err.Error(), "Can't reach") {
		t.Fatalf("unreachable repo: %v", err)
	}

	project(t, laptop, "api")
	if url, err := SetRemote(laptop, repo, false); err != nil || url != repo {
		t.Fatalf("set remote: %q %v", url, err)
	}
	if RemoteURL(laptop) != repo {
		t.Fatalf("remote %q", RemoteURL(laptop))
	}
	syncs(t, laptop, KeepNone, "for the first time")
	syncs(t, laptop, KeepNone, "Already in sync")

	// The desktop has its own board: the first sync asks which one to keep.
	project(t, desktop, "scratch")
	if _, err := SetRemote(desktop, repo, false); err != nil {
		t.Fatal(err)
	}
	_, err := Sync(desktop, KeepNone, false)
	if d, ok := errors.AsType[*Diverged](err); !ok || !d.First {
		t.Fatalf("first sync of a second board: %v", err)
	}
	syncs(t, desktop, KeepRemote, "kando.db.bak")
	if got := slugs(t, desktop); !slices.Equal(got, []string{"api"}) {
		t.Fatalf("desktop has %v", got)
	}

	// Changes on one side travel to the other.
	project(t, desktop, "web")
	syncs(t, desktop, KeepNone, "Pushed this machine's changes")
	syncs(t, laptop, KeepNone, "Pulled the latest board")
	if got := slugs(t, laptop); !slices.Equal(got, []string{"api", "web"}) {
		t.Fatalf("laptop has %v", got)
	}

	// Both sides changed: keep one.
	project(t, laptop, "docs")
	project(t, desktop, "infra")
	syncs(t, desktop, KeepNone, "Pushed")
	_, err = Sync(laptop, KeepNone, false)
	if d, ok := errors.AsType[*Diverged](err); !ok || d.First {
		t.Fatalf("both changed: %v", err)
	}
	syncs(t, laptop, KeepLocal, "Overwrote")
	syncs(t, desktop, KeepRemote, "Took the board")
	if got := slugs(t, desktop); !slices.Equal(got, []string{"api", "docs", "web"}) {
		t.Fatalf("desktop has %v", got)
	}
}
