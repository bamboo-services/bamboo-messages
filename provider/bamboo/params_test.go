package bamboo

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// === buildParams 测试 ===

func TestBuildParams_FullConfigMapping(t *testing.T) {
	temp := 0.7
	topP := 0.9
	cc := provider.NewEphemeralCacheControl()
	config := &provider.ChatConfig{
		Model:              "claude-sonnet-4-20250514",
		MaxTokens:          1024,
		Temperature:        &temp,
		TopP:               &topP,
		Stop:               []string{"STOP", "END"},
		Metadata:           map[string]string{"key": "val"},
		UserID:             "user-123",
		ToolChoice:         "auto",
		ResponseFormat:     "json_object",
		ParallelToolCalls:  true,
		PromptCacheKey:     "cache-key-abc",
		ProviderExtra:      map[string]any{"custom": "value"},
		SystemCacheControl: cc,
		ThinkingConfig: &provider.ThinkingConfig{
			Effort:  "high",
			Display: "summarized",
		},
	}
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "hi"}}

	req := buildParams("be helpful", msgs, config)

	// 验证消息
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	// 验证 system
	if req.System != "be helpful" {
		t.Errorf("expected system 'be helpful', got %s", req.System)
	}
	// 验证 Stream 未设置
	if req.Stream {
		t.Error("expected Stream to be false (not set)")
	}
	// 验证 Config
	if req.Config == nil {
		t.Fatal("expected non-nil Config")
	}
	cfg := req.Config
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected Model, got %s", cfg.Model)
	}
	if cfg.MaxTokens != 1024 {
		t.Errorf("expected MaxTokens 1024, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
		t.Errorf("expected Temperature 0.7, got %+v", cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Errorf("expected TopP 0.9, got %+v", cfg.TopP)
	}
	if len(cfg.StopSequences) != 2 || cfg.StopSequences[0] != "STOP" || cfg.StopSequences[1] != "END" {
		t.Errorf("expected StopSequences [STOP END], got %+v", cfg.StopSequences)
	}
	if cfg.Metadata == nil || cfg.Metadata["key"] != "val" {
		t.Errorf("expected Metadata key=val, got %+v", cfg.Metadata)
	}
	if cfg.UserID != "user-123" {
		t.Errorf("expected UserID user-123, got %s", cfg.UserID)
	}
	if cfg.ToolChoice != "auto" {
		t.Errorf("expected ToolChoice auto, got %s", cfg.ToolChoice)
	}
	if cfg.ResponseFormat != "json_object" {
		t.Errorf("expected ResponseFormat json_object, got %s", cfg.ResponseFormat)
	}
	if !cfg.ParallelToolCalls {
		t.Error("expected ParallelToolCalls true")
	}
	if cfg.PromptCacheKey != "cache-key-abc" {
		t.Errorf("expected PromptCacheKey cache-key-abc, got %s", cfg.PromptCacheKey)
	}
	if cfg.ProviderExtra == nil || cfg.ProviderExtra["custom"] != "value" {
		t.Errorf("expected ProviderExtra custom=value, got %+v", cfg.ProviderExtra)
	}
	if len(cfg.SystemCacheControl) == 0 {
		t.Error("expected non-empty SystemCacheControl")
	}
	if cfg.ThinkingConfig == nil {
		t.Fatal("expected non-nil ThinkingConfig")
	}
	if cfg.ThinkingConfig.Effort != "high" {
		t.Errorf("expected Effort high, got %s", cfg.ThinkingConfig.Effort)
	}
	if cfg.ThinkingConfig.Display != "summarized" {
		t.Errorf("expected Display summarized, got %s", cfg.ThinkingConfig.Display)
	}
}

func TestBuildParams_SystemEmpty(t *testing.T) {
	config := &provider.ChatConfig{Model: "m", MaxTokens: 100}
	req := buildParams("", []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, config)
	if req.System != "" {
		t.Errorf("expected empty System, got %s", req.System)
	}
}

func TestBuildParams_SystemPropagated(t *testing.T) {
	config := &provider.ChatConfig{Model: "m", MaxTokens: 100}
	req := buildParams("you are helpful", []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, config)
	if req.System != "you are helpful" {
		t.Errorf("expected System 'you are helpful', got %s", req.System)
	}
}

func TestBuildParams_StreamNotSet(t *testing.T) {
	config := &provider.ChatConfig{Model: "m", MaxTokens: 100}
	req := buildParams("", []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, config)
	if req.Stream {
		t.Error("Stream must not be set by buildParams")
	}
	// 验证 JSON 序列化时 stream 字段被省略
	data, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["stream"]; ok {
		t.Error("expected stream to be omitted from JSON")
	}
}

func TestBuildParams_NilConfig(t *testing.T) {
	req := buildParams("", []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, nil)
	if req.Config != nil {
		t.Errorf("expected nil Config for nil config param, got %+v", req.Config)
	}
	// 验证 JSON 中 config 字段被省略
	data, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["config"]; ok {
		t.Error("expected config to be omitted from JSON when nil")
	}
}

func TestBuildParams_MessagesPropagated(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "hi there"},
	}
	req := buildParams("", msgs, nil)
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
}

