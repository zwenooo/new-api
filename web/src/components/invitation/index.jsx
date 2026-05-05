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

import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BarChart2, Coins, Copy, Gift, Users, Wallet } from 'lucide-react';
import {
  API,
  copy,
  isEffectiveAdmin,
  renderCnyFen,
  showError,
  showSuccess,
  timestamp2string,
  yuanToFen,
} from '../../helpers';
import { StatusContext } from '../../context/Status';
import { UserContext } from '../../context/User';
import ConsolePage from '../layout/ConsolePage';
import CardPro from '../common/ui/CardPro';
import CardTable from '../common/ui/CardTable';
import TransferModal from '../topup/modals/TransferModal';
import Forbidden from '../../pages/Forbidden';

const Invitation = () => {
  const { t } = useTranslation();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const isAdminUser = isEffectiveAdmin();

  const personalSetting = statusState?.status?.personal_setting;
  const invitationEnabled =
    isAdminUser || personalSetting?.invitation_page_visible !== false;

  const commissionFirstPercent = Number(
    statusState?.status?.subscription_invite_commission_first_percent || 0,
  );
  const commissionRepeatPercent = Number(
    statusState?.status?.subscription_invite_commission_repeat_percent || 0,
  );

  const affFetchedRef = useRef(false);
  const [affLink, setAffLink] = useState('');

  const [openTransfer, setOpenTransfer] = useState(false);
  const [transferAmountYuan, setTransferAmountYuan] = useState(0.01);
  const [paidUserCount, setPaidUserCount] = useState(0);
  const [recordsLoading, setRecordsLoading] = useState(false);
  const [records, setRecords] = useState([]);
  const [recordsTotal, setRecordsTotal] = useState(0);
  const [recordsPage, setRecordsPage] = useState(1);
  const recordsPageSize = 20;

  const [balanceRecordsLoading, setBalanceRecordsLoading] = useState(false);
  const [balanceRecords, setBalanceRecords] = useState([]);
  const [balanceRecordsTotal, setBalanceRecordsTotal] = useState(0);
  const [balanceRecordsPage, setBalanceRecordsPage] = useState(1);
  const balanceRecordsPageSize = 20;

  useEffect(() => {
    document.documentElement.classList.add('scrollbar-hide');
    document.body.classList.add('scrollbar-hide');
    return () => {
      document.documentElement.classList.remove('scrollbar-hide');
      document.body.classList.remove('scrollbar-hide');
    };
  }, []);

  const refreshUser = async () => {
    try {
      const res = await API.get('/api/user/self');
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
      } else {
        showError(message || t('刷新失败'));
      }
    } catch (e) {
      showError(e?.message || t('刷新失败'));
    }
  };

  const getAffLink = async () => {
    const res = await API.get('/api/user/aff');
    const { success, message, data } = res.data;
    if (success) {
      setAffLink(`${window.location.origin}/register?aff=${data}`);
    } else {
      showError(message);
    }
  };

  const loadBalanceRecords = async (page = 1, append = false) => {
    setBalanceRecordsLoading(true);
    try {
      const res = await API.get('/api/user/balance/records', {
        params: { p: page, page_size: balanceRecordsPageSize },
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取失败'));
        return;
      }

      const items = data?.items;
      if (!Array.isArray(items)) {
        showError(t('返回数据异常'));
        return;
      }

      setBalanceRecordsTotal(Number(data?.total || 0));
      setBalanceRecordsPage(page);
      if (append) {
        setBalanceRecords((prev) => [...prev, ...items]);
      } else {
        setBalanceRecords(items);
      }
    } catch (e) {
      showError(e?.message || t('获取失败'));
    } finally {
      setBalanceRecordsLoading(false);
    }
  };

  const loadRecords = async (page = 1, append = false) => {
    setRecordsLoading(true);
    try {
      const res = await API.get('/api/user/aff/records', {
        params: { p: page, page_size: recordsPageSize },
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取失败'));
        return;
      }

      const items = data?.items;
      if (!Array.isArray(items)) {
        showError(t('返回数据异常'));
        return;
      }

      setPaidUserCount(Number(data?.paid_user_count || 0));
      setRecordsTotal(Number(data?.total || 0));
      setRecordsPage(page);
      if (append) {
        setRecords((prev) => [...prev, ...items]);
      } else {
        setRecords(items);
      }
    } catch (e) {
      showError(e?.message || t('获取失败'));
    } finally {
      setRecordsLoading(false);
    }
  };

  const handleAffLinkClick = async () => {
    if (!affLink) return;
    await copy(affLink);
    showSuccess(t('邀请链接已复制到剪切板'));
  };

  const transfer = async () => {
    let amountFen = 0;
    try {
      amountFen = yuanToFen(transferAmountYuan);
    } catch (e) {
      showError(e?.message || t('金额格式错误'));
      return;
    }
    if (amountFen <= 0) {
      showError(t('转入金额必须大于0'));
      return;
    }
    const maxFen = userState?.user?.aff_quota || 0;
    if (amountFen > maxFen) {
      showError(t('转入金额不能大于返利余额'));
      return;
    }
    const res = await API.post('/api/user/aff_transfer', {
      amount_fen: amountFen,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess(message);
      setOpenTransfer(false);
      await refreshUser();
      loadBalanceRecords(1, false);
    } else {
      showError(message);
    }
  };

  useEffect(() => {
    setTransferAmountYuan(0.01);
    refreshUser();
    loadRecords(1, false);
    loadBalanceRecords(1, false);
  }, []);

  useEffect(() => {
    if (affFetchedRef.current) return;
    affFetchedRef.current = true;
    getAffLink().then();
  }, []);

  const stats = useMemo(
    () => [
      {
        key: 'invite_count',
        label: t('累计邀请'),
        value: userState?.user?.aff_count || 0,
        helper: t('成功注册的总人数'),
        icon: Users,
      },
      {
        key: 'paid_count',
        label: t('产生付费'),
        value: paidUserCount,
        helper: t('已转化为付费用户'),
        icon: BarChart2,
      },
      {
        key: 'history_quota',
        label: t('累计返利'),
        value: renderCnyFen(userState?.user?.aff_history_quota || 0),
        helper: t('含已结算返利总额'),
        icon: Coins,
      },
      {
        key: 'rebate_quota',
        label: t('返利余额'),
        value: renderCnyFen(userState?.user?.aff_quota || 0),
        helper: t('可转入账户余额'),
        icon: Gift,
      },
    ],
    [
      paidUserCount,
      t,
      userState?.user?.aff_count,
      userState?.user?.aff_quota,
      userState?.user?.aff_history_quota,
    ],
  );

  if (!invitationEnabled) {
    return <Forbidden />;
  }

  const availableRebateFen = userState?.user?.aff_quota || 0;
  const accountBalanceFen = userState?.user?.balance_fen || 0;

  const renderSignedCnyFen = (fen) => {
    const fenNumber =
      typeof fen === 'string' ? parseInt(fen, 10) : Number(fen || 0);
    if (!Number.isFinite(fenNumber) || fenNumber === 0) return renderCnyFen(0);
    if (fenNumber > 0) return `+${renderCnyFen(fenNumber)}`;
    return renderCnyFen(fenNumber);
  };

  const renderBalanceRecordType = (type) => {
    if (type === 'aff_transfer_in') return t('返利转入');
    if (type === 'subscription_pay_out') return t('订阅消费');
    return type || '-';
  };

  const rebateColumns = [
    {
      title: t('邮箱'),
      dataIndex: 'email',
      key: 'email',
      render: (value, record) => (
        <div className='min-w-[180px]'>
          <div className='font-medium text-semi-color-text-0'>
            {value || '-'}
          </div>
          <div className='mt-1 text-xs text-semi-color-text-2'>
            ID: {record.user_id || '-'}
          </div>
        </div>
      ),
    },
    {
      title: t('购买套餐'),
      dataIndex: 'plan_name',
      key: 'plan_name',
      render: (value) => (
        <div className='max-w-[200px] break-words text-semi-color-text-1'>
          {value || '-'}
        </div>
      ),
    },
    {
      title: <span className='block w-full text-right'>{t('购买金额')}</span>,
      dataIndex: 'amount_fen',
      key: 'amount_fen',
      align: 'right',
      render: (value) => renderCnyFen(value || 0),
    },
    {
      title: <span className='block w-full text-right'>{t('返利金额')}</span>,
      dataIndex: 'commission_fen',
      key: 'commission_fen',
      align: 'right',
      render: (value, record) => (
        <span className='inline-flex w-full items-center justify-end gap-2 text-right'>
          <span>{renderCnyFen(value || 0)}</span>
          <span className='rounded-full bg-[var(--app-card-muted)] px-2 py-0.5 text-xs text-semi-color-text-2'>
            {record.is_first_purchase ? t('首次') : t('续订')}
          </span>
        </span>
      ),
    },
    {
      title: <span className='block w-full text-right'>{t('购买时间')}</span>,
      dataIndex: 'paid_at',
      key: 'paid_at',
      align: 'right',
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
  ];

  const balanceColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('变动类型'),
      dataIndex: 'type',
      key: 'type',
      render: (value) => renderBalanceRecordType(value),
    },
    {
      title: <span className='block w-full text-right'>{t('变动金额')}</span>,
      dataIndex: 'delta_fen',
      key: 'delta_fen',
      align: 'right',
      render: (value) => renderSignedCnyFen(value || 0),
    },
    {
      title: <span className='block w-full text-right'>{t('变动后余额')}</span>,
      dataIndex: 'balance_after_fen',
      key: 'balance_after_fen',
      align: 'right',
      render: (value) => renderCnyFen(value || 0),
    },
    {
      title: t('说明'),
      dataIndex: 'remark',
      key: 'remark',
      render: (value) => (
        <div className='max-w-[260px] break-words text-semi-color-text-1'>
          {value || '-'}
        </div>
      ),
    },
  ];

  return (
    <ConsolePage className='min-h-0'>
      <div className='relative min-h-screen lg:min-h-0'>
        <TransferModal
          t={t}
          openTransfer={openTransfer}
          transfer={transfer}
          handleTransferCancel={() => setOpenTransfer(false)}
          userState={userState}
          renderMoneyFen={renderCnyFen}
          transferAmountYuan={transferAmountYuan}
          setTransferAmountYuan={setTransferAmountYuan}
        />

        <main className='w-full px-0 py-0'>
          <section className='flex flex-col gap-6'>
            <section className='overflow-hidden rounded-2xl bg-[var(--app-card)] shadow-[var(--app-shadow)]'>
              <div className='relative overflow-hidden px-5 py-6 sm:px-6 sm:py-7'>
                <div className='absolute -right-16 -top-16 h-40 w-40 rounded-full bg-primary/10 blur-3xl' />
                <div className='absolute -bottom-12 left-1/3 h-32 w-32 rounded-full bg-primary/5 blur-3xl' />

                <div className='relative flex flex-col gap-5'>
                  <div className='min-w-0'>
                    <span className='inline-flex items-center rounded-full bg-[var(--app-card-muted)] px-3 py-1 text-xs font-medium text-semi-color-text-2'>
                      {t('我的邀请')}
                    </span>
                    <h1 className='mt-4 text-2xl font-semibold tracking-tight text-semi-color-text-0 sm:text-3xl'>
                      {t('邀请好友完成付费获得返利')}
                    </h1>
                    <p className='mt-3 max-w-2xl text-sm leading-6 text-semi-color-text-2'>
                      {t('邀请好友注册，好友完成付费后您可获得返利')}
                    </p>
                    <p className='mt-4 max-w-2xl text-sm leading-6 text-semi-color-text-2'>
                      {t(
                        '返利会自动计入返利余额，可转入账户余额用于抵扣套餐。',
                      )}
                    </p>
                  </div>

                  <div className='flex flex-wrap items-center gap-3'>
                    <button
                      type='button'
                      disabled={!availableRebateFen || availableRebateFen <= 0}
                      onClick={() => setOpenTransfer(true)}
                      className='inline-flex items-center justify-center whitespace-nowrap rounded-md bg-neutral-900 px-3 py-2 text-sm font-semibold text-white transition focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-neutral-200 dark:text-neutral-900'
                    >
                      {t('转入账户余额')}
                    </button>

                    <button
                      type='button'
                      disabled={!affLink}
                      onClick={handleAffLinkClick}
                      className='inline-flex items-center justify-center whitespace-nowrap rounded-md border border-[var(--app-border)] bg-transparent px-3 py-2 text-sm font-semibold text-semi-color-text-0 transition focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60'
                    >
                      <Copy className='mr-2 h-4 w-4' />
                      {t('复制邀请链接')}
                    </button>
                  </div>
                </div>
              </div>

              <div className='border-t border-[var(--app-border)] px-5 py-5 sm:px-6'>
                <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
                  {stats.map((item) => {
                    const Icon = item.icon;
                    return (
                      <div
                        key={item.key}
                        className='rounded-xl bg-[var(--app-card-muted)] p-4'
                      >
                        <div className='flex items-center justify-between gap-3 text-sm text-semi-color-text-2'>
                          <span>{item.label}</span>
                          <Icon className='h-5 w-5' />
                        </div>
                        <div className='mt-3 text-2xl font-semibold text-semi-color-text-0'>
                          {item.value}
                        </div>
                        <div className='mt-1 text-xs text-semi-color-text-2'>
                          {item.helper}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </section>

            <section className='grid grid-cols-1 gap-4 lg:grid-cols-12'>
              <div className='flex flex-col gap-4 lg:col-span-4'>
                <section className='rounded-2xl bg-[var(--app-card)] shadow-[var(--app-shadow)]'>
                  <div className='p-5'>
                    <div className='flex items-start justify-between gap-3'>
                      <div>
                        <h3 className='text-lg font-medium text-semi-color-text-0'>
                          {t('推广素材')}
                        </h3>
                        <p className='mt-1 text-sm leading-6 text-semi-color-text-2'>
                          {t('分享专属链接，好友付费后系统将自动结算返利。')}
                        </p>
                      </div>
                      <Copy className='mt-1 h-5 w-5 text-semi-color-text-2' />
                    </div>

                    <div className='mt-5 rounded-xl bg-[var(--app-card-muted)] p-4'>
                      <div className='text-xs uppercase tracking-wide text-semi-color-text-2'>
                        {t('邀请链接')}
                      </div>
                      <div className='mt-3 break-all rounded-lg border border-dashed border-[var(--app-border)] bg-[var(--app-card)] px-3 py-3 font-mono text-sm text-semi-color-text-1'>
                        {affLink || t('加载中...')}
                      </div>
                      <button
                        type='button'
                        disabled={!affLink}
                        onClick={handleAffLinkClick}
                        className='mt-3 inline-flex items-center justify-center whitespace-nowrap rounded-md bg-neutral-900 px-3 py-2 text-xs font-semibold text-white transition focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-neutral-200 dark:text-neutral-900'
                      >
                        <Copy className='mr-1 h-4 w-4' />
                        {t('复制链接')}
                      </button>
                    </div>
                  </div>
                </section>

                <section className='rounded-2xl bg-[var(--app-card)] shadow-[var(--app-shadow)]'>
                  <div className='p-5'>
                    <div className='flex items-start justify-between gap-3'>
                      <div>
                        <h3 className='text-lg font-medium text-semi-color-text-0'>
                          {t('分享福利')}
                        </h3>
                        <p className='mt-1 text-sm leading-6 text-semi-color-text-2'>
                          {t(
                            '返利会自动计入返利余额，可转入账户余额用于抵扣套餐。',
                          )}
                        </p>
                      </div>
                      <Coins className='mt-1 h-5 w-5 text-semi-color-text-2' />
                    </div>

                    <div className='mt-5 grid gap-3 sm:grid-cols-2'>
                      <div className='rounded-xl bg-[var(--app-card-muted)] p-4'>
                        <div className='text-xs uppercase tracking-wide text-semi-color-text-2'>
                          {t('首次')}
                        </div>
                        <div className='mt-2 text-2xl font-semibold text-semi-color-text-0'>
                          {commissionFirstPercent}%
                        </div>
                      </div>
                      <div className='rounded-xl bg-[var(--app-card-muted)] p-4'>
                        <div className='text-xs uppercase tracking-wide text-semi-color-text-2'>
                          {t('续订')}
                        </div>
                        <div className='mt-2 text-2xl font-semibold text-semi-color-text-0'>
                          {commissionRepeatPercent}%
                        </div>
                      </div>
                    </div>

                    <ul className='mt-5 space-y-3 text-sm leading-6 text-semi-color-text-2'>
                      <li className='flex gap-2'>
                        <span className='mt-2 h-1.5 w-1.5 flex-none rounded-full bg-primary' />
                        <span>
                          {t(
                            '好友通过专属链接注册并完成付费（在线支付购买订阅、按量商品或兑换付费码）后，你将获得返利。',
                          )}
                        </span>
                      </li>
                      <li className='flex gap-2'>
                        <span className='mt-2 h-1.5 w-1.5 flex-none rounded-full bg-primary' />
                        <span>
                          {t(
                            '首订返利比例：{{firstPercent}}%，续订返利比例：{{repeatPercent}}%。',
                            {
                              firstPercent: commissionFirstPercent,
                              repeatPercent: commissionRepeatPercent,
                            },
                          )}
                        </span>
                      </li>
                      <li className='flex gap-2'>
                        <span className='mt-2 h-1.5 w-1.5 flex-none rounded-full bg-primary' />
                        <span>
                          {t(
                            '返利会自动计入返利余额，可转入账户余额用于抵扣套餐。',
                          )}
                        </span>
                      </li>
                    </ul>
                  </div>
                </section>
              </div>

              <div className='flex lg:col-span-8'>
                <CardPro
                  type='type1'
                  className='w-full'
                  descriptionArea={
                    <div className='flex flex-wrap items-start justify-between gap-3'>
                      <div>
                        <h3 className='text-lg font-medium text-semi-color-text-0'>
                          {t('返利记录')}
                        </h3>
                        <p className='text-xs text-semi-color-text-2'>
                          {t('最近邀请转化明细，可按需接入筛选与导出功能。')}
                        </p>
                      </div>

                      <div className='rounded-xl bg-[var(--app-card-muted)] px-3 py-2'>
                        <div className='text-xs text-semi-color-text-2'>
                          {t('产生付费')}
                        </div>
                        <div className='mt-1 flex items-center gap-2 text-sm font-semibold text-semi-color-text-0'>
                          <BarChart2 className='h-4 w-4' />
                          {paidUserCount}
                        </div>
                      </div>
                    </div>
                  }
                  paginationArea={
                    recordsTotal > records.length ? (
                      <div className='flex w-full justify-center'>
                        <button
                          type='button'
                          disabled={recordsLoading}
                          onClick={() => loadRecords(recordsPage + 1, true)}
                          className='inline-flex items-center justify-center whitespace-nowrap rounded-md bg-neutral-900 px-3 py-2 text-xs font-semibold text-white transition focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-neutral-200 dark:text-neutral-900'
                        >
                          {recordsLoading ? t('加载中...') : t('加载更多')}
                        </button>
                      </div>
                    ) : null
                  }
                  t={t}
                >
                  <CardTable
                    columns={rebateColumns}
                    dataSource={records}
                    loading={recordsLoading && records.length === 0}
                    rowKey={(record) =>
                      record.order_id || `${record.user_id}-${record.paid_at}`
                    }
                    hidePagination={true}
                    scroll={{ x: '100%' }}
                    empty={
                      <div className='py-12 text-center text-sm text-semi-color-text-2'>
                        {t('暂无返利记录')}
                      </div>
                    }
                    className='overflow-hidden rounded-xl'
                    size='middle'
                  />
                </CardPro>
              </div>
            </section>

            <CardPro
              type='type1'
              descriptionArea={
                <div className='flex flex-wrap items-start justify-between gap-3'>
                  <div>
                    <h3 className='text-lg font-medium text-semi-color-text-0'>
                      {t('账户余额记录')}
                    </h3>
                    <p className='text-xs text-semi-color-text-2'>
                      {t('最近账户余额变动明细')}
                    </p>
                  </div>

                  <div className='rounded-xl bg-[var(--app-card-muted)] px-3 py-2'>
                    <div className='text-xs text-semi-color-text-2'>
                      {t('账户余额')}
                    </div>
                    <div className='mt-1 flex items-center gap-2 text-sm font-semibold text-semi-color-text-0'>
                      <Wallet className='h-4 w-4' />
                      {renderCnyFen(accountBalanceFen)}
                    </div>
                  </div>
                </div>
              }
              paginationArea={
                balanceRecordsTotal > balanceRecords.length ? (
                  <div className='flex w-full justify-center'>
                    <button
                      type='button'
                      disabled={balanceRecordsLoading}
                      onClick={() =>
                        loadBalanceRecords(balanceRecordsPage + 1, true)
                      }
                      className='inline-flex items-center justify-center whitespace-nowrap rounded-md bg-neutral-900 px-3 py-2 text-xs font-semibold text-white transition focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-neutral-200 dark:text-neutral-900'
                    >
                      {balanceRecordsLoading ? t('加载中...') : t('加载更多')}
                    </button>
                  </div>
                ) : null
              }
              t={t}
            >
              <CardTable
                columns={balanceColumns}
                dataSource={balanceRecords}
                loading={balanceRecordsLoading && balanceRecords.length === 0}
                rowKey={(record) =>
                  record.id || `${record.type}-${record.created_at}`
                }
                hidePagination={true}
                scroll={{ x: '100%' }}
                empty={
                  <div className='py-12 text-center text-sm text-semi-color-text-2'>
                    {t('暂无余额记录')}
                  </div>
                }
                className='overflow-hidden rounded-xl'
                size='middle'
              />
            </CardPro>
          </section>
        </main>
      </div>
    </ConsolePage>
  );
};

export default Invitation;
