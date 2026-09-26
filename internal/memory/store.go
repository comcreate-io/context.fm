package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/comcreate-io/context.fm/internal/queue"
	_ "modernc.org/sqlite"
)

// Store persists play records in SQLite. Single file at
// $CONTEXTFM_DATA_DIR/state.db (default ~/.local/share/context.fm).
// Raw prompt text never enters this store: only derived context labels
// plus track outcomes.
type Store struct {
	db   *sql.DB
	path string
}

// StatePath resolves the database file.
func StatePath() string { return filepath.Join(queue.DataDir(), "state.db") }

// Open creates the schema if needed and returns the store.
func Open() (*Store, error) {
	return OpenPath(StatePath())
}

// OpenPath opens an explicit database file (tests).
func OpenPath(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS play_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		track_id TEXT NOT NULL,
		context_label TEXT NOT NULL,
		outcome TEXT NOT NULL,
		chosen_by TEXT NOT NULL,
		session_id TEXT NOT NULL,
		ts TEXT NOT NULL
	); CREATE INDEX IF NOT EXISTS idx_records_track_label ON play_records(track_id, context_label);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Save appends one observed play.
func (s *Store) Save(r Record) error {
	if r.TrackID == "" {
		return fmt.Errorf("memory: empty track_id")
	}
	ts := r.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO play_records
		(track_id, context_label, outcome, chosen_by, session_id, ts)
		VALUES (?, ?, ?, ?, ?, ?)`,
		r.TrackID, r.ContextLabel, string(r.Outcome), string(r.ChosenBy),
		r.SessionID, ts.UTC().Format(time.RFC3339Nano))
	return err
}

// List returns records for a track+label pair, newest first.
func (s *Store) List(trackID, label string, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT track_id, context_label, outcome, chosen_by, session_id, ts
		FROM play_records WHERE track_id = ? AND context_label = ? ORDER BY id DESC LIMIT ?`,
		trackID, label, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var outcome, chosen, ts string
		if err := rows.Scan(&r.TrackID, &r.ContextLabel, &outcome, &chosen, &r.SessionID, &ts); err != nil {
			return nil, err
		}
		r.Outcome = Outcome(outcome)
		r.ChosenBy = SelectedBy(chosen)
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			r.Timestamp = t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LearnedAffinity loads stored records and scores them with the pure model.
func (s *Store) LearnedAffinity(trackID, label string, now time.Time) (float64, error) {
	records, err := s.List(trackID, label, 500)
	if err != nil {
		return 0, err
	}
	return Affinity(trackID, label, records, now), nil
}

// Count returns total stored records (audit surface for the TUI).
func (s *Store) Count() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM play_records`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Reset deletes all learned records. Associations are resettable by design.
func (s *Store) Reset() error {
	_, err := s.db.Exec(`DELETE FROM play_records`)
	return err
}
