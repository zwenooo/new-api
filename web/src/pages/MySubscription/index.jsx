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

import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Radio,
  Typography,
} from '@douyinfe/semi-ui';
import { WalletCards } from 'lucide-react';
import {
  API,
  renderNumber,
  renderQuota,
  renderQuotaToUSD,
  setUserData,
  showError,
  showInfo,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import ConsolePage from '../../components/layout/ConsolePage';
import { UserContext } from '../../context/User';
import { useDashboardStats } from '../../hooks/dashboard/useDashboardStats';

const { Text } = Typography;

const EMPTY_TREND_DATA = {
  balance: [],
  usedQuota: [],
  requestCount: [],
  times: [],
  consumeQuota: [],
  tokens: [],
  rpm: [],
  tpm: [],
};

const EMPTY_PERFORMANCE_METRICS = {
  avgRPM: '0',
  avgTPM: '0',
  timeDiff: 0,
};

const formatRatio = (ratioValue) => {
  const ratio = Number(ratioValue);
  if (!Number.isFinite(ratio)) return '1';
  return ratio.toFixed(6).replace(/\.?0+$/, '');
};

const normalizeGroupIds = (rawIds) => {
  if (!Array.isArray(rawIds)) return [];
  const seen = new Set();
  const out = [];
  rawIds.forEach((raw) => {
    const num = Number(raw);
    if (!Number.isFinite(num) || num <= 0) return;
    const id = Math.floor(num);
    if (id <= 0) return;
    if (seen.has(id)) return;
    seen.add(id);
    out.push(id);
  });
  return out.sort((a, b) => a - b);
};

const MySubscription = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);

  const [refreshing, setRefreshing] = useState(false);
  const [groupRatios, setGroupRatios] = useState({});
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);
  const [redeemCode, setRedeemCode] = useState('');
  const [redeemLoading, setRedeemLoading] = useState(false);
  const [redeemApplyMode, setRedeemApplyMode] = useState('stack');
  const [redeemConfirmOpen, setRedeemConfirmOpen] = useState(false);
  const [recordView, setRecordView] = useState('active');
  const [activatingRecordKey, setActivatingRecordKey] = useState('');

  const groupLabelById = useMemo(() => {
    const m = {};
    (Array.isArray(availableGroups) ? availableGroups : []).forEach((g) => {
      const idRaw = Number(g?.id ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return;
      const label = String(g?.display_name || '').trim();
      const code = String(g?.code || '').trim();
      const name = label || code;
      if (!name) return;
      m[id] = name;
    });
    return m;
  }, [availableGroups]);
  const getGroupLabel = useCallback(
    (rawId) => {
      const idRaw = Number(rawId ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return t('未知分组');
      return groupLabelById[id] || t('未知分组');
    },
    [groupLabelById, t],
  );

  const fetchGroups = useCallback(async () => {
    setGroupsLoading(true);
    try {
      const res = await API.get('/api/group/resolve');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        setAvailableGroups([]);
        return false;
      }
      setAvailableGroups(Array.isArray(data) ? data : []);
      return true;
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
      setAvailableGroups([]);
      return false;
    } finally {
      setGroupsLoading(false);
    }
  }, [t]);

  const loadGroupRatios = useCallback(async () => {
    try {
      const res = await API.get('/api/pricing');
      const { success, message, group_ratio: groupRatio } = res.data || {};
      if (!success) {
        showError(message || t('获取分组倍率失败'));
        return false;
      }
      setGroupRatios(
        groupRatio && typeof groupRatio === 'object' ? groupRatio : {},
      );
      return true;
    } catch (e) {
      showError(e?.message || t('获取分组倍率失败'));
      return false;
    }
  }, [t]);

  const syncUserState = useCallback(async () => {
    try {
      const res = await API.get('/api/user/self');
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        setUserData(data);
        return true;
      } else {
        showError(message || t('刷新失败'));
        return false;
      }
    } catch (e) {
      showError(e?.message || t('刷新失败'));
      return false;
    }
  }, [t, userDispatch]);

  const refreshSubscriptionView = useCallback(
    async ({ silent = false } = {}) => {
      if (!silent) setRefreshing(true);
      try {
        const results = await Promise.all([
          syncUserState(),
          loadGroupRatios(),
          fetchGroups(),
        ]);
        if (!silent && results.every(Boolean)) {
          showSuccess(t('刷新成功'));
        }
        return results.every(Boolean);
      } finally {
        if (!silent) setRefreshing(false);
      }
    },
    [fetchGroups, loadGroupRatios, syncUserState, t],
  );

  useEffect(() => {
    void refreshSubscriptionView({ silent: true });
  }, [refreshSubscriptionView]);

  const openRedeemConfirm = () => {
    const code = redeemCode.trim();
    if (!code) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setRedeemApplyMode('stack');
    setRedeemConfirmOpen(true);
  };

  const confirmRedeem = async () => {
    const code = redeemCode.trim();
    if (!code) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    if (redeemApplyMode !== 'stack' && redeemApplyMode !== 'defer') {
      showError(t('请选择叠加或顺延'));
      return;
    }

    setRedeemLoading(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: code,
        apply_mode: redeemApplyMode,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      const addedQuota = data?.added_quota ?? data ?? 0;
      const refreshedUser = data?.user;
      const redeem = data?.redeem;

      showSuccess(t('兑换成功！'));
      let content = t('成功兑换额度：') + renderQuota(addedQuota);
      if (redeem?.mode === 'tokens') {
        content = `${t('成功兑换tokens：')}${renderNumber(addedQuota)} tokens`;
      } else if (redeem?.mode === 'pay_token') {
        content = `${t('成功兑换tokens：')}${renderNumber(addedQuota)} tokens`;
      } else if (redeem?.mode === 'request') {
        content = t('次数订阅已开通');
      } else if (redeem?.mode === 'pay_request') {
        content = `${t('成功兑换次数：')}${renderNumber(addedQuota)} ${t('次')}`;
      }
      Modal.success({
        title: t('兑换成功！'),
        content,
        centered: true,
      });

      if (refreshedUser && Object.keys(refreshedUser).length > 0) {
        userDispatch({ type: 'login', payload: refreshedUser });
        setUserData(refreshedUser);
      }
      await refreshSubscriptionView({ silent: true });

      setRedeemCode('');
      setRedeemConfirmOpen(false);
    } catch (err) {
      showError(t('请求失败'));
    } finally {
      setRedeemLoading(false);
    }
  };

  const activatePendingRecord = useCallback(
    async (item) => {
      const subId = Number(item?.id ?? 0) || 0;
      const activationKind = String(item?.activationKind || '').trim();
      if (subId <= 0 || !activationKind) {
        showError(t('无效的订阅记录'));
        return;
      }

      let path = '';
      if (activationKind === 'subscription') {
        path = `/api/user/self/subscriptions/${subId}/activate`;
      } else if (activationKind === 'request') {
        path = `/api/user/self/request_subscriptions/${subId}/activate`;
      } else {
        showError(t('无效的订阅记录'));
        return;
      }

      const recordKey = `${activationKind}:${subId}`;
      setActivatingRecordKey(recordKey);
      try {
        const res = await API.post(path);
        const { success, message } = res.data || {};
        if (!success) {
          showError(message || t('立即生效失败'));
          return;
        }
        showSuccess(t('已立即生效'));
        await refreshSubscriptionView({ silent: true });
      } catch (e) {
        showError(e?.message || t('立即生效失败'));
      } finally {
        setActivatingRecordKey('');
      }
    },
    [refreshSubscriptionView, t],
  );

  const { groupedStatsData } = useDashboardStats(
    userState,
    0,
    0,
    0,
    0,
    0,
    0,
    0,
    null,
    null,
    EMPTY_TREND_DATA,
    EMPTY_PERFORMANCE_METRICS,
    navigate,
    t,
    { hideFreeQuota: true },
  );

  const accountItems = useMemo(() => {
    const group = groupedStatsData.find((g) => g.key === 'account');
    return Array.isArray(group?.items) ? group.items : [];
  }, [groupedStatsData]);

  const recordItems = useMemo(() => {
    const renderGroupTags = (groupIds) => {
      const normalized = normalizeGroupIds(groupIds);
      if (normalized.length === 0) return '-';
      return (
        <span className='flex flex-col items-start gap-1'>
          {normalized.map((gid) => {
            const label = getGroupLabel(gid);
            return (
              <Text key={gid} code style={{ fontSize: 12 }}>
                {`${label} * ${formatRatio(groupRatios?.[gid])}`}
              </Text>
            );
          })}
        </span>
      );
    };

    const formatTs = (timestamp) => {
      const ts = Number(timestamp) || 0;
      if (!ts || Number.isNaN(ts)) return t('未知');
      return timestamp2string(ts).replaceAll('-', '/');
    };

    const formatPeriod = (startAtSec, endAtSec) => {
      const startLabel = startAtSec ? formatTs(startAtSec) : t('未知');
      const endLabel = endAtSec ? formatTs(endAtSec) : t('不限时');
      return `${startLabel} - ${endLabel}`;
    };

    const items = [];

    const payRequestBalances = Array.isArray(
      userState?.user?.pay_request_balances,
    )
      ? userState.user.pay_request_balances
      : [];
    payRequestBalances
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.product_id ?? 0) || 0;
        const ib = Number(b?.product_id ?? 0) || 0;
        return ib - ia;
      })
      .forEach((b) => {
        const remaining = Number(b?.remaining_requests ?? 0) || 0;
        if (remaining <= 0) return;
        const name = String(b?.product_name ?? '').trim();
        const title = name || t('按次付费');
        const allowedGroupIds = normalizeGroupIds(b?.allowed_group_ids);
        items.push({
          recordType: 'quota',
          title,
          period: t('不限时'),
          quotaTable: {
            headers: [t('分组'), t('剩余次数')],
            values: [renderGroupTags(allowedGroupIds), renderNumber(remaining)],
          },
        });
      });
    const payRequestQuota =
      Number(userState?.user?.pay_request_quota ?? 0) || 0;
    if (payRequestBalances.length === 0 && payRequestQuota > 0) {
      items.push({
        recordType: 'quota',
        title: t('按次付费'),
        period: t('不限时'),
        quotaTable: {
          headers: [t('剩余次数')],
          values: [renderNumber(payRequestQuota)],
        },
      });
    }

    const payTokenBalances = Array.isArray(userState?.user?.pay_token_balances)
      ? userState.user.pay_token_balances
      : [];
    payTokenBalances
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.product_id ?? 0) || 0;
        const ib = Number(b?.product_id ?? 0) || 0;
        return ib - ia;
      })
      .forEach((b) => {
        const remaining = Number(b?.remaining_tokens ?? 0) || 0;
        if (remaining <= 0) return;
        const name = String(b?.product_name ?? '').trim();
        const title = name || t('按token付费');
        const allowedGroupIds = normalizeGroupIds(b?.allowed_group_ids);
        items.push({
          recordType: 'quota',
          title,
          period: t('不限时'),
          quotaTable: {
            headers: [t('分组'), 'Tokens'],
            values: [renderGroupTags(allowedGroupIds), renderNumber(remaining)],
          },
        });
      });
    const payTokenQuota = Number(userState?.user?.pay_token_quota ?? 0) || 0;
    if (payTokenBalances.length === 0 && payTokenQuota > 0) {
      items.push({
        recordType: 'quota',
        title: t('按token付费'),
        period: t('不限时'),
        quotaTable: {
          headers: ['Tokens'],
          values: [renderNumber(payTokenQuota)],
        },
      });
    }

    const paygBalances = Array.isArray(userState?.user?.payg_balances)
      ? userState.user.payg_balances
      : [];
    paygBalances
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.product_id ?? 0) || 0;
        const ib = Number(b?.product_id ?? 0) || 0;
        return ib - ia;
      })
      .forEach((b) => {
        const name = String(b?.product_name ?? '').trim();
        const title = name || t('按量付费');
        const allowedGroupIds = normalizeGroupIds(b?.allowed_group_ids);
        const remaining = Number(b?.remaining_quota ?? 0) || 0;
        if (remaining <= 0) return;
        items.push({
          recordType: 'quota',
          title,
          period: t('不限时'),
          quotaTable: {
            headers: [t('分组'), t('剩余额度')],
            values: [
              renderGroupTags(allowedGroupIds),
              renderQuotaToUSD(remaining),
            ],
          },
        });
      });

    const requestBreakdown = userState?.user?.request_subscription_breakdown;
    const requestSubscriptions = Array.isArray(requestBreakdown?.subscriptions)
      ? requestBreakdown.subscriptions
      : [];
    const nowUnix = Math.floor(Date.now() / 1000);
    requestSubscriptions.forEach((subscription, idx) => {
      const dailyLimit = Number(subscription?.daily_request_limit ?? 0) || 0;
      const totalLimit = Number(subscription?.total_request_limit ?? 0) || 0;
      const dailyUnlimited = dailyLimit === 0;
      const totalUnlimited = totalLimit === 0;
      const dailyUsed = Number(subscription?.daily_request_used ?? 0) || 0;
      const dailyRemaining =
        Number(subscription?.daily_request_remaining ?? 0) || 0;
      const totalRemaining =
        Number(subscription?.total_request_remaining ?? 0) || 0;
      const startAt = Number(subscription?.start_at ?? 0) || 0;
      const expireAt = Number(subscription?.expire_at ?? 0) || 0;
      const isExpired = expireAt > 0 && expireAt < nowUnix;
      const isPending = startAt > nowUnix;
      const totalDepleted =
        !totalUnlimited && !isExpired && !isPending && totalRemaining <= 0;
      const dailyDepleted =
        !dailyUnlimited && !isExpired && !isPending && dailyRemaining <= 0;
      const isHistorical = isExpired || totalDepleted;
      const maskText = isExpired
        ? t('已过期')
        : isPending
          ? t('待生效')
          : totalDepleted
            ? t('总次数已用尽')
            : dailyDepleted
              ? t('当日已用尽')
              : null;
      const allowedGroupIds = normalizeGroupIds(
        subscription?.allowed_group_ids,
      );
      const presetName = String(subscription?.source_preset_name || '').trim();
      const redemptionName = String(
        subscription?.source_redemption_name || '',
      ).trim();
      const sourceName = presetName || redemptionName;
      const titleExtra = totalUnlimited
        ? `${t('总次数')}：${t('无限')}`
        : `${t('总剩余')}：${renderNumber(totalRemaining)} / ${renderNumber(totalLimit)}`;
      items.push({
        id: Number(subscription?.id ?? 0) || 0,
        recordType: 'subscription',
        isHistorical,
        isPending,
        activationKind: 'request',
        title: sourceName
          ? sourceName
          : requestSubscriptions.length > 1
            ? t('次数订阅 {{index}}', { index: idx + 1 })
            : t('次数订阅'),
        period: formatPeriod(startAt, expireAt),
        maskText,
        titleExtra,
        allowed_group_ids: [],
        quotaTable: {
          headers: [t('分组'), t('当日消耗'), t('当日剩余'), t('当日限额')],
          values: [
            renderGroupTags(allowedGroupIds),
            renderNumber(dailyUsed),
            dailyUnlimited ? t('无限') : renderNumber(dailyRemaining),
            dailyUnlimited ? t('无限') : renderNumber(dailyLimit),
          ],
        },
      });
    });

    const tokenBreakdown = userState?.user?.token_subscription_breakdown;
    const tokenGroupCaps = Array.isArray(tokenBreakdown?.group_capacities)
      ? tokenBreakdown.group_capacities
      : [];
    if (tokenGroupCaps.length > 0) {
      const values = [];
      tokenGroupCaps
        .map((cap) => {
          const rawId = Number(cap?.group_id ?? 0);
          const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
          if (groupId <= 0) return null;
          const totalRemaining = Number(cap?.total_remaining ?? 0);
          const dailyCapacity = Number(cap?.daily_capacity ?? 0);
          return {
            group_id: groupId,
            total_remaining: Number.isFinite(totalRemaining)
              ? Math.max(0, Math.floor(totalRemaining))
              : 0,
            daily_capacity: Number.isFinite(dailyCapacity)
              ? Math.max(0, Math.floor(dailyCapacity))
              : 0,
            total_unlimited: Boolean(cap?.total_unlimited),
            daily_unlimited: Boolean(cap?.daily_unlimited),
          };
        })
        .filter(Boolean)
        .sort((a, b) => a.group_id - b.group_id)
        .forEach((cap) => {
          values.push(
            <Text key={`tok-cap-${cap.group_id}`} code style={{ fontSize: 12 }}>
              {`${getGroupLabel(cap.group_id)} * ${formatRatio(groupRatios?.[cap.group_id])}`}
            </Text>,
          );
          values.push(
            cap.total_unlimited ? t('无限') : renderNumber(cap.total_remaining),
          );
          values.push(
            cap.daily_unlimited ? t('无限') : renderNumber(cap.daily_capacity),
          );
        });

      if (values.length > 0) {
        items.push({
          recordType: 'subscription',
          isHistorical: false,
          title: t('Tokens订阅（分组）'),
          period: t('今日'),
          allowed_group_ids: [],
          quotaTable: {
            headers: [t('分组'), t('总剩余'), t('今日可用')],
            values,
          },
        });
      }
    }

    const tokenSubscriptions = Array.isArray(tokenBreakdown?.subscriptions)
      ? tokenBreakdown.subscriptions
      : [];
    tokenSubscriptions.forEach((subscription, idx) => {
      const totalLimit = Number(subscription?.total_tokens ?? 0) || 0;
      const totalUnlimited = totalLimit === 0;
      const totalRemaining = Number(subscription?.remaining_tokens ?? 0) || 0;

      const dailyLimit = Number(subscription?.daily_tokens_limit ?? 0) || 0;
      const dailyUnlimited = dailyLimit === 0;
      const dailyUsed = Number(subscription?.daily_tokens_used ?? 0) || 0;
      const dailyRemaining = dailyUnlimited
        ? 0
        : Math.max(0, dailyLimit - dailyUsed);

      const startAt = Number(subscription?.start_at ?? 0) || 0;
      const expireAt = Number(subscription?.expire_at ?? 0) || 0;
      const isExpired = expireAt > 0 && expireAt < nowUnix;
      const isPending = startAt > nowUnix;
      const totalDepleted =
        !totalUnlimited && !isExpired && !isPending && totalRemaining <= 0;
      const dailyDepleted =
        !dailyUnlimited && !isExpired && !isPending && dailyRemaining <= 0;
      const isHistorical = isExpired || totalDepleted;

      const maskText = isExpired
        ? t('已过期')
        : isPending
          ? t('待生效')
          : totalDepleted
            ? t('总tokens已用尽')
            : dailyDepleted
              ? t('当日已用尽')
              : null;

      const allowedGroupIds = normalizeGroupIds(
        subscription?.allowed_group_ids,
      );
      const presetName = String(subscription?.source_preset_name || '').trim();
      const redemptionName = String(
        subscription?.source_redemption_name || '',
      ).trim();
      const sourceName = presetName || redemptionName;

      const titleExtra = totalUnlimited
        ? `${t('总tokens')}：${t('无限')}`
        : `${t('总剩余')}：${renderNumber(totalRemaining)} / ${renderNumber(totalLimit)}`;

      items.push({
        id: Number(subscription?.id ?? 0) || 0,
        recordType: 'subscription',
        isHistorical,
        isPending,
        activationKind: 'subscription',
        title: sourceName
          ? sourceName
          : tokenSubscriptions.length > 1
            ? t('Tokens订阅 {{index}}', { index: idx + 1 })
            : t('Tokens订阅'),
        period: formatPeriod(startAt, expireAt),
        maskText,
        titleExtra,
        allowed_group_ids: [],
        quotaTable: {
          headers: [t('分组'), t('当日消耗'), t('当日剩余'), t('当日限额')],
          values: [
            renderGroupTags(allowedGroupIds),
            renderNumber(dailyUsed),
            dailyUnlimited ? t('无限') : renderNumber(dailyRemaining),
            dailyUnlimited ? t('无限') : renderNumber(dailyLimit),
          ],
        },
      });
    });

    const patchedAccountItems = (
      Array.isArray(accountItems) ? accountItems : []
    ).map((item) => {
      const patchedBase = {
        ...item,
        recordType: 'subscription',
        isHistorical: Boolean(item?.isExpired),
      };

      const quotaTable = patchedBase?.quotaTable;
      if (
        !quotaTable ||
        !Array.isArray(quotaTable.headers) ||
        !Array.isArray(quotaTable.values)
      ) {
        return patchedBase;
      }
      const groupBreakdownRaw = Array.isArray(
        patchedBase?.group_quota_breakdown,
      )
        ? patchedBase.group_quota_breakdown
        : [];
      const groupBreakdown = groupBreakdownRaw
        .map((row) => {
          const rawId = Number(row?.group_id ?? 0);
          const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
          if (groupId <= 0) return null;
          const dailyQuotaUsed = Number(row?.daily_quota_used ?? 0);
          const dailyQuotaAvailable = Number(row?.daily_quota_available ?? 0);
          const dailyQuotaLimit = Number(row?.daily_quota_limit ?? 0);
          return {
            group_id: groupId,
            daily_quota_used: Number.isFinite(dailyQuotaUsed)
              ? Math.max(0, Math.floor(dailyQuotaUsed))
              : 0,
            daily_quota_available: Number.isFinite(dailyQuotaAvailable)
              ? Math.max(0, Math.floor(dailyQuotaAvailable))
              : 0,
            daily_quota_limit: Number.isFinite(dailyQuotaLimit)
              ? Math.max(0, Math.floor(dailyQuotaLimit))
              : 0,
          };
        })
        .filter(Boolean)
        .sort((a, b) => a.group_id - b.group_id);

      if (
        groupBreakdown.length > 0 &&
        quotaTable.headers.length === 4 &&
        quotaTable.values.length === 4
      ) {
        const headers = [t('分组'), ...quotaTable.headers.slice(0, 3)];
        const values = [];
        const totalQuotaValue = quotaTable.values[3];
        const titleExtra = totalQuotaValue
          ? `${t('总额度')}：${totalQuotaValue}`
          : null;
        groupBreakdown.forEach((row) => {
          values.push(
            <Text
              key={`sub-group-${patchedBase?.title || ''}-${row.group_id}`}
              code
              style={{ fontSize: 12 }}
            >
              {`${getGroupLabel(row.group_id)} * ${formatRatio(groupRatios?.[row.group_id])}`}
            </Text>,
          );
          values.push(renderQuota(row.daily_quota_used));
          values.push(renderQuota(row.daily_quota_available));
          values.push(
            row.daily_quota_limit > 0
              ? renderQuota(row.daily_quota_limit)
              : t('不限额'),
          );
        });
        return {
          ...patchedBase,
          titleExtra,
          allowed_group_ids: [],
          quotaTable: {
            headers,
            values,
          },
        };
      }

      const allowedGroupIds = normalizeGroupIds(patchedBase?.allowed_group_ids);
      if (allowedGroupIds.length === 0) return patchedBase;
      return {
        ...patchedBase,
        allowed_group_ids: [],
        quotaTable: {
          headers: [t('分组'), ...quotaTable.headers],
          values: [renderGroupTags(allowedGroupIds), ...quotaTable.values],
        },
      };
    });

    return [...items, ...patchedAccountItems];
  }, [
    accountItems,
    groupLabelById,
    groupRatios,
    t,
    userState?.user?.request_subscription_breakdown,
    userState?.user?.token_subscription_breakdown,
    userState?.user?.pay_request_quota,
    userState?.user?.payg_balances,
  ]);

  const activeRecordItems = useMemo(() => {
    return recordItems.filter(
      (item) => item?.recordType !== 'subscription' || !item?.isHistorical,
    );
  }, [recordItems]);

  const historyRecordItems = useMemo(() => {
    return recordItems.filter(
      (item) =>
        item?.recordType === 'subscription' && Boolean(item?.isHistorical),
    );
  }, [recordItems]);

  const visibleRecordItems =
    recordView === 'history' ? historyRecordItems : activeRecordItems;

  const renderQuotaItem = (item, idx) => {
    const hasQuotaTable =
      item.quotaTable &&
      Array.isArray(item.quotaTable.headers) &&
      Array.isArray(item.quotaTable.values);
    const isSingleQuotaTable =
      hasQuotaTable &&
      item.quotaTable.headers.length === 1 &&
      item.quotaTable.values.length === 1;
    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);
    const canActivateNow =
      Boolean(item?.isPending) &&
      Number(item?.id ?? 0) > 0 &&
      (item?.activationKind === 'subscription' ||
        item?.activationKind === 'request');
    const activating =
      canActivateNow &&
      activatingRecordKey === `${item.activationKind}:${Number(item.id ?? 0)}`;

    return (
      <div
        key={idx}
        className='subscription-record-item dashboard-info-card dashboard-info-card--static relative flex items-start justify-between gap-4 rounded-2xl px-4 py-3'
      >
        <div className='min-w-0 flex-1'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5'>
            <div className='text-sm font-semibold tracking-[0.01em] text-neutral-800 dark:text-neutral-100'>
              {item.title}
            </div>
            {item.period ? (
              <div className='text-[10px] leading-tight text-neutral-500 dark:text-neutral-400'>
                ({item.period})
              </div>
            ) : null}
            {item.titleExtra ? (
              <div className='text-[10px] leading-tight text-neutral-500 dark:text-neutral-400'>
                {item.titleExtra}
              </div>
            ) : null}
          </div>
          {!hasQuotaTable && item.subtitle && (
            <div className='mt-0.5 text-xs text-neutral-500 dark:text-neutral-400'>
              {item.subtitle}
            </div>
          )}
          {hasQuotaTable && (
            <div className='mt-2 min-h-8 text-xs text-neutral-600 dark:text-neutral-300'>
              {isSingleQuotaTable ? (
                <div className='flex min-h-8 items-center gap-1'>
                  <span>{item.quotaTable.headers[0]}</span>
                  <span className='text-neutral-500 dark:text-neutral-400'>
                    {' '}
                    :{' '}
                  </span>
                  <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                    {item.quotaTable.values[0]}
                  </span>
                </div>
              ) : (
                <div
                  className='grid gap-x-6 gap-y-1.5 text-[11px]'
                  style={{
                    gridTemplateColumns: `repeat(${item.quotaTable.headers.length || 1}, minmax(0, 1fr))`,
                  }}
                >
                  {item.quotaTable.headers.map((header, hIdx) => (
                    <div
                      key={`h-${hIdx}`}
                      className='truncate text-neutral-500 dark:text-neutral-400'
                    >
                      {header}
                    </div>
                  ))}
                  {item.quotaTable.values.map((val, vIdx) => (
                    <div
                      key={`v-${vIdx}`}
                      className='font-semibold text-neutral-800 dark:text-neutral-100'
                    >
                      {val}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
          {allowedGroupIds.length > 0 ? (
            <div className='mt-2 flex flex-wrap items-center gap-1 text-xs text-neutral-600 dark:text-neutral-300'>
              <span className='text-neutral-500 dark:text-neutral-400'>
                {t('可选分组')}：
              </span>
              <span className='inline-flex flex-wrap gap-1'>
                {allowedGroupIds.map((gid) => (
                  <Text key={gid} code style={{ fontSize: 12 }}>
                    {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                  </Text>
                ))}
              </span>
            </div>
          ) : null}
        </div>

        {item.maskText || canActivateNow ? (
          <div className='shrink-0 flex flex-col items-end gap-2'>
            {item.maskText ? (
              <div className='rounded-full bg-neutral-900/70 px-2 py-0.5 text-[10px] text-white shadow-sm'>
                {item.maskText}
              </div>
            ) : null}
            {canActivateNow ? (
              <Button
                theme='solid'
                type='primary'
                size='small'
                loading={activating}
                disabled={activating}
                onClick={() => void activatePendingRecord(item)}
              >
                {t('立即生效')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    );
  };

  return (
    <ConsolePage fillHeight className='my-subscription-page'>
      <div className='relative flex min-h-0 flex-1 flex-col'>
        <Modal
          title={t('兑换额度')}
          visible={redeemConfirmOpen}
          onOk={confirmRedeem}
          onCancel={() => setRedeemConfirmOpen(false)}
          maskClosable={false}
          centered
          okText={t('确认兑换')}
          cancelText={t('取消')}
          confirmLoading={redeemLoading}
          okButtonProps={{ disabled: redeemLoading }}
        >
          <div className='space-y-3 pb-2'>
            <div>
              <div className='mb-2 font-medium'>{t('生效方式')}</div>
              <Radio.Group
                type='button'
                value={redeemApplyMode}
                onChange={(val) => {
                  const selected = val && val.target ? val.target.value : val;
                  setRedeemApplyMode(selected);
                }}
              >
                <Radio value='stack'>{t('叠加（立即生效）')}</Radio>
                <Radio value='defer'>{t('顺延（到期后生效）')}</Radio>
              </Radio.Group>
              <div className='text-xs text-gray-500'>
                {redeemApplyMode === 'defer'
                  ? t(
                      '若当前仍有有效的额度订阅，新兑换的订阅包将从当前订阅到期后开始计算有效期',
                    )
                  : t('订阅额度兑换后将立即生效')}
              </div>
            </div>
            <div className='text-xs text-gray-500'>
              {t('该选项仅对订阅类兑换码生效，非订阅类兑换码不受影响')}
            </div>
          </div>
        </Modal>

        <div className='flex w-full min-h-0 flex-1 flex-col gap-3'>
          <Card
            className='my-subscription-shell-card !rounded-2xl !border-0 !shadow-none'
            bodyStyle={{ padding: 18 }}
          >
            <div className='flex items-center justify-between gap-3'>
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  <WalletCards size={18} />
                  <Text className='text-lg font-medium'>{t('我的订阅')}</Text>
                </div>
              </div>
              <Button
                theme='light'
                onClick={() => void refreshSubscriptionView()}
                loading={refreshing}
                disabled={refreshing}
              >
                {t('刷新')}
              </Button>
            </div>
          </Card>

          <div className='grid grid-cols-1 gap-4'>
            <Card
              className='my-subscription-shell-card !rounded-2xl !border-0 !shadow-none'
              bodyStyle={{ padding: 18 }}
            >
              <div className='space-y-3'>
                <div className='text-sm font-medium'>{t('兑换码兑换')}</div>
                <Input
                  className='app-inline-action-input'
                  placeholder={t('请输入兑换码')}
                  value={redeemCode}
                  onChange={setRedeemCode}
                  onEnterPress={openRedeemConfirm}
                  addonAfter={
                    <Button
                      type='primary'
                      theme='solid'
                      loading={redeemLoading}
                      onClick={openRedeemConfirm}
                      className='!px-4'
                    >
                      {t('兑换额度')}
                    </Button>
                  }
                />
                <div className='text-xs text-gray-500'>
                  {t('兑换成功后可在「订阅记录」中查看生效情况')}
                </div>
              </div>
            </Card>
          </div>

          <Card
            className='my-subscription-shell-card !rounded-2xl !border-0 !shadow-none flex min-h-0 flex-1 flex-col'
            bodyStyle={{
              padding: 18,
              display: 'flex',
              flexDirection: 'column',
              flex: 1,
              minHeight: 0,
            }}
          >
            <div className='flex flex-wrap items-center justify-between gap-3'>
              <div className='text-sm font-medium'>{t('订阅记录')}</div>
              <Radio.Group
                type='button'
                value={recordView}
                onChange={(val) => {
                  const selected = val && val.target ? val.target.value : val;
                  if (selected !== 'active' && selected !== 'history') return;
                  setRecordView(selected);
                }}
              >
                <Radio value='active'>{t('有效订阅')}</Radio>
                <Radio value='history'>{t('历史订阅')}</Radio>
              </Radio.Group>
            </div>

            <div className='mt-3 flex min-h-0 flex-1 flex-col overflow-hidden'>
              {visibleRecordItems.length > 0 ? (
                <div className='min-h-0 flex-1 overflow-y-auto card-content-scroll pr-2'>
                  <div className='space-y-2'>
                    {visibleRecordItems.map((item, idx) =>
                      renderQuotaItem(item, idx),
                    )}
                  </div>
                </div>
              ) : (
                <div className='flex min-h-0 flex-1 justify-center items-center w-full'>
                  <Empty
                    title={
                      recordView === 'history'
                        ? t('暂无历史订阅')
                        : t('暂无订阅')
                    }
                    description={
                      recordView === 'history'
                        ? t('这里展示已过期或已用尽的订阅')
                        : historyRecordItems.length > 0
                          ? t('当前无有效订阅，可切换查看历史订阅')
                          : t('可通过兑换码获取订阅')
                    }
                  />
                </div>
              )}
            </div>
          </Card>
        </div>
      </div>
    </ConsolePage>
  );
};

export default MySubscription;
