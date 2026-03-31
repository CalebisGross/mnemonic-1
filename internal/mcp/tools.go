package mcp

// Tool schema definitions for the MCP server.
// Each function returns a ToolDefinition describing one MCP tool.

func rememberToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "remember",
		Description: "Store a memory. Auto-tagged with project and session. Set type: decision, error, insight, learning, or general.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "The memory content to store",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Memory type",
					"enum":        []string{"decision", "error", "insight", "learning", "general"},
				},
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Project name (auto-detected if omitted)",
				},
				"associate_with": map[string]interface{}{
					"type":        "array",
					"description": "Link to existing memories",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"memory_id": map[string]interface{}{
								"type":        "string",
								"description": "Memory ID to associate with",
							},
							"relation": map[string]interface{}{
								"type":        "string",
								"description": "Relation type",
								"enum":        []string{"similar", "caused_by", "part_of", "contradicts", "temporal", "reinforces"},
							},
						},
						"required": []string{"memory_id", "relation"},
					},
				},
			},
			"required": []string{"text"},
		},
		Annotations: &ToolAnnotations{
			Title:           "Remember",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(false),
		},
		Meta: map[string]interface{}{
			"anthropic/alwaysLoad": true,
			"anthropic/searchHint": "store save persist memory decision error insight learning",
		},
	}
}

func recallToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "recall",
		Description: "Semantic search over memories, or direct lookup by ID. Returns ranked results with scores.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search query (required unless id is set)",
				},
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Direct lookup by memory ID or raw ID (skips search)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max results (default: 5)",
				},
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Filter by project",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by type",
					"enum":        []string{"decision", "error", "insight", "learning", "general"},
				},
				"types": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Filter by multiple types (overrides type)",
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Min salience threshold (0.0-1.0)",
				},
				"explain": map[string]interface{}{
					"type":        "boolean",
					"description": "Include score breakdown per result",
				},
				"include_associations": map[string]interface{}{
					"type":        "boolean",
					"description": "Include associated memories (default: false)",
				},
				"include_patterns": map[string]interface{}{
					"type":        "boolean",
					"description": "Include matching patterns (default: true)",
				},
				"include_abstractions": map[string]interface{}{
					"type":        "boolean",
					"description": "Include matching principles (default: true)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text or json",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
		Annotations: &ToolAnnotations{
			Title:        "Recall",
			ReadOnlyHint: boolPtr(true),
		},
		Meta: map[string]interface{}{
			"anthropic/alwaysLoad": true,
			"anthropic/searchHint": "search find retrieve query memories semantic recall remember lookup id",
		},
	}
}

func batchRecallToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "batch_recall",
		Description: "Run multiple recall queries in one request. Ideal for session start.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"queries": map[string]interface{}{
					"type":        "array",
					"description": "Array of recall queries",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "The search query",
							},
							"limit": map[string]interface{}{
								"type":        "integer",
								"description": "Max results (default: 5)",
							},
							"project": map[string]interface{}{
								"type":        "string",
								"description": "Filter by project",
							},
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Filter by memory type",
							},
							"min_salience": map[string]interface{}{
								"type":        "number",
								"description": "Min salience (0.0-1.0)",
							},
						},
						"required": []string{"query"},
					},
				},
			},
			"required": []string{"queries"},
		},
		Annotations: &ToolAnnotations{
			Title:        "Batch Recall",
			ReadOnlyHint: boolPtr(true),
		},
		Meta: map[string]interface{}{
			"anthropic/alwaysLoad": true,
			"anthropic/searchHint": "batch multi-query parallel recall session start memories",
		},
	}
}

func getContextToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "get_context",
		Description: "Get proactive memory suggestions based on recent daemon activity (file edits, terminal commands, clipboard). Call at natural breakpoints to discover relevant context you haven't recalled yet. Returns memories related to what you're currently working on without needing to formulate a query.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"since_minutes": map[string]interface{}{
					"type":        "integer",
					"description": "Look-back window in minutes (default: 10)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum suggestions to return (default: 5)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
	}
}

func forgetToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "forget",
		Description: "Archive one or more memories by ID. Supports single memory_id or bulk memory_ids array.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "Single memory ID to archive",
				},
				"memory_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Array of memory IDs to archive in bulk",
				},
			},
		},
	}
}

func dismissPatternToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "dismiss_pattern",
		Description: "Archive a pattern by ID. Use this to dismiss stale or irrelevant patterns that keep surfacing in recall results.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the pattern to archive",
				},
			},
			"required": []string{"pattern_id"},
		},
	}
}

func dismissAbstractionToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "dismiss_abstraction",
		Description: "Archive an abstraction (principle/axiom) by ID. Use this to dismiss abstract or unhelpful principles that keep surfacing in recall results.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"abstraction_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the abstraction to archive",
				},
			},
			"required": []string{"abstraction_id"},
		},
	}
}

func statusToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "status",
		Description: "Memory system health: stats, encoding pipeline, project breakdown.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
		Annotations: &ToolAnnotations{
			Title:        "Status",
			ReadOnlyHint: boolPtr(true),
		},
		Meta: map[string]interface{}{
			"anthropic/searchHint": "health stats status memory system diagnostics",
		},
	}
}

func recallProjectToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "recall_project",
		Description: "Get project context: recent memories, patterns, and key decisions. Call at session start.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Project name (auto-detected if omitted)",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Optional search query within project",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max memories to return (default: 10)",
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Min salience threshold (0.0-1.0)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text or json",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
		Annotations: &ToolAnnotations{
			Title:        "Recall Project",
			ReadOnlyHint: boolPtr(true),
		},
		Meta: map[string]interface{}{
			"anthropic/alwaysLoad": true,
			"anthropic/searchHint": "project context session start overview decisions patterns",
		},
	}
}

func recallTimelineToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "recall_timeline",
		Description: "Retrieve memories in chronological order within a time range. Useful for reconstructing what happened during a specific period.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hours_back": map[string]interface{}{
					"type":        "integer",
					"description": "How many hours back to look (default: 24)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of memories to return (default: 20)",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory source: mcp, filesystem, terminal, clipboard",
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Minimum salience threshold (0.0-1.0). Filters out low-quality memories.",
				},
				"state": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory state: active, fading, archived",
					"enum":        []string{"active", "fading", "archived"},
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
	}
}

func sessionSummaryToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "session_summary",
		Description: "Summarize the current or most recent session. Shows what was worked on, decisions made, errors encountered, and insights gained.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "Session ID (uses current session if omitted)",
				},
			},
			"required": []string{},
		},
	}
}

func getPatternsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "get_patterns",
		Description: "Retrieve discovered patterns from the memory system. Patterns are recurring themes, practices, or behaviors detected across memories.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Filter by project name",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of patterns to return (default: 10)",
				},
				"min_strength": map[string]interface{}{
					"type":        "number",
					"description": "Minimum pattern strength to return (default: 0.3). Set to 0 for all patterns.",
				},
			},
			"required": []string{},
		},
	}
}

func getInsightsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "get_insights",
		Description: "Return metacognition observations and abstractions. Shows what the memory system has learned about your work patterns, knowledge gaps, and system health.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of insights to return (default: 10)",
				},
			},
			"required": []string{},
		},
	}
}

func feedbackToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "feedback",
		Description: "Rate recall quality. Trains retrieval ranking and association strength.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The original recall query",
				},
				"quality": map[string]interface{}{
					"type":        "string",
					"description": "helpful, partial, or irrelevant",
					"enum":        []string{"helpful", "partial", "irrelevant"},
				},
				"memory_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "IDs of returned memories",
				},
				"query_id": map[string]interface{}{
					"type":        "string",
					"description": "query_id from recall response (enables association tuning)",
				},
			},
			"required": []string{"query", "quality"},
		},
		Annotations: &ToolAnnotations{
			Title:           "Feedback",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(false),
		},
		Meta: map[string]interface{}{
			"anthropic/alwaysLoad": true,
			"anthropic/searchHint": "rate feedback quality recall helpful partial irrelevant",
		},
	}
}

func auditEncodingsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "audit_encodings",
		Description: "Return recent raw→encoded memory pairs for quality review. Shows what the local LLM produced from each raw observation. Use this to spot weak summaries, miscalibrated salience, or concept extraction gaps.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Number of raw→encoded pairs to return (default: 5, max: 20)",
				},
				"hours_back": map[string]interface{}{
					"type":        "integer",
					"description": "How many hours back to look for raw memories (default: 24)",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by source: filesystem, terminal, clipboard, mcp (optional)",
				},
			},
			"required": []string{},
		},
	}
}

func coachLocalLLMToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "coach_local_llm",
		Description: "Write coaching instructions for the local LLM's encoding agent. Writes YAML that improves how the local model encodes memories. Changes take effect after daemon restart.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"coaching_yaml": map[string]interface{}{
					"type":        "string",
					"description": "Full YAML content for the coaching file. Must have a top-level 'coaching' key with an 'encoding' sub-key containing 'notes' and 'instructions'.",
				},
			},
			"required": []string{"coaching_yaml"},
		},
	}
}

func ingestProjectToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "ingest_project",
		Description: "Ingest a local directory into the memory system. Walks the directory, filters binary/excluded files, deduplicates against existing memories, and writes raw memories for encoding. Re-running on the same directory is safe — duplicates are skipped.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directory": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to the directory to ingest",
				},
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Project name (default: directory basename)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, scan and report without writing (default: false)",
				},
			},
			"required": []string{"directory"},
		},
	}
}

func listSessionsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "list_sessions",
		Description: "List recent MCP sessions with metadata (time range, memory count). Useful for finding a specific past session to recall.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum sessions to return (default: 10)",
				},
				"days_back": map[string]interface{}{
					"type":        "integer",
					"description": "How many days back to search (default: 30)",
				},
			},
		},
	}
}

func recallSessionToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "recall_session",
		Description: "Retrieve all memories from a specific MCP session, ordered by creation time. Use \"current\" for the active session, or list_sessions to find past session IDs.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The session ID to recall memories from",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum memories to return (default: 20)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{"session_id"},
		},
	}
}

func excludePathToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "exclude_path",
		Description: "Add a path pattern to the watcher exclusion list. Prevents future watcher events from matching paths. Takes effect on daemon restart. Use list_exclusions to see current patterns.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Path substring to exclude (e.g., '.cache/', 'node_modules/', '/tmp/')",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func listExclusionsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "list_exclusions",
		Description: "List all runtime watcher exclusion patterns (added via exclude_path).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func amendToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "amend",
		Description: "Update a memory's content in place. Preserves ID, associations, and history.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "The memory ID to amend",
				},
				"corrected_content": map[string]interface{}{
					"type":        "string",
					"description": "The updated memory content",
				},
			},
			"required": []string{"memory_id", "corrected_content"},
		},
		Annotations: &ToolAnnotations{
			Title:           "Amend",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(false),
		},
		Meta: map[string]interface{}{
			"anthropic/searchHint": "update correct fix amend edit memory stale",
		},
	}
}

func checkMemoryToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "check_memory",
		Description: "Inspect a memory's encoding status, extracted concepts, associations, and current salience. Use raw_id (from remember) or memory_id to look up a specific memory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"raw_id": map[string]interface{}{
					"type":        "string",
					"description": "The raw memory ID returned by remember",
				},
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "The encoded memory ID",
				},
			},
		},
	}
}

func createHandoffToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "create_handoff",
		Description: "Create a structured session handoff note for the next session. Stored with high salience and automatically surfaced by recall_project. Use at session end to preserve continuity.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"completed": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Tasks completed this session",
				},
				"pending": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Tasks started but not finished",
				},
				"to_test": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Items that need testing",
				},
				"known_issues": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Known bugs or issues discovered",
				},
				"next_session_hint": map[string]interface{}{
					"type":        "string",
					"description": "Suggested starting point for the next session",
				},
			},
		},
	}
}

// ToolCount returns the number of registered MCP tools.
func ToolCount() int {
	return len(allToolDefs())
}

// allToolDefs returns the complete list of MCP tool definitions.
func allToolDefs() []ToolDefinition {
	return []ToolDefinition{
		// Core tools — what agents actually use
		rememberToolDef(),
		recallToolDef(),
		recallProjectToolDef(),
		batchRecallToolDef(),
		feedbackToolDef(),
		statusToolDef(),
		amendToolDef(),
	}
}

// AllToolDefsExtended returns ALL tools including deprecated/dev ones.
// Used for backward compatibility if needed.
func AllToolDefsExtended() []ToolDefinition {
	return []ToolDefinition{
		rememberToolDef(),
		recallToolDef(),
		recallProjectToolDef(),
		batchRecallToolDef(),
		feedbackToolDef(),
		statusToolDef(),
		amendToolDef(),
		// Extended tools (not exposed by default)
		getContextToolDef(),
		forgetToolDef(),
		recallTimelineToolDef(),
		sessionSummaryToolDef(),
		getPatternsToolDef(),
		getInsightsToolDef(),
		auditEncodingsToolDef(),
		coachLocalLLMToolDef(),
		ingestProjectToolDef(),
		listSessionsToolDef(),
		recallSessionToolDef(),
		excludePathToolDef(),
		listExclusionsToolDef(),
		checkMemoryToolDef(),
		dismissPatternToolDef(),
		dismissAbstractionToolDef(),
		createHandoffToolDef(),
	}
}
