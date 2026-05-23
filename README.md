# hopesandmemes

Agent skill ingestion system powered by Gemini 2.5 Flash + Go.

Drop any file — code, runbooks, infra docs, Terraform, proto — specialist agents ingest, classify, and index it. A router agent answers queries using the right skill.

## Setup

```bash
cp .env.example .env
# Add your Gemini API key
go build ./cmd/hopesandmemes
```

## Architecture

```
Input (file/folder)
      ↓
Classifier Agent (Gemini 2.5 Flash)
      ↓
┌──────┬──────┬──────┬──────┐
Code  Runbook Infra  Proto  Terraform
      ↓
Skill Store (in-memory)
      ↓
Router Agent (Gemini 2.5 Flash)
```

## Usage

```bash
export GEMINI_API_KEY=your-key
./hopesandmemes
```
