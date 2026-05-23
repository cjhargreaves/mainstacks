package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cjhargre/mainstacks/internal/gemini"
	"github.com/cjhargre/mainstacks/internal/skill"
)

type File struct {
	Path    string
	Content string
}

func IngestRepo(ctx context.Context, client *gemini.Client, dir string) ([]skill.Skill, error) {
	files, err := loadFiles(dir)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in %s", dir)
	}

	// Build a combined view of the codebase
	var codebase strings.Builder
	for _, f := range files {
		content := f.Content
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		codebase.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", f.Path, content))
	}

	system := `You are a skill extraction agent. You analyze entire codebases and identify distinct, reusable skills — concepts, patterns, or capabilities that a developer could apply in future projects.

Rules:
- Do NOT create one skill per file. Group related files together.
- A skill is transferable knowledge, not a file summary.
- Only create skills for meaningful, distinct capabilities.
- A small project might have 1-3 skills. A large one might have 5-8.
- Each skill should be something someone would actually want to reuse.`

	prompt := fmt.Sprintf(`Analyze this codebase and extract the distinct reusable skills/knowledge from it.

Files in project:
%s

Respond with ONLY a valid JSON array of skills:
[
  {
    "name": "short name for the skill",
    "type": "code|runbook|infra|proto|terraform|doc",
    "sources": ["file1.go", "file2.go"],
    "tags": ["tag1", "tag2"],
    "dependencies": ["dep1", "dep2"],
    "summary": "2-3 sentences explaining what this skill is and why it's useful",
    "pattern": "the actual implementation pattern — code snippet, pseudocode, config template, or step-by-step procedure that makes this skill reusable",
    "usage": "how to apply this skill in another project"
  }
]`, codebase.String())

	resp, err := client.GenerateWithSystem(ctx, system, prompt)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}

	// Strip markdown code fences if present
	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var parsed []struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Sources      []string `json:"sources"`
		Tags         []string `json:"tags"`
		Dependencies []string `json:"dependencies"`
		Summary      string   `json:"summary"`
		Pattern      string   `json:"pattern"`
		Usage        string   `json:"usage"`
	}

	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w\nraw: %s", err, resp[:min(len(resp), 200)])
	}

	var skills []skill.Skill
	for _, p := range parsed {
		skills = append(skills, skill.Skill{
			Name:         p.Name,
			Type:         parseType(p.Type),
			Source:       strings.Join(p.Sources, ", "),
			Tags:         p.Tags,
			Dependencies: p.Dependencies,
			Summary:      p.Summary,
			Pattern:      p.Pattern,
			Usage:        p.Usage,
		})
	}

	return skills, nil
}

func parseType(s string) skill.Type {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "code":
		return skill.TypeCode
	case "runbook":
		return skill.TypeRunbook
	case "infra":
		return skill.TypeInfra
	case "proto":
		return skill.TypeProto
	case "terraform":
		return skill.TypeTerraform
	default:
		return skill.TypeDoc
	}
}

func loadFiles(dir string) ([]File, error) {
	var files []File
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden dirs and common noise
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		// Only include known text extensions
		if !isTextFile(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 100_000 {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Skip if content looks binary
		if isBinary(content) {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, File{Path: rel, Content: string(content)})
		return nil
	})
	return files, nil
}

func isTextFile(name string) bool {
	textExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".rs": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true, ".java": true,
		".rb": true, ".php": true, ".swift": true, ".kt": true, ".scala": true,
		".md": true, ".txt": true, ".yaml": true, ".yml": true, ".toml": true,
		".json": true, ".xml": true, ".html": true, ".css": true, ".scss": true,
		".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".tf": true, ".proto": true, ".sql": true, ".graphql": true,
		".dockerfile": true, ".makefile": true, ".cmake": true,
		".env": true, ".cfg": true, ".ini": true, ".conf": true,
	}
	ext := strings.ToLower(filepath.Ext(name))
	if textExts[ext] {
		return true
	}
	// Also allow extensionless files with known names
	lower := strings.ToLower(name)
	return lower == "makefile" || lower == "dockerfile" || lower == "readme" || lower == "license"
}

func isBinary(content []byte) bool {
	// Check first 512 bytes for null bytes
	check := content
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
