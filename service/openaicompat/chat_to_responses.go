package openaicompat

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/compatir"
)

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	ir, err := compatir.FromChatRequest(req)
	if err != nil {
		return nil, err
	}
	return compatir.ToResponsesRequest(ir)
}
