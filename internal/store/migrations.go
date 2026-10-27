package store

import (
	_ "embed"
	"fmt"
	"strconv"
)

//go:embed schema.sql
var schemaV1 string

// compatibleFrom is the oldest schema version whose kan-do can still use a database at
// the latest one. Adding a table, or a column with a default, leaves older versions
// working, so a board synced from a newer kan-do still opens in an older one. Raise it to
// the new version only for a migration that older code can't live with, like a renamed column.
const compatibleFrom = 4

// migrations are applied in order; index + 1 is the schema_version each one leads to.
var migrations = []string{
	schemaV1,
	// 2: a color picked for the project; NULL leaves the board to choose one.
	`ALTER TABLE projects ADD COLUMN color TEXT;`,
	// 3: a task's tags, comma-separated (see cleanTags).
	`ALTER TABLE tasks ADD COLUMN tags TEXT NOT NULL DEFAULT '';`,
	// 4: any number of repos per project. Each repo belongs to one project. The old
	// projects.repo column stays, empty: SQLite can only drop it by rebuilding the table.
	`CREATE TABLE project_repos (
	     repo       TEXT PRIMARY KEY,
	     project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE
	 );
	 CREATE INDEX project_repos_by_project ON project_repos(project_id);
	 INSERT INTO project_repos (repo, project_id) SELECT repo, id FROM projects WHERE repo IS NOT NULL;
	 UPDATE projects SET repo = NULL;
	 CREATE TRIGGER project_repos_insert AFTER INSERT ON project_repos BEGIN UPDATE meta SET value = value + 1 WHERE key = 'changes'; END;
	 CREATE TRIGGER project_repos_delete AFTER DELETE ON project_repos BEGIN UPDATE meta SET value = value + 1 WHERE key = 'changes'; END;`,
	// 5: images attached to a task, or to one of its comments; the files are in images/.
	`CREATE TABLE images (
	     id         INTEGER PRIMARY KEY AUTOINCREMENT,
	     task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	     comment_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
	     file       TEXT NOT NULL,
	     created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	 );
	 CREATE INDEX images_by_task ON images(task_id);
	 CREATE TRIGGER images_insert AFTER INSERT ON images BEGIN UPDATE meta SET value = value + 1 WHERE key = 'changes'; END;
	 CREATE TRIGGER images_delete AFTER DELETE ON images BEGIN UPDATE meta SET value = value + 1 WHERE key = 'changes'; END;`,
	// 6: tasks for a person, which agents leave alone.
	`ALTER TABLE tasks ADD COLUMN human INTEGER NOT NULL DEFAULT 0;`,
}

// Migrate brings the schema up to date. It writes nothing when the schema already is,
// so starting `kando mcp` for every agent session costs no disk write.
func (s *Store) Migrate() error {
	latest := len(migrations)
	version, okV := metaInt(s.db, "schema_version")
	compatible, okC := metaInt(s.db, "compatible")
	if okV && okC && version == latest && compatible == compatibleFrom {
		return nil
	}
	return s.write(func(q querier) error {
		if _, err := q.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
			return dbErr(err)
		}
		version, _ := metaInt(q, "schema_version")
		if version > latest {
			// From a newer kan-do, maybe synced from another machine: use it as it is
			// when this version can, without touching its schema.
			if oldest, ok := metaInt(q, "compatible"); ok && oldest <= latest {
				return nil
			}
			return invalid("this board was saved by a newer version of kan-do; update this one with `kando update`")
		}
		for i, sql := range migrations[version:] {
			if _, err := q.Exec(sql); err != nil {
				return fmt.Errorf("migration %d: %w", version+i+1, dbErr(err))
			}
		}
		_, err := q.Exec(`INSERT INTO meta (key, value) VALUES ('schema_version', ?1), ('compatible', ?2)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			strconv.Itoa(latest), strconv.Itoa(compatibleFrom))
		return dbErr(err)
	})
}

// metaInt reads a number from meta; false when the table, the key or a number is missing.
func metaInt(q querier, key string) (int, bool) {
	var v string
	if err := q.QueryRow(`SELECT value FROM meta WHERE key = ?1`, key).Scan(&v); err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
