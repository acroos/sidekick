package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver.
)

// SQLiteStore implements Store using a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at the given path
// and initializes the tasks table. Use ":memory:" for an in-memory database.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// For in-memory databases, restrict to a single connection so the pool
	// doesn't open multiple connections (each getting a separate database).
	if dbPath == ":memory:" {
		db.SetMaxOpenConns(1)
	}

	if err := migrateTasksTable(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating tasks schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Create inserts a new task.
func (s *SQLiteStore) Create(ctx context.Context, t *Task) error {
	variables, err := json.Marshal(t.Variables)
	if err != nil {
		return fmt.Errorf("marshaling variables: %w", err)
	}
	steps, err := json.Marshal(t.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, workflow_ref, variables, status, steps, webhook_url, created_at, started_at, completed_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID,
		t.WorkflowRef,
		string(variables),
		string(t.Status),
		string(steps),
		t.WebhookURL,
		t.CreatedAt.Format(time.RFC3339Nano),
		formatTimePtr(t.StartedAt),
		formatTimePtr(t.CompletedAt),
		t.Error,
	)
	if err != nil {
		return fmt.Errorf("inserting task: %w", err)
	}
	return nil
}

// Get returns a task by ID, or nil if not found.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workflow_ref, variables, status, steps, webhook_url, created_at, started_at, completed_at, error
		 FROM tasks WHERE id = ?`, id)

	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}
	return t, nil
}

// List returns tasks matching the filter.
func (s *SQLiteStore) List(ctx context.Context, filter ListFilter) ([]*Task, error) {
	query := `SELECT id, workflow_ref, variables, status, steps, webhook_url, created_at, started_at, completed_at, error FROM tasks`
	var args []any
	var conditions []string

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.WorkflowRef != "" {
		conditions = append(conditions, "workflow_ref = ?")
		args = append(args, filter.WorkflowRef)
	}

	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}

	query += " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Update overwrites the mutable fields of a task.
func (s *SQLiteStore) Update(ctx context.Context, t *Task) error {
	steps, err := json.Marshal(t.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, steps = ?, started_at = ?, completed_at = ?, error = ?
		 WHERE id = ?`,
		string(t.Status),
		string(steps),
		formatTimePtr(t.StartedAt),
		formatTimePtr(t.CompletedAt),
		t.Error,
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("updating task: %w", err)
	}
	return nil
}

// scanner is an interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row *sql.Row) (*Task, error) {
	var (
		t            Task
		variables    string
		steps        string
		status       string
		createdAt    string
		startedAt    sql.NullString
		completedAt  sql.NullString
	)
	err := row.Scan(
		&t.ID, &t.WorkflowRef, &variables, &status, &steps,
		&t.WebhookURL, &createdAt, &startedAt, &completedAt, &t.Error,
	)
	if err != nil {
		return nil, err
	}
	return parseTaskFields(&t, variables, steps, status, createdAt, startedAt, completedAt)
}

func scanTaskRows(rows *sql.Rows) (*Task, error) {
	var (
		t            Task
		variables    string
		steps        string
		status       string
		createdAt    string
		startedAt    sql.NullString
		completedAt  sql.NullString
	)
	err := rows.Scan(
		&t.ID, &t.WorkflowRef, &variables, &status, &steps,
		&t.WebhookURL, &createdAt, &startedAt, &completedAt, &t.Error,
	)
	if err != nil {
		return nil, err
	}
	return parseTaskFields(&t, variables, steps, status, createdAt, startedAt, completedAt)
}

func parseTaskFields(t *Task, variables, steps, status, createdAt string, startedAt, completedAt sql.NullString) (*Task, error) {
	t.Status = Status(status)

	if err := json.Unmarshal([]byte(variables), &t.Variables); err != nil {
		return nil, fmt.Errorf("parsing variables: %w", err)
	}
	if err := json.Unmarshal([]byte(steps), &t.Steps); err != nil {
		return nil, fmt.Errorf("parsing steps: %w", err)
	}

	var err error
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}
	t.StartedAt = parseNullTime(startedAt)
	t.CompletedAt = parseNullTime(completedAt)

	return t, nil
}

func formatTimePtr(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339Nano), Valid: true}
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

func migrateTasksTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id           TEXT PRIMARY KEY,
			workflow_ref TEXT NOT NULL,
			variables    TEXT NOT NULL,
			status       TEXT NOT NULL,
			steps        TEXT NOT NULL,
			webhook_url  TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			started_at   TEXT,
			completed_at TEXT,
			error        TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_workflow_ref ON tasks(workflow_ref);
	`)
	return err
}
