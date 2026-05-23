package agent

import (
	"context"
	"fmt"

	"github.com/cjhargre/hopesandmemes/internal/gemini"
	"github.com/cjhargre/hopesandmemes/internal/skill"
)

// GenericAgent handles ingestion for any file type using a type-specific system prompt.
type GenericAgent struct {
	client    *gemini.Client
	skillType skill.Type
}

func (a *GenericAgent) Ingest(ctx context.Context, f File) (skill.Skill, error) {
	system := systemPromptFor(a.skillType)
	prompt := fmt.Sprintf("File: %s\n\nContent:\n%s", f.Path, truncate(f.Content, 4000))

	summary, err := a.client.GenerateWithSystem(ctx, system, prompt)
	if err != nil {
		return skill.Skill{}, err
	}

	return skill.Skill{
		Type:    a.skillType,
		Source:  f.Path,
		Summary: summary,
		Chunks:  chunk(f.Content, 1000),
		Metadata: map[string]string{
			"model": a.client.Model(),
		},
	}, nil
}

func systemPromptFor(t skill.Type) string {
	switch t {
	case skill.TypeCode:
		return "Extract key functions, interfaces, dependencies, and what this service owns. Be concise."
	case skill.TypeRunbook:
		return "Extract steps, commands, services referenced, and any prerequisites. Be concise."
	case skill.TypeInfra:
		return "Extract resources, dependencies, blast radius, and any misconfigurations. Be concise."
	case skill.TypeProto:
		return "Extract service contracts, methods, message types, and breaking change surface area. Be concise."
	case skill.TypeTerraform:
		return "Extract resources, providers, variables, and dependency graph. Be concise."
	default:
		return "Summarize the key information in this document. Be concise."
	}
}

func chunk(s string, size int) []string {
	var chunks []string
	for len(s) > 0 {
		end := size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[:end])
		s = s[end:]
	}
	return chunks
}
