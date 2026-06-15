package relay

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func responsesViaChatCompletions(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	chatReq, err := service.ResponsesRequestToChatCompletionsRequest(request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	applySystemPromptIfNeeded(c, info, chatReq)

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()

	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RequestURLPath = "/v1/chat/completions"
	info.UpstreamRequestURLPath = info.RequestURLPath
	info.AppendRequestConversion(types.RelayFormatOpenAI)

	convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, chatReq)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}

	body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	info.UpstreamRequestBodySize = size
	var requestBody io.Reader = body

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	httpResp := resp.(*http.Response)
	info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
	if httpResp.StatusCode != http.StatusOK {
		return nil, service.RelayErrorHandler(c.Request.Context(), httpResp, false)
	}

	if info.IsStream {
		return openaichannel.OaiChatToResponsesStreamHandler(c, info, httpResp, request)
	}
	return openaichannel.OaiChatToResponsesHandler(c, info, httpResp, request)
}

func shouldResponsesUseChatCompletionsBridge(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) bool {
	if info == nil || request == nil {
		return false
	}
	if info.RelayMode != relayconstant.RelayModeResponses {
		return false
	}
	if !isResponsesViaChatBridgeCapableChannel(info.ChannelType) {
		return false
	}
	if supportsNativeResponses(info, request.Model) {
		return false
	}
	return true
}

func isResponsesViaChatBridgeCapableChannel(channelType int) bool {
	return common.IsResponsesViaChatBridgeCapableChannel(channelType)
}

func supportsNativeResponses(info *relaycommon.RelayInfo, requestModel string) bool {
	if info == nil {
		return false
	}
	if endpointTypesSupportNativeResponses(model.GetModelSupportEndpointTypes(requestModel)) {
		return true
	}
	if endpointTypesSupportNativeResponses(common.GetEndpointTypesByChannelType(info.ChannelType, requestModel)) {
		return true
	}
	if info.ChannelType != constant.ChannelTypeOpenAI {
		return false
	}
	baseURL := strings.TrimRight(strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl)), "/")
	return baseURL == "https://api.openai.com" || baseURL == "https://api.openai.com/v1"
}

func endpointTypesSupportNativeResponses(endpoints []constant.EndpointType) bool {
	for _, endpoint := range endpoints {
		switch endpoint {
		case constant.EndpointTypeOpenAIResponse, constant.EndpointTypeOpenAIResponseCompact:
			return true
		}
	}
	return false
}
