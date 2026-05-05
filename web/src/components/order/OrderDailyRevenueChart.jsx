import React, { useCallback, useEffect, useMemo, useState } from 'react';
import dayjs from 'dayjs';
import { DatePicker, Empty, Select, Spin } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { VChart } from '@visactor/react-vchart';
import { API, showError } from '../../helpers';
import { useTranslation } from 'react-i18next';

const ORDER_TYPE_LABELS = {
  subscription: '订阅订单',
  topup: '充值订单',
  payg: '按量订单',
  pay_request: '按次订单',
  pay_token: '按token订单',
};

const PERIOD_OPTIONS = [
  { value: 'day', label: '自然日' },
  { value: 'week', label: '自然周' },
  { value: 'month', label: '自然月' },
  { value: 'year', label: '自然年' },
];

const PERIOD_AVERAGE_LABELS = {
  day: '小时均值',
  week: '日均值',
  month: '日均值',
  year: '月均值',
};

const TYPE_COLORS = {
  subscription: '#3370FF',
  topup: '#F59E0B',
  payg: '#16A34A',
  pay_request: '#EF4444',
  pay_token: '#8B5CF6',
};

const formatYuan = (n) => {
  const v = Number(n || 0);
  if (!Number.isFinite(v)) return '¥0.00';
  return `¥${v.toFixed(2)}`;
};

const formatDateParam = (value) => {
  const d = dayjs(value);
  if (!d.isValid()) return '';
  return d.format('YYYY-MM-DD');
};

const getBrowserTimeZone = () => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
  } catch {
    return '';
  }
};

const formatRangeLabel = (stats) => {
  if (!stats?.start_date || !stats?.end_date) return '';
  if (stats.start_date === stats.end_date) return stats.start_date;
  return `${stats.start_date} ~ ${stats.end_date}`;
};

const getPeriodHint = (period, t) => {
  switch (period) {
    case 'week':
      return t('按所选日期所在自然周统计');
    case 'month':
      return t('按所选日期所在自然月统计');
    case 'year':
      return t('按所选日期所在自然年统计');
    default:
      return t('按所选日期统计当日累计收入');
  }
};

function transformData(stats, t) {
  const items = Array.isArray(stats?.items) ? stats.items : [];
  const dateSet = new Set();
  const typeSet = new Set();
  const map = {};
  const totalsByBucket = {};

  items.forEach((item) => {
    if (!item || !item.bucket) return;
    const bucket = String(item.bucket);
    const label = String(item.label || item.bucket);
    const orderType = String(item.order_type || '');
    const amountFen = Number(item.amount_fen || 0);
    dateSet.add(bucket);
    if (orderType) {
      typeSet.add(orderType);
    }
    const key = `${bucket}__${orderType}`;
    map[key] = {
      bucket,
      label,
      orderType,
      amountFen,
    };
    totalsByBucket[bucket] = (totalsByBucket[bucket] || 0) + amountFen;
  });

  const buckets = Array.from(dateSet);
  const types = Array.from(typeSet);
  const values = [];

  buckets.forEach((bucket) => {
    types.forEach((type) => {
      const item = map[`${bucket}__${type}`];
      const label = item?.label || bucket;
      const amountFen = Number(item?.amountFen || 0);
      values.push({
        bucket,
        label,
        type: t(ORDER_TYPE_LABELS[type] || type),
        rawType: type,
        amount: parseFloat((amountFen / 100).toFixed(2)),
      });
    });
  });

  const totalFen = Number(stats?.total_fen || 0);
  const bucketCount = buckets.length || 1;
  const averageFen = totalFen / bucketCount;

  let peakBucket = '-';
  let peakLabel = '-';
  let peakAmountFen = 0;
  buckets.forEach((bucket) => {
    const amountFen = totalsByBucket[bucket] || 0;
    if (amountFen > peakAmountFen) {
      peakAmountFen = amountFen;
      peakBucket = bucket;
      peakLabel = map[`${bucket}__${types[0] || ''}`]?.label || bucket;
    }
  });

  return {
    values,
    types,
    isEmpty: totalFen <= 0,
    kpi: {
      totalFen,
      averageFen,
      peakLabel,
      peakBucket,
      peakAmountFen,
    },
  };
}

