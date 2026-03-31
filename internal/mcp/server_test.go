package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/store/storetest"
)

// mockStore embeds the shared base mock with optional overrides for testing.
type mockStore struct {
	storetest.MockStore
	getMemoryFunc      func(ctx context.Context, id string) (store.Memory, error)
	getMemoryByRawFunc func(ctx context.Context, rawID string) (store.Memory, error)
	getRawFunc         func(ctx context.Context, id string) (store.RawMemory, error)
	archiveMemoryFunc  func(ctx context.Context, id string) error
	searchByProjectFunc func(ctx context.Context, project, query string, limit int) ([]store.Memory, error)
}

func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	if m.getMemoryFunc != nil {
		return m.getMemoryFunc(ctx, id)
	}
	return m.MockStore.GetMemory(ctx, id)
}

func (m *mockStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	if m.getMemoryByRawFunc != nil {
		return m.getMemoryByRawFunc(ctx, rawID)
	}
	return m.MockStore.GetMemoryByRawID(ctx, rawID)
}

func (m *mockStore) GetRaw(ctx context.Context, id string) (store.RawMemory, error) {
	if m.getRawFunc != nil {
		return m.getRawFunc(ctx, id)
	}
	return m.MockStore.GetRaw(ctx, id)
}

func (m *mockStore) ArchiveMemory(ctx context.Context, id string) error {
	if m.archiveMemoryFunc != nil {
		return m.archiveMemoryFunc(ctx, id)
	}
	return m.MockStore.ArchiveMemory(ctx, id)
}

func (m *mockStore) SearchByProject(ctx context.Context, project, query string, limit int) ([]store.Memory, error) {
	if m.searchByProjectFunc != nil {
		return m.searchByProjectFunc(ctx, project, query, limit)
	}
	return m.MockStore.SearchByProject(ctx, project, query, limit)
}

// mockBus is a minimal mock of the Bus interface for testing.
type mockBus struct{}

func (m *mockBus) Publish(ctx context.Context, event events.Event) error { return nil }
func (m *mockBus) Subscribe(eventType string, handler events.Handler) string {
	return "test-sub-id"
}
func (m *mockBus) Unsubscribe(subscriptionID string) {}
func (m *mockBus) Close() error                      { return nil }

// TestHandleInitialize tests handleInitialize returns correct protocol version and server info.
func TestHandleInitialize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := srv.handleInitialize(req)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 1 {
		t.Fatalf("expected ID 1, got %v", resp.ID)
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Round-trip through JSON to get standard Go types
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if protocolVersion, ok := result["protocolVersion"]; !ok {
		t.Fatal("protocolVersion not in result")
	} else if protocolVersion != "2024-11-05" {
		t.Fatalf("expected protocol version 2024-11-05, got %v", protocolVersion)
	}

	// Check serverInfo
	if serverInfo, ok := result["serverInfo"]; !ok {
		t.Fatal("serverInfo not in result")
	} else {
		serverInfoMap := serverInfo.(map[string]interface{})
		if serverInfoMap["name"] != "mnemonic" {
			t.Fatalf("expected server name 'mnemonic', got %v", serverInfoMap["name"])
		}
		if serverInfoMap["version"] != "test" {
			t.Fatalf("expected server version 'test', got %v", serverInfoMap["version"])
		}
	}
}

// TestHandleToolsList tests handleToolsList returns all 10 tools.
func TestHandleToolsList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := srv.handleToolsList(req)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Round-trip through JSON to get standard Go types (like a real MCP client)
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	toolsInterface, ok := result["tools"]
	if !ok {
		t.Fatal("tools not in result")
	}

	toolsArray, ok := toolsInterface.([]interface{})
	if !ok {
		t.Fatalf("tools is not an array, got %T", toolsInterface)
	}

	if len(toolsArray) != 8 {
		t.Fatalf("expected 8 core tools, got %d", len(toolsArray))
	}

	// Verify core tool names
	expectedTools := map[string]bool{
		"remember":       false,
		"recall":         false,
		"batch_recall":   false,
		"recall_project": false,
		"feedback":       false,
		"status":         false,
		"amend":          false,
		"forget":         false,
	}

	for _, toolInterface := range toolsArray {
		toolMap := toolInterface.(map[string]interface{})
		toolName := toolMap["name"].(string)
		if _, ok := expectedTools[toolName]; ok {
			expectedTools[toolName] = true
		} else {
			t.Fatalf("unexpected tool: %s", toolName)
		}
	}

	// Verify all expected tools were found
	for toolName, found := range expectedTools {
		if !found {
			t.Fatalf("tool %s not found", toolName)
		}
	}
}

