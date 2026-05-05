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

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  API,
  inferPresetMode,
  renderNumber,
  showError,
  showSuccess,
  yuanToFen,
} from '../../../../helpers';
import {
  Avatar,
  Button,
  Card,
  Col,
  Form,
  InputNumber,
  Row,
  SideSheet,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconClose, IconGift, IconSave } from '@douyinfe/semi-icons';

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

const normalizeStockValue = (rawStock) => {
  if (rawStock === null || rawStock === undefined) return null;
  if (typeof rawStock === 'string') {
    const trimmed = rawStock.trim();
    if (!trimmed) return null;
    rawStock = trimmed;
  }
  const num = Number(rawStock);
  if (!Number.isFinite(num)) return undefined;
  const stock = Math.floor(num);
  if (stock < 0) return undefined;
  return stock;
};

const parseNonNegativeIntegerValue = (raw) => {
  const num = Number(raw);
  if (!Number.isFinite(num) || num < 0 || !Number.isInteger(num)) {
    return null;
  }
  return num;
};

const normalizeGroupDailyLimitsUSD = (val) => {
  const list = Array.isArray(val) ? val : [];
  const seen = new Set();
  const out = [];
  list.forEach((raw) => {
    const rawId = Number(raw?.group_id ?? 0);
    const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
    if (groupId <= 0) return;
    if (seen.has(groupId)) return;
    seen.add(groupId);
    out.push({
      group_id: groupId,
      daily_quota_limit_usd: raw?.daily_quota_limit_usd ?? 0,
    });
  });
  return out;
};

const syncGroupDailyLimitsUSD = (allowedGroups, val) => {
  const allowed = normalizeGroupIds(allowedGroups);
  const existing = normalizeGroupDailyLimitsUSD(val);
  const map = new Map(existing.map((item) => [item.group_id, item.daily_quota_limit_usd]));
  return allowed.map((groupId) => ({
    group_id: groupId,
    daily_quota_limit_usd: map.get(groupId) ?? 0,
  }));
};

const normalizeGroupDailyLimitsTokens = (val) => {
  const list = Array.isArray(val) ? val : [];
  const seen = new Set();
  const out = [];
  list.forEach((raw) => {
    const rawId = Number(raw?.group_id ?? 0);
    const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
    if (groupId <= 0) return;
    if (seen.has(groupId)) return;
    seen.add(groupId);
    const dailyLimitRaw = Number(raw?.daily_quota_limit_tokens ?? 0);
    out.push({
      group_id: groupId,
      daily_quota_limit_tokens: Number.isFinite(dailyLimitRaw)
        ? Math.max(0, Math.floor(dailyLimitRaw))
        : 0,
    });
  });
  return out;
};

const syncGroupDailyLimitsTokens = (allowedGroups, val) => {
  const allowed = normalizeGroupIds(allowedGroups);
  const existing = normalizeGroupDailyLimitsTokens(val);
  const map = new Map(existing.map((item) => [item.group_id, item.daily_quota_limit_tokens]));
  return allowed.map((groupId) => ({
    group_id: groupId,
    daily_quota_limit_tokens: map.get(groupId) ?? 0,
  }));
};

