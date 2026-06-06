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
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconSearch } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

const { Text, Title } = Typography;

const PAGE_SIZE = 500;
const PERF_BATCH_SIZE = 300;
const HEALTHY_RATE = 99.9;
const WARNING_RATE = 99;

const formatLatency = (latencyMs) => {
  const value = Number(latencyMs);
  if (!Number.isFinite(value) || value <= 0) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(2)}s`;
};

const formatTps = (tps) => {
  const value = Number(tps);
  if (!Number.isFinite(value) || value <= 0) return '-';
  return `${value.toFixed(value >= 10 ? 1 : 2)} tps`;
};

const getHealthColor = (successRate) => {
  if (successRate >= HEALTHY_RATE) return 'green';
  if (successRate >= WARNING_RATE) return 'orange';
  return 'red';
};

const getHealthLabel = (successRate, t) => {
  if (successRate >= HEALTHY_RATE) return t('良好');
  if (successRate >= WARNING_RATE) return t('波动');
  return t('异常');
};

const extractItems = (payload) => {
  const items = payload?.items || payload || [];
  return Array.isArray(items) ? items : [];
};

const StatCard = ({ label, value, color }) => (
  <Card shadows='hover' className='rounded-xl'>
    <div className='flex items-center justify-between gap-3'>
      <Text type='tertiary'>{label}</Text>
      <Text
        strong
        style={{
          color,
          fontSize: 22,
          fontVariantNumeric: 'tabular-nums',
          lineHeight: 1,
        }}
      >
        {value}
      </Text>
    </div>
  </Card>
);

const ModelHealthPage = () => {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [vendors, setVendors] = useState([]);
  const [perfMap, setPerfMap] = useState({});
  const [loading, setLoading] = useState(true);
  const [keyword, setKeyword] = useState('');
  const [healthFilter, setHealthFilter] = useState('all');
  const [hours, setHours] = useState(24);

  const vendorMap = useMemo(() => {
    const map = {};
    for (const vendor of vendors) {
      map[vendor.id] = vendor;
    }
    return map;
  }, [vendors]);

  const loadAllModels = useCallback(async () => {
    const first = await API.get(`/api/models/?p=1&page_size=${PAGE_SIZE}`);
    const firstData = first.data?.data || {};
    const firstItems = extractItems(firstData);
    const total = firstData.total || firstItems.length;
    const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));
    const allItems = [...firstItems];

    for (let page = 2; page <= pageCount; page += 1) {
      const res = await API.get(
        `/api/models/?p=${page}&page_size=${PAGE_SIZE}`,
      );
      allItems.push(...extractItems(res.data?.data));
    }

    return allItems;
  }, []);

  const loadPerfSummary = useCallback(async (modelNames, targetHours) => {
    const nextMap = {};
    for (let start = 0; start < modelNames.length; start += PERF_BATCH_SIZE) {
      const batch = modelNames.slice(start, start + PERF_BATCH_SIZE);
      if (batch.length === 0) continue;

      const res = await API.post(
        '/api/models/perf-summary',
        { models: batch, hours: targetHours },
        { skipErrorHandler: true },
      );
      const items = res.data?.data?.models || [];
      for (const item of items) {
        if (item?.model_name) {
          nextMap[item.model_name] = item;
        }
      }
    }
    return nextMap;
  }, []);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [modelItems, vendorRes] = await Promise.all([
        loadAllModels(),
        API.get('/api/vendors/?page_size=1000').catch(() => null),
      ]);

      const vendorItems = extractItems(vendorRes?.data?.data);
      const names = modelItems.map((model) => model.model_name).filter(Boolean);
      const nextPerfMap = await loadPerfSummary(names, hours);

      setModels(modelItems);
      setVendors(vendorItems);
      setPerfMap(nextPerfMap);
    } catch (error) {
      console.error(error);
      showError(t('获取模型健康度失败'));
      setModels([]);
      setVendors([]);
      setPerfMap({});
    } finally {
      setLoading(false);
    }
  }, [hours, loadAllModels, loadPerfSummary, t]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const rows = useMemo(() => {
    return models.map((model) => {
      const perf = perfMap[model.model_name];
      const successRate = Number(perf?.success_rate);
      return {
        ...model,
        key: model.id || model.model_name,
        vendorName: vendorMap[model.vendor_id]?.name || '-',
        hasPerf: !!perf,
        successRate: Number.isFinite(successRate) ? successRate : null,
        avgLatencyMs: Number(perf?.avg_latency_ms) || 0,
        avgTps: Number(perf?.avg_tps) || 0,
        requestCount: Number(perf?.request_count) || 0,
      };
    });
  }, [models, perfMap, vendorMap]);

  const filteredRows = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();

    return rows.filter((row) => {
      if (
        normalizedKeyword &&
        !row.model_name?.toLowerCase().includes(normalizedKeyword) &&
        !row.vendorName?.toLowerCase().includes(normalizedKeyword)
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
    let healthy = 0;
    let warning = 0;
    let down = 0;
    let noData = 0;

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

    return { total: rows.length, healthy, warning, down, noData };
  }, [rows]);

  const columns = useMemo(
    () => [
      {
        title: t('模型名称'),
        dataIndex: 'model_name',
        width: 260,
        fixed: 'left',
        render: (value) => (
          <Text
            copyable={{ content: value }}
            style={{ fontFamily: 'monospace' }}
          >
            {value}
          </Text>
        ),
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
                style={{ minWidth: 62, textAlign: 'center' }}
              >
                {value.toFixed(1)}%
              </Tag>
              <Text type='tertiary' size='small'>
                {getHealthLabel(value, t)}
              </Text>
            </Space>
          );
        },
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
        title: t('样本数'),
        dataIndex: 'requestCount',
        width: 110,
        sorter: (a, b) => a.requestCount - b.requestCount,
        render: (value, record) => (record.hasPerf ? value : '-'),
      },
      {
        title: t('供应商'),
        dataIndex: 'vendorName',
        width: 150,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 100,
        render: (value) => (
          <Tag
            size='small'
            shape='circle'
            color={value === 1 ? 'green' : 'red'}
          >
            {value === 1 ? t('启用') : t('禁用')}
          </Tag>
        ),
      },
    ],
    [t],
  );

  return (
    <div className='mt-[60px] px-2'>
      <div className='mb-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
        <div>
          <Title heading={4} style={{ margin: 0 }}>
            {t('模型健康度')}
          </Title>
          <Text type='tertiary'>
            {t('基于历史调用和渠道测试统计模型成功率、延迟和吞吐')}
          </Text>
        </div>
        <Space wrap>
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型或供应商')}
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
          </Select>
          <Button icon={<IconRefresh />} loading={loading} onClick={loadData}>
            {t('刷新')}
          </Button>
        </Space>
      </div>

      <div className='mb-3 grid grid-cols-2 gap-3 lg:grid-cols-5'>
        <StatCard label={t('模型总数')} value={summary.total} />
        <StatCard
          label={t('良好')}
          value={summary.healthy}
          color='var(--semi-color-success)'
        />
        <StatCard
          label={t('波动')}
          value={summary.warning}
          color='var(--semi-color-warning)'
        />
        <StatCard
          label={t('异常')}
          value={summary.down}
          color='var(--semi-color-danger)'
        />
        <StatCard
          label={t('暂无数据')}
          value={summary.noData}
          color='var(--semi-color-text-2)'
        />
      </div>

      <Card className='rounded-xl'>
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
