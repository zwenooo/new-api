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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Collapse,
  Divider,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  TagInput,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete } from '@douyinfe/semi-icons';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import RequestRateLimit from '../RateLimit/SettingsRequestRateLimit';
import GroupUserPriceOverridesModal from './GroupUserPriceOverridesModal';

export default function GroupRatioSettings(props) {
  const { t } = useTranslation();
  const { Text, Title } = Typography;
  const readOnlyOptionKeys = useMemo(() => new Set(['GroupGroupRatio']), []);
  const noBillingProductKindLabels = useMemo(
    () => ({
      subscription: t('订阅额度'),
      tokens: t('tokens 订阅'),
      request: t('次数订阅'),
      payg: t('按量商品'),
      pay_request: t('按次商品'),
      pay_token: t('按token商品'),
    }),
    [t],
  );

  const maxInt32 = 2147483647;
  const rateLimitGroupOptionRaw =
    typeof props.options?.ModelRequestRateLimitGroup === 'string'
      ? props.options.ModelRequestRateLimitGroup
      : '';
  const lastSyncedRateLimitGroupOptionRawRef = useRef('');

  const [loading, setLoading] = useState(false);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsSaving, setGroupsSaving] = useState(false);
  const [deletingGroupId, setDeletingGroupId] = useState(0);
  const [groups, setGroups] = useState([]);
  const [originGroups, setOriginGroups] = useState([]);
  const [groupKeyword, setGroupKeyword] = useState('');
  const persistedHideDisabledGroups =
    typeof props.options?.GroupManagementHideDisabledEnabled === 'boolean'
      ? props.options.GroupManagementHideDisabledEnabled
      : true;
  const [hideDisabledGroups, setHideDisabledGroups] = useState(
    persistedHideDisabledGroups,
  );
  const [hideDisabledGroupsSaving, setHideDisabledGroupsSaving] =
    useState(false);
  const [createVisible, setCreateVisible] = useState(false);
  const [creating, setCreating] = useState(false);
  const [modelsEditorVisible, setModelsEditorVisible] = useState(false);
  const [modelsEditorGroupId, setModelsEditorGroupId] = useState(0);
  const [modelsEditorValue, setModelsEditorValue] = useState([]);
  const [modelsEditorPrefillGroupIds, setModelsEditorPrefillGroupIds] =
    useState([]);
  const [uaEditorVisible, setUaEditorVisible] = useState(false);
  const [uaEditorGroupId, setUaEditorGroupId] = useState(0);
  const [uaEditorValue, setUaEditorValue] = useState([]);
  const [modelPrefillGroups, setModelPrefillGroups] = useState([]);
  const [modelPrefillGroupsLoading, setModelPrefillGroupsLoading] =
    useState(false);
  const [selectedModelPrefillGroupId, setSelectedModelPrefillGroupId] =
    useState(0);
  const [noBillingProductOptions, setNoBillingProductOptions] = useState([]);
  const [noBillingProductOptionsLoading, setNoBillingProductOptionsLoading] =
    useState(false);
  const [noBillingProductsEditorVisible, setNoBillingProductsEditorVisible] =
    useState(false);
  const [noBillingProductsEditorGroupId, setNoBillingProductsEditorGroupId] =
    useState(0);
  const [noBillingProductsEditorValue, setNoBillingProductsEditorValue] =
    useState([]);
  const [groupChannelBindingsByGroupId, setGroupChannelBindingsByGroupId] =
    useState({});
  const [groupChannelsEditorVisible, setGroupChannelsEditorVisible] =
    useState(false);
  const [groupChannelsEditorGroupId, setGroupChannelsEditorGroupId] =
    useState(0);
  const [groupChannelsEditorValue, setGroupChannelsEditorValue] = useState([]);
  const [groupChannelsEditorLoading, setGroupChannelsEditorLoading] =
    useState(false);
  const [groupChannelsEditorSaving, setGroupChannelsEditorSaving] =
    useState(false);
  const [tokenRemapVisible, setTokenRemapVisible] = useState(false);
  const [tokenRemapTargetGroupId, setTokenRemapTargetGroupId] = useState(0);
  const [tokenRemapSaving, setTokenRemapSaving] = useState(false);
  const [
    groupUserPriceOverridesByGroupId,
    setGroupUserPriceOverridesByGroupId,
  ] = useState({});
  const [groupUserPriceOverridesVisible, setGroupUserPriceOverridesVisible] =
    useState(false);

  const parseRateLimitGroupOption = useCallback((raw) => {
    if (!raw || typeof raw !== 'string') return {};
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (e) {
      return {};
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
      return {};

    const out = {};
    Object.entries(parsed).forEach(([rawKey, rawValue]) => {
      const groupID = parseInt(String(rawKey || '').trim(), 10);
      if (!Number.isInteger(groupID) || groupID <= 0) return;
      if (!Array.isArray(rawValue) || rawValue.length !== 2) return;
      const total = Number(rawValue[0]);
      const success = Number(rawValue[1]);
      if (!Number.isFinite(total) || !Number.isInteger(total) || total < 0)
        return;
      if (
        !Number.isFinite(success) ||
        !Number.isInteger(success) ||
        success < 0
      )
        return;
      out[groupID] = { total, success };
    });
    return out;
  }, []);

  const normalizeEntityIds = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    const out = [];
    const seen = new Set();
    list.forEach((item) => {
      const id = Number(item);
      if (!Number.isInteger(id) || id <= 0) return;
      if (seen.has(id)) return;
      seen.add(id);
      out.push(id);
    });
    out.sort((a, b) => a - b);
    return out;
  }, []);

  const [rateLimitGroup, setRateLimitGroup] = useState({});
  const [originRateLimitGroup, setOriginRateLimitGroup] = useState({});
  const [rateLimitGroupSaving, setRateLimitGroupSaving] = useState(false);

  const [inputs, setInputs] = useState({
    GroupGroupRatio: '',
    AutoGroups: '',
    DefaultUseAutoGroup: false,
  });
  const refForm = useRef();
  const refCreateForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const [selectedGroupId, setSelectedGroupId] = useState(0);

  const groupNameById = useMemo(() => {
    const m = new Map();
    (Array.isArray(groups) ? groups : []).forEach((g) => {
      const id = Number(g?.id || 0);
      if (!Number.isInteger(id) || id <= 0) return;
      const name =
        String(g?.name || '').trim() ||
        String(g?.display_name || '').trim() ||
        String(g?.code || '').trim() ||
        String(id);
      m.set(id, name);
    });
    return m;
  }, [groups]);

  const originGroupById = useMemo(() => {
    const m = new Map();
    (Array.isArray(originGroups) ? originGroups : []).forEach((g) => {
      const id = Number(g?.id || 0);
      if (!Number.isInteger(id) || id <= 0) return;
      m.set(id, g);
    });
    return m;
  }, [originGroups]);

  const dirtyRateLimitGroupIds = useMemo(() => {
    const keys = new Set([
      ...Object.keys(originRateLimitGroup || {}),
      ...Object.keys(rateLimitGroup || {}),
    ]);

    const dirty = [];
    keys.forEach((gidStr) => {
      const gid = Number(gidStr);
      if (!Number.isInteger(gid) || gid <= 0) return;

      const cur = rateLimitGroup?.[gid];
      const org = originRateLimitGroup?.[gid];

      const curTotal = Number.isFinite(cur?.total) ? cur.total : undefined;
      const curSuccess = Number.isFinite(cur?.success)
        ? cur.success
        : undefined;
      const orgTotal = Number.isFinite(org?.total) ? org.total : undefined;
      const orgSuccess = Number.isFinite(org?.success)
        ? org.success
        : undefined;

      if (curTotal !== orgTotal || curSuccess !== orgSuccess) {
        dirty.push(gid);
      }
    });
    dirty.sort((a, b) => a - b);
    return dirty;
  }, [originRateLimitGroup, rateLimitGroup]);

  useEffect(() => {
    if (dirtyRateLimitGroupIds.length) return;
    if (
      rateLimitGroupOptionRaw === lastSyncedRateLimitGroupOptionRawRef.current
    )
      return;
    const parsed = parseRateLimitGroupOption(rateLimitGroupOptionRaw);
    setRateLimitGroup(parsed);
    setOriginRateLimitGroup(structuredClone(parsed));
    lastSyncedRateLimitGroupOptionRawRef.current = rateLimitGroupOptionRaw;
  }, [
    dirtyRateLimitGroupIds.length,
    parseRateLimitGroupOption,
    rateLimitGroupOptionRaw,
  ]);

  useEffect(() => {
    setHideDisabledGroups(persistedHideDisabledGroups);
  }, [persistedHideDisabledGroups]);

  const updateRateLimitGroupLocal = useCallback((groupID, field, value) => {
    const gid = Number(groupID || 0);
    if (!Number.isInteger(gid) || gid <= 0) return;

    setRateLimitGroup((prev) => {
      const next = { ...(prev || {}) };
      const current = next[gid] ? { ...next[gid] } : {};
      let normalizedValue;
      if (value === undefined || value === null || value === '') {
        normalizedValue = undefined;
      } else {
        const n = Number(value);
        normalizedValue = Number.isFinite(n) ? n : undefined;
      }

      if (normalizedValue === undefined) {
        delete current[field];
      } else {
        current[field] = normalizedValue;
      }
      const hasTotal = Number.isFinite(current.total);
      const hasSuccess = Number.isFinite(current.success);
      if (!hasTotal && !hasSuccess) {
        delete next[gid];
      } else {
        next[gid] = current;
      }
      return next;
    });
  }, []);

  const saveRateLimitGroup = useCallback(async () => {
    if (!dirtyRateLimitGroupIds.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }

    const payload = {};
    for (const [gidStr, raw] of Object.entries(rateLimitGroup || {})) {
      const gid = Number(gidStr);
      if (!Number.isInteger(gid) || gid <= 0) continue;

      const total = raw?.total;
      const success = raw?.success;
      const groupName = groupNameById.get(gid) || String(gid);

      const hasTotal = Number.isFinite(total);
      const hasSuccess = Number.isFinite(success);
      if (hasTotal !== hasSuccess) {
        showError(
          t(
            '分组 {{group}} 的限速配置不完整：请同时填写「总请求次数」和「完成次数」，或都留空表示继承全局',
            { group: groupName },
          ),
        );
        return;
      }

      if (!hasTotal && !hasSuccess) {
        continue;
      }

      if (!Number.isInteger(total) || total < 0 || total > maxInt32) {
        showError(
          t('分组 {{group}} 的「总请求次数」无效', { group: groupName }),
        );
        return;
      }
      if (!Number.isInteger(success) || success < 0 || success > maxInt32) {
        showError(t('分组 {{group}} 的「完成次数」无效', { group: groupName }));
        return;
      }
      payload[String(gid)] = [total, success];
    }

    setRateLimitGroupSaving(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ModelRequestRateLimitGroup',
        value: JSON.stringify(payload, null, 2),
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('保存失败'));
        return;
      }
      // Avoid sync-from-props overwriting local saved state before refresh returns.
      lastSyncedRateLimitGroupOptionRawRef.current = rateLimitGroupOptionRaw;
      setOriginRateLimitGroup(structuredClone(rateLimitGroup));
      showSuccess(t('保存成功'));
      props.refresh();
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setRateLimitGroupSaving(false);
    }
  }, [
    dirtyRateLimitGroupIds.length,
    groupNameById,
    maxInt32,
    props.refresh,
    rateLimitGroup,
    rateLimitGroupOptionRaw,
    t,
  ]);

  const updateHideDisabledGroups = useCallback(
    async (checked) => {
      const nextValue = Boolean(checked);
      const previousValue = hideDisabledGroups;
      setHideDisabledGroups(nextValue);
      setHideDisabledGroupsSaving(true);
      try {
        const res = await API.put('/api/option/', {
          key: 'GroupManagementHideDisabledEnabled',
          value: String(nextValue),
        });
        const { success, message } = res.data;
        if (!success) {
          setHideDisabledGroups(previousValue);
          showError(message || t('保存失败'));
          return;
        }
        await props.refresh();
      } catch (e) {
        setHideDisabledGroups(previousValue);
        showError(e?.message || t('保存失败'));
      } finally {
        setHideDisabledGroupsSaving(false);
      }
    },
    [hideDisabledGroups, props.refresh, t],
  );

  const normalizeAllowedModels = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    const out = [];
    const seen = new Set();
    list.forEach((item) => {
      const name = String(item || '').trim();
      if (!name) return;
      if (seen.has(name)) return;
      seen.add(name);
      out.push(name);
    });
    out.sort((a, b) => a.localeCompare(b));
    return out;
  }, []);

  const normalizeAllowedModelPrefillGroupIds = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    const out = [];
    const seen = new Set();
    list.forEach((item) => {
      const id = Number(item);
      if (!Number.isInteger(id) || id <= 0) return;
      if (seen.has(id)) return;
      seen.add(id);
      out.push(id);
    });
    out.sort((a, b) => a - b);
    return out;
  }, []);

  const normalizeAllowedUserAgents = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    const out = [];
    const seen = new Set();
    list.forEach((item) => {
      const keyword = String(item || '')
        .trim()
        .toLowerCase();
      if (!keyword) return;
      if (seen.has(keyword)) return;
      seen.add(keyword);
      out.push(keyword);
    });
    out.sort((a, b) => a.localeCompare(b));
    return out;
  }, []);

  const normalizeNoBillingProductKeys = useCallback(
    (raw) => {
      const list = Array.isArray(raw) ? raw : [];
      const out = [];
      const seen = new Set();
      list.forEach((item) => {
        const value = String(item || '').trim();
        if (!value) return;
        const parts = value.split(':');
        if (parts.length !== 2) return;
        const kind = String(parts[0] || '').trim();
        const productId = Number(parts[1]);
        if (!noBillingProductKindLabels[kind]) return;
        if (!Number.isInteger(productId) || productId <= 0) return;
        const key = `${kind}:${productId}`;
        if (seen.has(key)) return;
        seen.add(key);
        out.push(key);
      });
      out.sort((a, b) => a.localeCompare(b));
      return out;
    },
    [noBillingProductKindLabels],
  );

  const normalizeChannelBindingIds = normalizeEntityIds;

  const normalizeGroupChannelBindingItems = useCallback(
    (raw, groupId) => {
      const targetGroupId = Number(groupId || 0);
      const list = Array.isArray(raw) ? raw : [];
      return list
        .map((item) => {
          const id = Number(item?.id || 0);
          if (!Number.isInteger(id) || id <= 0) return null;
          const groupIds = normalizeChannelBindingIds(item?.group_ids);
          return {
            id,
            name: String(item?.name || '').trim(),
            status: Number(item?.status || 0),
            type: Number(item?.type || 0),
            tag: String(item?.tag || '').trim(),
            group_ids: groupIds,
            selected:
              item?.selected === true ||
              (Number.isInteger(targetGroupId) &&
                targetGroupId > 0 &&
                groupIds.includes(targetGroupId)),
          };
        })
        .filter(Boolean);
    },
    [normalizeChannelBindingIds],
  );

  const formatNoBillingProductKeyLabel = useCallback(
    (key) => {
      const value = String(key || '').trim();
      if (!value) return '';
      const parts = value.split(':');
      if (parts.length !== 2) return value;
      const kind = String(parts[0] || '').trim();
      const productId = Number(parts[1]);
      const kindLabel = noBillingProductKindLabels[kind] || kind;
      if (!Number.isInteger(productId) || productId <= 0) {
        return kindLabel;
      }
      return `${kindLabel} (#${productId})`;
    },
    [noBillingProductKindLabels],
  );

  const noBillingProductOptionList = useMemo(() => {
    return (
      Array.isArray(noBillingProductOptions) ? noBillingProductOptions : []
    ).map((item) => {
      const kindLabel =
        noBillingProductKindLabels[String(item?.kind || '').trim()] ||
        String(item?.kind || '').trim();
      const productId = Number(item?.product_id || 0);
      const suffix =
        Number.isInteger(productId) && productId > 0 ? ` (#${productId})` : '';
      const disabledText = item?.enabled === false ? ` ${t('（已下架）')}` : '';
      return {
        value: String(item?.key || '').trim(),
        label: `[${kindLabel}] ${
          String(item?.name || '').trim() || t('未命名商品')
        }${suffix}${disabledText}`,
      };
    });
  }, [noBillingProductKindLabels, noBillingProductOptions, t]);

  const noBillingProductLabelByKey = useMemo(() => {
    const m = new Map();
    noBillingProductOptionList.forEach((item) => {
      const key = String(item?.value || '').trim();
      if (!key) return;
      m.set(key, String(item?.label || '').trim());
    });
    return m;
  }, [noBillingProductOptionList]);

  const normalizeGroups = useCallback(
    (raw) => {
      const list = Array.isArray(raw) ? raw : [];
      return list
        .map((g) => {
          if (typeof g === 'string') {
            const name = String(g || '').trim();
            return name
              ? {
                  id: 0,
                  code: name,
                  name,
                  display_name: name,
                  description: name,
                  allowed_models: [],
                  allowed_model_prefill_group_ids: [],
                  allowed_user_agents: [],
                  ratio: 1,
                  no_billing: false,
                  no_billing_product_keys: [],
                  user_selectable: true,
                  enabled: true,
                }
              : null;
          }
          const name = String(
            g?.name || g?.code || g?.display_name || '',
          ).trim();
          if (!name) return null;
          const code = String(g?.code || name).trim() || name;
          const id = Number(g?.id ?? 0);
          const description = String(g?.description || '').trim();
          const allowedModels = normalizeAllowedModels(g?.allowed_models);
          const allowedModelPrefillGroupIds =
            normalizeAllowedModelPrefillGroupIds(
              g?.allowed_model_prefill_group_ids,
            );
          const allowedUserAgents = normalizeAllowedUserAgents(
            g?.allowed_user_agents,
          );
          return {
            id: Number.isInteger(id) && id > 0 ? id : 0,
            code,
            name,
            display_name: name,
            description,
            allowed_models: allowedModels,
            allowed_model_prefill_group_ids: allowedModelPrefillGroupIds,
            allowed_user_agents: allowedUserAgents,
            ratio: Number(g?.ratio ?? 1),
            no_billing: Boolean(g?.no_billing),
            no_billing_product_keys: normalizeNoBillingProductKeys(
              g?.no_billing_product_keys,
            ),
            user_selectable: Boolean(g?.user_selectable),
            enabled: g?.enabled === undefined ? true : Boolean(g?.enabled),
          };
        })
        .filter(Boolean);
    },
    [
      normalizeNoBillingProductKeys,
      normalizeAllowedModelPrefillGroupIds,
      normalizeAllowedModels,
      normalizeAllowedUserAgents,
    ],
  );

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    try {
      const res = await API.get('/api/group/');
      const { success, message, data } = res.data;
      if (success) {
        const normalized = normalizeGroups(data);
        setGroups(normalized);
        setOriginGroups(structuredClone(normalized));
      } else {
        showError(message || t('获取分组失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
    } finally {
      setGroupsLoading(false);
    }
  }, [normalizeGroups, t]);

  const loadNoBillingProductOptions = useCallback(async () => {
    setNoBillingProductOptionsLoading(true);
    try {
      const res = await API.get('/api/group/no_billing/product_options');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('加载限定商品失败'));
        return;
      }
      setNoBillingProductOptions(Array.isArray(data) ? data : []);
    } catch (e) {
      showError(e?.message || t('加载限定商品失败'));
    } finally {
      setNoBillingProductOptionsLoading(false);
    }
  }, [t]);

  const updateGroupLocal = useCallback((id, patch) => {
    const groupID = Number(id || 0);
    setGroups((prev) =>
      (Array.isArray(prev) ? prev : []).map((g) =>
        Number(g?.id || 0) === groupID ? { ...g, ...patch } : g,
      ),
    );
  }, []);

  const updateOriginGroupLocal = useCallback((id, patch) => {
    const groupID = Number(id || 0);
    setOriginGroups((prev) =>
      (Array.isArray(prev) ? prev : []).map((g) =>
        Number(g?.id || 0) === groupID ? { ...g, ...patch } : g,
      ),
    );
  }, []);

  const buildGroupUpdatePayload = useCallback(
    (group) => {
      return {
        id: Number(group?.id || 0),
        name: String(group?.name || '').trim(),
        description: String(group?.description || '').trim(),
        allowed_models: normalizeAllowedModels(group?.allowed_models),
        allowed_model_prefill_group_ids: normalizeAllowedModelPrefillGroupIds(
          group?.allowed_model_prefill_group_ids,
        ),
        allowed_user_agents: normalizeAllowedUserAgents(
          group?.allowed_user_agents,
        ),
        ratio: Number(group?.ratio ?? 1),
        no_billing: Boolean(group?.no_billing),
        no_billing_product_keys: normalizeNoBillingProductKeys(
          group?.no_billing_product_keys,
        ),
        user_selectable: Boolean(group?.user_selectable),
        enabled: Boolean(group?.enabled),
      };
    },
    [
      normalizeNoBillingProductKeys,
      normalizeAllowedModelPrefillGroupIds,
      normalizeAllowedModels,
      normalizeAllowedUserAgents,
    ],
  );

  const dirtyGroupIds = useMemo(() => {
    const originById = new Map();
    (Array.isArray(originGroups) ? originGroups : []).forEach((g) => {
      const id = Number(g?.id || 0);
      if (Number.isInteger(id) && id > 0) {
        originById.set(id, buildGroupUpdatePayload(g));
      }
    });

    const dirty = [];
    (Array.isArray(groups) ? groups : []).forEach((g) => {
      const payload = buildGroupUpdatePayload(g);
      if (!Number.isInteger(payload.id) || payload.id <= 0) return;
      const origin = originById.get(payload.id);
      if (!origin) return;
      if (
        origin.name !== payload.name ||
        origin.description !== payload.description ||
        JSON.stringify(origin.allowed_models || []) !==
          JSON.stringify(payload.allowed_models || []) ||
        JSON.stringify(origin.allowed_model_prefill_group_ids || []) !==
          JSON.stringify(payload.allowed_model_prefill_group_ids || []) ||
        JSON.stringify(origin.allowed_user_agents || []) !==
          JSON.stringify(payload.allowed_user_agents || []) ||
        JSON.stringify(origin.no_billing_product_keys || []) !==
          JSON.stringify(payload.no_billing_product_keys || []) ||
        origin.ratio !== payload.ratio ||
        origin.no_billing !== payload.no_billing ||
        origin.user_selectable !== payload.user_selectable ||
        origin.enabled !== payload.enabled
      ) {
        dirty.push(payload.id);
      }
    });
    return dirty;
  }, [buildGroupUpdatePayload, groups, originGroups]);

  const dirtyGroupSet = useMemo(() => new Set(dirtyGroupIds), [dirtyGroupIds]);
  const dirtyRateLimitGroupSet = useMemo(
    () => new Set(dirtyRateLimitGroupIds),
    [dirtyRateLimitGroupIds],
  );

  const filteredGroups = useMemo(() => {
    const keyword = String(groupKeyword || '')
      .trim()
      .toLowerCase();
    const dirtySet = new Set(dirtyGroupIds);
    return (Array.isArray(groups) ? groups : []).filter((group) => {
      const id = Number(group?.id || 0);
      if (hideDisabledGroups && !Boolean(group?.enabled)) {
        if (!Number.isInteger(id) || id <= 0 || !dirtySet.has(id)) {
          return false;
        }
      }
      if (!keyword) {
        return true;
      }
      const fields = [
        String(group?.name || '').trim(),
        String(group?.display_name || '').trim(),
        String(group?.code || '').trim(),
        String(group?.description || '').trim(),
        Number.isInteger(id) && id > 0 ? String(id) : '',
      ];
      return fields.some((field) => field.toLowerCase().includes(keyword));
    });
  }, [dirtyGroupIds, groupKeyword, groups, hideDisabledGroups]);

  const selectedGroup = useMemo(() => {
    const id = Number(selectedGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return null;
    return (
      (Array.isArray(groups) ? groups : []).find(
        (group) => Number(group?.id || 0) === id,
      ) || null
    );
  }, [groups, selectedGroupId]);

  const selectedGroupNumericId = Number(selectedGroup?.id || 0);
  const selectedGroupDirty = dirtyGroupSet.has(selectedGroupNumericId);
  const selectedGroupRateDirty = dirtyRateLimitGroupSet.has(
    selectedGroupNumericId,
  );

  useEffect(() => {
    const firstVisibleGroupId = Number(filteredGroups?.[0]?.id || 0);
    if (!filteredGroups.length) {
      if (selectedGroupId !== 0) setSelectedGroupId(0);
      return;
    }
    const hasSelectedGroup = filteredGroups.some(
      (group) => Number(group?.id || 0) === Number(selectedGroupId || 0),
    );
    if (
      !hasSelectedGroup &&
      Number.isInteger(firstVisibleGroupId) &&
      firstVisibleGroupId > 0
    ) {
      setSelectedGroupId(firstVisibleGroupId);
    }
  }, [filteredGroups, selectedGroupId]);

  const saveAllGroups = useCallback(async () => {
    if (!dirtyGroupIds.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }

    const dirtySet = new Set(dirtyGroupIds);
    const toSave = (Array.isArray(groups) ? groups : [])
      .filter((g) => dirtySet.has(Number(g?.id || 0)))
      .map((g) => buildGroupUpdatePayload(g));

    for (const payload of toSave) {
      if (!Number.isInteger(payload.id) || payload.id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      if (payload.no_billing && !payload.no_billing_product_keys.length) {
        showError(
          t('分组 {{group}} 开启不计费时必须至少选择一个限定商品', {
            group: payload.name || payload.id,
          }),
        );
        return;
      }
    }

    setGroupsSaving(true);
    try {
      for (const payload of toSave) {
        const res = await API.put('/api/group/', payload);
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('保存失败'));
          return;
        }
        const normalized = normalizeGroups([data])[0];
        if (normalized) {
          updateGroupLocal(payload.id, normalized);
          updateOriginGroupLocal(payload.id, normalized);
        }
      }
      showSuccess(t('保存成功'));
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setGroupsSaving(false);
    }
  }, [
    buildGroupUpdatePayload,
    dirtyGroupIds,
    groups,
    normalizeGroups,
    t,
    updateGroupLocal,
    updateOriginGroupLocal,
  ]);

  const deleteGroup = useCallback(
    async (group) => {
      const id = Number(group?.id || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      setDeletingGroupId(id);
      try {
        const res = await API.delete(`/api/group/${id}`);
        const { success, message } = res.data;
        if (!success) {
          showError(message || t('删除失败'));
          return;
        }
        setGroups((prev) =>
          (Array.isArray(prev) ? prev : []).filter(
            (g) => Number(g?.id || 0) !== id,
          ),
        );
        setOriginGroups((prev) =>
          (Array.isArray(prev) ? prev : []).filter(
            (g) => Number(g?.id || 0) !== id,
          ),
        );
        showSuccess(t('删除成功'));
      } catch (e) {
        showError(e?.message || t('删除失败'));
      } finally {
        setDeletingGroupId(0);
      }
    },
    [t],
  );

  const submitCreateGroup = useCallback(
    async (values) => {
      const name = String(values?.name || '').trim();
      const payload = {
        name,
        code: name,
        description: String(values?.description || '').trim(),
        ratio: Number(values?.ratio ?? 1),
        no_billing: Boolean(values?.no_billing),
        no_billing_product_keys: normalizeNoBillingProductKeys(
          values?.no_billing_product_keys,
        ),
        user_selectable: Boolean(values?.user_selectable),
        enabled: Boolean(values?.enabled),
      };
      if (payload.no_billing && !payload.no_billing_product_keys.length) {
        showError(t('开启不计费时必须至少选择一个限定商品'));
        return;
      }
      setCreating(true);
      try {
        const res = await API.post('/api/group/', payload);
        const { success, message } = res.data;
        if (!success) {
          showError(message || t('创建失败'));
          return;
        }
        showSuccess(t('创建成功'));
        setCreateVisible(false);
        await loadGroups();
      } catch (e) {
        showError(e?.message || t('创建失败'));
      } finally {
        setCreating(false);
      }
    },
    [loadGroups, normalizeNoBillingProductKeys, t],
  );

  const openModelsEditor = useCallback(
    (group) => {
      const id = Number(group?.id || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      setModelsEditorGroupId(id);
      setModelsEditorValue(normalizeAllowedModels(group?.allowed_models));
      setModelsEditorPrefillGroupIds(
        normalizeAllowedModelPrefillGroupIds(
          group?.allowed_model_prefill_group_ids,
        ),
      );
      setModelsEditorVisible(true);
    },
    [normalizeAllowedModelPrefillGroupIds, normalizeAllowedModels, t],
  );

  const closeModelsEditor = useCallback(() => {
    setModelsEditorVisible(false);
    setModelsEditorGroupId(0);
    setModelsEditorValue([]);
    setModelsEditorPrefillGroupIds([]);
  }, []);

  const saveModelsEditor = useCallback(() => {
    const id = Number(modelsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) {
      showError(t('分组 id 无效'));
      return;
    }
    updateGroupLocal(id, {
      allowed_models: normalizeAllowedModels(modelsEditorValue),
      allowed_model_prefill_group_ids: normalizeAllowedModelPrefillGroupIds(
        modelsEditorPrefillGroupIds,
      ),
    });
    closeModelsEditor();
  }, [
    closeModelsEditor,
    modelsEditorGroupId,
    modelsEditorPrefillGroupIds,
    modelsEditorValue,
    normalizeAllowedModelPrefillGroupIds,
    normalizeAllowedModels,
    t,
    updateGroupLocal,
  ]);

  const modelsEditorGroupName = useMemo(() => {
    const id = Number(modelsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return '';
    return groupNameById.get(id) || String(id);
  }, [groupNameById, modelsEditorGroupId]);

  const openUAEditor = useCallback(
    (group) => {
      const id = Number(group?.id || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      setUaEditorGroupId(id);
      setUaEditorValue(normalizeAllowedUserAgents(group?.allowed_user_agents));
      setUaEditorVisible(true);
    },
    [normalizeAllowedUserAgents, t],
  );

  const closeUAEditor = useCallback(() => {
    setUaEditorVisible(false);
    setUaEditorGroupId(0);
    setUaEditorValue([]);
  }, []);

  const saveUAEditor = useCallback(() => {
    const id = Number(uaEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) {
      showError(t('分组 id 无效'));
      return;
    }
    updateGroupLocal(id, {
      allowed_user_agents: normalizeAllowedUserAgents(uaEditorValue),
    });
    closeUAEditor();
  }, [
    closeUAEditor,
    normalizeAllowedUserAgents,
    t,
    uaEditorGroupId,
    uaEditorValue,
    updateGroupLocal,
  ]);

  const uaEditorGroupName = useMemo(() => {
    const id = Number(uaEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return '';
    return groupNameById.get(id) || String(id);
  }, [groupNameById, uaEditorGroupId]);

  const openNoBillingProductsEditor = useCallback(
    (group) => {
      const id = Number(group?.id || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      setNoBillingProductsEditorGroupId(id);
      setNoBillingProductsEditorValue(
        normalizeNoBillingProductKeys(group?.no_billing_product_keys),
      );
      setNoBillingProductsEditorVisible(true);
      void loadNoBillingProductOptions();
    },
    [loadNoBillingProductOptions, normalizeNoBillingProductKeys, t],
  );

  const closeNoBillingProductsEditor = useCallback(() => {
    setNoBillingProductsEditorVisible(false);
    setNoBillingProductsEditorGroupId(0);
    setNoBillingProductsEditorValue([]);
  }, []);

  const saveNoBillingProductsEditor = useCallback(() => {
    const id = Number(noBillingProductsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) {
      showError(t('分组 id 无效'));
      return;
    }
    updateGroupLocal(id, {
      no_billing_product_keys: normalizeNoBillingProductKeys(
        noBillingProductsEditorValue,
      ),
    });
    closeNoBillingProductsEditor();
  }, [
    closeNoBillingProductsEditor,
    noBillingProductsEditorGroupId,
    noBillingProductsEditorValue,
    normalizeNoBillingProductKeys,
    t,
    updateGroupLocal,
  ]);

  const noBillingProductsEditorGroupName = useMemo(() => {
    const id = Number(noBillingProductsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return '';
    return groupNameById.get(id) || String(id);
  }, [groupNameById, noBillingProductsEditorGroupId]);

  const loadGroupChannels = useCallback(
    async (groupId) => {
      const id = Number(groupId || 0);
      if (!Number.isInteger(id) || id <= 0) {
        throw new Error(t('分组 id 无效'));
      }
      const res = await API.get(`/api/group/${id}/channels`);
      const { success, message, data } = res.data;
      if (!success) {
        throw new Error(message || t('加载分组渠道失败'));
      }
      const items = normalizeGroupChannelBindingItems(data?.items, id);
      setGroupChannelBindingsByGroupId((prev) => ({
        ...(prev || {}),
        [id]: items,
      }));
      return items;
    },
    [normalizeGroupChannelBindingItems, t],
  );

  useEffect(() => {
    if (
      !Number.isInteger(selectedGroupNumericId) ||
      selectedGroupNumericId <= 0
    )
      return;
    if (
      Object.prototype.hasOwnProperty.call(
        groupChannelBindingsByGroupId || {},
        selectedGroupNumericId,
      )
    ) {
      return;
    }
    void loadGroupChannels(selectedGroupNumericId).catch(() => {});
  }, [
    groupChannelBindingsByGroupId,
    loadGroupChannels,
    selectedGroupNumericId,
  ]);

  const openGroupChannelsEditor = useCallback(
    async (group) => {
      const id = Number(group?.id || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('分组 id 无效'));
        return;
      }
      setGroupChannelsEditorGroupId(id);
      setGroupChannelsEditorValue([]);
      setGroupChannelsEditorVisible(true);
      setGroupChannelsEditorLoading(true);
      try {
        const items = await loadGroupChannels(id);
        setGroupChannelsEditorValue(
          normalizeChannelBindingIds(
            items.filter((item) => item?.selected).map((item) => item.id),
          ),
        );
      } catch (e) {
        showError(e?.message || t('加载分组渠道失败'));
      } finally {
        setGroupChannelsEditorLoading(false);
      }
    },
    [loadGroupChannels, normalizeChannelBindingIds, t],
  );

  const closeGroupChannelsEditor = useCallback(() => {
    setGroupChannelsEditorVisible(false);
    setGroupChannelsEditorGroupId(0);
    setGroupChannelsEditorValue([]);
    setGroupChannelsEditorLoading(false);
  }, []);

  const saveGroupChannelsEditor = useCallback(async () => {
    const id = Number(groupChannelsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) {
      showError(t('分组 id 无效'));
      return;
    }

    setGroupChannelsEditorSaving(true);
    try {
      const channelIds = normalizeChannelBindingIds(groupChannelsEditorValue);
      const res = await API.put(`/api/group/${id}/channels`, {
        channel_ids: channelIds,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('保存分组渠道失败'));
        return;
      }
      await loadGroupChannels(id);
      showSuccess(
        t('已同步 {{count}} 个渠道的分组绑定', {
          count: Number(data?.affected ?? 0),
        }),
      );
      closeGroupChannelsEditor();
    } catch (e) {
      showError(
        t('保存分组渠道失败: ') +
          (e?.response?.data?.message || e?.message || e),
      );
    } finally {
      setGroupChannelsEditorSaving(false);
    }
  }, [
    closeGroupChannelsEditor,
    groupChannelsEditorGroupId,
    groupChannelsEditorValue,
    loadGroupChannels,
    normalizeChannelBindingIds,
    t,
  ]);

  const groupChannelsEditorGroupName = useMemo(() => {
    const id = Number(groupChannelsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return '';
    return groupNameById.get(id) || String(id);
  }, [groupChannelsEditorGroupId, groupNameById]);

  const groupChannelsEditorItems = useMemo(() => {
    const id = Number(groupChannelsEditorGroupId || 0);
    if (!Number.isInteger(id) || id <= 0) return [];
    return Array.isArray(groupChannelBindingsByGroupId?.[id])
      ? groupChannelBindingsByGroupId[id]
      : [];
  }, [groupChannelBindingsByGroupId, groupChannelsEditorGroupId]);

  const formatGroupChannelLabel = useCallback(
    (channel) => {
      const id = Number(channel?.id || 0);
      const name = String(channel?.name || '').trim() || t('未命名渠道');
      const suffix = [];
      if (Number.isInteger(id) && id > 0) {
        suffix.push(`#${id}`);
      }
      if (String(channel?.tag || '').trim()) {
        suffix.push(String(channel.tag).trim());
      }
      if (Number(channel?.status) !== 1) {
        suffix.push(t('已停用'));
      }
      return suffix.length ? `${name} (${suffix.join(' · ')})` : name;
    },
    [t],
  );

  const groupChannelsEditorOptionList = useMemo(
    () =>
      groupChannelsEditorItems.map((channel) => ({
        value: channel.id,
        label: formatGroupChannelLabel(channel),
      })),
    [formatGroupChannelLabel, groupChannelsEditorItems],
  );

  const tokenRemapGroupOptionList = useMemo(() => {
    const currentGroupId = Number(selectedGroup?.id || 0);
    return (Array.isArray(groups) ? groups : [])
      .filter((group) => {
        const groupId = Number(group?.id || 0);
        return (
          Number.isInteger(groupId) && groupId > 0 && groupId !== currentGroupId
        );
      })
      .map((group) => {
        const groupId = Number(group?.id || 0);
        return {
          value: groupId,
          label: `${
            String(group?.name || '').trim() || t('未命名分组')
          } (#${groupId})`,
        };
      });
  }, [groups, selectedGroup, t]);

  const saveSelectedGroup = useCallback(async () => {
    const groupId = Number(selectedGroupNumericId || 0);
    if (!Number.isInteger(groupId) || groupId <= 0 || !selectedGroup) {
      showWarning(t('请先选择一个分组'));
      return false;
    }
    if (!selectedGroupDirty) {
      return true;
    }

    const payload = buildGroupUpdatePayload(selectedGroup);
    if (payload.no_billing && !payload.no_billing_product_keys.length) {
      showError(
        t('分组 {{group}} 开启不计费时必须至少选择一个限定商品', {
          group: payload.name || payload.id,
        }),
      );
      return false;
    }

    setGroupsSaving(true);
    try {
      const res = await API.put('/api/group/', payload);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('保存失败'));
        return false;
      }
      const normalized = normalizeGroups([data])[0];
      if (normalized) {
        updateGroupLocal(groupId, normalized);
        updateOriginGroupLocal(groupId, normalized);
      }
      return true;
    } catch (e) {
      showError(e?.message || t('保存失败'));
      return false;
    } finally {
      setGroupsSaving(false);
    }
  }, [
    buildGroupUpdatePayload,
    normalizeGroups,
    selectedGroup,
    selectedGroupDirty,
    selectedGroupNumericId,
    t,
    updateGroupLocal,
    updateOriginGroupLocal,
  ]);

  const saveSelectedRateLimitGroup = useCallback(async () => {
    const groupId = Number(selectedGroupNumericId || 0);
    if (!Number.isInteger(groupId) || groupId <= 0 || !selectedGroup) {
      showWarning(t('请先选择一个分组'));
      return false;
    }
    if (!selectedGroupRateDirty) {
      return true;
    }

    const payload = {};
    const keys = new Set([
      ...Object.keys(originRateLimitGroup || {}),
      ...Object.keys(rateLimitGroup || {}),
    ]);
    for (const gidStr of keys) {
      const gid = Number(gidStr);
      if (!Number.isInteger(gid) || gid <= 0) continue;

      const source =
        gid === groupId ? rateLimitGroup?.[gid] : originRateLimitGroup?.[gid];
      const total = source?.total;
      const success = source?.success;
      const groupName = groupNameById.get(gid) || String(gid);
      const hasTotal = Number.isFinite(total);
      const hasSuccess = Number.isFinite(success);

      if (gid === groupId && hasTotal !== hasSuccess) {
        showError(
          t(
            '分组 {{group}} 的限速配置不完整：请同时填写「总请求次数」和「完成次数」，或都留空表示继承全局',
            { group: groupName },
          ),
        );
        return false;
      }
      if (!hasTotal && !hasSuccess) {
        continue;
      }

      if (gid === groupId) {
        if (!Number.isInteger(total) || total < 0 || total > maxInt32) {
          showError(
            t('分组 {{group}} 的「总请求次数」无效', { group: groupName }),
          );
          return false;
        }
        if (!Number.isInteger(success) || success < 0 || success > maxInt32) {
          showError(
            t('分组 {{group}} 的「完成次数」无效', { group: groupName }),
          );
          return false;
        }
      }

      if (hasTotal && hasSuccess) {
        payload[String(gid)] = [total, success];
      }
    }

    setRateLimitGroupSaving(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ModelRequestRateLimitGroup',
        value: JSON.stringify(payload, null, 2),
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('保存失败'));
        return false;
      }
      lastSyncedRateLimitGroupOptionRawRef.current = JSON.stringify(
        payload,
        null,
        2,
      );
      setOriginRateLimitGroup((prev) => {
        const next = { ...(prev || {}) };
        if (rateLimitGroup?.[groupId]) {
          next[groupId] = structuredClone(rateLimitGroup[groupId]);
        } else {
          delete next[groupId];
        }
        return next;
      });
      props.refresh();
      return true;
    } catch (e) {
      showError(e?.message || t('保存失败'));
      return false;
    } finally {
      setRateLimitGroupSaving(false);
    }
  }, [
    groupNameById,
    maxInt32,
    originRateLimitGroup,
    props.refresh,
    rateLimitGroup,
    selectedGroup,
    selectedGroupNumericId,
    selectedGroupRateDirty,
    t,
  ]);

  const saveSelectedWorkspace = useCallback(async () => {
    const groupId = Number(selectedGroupNumericId || 0);
    if (!Number.isInteger(groupId) || groupId <= 0 || !selectedGroup) {
      showWarning(t('请先选择一个分组'));
      return;
    }
    if (!selectedGroupDirty && !selectedGroupRateDirty) {
      showWarning(t('当前分组没有待保存改动'));
      return;
    }

    const groupSaved = await saveSelectedGroup();
    if (!groupSaved) {
      return;
    }
    const rateSaved = await saveSelectedRateLimitGroup();
    if (!rateSaved) {
      return;
    }
    showSuccess(t('当前分组已保存'));
  }, [
    saveSelectedGroup,
    saveSelectedRateLimitGroup,
    selectedGroup,
    selectedGroupDirty,
    selectedGroupNumericId,
    selectedGroupRateDirty,
    t,
  ]);

  const resetSelectedWorkspace = useCallback(() => {
    const groupId = Number(selectedGroupNumericId || 0);
    if (!Number.isInteger(groupId) || groupId <= 0 || !selectedGroup) {
      showWarning(t('请先选择一个分组'));
      return;
    }
    if (!selectedGroupDirty && !selectedGroupRateDirty) {
      showWarning(t('当前分组没有待撤销改动'));
      return;
    }

    const originGroup = originGroupById.get(groupId);
    if (originGroup) {
      updateGroupLocal(groupId, structuredClone(originGroup));
    }

    setRateLimitGroup((prev) => {
      const next = { ...(prev || {}) };
      if (originRateLimitGroup?.[groupId]) {
        next[groupId] = structuredClone(originRateLimitGroup[groupId]);
      } else {
        delete next[groupId];
      }
      return next;
    });
    showSuccess(t('已撤销当前分组的未保存改动'));
  }, [
    originGroupById,
    originRateLimitGroup,
    selectedGroup,
    selectedGroupDirty,
    selectedGroupNumericId,
    selectedGroupRateDirty,
    t,
    updateGroupLocal,
  ]);

  const openTokenRemapModal = useCallback(() => {
    const currentGroupId = Number(selectedGroup?.id || 0);
    if (!Number.isInteger(currentGroupId) || currentGroupId <= 0) {
      showWarning(t('请先选择一个分组'));
      return;
    }
    setTokenRemapTargetGroupId(0);
    setTokenRemapVisible(true);
  }, [selectedGroup, t]);

  const closeTokenRemapModal = useCallback(() => {
    setTokenRemapVisible(false);
    setTokenRemapTargetGroupId(0);
  }, []);

  const submitTokenRemap = useCallback(async () => {
    const sourceGroupId = Number(selectedGroup?.id || 0);
    const targetGroupId = Number(tokenRemapTargetGroupId || 0);
    if (!Number.isInteger(sourceGroupId) || sourceGroupId <= 0) {
      showError(t('分组 id 无效'));
      return;
    }
    if (!Number.isInteger(targetGroupId) || targetGroupId <= 0) {
      showError(t('请选择目标分组'));
      return;
    }
    setTokenRemapSaving(true);
    try {
      const res = await API.post(`/api/group/${sourceGroupId}/token_remap`, {
        target_group_id: targetGroupId,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('令牌分组改绑失败'));
        return;
      }
      showSuccess(
        t('已批量更新 {{count}} 个令牌', {
          count: Number(data?.updated_tokens ?? 0),
        }),
      );
      closeTokenRemapModal();
    } catch (e) {
      showError(
        t('令牌分组改绑失败: ') +
          (e?.response?.data?.message || e?.message || e),
      );
    } finally {
      setTokenRemapSaving(false);
    }
  }, [closeTokenRemapModal, selectedGroup, t, tokenRemapTargetGroupId]);

  const openGroupUserPriceOverrides = useCallback(() => {
    const groupId = Number(selectedGroup?.id || 0);
    if (!Number.isInteger(groupId) || groupId <= 0) {
      showWarning(t('请先选择一个分组'));
      return;
    }
    setGroupUserPriceOverridesVisible(true);
  }, [selectedGroup, t]);

  const closeGroupUserPriceOverrides = useCallback(() => {
    setGroupUserPriceOverridesVisible(false);
  }, []);

  const handleGroupUserPriceOverridesSaved = useCallback(
    (entries) => {
      const groupId = Number(selectedGroup?.id || 0);
      if (!Number.isInteger(groupId) || groupId <= 0) return;
      setGroupUserPriceOverridesByGroupId((prev) => ({
        ...(prev || {}),
        [groupId]: Array.isArray(entries) ? entries : [],
      }));
    },
    [selectedGroup],
  );

  const loadModelPrefillGroups = useCallback(async () => {
    setModelPrefillGroupsLoading(true);
    try {
      const res = await API.get('/api/prefill_group?type=model', {
        skipErrorHandler: true,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('加载模型预填组失败'));
        return;
      }
      const list = Array.isArray(data) ? data : [];
      setModelPrefillGroups(list);
      setSelectedModelPrefillGroupId((prev) => {
        const cur = Number(prev || 0);
        if (Number.isInteger(cur) && cur > 0) {
          if (list.some((g) => Number(g?.id) === cur)) return cur;
        }
        const next = Number(list?.[0]?.id || 0);
        return Number.isInteger(next) && next > 0 ? next : 0;
      });
    } catch (e) {
      showError(e?.message || t('加载模型预填组失败'));
    } finally {
      setModelPrefillGroupsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (!modelsEditorVisible) return;
    void loadModelPrefillGroups();
  }, [loadModelPrefillGroups, modelsEditorVisible]);

  useEffect(() => {
    void loadNoBillingProductOptions();
  }, [loadNoBillingProductOptions]);

  const applyModelPrefillGroupToEditor = useCallback(
    (mode) => {
      const id = Number(selectedModelPrefillGroupId || 0);
      if (!Number.isInteger(id) || id <= 0) {
        showError(t('请选择一个模型预填组'));
        return;
      }
      const group = (
        Array.isArray(modelPrefillGroups) ? modelPrefillGroups : []
      ).find((g) => Number(g?.id) === id);
      if (!group) {
        showError(t('模型预填组不存在'));
        return;
      }
      const items = group?.items;
      if (!Array.isArray(items) || items.length === 0) {
        showError(
          t('预填组 {{group}} items 为空或无效', { group: group?.name || id }),
        );
        return;
      }
      const normalizedItems = normalizeAllowedModels(items);
      if (!normalizedItems.length) {
        showError(
          t('预填组 {{group}} items 为空或无效', { group: group?.name || id }),
        );
        return;
      }

      if (mode === 'replace') {
        setModelsEditorValue(normalizedItems);
        return;
      }
      if (mode === 'append') {
        setModelsEditorValue((prev) =>
          normalizeAllowedModels([
            ...(Array.isArray(prev) ? prev : []),
            ...normalizedItems,
          ]),
        );
        return;
      }
      showError(t('无效的填充模式'));
    },
    [
      modelPrefillGroups,
      normalizeAllowedModels,
      selectedModelPrefillGroupId,
      t,
    ],
  );

  async function onSubmit() {
    try {
      await refForm.current
        .validate()
        .then(() => {
          const updateArray = compareObjects(inputs, inputsRow).filter(
            (item) => !readOnlyOptionKeys.has(item.key),
          );
          if (!updateArray.length)
            return showWarning(t('你似乎并没有修改什么'));

          const requestQueue = updateArray.map((item) => {
            const value =
              typeof inputs[item.key] === 'boolean'
                ? String(inputs[item.key])
                : inputs[item.key];
            return API.put('/api/option/', { key: item.key, value });
          });

          setLoading(true);
          Promise.all(requestQueue)
            .then((res) => {
              if (res.includes(undefined)) {
                return showError(
                  requestQueue.length > 1
                    ? t('部分保存失败，请重试')
                    : t('保存失败'),
                );
              }

              for (let i = 0; i < res.length; i++) {
                if (!res[i].data.success) {
                  return showError(res[i].data.message);
                }
              }

              showSuccess(t('保存成功'));
              props.refresh();
            })
            .catch((error) => {
              console.error('Unexpected error:', error);
              showError(t('保存失败，请重试'));
            })
            .finally(() => {
              setLoading(false);
            });
        })
        .catch(() => {
          showError(t('请检查输入'));
        });
    } catch (error) {
      showError(t('请检查输入'));
      console.error(error);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);
  }, [props.options]);

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  const summarizeList = useCallback(
    (rawList, emptyText, formatter) => {
      const list = Array.isArray(rawList) ? rawList : [];
      if (!list.length) {
        return {
          countText: emptyText,
          previewText: '',
        };
      }
      const mapped = list
        .map((item) => {
          if (typeof formatter === 'function') {
            return String(formatter(item) || '').trim();
          }
          return String(item || '').trim();
        })
        .filter(Boolean);
      if (!mapped.length) {
        return {
          countText: emptyText,
          previewText: '',
        };
      }
      return {
        countText: t('{{count}} 项', { count: mapped.length }),
        previewText:
          mapped.slice(0, 3).join(' / ') +
          (mapped.length > 3
            ? t(' +{{count}}', { count: mapped.length - 3 })
            : ''),
      };
    },
    [t],
  );

  const getGroupBadges = useCallback(
    (group) => {
      if (!group) return [];
      const id = Number(group?.id || 0);
      const badges = [
        {
          key: 'ratio',
          color: 'blue',
          text: `x${formatNumber(Number(group?.ratio ?? 1))}`,
        },
      ];
      if (dirtyGroupSet.has(id)) {
        badges.push({
          key: 'group_dirty',
          color: 'orange',
          text: t('未保存'),
        });
      }
      if (dirtyRateLimitGroupSet.has(id)) {
        badges.push({
          key: 'rate_dirty',
          color: 'yellow',
          text: t('限速待保存'),
        });
      }
      if (!Boolean(group?.enabled)) {
        badges.push({ key: 'disabled', color: 'grey', text: t('已停用') });
      }
      if (!Boolean(group?.user_selectable)) {
        badges.push({
          key: 'hidden',
          color: 'cyan',
          text: t('用户不可选'),
        });
      }
      if (Boolean(group?.no_billing)) {
        badges.push({
          key: 'no_billing',
          color: 'green',
          text: t('不计费'),
        });
      }
      return badges;
    },
    [dirtyGroupSet, dirtyRateLimitGroupSet, t],
  );

  const visibleGroupCount = filteredGroups.length;
  const hiddenGroupCount = Math.max(
    0,
    (Array.isArray(groups) ? groups.length : 0) - visibleGroupCount,
  );

  const selectedGroupNoBillingSummary = useMemo(() => {
    if (!selectedGroup) return { countText: t('未配置'), previewText: '' };
    const keys = normalizeNoBillingProductKeys(
      selectedGroup?.no_billing_product_keys,
    );
    if (!keys.length) {
      return {
        countText: selectedGroup?.no_billing ? t('未配置') : t('未启用'),
        previewText: '',
      };
    }
    return summarizeList(
      keys,
      t('未配置'),
      (key) =>
        noBillingProductLabelByKey.get(key) ||
        formatNoBillingProductKeyLabel(key),
    );
  }, [
    formatNoBillingProductKeyLabel,
    noBillingProductLabelByKey,
    normalizeNoBillingProductKeys,
    selectedGroup,
    summarizeList,
    t,
  ]);

  const selectedGroupModelSummary = useMemo(
    () => summarizeList(selectedGroup?.allowed_models, t('不限')),
    [selectedGroup?.allowed_models, summarizeList, t],
  );

  const selectedGroupUASummary = useMemo(
    () => summarizeList(selectedGroup?.allowed_user_agents, t('不限')),
    [selectedGroup?.allowed_user_agents, summarizeList, t],
  );

  const selectedGroupPrefillSummary = useMemo(
    () =>
      summarizeList(
        selectedGroup?.allowed_model_prefill_group_ids,
        t('未绑定'),
        (groupId) => {
          const id = Number(groupId || 0);
          const label = groupNameById.get(id);
          return label ? `${label} (#${id})` : `#${id}`;
        },
      ),
    [
      groupNameById,
      selectedGroup?.allowed_model_prefill_group_ids,
      summarizeList,
      t,
    ],
  );

  const selectedGroupChannelSummary = useMemo(() => {
    if (!selectedGroup)
      return { countText: t('点击编辑查看'), previewText: '' };
    const groupId = Number(selectedGroup?.id || 0);
    if (!Number.isInteger(groupId) || groupId <= 0) {
      return { countText: t('点击编辑查看'), previewText: '' };
    }
    const items = Array.isArray(groupChannelBindingsByGroupId?.[groupId])
      ? groupChannelBindingsByGroupId[groupId]
      : null;
    if (!items) {
      return { countText: t('加载中'), previewText: '' };
    }
    return summarizeList(
      items.filter((item) => item?.selected),
      t('未绑定'),
      formatGroupChannelLabel,
    );
  }, [
    formatGroupChannelLabel,
    groupChannelBindingsByGroupId,
    selectedGroup,
    summarizeList,
    t,
  ]);

  const selectedGroupUserPriceOverrideSummary = useMemo(() => {
    const groupId = Number(selectedGroup?.id || 0);
    if (!Number.isInteger(groupId) || groupId <= 0) {
      return { countText: t('点击编辑查看'), previewText: '' };
    }
    const entries = Array.isArray(groupUserPriceOverridesByGroupId?.[groupId])
      ? groupUserPriceOverridesByGroupId[groupId]
      : null;
    if (!entries) {
      return { countText: t('点击编辑查看'), previewText: '' };
    }
    return summarizeList(entries, t('未设置'), (entry) => {
      const label =
        String(
          entry?.display_name || entry?.username || entry?.email || '',
        ).trim() || `#${entry?.user_id ?? 0}`;
      return `${label} (${formatNumber(entry?.factor)}x)`;
    });
  }, [groupUserPriceOverridesByGroupId, selectedGroup, summarizeList, t]);

  const selectedGroupChannelCount = useMemo(() => {
    const groupId = Number(selectedGroup?.id || 0);
    if (!Number.isInteger(groupId) || groupId <= 0) return 0;
    const items = Array.isArray(groupChannelBindingsByGroupId?.[groupId])
      ? groupChannelBindingsByGroupId[groupId]
      : [];
    return items.filter((item) => item?.selected).length;
  }, [groupChannelBindingsByGroupId, selectedGroup]);

  const selectedGroupRateSummary = useMemo(() => {
    const config = rateLimitGroup?.[selectedGroupNumericId];
    const hasTotal = Number.isFinite(config?.total);
    const hasSuccess = Number.isFinite(config?.success);
    if (!hasTotal && !hasSuccess) {
      return {
        label: t('继承全局'),
        detail: t('未设置当前分组的独立限速'),
      };
    }
    return {
      label: t('独立限速'),
      detail: t('{{total}} / {{success}}', {
        total: config?.total ?? '-',
        success: config?.success ?? '-',
      }),
    };
  }, [rateLimitGroup, selectedGroupNumericId, t]);

  const selectedGroupWorkspaceCards = useMemo(() => {
    if (!selectedGroup) return [];
    return [
      {
        key: 'ratio',
        label: t('当前倍率'),
        value: `x${formatNumber(selectedGroup?.ratio)}`,
        tone: 'var(--semi-color-primary)',
      },
      {
        key: 'channels',
        label: t('绑定渠道'),
        value: String(selectedGroupChannelCount),
        tone: 'var(--semi-color-success)',
      },
      {
        key: 'models',
        label: t('模型策略'),
        value: Array.isArray(selectedGroup?.allowed_models)
          ? selectedGroup.allowed_models.length
            ? t('{{count}} 项', {
                count: selectedGroup.allowed_models.length,
              })
            : t('不限')
          : t('不限'),
        tone: 'var(--semi-color-warning)',
      },
      {
        key: 'rate',
        label: t('限速模式'),
        value: selectedGroupRateSummary.label,
        tone: 'var(--semi-color-info)',
      },
    ];
  }, [
    selectedGroup,
    selectedGroupChannelCount,
    selectedGroupRateSummary.label,
    t,
  ]);

  const hasWorkspaceChanges =
    dirtyGroupIds.length > 0 || dirtyRateLimitGroupIds.length > 0;
  const selectedGroupNoBillingInvalid =
    Boolean(selectedGroup?.no_billing) &&
    !normalizeNoBillingProductKeys(selectedGroup?.no_billing_product_keys)
      .length;

  function formatNumber(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '1';
    return num.toFixed(6).replace(/\.?0+$/, '');
  }

  const panelCardStyle = {
    borderRadius: 24,
    border: '1px solid var(--semi-color-border)',
    background: 'var(--semi-color-bg-1)',
    boxShadow: '0 24px 50px rgba(15, 23, 42, 0.06)',
    color: 'var(--semi-color-text-0)',
  };

  const subPanelStyle = {
    borderRadius: 18,
    border: '1px solid var(--semi-color-border)',
    background: 'var(--semi-color-fill-0)',
    color: 'var(--semi-color-text-0)',
  };

  const metricCardThemes = {
    total: {
      accent: 'var(--semi-color-success)',
      background:
        'color-mix(in srgb, var(--semi-color-success-light-default) 62%, var(--semi-color-bg-1))',
      borderColor:
        'color-mix(in srgb, var(--semi-color-success) 22%, var(--semi-color-border))',
    },
    visible: {
      accent: 'var(--semi-color-primary)',
      background:
        'color-mix(in srgb, var(--semi-color-primary-light-default) 62%, var(--semi-color-bg-1))',
      borderColor:
        'color-mix(in srgb, var(--semi-color-primary) 22%, var(--semi-color-border))',
    },
    dirty: {
      accent: 'var(--semi-color-warning)',
      background:
        'color-mix(in srgb, var(--semi-color-warning-light-default) 62%, var(--semi-color-bg-1))',
      borderColor:
        'color-mix(in srgb, var(--semi-color-warning) 22%, var(--semi-color-border))',
    },
    dirty_rate_limit: {
      accent: 'var(--semi-color-info)',
      background:
        'color-mix(in srgb, var(--semi-color-info-light-default) 62%, var(--semi-color-bg-1))',
      borderColor:
        'color-mix(in srgb, var(--semi-color-info) 22%, var(--semi-color-border))',
    },
  };

  const workspaceHeroStyle = {
    ...panelCardStyle,
    borderRadius: 28,
    background:
      'linear-gradient(135deg, color-mix(in srgb, var(--semi-color-primary-light-default) 68%, var(--semi-color-bg-1)) 0%, color-mix(in srgb, var(--semi-color-info-light-default) 40%, var(--semi-color-bg-1)) 100%)',
    border:
      '1px solid color-mix(in srgb, var(--semi-color-primary) 18%, var(--semi-color-border))',
  };

  const stickySidebarStyle = {
    position: 'sticky',
    top: 20,
    alignSelf: 'start',
  };

  return (
    <Spin spinning={loading || groupsLoading}>
      <div style={{ display: 'grid', gap: 16 }}>
        <Card bodyStyle={{ padding: 20 }} style={workspaceHeroStyle}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'flex-start',
              gap: 16,
              flexWrap: 'wrap',
            }}
          >
            <div style={{ minWidth: 0, maxWidth: 760 }}>
              <Text
                type='tertiary'
                style={{
                  display: 'block',
                  fontSize: 12,
                  letterSpacing: '0.08em',
                  textTransform: 'uppercase',
                }}
              >
                {t('分组倍率工作台')}
              </Text>
              <Title heading={4} style={{ margin: '6px 0 0' }}>
                {t('先选分组，再在右侧一次性完成倍率、计费、渠道和限速')}
              </Title>
              <Text
                type='tertiary'
                style={{ display: 'block', marginTop: 8, maxWidth: 680 }}
              >
                {t(
                  '这个页面现在按“当前分组”组织，而不是把全局项、兼容项和分组明细混在一起。低频的全局默认值与 legacy 兼容配置已收纳到页面底部。',
                )}
              </Text>
            </div>
            <Space wrap>
              <Button
                disabled={groupsSaving || deletingGroupId !== 0}
                onClick={loadGroups}
              >
                {t('刷新')}
              </Button>
              <Button
                type='primary'
                loading={groupsSaving}
                disabled={!dirtyGroupIds.length || deletingGroupId !== 0}
                onClick={saveAllGroups}
              >
                {dirtyGroupIds.length
                  ? t('保存全部分组（{{count}}）', {
                      count: dirtyGroupIds.length,
                    })
                  : t('保存全部分组')}
              </Button>
              <Button
                loading={rateLimitGroupSaving}
                disabled={
                  !dirtyRateLimitGroupIds.length || deletingGroupId !== 0
                }
                onClick={saveRateLimitGroup}
              >
                {dirtyRateLimitGroupIds.length
                  ? t('保存全部限速（{{count}}）', {
                      count: dirtyRateLimitGroupIds.length,
                    })
                  : t('保存全部限速')}
              </Button>
              <Button
                disabled={groupsSaving || deletingGroupId !== 0}
                onClick={() => setCreateVisible(true)}
              >
                {t('新增分组')}
              </Button>
            </Space>
          </div>

          <div
            style={{
              marginTop: 16,
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))',
              gap: 12,
            }}
          >
            {[
              {
                key: 'total',
                label: t('全部分组'),
                value: Array.isArray(groups) ? groups.length : 0,
              },
              {
                key: 'visible',
                label: t('当前显示'),
                value: visibleGroupCount,
              },
              {
                key: 'dirty',
                label: t('待保存分组'),
                value: dirtyGroupIds.length,
              },
              {
                key: 'dirty_rate_limit',
                label: t('待保存限速'),
                value: dirtyRateLimitGroupIds.length,
              },
            ].map((item) => (
              <div
                key={item.key}
                style={{
                  borderRadius: 18,
                  padding: '14px 16px',
                  background: metricCardThemes[item.key]?.background,
                  border: `1px solid ${
                    metricCardThemes[item.key]?.borderColor
                  }`,
                  color: 'var(--semi-color-text-0)',
                }}
              >
                <Text
                  type='tertiary'
                  style={{ display: 'block', fontSize: 12, letterSpacing: 0.4 }}
                >
                  {item.label}
                </Text>
                <Text
                  strong
                  style={{
                    display: 'block',
                    marginTop: 6,
                    fontSize: 24,
                    lineHeight: 1.1,
                    color: metricCardThemes[item.key]?.accent,
                  }}
                >
                  {item.value}
                </Text>
              </div>
            ))}
          </div>

          <Banner
            type={hasWorkspaceChanges ? 'warning' : 'info'}
            description={
              hasWorkspaceChanges
                ? t(
                    '当前还有 {{groupCount}} 个分组改动和 {{rateCount}} 个限速改动未保存。你可以直接在右侧保存当前分组，也可以在这里批量保存全部。',
                    {
                      groupCount: dirtyGroupIds.length,
                      rateCount: dirtyRateLimitGroupIds.length,
                    },
                  )
                : t(
                    '推荐操作流：左侧挑选分组，右侧工作台完成当前分组设置；只有全局默认值或兼容项才需要打开底部高级区。',
                  )
            }
            style={{ marginTop: 16 }}
          />
        </Card>

        <Row gutter={[16, 16]} align='top'>
          <Col xs={24} lg={8} xl={6}>
            <div style={stickySidebarStyle}>
              <Card
                bodyStyle={{ padding: 12 }}
                style={{ ...panelCardStyle, borderRadius: 22 }}
              >
                <div
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    gap: 12,
                  }}
                >
                  <Text strong>{t('分组目录')}</Text>
                  <Tag color='light-blue'>{visibleGroupCount}</Tag>
                </div>

                <div style={{ marginTop: 12, display: 'grid', gap: 10 }}>
                  <Input
                    showClear
                    placeholder={t('搜索分组名 / code / 说明 / id')}
                    value={groupKeyword}
                    onChange={(value) => setGroupKeyword(value)}
                    disabled={hideDisabledGroupsSaving}
                  />
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      gap: 12,
                      padding: '10px 12px',
                      borderRadius: 14,
                      background: 'var(--semi-color-fill-0)',
                    }}
                  >
                    <div>
                      <Text strong>{t('隐藏停用分组')}</Text>
                      <Text
                        type='tertiary'
                        style={{ display: 'block', marginTop: 2, fontSize: 12 }}
                      >
                        {hiddenGroupCount > 0
                          ? t('已折叠 {{count}} 个分组', {
                              count: hiddenGroupCount,
                            })
                          : t('当前没有折叠项')}
                      </Text>
                    </div>
                    <Switch
                      checked={hideDisabledGroups}
                      disabled={hideDisabledGroupsSaving}
                      onChange={updateHideDisabledGroups}
                    />
                  </div>
                </div>

                <Divider margin='16px 0 12px 0' />

                {filteredGroups.length ? (
                  <div
                    style={{
                      display: 'grid',
                      gap: 6,
                      maxHeight: 880,
                      overflowY: 'auto',
                      paddingRight: 4,
                    }}
                  >
                    {filteredGroups.map((group) => {
                      const id = Number(group?.id || 0);
                      const selected = id === Number(selectedGroupId || 0);
                      return (
                        <div
                          key={
                            id > 0
                              ? String(id)
                              : String(group?.code || group?.name || '')
                          }
                          role='button'
                          tabIndex={0}
                          onClick={() => setSelectedGroupId(id)}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter' || event.key === ' ') {
                              event.preventDefault();
                              setSelectedGroupId(id);
                            }
                          }}
                          style={{
                            borderRadius: 14,
                            padding: '10px 11px',
                            cursor: 'pointer',
                            border: selected
                              ? '1px solid var(--semi-color-primary)'
                              : '1px solid var(--semi-color-border)',
                            background: selected
                              ? 'color-mix(in srgb, var(--semi-color-primary-light-default) 72%, var(--semi-color-bg-1))'
                              : 'var(--semi-color-fill-0)',
                            boxShadow: selected
                              ? '0 16px 30px rgba(37, 99, 235, 0.12)'
                              : '0 8px 20px rgba(15, 23, 42, 0.04)',
                          }}
                        >
                          <div
                            style={{
                              display: 'grid',
                              gridTemplateColumns: 'minmax(0, 1fr) auto auto',
                              alignItems: 'start',
                              gap: 8,
                            }}
                          >
                            <div
                              style={{
                                minWidth: 0,
                                display: 'flex',
                                alignItems: 'baseline',
                                flexWrap: 'wrap',
                                gap: '2px 6px',
                              }}
                            >
                              <Text
                                type='tertiary'
                                style={{
                                  fontSize: 12,
                                  fontFamily: 'monospace',
                                  lineHeight: 1.35,
                                }}
                              >
                                #{id}
                              </Text>
                              <Text
                                strong
                                style={{
                                  minWidth: 0,
                                  fontSize: 13,
                                  lineHeight: 1.35,
                                  whiteSpace: 'normal',
                                  overflowWrap: 'anywhere',
                                }}
                              >
                                {group?.name || t('未命名分组')}
                              </Text>
                            </div>
                            <Tag
                              color={selected ? 'green' : 'blue'}
                              size='small'
                            >
                              x{formatNumber(group?.ratio)}
                            </Tag>
                            <div
                              style={{
                                flexShrink: 0,
                              }}
                              onClick={(event) => event.stopPropagation()}
                              onKeyDown={(event) => event.stopPropagation()}
                            >
                              <Popconfirm
                                title={t('确定删除该分组吗？')}
                                content={t(
                                  '这是软删除：系统会归档该分组，并清理渠道、令牌、商品等依赖关系。',
                                )}
                                okType='danger'
                                position='leftTop'
                                onConfirm={() => deleteGroup(group)}
                              >
                                <Button
                                  type='danger'
                                  theme='borderless'
                                  size='small'
                                  icon={<IconDelete />}
                                  loading={deletingGroupId === id}
                                  disabled={
                                    !id ||
                                    groupsSaving ||
                                    deletingGroupId === id
                                  }
                                />
                              </Popconfirm>
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <Empty
                    imageStyle={{ height: 96 }}
                    title={t('没有匹配的分组')}
                    description={t('调整搜索条件后再试')}
                  />
                )}
              </Card>
            </div>
          </Col>

          <Col xs={24} lg={16} xl={18}>
            {selectedGroup ? (
              <div style={{ display: 'grid', gap: 16 }}>
                <Card bodyStyle={{ padding: 20 }} style={workspaceHeroStyle}>
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'flex-start',
                      gap: 16,
                      flexWrap: 'wrap',
                    }}
                  >
                    <div style={{ minWidth: 0 }}>
                      <Text
                        type='tertiary'
                        style={{
                          display: 'block',
                          fontSize: 12,
                          letterSpacing: '0.08em',
                          textTransform: 'uppercase',
                        }}
                      >
                        {t('当前分组工作台')}
                      </Text>
                      <Title heading={4} style={{ margin: '6px 0 0' }}>
                        {selectedGroup?.name || t('未命名分组')}
                      </Title>
                      <Text
                        type='tertiary'
                        style={{
                          display: 'block',
                          marginTop: 6,
                          fontFamily: 'monospace',
                        }}
                      >
                        #{selectedGroup.id} ·{' '}
                        {String(selectedGroup?.code || '').trim() || '-'}
                      </Text>
                      <Text
                        type='tertiary'
                        style={{
                          display: 'block',
                          marginTop: 8,
                          maxWidth: 680,
                        }}
                      >
                        {String(selectedGroup?.description || '').trim() ||
                          t(
                            '这个分组还没有说明。建议写清楚它面向谁、消费规则是什么。',
                          )}
                      </Text>
                    </div>
                    <Space wrap>
                      <Button
                        type='primary'
                        loading={groupsSaving || rateLimitGroupSaving}
                        disabled={
                          (!selectedGroupDirty && !selectedGroupRateDirty) ||
                          deletingGroupId === selectedGroup.id
                        }
                        onClick={saveSelectedWorkspace}
                      >
                        {t('保存当前分组')}
                      </Button>
                      <Button
                        disabled={
                          (!selectedGroupDirty && !selectedGroupRateDirty) ||
                          groupsSaving ||
                          rateLimitGroupSaving ||
                          deletingGroupId === selectedGroup.id
                        }
                        onClick={resetSelectedWorkspace}
                      >
                        {t('撤销当前改动')}
                      </Button>
                    </Space>
                  </div>

                  <div style={{ marginTop: 14 }}>
                    <Space wrap>
                      {getGroupBadges(selectedGroup).map((badge) => (
                        <Tag key={badge.key} color={badge.color}>
                          {badge.text}
                        </Tag>
                      ))}
                    </Space>
                  </div>

                  {selectedGroupNoBillingInvalid ? (
                    <Banner
                      type='warning'
                      description={t(
                        '当前分组已打开“不计费”，但还没有绑定限定商品。保存前需要先补齐，否则后端会拒绝。',
                      )}
                      style={{ marginTop: 14 }}
                    />
                  ) : null}

                  <div
                    style={{
                      marginTop: 16,
                      display: 'grid',
                      gridTemplateColumns:
                        'repeat(auto-fit, minmax(160px, 1fr))',
                      gap: 12,
                    }}
                  >
                    {selectedGroupWorkspaceCards.map((item) => (
                      <div
                        key={item.key}
                        style={{
                          borderRadius: 18,
                          padding: '14px 16px',
                          background: 'var(--semi-color-bg-1)',
                          border: `1px solid color-mix(in srgb, ${item.tone} 22%, var(--semi-color-border))`,
                          boxShadow: '0 12px 24px rgba(15, 23, 42, 0.08)',
                        }}
                      >
                        <Text
                          type='tertiary'
                          style={{ display: 'block', fontSize: 12 }}
                        >
                          {item.label}
                        </Text>
                        <Text
                          strong
                          style={{
                            display: 'block',
                            marginTop: 6,
                            fontSize: 22,
                            lineHeight: 1.15,
                            color: item.tone,
                          }}
                        >
                          {item.value}
                        </Text>
                        {item.key === 'rate' ? (
                          <Text
                            type='tertiary'
                            style={{ display: 'block', marginTop: 6 }}
                          >
                            {selectedGroupRateSummary.detail}
                          </Text>
                        ) : null}
                      </div>
                    ))}
                  </div>
                </Card>

                <Row gutter={[16, 16]}>
                  <Col xs={24} xl={12}>
                    <Card bodyStyle={{ padding: 18 }} style={panelCardStyle}>
                      <Text strong>{t('基础与计费')}</Text>
                      <Text
                        type='tertiary'
                        style={{ display: 'block', marginTop: 4 }}
                      >
                        {t(
                          '先决定这个分组是什么、是否可见，以及它的基础倍率。',
                        )}
                      </Text>

                      <div style={{ marginTop: 16 }}>
                        <Row gutter={[16, 16]}>
                          <Col xs={24} md={16}>
                            <Text
                              type='tertiary'
                              style={{ display: 'block', marginBottom: 6 }}
                            >
                              {t('分组名')}
                            </Text>
                            <Input
                              value={selectedGroup?.name}
                              disabled={
                                groupsSaving ||
                                deletingGroupId === selectedGroup.id
                              }
                              onChange={(value) =>
                                updateGroupLocal(selectedGroup.id, {
                                  name: value,
                                })
                              }
                              placeholder={t('请输入分组名')}
                              style={{ width: '100%' }}
                            />
                          </Col>
                          <Col xs={24} md={8}>
                            <Text
                              type='tertiary'
                              style={{ display: 'block', marginBottom: 6 }}
                            >
                              {t('倍率')}
                            </Text>
                            <InputNumber
                              min={0}
                              value={selectedGroup?.ratio}
                              disabled={
                                groupsSaving ||
                                deletingGroupId === selectedGroup.id ||
                                rateLimitGroupSaving
                              }
                              onChange={(value) =>
                                updateGroupLocal(selectedGroup.id, {
                                  ratio: value,
                                })
                              }
                              style={{ width: '100%' }}
                            />
                          </Col>
                          <Col xs={24}>
                            <Text
                              type='tertiary'
                              style={{ display: 'block', marginBottom: 6 }}
                            >
                              {t('说明')}
                            </Text>
                            <Input
                              value={selectedGroup?.description}
                              disabled={
                                groupsSaving ||
                                deletingGroupId === selectedGroup.id
                              }
                              onChange={(value) =>
                                updateGroupLocal(selectedGroup.id, {
                                  description: value,
                                })
                              }
                              placeholder={t('给运营和用户都看得懂的说明')}
                              style={{ width: '100%' }}
                            />
                          </Col>
                        </Row>
                      </div>

                      <Divider margin='16px 0' />

                      <div
                        style={{
                          display: 'grid',
                          gridTemplateColumns:
                            'repeat(auto-fit, minmax(190px, 1fr))',
                          gap: 12,
                        }}
                      >
                        {[
                          {
                            key: 'enabled',
                            label: t('启用'),
                            helper: t('关闭后目录可隐藏，且不再可用'),
                            checked: Boolean(selectedGroup?.enabled),
                            onChange: (checked) =>
                              updateGroupLocal(selectedGroup.id, {
                                enabled: checked,
                              }),
                          },
                          {
                            key: 'user_selectable',
                            label: t('用户可选'),
                            helper: t('决定是否出现在用户侧可选分组列表'),
                            checked: Boolean(selectedGroup?.user_selectable),
                            onChange: (checked) =>
                              updateGroupLocal(selectedGroup.id, {
                                user_selectable: checked,
                              }),
                          },
                          {
                            key: 'no_billing',
                            label: t('不计费'),
                            helper: t('仍需绑定限定商品才能生效'),
                            checked: Boolean(selectedGroup?.no_billing),
                            onChange: (checked) =>
                              updateGroupLocal(selectedGroup.id, {
                                no_billing: checked,
                              }),
                          },
                        ].map((item) => (
                          <div
                            key={item.key}
                            style={{ ...subPanelStyle, padding: '14px 16px' }}
                          >
                            <div
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                gap: 12,
                              }}
                            >
                              <div style={{ minWidth: 0 }}>
                                <Text strong>{item.label}</Text>
                                <Text
                                  type='tertiary'
                                  style={{
                                    display: 'block',
                                    marginTop: 4,
                                    fontSize: 12,
                                  }}
                                >
                                  {item.helper}
                                </Text>
                              </div>
                              <Switch
                                checked={item.checked}
                                disabled={
                                  groupsSaving ||
                                  deletingGroupId === selectedGroup.id
                                }
                                onChange={item.onChange}
                              />
                            </div>
                          </div>
                        ))}
                      </div>

                      <div
                        style={{
                          marginTop: 14,
                          ...subPanelStyle,
                          padding: '16px',
                        }}
                      >
                        <div
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'flex-start',
                            gap: 12,
                            flexWrap: 'wrap',
                          }}
                        >
                          <div>
                            <Text strong>{t('不计费限定商品')}</Text>
                            <Text
                              type='tertiary'
                              style={{ display: 'block', marginTop: 4 }}
                            >
                              {selectedGroupNoBillingSummary.countText}
                            </Text>
                            {selectedGroupNoBillingSummary.previewText ? (
                              <Text
                                type='tertiary'
                                style={{ display: 'block', marginTop: 6 }}
                                ellipsis={{ showTooltip: true }}
                              >
                                {selectedGroupNoBillingSummary.previewText}
                              </Text>
                            ) : null}
                          </div>
                          <Button
                            size='small'
                            disabled={
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id ||
                              noBillingProductOptionsLoading
                            }
                            onClick={() =>
                              openNoBillingProductsEditor(selectedGroup)
                            }
                          >
                            {t('编辑')}
                          </Button>
                        </div>
                      </div>

                      <div
                        style={{
                          marginTop: 14,
                          ...subPanelStyle,
                          padding: '16px',
                        }}
                      >
                        <div
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'flex-start',
                            gap: 12,
                            flexWrap: 'wrap',
                          }}
                        >
                          <div>
                            <Text strong>{t('用户专属倍率')}</Text>
                            <Text
                              type='tertiary'
                              style={{ display: 'block', marginTop: 4 }}
                            >
                              {selectedGroupUserPriceOverrideSummary.countText}
                            </Text>
                            {selectedGroupUserPriceOverrideSummary.previewText ? (
                              <Text
                                type='tertiary'
                                style={{ display: 'block', marginTop: 6 }}
                                ellipsis={{ showTooltip: true }}
                              >
                                {
                                  selectedGroupUserPriceOverrideSummary.previewText
                                }
                              </Text>
                            ) : null}
                          </div>
                          <Button
                            size='small'
                            disabled={
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id
                            }
                            onClick={openGroupUserPriceOverrides}
                          >
                            {t('管理')}
                          </Button>
                        </div>
                      </div>
                    </Card>
                  </Col>

                  <Col xs={24} xl={12}>
                    <Card bodyStyle={{ padding: 18 }} style={panelCardStyle}>
                      <div
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          alignItems: 'flex-start',
                          gap: 12,
                          flexWrap: 'wrap',
                        }}
                      >
                        <div>
                          <Text strong>{t('访问范围与路由')}</Text>
                          <Text
                            type='tertiary'
                            style={{ display: 'block', marginTop: 4 }}
                          >
                            {t(
                              '这部分决定这个分组能走哪些渠道、看到哪些模型，以及什么客户端可以使用。',
                            )}
                          </Text>
                        </div>
                        <Button
                          size='small'
                          disabled={tokenRemapSaving || groupsSaving}
                          onClick={openTokenRemapModal}
                        >
                          {t('批量改绑令牌分组')}
                        </Button>
                      </div>

                      <div
                        style={{
                          marginTop: 16,
                          display: 'grid',
                          gridTemplateColumns:
                            'repeat(auto-fit, minmax(220px, 1fr))',
                          gap: 12,
                        }}
                      >
                        {[
                          {
                            key: 'channels',
                            title: t('绑定渠道'),
                            summary: selectedGroupChannelSummary,
                            action: () =>
                              openGroupChannelsEditor(selectedGroup),
                            disabled:
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id ||
                              groupChannelsEditorSaving,
                          },
                          {
                            key: 'models',
                            title: t('可选模型'),
                            summary: selectedGroupModelSummary,
                            action: () => openModelsEditor(selectedGroup),
                            disabled:
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id,
                          },
                          {
                            key: 'prefill_groups',
                            title: t('绑定预填组'),
                            summary: selectedGroupPrefillSummary,
                            action: () => openModelsEditor(selectedGroup),
                            disabled:
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id,
                          },
                          {
                            key: 'ua',
                            title: t('允许 UA'),
                            summary: selectedGroupUASummary,
                            action: () => openUAEditor(selectedGroup),
                            disabled:
                              groupsSaving ||
                              deletingGroupId === selectedGroup.id,
                          },
                        ].map((item) => (
                          <div
                            key={item.key}
                            style={{ ...subPanelStyle, padding: '16px' }}
                          >
                            <div
                              style={{
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'flex-start',
                                gap: 12,
                              }}
                            >
                              <div style={{ minWidth: 0 }}>
                                <Text strong>{item.title}</Text>
                                <Text
                                  strong
                                  style={{ display: 'block', marginTop: 10 }}
                                >
                                  {item.summary.countText}
                                </Text>
                                {item.summary.previewText ? (
                                  <Text
                                    type='tertiary'
                                    style={{ display: 'block', marginTop: 6 }}
                                    ellipsis={{ showTooltip: true }}
                                  >
                                    {item.summary.previewText}
                                  </Text>
                                ) : null}
                              </div>
                              <Button
                                size='small'
                                disabled={item.disabled}
                                onClick={item.action}
                              >
                                {t('编辑')}
                              </Button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </Card>
                  </Col>
                </Row>

                <Card bodyStyle={{ padding: 18 }} style={panelCardStyle}>
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'flex-start',
                      gap: 12,
                      flexWrap: 'wrap',
                    }}
                  >
                    <div>
                      <Text strong>{t('当前分组限速')}</Text>
                      <Text
                        type='tertiary'
                        style={{ display: 'block', marginTop: 4 }}
                      >
                        {t('只配置当前分组的覆盖值；留空表示继承全局默认值。')}
                      </Text>
                    </div>
                    {selectedGroupRateDirty ? (
                      <Tag color='yellow'>{t('当前分组限速尚未保存')}</Tag>
                    ) : (
                      <Tag color='light-blue'>
                        {selectedGroupRateSummary.label}
                      </Tag>
                    )}
                  </div>

                  <div style={{ marginTop: 16 }}>
                    <Row gutter={[16, 16]}>
                      <Col xs={24} md={12}>
                        <Text
                          type='tertiary'
                          style={{ display: 'block', marginBottom: 6 }}
                        >
                          {t('总请求/周期')}
                        </Text>
                        <InputNumber
                          min={0}
                          max={maxInt32}
                          precision={0}
                          placeholder={t('继承')}
                          value={
                            Number.isFinite(
                              rateLimitGroup?.[Number(selectedGroup?.id || 0)]
                                ?.total,
                            )
                              ? rateLimitGroup?.[Number(selectedGroup?.id || 0)]
                                  ?.total
                              : ''
                          }
                          disabled={
                            groupsSaving ||
                            deletingGroupId === selectedGroup.id ||
                            rateLimitGroupSaving
                          }
                          onChange={(value) =>
                            updateRateLimitGroupLocal(
                              selectedGroup.id,
                              'total',
                              value,
                            )
                          }
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col xs={24} md={12}>
                        <Text
                          type='tertiary'
                          style={{ display: 'block', marginBottom: 6 }}
                        >
                          {t('完成/周期')}
                        </Text>
                        <InputNumber
                          min={0}
                          max={maxInt32}
                          precision={0}
                          placeholder={t('继承')}
                          value={
                            Number.isFinite(
                              rateLimitGroup?.[Number(selectedGroup?.id || 0)]
                                ?.success,
                            )
                              ? rateLimitGroup?.[Number(selectedGroup?.id || 0)]
                                  ?.success
                              : ''
                          }
                          disabled={
                            groupsSaving ||
                            deletingGroupId === selectedGroup.id ||
                            rateLimitGroupSaving
                          }
                          onChange={(value) =>
                            updateRateLimitGroupLocal(
                              selectedGroup.id,
                              'success',
                              value,
                            )
                          }
                          style={{ width: '100%' }}
                        />
                      </Col>
                    </Row>
                  </div>
                </Card>
              </div>
            ) : (
              <Card bodyStyle={{ padding: 28 }} style={panelCardStyle}>
                <Empty
                  imageStyle={{ height: 120 }}
                  title={t('没有可编辑的分组')}
                  description={t(
                    '先从左侧目录选择分组，或新建一个分组开始配置',
                  )}
                />
              </Card>
            )}
          </Col>
        </Row>

        <Card bodyStyle={{ padding: 18 }} style={panelCardStyle}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'flex-start',
              gap: 12,
              flexWrap: 'wrap',
            }}
          >
            <div>
              <Text strong>{t('高级与兼容设置')}</Text>
              <Text type='tertiary' style={{ display: 'block', marginTop: 4 }}>
                {t(
                  '这一部分只在你需要调整全局限速默认值、auto 分组或查看 legacy 兼容项时再展开。',
                )}
              </Text>
            </div>
          </div>

          <Collapse style={{ marginTop: 16 }} defaultActiveKey={[]}>
            <Collapse.Panel
              itemKey='global-rate-limit'
              header={t('全局限速默认值')}
            >
              <Banner
                type='info'
                description={t(
                  '这里只控制全局默认值。当前分组自己的限速覆盖值，请直接在上面的“当前分组限速”区域编辑。',
                )}
                style={{ marginBottom: 12 }}
              />
              <RequestRateLimit
                options={props.options}
                refresh={props.refresh}
                hideGroupOverrides
              />
            </Collapse.Panel>

            <Collapse.Panel
              itemKey='legacy-group-options'
              header={t('兼容与 Legacy 分组选项')}
            >
              <Banner
                type='warning'
                description={t(
                  '这部分是低频兼容项。若你日常维护的是“分组倍率/渠道/限速”，通常不需要打开这里。',
                )}
                style={{ marginBottom: 12 }}
              />
              <Form
                values={inputs}
                getFormApi={(formAPI) => (refForm.current = formAPI)}
                style={{ marginTop: 8 }}
              >
                <Row gutter={16}>
                  <Col xs={24} xl={12}>
                    <Form.TextArea
                      label={t('分组特殊倍率（兼容只读）')}
                      placeholder={t('为一个 JSON 文本')}
                      extraText={t(
                        '旧规则只保留兼容展示；用户专属倍率请直接在分组工作台里维护',
                      )}
                      field={'GroupGroupRatio'}
                      autosize={{ minRows: 6, maxRows: 12 }}
                      trigger='blur'
                      stopValidateWithError
                      readonly
                      disabled
                      rules={[
                        {
                          validator: (rule, value) => verifyJSON(value),
                          message: t('不是合法的 JSON 字符串'),
                        },
                      ]}
                      onChange={(value) =>
                        setInputs({ ...inputs, GroupGroupRatio: value })
                      }
                    />
                  </Col>
                  <Col xs={24} xl={12}>
                    <Form.TextArea
                      label={t('自动分组 auto，从第一个开始选择')}
                      placeholder={t('为一个 JSON 文本')}
                      field={'AutoGroups'}
                      autosize={{ minRows: 6, maxRows: 12 }}
                      trigger='blur'
                      stopValidateWithError
                      rules={[
                        {
                          validator: (rule, value) => {
                            if (!value || value.trim() === '') {
                              return true;
                            }
                            try {
                              const parsed = JSON.parse(value);
                              if (!Array.isArray(parsed)) {
                                return false;
                              }
                              return parsed.every((item) => {
                                if (typeof item !== 'number') return false;
                                if (!Number.isFinite(item)) return false;
                                if (!Number.isInteger(item)) return false;
                                return item > 0;
                              });
                            } catch (error) {
                              return false;
                            }
                          },
                          message: t('必须是有效的 JSON 数字数组，例如：[4,5]'),
                        },
                      ]}
                      onChange={(value) =>
                        setInputs({ ...inputs, AutoGroups: value })
                      }
                    />
                  </Col>
                </Row>

                <div style={{ marginTop: 12 }}>
                  <Button onClick={onSubmit}>{t('保存兼容分组设置')}</Button>
                </div>
              </Form>
            </Collapse.Panel>
          </Collapse>
        </Card>
      </div>

      <Modal
        title={t('配置分组 {{group}} 可选模型', {
          group: modelsEditorGroupName,
        })}
        visible={modelsEditorVisible}
        centered
        okText={t('确定')}
        cancelText={t('取消')}
        okButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        cancelButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        onOk={saveModelsEditor}
        onCancel={closeModelsEditor}
      >
        <Spin spinning={modelPrefillGroupsLoading}>
          <Text type='tertiary'>
            {t(
              '留空表示不限制（保持现有行为）。最终可用模型 = 手动列表 ∪ 绑定预填组 items。',
            )}
          </Text>

          {modelPrefillGroups.length > 0 ? (
            <div style={{ marginTop: 12 }}>
              <Text type='tertiary'>
                {t('绑定模型预填组（可多选）：预填组变更会自动生效。')}
              </Text>
              <div style={{ marginTop: 8 }}>
                <Select
                  multiple
                  value={modelsEditorPrefillGroupIds}
                  placeholder={t('选择预填组（可多选）')}
                  style={{ width: '100%' }}
                  filter
                  showClear
                  onClear={() => setModelsEditorPrefillGroupIds([])}
                  onChange={(value) =>
                    setModelsEditorPrefillGroupIds(
                      normalizeAllowedModelPrefillGroupIds(value),
                    )
                  }
                >
                  {(Array.isArray(modelPrefillGroups)
                    ? modelPrefillGroups
                    : []
                  ).map((g) => (
                    <Select.Option key={g.id} value={g.id}>
                      {g.name} {`(#${g.id})`}
                    </Select.Option>
                  ))}
                </Select>
              </div>

              <Text type='tertiary' style={{ display: 'block', marginTop: 12 }}>
                {t('从预填组填充手动列表：')}
              </Text>
              <Space wrap style={{ marginTop: 8 }}>
                <Select
                  value={selectedModelPrefillGroupId || undefined}
                  placeholder={t('选择预填组')}
                  style={{ minWidth: 220 }}
                  filter
                  showClear
                  onClear={() => setSelectedModelPrefillGroupId(0)}
                  onChange={(value) => {
                    const id = Number(value || 0);
                    setSelectedModelPrefillGroupId(
                      Number.isInteger(id) && id > 0 ? id : 0,
                    );
                  }}
                >
                  {(Array.isArray(modelPrefillGroups)
                    ? modelPrefillGroups
                    : []
                  ).map((g) => (
                    <Select.Option key={g.id} value={g.id}>
                      {g.name} {`(#${g.id})`}
                    </Select.Option>
                  ))}
                </Select>

                <Button
                  disabled={!selectedModelPrefillGroupId}
                  onClick={() => applyModelPrefillGroupToEditor('replace')}
                >
                  {t('覆盖')}
                </Button>
                <Button
                  disabled={!selectedModelPrefillGroupId}
                  onClick={() => applyModelPrefillGroupToEditor('append')}
                >
                  {t('追加')}
                </Button>
              </Space>
            </div>
          ) : (
            <Text type='tertiary' style={{ display: 'block', marginTop: 12 }}>
              {t('暂无模型预填组')}
            </Text>
          )}

          <div style={{ marginTop: 12 }}>
            <TagInput
              value={modelsEditorValue}
              onChange={(value) =>
                setModelsEditorValue(normalizeAllowedModels(value))
              }
              placeholder={t(
                '输入模型名后回车，例如：gpt-4o、claude-3-5-sonnet',
              )}
              addOnBlur
              showClear
              style={{ width: '100%' }}
            />
          </div>
        </Spin>
      </Modal>

      <Modal
        title={t('配置分组 {{group}} 允许UA', { group: uaEditorGroupName })}
        visible={uaEditorVisible}
        centered
        okText={t('确定')}
        cancelText={t('取消')}
        okButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        cancelButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        onOk={saveUAEditor}
        onCancel={closeUAEditor}
      >
        <Text type='tertiary'>
          {t(
            '留空表示不限制。匹配规则：大小写不敏感；UA 包含任一关键词即可；UA 为空且配置了允许UA时拒绝。',
          )}
        </Text>
        <div style={{ marginTop: 12 }}>
          <TagInput
            value={uaEditorValue}
            onChange={(value) =>
              setUaEditorValue(normalizeAllowedUserAgents(value))
            }
            placeholder={t('输入 UA 关键词后回车，例如：opencode/、openai/js')}
            addOnBlur
            showClear
            style={{ width: '100%' }}
          />
        </div>
      </Modal>

      <Modal
        title={t('配置分组 {{group}} 的不计费限定商品', {
          group: noBillingProductsEditorGroupName,
        })}
        visible={noBillingProductsEditorVisible}
        centered
        okText={t('确定')}
        cancelText={t('取消')}
        okButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        cancelButtonProps={{ disabled: groupsSaving || deletingGroupId !== 0 }}
        onOk={saveNoBillingProductsEditor}
        onCancel={closeNoBillingProductsEditor}
      >
        <Spin spinning={noBillingProductOptionsLoading}>
          <Text type='tertiary'>
            {t(
              '只有当前仍持有以下任一商品的用户，才会在该分组触发“不计费”。开启不计费时，至少选择一个商品。',
            )}
          </Text>
          <div style={{ marginTop: 12 }}>
            <Select
              value={noBillingProductsEditorValue}
              optionList={noBillingProductOptionList}
              multiple
              search
              showClear
              style={{ width: '100%' }}
              placeholder={t('请选择限定商品')}
              onChange={(value) =>
                setNoBillingProductsEditorValue(
                  normalizeNoBillingProductKeys(value),
                )
              }
            />
          </div>
        </Spin>
      </Modal>

      <Modal
        title={t('配置分组 {{group}} 绑定渠道', {
          group: groupChannelsEditorGroupName,
        })}
        visible={groupChannelsEditorVisible}
        centered
        okText={t('确定')}
        cancelText={t('取消')}
        confirmLoading={groupChannelsEditorSaving}
        okButtonProps={{
          disabled: groupChannelsEditorLoading || groupsSaving,
        }}
        cancelButtonProps={{
          disabled: groupChannelsEditorSaving,
        }}
        onOk={saveGroupChannelsEditor}
        onCancel={closeGroupChannelsEditor}
      >
        <Spin spinning={groupChannelsEditorLoading}>
          <Text type='tertiary'>
            {t(
              '在这里直接维护“当前分组包含哪些渠道”。取消选择时只会移除当前分组绑定，不会影响该渠道绑定的其他分组。',
            )}
          </Text>
          <Text
            type='tertiary'
            style={{ display: 'block', marginTop: 8, marginBottom: 12 }}
          >
            {t(
              '如果某个渠道当前只剩这个主分组，系统会拒绝保存，避免把渠道改成无主分组。',
            )}
          </Text>
          <Select
            value={groupChannelsEditorValue}
            optionList={groupChannelsEditorOptionList}
            multiple
            search
            filter
            showClear
            style={{ width: '100%' }}
            placeholder={t('请选择要绑定到当前分组的渠道')}
            onChange={(value) =>
              setGroupChannelsEditorValue(normalizeChannelBindingIds(value))
            }
          />
          <Text type='tertiary' style={{ display: 'block', marginTop: 12 }}>
            {t('当前已选择 {{count}} 个渠道', {
              count: groupChannelsEditorValue.length,
            })}
          </Text>
        </Spin>
      </Modal>

      <Modal
        title={t('批量改绑令牌分组')}
        visible={tokenRemapVisible}
        centered
        okText={t('开始改绑')}
        cancelText={t('取消')}
        confirmLoading={tokenRemapSaving}
        onOk={submitTokenRemap}
        onCancel={closeTokenRemapModal}
      >
        <Text type='tertiary'>
          {t(
            '把所有“允许分组”里包含当前分组的用户令牌，统一改绑到另一个分组。若令牌本来同时拥有目标分组，系统会自动去重。',
          )}
        </Text>
        <div style={{ marginTop: 12 }}>
          <Text type='tertiary' style={{ display: 'block', marginBottom: 6 }}>
            {t('目标分组')}
          </Text>
          <Select
            value={tokenRemapTargetGroupId || undefined}
            optionList={tokenRemapGroupOptionList}
            search
            filter
            showClear
            style={{ width: '100%' }}
            placeholder={t('请选择新的分组')}
            onChange={(value) => {
              const nextValue = Number(value || 0);
              setTokenRemapTargetGroupId(
                Number.isInteger(nextValue) && nextValue > 0 ? nextValue : 0,
              );
            }}
          />
        </div>
      </Modal>

      <GroupUserPriceOverridesModal
        visible={groupUserPriceOverridesVisible}
        group={selectedGroup}
        onClose={closeGroupUserPriceOverrides}
        onSaved={handleGroupUserPriceOverridesSaved}
      />

      <Modal
        title={t('新增分组')}
        visible={createVisible}
        confirmLoading={creating}
        onOk={() => refCreateForm.current?.submitForm()}
        onCancel={() => setCreateVisible(false)}
      >
        <Form
          initValues={{
            name: '',
            description: '',
            ratio: 1,
            no_billing: false,
            no_billing_product_keys: [],
            user_selectable: true,
            enabled: true,
          }}
          getFormApi={(api) => (refCreateForm.current = api)}
          onSubmit={submitCreateGroup}
        >
          <Form.Input
            field='name'
            label={t('分组名')}
            placeholder={t('例如：codex')}
            rules={[{ required: true, message: t('请输入分组名') }]}
            showClear
          />
          <Form.Input
            field='description'
            label={t('说明')}
            placeholder={t('可选：用于用户可选分组文案')}
            showClear
          />
          <Form.InputNumber field='ratio' label={t('倍率')} min={0} />
          <Form.Switch field='no_billing' label={t('不计费')} />
          <Form.Select
            field='no_billing_product_keys'
            label={t('限定商品')}
            placeholder={t('开启不计费时请选择')}
            optionList={noBillingProductOptionList}
            loading={noBillingProductOptionsLoading}
            multiple
            search
            extraText={t(
              '只有当前仍持有这些商品的用户，才能在该分组触发不计费',
            )}
            style={{ width: '100%' }}
          />
          <Form.Switch field='user_selectable' label={t('用户可选')} />
          <Form.Switch field='enabled' label={t('启用')} />
        </Form>
      </Modal>
    </Spin>
  );
}
