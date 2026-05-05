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

import React, { useEffect, useMemo, useState } from 'react';
import { Banner, Button, Card, Empty, Select, Spin, Typography } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import ConsolePage from '../../components/layout/ConsolePage';
import { API, showError, showSuccess } from '../../helpers';

const { Title, Text } = Typography;

const RANGE_OPTIONS = [
  { label: '最近 15 分钟', value: 15 },
  { label: '最近 60 分钟', value: 60 },
  { label: '最近 180 分钟', value: 180 },
];

const formatMinuteLabel = (ts) => {
  if (!Number.isFinite(ts) || ts <= 0) return '-';
  const d = new Date(ts * 1000);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${hh}:${mm}`;
};

const buildLineValues = (points, series) => {
  const values = [];
  const normalizedPoints = Array.isArray(points) ? points : [];
  const normalizedSeries = Array.isArray(series) ? series : [];
  normalizedSeries.forEach((item) => {
    const valuesList = Array.isArray(item?.values) ? item.values : [];
    normalizedPoints.forEach((point, idx) => {
      values.push({
        time: formatMinuteLabel(point),
        value: Number(valuesList[idx] || 0),
        type: item?.label || item?.key || 'unknown',
      });
    });
  });
  return values;
};

const buildLineSpec = (title, points, series) => ({
  type: 'line',
  data: [
    {
      id: 'tmp-stats',
      values: buildLineValues(points, series),
    },
  ],
  xField: 'time',
  yField: 'value',
  seriesField: 'type',
  padding: { top: 32, right: 24, bottom: 40, left: 48 },
  legends: {
    visible: true,
    orient: 'top',
  },
  title: {
    visible: true,
    text: title,
  },
  line: {
    style: {
      lineWidth: 2,
    },
  },
  point: {
    visible: false,
  },
});

const buildBarSpec = (items) => ({
  type: 'bar',
  data: [
    {
      id: 'top-groups',
      values: (Array.isArray(items) ? items : []).map((item) => ({
        group: item?.label || `#${item?.group_id || 0}`,
        value: Number(item?.count || 0),
      })),
    },
  ],
  xField: 'group',
  yField: 'value',
  padding: { top: 32, right: 24, bottom: 56, left: 48 },
  title: {
    visible: true,
    text: 'Top using_group_id',
  },
  label: {
    visible: true,
    position: 'top',
  },
});

const logicModeLabelMap = {
  legacy_default_model_group_logic: '旧逻辑',
  new_runtime_logic: '新逻辑',
  unknown_logic: '未知',
};

