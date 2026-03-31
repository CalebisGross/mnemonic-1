package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/google/uuid"
)

// JSON-RPC 2.0 types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ProjectResolver resolves paths and names to canonical project names.
type ProjectResolver interface {
	Resolve(input string) string
}

// MemoryDefaults holds shared salience and feedback tuning values.
type MemoryDefaults struct {
	SalienceGeneral       float32
	SalienceDecision      float32
	SalienceError         float32
	SalienceInsight       float32
	SalienceLearning      float32
	SalienceHandoff       float32
	FeedbackStrengthDelta float32
	FeedbackSalienceBoost float32
}

// SalienceForType returns the initial salience for a given memory type.
func (d MemoryDefaults) SalienceForType(memType string) float32 {
	switch memType {
	case "decision":
		return d.SalienceDecision
	case "error":
		return d.SalienceError
	case "insight":
		return d.SalienceInsight
	case "learning":
		return d.SalienceLearning
	case "handoff":
		return d.SalienceHandoff
	default:
		return d.SalienceGeneral
	}
}

// DefaultMemoryDefaults returns the built-in defaults (used when no config override).
func DefaultMemoryDefaults() MemoryDefaults {
	return MemoryDefaults{
		SalienceGeneral:       0.7,
		SalienceDecision:      0.85,
		SalienceError:         0.8,
		SalienceInsight:       0.9,
		SalienceLearning:      0.8,
		SalienceHandoff:       0.95,
		FeedbackStrengthDelta: 0.05,
		FeedbackSalienceBoost: 0.02,
	}
}

// MCPServer implements the Model Context Protocol over JSON-RPC 2.0
type MCPServer struct {
	store           store.Store
	retriever       *retrieval.RetrievalAgent
	bus             events.Bus
	log             *slog.Logger
	version         string // binary version, injected from main
	sessionID       string // auto-generated per MCP server lifetime
	project         string // auto-detected from working directory
	resolver        ProjectResolver
	excludePatterns []string
	maxContentBytes int
	memDefaults     MemoryDefaults // shared salience and feedback tuning

	sessionRecalledIDs map[string]bool // memory IDs already surfaced via recall this session

	// Daemon activity sync (for context_boost in MCP processes)
	daemonURL string // base URL of daemon API (e.g. "http://127.0.0.1:9999")
}

// NewMCPServer creates a new MCP server with the given dependencies.
func NewMCPServer(s store.Store, r *retrieval.RetrievalAgent, bus events.Bus, log *slog.Logger, version string, excludePatterns []string, maxContentBytes int, resolver ProjectResolver, daemonURL string, memDefaults MemoryDefaults) *MCPServer {
	// Auto-detect project from working directory
	wd, _ := os.Getwd()
	var project string
	if resolver != nil {
		project = resolver.Resolve(wd)
	}
	if project == "" {
		project = detectProject()
	}

	// Generate session ID for this MCP server lifetime
	sessionID := fmt.Sprintf("mcp-%s", uuid.New().String()[:8])

	log.Info("MCP server initialized", "session_id", sessionID, "project", project)

	return &MCPServer{
		store:               s,
		retriever:           r,
		bus:                 bus,
		log:                 log,
		version:             version,
		sessionID:           sessionID,
		project:             project,
		resolver:            resolver,
		excludePatterns:    excludePatterns,
		maxContentBytes:    maxContentBytes,
		memDefaults:        memDefaults,
		daemonURL:          daemonURL,
		sessionRecalledIDs: make(map[string]bool),
	}
}

// detectProject determines the project name from the current working directory.
func detectProject() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Use the last path component as project name
	parts := strings.Split(wd, string(os.PathSeparator))
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// resolveProjectName resolves a user-supplied project name through the resolver.
func (srv *MCPServer) resolveProjectName(name string) string {
	if srv.resolver != nil && name != "" {
		if resolved := srv.resolver.Resolve(name); resolved != "" {
			return resolved
		}
	}
	return name
}

// Run starts the MCP server, reading JSON-RPC requests from stdin and writing responses to stdout.
func (srv *MCPServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}

		line := scanner.Bytes()

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			srv.log.Debug("parse error", "error", err)
			if err := enc.Encode(errorResponse(nil, -32700, "Parse error")); err != nil {
				srv.log.Warn("failed to encode error response to stdout", "error", err)
			}
			continue
		}

		srv.log.Debug("received request", "method", req.Method, "id", req.ID)

		resp := srv.handleRequest(ctx, &req)

		// Skip encoding nil responses (for notifications)
		if resp != nil {
			if err := enc.Encode(resp); err != nil {
				srv.log.Warn("failed to encode response to stdout", "error", err)
			}
		}
	}

	// Session ended (stdin closed or context cancelled)
	srv.onSessionEnd(ctx)

	return scanner.Err()
}

// handleRequest dispatches the request to the appropriate handler based on method.
func (srv *MCPServer) handleRequest(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return srv.handleInitialize(req)
	case "notifications/initialized":
		return nil // notifications don't send responses
	case "tools/list":
		return srv.handleToolsList(req)
	case "tools/call":
		return srv.handleToolCall(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "Method not found")
	}
}

// handleInitialize returns the MCP initialization response.
// serverInstructions returns guidance injected into the agent's context each turn.
// Keep concise — this competes for context window space.
const serverInstructions = `Mnemonic is your long-term semantic memory. It persists across sessions — what you store now, future agents can recall.

Session start: call recall_project, then recall with task-relevant keywords.
During work: remember decisions, errors, insights worth preserving. Set the type (decision/error/insight/learning/general).
After recalls: call feedback (helpful/partial/irrelevant) — this trains retrieval ranking via Hebbian learning.
Direct lookup: recall with id parameter accepts any memory ID (from remember or recall results).
Memories are project-scoped and session-tagged automatically. Only store what a future session would need — not file paths or things derivable from code.`

func (srv *MCPServer) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "mnemonic",
			"version": srv.version,
		},
		"instructions": serverInstructions,
	}
	return successResponse(req.ID, result)
}

// ToolAnnotations describes MCP tool behavior hints for clients.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ToolDefinition describes an MCP tool.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Annotations *ToolAnnotations       `json:"annotations,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool { return &b }

// handleToolsList returns the list of available tools.
func (srv *MCPServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	result := map[string]interface{}{
		"tools": allToolDefs(),
	}

	return successResponse(req.ID, result)
}

