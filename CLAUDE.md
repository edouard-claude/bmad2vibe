# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build
go build -o bmad2vibe .

# Run (clones BMAD repos from GitHub, writes to ~/.vibe/)
./bmad2vibe

# Specific modules only
./bmad2vibe -modules bmm,cis

# Dry run with verbose output
./bmad2vibe -dry-run -verbose

# Use local source dirs instead of cloning
./bmad2vibe -bundles-dir ~/src/bmad-bundles -method-dir ~/src/BMAD-METHOD
```

No external dependencies — stdlib only. Requires Go 1.24+ and `git` in PATH.

There are no tests in this project.

## Architecture

Single-file CLI (`main.go`, ~1000 lines) that converts **BMAD Method** artifacts into **Mistral Vibe** format, writing output to `~/.vibe/`.

### Conversion pipeline (7 sequential phases)

1. **Agents** — Reads XML bundles from `bmad-bundles/<module>/agents/*.xml`, extracts metadata via regex on the XML opening tag, produces a `.toml` agent config + `.md` system prompt (embeds the full XML verbatim).
2. **Workflows** — Walks `BMAD-METHOD/src/modules/<module>/workflows/`, inlines step files, templates and data into a single `SKILL.md` with YAML frontmatter.
3. **Tasks** — Reads `tasks/*.md`, wraps each in a skill with YAML frontmatter.
4. **Workflow shortcuts** — Generates lightweight agent TOML + prompt pairs that point to a skill. Skips if a persona agent already exists for that slug (avoids overwriting Phase 1 output).
5. **Data** — Copies `data/` and `docs/` directories as-is into skills.
6. **AGENTS.md** — Generates a discovery index by reading all produced TOML files.
7. **Validation** — Cross-refs TOML↔prompt, checks required fields, safety values, prompt size, orphans, skill dirs, workflow shortcut targets. Exits with code 1 on errors.

### Key mappings

- **Safety levels** (`safe`, `neutral`, `destructive`) are hardcoded in `agentSafetyMap` per agent slug. Each level maps to a set of allowed tools in `safetyToolsMap`.
- **Slug convention**: all output artifacts are prefixed `bmad-<module>-` (e.g. `bmad-bmm-architect`).
- **Modules**: default set is `bmm,cis,bmgd` — each is a subdirectory in both source repos.

### Source repos (cloned at runtime)

- `bmad-bundles` — XML agent personas per module
- `BMAD-METHOD` — workflows, tasks, data, docs organized under `src/modules/<module>/`
