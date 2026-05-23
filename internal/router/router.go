package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/cjhargre/hopesandmemes/internal/gemini"
	"github.com/cjhargre/hopesandmemes/internal/skill"
)

type SkillProvider interface {
	All() []skill.Skill
}

type Router struct {
	client *gemini.Client
	store  SkillProvider
}

func New(client *gemini.Client, store SkillProvider) *Router {
	return &Router{client: client, store: store}
}

func (r *Router) Query(ctx context.Context, question string) (string, error) {
	skills := r.store.All()
	if len(skills) == 0 {
		return "", fmt.Errorf("no skills ingested yet")
	}

	var parts []string
	for _, sk := range skills {
		parts = append(parts, fmt.Sprintf("[%s] %s: %s", sk.Type, sk.Source, sk.Summary))
	}

	system := `You are a routing agent. You have access to ingested skills from various sources.
Use the provided skill summaries to answer the user's question accurately.
If multiple skills are relevant, synthesize information from all of them.`

	prompt := fmt.Sprintf("Available skills:\n%s\n\nQuestion: %s",
		strings.Join(parts, "\n"), question)

	return r.client.GenerateWithSystem(ctx, system, prompt)
}
