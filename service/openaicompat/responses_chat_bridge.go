package openaicompat

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/compatir"
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	ir, err := compatir.FromResponsesRequest(req)
	if err != nil {
		return nil, err
	}
	return compatir.ToChatRequest(ir)
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, original *dto.OpenAIResponsesRequest, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	ir, err := compatir.FromChatResponse(resp)
	if err != nil {
		return nil, nil, err
	}
	return compatir.ToResponsesResponse(ir, original, id)
}
