// Package mcp serves the board as MCP tools: over stdio for `kando mcp`, and over HTTP
// at /mcp of the board. Replies are short plain text to save agents' tokens.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/store"
	"github.com/azmainadel/kan-do/internal/version"
)

// doneListed is the most done tasks list_tasks shows, newest first: an old board has thousands.
const doneListed = 20

const instructions = "kan-do is the user's kanban board, shared by all their projects. " +
	"Tasks are written project#id, like api#12. When you leave out `project`, kan-do uses the project " +
	"linked to the GitHub repo of the folder you are working in. A good loop: next_task to pick up work, " +
	"add_comment to report what you did, then update_task to move it to review. Tag tasks when it helps, like " +
	"bug or ui: create_task and update_task take `tags`. Leave tasks marked human alone: they're for the user."

const httpInstructions = "kan-do is the user's kanban board, shared by all their projects. " +
	"Tasks are written project#id, like api#12. Over HTTP kan-do cannot see which folder you work in, so " +
	"pass `project` (see list_projects). A good loop: next_task to pick up work, add_comment to report " +
	"what you did, then update_task to move it to review. Tag tasks when it helps, like " +
	"bug or ui: create_task and update_task take `tags`. Leave tasks marked human alone: they're for the user."

// ServeStdio runs the MCP server for one agent on stdin and stdout, in the folder it
// started us in.
func ServeStdio(ctx context.Context, h *home.Home) error {
	cwd, _ := os.Getwd()
	b := &board{home: h, cwd: cwd, repos: gitRepos(cwd)}
	return b.server().Run(ctx, &sdk.StdioTransport{})
}

// HTTPHandler serves MCP over HTTP. The board's own Host and Origin checks guard it,
// including when it listens on the network, so the SDK's localhost-only check is off.
func HTTPHandler(h *home.Home) http.Handler {
	server := (&board{home: h}).server()
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
		SessionTimeout:             time.Hour,
	})
}

// board is what the tools work on.
type board struct {
	home *home.Home
	// cwd is the agent's working folder; empty over HTTP, where we cannot know it.
	cwd string
	// repos are the GitHub repos (owner/repo) of the git remotes of cwd.
	repos []string
}

// project returns the named project, or the one linked to this folder's git remote.
func (b *board) project(s *store.Store, name string) (*store.Project, error) {
	if name = strings.TrimSpace(name); name != "" {
		return s.FindProject(name)
	}
	for _, repo := range b.repos {
		p, err := s.ProjectByRepo(repo)
		if err != nil || p != nil {
			return p, err
		}
	}
	projects, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(projects))
	for i, p := range projects {
		slugs[i] = p.Slug
	}
	known := strings.Join(slugs, ", ")
	fail := func(format string, a ...any) error {
		return &store.Error{Kind: store.NotFound, Msg: fmt.Sprintf(format, a...)}
	}
	switch {
	case known == "":
		if len(b.repos) > 0 {
			return nil, fail("No project is linked to %s (the git remote of %s), and there are no projects yet. "+
				"Create one with create_project and repo %q.", b.repos[0], b.cwd, b.repos[0])
		}
		return nil, fail("There are no projects yet. Create one with create_project.")
	case b.cwd == "":
		return nil, fail("Pass `project` (one of: %s); over HTTP kan-do cannot see which folder you work in.", known)
	case len(b.repos) > 0:
		return nil, fail("No project is linked to %s (the git remote of %s). Pass `project` (one of: %s), "+
			"link this repo to one with link_repo, or create a project for it with create_project and repo %q.",
			b.repos[0], b.cwd, known, b.repos[0])
	}
	return nil, fail("Cannot tell the project: %s has no git remote. Pass `project` (one of: %s).", b.cwd, known)
}

// scope resolves an optional project argument where "all" means every project (nil).
func (b *board) scope(s *store.Store, name string) (*store.Project, error) {
	if strings.EqualFold(strings.TrimSpace(name), "all") {
		return nil, nil
	}
	return b.project(s, name)
}

// gitRepos returns the GitHub repos of dir's git remotes, origin first, so it wins when
// several remotes are linked to different projects.
func gitRepos(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "remote", "-v").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	slices.SortStableFunc(lines, func(a, b string) int {
		ao, bo := strings.HasPrefix(a, "origin\t"), strings.HasPrefix(b, "origin\t")
		switch {
		case ao && !bo:
			return -1
		case bo && !ao:
			return 1
		}
		return 0
	})
	var repos []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if repo, ok := store.RepoFromURL(f[1]); ok && !slices.Contains(repos, repo) {
			repos = append(repos, repo)
		}
	}
	return repos
}

// TaskKey is a task, as api#12 or 12. A bare JSON number is accepted too.
type TaskKey string

