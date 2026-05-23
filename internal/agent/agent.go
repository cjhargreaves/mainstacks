package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cjhargre/hopesandmemes/internal/gemini"
	"github.com/cjhargre/hopesandmemes/internal/skill"
)

type File struct {
	Path    string
	Content string
}

type Agent interface {
	Ingest(ctx context.Context, f File) (skill.Skill, error)
}

type Classifier struct {
	client *gemini.Client
}

func NewClassifier(client *gemini.Client) *Classifier {
	return &Classifier{client: client}
}

func (c *Classifier) Classify(ctx context.Context, f File) (skill.Type, error) {
	prompt := fmt.Sprintf(`Classify this file into exactly one category: code, runbook, infra, proto, terraform, doc

Filename: %s
Content (first 2000 chars):
%s

Respond with ONLY the category name, nothing else.`, f.Path, truncate(f.Content, 2000))

	resp, err := c.client.Generate(ctx, prompt)
	if err != nil {
		return skill.TypeDoc, err
	}

	return parseType(strings.TrimSpace(strings.ToLower(resp))), nil
}

func parseType(s string) skill.Type {
	switch {
	case strings.Contains(s, "code"):
		return skill.TypeCode
	case strings.Contains(s, "runbook"):
		return skill.TypeRunbook
	case strings.Contains(s, "infra"):
		return skill.TypeInfra
	case strings.Contains(s, "proto"):
		return skill.TypeProto
	case strings.Contains(s, "terraform"):
		return skill.TypeTerraform
	default:
		return skill.TypeDoc
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// IngestAll fans out ingestion across goroutines.
func IngestAll(ctx context.Context, client *gemini.Client, files []File, store *skill.Store) error {
	classifier := NewClassifier(client)
	var wg sync.WaitGroup
	errs := make(chan error, len(files))

	for _, f := range files {
		wg.Add(1)
		go func(file File) {
			defer wg.Done()

			fileType, err := classifier.Classify(ctx, file)
			if err != nil {
				errs <- fmt.Errorf("classify %s: %w", file.Path, err)
				return
			}

			agent := agentForType(client, fileType)
			sk, err := agent.Ingest(ctx, file)
			if err != nil {
				errs <- fmt.Errorf("ingest %s: %w", file.Path, err)
				return
			}

			store.Add(sk)
		}(f)
	}

	wg.Wait()
	close(errs)

	var combined []string
	for e := range errs {
		combined = append(combined, e.Error())
	}
	if len(combined) > 0 {
		return fmt.Errorf("ingestion errors: %s", strings.Join(combined, "; "))
	}
	return nil
}

func agentForType(client *gemini.Client, t skill.Type) Agent {
	return NewGenericAgent(client, t)
}

func NewGenericAgent(client *gemini.Client, t skill.Type) *GenericAgent {
	return &GenericAgent{client: client, skillType: t}
}
