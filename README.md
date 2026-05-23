# mainstacks

A portable skill library for developers. Point it at any codebase and it extracts reusable skills — patterns, techniques, and knowledge — into a personal library you can query and write to any project.

Skills aren't file summaries. They're transferable knowledge: the actual implementation pattern, what it does, when to use it, and what it depends on. Your library grows over time as you ingest more projects, and travels with you everywhere.

## Install

```bash
brew tap cjhargreaves/tap
brew install mainstacks
```

Or build from source:

```bash
git clone https://github.com/cjhargreaves/mainstacks.git
cd mainstacks
go build ./cmd/mainstacks
cp mainstacks ~/.local/bin/
```

## Quick start

```bash
cd ~/my-project
mainstacks
```

First run will ask for a [Google AI API key](https://aistudio.google.com/apikey) (free tier works).

## Usage

### TUI

Run `mainstacks` in any directory to open the interactive interface:

```
⚡ mainstacks (5 skills in library)

→ Ingest repo
  Write skills
  Browse skills
  Quit
```

- **Ingest repo** — scans a directory, extracts skills into your library
- **Write skills** — select skills and write a `SKILLS.md` to the current directory
- **Browse skills** — view, inspect, and delete skills from your library

### CLI

```bash
# Ingest current directory
mainstacks ingest .

# Ingest a specific path
mainstacks ingest ~/projects/my-api

# Ask a question against your skill library
mainstacks query "how do I implement stride-based indexing?"
```

## What's a skill?

```
┌─────────────────────────────────────────────────────┐
│ Name:    gRPC Auth Interceptor                      │
│ Type:    code                                       │
│ Source:  auth.go, middleware.go                     │
│ Tags:    grpc, auth, middleware                     │
│ Deps:    google.golang.org/grpc                     │
│                                                     │
│ Summary: Server-side unary interceptor that         │
│   validates JWT tokens and injects claims           │
│   into the context.                                 │
│                                                     │
│ Pattern:                                            │
│   func AuthInterceptor(ctx context.Context,         │
│     req interface{}, info *grpc.UnaryServerInfo,    │
│     handler grpc.UnaryHandler) (interface{}, error) │
│                                                     │
│ Usage: Add to grpc.NewServer() chain with           │
│   grpc.UnaryInterceptor(AuthInterceptor)            │
└─────────────────────────────────────────────────────┘
```

Skills are grouped by category:

- **Patterns** — reusable code patterns, algorithms, implementations
- **Infrastructure** — deployment, CI/CD, cloud configs
- **Operations** — runbooks, procedures, checklists
- **Design** — architecture decisions, system designs

## How it works

```
cd any-repo && mainstacks ingest .
         ↓
  Reads all text files (skips binaries, node_modules, etc.)
         ↓
  Gemini 2.5 Flash analyzes the ENTIRE codebase at once
         ↓
  Extracts distinct, transferable skills (not one per file)
         ↓
  Stores in ~/.mainstacks/skills.db (persists across projects)
         ↓
  Write selected skills to SKILLS.md in any project
```

## Config

Your API key and settings live at `~/.mainstacks/config`. The skill database lives at `~/.mainstacks/skills.db`. Everything is local — the only external call is to Google's Gemini API during ingestion.

## License

MIT
