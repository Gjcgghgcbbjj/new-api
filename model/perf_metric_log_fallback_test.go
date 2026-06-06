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

	allGroups, err := GetLogPerfMetricsSummaryAll(now-60, now, nil, nil)
	require.NoError(t, err)
	require.Len(t, allGroups, 2)
	modelNames := make(map[string]bool, len(allGroups))
	for _, row := range allGroups {
		modelNames[row.ModelName] = true
	}
	require.True(t, modelNames["gpt-test"])
	require.True(t, modelNames["other-model"])

	filtered, err := GetLogPerfMetricsSummaryAll(now-60, now, nil, []string{"other-model"})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "other-model", filtered[0].ModelName)

	empty, err := GetLogPerfMetricsSummaryAll(now-60, now, nil, []string{})
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestGetPerfMetricsBucketSummaryAll(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM perf_metrics").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM perf_metrics").Error)
	})

	bucket := int64(1700000000)
	bucket = bucket - bucket%3600
	require.NoError(t, DB.Create([]PerfMetric{
		{
			ModelName:      "gpt-test",
			Group:          "default",
			BucketTs:       bucket + 60,
			RequestCount:   2,
			SuccessCount:   1,
			TotalLatencyMs: 3000,
			OutputTokens:   20,
			GenerationMs:   2000,
		},
		{
			ModelName:      "gpt-test",
			Group:          "vip",
			BucketTs:       bucket + 120,
			RequestCount:   3,
			SuccessCount:   3,
			TotalLatencyMs: 6000,
			OutputTokens:   30,
			GenerationMs:   3000,
		},
		{
			ModelName:      "other-model",
			Group:          "vip",
			BucketTs:       bucket + 3600 + 30,
			RequestCount:   1,
			SuccessCount:   1,
			TotalLatencyMs: 1000,
			OutputTokens:   10,
			GenerationMs:   1000,
		},
	}).Error)

	summaries, err := GetPerfMetricsBucketSummaryAll(bucket, bucket+7200, 3600, nil, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, "gpt-test", summaries[0].ModelName)
	require.Equal(t, bucket, summaries[0].BucketTs)
	require.EqualValues(t, 5, summaries[0].RequestCount)
	require.EqualValues(t, 4, summaries[0].SuccessCount)
	require.EqualValues(t, 9000, summaries[0].TotalLatencyMs)

	defaultOnly, err := GetPerfMetricsBucketSummaryAll(bucket, bucket+7200, 3600, []string{"default"}, nil)
	require.NoError(t, err)
	require.Len(t, defaultOnly, 1)
	require.EqualValues(t, 2, defaultOnly[0].RequestCount)
}

func TestGetLogPerfMetricsBucketSummaryAll(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	})

	bucket := int64(1700000000)
	bucket = bucket - bucket%3600
	require.NoError(t, DB.Create([]Log{
		{
			CreatedAt:        bucket + 60,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			CompletionTokens: 20,
			UseTime:          2,
			Group:            "default",
		},
		{
			CreatedAt: bucket + 120,
			Type:      LogTypeError,
			ModelName: "gpt-test",
			UseTime:   1,
			Group:     "default",
		},
		{
			CreatedAt:        bucket + 3600 + 30,
			Type:             LogTypeConsume,
			ModelName:        "other-model",
			CompletionTokens: 10,
			UseTime:          1,
			Group:            "vip",
		},
	}).Error)

	summaries, err := GetLogPerfMetricsBucketSummaryAll(bucket, bucket+7200, 3600, nil, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, "gpt-test", summaries[0].ModelName)
	require.Equal(t, bucket, summaries[0].BucketTs)
	require.EqualValues(t, 2, summaries[0].RequestCount)
	require.EqualValues(t, 1, summaries[0].SuccessCount)
	require.EqualValues(t, 3000, summaries[0].TotalLatencyMs)
	require.EqualValues(t, 20, summaries[0].OutputTokens)
	require.EqualValues(t, 2000, summaries[0].GenerationMs)
	require.Equal(t, bucket+120, summaries[0].LastSeen)
}