const TmpStatsPage = () => {
  const [minutes, setMinutes] = useState(60);
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);

  const loadData = async (nextMinutes = minutes, silent = false) => {
    if (!silent) {
      setLoading(true);
    }
    try {
      const res = await API.get('/api/tmp_stats/relay_billing', {
        params: { minutes: nextMinutes },
      });
      if (!res.data?.success) {
        showError(res.data?.message || '获取临时统计失败');
        return;
      }
      setData(res.data?.data || null);
    } catch (error) {
      showError(error?.message || '获取临时统计失败');
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  };

  const resetData = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/tmp_stats/relay_billing/reset');
      if (!res.data?.success) {
        showError(res.data?.message || '清空临时统计失败');
        return;
      }
      showSuccess('已清空临时统计');
      await loadData(minutes, true);
    } catch (error) {
      showError(error?.message || '清空临时统计失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData(minutes);
  }, [minutes]);

  useEffect(() => {
    const timer = setInterval(() => {
      loadData(minutes, true);
    }, 5000);
    return () => clearInterval(timer);
  }, [minutes]);

  const quotaBucketSpec = useMemo(
    () =>
      buildLineSpec(
        'Quota bucket 实际命中趋势',
        data?.points,
        data?.quota_bucket_series,
      ),
    [data],
  );

  const decisionSourceSpec = useMemo(
    () =>
      buildLineSpec(
        'Decision source 细分趋势',
        data?.points,
        data?.decision_source_series,
      ),
    [data],
  );

  const logicModeSpec = useMemo(() => {
    const normalizedSeries = Array.isArray(data?.logic_mode_series)
      ? data.logic_mode_series.map((item) => ({
          ...item,
          label: logicModeLabelMap[item?.key] || item?.label || item?.key,
        }))
      : [];
    return buildLineSpec('新逻辑 / 旧逻辑趋势', data?.points, normalizedSeries);
  }, [data]);

  const topUsingGroupsSpec = useMemo(
    () => buildBarSpec(data?.top_using_groups),
    [data],
  );

  const hasData = Number(data?.total || 0) > 0;
  const legacyCount = Number(data?.legacy_count || 0);
  const newCount = Number(data?.new_count || 0);
  const totalCount = Number(data?.total || 0);
  const legacyRatio =
    totalCount > 0 ? `${((legacyCount / totalCount) * 100).toFixed(2)}%` : '0.00%';

  return (
    <ConsolePage className='px-4 py-4' innerClassName='gap-4' fillHeight>
      <div className='flex items-center justify-between gap-3'>
        <div>
          <Title heading={4} style={{ margin: 0 }}>
            临时消费决策统计
          </Title>
          <Text type='secondary'>
            仅内存统计，不落库。优先判断当前请求究竟走的是新逻辑还是旧逻辑。
          </Text>
        </div>
        <div className='flex items-center gap-2'>
          <Select
            value={minutes}
            optionList={RANGE_OPTIONS}
            onChange={(value) => setMinutes(Number(value) || 60)}
            style={{ width: 160 }}
          />
          <Button theme='light' onClick={() => loadData(minutes)}>
            刷新
          </Button>
          <Button type='danger' theme='light' onClick={resetData}>
            清空
          </Button>
        </div>
      </div>

      <Banner
        type='warning'
        bordered
        closeIcon={null}
        description='旧逻辑 = using_group_id 最终直接落在默认模型分组 fallback；新逻辑 = using_group_id 来自 token 主分组或运行时重新选择。'
      />

      <Card>
        <div className='flex flex-wrap gap-6'>
          <div>
            <Text type='secondary'>统计窗口</Text>
            <div className='text-xl font-semibold'>{data?.range_minutes || minutes} 分钟</div>
          </div>
          <div>
            <Text type='secondary'>总样本数</Text>
            <div className='text-xl font-semibold'>{data?.total || 0}</div>
          </div>
          <div>
            <Text type='secondary'>旧逻辑样本数</Text>
            <div className='text-xl font-semibold text-amber-600'>{legacyCount}</div>
          </div>
          <div>
            <Text type='secondary'>新逻辑样本数</Text>
            <div className='text-xl font-semibold text-emerald-600'>{newCount}</div>
          </div>
          <div>
            <Text type='secondary'>旧逻辑占比</Text>
            <div className='text-xl font-semibold'>{legacyRatio}</div>
          </div>
          <div>
            <Text type='secondary'>最近更新时间</Text>
            <div className='text-xl font-semibold'>
              {data?.generated_at ? formatMinuteLabel(data.generated_at) : '-'}
            </div>
          </div>
        </div>
      </Card>

      <Spin spinning={loading}>
        {hasData ? (
          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <Card>
              <div style={{ height: 360 }}>
                <VChart spec={logicModeSpec} />
              </div>
            </Card>
            <Card>
              <div style={{ height: 360 }}>
                <VChart spec={quotaBucketSpec} />
              </div>
            </Card>
            <Card>
              <div style={{ height: 360 }}>
                <VChart spec={decisionSourceSpec} />
              </div>
            </Card>
            <Card>
              <div style={{ height: 360 }}>
                <VChart spec={topUsingGroupsSpec} />
              </div>
            </Card>
          </div>
        ) : (
          <Card>
            <Empty title='暂无统计数据' description='等有真实请求命中消费决策后，这里会自动出现图表。' />
          </Card>
        )}
      </Spin>
    </ConsolePage>
  );
};

export default TmpStatsPage;
