package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/store"
)

// connect runs the tools for an agent in a folder whose git remote is repos[0], and
// returns a client session named like Claude Code.
func connect(t *testing.T, cwd string, repos ...string) (*sdk.ClientSession, *home.Home) {
	t.Helper()
	h, err := home.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	server := (&board{home: h, cwd: cwd, repos: repos}).server()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session, h
}

// call runs a tool and returns its text, with "error: " in front for a tool error.
func call(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var texts []string
	for _, c := range res.Content {
		texts = append(texts, c.(*sdk.TextContent).Text)
	}
	text := strings.Join(texts, "\n")
	if res.IsError {
		return "error: " + text
	}
	return text
}

func expect(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func expectMatch(t *testing.T, got, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(got) {
		t.Fatalf("got:\n%s\nwant a match for:\n%s", got, pattern)
	}
}

func TestToolsForAnAgentInARepo(t *testing.T) {
	s, _ := connect(t, "/work/api", "me/api")

	expect(t, call(t, s, "list_projects", nil), "No projects yet. Create one with create_project.")
	expect(t, call(t, s, "create_task", map[string]any{"title": "x"}),
		`error: No project is linked to me/api (the git remote of /work/api), and there are no projects yet. Create one with create_project and repo "me/api".`)
	expect(t, call(t, s, "create_project", map[string]any{"slug": "api", "name": "API", "repo": "me/api"}),
		"Created project api (API), linked to me/api.")
	call(t, s, "create_project", map[string]any{"slug": "web"})
	expect(t, call(t, s, "link_repo", map[string]any{"project": "api", "repo": "me/api-docs"}),
		"Project api is linked to me/api, me/api-docs.")
	expect(t, call(t, s, "list_projects", nil),
		"api \"API\" (me/api, me/api-docs): 0 open of 0 <- current repo\nweb: 0 open of 0")

	expect(t, call(t, s, "create_task", map[string]any{"title": "Fix login", "tags": "Bug, UI"}), "Created api#1 in todo: Fix login")
	expect(t, call(t, s, "create_task", map[string]any{"title": "Pricing", "project": "web"}), "Created web#2 in todo: Pricing")
	call(t, s, "create_task", map[string]any{"title": "Docs"})
	expect(t, call(t, s, "list_tasks", nil), "api#1 [todo] Fix login {bug, ui}\napi#3 [todo] Docs")
	expect(t, call(t, s, "list_tasks", map[string]any{"project": "all"}),
		"api#1 [todo] Fix login {bug, ui}\nweb#2 [todo] Pricing\napi#3 [todo] Docs")
	expect(t, call(t, s, "list_tasks", map[string]any{"status": "nope"}),
		`error: unknown status "nope"; use one of: todo, doing, review, done, or all`)

	next := call(t, s, "next_task", nil)
	expectMatch(t, next, `^Moved to doing:\napi#1 Fix login \{bug, ui\}\nstatus: doing, created \d{4}-\d\d-\d\d \d\d:\d\d UTC$`)

	// A bare number works as a task id too.
	expect(t, call(t, s, "add_comment", map[string]any{"id": 1, "body": "Working on it"}), "Comment added to api#1 as claude-code.")
	expect(t, call(t, s, "add_comment", map[string]any{"id": "api#1", "body": "Done", "author": "codex"}), "Comment added to api#1 as codex.")
	expect(t, call(t, s, "update_task", map[string]any{"id": "api#1", "status": " Review "}), "Updated api#1, doing -> review: Fix login")
	expect(t, call(t, s, "update_task", map[string]any{"id": "web#1"}), "error: no task web#1 (#1 is api#1)")

	detail := call(t, s, "get_task", map[string]any{"id": "1"})
	expectMatch(t, detail, `(?s)^api#1 Fix login \{bug, ui\}\nstatus: review, created .* UTC\n\nComments \(2\):\n- claude-code, .* UTC: Working on it\n- codex, .* UTC: Done$`)

	expect(t, call(t, s, "list_tasks", map[string]any{"status": "review"}), "api#1 [review] Fix login {bug, ui} (2 comments)")
	expect(t, call(t, s, "delete_task", map[string]any{"id": "api#1"}), "Deleted api#1 and its 2 comments: Fix login")
	// Arguments run left to right: next_task moves the task before get_task shows it.
	expect(t, call(t, s, "next_task", map[string]any{"project": "web"}), "Moved to doing:\n"+call(t, s, "get_task", map[string]any{"id": "web#2"}))
	expect(t, call(t, s, "next_task", map[string]any{"project": "all"}), "Moved to doing:\n"+call(t, s, "get_task", map[string]any{"id": 3}))
	expect(t, call(t, s, "next_task", nil), "Nothing in todo for api.")
}

func TestHiddenAndHumanTasks(t *testing.T) {
	s, h := connect(t, "/work/api", "me/api")
	call(t, s, "create_project", map[string]any{"slug": "api", "repo": "me/api"})
	st, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	human, _ := st.CreateTask(1, "Call the bank", "")
	yes := true
	st.UpdateTask(human.ID, store.TaskPatch{Human: &yes})
	for i := range 22 {
		task, _ := st.CreateTask(1, fmt.Sprintf("d%d", i), "")
		st.MoveTask(task.ID, "done", 0)
	}
	expect(t, call(t, s, "list_tasks", nil),
		"No tasks in api.\n(1 human task not shown: for the user, not for agents)\n(22 done not shown; pass status \"done\" or \"all\")")
	got := call(t, s, "list_tasks", map[string]any{"status": "done"})
	lines := strings.Split(got, "\n")
	if len(lines) != 21 || lines[20] != "(2 older done not shown)" {
		t.Fatalf("got:\n%s", got)
	}
	expect(t, call(t, s, "next_task", nil), "Nothing in todo for api.")
	detail := call(t, s, "get_task", map[string]any{"id": human.ID})
	if !strings.Contains(detail, "status: todo, human: for the user, not for agents") {
		t.Fatalf("got:\n%s", detail)
	}
}

func TestProjectHints(t *testing.T) {
	s, _ := connect(t, "/work/site", "me/site")
	call(t, s, "create_project", map[string]any{"slug": "api"})
	expect(t, call(t, s, "list_tasks", nil),
		`error: No project is linked to me/site (the git remote of /work/site). Pass `+"`project`"+` (one of: api), link this repo to one with link_repo, or create a project for it with create_project and repo "me/site".`)

	noRemote, _ := connect(t, "/tmp/scratch")
	call(t, noRemote, "create_project", map[string]any{"slug": "api"})
	expect(t, call(t, noRemote, "list_tasks", nil), "error: Cannot tell the project: /tmp/scratch has no git remote. Pass `project` (one of: api).")

	overHTTP, _ := connect(t, "")
	call(t, overHTTP, "create_project", map[string]any{"slug": "api"})
	expect(t, call(t, overHTTP, "list_tasks", nil), "error: Pass `project` (one of: api); over HTTP kan-do cannot see which folder you work in.")
}

func TestImagePathsInTaskDetail(t *testing.T) {
	s, h := connect(t, "/work/api", "me/api")
	call(t, s, "create_project", map[string]any{"slug": "api", "repo": "me/api"})
	call(t, s, "create_task", map[string]any{"title": "Bug"})
	st, _ := h.Store()
	st.AddImage(1, "0123456789abcdef.png")
	st.AddComment(1, "me", "", []string{"fedcba9876543210.jpg"})
	st.Close()
	detail := call(t, s, "get_task", map[string]any{"id": "api#1"})
	dir := h.ImagesDir()
	if !strings.Contains(detail, "\nImages: "+filepath.Join(dir, "0123456789abcdef.png")+"\n") ||
		!strings.HasSuffix(detail, "UTC: [images: "+filepath.Join(dir, "fedcba9876543210.jpg")+"]") {
		t.Fatalf("got:\n%s", detail)
	}
}

func TestToolListIsSmall(t *testing.T) {
	s, _ := connect(t, "/work/api")
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 10 {
		t.Fatalf("%d tools", len(res.Tools))
	}
	data, _ := json.Marshal(res.Tools)
	if strings.Contains(string(data), "$schema") {
		t.Fatal("tool schemas carry $schema")
	}
	// About 900 tokens; a rough 4 bytes per token.
	if len(data) > 5000 {
		t.Errorf("tool list is %d bytes", len(data))
	}
	for _, tool := range res.Tools {
		if tool.Name == "update_task" {
			schema, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(schema), `"required":["id"]`) {
				t.Errorf("update_task schema: %s", schema)
			}
		}
	}
}
