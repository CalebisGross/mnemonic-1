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

func forgetToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "forget",
		Description: "Archive a memory by ID. Removes it from active recall results.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Memory ID to archive",
				},
			},
			"required": []string{"id"},
		},
		Annotations: &ToolAnnotations{
			Title:           "Forget",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(true),
		},
		Meta: map[string]interface{}{
			"anthropic/searchHint": "forget archive remove delete memory cleanup",
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
		rememberToolDef(),
		recallToolDef(),
		recallProjectToolDef(),
		batchRecallToolDef(),
		feedbackToolDef(),
		statusToolDef(),
		amendToolDef(),
		forgetToolDef(),
	}
}
