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

import { useMemo } from 'react';
import { Wallet, Activity, Zap, Gauge } from 'lucide-react';
import {
  IconHistogram,
  IconCoinMoneyStroked,
  IconTextStroked,
  IconPulse,
  IconStopwatchStroked,
  IconTypograph,
  IconSend,
} from '@douyinfe/semi-icons';
import { isEffectiveAdmin, renderQuota, timestamp2string } from '../../helpers';
import { createSectionTitle } from '../../helpers/dashboard';

const formatTs = (timestamp) => {
  const ts = Number(timestamp) || 0;
  if (!ts || Number.isNaN(ts)) return '';
  return timestamp2string(ts).replaceAll('-', '/');
};

export const useDashboardStats = (
  userState,
  consumeQuota,
  standardConsumeQuota,
  consumeTokens,
  times,
  todayUsedQuota,
  todayStandardUsedQuota,
  todayUsedTimes,
  todayCacheHitRate,
  todayGlobalCacheHitRate,
  trendData,
  performanceMetrics,
  navigate,
  t,
  options = {},
) => {
  const nowSec = Math.floor(Date.now() / 1000);
  const quotaBreakdown = userState?.user?.quota_breakdown ?? {};
  const paygRemaining = Number(quotaBreakdown?.payg_remaining ?? 0) || 0;
  const actualUsedQuota = Number(userState?.user?.used_quota ?? 0) || 0;
  const standardUsedQuota =
    userState?.user?.cost_used_quota !== undefined &&
    userState?.user?.cost_used_quota !== null
      ? Number(userState.user.cost_used_quota) || 0
      : 0;

  const activeSubscriptions = quotaBreakdown?.subscriptions ?? [];
  const subscriptionsAll = quotaBreakdown?.subscriptions_all ?? [];

  const formatPeriod = (startAtSec, endAtSec) => {
    const startLabel = formatTs(startAtSec) || t('未知');
    const endLabel =
      Number(endAtSec) > 0 ? formatTs(endAtSec) || t('未知') : t('不限时');
    return `${startLabel} - ${endLabel}`;
  };

  // 额度展示表头
  const quotaHeaderLabels = t('当日消耗/当日剩余/当日限额/总额度')
    .split('/')
    .map((label) => label.trim());
  const isSubscriptionInactive = (sub) => {
    const unlimitedTotal = Number(sub?.total_quota ?? 0) === 0;
    const remaining = Number(sub?.remaining_quota ?? 0);
    if (!unlimitedTotal && remaining <= 0) return true;
    const invalidAt = Number(sub?.invalid_at ?? 0);
    if (invalidAt > 0) return true;
    const expireAt = Number(sub?.expire_at ?? 0);
    return expireAt > 0 && expireAt < nowSec;
  };

  const getSubscriptionInactiveAt = (sub) => {
    const invalidAt = Number(sub?.invalid_at ?? 0);
    if (invalidAt > 0) return invalidAt;
    return Number(sub?.expire_at ?? 0);
  };

  const sortedSubscriptions = [...subscriptionsAll].sort((a, b) => {
    const aExpire = Number(a?.expire_at) || 0;
    const bExpire = Number(b?.expire_at) || 0;
    const aInactive = isSubscriptionInactive(a);
    const bInactive = isSubscriptionInactive(b);
    if (aInactive !== bInactive) return aInactive ? 1 : -1;
    if (!aInactive) {
      const aSort = aExpire === 0 ? Number.MAX_SAFE_INTEGER : aExpire;
      const bSort = bExpire === 0 ? Number.MAX_SAFE_INTEGER : bExpire;
      return aSort - bSort;
    }
    const aInactiveAt = getSubscriptionInactiveAt(a);
    const bInactiveAt = getSubscriptionInactiveAt(b);
    if (aInactiveAt !== bInactiveAt) return bInactiveAt - aInactiveAt;
    return bExpire - aExpire;
  });

  const subscriptionItemsBase = sortedSubscriptions.map((sub, idx) => {
    const id = Number(sub?.id ?? 0) || 0;
    const startAt = Number(sub.start_at) || 0;
    const expireAt = Number(sub.expire_at) || 0;
    const invalidAt = Number(sub.invalid_at) || 0;
    const unlimitedTotal = Number(sub.total_quota ?? 0) === 0;
    const remaining = Number(sub.remaining_quota) || 0;
    const isPending = startAt > nowSec;
    const isExpired =
      (!unlimitedTotal && remaining <= 0) ||
      invalidAt > 0 ||
      (expireAt > 0 && expireAt < nowSec);
    const displayEndAt = invalidAt > 0 ? invalidAt : expireAt;
    const presetName = String(sub?.source_preset_name || '').trim();
    const redemptionName = String(sub?.source_redemption_name || '').trim();
    const sourceName = presetName || redemptionName;
    const dailyLimitLabel =
      sub.daily_quota_limit > 0
        ? renderQuota(sub.daily_quota_limit)
        : t('每日额度不限');
    const dailyRemainingLabel =
      sub.daily_quota_limit > 0
        ? renderQuota(Math.max(sub.daily_quota_limit - sub.daily_quota_used, 0))
        : t('无限');
    const totalQuotaLabel = unlimitedTotal
      ? t('无限')
      : renderQuota(sub.total_quota);
    return {
      id,
      title: sourceName
        ? sourceName
        : sortedSubscriptions.length > 1
          ? t('订阅额度 {{index}}', { index: idx + 1 })
          : t('订阅额度'),
      period: formatPeriod(startAt, displayEndAt),
      isExpired,
      isPending,
      activationKind: id > 0 ? 'subscription' : '',
      maskText: isExpired ? t('已失效') : isPending ? t('待生效') : null,
      allowed_group_ids: sub.allowed_group_ids,
      group_quota_breakdown: sub.group_quota_breakdown,
      // 为订阅额度提供结构化表格数据，便于不同面板以表格形式展示
      quotaTable: {
        headers: quotaHeaderLabels,
        values: [
          renderQuota(sub.daily_quota_used),
          dailyRemainingLabel,
          dailyLimitLabel,
          totalQuotaLabel,
        ],
      },
      // 兼容旧布局的聚合值（如果有需要仍可使用）
      value: `${renderQuota(sub.daily_quota_used)} / ${dailyRemainingLabel} / ${dailyLimitLabel} / ${totalQuotaLabel}`,
      icon: <IconHistogram />,
      avatarColor: 'purple',
      trendData: [],
      trendColor: '#8b5cf6',
    };
  });

  const subscriptionItems = subscriptionItemsBase;

  const isAdminUser = isEffectiveAdmin();

  const todayTotalUsed = todayUsedQuota ?? 0;
  const todayStandardTotalUsed =
    todayStandardUsedQuota === undefined || todayStandardUsedQuota === null
      ? 0
      : todayStandardUsedQuota;
  const todayTotalTimes = todayUsedTimes ?? 0;
  const subscriptionTodayAvailableQuota = activeSubscriptions.reduce((sum, sub) => {
    if (!Number.isFinite(sum)) return sum;

    const invalidAt = Number(sub?.invalid_at ?? 0);
    if (invalidAt > 0) return sum;

    const unlimitedTotal = Number(sub?.total_quota ?? 0) === 0;
    const remainingQuota = Number(sub?.remaining_quota ?? 0);

    if (unlimitedTotal) {
      const dailyLimit = Number(sub?.daily_quota_limit ?? 0);
      const dailyUsed = Number(sub?.daily_quota_used ?? 0);
      if (dailyLimit > 0) {
        const dailyRemaining = Math.max(dailyLimit - dailyUsed, 0);
        return sum + dailyRemaining;
      }
      return Number.POSITIVE_INFINITY;
    }

    if (remainingQuota <= 0) return sum;

    if (sub.daily_quota_limit > 0) {
      const dailyRemaining = Math.max(
        (sub.daily_quota_limit ?? 0) - (sub.daily_quota_used ?? 0),
        0,
      );
      return sum + Math.min(dailyRemaining, remainingQuota);
    }

    return sum + remainingQuota;
  }, 0);
  const todayAvailableQuota = Number.isFinite(subscriptionTodayAvailableQuota)
    ? paygRemaining + subscriptionTodayAvailableQuota
    : Number.POSITIVE_INFINITY;
  const hasDailyReset = activeSubscriptions.some(
    (sub) => (sub.daily_quota_limit ?? 0) > 0,
  );

  const groupedStatsData = useMemo(() => {
    const groups = [];

    const accountItems = subscriptionItems;

    if (accountItems.length > 0) {
      groups.push({
        key: 'account',
        title: createSectionTitle(Wallet, t('账户订阅')),
        color: 'bg-blue-50',
        items: accountItems,
      });
    }

    if (isAdminUser) {
      // 管理员：保留原有「使用统计 / 资源消耗」
      const selfCacheHitRateValue = Number.isFinite(todayCacheHitRate)
        ? `${(todayCacheHitRate * 100).toFixed(2)}%`
        : '-';
      const globalCacheHitRateValue = Number.isFinite(todayGlobalCacheHitRate)
        ? `${(todayGlobalCacheHitRate * 100).toFixed(2)}%`
        : '-';

      groups.push(
        {
          key: 'usage',
          title: createSectionTitle(Activity, t('使用统计')),
          color: 'bg-green-50',
          items: [
            {
              title: t('请求次数'),
              value: userState.user?.request_count,
              icon: <IconSend />,
              avatarColor: 'green',
              trendData: [],
              trendColor: '#10b981',
            },
            {
              title: t('统计次数'),
              value: times,
              icon: <IconPulse />,
              avatarColor: 'cyan',
              trendData: trendData.times,
              trendColor: '#06b6d4',
            },
          ],
        },
        {
          key: 'cache',
          title: createSectionTitle(Gauge, t('缓存命中率')),
          color: 'bg-indigo-50',
          items: [
            {
              title: t('全站'),
              value: globalCacheHitRateValue,
              icon: <IconPulse />,
              avatarColor: 'indigo',
              trendData: [],
              trendColor: '#6366f1',
            },
            {
              title: t('我的'),
              value: selfCacheHitRateValue,
              icon: <IconPulse />,
              avatarColor: 'cyan',
              trendData: [],
              trendColor: '#06b6d4',
            },
          ],
        },
        {
          key: 'resource',
          title: createSectionTitle(Zap, t('资源消耗')),
          color: 'bg-yellow-50',
          items: [
            {
              title: t('统计额度'),
              value: renderQuota(consumeQuota),
              subtitle:
                standardConsumeQuota > 0 &&
                standardConsumeQuota !== consumeQuota
                  ? `${t('标准费用')} ${renderQuota(standardConsumeQuota)}`
                  : null,
              subtitleClassName: 'text-[13px]',
              subtitleBelow: true,
              icon: <IconCoinMoneyStroked />,
              avatarColor: 'yellow',
              trendData: trendData.consumeQuota,
              trendColor: '#f59e0b',
            },
            {
              title: t('统计Tokens'),
              value: isNaN(consumeTokens) ? 0 : consumeTokens.toLocaleString(),
              icon: <IconTextStroked />,
              avatarColor: 'pink',
              trendData: trendData.tokens,
              trendColor: '#ec4899',
            },
          ],
        },
      );
    } else {
      // 普通用户：将「使用统计」改为「今日消费」，将「资源消耗」改为「今日剩余额度」，并新增「历史消耗」
      const historyTotalConsumed = actualUsedQuota;
      const historyStandardConsumed = standardUsedQuota;
      const cacheHitRateValue = Number.isFinite(todayCacheHitRate)
        ? `${(todayCacheHitRate * 100).toFixed(2)}%`
        : '-';
      const globalCacheHitRateValue = Number.isFinite(todayGlobalCacheHitRate)
        ? `${(todayGlobalCacheHitRate * 100).toFixed(2)}%`
        : '-';

      groups.push(
        {
          key: 'usage',
          title: createSectionTitle(Activity, t('今日消费')),
          color: 'bg-green-50',
          items: [
            {
              title: t('今日消费'),
              value: renderQuota(todayTotalUsed),
              subtitle:
                todayStandardTotalUsed > 0 &&
                todayStandardTotalUsed !== todayTotalUsed
                  ? `${t('标准费用')} ${renderQuota(todayStandardTotalUsed)} · ${t('调用 {{count}} 次', { count: todayTotalTimes })}`
                  : t('调用 {{count}} 次', { count: todayTotalTimes }),
              subtitleClassName: 'text-[13px]',
              subtitleBelow: true,
              icon: <IconSend />,
              avatarColor: 'green',
              trendData: [],
              trendColor: '#10b981',
            },
          ],
        },
        {
          key: 'cache',
          title: createSectionTitle(Gauge, t('缓存命中率')),
          color: 'bg-indigo-50',
          items: [
            {
              title: t('缓存命中率'),
              value: cacheHitRateValue,
              subtitle: `${t('全站')}: ${globalCacheHitRateValue}`,
              subtitleClassName: 'text-[13px]',
              subtitleBelow: true,
              icon: <IconPulse />,
              avatarColor: 'indigo',
              trendData: [],
              trendColor: '#6366f1',
            },
          ],
        },
        {
          key: 'history',
          title: createSectionTitle(Activity, t('历史消耗')),
          color: 'bg-purple-50',
          items: [
            {
              title: t('历史消耗'),
              value: renderQuota(historyTotalConsumed),
              subtitle:
                historyStandardConsumed > 0 &&
                historyStandardConsumed !== historyTotalConsumed
                  ? `${t('标准费用')} ${renderQuota(historyStandardConsumed)}`
                  : null,
              subtitleClassName: 'text-[13px]',
              subtitleBelow: true,
              icon: <IconHistogram />,
              avatarColor: 'purple',
              trendData: [],
              trendColor: '#8b5cf6',
            },
          ],
        },
        {
          key: 'resource',
          title: createSectionTitle(Zap, t('今日剩余额度')),
          color: 'bg-yellow-50',
          items: [
            {
              title: t('今日剩余额度'),
              value: Number.isFinite(todayAvailableQuota)
                ? renderQuota(todayAvailableQuota)
                : t('无限'),
              subtitle: hasDailyReset ? t('每日0点重置') : null,
              subtitleClassName: hasDailyReset ? 'text-[13px]' : undefined,
              subtitleBelow: true,
              icon: <IconCoinMoneyStroked />,
              avatarColor: 'yellow',
              trendData: [],
              trendColor: '#f59e0b',
            },
          ],
        },
      );
    }

    if (isAdminUser) {
      // 管理员：保留性能指标
      groups.push({
        key: 'performance',
        title: createSectionTitle(Gauge, t('性能指标')),
        color: 'bg-indigo-50',
        items: [
          {
            title: t('平均RPM'),
            value: performanceMetrics.avgRPM,
            icon: <IconStopwatchStroked />,
            avatarColor: 'indigo',
            trendData: trendData.rpm,
            trendColor: '#6366f1',
          },
          {
            title: t('平均TPM'),
            value: performanceMetrics.avgTPM,
            icon: <IconTypograph />,
            avatarColor: 'orange',
            trendData: trendData.tpm,
            trendColor: '#f97316',
          },
        ],
      });
    }

    return groups;
  }, [
    consumeQuota,
    consumeTokens,
    isAdminUser,
    hasDailyReset,
    navigate,
    performanceMetrics,
    subscriptionItems,
    t,
    times,
    todayAvailableQuota,
    todayCacheHitRate,
    todayGlobalCacheHitRate,
    todayTotalTimes,
    todayTotalUsed,
    standardConsumeQuota,
    trendData,
    standardUsedQuota,
    actualUsedQuota,
    userState?.user?.quota_breakdown,
    userState?.user?.request_count,
  ]);

  return {
    groupedStatsData,
  };
};
