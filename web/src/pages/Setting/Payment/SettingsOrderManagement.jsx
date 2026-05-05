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
import {
  Button,
  Input,
  RadioGroup,
  Radio,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconCopy, IconRefresh, IconSearch } from '@douyinfe/semi-icons';
import {
  API,
  copy,
  renderCnyFen,
  renderQuotaToUSD,
  showError,
  showSuccess,
  timestamp2string,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import OrderDailyRevenueChart from '../../../components/order/OrderDailyRevenueChart';

const { Text } = Typography;

const formatTime = (ts) => {
  const n = Number(ts || 0);
  if (!Number.isFinite(n) || n <= 0) return '-';
  return timestamp2string(n);
};

const getTopupPayChannelLabel = (tradeNo) => {
  const no = String(tradeNo || '');
  if (no.startsWith('ref_')) return 'Stripe';
  if (no.startsWith('USR')) return '易支付';
  return '-';
};

const SettingsOrderManagement = () => {
  const { t } = useTranslation();

  const [activeTab, setActiveTab] = useState('subscription');

  // Stats panel visibility per tab
  const [showStatsSubscription, setShowStatsSubscription] = useState(false);
  const [showStatsTopup, setShowStatsTopup] = useState(false);
  const [showStatsPayg, setShowStatsPayg] = useState(false);
  const [showStatsPayRequest, setShowStatsPayRequest] = useState(false);
  const [showStatsPayToken, setShowStatsPayToken] = useState(false);

  // Subscription orders
  const [subscriptionLoading, setSubscriptionLoading] = useState(false);
  const [subscriptionOrders, setSubscriptionOrders] = useState([]);
  const [subscriptionTotal, setSubscriptionTotal] = useState(0);
  const [subscriptionPage, setSubscriptionPage] = useState(1);
  const [subscriptionPageSize, setSubscriptionPageSize] = useState(10);
  const [subscriptionKeyword, setSubscriptionKeyword] = useState('');
  const [subscriptionStatus, setSubscriptionStatus] = useState('');
  const [subscriptionPayMethod, setSubscriptionPayMethod] = useState('');
  const [subscriptionUserId, setSubscriptionUserId] = useState('');
  const [subscriptionLoaded, setSubscriptionLoaded] = useState(false);

  // Topup orders
  const [topupLoading, setTopupLoading] = useState(false);
  const [topupOrders, setTopupOrders] = useState([]);
  const [topupTotal, setTopupTotal] = useState(0);
  const [topupPage, setTopupPage] = useState(1);
  const [topupPageSize, setTopupPageSize] = useState(10);
  const [topupKeyword, setTopupKeyword] = useState('');
  const [topupStatus, setTopupStatus] = useState('');
  const [topupUserId, setTopupUserId] = useState('');
  const [topupLoaded, setTopupLoaded] = useState(false);

  // PayAsYouGo orders
  const [paygLoading, setPaygLoading] = useState(false);
  const [paygOrders, setPaygOrders] = useState([]);
  const [paygTotal, setPaygTotal] = useState(0);
  const [paygPage, setPaygPage] = useState(1);
  const [paygPageSize, setPaygPageSize] = useState(10);
  const [paygKeyword, setPaygKeyword] = useState('');
  const [paygStatus, setPaygStatus] = useState('');
  const [paygPayMethod, setPaygPayMethod] = useState('');
  const [paygUserId, setPaygUserId] = useState('');
  const [paygLoaded, setPaygLoaded] = useState(false);

  // Pay-per-request orders
  const [payRequestLoading, setPayRequestLoading] = useState(false);
  const [payRequestOrders, setPayRequestOrders] = useState([]);
  const [payRequestTotal, setPayRequestTotal] = useState(0);
  const [payRequestPage, setPayRequestPage] = useState(1);
  const [payRequestPageSize, setPayRequestPageSize] = useState(10);
  const [payRequestKeyword, setPayRequestKeyword] = useState('');
  const [payRequestStatus, setPayRequestStatus] = useState('');
  const [payRequestPayMethod, setPayRequestPayMethod] = useState('');
  const [payRequestUserId, setPayRequestUserId] = useState('');
  const [payRequestLoaded, setPayRequestLoaded] = useState(false);

  // Pay-per-token orders
  const [payTokenLoading, setPayTokenLoading] = useState(false);
  const [payTokenOrders, setPayTokenOrders] = useState([]);
  const [payTokenTotal, setPayTokenTotal] = useState(0);
  const [payTokenPage, setPayTokenPage] = useState(1);
  const [payTokenPageSize, setPayTokenPageSize] = useState(10);
  const [payTokenKeyword, setPayTokenKeyword] = useState('');
  const [payTokenStatus, setPayTokenStatus] = useState('');
  const [payTokenPayMethod, setPayTokenPayMethod] = useState('');
  const [payTokenUserId, setPayTokenUserId] = useState('');
  const [payTokenLoaded, setPayTokenLoaded] = useState(false);

  const copyText = async (text) => {
    const ok = await copy(String(text || ''));
    if (ok) {
      showSuccess(t('复制成功'));
    } else {
      showError(t('复制失败'));
    }
  };

  const renderViewToggle = (showStats, setShowStats) => (
    <RadioGroup
      type='button'
      buttonSize='middle'
      value={showStats ? 'stats' : 'detail'}
      onChange={(e) => setShowStats(e.target.value === 'stats')}
      className='shrink-0'
    >
      <Radio value='detail'>{t('明细')}</Radio>
      <Radio value='stats'>{t('统计')}</Radio>
    </RadioGroup>
  );

  const renderOrderFilters = ({ showStats, setShowStats, detailControls }) => (
    <div className='mb-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between'>
      <div className='flex items-center gap-3'>
        {renderViewToggle(showStats, setShowStats)}
        <div className='hidden md:block'>
          <Text strong>{t('订单管理')}</Text>
          <div className='text-xs text-gray-500'>
            {showStats
              ? t('展示按自然日/周/月/年统计的收入（已付款且不含余额支付）')
              : t('可按订单号/用户/状态查询（只读）')}
          </div>
        </div>
      </div>

      {!showStats && (
        <div className='flex w-full flex-col gap-2 sm:flex-row md:w-auto md:flex-wrap md:justify-end'>
          {detailControls}
        </div>
      )}
    </div>
  );

  const loadSubscriptionOrders = async (page = 1, pageSize = subscriptionPageSize) => {
    setSubscriptionLoading(true);
    try {
      const params = {
        p: page,
        page_size: pageSize,
        keyword: subscriptionKeyword?.trim() || '',
        status: subscriptionStatus || '',
        pay_method: subscriptionPayMethod || '',
      };
      const uid = String(subscriptionUserId || '').trim();
      if (uid) params.user_id = uid;

      const res = await API.get('/api/order/subscriptions', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取订阅订单失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      setSubscriptionOrders(items);
      setSubscriptionTotal(Number(data?.total || 0));
      setSubscriptionPage(page);
      setSubscriptionPageSize(pageSize);
      setSubscriptionLoaded(true);
    } catch (e) {
      showError(e?.message || t('获取订阅订单失败'));
    } finally {
      setSubscriptionLoading(false);
    }
  };

  const loadTopupOrders = async (page = 1, pageSize = topupPageSize) => {
    setTopupLoading(true);
    try {
      const params = {
        p: page,
        page_size: pageSize,
        keyword: topupKeyword?.trim() || '',
        status: topupStatus || '',
      };
      const uid = String(topupUserId || '').trim();
      if (uid) params.user_id = uid;

      const res = await API.get('/api/order/topups', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取充值订单失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      setTopupOrders(items);
      setTopupTotal(Number(data?.total || 0));
      setTopupPage(page);
      setTopupPageSize(pageSize);
      setTopupLoaded(true);
    } catch (e) {
      showError(e?.message || t('获取充值订单失败'));
    } finally {
      setTopupLoading(false);
    }
  };

  const loadPaygOrders = async (page = 1, pageSize = paygPageSize) => {
    setPaygLoading(true);
    try {
      const params = {
        p: page,
        page_size: pageSize,
        keyword: paygKeyword?.trim() || '',
        status: paygStatus || '',
        pay_method: paygPayMethod || '',
      };
      const uid = String(paygUserId || '').trim();
      if (uid) params.user_id = uid;

      const res = await API.get('/api/order/paygs', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取按量订单失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      setPaygOrders(items);
      setPaygTotal(Number(data?.total || 0));
      setPaygPage(page);
      setPaygPageSize(pageSize);
      setPaygLoaded(true);
    } catch (e) {
      showError(e?.message || t('获取按量订单失败'));
    } finally {
      setPaygLoading(false);
    }
  };

  const loadPayRequestOrders = async (page = 1, pageSize = payRequestPageSize) => {
    setPayRequestLoading(true);
    try {
      const params = {
        p: page,
        page_size: pageSize,
        keyword: payRequestKeyword?.trim() || '',
        status: payRequestStatus || '',
        pay_method: payRequestPayMethod || '',
      };
      const uid = String(payRequestUserId || '').trim();
      if (uid) params.user_id = uid;

      const res = await API.get('/api/order/pay_requests', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取按次订单失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      setPayRequestOrders(items);
      setPayRequestTotal(Number(data?.total || 0));
      setPayRequestPage(page);
      setPayRequestPageSize(pageSize);
      setPayRequestLoaded(true);
    } catch (e) {
      showError(e?.message || t('获取按次订单失败'));
    } finally {
      setPayRequestLoading(false);
    }
  };

  const loadPayTokenOrders = async (page = 1, pageSize = payTokenPageSize) => {
    setPayTokenLoading(true);
    try {
      const params = {
        p: page,
        page_size: pageSize,
        keyword: payTokenKeyword?.trim() || '',
        status: payTokenStatus || '',
        pay_method: payTokenPayMethod || '',
      };
      const uid = String(payTokenUserId || '').trim();
      if (uid) params.user_id = uid;

      const res = await API.get('/api/order/pay_tokens', { params });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取按token订单失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      setPayTokenOrders(items);
      setPayTokenTotal(Number(data?.total || 0));
      setPayTokenPage(page);
      setPayTokenPageSize(pageSize);
      setPayTokenLoaded(true);
    } catch (e) {
      showError(e?.message || t('获取按token订单失败'));
    } finally {
      setPayTokenLoading(false);
    }
  };

  useEffect(() => {
    loadSubscriptionOrders(1, subscriptionPageSize);
  }, []);

  useEffect(() => {
    if (activeTab === 'topup' && !topupLoaded) {
      loadTopupOrders(1, topupPageSize);
    }
    if (activeTab === 'payg' && !paygLoaded) {
      loadPaygOrders(1, paygPageSize);
    }
    if (activeTab === 'pay_request' && !payRequestLoaded) {
      loadPayRequestOrders(1, payRequestPageSize);
    }
    if (activeTab === 'pay_token' && !payTokenLoaded) {
      loadPayTokenOrders(1, payTokenPageSize);
    }
  }, [activeTab, topupLoaded, paygLoaded, payRequestLoaded, payTokenLoaded]);

  const subscriptionColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        width: 320,
        render: (v) => (
          <div className='flex items-center gap-2'>
            <Text className='font-mono'>{v || '-'}</Text>
            {v && (
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconCopy />}
                onClick={(e) => {
                  e.stopPropagation();
                  copyText(v);
                }}
              />
            )}
          </div>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'user_id',
        width: 220,
        render: (_, r) => (
          <div className='space-y-0.5'>
            <div className='text-sm'>
              {r.username || '-'} <span className='text-xs text-gray-500'>#{r.user_id}</span>
            </div>
            <div className='text-xs text-gray-500'>{r.email || '-'}</div>
          </div>
        ),
      },
      {
        title: t('商品'),
        dataIndex: 'plan_name',
        width: 200,
        render: (v) => v || '-',
      },
      {
        title: t('金额'),
        dataIndex: 'amount_fen',
        width: 120,
        render: (v) => renderCnyFen(v),
      },
      {
        title: t('支付方式'),
        dataIndex: 'pay_method',
        width: 120,
        render: (v) => {
          if (v === 'epay') return <Tag color='blue'>{t('易支付')}</Tag>;
          if (v === 'balance') return <Tag color='grey'>{t('余额')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 120,
        render: (v) => {
          if (v === 'success') return <Tag color='green'>{t('已支付')}</Tag>;
          if (v === 'pending') return <Tag color='yellow'>{t('待支付')}</Tag>;
          if (v === 'failed') return <Tag color='red'>{t('失败')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('支付时间'),
        dataIndex: 'paid_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('订阅到期'),
        dataIndex: 'membership_expire_at',
        width: 180,
        render: (v) => formatTime(v),
      },
    ],
    [t],
  );

  const paygColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        width: 320,
        render: (v) => (
          <div className='flex items-center gap-2'>
            <Text className='font-mono'>{v || '-'}</Text>
            {v && (
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconCopy />}
                onClick={(e) => {
                  e.stopPropagation();
                  copyText(v);
                }}
              />
            )}
          </div>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'user_id',
        width: 220,
        render: (_, r) => (
          <div className='space-y-0.5'>
            <div className='text-sm'>
              {r.username || '-'} <span className='text-xs text-gray-500'>#{r.user_id}</span>
            </div>
            <div className='text-xs text-gray-500'>{r.email || '-'}</div>
          </div>
        ),
      },
      {
        title: t('支付方式'),
        dataIndex: 'pay_method',
        width: 200,
        render: (v, r) => {
          if (v === 'epay') {
            return (
              <div className='flex items-center gap-2'>
                <Tag color='blue'>{t('易支付')}</Tag>
                {r?.epay_method ? <Tag color='white'>{r.epay_method}</Tag> : null}
              </div>
            );
          }
          if (v === 'balance') return <Tag color='grey'>{t('余额')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('支付金额'),
        dataIndex: 'amount_fen',
        width: 140,
        render: (v) => renderCnyFen(v),
      },
      {
        title: t('获得额度'),
        dataIndex: 'credit_quota',
        width: 220,
        render: (v) => {
          const q = Number(v ?? 0);
          if (!Number.isFinite(q) || q <= 0) return '-';
          return (
            <div className='space-y-0.5'>
              <div>{String(q)}</div>
              <div className='text-xs text-gray-500'>
                {t('折合约')} {renderQuotaToUSD(q, 2)}
              </div>
            </div>
          );
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 120,
        render: (v) => {
          if (v === 'success') return <Tag color='green'>{t('已支付')}</Tag>;
          if (v === 'pending') return <Tag color='yellow'>{t('待支付')}</Tag>;
          if (v === 'failed') return <Tag color='red'>{t('失败')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('支付时间'),
        dataIndex: 'paid_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('完成时间'),
        dataIndex: 'finished_at',
        width: 180,
        render: (v) => formatTime(v),
      },
    ],
    [t],
  );

  const payRequestColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        width: 320,
        render: (v) => (
          <div className='flex items-center gap-2'>
            <Text className='font-mono'>{v || '-'}</Text>
            {v && (
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconCopy />}
                onClick={(e) => {
                  e.stopPropagation();
                  copyText(v);
                }}
              />
            )}
          </div>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'user_id',
        width: 220,
        render: (_, r) => (
          <div className='space-y-0.5'>
            <div className='text-sm'>
              {r.username || '-'} <span className='text-xs text-gray-500'>#{r.user_id}</span>
            </div>
            <div className='text-xs text-gray-500'>{r.email || '-'}</div>
          </div>
        ),
      },
      {
        title: t('支付方式'),
        dataIndex: 'pay_method',
        width: 200,
        render: (v, r) => {
          if (v === 'epay') {
            return (
              <div className='flex items-center gap-2'>
                <Tag color='blue'>{t('易支付')}</Tag>
                {r?.epay_method ? <Tag color='white'>{r.epay_method}</Tag> : null}
              </div>
            );
          }
          if (v === 'balance') return <Tag color='grey'>{t('余额')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('支付金额'),
        dataIndex: 'amount_fen',
        width: 140,
        render: (v) => renderCnyFen(v),
      },
      {
        title: t('获得次数'),
        dataIndex: 'credit_requests',
        width: 140,
        render: (v) => {
          const n = Number(v ?? 0);
          if (!Number.isFinite(n) || n <= 0) return '-';
          return `${String(n)} ${t('次')}`;
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 120,
        render: (v) => {
          if (v === 'success') return <Tag color='green'>{t('已支付')}</Tag>;
          if (v === 'pending') return <Tag color='yellow'>{t('待支付')}</Tag>;
          if (v === 'failed') return <Tag color='red'>{t('失败')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('支付时间'),
        dataIndex: 'paid_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('完成时间'),
        dataIndex: 'finished_at',
        width: 180,
        render: (v) => formatTime(v),
      },
    ],
    [t],
  );

  const payTokenColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        width: 320,
        render: (v) => (
          <div className='flex items-center gap-2'>
            <Text className='font-mono'>{v || '-'}</Text>
            {v && (
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconCopy />}
                onClick={(e) => {
                  e.stopPropagation();
                  copyText(v);
                }}
              />
            )}
          </div>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'user_id',
        width: 220,
        render: (_, r) => (
          <div className='space-y-0.5'>
            <div className='text-sm'>
              {r.username || '-'} <span className='text-xs text-gray-500'>#{r.user_id}</span>
            </div>
            <div className='text-xs text-gray-500'>{r.email || '-'}</div>
          </div>
        ),
      },
      {
        title: t('支付方式'),
        dataIndex: 'pay_method',
        width: 200,
        render: (v, r) => {
          if (v === 'epay') {
            return (
              <div className='flex items-center gap-2'>
                <Tag color='blue'>{t('易支付')}</Tag>
                {r?.epay_method ? <Tag color='white'>{r.epay_method}</Tag> : null}
              </div>
            );
          }
          if (v === 'balance') return <Tag color='grey'>{t('余额')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('支付金额'),
        dataIndex: 'amount_fen',
        width: 140,
        render: (v) => renderCnyFen(v),
      },
      {
        title: t('获得Tokens'),
        dataIndex: 'credit_tokens',
        width: 160,
        render: (v) => {
          const n = Number(v ?? 0);
          if (!Number.isFinite(n) || n <= 0) return '-';
          return `${String(n)} tokens`;
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 120,
        render: (v) => {
          if (v === 'success') return <Tag color='green'>{t('已支付')}</Tag>;
          if (v === 'pending') return <Tag color='yellow'>{t('待支付')}</Tag>;
          if (v === 'failed') return <Tag color='red'>{t('失败')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('支付时间'),
        dataIndex: 'paid_at',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('完成时间'),
        dataIndex: 'finished_at',
        width: 180,
        render: (v) => formatTime(v),
      },
    ],
    [t],
  );

  const topupColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        width: 320,
        render: (v) => (
          <div className='flex items-center gap-2'>
            <Text className='font-mono'>{v || '-'}</Text>
            {v && (
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={<IconCopy />}
                onClick={(e) => {
                  e.stopPropagation();
                  copyText(v);
                }}
              />
            )}
          </div>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'user_id',
        width: 220,
        render: (_, r) => (
          <div className='space-y-0.5'>
            <div className='text-sm'>
              {r.username || '-'} <span className='text-xs text-gray-500'>#{r.user_id}</span>
            </div>
            <div className='text-xs text-gray-500'>{r.email || '-'}</div>
          </div>
        ),
      },
      {
        title: t('通道'),
        dataIndex: 'trade_no',
        width: 120,
        render: (v) => <Tag color='white'>{getTopupPayChannelLabel(v)}</Tag>,
      },
      {
        title: t('充值数量'),
        dataIndex: 'amount',
        width: 120,
        render: (v) => (v === undefined || v === null ? '-' : String(v)),
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        width: 140,
        render: (v) => {
          const n = Number(v || 0);
          if (!Number.isFinite(n)) return '￥0.00';
          return `￥${n.toFixed(2)}`;
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 120,
        render: (v) => {
          if (v === 'success') return <Tag color='green'>{t('已支付')}</Tag>;
          if (v === 'pending') return <Tag color='yellow'>{t('待支付')}</Tag>;
          if (v === 'expired') return <Tag color='grey'>{t('已过期')}</Tag>;
          return <Tag color='grey'>{v || '-'}</Tag>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'create_time',
        width: 180,
        render: (v) => formatTime(v),
      },
      {
        title: t('完成时间'),
        dataIndex: 'complete_time',
        width: 180,
        render: (v) => formatTime(v),
      },
    ],
    [t],
  );

  const paygFilters = renderOrderFilters({
    showStats: showStatsPayg,
    setShowStats: setShowStatsPayg,
    detailControls: (
      <>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 用户名 / 邮箱 / 支付方式')}
          value={paygKeyword}
          onChange={setPaygKeyword}
          className='w-full sm:w-72'
          showClear
        />
        <Input
          placeholder={t('用户ID（可选）')}
          value={paygUserId}
          onChange={setPaygUserId}
          className='w-full sm:w-40'
          showClear
        />
        <Select
          placeholder={t('状态')}
          value={paygStatus}
          onChange={setPaygStatus}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPaygStatus('')}
        >
          <Select.Option value='pending'>{t('待支付')}</Select.Option>
          <Select.Option value='success'>{t('已支付')}</Select.Option>
          <Select.Option value='failed'>{t('失败')}</Select.Option>
        </Select>
        <Select
          placeholder={t('支付方式')}
          value={paygPayMethod}
          onChange={setPaygPayMethod}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPaygPayMethod('')}
        >
          <Select.Option value='epay'>{t('易支付')}</Select.Option>
          <Select.Option value='balance'>{t('余额')}</Select.Option>
        </Select>
        <Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconSearch />}
            onClick={() => loadPaygOrders(1, paygPageSize)}
            loading={paygLoading}
          >
            {t('查询')}
          </Button>
          <Button
            theme='light'
            icon={<IconRefresh />}
            onClick={() => loadPaygOrders(paygPage, paygPageSize)}
            loading={paygLoading}
          >
            {t('刷新')}
          </Button>
        </Space>
      </>
    ),
  });

  const payRequestFilters = renderOrderFilters({
    showStats: showStatsPayRequest,
    setShowStats: setShowStatsPayRequest,
    detailControls: (
      <>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 用户名 / 邮箱 / 支付方式')}
          value={payRequestKeyword}
          onChange={setPayRequestKeyword}
          className='w-full sm:w-72'
          showClear
        />
        <Input
          placeholder={t('用户ID（可选）')}
          value={payRequestUserId}
          onChange={setPayRequestUserId}
          className='w-full sm:w-40'
          showClear
        />
        <Select
          placeholder={t('状态')}
          value={payRequestStatus}
          onChange={setPayRequestStatus}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPayRequestStatus('')}
        >
          <Select.Option value='pending'>{t('待支付')}</Select.Option>
          <Select.Option value='success'>{t('已支付')}</Select.Option>
          <Select.Option value='failed'>{t('失败')}</Select.Option>
        </Select>
        <Select
          placeholder={t('支付方式')}
          value={payRequestPayMethod}
          onChange={setPayRequestPayMethod}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPayRequestPayMethod('')}
        >
          <Select.Option value='epay'>{t('易支付')}</Select.Option>
          <Select.Option value='balance'>{t('余额')}</Select.Option>
        </Select>
        <Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconSearch />}
            onClick={() => loadPayRequestOrders(1, payRequestPageSize)}
            loading={payRequestLoading}
          >
            {t('查询')}
          </Button>
          <Button
            theme='light'
            icon={<IconRefresh />}
            onClick={() => loadPayRequestOrders(payRequestPage, payRequestPageSize)}
            loading={payRequestLoading}
          >
            {t('刷新')}
          </Button>
        </Space>
      </>
    ),
  });

  const payTokenFilters = renderOrderFilters({
    showStats: showStatsPayToken,
    setShowStats: setShowStatsPayToken,
    detailControls: (
      <>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 用户名 / 邮箱 / 支付方式')}
          value={payTokenKeyword}
          onChange={setPayTokenKeyword}
          className='w-full sm:w-72'
          showClear
        />
        <Input
          placeholder={t('用户ID（可选）')}
          value={payTokenUserId}
          onChange={setPayTokenUserId}
          className='w-full sm:w-40'
          showClear
        />
        <Select
          placeholder={t('状态')}
          value={payTokenStatus}
          onChange={setPayTokenStatus}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPayTokenStatus('')}
        >
          <Select.Option value='pending'>{t('待支付')}</Select.Option>
          <Select.Option value='success'>{t('已支付')}</Select.Option>
          <Select.Option value='failed'>{t('失败')}</Select.Option>
        </Select>
        <Select
          placeholder={t('支付方式')}
          value={payTokenPayMethod}
          onChange={setPayTokenPayMethod}
          className='w-full sm:w-36'
          showClear
          onClear={() => setPayTokenPayMethod('')}
        >
          <Select.Option value='epay'>{t('易支付')}</Select.Option>
          <Select.Option value='balance'>{t('余额')}</Select.Option>
        </Select>
        <Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconSearch />}
            onClick={() => loadPayTokenOrders(1, payTokenPageSize)}
            loading={payTokenLoading}
          >
            {t('查询')}
          </Button>
          <Button
            theme='light'
            icon={<IconRefresh />}
            onClick={() => loadPayTokenOrders(payTokenPage, payTokenPageSize)}
            loading={payTokenLoading}
          >
            {t('刷新')}
          </Button>
        </Space>
      </>
    ),
  });

  const subscriptionFilters = renderOrderFilters({
    showStats: showStatsSubscription,
    setShowStats: setShowStatsSubscription,
    detailControls: (
      <>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 用户名 / 邮箱 / 商品名')}
          value={subscriptionKeyword}
          onChange={setSubscriptionKeyword}
          className='w-full sm:w-72'
          showClear
        />
        <Input
          placeholder={t('用户ID（可选）')}
          value={subscriptionUserId}
          onChange={setSubscriptionUserId}
          className='w-full sm:w-40'
          showClear
        />
        <Select
          placeholder={t('状态')}
          value={subscriptionStatus}
          onChange={setSubscriptionStatus}
          className='w-full sm:w-36'
          showClear
          onClear={() => setSubscriptionStatus('')}
        >
          <Select.Option value='pending'>{t('待支付')}</Select.Option>
          <Select.Option value='success'>{t('已支付')}</Select.Option>
          <Select.Option value='failed'>{t('失败')}</Select.Option>
        </Select>
        <Select
          placeholder={t('支付方式')}
          value={subscriptionPayMethod}
          onChange={setSubscriptionPayMethod}
          className='w-full sm:w-36'
          showClear
          onClear={() => setSubscriptionPayMethod('')}
        >
          <Select.Option value='epay'>{t('易支付')}</Select.Option>
          <Select.Option value='balance'>{t('余额')}</Select.Option>
        </Select>
        <Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconSearch />}
            onClick={() => loadSubscriptionOrders(1, subscriptionPageSize)}
            loading={subscriptionLoading}
          >
            {t('查询')}
          </Button>
          <Button
            theme='light'
            icon={<IconRefresh />}
            onClick={() => loadSubscriptionOrders(subscriptionPage, subscriptionPageSize)}
            loading={subscriptionLoading}
          >
            {t('刷新')}
          </Button>
        </Space>
      </>
    ),
  });

  const topupFilters = renderOrderFilters({
    showStats: showStatsTopup,
    setShowStats: setShowStatsTopup,
    detailControls: (
      <>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 用户名 / 邮箱')}
          value={topupKeyword}
          onChange={setTopupKeyword}
          className='w-full sm:w-72'
          showClear
        />
        <Input
          placeholder={t('用户ID（可选）')}
          value={topupUserId}
          onChange={setTopupUserId}
          className='w-full sm:w-40'
          showClear
        />
        <Select
          placeholder={t('状态')}
          value={topupStatus}
          onChange={setTopupStatus}
          className='w-full sm:w-36'
          showClear
          onClear={() => setTopupStatus('')}
        >
          <Select.Option value='pending'>{t('待支付')}</Select.Option>
          <Select.Option value='success'>{t('已支付')}</Select.Option>
          <Select.Option value='expired'>{t('已过期')}</Select.Option>
        </Select>
        <Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconSearch />}
            onClick={() => loadTopupOrders(1, topupPageSize)}
            loading={topupLoading}
          >
            {t('查询')}
          </Button>
          <Button
            theme='light'
            icon={<IconRefresh />}
            onClick={() => loadTopupOrders(topupPage, topupPageSize)}
            loading={topupLoading}
          >
            {t('刷新')}
          </Button>
        </Space>
      </>
    ),
  });

  return (
    <div>
      <Tabs
        type='line'
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key)}
      >
        <Tabs.TabPane itemKey='subscription' tab={t('订阅订单')}>
          {subscriptionFilters}
          {showStatsSubscription ? (
            <OrderDailyRevenueChart orderType='subscription' />
          ) : (
            <Table
              columns={subscriptionColumns}
              dataSource={subscriptionOrders}
              rowKey='id'
              loading={subscriptionLoading}
              scroll={{ x: 'max-content' }}
              pagination={{
                currentPage: subscriptionPage,
                pageSize: subscriptionPageSize,
                total: subscriptionTotal,
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50', '100'],
                onChange: (page, size) => { loadSubscriptionOrders(page, size); },
                onShowSizeChange: (current, size) => { loadSubscriptionOrders(1, size); },
              }}
              size='middle'
              className='overflow-hidden'
            />
          )}
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='topup' tab={t('充值订单')}>
          {topupFilters}
          {showStatsTopup ? (
            <OrderDailyRevenueChart orderType='topup' />
          ) : (
            <Table
              columns={topupColumns}
              dataSource={topupOrders}
              rowKey='id'
              loading={topupLoading}
              scroll={{ x: 'max-content' }}
              pagination={{
                currentPage: topupPage,
                pageSize: topupPageSize,
                total: topupTotal,
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50', '100'],
                onChange: (page, size) => { loadTopupOrders(page, size); },
                onShowSizeChange: (current, size) => { loadTopupOrders(1, size); },
              }}
              size='middle'
              className='overflow-hidden'
            />
          )}
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='payg' tab={t('按量订单')}>
          {paygFilters}
          {showStatsPayg ? (
            <OrderDailyRevenueChart orderType='payg' />
          ) : (
            <Table
              columns={paygColumns}
              dataSource={paygOrders}
              rowKey='id'
              loading={paygLoading}
              scroll={{ x: 'max-content' }}
              pagination={{
                currentPage: paygPage,
                pageSize: paygPageSize,
                total: paygTotal,
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50', '100'],
                onChange: (page, size) => { loadPaygOrders(page, size); },
                onShowSizeChange: (current, size) => { loadPaygOrders(1, size); },
              }}
              size='middle'
              className='overflow-hidden'
            />
          )}
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='pay_request' tab={t('按次订单')}>
          {payRequestFilters}
          {showStatsPayRequest ? (
            <OrderDailyRevenueChart orderType='pay_request' />
          ) : (
            <Table
              columns={payRequestColumns}
              dataSource={payRequestOrders}
              rowKey='id'
              loading={payRequestLoading}
              scroll={{ x: 'max-content' }}
              pagination={{
                currentPage: payRequestPage,
                pageSize: payRequestPageSize,
                total: payRequestTotal,
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50', '100'],
                onChange: (page, size) => { loadPayRequestOrders(page, size); },
                onShowSizeChange: (current, size) => { loadPayRequestOrders(1, size); },
              }}
              size='middle'
              className='overflow-hidden'
            />
          )}
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='pay_token' tab={t('按token订单')}>
          {payTokenFilters}
          {showStatsPayToken ? (
            <OrderDailyRevenueChart orderType='pay_token' />
          ) : (
            <Table
              columns={payTokenColumns}
              dataSource={payTokenOrders}
              rowKey='id'
              loading={payTokenLoading}
              scroll={{ x: 'max-content' }}
              pagination={{
                currentPage: payTokenPage,
                pageSize: payTokenPageSize,
                total: payTokenTotal,
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50', '100'],
                onChange: (page, size) => { loadPayTokenOrders(page, size); },
                onShowSizeChange: (current, size) => { loadPayTokenOrders(1, size); },
              }}
              size='middle'
              className='overflow-hidden'
            />
          )}
        </Tabs.TabPane>
      </Tabs>
    </div>
  );
};

export default SettingsOrderManagement;
