package bamboo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/bamboo-services/bamboo-messages/provider/gemini"
)

func TestToolResultErrorPropagationFlags(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		isError bool
		want    map[string]string
	}{
		{"false_literal_error", "  error is a literal result\n", false, map[string]string{"output": "  error is a literal result\n"}},
		{"false_json_error_key", `{"error":"ordinary data"}`, false, map[string]string{"output": `{"error":"ordinary data"}`}},
		{"true_explicit_flag", "  fixture unavailable\n", true, map[string]string{"output": "  fixture unavailable\n", "error": "  fixture unavailable\n"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given: 有效配对，调用 ID 与工具名称不同。
			block := &ToolResultBlock{ToolUseID: "call_A", ToolName: "inspect_state", Content: tt.content, IsError: tt.isError}
			messages := []BambooMessage{
				NewAssistantMessageBlocks(NewToolUseBlockWithRawInput("call_A", "inspect_state", `{}`)),
				NewUserMessageBlocks(block),
			}

			// When: 门面转换工具结果。
			converted, err := messagesToProvider(messages)

			// Then: 标志、身份与内容逐字段保留。
			if err != nil || len(converted) != 2 {
				t.Fatalf("conversion = %+v, %v; want paired messages", converted, err)
			}
			want := provider.Message{Role: provider.RoleTool, Content: tt.content, ToolCallID: "call_A", ToolName: "inspect_state", IsError: tt.isError, CacheControlBlockType: "tool_result"}
			if !reflect.DeepEqual(converted[1], want) {
				t.Errorf("tool result = %+v, want %+v", converted[1], want)
			}
			t.Run("gemini_wire", func(t *testing.T) {
				// When: 真实门面与 Gemini Provider 发送本地 HTTP 请求。
				contents := captureToolResultErrorRequest(t, messages)

				// Then: 上游收到完整配对和精确的成功/错误包装。
				if len(contents) != 2 || len(contents[0].Parts) != 1 || len(contents[1].Parts) != 1 {
					t.Fatalf("unexpected contents: %+v", contents)
				}
				call := contents[0].Parts[0].FunctionCall
				response := contents[1].Parts[0].FunctionResponse
				if contents[0].Role != "model" || call == nil || call.ID != "call_A" || call.Name != "inspect_state" {
					t.Fatalf("call identity changed: %+v", contents[0])
				}
				if contents[1].Role != "user" || response == nil || response.ID != "call_A" || response.Name != "inspect_state" {
					t.Fatalf("response identity changed: %+v", contents[1])
				}
				if !reflect.DeepEqual(response.Response, tt.want) {
					t.Errorf("functionResponse.response = %#v, want %#v", response.Response, tt.want)
				}
			})
		})
	}
}

func TestToolResultErrorPropagationOrphanFiltered(t *testing.T) {
	for _, id := range []string{"", "call_unknown"} {
		t.Run("id_"+id, func(t *testing.T) {
			// Given: 同名但无有效 ID 配对的错误结果，保留普通用户消息。
			messages := []BambooMessage{
				NewUserMessage("inspect fixture"),
				NewAssistantMessageBlocks(NewToolUseBlockWithRawInput("call_A", "inspect_state", `{}`)),
				NewUserMessageBlocks(&ToolResultBlock{ToolUseID: id, ToolName: "inspect_state", Content: "orphan failure", IsError: true}),
			}

			// When: 门面清理孤儿历史。
			converted, err := messagesToProvider(messages)

			// Then: 不按名称修复身份，不透传孤儿错误。
			want := []provider.Message{{Role: provider.RoleUser, Content: "inspect fixture"}}
			if err != nil || !reflect.DeepEqual(converted, want) {
				t.Fatalf("orphan conversion = %+v, %v; want %+v", converted, err, want)
			}
			t.Run("gemini_wire", func(t *testing.T) {
				// When: 真实 Provider 发送过滤后的历史。
				contents := captureToolResultErrorRequest(t, messages)

				// Then: HTTP 请求只包含原有用户文本。
				want := []toolResultErrorContent{{Role: "user", Parts: []toolResultErrorPart{{Text: "inspect fixture"}}}}
				if !reflect.DeepEqual(contents, want) {
					t.Fatalf("orphan reached upstream: %+v", contents)
				}
			})
		})
	}
}

type toolResultErrorContent struct {
	Role  string                `json:"role"`
	Parts []toolResultErrorPart `json:"parts"`
}

type toolResultErrorPart struct {
	Text         string `json:"text,omitempty"`
	FunctionCall *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"functionCall,omitempty"`
	FunctionResponse *struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		Response map[string]string `json:"response"`
	} `json:"functionResponse,omitempty"`
}

func captureToolResultErrorRequest(t *testing.T, messages []BambooMessage) []toolResultErrorContent {
	t.Helper()
	requests := make(chan []toolResultErrorContent, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/models/task-4-fixture:generateContent" {
			t.Errorf("unexpected upstream route: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected route", http.StatusBadRequest)
			return
		}
		var body struct {
			Contents []toolResultErrorContent `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream request: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		select {
		case requests <- body.Contents:
		default:
			t.Error("unexpected additional upstream request")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"fixture complete"}]},"finishReason":"STOP"}]}`); err != nil {
			t.Errorf("write fixture response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := NewClient(gemini.NewProviderWithOptions(gemini.WithBaseURL(server.URL), gemini.WithAPIKey("task-4-dummy")))
	if _, err := client.Complete(ctx, messages, "", &RequestConfig{Model: "task-4-fixture"}); err != nil {
		t.Fatalf("localhost Complete: %v", err)
	}
	select {
	case contents := <-requests:
		encoded, err := json.Marshal(contents)
		if err != nil {
			t.Fatalf("encode decoded capture: %v", err)
		}
		t.Logf("decoded localhost contents: %s", encoded)
		return contents
	case <-ctx.Done():
		t.Fatalf("missing localhost request: %v", ctx.Err())
		return nil
	}
}
