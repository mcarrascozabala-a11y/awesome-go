package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// ErrNotFound is returned when an entry does not exist.
var ErrNotFound = errors.New("poste introuvable")

// Store keeps shift entries in a SQLite database.
type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS entries (
  id             TEXT PRIMARY KEY,
  date           TEXT NOT NULL,
  year           INTEGER NOT NULL,
  week           INTEGER NOT NULL,
  poste          TEXT NOT NULL,
  equipe         TEXT NOT NULL,
  ref            TEXT NOT NULL DEFAULT '',
  pieces         INTEGER NOT NULL DEFAULT 0,
  ouverture      REAL NOT NULL,
  nb_changements INTEGER NOT NULL DEFAULT 0,
  tps_changement REAL NOT NULL DEFAULT 0,
  arrets         TEXT NOT NULL DEFAULT '[]',
  commentaire    TEXT NOT NULL DEFAULT '',
  auteur         TEXT NOT NULL DEFAULT '',
  updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS entries_week ON entries(year, week);
`

// OpenStore opens (and creates if needed) the database at path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Put creates or replaces an entry. e must already be normalized.
func (s *Store) Put(ctx context.Context, e *Entry) error {
	arrets, err := json.Marshal(e.Arrets)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO entries (id,date,year,week,poste,equipe,ref,pieces,ouverture,nb_changements,tps_changement,arrets,commentaire,auteur,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  date=excluded.date, year=excluded.year, week=excluded.week, poste=excluded.poste,
  equipe=excluded.equipe, ref=excluded.ref, pieces=excluded.pieces, ouverture=excluded.ouverture,
  nb_changements=excluded.nb_changements, tps_changement=excluded.tps_changement,
  arrets=excluded.arrets, commentaire=excluded.commentaire, auteur=excluded.auteur,
  updated_at=excluded.updated_at`,
		e.ID, e.Date, e.Year, e.Week, e.Poste, e.Equipe, e.Ref, e.Pieces, e.Ouverture,
		e.NbChangements, e.TpsChangement, string(arrets), e.Commentaire, e.Auteur, e.UpdatedAt)
	return err
}

// Delete removes an entry.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM entries WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const cols = `id,date,year,week,poste,equipe,ref,pieces,ouverture,nb_changements,tps_changement,arrets,commentaire,auteur,updated_at`

// Get returns one entry.
func (s *Store) Get(ctx context.Context, id string) (*Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM entries WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	es, err := scan(rows)
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return nil, ErrNotFound
	}
	return &es[0], nil
}

// Week returns the entries of an ISO week, ordered by date.
func (s *Store) Week(ctx context.Context, year, week int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM entries WHERE year=? AND week=? ORDER BY date, poste`, year, week)
	if err != nil {
		return nil, err
	}
	return scan(rows)
}

// All returns every entry, most recent first.
func (s *Store) All(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM entries ORDER BY date DESC, poste`)
	if err != nil {
		return nil, err
	}
	return scan(rows)
}

// Refs returns the most recently used product references.
func (s *Store) Refs(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ref FROM entries WHERE ref<>'' GROUP BY ref ORDER BY MAX(date) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scan(rows *sql.Rows) ([]Entry, error) {
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var arrets string
		if err := rows.Scan(&e.ID, &e.Date, &e.Year, &e.Week, &e.Poste, &e.Equipe, &e.Ref, &e.Pieces,
			&e.Ouverture, &e.NbChangements, &e.TpsChangement, &arrets, &e.Commentaire, &e.Auteur, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(arrets), &e.Arrets); err != nil {
			return nil, fmt.Errorf("entry %s: %w", e.ID, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
