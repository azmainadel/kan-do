// Package store holds all reads and writes of kando.db: plain SQL, one short-lived
// connection per operation.
//
// The database uses a rollback journal (journal_mode=TRUNCATE), not WAL. With a
// connection per operation, WAL would set up and tear down its -wal and -shm files on
// every request; the rollback journal makes a read touch no extra files at all. Writes
// queue on the busy timeout.
package store

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"slices"
	"strings"

	_ "modernc.org/sqlite"
)

const now = `strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`

// KeepAll, passed as keepDone, lists every done task.
const KeepAll = -1

// Project is a board project with its open and total task counts.
type Project struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Repos are linked GitHub repos as owner/repo, in the order they were linked.
	Repos []string `json:"repos"`
	// Color is one of projectColors, or nil when never picked.
	Color     *string `json:"color"`
	CreatedAt string  `json:"created_at"`
	// Open counts tasks not in the last (done) column.
	Open  int64 `json:"open"`
	Total int64 `json:"total"`
}

// Task is a card on the board.
type Task struct {
	ID          int64  `json:"id"`
	Project     string `json:"project"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Tags are lowercase, without duplicates, in the order they were given.
	Tags []string `json:"tags"`
	// Human marks work for a person: agents leave it alone, and NextTask never picks it.
	Human     bool    `json:"human"`
	Status    string  `json:"status"`
	Position  float64 `json:"position"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Comments  int64   `json:"comments"`
}

// Key is how tasks are shown everywhere: api#12.
func (t *Task) Key() string { return fmt.Sprintf("%s#%d", t.Project, t.ID) }

// Comment is a note on a task, from the person or an agent.
type Comment struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// Image is attached to a task, or to one of its comments.
type Image struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	CommentID *int64 `json:"comment_id"`
	// File is its name in the data folder's images/, like 6fec521a01297ab5.png.
	File string `json:"file"`
}

// TaskPatch holds the fields of a task to change; nil leaves a field as it is.
type TaskPatch struct {
	Title       *string
	Description *string
	Status      *string
	// ProjectID moves the task to another project.
	ProjectID *int64
	// Tags are comma-separated; they replace all the tags, and "" clears them.
	Tags  *string
	Human *bool
}

// querier is what *sql.DB and *sql.Tx have in common.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Store is one open connection to the board.
type Store struct {
	db       *sql.DB
	statuses []string
	lock     io.Closer
}

// Open opens the database at path. lock, if not nil, is closed after the connection.
func Open(path string, statuses []string, lock io.Closer) (*Store, error) {
	// The board and any number of agents share this file: wait instead of failing.
	// Pragmas are per connection, not stored in the file. TRUNCATE also turns a WAL
	// database back into a rollback-journal one once nothing else has it open.
	// synchronous stays at SQLite's default FULL, which keeps the file intact on power loss.
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(TRUNCATE)")
	q.Add("_pragma", "foreign_keys(1)")
	// BEGIN IMMEDIATE, so concurrent writers queue up on the busy timeout instead of failing halfway.
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, dbErr(err)
	}
	// One connection: a Store lives for one operation.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, dbErr(err)
	}
	return &Store{db: db, statuses: statuses, lock: lock}, nil
}

// Close closes the connection, then releases the lock.
func (s *Store) Close() error {
	err := s.db.Close()
	if s.lock != nil {
		s.lock.Close()
	}
	return err
}

// Statuses are the board's columns, left to right.
func (s *Store) Statuses() []string { return s.statuses }

func (s *Store) firstStatus() string { return s.statuses[0] }
func (s *Store) doneStatus() string  { return s.statuses[len(s.statuses)-1] }

// write runs f in an immediate transaction.
func (s *Store) write(f func(q querier) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return dbErr(err)
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return dbErr(tx.Commit())
}