// toolCallParams represents the parameters for a tools/call request.
type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// handleToolCall dispatches tool calls to their respective handlers.
func (srv *MCPServer) handleToolCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	start := time.Now()
	var result interface{}
	var toolErr error

	switch params.Name {
	case "remember":
		result, toolErr = srv.handleRemember(ctx, params.Arguments)
	case "recall":
		result, toolErr = srv.handleRecall(ctx, params.Arguments)
	case "recall_project":
		result, toolErr = srv.handleRecallProject(ctx, params.Arguments)
	case "batch_recall":
		result, toolErr = srv.handleBatchRecall(ctx, params.Arguments)
	case "feedback":
		result, toolErr = srv.handleFeedback(ctx, params.Arguments)
	case "status":
		result, toolErr = srv.handleStatus(ctx, params.Arguments)
	case "amend":
		result, toolErr = srv.handleAmend(ctx, params.Arguments)
	case "forget":
		result, toolErr = srv.handleForget(ctx, params.Arguments)
	default:
		return errorResponse(req.ID, -32602, fmt.Sprintf("Unknown tool: %s", params.Name))
	}

	// Record tool usage metrics
	srv.recordToolUsage(ctx, params, start, result, toolErr)

	if toolErr != nil {
		return successResponse(req.ID, toolError(toolErr.Error()))
	}

	return successResponse(req.ID, result)
}

// recordToolUsage logs metrics for an MCP tool invocation.
func (srv *MCPServer) recordToolUsage(ctx context.Context, params toolCallParams, start time.Time, result interface{}, toolErr error) {
	rec := store.ToolUsageRecord{
		Timestamp: start,
		ToolName:  params.Name,
		SessionID: srv.sessionID,
		Project:   srv.project,
		LatencyMs: time.Since(start).Milliseconds(),
		Success:   toolErr == nil,
	}
	if toolErr != nil {
		rec.ErrorMessage = toolErr.Error()
	}

	// Extract tool-specific context from arguments
	switch params.Name {
	case "recall", "recall_project", "recall_timeline":
		if q, ok := params.Arguments["query"].(string); ok {
			rec.QueryText = q
		}
	case "remember":
		if t, ok := params.Arguments["type"].(string); ok {
			rec.MemoryType = t
		}
	case "feedback":
		if r, ok := params.Arguments["quality"].(string); ok {
			rec.Rating = r
		}
	}

	// Measure response size and track get_context acceptance.
	if result != nil {
		if respBytes, err := json.Marshal(result); err == nil {
			rec.ResponseSize = len(respBytes)
		}
	}

	if err := srv.store.RecordToolUsage(ctx, rec); err != nil {
		srv.log.Warn("failed to record tool usage", "tool", params.Name, "error", err)
	}
}

