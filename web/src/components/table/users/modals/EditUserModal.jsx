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

import React, { useEffect, useState, useRef, useMemo } from 'react';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import {
  API,
  inferPresetMode,
  isRoot,
  showError,
  showSuccess,
  renderNumber,
  renderQuota,
  renderQuotaToUSD,
} from '../../../../helpers';
import { PRICING_PROFILE_REQUEST_VERSION } from '../../../../constants/common.constant';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import GroupRatioPill from '../../../common/ui/GroupRatioPill';
import {
  Button,
  Dropdown,
  Modal,
  Radio,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Form,
  Avatar,
  Row,
  Col,
  Input,
  InputNumber,
  Select,
  Divider,
  Empty,
} from '@douyinfe/semi-ui';
import {
  IconUser,
  IconSave,
  IconClose,
  IconLink,
  IconUserGroup,
  IconPlus,
  IconEdit,
  IconDelete,
  IconTreeTriangleDown,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

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

const normalizePositiveFactor = (rawFactor, fallback = 1) => {
  const factor = Number(rawFactor);
  if (!Number.isFinite(factor) || factor <= 0) return fallback;
  return factor;
};

const normalizePricingProfileGroupFactors = (rawFactors) => {
  const list = Array.isArray(rawFactors) ? rawFactors : [];
  const out = [];
  const seen = new Set();
  list.forEach((item) => {
    const rawGroupId = Number(item?.group_id ?? item?.groupId ?? 0);
    const groupId = Number.isFinite(rawGroupId) ? Math.floor(rawGroupId) : 0;
    if (groupId <= 0 || seen.has(groupId)) return;
    const factor = Number(item?.factor ?? 0);
    if (!Number.isFinite(factor) || factor <= 0) return;
    seen.add(groupId);
    out.push({
      group_id: groupId,
      factor,
    });
  });
  return out.sort((a, b) => a.group_id - b.group_id);
};

const normalizeResolvedGroupPricing = (rawItems) => {
  const list = Array.isArray(rawItems) ? rawItems : [];
  const out = {};
  list.forEach((item) => {
    const rawGroupId = Number(item?.group_id ?? item?.groupId ?? 0);
    const groupId = Number.isFinite(rawGroupId) ? Math.floor(rawGroupId) : 0;
    if (groupId <= 0) return;
    const publicRatio = Number(item?.public_ratio ?? 1);
    const normalizedPublicRatio =
      Number.isFinite(publicRatio) && publicRatio >= 0 ? publicRatio : 1;
    const effectiveRatio = Number(
      item?.effective_ratio ?? normalizedPublicRatio,
    );
    const normalizedEffectiveRatio =
      Number.isFinite(effectiveRatio) && effectiveRatio >= 0
        ? effectiveRatio
        : normalizedPublicRatio;
    const appliedFactor = Number(item?.applied_factor ?? 0);
    const normalizedAppliedFactor =
      Number.isFinite(appliedFactor) && appliedFactor >= 0
        ? appliedFactor
        : normalizedPublicRatio > 0
          ? normalizedEffectiveRatio / normalizedPublicRatio
          : normalizedEffectiveRatio > 0
            ? 1
            : 0;
    out[groupId] = {
      group_id: groupId,
      public_ratio: normalizedPublicRatio,
      effective_ratio: normalizedEffectiveRatio,
      applied_factor: normalizedAppliedFactor,
      source:
        String(item?.source || 'public')
          .trim()
          .toLowerCase() || 'public',
      pricing_profile_id:
        Number(item?.pricing_profile_id ?? item?.pricingProfileId ?? 0) || 0,
    };
  });
  return out;
};

const buildUserPricingPreview = ({
  values,
  pricingProfiles,
  groupPriceOverrides,
  publicGroupRatios,
  savedResolvedGroupPricing,
}) => {
  const profileIdRaw = Number(values?.pricing_profile_id ?? 0);
  const pricingProfileId = Number.isFinite(profileIdRaw)
    ? Math.max(0, Math.floor(profileIdRaw))
    : 0;
  const selectedProfile =
    (Array.isArray(pricingProfiles) ? pricingProfiles : []).find(
      (item) => Number(item?.id ?? 0) === pricingProfileId,
    ) || null;
  const baseMultiplier = normalizePositiveFactor(values?.base_multiplier, 1);
  const templateGroupFactors = normalizePricingProfileGroupFactors(
    selectedProfile?.group_factors,
  );
  const templateFactorByGroup = new Map(
    templateGroupFactors.map((item) => [item.group_id, item.factor]),
  );
  const userOverrides = normalizeGroupPriceOverrides(groupPriceOverrides);
  const overrideFactorByGroup = new Map(
    userOverrides.map((item) => [item.group_id, item.factor]),
  );
  const savedMap =
    savedResolvedGroupPricing && typeof savedResolvedGroupPricing === 'object'
      ? savedResolvedGroupPricing
      : {};
  const groupRatioMap =
    publicGroupRatios && typeof publicGroupRatios === 'object'
      ? publicGroupRatios
      : {};
  const hasIncompleteProfileDraft = pricingProfileId > 0 && !selectedProfile;

  const hasDraftNewRule =
    pricingProfileId > 0 || baseMultiplier !== 1 || userOverrides.length > 0;
  const profileDefaultFactor =
    pricingProfileId > 0
      ? normalizePositiveFactor(selectedProfile?.default_factor, 1)
      : 1;
  const effectiveDefaultFactor =
    pricingProfileId > 0
      ? profileDefaultFactor * baseMultiplier
      : baseMultiplier;

  const groupIdSet = new Set();
  Object.keys(groupRatioMap).forEach((key) => {
    const groupId = Number(key);
    if (Number.isFinite(groupId) && groupId > 0) groupIdSet.add(groupId);
  });
  Object.keys(savedMap).forEach((key) => {
    const groupId = Number(key);
    if (Number.isFinite(groupId) && groupId > 0) groupIdSet.add(groupId);
  });
  templateGroupFactors.forEach((item) => {
    if (item.group_id > 0) groupIdSet.add(item.group_id);
  });
  userOverrides.forEach((item) => {
    if (item.group_id > 0) groupIdSet.add(item.group_id);
  });

  const resolvedGroupRatios = {};
  const resolvedGroupSources = {};
  const sortedGroupIds = Array.from(groupIdSet).sort((a, b) => a - b);
  sortedGroupIds.forEach((groupId) => {
    const savedItem = savedMap[groupId];
    const savedSource = String(savedItem?.source || '')
      .trim()
      .toLowerCase();
    const publicRatioRaw =
      groupRatioMap[groupId] ??
      savedItem?.public_ratio ??
      savedItem?.effective_ratio ??
      1;
    const publicRatio =
      Number.isFinite(Number(publicRatioRaw)) && Number(publicRatioRaw) >= 0
        ? Number(publicRatioRaw)
        : 1;

    if (hasDraftNewRule) {
      if (overrideFactorByGroup.has(groupId)) {
        resolvedGroupRatios[groupId] = overrideFactorByGroup.get(groupId);
        resolvedGroupSources[groupId] = 'override';
        return;
      }
      if (
        !hasIncompleteProfileDraft &&
        pricingProfileId > 0 &&
        templateFactorByGroup.has(groupId)
      ) {
        resolvedGroupRatios[groupId] = templateFactorByGroup.get(groupId);
        resolvedGroupSources[groupId] = 'profile';
        return;
      }
      if (!hasIncompleteProfileDraft && pricingProfileId > 0) {
        resolvedGroupRatios[groupId] = publicRatio * effectiveDefaultFactor;
        resolvedGroupSources[groupId] =
          profileDefaultFactor === 1 && baseMultiplier === 1
            ? 'public'
            : 'profile';
        return;
      }
      if (pricingProfileId <= 0 && baseMultiplier !== 1) {
        if (
          savedSource === 'legacy' &&
          Number.isFinite(Number(savedItem?.effective_ratio)) &&
          Number(savedItem?.effective_ratio) >= 0
        ) {
          resolvedGroupRatios[groupId] = Number(savedItem.effective_ratio);
          resolvedGroupSources[groupId] = 'legacy';
          return;
        }
        resolvedGroupRatios[groupId] = publicRatio * baseMultiplier;
        resolvedGroupSources[groupId] = 'base_multiplier';
        return;
      }
    }

    resolvedGroupRatios[groupId] = savedItem?.effective_ratio ?? publicRatio;
    resolvedGroupSources[groupId] = savedItem?.source || 'public';
  });

  return {
    selectedProfile,
    templateGroupFactors,
    userOverrides,
    baseMultiplier,
    profileDefaultFactor,
    effectiveDefaultFactor,
    resolvedGroupRatios,
    resolvedGroupSources,
  };
};

const getPricingSourceTagProps = (source, t) => {
  switch (
    String(source || '')
      .trim()
      .toLowerCase()
  ) {
    case 'override':
      return { color: 'red', label: t('覆写') };
    case 'profile':
      return { color: 'blue', label: t('模板') };
    case 'base_multiplier':
      return { color: 'cyan', label: t('默认') };
    case 'legacy':
      return { color: 'orange', label: t('兼容') };
    default:
      return { color: 'grey', label: t('公开') };
  }
};

const normalizeGroupPriceOverrides = (rawOverrides) => {
  const list = Array.isArray(rawOverrides) ? rawOverrides : [];
  const out = [];
  const seen = new Set();
  list.forEach((item) => {
    const rawGroupId = Number(item?.group_id ?? item?.groupId ?? 0);
    const groupId = Number.isFinite(rawGroupId) ? Math.floor(rawGroupId) : 0;
    if (groupId <= 0 || seen.has(groupId)) return;

    const factor = Number(item?.factor ?? 0);
    if (!Number.isFinite(factor) || factor <= 0) return;

    seen.add(groupId);
    out.push({
      group_id: groupId,
      factor,
    });
  });
  return out.sort((a, b) => a.group_id - b.group_id);
};

const normalizePaygProducts = (rawProducts) => {
  const list = Array.isArray(rawProducts) ? rawProducts : [];
  return list
    .map((item) => {
      const id = Number(item?.id ?? 0);
      if (!Number.isFinite(id) || id <= 0) return null;
      const name = String(item?.name ?? '').trim();
      if (!name) return null;
      const description = String(item?.description ?? '').trim();
      const sortOrderRaw = Number(item?.sort_order ?? 0);
      const sortOrder = Number.isFinite(sortOrderRaw)
        ? Math.max(0, Math.floor(sortOrderRaw))
        : 0;
      const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);
      if (allowedGroupIds.length === 0) return null;

      return {
        id,
        name,
        description,
        sort_order: sortOrder,
        enabled: item?.enabled !== false,
        allowed_group_ids: allowedGroupIds,
      };
    })
    .filter(Boolean);
};

const normalizeGroupDailyLimits = (rawLimits) => {
  const list = Array.isArray(rawLimits) ? rawLimits : [];
  const out = [];
  const seen = new Set();
  list.forEach((item) => {
    const rawId = Number(item?.group_id ?? 0);
    const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
    if (groupId <= 0) return;
    if (seen.has(groupId)) return;
    seen.add(groupId);
    const dailyLimit = Number(item?.daily_quota_limit ?? 0);
    out.push({
      group_id: groupId,
      daily_quota_limit: Number.isFinite(dailyLimit)
        ? Math.max(0, Math.floor(dailyLimit))
        : 0,
    });
  });
  return out.sort((a, b) => a.group_id - b.group_id);
};

const normalizeGroupDailyLimitsUSD = (rawLimits) => {
  const list = Array.isArray(rawLimits) ? rawLimits : [];
  const out = [];
  const seen = new Set();
  list.forEach((item) => {
    const rawId = Number(item?.group_id ?? 0);
    const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
    if (groupId <= 0) return;
    if (seen.has(groupId)) return;
    seen.add(groupId);
    const dailyLimitRaw = Number(item?.daily_quota_limit_usd ?? 0);
    out.push({
      group_id: groupId,
      daily_quota_limit_usd: Number.isFinite(dailyLimitRaw)
        ? Math.max(0, dailyLimitRaw)
        : 0,
    });
  });
  return out.sort((a, b) => a.group_id - b.group_id);
};

const syncGroupDailyLimitsUSD = (allowedGroups, val) => {
  const allowed = normalizeGroupIds(allowedGroups);
  const existing = normalizeGroupDailyLimitsUSD(val);
  const map = new Map(
    existing.map((item) => [item.group_id, item.daily_quota_limit_usd]),
  );
  return allowed.map((groupId) => ({
    group_id: groupId,
    daily_quota_limit_usd: map.get(groupId) ?? 0,
  }));
};

const normalizeOrderedUniquePositiveIds = (rawIds) => {
  if (!Array.isArray(rawIds)) return [];
  const seen = new Set();
  const out = [];
  rawIds.forEach((raw) => {
    const num = Number(raw);
    if (!Number.isFinite(num) || num <= 0) return;
    const id = Math.trunc(num);
    if (id <= 0 || seen.has(id)) return;
    seen.add(id);
    out.push(id);
  });
  return out;
};

const normalizeOrderedUniqueNonZeroIds = (rawIds) => {
  if (!Array.isArray(rawIds)) return [];
  const seen = new Set();
  const out = [];
  rawIds.forEach((raw) => {
    const num = Number(raw);
    if (!Number.isFinite(num) || num === 0) return;
    const id = Math.trunc(num);
    if (id === 0 || seen.has(id)) return;
    seen.add(id);
    out.push(id);
  });
  return out;
};

const reorderIdsBefore = (ids, sourceId, targetId) => {
  const current = Array.isArray(ids) ? [...ids] : [];
  const source = Number(sourceId);
  const target = Number(targetId);
  if (
    !Number.isFinite(source) ||
    !Number.isFinite(target) ||
    source === 0 ||
    target === 0
  ) {
    return current;
  }
  if (source === target) return current;
  const next = current.filter((item) => item !== source);
  if (next.length === current.length) return current;
  const targetIndex = next.findIndex((item) => item === target);
  if (targetIndex < 0) return current;
  next.splice(targetIndex, 0, source);
  return next;
};

const getBillingStatusBucket = (
  record,
  nowUnix = Math.floor(Date.now() / 1000),
) => {
  const startAt = Number(record?.start_at ?? 0) || 0;
  const expireAt = Number(record?.expire_at ?? 0) || 0;
  if (expireAt > 0 && expireAt < nowUnix) return 'expired';
  if (startAt > nowUnix) return 'pending';
  return 'active';
};

const pricingProfileRequestConfig = {
  disableDuplicate: true,
  params: {
    _v: PRICING_PROFILE_REQUEST_VERSION,
  },
};