// Changes is the counter that triggers bump on every write; the board watches it.
func (s *Store) Changes() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'changes'`).Scan(&n)
	return n, dbErr(err)
}

// Now is the current time, in the same format as the stored timestamps.
func (s *Store) Now() (string, error) {
	var t string
	err := s.db.QueryRow(`SELECT ` + now).Scan(&t)
	return t, dbErr(err)
}

// Settle makes kando.db complete on its own: it rolls back a write that a crashed
// process left half done, and folds in a leftover WAL.
func (s *Store) Settle() error {
	var n int64
	return dbErr(s.db.QueryRow(`SELECT count(*) FROM meta`).Scan(&n))
}

// Projects

const projectSelect = `SELECT p.id, p.slug, p.name,
        (SELECT group_concat(repo, ',' ORDER BY r.rowid) FROM project_repos r WHERE r.project_id = p.id),
        p.color, p.created_at,
        (SELECT count(*) FROM tasks t WHERE t.project_id = p.id AND t.status <> ?1),
        (SELECT count(*) FROM tasks t WHERE t.project_id = p.id)
    FROM projects p`

type scanner interface{ Scan(dest ...any) error }

func scanProject(r scanner) (Project, error) {
	var p Project
	var repos, color sql.NullString
	err := r.Scan(&p.ID, &p.Slug, &p.Name, &repos, &color, &p.CreatedAt, &p.Open, &p.Total)
	p.Repos = []string{}
	if repos.Valid {
		p.Repos = strings.Split(repos.String, ",")
	}
	if color.Valid {
		p.Color = &color.String
	}
	return p, err
}

// ListProjects returns every project, by slug.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(projectSelect+` ORDER BY p.slug`, s.doneStatus())
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		projects = append(projects, p)
	}
	return projects, dbErr(rows.Err())
}

// projectWhere returns the project matching clause (with ?2 for value), or nil.
func (s *Store) projectWhere(clause, value string) (*Project, error) {
	p, err := scanProject(s.db.QueryRow(projectSelect+" WHERE "+clause, s.doneStatus(), value))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return &p, nil
}

// ProjectBySlug returns the project with this slug, or nil.
func (s *Store) ProjectBySlug(slug string) (*Project, error) {
	return s.projectWhere("p.slug = ?2", strings.ToLower(strings.TrimSpace(slug)))
}

// ProjectByRepo returns the project linked to repo (owner/repo or any git URL), or nil.
func (s *Store) ProjectByRepo(repo string) (*Project, error) {
	r, ok := RepoFromURL(repo)
	if !ok {
		return nil, nil
	}
	return s.projectWhere("p.id = (SELECT project_id FROM project_repos WHERE repo = ?2)", r)
}

// FindProject finds a project by slug, or by linked repo when given owner/repo.
func (s *Store) FindProject(name string) (*Project, error) {
	p, err := s.ProjectBySlug(name)
	if err == nil && p == nil && strings.Contains(name, "/") {
		p, err = s.ProjectByRepo(name)
	}
	if err != nil || p != nil {
		return p, err
	}
	hint := "there are no projects yet"
	if known, _ := s.ListProjects(); len(known) > 0 {
		slugs := make([]string, len(known))
		for i, k := range known {
			slugs[i] = k.Slug
		}
		hint = "projects: " + strings.Join(slugs, ", ")
	}
	return nil, notFound("no project %q (%s)", name, hint)
}

// CreateProject creates a project, linked to repo unless repo is empty.
func (s *Store) CreateProject(slug, name, repo string) (*Project, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !validSlug(slug) {
		return nil, invalid("project slug %q must be 1-32 lowercase letters, digits or -, starting with a letter or digit", slug)
	}
	if name = strings.TrimSpace(name); name == "" {
		name = slug
	}
	if repo = strings.TrimSpace(repo); repo != "" {
		var err error
		if repo, err = cleanRepo(repo); err != nil {
			return nil, err
		}
	}
	if p, err := s.ProjectBySlug(slug); err != nil {
		return nil, err
	} else if p != nil {
		return nil, invalid("project %s already exists", slug)
	}
	if repo != "" {
		if err := s.checkRepoFree(repo); err != nil {
			return nil, err
		}
	}
	err := s.write(func(q querier) error {
		res, err := q.Exec(`INSERT INTO projects (slug, name) VALUES (?1, ?2)`, slug, name)
		if err != nil || repo == "" {
			return err
		}
		id, _ := res.LastInsertId()
		_, err = q.Exec(`INSERT INTO project_repos (repo, project_id) VALUES (?1, ?2)`, repo, id)
		return err
	})
	if isConstraint(err) {
		// Someone created the same project or linked the repo between our checks and the insert.
		return nil, invalid("project %s or its repo already exists", slug)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return s.FindProject(slug)
}

// LinkRepo links another repo to a project. Each repo belongs to one project.
func (s *Store) LinkRepo(slug, repo string) (*Project, error) {
	p, err := s.FindProject(slug)
	if err != nil {
		return nil, err
	}
	if repo, err = cleanRepo(repo); err != nil {
		return nil, err
	}
	if slices.Contains(p.Repos, repo) {
		return p, nil
	}
	if err := s.checkRepoFree(repo); err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`INSERT INTO project_repos (repo, project_id) VALUES (?1, ?2)`, repo, p.ID)
	if isConstraint(err) {
		return nil, invalid("repo %s is already linked to another project", repo)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return s.FindProject(p.Slug)
}

// UnlinkRepo removes a repo from a project.
func (s *Store) UnlinkRepo(slug, repo string) (*Project, error) {
	p, err := s.FindProject(slug)
	if err != nil {
		return nil, err
	}
	r, ok := RepoFromURL(repo)
	if !ok {
		r = strings.ToLower(strings.TrimSpace(repo))
	}
	res, err := s.db.Exec(`DELETE FROM project_repos WHERE repo = ?1 AND project_id = ?2`, r, p.ID)
	if err != nil {
		return nil, dbErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, notFound("repo %s is not linked to project %s", r, p.Slug)
	}
	return s.FindProject(p.Slug)
}

func (s *Store) checkRepoFree(repo string) error {
	other, err := s.ProjectByRepo(repo)
	if err != nil {
		return err
	}
	if other != nil {
		return invalid("repo %s is already linked to project %s", repo, other.Slug)
	}
	return nil
}

// RenameProject renames a project. Its slug, and so its task keys like api#12, stay.
func (s *Store) RenameProject(slug, name string) (*Project, error) {
	if name = squash(name); name == "" {
		return nil, invalid("project name is empty")
	}
	p, err := s.FindProject(slug)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE projects SET name = ?1 WHERE id = ?2`, name, p.ID); err != nil {
		return nil, dbErr(err)
	}
	return s.FindProject(p.Slug)
}

