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
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Card,
  DatePicker,
  Empty,
  InputNumber,
  Select,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  RefreshCw,
} from 'lucide-react';
import ConsolePage from '../../components/layout/ConsolePage';
import { API, showError } from '../../helpers';

const { Title } = Typography;

const STATUS = {
  NO_DATA: 0,
  UP: 1,
  DEGRADED: 2,
  DOWN: 3,
};

const STATUS_META = {
  [STATUS.NO_DATA]: { color: '#94a3b8', label: '无数据' },
  [STATUS.UP]: { color: '#10b981', label: '正常' },
  [STATUS.DEGRADED]: { color: '#f59e0b', label: '部分故障' },
  [STATUS.DOWN]: { color: '#ef4444', label: '服务中断' },
};

const SEGMENT_WIDTH = 6;
const SEGMENT_HEIGHT = 18;
const SEGMENT_GAP = 2;
const SEGMENT_PITCH = SEGMENT_WIDTH + SEGMENT_GAP;
const LABEL_COL_WIDTH = 260;

const pad2 = (n) => String(n).padStart(2, '0');

const clamp01 = (value) => Math.min(1, Math.max(0, value));

const availabilityColor = (serverErrorRate) => {
  const t = Math.pow(clamp01(serverErrorRate), 2);
  const hue = 140 * (1 - t);
  return `hsl(${hue}, 82%, 45%)`;
};

const formatBucketLabel = (ts, bucket) => {
  const d = new Date(ts * 1000);
  const y = d.getFullYear();
  const m = pad2(d.getMonth() + 1);
  const day = pad2(d.getDate());
  if (bucket === 'minute') {
    const hh = pad2(d.getHours());
    const mm = pad2(d.getMinutes());
    return `${y}-${m}-${day} ${hh}:${mm}`;
  }
  if (bucket === 'hour') {
    const hh = pad2(d.getHours());
    return `${y}-${m}-${day} ${hh}:00`;
  }
  return `${y}-${m}-${day}`;
};

const formatPercent = (value) => `${(value * 100).toFixed(2)}%`;

const buildMonthMarkers = (bucketStarts) => {
  if (!Array.isArray(bucketStarts) || bucketStarts.length === 0) return [];
  const markers = [];
  let lastKey = '';
  bucketStarts.forEach((ts, idx) => {
    const d = new Date(ts * 1000);
    const key = `${d.getFullYear()}-${d.getMonth() + 1}`;
    if (key === lastKey) return;
    lastKey = key;
    markers.push({
      idx,
      label: `${d.getFullYear()}-${pad2(d.getMonth() + 1)}`,
    });
  });
  return markers;
};

const buildBackfillHint = (timeline, t) => {
  if (!timeline?.backfill_pending) return '';
  const boundary = Number(timeline?.backfilled_start || 0);
  if (
    Number.isFinite(boundary) &&
    boundary > 0 &&
    boundary > Number(timeline?.start || 0) &&
    boundary < Number(timeline?.end || 0)
  ) {
    return `${formatBucketLabel(boundary, timeline.bucket)} ${t('之前的历史区间正在后台回填，页面会自动刷新')}`;
  }
  return t('当前查询范围正在后台回填，页面会自动刷新');
};

