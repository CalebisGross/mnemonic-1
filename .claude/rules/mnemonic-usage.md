# Mnemonic MCP Tool Usage

## Tools (7)

| Tool | Purpose |
|------|---------|
| `remember` | Store a memory (auto-tagged with project + session) |
| `recall` | Semantic search, or direct ID lookup via `id` param |
| `recall_project` | Project context at session start |
| `batch_recall` | Multiple queries in one round-trip |
| `feedback` | Rate recall quality (trains Hebbian learning) |
| `status` | System health and stats |
| `amend` | Update a memory in place (preserves ID + associations) |

## Session Start

1. Call `recall_project` to load project context
2. Call `recall` with keywords relevant to the user's request
3. If useful context found, use it. If not, move on.

Alternative: `batch_recall` to combine project context + task-specific queries.

For trivial tasks: skip recall, just do the work.

## During Work

### Remember (be selective)

Only store things a future session would need:

- **Decisions**: "chose X because Y" — `type: "decision"`
- **Errors**: bugs found and how they were fixed — `type: "error"`
- **Insights**: non-obvious discoveries — `type: "insight"`
- **Learnings**: API/framework behavior — `type: "learning"`

Do NOT remember: file paths, trivial changes, things derivable from git history or code.

### Recall mid-session

When entering unfamiliar territory, recall before assuming. Check if there's a prior decision or known issue.

### Direct ID lookup

Use `recall(id: "...")` to look up any memory by ID. Works with IDs from `remember` responses or `recall` results. If a memory was deduplicated, this will tell you.

### Amend stale memories

If recall returns outdated info, use `amend` to fix it in place. This preserves associations.

## After Recalls

Call `feedback` after acting on recall results:

- `helpful` — memories informed your work
- `partial` — some useful, some noise
- `irrelevant` — didn't help

This trains retrieval. Skipping it degrades future quality.

## What NOT to Do

- Don't store experiment results in memory — those go in docs
- Don't remember things that belong in code comments or commit messages
- Don't create memories about file structure or architecture — read the code instead
