package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var statuses = []string{"todo", "doing", "review", "done"}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "kando.db"), statuses, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s
}

// ok fails the test on err and passes v through: ok(s.GetTask(1))(t).
func ok[T any](v T, err error) func(t *testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func fails(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want one containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q, want one containing %q", err, want)
	}
}

func eq[T any](t *testing.T, got, want T) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func titles(tasks []Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.Title
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	noErr(t, s.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
	return n
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := open(t)
	noErr(t, s.Migrate())
	v, _ := metaInt(s.db, "schema_version")
	eq(t, v, len(migrations))
}

func TestKeepsOnlyTheNewestDoneTasks(t *testing.T) {
	s := open(t)
	p := ok(s.CreateProject("api", "", ""))(t)
	ok(s.CreateTask(p.ID, "open", ""))(t)
	for _, title := range []string{"d1", "d2", "d3", "d4"} {
		task := ok(s.CreateTask(p.ID, title, ""))(t)
		ok(s.MoveTask(task.ID, "done", 0))(t)
	}
	tasks, hidden, err := s.ListTasksKeepingDone(0, "", 2)
	noErr(t, err)
	eq(t, titles(tasks), []string{"open", "d3", "d4"})
	eq(t, hidden, 2)
	tasks, hidden, _ = s.ListTasksKeepingDone(p.ID, "done", 0)
	eq(t, len(tasks), 0)
	eq(t, hidden, 4)
	_, hidden, _ = s.ListTasksKeepingDone(0, "todo", 0)
	eq(t, hidden, 0)
	all, hidden, _ := s.ListTasksKeepingDone(0, "", KeepAll)
	eq(t, titles(all), []string{"open", "d1", "d2", "d3", "d4"})
	eq(t, hidden, 0)
}

func TestOpensABoardFromANewerVersionWhenItCan(t *testing.T) {
	s := open(t)
	set := func(version, compatible int) {
		ok(s.db.Exec(fmt.Sprintf(`UPDATE meta SET value = '%d' WHERE key = 'schema_version';
			UPDATE meta SET value = '%d' WHERE key = 'compatible';`, version, compatible)))(t)
	}
	newer := len(migrations) + 1
	// A newer kan-do only added things: this one goes on using the board, untouched.
	set(newer, len(migrations))
	noErr(t, s.Migrate())
	ok(s.CreateProject("api", "", ""))(t)
	v, _ := metaInt(s.db, "schema_version")
	eq(t, v, newer)
	// A change this version can't live with: it says how to update.
	set(newer, newer)
	fails(t, s.Migrate(), "kando update")
}

func TestProjects(t *testing.T) {
	s := open(t)
	p := ok(s.CreateProject("API", "API server", "https://github.com/Me/Api.git"))(t)
	eq(t, p.Slug, "api")
	eq(t, p.Repos, []string{"me/api"})
	eq(t, ok(s.FindProject("me/api"))(t).ID, p.ID)
	eq(t, ok(s.ProjectByRepo("git@github.com:me/api.git"))(t).ID, p.ID)
	_, err := s.CreateProject("api", "", "")
	fails(t, err, "already exists")
	_, err = s.CreateProject("web", "", "me/api")
	fails(t, err, "already linked")
	_, err = s.CreateProject("bad slug", "", "")
	fails(t, err, "must be")
	web := ok(s.CreateProject("web", "", ""))(t)
	eq(t, web.Name, "web")
	if web.Color != nil {
		t.Fatalf("color %q, want none", *web.Color)
	}
	eq(t, *ok(s.SetProjectColor("web", "teal"))(t).Color, "teal")
	_, err = s.SetProjectColor("web", "#ff0000")
	fails(t, err, "unknown color")
	renamed := ok(s.RenameProject("web", "  Web   site "))(t)
	eq(t, renamed.Slug, "web")
	eq(t, renamed.Name, "Web site")
	_, err = s.RenameProject("web", "  ")
	fails(t, err, "empty")
	_, err = s.FindProject("nope")
	fails(t, err, "api, web")
	eq(t, KindOf(err), NotFound)
}

func TestManyReposPerProject(t *testing.T) {
	s := open(t)
	api := ok(s.CreateProject("api", "", "me/api"))(t)
	ok(s.CreateProject("web", "", ""))(t)
	eq(t, ok(s.LinkRepo("api", "https://github.com/Me/API-docs.git"))(t).Repos, []string{"me/api", "me/api-docs"})
	eq(t, ok(s.FindProject("me/api-docs"))(t).ID, api.ID)
	eq(t, len(ok(s.LinkRepo("api", "me/api"))(t).Repos), 2) // linking twice changes nothing
	_, err := s.LinkRepo("web", "me/api-docs")
	fails(t, err, "already linked to project api")
	_, err = s.LinkRepo("api", "not a repo")
	fails(t, err, "owner/repo")
	eq(t, ok(s.UnlinkRepo("api", "me/api"))(t).Repos, []string{"me/api-docs"})
	if p := ok(s.ProjectByRepo("me/api"))(t); p != nil {
		t.Fatalf("me/api still linked to %s", p.Slug)
	}
	_, err = s.UnlinkRepo("api", "me/api")
	fails(t, err, "not linked")
	// A deleted project frees its repos.
	ok(s.DeleteProject("api"))(t)
	eq(t, ok(s.LinkRepo("web", "me/api-docs"))(t).Repos, []string{"me/api-docs"})
}

func TestSearch(t *testing.T) {
	s := open(t)
	api := ok(s.CreateProject("api", "", ""))(t)
	web := ok(s.CreateProject("web", "", ""))(t)
	login := ok(s.CreateTask(api.ID, "Fix login", "Users loop on /login"))(t)
	old := ok(s.CreateTask(web.ID, "Login page copy", ""))(t)
	ok(s.UpdateTask(old.ID, TaskPatch{Status: ptr("done")}))(t)
	other := ok(s.CreateTask(web.ID, "Pricing 100%", ""))(t)
	ok(s.UpdateTask(other.ID, TaskPatch{Tags: ptr("ui")}))(t)
	search := func(q string) []string { return titles(ok(s.Search(q, 10))(t)) }
	eq(t, search("LOGIN"), []string{"Fix login", "Login page copy"}) // any project, open first, done too
	eq(t, search("login api"), []string{"Fix login"})                // every word must match, the project too
	eq(t, search("web#"+strconv.FormatInt(other.ID, 10)), []string{"Pricing 100%"})
	eq(t, search("ui"), []string{"Pricing 100%"})
	eq(t, search("100%"), []string{"Pricing 100%"}) // % is a plain character
	eq(t, len(search("nothing-like-this")), 0)
	eq(t, len(search("")), 3) // no words: the latest changes
	eq(t, ok(s.Search("login", 1))(t)[0].ID, login.ID)
}

func TestImages(t *testing.T) {
	s := open(t)
	p := ok(s.CreateProject("api", "", ""))(t)
	task := ok(s.CreateTask(p.ID, "Bug", ""))(t)
	shot := "0123456789abcdef.png"
	onTask := ok(s.AddImage(task.ID, shot))(t)
	_, err := s.AddImage(task.ID, "../../etc/passwd")
	fails(t, err, "not an image")
	_, err = s.AddImage(task.ID+1, shot)
	fails(t, err, "no task")
	// A comment can be only images.
	c := ok(s.AddComment(task.ID, "me", "", []string{"fedcba9876543210.jpg", shot}))(t)
	_, err = s.AddComment(task.ID, "me", "x", []string{"evil.svg"})
	fails(t, err, "not an image")
	var all []string
	for _, i := range ok(s.ListImages(task.ID))(t) {
		owner := "task"
		if i.CommentID != nil {
			owner = "comment" + strconv.FormatInt(*i.CommentID, 10)
		}
		all = append(all, owner+":"+i.File)
	}
	cid := "comment" + strconv.FormatInt(c.ID, 10)
	eq(t, all, []string{"task:" + shot, cid + ":fedcba9876543210.jpg", cid + ":" + shot})
	eq(t, ok(s.ImageFiles(OfTask, task.ID))(t), []string{shot, "fedcba9876543210.jpg"})
	eq(t, len(ok(s.ImageFiles(OfProject, p.ID))(t)), 2)
	// The same file twice: removing one attachment leaves it in use.
	ok(s.DeleteImage(onTask.ID))(t)
	eq(t, len(ok(s.UnusedImages([]string{shot}))(t)), 0)
	files := ok(s.ImageFiles(OfComment, c.ID))(t)
	ok(s.DeleteComment(c.ID))(t)
	eq(t, len(ok(s.ListImages(task.ID))(t)), 0)
	eq(t, ok(s.UnusedImages(files))(t), []string{"fedcba9876543210.jpg", shot})
	if !ValidImageFile("6fec521a01297ab5.webp") || ValidImageFile("6FEC521A01297AB5.png") || ValidImageFile("6fec521a01297ab5.svg") {
		t.Fatal("ValidImageFile")
	}
}

func TestMigrationMovesReposToTheirOwnTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kando.db")
	// A board at schema version 3, from before projects could have several repos.
	db := ok(sql.Open("sqlite", path))(t)
	ok(db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`))(t)
	for _, m := range migrations[:3] {
		ok(db.Exec(m))(t)
	}
	ok(db.Exec(`INSERT INTO meta VALUES ('schema_version', '3');
		INSERT INTO projects (slug, name, repo) VALUES ('api', 'api', 'me/api'), ('web', 'web', NULL);`))(t)
	db.Close()

	s := ok(Open(path, statuses, nil))(t)
	defer s.Close()
	noErr(t, s.Migrate())
	eq(t, ok(s.FindProject("api"))(t).Repos, []string{"me/api"})
	eq(t, len(ok(s.FindProject("web"))(t).Repos), 0)
	eq(t, ok(s.FindProject("me/api"))(t).Slug, "api")
}

func TestTaskLifecycle(t *testing.T) {
	s := open(t)
	p := ok(s.CreateProject("api", "API", ""))(t)
	task := ok(s.CreateTask(p.ID, "  Fix   login ", "Steps:\n1. log in\n"))(t)
	eq(t, task.Title, "Fix login")
	eq(t, task.Status, "todo")
	eq(t, task.Description, "Steps:\n1. log in")
	eq(t, task.Key(), fmt.Sprintf("api#%d", task.ID))
	eq(t, ok(s.FindTask(task.Key()))(t).ID, task.ID)
	_, err := s.FindTask(fmt.Sprintf("web#%d", task.ID))
	fails(t, err, "no task")

	task = ok(s.UpdateTask(task.ID, TaskPatch{Status: ptr("review"), Title: ptr("Fix login bug")}))(t)
	eq(t, task.Title, "Fix login bug")
	eq(t, task.Status, "review")
	_, err = s.UpdateTask(task.ID, TaskPatch{Status: ptr("nope")})
	fails(t, err, "unknown status")
	_, err = s.UpdateTask(task.ID, TaskPatch{Title: ptr("  ")})
	fails(t, err, "title is empty")
	web := ok(s.CreateProject("web", "", ""))(t)
	moved := ok(s.UpdateTask(task.ID, TaskPatch{ProjectID: &web.ID}))(t)
	eq(t, moved.Key(), fmt.Sprintf("web#%d", task.ID))
	eq(t, moved.Status, "review")
	task = ok(s.UpdateTask(task.ID, TaskPatch{ProjectID: &p.ID}))(t)

	ok(s.AddComment(task.ID, "claude-code", "Done, see PR", nil))(t)
	ok(s.AddComment(task.ID, "", "thanks", nil))(t)
	var authors []string
	for _, c := range ok(s.ListComments(task.ID))(t) {
		authors = append(authors, c.Author)
	}
	eq(t, authors, []string{"claude-code", "me"})
	_, err = s.AddComment(task.ID, "me", " ", nil)
	fails(t, err, "empty")
	eq(t, ok(s.GetTask(task.ID))(t).Comments, 2)
	eq(t, ok(s.UpdateTask(task.ID, TaskPatch{Tags: ptr(" Bug, UI,bug ,, needs  review")}))(t).Tags,
		[]string{"bug", "ui", "needs review"})
	eq(t, len(ok(s.UpdateTask(task.ID, TaskPatch{Tags: ptr(" , ")}))(t).Tags), 0)
	gone := ok(s.AddComment(task.ID, "me", "typo", nil))(t)
	eq(t, ok(s.DeleteComment(gone.ID))(t).Body, "typo")
	_, err = s.DeleteComment(gone.ID)
	fails(t, err, "no comment")
	eq(t, ok(s.GetTask(task.ID))(t).Comments, 2)
	eq(t, ok(s.FindProject("api"))(t).Open, 1)

	ok(s.DeleteTask(task.ID))(t)
	_, err = s.GetTask(task.ID)
	fails(t, err, "no task")
	eq(t, count(t, s, "comments"), 0)
	// Ids are never reused, so api#12 always means the same task.
	if t2 := ok(s.CreateTask(p.ID, "Another", ""))(t); t2.ID <= task.ID {
		t.Fatalf("id %d reused after %d", t2.ID, task.ID)
	}
}

func TestDeleteProjectRemovesItsTasksAndComments(t *testing.T) {
	s := open(t)
	api := ok(s.CreateProject("api", "", "me/api"))(t)
	web := ok(s.CreateProject("web", "", ""))(t)
	task := ok(s.CreateTask(api.ID, "a", ""))(t)
	ok(s.AddComment(task.ID, "me", "hi", nil))(t)
	kept := ok(s.CreateTask(web.ID, "b", ""))(t)
	ok(s.AddComment(kept.ID, "me", "hello", nil))(t)

	eq(t, ok(s.DeleteProject("api"))(t).ID, api.ID)
	_, err := s.FindProject("api")
	fails(t, err, "no project")
	_, err = s.GetTask(task.ID)
	fails(t, err, "no task")
	eq(t, []int{count(t, s, "projects"), count(t, s, "tasks"), count(t, s, "comments")}, []int{1, 1, 1})
	_, err = s.DeleteProject("api")
	fails(t, err, "no project")
	// The slug and repo are free again.
	ok(s.CreateProject("api", "", "me/api"))(t)
}

func TestOrderingAndMoves(t *testing.T) {
	s := open(t)
	p := ok(s.CreateProject("api", "", ""))(t)
	a := ok(s.CreateTask(p.ID, "a", ""))(t)
	b := ok(s.CreateTask(p.ID, "b", ""))(t)
	c := ok(s.CreateTask(p.ID, "c", ""))(t)
	eq(t, titles(ok(s.ListTasks(p.ID, ""))(t)), []string{"a", "b", "c"})

	ok(s.MoveTask(c.ID, "todo", a.ID))(t)
	eq(t, titles(ok(s.ListTasks(0, "todo"))(t)), []string{"c", "a", "b"})
	ok(s.MoveTask(c.ID, "todo", 0))(t)
	eq(t, titles(ok(s.ListTasks(0, "todo"))(t)), []string{"a", "b", "c"})
	ok(s.MoveTask(a.ID, "doing", 0))(t)
	ok(s.MoveTask(b.ID, "doing", a.ID))(t)
	eq(t, titles(ok(s.ListTasks(0, ""))(t)), []string{"c", "b", "a"})

	// Many inserts at the same spot run out of float room and trigger a renumber.
	moving := [2]int64{b.ID, c.ID}
	for range 80 {
		ok(s.MoveTask(moving[0], "doing", a.ID))(t)
		moving[0], moving[1] = moving[1], moving[0]
	}
	doing := titles(ok(s.ListTasks(0, "doing"))(t))
	eq(t, doing[len(doing)-1], "a")
	_, err := s.MoveTask(a.ID, "nope", 0)
	fails(t, err, "unknown status")
}

func TestNextTaskTakesTopTodo(t *testing.T) {
	s := open(t)
	api := ok(s.CreateProject("api", "", ""))(t)
	web := ok(s.CreateProject("web", "", ""))(t)
	ok(s.CreateTask(web.ID, "web first", ""))(t)
	// At the top of api's todo, but for a person: agents never get it.
	human := ok(s.CreateTask(api.ID, "Call the bank", ""))(t)
	eq(t, ok(s.UpdateTask(human.ID, TaskPatch{Human: ptr(true)}))(t).Human, true)
	ok(s.CreateTask(api.ID, "api first", ""))(t)
	ok(s.CreateTask(api.ID, "api second", ""))(t)

	next := ok(s.NextTask(api.ID))(t)
	eq(t, next.Title, "api first")
	eq(t, next.Status, "doing")
	eq(t, ok(s.NextTask(0))(t).Title, "web first")
	eq(t, ok(s.NextTask(api.ID))(t).Title, "api second")
	if next := ok(s.NextTask(api.ID))(t); next != nil {
		t.Fatalf("got %s, want nothing", next.Title)
	}
	eq(t, ok(s.GetTask(human.ID))(t).Status, "todo")
}

func TestChangeCounterMovesOnEveryWrite(t *testing.T) {
	s := open(t)
	c0 := ok(s.Changes())(t)
	p := ok(s.CreateProject("api", "", ""))(t)
	c1 := ok(s.Changes())(t)
	task := ok(s.CreateTask(p.ID, "x", ""))(t)
	c2 := ok(s.Changes())(t)
	ok(s.AddComment(task.ID, "me", "hi", nil))(t)
	c3 := ok(s.Changes())(t)
	if !(c0 < c1 && c1 < c2 && c2 < c3) {
		t.Fatalf("changes %d %d %d %d", c0, c1, c2, c3)
	}
}

func TestTaskKeys(t *testing.T) {
	for key, want := range map[string]string{"api#12": "api 12", "#12": " 12", " 12 ": " 12", "API #7": "api 7"} {
		slug, id, err := parseTaskKey(key)
		noErr(t, err)
		eq(t, fmt.Sprintf("%s %d", slug, id), want)
	}
	_, _, err := parseTaskKey("api#x")
	fails(t, err, "not a task id")
}

func TestRepos(t *testing.T) {
	for _, url := range []string{
		"owner/repo",
		"Owner/Repo",
		"https://github.com/owner/repo",
		"https://github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"git@github-work:owner/repo.git",
		"https://github.com/owner/repo/",
		"https://github.com/owner/repo/tree/main",
		"https://github.com/owner/repo/pull/3",
	} {
		if r, ok := RepoFromURL(url); !ok || r != "owner/repo" {
			t.Errorf("%s: got %q", url, r)
		}
	}
	for _, bad := range []string{"repo", "a/b/c", "own er/repo"} {
		if r, ok := RepoFromURL(bad); ok {
			t.Errorf("%s: got %q, want none", bad, r)
		}
	}
}

func TestCleanTags(t *testing.T) {
	eq(t, cleanTags(" Bug, UI,bug ,, needs  review"), []string{"bug", "ui", "needs review"})
	if !slices.Equal(cleanTags(""), []string{}) {
		t.Fatal("empty input gives no tags")
	}
}
