package perfmetrics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

var hotBuckets sync.Map
var logFallbackCache sync.Map

// seriesSchema is a stable client cache/schema marker. Do not change it when
// hiding fields or making response-only privacy hardening changes.
const seriesSchema = "dbcd0a3c01b55203"
const logFallbackCacheTTL = 60 * time.Second
const healthHealthyRate = 99.9
const healthWarningRate = 99.0

type logFallbackCacheEntry struct {
	expiresAt time.Time
	rows      []model.PerfMetricSummary
}

type healthBucketModelKey struct {
	ts    int64
	model string
}

type healthBucketAggregate struct {
	counters counters
	lastSeen int64
}

type healthModelAggregate struct {
	counters counters
	lastSeen int64
}

func Init() {
	go flushLoop()
}

func RecordRelaySample(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
	if info == nil {
		return
	}
	now := time.Now()
	hasTtft := info.IsStream && info.HasSendResponse()
	ttftMs := int64(0)
	if hasTtft {
		ttftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	}
	latencyMs := now.Sub(info.StartTime).Milliseconds()
	generationMs := latencyMs
	if hasTtft {
		generationMs = now.Sub(info.FirstResponseTime).Milliseconds()
	}
	if generationMs <= 0 {
		generationMs = latencyMs
	}
	Record(Sample{
		Model:        info.OriginModelName,
		Group:        info.UsingGroup,
		LatencyMs:    latencyMs,
		TtftMs:       ttftMs,
		HasTtft:      hasTtft,
		Success:      success,
		OutputTokens: outputTokens,
		GenerationMs: generationMs,
	})
}

func Record(sample Sample) {
	setting := perf_metrics_setting.GetSetting()
	if !setting.Enabled || sample.Model == "" {
		return
	}
	if sample.Group == "" {
		sample.Group = "default"
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}

	key := bucketKey{
		model:    sample.Model,
		group:    sample.Group,
		bucketTs: bucketStart(time.Now().Unix()),
	}
	actual, _ := hotBuckets.LoadOrStore(key, &atomicBucket{})
	actual.(*atomicBucket).add(sample)
	recordRedis(key, sample)
}

func Query(params QueryParams) (QueryResult, error) {
	if params.Hours <= 0 {
		params.Hours = 24
	}
	if params.Hours > 24*30 {
		params.Hours = 24 * 30
	}
	endTs := time.Now().Unix()
	startTs := endTs - int64(params.Hours)*3600

	merged := map[bucketKey]counters{}
	rows, err := model.GetPerfMetrics(params.Model, params.Group, startTs, endTs)
	if err != nil {
		return QueryResult{}, err
	}
	for _, row := range rows {
		mergeCounters(merged, bucketKey{
			model:    row.ModelName,
			group:    row.Group,
			bucketTs: row.BucketTs,
		}, counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.model != params.Model || k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if params.Group != "" && k.group != params.Group {
			return true
		}
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})

	return buildQueryResult(params.Model, merged), nil
}

func QuerySummaryAll(hours int, groups []string) (SummaryAllResult, error) {
	return querySummary(hours, groups, nil)
}

func QuerySummaryForModels(hours int, groups []string, modelNames []string) (SummaryAllResult, error) {
	modelNames = normalizeStringList(modelNames)
	if len(modelNames) == 0 {
		return SummaryAllResult{Models: []ModelSummary{}}, nil
	}
	return querySummary(hours, groups, modelNames)
}