func TestBuildParams_MinimalConfig(t *testing.T) {
	config := &provider.ChatConfig{
		Model:     "claude-3-haiku",
		MaxTokens: 256,
	}
	req := buildParams("", []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, config)
	if req.Config == nil {
		t.Fatal("expected non-nil Config")
	}
	if req.Config.Model != "claude-3-haiku" {
		t.Errorf("expected Model claude-3-haiku, got %s", req.Config.Model)
	}
	if req.Config.MaxTokens != 256 {
		t.Errorf("expected MaxTokens 256, got %d", req.Config.MaxTokens)
	}
	if req.Config.Temperature != nil {
		t.Error("expected nil Temperature for minimal config")
	}
	if req.Config.ThinkingConfig != nil {
		t.Error("expected nil ThinkingConfig for minimal config")
	}
}

// === buildTools 测试 ===

func TestBuildTools_BasicFunctionTool(t *testing.T) {
	tools := []provider.Tool{
		{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        "get_weather",
				Description: "Get weather for a city",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	result := buildTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	wt := result[0]
	if wt.Name != "get_weather" {
		t.Errorf("expected Name get_weather, got %s", wt.Name)
	}
	if wt.Description != "Get weather for a city" {
		t.Errorf("expected Description, got %s", wt.Description)
	}
	// InputSchema 必须存在且为合法 JSON
	if len(wt.InputSchema) == 0 {
		t.Fatal("expected non-empty InputSchema")
	}
	var schema map[string]any
	if err := json.Unmarshal(wt.InputSchema, &schema); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected schema type object, got %v", schema["type"])
	}
}

func TestBuildTools_InputSchemaAlwaysPresent(t *testing.T) {
	// 即使 Parameters 为 nil，InputSchema 也必须输出 {}
	tools := []provider.Tool{
		{
			Type: "function",
			Function: provider.FunctionDef{
				Name: "no_params",
			},
		},
	}
	result := buildTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if string(result[0].InputSchema) != `{}` {
		t.Errorf("expected InputSchema {}, got %s", string(result[0].InputSchema))
	}
	// 验证 JSON 中 input_schema 字段存在（无 omitempty）
	data, _ := json.Marshal(result[0])
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["input_schema"]; !ok {
		t.Error("expected input_schema to be present in JSON (no omitempty)")
	}
}

func TestBuildTools_CacheControlPassthrough(t *testing.T) {
	cc := provider.NewEphemeralCacheControl()
	tools := []provider.Tool{
		{
			Type:         "function",
			Function:     provider.FunctionDef{Name: "fn", Parameters: map[string]any{"type": "object"}},
			CacheControl: cc,
		},
	}
	result := buildTools(tools)
	if len(result[0].CacheControl) == 0 {
		t.Fatal("expected non-empty CacheControl")
	}
	var parsed provider.CacheControl
	if err := json.Unmarshal(result[0].CacheControl, &parsed); err != nil {
		t.Fatalf("failed to unmarshal CacheControl: %v", err)
	}
	if parsed.Type != "ephemeral" {
		t.Errorf("expected type ephemeral, got %s", parsed.Type)
	}
}

func TestBuildTools_SkipNonFunctionTools(t *testing.T) {
	tools := []provider.Tool{
		{Type: "integration", Function: provider.FunctionDef{Name: "skip_me"}},
		{Type: "function", Function: provider.FunctionDef{Name: "keep_me"}},
	}
	result := buildTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool (non-function skipped), got %d", len(result))
	}
	if result[0].Name != "keep_me" {
		t.Errorf("expected Name keep_me, got %s", result[0].Name)
	}
}