// SetProjectColor sets a project's color to one of the named colors.
func (s *Store) SetProjectColor(slug, color string) (*Project, error) {
	if !slices.Contains(projectColors, color) {
		return nil, invalid("unknown color %q; use one of: %s", color, strings.Join(projectColors, ", "))
	}
	p, err := s.FindProject(slug)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE projects SET color = ?1 WHERE id = ?2`, color, p.ID); err != nil {
		return nil, dbErr(err)
	}
	return s.FindProject(p.Slug)
}

// DeleteProject deletes a project with all its tasks and their comments, and returns
// the project as it was.
func (s *Store) DeleteProject(slug string) (*Project, error) {
	p, err := s.FindProject(slug)
	if err != nil {
		return nil, err
	}
	// Tasks, comments, images and repo links go with it through ON DELETE CASCADE.
	if _, err := s.db.Exec(`DELETE FROM projects WHERE id = ?1`, p.ID); err != nil {
		return nil, dbErr(err)
	}
	return p, nil
}

// Tasks

const taskSelect = `SELECT t.id, p.slug, t.title, t.description, t.status, t.position,
        t.created_at, t.updated_at, (SELECT count(*) FROM comments c WHERE c.task_id = t.id), t.tags, t.human
    FROM tasks t JOIN projects p ON p.id = t.project_id`

func scanTask(r scanner) (Task, error) {
	var t Task
	var tags string
	err := r.Scan(&t.ID, &t.Project, &t.Title, &t.Description, &t.Status, &t.Position,
		&t.CreatedAt, &t.UpdatedAt, &t.Comments, &tags, &t.Human)
	t.Tags = []string{}
	for tag := range strings.SplitSeq(tags, ",") {
		if tag != "" {
			t.Tags = append(t.Tags, tag)
		}
	}
	return t, err
}

func scanTasks(rows *sql.Rows, err error) ([]Task, error) {
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		tasks = append(tasks, t)
	}
	return tasks, dbErr(rows.Err())
}

// nullable turns the zero value into SQL NULL, for optional filters.
func nullable[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

// ListTasks returns tasks in board order: by column as configured, then by position.
// projectID 0 means every project, status "" every column. Tasks whose status is no
// longer in the config sort last.
func (s *Store) ListTasks(projectID int64, status string) ([]Task, error) {
	tasks, _, err := s.ListTasksKeepingDone(projectID, status, KeepAll)
	return tasks, err
}

// ListTasksKeepingDone is ListTasks, but of the done column, which only grows, it reads
// just the newest keepDone tasks (all with KeepAll). It also says how many it left out.
func (s *Store) ListTasksKeepingDone(projectID int64, status string, keepDone int) ([]Task, int, error) {
	done := s.doneStatus()
	pid, st := nullable(projectID), nullable(status)
	// SQLite reads a negative LIMIT as no limit.
	tasks, err := scanTasks(s.db.Query(taskSelect+`
		WHERE (?1 IS NULL OR t.project_id = ?1) AND (?2 IS NULL OR t.status = ?2)
		  AND (t.status <> ?3 OR t.id IN (SELECT id FROM tasks WHERE status = ?3
		       AND (?1 IS NULL OR project_id = ?1) ORDER BY position DESC LIMIT ?4))
		ORDER BY t.position`, pid, st, done, keepDone))
	if err != nil {
		return nil, 0, err
	}
	column := func(t Task) int {
		if i := slices.Index(s.statuses, t.Status); i >= 0 {
			return i
		}
		return len(s.statuses)
	}
	slices.SortStableFunc(tasks, func(a, b Task) int { return cmp.Compare(column(a), column(b)) })
	hidden := 0
	if keepDone >= 0 && (status == "" || status == done) {
		var all int
		err := s.db.QueryRow(`SELECT count(*) FROM tasks WHERE status = ?1 AND (?2 IS NULL OR project_id = ?2)`,
			done, pid).Scan(&all)
		if err != nil {
			return nil, 0, dbErr(err)
		}
		hidden = max(all-keepDone, 0)
	}
	return tasks, hidden, nil
}

// Search returns tasks of any project whose key, title, description, tags or project
// hold every word of query: open ones first, then the most recently changed. An empty
// query gives the most recently changed tasks.
func (s *Store) Search(query string, limit int) ([]Task, error) {
	words := strings.Fields(query)
	words = words[:min(len(words), 10)]
	var b strings.Builder
	b.WriteString(taskSelect + " WHERE 1")
	args := []any{s.doneStatus()}
	escape := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	for i, w := range words {
		fmt.Fprintf(&b, ` AND lower(p.slug || '#' || t.id || ' ' || t.title || ' ' || t.description || ' ' || t.tags) LIKE ?%d ESCAPE '\'`, i+2)
		args = append(args, "%"+escape.Replace(strings.ToLower(w))+"%")
	}
	fmt.Fprintf(&b, " ORDER BY t.status = ?1, t.updated_at DESC, t.id DESC LIMIT %d", limit)
	return scanTasks(s.db.Query(b.String(), args...))
}

