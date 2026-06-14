package openaicompat

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/compatir"
)

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	ir, err := compatir.FromResponsesResponse(resp)
	if err != nil {
		return nil, nil, err
	}
	return compatir.ToChatResponse(ir, id)
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return compatir.ExtractOutputTextFromResponses(resp)
}
