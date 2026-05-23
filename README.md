# mainstacks

A portable skill library for developers. Ingest any codebase, extract reusable knowledge as "skills," and write them to any project as a `SKILLS.md`.

## Install

```bash
brew install mainstacks
```

## How it works

```bash
cd my-project
mainstacks
```

That's it. You get a TUI with three options:

```
⚡ mainstacks (12 skills in library)

→ Ingest this repo        # scan current directory, extract skills
  Write skills            # pick skills from your library → writes SKILLS.md here
  Browse skills           # see everything in your library
```

### Ingest

Scans the current directory. Gemini 2.5 Flash classifies each file and specialist agents extract structured skills — what it does, how to use it, dependencies, tags. Skills are saved to your personal library (persists across projects).

### Write skills

Pick skills from your library to include in this project. Writes a `SKILLS.md` file to the current directory — a portable knowledge doc that anyone on the team can read, or that AI tools can use as context.

### Browse skills

View all skills in your library with type, tags, and a short description.

## What's a skill?

A skill is a structured piece of knowledge extracted from code:

```
┌─────────────────────────────────────────┐
│ Name: gRPC Auth Interceptor             │
│ Type: code                              │
│ Source: payments-service/auth.go        │
│ Tags: [grpc, auth, middleware]          │
│ Dependencies: [google.golang.org/grpc]  │
│ Summary: Server-side unary interceptor  │
│   that validates JWT tokens from the    │
│   metadata header and injects user      │
│   claims into the context.              │
│ Usage: Add to grpc.NewServer() chain    │
│   with grpc.UnaryInterceptor(AuthFn)    │
└─────────────────────────────────────────┘
```

## Architecture

```
cd any-repo && mainstacks
         ↓
  Classifier Agent (Gemini 2.5 Flash)
         ↓
  ┌──────┬──────┬──────┬──────┐
  Code  Runbook Infra  Proto  Terraform
  Agent  Agent  Agent  Agent   Agent
         ↓
  Skill Library (~/.mainstacks/skills.db)
         ↓
  Write to SKILLS.md in any project
```

## Setup (for development)

```bash
cp .env.example .env
# Add your Gemini API key
go build ./cmd/mainstacks
```

## Config

Set your Gemini API key once:

```bash
export GEMINI_API_KEY=your-key
```

Or add it to `~/.mainstacks/config`.