func (k *TaskKey) UnmarshalJSON(data []byte) error {
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*k = TaskKey(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*k = TaskKey(s)
	return nil
}

type (
	createProjectArgs struct {
		Slug string `json:"slug" jsonschema:"Short lowercase name used in task ids, like api for api#12."`
		Name string `json:"name,omitempty" jsonschema:"Display name. Defaults to the slug."`
		Repo string `json:"repo,omitempty" jsonschema:"GitHub repo to link, as owner/repo. Agents working in that repo then find the project on their own."`
	}
	linkRepoArgs struct {
		Project string `json:"project" jsonschema:"Project slug."`
		Repo    string `json:"repo" jsonschema:"GitHub repo as owner/repo."`
	}
	listTasksArgs struct {
		Project string `json:"project,omitempty" jsonschema:"Project slug, or \"all\" for every project. Defaults to the project of the current repo."`
		Status  string `json:"status,omitempty" jsonschema:"Only this column. Without it, done tasks are left out; \"all\" includes them. Shows the newest 20 done."`
	}
	taskArgs struct {
		ID TaskKey `json:"id" jsonschema:"Task id, like api#12 or 12."`
	}
	createTaskArgs struct {
		Title       string `json:"title"`
		Description string `json:"description,omitempty" jsonschema:"Markdown."`
		Project     string `json:"project,omitempty" jsonschema:"Project slug. Defaults to the project of the current repo."`
		Tags        string `json:"tags,omitempty" jsonschema:"Comma-separated, like \"bug, ui\"."`
	}
	updateTaskArgs struct {
		ID          TaskKey `json:"id" jsonschema:"Task id, like api#12 or 12."`
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty" jsonschema:"Markdown; replaces the whole description."`
		Status      *string `json:"status,omitempty" jsonschema:"New column, like doing, review or done. The task goes to the bottom of it."`
		Tags        *string `json:"tags,omitempty" jsonschema:"Comma-separated; replaces all tags, and \"\" clears them."`
	}
	addCommentArgs struct {
		ID     TaskKey `json:"id" jsonschema:"Task id, like api#12 or 12."`
		Body   string  `json:"body" jsonschema:"Markdown."`
		Author string  `json:"author,omitempty" jsonschema:"Who is writing. Defaults to the name of your MCP client, like claude-code."`
	}
	nextTaskArgs struct {
		Project string `json:"project,omitempty" jsonschema:"Project slug, or \"all\" for any project. Defaults to the project of the current repo."`
	}
)

// schemaFor infers a tool's input schema. Task keys are a string or a number, and
// optional strings are plain strings: agents read this schema in every session, so
// each byte counts.
func schemaFor[T any]() *jsonschema.Schema {
	str := &jsonschema.Schema{Type: "string"}
	s, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[TaskKey](): {Types: []string{"string", "integer"}},
		reflect.TypeFor[*string](): str,
	}})
	if err != nil {
		panic(err)
	}
	return s
}

// tool registers a handler whose text reply or error goes back to the agent as is.
func tool[In any](server *sdk.Server, b *board, name, description string,
	run func(ctx context.Context, req *sdk.CallToolRequest, s *store.Store, in In) (string, error)) {
	t := &sdk.Tool{Name: name, Description: description, InputSchema: schemaFor[In]()}
	sdk.AddTool(server, t, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		s, err := b.home.Store()
		if err != nil {
			return nil, nil, err
		}
		defer s.Close()
		text, err := run(ctx, req, s, in)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, nil, nil
	})
}

