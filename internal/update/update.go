// Package update finds the latest kan-do release on GitHub and installs it over this
// program: download the archive for this machine, check it against the release's
// checksums, and swap the binary in place.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/azmainadel/kan-do/internal/version"
)

// releases is the GitHub API for this repo's releases; $KANDO_RELEASES_URL points
// elsewhere, for tests.
const releases = "https://api.github.com/repos/azmainadel/kan-do"

// checkEvery is how long a check for the latest version is reused: the board checks once a day.
const checkEvery = 24 * time.Hour

var client = &http.Client{Timeout: 60 * time.Second}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var cache struct {
	sync.Mutex
	at      time.Time
	release *release
}

func base() string {
	if url := os.Getenv("KANDO_RELEASES_URL"); url != "" {
		return url
	}
	return releases
}

func get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	return io.ReadAll(res.Body)
}

func latestRelease(now bool) *release {
	cache.Lock()
	defer cache.Unlock()
	if !now && !cache.at.IsZero() && time.Since(cache.at) < checkEvery {
		return cache.release
	}
	var r *release
	if body, err := get(base() + "/releases/latest"); err == nil {
		r = new(release)
		if json.Unmarshal(body, r) != nil || r.Tag == "" {
			r = nil
		}
	}
	cache.at, cache.release = time.Now(), r
	return r
}

// Latest is the latest released version, without a leading v, checked at most once a
// day unless now. It is "" when it can't be found, like offline.
func Latest(now bool) string {
	if r := latestRelease(now); r != nil {
		return strings.TrimPrefix(r.Tag, "v")
	}
	return ""
}

// Newer reports whether v is newer than this program: 0.2.0 > 0.1.9.
func Newer(v string) bool { return compare(v, version.Version) > 0 }

// compare orders versions by their first three numbers; anything else counts as 0.
func compare(a, b string) int {
	parts := func(v string) []int {
		f := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
		out := make([]int, 3)
		for i := 0; i < 3 && i < len(f); i++ {
			out[i], _ = strconv.Atoi(f[i])
		}
		return out
	}
	return slices.Compare(parts(a), parts(b))
}

// Archive is the release file for this machine, as GoReleaser names it.
func Archive(v string) string {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	return fmt.Sprintf("kan-do_%s_%s_%s.tar.gz", v, runtime.GOOS, arch)
}

// Install puts the latest release in place of this program, and returns its path, ready
// to restart.
func Install() (string, error) {
	r := latestRelease(true)
	if r == nil {
		return "", errors.New("can't reach GitHub to find the latest release")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	name := Archive(strings.TrimPrefix(r.Tag, "v"))
	asset := func(n string) string {
		for _, a := range r.Assets {
			if a.Name == n {
				return a.URL
			}
		}
		return ""
	}
	archiveURL, sumsURL := asset(name), asset("checksums.txt")
	if archiveURL == "" || sumsURL == "" {
		return "", fmt.Errorf("release %s has no %s for this machine", r.Tag, name)
	}
	archive, err := get(archiveURL)
	if err != nil {
		return "", fmt.Errorf("cannot download %s: %w", name, err)
	}
	sums, err := get(sumsURL)
	if err != nil {
		return "", fmt.Errorf("cannot download the checksums: %w", err)
	}
	if err := verify(archive, sums, name); err != nil {
		return "", err
	}
	bin, err := extract(archive, "kan-do")
	if err != nil {
		return "", err
	}
	return exe, replace(exe, bin)
}

// verify checks data against its line in a checksums.txt of "<sha256>  <file>" lines.
func verify(data, sums []byte, name string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	lines := bufio.NewScanner(bytes.NewReader(sums))
	for lines.Scan() {
		f := strings.Fields(lines.Text())
		if len(f) == 2 && f[1] == name {
			if f[0] != got {
				return fmt.Errorf("%s does not match its checksum; not installing it", name)
			}
			return nil
		}
	}
	return fmt.Errorf("checksums.txt has no line for %s", name)
}

// extract returns the file called name from a .tar.gz archive.
func extract(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("the download is not a .tar.gz: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("the archive has no %s", name)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
}

// replace writes the new program next to the old one, then renames it over, so the old
// one stays whole if anything fails, and a running copy keeps its file.
func replace(exe string, bin []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".kan-do-update-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s (reinstall with the command in the README): %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}

// RestartArgs are the arguments to start again with: the same ones, without opening
// another browser tab.
func RestartArgs(args []string) []string {
	if slices.Contains(args, "--no-open") {
		return args
	}
	return append(slices.Clone(args), "--no-open")
}
