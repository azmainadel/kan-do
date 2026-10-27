// Package export writes the whole board as Markdown or JSON, readable without kan-do.
package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/store"
)

// Board is everything exported.
type Board struct {
	ExportedAt string    `json:"exported_at"`
	Statuses   []string  `json:"statuses"`
	Projects   []Project `json:"projects"`
}

// Project is a project with its tasks.
type Project struct {
	store.Project
	Tasks []Task `json:"tasks"`
}

// Task is a task with its comments and the paths of its images.
type Task struct {
	ID          int64    `json:"id"`
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	// Human marks work for a person, which agents leave alone.
	Human bool `json:"human"`
	// Images are the paths of the images attached to the task and its comments.
	Images    []string        `json:"images"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Comments  []store.Comment `json:"comments"`
}

// Run exports the board, or one project, as "markdown" or "json", to output or to stdout.
func Run(h *home.Home, format, project, output string) error {
	s, err := h.Store()
	if err != nil {
		return err
	}
	defer s.Close()
	b, err := Collect(s, project, h.ImagesDir())
	if err != nil {
		return err
	}
	var text string
	switch format {
	case "json":
		data, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			return err
		}
		text = string(data) + "\n"
	case "markdown", "md", "":
		text = Markdown(b)
	default:
		return fmt.Errorf("unknown format %q; use markdown or json", format)
	}
	if output == "" {
		_, err := fmt.Print(text)
		return err
	}
	if err := os.WriteFile(output, []byte(text), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", output, err)
	}
	tasks := 0
	for _, p := range b.Projects {
		tasks += len(p.Tasks)
	}
	fmt.Fprintf(os.Stderr, "Exported %d projects and %d tasks to %s\n", len(b.Projects), tasks, output)
	return nil
}

// Collect reads the board, or the project named only, with image paths under images.
func Collect(s *store.Store, only, images string) (*Board, error) {
	var projects []store.Project
	if only != "" {
		p, err := s.FindProject(only)
		if err != nil {
			return nil, err
		}
		projects = []store.Project{*p}
	} else {
		var err error
		if projects, err = s.ListProjects(); err != nil {
			return nil, err
		}
	}
	b := &Board{Statuses: s.Statuses(), Projects: []Project{}}
	var err error
	if b.ExportedAt, err = s.Now(); err != nil {
		return nil, err
	}
	for _, p := range projects {
		tasks, err := s.ListTasks(p.ID, "")
		if err != nil {
			return nil, err
		}
		out := Project{Project: p, Tasks: []Task{}}
		for _, t := range tasks {
			comments, err := s.ListComments(t.ID)
			if err != nil {
				return nil, err
			}
			imgs, err := s.ListImages(t.ID)
			if err != nil {
				return nil, err
			}
			paths := []string{}
			for _, i := range imgs {
				paths = append(paths, filepath.Join(images, i.File))
			}
			out.Tasks = append(out.Tasks, Task{
				ID: t.ID, Key: t.Key(), Title: t.Title, Description: t.Description, Tags: t.Tags, Human: t.Human,
				Images: paths, Status: t.Status, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Comments: comments,
			})
		}
		b.Projects = append(b.Projects, out)
	}
	return b, nil
}

// Markdown renders the board as Markdown: a section per project, then per column.
func Markdown(b *Board) string {
	var md strings.Builder
	fmt.Fprintf(&md, "# kan-do board\n\nExported %s.\n", store.ShortTime(b.ExportedAt))
	for _, p := range b.Projects {
		var links []string
		for _, r := range p.Repos {
			links = append(links, fmt.Sprintf("[%s](https://github.com/%s)", r, r))
		}
		repo := ""
		if len(links) > 0 {
			repo = " (" + strings.Join(links, ", ") + ")"
		}
		noun := "tasks"
		if p.Total == 1 {
			noun = "task"
		}
		fmt.Fprintf(&md, "\n## %s `%s`%s\n\n%d open of %d %s.\n", p.Name, p.Slug, repo, p.Open, p.Total, noun)
		// Columns in board order, then any status no longer in the config.
		statuses := slices.Clone(b.Statuses)
		for _, t := range p.Tasks {
			if !slices.Contains(statuses, t.Status) {
				statuses = append(statuses, t.Status)
			}
		}
		for _, status := range statuses {
			header := false
			for _, t := range p.Tasks {
				if t.Status != status {
					continue
				}
				if !header {
					fmt.Fprintf(&md, "\n### %s\n", status)
					header = true
				}
				fmt.Fprintf(&md, "\n#### %s %s\n\nCreated %s, updated %s.\n", t.Key, t.Title, store.ShortTime(t.CreatedAt), store.ShortTime(t.UpdatedAt))
				if t.Human {
					md.WriteString("Human task.\n")
				}
				if len(t.Tags) > 0 {
					fmt.Fprintf(&md, "Tags: %s.\n", strings.Join(t.Tags, ", "))
				}
				if len(t.Images) > 0 {
					fmt.Fprintf(&md, "Images: %s.\n", strings.Join(t.Images, ", "))
				}
				if t.Description != "" {
					fmt.Fprintf(&md, "\n%s\n", t.Description)
				}
				if len(t.Comments) > 0 {
					md.WriteString("\nComments:\n\n")
					for _, c := range t.Comments {
						// Indent follow-on lines so a multi-line comment stays inside its list item.
						body := strings.ReplaceAll(c.Body, "\n", "\n  ")
						fmt.Fprintf(&md, "- **%s**, %s: %s\n", c.Author, store.ShortTime(c.CreatedAt), body)
					}
				}
			}
		}
	}
	return md.String()
}