func (b *board) server() *sdk.Server {
	instr := instructions
	if b.cwd == "" {
		instr = httpInstructions
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "kando", Version: version.Version},
		&sdk.ServerOptions{Instructions: instr, Capabilities: &sdk.ServerCapabilities{}})

	tool(server, b, "list_projects", "List all projects with their linked repo and open task counts.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, _ struct{}) (string, error) {
			projects, err := s.ListProjects()
			if err != nil || len(projects) == 0 {
				return "No projects yet. Create one with create_project.", err
			}
			var out []string
			here := false
			for _, p := range projects {
				line := p.Slug
				if p.Name != p.Slug {
					line += fmt.Sprintf(" %q", p.Name)
				}
				if len(p.Repos) > 0 {
					line += " (" + strings.Join(p.Repos, ", ") + ")"
				}
				line += fmt.Sprintf(": %d open of %d", p.Open, p.Total)
				if !here && slices.ContainsFunc(p.Repos, func(r string) bool { return slices.Contains(b.repos, r) }) {
					line += " <- current repo"
					here = true
				}
				out = append(out, line)
			}
			return strings.Join(out, "\n"), nil
		})

	tool(server, b, "create_project", "Create a project, optionally linked to a GitHub repo (owner/repo).",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in createProjectArgs) (string, error) {
			p, err := s.CreateProject(in.Slug, in.Name, in.Repo)
			if err != nil {
				return "", err
			}
			linked := ""
			if len(p.Repos) > 0 {
				linked = ", linked to " + p.Repos[0]
			}
			return fmt.Sprintf("Created project %s (%s)%s.", p.Slug, p.Name, linked), nil
		})

	tool(server, b, "link_repo", "Link another GitHub repo to a project, so agents working in it find the project.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in linkRepoArgs) (string, error) {
			p, err := s.LinkRepo(in.Project, in.Repo)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Project %s is linked to %s.", p.Slug, strings.Join(p.Repos, ", ")), nil
		})

	tool(server, b, "list_tasks", "List tasks one per line, in board order. Leaves out done tasks unless `status` is given.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in listTasksArgs) (string, error) {
			return b.listTasks(s, in)
		})

	tool(server, b, "get_task", "Show a task with its description, comments and the paths of its images.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in taskArgs) (string, error) {
			t, err := s.FindTask(string(in.ID))
			if err != nil {
				return "", err
			}
			return b.taskDetail(s, t)
		})

	tool(server, b, "create_task", "Add a task to the first column (todo) of a project.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in createTaskArgs) (string, error) {
			p, err := b.project(s, in.Project)
			if err != nil {
				return "", err
			}
			t, err := s.CreateTask(p.ID, in.Title, in.Description)
			if err != nil {
				return "", err
			}
			if in.Tags != "" {
				if t, err = s.UpdateTask(t.ID, store.TaskPatch{Tags: &in.Tags}); err != nil {
					return "", err
				}
			}
			return fmt.Sprintf("Created %s in %s: %s", t.Key(), t.Status, t.Title), nil
		})

	tool(server, b, "update_task", "Change a task's title, description, status or tags. Only the fields you pass change.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in updateTaskArgs) (string, error) {
			before, err := s.FindTask(string(in.ID))
			if err != nil {
				return "", err
			}
			if in.Status != nil {
				status := strings.ToLower(strings.TrimSpace(*in.Status))
				in.Status = &status
			}
			after, err := s.UpdateTask(before.ID, store.TaskPatch{Title: in.Title, Description: in.Description, Status: in.Status, Tags: in.Tags})
			if err != nil {
				return "", err
			}
			moved := ""
			if before.Status != after.Status {
				moved = fmt.Sprintf(", %s -> %s", before.Status, after.Status)
			}
			return fmt.Sprintf("Updated %s%s: %s", after.Key(), moved, after.Title), nil
		})

	tool(server, b, "add_comment", "Add a comment to a task, for example to report progress or a result.",
		func(_ context.Context, req *sdk.CallToolRequest, s *store.Store, in addCommentArgs) (string, error) {
			author := strings.TrimSpace(in.Author)
			if author == "" {
				author = clientName(req)
			}
			t, err := s.FindTask(string(in.ID))
			if err != nil {
				return "", err
			}
			if _, err := s.AddComment(t.ID, author, in.Body, nil); err != nil {
				return "", err
			}
			return fmt.Sprintf("Comment added to %s as %s.", t.Key(), author), nil
		})

	tool(server, b, "next_task", "Take the top task of the todo column, move it to doing and show it. Use this to pick up the next piece of work.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in nextTaskArgs) (string, error) {
			p, err := b.scope(s, in.Project)
			if err != nil {
				return "", err
			}
			var pid int64
			scope := "any project"
			if p != nil {
				pid, scope = p.ID, p.Slug
			}
			t, err := s.NextTask(pid)
			if err != nil {
				return "", err
			}
			if t == nil {
				return fmt.Sprintf("Nothing in %s for %s.", s.Statuses()[0], scope), nil
			}
			detail, err := b.taskDetail(s, t)
			return fmt.Sprintf("Moved to %s:\n%s", t.Status, detail), err
		})

	tool(server, b, "delete_task", "Delete a task with its comments and images. This cannot be undone.",
		func(_ context.Context, _ *sdk.CallToolRequest, s *store.Store, in taskArgs) (string, error) {
			t, err := s.FindTask(string(in.ID))
			if err != nil {
				return "", err
			}
			if _, err := home.DeletingImages(b.home, s, store.OfTask, t.ID, func() (*store.Task, error) { return s.DeleteTask(t.ID) }); err != nil {
				return "", err
			}
			comments := ""
			switch t.Comments {
			case 0:
			case 1:
				comments = " and its comment"
			default:
				comments = fmt.Sprintf(" and its %d comments", t.Comments)
			}
			return fmt.Sprintf("Deleted %s%s: %s", t.Key(), comments, t.Title), nil
		})

	return server
}