func QueryHealth(hours int, groups []string, modelNames []string) (HealthResult, error) {
	if hours <= 0 {
		hours = 24 * 30
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	endTs := time.Now().Unix()
	startTs := endTs - int64(hours)*3600
	bucketSeconds := healthBucketSeconds(hours)
	startBucket := alignBucket(startTs, bucketSeconds)
	endBucket := alignBucket(endTs, bucketSeconds)

	if modelNames != nil {
		modelNames = normalizeStringList(modelNames)
		if len(modelNames) == 0 {
			return emptyHealthResult(hours, bucketSeconds, endTs), nil
		}
	}

	allowedGroups := allowedGroupSet(groups)
	allowedModels := allowedModelSet(modelNames)
	bucketModels := map[healthBucketModelKey]healthBucketAggregate{}
	metricKeys := map[healthBucketModelKey]struct{}{}

	metricRows, err := model.GetPerfMetricsBucketSummaryAll(startTs, endTs, bucketSeconds, groups, modelNames)
	if err != nil {
		return HealthResult{}, err
	}
	for _, row := range metricRows {
		key := healthBucketModelKey{ts: row.BucketTs, model: row.ModelName}
		addHealthBucketAggregate(bucketModels, key, rowCounters(row), row.LastSeen)
		metricKeys[key] = struct{}{}
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if allowedModels != nil {
			if _, ok := allowedModels[k.model]; !ok {
				return true
			}
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[k.group]; !ok {
				return true
			}
		}
		snap := value.(*atomicBucket).snapshot()
		if snap.requestCount == 0 {
			return true
		}
		healthKey := healthBucketModelKey{
			ts:    alignBucket(k.bucketTs, bucketSeconds),
			model: k.model,
		}
		addHealthBucketAggregate(bucketModels, healthKey, snap, time.Now().Unix())
		metricKeys[healthKey] = struct{}{}
		return true
	})

	logRows, logErr := model.GetLogPerfMetricsBucketSummaryAll(startTs, endTs, bucketSeconds, groups, modelNames)
	if logErr != nil {
		common.SysError("failed to query log health history fallback: " + logErr.Error())
	} else {
		for _, row := range logRows {
			key := healthBucketModelKey{ts: row.BucketTs, model: row.ModelName}
			if _, ok := metricKeys[key]; ok {
				continue
			}
			addHealthBucketAggregate(bucketModels, key, rowCounters(row), row.LastSeen)
		}
	}

	return buildHealthResult(bucketModels, startBucket, endBucket, bucketSeconds, hours, endTs), nil
}

func querySummary(hours int, groups []string, modelNames []string) (SummaryAllResult, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	endTs := time.Now().Unix()
	startTs := endTs - int64(hours)*3600
	allowedGroups := allowedGroupSet(groups)
	allowedModels := allowedModelSet(modelNames)

	rows, err := model.GetPerfMetricsSummaryAll(startTs, endTs, groups, modelNames)
	if err != nil {
		return SummaryAllResult{}, err
	}

	totals := map[string]counters{}
	for _, row := range rows {
		totals[row.ModelName] = counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		}
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if allowedModels != nil {
			if _, ok := allowedModels[k.model]; !ok {
				return true
			}
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[k.group]; !ok {
				return true
			}
		}
		snap := value.(*atomicBucket).snapshot()
		if snap.requestCount == 0 {
			return true
		}
		cur := totals[k.model]
		cur.requestCount += snap.requestCount
		cur.successCount += snap.successCount
		cur.totalLatencyMs += snap.totalLatencyMs
		cur.outputTokens += snap.outputTokens
		cur.generationMs += snap.generationMs
		totals[k.model] = cur
		return true
	})

	missingModels := missingModelNames(modelNames, totals)
	if logRows, logErr := getLogFallbackRows(startTs, endTs, groups, missingModels); logErr == nil {
		for _, row := range logRows {
			if row.RequestCount == 0 {
				continue
			}
			if _, ok := totals[row.ModelName]; ok {
				continue
			}
			totals[row.ModelName] = counters{
				requestCount:   row.RequestCount,
				successCount:   row.SuccessCount,
				totalLatencyMs: row.TotalLatencyMs,
				outputTokens:   row.OutputTokens,
				generationMs:   row.GenerationMs,
			}
		}
	} else {
		common.SysError("failed to query log perf metrics fallback: " + logErr.Error())
	}

	models := make([]ModelSummary, 0, len(totals))
	for name, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		avgLatency := total.totalLatencyMs / total.requestCount
		successRate := float64(total.successCount) / float64(total.requestCount) * 100
		avgTps := 0.0
		if total.generationMs > 0 {
			avgTps = float64(total.outputTokens) / (float64(total.generationMs) / 1000.0)
		}
		models = append(models, ModelSummary{
			ModelName:    name,
			AvgLatencyMs: avgLatency,
			SuccessRate:  math.Round(successRate*100) / 100,
			AvgTps:       math.Round(avgTps*100) / 100,
			RequestCount: total.requestCount,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].RequestCount > models[j].RequestCount
	})

	return SummaryAllResult{Models: models}, nil
}

