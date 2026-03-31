<p align="center">
  <img src="docs/images/mnemonic.png" alt="mnemonic" width="600">
</p>

# Mnemonic

**Your AI agent remembers everything.**

A local-first semantic memory system for AI agents. No LLM required. No cloud. No config. Just persistent memory that gets smarter over time.

## The Problem

Every AI agent session starts from zero. You explain your project, your decisions, your constraints — then the session ends and it all evaporates. Next session, you explain it again. And again.

## The Fix

Mnemonic gives your AI agent long-term memory. Decisions persist. Errors are remembered. Insights compound. The next agent picks up exactly where the last one left off — automatically.

```
Session start — agent sees this before making any tool call:

  Project: my-app (47 memories)
  Last session: Migrated auth from sessions to JWT. Updated middleware,
    added token refresh endpoint. Tests passing. Still need to update
    the mobile client SDK.
  Contains: 12 decisions, 3 errors, 8 insights, 4 learnings
```

## How It Works

Mnemonic runs as a local daemon and exposes 8 tools via the [Model Context Protocol](https://modelcontextprotocol.io/) (MCP). Any MCP-compatible agent (Claude Code, Cursor, Windsurf, custom agents) can use it.

| Tool | What it does |
|------|-------------|
| `remember` | Store a decision, error, insight, or learning |
| `recall` | Semantic search with spread activation, or direct ID lookup |
| `recall_project` | Structured briefing: decisions, errors, insights grouped by type |
| `batch_recall` | Multiple queries in one round-trip |
| `feedback` | Rate recall quality — trains retrieval via Hebbian learning |
| `forget` | Archive a memory (remove from active recall) |
| `amend` | Update a memory in place (preserves associations) |
| `status` | System health and stats |

**What makes it different:**

- **Zero-call session context** — Project briefing is embedded in the MCP handshake. The agent has context on turn 1, before making any tool call.
- **Semantic search + spread activation** — Finds associated memories, not just keyword matches. Traverses the association graph 3 hops deep.
- **Hebbian learning** — Feedback on recall quality strengthens useful associations and weakens noise. Retrieval gets better over time.
- **Unified IDs** — One memory, one ID. `remember` returns an ID that works in `recall`, `feedback`, `amend`, and `forget`.
- **Context-efficient** — Recall output is compact (truncated content with drill-down). Respects your agent's context window.
- **No LLM required** — Heuristic encoding with RAKE concept extraction. Three embedding providers: bag-of-words (instant, zero dependencies), MiniLM-L6-v2 (pure Go, no CGo), or any OpenAI-compatible API.
- **Local-first** — SQLite + FTS5 + vector search. Air-gapped. Your data stays on your machine.

## Quick Start

**Install:**

```bash
# macOS (Homebrew)
brew install appsprout-dev/tap/mnemonic

# macOS / Linux (manual)
curl -L https://github.com/appsprout-dev/mnemonic/releases/latest/download/mnemonic_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz | tar xz
sudo mv mnemonic /usr/local/bin/
```

Or [build from source](#development) (requires Go 1.23+).

**Run:**

```bash
mnemonic serve        # Foreground (first run — creates ~/.mnemonic/ automatically)
mnemonic install      # Install as background service (launchd / systemd / Windows Services)
```

**Connect to your agent:**

```bash
mnemonic setup claude-code   # Auto-configures ~/.claude/settings.local.json
mnemonic setup cursor        # Auto-configures ~/.cursor/mcp.json
```

That's it. Start a session and your agent has persistent memory.

## Platform Support

| Platform | Daemon |
|----------|--------|
| macOS ARM / x86 | launchd |
| Linux x86_64 | systemd |
| Windows x86_64 | Windows Services |

## Development

```bash
make build    # go build
make test     # go test ./...
make check    # go fmt + go vet
make run      # Build and run in foreground
```

Pure-Go SQLite (`modernc.org/sqlite`) — no CGO required.

See [CLAUDE.md](CLAUDE.md) for the full development guide.

## License

AGPL-3.0. See [LICENSE](LICENSE) for details.