// clientName is the MCP client's name, like claude-code, or "agent".
func clientName(req *sdk.CallToolRequest) string {
	if req != nil && req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
			return p.ClientInfo.Name
		}
	}
	return "agent"
}

func (b *board) listTasks(s *store.Store, in listTasksArgs) (string, error) {
	p, err := b.scope(s, in.Project)
	if err != nil {
		return "", err
	}
	status := strings.TrimSpace(in.Status)
	only := ""
	switch {
	case status == "" || status == "all":
	case slices.Contains(s.Statuses(), status):
		only = status
	default:
		return "", &store.Error{Kind: store.Invalid, Msg: fmt.Sprintf("unknown status %q; use one of: %s, or all", status, strings.Join(s.Statuses(), ", "))}
	}
	// Done tasks only pile up: without a status leave them out, with one keep the newest.
	keep := 0
	if status != "" {
		keep = doneListed
	}
	var pid int64
	if p != nil {
		pid = p.ID
	}
	tasks, hidden, err := s.ListTasksKeepingDone(pid, only, keep)
	if err != nil {
		return "", err
	}
	done := s.Statuses()[len(s.Statuses())-1]
	var out []string
	// Tasks for a person are left out: agents leave them alone.
	people := 0
	for _, t := range tasks {
		if t.Human {
			people++
		} else {
			out = append(out, taskLine(&t))
		}
	}
	if len(out) == 0 {
		scope := "any project"
		if p != nil {
			scope = p.Slug
		}
		out = append(out, fmt.Sprintf("No tasks in %s.", scope))
	}
	if people > 0 {
		out = append(out, fmt.Sprintf("(%d human %s not shown: for the user, not for agents)", people, plural(people, "task", "tasks")))
	}
	switch {
	case hidden > 0 && status == "":
		out = append(out, fmt.Sprintf("(%d %s not shown; pass status %q or \"all\")", hidden, done, done))
	case hidden > 0:
		out = append(out, fmt.Sprintf("(%d older %s not shown)", hidden, done))
	}
	return strings.Join(out, "\n"), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// taskLine is api#12 [todo] Fix login {bug} (2 comments).
func taskLine(t *store.Task) string {
	line := fmt.Sprintf("%s [%s] %s%s", t.Key(), t.Status, t.Title, tags(t))
	switch t.Comments {
	case 0:
	case 1:
		line += " (1 comment)"
	default:
		line += fmt.Sprintf(" (%d comments)", t.Comments)
	}
	return line
}

// tags is " {bug, ui}", or nothing without tags.
func tags(t *store.Task) string {
	if len(t.Tags) == 0 {
		return ""
	}
	return " {" + strings.Join(t.Tags, ", ") + "}"
}

// taskDetail is a task in full: description, comments, and the paths of its images,
// which agents can open.
func (b *board) taskDetail(s *store.Store, t *store.Task) (string, error) {
	comments, err := s.ListComments(t.ID)
	if err != nil {
		return "", err
	}
	images, err := s.ListImages(t.ID)
	if err != nil {
		return "", err
	}
	paths := func(comment *int64) []string {
		var out []string
		for _, i := range images {
			if comment == nil && i.CommentID == nil || comment != nil && i.CommentID != nil && *i.CommentID == *comment {
				out = append(out, filepath.Join(b.home.ImagesDir(), i.File))
			}
		}
		return out
	}
	created, updated := store.ShortTime(t.CreatedAt), store.ShortTime(t.UpdatedAt)
	var out strings.Builder
	fmt.Fprintf(&out, "%s %s%s\nstatus: %s", t.Key(), t.Title, tags(t), t.Status)
	if t.Human {
		out.WriteString(", human: for the user, not for agents")
	}
	fmt.Fprintf(&out, ", created %s", created)
	// The update time only when it says something the creation time does not.
	if updated != created {
		fmt.Fprintf(&out, ", updated %s", updated)
	}
	out.WriteString("\n")
	if t.Description != "" {
		fmt.Fprintf(&out, "\n%s\n", t.Description)
	}
	if own := paths(nil); len(own) > 0 {
		fmt.Fprintf(&out, "\nImages: %s\n", strings.Join(own, ", "))
	}
	if len(comments) > 0 {
		fmt.Fprintf(&out, "\nComments (%d):\n", len(comments))
		for _, c := range comments {
			fmt.Fprintf(&out, "- %s, %s: %s", c.Author, store.ShortTime(c.CreatedAt), c.Body)
			if attached := paths(&c.ID); len(attached) > 0 {
				sep := " "
				if c.Body == "" {
					sep = ""
				}
				fmt.Fprintf(&out, "%s[images: %s]", sep, strings.Join(attached, ", "))
			}
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), " \t\n"), nil
}
