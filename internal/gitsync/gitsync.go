// Package gitsync keeps the data folder in step with its git remote, one machine at a time.
package gitsync

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/store"
)

// Keep says which board wins when both sides changed.
type Keep int

const (
	// KeepNone asks, by returning a *Diverged error.
	KeepNone Keep = iota
	KeepLocal
	KeepRemote
)

// Diverged is the error when both sides changed the board: the caller asks which one to
// keep. First is set on the first sync with a repo that already holds a board.
type Diverged struct {
	Message string
	First   bool
}

func (d *Diverged) Error() string { return d.Message }

// git runs git in a data folder. Without prompt, git fails rather than ask for a password
// or passphrase, as the board's button needs.
type git struct {
	dir    string
	prompt bool
}

func (g git) command(args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", g.dir}, args...)...)
	if !g.prompt {
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	} else {
		cmd.Stdin = os.Stdin
	}
	return cmd
}

// run returns git's trimmed output, or fails with git's own message.
func (g git) run(args ...string) (string, error) {
	cmd := g.command(args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if g.prompt {
		// Let ssh and credential helpers ask on the terminal.
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	}
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", errors.New("cannot run git; is it installed?")
	}
	if err != nil {
		return "", fmt.Errorf("git %s failed:\n%s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (g git) ok(args ...string) bool { return g.command(args...).Run() == nil }

// RemoteURL is the URL of the repo the board syncs with, or "" before sync is set up.
func RemoteURL(h *home.Home) string {
	_, url, _ := remote(h.Dir)
	return url
}

// remote returns the remote the board syncs with, origin or the only one there is, and
// its URL; empty before sync is set up.
func remote(dir string) (name, url string, err error) {
	if _, err := os.Stat(dir + "/.git"); err != nil {
		return "", "", nil
	}
	out, err := git{dir: dir}.run("remote", "-v")
	if err != nil {
		return "", "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		n, rest, ok := strings.Cut(line, "\t")
		u, fetch := strings.CutSuffix(rest, " (fetch)")
		if !ok || !fetch {
			continue
		}
		if n == "origin" || name == "" {
			name, url = n, u
		}
		if n == "origin" {
			break
		}
	}
	return name, url, nil
}

// SetRemote points the board at the git repo url, once this machine can reach it: the
// first time, it makes the data folder a git repo; later, it replaces the old repo, and
// the next sync asks which board to keep if the new one already holds one. owner/repo is
// short for that repo on GitHub. It returns the URL saved.
func SetRemote(h *home.Home, url string, prompt bool) (string, error) {
	g := git{dir: h.Dir, prompt: prompt}
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if url == "" {
		return "", errors.New("Paste the URL of a git repository, like https://github.com/you/kando-data")
	}
	var failure, head string
	found := ""
	for _, candidate := range candidates(url) {
		out, err := g.run("ls-remote", "--symref", candidate, "HEAD")
		if err == nil {
			found, head = candidate, out
			break
		}
		failure = reason(err.Error())
	}
	if found == "" {
		return "", fmt.Errorf("Can't reach that repo. Check the URL, and that this machine can push to it.\n%s", failure)
	}
	if _, err := os.Stat(h.Dir + "/.git"); err != nil {
		// The repo's own branch, so this machine joins the board already there.
		branch := "main"
		for line := range strings.SplitSeq(head, "\n") {
			if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
				branch, _, _ = strings.Cut(ref, "\t")
				break
			}
		}
		if _, err := g.run("init", "--quiet", "--initial-branch", branch); err != nil {
			return "", err
		}
	}
	name, current, err := remote(h.Dir)
	if err != nil {
		return "", err
	}
	if current != found {
		// Removing the old remote forgets its branches, so the next sync starts afresh.
		if name != "" {
			if _, err := g.run("remote", "remove", name); err != nil {
				return "", err
			}
		}
		if _, err := g.run("remote", "add", "origin", found); err != nil {
			return "", err
		}
	}
	return found, nil
}

// reason picks the line of git's error that says what went wrong, in a few words.
func reason(msg string) string {
	if strings.Contains(msg, "terminal prompts disabled") {
		return "Git asked to sign in: sign in on this machine first (`gh auth login` for GitHub), or use the repo's SSH URL."
	}
	var lines []string
	for l := range strings.SplitSeq(msg, "\n") {
		lines = append(lines, strings.TrimSpace(l))
	}
	pick := func(prefix string) string {
		for _, l := range lines {
			if strings.HasPrefix(l, prefix) {
				return l
			}
		}
		return ""
	}
	line := ""
	for _, prefix := range []string{"ERROR:", "Permission denied", "Host key verification failed", "fatal:"} {
		if line = pick(prefix); line != "" {
			break
		}
	}
	if line == "" && len(lines) > 0 {
		line = lines[len(lines)-1]
	}
	line = strings.TrimPrefix(strings.TrimPrefix(line, "ERROR:"), "fatal:")
	return strings.TrimSpace(line)
}

// candidates are the URLs url may mean: itself, or for owner/repo that repo on GitHub, by
// SSH and then HTTPS, as either may be the one this machine can push with.
func candidates(url string) []string {
	short := strings.TrimPrefix(url, "github.com/")
	parts := 0
	for p := range strings.SplitSeq(short, "/") {
		if p != "" {
			parts++
		}
	}
	shorthand := !strings.Contains(short, ":") && parts == 2 &&
		!strings.HasPrefix(short, ".") && !strings.HasPrefix(short, "/") && !strings.HasPrefix(short, "~") &&
		!strings.ContainsAny(short, " \t\n")
	if !shorthand {
		return []string{url}
	}
	repo := strings.TrimSuffix(short, ".git")
	return []string{"git@github.com:" + repo + ".git", "https://github.com/" + repo + ".git"}
}

// Sync commits the board, then pulls or pushes it, and says what it did. With prompt
// false, as from the board's button, git never waits for a password; a missing
// credential fails with git's message instead.
func Sync(h *home.Home, keep Keep, prompt bool) (string, error) {
	g := git{dir: h.Dir, prompt: prompt}
	name, _, err := remote(h.Dir)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", errors.New("Sync isn't set up yet. Create an empty private repo, on GitHub for example, " +
			"then press the sync button on the board and paste its URL, or run:\n  kando sync --repo <url>")
	}
	if _, err := g.run("fetch", "--quiet", "--prune", name); err != nil {
		return "", err
	}

	// Wait for open connections to finish and keep new ones out while the database is
	// staged. The lock is held only while git reads or replaces kando.db, never across a
	// commit (hooks, signing prompts) or the network.
	if err := withLock(h, func() error {
		if err := settle(h); err != nil {
			return err
		}
		_, err := g.run("add", "--all")
		return err
	}); err != nil {
		return "", err
	}
	if !g.ok("diff", "--cached", "--quiet") {
		args := []string{"commit", "--quiet", "--message", "kando sync from " + hostname()}
		// A machine where git doesn't know the user yet still syncs.
		if !g.ok("config", "user.email") {
			args = append([]string{"-c", "user.name=kando", "-c", "user.email=kando@localhost"}, args...)
		}
		if _, err := g.run(args...); err != nil {
			return "", err
		}
	}

	branch, err := g.run("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	if !g.ok("rev-parse", "--verify", "--quiet", "refs/remotes/"+name+"/"+branch) {
		out, err := g.run("branch", "--remotes", "--format=%(refname:short)")
		if err != nil {
			return "", err
		}
		var others []string
		for b := range strings.SplitSeq(out, "\n") {
			if rest, ok := strings.CutPrefix(b, name+"/"); ok && rest != "HEAD" && rest != "" {
				others = append(others, rest)
			}
		}
		switch len(others) {
		case 0:
			if _, err := g.run("push", "--quiet", "--set-upstream", name, branch); err != nil {
				return "", err
			}
			return fmt.Sprintf("Pushed the board to %s/%s for the first time.", name, branch), nil
		case 1:
			// The repo's board is on another branch, like master: follow it, rather than
			// start a second board next to it.
			if _, err := g.run("branch", "--move", others[0]); err != nil {
				return "", err
			}
			branch = others[0]
		default:
			return "", fmt.Errorf("%s has several branches (%s), and none is %s, so it's not clear which holds "+
				"the board. Sync with an empty repo, or one used only by kan-do.", name, strings.Join(others, ", "), branch)
		}
	}
	upstream := name + "/" + branch
	first := !g.ok("rev-parse", "--verify", "--quiet", "@{upstream}")
	if first {
		if _, err := g.run("branch", "--quiet", "--set-upstream-to", upstream); err != nil {
			return "", err
		}
	}

	counts, err := g.run("rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return "", err
	}
	f := strings.Fields(counts)
	if len(f) != 2 {
		return "", fmt.Errorf("unexpected git output: %s", counts)
	}
	ahead, err1 := strconv.Atoi(f[0])
	behind, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("unexpected git output: %s", counts)
	}

	switch {
	case ahead == 0 && behind == 0:
		return "Already in sync with " + upstream + ".", nil
	case behind == 0:
		if _, err := g.run("push", "--quiet"); err != nil {
			return "", err
		}
		return "Pushed this machine's changes to " + upstream + ".", nil
	case ahead == 0:
		if err := withLock(h, func() error {
			_, err := g.run("merge", "--quiet", "--ff-only", "@{upstream}")
			return err
		}); err != nil {
			return "", err
		}
		return "Pulled the latest board from " + upstream + ".", migrate(h)
	case keep == KeepLocal:
		if _, err := g.run("push", "--quiet", "--force-with-lease"); err != nil {
			return "", err
		}
		return "Overwrote " + upstream + " with this machine's board.", nil
	case keep == KeepRemote:
		if err := withLock(h, func() error {
			if err := copyFile(h.DBPath(), h.Dir+"/kando.db.bak"); err != nil {
				return fmt.Errorf("cannot back up kando.db: %w", err)
			}
			_, err := g.run("reset", "--quiet", "--hard", "@{upstream}")
			return err
		}); err != nil {
			return "", err
		}
		return "Took the board from " + upstream + "; this machine's old board is in kando.db.bak.", migrate(h)
	}
	what := "This machine and " + upstream + " both changed the board since the last sync, and a database file can't be merged."
	if first {
		what = upstream + " already holds a board."
	}
	return "", &Diverged{
		Message: what + " Keep one side:\n" +
			"  kando sync --keep-remote  use the board in " + upstream + " here (this machine's is kept in kando.db.bak)\n" +
			"  kando sync --keep-local   overwrite " + upstream + " with this machine's board",
		First: first,
	}
}

// withLock runs f while no other connection to the board is open.
func withLock(h *home.Home, f func() error) error {
	lock, err := h.LockExclusive()
	if err != nil {
		return err
	}
	if lock != nil {
		defer lock.Close()
	}
	return f()
}

// settle makes kando.db complete on its own, without its journal. It opens the file
// directly: the caller holds the exclusive lock that Home.Store would wait on.
func settle(h *home.Home) error {
	s, err := store.Open(h.DBPath(), h.Config.Statuses, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Settle()
}

// migrate brings a board pulled from a newer or older machine up to this version's schema.
func migrate(h *home.Home) error {
	s, err := h.Store()
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Migrate()
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}

// hostname is the machine's name for commit messages, like "Ada's MacBook Air".
func hostname() string {
	if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}
	return "unknown"
}
