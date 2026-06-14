package compatir

import (
	"encoding/json"
	"sort"

	"github.com/QuantumNous/new-api/dto"
)

const (
	ContentTypeText       = "text"
	ContentTypeImageURL   = "image_url"
	ContentTypeInputAudio = "input_audio"
	ContentTypeFile       = "file"
	ContentTypeVideoURL   = "video_url"

	ToolTypeFunction = "function"

	OutputTypeMessage      = "message"
	OutputTypeFunctionCall = "function_call"
)

type Request struct {
	Model                string
	Instructions         string
	Messages             []Message
	Tools                []ToolDefinition
	ToolChoice           *ToolChoice
	Stream               *bool
	StreamOptions        *dto.StreamOptions
	MaxOutputTokens      *uint
	Temperature          *float64
	TopP                 *float64
	User                 json.RawMessage
	Metadata             json.RawMessage
	Store                json.RawMessage
	Text                 json.RawMessage
	PromptCacheKey       string
	PromptCacheRetention json.RawMessage
	Reasoning            *dto.Reasoning
	ParallelToolCalls    *bool
	Warnings             []ConversionWarning
}

func (r *Request) AddWarning(code WarningCode, field string, message string) {
	if r == nil {
		return
	}
	r.Warnings = append(r.Warnings, ConversionWarning{
		Code:    code,
		Field:   field,
		Message: message,
	})
}

type Response struct {
	ID               string
	CreatedAt        int
	Model            string
	Status           string
	IncompleteReason string
	Output           []OutputItem
	Usage            *dto.Usage
	Warnings         []ConversionWarning
}

func (r *Response) AddWarning(code WarningCode, field string, message string) {
	if r == nil {
		return
	}
	r.Warnings = append(r.Warnings, ConversionWarning{
		Code:    code,
		Field:   field,
		Message: message,
	})
}

type Message struct {
	Role       string
	Content    []ContentPart
	ToolCalls  []ToolCall
	ToolCallID string
}

type ContentPart struct {
	Type       string
	Text       string
	ImageURL   any
	InputAudio any
	File       any
	VideoURL   any
}

type ToolDefinition struct {
	Type        string
	Name        string
	Description string
	Parameters  any
}

type ToolChoice struct {
	Mode         string
	FunctionName string
	Raw          any
}

type ToolCall struct {
	Index     int
	ID        string
	Type      string
	Name      string
	Arguments string
}

type OutputItem struct {
	Type     string
	ID       string
	Status   string
	Role     string
	Content  []ContentPart
	ToolCall ToolCall
}

func StableToolCalls(calls []ToolCall) []ToolCall {
	out := make([]ToolCall, len(calls))
	copy(out, calls)
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Index
		right := out[j].Index
		if left < 0 && right < 0 {
			return false
		}
		if left < 0 {
			return false
		}
		if right < 0 {
			return true
		}
		return left < right
	})
	return out
}