function buildSpec({ values, types }, t) {
  const isSingleType = types.length <= 1;

  const spec = {
    type: 'bar',
    data: [{ id: 'revenue', values }],
    xField: 'label',
    yField: 'amount',
    padding: { top: 16, right: 12, bottom: 12, left: 12 },
    barWidthRatio: 0.55,
    bar: {
      style: {
        cornerRadius: [4, 4, 0, 0],
      },
      state: {
        hover: { fillOpacity: 0.85 },
      },
    },
    axes: [
      {
        orient: 'left',
        grid: {
          visible: true,
          style: {
            lineDash: [4, 4],
            stroke: 'var(--semi-color-border, #e5e7eb)',
            lineWidth: 1,
          },
        },
        domainLine: { visible: false },
        tick: { visible: false },
        label: {
          formatMethod: (v) => formatYuan(v),
          style: { fill: 'var(--semi-color-text-2, #6b7280)', fontSize: 11 },
        },
      },
      {
        orient: 'bottom',
        domainLine: {
          style: { stroke: 'var(--semi-color-border, #e5e7eb)' },
        },
        tick: { visible: false },
        label: {
          autoHide: true,
          autoRotate: true,
          style: { fill: 'var(--semi-color-text-2, #6b7280)', fontSize: 11 },
        },
      },
    ],
    tooltip: {
      mark: {
        title: {
          value: (d) => d.bucket,
        },
        content: [
          {
            key: (d) => d.type,
            value: (d) => formatYuan(d.amount),
          },
        ],
      },
    },
    animationAppear: { duration: 500, easing: 'cubicOut' },
  };

  if (isSingleType) {
    spec.color = [TYPE_COLORS[types[0]] || '#3370FF'];
  } else {
    spec.seriesField = 'type';
    spec.stack = true;
    spec.color = {
      specified: Object.fromEntries(
        types.map((tp) => [t(ORDER_TYPE_LABELS[tp] || tp), TYPE_COLORS[tp] || '#888']),
      ),
    };
    spec.legends = [
      {
        visible: true,
        orient: 'top',
        position: 'start',
        padding: { bottom: 8 },
      },
    ];
  }

  return spec;
}

const KpiCard = ({ label, value, accent }) => (
  <div className='flex-1 rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-1)] px-4 py-3'>
    <div className='text-xs text-[var(--semi-color-text-2)]'>{label}</div>
    <div className='mt-1 text-lg font-semibold tabular-nums' style={{ color: accent }}>
      {value}
    </div>
  </div>
);

const OrderDailyRevenueChart = ({ orderType }) => {
  const { t } = useTranslation();
  const [period, setPeriod] = useState('day');
  const [anchorDate, setAnchorDate] = useState(() => new Date());
  const [loading, setLoading] = useState(false);
  const [stats, setStats] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = {
        period,
        date: formatDateParam(anchorDate),
        time_zone: getBrowserTimeZone(),
      };
      if (orderType) params.order_type = orderType;

      const res = await API.get('/api/order/stats/daily', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取统计数据失败'));
        setStats(null);
        return;
      }
      setStats(data || null);
    } catch (e) {
      showError(e?.message || t('获取统计数据失败'));
      setStats(null);
    } finally {
      setLoading(false);
    }
  }, [anchorDate, orderType, period, t]);

  useEffect(() => {
    load();
  }, [load]);

  const transformed = useMemo(() => transformData(stats, t), [stats, t]);
  const spec = useMemo(() => buildSpec(transformed, t), [transformed, t]);

  const accent =
    (orderType && TYPE_COLORS[orderType]) || 'var(--semi-color-primary, #3370FF)';

  return (
    <div className='mb-4 rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-1)] p-5 shadow-sm'>
      <div className='mb-4 flex flex-wrap items-start justify-between gap-3'>
        <div>
          <div className='text-sm font-semibold text-[var(--semi-color-text-0)]'>
            {t('收入统计')}
          </div>
          <div className='mt-0.5 text-xs text-[var(--semi-color-text-2)]'>
            {formatRangeLabel(stats)} {formatRangeLabel(stats) ? '· ' : ''}
            {t('已付款 · 不含余额支付')}
          </div>
          <div className='mt-0.5 text-xs text-[var(--semi-color-text-2)]'>
            {getPeriodHint(period, t)}
          </div>
        </div>

        <div className='flex flex-wrap items-center gap-2'>
          <Select
            size='small'
            value={period}
            onChange={setPeriod}
            optionList={PERIOD_OPTIONS.map((item) => ({
              value: item.value,
              label: t(item.label),
            }))}
            className='w-[112px]'
          />
          <DatePicker
            type='date'
            size='small'
            value={anchorDate}
            onChange={setAnchorDate}
            className='w-[140px]'
          />
        </div>
      </div>

      <div className='mb-4 grid grid-cols-2 gap-3 md:grid-cols-3'>
        <KpiCard
          label={t('累计收入')}
          value={formatYuan(transformed.kpi.totalFen / 100)}
          accent={accent}
        />
        <KpiCard
          label={t(PERIOD_AVERAGE_LABELS[period] || '均值')}
          value={formatYuan(transformed.kpi.averageFen / 100)}
          accent={accent}
        />
        <KpiCard
          label={t('峰值桶')}
          value={
            transformed.kpi.peakAmountFen > 0
              ? `${formatYuan(transformed.kpi.peakAmountFen / 100)} · ${transformed.kpi.peakLabel}`
              : '-'
          }
          accent={accent}
        />
      </div>

      <div className='relative' style={{ minHeight: 320 }}>
        {loading ? (
          <div className='flex h-80 items-center justify-center'>
            <Spin />
          </div>
        ) : transformed.isEmpty ? (
          <div className='flex h-80 items-center justify-center'>
            <Empty
              image={<IllustrationNoResult style={{ width: 120, height: 120 }} />}
              darkModeImage={<IllustrationNoResultDark style={{ width: 120, height: 120 }} />}
              title={t('暂无统计数据')}
              description={t('当前周期没有已付款订单')}
            />
          </div>
        ) : (
          <div style={{ height: 320 }}>
            <VChart spec={spec} option={{ mode: 'desktop-browser' }} />
          </div>
        )}
      </div>
    </div>
  );
};

export default OrderDailyRevenueChart;
