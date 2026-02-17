# bmad2vibe

Convertit les agents, workflows, commandes et tasks **BMAD Method** en format **Mistral Vibe**.

## Mapping BMAD → Vibe

| Artefact BMAD | Concept Vibe | Fichiers générés |
|---|---|---|
| **Agent** (persona XML, ~5-25k lignes) | Agent + Prompt | `agents/*.toml` + `prompts/*.md` |
| **Command** (stub `/bmad-bmm-create-prd`) | Workflow shortcut agent | `agents/*.toml` + `prompts/*.md` |
| **Workflow** (multi-step process) | Skill | `skills/*/SKILL.md` (steps + data inlinés) |
| **Task/Tool** (`shard-doc`, `help`) | Skill | `skills/bmad-*-task-*/SKILL.md` |

## Pipeline (7 phases)

1. **Agents** — XML bundles → TOML (meta) + MD (system prompt complet)
2. **Workflows** → Skills avec steps, templates et data inlinés
3. **Tasks/Tools** → Skills user-invocable
4. **Workflow shortcuts** — agents légers pour invocation directe (`vibe --agent bmad-bmm-create-prd`)
5. **Data** — docs, CSV, templates copiés dans `skills/bmad-*-data/`
6. **AGENTS.md** — fichier de découverte pour la racine du projet
7. **Validation** — cross-ref TOML↔prompt, champs requis, safety, orphelins, skills

## Installation

```bash
go build -o bmad2vibe .
```

## Usage

```bash
# Conversion complète (clone depuis GitHub)
./bmad2vibe

# Modules spécifiques
./bmad2vibe -modules bmm,cis

# Dry run
./bmad2vibe -dry-run -verbose

# Sources locales
./bmad2vibe -bundles-dir ~/src/bmad-bundles -method-dir ~/src/BMAD-METHOD
```

## Structure générée

```
~/.vibe/
├── AGENTS.md                              # Copier à la racine du projet
├── agents/
│   ├── bmad-bmm-quick-flow-solo-dev.toml  # Agent persona (Barry)
│   ├── bmad-bmm-pm.toml                   # Agent persona (John)
│   ├── bmad-bmm-architect.toml            # Agent persona (Winston)
│   ├── bmad-bmm-dev.toml                  # Agent persona (Amelia)
│   ├── bmad-bmm-quick-flow-quick-spec.toml # Workflow shortcut
│   ├── bmad-bmm-create-prd.toml           # Workflow shortcut
│   └── ...
├── prompts/
│   ├── bmad-bmm-quick-flow-solo-dev.md    # System prompt complet (XML BMAD)
│   ├── bmad-bmm-pm.md
│   └── ...
└── skills/
    ├── bmad-bmm-quick-flow-quick-spec/SKILL.md  # Workflow + steps inlinés
    ├── bmad-bmm-quick-flow-quick-dev/SKILL.md
    ├── bmad-bmm-task-shard-doc/SKILL.md          # Task standalone
    ├── bmad-bmm-data/                            # CSV, templates
    └── bmad-bmm-docs/                            # Documentation
```

## Utilisation dans Vibe

```bash
# Agent persona (avec menu interactif)
vibe --agent bmad-bmm-quick-flow-solo-dev

# Workflow direct (sans persona)
vibe --agent bmad-bmm-quick-flow-quick-spec

# Sélection interactive
vibe    # puis Shift+Tab
```

## Validations

| Check | Description |
|---|---|
| TOML → Prompt | `system_prompt_id` pointe vers un `.md` existant |
| Champs requis | `display_name`, `description`, `safety`, `enabled_tools` |
| Safety | Parmi `safe`, `neutral`, `destructive`, `yolo` |
| Taille prompt | Alerte si < 50 bytes |
| Orphelins | Prompts sans TOML correspondant |
| Skills | Chaque dir skill a un `SKILL.md` |
| Workflow shortcuts | Le skill référencé existe |

## Pré-requis

- Go 1.24+
- `git` (pour cloner les repos BMAD)
- Mistral Vibe installé