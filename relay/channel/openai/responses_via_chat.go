package openai

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/compatir"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, original *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	var chatResp dto.OpenAITextResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responsesResp, usage, err := service.ChatCompletionsResponseToResponsesResponse(&chatResp, original, helper.GetResponseID(c))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, responsesResp.OutputText, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = usage
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, original *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	createdAt := int(time.Now().Unix())
	model := info.UpstreamModelName
	if original != nil && original.Model != "" {
		model = original.Model
	}

	streamErr := (*types.NewAPIError)(nil)
	converter := compatir.NewChatToResponsesStreamConverter(compatir.ChatToResponsesStreamOptions{
		ResponseID: responseID,
		MessageID:  messageID,
		Model:      model,
		CreatedAt:  createdAt,
	})

	sendEvent := func(event compatir.ResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data))
		return true
	}

	sendEvents := func(events []compatir.ResponsesStreamEvent) bool {
		for _, event := range events {
			if !sendEvent(event) {
				return false
			}
		}
		return true
	}

	helper.SetEventStreamHeaders(c)
	if !sendEvents(converter.StartEvents()) {
		return nil, streamErr
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			sr.Error(err)
			return
		}
		if !sendEvents(converter.EventsFromChatChunk(&chunk)) {
			sr.Stop(streamErr)
			return
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := converter.Usage()
	text := converter.OutputText()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		converter.SetUsage(usage)
	}
	finalEvents, usage := converter.FinalEvents()
	if !sendEvents(finalEvents) {
		return nil, streamErr
	}
	helper.Done(c)
	return usage, nil
}