// TestJSONRPCErrorResponse tests JSON-RPC error response format.
func TestJSONRPCErrorResponse(t *testing.T) {
	resp := errorResponse(42, -32601, "Method not found")

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 42 {
		t.Fatalf("expected ID 42, got %v", resp.ID)
	}

	if resp.Error == nil {
		t.Fatal("expected error object")
	}

	if resp.Error.Code != -32601 {
		t.Fatalf("expected code -32601, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Method not found" {
		t.Fatalf("expected message 'Method not found', got %s", resp.Error.Message)
	}

	if resp.Result != nil {
		t.Fatalf("expected nil result, got %v", resp.Result)
	}
}

// TestToolResult tests toolResult helper function.
func TestToolResult(t *testing.T) {
	text := "Test result text"
	result := toolResult(text)

	contentInterface, ok := result["content"]
	if !ok {
		t.Fatal("content not in result")
	}

	contentArray, ok := contentInterface.([]map[string]interface{})
	if !ok {
		t.Fatal("content is not an array of maps")
	}

	if len(contentArray) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contentArray))
	}

	item := contentArray[0]
	if item["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", item["type"])
	}

	if item["text"] != text {
		t.Fatalf("expected text %q, got %q", text, item["text"])
	}
}

// TestToolError tests toolError helper function.
func TestToolError(t *testing.T) {
	errorText := "Test error"
	result := toolError(errorText)

	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result["isError"])
	}

	contentInterface, ok := result["content"]
	if !ok {
		t.Fatal("content not in result")
	}

	contentArray, ok := contentInterface.([]map[string]interface{})
	if !ok {
		t.Fatal("content is not an array of maps")
	}

	if len(contentArray) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contentArray))
	}

	item := contentArray[0]
	if item["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", item["type"])
	}

	expectedText := "Error: " + errorText
	if item["text"] != expectedText {
		t.Fatalf("expected text %q, got %q", expectedText, item["text"])
	}
}

// TestSuccessResponse tests successResponse helper function.
func TestSuccessResponse(t *testing.T) {
	testResult := map[string]interface{}{"key": "value"}
	resp := successResponse(99, testResult)

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 99 {
		t.Fatalf("expected ID 99, got %v", resp.ID)
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Verify result
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	if resultMap["key"] != "value" {
		t.Fatalf("expected key=value in result, got %v", resultMap)
	}
}

// TestHandleRequestDispatch tests that handleRequest correctly dispatches to handlers.
func TestHandleRequestDispatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	tests := []struct {
		method  string
		wantErr bool
	}{
		{"initialize", false},
		{"tools/list", false},
		{"notifications/initialized", false}, // Returns nil response
		{"invalid_method", true},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			req := &jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  tc.method,
			}

			resp := srv.handleRequest(context.Background(), req)

			if tc.method == "notifications/initialized" {
				if resp != nil {
					t.Fatal("expected nil response for notifications/initialized")
				}
			} else {
				if resp == nil {
					t.Fatal("expected non-nil response")
				}

				if tc.wantErr {
					if resp.Error == nil {
						t.Fatal("expected error in response")
					}
				} else {
					if resp.Error != nil {
						t.Fatalf("expected no error, got %v", resp.Error)
					}
				}
			}
		})
	}
}

// TestJSONRPCMarshal tests that responses can be marshalled to JSON correctly.
func TestJSONRPCMarshal(t *testing.T) {
	resp := errorResponse(1, -32700, "Parse error")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify it's valid JSON
	var unmarshalled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if unmarshalled["jsonrpc"] != "2.0" {
		t.Fatalf("jsonrpc field mismatch")
	}

	if unmarshalled["id"] != float64(1) {
		t.Fatalf("id field mismatch")
	}

	errorObj, ok := unmarshalled["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error field is not a map")
	}

	if errorObj["code"] != float64(-32700) {
		t.Fatalf("error code mismatch")
	}

	if errorObj["message"] != "Parse error" {
		t.Fatalf("error message mismatch")
	}
}

// TestFormatDuration tests human-readable duration formatting.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 3*time.Minute + 30*time.Second, "3m"},
		{"hours and minutes", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"zero", 0, "0s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatDuration(tc.d)
			if got != tc.expected {
				t.Fatalf("formatDuration(%v) = %q, want %q", tc.d, got, tc.expected)
			}
		})
	}
}

