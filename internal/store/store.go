package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"atad-project/internal/links"
)

const (
	maxGenerateAttempts = 5
	busyTimeoutMillis   = 5000
	timeLayout          = time.RFC3339Nano
)

var (
	ErrCodeTaken  = errors.New("code is already taken")
	ErrNotFound   = errors.New("link not found")
	ErrNoFreeCode = errors.New("no free code after retries")
)

const schema = `
CREATE TABLE IF NOT EXISTS links (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  code          TEXT    NOT NULL UNIQUE,
  url           TEXT    NOT NULL,
  created_at    TEXT    NOT NULL,
  visits        INTEGER NOT NULL DEFAULT 0,
  last_visit_at TEXT
);`

type Link struct {
	ID        int64
	Code      string
	URL       string
	CreatedAt time.Time
}

type Stats struct {
	Code        string
	Visits      int64
	LastVisitAt *time.Time
}

type Store struct {
	db       *sql.DB
	generate func() (string, error)
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, busyTimeoutMillis)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db, generate: links.Generate}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(ctx context.Context, code, url string) (Link, error) {
	link := Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO links (code, url, created_at) VALUES (?, ?, ?) RETURNING id`,
		code, url, link.CreatedAt.Format(timeLayout),
	).Scan(&link.ID)
	if isUniqueViolation(err) {
		return Link{}, ErrCodeTaken
	}
	if err != nil {
		return Link{}, err
	}
	return link, nil
}

func (s *Store) CreateGenerated(ctx context.Context, url string) (Link, error) {
	for range maxGenerateAttempts {
		code, err := s.generate()
		if err != nil {
			return Link{}, err
		}
		link, err := s.Create(ctx, code, url)
		if !errors.Is(err, ErrCodeTaken) {
			return link, err
		}
	}
	return Link{}, ErrNoFreeCode
}

func (s *Store) Visit(ctx context.Context, code string) (string, error) {
	var url string
	err := s.db.QueryRowContext(ctx,
		`UPDATE links SET visits = visits + 1, last_visit_at = ? WHERE code = ? RETURNING url`,
		time.Now().UTC().Format(timeLayout), code,
	).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return url, err
}

func (s *Store) Stats(ctx context.Context, code string) (Stats, error) {
	var (
		st   = Stats{Code: code}
		last sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT visits, last_visit_at FROM links WHERE code = ?`, code,
	).Scan(&st.Visits, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return Stats{}, ErrNotFound
	}
	if err != nil {
		return Stats{}, err
	}
	if last.Valid {
		t, err := time.Parse(timeLayout, last.String)
		if err != nil {
			return Stats{}, err
		}
		st.LastVisitAt = &t
	}
	return st, nil
}

func (s *Store) List(ctx context.Context) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, code, url, created_at FROM links ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Link{}
	for rows.Next() {
		var (
			l       Link
			created string
		)
		if err := rows.Scan(&l.ID, &l.Code, &l.URL, &created); err != nil {
			return nil, err
		}
		if l.CreatedAt, err = time.Parse(timeLayout, created); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

func (s *Store) Delete(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE code = ?`, code)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Reset(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM links`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name = 'links'`); err != nil {
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
