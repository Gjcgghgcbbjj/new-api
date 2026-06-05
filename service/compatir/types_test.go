package compatir

import "testing"

func TestCompatIRRequestWarnings(t *testing.T) {
	req := &Request{}
	req.AddWarning(WarningUnsupportedField, "previous_response_id", "cannot be represented by chat completions")

	if len(req.Warnings) != 1 {
		t.Fatalf("warnings = %+v", req.Warnings)
	}
	warning := req.Warnings[0]
	if warning.Code != WarningUnsupportedField || warning.Field != "previous_response_id" {
		t.Fatalf("warning = %+v", warning)
	}
}

func TestCompatIRResponseWarnings(t *testing.T) {
	resp := &Response{}
	resp.AddWarning(WarningLossyConversion, "output[0].annotations", "annotations are not represented by chat completions")

	if len(resp.Warnings) != 1 {
		t.Fatalf("warnings = %+v", resp.Warnings)
	}
	warning := resp.Warnings[0]
	if warning.Code != WarningLossyConversion || warning.Field != "output[0].annotations" {
		t.Fatalf("warning = %+v", warning)
	}
}

func TestCompatIRStableToolCallOrdering(t *testing.T) {
	calls := []ToolCall{
		{Index: 2, ID: "call_2", Name: "third"},
		{Index: 0, ID: "call_0", Name: "first"},
		{Index: 1, ID: "call_1", Name: "second"},
	}

	ordered := StableToolCalls(calls)
	if len(ordered) != 3 {
		t.Fatalf("ordered calls = %+v", ordered)
	}
	if ordered[0].ID != "call_0" || ordered[1].ID != "call_1" || ordered[2].ID != "call_2" {
		t.Fatalf("ordered calls = %+v", ordered)
	}

	if calls[0].ID != "call_2" {
		t.Fatalf("StableToolCalls mutated input: %+v", calls)
	}
}

func TestCompatIRStableToolCallOrderingKeepsMissingIndexOrder(t *testing.T) {
	calls := []ToolCall{
		{Index: -1, ID: "call_a", Name: "a"},
		{Index: 0, ID: "call_0", Name: "zero"},
		{Index: -1, ID: "call_b", Name: "b"},
	}

	ordered := StableToolCalls(calls)
	if ordered[0].ID != "call_0" || ordered[1].ID != "call_a" || ordered[2].ID != "call_b" {
		t.Fatalf("ordered calls = %+v", ordered)
	}
}

func TestCompatIRCoreShapes(t *testing.T) {
	req := &Request{
		Model:        "model-a",
		Instructions: "be brief",
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentPart{
					{Type: ContentTypeText, Text: "hello"},
					{Type: ContentTypeImageURL, ImageURL: "https://example.test/image.png"},
				},
			},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{Index: 0, ID: "call_0", Type: ToolTypeFunction, Name: "lookup", Arguments: `{"q":"x"}`},
				},
			},
		},
		Tools: []ToolDefinition{
			{Type: ToolTypeFunction, Name: "lookup", Description: "search", Parameters: map[string]any{"type": "object"}},
		},
	}

	if req.Model != "model-a" || req.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("request = %+v", req)
	}
	if req.Messages[1].ToolCalls[0].Name != "lookup" {
		t.Fatalf("tool calls = %+v", req.Messages[1].ToolCalls)
	}
}
