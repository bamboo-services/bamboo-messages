package provider

import "testing"

// TestNewBlockStartDelta 测试创建内容块开始事件（无 ID）
func TestNewBlockStartDelta(t *testing.T) {
	delta := NewBlockStartDelta("text")

	// 验证 Type
	if delta.Type != StreamDeltaTypeBlockStart {
		t.Errorf("NewBlockStartDelta() Type = %v, want %v", delta.Type, StreamDeltaTypeBlockStart)
	}

	// 验证 Data 类型
	data, ok := delta.Data.(BlockStartData)
	if !ok {
		t.Errorf("NewBlockStartDelta() Data type assertion failed, want BlockStartData")
	}

	// 验证 BlockType
	if data.BlockType != "text" {
		t.Errorf("NewBlockStartDelta() BlockType = %v, want %v", data.BlockType, "text")
	}

	// 验证 ID 和 Name 应为空
	if data.ID != "" {
		t.Errorf("NewBlockStartDelta() ID = %v, want empty string", data.ID)
	}
	if data.Name != "" {
		t.Errorf("NewBlockStartDelta() Name = %v, want empty string", data.Name)
	}
}

// TestNewBlockStartDeltaThinking 测试创建思考类型的内容块
func TestNewBlockStartDeltaThinking(t *testing.T) {
	delta := NewBlockStartDelta("thinking")

	if delta.Type != StreamDeltaTypeBlockStart {
		t.Errorf("Type = %v, want %v", delta.Type, StreamDeltaTypeBlockStart)
	}

	data, ok := delta.Data.(BlockStartData)
	if !ok {
		t.Fatal("Data type assertion failed")
	}

	if data.BlockType != "thinking" {
		t.Errorf("BlockType = %v, want %v", data.BlockType, "thinking")
	}
}

// TestNewBlockStartDeltaWithID 测试创建带 ID 的内容块开始事件
func TestNewBlockStartDeltaWithID(t *testing.T) {
	delta := NewBlockStartDeltaWithID("tool_use", "call_123", "func_name")

	// 验证 Type
	if delta.Type != StreamDeltaTypeBlockStart {
		t.Errorf("NewBlockStartDeltaWithID() Type = %v, want %v", delta.Type, StreamDeltaTypeBlockStart)
	}

	// 验证 Data 类型
	data, ok := delta.Data.(BlockStartData)
	if !ok {
		t.Errorf("NewBlockStartDeltaWithID() Data type assertion failed, want BlockStartData")
	}

	// 验证 BlockType
	if data.BlockType != "tool_use" {
		t.Errorf("NewBlockStartDeltaWithID() BlockType = %v, want %v", data.BlockType, "tool_use")
	}

	// 验证 ID
	if data.ID != "call_123" {
		t.Errorf("NewBlockStartDeltaWithID() ID = %v, want %v", data.ID, "call_123")
	}

	// 验证 Name
	if data.Name != "func_name" {
		t.Errorf("NewBlockStartDeltaWithID() Name = %v, want %v", data.Name, "func_name")
	}
}

// TestNewBlockStartDeltaWithIDEmptyFields 测试带 ID 但字段为空的情况
func TestNewBlockStartDeltaWithIDEmptyFields(t *testing.T) {
	tests := []struct {
		testName  string
		blockType string
		id        string
		funcName  string
	}{
		{
			testName:  "空 ID 和空 Name",
			blockType: "tool_use",
			id:        "",
			funcName:  "",
		},
		{
			testName:  "空 ID 有 Name",
			blockType: "tool_use",
			id:        "",
			funcName:  "my_func",
		},
		{
			testName:  "有 ID 空 Name",
			blockType: "tool_use",
			id:        "call_456",
			funcName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			delta := NewBlockStartDeltaWithID(tt.blockType, tt.id, tt.funcName)

			if delta.Type != StreamDeltaTypeBlockStart {
				t.Errorf("Type = %v, want %v", delta.Type, StreamDeltaTypeBlockStart)
			}

			data, ok := delta.Data.(BlockStartData)
			if !ok {
				t.Fatal("Data type assertion failed")
			}

			if data.BlockType != tt.blockType {
				t.Errorf("BlockType = %v, want %v", data.BlockType, tt.blockType)
			}

			if data.ID != tt.id {
				t.Errorf("ID = %v, want %v", data.ID, tt.id)
			}

			if data.Name != tt.funcName {
				t.Errorf("Name = %v, want %v", data.Name, tt.funcName)
			}
		})
	}
}

// TestStreamDeltaTypeConstants 测试流增量类型常量的值
func TestStreamDeltaTypeConstants(t *testing.T) {
	tests := []struct {
		testName  string
		constant  StreamDeltaType
		wantValue string
	}{
		{"TextOutput", StreamDeltaTypeTextOutput, "text_output"},
		{"Thinking", StreamDeltaTypeThinking, "thinking"},
		{"ToolCall", StreamDeltaTypeToolCall, "tool_call"},
		{"ToolCallDelta", StreamDeltaTypeToolCallDelta, "tool_call_delta"},
		{"Usage", StreamDeltaTypeUsage, "usage"},
		{"BlockStart", StreamDeltaTypeBlockStart, "block_start"},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if string(tt.constant) != tt.wantValue {
				t.Errorf("%s = %v, want %v", tt.testName, tt.constant, tt.wantValue)
			}
		})
	}
}

// TestStreamTypeConstants 测试流事件类型常量的值
func TestStreamTypeConstants(t *testing.T) {
	tests := []struct {
		testName  string
		constant  StreamType
		wantValue string
	}{
		{"Start", StreamTypeStart, "start"},
		{"Stop", StreamTypeStop, "stop"},
		{"Done", StreamTypeDone, "done"},
		{"Error", StreamTypeError, "error"},
		{"Delta", StreamTypeDelta, "delta"},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if string(tt.constant) != tt.wantValue {
				t.Errorf("%s = %v, want %v", tt.testName, tt.constant, tt.wantValue)
			}
		})
	}
}
