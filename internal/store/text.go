package store

import (
	"slices"
	"strconv"
	"strings"
)

// projectColors are the colors a project can be given; the board maps each name to a shade.
var projectColors = []string{"gray", "red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"}

var imageExts = []string{"png", "jpg", "gif", "webp"}

// ValidImageFile reports whether file is a name kan-do gives a saved image:
// 16 lowercase hex digits and its type, like 6fec521a01297ab5.png.
func ValidImageFile(file string) bool {
	hash, ext, ok := strings.Cut(file, ".")
	if !ok || len(hash) != 16 || !slices.Contains(imageExts, ext) {
		return false
	}
	for _, c := range []byte(hash) {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

func validSlug(s string) bool {
	if s == "" || len(s) > 32 || !isAlnum(s[0]) {
		return false
	}
	for _, c := range []byte(s) {
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// ShortTime turns 2026-10-25T09:30:00Z into 2026-10-25 09:30 UTC.
func ShortTime(ts string) string {
	if len(ts) < 16 {
		return ts
	}
	return ts[:10] + " " + ts[11:16] + " UTC"
}

// squash joins the words of s with single spaces: "  a   b " -> "a b".
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cleanTags turns " Bug, UI,bug ,, needs  review" into [bug ui "needs review"].
func cleanTags(input string) []string {
	tags := []string{}
	for tag := range strings.SplitSeq(input, ",") {
		tag = strings.ToLower(squash(tag))
		if tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func cleanTitle(title string) (string, error) {
	title = squash(title)
	if title == "" {
		return "", invalid("task title is empty")
	}
	return title, nil
}

func cleanRepo(repo string) (string, error) {
	r, ok := RepoFromURL(repo)
	if !ok {
		return "", invalid("repo %q should look like owner/repo", strings.TrimSpace(repo))
	}
	return r, nil
}

// parseTaskKey splits api#12, #12 or 12 into an optional project slug and the task id.
func parseTaskKey(key string) (slug string, id int64, err error) {
	key = strings.TrimSpace(key)
	num := key
	if i := strings.LastIndexByte(key, '#'); i >= 0 {
		slug = strings.ToLower(strings.TrimSpace(key[:i]))
		num = key[i+1:]
	}
	id, err = strconv.ParseInt(strings.TrimSpace(num), 10, 64)
	if err != nil {
		return "", 0, invalid("%q is not a task id; use a form like api#12 or 12", key)
	}
	return slug, id, nil
}

// RepoFromURL normalizes a GitHub repo given as owner/repo or as any git remote URL
// (https://github.com/owner/repo.git, git@github.com:owner/repo, ...) to lowercase owner/repo.
// A browser URL like https://github.com/owner/repo/tree/main gives owner/repo too.
func RepoFromURL(input string) (string, bool) {
	s := strings.TrimSpace(input)
	path, bare := s, true
	if _, rest, ok := strings.Cut(s, "://"); ok {
		// scheme://[user@]host[:port]/owner/repo...
		_, p, ok := strings.Cut(rest, "/")
		if !ok {
			return "", false
		}
		path, bare = p, false
	} else if _, rest, ok := strings.Cut(s, ":"); ok {
		// user@host:owner/repo
		path, bare = rest, false
	}
	var parts []string
	for p := range strings.SplitSeq(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 || bare && len(parts) != 2 {
		return "", false
	}
	owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
	if !repoPart(owner) || !repoPart(repo) {
		return "", false
	}
	return strings.ToLower(owner + "/" + repo), true
}

func repoPart(p string) bool {
	for _, c := range []byte(p) {
		if !isAlnum(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}
