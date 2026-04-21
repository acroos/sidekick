package event

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
// and initializes the schema. Use ":memory:" for an in-memory database.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating sqlite schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Append inserts an event and returns its auto-assigned monotonic ID.
func (s *SQLiteStore) Append(ctx context.Context, taskID string, evt *Event) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO events (task_id, type, timestamp, data) VALUES (?, ?, ?, ?)`,
		taskID,
		evt.Type,
		evt.Timestamp.Format(time.RFC3339Nano),
		string(evt.Data),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	evt.ID = id
	evt.TaskID = taskID
	return id, nil
}

// Fetch returns events for a task with ID greater than afterID, up to limit.
func (s *SQLiteStore) Fetch(ctx context.Context, taskID string, afterID int64, limit int) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, task_id, type, timestamp, data FROM events
		 WHERE task_id = ? AND id > ?
		 ORDER BY id
		 LIMIT ?`,
		taskID, afterID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []*Event
	for rows.Next() {
		var (
			evt  Event
			ts   string
			data string
		)
		if err := rows.Scan(&evt.ID, &evt.TaskID, &evt.Type, &ts, &data); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		evt.Timestamp, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp: %w", err)
		}
		evt.Data = json.RawMessage(data)
		events = append(events, &evt)
	}

	return events, rows.Err()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id   TEXT NOT NULL,
			type      TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			data      TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id);
		CREATE INDEX IF NOT EXISTS idx_events_task_id_id ON events(task_id, id);
	`)
	return err
}
