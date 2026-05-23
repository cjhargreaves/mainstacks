package skill

import (
	"database/sql"
	"fmt"
	"strings"

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
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		tags TEXT NOT NULL DEFAULT '',
		dependencies TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL,
		pattern TEXT NOT NULL DEFAULT '',
		usage_info TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Add(sk Skill) error {
	_, err := s.db.Exec(
		`INSERT INTO skills (name, type, source, tags, dependencies, summary, pattern, usage_info) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sk.Name, string(sk.Type), sk.Source,
		strings.Join(sk.Tags, ","),
		strings.Join(sk.Dependencies, ","),
		sk.Summary, sk.Pattern, sk.Usage,
	)
	return err
}

func (s *SQLiteStore) Exists(name string) bool {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM skills WHERE name = ? OR source = ?`, name, name).Scan(&count)
	return count > 0
}

func (s *SQLiteStore) ExistsBySource(source string) bool {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM skills WHERE source = ?`, source).Scan(&count)
	return count > 0
}

func (s *SQLiteStore) Count() int {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM skills`).Scan(&count)
	return count
}

func (s *SQLiteStore) All() []Skill {
	rows, err := s.db.Query(`SELECT name, type, source, tags, dependencies, summary, pattern, usage_info FROM skills`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLiteStore) ByType(t Type) []Skill {
	rows, err := s.db.Query(`SELECT name, type, source, tags, dependencies, summary, pattern, usage_info FROM skills WHERE type = ?`, string(t))
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Delete(name string) error {
	_, err := s.db.Exec(`DELETE FROM skills WHERE name = ?`, name)
	return err
}

func scanSkills(rows *sql.Rows) []Skill {
	var skills []Skill
	for rows.Next() {
		var sk Skill
		var t, tags, deps string
		rows.Scan(&sk.Name, &t, &sk.Source, &tags, &deps, &sk.Summary, &sk.Pattern, &sk.Usage)
		sk.Type = Type(t)
		if tags != "" {
			sk.Tags = strings.Split(tags, ",")
		}
		if deps != "" {
			sk.Dependencies = strings.Split(deps, ",")
		}
		skills = append(skills, sk)
	}
	return skills
}
