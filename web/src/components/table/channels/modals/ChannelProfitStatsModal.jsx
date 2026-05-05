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
  Button,
  DatePicker,
  Descriptions,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError } from '../../../../helpers';

const formatDay = (day) => {
  const s = String(day || '').padStart(8, '0');
  if (!/^\d{8}$/.test(s)) return '--';
  return `${s.slice(0, 4)}-${s.slice(4, 6)}-${s.slice(6)}`;
};

const formatCny = (value, digits = 2) => {
  const n = Number(value);
  if (!Number.isFinite(n)) return '--';
  const sign = n < 0 ? '-' : '';
  return `${sign}￥${Math.abs(n).toFixed(digits)}`;
};

const formatPercent = (value, digits = 2) => {
  const n = Number(value);
  if (!Number.isFinite(n)) return '--';
  return `${(n * 100).toFixed(digits)}%`;
};

const toYmd = (date) => {
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) return '';
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
};

const ChannelProfitStatsModal = ({ visible, onCancel, channel }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [summary, setSummary] = useState(null);

  const [startAt, setStartAt] = useState(null);
  const [endAt, setEndAt] = useState(null);
  const [granularity, setGranularity] = useState('day');
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const channelId = Number(channel?.id || 0);

  const pagedItems = useMemo(() => {
    const list = Array.isArray(items) ? items : [];
    const page = Number(currentPage) || 1;
    const size = Number(pageSize) || 20;
    const start = Math.max(0, (page - 1) * size);
    return list.slice(start, start + size);
  }, [items, currentPage, pageSize]);

  const columns = useMemo(
    () => [
      {
        title: t('日期'),
        dataIndex: 'day',
        render: (v, record) => {
          const start = formatDay(v);
          const endRaw = record?.bucket_end_day;
          const end = endRaw ? formatDay(endRaw) : '';
          const text =
            end && end !== '--' && start !== '--' && end !== start
              ? `${start} ~ ${end}`
              : start;
          return <span className='font-mono'>{text}</span>;
        },
      },
      {
        title: t('成功次数'),
        dataIndex: 'success_count',
        render: (v) => (
          <Tag color='cyan' shape='circle'>
            {Number(v || 0)}
          </Tag>
        ),
      },
      {
        title: t('销售额'),
        dataIndex: 'revenue_cny',
        render: (v) => formatCny(v),
      },
      {
        title: t('成本'),
        dataIndex: 'cost_cny',
        render: (v) => formatCny(v),
      },
      {
        title: t('利润'),
        dataIndex: 'profit_cny',
        render: (v) => {
          const n = Number(v);
          const color =
            Number.isFinite(n) && n !== 0 ? (n > 0 ? 'green' : 'red') : 'grey';
          return (
            <Tag color={color} shape='circle'>
              {formatCny(v)}
            </Tag>
          );
        },
      },
      {
        title: t('利润率'),
        dataIndex: 'profit_rate',
        render: (v) => formatPercent(v),
      },
    ],
    [t],
  );

  const load = async () => {
    if (!Number.isFinite(channelId) || channelId <= 0) return;

    const params = {};
    const startStr = toYmd(startAt);
    const endStr = toYmd(endAt);
    if (startStr) params.start = startStr;
    if (endStr) params.end = endStr;
    if (granularity) params.granularity = granularity;

    setLoading(true);
    try {
      const res = await API.get(`/api/channel/${channelId}/profit_stats/daily`, {
        params,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取统计失败'));
        return;
      }
      const nextItems = Array.isArray(data?.items) ? data.items : [];
      setItems(nextItems);
      setCurrentPage(1);
      setSummary({
        granularity: data?.granularity,
        billing_mode: data?.billing_mode,
        buy_requests_per_cny: data?.buy_requests_per_cny,
        sell_requests_per_cny: data?.sell_requests_per_cny,
        buy_cny_per_usd: data?.buy_cny_per_usd,
        credit_usd_per_cny: data?.credit_usd_per_cny,
        total_success_count: data?.total_success_count,
        total_used_quota: data?.total_used_quota,
        total_cost_used_quota: data?.total_cost_used_quota,
        total_revenue_cny: data?.total_revenue_cny,
        total_cost_cny: data?.total_cost_cny,
        total_profit_cny: data?.total_profit_cny,
        total_profit_rate: data?.total_profit_rate,
        start_day: data?.start_day,
        end_day: data?.end_day,
      });
    } catch (err) {
      showError(err?.message || t('获取统计失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) return;
    if (!Number.isFinite(channelId) || channelId <= 0) return;

    const end = new Date();
    end.setHours(0, 0, 0, 0);
    const start = new Date(end.getTime());
    start.setDate(start.getDate() - 29);

    setStartAt(start);
    setEndAt(end);
    setGranularity('day');
    setCurrentPage(1);
    setItems([]);
    setSummary(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, channelId]);

  useEffect(() => {
    if (!visible) return;
    if (!Number.isFinite(channelId) || channelId <= 0) return;
    if (!(startAt instanceof Date) || !(endAt instanceof Date)) return;
    if (Number.isNaN(startAt.getTime()) || Number.isNaN(endAt.getTime())) return;
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, channelId, startAt, endAt, granularity]);

  const descriptionData = useMemo(() => {
    if (!summary) return [];
    const start = summary.start_day ? formatDay(summary.start_day) : '--';
    const end = summary.end_day ? formatDay(summary.end_day) : '--';
    const mode = String(summary.billing_mode || '').trim();
    const granularityLabel =
      summary.granularity === 'week'
        ? t('按周')
        : summary.granularity === 'month'
          ? t('按月')
          : t('按天');
    return [
      { key: t('范围'), value: `${start} ~ ${end}` },
      { key: t('粒度'), value: granularityLabel },
      { key: t('计费模式'), value: mode || '--' },
      ...(mode === 'request'
        ? [
            {
              key: t('进价'),
              value:
                Number(summary.buy_requests_per_cny || 0) > 0
                  ? t('¥1 = {{n}} 次', { n: summary.buy_requests_per_cny })
                  : '--',
            },
            {
              key: t('售价'),
              value:
                Number(summary.sell_requests_per_cny || 0) > 0
                  ? t('¥1 = {{n}} 次', { n: summary.sell_requests_per_cny })
                  : '--',
            },
          ]
        : [
            {
              key: t('进价'),
              value:
                Number(summary.buy_cny_per_usd || 0) > 0
                  ? `￥${Number(summary.buy_cny_per_usd).toFixed(6).replace(/\.?0+$/, '')}/$`
                  : '--',
            },
            {
              key: t('销售兑换'),
              value:
                Number(summary.credit_usd_per_cny || 0) > 0
                  ? t('¥1 = {{n}} $', {
                      n: Number(summary.credit_usd_per_cny)
                        .toFixed(6)
                        .replace(/\.?0+$/, ''),
                    })
                  : '--',
            },
          ]),
      {
        key: t('合计成功'),
        value: Number(summary.total_success_count || 0),
      },
      { key: t('合计销售额'), value: formatCny(summary.total_revenue_cny) },
      { key: t('合计成本'), value: formatCny(summary.total_cost_cny) },
      { key: t('合计利润'), value: formatCny(summary.total_profit_cny) },
      { key: t('合计利润率'), value: formatPercent(summary.total_profit_rate) },
    ];
  }, [summary, t]);

  const title = useMemo(() => {
    const name = String(channel?.name || '').trim();
    if (name) return t('利润统计 - {{name}}', { name });
    return t('利润统计');
  }, [channel?.name, t]);

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={onCancel}
      maskClosable={false}
      footer={
        <Space>
          <Button onClick={onCancel}>{t('关闭')}</Button>
        </Space>
      }
      centered
      style={{ width: 920, maxWidth: '95vw' }}
    >
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <Select
            size='small'
            value={granularity}
            onChange={(v) => setGranularity(v)}
            optionList={[
              { label: t('按天'), value: 'day' },
              { label: t('按周'), value: 'week' },
              { label: t('按月'), value: 'month' },
            ]}
            className='w-[110px]'
          />
          <DatePicker
            type='date'
            size='small'
            value={startAt}
            onChange={(v) => setStartAt(v)}
            placeholder={t('起始日期')}
            className='w-[140px]'
          />
          <DatePicker
            type='date'
            size='small'
            value={endAt}
            onChange={(v) => setEndAt(v)}
            placeholder={t('结束日期')}
            className='w-[140px]'
          />
          <Button
            type='primary'
            theme='solid'
            size='small'
            onClick={load}
            loading={loading}
            className='!rounded-lg'
          >
            {t('刷新')}
          </Button>
        </div>

        {descriptionData.length > 0 ? (
          <Descriptions
            data={descriptionData}
            size='small'
            column={2}
            className='rounded-lg'
          />
        ) : (
          <Typography.Text type='tertiary'>
            {t('暂无统计信息')}
          </Typography.Text>
        )}

        <Table
          columns={columns}
          dataSource={pagedItems}
          loading={loading}
          rowKey={(r) =>
            `${String(r?.day || '')}-${String(r?.bucket_end_day || '')}`
          }
          pagination={{
            currentPage,
            pageSize,
            total: Array.isArray(items) ? items.length : 0,
            showSizeChanger: true,
            showQuickJumper: true,
            pageSizeOpts: [10, 20, 50, 100],
            onChange: (page, size) => {
              setCurrentPage(page);
              setPageSize(size);
            },
            onShowSizeChange: (page, size) => {
              setCurrentPage(1);
              setPageSize(size);
            },
          }}
          size='small'
        />
      </div>
    </Modal>
  );
};

export default ChannelProfitStatsModal;