// handleRemember stores a new memory in the system.
func (srv *MCPServer) handleRemember(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	text, ok := args["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("text parameter is required and must be a string")
	}

	source := "mcp"
	if s, ok := args["source"].(string); ok {
		source = s
	}

	memType := "general"
	if t, ok := args["type"].(string); ok && t != "" {
		memType = t
	}

	project := srv.project
	if p, ok := args["project"].(string); ok && p != "" {
		project = srv.resolveProjectName(p)
	}

	// Parse optional explicit associations.
	var explicitAssoc []map[string]string
	var invalidAssocIDs []string
	if rawAssoc, ok := args["associate_with"].([]interface{}); ok {
		for _, entry := range rawAssoc {
			if m, ok := entry.(map[string]interface{}); ok {
				memID, _ := m["memory_id"].(string)
				relation, _ := m["relation"].(string)
				if memID != "" && relation != "" {
					// Validate that the target memory exists.
					if _, err := srv.store.GetMemory(ctx, memID); err != nil {
						invalidAssocIDs = append(invalidAssocIDs, memID)
						continue
					}
					explicitAssoc = append(explicitAssoc, map[string]string{
						"memory_id": memID,
						"relation":  relation,
					})
				}
			}
		}
	}

	metadata := map[string]interface{}{
		"mcp_session_id": srv.sessionID,
		"memory_type":    memType,
		"project":        project,
	}
	if len(explicitAssoc) > 0 {
		metadata["explicit_associations"] = explicitAssoc
	}

	raw := store.RawMemory{
		ID:              uuid.New().String(),
		Source:          source,
		Type:            memType,
		Content:         text,
		Timestamp:       time.Now(),
		CreatedAt:       time.Now(),
		HeuristicScore:  0.5,
		InitialSalience: 0.7,
		Processed:       false,
		Project:         project,
		SessionID:       srv.sessionID,
		Metadata:        metadata,
	}

	// Boost salience for specific types
	raw.InitialSalience = srv.memDefaults.SalienceForType(memType)

	if err := srv.store.WriteRaw(ctx, raw); err != nil {
		srv.log.Error("failed to write raw memory", "error", err)
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	// Publish event so encoding agent picks it up
	if err := srv.bus.Publish(ctx, events.RawMemoryCreated{
		ID:     raw.ID,
		Source: raw.Source,
		Ts:     time.Now(),
	}); err != nil {
		srv.log.Warn("failed to publish raw memory created event", "error", err)
	}

	srv.log.Info("memory stored", "id", raw.ID, "source", source, "type", memType, "project", project)

	msg := fmt.Sprintf("Stored memory %s (type: %s, project: %s, salience: %.2f)",
		raw.ID, memType, project, raw.InitialSalience)
	if len(invalidAssocIDs) > 0 {
		msg += fmt.Sprintf("\n\nWarning: %d association target(s) not found and skipped: %s",
			len(invalidAssocIDs), strings.Join(invalidAssocIDs, ", "))
	}
	return toolResult(msg), nil
}

// syncActivityFromDaemon fetches the daemon's watcher activity tracker state
// and loads it into the local retrieval agent. This bridges the gap between
// the daemon process (which runs watchers) and the MCP process (which doesn't).
// Errors are logged but never block recall.
func (srv *MCPServer) syncActivityFromDaemon() {
	if srv.daemonURL == "" {
		return
	}
	resp, err := http.Get(srv.daemonURL + "/api/v1/activity")
	if err != nil {
		srv.log.Debug("activity sync failed", "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return
	}
	var body struct {
		Concepts map[string]time.Time `json:"concepts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		srv.log.Debug("activity sync decode failed", "error", err)
		return
	}
	if len(body.Concepts) > 0 {
		srv.retriever.SyncActivity(body.Concepts)
	}
}

// handleRecall retrieves memories using semantic search and spread activation,
// or does direct lookup by ID.
func (srv *MCPServer) handleRecall(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	// Direct ID lookup — bypass search entirely
	if id, ok := args["id"].(string); ok && id != "" {
		return srv.handleRecallByID(ctx, id)
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query or id parameter is required")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Parse optional filters
	project := ""
	if p, ok := args["project"].(string); ok {
		project = p
	}

	source := ""
	if s, ok := args["source"].(string); ok {
		source = s
	}

	state := ""
	if s, ok := args["state"].(string); ok {
		state = s
	}

	memType := ""
	if t, ok := args["type"].(string); ok {
		memType = t
	}

	var minSalience float32
	if ms, ok := args["min_salience"].(float64); ok {
		minSalience = float32(ms)
	}

	var concepts []string
	if c, ok := args["concepts"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				concepts = append(concepts, s)
			}
		}
	}

	var excludeConcepts []string
	if c, ok := args["exclude_concepts"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				excludeConcepts = append(excludeConcepts, s)
			}
		}
	}

	explain := false
	if e, ok := args["explain"].(bool); ok {
		explain = e
	}

	includeAssociations := false
	if ia, ok := args["include_associations"].(bool); ok {
		includeAssociations = ia
	}

	synthesize := false
	if s, ok := args["synthesize"].(bool); ok {
		synthesize = s
	}

	// Parse types (plural) — overrides type (singular) if set
	if types, ok := args["types"].([]interface{}); ok && len(types) > 0 {
		var typeStrs []string
		for _, v := range types {
			if s, ok := v.(string); ok {
				typeStrs = append(typeStrs, s)
			}
		}
		if len(typeStrs) > 0 {
			memType = strings.Join(typeStrs, ",")
		}
	}

	includePatterns := true
	if ip, ok := args["include_patterns"].(bool); ok {
		includePatterns = ip
	}

	includeAbstractions := true
	if ia, ok := args["include_abstractions"].(bool); ok {
		includeAbstractions = ia
	}

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	// If concepts are specified, use concept-based search (no spread activation available)
	if len(concepts) > 0 {
		memories, err := srv.store.SearchByConcepts(ctx, concepts, limit)
		if err != nil {
			srv.log.Error("concept recall failed", "concepts", concepts, "error", err)
			return nil, fmt.Errorf("concept recall failed: %w", err)
		}
		filtered := filterMemories(memories, source, state, memType, minSalience)
		if len(excludeConcepts) > 0 {
			var kept []store.Memory
			for _, m := range filtered {
				if !conceptOverlap(m.Concepts, excludeConcepts) {
					kept = append(kept, m)
				}
			}
			filtered = kept
		}
		text := fmt.Sprintf("Found %d memories matching concepts %v:\n\n", len(filtered), concepts)
		for i, mem := range filtered {
			text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n\n",
				i+1, mem.ID, mem.Summary, mem.Concepts)
		}
		return toolResult(text), nil
	}

	// All other queries go through the retrieval agent (including project-scoped)
	queryReq := retrieval.QueryRequest{
		Query:               query,
		MaxResults:          limit,
		IncludeReasoning:    true,
		Synthesize:          synthesize,
		IncludePatterns:     includePatterns,
		IncludeAbstractions: includeAbstractions,
		Project:             project,
		Source:              source,
		State:               state,
		Type:                memType,
		MinSalience:         minSalience,
		ExcludeConcepts:     excludeConcepts,
	}

	result, err := srv.retriever.Query(ctx, queryReq)
	if err != nil {
		srv.log.Error("retrieval failed", "query", query, "error", err)
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// Filter patterns and abstractions by exclude_concepts
	if len(excludeConcepts) > 0 {
		var filteredPatterns []store.Pattern
		for _, p := range result.Patterns {
			if !conceptOverlap(p.Concepts, excludeConcepts) {
				filteredPatterns = append(filteredPatterns, p)
			}
		}
		result.Patterns = filteredPatterns

		var filteredAbstractions []store.Abstraction
		for _, a := range result.Abstractions {
			if !conceptOverlap(a.Concepts, excludeConcepts) {
				filteredAbstractions = append(filteredAbstractions, a)
			}
		}
		result.Abstractions = filteredAbstractions
	}

	// Save traversal data and access snapshot for feedback loop
	var retrievedIDs []string
	var snapshot []store.AccessSnapshotEntry
	for i, mem := range result.Memories {
		retrievedIDs = append(retrievedIDs, mem.Memory.ID)
		snapshot = append(snapshot, store.AccessSnapshotEntry{
			MemoryID: mem.Memory.ID,
			Rank:     i + 1,
			Score:    mem.Score,
		})
	}
	fb := store.RetrievalFeedback{
		QueryID:         result.QueryID,
		QueryText:       query,
		RetrievedIDs:    retrievedIDs,
		TraversedAssocs: result.TraversedAssocs,
		AccessSnapshot:  snapshot,
		CreatedAt:       time.Now(),
	}
	// Track recalled IDs for proactive context dedup.
	for _, id := range retrievedIDs {
		srv.sessionRecalledIDs[id] = true
	}

	if err := srv.store.WriteRetrievalFeedback(ctx, fb); err != nil {
		srv.log.Warn("failed to save retrieval feedback record", "query_id", result.QueryID, "error", err)
	}

	text := fmt.Sprintf("Found %d memories (query_id: %s):\n\n", len(result.Memories), result.QueryID)
	for i, mem := range result.Memories {
		projectInfo := ""
		if mem.Memory.Project != "" {
			projectInfo = fmt.Sprintf("\n   Project: %s", mem.Memory.Project)
		}
		contentSnippet := ""
		if mem.Memory.Content != "" && mem.Memory.Content != mem.Memory.Summary {
			contentSnippet = fmt.Sprintf("\n   Content: %s", mem.Memory.Content)
		}
		explanationInfo := ""
		if explain && mem.Explanation != "" {
			explanationInfo = fmt.Sprintf("\n   Explanation: %s", mem.Explanation)
		}
		associationInfo := ""
		if includeAssociations {
			assocs, aErr := srv.store.GetAssociations(ctx, mem.Memory.ID)
			if aErr == nil && len(assocs) > 0 {
				limit := 3
				if len(assocs) < limit {
					limit = len(assocs)
				}
				associationInfo = "\n   Related:"
				for j := 0; j < limit; j++ {
					a := assocs[j]
					targetSummary := a.TargetID[:8]
					if tm, tErr := srv.store.GetMemory(ctx, a.TargetID); tErr == nil {
						targetSummary = tm.Summary
						if len(targetSummary) > 80 {
							targetSummary = targetSummary[:80] + "..."
						}
					}
					associationInfo += fmt.Sprintf("\n     - [%.2f, %s] %s", a.Strength, a.RelationType, targetSummary)
				}
			}
		}
		rawInfo := ""
		if mem.Memory.RawID != "" && mem.Memory.RawID != mem.Memory.ID {
			rawInfo = fmt.Sprintf("\n   Raw ID: %s", mem.Memory.RawID)
		}
		text += fmt.Sprintf("%d. [%.3f] %s\n   Summary: %s%s\n   Created: %s%s%s%s%s\n\n",
			i+1, mem.Score, mem.Memory.ID, mem.Memory.Summary, contentSnippet,
			mem.Memory.CreatedAt.Format("2006-01-02 15:04"), projectInfo, rawInfo, explanationInfo, associationInfo)
	}

	if result.Synthesis != "" {
		text += fmt.Sprintf("Synthesis:\n%s\n", result.Synthesis)
	}

	if len(result.Patterns) > 0 {
		text += fmt.Sprintf("\nRelevant Patterns (%d):\n", len(result.Patterns))
		for _, p := range result.Patterns {
			text += fmt.Sprintf("  - [%s] [strength:%.2f] %s (%s): %s\n", p.ID, p.Strength, p.Title, p.PatternType, p.Description)
		}
	}

	if len(result.Abstractions) > 0 {
		text += "\nApplicable Principles:\n"
		for _, a := range result.Abstractions {
			levelLabel := "principle"
			if a.Level == 3 {
				levelLabel = "axiom"
			}
			text += fmt.Sprintf("  - [%s] [%s, confidence:%.2f] %s: %s\n", a.ID, levelLabel, a.Confidence, a.Title, a.Description)
		}
	}

	srv.log.Info("recall completed", "query", query, "query_id", result.QueryID, "results", len(result.Memories), "patterns", len(result.Patterns), "abstractions", len(result.Abstractions), "took_ms", result.TookMs)

	if outputFormat == "json" {
		var assocMap map[string][]store.Association
		if includeAssociations {
			assocMap = make(map[string][]store.Association, len(result.Memories))
			for _, m := range result.Memories {
				assocs, aErr := srv.store.GetAssociations(ctx, m.Memory.ID)
				if aErr == nil {
					assocMap[m.Memory.ID] = assocs
				}
			}
		}
		jsonResp := formatRecallJSON(result, assocMap)
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult(text), nil // fallback to text
		}
		return toolResult(string(jsonBytes)), nil
	}

	return toolResult(text), nil
}

// handleRecallByID does direct memory lookup by ID.
// Tries encoded memory ID first, then raw memory ID.
func (srv *MCPServer) handleRecallByID(ctx context.Context, id string) (interface{}, error) {
	// Try as encoded memory ID
	mem, err := srv.store.GetMemory(ctx, id)
	if err == nil {
		return toolResult(formatSingleMemory(mem)), nil
	}

	// Try as raw memory ID → look up encoded memory
	mem, err = srv.store.GetMemoryByRawID(ctx, id)
	if err == nil {
		return toolResult(formatSingleMemory(mem)), nil
	}

	// Check if raw memory exists but wasn't encoded (dedup or still pending)
	raw, rawErr := srv.store.GetRaw(ctx, id)
	if rawErr == nil {
		if raw.Processed {
			text := fmt.Sprintf("Memory %s was deduplicated — a similar memory already existed and was boosted instead.\n", id)
			text += fmt.Sprintf("  Content: %s\n", raw.Content)
			return toolResult(text), nil
		}
		text := fmt.Sprintf("Memory %s exists but is still encoding.\n", id)
		text += fmt.Sprintf("  Type: %s\n  Created: %s\n  Content: %s\n",
			raw.Type, raw.CreatedAt.Format("2006-01-02 15:04"), raw.Content)
		return toolResult(text), nil
	}

	return nil, fmt.Errorf("memory not found: %s", id)
}

// formatSingleMemory formats a single memory for text output.
func formatSingleMemory(mem store.Memory) string {
	text := fmt.Sprintf("Memory %s", mem.ID)
	if mem.RawID != "" && mem.RawID != mem.ID {
		text += fmt.Sprintf(" (raw: %s)", mem.RawID)
	}
	text += "\n"
	text += fmt.Sprintf("  Summary: %s\n", mem.Summary)
	if mem.Content != "" && mem.Content != mem.Summary {
		text += fmt.Sprintf("  Content: %s\n", mem.Content)
	}
	text += fmt.Sprintf("  Type: %s\n", mem.Type)
	text += fmt.Sprintf("  Project: %s\n", mem.Project)
	text += fmt.Sprintf("  Salience: %.2f\n", mem.Salience)
	text += fmt.Sprintf("  Created: %s\n", mem.CreatedAt.Format("2006-01-02 15:04"))
	return text
}

// formatRecallJSON builds a structured map from retrieval results.
// assocMap is optional — when non-nil, associations are included per memory.
func formatRecallJSON(result retrieval.QueryResponse, assocMap map[string][]store.Association) map[string]interface{} {
	memories := make([]map[string]interface{}, len(result.Memories))
	for i, m := range result.Memories {
		memories[i] = map[string]interface{}{
			"id":           m.Memory.ID,
			"raw_id":       m.Memory.RawID,
			"score":        m.Score,
			"summary":      m.Memory.Summary,
			"content":      m.Memory.Content,
			"concepts":     m.Memory.Concepts,
			"source":       m.Memory.Source,
			"type":         m.Memory.Type,
			"project":      m.Memory.Project,
			"salience":     m.Memory.Salience,
			"state":        m.Memory.State,
			"access_count": m.Memory.AccessCount,
			"session_id":   m.Memory.SessionID,
			"created_at":   m.Memory.CreatedAt,
			"explanation":  m.Explanation,
		}
		if assocMap != nil {
			if assocs, ok := assocMap[m.Memory.ID]; ok && len(assocs) > 0 {
				limit := 3
				if len(assocs) < limit {
					limit = len(assocs)
				}
				jsonAssocs := make([]map[string]interface{}, limit)
				for j := 0; j < limit; j++ {
					a := assocs[j]
					jsonAssocs[j] = map[string]interface{}{
						"target_id":     a.TargetID,
						"strength":      a.Strength,
						"relation_type": a.RelationType,
					}
				}
				memories[i]["associations"] = jsonAssocs
			}
		}
	}

	patterns := make([]map[string]interface{}, len(result.Patterns))
	for i, p := range result.Patterns {
		patterns[i] = map[string]interface{}{
			"id":          p.ID,
			"title":       p.Title,
			"type":        p.PatternType,
			"strength":    p.Strength,
			"description": p.Description,
		}
	}

	abstractions := make([]map[string]interface{}, len(result.Abstractions))
	for i, a := range result.Abstractions {
		abstractions[i] = map[string]interface{}{
			"id":          a.ID,
			"title":       a.Title,
			"level":       a.Level,
			"confidence":  a.Confidence,
			"description": a.Description,
		}
	}

	return map[string]interface{}{
		"query_id":     result.QueryID,
		"memories":     memories,
		"patterns":     patterns,
		"abstractions": abstractions,
		"synthesis":    result.Synthesis,
		"took_ms":      result.TookMs,
	}
}

// formatMemoriesJSON builds a JSON array from a slice of memories (for recall_project, recall_timeline, recall_session).
func formatMemoriesJSON(memories []store.Memory) []map[string]interface{} {
	result := make([]map[string]interface{}, len(memories))
	for i, m := range memories {
		result[i] = map[string]interface{}{
			"id":           m.ID,
			"raw_id":       m.RawID,
			"summary":      m.Summary,
			"content":      m.Content,
			"concepts":     m.Concepts,
			"source":       m.Source,
			"type":         m.Type,
			"project":      m.Project,
			"salience":     m.Salience,
			"state":        m.State,
			"access_count": m.AccessCount,
			"session_id":   m.SessionID,
			"created_at":   m.CreatedAt,
		}
	}
	return result
}

// formatPatternsJSON builds a JSON array from a slice of patterns.
func formatPatternsJSON(patterns []store.Pattern) []map[string]interface{} {
	result := make([]map[string]interface{}, len(patterns))
	for i, p := range patterns {
		result[i] = map[string]interface{}{
			"title":       p.Title,
			"type":        p.PatternType,
			"strength":    p.Strength,
			"description": p.Description,
			"concepts":    p.Concepts,
			"project":     p.Project,
		}
	}
	return result
}

// handleBatchRecall runs multiple recall queries in parallel and returns combined JSON results.
func (srv *MCPServer) handleBatchRecall(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	queriesRaw, ok := args["queries"].([]interface{})
	if !ok || len(queriesRaw) == 0 {
		return nil, fmt.Errorf("queries parameter is required and must be a non-empty array")
	}

	if len(queriesRaw) > 10 {
		return nil, fmt.Errorf("maximum 10 queries per batch (got %d)", len(queriesRaw))
	}

	type batchResult struct {
		Index int
		Query string
		Data  map[string]interface{}
		Err   error
	}

	results := make(chan batchResult, len(queriesRaw))

	for i, qRaw := range queriesRaw {
		qMap, ok := qRaw.(map[string]interface{})
		if !ok {
			results <- batchResult{Index: i, Err: fmt.Errorf("query %d: invalid format", i)}
			continue
		}

		query, _ := qMap["query"].(string)
		if query == "" {
			results <- batchResult{Index: i, Err: fmt.Errorf("query %d: query string is required", i)}
			continue
		}

		go func(idx int, q string, qArgs map[string]interface{}) {
			limit := 5
			if l, ok := qArgs["limit"].(float64); ok {
				limit = int(l)
			}
			project := ""
			if p, ok := qArgs["project"].(string); ok {
				project = p
			}
			source := ""
			if s, ok := qArgs["source"].(string); ok {
				source = s
			}
			memType := ""
			if t, ok := qArgs["type"].(string); ok {
				memType = t
			}
			var minSalience float32
			if ms, ok := qArgs["min_salience"].(float64); ok {
				minSalience = float32(ms)
			}

			qr, err := srv.retriever.Query(ctx, retrieval.QueryRequest{
				Query:               q,
				MaxResults:          limit,
				IncludeReasoning:    true,
				IncludePatterns:     true,
				IncludeAbstractions: true,
				Project:             project,
				Source:              source,
				Type:                memType,
				MinSalience:         minSalience,
			})
			if err != nil {
				results <- batchResult{Index: idx, Query: q, Err: err}
				return
			}

			results <- batchResult{
				Index: idx,
				Query: q,
				Data:  formatRecallJSON(qr, nil),
			}
		}(i, query, qMap)
	}

	// Collect results in order.
	collected := make([]map[string]interface{}, len(queriesRaw))
	for range queriesRaw {
		r := <-results
		if r.Err != nil {
			collected[r.Index] = map[string]interface{}{
				"query": r.Query,
				"error": r.Err.Error(),
			}
		} else {
			r.Data["query"] = r.Query
			collected[r.Index] = r.Data
		}
	}

	jsonBytes, err := json.Marshal(map[string]interface{}{
		"results": collected,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling batch results: %w", err)
	}

	srv.log.Info("batch recall completed", "queries", len(queriesRaw))
	return toolResult(string(jsonBytes)), nil
}

// formatDuration returns a human-readable short duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// handleStatus returns system statistics and health information.
func (srv *MCPServer) handleStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	stats, err := srv.store.GetStatistics(ctx)
	if err != nil {
		srv.log.Error("failed to get statistics", "error", err)
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	observations, err := srv.store.ListMetaObservations(ctx, "", 5)
	if err != nil {
		srv.log.Warn("failed to get meta observations", "error", err)
		observations = []store.MetaObservation{}
	}

	text := "Mnemonic Status:\n\n"
	text += fmt.Sprintf("Session: %s\n", srv.sessionID)
	text += fmt.Sprintf("Project: %s\n\n", srv.project)
	text += fmt.Sprintf("Total memories: %d\n", stats.TotalMemories)
	text += fmt.Sprintf("Active: %d, Fading: %d, Archived: %d, Merged: %d\n",
		stats.ActiveMemories, stats.FadingMemories, stats.ArchivedMemories, stats.MergedMemories)
	text += fmt.Sprintf("Total associations: %d (avg %.2f per memory)\n",
		stats.TotalAssociations, stats.AvgAssociationsPerMem)
	text += fmt.Sprintf("Storage size: %.2f MB\n", float64(stats.StorageSizeBytes)/(1024*1024))

	if !stats.LastConsolidation.IsZero() {
		text += fmt.Sprintf("Last consolidation: %s\n", stats.LastConsolidation.Format("2006-01-02 15:04:05"))
	}

	// Project breakdown
	projects, err := srv.store.ListProjects(ctx)
	if err == nil && len(projects) > 0 {
		text += fmt.Sprintf("\nProjects (%d):\n", len(projects))
		for _, p := range projects {
			text += fmt.Sprintf("  - %s\n", p)
		}
	}

	// Retrieval quality summary from recent feedback
	feedbacks, err := srv.store.ListMetaObservations(ctx, "retrieval_feedback", 50)
	if err == nil && len(feedbacks) > 0 {
		helpful, partial, irrelevant := 0, 0, 0
		for _, fb := range feedbacks {
			if q, ok := fb.Details["quality"].(string); ok {
				switch q {
				case "helpful":
					helpful++
				case "partial":
					partial++
				case "irrelevant":
					irrelevant++
				}
			}
		}
		total := helpful + partial + irrelevant
		if total > 0 {
			text += fmt.Sprintf("\nRetrieval quality (last %d feedbacks):\n", total)
			text += fmt.Sprintf("  Helpful: %d (%.0f%%), Partial: %d (%.0f%%), Irrelevant: %d (%.0f%%)\n",
				helpful, float64(helpful)/float64(total)*100,
				partial, float64(partial)/float64(total)*100,
				irrelevant, float64(irrelevant)/float64(total)*100)
			if float64(irrelevant)/float64(total) > 0.3 {
				text += "  ⚠ High irrelevant rate — feedback is driving association adjustments\n"
			}
		}
	}

	// Pipeline health: pending encodings
	pendingRaws, pendingErr := srv.store.ListRawUnprocessed(ctx, 100)
	if pendingErr == nil {
		text += "\nEncoding pipeline:\n"
		text += fmt.Sprintf("  Pending: %d raw memories awaiting encoding\n", len(pendingRaws))
		if len(pendingRaws) > 0 {
			oldest := pendingRaws[len(pendingRaws)-1] // last in priority-sorted list = oldest/lowest priority
			text += fmt.Sprintf("  Oldest pending: %s (%s, source: %s)\n", oldest.ID[:8], oldest.CreatedAt.Format("2006-01-02 15:04"), oldest.Source)
			// Count by source
			srcCounts := make(map[string]int)
			for _, r := range pendingRaws {
				srcCounts[r.Source]++
			}
			for src, count := range srcCounts {
				text += fmt.Sprintf("    %s: %d\n", src, count)
			}
		}
	}

	// Source distribution
	srcDist, srcErr := srv.store.GetSourceDistribution(ctx)
	if srcErr == nil && len(srcDist) > 0 {
		text += "\nMemory sources:\n"
		for src, count := range srcDist {
			text += fmt.Sprintf("  %s: %d\n", src, count)
		}
	}

	if len(observations) > 0 {
		text += fmt.Sprintf("\nRecent observations (%d):\n", len(observations))
		for _, obs := range observations {
			text += fmt.Sprintf("  - [%s] %s: %s\n", obs.Severity, obs.ObservationType, obs.CreatedAt.Format("2006-01-02 15:04:05"))
		}
	}

	srv.log.Info("status retrieved", "total_memories", stats.TotalMemories)

	return toolResult(text), nil
}

// handleRecallProject retrieves project-scoped memories with an activity summary.
// Routes through the retrieval agent for spread activation and synthesis.
func (srv *MCPServer) handleRecallProject(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	project := srv.project
	if p, ok := args["project"].(string); ok && p != "" {
		project = srv.resolveProjectName(p)
	}
	if project == "" {
		return nil, fmt.Errorf("project name is required (set project param or run from a project directory)")
	}

	query := ""
	if q, ok := args["query"].(string); ok {
		query = q
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	// Parse optional filters — no default min_salience since all memories
	// are now deliberate MCP memories (watchers removed).
	source, state, memType, minSalience := parseRecallFilters(args)

	// Get project summary
	summary, err := srv.store.GetProjectSummary(ctx, project)
	if err != nil {
		srv.log.Warn("failed to get project summary", "project", project, "error", err)
	}

	// Get patterns for this project
	patterns, err := srv.store.ListPatterns(ctx, project, 5)
	if err != nil {
		srv.log.Warn("failed to get project patterns", "project", project, "error", err)
	}

	text := fmt.Sprintf("Project: %s\n\n", project)

	if summary != nil {
		if total, ok := summary["total_memories"]; ok {
			text += fmt.Sprintf("Total memories: %v\n", total)
		}
		if lastActivity, ok := summary["last_activity"]; ok {
			text += fmt.Sprintf("Last activity: %v\n", lastActivity)
		}
	}

	// Filter patterns to quality threshold
	if len(patterns) > 0 {
		filtered := patterns[:0]
		for _, p := range patterns {
			if p.Strength >= 0.3 {
				filtered = append(filtered, p)
			}
		}
		patterns = filtered
	}
	if len(patterns) > 0 {
		text += fmt.Sprintf("\nPatterns (%d):\n", len(patterns))
		for _, p := range patterns {
			text += fmt.Sprintf("  - [%.2f] %s: %s\n", p.Strength, p.Title, p.Description)
		}
	}

	// Collect memories from either the retrieval agent or recent project search.
	var resultMemories []store.Memory
	var synthesis string

	if query != "" {
		queryReq := retrieval.QueryRequest{
			Query:               query,
			MaxResults:          limit,
			IncludeReasoning:    true,
			Synthesize:          true,
			IncludePatterns:     false, // already fetched above
			IncludeAbstractions: true,
			Project:             project,
			Source:              source,
			State:               state,
			Type:                memType,
			MinSalience:         minSalience,
		}

		result, err := srv.retriever.Query(ctx, queryReq)
		if err != nil {
			srv.log.Error("project recall failed", "project", project, "error", err)
			return nil, fmt.Errorf("project recall failed: %w", err)
		}

		for _, mem := range result.Memories {
			resultMemories = append(resultMemories, mem.Memory)
		}
		synthesis = result.Synthesis
	} else {
		memories, err := srv.store.SearchByProject(ctx, project, "", limit)
		if err != nil {
			srv.log.Error("project recall failed", "project", project, "error", err)
			return nil, fmt.Errorf("project recall failed: %w", err)
		}
		resultMemories = filterMemories(memories, source, state, memType, minSalience)
	}

	srv.log.Info("project recall completed", "project", project)

	if outputFormat == "json" {
		jsonResp := map[string]interface{}{
			"project":  project,
			"summary":  summary,
			"patterns": formatPatternsJSON(patterns),
			"memories": formatMemoriesJSON(resultMemories),
		}
		if synthesis != "" {
			jsonResp["synthesis"] = synthesis
		}
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult(text), nil
		}
		return toolResult(string(jsonBytes)), nil
	}

	// Group memories by type for a structured briefing
	grouped := make(map[string][]store.Memory)
	for _, mem := range resultMemories {
		t := mem.Type
		if t == "" {
			t = "general"
		}
		grouped[t] = append(grouped[t], mem)
	}

	// Sort each group by salience descending
	for _, mems := range grouped {
		sort.Slice(mems, func(i, j int) bool {
			return mems[i].Salience > mems[j].Salience
		})
	}

	// Render grouped briefing — ordered by importance to agents
	typeOrder := []string{"decision", "error", "insight", "learning", "general"}
	typeLabels := map[string]string{
		"decision": "Decisions",
		"error":    "Errors",
		"insight":  "Insights",
		"learning": "Learnings",
		"general":  "General",
	}
	maxPerGroup := 5

	for _, t := range typeOrder {
		mems, ok := grouped[t]
		if !ok || len(mems) == 0 {
			continue
		}
		label := typeLabels[t]
		shown := len(mems)
		if shown > maxPerGroup {
			shown = maxPerGroup
		}
		text += fmt.Sprintf("\n%s (%d):\n", label, len(mems))
		for _, mem := range mems[:shown] {
			summary := mem.Summary
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			text += fmt.Sprintf("  - [%s] %s (%s)\n", mem.ID, summary, mem.CreatedAt.Format("2006-01-02"))
		}
		if len(mems) > maxPerGroup {
			text += fmt.Sprintf("  ... and %d more\n", len(mems)-maxPerGroup)
		}
	}

	return toolResult(text), nil
}

// srv.memDefaults.FeedbackStrengthDelta and srv.memDefaults.FeedbackSalienceBoost are now on srv.memDefaults.

// handleFeedback records quality feedback for a recall result and adjusts association strengths.
func (srv *MCPServer) handleFeedback(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required")
	}

	quality, ok := args["quality"].(string)
	if !ok || quality == "" {
		return nil, fmt.Errorf("quality parameter is required (helpful, partial, or irrelevant)")
	}

	var memoryIDs []string
	if ids, ok := args["memory_ids"].([]interface{}); ok {
		for _, v := range ids {
			if s, ok := v.(string); ok {
				memoryIDs = append(memoryIDs, s)
			}
		}
	}

	queryID, _ := args["query_id"].(string)

	// Store feedback as a meta observation
	obs := store.MetaObservation{
		ID:              uuid.New().String(),
		ObservationType: "retrieval_feedback",
		Severity:        "info",
		Details: map[string]interface{}{
			"query":      query,
			"quality":    quality,
			"memory_ids": memoryIDs,
			"query_id":   queryID,
			"session_id": srv.sessionID,
			"project":    srv.project,
		},
		CreatedAt: time.Now(),
	}

	if err := srv.store.WriteMetaObservation(ctx, obs); err != nil {
		srv.log.Error("failed to write feedback", "error", err)
		return nil, fmt.Errorf("failed to store feedback: %w", err)
	}

	// If query_id is provided, look up traversal data and adjust association strengths
	adjustments := 0
	if queryID != "" {
		fb, err := srv.store.GetRetrievalFeedback(ctx, queryID)
		if err != nil {
			srv.log.Warn("failed to look up retrieval feedback record", "query_id", queryID, "error", err)
		} else {
			// Update the feedback record with the quality rating
			fb.Feedback = quality
			_ = srv.store.WriteRetrievalFeedback(ctx, fb)

			switch quality {
			case "helpful":
				// Strengthen traversed associations and boost returned memory salience
				for _, ta := range fb.TraversedAssocs {
					assocs, err := srv.store.GetAssociations(ctx, ta.SourceID)
					if err != nil {
						continue
					}
					for _, a := range assocs {
						if a.TargetID == ta.TargetID {
							newStrength := a.Strength + srv.memDefaults.FeedbackStrengthDelta
							if newStrength > 1.0 {
								newStrength = 1.0
							}
							if err := srv.store.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
								adjustments++
							}
							break
						}
					}
				}
				// Boost salience of returned memories
				for _, memID := range fb.RetrievedIDs {
					mem, err := srv.store.GetMemory(ctx, memID)
					if err != nil {
						continue
					}
					newSalience := mem.Salience + srv.memDefaults.FeedbackSalienceBoost
					if newSalience > 1.0 {
						newSalience = 1.0
					}
					if err := srv.store.UpdateSalience(ctx, memID, newSalience); err != nil {
						srv.log.Warn("failed to update salience", "memory_id", memID, "error", err)
					}
				}

			case "irrelevant":
				// Weaken traversed associations
				for _, ta := range fb.TraversedAssocs {
					assocs, err := srv.store.GetAssociations(ctx, ta.SourceID)
					if err != nil {
						continue
					}
					for _, a := range assocs {
						if a.TargetID == ta.TargetID {
							newStrength := a.Strength - srv.memDefaults.FeedbackStrengthDelta
							if newStrength < 0.05 {
								newStrength = 0.05
							}
							if err := srv.store.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
								adjustments++
							}
							break
						}
					}
				}
			}
		}
	}

	// Update per-memory feedback scores for auto-suppression.
	// Uses memoryIDs from the feedback call (the memories that were returned).
	suppressionThreshold := -3
	suppressed := 0
	unsuppressed := 0
	for _, memID := range memoryIDs {
		mem, err := srv.store.GetMemory(ctx, memID)
		if err != nil {
			continue
		}
		switch quality {
		case "helpful":
			mem.FeedbackScore++
			// Lift suppression if a helpful rating comes in
			if mem.RecallSuppressed {
				mem.RecallSuppressed = false
				unsuppressed++
			}
		case "irrelevant":
			mem.FeedbackScore--
		}
		// Check suppression threshold
		if mem.FeedbackScore <= suppressionThreshold && !mem.RecallSuppressed {
			mem.RecallSuppressed = true
			suppressed++
		}
		mem.UpdatedAt = time.Now()
		if err := srv.store.UpdateMemory(ctx, mem); err != nil {
			srv.log.Warn("failed to update memory feedback score", "memory_id", memID, "error", err)
		}
	}

	srv.log.Info("feedback recorded", "query", query, "quality", quality, "query_id", queryID, "adjustments", adjustments)

	responseText := fmt.Sprintf("Feedback recorded: %s (query: %q)", quality, query)
	if adjustments > 0 {
		responseText += fmt.Sprintf(" — adjusted %d association strengths", adjustments)
	}
	if suppressed > 0 {
		responseText += fmt.Sprintf(", suppressed %d memories from future recall", suppressed)
	}
	if unsuppressed > 0 {
		responseText += fmt.Sprintf(", un-suppressed %d memories", unsuppressed)
	}

	return toolResult(responseText), nil
}

// Helper functions

// errorResponse creates a JSON-RPC error response.
func errorResponse(id interface{}, code int, message string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
}

// successResponse creates a JSON-RPC success response.
func successResponse(id interface{}, result interface{}) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// toolResult creates an MCP tool result with text content.
func toolResult(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
	}
}

// toolError creates an MCP tool error result.
func toolError(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Error: %s", text),
			},
		},
		"isError": true,
	}
}

// parseRecallFilters extracts optional source/state/min_salience from MCP args.
func parseRecallFilters(args map[string]interface{}) (source, state, memType string, minSalience float32) {
	if s, ok := args["source"].(string); ok {
		source = s
	}
	if s, ok := args["state"].(string); ok {
		state = s
	}
	if t, ok := args["type"].(string); ok {
		memType = t
	}
	if ms, ok := args["min_salience"].(float64); ok {
		minSalience = float32(ms)
	}
	return
}

// filterMemories filters a slice of memories by source, state, and minimum salience.
func filterMemories(memories []store.Memory, source, state, memType string, minSalience float32) []store.Memory {
	var filtered []store.Memory
	for _, m := range memories {
		if source != "" && m.Source != source {
			continue
		}
		if state != "" && m.State != state {
			continue
		}
		if memType != "" && m.Type != memType {
			continue
		}
		if minSalience > 0 && m.Salience < minSalience {
			continue
		}
		if m.RecallSuppressed {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// conceptOverlap returns true if any memory concept matches any excluded concept (case-insensitive).
func conceptOverlap(memoryConcepts, excluded []string) bool {
	for _, mc := range memoryConcepts {
		for _, ec := range excluded {
			if strings.EqualFold(mc, ec) {
				return true
			}
		}
	}
	return false
}

// onSessionEnd is called when stdin closes (Claude Code disconnected) or context is cancelled.
// It records session metadata so future sessions can see what happened.
func (srv *MCPServer) onSessionEnd(ctx context.Context) {
	srv.log.Info("MCP session ending", "session_id", srv.sessionID, "project", srv.project)

	// Count memories created during this session
	memories, err := srv.store.ListMemoriesBySession(ctx, srv.sessionID)
	memCount := 0
	if err == nil {
		memCount = len(memories)
	}

	// Record session end as a meta observation
	obs := store.MetaObservation{
		ID:              fmt.Sprintf("session-end-%s", srv.sessionID),
		ObservationType: "session_end",
		Severity:        "info",
		Details: map[string]interface{}{
			"session_id":       srv.sessionID,
			"project":          srv.project,
			"memories_created": memCount,
			"ended_at":         time.Now().Format(time.RFC3339),
		},
		CreatedAt: time.Now(),
	}
	if err := srv.store.WriteMetaObservation(ctx, obs); err != nil {
		srv.log.Warn("failed to write session end observation", "error", err)
	}

	// Publish session end event for other agents to react to
	if srv.bus != nil {
		_ = srv.bus.Publish(ctx, events.SessionEnded{
			SessionID: srv.sessionID,
			Project:   srv.project,
			Ts:        time.Now(),
		})
	}

	srv.log.Info("MCP session ended", "session_id", srv.sessionID, "memories_created", memCount)
}

// handleAmend updates a memory's content in place, preserving associations and history.
func (srv *MCPServer) handleAmend(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	memoryID, ok := args["memory_id"].(string)
	if !ok || memoryID == "" {
		return nil, fmt.Errorf("memory_id parameter is required")
	}

	correctedContent, ok := args["corrected_content"].(string)
	if !ok || correctedContent == "" {
		return nil, fmt.Errorf("corrected_content parameter is required")
	}

	// Generate a simple summary (first 120 chars of content)
	summary := correctedContent
	if len(summary) > 120 {
		summary = summary[:120] + "..."
	}

	// Use empty concepts and embedding — encoding agent can re-process if needed
	if err := srv.store.AmendMemory(ctx, memoryID, correctedContent, summary, nil, nil); err != nil {
		srv.log.Error("failed to amend memory", "memory_id", memoryID, "error", err)
		return nil, fmt.Errorf("failed to amend memory: %w", err)
	}

	// Publish event
	if srv.bus != nil {
		_ = srv.bus.Publish(ctx, events.MemoryAmended{
			MemoryID:   memoryID,
			NewSummary: summary,
			Ts:         time.Now(),
		})
	}

	srv.log.Info("memory amended", "memory_id", memoryID)
	return toolResult(fmt.Sprintf("Amended memory %s. Content updated, associations and history preserved. Salience bumped +0.05.", memoryID)), nil
}

// handleForget archives a memory, removing it from active recall results.
func (srv *MCPServer) handleForget(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("id parameter is required")
	}

	if err := srv.store.ArchiveMemory(ctx, id); err != nil {
		// Try as raw_id
		if mem, rerr := srv.store.GetMemoryByRawID(ctx, id); rerr == nil {
			if err2 := srv.store.ArchiveMemory(ctx, mem.ID); err2 != nil {
				return nil, fmt.Errorf("failed to archive memory: %w", err2)
			}
			srv.log.Info("memory archived", "memory_id", mem.ID, "via_raw_id", id)
			return toolResult(fmt.Sprintf("Archived memory %s", mem.ID)), nil
		}
		return nil, fmt.Errorf("memory not found: %s", id)
	}

	srv.log.Info("memory archived", "memory_id", id)
	return toolResult(fmt.Sprintf("Archived memory %s", id)), nil
}
