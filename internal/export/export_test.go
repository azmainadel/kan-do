package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/azmainadel/kan-do/internal/home"
)

func TestExportsEverything(t *testing.T) {
	h, err := home.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := h.Store()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	api, _ := s.CreateProject("api", "API", "me/api")
	s.CreateProject("web", "Web", "")
	task, _ := s.CreateTask(api.ID, "Fix login", "Steps:\n1. log in")
	s.AddComment(task.ID, "claude-code", "Found it\nin auth.go", nil)
	done, _ := s.CreateTask(api.ID, "Old work", "")
	s.MoveTask(done.ID, "done", 0)

	b, err := Collect(s, "", "/k/images")
	if err != nil {
		t.Fatal(err)
	}
	md := Markdown(b)
	for _, want := range []string{
		"## API `api` ([me/api](https://github.com/me/api))",
		"### todo\n\n#### api#1 Fix login",
		"Steps:\n1. log in",
		"- **claude-code**, ",
		"Found it\n  in auth.go",
		"### done\n\n#### api#2 Old work",
		"## Web `web`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown has no %q:\n%s", want, md)
		}
	}

	data, _ := json.Marshal(b)
	var v struct {
		Statuses []string `json:"statuses"`
		Projects []struct {
			Slug  string `json:"slug"`
			Tasks []struct {
				Key      string `json:"key"`
				Comments []struct {
					Author string `json:"author"`
				} `json:"comments"`
			} `json:"tasks"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Statuses[0] != "todo" || v.Projects[0].Slug != "api" || v.Projects[0].Tasks[0].Key != "api#1" ||
		v.Projects[0].Tasks[0].Comments[0].Author != "claude-code" {
		t.Fatalf("json: %s", data)
	}

	only, err := Collect(s, "web", "/k/images")
	if err != nil || len(only.Projects) != 1 {
		t.Fatalf("one project: %v %v", only, err)
	}
	if _, err := Collect(s, "nope", "/k/images"); err == nil {
		t.Fatal("unknown project exported")
	}
}