func getTask(q querier, id int64) (*Task, error) {
	t, err := scanTask(q.QueryRow(taskSelect+" WHERE t.id = ?1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("no task #%d", id)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return &t, nil
}

// GetTask returns the task with this id.
func (s *Store) GetTask(id int64) (*Task, error) { return getTask(s.db, id) }

// FindTask looks up api#12, #12 or 12. A project prefix must match the task's project.
func (s *Store) FindTask(key string) (*Task, error) {
	slug, id, err := parseTaskKey(key)
	if err != nil {
		return nil, err
	}
	t, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	if slug != "" && slug != t.Project {
		return nil, notFound("no task %s (#%d is %s)", key, id, t.Key())
	}
	return t, nil
}

func (s *Store) checkStatus(status string) error {
	if !slices.Contains(s.statuses, status) {
		return invalid("unknown status %q; use one of: %s", status, strings.Join(s.statuses, ", "))
	}
	return nil
}

// endOfColumn is the position just below the last task of a column, ignoring task except.
func endOfColumn(q querier, status string, except int64) (float64, error) {
	var top sql.NullFloat64
	if err := q.QueryRow(`SELECT max(position) FROM tasks WHERE status = ?1 AND id <> ?2`, status, except).Scan(&top); err != nil {
		return 0, dbErr(err)
	}
	if !top.Valid {
		return 1, nil
	}
	return math.Floor(top.Float64) + 1, nil
}

// CreateTask adds a task to the bottom of the first column.
func (s *Store) CreateTask(projectID int64, title, description string) (*Task, error) {
	title, err := cleanTitle(title)
	if err != nil {
		return nil, err
	}
	status := s.firstStatus()
	var id int64
	err = s.write(func(q querier) error {
		pos, err := endOfColumn(q, status, 0)
		if err != nil {
			return err
		}
		res, err := q.Exec(`INSERT INTO tasks (project_id, title, description, status, position) VALUES (?1, ?2, ?3, ?4, ?5)`,
			projectID, title, strings.TrimSpace(description), status, pos)
		if err != nil {
			return dbErr(err)
		}
		id, err = res.LastInsertId()
		return dbErr(err)
	})
	if err != nil {
		return nil, err
	}
	return s.GetTask(id)
}

// UpdateTask changes the fields set in patch. A new status puts the task at the bottom of that column.
func (s *Store) UpdateTask(id int64, patch TaskPatch) (*Task, error) {
	var title string
	if patch.Title != nil {
		var err error
		if title, err = cleanTitle(*patch.Title); err != nil {
			return nil, err
		}
	}
	if patch.Status != nil {
		if err := s.checkStatus(*patch.Status); err != nil {
			return nil, err
		}
	}
	err := s.write(func(q querier) error {
		current, err := getTask(q, id)
		if err != nil {
			return err
		}
		set := func(query string, args ...any) error {
			_, err := q.Exec(query, args...)
			return dbErr(err)
		}
		if patch.Title != nil {
			if err := set(`UPDATE tasks SET title = ?1, updated_at = `+now+` WHERE id = ?2`, title, id); err != nil {
				return err
			}
		}
		if patch.ProjectID != nil {
			if err := set(`UPDATE tasks SET project_id = ?1, updated_at = `+now+` WHERE id = ?2 AND project_id <> ?1`, *patch.ProjectID, id); err != nil {
				return err
			}
		}
		if patch.Human != nil {
			if err := set(`UPDATE tasks SET human = ?1, updated_at = `+now+` WHERE id = ?2 AND human <> ?1`, *patch.Human, id); err != nil {
				return err
			}
		}
		if patch.Tags != nil {
			if err := set(`UPDATE tasks SET tags = ?1, updated_at = `+now+` WHERE id = ?2`, strings.Join(cleanTags(*patch.Tags), ","), id); err != nil {
				return err
			}
		}
		if patch.Description != nil {
			if err := set(`UPDATE tasks SET description = ?1, updated_at = `+now+` WHERE id = ?2`, strings.TrimSpace(*patch.Description), id); err != nil {
				return err
			}
		}
		if patch.Status != nil && *patch.Status != current.Status {
			pos, err := endOfColumn(q, *patch.Status, id)
			if err != nil {
				return err
			}
			return set(`UPDATE tasks SET status = ?1, position = ?2, updated_at = `+now+` WHERE id = ?3`, *patch.Status, pos, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetTask(id)
}

// MoveTask moves a task to status, just above task before, or to the bottom when before is 0.
func (s *Store) MoveTask(id int64, status string, before int64) (*Task, error) {
	if err := s.checkStatus(status); err != nil {
		return nil, err
	}
	err := s.write(func(q querier) error {
		current, err := getTask(q, id)
		if err != nil {
			return err
		}
		if before == id {
			before = 0
		}
		// The task to go above, with its position, if it's in that column.
		var next float64
		anchored := false
		if before != 0 {
			err := q.QueryRow(`SELECT position FROM tasks WHERE id = ?1 AND status = ?2`, before, status).Scan(&next)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return dbErr(err)
			}
			anchored = err == nil
		}
		var pos float64
		if !anchored {
			if pos, err = endOfColumn(q, status, id); err != nil {
				return err
			}
		} else {
			prevOf := func(next float64) (sql.NullFloat64, error) {
				var p sql.NullFloat64
				err := q.QueryRow(`SELECT max(position) FROM tasks WHERE status = ?1 AND position < ?2 AND id <> ?3`,
					status, next, id).Scan(&p)
				return p, dbErr(err)
			}
			prev, err := prevOf(next)
			if err != nil {
				return err
			}
			if prev.Valid && next-prev.Float64 < 1e-9 {
				// Out of room between two neighbours: renumber the column 1, 2, 3...
				if err := renumber(q, status); err != nil {
					return err
				}
				if err := q.QueryRow(`SELECT position FROM tasks WHERE id = ?1`, before).Scan(&next); err != nil {
					return dbErr(err)
				}
				if prev, err = prevOf(next); err != nil {
					return err
				}
			}
			if prev.Valid {
				pos = (prev.Float64 + next) / 2
			} else {
				pos = next - 1
			}
		}
		touched := ""
		if current.Status != status {
			touched = ", updated_at = " + now
		}
		_, err = q.Exec(`UPDATE tasks SET status = ?1, position = ?2`+touched+` WHERE id = ?3`, status, pos, id)
		return dbErr(err)
	})
	if err != nil {
		return nil, err
	}
	return s.GetTask(id)
}

func renumber(q querier, status string) error {
	rows, err := q.Query(`SELECT id FROM tasks WHERE status = ?1 ORDER BY position`, status)
	if err != nil {
		return dbErr(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return dbErr(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	for i, id := range ids {
		if _, err := q.Exec(`UPDATE tasks SET position = ?1 WHERE id = ?2`, float64(i+1), id); err != nil {
			return dbErr(err)
		}
	}
	return nil
}

// NextTask takes the top task of the first column that isn't for a person, and moves it
// to the bottom of the second. It returns nil when there is none.
func (s *Store) NextTask(projectID int64) (*Task, error) {
	from, to := s.firstStatus(), s.statuses[1]
	var id int64
	err := s.write(func(q querier) error {
		err := q.QueryRow(`SELECT id FROM tasks WHERE status = ?1 AND human = 0 AND (?2 IS NULL OR project_id = ?2)
			ORDER BY position LIMIT 1`, from, nullable(projectID)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return dbErr(err)
		}
		pos, err := endOfColumn(q, to, id)
		if err != nil {
			return err
		}
		_, err = q.Exec(`UPDATE tasks SET status = ?1, position = ?2, updated_at = `+now+` WHERE id = ?3`, to, pos, id)
		return dbErr(err)
	})
	if err != nil || id == 0 {
		return nil, err
	}
	return s.GetTask(id)
}

// DeleteTask deletes a task and its comments, and returns the task as it was.
func (s *Store) DeleteTask(id int64) (*Task, error) {
	t, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?1`, id); err != nil {
		return nil, dbErr(err)
	}
	return t, nil
}

// Comments

const commentSelect = `SELECT id, task_id, author, body, created_at FROM comments`

func scanComment(r scanner) (Comment, error) {
	var c Comment
	err := r.Scan(&c.ID, &c.TaskID, &c.Author, &c.Body, &c.CreatedAt)
	return c, err
}

// ListComments returns a task's comments, oldest first.
func (s *Store) ListComments(taskID int64) ([]Comment, error) {
	rows, err := s.db.Query(commentSelect+` WHERE task_id = ?1 ORDER BY id`, taskID)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	comments := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		comments = append(comments, c)
	}
	return comments, dbErr(rows.Err())
}

// AddComment adds a comment, with images (files already in images/) attached to it.
// A comment needs text, an image, or both.
func (s *Store) AddComment(taskID int64, author, body string, images []string) (*Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" && len(images) == 0 {
		return nil, invalid("comment body is empty")
	}
	for _, f := range images {
		if !ValidImageFile(f) {
			return nil, invalid("%q is not an image kan-do saved", f)
		}
	}
	if author = strings.TrimSpace(author); author == "" {
		author = "me"
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	var id int64
	err := s.write(func(q querier) error {
		res, err := q.Exec(`INSERT INTO comments (task_id, author, body) VALUES (?1, ?2, ?3)`, taskID, author, body)
		if err != nil {
			return dbErr(err)
		}
		id, _ = res.LastInsertId()
		for _, f := range images {
			if _, err := q.Exec(`INSERT INTO images (task_id, comment_id, file) VALUES (?1, ?2, ?3)`, taskID, id, f); err != nil {
				return dbErr(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.getComment(id)
}

func (s *Store) getComment(id int64) (*Comment, error) {
	c, err := scanComment(s.db.QueryRow(commentSelect+` WHERE id = ?1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("no comment %d", id)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return &c, nil
}

// DeleteComment deletes a comment and returns it as it was.
func (s *Store) DeleteComment(id int64) (*Comment, error) {
	c, err := s.getComment(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM comments WHERE id = ?1`, id); err != nil {
		return nil, dbErr(err)
	}
	return c, nil
}

// Images

const imageSelect = `SELECT id, task_id, comment_id, file FROM images`

func scanImage(r scanner) (Image, error) {
	var i Image
	var comment sql.NullInt64
	err := r.Scan(&i.ID, &i.TaskID, &comment, &i.File)
	if comment.Valid {
		i.CommentID = &comment.Int64
	}
	return i, err
}

// AddImage attaches an image, already saved in images/, to a task.
func (s *Store) AddImage(taskID int64, file string) (*Image, error) {
	if !ValidImageFile(file) {
		return nil, invalid("%q is not an image kan-do saved", file)
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`INSERT INTO images (task_id, file) VALUES (?1, ?2)`, taskID, file)
	if err != nil {
		return nil, dbErr(err)
	}
	id, _ := res.LastInsertId()
	return &Image{ID: id, TaskID: taskID, File: file}, nil
}

// ListImages returns the images of a task and of its comments, oldest first.
func (s *Store) ListImages(taskID int64) ([]Image, error) {
	rows, err := s.db.Query(imageSelect+` WHERE task_id = ?1 ORDER BY id`, taskID)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	images := []Image{}
	for rows.Next() {
		i, err := scanImage(rows)
		if err != nil {
			return nil, dbErr(err)
		}
		images = append(images, i)
	}
	return images, dbErr(rows.Err())
}

// DeleteImage removes an image from its task or comment, and returns it as it was.
func (s *Store) DeleteImage(id int64) (*Image, error) {
	i, err := scanImage(s.db.QueryRow(imageSelect+` WHERE id = ?1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("no image %d", id)
	}
	if err != nil {
		return nil, dbErr(err)
	}
	if _, err := s.db.Exec(`DELETE FROM images WHERE id = ?1`, id); err != nil {
		return nil, dbErr(err)
	}
	return &i, nil
}

// Owner says whose images ImageFiles lists.
type Owner int

const (
	OfImage Owner = iota
	OfComment
	OfTask
	OfProject
)

// ImageFiles returns the image files attached to one image, comment, task (with its
// comments) or project.
func (s *Store) ImageFiles(of Owner, id int64) ([]string, error) {
	query := map[Owner]string{
		OfImage:   `SELECT file FROM images WHERE id = ?1`,
		OfComment: `SELECT DISTINCT file FROM images WHERE comment_id = ?1`,
		OfTask:    `SELECT DISTINCT file FROM images WHERE task_id = ?1`,
		OfProject: `SELECT DISTINCT i.file FROM images i JOIN tasks t ON t.id = i.task_id WHERE t.project_id = ?1`,
	}[of]
	rows, err := s.db.Query(query, id)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	files := []string{}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, dbErr(err)
		}
		files = append(files, f)
	}
	return files, dbErr(rows.Err())
}

// UnusedImages returns those of files that no task or comment uses any more.
func (s *Store) UnusedImages(files []string) ([]string, error) {
	unused := []string{}
	for _, f := range files {
		var used bool
		if err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM images WHERE file = ?1)`, f).Scan(&used); err != nil {
			return nil, dbErr(err)
		}
		if !used {
			unused = append(unused, f)
		}
	}
	return unused, nil
}