const ServiceStatus = () => {
  const { t } = useTranslation();
  const [bucket, setBucket] = useState('');
  const [rangeMinutes, setRangeMinutes] = useState(null);
  const [startAt, setStartAt] = useState(null);
  const [endAt, setEndAt] = useState(null);
  const [loading, setLoading] = useState(false);
  const [timeline, setTimeline] = useState(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState(null);

  const bucketStarts = timeline?.bucket_starts || [];
  const groups = timeline?.groups || [];

  const monthMarkers = useMemo(
    () => buildMonthMarkers(bucketStarts),
    [bucketStarts],
  );

  const barWidthPx = useMemo(
    () => Math.max(0, bucketStarts.length * SEGMENT_PITCH),
    [bucketStarts.length],
  );

  const backfillHintText = useMemo(
    () => buildBackfillHint(timeline, t),
    [timeline, t],
  );

  const loadTimeline = async () => {
    const params = {};
    if (bucket) {
      params.bucket = bucket;
    }

    if (bucket === 'minute') {
      const minutes = Number(rangeMinutes);
      if (Number.isFinite(minutes) && minutes > 0) {
        const end = Math.floor(Date.now() / 1000);
        const start = end - Math.floor(minutes) * 60;
        if (!Number.isFinite(start) || !Number.isFinite(end) || start >= end) {
          showError(t('start/end 无效'));
          return;
        }
        params.start = start;
        params.end = end;
      }
    } else {
      const startDate =
        startAt instanceof Date && !Number.isNaN(startAt.getTime())
          ? new Date(startAt)
          : null;
      const endDate =
        endAt instanceof Date && !Number.isNaN(endAt.getTime())
          ? new Date(endAt)
          : null;

      if (startDate && endDate) {
        // DatePicker 选择的是「日期」，这里按自然日统计：start=当日 00:00:00，end=次日 00:00:00（区间右开）
        startDate.setHours(0, 0, 0, 0);
        endDate.setHours(0, 0, 0, 0);
        const start = Math.floor(startDate.getTime() / 1000);
        const end = Math.floor(endDate.getTime() / 1000) + 86400;
        if (!Number.isFinite(start) || !Number.isFinite(end) || start >= end) {
          showError(t('start/end 无效'));
          return;
        }
        params.start = start;
        params.end = end;
      }
    }

    setLoading(true);
    try {
      const res = await API.get('/api/service_status/timeline', {
        params,
      });
      if (!res.data?.success) {
        showError(res.data?.message || t('获取服务状态失败'));
        setTimeline(null);
        return;
      }
      const data = res.data.data || null;
      setTimeline(data);
      if (data?.bucket) {
        setBucket(data.bucket);
      }
      if (data?.bucket === 'minute' && data?.start && data?.end) {
        const minutes = Math.round((data.end - data.start) / 60);
        setRangeMinutes(Number.isFinite(minutes) && minutes > 0 ? minutes : null);
      }
      if (data?.start && data?.end) {
        const nextStart = new Date(data.start * 1000);
        nextStart.setHours(0, 0, 0, 0);

        const nextEnd = new Date((data.end - 1) * 1000);
        nextEnd.setHours(0, 0, 0, 0);

        setStartAt(nextStart);
        setEndAt(nextEnd);
      }
      setLastUpdatedAt(new Date());
    } catch (err) {
      showError(err?.message || t('获取服务状态失败'));
      setTimeline(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTimeline();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!timeline?.backfill_pending || loading) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      loadTimeline();
    }, 3000);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeline?.backfill_pending, timeline?.backfilled_start, loading]);

  const hasData = Array.isArray(groups) && groups.length > 0;

  const legendItems = useMemo(
    () => [
      {
        status: STATUS.UP,
        label: t(STATUS_META[STATUS.UP].label),
        color: STATUS_META[STATUS.UP].color,
      },
      {
        status: STATUS.DEGRADED,
        label: t(STATUS_META[STATUS.DEGRADED].label),
        color: STATUS_META[STATUS.DEGRADED].color,
      },
      {
        status: STATUS.DOWN,
        label: t(STATUS_META[STATUS.DOWN].label),
        color: STATUS_META[STATUS.DOWN].color,
      },
      {
        status: STATUS.NO_DATA,
        label: t(STATUS_META[STATUS.NO_DATA].label),
        color: STATUS_META[STATUS.NO_DATA].color,
      },
    ],
    [t],
  );

  const bucketOptions = useMemo(
    () => [
      { label: t('分钟'), value: 'minute' },
      { label: t('小时'), value: 'hour' },
      { label: t('天'), value: 'day' },
    ],
    [t],
  );

  return (
    <ConsolePage fillHeight>
      <div className='flex min-h-0 flex-1 flex-col gap-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <div className='inline-flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500/15 via-sky-500/10 to-rose-500/10 ring-1 ring-black/5 dark:ring-white/10'>
                <Activity size={18} className='text-emerald-600' />
              </div>
              <div>
                <Title heading={4} className='!mb-0'>
                  {t('服务状态')}
                </Title>
              </div>
            </div>
          </div>

          <div className='flex flex-wrap items-center gap-2'>
            {bucket === 'minute' ? (
              <InputNumber
                size='small'
                step={1}
                min={1}
                max={4000}
                value={rangeMinutes ?? undefined}
                onChange={(v) => setRangeMinutes(v)}
                suffix={t('分钟')}
                placeholder={t('近 N 分钟')}
                className='w-[150px]'
              />
            ) : (
              <>
                <DatePicker
                  type='date'
                  size='small'
                  value={startAt}
                  onChange={(v) => setStartAt(v)}
                  placeholder={t('起始时间')}
                  className='w-[140px]'
                />
                <DatePicker
                  type='date'
                  size='small'
                  value={endAt}
                  onChange={(v) => setEndAt(v)}
                  placeholder={t('结束时间')}
                  className='w-[140px]'
                />
              </>
            )}
            <Select
              size='small'
              value={bucket}
              optionList={bucketOptions}
              onChange={(v) => setBucket(v)}
              className='w-[110px]'
            />
            <Button
              type='primary'
              theme='solid'
              size='small'
              onClick={loadTimeline}
              loading={loading}
              className='!rounded-lg'
            >
              {t('确认')}
            </Button>
            <Button
              icon={<RefreshCw size={14} />}
              theme='light'
              size='small'
              onClick={loadTimeline}
              loading={loading}
              className='!rounded-lg'
            >
              {t('刷新')}
            </Button>
          </div>
        </div>

        <Card
          className='!rounded-2xl !border-0 shadow-sm'
          bodyStyle={{ padding: 16 }}
        >
          <div className='flex flex-wrap items-center gap-3'>
            {legendItems.map((item) => (
              <div key={item.status} className='flex items-center gap-2'>
                <span
                  className='h-2.5 w-2.5 rounded-full'
                  style={{ backgroundColor: item.color }}
                />
                <span className='text-xs text-neutral-600 dark:text-neutral-300'>
                  {item.label}
                </span>
              </div>
            ))}
          </div>

          <div className='mt-4'>
            {timeline?.backfill_pending ? (
              <Banner
                type='info'
                description={backfillHintText}
                className='mb-4'
              />
            ) : null}
            <Spin spinning={loading}>
              {!hasData ? (
                <Empty
                  description={t('暂无监控数据')}
                  className='py-12'
                />
              ) : (
                <div className='scrollbar-hide max-h-[520px] overflow-y-auto'>
                  <div className='overflow-x-auto'>
                    <div
                      className='min-w-full'
                      style={{
                        width: LABEL_COL_WIDTH + barWidthPx,
                      }}
                    >
                      {/* Axis */}
                      <div className='flex items-end border-b border-black/5 dark:border-white/10'>
                        <div
                          className='sticky left-0 z-20 flex items-center gap-2 px-4 py-3 bg-white dark:bg-neutral-950'
                          style={{
                            width: LABEL_COL_WIDTH,
                            minWidth: LABEL_COL_WIDTH,
                          }}
                        >
                          <span className='text-xs font-medium text-neutral-600 dark:text-neutral-300'>
                            {t('分组')}
                          </span>
                          <span className='text-[10px] text-neutral-400'>
                            {lastUpdatedAt
                              ? `${t('更新于')}: ${lastUpdatedAt.toLocaleString()}`
                              : ''}
                          </span>
                        </div>
                        <div
                          className='relative px-3 pb-3 pt-2'
                          style={{ width: barWidthPx }}
                        >
                          {monthMarkers.map((m) => (
                            <div
                              key={`${m.label}-${m.idx}`}
                              className='absolute top-2 text-[10px] text-neutral-500 dark:text-neutral-400'
                              style={{
                                left: m.idx * SEGMENT_PITCH,
                              }}
                            >
                              {m.label}
                            </div>
                          ))}
                          <div className='h-5' />
                          <div className='text-[10px] text-neutral-400'>
                            {t('时间跨度')}: {bucketStarts.length}{' '}
                            {bucket === 'minute'
                              ? t('分钟')
                              : bucket === 'hour'
                                ? t('小时')
                                : t('天')}
                          </div>
                        </div>
                      </div>

                      {/* Rows */}
                      {groups.map((g) => {
                        const uptimeText =
                          g.data_buckets > 0 ? formatPercent(g.uptime) : '—';
                        const description = String(g.description || '').trim();
                        return (
                          <div
                            key={g.code}
                            className='flex border-b border-black/5 dark:border-white/10'
                          >
                            <div
                              className='sticky left-0 z-10 px-4 py-2.5 bg-white dark:bg-neutral-950'
                              style={{
                                width: LABEL_COL_WIDTH,
                                minWidth: LABEL_COL_WIDTH,
                              }}
                            >
                              <div className='flex items-start justify-between gap-2'>
                                <div className='min-w-0'>
                                  <div className='truncate text-sm font-semibold text-neutral-900 dark:text-neutral-50'>
                                    {g.display_name || g.code}
                                  </div>
                                  <div className='mt-0.5 text-[11px] text-neutral-500 dark:text-neutral-400'>
                                    {t('可用率')}: {uptimeText}
                                    {g.data_buckets > 0
                                      ? ` · ${g.available_buckets}/${g.data_buckets}`
                                      : ''}
                                  </div>
                                </div>
                              </div>
                            </div>

                            <div
                              className='flex flex-col items-start justify-center px-3 py-2.5'
                              style={{ width: barWidthPx }}
                            >
                              {description ? (
                                <div
                                  className='mb-1 w-full truncate text-left text-[11px] text-neutral-500 dark:text-neutral-400'
                                  title={description}
                                >
                                  {description}
                                </div>
                              ) : null}
                              <div className='flex' style={{ gap: SEGMENT_GAP }}>
                                {bucketStarts.map((ts, idx) => {
                                  const status = Number(g.statuses?.[idx] ?? 0);
                                  const meta =
                                    STATUS_META[status] ||
                                    STATUS_META[STATUS.NO_DATA];
                                  const success = Number(g.success?.[idx] ?? 0);
                                  const serverErrors = Number(
                                    g.server_errors?.[idx] ?? 0,
                                  );
                                  const total = success + serverErrors;
                                  const serverErrorRate =
                                    total > 0 ? serverErrors / total : 0;
                                  const segmentUptimeText =
                                    total > 0
                                      ? formatPercent(success / total)
                                      : '—';
                                  const label = t(meta.label);
                                  const bucketLabel = formatBucketLabel(
                                    ts,
                                    bucket,
                                  );
                                  const title = `${g.display_name || g.code}\n${bucketLabel}\n${label}\n${t('可用率')}: ${segmentUptimeText}`;
                                  const fillColor =
                                    status === STATUS.NO_DATA
                                      ? STATUS_META[STATUS.NO_DATA].color
                                      : availabilityColor(serverErrorRate);

                                  return (
                                    <div
                                      key={`${g.code}-${ts}`}
                                      title={title}
                                      className='rounded-sm'
                                      style={{
                                        width: SEGMENT_WIDTH,
                                        height: SEGMENT_HEIGHT,
                                        backgroundColor: fillColor,
                                        boxShadow: 'inset 0 0 0 1px rgba(15, 23, 42, 0.06)',
                                        opacity:
                                          status === STATUS.NO_DATA ? 0.35 : 1,
                                      }}
                                    />
                                  );
                                })}
                              </div>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </div>
              )}
            </Spin>
          </div>
        </Card>
      </div>
    </ConsolePage>
  );
};

export default ServiceStatus;
