# Mnemonic — Development Guide

Mnemonic is a local-first, air-gapped semantic memory daemon for AI agents. Built in Go, it provides persistent long-term memory via SQLite with FTS5 + vector search, heuristic encoding, and spread activation retrieval. No LLM required.

## Build & Test

```bash
make build                    # go build ...
make test                     # go test ./... -v
make check                    # go fmt + go vet
make run                      # Build and run in foreground (serve mode)
golangci-lint run             # Lint (uses .golangci.yml config)
```

**Version** is injected via ldflags from `Makefile` (managed by release-please). The binary var is in `cmd/mnemonic/main.go`.

## Architecture

### Embedding Pipeline (no LLM)

All encoding uses heuristic Go code — no generative LLM calls anywhere:

```
MCP remember → raw memory → heuristic encoding (RAKE concepts + salience) → embedding (hugot/bow/api) → SQLite + FTS5
MCP recall   → FTS5 + embedding search → spread activation → rank → return
```

Three embedding providers available via `config.yaml`:
- `bow` — 128-dim bag-of-words (instant, zero dependencies)
- `hugot` — 384-dim MiniLM-L6-v2 via pure Go (no CGo, no shared library)
- `api` — OpenAI-compatible endpoint (for cloud embeddings)

### Cognitive Agents

Agents communicate via event bus, never direct calls. Their value is in **side effects** (association strengthening, salience decay, clustering), not text output:

- **Encoding** — Raw memories → concepts + embeddings + associations
- **Retrieval** — FTS5 + vector search + spread activation
- **Consolidation** — Decay salience, merge related memories, prune dead associations
- **Dreaming** — Replay memories, strengthen associations, cross-pollinate
- **Orchestrator** — Schedule agent cycles, health monitoring

### Unified Memory IDs

`remember` returns an ID. That same ID is used everywhere — `recall`, `feedback`, `amend`. One memory, one ID. Old memories (pre-unification) have a separate `raw_id` field; the `recall(id: ...)` lookup handles both transparently.

## Project Layout

```
cmd/mnemonic/          CLI + daemon entry point
cmd/benchmark/         End-to-end benchmark
internal/
  agent/               Cognitive agents + orchestrator + reactor
  api/                 REST API server + routes
  web/                 Embedded dashboard
  mcp/                 MCP server (7 core tools)
  embedding/           Embedding providers (bow, hugot, api) + RAKE + TurboQuant
  store/               Store interface + SQLite implementation
  usage/               Telemetry types (shared across store, embedding, API)
  fsutil/              Filesystem utilities (path matching, binary detection)
  daemon/              Service management (launchd, systemd, Windows Services)
  events/              Event bus (in-memory pub/sub)
  config/              Config loading (config.yaml)
  logger/              Structured logging (slog)
sdk/                   Python agent SDK
migrations/            SQLite schema migrations
```

## MCP Protocol

### Agent Discovery

Tools use MCP protocol metadata for proper agent discovery:
- `_meta["anthropic/alwaysLoad"]` — core tools (remember, recall, recall_project, batch_recall, feedback) load immediately without ToolSearch
- `_meta["anthropic/searchHint"]` — all tools have search hints for keyword discovery
- `annotations` — readOnlyHint, destructiveHint for parallel execution
- `InitializeResult.instructions` — usage guidance injected into agent context each turn

### Tools (7)

| Tool | Purpose |
|------|---------|
| `remember` | Store a memory (auto-tagged with project + session) |
| `recall` | Semantic search, or direct ID lookup via `id` param |
| `recall_project` | Project context at session start |
| `batch_recall` | Multiple queries in one round-trip |
| `feedback` | Rate recall quality (trains Hebbian learning) |
| `status` | System health and stats |
| `amend` | Update a memory in place (preserves ID + associations) |

## Conventions

- **Event bus architecture:** Agents communicate via events, never direct calls.
- **Store interface:** All data access goes through `store.Store` interface.
- **Error handling:** Wrap errors with context: `fmt.Errorf("encoding memory %s: %w", id, err)`
- **Platform-specific code:** Use Go build tags (`//go:build darwin`, `//go:build !darwin`).
- **Config:** All tunables live in `config.yaml`. Add new fields to `internal/config/config.go`.

## Platform Support

| Platform | Status |
|----------|--------|
| macOS ARM | Full support |
| Linux x86_64 | Full support (systemd) |
| Windows x86_64 | Full support (Windows Services) |

## Known Issues

See [GitHub Issues](https://github.com/appsprout-dev/mnemonic/issues) for tracked bugs.