const EditUserModal = (props) => {
  const { t } = useTranslation();
  const userId = props.editingUser.id;
  const isRootUser = isRoot();
  const [subscriptionFormKey, setSubscriptionFormKey] = useState(0);
  const isEdit = Boolean(userId);
  const [loading, setLoading] = useState(true);
  const isMobile = useIsMobile();
  const [groupIdOptions, setGroupIdOptions] = useState([]);
  const [userGroupIdOptions, setUserGroupIdOptions] = useState([]);
  const [groupOptionsLoading, setGroupOptionsLoading] = useState(false);
  const [userGroupOptionsLoading, setUserGroupOptionsLoading] = useState(false);
  const [groupRatios, setGroupRatios] = useState({});
  const [pricingProfiles, setPricingProfiles] = useState([]);
  const [profilesLoading, setProfilesLoading] = useState(false);
  const [pricingFormValues, setPricingFormValues] = useState({
    customer_type: 'retail',
    pricing_profile_id: 0,
    base_multiplier: 1,
    user_group_id: 0,
  });
  const [groupPriceOverrides, setGroupPriceOverrides] = useState([]);
  const [resolvedGroupPricing, setResolvedGroupPricing] = useState({});
  const [paygProductsLoading, setPaygProductsLoading] = useState(false);
  const [paygProducts, setPaygProducts] = useState([]);
  const [userDetail, setUserDetail] = useState(null);
  const formApiRef = useRef(null);

  const groupLabelById = useMemo(() => {
    const map = {};
    (Array.isArray(groupIdOptions) ? groupIdOptions : []).forEach((opt) => {
      const idRaw = Number(opt?.value ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return;
      const label = String(opt?.label || '').trim();
      const code = String(opt?.code || '').trim();
      const name = label || code;
      if (!name) return;
      map[id] = name;
    });
    return map;
  }, [groupIdOptions]);

  const [subscriptions, setSubscriptions] = useState([]);
  const [subscriptionSummary, setSubscriptionSummary] = useState(null);
  const [subscriptionRemaining, setSubscriptionRemaining] = useState(0);
  const [subscriptionConfigErrors, setSubscriptionConfigErrors] = useState([]);
  const [subscriptionsLoading, setSubscriptionsLoading] = useState(false);
  const [paygRemaining, setPaygRemaining] = useState(0);
  const [subscriptionModalVisible, setSubscriptionModalVisible] =
    useState(false);
  const [editingSubscription, setEditingSubscription] = useState(null);
  const [subscriptionSubmitting, setSubscriptionSubmitting] = useState(false);
  const [subscriptionInitialValues, setSubscriptionInitialValues] = useState({
    quota_usd: 0,
    remaining_quota_usd: 0,
    daily_quota_limit_usd: 0,
    allowed_group_ids: [],
    use_group_daily_limits: false,
    group_daily_limits: [],
    start_at: dayjs().startOf('day').toDate(),
    expire_at: dayjs().add(30, 'day').endOf('day').toDate(),
    source: '',
  });
  const subscriptionFormApiRef = useRef(null);

  const [subscriptionPresetModalVisible, setSubscriptionPresetModalVisible] =
    useState(false);
  const [subscriptionPresetsLoading, setSubscriptionPresetsLoading] =
    useState(false);
  const [subscriptionPresets, setSubscriptionPresets] = useState([]);
  const [subscriptionPresetSubmitting, setSubscriptionPresetSubmitting] =
    useState(false);
  const [subscriptionPresetFormKey, setSubscriptionPresetFormKey] = useState(0);
  const subscriptionPresetFormApiRef = useRef(null);
  const [subscriptionPresetInitValues, setSubscriptionPresetInitValues] =
    useState({
      preset_id: undefined,
      apply_mode: 'stack',
      quantity: 1,
    });

  const [requestSubscriptions, setRequestSubscriptions] = useState([]);
  const [requestSubscriptionSummary, setRequestSubscriptionSummary] =
    useState(null);
  const [requestSubscriptionsLoading, setRequestSubscriptionsLoading] =
    useState(false);
  const [requestSubscriptionModalVisible, setRequestSubscriptionModalVisible] =
    useState(false);
  const [editingRequestSubscription, setEditingRequestSubscription] =
    useState(null);
  const [requestSubscriptionSubmitting, setRequestSubscriptionSubmitting] =
    useState(false);
  const [requestSubscriptionFormKey, setRequestSubscriptionFormKey] =
    useState(0);
  const [
    requestSubscriptionInitialValues,
    setRequestSubscriptionInitialValues,
  ] = useState({
    daily_request_limit: 0,
    total_request_limit: 0,
    allowed_group_ids: [],
    start_at: dayjs().startOf('day').toDate(),
    expire_at: dayjs().add(30, 'day').endOf('day').toDate(),
    source: '',
  });
  const requestSubscriptionFormApiRef = useRef(null);

  const [
    requestSubscriptionPresetModalVisible,
    setRequestSubscriptionPresetModalVisible,
  ] = useState(false);
  const [requestSubscriptionPresets, setRequestSubscriptionPresets] = useState(
    [],
  );
  const [
    requestSubscriptionPresetSubmitting,
    setRequestSubscriptionPresetSubmitting,
  ] = useState(false);
  const [
    requestSubscriptionPresetFormKey,
    setRequestSubscriptionPresetFormKey,
  ] = useState(0);
  const requestSubscriptionPresetFormApiRef = useRef(null);
  const [
    requestSubscriptionPresetInitValues,
    setRequestSubscriptionPresetInitValues,
  ] = useState({
    preset_id: undefined,
    apply_mode: 'stack',
    quantity: 1,
  });

  const [paygTopupModalVisible, setPaygTopupModalVisible] = useState(false);
  const [paygTopupSubmitting, setPaygTopupSubmitting] = useState(false);
  const [paygTopupFormKey, setPaygTopupFormKey] = useState(0);
  const paygTopupFormApiRef = useRef(null);
  const [paygTopupInitValues, setPaygTopupInitValues] = useState({
    product_id: undefined,
    quota_usd: 1,
  });
  const [paygManualModalVisible, setPaygManualModalVisible] = useState(false);
  const [paygManualSubmitting, setPaygManualSubmitting] = useState(false);
  const [paygManualFormKey, setPaygManualFormKey] = useState(0);
  const paygManualFormApiRef = useRef(null);
  const [paygManualInitValues, setPaygManualInitValues] = useState({
    group_id: undefined,
    quota_usd: 1,
  });
  const [paygBalanceGroupModalVisible, setPaygBalanceGroupModalVisible] =
    useState(false);
  const [paygBalanceGroupSubmitting, setPaygBalanceGroupSubmitting] =
    useState(false);
  const [paygBalanceGroupFormKey, setPaygBalanceGroupFormKey] = useState(0);
  const paygBalanceGroupFormApiRef = useRef(null);
  const [editingPaygBalance, setEditingPaygBalance] = useState(null);
  const [paygBalanceGroupInitValues, setPaygBalanceGroupInitValues] = useState({
    allowed_group_ids: [],
  });
  const [subscriptionDraggingId, setSubscriptionDraggingId] = useState(0);
  const [subscriptionDropTargetId, setSubscriptionDropTargetId] = useState(0);
  const [subscriptionReordering, setSubscriptionReordering] = useState(false);
  const [requestSubscriptionDraggingId, setRequestSubscriptionDraggingId] =
    useState(0);
  const [requestSubscriptionDropTargetId, setRequestSubscriptionDropTargetId] =
    useState(0);
  const [requestSubscriptionReordering, setRequestSubscriptionReordering] =
    useState(false);
  const [paygDraggingProductId, setPaygDraggingProductId] = useState(0);
  const [paygDropTargetProductId, setPaygDropTargetProductId] = useState(0);
  const [paygReordering, setPaygReordering] = useState(false);

  const DEFAULT_QUOTA_PER_USD = 500000;

  const getQuotaPerUnit = () => {
    if (typeof window === 'undefined') {
      return DEFAULT_QUOTA_PER_USD;
    }
    const raw = window.localStorage.getItem('quota_per_unit');
    const parsed = parseFloat(raw);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      return DEFAULT_QUOTA_PER_USD;
    }
    return parsed;
  };

  const convertQuotaToUSD = (quotaValue) => {
    const quotaNumber = Number(quotaValue);
    if (!Number.isFinite(quotaNumber) || quotaNumber <= 0) {
      return 0;
    }
    const perUnit = getQuotaPerUnit();
    if (!perUnit) {
      return 0;
    }
    return Number((quotaNumber / perUnit).toFixed(6));
  };

  const convertUSDToQuota = (usdValue) => {
    const usdNumber = Number(usdValue);
    if (!Number.isFinite(usdNumber) || usdNumber <= 0) {
      return 0;
    }
    const perUnit = getQuotaPerUnit();
    if (!perUnit) {
      return 0;
    }
    return Math.round(usdNumber * perUnit);
  };

  const buildSubscriptionFormValues = (subscription) => {
    const startDay = subscription?.start_at
      ? dayjs(subscription.start_at * 1000)
      : dayjs();
    const expireDay = subscription
      ? subscription.expire_at && subscription.expire_at > 0
        ? dayjs(subscription.expire_at * 1000)
        : null
      : startDay.clone().add(30, 'day').endOf('day');

    const quotaTokens = subscription ? subscription.total_quota : 0;
    const remainingTokens = subscription ? subscription.remaining_quota : 0;
    const dailyLimitTokens =
      subscription?.daily_quota_limit > 0 ? subscription.daily_quota_limit : 0;

    const allowedGroupIds = normalizeGroupIds(subscription?.allowed_group_ids);
    const groupDailyLimitsTokens = normalizeGroupDailyLimits(
      subscription?.group_daily_limits,
    );
    const useGroupDailyLimits = groupDailyLimitsTokens.length > 0;
    const groupDailyLimitsUSD = groupDailyLimitsTokens.map((item) => ({
      group_id: item.group_id,
      daily_quota_limit_usd: convertQuotaToUSD(item.daily_quota_limit),
    }));

    return {
      quota_usd: convertQuotaToUSD(quotaTokens),
      remaining_quota_usd: convertQuotaToUSD(remainingTokens),
      daily_quota_limit_usd: convertQuotaToUSD(dailyLimitTokens),
      allowed_group_ids: allowedGroupIds,
      use_group_daily_limits: useGroupDailyLimits,
      group_daily_limits: useGroupDailyLimits
        ? syncGroupDailyLimitsUSD(allowedGroupIds, groupDailyLimitsUSD)
        : [],
      start_at: startDay.toDate(),
      expire_at: expireDay ? expireDay.toDate() : null,
      source: subscription?.source || '',
    };
  };

  const buildRequestSubscriptionFormValues = (subscription) => {
    const startDay = subscription?.start_at
      ? dayjs(subscription.start_at * 1000)
      : dayjs();
    const expireDay = subscription
      ? subscription.expire_at && subscription.expire_at > 0
        ? dayjs(subscription.expire_at * 1000)
        : null
      : startDay.clone().add(30, 'day').endOf('day');

    return {
      daily_request_limit: Number(subscription?.daily_request_limit ?? 0) || 0,
      total_request_limit: Number(subscription?.total_request_limit ?? 0) || 0,
      allowed_group_ids: normalizeGroupIds(subscription?.allowed_group_ids),
      start_at: startDay.toDate(),
      expire_at: expireDay ? expireDay.toDate() : null,
      source: subscription?.source || '',
    };
  };

  const getInitValues = () => ({
    username: '',
    display_name: '',
    password: '',
    aff_code: '',
    aff_count: 0,
    inviter_id: 0,
    github_id: '',
    oidc_id: '',
    wechat_id: '',
    telegram_id: '',
    email: '',
    base_multiplier: 1,
    customer_type: 'retail',
    pricing_profile_id: 0,
    group_id: 0,
    user_group_id: 0,
    remark: '',
    admin_permissions: {
      product_management: false,
      order: false,
    },
  });

  const editingTargetRole = Number(
    userDetail?.role ?? props.editingUser?.role ?? 0,
  );
  const canEditAdminPermissions =
    isRootUser && editingTargetRole === 10 && editingTargetRole < 100;

  useEffect(() => {
    if (!subscriptionModalVisible) return;
    if (!subscriptionFormApiRef.current) return;
    subscriptionFormApiRef.current.setValues(subscriptionInitialValues);
  }, [subscriptionModalVisible, subscriptionInitialValues]);

  useEffect(() => {
    if (!subscriptionPresetModalVisible) return;
    if (!subscriptionPresetFormApiRef.current) return;
    subscriptionPresetFormApiRef.current.setValues(
      subscriptionPresetInitValues,
    );
  }, [subscriptionPresetModalVisible, subscriptionPresetInitValues]);

  useEffect(() => {
    if (!requestSubscriptionModalVisible) return;
    if (!requestSubscriptionFormApiRef.current) return;
    requestSubscriptionFormApiRef.current.setValues(
      requestSubscriptionInitialValues,
    );
  }, [requestSubscriptionInitialValues, requestSubscriptionModalVisible]);

  useEffect(() => {
    if (!requestSubscriptionPresetModalVisible) return;
    if (!requestSubscriptionPresetFormApiRef.current) return;
    requestSubscriptionPresetFormApiRef.current.setValues(
      requestSubscriptionPresetInitValues,
    );
  }, [
    requestSubscriptionPresetInitValues,
    requestSubscriptionPresetModalVisible,
  ]);

  useEffect(() => {
    if (!paygTopupModalVisible) return;
    if (!paygTopupFormApiRef.current) return;
    paygTopupFormApiRef.current.setValues(paygTopupInitValues);
  }, [paygTopupModalVisible, paygTopupInitValues]);

  useEffect(() => {
    if (!paygManualModalVisible) return;
    if (!paygManualFormApiRef.current) return;
    paygManualFormApiRef.current.setValues(paygManualInitValues);
  }, [paygManualInitValues, paygManualModalVisible]);

  useEffect(() => {
    if (!paygBalanceGroupModalVisible) return;
    if (!paygBalanceGroupFormApiRef.current) return;
    paygBalanceGroupFormApiRef.current.setValues(paygBalanceGroupInitValues);
  }, [paygBalanceGroupInitValues, paygBalanceGroupModalVisible]);

  const applySubscriptionBreakdown = (breakdown) => {
    if (!breakdown) {
      setSubscriptions([]);
      setSubscriptionSummary(null);
      setSubscriptionRemaining(0);
      setSubscriptionConfigErrors([]);
      setPaygRemaining(0);
      return;
    }
    const configErrors = Array.isArray(breakdown.subscription_config_errors)
      ? breakdown.subscription_config_errors.filter(Boolean)
      : [];
    const rawList =
      (Array.isArray(breakdown.subscriptions_all) &&
      breakdown.subscriptions_all.length > 0
        ? breakdown.subscriptions_all
        : breakdown.subscriptions) || [];
    const list = rawList.map((item, index) => ({
      ...item,
      index: index + 1,
    }));
    setSubscriptions(list);
    setSubscriptionSummary({
      total: breakdown.subscription_total || 0,
      consumed: breakdown.subscription_consumed || 0,
      remaining: breakdown.subscription_remaining || 0,
      dailyUsed: breakdown.subscription_daily_used || 0,
      dailyLimit: breakdown.subscription_daily_limit || 0,
      unlimited: breakdown.subscription_daily_limit_unlimited || false,
      windowStart: breakdown.subscription_window_start || 0,
      windowEnd: breakdown.subscription_window_end || 0,
    });
    const paygRemaining = breakdown.payg_remaining || 0;
    setPaygRemaining(paygRemaining);
    setSubscriptionRemaining(breakdown.subscription_remaining || 0);
    setSubscriptionConfigErrors(configErrors);
  };

  const applyRequestSubscriptionBreakdown = (breakdown) => {
    if (!breakdown) {
      setRequestSubscriptions([]);
      setRequestSubscriptionSummary(null);
      return;
    }
    const rawList = Array.isArray(breakdown.subscriptions)
      ? breakdown.subscriptions
      : [];
    const list = rawList.map((item, index) => ({
      ...item,
      index: index + 1,
    }));
    setRequestSubscriptions(list);
    setRequestSubscriptionSummary({
      todayDate: Number(breakdown.today_date) || 0,
      limitTotal: Number(breakdown.daily_request_limit_total) || 0,
      usedTotal: Number(breakdown.daily_request_used_total) || 0,
      remainingTotal: Number(breakdown.daily_request_remaining_total) || 0,
      limitUnlimited: Boolean(breakdown.daily_request_limit_unlimited_total),
      totalLimitTotal: Number(breakdown.total_request_limit_total) || 0,
      totalUsedTotal: Number(breakdown.total_request_used_total) || 0,
      totalRemainingTotal: Number(breakdown.total_request_remaining_total) || 0,
      totalUnlimited: Boolean(breakdown.total_request_limit_unlimited_total),
    });
  };

  const formatDateLabel = (value) => {
    if (!value) return '';
    if (value instanceof Date) {
      if (Number.isNaN(value.getTime())) return '';
      const year = value.getFullYear();
      const month = `${value.getMonth() + 1}`.padStart(2, '0');
      const day = `${value.getDate()}`.padStart(2, '0');
      const hours = `${value.getHours()}`.padStart(2, '0');
      const minutes = `${value.getMinutes()}`.padStart(2, '0');
      const seconds = `${value.getSeconds()}`.padStart(2, '0');
      return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
    }
    const numeric = Number(value);
    if (!numeric || Number.isNaN(numeric)) return '';
    const date = new Date(numeric * 1000);
    if (Number.isNaN(date.getTime())) return '';
    const year = date.getFullYear();
    const month = `${date.getMonth() + 1}`.padStart(2, '0');
    const day = `${date.getDate()}`.padStart(2, '0');
    const hours = `${date.getHours()}`.padStart(2, '0');
    const minutes = `${date.getMinutes()}`.padStart(2, '0');
    const seconds = `${date.getSeconds()}`.padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
  };

  const fetchGroups = async () => {
    setGroupOptionsLoading(true);
    try {
      const res = await API.get(`/api/group/`);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        setGroupIdOptions([]);
        setGroupRatios({});
        return [];
      }
      const list = Array.isArray(data) ? data : [];
      const ratioMap = {};
      const idOptions = list
        .map((g) => {
          const rawId = Number(g?.id ?? 0);
          const id = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
          if (id <= 0) return null;
          const rawRatio = Number(g?.ratio ?? 1);
          ratioMap[id] =
            Number.isFinite(rawRatio) && rawRatio >= 0 ? rawRatio : 1;
          const code = String(g?.code || '').trim();
          const displayName = String(g?.display_name || g?.name || '').trim();
          const label = displayName || code;
          if (!label) return null;
          return {
            label,
            value: id,
            code,
          };
        })
        .filter(Boolean)
        .sort((a, b) => Number(a.value) - Number(b.value));

      setGroupIdOptions(idOptions);
      setGroupRatios(ratioMap);
      return idOptions;
    } catch (e) {
      showError(e.message);
      setGroupIdOptions([]);
      setGroupRatios({});
      return [];
    } finally {
      setGroupOptionsLoading(false);
    }
  };

  const fetchUserGroups = async () => {
    setUserGroupOptionsLoading(true);
    try {
      const res = await API.get('/api/user_group/');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取用户分组失败'));
        setUserGroupIdOptions([]);
        return [];
      }
      const options = (Array.isArray(data) ? data : [])
        .map((item) => {
          const rawId = Number(item?.id ?? 0);
          const id = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
          const code = String(item?.code || '').trim();
          const label = String(item?.name || code).trim();
          if (id <= 0 || !label) return null;
          return { label, value: id, code };
        })
        .filter(Boolean)
        .sort((a, b) => Number(a.value) - Number(b.value));
      setUserGroupIdOptions(options);
      return options;
    } catch (error) {
      showError(error?.message || t('获取用户分组失败'));
      setUserGroupIdOptions([]);
      return [];
    } finally {
      setUserGroupOptionsLoading(false);
    }
  };

  const loadPaygProducts = async () => {
    setPaygProductsLoading(true);
    try {
      const res = await API.get('/api/user/topup/info', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取按量付费商品失败'));
        setPaygProducts([]);
        return [];
      }
      const normalized = normalizePaygProducts(data?.payg_products);
      setPaygProducts(normalized);
      return normalized;
    } catch (e) {
      showError(e?.message || t('获取按量付费商品失败'));
      setPaygProducts([]);
      return [];
    } finally {
      setPaygProductsLoading(false);
    }
  };

  const loadPricingProfiles = async () => {
    setProfilesLoading(true);
    try {
      const res = await API.get(
        '/api/pricing_profiles/',
        pricingProfileRequestConfig,
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取价格模板失败'));
        setPricingProfiles([]);
        return [];
      }
      const list = Array.isArray(data) ? data : [];
      setPricingProfiles(list);
      return list;
    } catch (e) {
      showError(e?.message || t('获取价格模板失败'));
      setPricingProfiles([]);
      return [];
    } finally {
      setProfilesLoading(false);
    }
  };

  const getPricingProfileOptions = (customerType, currentProfileId = 0) => {
    const normalizedType = String(customerType || 'retail')
      .trim()
      .toLowerCase();
    const selectedProfileId = Number(currentProfileId || 0);
    return (Array.isArray(pricingProfiles) ? pricingProfiles : [])
      .filter((item) => {
        const id = Number(item?.id ?? 0);
        if (id <= 0) return false;
        if (id === selectedProfileId) return true;
        if (item?.enabled === false) return false;
        const audience = String(item?.audience || '')
          .trim()
          .toLowerCase();
        return !audience || audience === normalizedType;
      })
      .map((item) => {
        const id = Number(item?.id ?? 0);
        const baseLabel = String(item?.name || item?.code || '').trim();
        if (!baseLabel || id <= 0) return null;
        return {
          label:
            item?.enabled === false
              ? `${baseLabel} (${t('已停用')})`
              : baseLabel,
          value: id,
        };
      })
      .filter(Boolean);
  };

  const pricingPreview = useMemo(
    () =>
      buildUserPricingPreview({
        values: pricingFormValues,
        pricingProfiles,
        groupPriceOverrides,
        publicGroupRatios: groupRatios,
        savedResolvedGroupPricing: resolvedGroupPricing,
      }),
    [
      pricingFormValues,
      pricingProfiles,
      groupPriceOverrides,
      groupRatios,
      resolvedGroupPricing,
    ],
  );

  const selectedPricingProfile = pricingPreview.selectedProfile;
  const templateGroupFactors = pricingPreview.templateGroupFactors;
  const effectiveGroupRatios = pricingPreview.resolvedGroupRatios;
  const effectiveGroupSources = pricingPreview.resolvedGroupSources;

  const formatGroupRatioLabel = (groupId) => {
    const rawGroupId = Number(groupId);
    const normalizedGroupId = Number.isFinite(rawGroupId)
      ? Math.floor(rawGroupId)
      : 0;
    const label = groupLabelById[normalizedGroupId] || t('未知分组');
    const ratio =
      effectiveGroupRatios?.[normalizedGroupId] ??
      groupRatios?.[normalizedGroupId];
    return <GroupRatioPill label={label} ratio={ratio} />;
  };

  const applyLoadedUserFormData = (data) => {
    if (!data) {
      setUserDetail(null);
      setPricingFormValues({
        customer_type: 'retail',
        pricing_profile_id: 0,
        base_multiplier: 1,
      });
      setGroupPriceOverrides([]);
      setResolvedGroupPricing({});
      return;
    }
    data.password = '';
    setUserDetail(data);
    setGroupPriceOverrides(
      normalizeGroupPriceOverrides(data?.group_price_overrides),
    );
    setResolvedGroupPricing(
      normalizeResolvedGroupPricing(data?.resolved_group_pricing),
    );
    const adminPermissions =
      data?.admin_permissions && typeof data.admin_permissions === 'object'
        ? data.admin_permissions
        : {};
    const initValues = {
      ...getInitValues(),
      ...data,
      admin_permissions: {
        ...getInitValues().admin_permissions,
        ...adminPermissions,
      },
      daily_quota_limit: data.daily_quota_limit ?? 0,
      customer_type: data?.customer_type || 'retail',
      pricing_profile_id: Number(data?.pricing_profile_id ?? 0) || 0,
      base_multiplier:
        data.base_multiplier && data.base_multiplier > 0
          ? data.base_multiplier
          : 1,
      user_group_id: Number(data?.user_group_id ?? 0) || 0,
    };
    setPricingFormValues({
      customer_type: initValues.customer_type,
      pricing_profile_id: initValues.pricing_profile_id,
      base_multiplier: initValues.base_multiplier,
      user_group_id: initValues.user_group_id,
    });
    formApiRef.current?.setValues(initValues);
  };

  const applyLoadedUserSnapshot = (data) => {
    if (!data) {
      setUserDetail(null);
      setResolvedGroupPricing({});
      return;
    }
    setUserDetail(data);
    setResolvedGroupPricing(
      normalizeResolvedGroupPricing(data?.resolved_group_pricing),
    );
  };

  const addGroupPriceOverride = () => {
    setGroupPriceOverrides((prev) => [
      ...(Array.isArray(prev) ? prev : []),
      { group_id: 0, factor: 1 },
    ]);
  };

  const updateGroupPriceOverride = (index, key, value) => {
    setGroupPriceOverrides((prev) =>
      (Array.isArray(prev) ? prev : []).map((item, itemIndex) => {
        if (itemIndex !== index) return item;
        if (key === 'group_id') {
          const raw = Number(value ?? 0);
          return {
            ...item,
            group_id: Number.isFinite(raw) ? Math.floor(raw) : 0,
          };
        }
        return {
          ...item,
          factor: value,
        };
      }),
    );
  };

  const removeGroupPriceOverride = (index) => {
    setGroupPriceOverrides((prev) =>
      (Array.isArray(prev) ? prev : []).filter(
        (_, itemIndex) => itemIndex !== index,
      ),
    );
  };

  const loadSubscriptionPresets = async () => {
    if (!userId) {
      return {
        subscriptionPresets: [],
        requestSubscriptionPresets: [],
      };
    }
    setSubscriptionPresetsLoading(true);
    try {
      const res = await API.get('/api/redemption/presets', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取商品列表失败'));
        setSubscriptionPresets([]);
        setRequestSubscriptionPresets([]);
        return {
          subscriptionPresets: [],
          requestSubscriptionPresets: [],
        };
      }

      const list = Array.isArray(data) ? data : [];
      const normalized = list
        .map((item) => {
          const id = Number(item?.id ?? 0);
          if (!Number.isFinite(id) || id <= 0) return null;
          const mode = inferPresetMode(item);
          if (mode !== 'subscription' && mode !== 'request') return null;

          const name = String(item?.name ?? '').trim();
          if (!name) return null;
          const description = String(item?.description ?? '').trim();

          const sortRaw = Number(item?.sort_order ?? 0);
          const sortOrder = Number.isFinite(sortRaw)
            ? Math.max(0, Math.floor(sortRaw))
            : 0;

          return {
            ...item,
            id,
            name,
            description,
            _mode: mode,
            enabled: item?.enabled !== false,
            sort_order: sortOrder,
          };
        })
        .filter(Boolean);

      const sortPresets = (a, b) => {
        const ea = a?.enabled !== false;
        const eb = b?.enabled !== false;
        if (ea !== eb) return ea ? -1 : 1;

        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;

        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      };

      const nextSubscriptionPresets = normalized
        .filter((item) => item?._mode === 'subscription')
        .sort(sortPresets);
      const nextRequestSubscriptionPresets = normalized
        .filter((item) => item?._mode === 'request')
        .sort(sortPresets);

      setSubscriptionPresets(nextSubscriptionPresets);
      setRequestSubscriptionPresets(nextRequestSubscriptionPresets);
      return {
        subscriptionPresets: nextSubscriptionPresets,
        requestSubscriptionPresets: nextRequestSubscriptionPresets,
      };
    } catch (e) {
      showError(e?.message || t('获取商品列表失败'));
      setSubscriptionPresets([]);
      setRequestSubscriptionPresets([]);
      return {
        subscriptionPresets: [],
        requestSubscriptionPresets: [],
      };
    } finally {
      setSubscriptionPresetsLoading(false);
    }
  };

  const loadUser = async () => {
    setLoading(true);
    const url = userId
      ? `/api/user/${userId}?include_payg_balances=true`
      : `/api/user/self`;
    try {
      const res = await API.get(
        url,
        userId ? { disableDuplicate: true } : undefined,
      );
      const { success, message, data } = res.data;
      if (success) {
        applyLoadedUserFormData(data);
      } else {
        setUserDetail(null);
        setPricingFormValues({
          customer_type: 'retail',
          pricing_profile_id: 0,
          base_multiplier: 1,
        });
        setGroupPriceOverrides([]);
        setResolvedGroupPricing({});
        showError(message);
      }
    } catch (error) {
      setUserDetail(null);
      setPricingFormValues({
        customer_type: 'retail',
        pricing_profile_id: 0,
        base_multiplier: 1,
      });
      setGroupPriceOverrides([]);
      setResolvedGroupPricing({});
      showError(error.message || t('请求失败'));
    }
    setLoading(false);
  };

  useEffect(() => {
    void fetchGroups();
    void fetchUserGroups();
    applySubscriptionBreakdown(null);
    applyRequestSubscriptionBreakdown(null);
    void loadUser();
    if (userId) {
      setSubscriptionsLoading(true);
      setRequestSubscriptionsLoading(true);
      void loadPricingProfiles();
      void refreshSubscriptions();
      void refreshRequestSubscriptions();
    } else {
      setSubscriptionsLoading(false);
      setRequestSubscriptionsLoading(false);
      setRequestSubscriptions([]);
      setRequestSubscriptionSummary(null);
      setPricingFormValues({
        customer_type: 'retail',
        pricing_profile_id: 0,
        base_multiplier: 1,
      });
      setGroupPriceOverrides([]);
      setResolvedGroupPricing({});
    }
  }, [props.editingUser.id]);

  const refreshRequestSubscriptions = async () => {
    if (!userId) {
      setRequestSubscriptionsLoading(false);
      applyRequestSubscriptionBreakdown(null);
      return;
    }
    setRequestSubscriptionsLoading(true);
    try {
      const res = await API.get(`/api/user/${userId}/request_subscriptions`);
      const { success, message, data } = res.data;
      if (success) {
        applyRequestSubscriptionBreakdown(data);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message || t('请求失败'));
    } finally {
      setRequestSubscriptionsLoading(false);
    }
  };

  const refreshSubscriptions = async () => {
    if (!userId) {
      setSubscriptionsLoading(false);
      applySubscriptionBreakdown(null);
      return;
    }
    setSubscriptionsLoading(true);
    try {
      const res = await API.get(`/api/user/${userId}/subscriptions`);
      const { success, message, data } = res.data;
      if (success) {
        applySubscriptionBreakdown(data);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message || t('请求失败'));
    } finally {
      setSubscriptionsLoading(false);
    }
  };

  const refreshUserBillingSnapshot = async () => {
    if (!userId) return;
    try {
      const res = await API.get(
        `/api/user/${userId}?include_payg_balances=true`,
        { disableDuplicate: true },
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      if (data) {
        applyLoadedUserSnapshot(data);
      }
    } catch (error) {
      showError(error?.message || t('请求失败'));
    }
  };

  const submit = async (values) => {
    setLoading(true);
    const payload = { ...values };
    delete payload.group;
    delete payload.group_id;
    delete payload.aff_code;
    delete payload.aff_count;
    if (userId) {
      const rawInviterId = Number(payload.inviter_id ?? 0);
      if (!Number.isFinite(rawInviterId) || rawInviterId < 0) {
        showError(t('邀请人 ID 无效'));
        setLoading(false);
        return;
      }
      payload.inviter_id = Math.floor(rawInviterId);
    } else {
      delete payload.inviter_id;
    }
    const paygRemainingInput = Number(paygRemaining) || 0;
    if (paygRemainingInput < 0) {
      showError(t('按量付费余额不能小于0'));
      setLoading(false);
      return;
    }
    delete payload.quota;
    delete payload.payg_quota;
    if (typeof payload.base_multiplier === 'string') {
      payload.base_multiplier = parseFloat(payload.base_multiplier);
    }
    if (
      !Number.isFinite(payload.base_multiplier) ||
      payload.base_multiplier <= 0
    ) {
      showError(t('基础倍率必须大于0'));
      setLoading(false);
      return;
    }
    if (userId) {
      payload.customer_type = String(payload.customer_type || 'retail')
        .trim()
        .toLowerCase();
      const rawProfileId = Number(payload.pricing_profile_id ?? 0);
      payload.pricing_profile_id = Number.isFinite(rawProfileId)
        ? Math.max(0, Math.floor(rawProfileId))
        : 0;

      const normalizedOverrides = [];
      const seenGroupIds = new Set();
      for (const item of Array.isArray(groupPriceOverrides)
        ? groupPriceOverrides
        : []) {
        const rawGroupId = Number(item?.group_id ?? 0);
        const groupId = Number.isFinite(rawGroupId)
          ? Math.floor(rawGroupId)
          : 0;
        if (groupId <= 0) {
          showError(t('分组覆写中的分组无效'));
          setLoading(false);
          return;
        }
        if (seenGroupIds.has(groupId)) {
          showError(
            t('分组 {{group}} 的价格覆写重复', {
              group: groupLabelById[groupId] || String(groupId),
            }),
          );
          setLoading(false);
          return;
        }
        const factor = Number(item?.factor ?? 0);
        if (!Number.isFinite(factor) || factor <= 0) {
          showError(
            t('分组 {{group}} 的价格倍率必须大于0', {
              group: groupLabelById[groupId] || String(groupId),
            }),
          );
          setLoading(false);
          return;
        }
        seenGroupIds.add(groupId);
        normalizedOverrides.push({
          group_id: groupId,
          factor,
        });
      }
      payload.group_price_overrides = normalizedOverrides.sort(
        (a, b) => a.group_id - b.group_id,
      );
    } else {
      delete payload.customer_type;
      delete payload.pricing_profile_id;
      delete payload.group_price_overrides;
    }
    if (canEditAdminPermissions) {
      payload.admin_permissions = {
        product_management:
          payload.admin_permissions?.product_management === true,
        order: payload.admin_permissions?.order === true,
      };
    } else {
      delete payload.admin_permissions;
    }
    if (userId) {
      payload.id = parseInt(userId, 10);
    }

    try {
      const url = userId ? `/api/user/` : `/api/user/self`;
      const res = await API.put(url, payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('用户信息更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message || t('请求失败'));
    }
    setLoading(false);
  };

  const openSubscriptionModal = (subscription = null) => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    setEditingSubscription(subscription);
    const formValues = buildSubscriptionFormValues(subscription);
    setSubscriptionInitialValues(formValues);
    setSubscriptionModalVisible(true);
    setSubscriptionFormKey((prev) => prev + 1);
  };

  const openSubscriptionPresetModal = async () => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    if (subscriptionPresetsLoading) return;
    let presets = Array.isArray(subscriptionPresets) ? subscriptionPresets : [];
    if (presets.length === 0) {
      const loaded = await loadSubscriptionPresets();
      presets = Array.isArray(loaded?.subscriptionPresets)
        ? loaded.subscriptionPresets
        : [];
    }
    if (presets.length === 0) {
      showError(t('暂无可用订阅商品'));
      return;
    }

    const first = presets.find((p) => p?.enabled !== false) || presets[0];
    const presetId = Number(first?.id ?? 0);
    const initValues = {
      preset_id:
        Number.isFinite(presetId) && presetId > 0 ? presetId : undefined,
      apply_mode: 'stack',
      quantity: 1,
    };
    setSubscriptionPresetInitValues(initValues);
    setSubscriptionPresetFormKey((k) => k + 1);
    setSubscriptionPresetModalVisible(true);
  };

  const openRequestSubscriptionModal = (subscription = null) => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    setEditingRequestSubscription(subscription);
    const formValues = buildRequestSubscriptionFormValues(subscription);
    setRequestSubscriptionInitialValues(formValues);
    setRequestSubscriptionModalVisible(true);
    setRequestSubscriptionFormKey((prev) => prev + 1);
  };

  const openRequestSubscriptionPresetModal = async () => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    if (subscriptionPresetsLoading) return;
    let presets = Array.isArray(requestSubscriptionPresets)
      ? requestSubscriptionPresets
      : [];
    if (presets.length === 0) {
      const loaded = await loadSubscriptionPresets();
      presets = Array.isArray(loaded?.requestSubscriptionPresets)
        ? loaded.requestSubscriptionPresets
        : [];
    }
    if (presets.length === 0) {
      showError(t('暂无可用次数订阅商品'));
      return;
    }

    const first = presets.find((p) => p?.enabled !== false) || presets[0];
    const presetId = Number(first?.id ?? 0);
    const initValues = {
      preset_id:
        Number.isFinite(presetId) && presetId > 0 ? presetId : undefined,
      apply_mode: 'stack',
      quantity: 1,
    };
    setRequestSubscriptionPresetInitValues(initValues);
    setRequestSubscriptionPresetFormKey((k) => k + 1);
    setRequestSubscriptionPresetModalVisible(true);
  };

  const openPaygTopupModal = async () => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    let products = Array.isArray(paygProducts) ? paygProducts : [];
    if (products.length === 0) {
      products = (await loadPaygProducts()) || [];
    }
    if (!Array.isArray(products) || products.length === 0) {
      showError(t('暂无可用按量付费商品'));
      return;
    }

    const sortedProducts = products.slice().sort((a, b) => {
      const sa = Number(a?.sort_order ?? 0) || 0;
      const sb = Number(b?.sort_order ?? 0) || 0;
      if (sa !== sb) return sb - sa;
      const ia = Number(a?.id ?? 0) || 0;
      const ib = Number(b?.id ?? 0) || 0;
      return ib - ia;
    });

    const prevValues = paygTopupFormApiRef.current?.getValues?.();
    const prevProductIdRaw = parseInt(prevValues?.product_id, 10);
    const prevProductId =
      Number.isFinite(prevProductIdRaw) && prevProductIdRaw > 0
        ? prevProductIdRaw
        : 0;
    const selectedProduct =
      prevProductId > 0
        ? sortedProducts.find((p) => Number(p?.id ?? 0) === prevProductId)
        : null;
    const fallbackProduct =
      sortedProducts.find((p) => p?.enabled !== false) || sortedProducts[0];
    const productIdRaw = Number(
      selectedProduct?.id ?? fallbackProduct?.id ?? 0,
    );
    const productId =
      Number.isFinite(productIdRaw) && productIdRaw > 0
        ? productIdRaw
        : undefined;

    const prevQuotaUsdRaw = parseFloat(prevValues?.quota_usd);
    const quotaUsd =
      Number.isFinite(prevQuotaUsdRaw) && prevQuotaUsdRaw > 0
        ? prevQuotaUsdRaw
        : 1;
    setPaygTopupInitValues({
      product_id: productId,
      quota_usd: quotaUsd,
    });
    setPaygTopupFormKey((k) => k + 1);
    setPaygTopupModalVisible(true);
  };

  const openPaygManualModal = () => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    if (groupOptionsLoading) return;
    if (!Array.isArray(groupIdOptions) || groupIdOptions.length === 0) {
      showError(t('暂无可用分组'));
      return;
    }

    const currentGroupIdRaw = Number(
      formApiRef.current?.getValue('group_id') ?? 0,
    );
    const currentGroupId = Number.isFinite(currentGroupIdRaw)
      ? Math.floor(currentGroupIdRaw)
      : 0;
    const fallbackGroupIdRaw = Number(groupIdOptions?.[0]?.value ?? 0);
    const fallbackGroupId = Number.isFinite(fallbackGroupIdRaw)
      ? Math.floor(fallbackGroupIdRaw)
      : 0;
    const defaultGroupId = currentGroupId || fallbackGroupId;
    setPaygManualInitValues({
      group_id: defaultGroupId > 0 ? defaultGroupId : undefined,
      quota_usd: 1,
    });
    setPaygManualFormKey((k) => k + 1);
    setPaygManualModalVisible(true);
  };

  const openPaygBalanceGroupModal = (balance) => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    if (!balance) return;
    if (groupOptionsLoading) return;
    if (!Array.isArray(groupIdOptions) || groupIdOptions.length === 0) {
      showError(t('暂无可用分组'));
      return;
    }
    setEditingPaygBalance(balance);
    setPaygBalanceGroupInitValues({
      allowed_group_ids: normalizeGroupIds(balance?.allowed_group_ids),
    });
    setPaygBalanceGroupFormKey((k) => k + 1);
    setPaygBalanceGroupModalVisible(true);
  };

  const handlePaygBalanceGroupSubmit = async () => {
    if (!userId) return;
    if (!editingPaygBalance) return;
    if (!paygBalanceGroupFormApiRef.current) return;

    const values = paygBalanceGroupFormApiRef.current.getValues();
    const allowedGroupIds = normalizeGroupIds(values?.allowed_group_ids);
    if (allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      return;
    }

    setPaygBalanceGroupSubmitting(true);
    try {
      const res = await API.patch(
        `/api/user/${userId}/payg/balances/${editingPaygBalance.product_id}`,
        {
          allowed_group_ids: allowedGroupIds,
        },
      );
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('保存失败'));
        return;
      }
      showSuccess(t('保存成功'));
      setPaygBalanceGroupModalVisible(false);
      await refreshUserBillingSnapshot();
      props.refresh();
    } catch (error) {
      showError(error?.message || t('保存失败'));
    } finally {
      setPaygBalanceGroupSubmitting(false);
    }
  };

  const handleDeletePaygBalance = (balance) => {
    if (!userId) {
      showError(t('仅支持编辑已存在的用户'));
      return;
    }
    if (!balance) return;
    Modal.confirm({
      title: t('确认删除该按量付费余额？'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(
            `/api/user/${userId}/payg/balances/${balance.product_id}`,
          );
          const { success, message } = res.data || {};
          if (!success) {
            showError(message || t('请求失败'));
            return;
          }
          showSuccess(t('已删除'));
          await refreshUserBillingSnapshot();
          await refreshSubscriptions();
          props.refresh();
        } catch (error) {
          showError(error?.message || t('请求失败'));
        }
      },
    });
  };

  const handleSubscriptionSubmit = async () => {
    if (!subscriptionFormApiRef.current) return;
    const values = subscriptionFormApiRef.current.getValues();
    const totalQuotaUSD = parseFloat(values.quota_usd);
    const remainingQuotaUSD = parseFloat(values.remaining_quota_usd);
    const startAt = values.start_at
      ? Math.floor(new Date(values.start_at).getTime() / 1000)
      : 0;
    const expireAt = values.expire_at
      ? Math.floor(new Date(values.expire_at).getTime() / 1000)
      : 0;

    if (!Number.isFinite(totalQuotaUSD) || totalQuotaUSD <= 0) {
      showError(t('总额度必须大于0'));
      return;
    }
    if (!Number.isFinite(startAt) || startAt <= 0) {
      showError(t('请选择开始时间'));
      return;
    }

    const totalQuota = convertUSDToQuota(totalQuotaUSD);
    let remainingQuota =
      Number.isFinite(remainingQuotaUSD) && remainingQuotaUSD >= 0
        ? convertUSDToQuota(remainingQuotaUSD)
        : totalQuota;

    if (remainingQuota > totalQuota) {
      remainingQuota = totalQuota;
    }

    const sourcePresetId =
      Number(editingSubscription?.source_preset_id ?? 0) || 0;
    const sourceRedemptionId =
      Number(editingSubscription?.source_redemption_id ?? 0) || 0;
    const groupDailyLimitsEditable =
      !editingSubscription || (sourcePresetId <= 0 && sourceRedemptionId <= 0);

    const allowedGroupIds = normalizeGroupIds(values.allowed_group_ids);
    if (allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      return;
    }

    let dailyLimit = 0;
    let groupDailyLimitsPayload = null;

    if (groupDailyLimitsEditable) {
      const useGroupDailyLimits = Boolean(values.use_group_daily_limits);
      if (useGroupDailyLimits) {
        const synced = syncGroupDailyLimitsUSD(
          allowedGroupIds,
          values.group_daily_limits,
        );
        if (synced.length === 0) {
          showError(t('请选择可用分组'));
          return;
        }

        let hasUnlimited = false;
        let dailyLimitTotalTokens = 0;
        const payload = [];
        for (const item of synced) {
          const dailyUSD = parseFloat(item?.daily_quota_limit_usd);
          if (!Number.isFinite(dailyUSD) || dailyUSD < 0) {
            showError(
              `${t('每日额度必须大于等于0')}: ${
                groupLabelById[item.group_id] || t('未知分组')
              }`,
            );
            return;
          }
          const dailyTokens = dailyUSD > 0 ? convertUSDToQuota(dailyUSD) : 0;
          if (
            dailyUSD > 0 &&
            (!Number.isFinite(dailyTokens) || dailyTokens <= 0)
          ) {
            showError(
              `${t('每日额度必须大于0')}: ${
                groupLabelById[item.group_id] || t('未知分组')
              }`,
            );
            return;
          }
          payload.push({
            group_id: item.group_id,
            daily_quota_limit: dailyTokens,
          });
          if (dailyTokens === 0) {
            hasUnlimited = true;
          } else {
            dailyLimitTotalTokens += dailyTokens;
          }
        }
        groupDailyLimitsPayload = payload;
        dailyLimit = hasUnlimited ? 0 : dailyLimitTotalTokens;
      } else {
        const dailyLimitUSD = parseFloat(values.daily_quota_limit_usd);
        if (!Number.isFinite(dailyLimitUSD) || dailyLimitUSD < 0) {
          showError(t('每日额度必须大于等于0'));
          return;
        }
        dailyLimit = dailyLimitUSD > 0 ? convertUSDToQuota(dailyLimitUSD) : 0;
        if (
          dailyLimitUSD > 0 &&
          (!Number.isFinite(dailyLimit) || dailyLimit <= 0)
        ) {
          showError(t('每日额度必须大于0'));
          return;
        }
        groupDailyLimitsPayload = [];
      }
    } else {
      // Non-manual subscriptions: group config is derived from product/redemption, keep it read-only.
      const useGroupDailyLimits = Boolean(values.use_group_daily_limits);
      if (!useGroupDailyLimits) {
        const dailyLimitUSD = parseFloat(values.daily_quota_limit_usd);
        if (!Number.isFinite(dailyLimitUSD) || dailyLimitUSD < 0) {
          showError(t('每日额度必须大于等于0'));
          return;
        }
        dailyLimit = dailyLimitUSD > 0 ? convertUSDToQuota(dailyLimitUSD) : 0;
        if (
          dailyLimitUSD > 0 &&
          (!Number.isFinite(dailyLimit) || dailyLimit <= 0)
        ) {
          showError(t('每日额度必须大于0'));
          return;
        }
      } else {
        dailyLimit = Number(editingSubscription?.daily_quota_limit ?? 0) || 0;
      }
    }

    setSubscriptionSubmitting(true);

    try {
      if (editingSubscription) {
        const payload = {};
        if (totalQuota !== editingSubscription.total_quota) {
          payload.quota = totalQuota;
        }
        if (
          !Number.isNaN(remainingQuota) &&
          remainingQuota !== editingSubscription.remaining_quota
        ) {
          payload.remaining_quota = remainingQuota;
        }
        if (startAt !== (editingSubscription.start_at || 0)) {
          payload.start_at = startAt;
        }
        const prevGroupIds = normalizeGroupIds(
          editingSubscription.allowed_group_ids,
        );
        if (prevGroupIds.join(',') !== allowedGroupIds.join(',')) {
          payload.allowed_group_ids = allowedGroupIds;
        }

        if (groupDailyLimitsEditable) {
          payload.group_daily_limits = groupDailyLimitsPayload || [];
          payload.daily_quota_limit = dailyLimit;
        } else if (
          !values.use_group_daily_limits &&
          dailyLimit !== (editingSubscription.daily_quota_limit || 0)
        ) {
          payload.daily_quota_limit = dailyLimit;
        }
        if (expireAt !== (editingSubscription.expire_at || 0)) {
          payload.expire_at = expireAt;
        }
        if (Object.keys(payload).length === 0) {
          showSuccess(t('已保存'));
          setSubscriptionModalVisible(false);
          setSubscriptionSubmitting(false);
          return;
        }
        const res = await API.patch(
          `/api/user/${userId}/subscriptions/${editingSubscription.id}`,
          payload,
        );
        const { success, message, data } = res.data;
        if (success) {
          showSuccess(t('订阅额度已更新'));
          applySubscriptionBreakdown(data?.breakdown || data);
          props.refresh();
          setSubscriptionModalVisible(false);
        } else {
          showError(message);
        }
      } else {
        const payload = {
          quota: totalQuota,
          start_at: startAt,
        };
        if (!Number.isNaN(remainingQuota) && remainingQuota >= 0) {
          payload.remaining_quota = remainingQuota;
        }
        payload.daily_quota_limit = dailyLimit;
        if (expireAt > 0) {
          payload.expire_at = expireAt;
        }
        payload.allowed_group_ids = allowedGroupIds;
        payload.group_daily_limits = groupDailyLimitsPayload || [];
        if (values.source) {
          payload.source = values.source;
        }
        const res = await API.post(
          `/api/user/${userId}/subscriptions`,
          payload,
        );
        const { success, message, data } = res.data;
        if (success) {
          showSuccess(t('订阅额度已创建'));
          applySubscriptionBreakdown(data?.breakdown || data);
          props.refresh();
          setSubscriptionModalVisible(false);
        } else {
          showError(message);
        }
      }
    } catch (error) {
      showError(error.message || t('请求失败'));
    }

    setSubscriptionSubmitting(false);
  };

  const handleSubscriptionPresetSubmit = async () => {
    if (!userId) return;
    if (!subscriptionPresetFormApiRef.current) return;

    const values = subscriptionPresetFormApiRef.current.getValues();
    const presetId = parseInt(values?.preset_id, 10);
    const applyMode = String(values?.apply_mode || 'stack').trim();
    const quantityRaw = Number(values?.quantity ?? 1);
    const quantity = Number.isFinite(quantityRaw)
      ? Math.max(1, Math.floor(quantityRaw))
      : 1;

    if (!Number.isFinite(presetId) || presetId <= 0) {
      showError(t('请选择订阅商品'));
      return;
    }
    if (applyMode !== 'stack' && applyMode !== 'defer') {
      showError(t('请选择生效方式'));
      return;
    }
    if (quantity <= 0 || quantity > 100) {
      showError(t('数量不合法'));
      return;
    }

    const preset = subscriptionPresetById[presetId];
    if (!preset) {
      showError(t('订阅商品不存在'));
      return;
    }
    if (!preset?.multi_quantity_enabled && quantity !== 1) {
      showError(t('该商品不支持多数量'));
      return;
    }
    if (
      preset?.multi_quantity_defer_only &&
      quantity > 1 &&
      applyMode !== 'defer'
    ) {
      showError(t('该商品多数量仅支持顺延'));
      return;
    }

    setSubscriptionPresetSubmitting(true);
    try {
      const res = await API.post(`/api/user/${userId}/subscriptions/preset`, {
        preset_id: presetId,
        apply_mode: applyMode,
        quantity,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      showSuccess(t('订阅额度已创建'));
      applySubscriptionBreakdown(data?.breakdown || data);
      props.refresh();
      setSubscriptionPresetModalVisible(false);
    } catch (error) {
      showError(error?.message || t('请求失败'));
    } finally {
      setSubscriptionPresetSubmitting(false);
    }
  };

  const handleRequestSubscriptionSubmit = async () => {
    if (!userId) return;
    if (!requestSubscriptionFormApiRef.current) return;

    const values = requestSubscriptionFormApiRef.current.getValues();
    const dailyLimitRaw = parseFloat(values?.daily_request_limit);
    const dailyLimit = Number.isFinite(dailyLimitRaw) ? dailyLimitRaw : 0;
    if (dailyLimit < 0) {
      showError(t('每日次数必须大于等于0'));
      return;
    }
    const totalLimitRaw = parseFloat(values?.total_request_limit);
    const totalLimit = Number.isFinite(totalLimitRaw) ? totalLimitRaw : 0;
    if (totalLimit < 0) {
      showError(t('总次数必须大于等于0'));
      return;
    }

    const allowedGroupIds = normalizeGroupIds(values?.allowed_group_ids);
    if (!Array.isArray(allowedGroupIds) || allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      return;
    }

    const start = values?.start_at;
    if (!(start instanceof Date) || Number.isNaN(start.getTime())) {
      showError(t('开始时间不合法'));
      return;
    }
    const startAt = Math.floor(start.getTime() / 1000);

    let expireAt = 0;
    if (values?.expire_at) {
      const expire = values.expire_at;
      if (!(expire instanceof Date) || Number.isNaN(expire.getTime())) {
        showError(t('结束时间不合法'));
        return;
      }
      expireAt = Math.floor(expire.getTime() / 1000);
      if (expireAt <= startAt) {
        showError(t('结束时间必须晚于开始时间'));
        return;
      }
    }

    setRequestSubscriptionSubmitting(true);
    try {
      if (editingRequestSubscription) {
        const payload = {};
        if (
          dailyLimit !== (editingRequestSubscription.daily_request_limit || 0)
        ) {
          payload.daily_request_limit = dailyLimit;
        }
        if (
          totalLimit !== (editingRequestSubscription.total_request_limit || 0)
        ) {
          payload.total_request_limit = totalLimit;
        }
        const prevGroupIds = normalizeGroupIds(
          editingRequestSubscription.allowed_group_ids,
        );
        if (prevGroupIds.join(',') !== allowedGroupIds.join(',')) {
          payload.allowed_group_ids = allowedGroupIds;
        }
        if (startAt !== (editingRequestSubscription.start_at || 0)) {
          payload.start_at = startAt;
        }
        if (expireAt !== (editingRequestSubscription.expire_at || 0)) {
          payload.expire_at = expireAt;
        }

        if (Object.keys(payload).length === 0) {
          showSuccess(t('已保存'));
          setRequestSubscriptionModalVisible(false);
          return;
        }

        const res = await API.patch(
          `/api/user/${userId}/request_subscriptions/${editingRequestSubscription.id}`,
          payload,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message);
          return;
        }
        showSuccess(t('次数订阅已更新'));
        applyRequestSubscriptionBreakdown(data?.breakdown || data);
        props.refresh();
        setRequestSubscriptionModalVisible(false);
      } else {
        const payload = {
          daily_request_limit: dailyLimit,
          total_request_limit: totalLimit,
          start_at: startAt,
          allowed_group_ids: allowedGroupIds,
        };
        if (expireAt > 0) {
          payload.expire_at = expireAt;
        }
        if (values?.source) {
          payload.source = values.source;
        }
        const res = await API.post(
          `/api/user/${userId}/request_subscriptions`,
          payload,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message);
          return;
        }
        showSuccess(t('次数订阅已创建'));
        applyRequestSubscriptionBreakdown(data?.breakdown || data);
        props.refresh();
        setRequestSubscriptionModalVisible(false);
      }
    } catch (error) {
      showError(error.message || t('请求失败'));
    } finally {
      setRequestSubscriptionSubmitting(false);
    }
  };

  const handleRequestSubscriptionPresetSubmit = async () => {
    if (!userId) return;
    if (!requestSubscriptionPresetFormApiRef.current) return;

    const values = requestSubscriptionPresetFormApiRef.current.getValues();
    const presetId = parseInt(values?.preset_id, 10);
    const applyMode = String(values?.apply_mode || 'stack').trim();
    const quantityRaw = Number(values?.quantity ?? 1);
    const quantity = Number.isFinite(quantityRaw)
      ? Math.max(1, Math.floor(quantityRaw))
      : 1;

    if (!Number.isFinite(presetId) || presetId <= 0) {
      showError(t('请选择订阅商品'));
      return;
    }
    if (applyMode !== 'stack' && applyMode !== 'defer') {
      showError(t('请选择生效方式'));
      return;
    }
    if (quantity <= 0 || quantity > 100) {
      showError(t('数量不合法'));
      return;
    }

    const preset = requestSubscriptionPresetById[presetId];
    if (!preset) {
      showError(t('订阅商品不存在'));
      return;
    }
    if (!preset?.multi_quantity_enabled && quantity !== 1) {
      showError(t('该商品不支持多数量'));
      return;
    }
    if (
      preset?.multi_quantity_defer_only &&
      quantity > 1 &&
      applyMode !== 'defer'
    ) {
      showError(t('该商品多数量仅支持顺延'));
      return;
    }

    setRequestSubscriptionPresetSubmitting(true);
    try {
      const res = await API.post(
        `/api/user/${userId}/request_subscriptions/preset`,
        {
          preset_id: presetId,
          apply_mode: applyMode,
          quantity,
        },
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      showSuccess(t('次数订阅已创建'));
      applyRequestSubscriptionBreakdown(data?.breakdown || data);
      props.refresh();
      setRequestSubscriptionPresetModalVisible(false);
    } catch (error) {
      showError(error?.message || t('请求失败'));
    } finally {
      setRequestSubscriptionPresetSubmitting(false);
    }
  };

  const handlePaygTopupSubmit = async () => {
    if (!userId) return;
    if (!paygTopupFormApiRef.current) return;

    const values = paygTopupFormApiRef.current.getValues();
    const productIdRaw = parseInt(values?.product_id, 10);
    const productId =
      Number.isFinite(productIdRaw) && productIdRaw > 0 ? productIdRaw : 0;
    const usd = parseFloat(values?.quota_usd);
    if (!productId) {
      showError(t('请选择商品'));
      return;
    }
    if (!Number.isFinite(usd) || usd <= 0) {
      showError(t('充值金额必须大于 0'));
      return;
    }

    const quota = convertUSDToQuota(usd);
    if (!Number.isFinite(quota) || quota <= 0) {
      showError(t('充值额度必须大于 0'));
      return;
    }

    setPaygTopupSubmitting(true);
    try {
      const res = await API.post(`/api/user/${userId}/payg/topup`, {
        product_id: productId,
        quota,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      showSuccess(t('充值成功'));
      applySubscriptionBreakdown(data);
      props.refresh();
      setPaygTopupModalVisible(false);
      await refreshUserBillingSnapshot();
    } catch (error) {
      showError(error?.message || t('请求失败'));
    } finally {
      setPaygTopupSubmitting(false);
    }
  };

  const handlePaygManualSubmit = async () => {
    if (!userId) return;
    if (!paygManualFormApiRef.current) return;

    const values = paygManualFormApiRef.current.getValues();
    const groupIdRaw = Number(values?.group_id ?? 0);
    const groupId = Number.isFinite(groupIdRaw) ? Math.floor(groupIdRaw) : 0;
    const usd = parseFloat(values?.quota_usd);
    if (!groupId) {
      showError(t('请选择分组'));
      return;
    }
    if (!Number.isFinite(usd) || usd <= 0) {
      showError(t('充值金额必须大于 0'));
      return;
    }

    const quota = convertUSDToQuota(usd);
    if (!Number.isFinite(quota) || quota <= 0) {
      showError(t('充值额度必须大于 0'));
      return;
    }

    setPaygManualSubmitting(true);
    try {
      const res = await API.post(`/api/user/${userId}/payg/topup/group`, {
        group_id: groupId,
        quota,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      showSuccess(t('充值成功'));
      applySubscriptionBreakdown(data);
      props.refresh();
      setPaygManualModalVisible(false);
      await refreshUserBillingSnapshot();
    } catch (error) {
      showError(error?.message || t('请求失败'));
    } finally {
      setPaygManualSubmitting(false);
    }
  };

  const handleDeleteSubscription = (subscription) => {
    Modal.confirm({
      title: t('确认删除该订阅？'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(
            `/api/user/${userId}/subscriptions/${subscription.id}`,
          );
          const { success, message, data } = res.data;
          if (success) {
            showSuccess(t('订阅额度已删除'));
            applySubscriptionBreakdown(data);
            props.refresh();
          } else {
            showError(message);
          }
        } catch (error) {
          showError(error.message || t('请求失败'));
        }
      },
    });
  };

  const handleDeleteRequestSubscription = (subscription) => {
    Modal.confirm({
      title: t('确认删除该订阅？'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(
            `/api/user/${userId}/request_subscriptions/${subscription.id}`,
          );
          const { success, message, data } = res.data;
          if (success) {
            showSuccess(t('次数订阅已删除'));
            applyRequestSubscriptionBreakdown(data);
            props.refresh();
          } else {
            showError(message);
          }
        } catch (error) {
          showError(error.message || t('请求失败'));
        }
      },
    });
  };

  const clearSubscriptionDragState = () => {
    setSubscriptionDraggingId(0);
    setSubscriptionDropTargetId(0);
  };

  const clearRequestSubscriptionDragState = () => {
    setRequestSubscriptionDraggingId(0);
    setRequestSubscriptionDropTargetId(0);
  };

  const clearPaygDragState = () => {
    setPaygDraggingProductId(0);
    setPaygDropTargetProductId(0);
  };

  const submitSubscriptionReorder = async (sourceId, targetId) => {
    if (!userId || subscriptionReordering) {
      clearSubscriptionDragState();
      return;
    }
    const source = subscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(sourceId),
    );
    const target = subscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(targetId),
    );
    clearSubscriptionDragState();
    if (!source || !target) return;

    const sourceBucket = getBillingStatusBucket(source);
    const targetBucket = getBillingStatusBucket(target);
    if (sourceBucket === 'expired' || sourceBucket !== targetBucket) return;

    const currentIds = normalizeOrderedUniquePositiveIds(
      subscriptions
        .filter((item) => getBillingStatusBucket(item) === sourceBucket)
        .map((item) => Number(item?.id ?? 0)),
    );
    const nextIds = reorderIdsBefore(currentIds, sourceId, targetId);
    if (nextIds.join(',') === currentIds.join(',')) return;

    setSubscriptionReordering(true);
    try {
      const res = await API.patch(`/api/user/${userId}/subscriptions/reorder`, {
        subscription_ids: nextIds,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('保存排序失败'));
        return;
      }
      showSuccess(t('订阅额度扣费顺序已更新'));
      applySubscriptionBreakdown(data);
      props.refresh();
    } catch (error) {
      showError(error?.message || t('保存排序失败'));
    } finally {
      setSubscriptionReordering(false);
    }
  };

  const handleSubscriptionDragStart = (event, subscription) => {
    if (subscriptionReordering) return;
    if (getBillingStatusBucket(subscription) === 'expired') return;
    const id = Number(subscription?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    setSubscriptionDraggingId(id);
    setSubscriptionDropTargetId(id);
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', String(id));
    }
  };

  const handleSubscriptionDragOver = (event, subscription) => {
    if (!subscriptionDraggingId || subscriptionReordering) return;
    const targetId = Number(subscription?.id ?? 0);
    if (!Number.isFinite(targetId) || targetId <= 0) return;
    const source = subscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(subscriptionDraggingId),
    );
    if (!source) return;
    if (getBillingStatusBucket(source) !== getBillingStatusBucket(subscription))
      return;
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
    setSubscriptionDropTargetId(targetId);
  };

  const handleSubscriptionDrop = async (event, subscription) => {
    event.preventDefault();
    await submitSubscriptionReorder(subscriptionDraggingId, subscription?.id);
  };

  const submitRequestSubscriptionReorder = async (sourceId, targetId) => {
    if (!userId || requestSubscriptionReordering) {
      clearRequestSubscriptionDragState();
      return;
    }
    const source = requestSubscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(sourceId),
    );
    const target = requestSubscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(targetId),
    );
    clearRequestSubscriptionDragState();
    if (!source || !target) return;

    const sourceBucket = getBillingStatusBucket(source);
    const targetBucket = getBillingStatusBucket(target);
    if (sourceBucket === 'expired' || sourceBucket !== targetBucket) return;

    const currentIds = normalizeOrderedUniquePositiveIds(
      requestSubscriptions
        .filter((item) => getBillingStatusBucket(item) === sourceBucket)
        .map((item) => Number(item?.id ?? 0)),
    );
    const nextIds = reorderIdsBefore(currentIds, sourceId, targetId);
    if (nextIds.join(',') === currentIds.join(',')) return;

    setRequestSubscriptionReordering(true);
    try {
      const res = await API.patch(
        `/api/user/${userId}/request_subscriptions/reorder`,
        {
          subscription_ids: nextIds,
        },
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('保存排序失败'));
        return;
      }
      showSuccess(t('次数订阅扣费顺序已更新'));
      applyRequestSubscriptionBreakdown(data);
      props.refresh();
    } catch (error) {
      showError(error?.message || t('保存排序失败'));
    } finally {
      setRequestSubscriptionReordering(false);
    }
  };

  const handleRequestSubscriptionDragStart = (event, subscription) => {
    if (requestSubscriptionReordering) return;
    if (getBillingStatusBucket(subscription) === 'expired') return;
    const id = Number(subscription?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    setRequestSubscriptionDraggingId(id);
    setRequestSubscriptionDropTargetId(id);
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', String(id));
    }
  };

  const handleRequestSubscriptionDragOver = (event, subscription) => {
    if (!requestSubscriptionDraggingId || requestSubscriptionReordering) return;
    const targetId = Number(subscription?.id ?? 0);
    if (!Number.isFinite(targetId) || targetId <= 0) return;
    const source = requestSubscriptions.find(
      (item) => Number(item?.id ?? 0) === Number(requestSubscriptionDraggingId),
    );
    if (!source) return;
    if (getBillingStatusBucket(source) !== getBillingStatusBucket(subscription))
      return;
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
    setRequestSubscriptionDropTargetId(targetId);
  };

  const handleRequestSubscriptionDrop = async (event, subscription) => {
    event.preventDefault();
    await submitRequestSubscriptionReorder(
      requestSubscriptionDraggingId,
      subscription?.id,
    );
  };

  const submitPaygReorder = async (sourceProductId, targetProductId) => {
    if (!userId || paygReordering) {
      clearPaygDragState();
      return;
    }
    const currentIds = normalizeOrderedUniqueNonZeroIds(
      paygBalanceItems.map((item) => Number(item?.product_id ?? 0)),
    );
    const nextIds = reorderIdsBefore(
      currentIds,
      sourceProductId,
      targetProductId,
    );
    clearPaygDragState();
    if (nextIds.join(',') === currentIds.join(',')) return;

    setPaygReordering(true);
    try {
      const res = await API.patch(`/api/user/${userId}/payg/balances/reorder`, {
        product_ids: nextIds,
      });
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('保存排序失败'));
        return;
      }
      showSuccess(t('按量付费扣费顺序已更新'));
      await refreshUserBillingSnapshot();
      props.refresh();
    } catch (error) {
      showError(error?.message || t('保存排序失败'));
    } finally {
      setPaygReordering(false);
    }
  };

  const handlePaygDragStart = (event, balance) => {
    if (paygReordering) return;
    const productId = Number(balance?.product_id ?? 0);
    if (!Number.isFinite(productId) || productId === 0) return;
    setPaygDraggingProductId(productId);
    setPaygDropTargetProductId(productId);
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', String(productId));
    }
  };

  const handlePaygDragOver = (event, balance) => {
    if (!paygDraggingProductId || paygReordering) return;
    const productId = Number(balance?.product_id ?? 0);
    if (!Number.isFinite(productId) || productId === 0) return;
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
    setPaygDropTargetProductId(productId);
  };

  const handlePaygDrop = async (event, balance) => {
    event.preventDefault();
    await submitPaygReorder(paygDraggingProductId, balance?.product_id);
  };

  const subscriptionPresetById = useMemo(() => {
    const m = {};
    (Array.isArray(subscriptionPresets) ? subscriptionPresets : []).forEach(
      (p) => {
        const id = Number(p?.id ?? 0);
        if (!Number.isFinite(id) || id <= 0) return;
        m[id] = p;
      },
    );
    return m;
  }, [subscriptionPresets]);

  const subscriptionPresetOptionList = useMemo(() => {
    return (Array.isArray(subscriptionPresets) ? subscriptionPresets : [])
      .map((p) => {
        const id = Number(p?.id ?? 0);
        if (!Number.isFinite(id) || id <= 0) return null;
        const name = String(p?.name ?? '').trim();
        if (!name) return null;
        const enabled = p?.enabled !== false;
        return {
          label: enabled ? name : `${name} (${t('已下架')})`,
          value: id,
        };
      })
      .filter(Boolean);
  }, [subscriptionPresets, t]);

  const requestSubscriptionPresetById = useMemo(() => {
    const m = {};
    (Array.isArray(requestSubscriptionPresets)
      ? requestSubscriptionPresets
      : []
    ).forEach((p) => {
      const id = Number(p?.id ?? 0);
      if (!Number.isFinite(id) || id <= 0) return;
      m[id] = p;
    });
    return m;
  }, [requestSubscriptionPresets]);

  const requestSubscriptionPresetOptionList = useMemo(() => {
    return (
      Array.isArray(requestSubscriptionPresets)
        ? requestSubscriptionPresets
        : []
    )
      .map((p) => {
        const id = Number(p?.id ?? 0);
        if (!Number.isFinite(id) || id <= 0) return null;
        const name = String(p?.name ?? '').trim();
        if (!name) return null;
        const enabled = p?.enabled !== false;
        return {
          label: enabled ? name : `${name} (${t('已下架')})`,
          value: id,
        };
      })
      .filter(Boolean);
  }, [requestSubscriptionPresets, t]);

  const paygProductById = useMemo(() => {
    const m = {};
    (Array.isArray(paygProducts) ? paygProducts : []).forEach((p) => {
      const id = Number(p?.id ?? 0);
      if (!Number.isFinite(id) || id <= 0) return;
      m[id] = p;
    });
    return m;
  }, [paygProducts]);

  const paygProductOptionList = useMemo(() => {
    return (Array.isArray(paygProducts) ? paygProducts : [])
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      })
      .map((p) => {
        const id = Number(p?.id ?? 0);
        if (!Number.isFinite(id) || id <= 0) return null;
        const name = String(p?.name ?? '').trim();
        if (!name) return null;
        const enabled = p?.enabled !== false;
        return {
          label: enabled ? name : `${name} (${t('已下架')})`,
          value: id,
        };
      })
      .filter(Boolean);
  }, [paygProducts, t]);

  const paygBalanceItems = useMemo(() => {
    const list = userDetail?.payg_balances;
    if (!Array.isArray(list)) return [];
    return list
      .map((item) => {
        const productId = Number(item?.product_id ?? 0);
        if (!Number.isFinite(productId) || productId === 0) return null;
        const name = String(item?.product_name ?? '').trim();
        const sortOrderRaw = Number(item?.sort_order ?? 0);
        const sortOrder = Number.isFinite(sortOrderRaw)
          ? Math.max(0, Math.floor(sortOrderRaw))
          : 0;
        const remainingRaw = Number(item?.remaining_quota ?? 0);
        const remaining = Number.isFinite(remainingRaw)
          ? Math.max(0, Math.floor(remainingRaw))
          : 0;
        const createdAtRaw = Number(item?.created_at ?? 0);
        const createdAt = Number.isFinite(createdAtRaw)
          ? Math.max(0, Math.floor(createdAtRaw))
          : 0;
        const updatedAtRaw = Number(item?.updated_at ?? 0);
        const updatedAt = Number.isFinite(updatedAtRaw)
          ? Math.max(0, Math.floor(updatedAtRaw))
          : 0;
        const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);

        return {
          product_id: productId,
          product_name: name,
          sort_order: sortOrder,
          remaining_quota: remaining,
          created_at: createdAt,
          updated_at: updatedAt,
          allowed_group_ids: allowedGroupIds,
        };
      })
      .filter(Boolean)
      .filter((b) => b.remaining_quota > 0)
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.product_id ?? 0) || 0;
        const ib = Number(b?.product_id ?? 0) || 0;
        return ib - ia;
      });
  }, [userDetail?.payg_balances]);

  const renderSubscriptionCards = () => (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-start justify-between gap-3 mb-2'>
        <div className='flex items-center min-w-0'>
          <Avatar size='small' color='purple' className='mr-2 shadow-md'>
            <IconUserGroup size={16} />
          </Avatar>
          <div className='min-w-0'>
            <Text className='text-lg font-medium'>{t('订阅额度')}</Text>
            <div className='text-xs text-gray-500 mt-0.5'>
              {subscriptionReordering
                ? t('正在保存扣费顺序...')
                : t('可拖拽调整扣费顺序（生效中 / 待生效）')}
            </div>
          </div>
        </div>
        <Dropdown
          trigger='click'
          position='bottomRight'
          render={
            <Dropdown.Menu>
              <Dropdown.Item
                disabled={subscriptionPresetsLoading}
                onClick={() => void openSubscriptionPresetModal()}
              >
                {t('从商品创建')}
              </Dropdown.Item>
              <Dropdown.Item onClick={() => openSubscriptionModal(null)}>
                {t('手动创建')}
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <Button icon={<IconPlus />} disabled={!userId}>
            <span className='inline-flex items-center gap-1'>
              {t('新增订阅额度')}
              <IconTreeTriangleDown size={14} />
            </span>
          </Button>
        </Dropdown>
      </div>
      <Divider margin={12} />
      {subscriptionsLoading ? (
        <div className='py-8 flex justify-center'>
          <Spin />
        </div>
      ) : (
        <>
          {subscriptionConfigErrors.length > 0 ? (
            <div className='mb-3 text-xs text-red-500 whitespace-pre-wrap'>
              {subscriptionConfigErrors.slice(0, 5).map((msg) => (
                <div key={msg}>{msg}</div>
              ))}
              {subscriptionConfigErrors.length > 5 ? (
                <div>{t('更多错误已省略')}...</div>
              ) : null}
            </div>
          ) : null}
          {subscriptions.length === 0 ? (
            <Empty description={t('暂无订阅')} />
          ) : (
            subscriptions.map((subscription) => {
              const groupQuotaBreakdownRaw = Array.isArray(
                subscription?.group_quota_breakdown,
              )
                ? subscription.group_quota_breakdown
                : [];
              const groupQuotaBreakdown = groupQuotaBreakdownRaw
                .map((row) => {
                  const rawId = Number(row?.group_id ?? 0);
                  const groupId = Number.isFinite(rawId)
                    ? Math.floor(rawId)
                    : 0;
                  if (groupId <= 0) return null;
                  const dailyUsedRaw = Number(row?.daily_quota_used ?? 0);
                  const dailyAvailRaw = Number(row?.daily_quota_available ?? 0);
                  const dailyLimitRaw = Number(row?.daily_quota_limit ?? 0);
                  return {
                    group_id: groupId,
                    daily_quota_used: Number.isFinite(dailyUsedRaw)
                      ? Math.max(0, Math.floor(dailyUsedRaw))
                      : 0,
                    daily_quota_available: Number.isFinite(dailyAvailRaw)
                      ? Math.max(0, Math.floor(dailyAvailRaw))
                      : 0,
                    daily_quota_limit: Number.isFinite(dailyLimitRaw)
                      ? Math.max(0, Math.floor(dailyLimitRaw))
                      : 0,
                  };
                })
                .filter(Boolean)
                .sort((a, b) => a.group_id - b.group_id);
              const nowUnix = Math.floor(Date.now() / 1000);
              const isExpired =
                Number(subscription?.expire_at ?? 0) > 0 &&
                Number(subscription?.expire_at ?? 0) < nowUnix;
              const isPending = Number(subscription?.start_at ?? 0) > nowUnix;
              const statusBucket = getBillingStatusBucket(
                subscription,
                nowUnix,
              );
              const canReorder = statusBucket !== 'expired';
              const isDragging = subscriptionDraggingId === subscription.id;
              const isDropTarget =
                subscriptionDropTargetId === subscription.id &&
                subscriptionDraggingId > 0 &&
                subscriptionDraggingId !== subscription.id;
              const isDepleted =
                !isExpired &&
                !isPending &&
                Number(subscription?.remaining_quota ?? 0) <= 0;
              const statusColor = isExpired
                ? 'grey'
                : isPending
                  ? 'orange'
                  : isDepleted
                    ? 'red'
                    : 'green';
              const statusLabel = isExpired
                ? t('已过期')
                : isPending
                  ? t('待生效')
                  : isDepleted
                    ? t('已用尽')
                    : t('生效中');
              const periodStart = subscription.start_at
                ? formatDateLabel(subscription.start_at)
                : '';
              const periodEnd = subscription.expire_at
                ? formatDateLabel(subscription.expire_at)
                : t('不限时');
              return (
                <Card
                  key={subscription.id}
                  draggable={canReorder && !subscriptionReordering}
                  onDragStart={(event) =>
                    handleSubscriptionDragStart(event, subscription)
                  }
                  onDragOver={(event) =>
                    handleSubscriptionDragOver(event, subscription)
                  }
                  onDrop={(event) =>
                    void handleSubscriptionDrop(event, subscription)
                  }
                  onDragEnd={clearSubscriptionDragState}
                  className={`mb-3 !rounded-xl border border-dashed ${
                    isDropTarget
                      ? 'border-blue-400 bg-blue-50/60'
                      : 'border-gray-200'
                  } ${
                    canReorder && !subscriptionReordering ? 'cursor-move' : ''
                  } ${isDragging ? 'opacity-70 ring-1 ring-blue-200' : ''}`}
                  bodyStyle={{ padding: '12px 16px' }}
                >
                  <div className='flex justify-between items-start'>
                    <div>
                      <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
                        <Text className='font-medium'>
                          {t('订阅额度 {{index}}', {
                            index: subscription.index,
                          })}
                        </Text>
                        <Tag color={statusColor}>{statusLabel}</Tag>
                        <div className='text-xs text-gray-500'>
                          (
                          {periodStart
                            ? `${periodStart} - ${periodEnd}`
                            : periodEnd}
                          )
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('总额度')}：{renderQuota(subscription.total_quota)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('剩余额度')}：
                          {renderQuota(subscription.remaining_quota)}
                        </div>
                      </div>
                    </div>
                    <Space>
                      <Button
                        size='small'
                        icon={<IconEdit />}
                        onClick={() => openSubscriptionModal(subscription)}
                        disabled={subscriptionReordering}
                      >
                        {t('编辑')}
                      </Button>
                      <Button
                        size='small'
                        type='danger'
                        icon={<IconDelete />}
                        onClick={() => handleDeleteSubscription(subscription)}
                        disabled={subscriptionReordering}
                      >
                        {t('删除')}
                      </Button>
                    </Space>
                  </div>
                  {groupQuotaBreakdown.length > 0 ? (
                    <div className='mt-3 text-xs text-gray-600'>
                      <div
                        className='grid gap-x-8 gap-y-1 text-[11px]'
                        style={{
                          gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
                        }}
                      >
                        <div className='truncate text-neutral-500 dark:text-neutral-400'>
                          {t('分组')}
                        </div>
                        <div className='truncate text-neutral-500 dark:text-neutral-400'>
                          {t('当日消耗')}
                        </div>
                        <div className='truncate text-neutral-500 dark:text-neutral-400'>
                          {t('当日剩余')}
                        </div>
                        <div className='truncate text-neutral-500 dark:text-neutral-400'>
                          {t('当日限额')}
                        </div>
                        {groupQuotaBreakdown.map((row) => (
                          <React.Fragment
                            key={`sub-${subscription.id}-group-${row.group_id}`}
                          >
                            <div className='font-semibold text-neutral-800 dark:text-neutral-100'>
                              {formatGroupRatioLabel(row.group_id)}
                            </div>
                            <div className='font-semibold text-neutral-800 dark:text-neutral-100'>
                              {renderQuota(row.daily_quota_used)}
                            </div>
                            <div className='font-semibold text-neutral-800 dark:text-neutral-100'>
                              {renderQuota(row.daily_quota_available)}
                            </div>
                            <div className='font-semibold text-neutral-800 dark:text-neutral-100'>
                              {row.daily_quota_limit > 0
                                ? renderQuota(row.daily_quota_limit)
                                : t('不限额')}
                            </div>
                          </React.Fragment>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  {subscription.source && (
                    <div className='text-xs text-gray-400 mt-2'>
                      {t('来源')}：{subscription.source}
                    </div>
                  )}
                </Card>
              );
            })
          )}
        </>
      )}
    </Card>
  );

  const renderRequestSubscriptionCards = () => (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-start justify-between gap-3 mb-2'>
        <div className='flex items-center min-w-0'>
          <Avatar size='small' color='orange' className='mr-2 shadow-md'>
            <IconUserGroup size={16} />
          </Avatar>
          <div className='min-w-0'>
            <Text className='text-lg font-medium'>{t('次数订阅')}</Text>
            <div className='text-xs text-gray-500 mt-0.5'>
              {requestSubscriptionReordering
                ? t('正在保存扣费顺序...')
                : t('可拖拽调整扣费顺序（生效中 / 待生效）')}
            </div>
            {requestSubscriptionSummary ? (
              <div className='text-xs text-gray-500 mt-0.5'>
                {requestSubscriptionSummary.limitUnlimited ? (
                  <>
                    {t('当日消耗')}：
                    {renderNumber(requestSubscriptionSummary.usedTotal)}，
                    {t('当日限额')}：{t('无限')}
                  </>
                ) : (
                  <>
                    {t('当日剩余')}：
                    {renderNumber(requestSubscriptionSummary.remainingTotal)} /{' '}
                    {renderNumber(requestSubscriptionSummary.limitTotal)}
                  </>
                )}
              </div>
            ) : null}
          </div>
        </div>
        <Dropdown
          trigger='click'
          position='bottomRight'
          render={
            <Dropdown.Menu>
              <Dropdown.Item
                disabled={subscriptionPresetsLoading}
                onClick={() => void openRequestSubscriptionPresetModal()}
              >
                {t('从商品创建')}
              </Dropdown.Item>
              <Dropdown.Item onClick={() => openRequestSubscriptionModal(null)}>
                {t('手动创建')}
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <Button icon={<IconPlus />} disabled={!userId}>
            <span className='inline-flex items-center gap-1'>
              {t('新增次数订阅')}
              <IconTreeTriangleDown size={14} />
            </span>
          </Button>
        </Dropdown>
      </div>
      <Divider margin={12} />
      {requestSubscriptionsLoading ? (
        <div className='py-8 flex justify-center'>
          <Spin />
        </div>
      ) : (
        <>
          {requestSubscriptions.length === 0 ? (
            <Empty description={t('暂无订阅')} />
          ) : (
            requestSubscriptions.map((subscription) => {
              const nowUnix = Math.floor(Date.now() / 1000);
              const isExpired =
                Number(subscription?.expire_at ?? 0) > 0 &&
                Number(subscription?.expire_at ?? 0) < nowUnix;
              const isPending = Number(subscription?.start_at ?? 0) > nowUnix;
              const statusBucket = getBillingStatusBucket(
                subscription,
                nowUnix,
              );
              const canReorder = statusBucket !== 'expired';
              const isDragging =
                requestSubscriptionDraggingId === subscription.id;
              const isDropTarget =
                requestSubscriptionDropTargetId === subscription.id &&
                requestSubscriptionDraggingId > 0 &&
                requestSubscriptionDraggingId !== subscription.id;
              const dailyLimit =
                Number(subscription?.daily_request_limit ?? 0) || 0;
              const dailyUnlimited = dailyLimit === 0;
              const dailyUsed =
                Number(subscription?.daily_request_used ?? 0) || 0;
              const dailyRemaining =
                Number(subscription?.daily_request_remaining ?? 0) || 0;

              const totalLimit =
                Number(subscription?.total_request_limit ?? 0) || 0;
              const totalUnlimited = totalLimit === 0;
              const totalUsed =
                Number(subscription?.total_request_used ?? 0) || 0;
              const totalRemaining =
                Number(subscription?.total_request_remaining ?? 0) || 0;

              const totalDepleted = !totalUnlimited && totalRemaining <= 0;
              const dailyDepleted = !dailyUnlimited && dailyRemaining <= 0;
              const isDepleted =
                !isExpired && !isPending && (totalDepleted || dailyDepleted);
              const statusColor = isExpired
                ? 'grey'
                : isPending
                  ? 'orange'
                  : isDepleted
                    ? 'red'
                    : 'green';
              const statusLabel = isExpired
                ? t('已过期')
                : isPending
                  ? t('待生效')
                  : isDepleted
                    ? totalDepleted
                      ? t('总次数已用尽')
                      : t('当日已用尽')
                    : t('生效中');
              const periodStart = subscription.start_at
                ? formatDateLabel(subscription.start_at)
                : '';
              const periodEnd = subscription.expire_at
                ? formatDateLabel(subscription.expire_at)
                : t('不限时');
              const groups = normalizeGroupIds(subscription?.allowed_group_ids);

              return (
                <Card
                  key={subscription.id}
                  draggable={canReorder && !requestSubscriptionReordering}
                  onDragStart={(event) =>
                    handleRequestSubscriptionDragStart(event, subscription)
                  }
                  onDragOver={(event) =>
                    handleRequestSubscriptionDragOver(event, subscription)
                  }
                  onDrop={(event) =>
                    void handleRequestSubscriptionDrop(event, subscription)
                  }
                  onDragEnd={clearRequestSubscriptionDragState}
                  className={`mb-3 !rounded-xl border border-dashed ${
                    isDropTarget
                      ? 'border-orange-400 bg-orange-50/60'
                      : 'border-gray-200'
                  } ${
                    canReorder && !requestSubscriptionReordering
                      ? 'cursor-move'
                      : ''
                  } ${isDragging ? 'opacity-70 ring-1 ring-orange-200' : ''}`}
                  bodyStyle={{ padding: '12px 16px' }}
                >
                  <div className='flex justify-between items-start gap-3'>
                    <div className='min-w-0'>
                      <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
                        <Text className='font-medium'>
                          {t('次数订阅 {{index}}', {
                            index: subscription.index,
                          })}
                        </Text>
                        <Tag color={statusColor}>{statusLabel}</Tag>
                        <div className='text-xs text-gray-500'>
                          (
                          {periodStart
                            ? `${periodStart} - ${periodEnd}`
                            : periodEnd}
                          )
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('当日限额')}：
                          {dailyUnlimited
                            ? t('无限')
                            : renderNumber(dailyLimit)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('当日消耗')}：{renderNumber(dailyUsed)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('当日剩余')}：
                          {dailyUnlimited
                            ? t('无限')
                            : renderNumber(dailyRemaining)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('总限额')}：
                          {totalUnlimited
                            ? t('无限')
                            : renderNumber(totalLimit)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('总消耗')}：
                          {totalUnlimited ? '-' : renderNumber(totalUsed)}
                        </div>
                        <div className='text-xs text-gray-500'>
                          {t('总剩余')}：
                          {totalUnlimited
                            ? t('无限')
                            : renderNumber(totalRemaining)}
                        </div>
                      </div>

                      {groups.length > 0 ? (
                        <div className='mt-2 flex flex-wrap items-center gap-1'>
                          <span className='text-xs text-gray-500'>
                            {t('可用分组')}：
                          </span>
                          <span className='inline-flex flex-wrap gap-1'>
                            {groups.map((g) => (
                              <span
                                key={`req-sub-${subscription.id}-${g}`}
                              >
                                {formatGroupRatioLabel(g)}
                              </span>
                            ))}
                          </span>
                        </div>
                      ) : null}

                      {subscription.source ? (
                        <div className='text-xs text-gray-400 mt-2'>
                          {t('来源')}：{subscription.source}
                        </div>
                      ) : null}
                    </div>
                    <Space>
                      <Button
                        size='small'
                        icon={<IconEdit />}
                        onClick={() =>
                          openRequestSubscriptionModal(subscription)
                        }
                        disabled={requestSubscriptionReordering}
                      >
                        {t('编辑')}
                      </Button>
                      <Button
                        size='small'
                        type='danger'
                        icon={<IconDelete />}
                        onClick={() =>
                          handleDeleteRequestSubscription(subscription)
                        }
                        disabled={requestSubscriptionReordering}
                      >
                        {t('删除')}
                      </Button>
                    </Space>
                  </div>
                </Card>
              );
            })
          )}
        </>
      )}
    </Card>
  );

  const subscriptionModalTitle = editingSubscription
    ? t('编辑订阅额度')
    : t('新增订阅额度');

  const requestSubscriptionModalTitle = editingRequestSubscription
    ? t('编辑次数订阅')
    : t('新增次数订阅');

  return (
    <>
      <SideSheet
        placement='right'
        title={
          <Space>
            <Tag color='blue' shape='circle'>
              {t(isEdit ? '编辑' : '新建')}
            </Tag>
            <Title heading={4} className='m-0'>
              {isEdit ? t('编辑用户') : t('创建用户')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: 0 }}
        visible={props.visible}
        width={isMobile ? '100%' : 640}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={props.handleClose}
                icon={<IconClose />}
                disabled={loading}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={props.handleClose}
      >
        <Spin spinning={loading}>
          <Form
            initValues={getInitValues()}
            getFormApi={(api) => (formApiRef.current = api)}
            onValueChange={(allValues) => {
              setPricingFormValues((prev) => ({
                ...prev,
                customer_type:
                  allValues?.customer_type ?? prev.customer_type ?? 'retail',
                pricing_profile_id:
                  Number(
                    allValues?.pricing_profile_id ??
                      prev.pricing_profile_id ??
                      0,
                  ) || 0,
                base_multiplier:
                  Number(
                    allValues?.base_multiplier ?? prev.base_multiplier ?? 1,
                  ) > 0
                    ? Number(
                        allValues?.base_multiplier ?? prev.base_multiplier ?? 1,
                      )
                    : 1,
              }));
            }}
            onSubmit={submit}
          >
            {({ values }) => {
              return (
                <div className='p-3 space-y-3'>
                  <Card className='!rounded-2xl shadow-sm border-0'>
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='blue'
                        className='mr-2 shadow-md'
                      >
                        <IconUser size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('基本信息')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('用户的基本账户信息')}
                        </div>
                      </div>
                    </div>
                    <Divider margin={12} />
                    <Row gutter={12}>
                      <Col span={24}>
                        <Form.Input
                          field='username'
                          label={t('用户名')}
                          placeholder={t('请输入新的用户名')}
                          autoComplete='username'
                          rules={[
                            { required: true, message: t('请输入用户名') },
                          ]}
                          showClear
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Input
                          field='password'
                          label={t('密码')}
                          placeholder={t('请输入新的密码，最短 8 位')}
                          autoComplete='new-password'
                          mode='password'
                          showClear
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Input
                          field='display_name'
                          label={t('显示名称')}
                          placeholder={t('请输入新的显示名称')}
                          showClear
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Input
                          field='remark'
                          label={t('备注')}
                          placeholder={t('请输入备注（仅管理员可见）')}
                          showClear
                        />
                      </Col>
                    </Row>
                  </Card>

                  {userId && (
                    <Card className='!rounded-2xl shadow-sm border-0'>
                      <div className='flex items-center mb-2'>
                        <Avatar
                          size='small'
                          color='green'
                          className='mr-2 shadow-md'
                        >
                          <IconUserGroup size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('用户分组')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('仅用于筛选与运营标签，不影响模型消费')}
                          </div>
                        </div>
                      </div>
                      <Divider margin={12} />
                      <Row gutter={12}>
                        <Col span={24}>
                          <Form.Select
                            field='user_group_id'
                            label={t('用户分组')}
                            placeholder={t('可选：仅用于筛选与运营标签')}
                            optionList={userGroupIdOptions}
                            loading={userGroupOptionsLoading}
                            search
                            allowClear
                          />
                        </Col>
                      </Row>
                    </Card>
                  )}

                  {canEditAdminPermissions && (
                    <Card className='!rounded-2xl shadow-sm border-0'>
                      <div className='flex items-center mb-2'>
                        <Avatar
                          size='small'
                          color='orange'
                          className='mr-2 shadow-md'
                        >
                          <IconLink size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('管理员模块权限')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('仅超级管理员可调整普通管理员可访问的后台模块')}
                          </div>
                        </div>
                      </div>
                      <Divider margin={12} />
                      <div className='mb-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700'>
                        {t(
                          '这里控制真实访问权限；即使全局侧边栏开启，未授权的管理员也无法进入对应模块。',
                        )}
                      </div>
                      <Row gutter={12}>
                        <Col span={12}>
                          <Form.Switch
                            field='admin_permissions.product_management'
                            label={t('商品管理')}
                          />
                        </Col>
                        <Col span={12}>
                          <Form.Switch
                            field='admin_permissions.order'
                            label={t('订单管理')}
                          />
                        </Col>
                      </Row>
                    </Card>
                  )}

                  {userId && (
                    <Card className='!rounded-2xl shadow-sm border-0'>
                      <div className='flex items-center mb-2'>
                        <Avatar
                          size='small'
                          color='purple'
                          className='mr-2 shadow-md'
                        >
                          <IconLink size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('邀请绑定')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('查看邀请码并调整当前用户的邀请人')}
                          </div>
                        </div>
                      </div>
                      <Divider margin={12} />
                      <Row gutter={12}>
                        <Col span={12}>
                          <Form.Slot label={t('邀请码')}>
                            <Text code>{values?.aff_code || '-'}</Text>
                          </Form.Slot>
                        </Col>
                        <Col span={12}>
                          <Form.Slot label={t('邀请人数')}>
                            <Text>
                              {renderNumber(
                                Number(values?.aff_count ?? 0) || 0,
                              )}
                            </Text>
                          </Form.Slot>
                        </Col>
                        <Col span={24}>
                          <Form.InputNumber
                            field='inviter_id'
                            label={t('邀请人 ID')}
                            placeholder={t('0 表示无邀请人')}
                            min={0}
                            precision={0}
                            style={{ width: '100%' }}
                            extraText={t(
                              '保存后会同步更新新旧邀请人的邀请人数统计',
                            )}
                          />
                        </Col>
                      </Row>
                    </Card>
                  )}

                  {/* 按量付费独立卡片 */}
                  <Card className='!rounded-2xl shadow-sm border-0'>
                    <div className='flex items-start justify-between gap-3 mb-2'>
                      <div className='flex items-center min-w-0'>
                        <Avatar
                          size='small'
                          color='cyan'
                          className='mr-2 shadow-md'
                        >
                          <IconUserGroup size={16} />
                        </Avatar>
                        <div className='min-w-0'>
                          <Text className='text-lg font-medium'>
                            {t('按量付费')}
                          </Text>
                          <div className='text-xs text-gray-500 mt-0.5'>
                            {paygReordering
                              ? t('正在保存扣费顺序...')
                              : t('可拖拽调整扣费顺序')}
                          </div>
                        </div>
                      </div>
                      <Dropdown
                        trigger='click'
                        position='bottomRight'
                        render={
                          <Dropdown.Menu>
                            <Dropdown.Item
                              onClick={() => void openPaygTopupModal()}
                            >
                              {t('从商品创建')}
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => openPaygManualModal()}
                            >
                              {t('手动创建')}
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        }
                      >
                        <Button
                          icon={<IconPlus />}
                          disabled={!userId}
                          loading={
                            paygProductsLoading && paygProducts.length === 0
                          }
                          htmlType='button'
                        >
                          <span className='inline-flex items-center gap-1'>
                            {t('新增按量付费')}
                            <IconTreeTriangleDown size={14} />
                          </span>
                        </Button>
                      </Dropdown>
                    </div>
                    <Divider margin={12} />
                    {paygBalanceItems.length === 0 ? (
                      <Empty description={t('暂无按量付费余额')} />
                    ) : (
                      <div className='mt-3 space-y-2'>
                        {paygBalanceItems.map((b, idx) => {
                          const paidAt = formatDateLabel(
                            b.updated_at || b.created_at,
                          );
                          const isDragging =
                            paygDraggingProductId === b.product_id;
                          const isDropTarget =
                            paygDropTargetProductId === b.product_id &&
                            paygDraggingProductId !== 0 &&
                            paygDraggingProductId !== b.product_id;
                          return (
                            <div
                              key={`payg-${b.product_id}`}
                              draggable={!paygReordering}
                              onDragStart={(event) =>
                                handlePaygDragStart(event, b)
                              }
                              onDragOver={(event) =>
                                handlePaygDragOver(event, b)
                              }
                              onDrop={(event) => void handlePaygDrop(event, b)}
                              onDragEnd={clearPaygDragState}
                              className={`flex items-start justify-between gap-3 rounded-lg px-3 py-2 ${
                                isDropTarget
                                  ? 'border border-cyan-400 bg-cyan-50/70'
                                  : 'bg-[var(--app-card-muted)]'
                              } ${!paygReordering ? 'cursor-move' : ''} ${
                                isDragging
                                  ? 'opacity-70 ring-1 ring-cyan-200'
                                  : ''
                              }`}
                            >
                              <div className='min-w-0'>
                                <div className='text-xs font-medium text-neutral-800 dark:text-neutral-100 truncate'>
                                  {t('按量付费 {{index}}', { index: idx + 1 })}
                                </div>
                                {b.product_name ? (
                                  <div className='mt-0.5 text-[11px] text-neutral-500 dark:text-neutral-400 truncate'>
                                    {b.product_name}
                                  </div>
                                ) : null}
                                <div className='mt-1 text-[11px] text-neutral-500 dark:text-neutral-400'>
                                  {t('支付时间')}：{paidAt || t('未知')}
                                </div>
                                {normalizeGroupIds(b.allowed_group_ids).length >
                                0 ? (
                                  <div className='mt-1 flex flex-wrap items-center gap-1'>
                                    {normalizeGroupIds(b.allowed_group_ids).map(
                                      (gid) => (
                                        <span
                                          key={`payg-${b.product_id}-${gid}`}
                                        >
                                          {formatGroupRatioLabel(gid)}
                                        </span>
                                      ),
                                    )}
                                  </div>
                                ) : null}
                              </div>
                              <div className='shrink-0 text-right'>
                                <div className='text-[11px] text-neutral-500 dark:text-neutral-400'>
                                  {t('剩余额度')}
                                </div>
                                <div className='text-sm font-semibold text-neutral-800 dark:text-neutral-100'>
                                  {renderQuotaToUSD(b.remaining_quota)}
                                </div>
                                <Space spacing={8} className='mt-2 justify-end'>
                                  <Button
                                    size='small'
                                    icon={<IconEdit />}
                                    onClick={() => openPaygBalanceGroupModal(b)}
                                    disabled={paygReordering}
                                  >
                                    {t('编辑分组')}
                                  </Button>
                                  <Button
                                    size='small'
                                    type='danger'
                                    icon={<IconDelete />}
                                    onClick={() => handleDeletePaygBalance(b)}
                                    disabled={paygReordering}
                                  >
                                    {t('删除')}
                                  </Button>
                                </Space>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </Card>

                  {renderSubscriptionCards()}
                  {renderRequestSubscriptionCards()}

                  <Card className='!rounded-2xl shadow-sm border-0'>
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='purple'
                        className='mr-2 shadow-md'
                      >
                        <IconLink size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('绑定信息')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('第三方账户绑定状态（只读）')}
                        </div>
                      </div>
                    </div>
                    <Divider margin={12} />
                    <Row gutter={12}>
                      {[
                        'github_id',
                        'oidc_id',
                        'wechat_id',
                        'email',
                        'telegram_id',
                      ].map((field) => (
                        <Col span={24} key={field}>
                          <Form.Input
                            field={field}
                            label={t(
                              `已绑定的 ${field
                                .replace('_id', '')
                                .toUpperCase()} 账户`,
                            )}
                            readonly
                            placeholder={t(
                              '此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改',
                            )}
                          />
                        </Col>
                      ))}
                    </Row>
                  </Card>
                </div>
              );
            }}
          </Form>
        </Spin>
      </SideSheet>

      <Modal
        centered
        visible={subscriptionPresetModalVisible}
        title={t('按商品新增订阅额度')}
        onCancel={() => setSubscriptionPresetModalVisible(false)}
        onOk={handleSubscriptionPresetSubmit}
        confirmLoading={subscriptionPresetSubmitting}
        width={560}
        okButtonProps={{
          disabled:
            subscriptionPresetsLoading ||
            subscriptionPresetSubmitting ||
            subscriptionPresets.length === 0,
        }}
      >
        <Form
          key={subscriptionPresetFormKey}
          initValues={subscriptionPresetInitValues}
          getFormApi={(api) => (subscriptionPresetFormApiRef.current = api)}
        >
          {({ values }) => {
            const presetId = parseInt(values?.preset_id, 10);
            const preset =
              Number.isFinite(presetId) && presetId > 0
                ? subscriptionPresetById[presetId]
                : null;

            const groupIds = normalizeGroupIds(preset?.allowed_group_ids);
            const groupDailyLimits = normalizeGroupDailyLimits(
              preset?.group_daily_limits,
            );
            const quota = Number(preset?.quota ?? 0) || 0;
            const quotaValidDays = Number(preset?.quota_valid_days ?? 0) || 0;
            const dailyLimit = Number(preset?.daily_quota_limit ?? 0) || 0;
            const multiEnabled = Boolean(preset?.multi_quantity_enabled);
            const deferOnly = Boolean(preset?.multi_quantity_defer_only);
            const qtyRaw = Number(values?.quantity ?? 1);
            const qty = Number.isFinite(qtyRaw)
              ? Math.max(1, Math.floor(qtyRaw))
              : 1;
            const applyMode = String(values?.apply_mode || 'stack').trim();

            return (
              <div className='space-y-3'>
                <Form.Select
                  field='preset_id'
                  label={t('订阅商品')}
                  placeholder={t('请选择订阅商品')}
                  optionList={subscriptionPresetOptionList}
                  search
                  disabled={
                    subscriptionPresetsLoading ||
                    subscriptionPresets.length === 0
                  }
                  rules={[{ required: true, message: t('请选择订阅商品') }]}
                  onChange={(val) => {
                    if (!subscriptionPresetFormApiRef.current) return;
                    const id = parseInt(val, 10);
                    const selected =
                      Number.isFinite(id) && id > 0
                        ? subscriptionPresetById[id]
                        : null;
                    if (!selected?.multi_quantity_enabled) {
                      subscriptionPresetFormApiRef.current.setValue(
                        'quantity',
                        1,
                      );
                    }
                    const currentQty = Number(
                      subscriptionPresetFormApiRef.current.getValue(
                        'quantity',
                      ) ?? 1,
                    );
                    if (selected?.multi_quantity_defer_only && currentQty > 1) {
                      subscriptionPresetFormApiRef.current.setValue(
                        'apply_mode',
                        'defer',
                      );
                    }
                  }}
                  style={{ width: '100%' }}
                />

                {preset ? (
                  <div className='rounded-lg bg-[var(--app-card-muted)] p-3 text-xs text-neutral-600 dark:text-neutral-300 space-y-1'>
                    {preset.description ? (
                      <div className='text-neutral-500 dark:text-neutral-400 whitespace-pre-wrap'>
                        {preset.description}
                      </div>
                    ) : null}
                    <div>
                      <span className='text-neutral-500 dark:text-neutral-400'>
                        {t('额度')}：
                      </span>
                      <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                        {renderQuotaToUSD(quota)}
                      </span>
                    </div>
                    {groupDailyLimits.length > 0 ? (
                      <div className='space-y-1'>
                        <div className='text-neutral-500 dark:text-neutral-400'>
                          {t('日限（按分组）')}：
                        </div>
                        <div className='flex flex-wrap items-center gap-1'>
                          {groupDailyLimits.map((item) => (
                            <Text
                              key={`preset-${preset.id}-${item.group_id}`}
                              code
                              style={{ fontSize: 12 }}
                            >
                              {`${
                                groupLabelById[item.group_id] || t('未知分组')
                              }: ${
                                item.daily_quota_limit <= 0
                                  ? t('无限')
                                  : renderQuotaToUSD(item.daily_quota_limit)
                              }`}
                            </Text>
                          ))}
                        </div>
                      </div>
                    ) : (
                      <div>
                        <span className='text-neutral-500 dark:text-neutral-400'>
                          {t('日限')}：
                        </span>
                        <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                          {dailyLimit <= 0
                            ? t('无限')
                            : renderQuotaToUSD(dailyLimit)}
                        </span>
                      </div>
                    )}
                    <div>
                      <span className='text-neutral-500 dark:text-neutral-400'>
                        {t('时长')}：
                      </span>
                      <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                        {quotaValidDays} {t('天')}
                      </span>
                    </div>
                    {groupIds.length > 0 ? (
                      <div className='flex flex-wrap items-center gap-1'>
                        <span className='text-neutral-500 dark:text-neutral-400'>
                          {t('可选分组')}：
                        </span>
                        <span className='inline-flex flex-wrap gap-1'>
                          {groupIds.map((gid) => (
                            <span
                              key={`preset-${preset.id}-${gid}`}
                            >
                              {formatGroupRatioLabel(gid)}
                            </span>
                          ))}
                        </span>
                      </div>
                    ) : null}
                  </div>
                ) : null}

                <Form.RadioGroup field='apply_mode' label={t('生效方式')}>
                  <Radio value='stack' disabled={Boolean(deferOnly && qty > 1)}>
                    {t('叠加（立即生效）')}
                  </Radio>
                  <Radio value='defer'>{t('顺延（到期后生效）')}</Radio>
                </Form.RadioGroup>

                <Form.InputNumber
                  field='quantity'
                  label={t('数量')}
                  placeholder={t('请输入数量')}
                  min={1}
                  max={multiEnabled ? 100 : 1}
                  step={1}
                  precision={0}
                  disabled={!multiEnabled}
                  extraText={
                    !multiEnabled
                      ? t('该商品不支持多数量')
                      : deferOnly
                        ? t('多数量仅支持顺延生效')
                        : ''
                  }
                  onChange={(val) => {
                    if (!subscriptionPresetFormApiRef.current) return;
                    const id = parseInt(
                      subscriptionPresetFormApiRef.current.getValue(
                        'preset_id',
                      ),
                      10,
                    );
                    const selected =
                      Number.isFinite(id) && id > 0
                        ? subscriptionPresetById[id]
                        : null;
                    const qtyValue = parseInt(val, 10);
                    if (selected?.multi_quantity_defer_only && qtyValue > 1) {
                      subscriptionPresetFormApiRef.current.setValue(
                        'apply_mode',
                        'defer',
                      );
                    }
                  }}
                  style={{ width: '100%' }}
                />

                {applyMode === 'defer' ? (
                  <div className='text-xs text-gray-500'>
                    {t(
                      '若当前仍有有效订阅额度，新创建的订阅将从到期后开始生效',
                    )}
                  </div>
                ) : (
                  <div className='text-xs text-gray-500'>
                    {t('订阅额度创建后将立即生效')}
                  </div>
                )}
              </div>
            );
          }}
        </Form>
      </Modal>

      <Modal
        centered
        visible={requestSubscriptionPresetModalVisible}
        title={t('按商品新增次数订阅')}
        onCancel={() => setRequestSubscriptionPresetModalVisible(false)}
        onOk={handleRequestSubscriptionPresetSubmit}
        confirmLoading={requestSubscriptionPresetSubmitting}
        width={560}
        okButtonProps={{
          disabled:
            subscriptionPresetsLoading ||
            requestSubscriptionPresetSubmitting ||
            requestSubscriptionPresets.length === 0,
        }}
      >
        <Form
          key={requestSubscriptionPresetFormKey}
          initValues={requestSubscriptionPresetInitValues}
          getFormApi={(api) =>
            (requestSubscriptionPresetFormApiRef.current = api)
          }
        >
          {({ values }) => {
            const presetId = parseInt(values?.preset_id, 10);
            const preset =
              Number.isFinite(presetId) && presetId > 0
                ? requestSubscriptionPresetById[presetId]
                : null;

            const groupIds = normalizeGroupIds(preset?.allowed_group_ids);
            const dailyRequestLimit =
              Number(preset?.daily_request_limit ?? 0) || 0;
            const requestQuota = Number(preset?.quota ?? 0) || 0;
            const quotaValidDays = Number(preset?.quota_valid_days ?? 0) || 0;
            const multiEnabled = Boolean(preset?.multi_quantity_enabled);
            const deferOnly = Boolean(preset?.multi_quantity_defer_only);
            const qtyRaw = Number(values?.quantity ?? 1);
            const qty = Number.isFinite(qtyRaw)
              ? Math.max(1, Math.floor(qtyRaw))
              : 1;
            const applyMode = String(values?.apply_mode || 'stack').trim();

            return (
              <div className='space-y-3'>
                <Form.Select
                  field='preset_id'
                  label={t('订阅商品')}
                  placeholder={t('请选择订阅商品')}
                  optionList={requestSubscriptionPresetOptionList}
                  search
                  disabled={
                    subscriptionPresetsLoading ||
                    requestSubscriptionPresets.length === 0
                  }
                  rules={[{ required: true, message: t('请选择订阅商品') }]}
                  onChange={(val) => {
                    if (!requestSubscriptionPresetFormApiRef.current) return;
                    const id = parseInt(val, 10);
                    const selected =
                      Number.isFinite(id) && id > 0
                        ? requestSubscriptionPresetById[id]
                        : null;
                    if (!selected?.multi_quantity_enabled) {
                      requestSubscriptionPresetFormApiRef.current.setValue(
                        'quantity',
                        1,
                      );
                    }
                    const currentQty = Number(
                      requestSubscriptionPresetFormApiRef.current.getValue(
                        'quantity',
                      ) ?? 1,
                    );
                    if (selected?.multi_quantity_defer_only && currentQty > 1) {
                      requestSubscriptionPresetFormApiRef.current.setValue(
                        'apply_mode',
                        'defer',
                      );
                    }
                  }}
                  style={{ width: '100%' }}
                />

                {preset ? (
                  <div className='rounded-lg bg-[var(--app-card-muted)] p-3 text-xs text-neutral-600 dark:text-neutral-300 space-y-1'>
                    {preset.description ? (
                      <div className='text-neutral-500 dark:text-neutral-400 whitespace-pre-wrap'>
                        {preset.description}
                      </div>
                    ) : null}
                    <div>
                      <span className='text-neutral-500 dark:text-neutral-400'>
                        {t('每日次数')}：
                      </span>
                      <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                        {dailyRequestLimit === 0
                          ? t('无限')
                          : renderNumber(dailyRequestLimit)}
                      </span>
                    </div>
                    <div>
                      <span className='text-neutral-500 dark:text-neutral-400'>
                        {t('总次数')}：
                      </span>
                      <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                        {requestQuota === 0
                          ? t('无限')
                          : renderNumber(requestQuota)}
                      </span>
                    </div>
                    <div>
                      <span className='text-neutral-500 dark:text-neutral-400'>
                        {t('时长')}：
                      </span>
                      <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                        {quotaValidDays} {t('天')}
                      </span>
                    </div>
                    {groupIds.length > 0 ? (
                      <div className='flex flex-wrap items-center gap-1'>
                        <span className='text-neutral-500 dark:text-neutral-400'>
                          {t('可选分组')}：
                        </span>
                        <span className='inline-flex flex-wrap gap-1'>
                          {groupIds.map((gid) => (
                            <span
                              key={`req-preset-${preset.id}-${gid}`}
                            >
                              {formatGroupRatioLabel(gid)}
                            </span>
                          ))}
                        </span>
                      </div>
                    ) : null}
                  </div>
                ) : null}

                <Form.RadioGroup field='apply_mode' label={t('生效方式')}>
                  <Radio value='stack' disabled={Boolean(deferOnly && qty > 1)}>
                    {t('叠加（立即生效）')}
                  </Radio>
                  <Radio value='defer'>{t('顺延（到期后生效）')}</Radio>
                </Form.RadioGroup>

                <Form.InputNumber
                  field='quantity'
                  label={t('数量')}
                  placeholder={t('请输入数量')}
                  min={1}
                  max={multiEnabled ? 100 : 1}
                  step={1}
                  precision={0}
                  disabled={!multiEnabled}
                  extraText={
                    !multiEnabled
                      ? t('该商品不支持多数量')
                      : deferOnly
                        ? t('多数量仅支持顺延生效')
                        : ''
                  }
                  onChange={(val) => {
                    if (!requestSubscriptionPresetFormApiRef.current) return;
                    const id = parseInt(
                      requestSubscriptionPresetFormApiRef.current.getValue(
                        'preset_id',
                      ),
                      10,
                    );
                    const selected =
                      Number.isFinite(id) && id > 0
                        ? requestSubscriptionPresetById[id]
                        : null;
                    const qtyValue = parseInt(val, 10);
                    if (selected?.multi_quantity_defer_only && qtyValue > 1) {
                      requestSubscriptionPresetFormApiRef.current.setValue(
                        'apply_mode',
                        'defer',
                      );
                    }
                  }}
                  style={{ width: '100%' }}
                />

                {applyMode === 'defer' ? (
                  <div className='text-xs text-gray-500'>
                    {t(
                      '若当前仍有有效次数订阅，新创建的订阅将从到期后开始生效',
                    )}
                  </div>
                ) : (
                  <div className='text-xs text-gray-500'>
                    {t('次数订阅创建后将立即生效')}
                  </div>
                )}
              </div>
            );
          }}
        </Form>
      </Modal>

      <Modal
        centered
        visible={paygBalanceGroupModalVisible}
        title={t('编辑按量付费分组')}
        onCancel={() => setPaygBalanceGroupModalVisible(false)}
        onOk={handlePaygBalanceGroupSubmit}
        confirmLoading={paygBalanceGroupSubmitting}
        width={520}
        okButtonProps={{
          disabled:
            paygBalanceGroupSubmitting ||
            !userId ||
            groupOptionsLoading ||
            !Array.isArray(groupIdOptions) ||
            groupIdOptions.length === 0 ||
            !editingPaygBalance,
        }}
      >
        <Form
          key={paygBalanceGroupFormKey}
          initValues={paygBalanceGroupInitValues}
          getFormApi={(api) => (paygBalanceGroupFormApiRef.current = api)}
        >
          {({ values }) => {
            const pid = Number(editingPaygBalance?.product_id ?? 0);
            const productName = String(
              editingPaygBalance?.product_name ?? '',
            ).trim();
            const isProductBased = pid > 0;
            const selectedGroupIds = normalizeGroupIds(
              values?.allowed_group_ids,
            );

            return (
              <div className='space-y-3'>
                {editingPaygBalance ? (
                  <div className='text-xs text-gray-500 space-y-1'>
                    {productName ? (
                      <div>
                        {t('商品')}：{productName}
                      </div>
                    ) : null}
                    {isProductBased ? (
                      <div>
                        {t(
                          '提示：该操作将覆盖商品分组设置，仅对该用户该笔余额生效',
                        )}
                      </div>
                    ) : null}
                  </div>
                ) : null}

                <Form.Select
                  field='allowed_group_ids'
                  label={t('可用分组')}
                  placeholder={t('请选择可用分组')}
                  optionList={groupIdOptions}
                  multiple
                  search
                  rules={[{ required: true, message: t('请选择可用分组') }]}
                  style={{ width: '100%' }}
                  extraText={t('限制该按量付费余额可消费的渠道分组')}
                />

                {selectedGroupIds.length > 0 ? (
                  <div className='text-xs text-gray-500'>
                    {t('倍率')}：
                    <span className='inline-flex flex-wrap gap-1'>
                      {selectedGroupIds.map((gid) => (
                        <span
                          key={`payg-balance-group-${pid}-${gid}`}
                        >
                          {formatGroupRatioLabel(gid)}
                        </span>
                      ))}
                    </span>
                  </div>
                ) : null}
              </div>
            );
          }}
        </Form>
      </Modal>

      <Modal
        centered
        visible={paygTopupModalVisible}
        title={t('从商品创建')}
        onCancel={() => setPaygTopupModalVisible(false)}
        onOk={handlePaygTopupSubmit}
        confirmLoading={paygTopupSubmitting}
        width={520}
        okButtonProps={{
          disabled:
            paygTopupSubmitting ||
            !userId ||
            paygProductsLoading ||
            paygProductOptionList.length === 0,
        }}
      >
        <Form
          key={paygTopupFormKey}
          initValues={paygTopupInitValues}
          getFormApi={(api) => (paygTopupFormApiRef.current = api)}
        >
          {({ values }) => (
            <div className='space-y-3'>
              <Form.Select
                field='product_id'
                label={t('商品')}
                placeholder={t('请选择商品')}
                optionList={paygProductOptionList}
                search
                rules={[{ required: true, message: t('请选择商品') }]}
                style={{ width: '100%' }}
              />

              {(() => {
                const pid = parseInt(values?.product_id, 10);
                const product =
                  Number.isFinite(pid) && pid > 0 ? paygProductById[pid] : null;
                if (!product) return null;
                const groupIds = normalizeGroupIds(product.allowed_group_ids);
                return (
                  <div className='text-xs text-gray-500 space-y-1'>
                    {product.description ? (
                      <div>{product.description}</div>
                    ) : null}
                    {groupIds.length > 0 ? (
                      <div className='flex flex-wrap items-center gap-1'>
                        {t('可选分组')}：
                        <span className='inline-flex flex-wrap gap-1'>
                          {groupIds.map((gid) => (
                            <span
                              key={`payg-topup-${pid}-${gid}`}
                            >
                              {formatGroupRatioLabel(gid)}
                            </span>
                          ))}
                        </span>
                      </div>
                    ) : null}
                  </div>
                );
              })()}

              <Form.InputNumber
                field='quota_usd'
                label={t('充值额度（USD）')}
                placeholder={t('请输入充值额度（美元）')}
                rules={[
                  { required: true, message: t('请输入充值额度（美元）') },
                ]}
                precision={2}
                step={1}
                min={0}
                extraText={`${t('折合约')} ${convertUSDToQuota(
                  values.quota_usd || 0,
                ).toLocaleString()} tokens`}
                style={{ width: '100%' }}
                prefix='$'
              />
            </div>
          )}
        </Form>
      </Modal>

      <Modal
        centered
        visible={paygManualModalVisible}
        title={t('按量付费手动创建')}
        onCancel={() => setPaygManualModalVisible(false)}
        onOk={handlePaygManualSubmit}
        confirmLoading={paygManualSubmitting}
        width={520}
        okButtonProps={{
          disabled:
            paygManualSubmitting ||
            !userId ||
            groupOptionsLoading ||
            !Array.isArray(groupIdOptions) ||
            groupIdOptions.length === 0,
        }}
      >
        <Form
          key={paygManualFormKey}
          initValues={paygManualInitValues}
          getFormApi={(api) => (paygManualFormApiRef.current = api)}
        >
          {({ values }) => {
            const groupIdRaw = Number(values?.group_id ?? 0);
            const groupId = Number.isFinite(groupIdRaw)
              ? Math.floor(groupIdRaw)
              : 0;
            return (
              <div className='space-y-3'>
                <Form.Select
                  field='group_id'
                  label={t('分组')}
                  placeholder={t('请选择分组')}
                  optionList={groupIdOptions}
                  search
                  rules={[{ required: true, message: t('请选择分组') }]}
                  style={{ width: '100%' }}
                />

                {groupId ? (
                  <div className='text-xs text-gray-500'>
                    {t('倍率')}：
                    {formatGroupRatioLabel(groupId)}
                  </div>
                ) : null}

                <Form.InputNumber
                  field='quota_usd'
                  label={t('额度（USD）')}
                  placeholder={t('请输入额度（美元）')}
                  rules={[{ required: true, message: t('请输入额度（美元）') }]}
                  precision={2}
                  step={1}
                  min={0}
                  extraText={`${t('折合约')} ${convertUSDToQuota(
                    values.quota_usd || 0,
                  ).toLocaleString()} tokens`}
                  style={{ width: '100%' }}
                  prefix='$'
                />
              </div>
            );
          }}
        </Form>
      </Modal>

      <Modal
        centered
        visible={subscriptionModalVisible}
        title={subscriptionModalTitle}
        onCancel={() => setSubscriptionModalVisible(false)}
        onOk={handleSubscriptionSubmit}
        confirmLoading={subscriptionSubmitting}
        width={480}
      >
        <Form
          key={subscriptionFormKey}
          initValues={subscriptionInitialValues}
          getFormApi={(api) => (subscriptionFormApiRef.current = api)}
        >
          {({ values }) => {
            const sourcePresetId =
              Number(editingSubscription?.source_preset_id ?? 0) || 0;
            const sourceRedemptionId =
              Number(editingSubscription?.source_redemption_id ?? 0) || 0;
            const groupDailyLimitsEditable =
              !editingSubscription ||
              (sourcePresetId <= 0 && sourceRedemptionId <= 0);
            const baselineGroupIds = normalizeGroupIds(
              editingSubscription?.allowed_group_ids,
            );
            const restrictAllowedGroupsToBaseline =
              !groupDailyLimitsEditable &&
              values.use_group_daily_limits &&
              baselineGroupIds.length > 0;
            const baselineSet = new Set(baselineGroupIds);
            const allowedGroupOptionList = restrictAllowedGroupsToBaseline
              ? (Array.isArray(groupIdOptions) ? groupIdOptions : []).filter(
                  (opt) => baselineSet.has(Number(opt?.value ?? 0)),
                )
              : groupIdOptions;
            const groupConfigExtra = groupDailyLimitsEditable
              ? t('限制该订阅可消费的渠道分组')
              : restrictAllowedGroupsToBaseline
                ? t(
                    '该订阅已启用分组日限额：可用分组仅支持在原分组范围内调整（如需新增分组请修改商品/兑换码配置）',
                  )
                : t(
                    '该订阅来源于商品/兑换码：可编辑可用分组（将覆盖默认配置）',
                  );

            return (
              <div className='space-y-3'>
                <Form.InputNumber
                  field='quota_usd'
                  label={t('总额度（USD）')}
                  placeholder={t('请输入订阅额度（美元）')}
                  rules={[
                    { required: true, message: t('请输入订阅额度（美元）') },
                  ]}
                  precision={2}
                  step={1}
                  min={0}
                  extraText={`${t('折合约')} ${convertUSDToQuota(
                    values.quota_usd || 0,
                  ).toLocaleString()} tokens`}
                  style={{ width: '100%' }}
                  prefix='$'
                />
                <Form.InputNumber
                  field='remaining_quota_usd'
                  label={t('剩余额度（USD）')}
                  placeholder={t('请输入剩余额度（美元）')}
                  precision={2}
                  step={1}
                  min={0}
                  extraText={`${t('折合约')} ${convertUSDToQuota(
                    values.remaining_quota_usd || 0,
                  ).toLocaleString()} tokens`}
                  style={{ width: '100%' }}
                  prefix='$'
                />

                <Form.Select
                  field='allowed_group_ids'
                  label={t('可用分组')}
                  placeholder={t('请选择可用分组')}
                  optionList={allowedGroupOptionList}
                  multiple
                  search
                  rules={[{ required: true, message: t('请选择可用分组') }]}
                  onChange={(val) => {
                    if (!subscriptionFormApiRef.current) return;
                    const useGroupDailyLimits = Boolean(
                      subscriptionFormApiRef.current.getValue(
                        'use_group_daily_limits',
                      ),
                    );
                    if (!useGroupDailyLimits) return;
                    const currentLimits =
                      subscriptionFormApiRef.current.getValue(
                        'group_daily_limits',
                      );
                    subscriptionFormApiRef.current.setValue(
                      'group_daily_limits',
                      syncGroupDailyLimitsUSD(
                        normalizeGroupIds(val),
                        currentLimits,
                      ),
                    );
                  }}
                  style={{ width: '100%' }}
                  extraText={groupConfigExtra}
                />

                <Form.Switch
                  field='use_group_daily_limits'
                  label={t('按分组设置日限额')}
                  disabled={!groupDailyLimitsEditable}
                  extraText={t(
                    '开启后，每个分组独立计算日限额（0 表示该分组无限制）；总剩余额度仍共享，任一分组消耗都会减少总剩余',
                  )}
                  onChange={(checked) => {
                    if (!subscriptionFormApiRef.current) return;
                    if (checked) {
                      const allowed = normalizeGroupIds(
                        subscriptionFormApiRef.current.getValue(
                          'allowed_group_ids',
                        ),
                      );
                      const current =
                        subscriptionFormApiRef.current.getValue(
                          'group_daily_limits',
                        );
                      subscriptionFormApiRef.current.setValue(
                        'group_daily_limits',
                        syncGroupDailyLimitsUSD(allowed, current),
                      );
                    } else {
                      subscriptionFormApiRef.current.setValue(
                        'group_daily_limits',
                        [],
                      );
                    }
                  }}
                />

                {values.use_group_daily_limits ? (
                  <Form.Slot
                    label={t('分组日限额（USD）')}
                    extraText={t('0 表示该分组无限制')}
                  >
                    {(() => {
                      const allowed = normalizeGroupIds(
                        values.allowed_group_ids,
                      );
                      const limits = syncGroupDailyLimitsUSD(
                        allowed,
                        values.group_daily_limits,
                      );
                      let totalUSD = 0;
                      let hasUnlimited = false;
                      limits.forEach((item) => {
                        const usd = parseFloat(item?.daily_quota_limit_usd);
                        if (!Number.isFinite(usd) || usd <= 0) {
                          hasUnlimited = true;
                        } else if (!hasUnlimited) {
                          totalUSD += usd;
                        }
                      });
                      return (
                        <div className='space-y-2'>
                          {limits.length === 0 ? (
                            <Text type='tertiary' size='small'>
                              {t('请先选择可用分组')}
                            </Text>
                          ) : (
                            limits.map((item) => (
                              <div
                                key={`sub-daily-limit-${item.group_id}`}
                                className='flex items-center justify-between gap-3'
                              >
                                <Text code style={{ fontSize: 12 }}>
                                  {groupLabelById[item.group_id] ||
                                    t('未知分组')}
                                </Text>
                                <InputNumber
                                  value={item.daily_quota_limit_usd}
                                  placeholder={t('0 表示无限制')}
                                  precision={2}
                                  min={0}
                                  step={1}
                                  prefix='$'
                                  style={{ width: 160 }}
                                  disabled={!groupDailyLimitsEditable}
                                  onChange={(v) => {
                                    const next = limits.map((row) =>
                                      row.group_id === item.group_id
                                        ? {
                                            ...row,
                                            daily_quota_limit_usd: v ?? 0,
                                          }
                                        : row,
                                    );
                                    subscriptionFormApiRef.current?.setValue(
                                      'group_daily_limits',
                                      next,
                                    );
                                  }}
                                />
                              </div>
                            ))
                          )}
                          <Text type='tertiary' size='small'>
                            {hasUnlimited
                              ? t('总日限额：无限制')
                              : t('总日限额：$ {{amount}}', {
                                  amount: totalUSD.toFixed(2),
                                })}
                          </Text>
                        </div>
                      );
                    })()}
                  </Form.Slot>
                ) : (
                  <Form.InputNumber
                    field='daily_quota_limit_usd'
                    label={t('每日限额（USD）')}
                    placeholder={t('请输入每日限额（美元，0 表示无限制）')}
                    precision={2}
                    step={1}
                    min={0}
                    extraText={`${t('折合约')} ${convertUSDToQuota(
                      values.daily_quota_limit_usd || 0,
                    ).toLocaleString()} tokens`}
                    style={{ width: '100%' }}
                    prefix='$'
                  />
                )}
                <Row gutter={12}>
                  <Col span={12}>
                    <Form.DatePicker
                      field='start_at'
                      label={t('开始时间')}
                      placeholder={t('请选择开始时间（精确到秒）')}
                      type='dateTime'
                      format='yyyy/MM/dd HH:mm:ss'
                      style={{ width: '100%' }}
                      showClear={false}
                    />
                  </Col>
                  <Col span={12}>
                    <Form.DatePicker
                      field='expire_at'
                      label={t('结束时间')}
                      placeholder={t(
                        '请选择到期时间（精确到秒，留空为不限时）',
                      )}
                      type='dateTime'
                      format='yyyy/MM/dd HH:mm:ss'
                      style={{ width: '100%' }}
                      showClear
                    />
                  </Col>
                </Row>
                <Form.Input
                  field='source'
                  label={editingSubscription ? t('来源（只读）') : t('来源')}
                  placeholder={t('请输入来源说明，可选')}
                  showClear
                  disabled={Boolean(editingSubscription)}
                />
              </div>
            );
          }}
        </Form>
      </Modal>

      <Modal
        centered
        visible={requestSubscriptionModalVisible}
        title={requestSubscriptionModalTitle}
        onCancel={() => setRequestSubscriptionModalVisible(false)}
        onOk={handleRequestSubscriptionSubmit}
        confirmLoading={requestSubscriptionSubmitting}
        width={480}
      >
        <Form
          key={requestSubscriptionFormKey}
          initValues={requestSubscriptionInitialValues}
          getFormApi={(api) => (requestSubscriptionFormApiRef.current = api)}
        >
          {() => (
            <div className='space-y-3'>
              <Form.InputNumber
                field='daily_request_limit'
                label={t('每日次数')}
                min={0}
                step={0.001}
                precision={3}
                rules={[
                  { required: true, message: t('请输入每日次数') },
                  {
                    validator: (rule, v) => {
                      const num = Number(v);
                      return num >= 0
                        ? Promise.resolve()
                        : Promise.reject(t('每日次数必须大于等于0'));
                    },
                  },
                ]}
                style={{ width: '100%' }}
              />

              <Form.InputNumber
                field='total_request_limit'
                label={t('总次数')}
                min={0}
                step={0.001}
                precision={3}
                rules={[
                  { required: true, message: t('请输入总次数') },
                  {
                    validator: (rule, v) => {
                      const num = Number(v);
                      return num >= 0
                        ? Promise.resolve()
                        : Promise.reject(t('总次数必须大于等于0'));
                    },
                  },
                ]}
                style={{ width: '100%' }}
              />

              <Form.Select
                field='allowed_group_ids'
                label={t('可用分组')}
                placeholder={t('请选择可用分组')}
                optionList={groupIdOptions}
                multiple
                search
                rules={[{ required: true, message: t('请选择可用分组') }]}
                style={{ width: '100%' }}
                extraText={t('限制该订阅可消费的渠道分组')}
              />

              <Row gutter={12}>
                <Col span={12}>
                  <Form.DatePicker
                    field='start_at'
                    label={t('开始时间')}
                    placeholder={t('请选择开始时间（精确到秒）')}
                    type='dateTime'
                    format='yyyy/MM/dd HH:mm:ss'
                    style={{ width: '100%' }}
                    showClear={false}
                  />
                </Col>
                <Col span={12}>
                  <Form.DatePicker
                    field='expire_at'
                    label={t('结束时间')}
                    placeholder={t('请选择到期时间（精确到秒，留空为不限时）')}
                    type='dateTime'
                    format='yyyy/MM/dd HH:mm:ss'
                    style={{ width: '100%' }}
                    showClear
                  />
                </Col>
              </Row>

              <Form.Input
                field='source'
                label={
                  editingRequestSubscription ? t('来源（只读）') : t('来源')
                }
                placeholder={t('请输入来源说明，可选')}
                showClear
                disabled={Boolean(editingRequestSubscription)}
              />
            </div>
          )}
        </Form>
      </Modal>
    </>
  );
};

export default EditUserModal;