func buildHealthResult(bucketModels map[healthBucketModelKey]healthBucketAggregate, startBucket int64, endBucket int64, bucketSeconds int64, hours int, generatedAt int64) HealthResult {
	bucketTotals := map[int64]counters{}
	bucketModelTotals := map[int64]map[string]counters{}
	modelTotals := map[string]healthModelAggregate{}

	for key, value := range bucketModels {
		if value.counters.requestCount == 0 {
			continue
		}
		curBucketTotal := bucketTotals[key.ts]
		curBucketTotal.requestCount += value.counters.requestCount
		curBucketTotal.successCount += value.counters.successCount
		curBucketTotal.totalLatencyMs += value.counters.totalLatencyMs
		curBucketTotal.outputTokens += value.counters.outputTokens
		curBucketTotal.generationMs += value.counters.generationMs
		bucketTotals[key.ts] = curBucketTotal

		if _, ok := bucketModelTotals[key.ts]; !ok {
			bucketModelTotals[key.ts] = map[string]counters{}
		}
		curModelBucket := bucketModelTotals[key.ts][key.model]
		curModelBucket.requestCount += value.counters.requestCount
		curModelBucket.successCount += value.counters.successCount
		curModelBucket.totalLatencyMs += value.counters.totalLatencyMs
		curModelBucket.outputTokens += value.counters.outputTokens
		curModelBucket.generationMs += value.counters.generationMs
		bucketModelTotals[key.ts][key.model] = curModelBucket

		curModel := modelTotals[key.model]
		curModel.counters.requestCount += value.counters.requestCount
		curModel.counters.successCount += value.counters.successCount
		curModel.counters.totalLatencyMs += value.counters.totalLatencyMs
		curModel.counters.outputTokens += value.counters.outputTokens
		curModel.counters.generationMs += value.counters.generationMs
		if value.lastSeen > curModel.lastSeen {
			curModel.lastSeen = value.lastSeen
		}
		modelTotals[key.model] = curModel
	}

	timestamps := healthTimestamps(startBucket, endBucket, bucketSeconds)
	history := make([]HealthBucket, 0, len(timestamps))
	totalCounters := counters{}
	for _, ts := range timestamps {
		bucketCounters := bucketTotals[ts]
		totalCounters.requestCount += bucketCounters.requestCount
		totalCounters.successCount += bucketCounters.successCount
		totalCounters.totalLatencyMs += bucketCounters.totalLatencyMs
		totalCounters.outputTokens += bucketCounters.outputTokens
		totalCounters.generationMs += bucketCounters.generationMs
		activeModels, downModels := bucketHealthCounts(bucketModelTotals[ts])
		history = append(history, healthBucketFromCounters(ts, bucketCounters, activeModels, downModels))
	}

	models := make([]HealthModelSummary, 0, len(modelTotals))
	for name, total := range modelTotals {
		if total.counters.requestCount == 0 {
			continue
		}
		trend := make([]HealthBucket, 0, len(timestamps))
		for _, ts := range timestamps {
			trend = append(trend, healthBucketFromCounters(ts, bucketModelTotals[ts][name], 0, 0))
		}
		models = append(models, HealthModelSummary{
			ModelName:    name,
			AvgLatencyMs: avg(total.counters.totalLatencyMs, total.counters.requestCount),
			SuccessRate:  roundedRate(successRate(total.counters)),
			AvgTps:       roundedRate(avgTps(total.counters)),
			RequestCount: total.counters.requestCount,
			SuccessCount: total.counters.successCount,
			LastSeen:     total.lastSeen,
			Trend:        trend,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].RequestCount > models[j].RequestCount
	})

	return HealthResult{
		Models:        models,
		History:       history,
		Totals:        healthTotals(models, totalCounters),
		WindowHours:   hours,
		BucketSeconds: bucketSeconds,
		GeneratedAt:   generatedAt,
	}
}

func emptyHealthResult(hours int, bucketSeconds int64, generatedAt int64) HealthResult {
	return HealthResult{
		Models:        []HealthModelSummary{},
		History:       []HealthBucket{},
		Totals:        HealthTotals{},
		WindowHours:   hours,
		BucketSeconds: bucketSeconds,
		GeneratedAt:   generatedAt,
	}
}

func healthTotals(models []HealthModelSummary, total counters) HealthTotals {
	result := HealthTotals{
		TotalModels:  len(models),
		RequestCount: total.requestCount,
		SuccessCount: total.successCount,
		AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
		SuccessRate:  roundedRate(successRate(total)),
		AvgTps:       roundedRate(avgTps(total)),
	}
	for _, item := range models {
		if item.SuccessRate >= healthHealthyRate {
			result.Healthy += 1
		} else if item.SuccessRate >= healthWarningRate {
			result.Warning += 1
		} else {
			result.Down += 1
		}
	}
	return result
}

