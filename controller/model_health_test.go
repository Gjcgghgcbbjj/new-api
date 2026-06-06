package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelHealthResponse struct {
	Success bool                     `json:"success"`
	Message string                   `json:"message"`
	Data    perfmetrics.HealthResult `json:"data"`
}

func TestGetModelsPerfHealthOnlyReturnsEnabledModels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}, &model.Log{}))

	now := time.Now().Unix()
	bucket := now - now%3600
	require.NoError(t, db.Create([]model.Ability{
		{Group: "default", Model: "current-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "disabled-model", ChannelId: 2, Enabled: false},
	}).Error)
	require.NoError(t, db.Create([]model.PerfMetric{
		{
			ModelName:      "current-model",
			Group:          "default",
			BucketTs:       bucket,
			RequestCount:   4,
			SuccessCount:   4,
			TotalLatencyMs: 4000,
			OutputTokens:   80,
			GenerationMs:   2000,
		},
		{
			ModelName:      "old-model",
			Group:          "default",
			BucketTs:       bucket,
			RequestCount:   9,
			SuccessCount:   9,
			TotalLatencyMs: 9000,
			OutputTokens:   180,
			GenerationMs:   3000,
		},
		{
			ModelName:      "disabled-model",
			Group:          "default",
			BucketTs:       bucket,
			RequestCount:   5,
			SuccessCount:   5,
			TotalLatencyMs: 5000,
			OutputTokens:   100,
			GenerationMs:   2500,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/models/perf-health", GetModelsPerfHealth)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/models/perf-health?hours=24", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload modelHealthResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)

	require.Len(t, payload.Data.Models, 1)
	require.Equal(t, "current-model", payload.Data.Models[0].ModelName)
	require.EqualValues(t, 1, payload.Data.Totals.TotalModels)
	require.EqualValues(t, 4, payload.Data.Totals.RequestCount)
}
