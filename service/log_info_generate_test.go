package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoIncludesUpstreamRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?ignored=true", nil)

	info := &relaycommon.RelayInfo{
		StartTime:              time.Unix(10, 0),
		FirstResponseTime:      time.Unix(11, 0),
		RequestURLPath:         "/v1/responses",
		UpstreamRequestURLPath: "/v1/chat/completions?api-version=test",
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatOpenAIResponses,
			types.RelayFormatOpenAI,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, -1)

	require.Equal(t, "/v1/responses", other["request_path"])
	require.Equal(t, "/v1/chat/completions", other["upstream_request_path"])
	require.Equal(t, []string{"OpenAI Responses", "OpenAI Compatible"}, other["request_conversion"])
}

func TestGenerateTextOtherInfoOmitsDuplicateUpstreamRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		StartTime:              time.Unix(10, 0),
		FirstResponseTime:      time.Unix(11, 0),
		RequestURLPath:         "/v1/chat/completions",
		UpstreamRequestURLPath: "/v1/chat/completions",
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI},
		ChannelMeta:            &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, -1)

	require.Equal(t, "/v1/chat/completions", other["request_path"])
	require.NotContains(t, other, "upstream_request_path")
	require.Equal(t, []string{"OpenAI Compatible"}, other["request_conversion"])
}
