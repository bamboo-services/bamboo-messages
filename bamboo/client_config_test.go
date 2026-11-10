package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider/openai/completions"
)

// TestChat_NilConfigUsesDefaultModel 验证 RequestConfig 为 nil 时不会 panic。
//
// 回归场景：Provider 契约明确「config 传 nil 时使用默认配置」，
// 但 OpenAI Completions 适配器在拼装 endpoint 时直接读取 config.Model，
// nil 配置会触发空指针 panic，客户端侧表现为整个进程崩溃。
func TestChat_NilConfigUsesDefaultModel(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		gotModel = extractModel(string(body[:n]))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := completions.NewCompletionsProviderWithOptions(
		completions.WithAPIKey("test-key"),
		completions.WithBaseURL(server.URL),
	)
	client := NewClientWithOptions(WithProvider(p), WithDefaultModel("gpt-4o-mini"))

	events, err := client.Chat(context.Background(), []BambooMessage{
		{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
	}, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	for range events {
	}

	if gotModel != "gpt-4o-mini" {
		t.Errorf("request model = %q, want %q (WithDefaultModel 应在未指定模型时生效)", gotModel, "gpt-4o-mini")
	}
}

// TestChat_ExplicitModelOverridesDefault 验证请求显式指定的模型优先于默认模型。
func TestChat_ExplicitModelOverridesDefault(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		gotModel = extractModel(string(body[:n]))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := completions.NewCompletionsProviderWithOptions(
		completions.WithAPIKey("test-key"),
		completions.WithBaseURL(server.URL),
	)
	client := NewClientWithOptions(WithProvider(p), WithDefaultModel("gpt-4o-mini"))

	events, err := client.Chat(context.Background(), []BambooMessage{
		{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
	}, "", &RequestConfig{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	for range events {
	}

	if gotModel != "gpt-4o" {
		t.Errorf("request model = %q, want %q (显式模型应优先)", gotModel, "gpt-4o")
	}
}

// extractModel 从请求体 JSON 中粗略提取 model 字段值。
func extractModel(body string) string {
	const marker = `"model":"`
	idx := indexOfString(body, marker)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(marker):]
	end := indexOfString(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// indexOfString 返回子串首次出现的下标，未找到返回 -1。
func indexOfString(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
