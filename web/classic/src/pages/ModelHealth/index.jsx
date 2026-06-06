/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Input,
  Progress,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconSearch } from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

const { Text, Title } = Typography;

const HEALTHY_RATE = 99.9;
const WARNING_RATE = 99;
const CHART_CONFIG = { mode: 'desktop-browser' };

const HEALTH_COLORS = {
  healthy: '#22c55e',
  warning: '#f59e0b',
  down: '#ef4444',
  volume: '#2563eb',
  latency: '#7c3aed',
  neutral: '#94a3b8',
};

const formatNumber = (value) => {
  const num = Number(value);
  if (!Number.isFinite(num)) return '0';
  return new Intl.NumberFormat('zh-CN').format(num);
};

const formatLatency = (latencyMs) => {
  const value = Number(latencyMs);
  if (!Number.isFinite(value) || value <= 0) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(2)}s`;
};

const formatRate = (value) => {
  const num = Number(value);
  if (!Number.isFinite(num)) return '-';
  return `${num.toFixed(num >= 99 ? 2 : 1)}%`;
};

const formatTps = (tps) => {
  const value = Number(tps);
  if (!Number.isFinite(value) || value <= 0) return '-';
  return `${value.toFixed(value >= 10 ? 1 : 2)} tps`;
};

const formatWindow = (hours, t) => {
  if (hours < 24) return t('近 {{count}} 小时', { count: hours });
  if (hours === 24) return t('近 24 小时');
  return t('近 {{count}} 天', { count: Math.round(hours / 24) });
};

const formatBucket = (bucketSeconds, t) => {
  if (bucketSeconds >= 86400) return t('按天');
  if (bucketSeconds >= 3600) return t('按 {{count}} 小时', {
    count: Math.round(bucketSeconds / 3600),
  });
  return t('按分钟');
};

const formatBucketLabel = (ts, bucketSeconds) => {
  if (!ts) return '-';
  if (bucketSeconds >= 86400) return dayjs.unix(ts).format('MM-DD');
  return dayjs.unix(ts).format('MM-DD HH:mm');
};

const formatLastSeen = (ts) => {
  const value = Number(ts);
  if (!Number.isFinite(value) || value <= 0) return '-';
  return dayjs.unix(value).format('MM-DD HH:mm');
};

const getHealthColor = (successRate) => {
  if (successRate >= HEALTHY_RATE) return 'green';
  if (successRate >= WARNING_RATE) return 'orange';
  return 'red';
};

const getHealthTone = (successRate) => {
  if (successRate >= HEALTHY_RATE) return 'healthy';
  if (successRate >= WARNING_RATE) return 'warning';
  return 'down';
};

const getHealthLabel = (successRate, t) => {
  if (successRate >= HEALTHY_RATE) return t('良好');
  if (successRate >= WARNING_RATE) return t('波动');
  return t('异常');
};

const getLineColor = (successRate) => {
  const tone = getHealthTone(successRate);
  return HEALTH_COLORS[tone];
};

const normalizeTrend = (trend) =>
  Array.isArray(trend)
    ? trend.map((item) => ({
        ts: Number(item.ts) || 0,
        requestCount: Number(item.request_count) || 0,
        successRate: Number(item.success_rate) || 0,
        avgLatencyMs: Number(item.avg_latency_ms) || 0,
      }))
    : [];

const MetricCard = ({ label, value, meta, tone = 'neutral', sparkline }) => (
  <Card shadows='hover' className='rounded-lg' bodyStyle={{ padding: 16 }}>
    <div className='flex min-h-[84px] items-center justify-between gap-4'>
      <div className='min-w-0'>
        <Text type='tertiary' size='small'>
          {label}
        </Text>
        <div
          className='mt-2 text-2xl font-semibold leading-none'
          style={{
            color: HEALTH_COLORS[tone] || 'var(--semi-color-text-0)',
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {value}
        </div>
        <Text type='tertiary' size='small' className='mt-2 block'>
          {meta}
        </Text>
      </div>
      <TrendBars trend={sparkline} width={92} height={38} />
    </div>
  </Card>
);

const TrendBars = ({ trend, width = 120, height = 34 }) => {
  const points = normalizeTrend(trend).slice(-18);
  if (points.length === 0 || points.every((point) => point.requestCount === 0)) {
    return <Text type='tertiary'>-</Text>;
  }

  const maxRequest = Math.max(...points.map((point) => point.requestCount), 1);
  return (
    <div
      className='flex items-end gap-[2px]'
      style={{ width, height, minWidth: width }}
    >
      {points.map((point, index) => {
        const barHeight = Math.max(3, (point.requestCount / maxRequest) * height);
        const color =
          point.requestCount === 0
            ? 'rgba(148, 163, 184, 0.25)'
            : getLineColor(point.successRate);
        return (
          <div
            key={`${point.ts}-${index}`}
            title={`${formatBucketLabel(point.ts, 3600)} ${formatNumber(
              point.requestCount,
            )} / ${formatRate(point.successRate)}`}
            style={{
              height: barHeight,
              flex: 1,
              minWidth: 3,
              borderRadius: 2,
              background: color,
              opacity: point.requestCount === 0 ? 0.35 : 0.9,
            }}
          />
        );
      })}
    </div>
  );
};

const buildVolumeSpec = (history, bucketSeconds, t) => {
  const values = history.map((point) => ({
    time: formatBucketLabel(point.ts, bucketSeconds),
    requests: Number(point.request_count) || 0,
    activeModels: Number(point.active_models) || 0,
  }));

  return {
    type: 'bar',
    data: [{ id: 'volume', values }],
    xField: 'time',
    yField: 'requests',
    height: 260,
    padding: { top: 12, right: 16, bottom: 28, left: 52 },
    bar: {
      style: {
        fill: HEALTH_COLORS.volume,
        fillOpacity: 0.78,
        cornerRadius: [3, 3, 0, 0],
      },
    },
    axes: [
      {
        orient: 'bottom',
        label: { style: { fill: 'var(--semi-color-text-2)', fontSize: 10 } },
        tick: { visible: false },
      },
      {
        orient: 'left',
        label: {
          formatMethod: (value) => formatNumber(value),
          style: { fill: 'var(--semi-color-text-2)', fontSize: 10 },
        },
        grid: {
          visible: true,
          style: { lineDash: [3, 3], stroke: 'rgba(148, 163, 184, 0.28)' },
        },
      },
    ],
    tooltip: {
      mark: {
        title: { value: (datum) => datum.time },
        content: [
          {
            key: t('调用量'),
            value: (datum) => formatNumber(datum.requests),
          },
          {
            key: t('活跃模型'),
            value: (datum) => formatNumber(datum.activeModels),
          },
        ],
      },
    },
  };
};

const buildRateSpec = (history, bucketSeconds, t) => {
  const values = history.map((point) => ({
    time: formatBucketLabel(point.ts, bucketSeconds),
    successRate: Number(point.success_rate) || 0,
    avgLatencyMs: Number(point.avg_latency_ms) || 0,
    downModels: Number(point.down_models) || 0,
  }));

  return {
    type: 'line',
    data: [{ id: 'rate', values }],
    xField: 'time',
    yField: 'successRate',
    height: 260,
    smooth: true,
    padding: { top: 12, right: 16, bottom: 28, left: 52 },
    line: {
      style: {
        stroke: HEALTH_COLORS.healthy,
        lineWidth: 2.5,
      },
    },
    point: {
      visible: true,
      style: {
        size: 5,
        stroke: '#fff',
        lineWidth: 1.5,
        fill: (datum) => getLineColor(datum.successRate),
      },
    },
    axes: [
      {
        orient: 'bottom',
        label: { style: { fill: 'var(--semi-color-text-2)', fontSize: 10 } },
        tick: { visible: false },
      },
      {
        orient: 'left',
        min: 0,
        max: 100,
        label: {
          formatMethod: (value) => `${value}%`,
          style: { fill: 'var(--semi-color-text-2)', fontSize: 10 },
        },
        grid: {
          visible: true,
          style: { lineDash: [3, 3], stroke: 'rgba(148, 163, 184, 0.28)' },
        },
      },
    ],
    tooltip: {
      mark: {
        title: { value: (datum) => datum.time },
        content: [
          {
            key: t('成功率'),
            value: (datum) => formatRate(datum.successRate),
          },
          {
            key: t('平均延迟'),
            value: (datum) => formatLatency(datum.avgLatencyMs),
          },
          {
            key: t('异常模型'),
            value: (datum) => formatNumber(datum.downModels),
          },
        ],
      },
    },
  };
};

const ChartCard = ({ title, meta, spec, hasData, emptyText }) => (
  <Card
    className='rounded-lg'
    title={
      <div className='flex flex-col gap-1'>
        <Text strong>{title}</Text>
        <Text type='tertiary' size='small'>
          {meta}
        </Text>
      </div>
    }
    bodyStyle={{ padding: 12 }}
  >
    {hasData ? (
      <div className='h-[260px]'>
        <VChart spec={spec} option={CHART_CONFIG} />
      </div>
    ) : (
      <div className='flex h-[260px] items-center justify-center'>
        <Empty description={emptyText} />
      </div>
    )}
  </Card>
);

const HealthDistribution = ({ summary, t }) => {
  const total = Math.max(Number(summary.total) || 0, 1);
  const items = [
    { key: 'healthy', label: t('良好'), value: summary.healthy, color: 'var(--semi-color-success)' },
    { key: 'warning', label: t('波动'), value: summary.warning, color: 'var(--semi-color-warning)' },
    { key: 'down', label: t('异常'), value: summary.down, color: 'var(--semi-color-danger)' },
    { key: 'noData', label: t('暂无数据'), value: summary.noData, color: 'var(--semi-color-text-2)' },
  ];

  return (
    <Card className='rounded-lg' title={t('健康分布')} bodyStyle={{ padding: 16 }}>
      <div className='space-y-4'>
        {items.map((item) => {
          const percent = Math.round(((Number(item.value) || 0) / total) * 100);
          return (
            <div key={item.key}>
              <div className='mb-1 flex items-center justify-between'>
                <Text>{item.label}</Text>
                <Text type='tertiary'>
                  {formatNumber(item.value)} / {percent}%
                </Text>
              </div>
              <Progress
                percent={percent}
                showInfo={false}
                stroke={item.color}
                style={{ margin: 0 }}
              />
            </div>
          );
        })}
      </div>
    </Card>
  );
};

const RiskList = ({ rows, t }) => {
  const riskRows = rows
    .filter((row) => row.hasPerf && row.successRate < WARNING_RATE)
    .sort((a, b) => a.successRate - b.successRate || b.requestCount - a.requestCount)
    .slice(0, 6);

  return (
    <Card className='rounded-lg' title={t('近期风险模型')} bodyStyle={{ padding: 16 }}>
      {riskRows.length === 0 ? (
        <Empty description={t('暂无异常模型')} />
      ) : (
        <div className='space-y-3'>
          {riskRows.map((row) => (
            <div
              key={row.model_name}
              className='flex items-center justify-between gap-3 rounded-md border border-solid border-[var(--semi-color-border)] px-3 py-2'
            >
              <div className='min-w-0'>
                <Text
                  strong
                  ellipsis={{ showTooltip: true }}
                  style={{ fontFamily: 'monospace', maxWidth: 220 }}
                >
                  {row.model_name}
                </Text>
                <div className='mt-1 flex gap-2 text-xs text-[var(--semi-color-text-2)]'>
                  <span>{formatNumber(row.requestCount)}</span>
                  <span>{formatLatency(row.avgLatencyMs)}</span>
                  <span>{formatLastSeen(row.lastSeen)}</span>
                </div>
              </div>
              <Tag color={getHealthColor(row.successRate)} shape='circle'>
                {formatRate(row.successRate)}
              </Tag>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
};

const ModelHealthPage = () => {
  const { t } = useTranslation();
  const [healthData, setHealthData] = useState({
    models: [],
    history: [],
    totals: {},
    bucket_seconds: 86400,
    generated_at: 0,
  });
  const [loading, setLoading] = useState(true);
  const [keyword, setKeyword] = useState('');
  const [healthFilter, setHealthFilter] = useState('all');
  const [hours, setHours] = useState(24 * 30);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/models/perf-health?hours=${hours}`, {
        skipErrorHandler: true,
      });
      const payload = res.data?.data || {};
      setHealthData({
        models: Array.isArray(payload.models) ? payload.models : [],
        history: Array.isArray(payload.history) ? payload.history : [],
        totals: payload.totals || {},
        bucket_seconds: Number(payload.bucket_seconds) || 86400,
        generated_at: Number(payload.generated_at) || 0,
      });
    } catch (error) {
      console.error(error);
      showError(t('获取模型健康度失败'));
      setHealthData({
        models: [],
        history: [],
        totals: {},
        bucket_seconds: 86400,
        generated_at: 0,
      });
    } finally {
      setLoading(false);
    }
  }, [hours, t]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const rows = useMemo(() => {
    return healthData.models.map((model) => {
      const requestCount = Number(model.request_count) || 0;
      const successCount = Number(model.success_count) || 0;
      const successRate = Number(model.success_rate);
      const hasPerf = requestCount > 0 && Number.isFinite(successRate);
      return {
        key: model.model_name,
        model_name: model.model_name,
        hasPerf,
        successRate: Number.isFinite(successRate) ? successRate : null,
        avgLatencyMs: Number(model.avg_latency_ms) || 0,
        avgTps: Number(model.avg_tps) || 0,
        requestCount,
        successCount,
        errorCount: Math.max(0, requestCount - successCount),
        lastSeen: Number(model.last_seen) || 0,
        trend: normalizeTrend(model.trend),
      };
    });
  }, [healthData.models]);

  const filteredRows = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();

    return rows.filter((row) => {
      if (
        normalizedKeyword &&
        !row.model_name?.toLowerCase().includes(normalizedKeyword)
      ) {
        return false;
      }

      if (healthFilter === 'no_data') return !row.hasPerf;
      if (!row.hasPerf) return healthFilter === 'all';
      if (healthFilter === 'healthy') return row.successRate >= HEALTHY_RATE;
      if (healthFilter === 'warning') {
        return (
          row.successRate >= WARNING_RATE && row.successRate < HEALTHY_RATE
        );
      }
      if (healthFilter === 'down') return row.successRate < WARNING_RATE;
      return true;
    });
  }, [healthFilter, keyword, rows]);

  const summary = useMemo(() => {
    const totals = healthData.totals || {};
    const total = Number(totals.total_models) || rows.length;
    let healthy = Number(totals.healthy) || 0;
    let warning = Number(totals.warning) || 0;
    let down = Number(totals.down) || 0;
    let noData = 0;

    if (!total && rows.length > 0) {
      for (const row of rows) {
        if (!row.hasPerf) {
          noData += 1;
        } else if (row.successRate >= HEALTHY_RATE) {
          healthy += 1;
        } else if (row.successRate >= WARNING_RATE) {
          warning += 1;
        } else {
          down += 1;
        }
      }
    }

    return {
      total,
      healthy,
      warning,
      down,
      noData,
      requestCount: Number(totals.request_count) || 0,
      successRate: Number(totals.success_rate) || 0,
      avgLatencyMs: Number(totals.avg_latency_ms) || 0,
      avgTps: Number(totals.avg_tps) || 0,
    };
  }, [healthData.totals, rows]);

  const historyTrend = useMemo(
    () =>
      healthData.history.map((point) => ({
        ts: Number(point.ts) || 0,
        requestCount: Number(point.request_count) || 0,
        successRate: Number(point.success_rate) || 0,
      })),
    [healthData.history],
  );

  const volumeSpec = useMemo(
    () => buildVolumeSpec(healthData.history, healthData.bucket_seconds, t),
    [healthData.bucket_seconds, healthData.history, t],
  );

  const rateSpec = useMemo(
    () => buildRateSpec(healthData.history, healthData.bucket_seconds, t),
    [healthData.bucket_seconds, healthData.history, t],
  );

  const columns = useMemo(
    () => [
      {
        title: t('模型名称'),
        dataIndex: 'model_name',
        width: 280,
        fixed: 'left',
        render: (value) => (
          <Text
            copyable={{ content: value }}
            ellipsis={{ showTooltip: true }}
            style={{ fontFamily: 'monospace', maxWidth: 250 }}
          >
            {value}
          </Text>
        ),
      },
      {
        title: t('历史趋势'),
        dataIndex: 'trend',
        width: 150,
        render: (value) => <TrendBars trend={value} />,
      },
      {
        title: t('健康度'),
        dataIndex: 'successRate',
        width: 150,
        sorter: (a, b) => (a.successRate ?? -1) - (b.successRate ?? -1),
        render: (value, record) => {
          if (!record.hasPerf) {
            return (
              <Tag size='small' shape='circle' color='white'>
                {t('暂无数据')}
              </Tag>
            );
          }

          return (
            <Space spacing={6}>
              <Tag
                size='small'
                shape='circle'
                color={getHealthColor(value)}
                style={{ minWidth: 70, textAlign: 'center' }}
              >
                {formatRate(value)}
              </Tag>
              <Text type='tertiary' size='small'>
                {getHealthLabel(value, t)}
              </Text>
            </Space>
          );
        },
      },
      {
        title: t('样本数'),
        dataIndex: 'requestCount',
        width: 120,
        sorter: (a, b) => a.requestCount - b.requestCount,
        render: (value) => formatNumber(value),
      },
      {
        title: t('失败数'),
        dataIndex: 'errorCount',
        width: 110,
        sorter: (a, b) => a.errorCount - b.errorCount,
        render: (value) => (
          <Text type={value > 0 ? 'danger' : 'tertiary'}>
            {formatNumber(value)}
          </Text>
        ),
      },
      {
        title: t('平均延迟'),
        dataIndex: 'avgLatencyMs',
        width: 130,
        sorter: (a, b) => a.avgLatencyMs - b.avgLatencyMs,
        render: (value, record) =>
          record.hasPerf ? formatLatency(value) : '-',
      },
      {
        title: 'TPS',
        dataIndex: 'avgTps',
        width: 120,
        sorter: (a, b) => a.avgTps - b.avgTps,
        render: (value, record) => (record.hasPerf ? formatTps(value) : '-'),
      },
      {
        title: t('最近活跃'),
        dataIndex: 'lastSeen',
        width: 140,
        sorter: (a, b) => a.lastSeen - b.lastSeen,
        render: (value) => formatLastSeen(value),
      },
    ],
    [t],
  );

  const windowText = formatWindow(hours, t);
  const bucketText = formatBucket(healthData.bucket_seconds, t);
  const updatedText = healthData.generated_at
    ? dayjs.unix(healthData.generated_at).format('MM-DD HH:mm:ss')
    : '-';

  return (
    <div className='mt-[60px] px-2'>
      <div className='mb-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
        <div>
          <Title heading={4} style={{ margin: 0 }}>
            {t('模型健康度')}
          </Title>
          <Text type='tertiary'>
            {windowText} · {bucketText} · {t('更新')} {updatedText}
          </Text>
        </div>
        <Space wrap>
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型')}
            value={keyword}
            onChange={setKeyword}
            style={{ width: 240 }}
            showClear
          />
          <Select
            value={healthFilter}
            onChange={setHealthFilter}
            style={{ width: 130 }}
          >
            <Select.Option value='all'>{t('全部')}</Select.Option>
            <Select.Option value='healthy'>{t('良好')}</Select.Option>
            <Select.Option value='warning'>{t('波动')}</Select.Option>
            <Select.Option value='down'>{t('异常')}</Select.Option>
            <Select.Option value='no_data'>{t('暂无数据')}</Select.Option>
          </Select>
          <Select value={hours} onChange={setHours} style={{ width: 120 }}>
            <Select.Option value={1}>{t('近 1 小时')}</Select.Option>
            <Select.Option value={6}>{t('近 6 小时')}</Select.Option>
            <Select.Option value={24}>{t('近 24 小时')}</Select.Option>
            <Select.Option value={72}>{t('近 72 小时')}</Select.Option>
            <Select.Option value={24 * 30}>{t('近 30 天')}</Select.Option>
          </Select>
          <Button icon={<IconRefresh />} loading={loading} onClick={loadData}>
            {t('刷新')}
          </Button>
        </Space>
      </div>

      <div className='mb-3 grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          label={t('模型总数')}
          value={formatNumber(summary.total)}
          meta={`${formatNumber(summary.healthy)} ${t('良好')} / ${formatNumber(
            summary.down,
          )} ${t('异常')}`}
          tone='volume'
          sparkline={historyTrend}
        />
        <MetricCard
          label={t('总调用量')}
          value={formatNumber(summary.requestCount)}
          meta={`${windowText} ${bucketText}`}
          tone='volume'
          sparkline={historyTrend}
        />
        <MetricCard
          label={t('整体成功率')}
          value={formatRate(summary.successRate)}
          meta={`${formatNumber(summary.down)} ${t('异常模型')}`}
          tone={getHealthTone(summary.successRate)}
          sparkline={historyTrend}
        />
        <MetricCard
          label={t('平均延迟')}
          value={formatLatency(summary.avgLatencyMs)}
          meta={formatTps(summary.avgTps)}
          tone='latency'
          sparkline={historyTrend}
        />
      </div>

      <div className='mb-3 grid grid-cols-1 gap-3 xl:grid-cols-2'>
        <ChartCard
          title={t('调用量历史')}
          meta={`${windowText} · ${bucketText}`}
          spec={volumeSpec}
          hasData={summary.requestCount > 0}
          emptyText={t('暂无历史调用数据')}
        />
        <ChartCard
          title={t('成功率历史')}
          meta={`${windowText} · ${bucketText}`}
          spec={rateSpec}
          hasData={summary.requestCount > 0}
          emptyText={t('暂无成功率数据')}
        />
      </div>

      <div className='mb-3 grid grid-cols-1 gap-3 xl:grid-cols-3'>
        <HealthDistribution summary={summary} t={t} />
        <div className='xl:col-span-2'>
          <RiskList rows={rows} t={t} />
        </div>
      </div>

      <Card className='rounded-lg'>
        <Table
          columns={columns}
          dataSource={filteredRows}
          loading={loading}
          pagination={{ pageSize: 20, showSizeChanger: true }}
          scroll={{ x: 'max-content' }}
          empty={<Empty description={t('暂无数据')} />}
          size='middle'
        />
      </Card>
    </div>
  );
};

export default ModelHealthPage;