func addHealthBucketAggregate(target map[healthBucketModelKey]healthBucketAggregate, key healthBucketModelKey, value counters, lastSeen int64) {
	if value.requestCount == 0 || key.model == "" {
		return
	}
	current := target[key]
	current.counters.requestCount += value.requestCount
	current.counters.successCount += value.successCount
	current.counters.totalLatencyMs += value.totalLatencyMs
	current.counters.outputTokens += value.outputTokens
	current.counters.generationMs += value.generationMs
	if lastSeen > current.lastSeen {
		current.lastSeen = lastSeen
	}
	target[key] = current
}

func rowCounters(row model.PerfMetricBucketSummary) counters {
	return counters{
		requestCount:   row.RequestCount,
		successCount:   row.SuccessCount,
		totalLatencyMs: row.TotalLatencyMs,
		outputTokens:   row.OutputTokens,
		generationMs:   row.GenerationMs,
	}
}

func healthBucketFromCounters(ts int64, value counters, activeModels int, downModels int) HealthBucket {
	return HealthBucket{
		Ts:           ts,
		RequestCount: value.requestCount,
		SuccessCount: value.successCount,
		AvgLatencyMs: avg(value.totalLatencyMs, value.requestCount),
		SuccessRate:  roundedRate(successRate(value)),
		AvgTps:       roundedRate(avgTps(value)),
		ActiveModels: activeModels,
		DownModels:   downModels,
	}
}

func bucketHealthCounts(models map[string]counters) (int, int) {
	active := 0
	down := 0
	for _, value := range models {
		if value.requestCount == 0 {
			continue
		}
		active += 1
		if successRate(value) < healthWarningRate {
			down += 1
		}
	}
	return active, down
}

func healthTimestamps(startBucket int64, endBucket int64, bucketSeconds int64) []int64 {
	if bucketSeconds <= 0 || endBucket < startBucket {
		return []int64{}
	}
	timestamps := make([]int64, 0, int((endBucket-startBucket)/bucketSeconds)+1)
	for ts := startBucket; ts <= endBucket; ts += bucketSeconds {
		timestamps = append(timestamps, ts)
	}
	return timestamps
}

func healthBucketSeconds(hours int) int64 {
	if hours <= 24 {
		return 3600
	}
	if hours <= 72 {
		return 6 * 3600
	}
	return 24 * 3600
}

func alignBucket(ts int64, bucketSeconds int64) int64 {
	if bucketSeconds <= 0 {
		return ts
	}
	return ts - (ts % bucketSeconds)
}

func roundedRate(value float64) float64 {
	return math.Round(value*100) / 100
}

func getLogFallbackRows(startTs int64, endTs int64, groups []string, modelNames []string) ([]model.PerfMetricSummary, error) {
	if modelNames != nil {
		modelNames = normalizeStringList(modelNames)
		if len(modelNames) == 0 {
			return []model.PerfMetricSummary{}, nil
		}
	}
	key := logFallbackCacheKey(startTs, endTs, groups, modelNames)
	now := time.Now()
	if cached, ok := logFallbackCache.Load(key); ok {
		entry := cached.(logFallbackCacheEntry)
		if now.Before(entry.expiresAt) {
			return entry.rows, nil
		}
		logFallbackCache.Delete(key)
	}
	rows, err := model.GetLogPerfMetricsSummaryAll(startTs, endTs, groups, modelNames)
	if err != nil {
		return nil, err
	}
	logFallbackCache.Store(key, logFallbackCacheEntry{
		expiresAt: now.Add(logFallbackCacheTTL),
		rows:      rows,
	})
	return rows, nil
}

func logFallbackCacheKey(startTs int64, endTs int64, groups []string, modelNames []string) string {
	normalizedGroups := normalizeStringList(groups)
	normalizedModels := normalizeStringList(modelNames)
	groupKey := "*"
	if groups != nil {
		groupKey = strings.Join(normalizedGroups, ",")
	}
	modelKey := "*"
	if modelNames != nil {
		modelKey = strings.Join(normalizedModels, ",")
	}
	return fmt.Sprintf("%d:%d:g=%s:m=%s", startTs/60, endTs/60, groupKey, modelKey)
}

func normalizeStringList(values []string) []string {
	if values == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func allowedModelSet(modelNames []string) map[string]struct{} {
	if modelNames == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(modelNames))
	for _, name := range modelNames {
		allowed[name] = struct{}{}
	}
	return allowed
}

