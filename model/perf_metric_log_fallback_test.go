package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetLogPerfMetricsSummaryAll(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	})

	now := time.Now().Unix()
	require.NoError(t, DB.Create([]Log{
		{
			CreatedAt:        now - 10,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			CompletionTokens: 20,
			UseTime:          2,
			Group:            "default",
		},
		{
			CreatedAt: now - 5,
			Type:      LogTypeError,
			ModelName: "gpt-test",
			UseTime:   1,
			Group:     "default",
		},
		{
			CreatedAt:        now - 5,
			Type:             LogTypeConsume,
			ModelName:        "other-model",
			CompletionTokens: 50,
			UseTime:          3,
			Group:            "vip",
		},
		{
			CreatedAt:        now - 3600,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			CompletionTokens: 100,
			UseTime:          5,
			Group:            "default",
		},
	}).Error)

	summaries, err := GetLogPerfMetricsSummaryAll(now-60, now, []string{"default"}, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	summary := summaries[0]
	require.Equal(t, "gpt-test", summary.ModelName)
	require.EqualValues(t, 2, summary.RequestCount)
	require.EqualValues(t, 1, summary.SuccessCount)
	require.EqualValues(t, 3000, summary.TotalLatencyMs)
	require.EqualValues(t, 20, summary.OutputTokens)
	require.EqualValues(t, 2000, summary.GenerationMs)

	filtered, err := GetLogPerfMetricsSummaryAll(now-60, now, nil, []string{"other-model"})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "other-model", filtered[0].ModelName)

	empty, err := GetLogPerfMetricsSummaryAll(now-60, now, nil, []string{})
	require.NoError(t, err)
	require.Empty(t, empty)
}
