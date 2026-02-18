# bmad2vibe

[![CI](https://github.com/edouard-claude/bmad2vibe/actions/workflows/ci.yml/badge.svg)](https://github.com/edouard-claude/bmad2vibe/actions/workflows/ci.yml)
[![Release](https://github.com/edouard-claude/bmad2vibe/actions/workflows/release.yml/badge.svg)](https://github.com/edouard-claude/bmad2vibe/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/edouard-claude/bmad2vibe)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/edouard-claude/bmad2vibe)](https://github.com/edouard-claude/bmad2vibe/releases/latest)

I use [BMAD Method](https://github.com/bmad-code-org/BMAD-METHOD) daily — it's a remarkable framework for structured AI-assisted development. [Mistral Vibe](https://mistral.ai/products/vibe) has real potential to support this kind of agent-driven workflow, so I built this tool to bridge the two.

Converts **BMAD Method** agents, workflows, commands and tasks into **Mistral Vibe** format.

## BMAD → Vibe Mapping

| BMAD Artifact | Vibe Concept | Generated Files |
|---|---|---|
| **Agent** (persona XML, ~5-25k lines) | Agent + Prompt | `agents/*.toml` + `prompts/*.md` |
| **Command** (stub `/bmad-bmm-create-prd`) | Workflow shortcut agent | `agents/*.toml` + `prompts/*.md` |
| **Workflow** (multi-step process, `.md` or `.yaml`) | Skill | `skills/*/SKILL.md` (steps + data inlined) |
| **Task/Tool** (`.md` or `.xml`) | Skill | `skills/bmad-*-task-*/SKILL.md` |

## Pipeline (7 phases)

1. **Agents** — XML bundles → TOML (metadata) + MD (full system prompt)
2. **Workflows** → Skills with inlined steps, templates and data
3. **Tasks/Tools** → User-invocable skills
4. **Workflow shortcuts** — lightweight agents for direct invocation (`vibe --agent bmad-bmm-create-prd`)
5. **Data** — docs, CSV, templates copied to `skills/bmad-*-data/`
6. **AGENTS.md** — discovery index for the project root
7. **Validation** — cross-ref TOML↔prompt, required fields, safety, orphans, skills

## Installation

```bash
go build -o bmad2vibe .
```

## Usage

```bash
# Full conversion (clones from GitHub)
./bmad2vibe

# Specific modules only
./bmad2vibe -modules bmm,cis

# Dry run with verbose output
./bmad2vibe -dry-run -verbose

# Local source directories
./bmad2vibe -bundles-dir ~/src/bmad-bundles -method-dir ~/src/BMAD-METHOD
```

Modules are auto-discovered from both source repos. Use `-modules` to override.

## Generated Structure

```
~/.vibe/
├── AGENTS.md                              # Copy to project root
├── agents/
│   ├── bmad-bmm-quick-flow-solo-dev.toml  # Persona agent (Barry)
│   ├── bmad-bmm-pm.toml                   # Persona agent (John)
│   ├── bmad-bmm-architect.toml            # Persona agent (Winston)
│   ├── bmad-bmm-4-implementation-dev-story.toml # Workflow shortcut
│   ├── bmad-core-brainstorming.toml       # Core workflow shortcut
│   └── ...
├── prompts/
│   ├── bmad-bmm-quick-flow-solo-dev.md    # Full system prompt (BMAD XML)
│   ├── bmad-bmm-pm.md
│   └── ...
└── skills/
    ├── bmad-bmm-quick-flow-quick-spec/SKILL.md  # Workflow + inlined steps
    ├── bmad-bmm-4-implementation-dev-story/SKILL.md
    ├── bmad-core-task-shard-doc/SKILL.md         # Standalone task
    ├── bmad-bmm-data/                            # CSV, templates
    └── bmad-bmm-docs/                            # Documentation
```

## Using with Vibe

```bash
# Persona agent (with interactive menu)
vibe --agent bmad-bmm-quick-flow-solo-dev

# Direct workflow (no persona)
vibe --agent bmad-bmm-quick-flow-quick-spec

# Interactive selection
vibe    # then Shift+Tab
```

## Validations

| Check | Description |
|---|---|
| TOML → Prompt | `system_prompt_id` points to an existing `.md` |
| Required fields | `display_name`, `description`, `safety`, `enabled_tools` |
| Safety | Must be `safe`, `neutral`, `destructive`, or `yolo` |
| Prompt size | Warning if < 50 bytes |
| Orphans | Prompts without a matching TOML |
| Skills | Each skill directory has a `SKILL.md` |
| Workflow shortcuts | Referenced skill exists |

## Prerequisites

- Go 1.24+
- `git` (to clone BMAD repos)
- Mistral Vibe installed
