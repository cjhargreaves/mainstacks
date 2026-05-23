package skill

import "sync"

type Type string

const (
	TypeCode      Type = "code"
	TypeRunbook   Type = "runbook"
	TypeInfra     Type = "infra"
	TypeProto     Type = "proto"
	TypeTerraform Type = "terraform"
	TypeDoc       Type = "doc"
)

type Skill struct {
	Type     Type
	Source   string
	Summary  string
	Chunks   []string
	Metadata map[string]string
}

type Store struct {
	mu     sync.RWMutex
	skills []Skill
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Add(sk Skill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skills = append(s.skills, sk)
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.skills)
}

func (s *Store) All() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, len(s.skills))
	copy(out, s.skills)
	return out
}

func (s *Store) ByType(t Type) []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Skill
	for _, sk := range s.skills {
		if sk.Type == t {
			out = append(out, sk)
		}
	}
	return out
}