// TestRecallByID tests direct memory lookup by encoded ID and raw ID.
func TestRecallByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	testMem := store.Memory{
		ID:        "mem-123",
		RawID:     "raw-456",
		Summary:   "Test decision",
		Content:   "Chose X because Y",
		Type:      "decision",
		Project:   "test-project",
		Salience:  0.85,
		State:     "active",
		CreatedAt: time.Now(),
	}

	ms := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			if id == "mem-123" {
				return testMem, nil
			}
			return store.Memory{}, store.ErrNotFound
		},
		getMemoryByRawFunc: func(_ context.Context, rawID string) (store.Memory, error) {
			if rawID == "raw-456" {
				return testMem, nil
			}
			return store.Memory{}, store.ErrNotFound
		},
		getRawFunc: func(_ context.Context, id string) (store.RawMemory, error) {
			return store.RawMemory{}, store.ErrNotFound
		},
	}

	srv := NewMCPServer(ms, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	t.Run("by encoded ID", func(t *testing.T) {
		result, err := srv.handleRecallByID(context.Background(), "mem-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := extractText(result)
		if !contains(text, "mem-123") || !contains(text, "Test decision") {
			t.Fatalf("expected memory content in result, got: %s", text)
		}
	})

	t.Run("by raw ID", func(t *testing.T) {
		result, err := srv.handleRecallByID(context.Background(), "raw-456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := extractText(result)
		if !contains(text, "mem-123") || !contains(text, "Test decision") {
			t.Fatalf("expected memory content in result, got: %s", text)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := srv.handleRecallByID(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent ID")
		}
	})
}

// TestRecallByIDDedup tests that deduplicated memories are reported correctly.
func TestRecallByIDDedup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ms := &mockStore{
		getMemoryFunc: func(_ context.Context, _ string) (store.Memory, error) {
			return store.Memory{}, store.ErrNotFound
		},
		getMemoryByRawFunc: func(_ context.Context, _ string) (store.Memory, error) {
			return store.Memory{}, store.ErrNotFound
		},
		getRawFunc: func(_ context.Context, id string) (store.RawMemory, error) {
			if id == "deduped-123" {
				return store.RawMemory{
					ID:        "deduped-123",
					Content:   "This was deduplicated",
					Processed: true,
				}, nil
			}
			return store.RawMemory{}, store.ErrNotFound
		},
	}

	srv := NewMCPServer(ms, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	result, err := srv.handleRecallByID(context.Background(), "deduped-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(result)
	if !contains(text, "deduplicated") {
		t.Fatalf("expected dedup message, got: %s", text)
	}
}

// TestForgetHandler tests the forget tool archives memories correctly.
func TestForgetHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	archived := map[string]bool{}

	ms := &mockStore{
		archiveMemoryFunc: func(_ context.Context, id string) error {
			if id == "mem-123" {
				archived[id] = true
				return nil
			}
			return store.ErrNotFound
		},
		getMemoryByRawFunc: func(_ context.Context, rawID string) (store.Memory, error) {
			if rawID == "raw-456" {
				return store.Memory{ID: "mem-123"}, nil
			}
			return store.Memory{}, store.ErrNotFound
		},
	}

	srv := NewMCPServer(ms, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	t.Run("by memory ID", func(t *testing.T) {
		result, err := srv.handleForget(context.Background(), map[string]interface{}{"id": "mem-123"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := extractText(result)
		if !contains(text, "Archived") {
			t.Fatalf("expected archive confirmation, got: %s", text)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		_, err := srv.handleForget(context.Background(), map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error for missing id")
		}
	})
}

// TestInitializeIncludesInstructions tests that initialize returns server instructions.
func TestInitializeIncludesInstructions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())

	resp := srv.handleInitialize(&jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})

	data, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)

	instructions, ok := result["instructions"].(string)
	if !ok || instructions == "" {
		t.Fatal("expected non-empty instructions in initialize response")
	}
	if !contains(instructions, "Mnemonic is your long-term semantic memory") {
		t.Fatalf("instructions missing expected content: %s", instructions[:100])
	}
}

// TestInitializeWithProjectBriefing tests that initialize embeds project context.
func TestInitializeWithProjectBriefing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ms := &mockStore{
		searchByProjectFunc: func(_ context.Context, project, _ string, _ int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Type: "decision", Summary: "Chose JWT", Content: "Chose JWT for auth", SessionID: "sess-1", CreatedAt: time.Now()},
				{ID: "m2", Type: "insight", Summary: "Redis is fast", Content: "Redis cache improved latency", SessionID: "sess-1", CreatedAt: time.Now()},
			}, nil
		},
	}

	srv := NewMCPServer(ms, nil, &mockBus{}, logger, "test", []string{}, 0, nil, "", DefaultMemoryDefaults())
	srv.project = "my-app"

	resp := srv.handleInitialize(&jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})

	data, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)

	instructions := result["instructions"].(string)
	if !contains(instructions, "my-app") {
		t.Fatal("expected project name in instructions")
	}
	if !contains(instructions, "2 memories") {
		t.Fatalf("expected memory count in instructions, got: %s", instructions)
	}
	if !contains(instructions, "1 decisions") {
		t.Fatalf("expected type breakdown in instructions, got: %s", instructions)
	}
}

// TestFormatSingleMemoryUnifiedID tests that unified IDs don't show redundant raw_id.
func TestFormatSingleMemoryUnifiedID(t *testing.T) {
	// Unified: ID == RawID
	mem := store.Memory{ID: "abc-123", RawID: "abc-123", Summary: "Test", CreatedAt: time.Now()}
	text := formatSingleMemory(mem)
	if contains(text, "(raw:") {
		t.Fatal("unified ID should not show raw_id annotation")
	}

	// Legacy: ID != RawID
	mem2 := store.Memory{ID: "enc-123", RawID: "raw-456", Summary: "Test", CreatedAt: time.Now()}
	text2 := formatSingleMemory(mem2)
	if !contains(text2, "(raw: raw-456)") {
		t.Fatal("legacy split ID should show raw_id annotation")
	}
}

// helpers
func extractText(result interface{}) string {
	// Round-trip through JSON to normalize Go types
	data, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if content, ok := m["content"].([]interface{}); ok && len(content) > 0 {
		if item, ok := content[0].(map[string]interface{}); ok {
			if text, ok := item["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
