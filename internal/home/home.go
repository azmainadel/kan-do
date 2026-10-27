// Package home is the data folder: where it is, what it holds, and the settings in config.toml.
package home

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/azmainadel/kan-do/internal/store"
)

const (
	DefaultPort = 4242
	// EnvHome names the data folder when --home isn't given.
	EnvHome = "KANDO_HOME"
	dbFile  = "kan-do.db"
	// lockWait is how long an operation waits for sync before giving up with "busy".
	lockWait = 10 * time.Second
)

const configTemplate = `# kan-do settings.

# Port for the board at http://localhost:<port>.
port = 4242

# Address the board listens on. 127.0.0.1 keeps it on this machine. Use
# "0.0.0.0" to open it from other devices, for example when kan-do runs on a
# Raspberry Pi with no screen. There are no logins: anyone who can reach the
# port can read and change the board, and use its MCP endpoint.
listen = "127.0.0.1"

# Board columns, left to right. New tasks go in the first one;
# tasks in the last one count as done.
statuses = ["todo", "doing", "review", "done"]

# Also serve MCP over HTTP at http://localhost:<port>/mcp, for agents
# that cannot start ` + "`kan-do mcp`" + ` themselves.
mcp_http = true
`

const gitignore = `# SQLite temporary files; kan-do sync makes sure kan-do.db is complete without them.
kan-do.db-journal
kan-do.db-wal
kan-do.db-shm
# Local to this machine.
kan-do.lock
*.bak
# macOS metadata (AppleDouble files appear on exFAT drives).
._*
.DS_Store
`

// Config is config.toml.
type Config struct {
	Port     uint16     `toml:"port"`
	Listen   netip.Addr `toml:"listen"`
	Statuses []string   `toml:"statuses"`
	MCPHTTP  bool       `toml:"mcp_http"`
}

// DefaultConfig is what a missing setting falls back to.
func DefaultConfig() Config {
	return Config{
		Port:     DefaultPort,
		Listen:   netip.MustParseAddr("127.0.0.1"),
		Statuses: []string{"todo", "doing", "review", "done"},
		MCPHTTP:  true,
	}
}

func (c *Config) validate() error {
	if len(c.Statuses) < 2 {
		return errors.New("config.toml: `statuses` needs at least two columns")
	}
	for i, s := range c.Statuses {
		valid := s != ""
		for _, ch := range []byte(s) {
			valid = valid && ('a' <= ch && ch <= 'z' || '0' <= ch && ch <= '9' || ch == '-' || ch == '_')
		}
		if !valid {
			return fmt.Errorf("config.toml: status %q must be lowercase letters, digits, - or _", s)
		}
		if slices.Contains(c.Statuses[:i], s) {
			return fmt.Errorf("config.toml: status %q is listed twice", s)
		}
	}
	return nil
}

// Home is an opened data folder.
type Home struct {
	Dir    string
	Config Config
}

// Resolve picks the data folder: flag, then $KANDO_HOME, then ~/kan-do.
func Resolve(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if dir := os.Getenv(EnvHome); dir != "" {
		return dir, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your home folder; set %s", EnvHome)
	}
	return filepath.Join(h, "kan-do"), nil
}

// Open opens the data folder, creating it and its files on first use, and brings the
// database schema up to date.
func Open(dir string) (*Home, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if err := writeIfMissing(filepath.Join(dir, "config.toml"), configTemplate); err != nil {
		return nil, err
	}
	if err := writeIfMissing(filepath.Join(dir, ".gitignore"), gitignore); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "config.toml")
	config := DefaultConfig()
	meta, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid: %w", path, err)
	}
	if unknown := meta.Undecoded(); len(unknown) > 0 {
		keys := make([]string, len(unknown))
		for i, k := range unknown {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s is not valid: unknown setting %s", path, strings.Join(keys, ", "))
	}
	if err := config.validate(); err != nil {
		return nil, err
	}

	h := &Home{Dir: dir, Config: config}
	s, err := h.Store()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if err := s.Migrate(); err != nil {
		return nil, err
	}
	return h, nil
}

// DBPath is the board's database file.
func (h *Home) DBPath() string { return filepath.Join(h.Dir, dbFile) }

// ImagesDir holds attached images.
func (h *Home) ImagesDir() string { return filepath.Join(h.Dir, "images") }

// Store opens a fresh connection. Callers keep it for one operation only, so a database
// replaced by sync is picked up on the next one. It holds a shared lock on kan-do.lock,
// so sync can wait for it to finish; while sync holds the lock, this waits up to
// lockWait and then reports busy.
func (h *Home) Store() (*store.Store, error) {
	lock, err := h.lockFile()
	if err != nil {
		return nil, &store.Error{Kind: store.Invalid, Msg: err.Error()}
	}
	deadline := time.Now().Add(lockWait)
	for {
		held, err := tryLockShared(lock)
		if err != nil {
			// Some filesystems (exFAT, network shares) have no locks; go on without.
			lock.Close()
			lock = nil
			break
		}
		if held {
			break
		}
		if time.Now().After(deadline) {
			lock.Close()
			return nil, store.BusyError("kan-do sync is updating the board; try again in a moment")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var closer interface{ Close() error }
	if lock != nil {
		closer = lock
	}
	s, err := store.Open(h.DBPath(), h.Config.Statuses, closer)
	if err != nil && lock != nil {
		lock.Close()
	}
	return s, err
}

// LockExclusive blocks until no connection is open, and keeps new ones out until the
// returned file is closed. It returns nil when the filesystem has no locks.
func (h *Home) LockExclusive() (*os.File, error) {
	lock, err := h.lockFile()
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(lock); err != nil {
		lock.Close()
		return nil, nil
	}
	return lock, nil
}

func (h *Home) lockFile() (*os.File, error) {
	path := filepath.Join(h.Dir, "kan-do.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	return f, nil
}

// DeletingImages runs del, then deletes the image files of the owner that nothing uses
// any more: the same image can be attached in several places, and is stored once.
func DeletingImages[T any](h *Home, s *store.Store, of store.Owner, id int64, del func() (T, error)) (T, error) {
	files, err := s.ImageFiles(of, id)
	if err != nil {
		var zero T
		return zero, err
	}
	deleted, err := del()
	if err != nil {
		return deleted, err
	}
	unused, err := s.UnusedImages(files)
	if err != nil {
		return deleted, err
	}
	for _, f := range unused {
		os.Remove(filepath.Join(h.ImagesDir(), f))
	}
	return deleted, nil
}

func writeIfMissing(path, contents string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}