const EditRedemptionPresetModal = ({
  visible,
  onClose,
  onSuccess,
  editingPreset,
  allowedModes,
  modeLocked,
  presetApiBase = '/api/redemption/presets',
}) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const lockedMode = String(modeLocked || '').trim();
  const firstAllowedMode = useMemo(() => {
    const list = Array.isArray(allowedModes) ? allowedModes : [];
    for (const item of list) {
      const mode = String(item || '').trim();
      if (mode) return mode;
    }
    return '';
  }, [allowedModes]);
  const allowedModeSet = useMemo(() => {
    const list = Array.isArray(allowedModes) ? allowedModes : [];
    const set = new Set();
    list.forEach((m) => {
      const mode = String(m || '').trim();
      if (!mode) return;
      set.add(mode);
    });
    return set.size > 0 ? set : null;
  }, [allowedModes]);

  const isEdit = editingPreset?.id !== undefined;
  const [loading, setLoading] = useState(false);

  const [groupsLoading, setGroupsLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);

  const normalizeGroups = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    return list
      .map((g) => {
        if (typeof g === 'string') return null;
        const idRaw = Number(g?.id ?? 0);
        const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
        if (id <= 0) return null;
        const code = String(g?.code || '').trim();
        if (!code) return null;
        const displayName = String(g?.display_name || code).trim() || code;
        return { id, code, display_name: displayName };
      })
      .filter(Boolean);
  }, []);

  const DEFAULT_QUOTA_PER_USD = 500000; // 本地存储缺失时的美元兑额度后备比例

  const getQuotaPerUnit = useCallback(() => {
    if (typeof window === 'undefined') {
      return DEFAULT_QUOTA_PER_USD;
    }
    const raw = window.localStorage.getItem('quota_per_unit');
    const parsed = parseFloat(raw);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      return DEFAULT_QUOTA_PER_USD;
    }
    return parsed;
  }, []);

  const convertQuotaToUSD = useCallback(
    (quotaValue) => {
      const quotaNumber = Number(quotaValue);
      if (!Number.isFinite(quotaNumber) || quotaNumber <= 0) {
        return 0;
      }
      const perUnit = getQuotaPerUnit();
      if (!perUnit) {
        return 0;
      }
      return Number((quotaNumber / perUnit).toFixed(4));
    },
    [getQuotaPerUnit],
  );

  const convertUSDToQuota = useCallback(
    (usdValue) => {
      const usdNumber = Number(usdValue);
      if (!Number.isFinite(usdNumber) || usdNumber <= 0) {
        return 0;
      }
      const perUnit = getQuotaPerUnit();
      if (!perUnit) {
        return 0;
      }
      return Math.round(usdNumber * perUnit);
    },
    [getQuotaPerUnit],
  );

  const defaultMode = useMemo(() => {
    if (lockedMode) return lockedMode;
    const inferredMode = inferPresetMode(editingPreset);
    if (allowedModeSet && !allowedModeSet.has(inferredMode)) {
      return firstAllowedMode || 'subscription';
    }
    return inferredMode || firstAllowedMode || 'subscription';
  }, [allowedModeSet, editingPreset, firstAllowedMode, lockedMode]);

  const getInitValues = useCallback(
    (mode) => {
      const normalizedMode = mode || 'subscription';
      const isSubscriptionProduct =
        normalizedMode === 'subscription' ||
        normalizedMode === 'tokens' ||
        normalizedMode === 'request';
      return {
        id: undefined,
        name: '',
        description: '',
        mode: normalizedMode,
        sync_sold_assets: false,
        enabled: isSubscriptionProduct ? false : true,
        archived: false,
        multi_quantity_enabled: false,
        multi_quantity_defer_only: true,
        sort_order: 0,
        purchase_limit: 0,
        stock: null,
        price_yuan: 0,
        quota_usd: 0,
        quota_tokens: 0,
        daily_quota_limit_usd: 0,
        daily_quota_limit_tokens: 0,
        daily_request_limit: 0,
        request_quota: 0,
        use_group_daily_limits: false,
        group_daily_limits: [],
        quota_valid_days: 0,
        plan_valid_days: 0,
        channel_ids: [],
        allowed_group_ids: [],
        expired_time: null,
      };
    },
    [],
  );

  const initValues = useMemo(() => {
    const base = getInitValues(defaultMode);
    if (!editingPreset || editingPreset.id === undefined) return base;
    const mode = lockedMode || inferPresetMode(editingPreset);
    const expiredTime =
      editingPreset.expired_time === 0 || !editingPreset.expired_time
        ? null
        : new Date(editingPreset.expired_time * 1000);
    const allowedGroupIds = normalizeGroupIds(editingPreset.allowed_group_ids);
    const groupDailyLimitsRaw = Array.isArray(editingPreset.group_daily_limits)
      ? editingPreset.group_daily_limits
      : [];
    const groupDailyLimitsUSD = groupDailyLimitsRaw
      .map((item) => {
        const rawId = Number(item?.group_id ?? 0);
        const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
        if (groupId <= 0) return null;
        return {
          group_id: groupId,
          daily_quota_limit_usd: convertQuotaToUSD(item?.daily_quota_limit || 0),
        };
      })
      .filter(Boolean);
    const groupDailyLimitsTokens = groupDailyLimitsRaw
      .map((item) => {
        const rawId = Number(item?.group_id ?? 0);
        const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
        if (groupId <= 0) return null;
        const rawDaily = Number(item?.daily_quota_limit ?? 0);
        const daily = Number.isFinite(rawDaily) ? Math.max(0, Math.floor(rawDaily)) : 0;
        return {
          group_id: groupId,
          daily_quota_limit_tokens: daily,
        };
      })
      .filter(Boolean);
    const useGroupDailyLimits =
      mode === 'tokens' ? groupDailyLimitsTokens.length > 0 : groupDailyLimitsUSD.length > 0;
    return {
      ...base,
      id: editingPreset.id,
      name: editingPreset.name || '',
      description: editingPreset.description || '',
      mode,
      enabled: editingPreset.enabled ?? true,
      archived: editingPreset.archived === true,
      multi_quantity_enabled: Boolean(editingPreset.multi_quantity_enabled),
      multi_quantity_defer_only:
        editingPreset.multi_quantity_defer_only !== false,
      sort_order: editingPreset.sort_order || 0,
      purchase_limit: editingPreset.purchase_limit || 0,
      stock: normalizeStockValue(editingPreset.stock),
      price_yuan: (editingPreset.price_fen || 0) / 100,
      quota_usd:
        mode === 'request' || mode === 'tokens'
          ? 0
          : convertQuotaToUSD(editingPreset.quota || 0),
      quota_tokens: mode === 'tokens' ? Number(editingPreset.quota || 0) : 0,
      daily_quota_limit_usd:
        mode === 'tokens'
          ? 0
          : convertQuotaToUSD(editingPreset.daily_quota_limit || 0),
      daily_quota_limit_tokens:
        mode === 'tokens' ? Number(editingPreset.daily_quota_limit || 0) : 0,
      daily_request_limit: editingPreset.daily_request_limit || 0,
      request_quota: mode === 'request' ? Number(editingPreset.quota || 0) : 0,
      quota_valid_days: editingPreset.quota_valid_days || 0,
      plan_valid_days: 0,
      channel_ids: [],
      allowed_group_ids: allowedGroupIds,
      use_group_daily_limits: useGroupDailyLimits,
      group_daily_limits: useGroupDailyLimits
        ? mode === 'tokens'
          ? syncGroupDailyLimitsTokens(allowedGroupIds, groupDailyLimitsTokens)
          : syncGroupDailyLimitsUSD(allowedGroupIds, groupDailyLimitsUSD)
        : [],
      expired_time: expiredTime,
    };
  }, [convertQuotaToUSD, defaultMode, editingPreset, getInitValues, lockedMode]);

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    try {
      const res = await API.get('/api/group/');
      const { success, message, data } = res.data;
      if (success) {
        setAvailableGroups(normalizeGroups(data));
      } else {
        showError(message || t('获取分组失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
    } finally {
      setGroupsLoading(false);
    }
  }, [normalizeGroups, t]);

  useEffect(() => {
    if (!visible) return;
    formApiRef.current?.setValues(initValues);
    loadGroups();
  }, [initValues, loadGroups, visible]);

  const groupOptions = useMemo(() => {
    return (Array.isArray(availableGroups) ? availableGroups : [])
      .map((g) => ({
        label: g?.display_name || g?.code,
        value: g?.id,
      }))
      .filter((opt) => opt.value);
  }, [availableGroups]);

  const groupLabelById = useMemo(() => {
    const map = {};
    (Array.isArray(availableGroups) ? availableGroups : []).forEach((g) => {
      const idRaw = Number(g?.id ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return;
      const label = String(g?.display_name || '').trim();
      const code = String(g?.code || '').trim();
      const name = label || code;
      if (!name) return;
      map[id] = name;
    });
    return map;
  }, [availableGroups]);

  const modeOptions = useMemo(() => {
    const list = [
      { label: t('订阅额度'), value: 'subscription' },
      { label: t('Tokens订阅'), value: 'tokens' },
      { label: t('次数订阅'), value: 'request' },
      { label: t('按量付费'), value: 'payg' },
    ];
    if (!allowedModeSet) return list;
    return list.filter((opt) => allowedModeSet.has(opt.value));
  }, [allowedModeSet, t]);

  const submit = async (values) => {
    const mode = lockedMode || values.mode || defaultMode || 'subscription';
    const name = String(values.name || '').trim();
    if (!name) {
      showError(t('请输入名称'));
      return;
    }

    let priceFen = 0;
    try {
      priceFen = yuanToFen(values.price_yuan || 0);
    } catch (e) {
      showError(e?.message || t('金额格式错误'));
      return;
    }

    const sortOrder = parseInt(values.sort_order ?? 0, 10);
    if (!Number.isFinite(sortOrder) || sortOrder < 0) {
      showError(t('排序不能小于0'));
      return;
    }

    const expiredTime = values.expired_time
      ? Math.floor(values.expired_time.getTime() / 1000)
      : 0;

    const payload = {
      id: values.id,
      name,
      description: String(values.description || ''),
      mode,
      archived: Boolean(values.archived),
      enabled: Boolean(values.archived) ? false : Boolean(values.enabled),
      multi_quantity_enabled:
        mode === 'subscription' || mode === 'tokens' || mode === 'request'
          ? Boolean(values.multi_quantity_enabled)
          : false,
      multi_quantity_defer_only:
        mode === 'subscription' || mode === 'tokens' || mode === 'request'
          ? Boolean(values.multi_quantity_defer_only)
          : true,
      sort_order: sortOrder,
      price_fen: priceFen,
      expired_time: expiredTime,
    };

    const allowedGroupIds = normalizeGroupIds(values.allowed_group_ids);
    if (mode === 'subscription' || mode === 'tokens' || mode === 'payg' || mode === 'request') {
      if (allowedGroupIds.length === 0) {
        showError(t('请选择可用分组'));
        return;
      }
      payload.allowed_group_ids = allowedGroupIds;
    } else {
      payload.allowed_group_ids = [];
    }

    let purchaseLimit = 0;
    if (mode === 'subscription' || mode === 'tokens' || mode === 'request') {
      purchaseLimit = parseInt(values.purchase_limit ?? 0, 10);
      if (!Number.isFinite(purchaseLimit) || purchaseLimit < 0) {
        showError(t('限购次数不能小于0'));
        return;
      }
    }
    payload.purchase_limit = purchaseLimit;

    const stock = normalizeStockValue(values.stock);
    if (stock === undefined) {
      showError(t('库存必须为大于等于 0 的整数，留空表示无限制'));
      return;
    }
    payload.stock = stock;

    payload.plan_valid_days = 0;
    payload.channel_ids = [];
    payload.daily_request_limit = 0;

    if (mode === 'request') {
      const dailyRequestLimit = parseNonNegativeIntegerValue(
        values.daily_request_limit,
      );
      if (dailyRequestLimit === null) {
        showError(t('每日次数必须大于等于0'));
        return;
      }
      const requestQuota = parseNonNegativeIntegerValue(values.request_quota);
      if (requestQuota === null) {
        showError(t('总次数必须大于等于0'));
        return;
      }
      payload.daily_request_limit = dailyRequestLimit;
      payload.quota = requestQuota;

      const days = parseInt(values.quota_valid_days, 10);
      if (!Number.isFinite(days) || days < 0) {
        showError(t('有效期（天）不能小于0'));
        return;
      }
      payload.quota_valid_days = days;

      payload.daily_quota_limit = 0;
      payload.group_daily_limits = [];
    } else {
      if (mode === 'tokens') {
        const quotaTokens = parseNonNegativeIntegerValue(values.quota_tokens ?? 0);
        if (quotaTokens === null) {
          showError(t('Tokens 必须大于等于0'));
          return;
        }
        payload.quota = quotaTokens;

        const days = parseInt(values.quota_valid_days, 10);
        if (!Number.isFinite(days) || days < 0) {
          showError(t('额度有效期（天）不能小于0'));
          return;
        }
        payload.quota_valid_days = days;

        const useGroupDailyLimits = Boolean(values.use_group_daily_limits);
        if (useGroupDailyLimits) {
          const syncedGroupDailyLimits = syncGroupDailyLimitsTokens(
            allowedGroupIds,
            values.group_daily_limits,
          );
          if (syncedGroupDailyLimits.length === 0) {
            showError(t('请选择可用分组'));
            return;
          }
          let dailyLimitTotalTokens = 0;
          let hasUnlimited = false;
          const groupDailyLimitsPayload = [];
          for (const item of syncedGroupDailyLimits) {
            const raw = parseNonNegativeIntegerValue(
              item?.daily_quota_limit_tokens ?? 0,
            );
            if (raw === null) {
              showError(
                `${t('每日额度必须大于等于0')}: ${groupLabelById[item.group_id] || t('未知分组')}`,
              );
              return;
            }
            groupDailyLimitsPayload.push({
              group_id: item.group_id,
              daily_quota_limit: raw,
            });
            if (raw === 0) {
              hasUnlimited = true;
            } else {
              dailyLimitTotalTokens += raw;
            }
          }
          payload.group_daily_limits = groupDailyLimitsPayload;
          payload.daily_quota_limit = hasUnlimited ? 0 : dailyLimitTotalTokens;
        } else {
          const dailyLimit = parseNonNegativeIntegerValue(
            values.daily_quota_limit_tokens ?? 0,
          );
          if (dailyLimit === null) {
            showError(t('每日额度必须大于等于0'));
            return;
          }
          payload.daily_quota_limit = dailyLimit;
          payload.group_daily_limits = [];
        }
      } else {
        const quotaUSD = parseFloat(values.quota_usd);
        if (!Number.isFinite(quotaUSD) || quotaUSD < 0) {
          showError(
            mode === 'subscription' ? t('额度必须大于等于0') : t('额度必须大于0'),
          );
          return;
        }
        if (mode === 'subscription' && quotaUSD === 0) {
          payload.quota = 0;
        } else {
          if (quotaUSD <= 0) {
            showError(t('额度必须大于0'));
            return;
          }
          const quotaTokens = convertUSDToQuota(quotaUSD);
          if (!Number.isFinite(quotaTokens) || quotaTokens <= 0) {
            showError(t('额度必须大于0'));
            return;
          }
          payload.quota = quotaTokens;
        }

        if (mode === 'subscription') {
          const days = parseInt(values.quota_valid_days, 10);
          if (!Number.isFinite(days) || days < 0) {
            showError(t('额度有效期（天）不能小于0'));
            return;
          }
          payload.quota_valid_days = days;

          const useGroupDailyLimits = Boolean(values.use_group_daily_limits);
          if (useGroupDailyLimits) {
            const syncedGroupDailyLimits = syncGroupDailyLimitsUSD(
              allowedGroupIds,
              values.group_daily_limits,
            );
            if (syncedGroupDailyLimits.length === 0) {
              showError(t('请选择可用分组'));
              return;
            }
            let dailyLimitTotalTokens = 0;
            let hasUnlimited = false;
            const groupDailyLimitsPayload = [];
            for (const item of syncedGroupDailyLimits) {
              const dailyUSD = parseFloat(item?.daily_quota_limit_usd);
              if (!Number.isFinite(dailyUSD) || dailyUSD < 0) {
                showError(
                  `${t('每日额度必须大于等于0')}: ${groupLabelById[item.group_id] || t('未知分组')}`,
                );
                return;
              }
              const dailyTokens = dailyUSD > 0 ? convertUSDToQuota(dailyUSD) : 0;
              if (
                dailyUSD > 0 &&
                (!Number.isFinite(dailyTokens) || dailyTokens <= 0)
              ) {
                showError(
                  `${t('每日额度必须大于0')}: ${groupLabelById[item.group_id] || t('未知分组')}`,
                );
                return;
              }
              groupDailyLimitsPayload.push({
                group_id: item.group_id,
                daily_quota_limit: dailyTokens,
              });
              if (dailyTokens === 0) {
                hasUnlimited = true;
              } else {
                dailyLimitTotalTokens += dailyTokens;
              }
            }
            payload.group_daily_limits = groupDailyLimitsPayload;
            payload.daily_quota_limit = hasUnlimited ? 0 : dailyLimitTotalTokens;
          } else {
            const dailyUSD = parseFloat(values.daily_quota_limit_usd);
            if (!Number.isFinite(dailyUSD) || dailyUSD < 0) {
              showError(t('每日额度必须大于等于0'));
              return;
            }
            const dailyTokens = dailyUSD > 0 ? convertUSDToQuota(dailyUSD) : 0;
            if (
              dailyUSD > 0 &&
              (!Number.isFinite(dailyTokens) || dailyTokens <= 0)
            ) {
              showError(t('每日额度必须大于0'));
              return;
            }
            payload.daily_quota_limit = dailyTokens;
            payload.group_daily_limits = [];
          }
        } else {
          payload.daily_quota_limit = 0;
          payload.quota_valid_days = 0;
        }
      }
    }

    if (
      isEdit &&
      (mode === 'subscription' || mode === 'tokens' || mode === 'request')
    ) {
      payload.sync_sold_assets = Boolean(values.sync_sold_assets);
    }

    setLoading(true);
    try {
      const res = await API.post(presetApiBase, payload);
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        onSuccess && onSuccess(data);
        onClose && onClose();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setLoading(false);
    }
  };

  const title = isEdit ? t('编辑预置商品') : t('新增预置商品');

  return (
    <>
      <SideSheet
        title={
          <div className='flex items-center gap-3'>
            <Avatar size='small' color='green' className='shadow-md'>
              <IconGift size={16} />
            </Avatar>
            <div>
              <Title heading={5} style={{ margin: 0 }}>
                {title}
              </Title>
              <Text type='tertiary' size='small'>
                {t('用于一键生成兑换码，并按价格结算返佣')}
              </Text>
            </div>
          </div>
        }
        placement={isMobile ? 'bottom' : 'right'}
        visible={visible}
        closeIcon={null}
        onCancel={onClose}
        width={isMobile ? undefined : 420}
        height={isMobile ? '70vh' : undefined}
        footer={
          <div className='flex justify-end gap-2'>
            <Button icon={<IconClose />} onClick={onClose}>
              {t('取消')}
            </Button>
            <Button
              type='primary'
              icon={<IconSave />}
              loading={loading}
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Spin spinning={loading}>
          <Form
            initValues={initValues}
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='name'
                        label={t('名称')}
                        placeholder={t('请输入名称')}
                        rules={[{ required: true, message: t('请输入名称') }]}
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.TextArea
                        field='description'
                        label={t('描述')}
                        autosize
                        placeholder={t('可选：描述信息（支持换行）')}
                      />
                    </Col>
                    {lockedMode ? null : (
                      <Col span={24}>
                        <Form.Select
                          field='mode'
                          label={t('类型')}
                          style={{ width: '100%' }}
                          optionList={modeOptions}
                          renderSelectedItem={(option) => (
                            <Tag color='white' shape='circle'>
                              {option?.label}
                            </Tag>
                          )}
                          rules={[{ required: true, message: t('请选择类型') }]}
                        />
                      </Col>
                    )}
                    {values.mode === 'subscription' ||
                    values.mode === 'tokens' ||
                    values.mode === 'payg' ||
                    values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.Select
                          field='allowed_group_ids'
                          label={t('可用分组')}
                          placeholder={t('请选择可用分组')}
                          optionList={groupOptions}
                          loading={groupsLoading}
                          multiple
                          search
                          onChange={(val) => {
                            if (!formApiRef.current) return;
                            const useGroupDailyLimits = Boolean(
                              formApiRef.current.getValue('use_group_daily_limits'),
                            );
                            if (!useGroupDailyLimits) return;
                            const currentLimits =
                              formApiRef.current.getValue('group_daily_limits');
                            const sync = values.mode === 'tokens' ? syncGroupDailyLimitsTokens : syncGroupDailyLimitsUSD;
                            formApiRef.current.setValue(
                              'group_daily_limits',
                              sync(val, currentLimits),
                            );
                          }}
                          rules={[{ required: true, message: t('请选择可用分组') }]}
                          style={{ width: '100%' }}
                          extraText={t('限制该商品可消费的渠道分组')}
                        />
                      </Col>
                    ) : null}
                    {values.mode === 'subscription' || values.mode === 'tokens' || values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.Switch
                          field='enabled'
                          label={t('是否上架')}
                          disabled={Boolean(values.archived)}
                        />
                      </Col>
                    ) : null}
                    {values.mode === 'subscription' ||
                    values.mode === 'tokens' ||
                    values.mode === 'payg' ||
                    values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.Switch
                          field='archived'
                          label={t('停用商品')}
                          extraText={t('停用后会自动下架，适合归档不再维护的商品')}
                          onChange={(checked) => {
                            if (checked) {
                              formApiRef.current?.setValue('enabled', false);
                            }
                          }}
                        />
                      </Col>
                    ) : null}
                    {values.mode === 'subscription' || values.mode === 'tokens' || values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.Switch
                          field='multi_quantity_enabled'
                          label={t('允许多数量购买')}
                          extraText={t('开启后可在订阅购买页选择购买数量')}
                        />
                      </Col>
                    ) : null}
                    {values.mode === 'subscription' || values.mode === 'tokens' || values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.Switch
                          field='multi_quantity_defer_only'
                          label={t('多数量仅允许顺延')}
                          extraText={t('关闭后，多数量购买也允许选择叠加')}
                        />
                      </Col>
                    ) : null}
                    {isEdit &&
                    (values.mode === 'subscription' ||
                      values.mode === 'tokens' ||
                      values.mode === 'request') ? (
                      <Col span={24}>
                        <Form.Switch
                          field='sync_sold_assets'
                          label={t('同步调整已售商品')}
                          extraText={t(
                            '默认关闭；开启后会将本次商品变更同步到该商品已售出的订阅资产',
                          )}
                        />
                      </Col>
                    ) : null}
                    <Col span={24}>
                      <Form.InputNumber
                        field='price_yuan'
                        label={t('结算价格')}
                        prefix='￥'
                        precision={2}
                        min={0}
                        step={1}
                        style={{ width: '100%' }}
                        extraText={t('用于邀请返佣结算')}
                      />
                    </Col>
                    <Col span={24}>
                      <Form.InputNumber
                        field='sort_order'
                        label={t('排序')}
                        precision={0}
                        min={0}
                        step={1}
                        style={{ width: '100%' }}
                        extraText={t('数值越大越靠前')}
                      />
                    </Col>
                    {values.mode === 'subscription' || values.mode === 'tokens' || values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.InputNumber
                          field='purchase_limit'
                          label={t('限购次数')}
                          precision={0}
                          min={0}
                          step={1}
                          style={{ width: '100%' }}
                          extraText={t('0 表示不限购')}
                        />
                      </Col>
                    ) : null}
                    {values.mode === 'subscription' || values.mode === 'tokens' || values.mode === 'request' ? (
                      <Col span={24}>
                        <Form.InputNumber
                          field='stock'
                          label={t('库存')}
                          precision={0}
                          min={0}
                          step={1}
                          style={{ width: '100%' }}
                          placeholder={t('留空表示无限制')}
                          extraText={t('留空表示无限制；0 表示售罄')}
                        />
                      </Col>
                    ) : null}
                    <Col span={24}>
                      <Form.DatePicker
                        field='expired_time'
                        label={t('过期时间')}
                        type='dateTime'
                        placeholder={t('选择过期时间（可选，留空为永久）')}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </Col>
                  </Row>
                </Card>

                <Card className='!rounded-2xl shadow-sm border-0'>
                  {values.mode === 'request' ? (
                    <Row gutter={12}>
                      <Col span={24}>
                        <Form.InputNumber
                          field='daily_request_limit'
                          label={t('每日次数')}
                          placeholder={t('0 表示无限制')}
                          min={0}
                          step={1}
                          precision={0}
                          rules={[
                            { required: true, message: t('请输入每日次数') },
                            {
                              validator: (rule, v) => {
                                const num = parseNonNegativeIntegerValue(v);
                                return num !== null
                                  ? Promise.resolve()
                                  : Promise.reject(t('每日次数必须大于等于0'));
                              },
                            },
                          ]}
                          extraText={t('0 表示无限制')}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.InputNumber
                          field='request_quota'
                          label={t('总次数')}
                          placeholder={t('0 表示无限制')}
                          min={0}
                          step={1}
                          precision={0}
                          rules={[
                            { required: true, message: t('请输入总次数') },
                            {
                              validator: (rule, v) => {
                                const num = parseNonNegativeIntegerValue(v);
                                return num !== null
                                  ? Promise.resolve()
                                  : Promise.reject(t('总次数必须大于等于0'));
                              },
                            },
                          ]}
                          extraText={t('0 表示无限制')}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.InputNumber
                          field='quota_valid_days'
                          label={t('有效期（天）')}
                          placeholder={t('0 表示永久')}
                          min={0}
                          step={1}
                          precision={0}
                          rules={[
                            {
                              required: true,
                              message: t('请输入有效期（天）'),
                            },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num >= 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('有效期（天）不能小于0'));
                              },
                            },
                          ]}
                          extraText={t('0 表示永久')}
                          style={{ width: '100%' }}
                        />
                      </Col>
                    </Row>
                  ) : values.mode === 'tokens' ? (
                    <Row gutter={12}>
                      <Col span={24}>
                        <Form.InputNumber
                          field='quota_tokens'
                          label={t('Tokens')}
                          placeholder={t('请输入 Tokens（0 表示无限制）')}
                          min={0}
                          step={1}
                          precision={0}
                          rules={[
                            { required: true, message: t('请输入 Tokens') },
                            {
                              validator: (rule, v) => {
                                const num = parseNonNegativeIntegerValue(v);
                                return num !== null
                                  ? Promise.resolve()
                                  : Promise.reject(t('Tokens 必须大于等于0'));
                              },
                            },
                          ]}
                          extraText={t('0 表示无限制')}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Switch
                          field='use_group_daily_limits'
                          label={t('按分组设置日限额')}
                          extraText={t(
                            '开启后，每个分组独立计算日限额（0 表示该分组无限制）；总剩余额度仍共享，任一分组消耗都会减少总剩余',
                          )}
                          onChange={(checked) => {
                            if (!formApiRef.current) return;
                            if (checked) {
                              const allowed = normalizeGroupIds(
                                formApiRef.current.getValue('allowed_group_ids'),
                              );
                              const current =
                                formApiRef.current.getValue('group_daily_limits');
                              formApiRef.current.setValue(
                                'group_daily_limits',
                                syncGroupDailyLimitsTokens(allowed, current),
                              );
                            } else {
                              formApiRef.current.setValue('group_daily_limits', []);
                            }
                          }}
                        />
                      </Col>
                      {values.use_group_daily_limits ? (
                        <Col span={24}>
                          <Form.Slot
                            label={t('分组日限额（tokens）')}
                            extraText={t('0 表示该分组无限制')}
                          >
                            {(() => {
                              const allowed = normalizeGroupIds(values.allowed_group_ids);
                              const limits = syncGroupDailyLimitsTokens(
                                allowed,
                                values.group_daily_limits,
                              );
                              let totalTokens = 0;
                              let hasUnlimited = false;
                              limits.forEach((item) => {
                                const tokens = parseNonNegativeIntegerValue(
                                  item?.daily_quota_limit_tokens ?? 0,
                                );
                                if (tokens === null || tokens <= 0) {
                                  hasUnlimited = true;
                                } else {
                                  totalTokens += tokens;
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
                                        key={`daily-limit-${item.group_id}`}
                                        className='flex items-center justify-between gap-3'
                                      >
                                        <Text code style={{ fontSize: 12 }}>
                                          {groupLabelById[item.group_id] || t('未知分组')}
                                        </Text>
                                        <InputNumber
                                          value={item.daily_quota_limit_tokens}
                                          placeholder={t('0 表示无限制')}
                                          precision={0}
                                          min={0}
                                          step={1}
                                          style={{ width: 160 }}
                                          onChange={(v) => {
                                            const next = limits.map((row) =>
                                              row.group_id === item.group_id
                                                ? {
                                                    ...row,
                                                    daily_quota_limit_tokens: v ?? 0,
                                                  }
                                                : row,
                                            );
                                            formApiRef.current?.setValue(
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
                                      : t('总日限额：{{amount}} tokens', {
                                          amount: renderNumber(totalTokens),
                                        })}
                                  </Text>
                                </div>
                              );
                            })()}
                          </Form.Slot>
                        </Col>
                      ) : (
                        <Col span={24}>
                          <Form.InputNumber
                            field='daily_quota_limit_tokens'
                            label={t('日限额（tokens）')}
                            placeholder={t('请输入日限额（0 表示无限制）')}
                            precision={0}
                            min={0}
                            step={1}
                            rules={[
                              { required: true, message: t('请输入日限额（0 表示无限制）') },
                              {
                                validator: (rule, v) => {
                                  const num = parseNonNegativeIntegerValue(v);
                                  return num !== null
                                    ? Promise.resolve()
                                    : Promise.reject(t('每日额度必须大于等于0'));
                                },
                              },
                            ]}
                            extraText={t('0 表示无限制')}
                            style={{ width: '100%' }}
                          />
                        </Col>
                      )}
                      <Col span={24}>
                        <Form.InputNumber
                          field='quota_valid_days'
                          label={t('额度有效期（天）')}
                          min={0}
                          step={1}
                          precision={0}
                          rules={[
                            {
                              required: true,
                              message: t('请输入额度有效期（天）'),
                            },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num >= 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度有效期（天）不能小于0'));
                              },
                            },
                          ]}
                          extraText={t('0 表示永久')}
                          style={{ width: '100%' }}
                        />
                      </Col>
                    </Row>
                  ) : (
                    <Row gutter={12}>
                      <Col span={24}>
                        <Form.AutoComplete
                          field='quota_usd'
                          label={`${t('额度')} ($)`}
                          placeholder={
                            values.mode === 'subscription'
                              ? t('请输入额度（美元，0 表示无限制）')
                              : `${t('请输入额度')} ($)`
                          }
                          style={{ width: '100%' }}
                          type='number'
                          rules={[
                            { required: true, message: t('请输入额度') },
                            {
                              validator: (rule, v) => {
                                const num = parseFloat(v);
                                if (!Number.isFinite(num)) {
                                  return Promise.reject(t('额度必须大于0'));
                                }
                                if (values.mode === 'subscription') {
                                  return num >= 0
                                    ? Promise.resolve()
                                    : Promise.reject(t('额度必须大于等于0'));
                                }
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度必须大于0'));
                              },
                            },
                          ]}
                          extraText={
                            values.mode === 'subscription' &&
                            Number(values.quota_usd) === 0
                              ? t('折算额度：无限制')
                              : t('折算额度：{{amount}} tokens', {
                                  amount: renderNumber(convertUSDToQuota(values.quota_usd)),
                                })
                          }
                          data={[
                            { value: 1, label: '$1' },
                            { value: 10, label: '$10' },
                            { value: 50, label: '$50' },
                            { value: 100, label: '$100' },
                            { value: 500, label: '$500' },
                            { value: 1000, label: '$1000' },
                          ]}
                          showClear
                        />
                      </Col>

                      {values.mode === 'subscription' ? (
                        <>
                          <Col span={24}>
                            <Form.Switch
                              field='use_group_daily_limits'
                              label={t('按分组设置日限额')}
                              extraText={t(
                                '开启后，每个分组独立计算日限额（0 表示该分组无限制）；总剩余额度仍共享，任一分组消耗都会减少总剩余',
                              )}
                              onChange={(checked) => {
                                if (!formApiRef.current) return;
                                if (checked) {
                                  const allowed = normalizeGroupIds(
                                    formApiRef.current.getValue('allowed_group_ids'),
                                  );
                                  const current =
                                    formApiRef.current.getValue('group_daily_limits');
                                  formApiRef.current.setValue(
                                    'group_daily_limits',
                                    syncGroupDailyLimitsUSD(allowed, current),
                                  );
                                } else {
                                  formApiRef.current.setValue('group_daily_limits', []);
                                }
                              }}
                            />
                          </Col>
                          {values.use_group_daily_limits ? (
                            <Col span={24}>
                              <Form.Slot
                                label={t('分组日限额（USD）')}
                                extraText={t('0 表示该分组无限制')}
                              >
                                {(() => {
                                  const allowed = normalizeGroupIds(values.allowed_group_ids);
                                  const limits = syncGroupDailyLimitsUSD(
                                    allowed,
                                    values.group_daily_limits,
                                  );
                                  let totalTokens = 0;
                                  let hasUnlimited = false;
                                  limits.forEach((item) => {
                                    const usd = parseFloat(item?.daily_quota_limit_usd);
                                    const tokens =
                                      Number.isFinite(usd) && usd > 0 ? convertUSDToQuota(usd) : 0;
                                    if (tokens === 0) {
                                      hasUnlimited = true;
                                    } else {
                                      totalTokens += tokens;
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
                                            key={`daily-limit-${item.group_id}`}
                                            className='flex items-center justify-between gap-3'
                                          >
                                            <Text code style={{ fontSize: 12 }}>
                                              {groupLabelById[item.group_id] || t('未知分组')}
                                            </Text>
                                            <InputNumber
                                              value={item.daily_quota_limit_usd}
                                              placeholder={t('0 表示无限制')}
                                              precision={2}
                                              min={0}
                                              step={1}
                                              prefix='$'
                                              style={{ width: 160 }}
                                              onChange={(v) => {
                                                const next = limits.map((row) =>
                                                  row.group_id === item.group_id
                                                    ? { ...row, daily_quota_limit_usd: v ?? 0 }
                                                    : row,
                                                );
                                                formApiRef.current?.setValue(
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
                                          : t('总日限额：{{amount}} tokens', {
                                              amount: renderNumber(totalTokens),
                                            })}
                                      </Text>
                                    </div>
                                  );
                                })()}
                              </Form.Slot>
                            </Col>
                          ) : (
                            <Col span={24}>
                              <Form.InputNumber
                                field='daily_quota_limit_usd'
                                label={t('日限额（USD）')}
                                placeholder={t('请输入日限额（美元，0 表示无限制）')}
                                precision={2}
                                min={0}
                                step={1}
                                prefix='$'
                                rules={[
                                  {
                                    required: true,
                                    message: t('请输入日限额（美元，0 表示无限制）'),
                                  },
                                  {
                                    validator: (rule, v) => {
                                      const num = parseFloat(v);
                                      return Number.isFinite(num) && num >= 0
                                        ? Promise.resolve()
                                        : Promise.reject(t('每日额度必须大于等于0'));
                                    },
                                  },
                                ]}
                                extraText={t('折算额度：{{amount}} tokens', {
                                  amount: renderNumber(
                                    convertUSDToQuota(values.daily_quota_limit_usd),
                                  ),
                                })}
                                style={{ width: '100%' }}
                              />
                            </Col>
                          )}
                          <Col span={24}>
                            <Form.InputNumber
                              field='quota_valid_days'
                              label={t('额度有效期（天）')}
                              min={0}
                              step={1}
                              precision={0}
                              rules={[
                                {
                                  required: true,
                                  message: t('请输入额度有效期（天）'),
                                },
                                {
                                  validator: (rule, v) => {
                                    const num = parseInt(v, 10);
                                    return num >= 0
                                      ? Promise.resolve()
                                      : Promise.reject(t('额度有效期（天）不能小于0'));
                                  },
                                },
                              ]}
                              extraText={t('0 表示永久')}
                              style={{ width: '100%' }}
                            />
                          </Col>
                        </>
                      ) : null}
                    </Row>
                  )}
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>

    </>
  );
};

export default EditRedemptionPresetModal;
