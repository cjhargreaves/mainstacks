package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS skills (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		summary TEXT NOT NULL,
		chunks TEXT NOT NULL,
		metadata TEXT NOT NULL
	)`)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Add(sk Skill) error {
	chunks, _ := json.Marshal(sk.Chunks)
	meta, _ := json.Marshal(sk.Metadata)
	_, err := s.db.Exec(
		`INSERT INTO skills (type, source, summary, chunks, metadata) VALUES (?, ?, ?, ?, ?)`,
		string(sk.Type), sk.Source, sk.Summary, string(chunks), string(meta),
	)
	return err
}

func (s *SQLiteStore) Count() int {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM skills`).Scan(&count)
	return count
}

func (s *SQLiteStore) All() []Skill {
	rows, err := s.db.Query(`SELECT type, source, summary, chunks, metadata FROM skills`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLiteStore) ByType(t Type) []Skill {
	rows, err := s.db.Query(`SELECT type, source, summary, chunks, metadata FROM skills WHERE type = ?`, string(t))
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func scanSkills(rows *sql.Rows) []Skill {
	var skills []Skill
	for rows.Next() {
		var sk Skill
		var t, chunksJSON, metaJSON string
		rows.Scan(&t, &sk.Source, &sk.Summary, &chunksJSON, &metaJSON)
		sk.Type = Type(t)
		json.Unmarshal([]byte(chunksJSON), &sk.Chunks)
		json.Unmarshal([]byte(metaJSON), &sk.Metadata)
		skills = append(skills, sk)
	}
	return skills
}