func TestBuildTools_NilTools(t *testing.T) {
	result := buildTools(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %+v", result)
	}
}

func TestBuildTools_EmptyTools(t *testing.T) {
	result := buildTools([]provider.Tool{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %+v", result)
	}
}

func TestBuildTools_DescriptionOmitEmpty(t *testing.T) {
	tools := []provider.Tool{
		{
			Type:     "function",
			Function: provider.FunctionDef{Name: "no_desc", Parameters: map[string]any{}},
		},
	}
	result := buildTools(tools)
	data, _ := json.Marshal(result[0])
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["description"]; ok {
		t.Error("expected description to be omitted when empty")
	}
}

// === buildWireRequestConfig 边界测试 ===

func TestBuildWireRequestConfig_ThinkingConfigNil(t *testing.T) {
	config := &provider.ChatConfig{
		Model:     "m",
		MaxTokens: 100,
	}
	cfg := buildWireRequestConfig(config)
	if cfg.ThinkingConfig != nil {
		t.Error("expected nil ThinkingConfig")
	}
	if len(cfg.SystemCacheControl) != 0 {
		t.Error("expected empty SystemCacheControl")
	}
}

func TestBuildWireRequestConfig_SystemCacheControlSerialized(t *testing.T) {
	cc := provider.NewEphemeralCacheControl(provider.CacheTTL1h)
	config := &provider.ChatConfig{
		Model:              "m",
		MaxTokens:          100,
		SystemCacheControl: cc,
	}
	cfg := buildWireRequestConfig(config)
	var parsed provider.CacheControl
	if err := json.Unmarshal(cfg.SystemCacheControl, &parsed); err != nil {
		t.Fatalf("failed to unmarshal SystemCacheControl: %v", err)
	}
	if parsed.TTL != provider.CacheTTL1h {
		t.Errorf("expected TTL 1h, got %s", parsed.TTL)
	}
}

// === JSON 序列化集成测试 ===

func TestBuildParams_JSONStructure(t *testing.T) {
	temp := 0.5
	config := &provider.ChatConfig{
		Model:       "test-model",
		MaxTokens:   512,
		Temperature: &temp,
		Tools: []provider.Tool{
			{Type: "function", Function: provider.FunctionDef{Name: "fn", Description: "d", Parameters: map[string]any{"type": "object"}}},
		},
	}
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "test"}}
	req := buildParams("system prompt", msgs, config)

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// 验证顶层结构
	if _, ok := raw["messages"]; !ok {
		t.Error("expected 'messages' in JSON")
	}
	if raw["system"] != "system prompt" {
		t.Errorf("expected system 'system prompt', got %v", raw["system"])
	}
	if _, ok := raw["stream"]; ok {
		t.Error("expected 'stream' to be omitted")
	}
	cfg, ok := raw["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config to be a map")
	}
	if cfg["model"] != "test-model" {
		t.Errorf("expected model test-model, got %v", cfg["model"])
	}
	if cfg["max_tokens"] != float64(512) {
		t.Errorf("expected max_tokens 512, got %v", cfg["max_tokens"])
	}
	tools, ok := cfg["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in config, got %+v", cfg["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "fn" {
		t.Errorf("expected tool name fn, got %v", tool["name"])
	}
	if _, ok := tool["input_schema"]; !ok {
		t.Error("expected input_schema in tool JSON")
	}
}