func missingModelNames(modelNames []string, totals map[string]counters) []string {
	if modelNames == nil {
		return nil
	}
	missing := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		if _, ok := totals[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func allowedGroupSet(groups []string) map[string]struct{} {
	if groups == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		allowed[group] = struct{}{}
	}
	return allowed
}

func bucketStart(ts int64) int64 {
	bucketSeconds := perf_metrics_setting.GetBucketSeconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	return ts - (ts % bucketSeconds)
}

func mergeCounters(merged map[bucketKey]counters, key bucketKey, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[key]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	merged[key] = current
}

func buildQueryResult(modelName string, merged map[bucketKey]counters) QueryResult {
	groupBuckets := map[string]map[int64]counters{}
	for key, value := range merged {
		if value.requestCount == 0 {
			continue
		}
		if _, ok := groupBuckets[key.group]; !ok {
			groupBuckets[key.group] = map[int64]counters{}
		}
		groupBuckets[key.group][key.bucketTs] = value
	}

	groups := make([]string, 0, len(groupBuckets))
	for group := range groupBuckets {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	results := make([]GroupResult, 0, len(groups))
	for _, group := range groups {
		buckets := groupBuckets[group]
		timestamps := make([]int64, 0, len(buckets))
		for ts := range buckets {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i] < timestamps[j]
		})

		total := counters{}
		series := make([]BucketPoint, 0, len(timestamps))
		for _, ts := range timestamps {
			value := buckets[ts]
			total.requestCount += value.requestCount
			total.successCount += value.successCount
			total.totalLatencyMs += value.totalLatencyMs
			total.ttftSumMs += value.ttftSumMs
			total.ttftCount += value.ttftCount
			total.outputTokens += value.outputTokens
			total.generationMs += value.generationMs
			series = append(series, bucketPoint(ts, value))
		}

		results = append(results, GroupResult{
			Group:        group,
			AvgTtftMs:    avg(total.ttftSumMs, total.ttftCount),
			AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
			SuccessRate:  successRate(total),
			AvgTps:       avgTps(total),
			Series:       series,
		})
	}

	return QueryResult{
		ModelName:    modelName,
		SeriesSchema: seriesSchema,
		Groups:       results,
	}
}

func bucketPoint(ts int64, value counters) BucketPoint {
	return BucketPoint{
		Ts:           ts,
		AvgTtftMs:    avg(value.ttftSumMs, value.ttftCount),
		AvgLatencyMs: avg(value.totalLatencyMs, value.requestCount),
		SuccessRate:  successRate(value),
		AvgTps:       avgTps(value),
	}
}

func avg(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func successRate(value counters) float64 {
	if value.requestCount <= 0 {
		return 0
	}
	return float64(value.successCount) / float64(value.requestCount) * 100
}

func avgTps(value counters) float64 {
	if value.outputTokens <= 0 || value.generationMs <= 0 {
		return 0
	}
	return float64(value.outputTokens) / (float64(value.generationMs) / 1000)
}

func recordRedis(key bucketKey, sample Sample) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	redisKey := redisBucketKey(key)
	pipe := common.RDB.TxPipeline()
	pipe.HIncrBy(ctx, redisKey, "req", 1)
	if sample.Success {
		pipe.HIncrBy(ctx, redisKey, "ok", 1)
	}
	if sample.LatencyMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "lat", sample.LatencyMs)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		pipe.HIncrBy(ctx, redisKey, "ttft", sample.TtftMs)
		pipe.HIncrBy(ctx, redisKey, "ttft_n", 1)
	}
	if sample.OutputTokens > 0 && sample.GenerationMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "out", sample.OutputTokens)
		pipe.HIncrBy(ctx, redisKey, "gen_ms", sample.GenerationMs)
	}
	pipe.Expire(ctx, redisKey, time.Hour)
	_, _ = pipe.Exec(ctx)
}

func mergeRedisActiveBuckets(merged map[bucketKey]counters, params QueryParams, startTs int64, endTs int64) {
	if !common.RedisEnabled || common.RDB == nil || params.Model == "" || params.Group == "" {
		return
	}
	active := bucketStart(time.Now().Unix())
	if active < startTs || active > endTs {
		return
	}
	key := bucketKey{model: params.Model, group: params.Group, bucketTs: active}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	values, err := common.RDB.HGetAll(ctx, redisBucketKey(key)).Result()
	if err != nil || len(values) == 0 {
		return
	}
	mergeCounters(merged, key, redisCounters(values))
}

func redisBucketKey(key bucketKey) string {
	return fmt.Sprintf("perf:%s:%s:%d", key.model, key.group, key.bucketTs)
}
