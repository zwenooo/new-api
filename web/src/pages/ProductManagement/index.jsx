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
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Navigate } from 'react-router-dom';
import {
  API,
  copy,
  downloadTextAsFile,
  getUserIdFromLocalStorage,
  inferPresetMode,
  isRoot,
  renderCnyFen,
  renderQuota,
  showError,
  showSuccess,
  timestamp2string,
  toBoolean,
} from '../../helpers';
import { useUserPermissions } from '../../hooks/common/useUserPermissions';
import ConsolePage from '../../components/layout/ConsolePage';
import CardTable from '../../components/common/ui/CardTable';
import EditRedemptionPresetModal from '../../components/table/redemptions/modals/EditRedemptionPresetModal';
import {
  Button,
  Card,
  Dropdown,
  Form,
  InputNumber,
  Modal,
  Popconfirm,
  SideSheet,
  Spin,
  Switch,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconDelete,
  IconEdit,
  IconPlus,
  IconSave,
  IconSetting,
  IconTreeTriangleDown,
} from '@douyinfe/semi-icons';
import { GripVertical } from 'lucide-react';

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

const PRODUCT_MANAGEMENT_PRESET_API_BASE = '/api/product_management/presets';
const PRODUCT_MANAGEMENT_PAY_PRODUCT_API_BASE =
  '/api/product_management/pay_products';
const PRODUCT_MANAGEMENT_OPTION_API_BASE = '/api/product_management/option';
const PRODUCT_MANAGEMENT_REORDER_API_BASE = '/api/product_management/reorder';
const PRODUCT_MANAGEMENT_HIDE_ARCHIVED_OPTION_KEY =
  'ProductManagementHideArchivedEnabled';

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

const normalizeNonNegativeNumberValue = (raw) => {
  const num = Number(raw);
  if (!Number.isFinite(num) || num < 0) {
    return 0;
  }
  return num;
};

const cloneProductRows = (rows) =>
  (Array.isArray(rows) ? rows : []).map((item) => ({
    ...item,
    raw: item?.raw ? { ...item.raw } : item?.raw,
  }));

const productRowKeyOf = (row) => String(row?.row_key || '');

const areProductRowOrdersEqual = (left, right) => {
  const l = Array.isArray(left) ? left : [];
  const r = Array.isArray(right) ? right : [];
  if (l.length !== r.length) return false;
  for (let index = 0; index < l.length; index += 1) {
    if (productRowKeyOf(l[index]) !== productRowKeyOf(r[index])) {
      return false;
    }
  }
  return true;
};

const areProductRowSortOrdersEqual = (left, right) => {
  const sortByKey = new Map(
    (Array.isArray(left) ? left : []).map((row) => [
      productRowKeyOf(row),
      Number(row?.sort_order ?? 0) || 0,
    ]),
  );
  const next = Array.isArray(right) ? right : [];
  if (sortByKey.size !== next.length) return false;
  return next.every(
    (row) =>
      sortByKey.get(productRowKeyOf(row)) ===
      (Number(row?.sort_order ?? 0) || 0),
  );
};

const reorderProductRowsBefore = (rows, sourceKey, targetKey) => {
  const list = Array.isArray(rows) ? rows : [];
  const src = String(sourceKey || '');
  if (!src) return list.slice();
  const next = list.filter((row) => productRowKeyOf(row) !== src);
  const source = list.find((row) => productRowKeyOf(row) === src);
  if (!source) return list.slice();
  const dst = String(targetKey || '');
  const targetIndex = dst
    ? next.findIndex((row) => productRowKeyOf(row) === dst)
    : -1;
  if (targetIndex < 0) {
    next.push(source);
    return next;
  }
  next.splice(targetIndex, 0, source);
  return next;
};

const mergeReorderedVisibleProductRows = (rows, visibleRows) => {
  const visible = Array.isArray(visibleRows) ? visibleRows : [];
  if (visible.length === 0) {
    return Array.isArray(rows) ? rows.slice() : [];
  }
  const visibleByKey = new Map(
    visible.map((row) => [productRowKeyOf(row), row]),
  );
  let visibleIndex = 0;
  return (Array.isArray(rows) ? rows : []).map((row) => {
    if (!visibleByKey.has(productRowKeyOf(row))) {
      return row;
    }
    const nextVisible = visible[visibleIndex] || row;
    visibleIndex += 1;
    return nextVisible;
  });
};

const findProductRowNode = (rowRefs, rowKey) => {
  const key = String(rowKey || '');
  if (!key) return null;
  const fromRef = rowRefs?.current?.get(key);
  if (fromRef) return fromRef;
  if (typeof document === 'undefined') return null;
  return document.querySelector(`[data-product-row-key="${key}"]`);
};

const withSequentialSortOrders = (rows) => {
  const list = Array.isArray(rows) ? rows : [];
  const step = 10;
  const total = list.length;
  return list.map((row, index) => ({
    ...row,
    sort_order: (total - index) * step,
    raw: row?.raw
      ? { ...row.raw, sort_order: (total - index) * step }
      : row?.raw,
  }));
};

const normalizePaygProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const out = [];
  const seen = new Set();
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const archived = item?.archived === true;
    const enabled = archived ? false : item?.enabled !== false;

    const sortRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const normalizePayRequestProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const out = [];
  const seen = new Set();
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const archived = item?.archived === true;
    const enabled = archived ? false : item?.enabled !== false;

    const sortRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const normalizePayTokenProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const out = [];
  const seen = new Set();
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const archived = item?.archived === true;
    const enabled = archived ? false : item?.enabled !== false;

    const sortRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const isSubscriptionProductType = (type) =>
  type === 'subscription' || type === 'tokens' || type === 'request';

const buildCopiedName = (rawName, nameExists) => {
  const name = String(rawName || '').trim();
  if (!name) return '';
  const suffix = '-复制';
  const first = `${name}${suffix}`;
  if (!nameExists(first)) return first;
  for (let i = 2; i <= 50; i++) {
    const next = `${name}${suffix}${i}`;
    if (!nameExists(next)) return next;
  }
  return `${name}${suffix}${Date.now()}`;
};

const normalizeManagedGroupDailyLimits = (rawLimits) => {
  if (!Array.isArray(rawLimits)) return [];
  const seen = new Set();
  const out = [];
  rawLimits.forEach((item) => {
    const rawGroupId = Number(item?.group_id ?? 0);
    const groupId = Number.isFinite(rawGroupId) ? Math.floor(rawGroupId) : 0;
    if (groupId <= 0 || seen.has(groupId)) return;
    seen.add(groupId);

    out.push({
      group_id: groupId,
      daily_quota_limit: normalizeNonNegativeNumberValue(
        item?.daily_quota_limit ?? 0,
      ),
    });
  });
  return out.sort((a, b) => a.group_id - b.group_id);
};

const normalizeManagedSubscriptionProduct = (record) => {
  if (!record) return null;

  const rawId = Number(record?.id ?? 0);
  const id = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
  const rawMode = String(record?.mode ?? '').trim();
  const mode = rawMode || 'subscription';
  const name = String(record?.name ?? '').trim();
  if (id <= 0 || !name) return null;

  const sortOrderRaw = Number(record?.sort_order ?? 0);
  const sortOrder = Number.isFinite(sortOrderRaw)
    ? Math.max(0, Math.floor(sortOrderRaw))
    : 0;

  const purchaseLimitRaw = Number(record?.purchase_limit ?? 0);
  const purchaseLimit = Number.isFinite(purchaseLimitRaw)
    ? Math.max(0, Math.floor(purchaseLimitRaw))
    : 0;

  const priceFenRaw = Number(record?.price_fen ?? 0);
  const priceFen = Number.isFinite(priceFenRaw)
    ? Math.max(0, Math.floor(priceFenRaw))
    : 0;

  const quota = normalizeNonNegativeNumberValue(record?.quota ?? 0);

  const dailyQuotaLimit = normalizeNonNegativeNumberValue(
    record?.daily_quota_limit ?? 0,
  );

  const dailyRequestLimit = normalizeNonNegativeNumberValue(
    record?.daily_request_limit ?? 0,
  );

  const quotaValidDaysRaw = Number(record?.quota_valid_days ?? 0);
  const quotaValidDays = Number.isFinite(quotaValidDaysRaw)
    ? Math.max(0, Math.floor(quotaValidDaysRaw))
    : 0;

  const planValidDaysRaw = Number(record?.plan_valid_days ?? 0);
  const planValidDays = Number.isFinite(planValidDaysRaw)
    ? Math.max(0, Math.floor(planValidDaysRaw))
    : 0;

  const expiredTimeRaw = Number(record?.expired_time ?? 0);
  const expiredTime = Number.isFinite(expiredTimeRaw)
    ? Math.max(0, Math.floor(expiredTimeRaw))
    : 0;

  const stock = normalizeStockValue(record?.stock);

  return {
    ...record,
    id,
    name,
    description: String(record?.description ?? '').trim(),
    mode,
    archived: record?.archived === true,
    enabled: record?.archived === true ? false : record?.enabled !== false,
    multi_quantity_enabled: false,
    multi_quantity_defer_only: true,
    sort_order: sortOrder,
    price_fen: priceFen,
    purchase_limit: purchaseLimit,
    stock: stock === undefined ? null : stock,
    quota,
    daily_quota_limit: dailyQuotaLimit,
    daily_request_limit: dailyRequestLimit,
    quota_valid_days: quotaValidDays,
    plan_valid_days: planValidDays,
    expired_time: expiredTime,
    channel_ids: Array.isArray(record?.channel_ids) ? record.channel_ids : [],
    allowed_group_ids: normalizeGroupIds(record?.allowed_group_ids),
    group_daily_limits: normalizeManagedGroupDailyLimits(
      record?.group_daily_limits,
    ),
  };
};

const normalizeManagedSubscriptionProducts = (records) => {
  if (!Array.isArray(records)) return [];
  return records
    .map((record) => normalizeManagedSubscriptionProduct(record))
    .filter(
      (record) =>
        record && ['subscription', 'tokens', 'request'].includes(record.mode),
    );
};

const buildEmptyManagedSubscriptionProduct = (mode) => ({
  id: undefined,
  mode,
  name: '',
  description: '',
  enabled: false,
  archived: false,
  multi_quantity_enabled: false,
  multi_quantity_defer_only: true,
  sort_order: 0,
  price_fen: 0,
  purchase_limit: 0,
  stock: null,
  quota: 0,
  daily_quota_limit: 0,
  daily_request_limit: 0,
  quota_valid_days: 0,
  plan_valid_days: 0,
  channel_ids: [],
  allowed_group_ids: [],
  expired_time: 0,
  group_daily_limits: [],
});

const clearSubscriptionMultiQuantity = (product) => ({
  ...(product || {}),
  multi_quantity_enabled: false,
  multi_quantity_defer_only: true,
});

const buildRedemptionPresetUpsertPayload = (preset, patch) => {
  const normalizedPreset =
    normalizeManagedSubscriptionProduct(preset) || preset;
  const mode = inferPresetMode(normalizedPreset);
  const stock = normalizeStockValue(normalizedPreset?.stock);
  const payload = {
    id: normalizedPreset?.id,
    name: normalizedPreset?.name,
    description: String(normalizedPreset?.description || ''),
    mode,
    enabled:
      normalizedPreset?.archived === true
        ? false
        : normalizedPreset?.enabled !== false,
    archived: normalizedPreset?.archived === true,
    multi_quantity_enabled: false,
    multi_quantity_defer_only: true,
    sort_order: Number(normalizedPreset?.sort_order) || 0,
    price_fen: Number(normalizedPreset?.price_fen) || 0,
    purchase_limit: Number(normalizedPreset?.purchase_limit) || 0,
    stock: stock === undefined ? null : stock,
    quota: Number(normalizedPreset?.quota) || 0,
    daily_quota_limit: Number(normalizedPreset?.daily_quota_limit) || 0,
    daily_request_limit: Number(normalizedPreset?.daily_request_limit) || 0,
    quota_valid_days: Number(normalizedPreset?.quota_valid_days) || 0,
    plan_valid_days: Number(normalizedPreset?.plan_valid_days) || 0,
    channel_ids: Array.isArray(normalizedPreset?.channel_ids)
      ? normalizedPreset.channel_ids
      : [],
    allowed_group_ids: normalizeGroupIds(normalizedPreset?.allowed_group_ids),
    expired_time: Number(normalizedPreset?.expired_time) || 0,
  };
  if (Array.isArray(normalizedPreset?.group_daily_limits)) {
    payload.group_daily_limits = normalizeManagedGroupDailyLimits(
      normalizedPreset.group_daily_limits,
    );
  }
  return { ...payload, ...(patch || {}) };
};

const ProductActionButton = ({ children, className = '', ...props }) => (
  <Button size='small' className={`shrink-0 ${className}`} {...props}>
    {children}
  </Button>
);

const ProductManagement = () => {
  const { t } = useTranslation();
  const actualRootUser = isRoot();
  const {
    permissions,
    loading: permissionsLoading,
    isSidebarModuleAllowed,
  } = useUserPermissions();
  const isRootUser =
    actualRootUser ||
    (!permissionsLoading &&
      permissions?.sidebar_modules &&
      isSidebarModuleAllowed('admin', 'product_management'));

  const paygFormApiRef = useRef(null);
  const paygProductFormApiRef = useRef(null);
  const payRequestProductFormApiRef = useRef(null);
  const payTokenProductFormApiRef = useRef(null);
  const [paygLoading, setPaygLoading] = useState(false);
  const [paygSaving, setPaygSaving] = useState(false);
  const [paygFormKey, setPaygFormKey] = useState(0);
  const [paygFormInitValues, setPaygFormInitValues] = useState({
    credit_usd_per_cny: 20,
    credit_requests_per_cny: 0,
    credit_tokens_per_cny: 0,
  });
  const [paygProducts, setPaygProducts] = useState([]);
  const [paygProductsSaving, setPaygProductsSaving] = useState(false);
  const [paygProductSheetVisible, setPaygProductSheetVisible] = useState(false);
  const [editingPaygProduct, setEditingPaygProduct] = useState(null);
  const [paygSettingSheetVisible, setPaygSettingSheetVisible] = useState(false);

  const [payRequestProducts, setPayRequestProducts] = useState([]);
  const [payRequestProductsSaving, setPayRequestProductsSaving] =
    useState(false);
  const [payRequestProductSheetVisible, setPayRequestProductSheetVisible] =
    useState(false);
  const [editingPayRequestProduct, setEditingPayRequestProduct] =
    useState(null);

  const [payTokenProducts, setPayTokenProducts] = useState([]);
  const [payTokenProductsSaving, setPayTokenProductsSaving] = useState(false);
  const [payTokenProductSheetVisible, setPayTokenProductSheetVisible] =
    useState(false);
  const [editingPayTokenProduct, setEditingPayTokenProduct] = useState(null);

  const [subscriptionProductsLoading, setSubscriptionProductsLoading] =
    useState(false);
  const [subscriptionProducts, setSubscriptionProducts] = useState([]);
  const [productManagementOptionsLoading, setProductManagementOptionsLoading] =
    useState(false);
  const [hideArchivedProducts, setHideArchivedProducts] = useState(true);
  const [hideArchivedProductsSaving, setHideArchivedProductsSaving] =
    useState(false);
  const [productDragState, setProductDragState] = useState(null);
  const [productDropTargetKey, setProductDropTargetKey] = useState('');
  const [productsReordering, setProductsReordering] = useState(false);
  const productRowsRef = useRef([]);
  const productVisibleRowsRef = useRef([]);
  const productDragStateRef = useRef(null);
  const productDragPointerRef = useRef(null);
  const productDragInitialRowsRef = useRef([]);
  const productDragInitialVisibleRowsRef = useRef([]);
  const productDropTargetKeyRef = useRef('');
  const productDragHandleRef = useRef(null);
  const persistProductOrderRef = useRef(null);
  const productRowRefs = useRef(new Map());
  const productDragOverlayRef = useRef(null);
  const productDragOverlayFrameRef = useRef(0);
  const [subscriptionEditVisible, setSubscriptionEditVisible] = useState(false);
  const [editingSubscriptionProduct, setEditingSubscriptionProduct] = useState(
    clearSubscriptionMultiQuantity(
      buildEmptyManagedSubscriptionProduct('subscription'),
    ),
  );
  const subscriptionEditResetTimerRef = useRef(null);
  const paygProductResetTimerRef = useRef(null);
  const payRequestProductResetTimerRef = useRef(null);
  const payTokenProductResetTimerRef = useRef(null);
  const [revisionHistoryVisible, setRevisionHistoryVisible] = useState(false);
  const [revisionHistoryLoading, setRevisionHistoryLoading] = useState(false);
  const [revisionHistoryPreset, setRevisionHistoryPreset] = useState(null);
  const [revisionHistoryProductType, setRevisionHistoryProductType] =
    useState('');
  const [revisionHistoryItems, setRevisionHistoryItems] = useState([]);
  const [restoreRevisionVisible, setRestoreRevisionVisible] = useState(false);
  const [restoreRevisionTarget, setRestoreRevisionTarget] = useState(null);
  const [restoreRevisionSyncSoldAssets, setRestoreRevisionSyncSoldAssets] =
    useState(false);
  const [restoringRevisionId, setRestoringRevisionId] = useState(0);

  const [groupsLoading, setGroupsLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);

  const normalizeGroups = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    return list
      .map((g) => {
        if (typeof g === 'string') {
          const code = String(g || '').trim();
          return code ? { id: 0, code, display_name: code } : null;
        }
        const idRaw = Number(g?.id ?? 0);
        const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
        const code = String(g?.code || '').trim();
        if (!code) return null;
        const displayName = String(g?.display_name || code).trim() || code;
        return { id, code, display_name: displayName };
      })
      .filter(Boolean);
  }, []);

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

  const groupIdOptions = useMemo(() => {
    return (Array.isArray(availableGroups) ? availableGroups : [])
      .filter((g) => Number(g?.id ?? 0) > 0)
      .map((g) => ({
        label: g?.display_name || g?.code,
        value: g?.id,
      }))
      .filter((opt) => Number(opt?.value ?? 0) > 0);
  }, [availableGroups]);

  const groupLabelById = useMemo(() => {
    const m = {};
    (Array.isArray(availableGroups) ? availableGroups : []).forEach((g) => {
      const id = Number(g?.id ?? 0);
      if (!Number.isFinite(id) || id <= 0) return;
      const label = String(g?.display_name || g?.code || '').trim();
      if (!label) return;
      m[Math.floor(id)] = label;
    });
    return m;
  }, [availableGroups]);

  const loadPaygOptions = async () => {
    setPaygLoading(true);
    try {
      const res = await API.get('/api/user/topup/info', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取失败'));
        return;
      }

      const rate = Number(data?.payg_credit_usd_per_cny ?? 0);
      const requestRate = Number(
        data?.pay_request_credit_requests_per_cny ?? 0,
      );
      const tokensRate = Number(data?.pay_token_credit_tokens_per_cny ?? 0);

      setPaygFormInitValues({
        credit_usd_per_cny: Number.isFinite(rate) ? rate : 0,
        credit_requests_per_cny: Number.isFinite(requestRate) ? requestRate : 0,
        credit_tokens_per_cny: Number.isFinite(tokensRate) ? tokensRate : 0,
      });
      setPaygProducts(normalizePaygProducts(data?.payg_products));
      setPayRequestProducts(
        normalizePayRequestProducts(data?.pay_request_products),
      );
      setPayTokenProducts(normalizePayTokenProducts(data?.pay_token_products));
      setPaygFormKey((k) => k + 1);
    } catch (e) {
      showError(e?.message || t('获取失败'));
    } finally {
      setPaygLoading(false);
    }
  };

  const loadProductManagementOptions = useCallback(async () => {
    setProductManagementOptionsLoading(true);
    try {
      const res = await API.get(PRODUCT_MANAGEMENT_OPTION_API_BASE);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取商品管理配置失败'));
        return;
      }
      const list = Array.isArray(data) ? data : [];
      const hideArchivedOption = list.find(
        (item) => item?.key === PRODUCT_MANAGEMENT_HIDE_ARCHIVED_OPTION_KEY,
      );
      setHideArchivedProducts(
        hideArchivedOption ? toBoolean(hideArchivedOption.value) : true,
      );
    } catch (e) {
      showError(e?.message || t('获取商品管理配置失败'));
    } finally {
      setProductManagementOptionsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (!actualRootUser && permissionsLoading) {
      return;
    }
    if (!isRootUser) {
      return;
    }
    void loadProductManagementOptions();
    void loadGroups();
    void loadPaygOptions();
    void loadSubscriptionProducts();
  }, [
    actualRootUser,
    isRootUser,
    loadProductManagementOptions,
    permissionsLoading,
  ]);

  useEffect(() => {
    return () => {
      if (subscriptionEditResetTimerRef.current) {
        clearTimeout(subscriptionEditResetTimerRef.current);
      }
      if (paygProductResetTimerRef.current) {
        clearTimeout(paygProductResetTimerRef.current);
      }
      if (payRequestProductResetTimerRef.current) {
        clearTimeout(payRequestProductResetTimerRef.current);
      }
      if (payTokenProductResetTimerRef.current) {
        clearTimeout(payTokenProductResetTimerRef.current);
      }
    };
  }, []);

  const updateOption = useCallback(
    async (key, value) => {
      const res = await API.put(PRODUCT_MANAGEMENT_OPTION_API_BASE, {
        key,
        value,
      });
      const { success, message } = res.data;
      if (!success) {
        throw new Error(message || t('保存失败'));
      }
    },
    [t],
  );

  const updateHideArchivedProducts = useCallback(
    async (checked) => {
      const nextValue = Boolean(checked);
      const previousValue = hideArchivedProducts;
      setHideArchivedProducts(nextValue);
      setHideArchivedProductsSaving(true);
      try {
        await updateOption(
          PRODUCT_MANAGEMENT_HIDE_ARCHIVED_OPTION_KEY,
          String(nextValue),
        );
        await loadProductManagementOptions();
      } catch (e) {
        setHideArchivedProducts(previousValue);
        showError(e?.message || t('保存失败'));
      } finally {
        setHideArchivedProductsSaving(false);
      }
    },
    [hideArchivedProducts, loadProductManagementOptions, t, updateOption],
  );

  const savePaygOptions = async () => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (!paygFormApiRef.current) return;

    const values = paygFormApiRef.current.getValues();
    const creditUsdPerCny = Number(values?.credit_usd_per_cny ?? 0);
    const creditRequestsPerCny = parseNonNegativeIntegerValue(
      values?.credit_requests_per_cny ?? 0,
    );
    const creditTokensPerCny = parseNonNegativeIntegerValue(
      values?.credit_tokens_per_cny ?? 0,
    );

    if (!Number.isFinite(creditUsdPerCny) || creditUsdPerCny <= 0) {
      showError(t('兑换比例必须大于 0'));
      return;
    }
    if (creditRequestsPerCny === null) {
      showError(t('按次付费兑换比例必须为大于等于 0 的整数'));
      return;
    }
    if (creditTokensPerCny === null) {
      showError(t('按token付费兑换比例必须为大于等于 0 的整数'));
      return;
    }

    setPaygSaving(true);
    try {
      await updateOption('payg.credit_usd_per_cny', creditUsdPerCny);
      await updateOption('payg.credit_requests_per_cny', creditRequestsPerCny);
      await updateOption('payg.credit_tokens_per_cny', creditTokensPerCny);
      showSuccess(t('保存成功'));
      void loadPaygOptions();
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setPaygSaving(false);
    }
  };

  const loadSubscriptionProducts = useCallback(async () => {
    setSubscriptionProductsLoading(true);
    try {
      const res = await API.get(PRODUCT_MANAGEMENT_PRESET_API_BASE);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取失败'));
        return;
      }
      setSubscriptionProducts(normalizeManagedSubscriptionProducts(data));
    } catch (e) {
      showError(e?.message || t('获取失败'));
    } finally {
      setSubscriptionProductsLoading(false);
    }
  }, [t]);

  const openCreateSubscriptionProduct = useCallback(() => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (subscriptionEditResetTimerRef.current) {
      clearTimeout(subscriptionEditResetTimerRef.current);
      subscriptionEditResetTimerRef.current = null;
    }
    setEditingSubscriptionProduct(
      clearSubscriptionMultiQuantity(
        buildEmptyManagedSubscriptionProduct('subscription'),
      ),
    );
    setSubscriptionEditVisible(true);
  }, [isRootUser, t]);

  const openCreateTokensProduct = useCallback(() => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (subscriptionEditResetTimerRef.current) {
      clearTimeout(subscriptionEditResetTimerRef.current);
      subscriptionEditResetTimerRef.current = null;
    }
    setEditingSubscriptionProduct(
      clearSubscriptionMultiQuantity(
        buildEmptyManagedSubscriptionProduct('tokens'),
      ),
    );
    setSubscriptionEditVisible(true);
  }, [isRootUser, t]);

  const openCreateRequestProduct = useCallback(() => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (subscriptionEditResetTimerRef.current) {
      clearTimeout(subscriptionEditResetTimerRef.current);
      subscriptionEditResetTimerRef.current = null;
    }
    setEditingSubscriptionProduct(
      clearSubscriptionMultiQuantity(
        buildEmptyManagedSubscriptionProduct('request'),
      ),
    );
    setSubscriptionEditVisible(true);
  }, [isRootUser, t]);

  const openEditSubscriptionProduct = useCallback(
    (preset) => {
      if (!isRootUser) {
        showError(t('需要 Root 权限'));
        return;
      }
      if (subscriptionEditResetTimerRef.current) {
        clearTimeout(subscriptionEditResetTimerRef.current);
        subscriptionEditResetTimerRef.current = null;
      }
      const raw =
        normalizeManagedSubscriptionProduct(preset) ||
        buildEmptyManagedSubscriptionProduct('subscription');
      setEditingSubscriptionProduct(clearSubscriptionMultiQuantity(raw));
      setSubscriptionEditVisible(true);
    },
    [isRootUser, t],
  );

  const closeSubscriptionEdit = useCallback(() => {
    setSubscriptionEditVisible(false);
    if (subscriptionEditResetTimerRef.current) {
      clearTimeout(subscriptionEditResetTimerRef.current);
    }
    subscriptionEditResetTimerRef.current = setTimeout(
      () =>
        setEditingSubscriptionProduct(
          clearSubscriptionMultiQuantity(
            buildEmptyManagedSubscriptionProduct('subscription'),
          ),
        ),
      300,
    );
  }, []);

  const handleSubscriptionProductSaved = useCallback(
    (savedProduct) => {
      const normalizedProduct =
        normalizeManagedSubscriptionProduct(savedProduct);
      if (!normalizedProduct || Number(normalizedProduct?.id ?? 0) <= 0) {
        void loadSubscriptionProducts();
        return;
      }
      const savedId = Number(normalizedProduct.id);
      setSubscriptionProducts((prev) => {
        const list = Array.isArray(prev) ? prev : [];
        const exists = list.some((item) => Number(item?.id ?? 0) === savedId);
        if (exists) {
          return list.map((item) =>
            Number(item?.id ?? 0) === savedId ? normalizedProduct : item,
          );
        }
        return [normalizedProduct, ...list];
      });
    },
    [loadSubscriptionProducts],
  );

  const formatTimestampLabel = useCallback(
    (timestamp) => {
      const ts = Number(timestamp) || 0;
      if (!ts) return t('未配置');
      return timestamp2string(ts);
    },
    [t],
  );

  const productTableBusy =
    !isRootUser ||
    productManagementOptionsLoading ||
    hideArchivedProductsSaving ||
    paygProductsSaving ||
    paygLoading ||
    payRequestProductsSaving ||
    payTokenProductsSaving ||
    subscriptionProductsLoading ||
    productsReordering;

  const buildProductMetaLabels = useCallback(
    (type, raw) => {
      const meta = [];
      if (raw?.archived === true) {
        meta.push(t('已停用'));
      } else if (raw?.enabled === false) {
        meta.push(t('已下架'));
      }
      const stock = normalizeStockValue(raw?.stock);
      if (stock === null) {
        meta.push(`${t('库存')}: ${t('无限')}`);
      } else if (stock === 0) {
        meta.push(`${t('库存')}: ${t('售罄')}`);
      } else if (typeof stock === 'number') {
        meta.push(`${t('库存')}: ${stock}`);
      }

      const priceFen = Number(raw?.price_fen ?? 0) || 0;
      if (priceFen > 0) {
        meta.push(`${t('售价')}: ${renderCnyFen(priceFen)}`);
      }

      const sortOrder = Number(raw?.sort_order ?? 0) || 0;
      meta.push(`${t('排序')}: ${sortOrder}`);

      if (type === 'subscription') {
        const quota = Number(raw?.quota ?? 0) || 0;
        meta.push(
          `${t('额度')}: ${quota <= 0 ? t('无限') : renderQuota(quota)}`,
        );
        const dailyLimit = Number(raw?.daily_quota_limit ?? 0) || 0;
        meta.push(
          `${t('日限')}: ${
            dailyLimit <= 0 ? t('无限') : renderQuota(dailyLimit)
          }`,
        );
      } else if (type === 'tokens') {
        const quota = Number(raw?.quota ?? 0) || 0;
        meta.push(
          `${t('Tokens')}: ${quota <= 0 ? t('无限') : quota.toLocaleString()}`,
        );
        const dailyLimit = Number(raw?.daily_quota_limit ?? 0) || 0;
        meta.push(
          `${t('日限')}: ${
            dailyLimit <= 0 ? t('无限') : dailyLimit.toLocaleString()
          }`,
        );
      } else if (type === 'request') {
        const totalCount = Number(raw?.quota ?? 0) || 0;
        meta.push(
          `${t('总次数')}: ${totalCount <= 0 ? t('无限') : totalCount}`,
        );
        const dailyCount = Number(raw?.daily_request_limit ?? 0) || 0;
        meta.push(`${t('日限')}: ${dailyCount <= 0 ? t('无限') : dailyCount}`);
      }

      if (type === 'subscription' || type === 'tokens' || type === 'request') {
        const validDays = Number(raw?.quota_valid_days ?? 0) || 0;
        meta.push(
          `${t('有效期')}: ${
            validDays <= 0 ? t('无限') : `${validDays} ${t('天')}`
          }`,
        );

        const limit = Number(raw?.purchase_limit ?? 0) || 0;
        if (limit > 0) {
          meta.push(`${t('限购')}: ${limit} ${t('次')}`);
        }
      }

      return meta;
    },
    [t],
  );

  const buildGroupDailyLimitLabels = useCallback(
    (type, raw) => {
      const items = Array.isArray(raw?.group_daily_limits)
        ? raw.group_daily_limits
        : [];
      return items
        .map((item) => {
          const gid = Number(item?.group_id ?? 0) || 0;
          if (gid <= 0) return null;
          const limit = Number(item?.daily_quota_limit ?? 0) || 0;
          const limitLabel =
            limit <= 0
              ? t('无限')
              : type === 'tokens'
              ? limit.toLocaleString()
              : renderQuota(limit);
          return `${groupLabelById[gid] || t('未知分组')}: ${limitLabel}`;
        })
        .filter(Boolean);
    },
    [groupLabelById, t],
  );

  const renderProductTypeTag = useCallback(
    (type) => {
      if (type === 'payg') {
        return (
          <Tag color='blue' shape='circle'>
            {t('按量付费')}
          </Tag>
        );
      }
      if (type === 'pay_request') {
        return (
          <Tag color='purple' shape='circle'>
            {t('按次付费')}
          </Tag>
        );
      }
      if (type === 'pay_token') {
        return (
          <Tag color='pink' shape='circle'>
            {t('按token付费')}
          </Tag>
        );
      }
      if (type === 'tokens') {
        return (
          <Tag color='orange' shape='circle'>
            {t('Tokens订阅')}
          </Tag>
        );
      }
      if (type === 'request') {
        return (
          <Tag color='cyan' shape='circle'>
            {t('次数订阅')}
          </Tag>
        );
      }
      return (
        <Tag color='green' shape='circle'>
          {t('订阅额度')}
        </Tag>
      );
    },
    [t],
  );

  const fetchProductRevisions = useCallback(
    async (productType, productId) => {
      const type = String(productType || '').trim();
      const pid = Number(productId ?? 0) || 0;
      if (!type || !pid) return [];
      const url = isSubscriptionProductType(type)
        ? `${PRODUCT_MANAGEMENT_PRESET_API_BASE}/${pid}/revisions`
        : `${PRODUCT_MANAGEMENT_PAY_PRODUCT_API_BASE}/${type}/${pid}/revisions`;
      const res = await API.get(url, {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data;
      if (!success) {
        throw new Error(message || t('获取失败'));
      }
      return Array.isArray(data) ? data : [];
    },
    [t],
  );

  const openProductRevisionHistory = useCallback(
    async (productType, product) => {
      if (!isRootUser) {
        showError(t('需要 Root 权限'));
        return;
      }
      const type = String(productType || '').trim();
      const productId = Number(product?.id ?? 0) || 0;
      if (!type || !productId) return;

      setRevisionHistoryPreset(product);
      setRevisionHistoryProductType(type);
      setRevisionHistoryItems([]);
      setRevisionHistoryVisible(true);
      setRevisionHistoryLoading(true);
      try {
        const revisions = await fetchProductRevisions(type, productId);
        setRevisionHistoryItems(revisions);
      } catch (e) {
        showError(e?.message || t('获取失败'));
      } finally {
        setRevisionHistoryLoading(false);
      }
    },
    [fetchProductRevisions, isRootUser, t],
  );

  const closeSubscriptionRevisionHistory = useCallback(() => {
    setRevisionHistoryVisible(false);
    setRevisionHistoryLoading(false);
    setRevisionHistoryPreset(null);
    setRevisionHistoryProductType('');
    setRevisionHistoryItems([]);
    setRestoreRevisionVisible(false);
    setRestoreRevisionTarget(null);
    setRestoreRevisionSyncSoldAssets(false);
    setRestoringRevisionId(0);
  }, []);

  const openRestoreRevisionDialog = useCallback((revision) => {
    setRestoreRevisionTarget(revision || null);
    setRestoreRevisionSyncSoldAssets(false);
    setRestoreRevisionVisible(true);
  }, []);

  const closeRestoreRevisionDialog = useCallback(() => {
    if (restoringRevisionId > 0) return;
    setRestoreRevisionVisible(false);
    setRestoreRevisionTarget(null);
    setRestoreRevisionSyncSoldAssets(false);
  }, [restoringRevisionId]);

  const handleRestoreSubscriptionRevision = useCallback(async () => {
    const presetId = Number(revisionHistoryPreset?.id ?? 0) || 0;
    const productType = String(revisionHistoryProductType || '').trim();
    const revisionId = Number(restoreRevisionTarget?.id ?? 0) || 0;
    if (!presetId || !revisionId || !productType) {
      showError(t('参数错误'));
      return;
    }

    setRestoringRevisionId(revisionId);
    setRevisionHistoryLoading(true);
    try {
      const payload = { revision_id: revisionId };
      if (isSubscriptionProductType(productType)) {
        payload.sync_sold_assets = restoreRevisionSyncSoldAssets;
      }
      const url = isSubscriptionProductType(productType)
        ? `${PRODUCT_MANAGEMENT_PRESET_API_BASE}/${presetId}/restore`
        : `${PRODUCT_MANAGEMENT_PAY_PRODUCT_API_BASE}/${productType}/${presetId}/restore`;
      const res = await API.post(url, payload);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('恢复失败'));
        return;
      }

      showSuccess(t('恢复成功'));

      if (data && Number(data?.id ?? 0) === presetId) {
        if (productType === 'payg') {
          const normalized = normalizePaygProducts([data])[0];
          if (normalized) {
            setPaygProducts((prev) =>
              (Array.isArray(prev) ? prev : []).map((item) =>
                Number(item?.id ?? 0) === presetId ? normalized : item,
              ),
            );
            setRevisionHistoryPreset(normalized);
          }
        } else if (productType === 'pay_request') {
          const normalized = normalizePayRequestProducts([data])[0];
          if (normalized) {
            setPayRequestProducts((prev) =>
              (Array.isArray(prev) ? prev : []).map((item) =>
                Number(item?.id ?? 0) === presetId ? normalized : item,
              ),
            );
            setRevisionHistoryPreset(normalized);
          }
        } else if (productType === 'pay_token') {
          const normalized = normalizePayTokenProducts([data])[0];
          if (normalized) {
            setPayTokenProducts((prev) =>
              (Array.isArray(prev) ? prev : []).map((item) =>
                Number(item?.id ?? 0) === presetId ? normalized : item,
              ),
            );
            setRevisionHistoryPreset(normalized);
          }
        } else {
          const normalized = normalizeManagedSubscriptionProduct(data);
          if (!normalized) {
            void loadSubscriptionProducts();
            return;
          }
          setSubscriptionProducts((prev) =>
            (Array.isArray(prev) ? prev : []).map((item) =>
              Number(item?.id ?? 0) === presetId ? normalized : item,
            ),
          );
          setRevisionHistoryPreset(normalized);
        }
      } else {
        if (isSubscriptionProductType(productType)) {
          void loadSubscriptionProducts();
        } else {
          void loadPaygOptions();
        }
      }

      const revisions = await fetchProductRevisions(productType, presetId);
      setRevisionHistoryItems(revisions);
      setRestoreRevisionVisible(false);
      setRestoreRevisionTarget(null);
      setRestoreRevisionSyncSoldAssets(false);
    } catch (e) {
      showError(e?.message || t('恢复失败'));
    } finally {
      setRevisionHistoryLoading(false);
      setRestoringRevisionId(0);
    }
  }, [
    fetchProductRevisions,
    loadPaygOptions,
    loadSubscriptionProducts,
    revisionHistoryProductType,
    restoreRevisionSyncSoldAssets,
    restoreRevisionTarget,
    revisionHistoryPreset,
    t,
  ]);

  const sortedPaygProducts = useMemo(() => {
    const list = Array.isArray(paygProducts) ? paygProducts : [];
    return list
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      })
      .map((p) => ({
        ...p,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
      }));
  }, [paygProducts]);

  const getNextPaygProductId = useCallback(() => {
    const ids = (Array.isArray(paygProducts) ? paygProducts : [])
      .map((p) => Number(p?.id ?? 0))
      .filter((id) => Number.isFinite(id) && id > 0);
    const maxId = ids.length > 0 ? Math.max(...ids) : 0;
    return maxId + 1;
  }, [paygProducts]);

  const openCreatePaygProduct = () => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (paygProductResetTimerRef.current) {
      clearTimeout(paygProductResetTimerRef.current);
      paygProductResetTimerRef.current = null;
    }
    setEditingPaygProduct({
      id: getNextPaygProductId(),
      name: '',
      description: '',
      enabled: true,
      archived: false,
      sort_order: 0,
      stock: null,
      allowed_group_ids: [],
    });
    setPaygProductSheetVisible(true);
  };

  const openEditPaygProduct = (product) => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (paygProductResetTimerRef.current) {
      clearTimeout(paygProductResetTimerRef.current);
      paygProductResetTimerRef.current = null;
    }
    const stock = normalizeStockValue(product?.stock);
    setEditingPaygProduct({
      id: Number(product?.id ?? 0) || 0,
      name: String(product?.name ?? ''),
      description: String(product?.description ?? ''),
      enabled: product?.archived === true ? false : product?.enabled !== false,
      archived: product?.archived === true,
      sort_order: Number(product?.sort_order ?? 0) || 0,
      stock: stock === undefined ? null : stock,
      allowed_group_ids: normalizeGroupIds(product?.allowed_group_ids),
    });
    setPaygProductSheetVisible(true);
  };

  const closePaygProductSheet = () => {
    setPaygProductSheetVisible(false);
    if (paygProductResetTimerRef.current) {
      clearTimeout(paygProductResetTimerRef.current);
    }
    paygProductResetTimerRef.current = setTimeout(() => {
      setEditingPaygProduct(null);
    }, 300);
  };

  const persistPaygProducts = async (nextProducts, options = {}) => {
    const silentSuccess = options?.silentSuccess === true;
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return false;
    }
    const normalized = normalizePaygProducts(nextProducts);
    if (
      normalized.length !==
      (Array.isArray(nextProducts) ? nextProducts.length : 0)
    ) {
      showError(t('按量付费商品配置不合法'));
      return false;
    }

    setPaygProductsSaving(true);
    try {
      await updateOption('payg.products', normalized);
      setPaygProducts(normalized);
      if (!silentSuccess) {
        showSuccess(t('保存成功'));
      }
      return true;
    } catch (e) {
      showError(e?.message || t('保存失败'));
      return false;
    } finally {
      setPaygProductsSaving(false);
    }
  };

  const savePaygProduct = async () => {
    if (!paygProductFormApiRef.current) return;
    if (!editingPaygProduct?.id) return;

    const values = paygProductFormApiRef.current.getValues();
    const id = Number(editingPaygProduct.id);
    const name = String(values?.name ?? '').trim();
    const description = String(values?.description ?? '').trim();
    const archived = Boolean(values?.archived);
    const enabled = archived ? false : Boolean(values?.enabled);
    const sortRaw = Number(values?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;
    const stock = normalizeStockValue(values?.stock);
    const allowedGroupIds = normalizeGroupIds(values?.allowed_group_ids || []);

    if (!name) {
      showError(t('请输入名称'));
      return;
    }
    if (stock === undefined) {
      showError(t('库存必须为大于等于 0 的整数，留空表示无限制'));
      return;
    }
    if (!archived && enabled && allowedGroupIds.length === 0) {
      showError(t('可用分组不能为空'));
      return;
    }

    const product = {
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    };

    const exists = (Array.isArray(paygProducts) ? paygProducts : []).some(
      (p) => Number(p?.id ?? 0) === id,
    );
    const nextProducts = exists
      ? (Array.isArray(paygProducts) ? paygProducts : []).map((p) =>
          Number(p?.id ?? 0) === id ? product : p,
        )
      : [...(Array.isArray(paygProducts) ? paygProducts : []), product];

    const ok = await persistPaygProducts(nextProducts);
    if (ok) {
      closePaygProductSheet();
    }
  };

  const deletePaygProduct = async (id) => {
    const pid = Number(id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (
      Array.isArray(paygProducts) ? paygProducts : []
    ).filter((p) => Number(p?.id ?? 0) !== pid);
    await persistPaygProducts(nextProducts);
  };

  const togglePaygProductEnabled = async (product, enabled) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (Array.isArray(paygProducts) ? paygProducts : []).map(
      (p) =>
        Number(p?.id ?? 0) === pid
          ? { ...p, enabled: Boolean(enabled), archived: false }
          : p,
    );
    await persistPaygProducts(nextProducts);
  };

  const updatePaygProductArchived = async (product, archived) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextArchived = Boolean(archived);
    const nextProducts = (Array.isArray(paygProducts) ? paygProducts : []).map(
      (p) =>
        Number(p?.id ?? 0) === pid
          ? {
              ...p,
              archived: nextArchived,
              enabled: nextArchived ? false : p?.enabled !== false,
            }
          : p,
    );
    await persistPaygProducts(nextProducts);
  };

  const sortedPayRequestProducts = useMemo(() => {
    const list = Array.isArray(payRequestProducts) ? payRequestProducts : [];
    return list
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      })
      .map((p) => ({
        ...p,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
      }));
  }, [payRequestProducts]);

  const getNextPayRequestProductId = useCallback(() => {
    const ids = (Array.isArray(payRequestProducts) ? payRequestProducts : [])
      .map((p) => Number(p?.id ?? 0))
      .filter((id) => Number.isFinite(id) && id > 0);
    const maxId = ids.length > 0 ? Math.max(...ids) : 0;
    return maxId + 1;
  }, [payRequestProducts]);

  const openCreatePayRequestProduct = () => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (payRequestProductResetTimerRef.current) {
      clearTimeout(payRequestProductResetTimerRef.current);
      payRequestProductResetTimerRef.current = null;
    }
    setEditingPayRequestProduct({
      id: getNextPayRequestProductId(),
      name: '',
      description: '',
      enabled: true,
      archived: false,
      sort_order: 0,
      stock: null,
      allowed_group_ids: [],
    });
    setPayRequestProductSheetVisible(true);
  };

  const openEditPayRequestProduct = (product) => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (payRequestProductResetTimerRef.current) {
      clearTimeout(payRequestProductResetTimerRef.current);
      payRequestProductResetTimerRef.current = null;
    }
    const stock = normalizeStockValue(product?.stock);
    setEditingPayRequestProduct({
      id: Number(product?.id ?? 0) || 0,
      name: String(product?.name ?? ''),
      description: String(product?.description ?? ''),
      enabled: product?.archived === true ? false : product?.enabled !== false,
      archived: product?.archived === true,
      sort_order: Number(product?.sort_order ?? 0) || 0,
      stock: stock === undefined ? null : stock,
      allowed_group_ids: normalizeGroupIds(product?.allowed_group_ids),
    });
    setPayRequestProductSheetVisible(true);
  };

  const closePayRequestProductSheet = () => {
    setPayRequestProductSheetVisible(false);
    if (payRequestProductResetTimerRef.current) {
      clearTimeout(payRequestProductResetTimerRef.current);
    }
    payRequestProductResetTimerRef.current = setTimeout(() => {
      setEditingPayRequestProduct(null);
    }, 300);
  };

  const persistPayRequestProducts = async (nextProducts, options = {}) => {
    const silentSuccess = options?.silentSuccess === true;
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return false;
    }
    const normalized = normalizePayRequestProducts(nextProducts);
    if (
      normalized.length !==
      (Array.isArray(nextProducts) ? nextProducts.length : 0)
    ) {
      showError(t('按次付费商品配置不合法'));
      return false;
    }

    setPayRequestProductsSaving(true);
    try {
      await updateOption('payg.pay_request_products', normalized);
      setPayRequestProducts(normalized);
      if (!silentSuccess) {
        showSuccess(t('保存成功'));
      }
      return true;
    } catch (e) {
      showError(e?.message || t('保存失败'));
      return false;
    } finally {
      setPayRequestProductsSaving(false);
    }
  };

  const savePayRequestProduct = async () => {
    if (!payRequestProductFormApiRef.current) return;
    if (!editingPayRequestProduct?.id) return;

    const values = payRequestProductFormApiRef.current.getValues();
    const id = Number(editingPayRequestProduct.id);
    const name = String(values?.name ?? '').trim();
    const description = String(values?.description ?? '').trim();
    const archived = Boolean(values?.archived);
    const enabled = archived ? false : Boolean(values?.enabled);
    const sortRaw = Number(values?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;
    const stock = normalizeStockValue(values?.stock);
    const allowedGroupIds = normalizeGroupIds(values?.allowed_group_ids || []);

    if (!name) {
      showError(t('请输入名称'));
      return;
    }
    if (stock === undefined) {
      showError(t('库存必须为大于等于 0 的整数，留空表示无限制'));
      return;
    }
    if (!archived && enabled && allowedGroupIds.length === 0) {
      showError(t('可用分组不能为空'));
      return;
    }

    const product = {
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    };

    const exists = (
      Array.isArray(payRequestProducts) ? payRequestProducts : []
    ).some((p) => Number(p?.id ?? 0) === id);
    const nextProducts = exists
      ? (Array.isArray(payRequestProducts) ? payRequestProducts : []).map(
          (p) => (Number(p?.id ?? 0) === id ? product : p),
        )
      : [
          ...(Array.isArray(payRequestProducts) ? payRequestProducts : []),
          product,
        ];

    const ok = await persistPayRequestProducts(nextProducts);
    if (ok) {
      closePayRequestProductSheet();
    }
  };

  const deletePayRequestProduct = async (id) => {
    const pid = Number(id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (
      Array.isArray(payRequestProducts) ? payRequestProducts : []
    ).filter((p) => Number(p?.id ?? 0) !== pid);
    await persistPayRequestProducts(nextProducts);
  };

  const togglePayRequestProductEnabled = async (product, enabled) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (
      Array.isArray(payRequestProducts) ? payRequestProducts : []
    ).map((p) =>
      Number(p?.id ?? 0) === pid
        ? { ...p, enabled: Boolean(enabled), archived: false }
        : p,
    );
    await persistPayRequestProducts(nextProducts);
  };

  const updatePayRequestProductArchived = async (product, archived) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextArchived = Boolean(archived);
    const nextProducts = (
      Array.isArray(payRequestProducts) ? payRequestProducts : []
    ).map((p) =>
      Number(p?.id ?? 0) === pid
        ? {
            ...p,
            archived: nextArchived,
            enabled: nextArchived ? false : p?.enabled !== false,
          }
        : p,
    );
    await persistPayRequestProducts(nextProducts);
  };

  const sortedPayTokenProducts = useMemo(() => {
    const list = Array.isArray(payTokenProducts) ? payTokenProducts : [];
    return list
      .slice()
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      })
      .map((p) => ({
        ...p,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
      }));
  }, [payTokenProducts]);

  const getNextPayTokenProductId = useCallback(() => {
    const ids = (Array.isArray(payTokenProducts) ? payTokenProducts : [])
      .map((p) => Number(p?.id ?? 0))
      .filter((id) => Number.isFinite(id) && id > 0);
    const maxId = ids.length > 0 ? Math.max(...ids) : 0;
    return maxId + 1;
  }, [payTokenProducts]);

  const openCreatePayTokenProduct = () => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (payTokenProductResetTimerRef.current) {
      clearTimeout(payTokenProductResetTimerRef.current);
      payTokenProductResetTimerRef.current = null;
    }
    setEditingPayTokenProduct({
      id: getNextPayTokenProductId(),
      name: '',
      description: '',
      enabled: true,
      archived: false,
      sort_order: 0,
      stock: null,
      allowed_group_ids: [],
    });
    setPayTokenProductSheetVisible(true);
  };

  const openEditPayTokenProduct = (product) => {
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return;
    }
    if (payTokenProductResetTimerRef.current) {
      clearTimeout(payTokenProductResetTimerRef.current);
      payTokenProductResetTimerRef.current = null;
    }
    const stock = normalizeStockValue(product?.stock);
    setEditingPayTokenProduct({
      id: Number(product?.id ?? 0) || 0,
      name: String(product?.name ?? ''),
      description: String(product?.description ?? ''),
      enabled: product?.archived === true ? false : product?.enabled !== false,
      archived: product?.archived === true,
      sort_order: Number(product?.sort_order ?? 0) || 0,
      stock: stock === undefined ? null : stock,
      allowed_group_ids: normalizeGroupIds(product?.allowed_group_ids),
    });
    setPayTokenProductSheetVisible(true);
  };

  const closePayTokenProductSheet = () => {
    setPayTokenProductSheetVisible(false);
    if (payTokenProductResetTimerRef.current) {
      clearTimeout(payTokenProductResetTimerRef.current);
    }
    payTokenProductResetTimerRef.current = setTimeout(() => {
      setEditingPayTokenProduct(null);
    }, 300);
  };

  const persistPayTokenProducts = async (nextProducts, options = {}) => {
    const silentSuccess = options?.silentSuccess === true;
    if (!isRootUser) {
      showError(t('需要 Root 权限'));
      return false;
    }
    const normalized = normalizePayTokenProducts(nextProducts);
    if (
      normalized.length !==
      (Array.isArray(nextProducts) ? nextProducts.length : 0)
    ) {
      showError(t('按token付费商品配置不合法'));
      return false;
    }

    setPayTokenProductsSaving(true);
    try {
      await updateOption('payg.pay_token_products', normalized);
      setPayTokenProducts(normalized);
      if (!silentSuccess) {
        showSuccess(t('保存成功'));
      }
      return true;
    } catch (e) {
      showError(e?.message || t('保存失败'));
      return false;
    } finally {
      setPayTokenProductsSaving(false);
    }
  };

  const savePayTokenProduct = async () => {
    if (!payTokenProductFormApiRef.current) return;
    if (!editingPayTokenProduct?.id) return;

    const values = payTokenProductFormApiRef.current.getValues();
    const id = Number(editingPayTokenProduct.id);
    const name = String(values?.name ?? '').trim();
    const description = String(values?.description ?? '').trim();
    const archived = Boolean(values?.archived);
    const enabled = archived ? false : Boolean(values?.enabled);
    const sortRaw = Number(values?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortRaw)
      ? Math.max(0, Math.floor(sortRaw))
      : 0;
    const stock = normalizeStockValue(values?.stock);
    const allowedGroupIds = normalizeGroupIds(values?.allowed_group_ids || []);

    if (!name) {
      showError(t('请输入名称'));
      return;
    }
    if (stock === undefined) {
      showError(t('库存必须为大于等于 0 的整数，留空表示无限制'));
      return;
    }
    if (!archived && enabled && allowedGroupIds.length === 0) {
      showError(t('可用分组不能为空'));
      return;
    }

    const product = {
      id,
      name,
      description,
      enabled,
      archived,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    };

    const exists = (
      Array.isArray(payTokenProducts) ? payTokenProducts : []
    ).some((p) => Number(p?.id ?? 0) === id);
    const nextProducts = exists
      ? (Array.isArray(payTokenProducts) ? payTokenProducts : []).map((p) =>
          Number(p?.id ?? 0) === id ? product : p,
        )
      : [...(Array.isArray(payTokenProducts) ? payTokenProducts : []), product];

    const ok = await persistPayTokenProducts(nextProducts);
    if (ok) {
      closePayTokenProductSheet();
    }
  };

  const deletePayTokenProduct = async (id) => {
    const pid = Number(id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (
      Array.isArray(payTokenProducts) ? payTokenProducts : []
    ).filter((p) => Number(p?.id ?? 0) !== pid);
    await persistPayTokenProducts(nextProducts);
  };

  const togglePayTokenProductEnabled = async (product, enabled) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextProducts = (
      Array.isArray(payTokenProducts) ? payTokenProducts : []
    ).map((p) =>
      Number(p?.id ?? 0) === pid
        ? { ...p, enabled: Boolean(enabled), archived: false }
        : p,
    );
    await persistPayTokenProducts(nextProducts);
  };

  const updatePayTokenProductArchived = async (product, archived) => {
    const pid = Number(product?.id ?? 0);
    if (!Number.isFinite(pid) || pid <= 0) return;
    const nextArchived = Boolean(archived);
    const nextProducts = (
      Array.isArray(payTokenProducts) ? payTokenProducts : []
    ).map((p) =>
      Number(p?.id ?? 0) === pid
        ? {
            ...p,
            archived: nextArchived,
            enabled: nextArchived ? false : p?.enabled !== false,
          }
        : p,
    );
    await persistPayTokenProducts(nextProducts);
  };

  const upsertSubscriptionProduct = useCallback(
    async (preset, patch, options = {}) => {
      if (!isRootUser) {
        showError(t('需要 Root 权限'));
        return false;
      }
      if (!preset?.id || !preset?.name) return false;
      const payload = buildRedemptionPresetUpsertPayload(preset, patch);
      setSubscriptionProductsLoading(true);
      try {
        const res = await API.post(PRODUCT_MANAGEMENT_PRESET_API_BASE, payload);
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('保存失败'));
          return false;
        }
        if (!options?.silentSuccess) {
          showSuccess(t('保存成功'));
        }
        if (data) {
          const normalizedData = clearSubscriptionMultiQuantity(
            normalizeManagedSubscriptionProduct(data),
          );
          if (!normalizedData) {
            void loadSubscriptionProducts();
            return true;
          }
          setSubscriptionProducts((prev) =>
            (Array.isArray(prev) ? prev : []).map((p) =>
              Number(p?.id ?? 0) === Number(preset.id) ? normalizedData : p,
            ),
          );
        } else {
          void loadSubscriptionProducts();
        }
        return true;
      } catch (e) {
        showError(e?.message || t('保存失败'));
        void loadSubscriptionProducts();
        return false;
      } finally {
        setSubscriptionProductsLoading(false);
      }
    },
    [isRootUser, loadSubscriptionProducts, t],
  );

  const updateSubscriptionProductEnabled = useCallback(
    async (preset, enabled) => {
      let syncSoldAssets = false;
      Modal.confirm({
        title: t('确认修改商品'),
        content: (
          <div className='space-y-3'>
            <div className='text-sm text-gray-700'>
              {t('将修改商品上架状态，并生成新的商品版本')}
            </div>
            <div className='rounded-xl border border-gray-200 px-3 py-3'>
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='font-medium'>{t('同步调整已售商品')}</div>
                  <div className='mt-1 text-xs text-gray-500'>
                    {t(
                      '默认关闭；关闭时仅影响后续售出商品，开启时同时调整已售订阅资产',
                    )}
                  </div>
                </div>
                <Switch
                  onChange={(v) => {
                    syncSoldAssets = Boolean(v);
                  }}
                />
              </div>
            </div>
          </div>
        ),
        onOk: () =>
          upsertSubscriptionProduct(preset, {
            enabled: Boolean(enabled),
            archived: false,
            sync_sold_assets: syncSoldAssets,
          }),
      });
    },
    [t, upsertSubscriptionProduct],
  );

  const updateSubscriptionProductArchived = useCallback(
    async (preset, archived) => {
      const nextArchived = Boolean(archived);
      await upsertSubscriptionProduct(preset, {
        archived: nextArchived,
        enabled: nextArchived ? false : preset?.enabled !== false,
        sync_sold_assets: false,
      });
    },
    [upsertSubscriptionProduct],
  );

  const copyProduct = useCallback(
    async (record) => {
      if (!isRootUser) {
        showError(t('需要 Root 权限'));
        return;
      }
      if (!record?.raw || !record?.name) return;

      if (record.type === 'payg') {
        const nextId = getNextPaygProductId();
        const existing = new Set(
          (Array.isArray(paygProducts) ? paygProducts : [])
            .map((p) => String(p?.name || '').trim())
            .filter(Boolean),
        );
        const nextName = buildCopiedName(record.name, (n) => existing.has(n));
        if (!nextName) {
          showError(t('复制失败'));
          return;
        }
        const nextProducts = [
          ...(Array.isArray(paygProducts) ? paygProducts : []),
          {
            ...record.raw,
            id: nextId,
            name: nextName,
            archived: false,
            enabled: false,
          },
        ];
        await persistPaygProducts(nextProducts);
        return;
      }

      if (record.type === 'pay_request') {
        const nextId = getNextPayRequestProductId();
        const existing = new Set(
          (Array.isArray(payRequestProducts) ? payRequestProducts : [])
            .map((p) => String(p?.name || '').trim())
            .filter(Boolean),
        );
        const nextName = buildCopiedName(record.name, (n) => existing.has(n));
        if (!nextName) {
          showError(t('复制失败'));
          return;
        }
        const nextProducts = [
          ...(Array.isArray(payRequestProducts) ? payRequestProducts : []),
          {
            ...record.raw,
            id: nextId,
            name: nextName,
            archived: false,
            enabled: false,
          },
        ];
        await persistPayRequestProducts(nextProducts);
        return;
      }

      if (record.type === 'pay_token') {
        const nextId = getNextPayTokenProductId();
        const existing = new Set(
          (Array.isArray(payTokenProducts) ? payTokenProducts : [])
            .map((p) => String(p?.name || '').trim())
            .filter(Boolean),
        );
        const nextName = buildCopiedName(record.name, (n) => existing.has(n));
        if (!nextName) {
          showError(t('复制失败'));
          return;
        }
        const nextProducts = [
          ...(Array.isArray(payTokenProducts) ? payTokenProducts : []),
          {
            ...record.raw,
            id: nextId,
            name: nextName,
            archived: false,
            enabled: false,
          },
        ];
        await persistPayTokenProducts(nextProducts);
        return;
      }

      const preset = record.raw;
      const existing = new Set(
        (Array.isArray(subscriptionProducts) ? subscriptionProducts : [])
          .map((p) => String(p?.name || '').trim())
          .filter(Boolean),
      );
      const nextName = buildCopiedName(preset.name, (n) => existing.has(n));
      if (!nextName) {
        showError(t('复制失败'));
        return;
      }
      const payload = buildRedemptionPresetUpsertPayload(preset, {
        id: undefined,
        name: nextName,
        archived: false,
        enabled: false,
      });

      setSubscriptionProductsLoading(true);
      try {
        const res = await API.post(PRODUCT_MANAGEMENT_PRESET_API_BASE, payload);
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('复制失败'));
          return;
        }
        showSuccess(t('复制成功'));
        if (data) {
          const normalizedData = normalizeManagedSubscriptionProduct(data);
          if (!normalizedData) {
            void loadSubscriptionProducts();
            return;
          }
          setSubscriptionProducts((prev) => [
            ...(Array.isArray(prev) ? prev : []),
            normalizedData,
          ]);
        } else {
          void loadSubscriptionProducts();
        }
      } catch (e) {
        showError(e?.message || t('复制失败'));
        void loadSubscriptionProducts();
      } finally {
        setSubscriptionProductsLoading(false);
      }
    },
    [
      getNextPaygProductId,
      getNextPayRequestProductId,
      getNextPayTokenProductId,
      isRootUser,
      loadSubscriptionProducts,
      paygProducts,
      payRequestProducts,
      payTokenProducts,
      persistPaygProducts,
      persistPayRequestProducts,
      persistPayTokenProducts,
      subscriptionProducts,
      t,
    ],
  );

  const deleteSubscriptionProduct = useCallback(
    async (id) => {
      if (!isRootUser) {
        showError(t('需要 Root 权限'));
        return;
      }
      const pid = Number(id ?? 0);
      if (!Number.isFinite(pid) || pid <= 0) return;
      setSubscriptionProductsLoading(true);
      try {
        const res = await API.delete(
          `${PRODUCT_MANAGEMENT_PRESET_API_BASE}/${pid}`,
        );
        const { success, message } = res.data;
        if (!success) {
          showError(message || t('删除失败'));
          return;
        }
        showSuccess(t('删除成功'));
        void loadSubscriptionProducts();
      } catch (e) {
        showError(e?.message || t('删除失败'));
      } finally {
        setSubscriptionProductsLoading(false);
      }
    },
    [isRootUser, loadSubscriptionProducts, t],
  );

  const generateBySubscriptionProduct = useCallback(
    (preset) => {
      const presetId = Number(preset?.id ?? 0) || 0;
      if (!presetId || !preset?.name) return;
      let count = 1;
      Modal.confirm({
        title: t('生成兑换码'),
        content: (
          <div className='space-y-3'>
            <div>
              <Text type='tertiary'>
                {t('预置商品')}: <Text strong>{preset.name}</Text>
              </Text>
            </div>
            <InputNumber
              defaultValue={1}
              min={1}
              max={100}
              precision={0}
              onChange={(v) => {
                const num = parseInt(v, 10);
                count = Number.isFinite(num) && num > 0 ? num : 1;
              }}
              style={{ width: '100%' }}
            />
            <Text type='tertiary' size='small'>
              {t('一次最多生成 100 个')}
            </Text>
          </div>
        ),
        onOk: async () => {
          const res = await API.post(
            `${PRODUCT_MANAGEMENT_PRESET_API_BASE}/generate`,
            {
              preset_id: presetId,
              count,
            },
          );
          const { success, message, data } = res.data;
          if (!success) {
            showError(message || t('生成失败'));
            return;
          }
          const keys = Array.isArray(data) ? data : [];
          showSuccess(t('生成成功'));
          if (keys.length > 0) {
            const text = keys.join('\n') + '\n';
            Modal.confirm({
              title: t('兑换码创建成功'),
              content: (
                <div>
                  <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
                  <p>
                    {t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}
                  </p>
                </div>
              ),
              onOk: () => {
                downloadTextAsFile(text, `${preset.name}.txt`);
              },
            });
          }
        },
      });
    },
    [t],
  );

  const buildGenerateRedemptionCurl = useCallback(
    (presetId, count = 1, presetName) => {
      const pid = Number(presetId ?? 0) || 0;
      const qty = Math.max(
        1,
        Math.min(100, Math.floor(Number(count ?? 1) || 1)),
      );
      const origin =
        typeof window !== 'undefined' && window.location?.origin
          ? window.location.origin
          : '$BASE_URL';
      const uidRaw =
        typeof getUserIdFromLocalStorage === 'function'
          ? getUserIdFromLocalStorage()
          : -1;
      const uid = uidRaw && uidRaw > 0 ? uidRaw : '<ADMIN_USER_ID>';
      const safePresetName = String(presetName ?? '')
        .replace(/[\r\n]+/g, ' ')
        .trim();
      const safePresetNameJson = safePresetName
        ? JSON.stringify(safePresetName).replace(/'/g, '\\u0027')
        : '';
      const nameField = safePresetNameJson
        ? `,\"name\":${safePresetNameJson}`
        : '';
      return `curl -X POST '${origin}${PRODUCT_MANAGEMENT_PRESET_API_BASE}/generate' \\\n  -H 'Content-Type: application/json' \\\n  -H 'Authorization: <ACCESS_TOKEN>' \\\n  -H 'Transfer-Api-User: ${uid}' \\\n  -d '{\"preset_id\":${pid},\"count\":${qty}${nameField}}'`;
    },
    [],
  );

  const copyGenerateRedemptionCurl = useCallback(
    async (preset) => {
      const pid = Number(preset?.id ?? 0) || 0;
      if (!pid) return;
      const cmd = buildGenerateRedemptionCurl(pid, 1, preset?.name);
      if (await copy(cmd)) {
        showSuccess(t('已复制 curl 命令'));
      } else {
        showError(t('复制失败'));
      }
    },
    [buildGenerateRedemptionCurl, t],
  );

  const mergedProducts = useMemo(() => {
    const rows = [];
    const normalizedSubscriptionProducts =
      normalizeManagedSubscriptionProducts(subscriptionProducts);

    normalizedSubscriptionProducts.forEach((p) => {
      const mode = inferPresetMode(p);
      rows.push({
        row_key: `${mode}-${p?.id}`,
        type: mode,
        id: Number(p?.id ?? 0) || 0,
        name: String(p?.name ?? ''),
        description: String(p?.description ?? ''),
        enabled: p?.archived === true ? false : p?.enabled !== false,
        archived: p?.archived === true,
        sort_order: Number(p?.sort_order ?? 0) || 0,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
        raw: p,
      });
    });

    (Array.isArray(sortedPaygProducts) ? sortedPaygProducts : []).forEach(
      (p) => {
        rows.push({
          row_key: `payg-${p?.id}`,
          type: 'payg',
          id: Number(p?.id ?? 0) || 0,
          name: String(p?.name ?? ''),
          description: String(p?.description ?? ''),
          enabled: p?.archived === true ? false : p?.enabled !== false,
          archived: p?.archived === true,
          sort_order: Number(p?.sort_order ?? 0) || 0,
          allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
          raw: p,
        });
      },
    );

    (Array.isArray(sortedPayRequestProducts)
      ? sortedPayRequestProducts
      : []
    ).forEach((p) => {
      rows.push({
        row_key: `pay_request-${p?.id}`,
        type: 'pay_request',
        id: Number(p?.id ?? 0) || 0,
        name: String(p?.name ?? ''),
        description: String(p?.description ?? ''),
        enabled: p?.archived === true ? false : p?.enabled !== false,
        archived: p?.archived === true,
        sort_order: Number(p?.sort_order ?? 0) || 0,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
        raw: p,
      });
    });

    (Array.isArray(sortedPayTokenProducts)
      ? sortedPayTokenProducts
      : []
    ).forEach((p) => {
      rows.push({
        row_key: `pay_token-${p?.id}`,
        type: 'pay_token',
        id: Number(p?.id ?? 0) || 0,
        name: String(p?.name ?? ''),
        description: String(p?.description ?? ''),
        enabled: p?.archived === true ? false : p?.enabled !== false,
        archived: p?.archived === true,
        sort_order: Number(p?.sort_order ?? 0) || 0,
        allowed_group_ids: normalizeGroupIds(p?.allowed_group_ids),
        raw: p,
      });
    });

    return rows.slice().sort((a, b) => {
      const sa = Number(a?.sort_order ?? 0) || 0;
      const sb = Number(b?.sort_order ?? 0) || 0;
      if (sa !== sb) return sb - sa;

      const ta = String(a?.type || '');
      const tb = String(b?.type || '');
      if (ta !== tb) return ta.localeCompare(tb);

      const ia = Number(a?.id ?? 0) || 0;
      const ib = Number(b?.id ?? 0) || 0;
      return ib - ia;
    });
  }, [
    sortedPaygProducts,
    sortedPayRequestProducts,
    sortedPayTokenProducts,
    subscriptionProducts,
  ]);

  const visibleProducts = useMemo(() => {
    const list = Array.isArray(mergedProducts) ? mergedProducts : [];
    if (!hideArchivedProducts) {
      return list;
    }
    return list.filter((product) => product?.archived !== true);
  }, [hideArchivedProducts, mergedProducts]);

  useEffect(() => {
    productRowsRef.current = cloneProductRows(mergedProducts);
  }, [mergedProducts]);

  useEffect(() => {
    productVisibleRowsRef.current = cloneProductRows(visibleProducts);
  }, [visibleProducts]);

  const applyProductSortOrders = useCallback(
    (orderedRows) => {
      const ordered = Array.isArray(orderedRows) ? orderedRows : [];
      const clonedOrdered = cloneProductRows(ordered);
      productRowsRef.current = clonedOrdered;
      productVisibleRowsRef.current = hideArchivedProducts
        ? clonedOrdered.filter((product) => product?.archived !== true)
        : clonedOrdered;

      const byKey = new Map(
        ordered.map((row) => [
          String(row?.row_key || ''),
          Number(row?.sort_order ?? 0) || 0,
        ]),
      );
      const patchSortOrder = (items, type) =>
        (Array.isArray(items) ? items : []).map((item) => {
          const key = `${type}-${item?.id}`;
          if (!byKey.has(key)) return item;
          return { ...item, sort_order: byKey.get(key) };
        });

      setSubscriptionProducts((prev) =>
        (Array.isArray(prev) ? prev : []).map((item) => {
          const mode = inferPresetMode(item);
          const key = `${mode}-${item?.id}`;
          if (!byKey.has(key)) return item;
          return { ...item, sort_order: byKey.get(key) };
        }),
      );
      setPaygProducts((prev) => patchSortOrder(prev, 'payg'));
      setPayRequestProducts((prev) => patchSortOrder(prev, 'pay_request'));
      setPayTokenProducts((prev) => patchSortOrder(prev, 'pay_token'));
      return ordered;
    },
    [hideArchivedProducts],
  );

  const setProductDropTarget = useCallback((targetKey) => {
    const nextKey = String(targetKey || '');
    if (productDropTargetKeyRef.current === nextKey) return;
    productDropTargetKeyRef.current = nextKey;
    setProductDropTargetKey(nextKey);
  }, []);

  const setProductRowRef = useCallback((rowKey, node) => {
    const key = String(rowKey || '');
    if (!key) return;
    if (node) {
      productRowRefs.current.set(key, node);
    } else {
      productRowRefs.current.delete(key);
    }
  }, []);

  const clearProductOverlayFrame = useCallback(() => {
    if (productDragOverlayFrameRef.current) {
      cancelAnimationFrame(productDragOverlayFrameRef.current);
      productDragOverlayFrameRef.current = 0;
    }
  }, []);

  const applyProductOverlayTransform = useCallback(() => {
    const overlayNode = productDragOverlayRef.current;
    const currentDrag = productDragStateRef.current;
    const pointer = productDragPointerRef.current;
    if (!overlayNode || !currentDrag || !pointer) return;

    const x = Math.round(pointer.x - currentDrag.offsetX);
    const y = Math.round(pointer.y - currentDrag.offsetY);
    overlayNode.style.transform = `translate3d(${x}px, ${y}px, 0) scale(1.01)`;
  }, []);

  const scheduleProductOverlayTransform = useCallback(() => {
    if (productDragOverlayFrameRef.current) return;
    productDragOverlayFrameRef.current = requestAnimationFrame(() => {
      productDragOverlayFrameRef.current = 0;
      applyProductOverlayTransform();
    });
  }, [applyProductOverlayTransform]);

  const clearProductDragState = useCallback(() => {
    clearProductOverlayFrame();
    if (productDragHandleRef.current && productDragStateRef.current) {
      try {
        productDragHandleRef.current.releasePointerCapture?.(
          productDragStateRef.current.pointerId,
        );
      } catch (_) {
        // Pointer capture may already have been released by the browser.
      }
    }
    productDragStateRef.current = null;
    productDragPointerRef.current = null;
    productDragInitialRowsRef.current = [];
    productDragInitialVisibleRowsRef.current = [];
    productDragHandleRef.current = null;
    setProductDropTarget('');
    setProductDragState(null);
  }, [clearProductOverlayFrame, setProductDropTarget]);

  const persistProductOrder = useCallback(
    async (orderedRows, previousRows) => {
      const ordered = Array.isArray(orderedRows) ? orderedRows : [];
      if (ordered.length === 0) return;
      setProductsReordering(true);
      try {
        const previousSortByKey = new Map(
          cloneProductRows(previousRows).map((row) => [
            productRowKeyOf(row),
            Number(row?.sort_order ?? 0) || 0,
          ]),
        );
        const hasSortChanged = (row) =>
          (previousSortByKey.get(productRowKeyOf(row)) ?? null) !==
          (Number(row?.sort_order ?? 0) || 0);

        const products = ordered.filter(hasSortChanged).map((row) => ({
          type: row?.type,
          id: row?.id,
          sort_order: Number(row?.sort_order ?? 0) || 0,
        }));
        if (products.length === 0) {
          return;
        }

        const res = await API.post(PRODUCT_MANAGEMENT_REORDER_API_BASE, {
          products,
        });
        const { success, message } = res.data;
        if (!success) {
          throw new Error(message || t('保存失败'));
        }

        showSuccess(t('排序已保存'));
      } catch (e) {
        if (Array.isArray(previousRows) && previousRows.length > 0) {
          applyProductSortOrders(previousRows);
        }
        const status = e?.response?.status;
        showError(
          status === 404
            ? t('排序接口未生效，请重启后端后重试')
            : e?.message || t('保存失败'),
        );
      } finally {
        setProductsReordering(false);
      }
    },
    [applyProductSortOrders, t],
  );

  useEffect(() => {
    persistProductOrderRef.current = persistProductOrder;
  }, [persistProductOrder]);

  const previewProductReorder = useCallback(
    (clientY) => {
      const activeDragState = productDragStateRef.current;
      if (!activeDragState) return;
      const visible = productDragInitialVisibleRowsRef.current;
      const others = visible.filter(
        (row) => productRowKeyOf(row) !== activeDragState.rowKey,
      );

      let targetKey = '';
      for (const row of others) {
        const key = productRowKeyOf(row);
        const node = findProductRowNode(productRowRefs, key);
        if (!node) continue;
        const rect = node.getBoundingClientRect();
        if (clientY < rect.top + rect.height / 2) {
          targetKey = key;
          break;
        }
      }

      setProductDropTarget(targetKey);
    },
    [setProductDropTarget],
  );

  const startProductPointerDrag = useCallback(
    (event, record) => {
      if (productTableBusy || productsReordering || event.button !== 0) return;
      const rowKey = String(record?.row_key || '');
      if (!rowKey) return;

      const node = findProductRowNode(productRowRefs, rowKey);
      if (!node) return;

      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.setPointerCapture?.(event.pointerId);

      const rect = node.getBoundingClientRect();
      const initialRows = cloneProductRows(productRowsRef.current);
      const initialVisibleRows = cloneProductRows(
        productVisibleRowsRef.current,
      );
      const nextDragState = {
        rowKey,
        pointerId: event.pointerId,
        width: rect.width,
        height: rect.height,
        offsetX: event.clientX - rect.left,
        offsetY: event.clientY - rect.top,
        record: cloneProductRows([record])[0],
      };

      productDragHandleRef.current = event.currentTarget;
      productDragInitialRowsRef.current = initialRows;
      productDragInitialVisibleRowsRef.current = initialVisibleRows;
      productDragStateRef.current = nextDragState;
      productDragPointerRef.current = { x: event.clientX, y: event.clientY };
      setProductDropTarget(rowKey);
      setProductDragState(nextDragState);
    },
    [productTableBusy, productsReordering, setProductDropTarget],
  );

  useEffect(() => {
    productDragStateRef.current = productDragState;
    if (productDragState) {
      scheduleProductOverlayTransform();
    }
  }, [productDragState, scheduleProductOverlayTransform]);

  useEffect(() => {
    if (!productDragState) return undefined;

    const previousUserSelect = document.body.style.userSelect;
    const previousCursor = document.body.style.cursor;
    document.body.style.userSelect = 'none';
    document.body.style.cursor = 'grabbing';

    const handlePointerMove = (event) => {
      const activeDragState = productDragStateRef.current;
      if (!activeDragState) return;
      if (
        activeDragState.pointerId !== undefined &&
        event.pointerId !== undefined &&
        activeDragState.pointerId !== event.pointerId
      ) {
        return;
      }

      productDragPointerRef.current = { x: event.clientX, y: event.clientY };
      scheduleProductOverlayTransform();
      previewProductReorder(event.clientY);
    };

    const finishDrag = (event) => {
      const activeDragState = productDragStateRef.current;
      if (
        activeDragState?.pointerId !== undefined &&
        event?.pointerId !== undefined &&
        activeDragState.pointerId !== event.pointerId
      ) {
        return;
      }

      const previousRows = cloneProductRows(productDragInitialRowsRef.current);
      const previousVisibleRows = cloneProductRows(
        productDragInitialVisibleRowsRef.current,
      );
      const dropTargetKey = productDropTargetKeyRef.current;
      const nextVisibleRows =
        dropTargetKey === activeDragState.rowKey
          ? previousVisibleRows
          : reorderProductRowsBefore(
              previousVisibleRows,
              activeDragState.rowKey,
              dropTargetKey,
            );
      const nextRows = withSequentialSortOrders(
        mergeReorderedVisibleProductRows(previousRows, nextVisibleRows),
      );
      const changed =
        !areProductRowOrdersEqual(previousRows, nextRows) ||
        !areProductRowSortOrdersEqual(previousRows, nextRows);
      clearProductDragState();
      if (changed) {
        applyProductSortOrders(nextRows);
        void persistProductOrderRef.current?.(nextRows, previousRows);
      }
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', finishDrag);
    window.addEventListener('pointercancel', finishDrag);

    return () => {
      document.body.style.userSelect = previousUserSelect;
      document.body.style.cursor = previousCursor;
      clearProductOverlayFrame();
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', finishDrag);
      window.removeEventListener('pointercancel', finishDrag);
    };
  }, [
    clearProductDragState,
    clearProductOverlayFrame,
    applyProductSortOrders,
    previewProductReorder,
    productDragState,
    scheduleProductOverlayTransform,
  ]);

  const getProductRowProps = useCallback(
    (record, index) => {
      const rowKey = String(record?.row_key || '');
      const isDragging = productDragState?.rowKey === rowKey;
      const isDropTarget =
        Boolean(productDragState) &&
        productDragState.rowKey !== rowKey &&
        productDropTargetKey === rowKey;
      const visibleRows = productDragState
        ? productDragInitialVisibleRowsRef.current
        : productVisibleRowsRef.current;
      const lastDropRowKey = visibleRows
        .filter((row) => productRowKeyOf(row) !== productDragState?.rowKey)
        .at(-1)?.row_key;
      const isDropEndTarget =
        Boolean(productDragState) &&
        productDropTargetKey === '' &&
        rowKey !== productDragState.rowKey &&
        String(lastDropRowKey || '') === rowKey;
      return {
        ref: (node) => setProductRowRef(rowKey, node),
        className: [
          'product-management-row',
          isDragging ? 'product-management-row--dragging' : '',
          isDropTarget ? 'product-management-row--drop-before' : '',
          isDropEndTarget ? 'product-management-row--drop-after' : '',
        ]
          .filter(Boolean)
          .join(' '),
        style: {
          transition:
            'box-shadow 140ms ease, background-color 140ms ease, opacity 140ms ease',
        },
        'data-product-row-key': rowKey,
        'data-product-row-index': index,
      };
    },
    [productDragState, productDropTargetKey, setProductRowRef],
  );

  const mergedProductColumns = useMemo(() => {
    return [
      {
        title: '',
        key: 'drag',
        width: 54,
        fixed: 'left',
        render: (_, record) => (
          <Tooltip content={t('拖拽排序')}>
            <span
              className={`inline-flex h-8 w-8 items-center justify-center rounded-md border border-transparent ${
                productTableBusy
                  ? 'cursor-not-allowed text-gray-300'
                  : 'cursor-grab touch-none text-gray-500 hover:border-gray-200 hover:bg-gray-50 active:cursor-grabbing'
              }`}
              onPointerDown={(event) => startProductPointerDrag(event, record)}
              onClick={(event) => event.stopPropagation()}
            >
              <GripVertical size={16} />
            </span>
          </Tooltip>
        ),
      },
      {
        title: t('类型'),
        key: 'type',
        width: 110,
        render: (_, record) => renderProductTypeTag(String(record?.type || '')),
      },
      {
        title: t('名称'),
        dataIndex: 'name',
        key: 'name',
        width: 220,
        render: (_, record) => (
          <div className='min-w-[180px] truncate font-medium'>
            {record?.name || '-'}
          </div>
        ),
      },
      {
        title: t('可用分组'),
        dataIndex: 'allowed_group_ids',
        key: 'allowed_group_ids',
        width: 360,
        render: (_, record) => {
          const groupIds = normalizeGroupIds(record?.allowed_group_ids);
          if (groupIds.length === 0) {
            return <Text type='danger'>{t('未配置')}</Text>;
          }
          return (
            <div className='flex min-w-[320px] max-w-[520px] flex-wrap items-center justify-start gap-1'>
              {groupIds.map((gid) => (
                <Text
                  key={`${record?.row_key}-${gid}`}
                  code
                  style={{ fontSize: 12, whiteSpace: 'nowrap' }}
                >
                  {groupLabelById[gid] || t('未知分组')}
                </Text>
              ))}
            </div>
          );
        },
      },
      {
        title: '',
        key: 'action',
        width: 360,
        fixed: 'right',
        render: (_, record) => {
          const type = String(record?.type || '');
          const isSubscriptionProduct = isSubscriptionProductType(type);
          const busy = productTableBusy;
          const archived = record?.archived === true;
          const saleEnabled = archived ? false : record?.enabled !== false;

          return (
            <div className='flex max-w-[360px] flex-wrap items-center justify-end gap-2'>
              <div className='flex shrink-0 items-center gap-2 rounded-md border border-gray-200 px-2 py-1'>
                <Switch
                  checked={saleEnabled}
                  size='small'
                  disabled={busy}
                  onChange={(v) => {
                    if (record?.type === 'payg') {
                      void togglePaygProductEnabled(record.raw, v);
                      return;
                    }
                    if (record?.type === 'pay_request') {
                      void togglePayRequestProductEnabled(record.raw, v);
                      return;
                    }
                    if (record?.type === 'pay_token') {
                      void togglePayTokenProductEnabled(record.raw, v);
                      return;
                    }
                    void updateSubscriptionProductEnabled(record.raw, v);
                  }}
                />
                <Text size='small' type='tertiary'>
                  {saleEnabled ? t('已上架') : t('下架')}
                </Text>
              </div>
              <ProductActionButton
                type={archived ? 'primary' : 'danger'}
                theme={archived ? 'solid' : 'light'}
                disabled={busy}
                onClick={() => {
                  if (record?.type === 'payg') {
                    void updatePaygProductArchived(record.raw, !archived);
                    return;
                  }
                  if (record?.type === 'pay_request') {
                    void updatePayRequestProductArchived(record.raw, !archived);
                    return;
                  }
                  if (record?.type === 'pay_token') {
                    void updatePayTokenProductArchived(record.raw, !archived);
                    return;
                  }
                  void updateSubscriptionProductArchived(record.raw, !archived);
                }}
              >
                {archived ? t('启用') : t('停用')}
              </ProductActionButton>
              {isSubscriptionProduct ? (
                <ProductActionButton
                  disabled={!isRootUser || subscriptionProductsLoading}
                  onClick={() => generateBySubscriptionProduct(record.raw)}
                >
                  {t('生成兑换码')}
                </ProductActionButton>
              ) : null}
              {isSubscriptionProduct ? (
                <ProductActionButton
                  disabled={!isRootUser || subscriptionProductsLoading}
                  onClick={() => void copyGenerateRedemptionCurl(record.raw)}
                >
                  {t('复制curl')}
                </ProductActionButton>
              ) : null}
              <ProductActionButton
                disabled={busy}
                onClick={() =>
                  void openProductRevisionHistory(type, record.raw)
                }
              >
                {t('历史')}
              </ProductActionButton>
              <ProductActionButton
                disabled={busy}
                onClick={() => void copyProduct(record)}
              >
                {t('复制')}
              </ProductActionButton>
              <ProductActionButton
                icon={<IconEdit />}
                disabled={busy}
                onClick={() => {
                  if (type === 'payg') {
                    openEditPaygProduct(record.raw);
                  } else if (type === 'pay_request') {
                    openEditPayRequestProduct(record.raw);
                  } else if (type === 'pay_token') {
                    openEditPayTokenProduct(record.raw);
                  } else {
                    openEditSubscriptionProduct(record.raw);
                  }
                }}
              >
                {t('编辑')}
              </ProductActionButton>
              <Popconfirm
                title={t('确定删除？')}
                onConfirm={() => {
                  if (type === 'payg') {
                    void deletePaygProduct(record?.id);
                  } else if (type === 'pay_request') {
                    void deletePayRequestProduct(record?.id);
                  } else if (type === 'pay_token') {
                    void deletePayTokenProduct(record?.id);
                  } else {
                    void deleteSubscriptionProduct(record?.id);
                  }
                }}
              >
                <ProductActionButton
                  type='danger'
                  icon={<IconDelete />}
                  disabled={busy}
                >
                  {t('删除')}
                </ProductActionButton>
              </Popconfirm>
            </div>
          );
        },
      },
    ];
  }, [
    copyGenerateRedemptionCurl,
    copyProduct,
    deletePaygProduct,
    deletePayRequestProduct,
    deletePayTokenProduct,
    deleteSubscriptionProduct,
    generateBySubscriptionProduct,
    groupLabelById,
    isRootUser,
    openEditPaygProduct,
    openEditPayRequestProduct,
    openEditPayTokenProduct,
    openEditSubscriptionProduct,
    openProductRevisionHistory,
    paygLoading,
    paygProductsSaving,
    payRequestProductsSaving,
    payTokenProductsSaving,
    productTableBusy,
    renderProductTypeTag,
    startProductPointerDrag,
    subscriptionProductsLoading,
    t,
    updatePayRequestProductArchived,
    updatePayTokenProductArchived,
    updatePaygProductArchived,
    updateSubscriptionProductArchived,
    togglePaygProductEnabled,
    togglePayRequestProductEnabled,
    togglePayTokenProductEnabled,
    updateSubscriptionProductEnabled,
  ]);

  const productDragOverlay =
    productDragState &&
    productDragState.record &&
    typeof document !== 'undefined'
      ? createPortal(
          <div
            ref={productDragOverlayRef}
            className='pointer-events-none fixed left-0 top-0 z-[9999] will-change-transform'
            style={{
              width: productDragState.width,
              maxWidth: 'min(960px, calc(100vw - 32px))',
            }}
          >
            <div className='rounded-xl border border-slate-200 bg-white px-4 py-3 shadow-[0_18px_42px_-24px_rgba(15,23,42,0.36)] dark:border-slate-700 dark:bg-[#313844]'>
              <div className='flex items-center gap-3'>
                <div className='flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-200'>
                  <GripVertical size={16} />
                </div>
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-sm font-medium text-slate-900 dark:text-slate-50'>
                    {productDragState.record.name || '-'}
                  </div>
                  <div className='mt-1 flex items-center gap-2'>
                    {renderProductTypeTag(
                      String(productDragState.record.type || ''),
                    )}
                    <span className='text-xs text-slate-400'>
                      {t('拖拽排序')}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>,
          document.body,
        )
      : null;

  const productManagementTableStyles = (
    <style>
      {`
        .product-management-table {
          width: 100%;
        }

        .product-management-table .semi-table,
        .product-management-table .semi-table-container,
        .product-management-table .semi-table-body {
          width: 100%;
        }

        .product-management-table .semi-table-body {
          overflow-x: auto;
        }

        .product-management-table .semi-table table {
          min-width: 1120px;
          width: 100%;
        }

        .product-management-row--dragging > td {
          background: color-mix(in srgb, var(--app-card-muted) 68%, transparent 32%) !important;
          opacity: 0.58;
        }

        .product-management-row--drop-before > td {
          box-shadow: inset 0 3px 0 rgba(14, 165, 233, 0.95);
        }

        .product-management-row--drop-after > td {
          box-shadow: inset 0 -3px 0 rgba(14, 165, 233, 0.95);
        }

        .product-management-row--drop-before > td:first-child,
        .product-management-row--drop-after > td:first-child {
          box-shadow: inherit;
        }

        html.dark .product-management-row--dragging > td {
          background: rgba(148, 163, 184, 0.08) !important;
        }

        @media (max-width: 767px) {
          .product-management-row--dragging {
            opacity: 0.58;
            background: color-mix(in srgb, var(--app-card-muted) 68%, transparent 32%) !important;
          }

          .product-management-row--drop-before {
            box-shadow: inset 0 3px 0 rgba(14, 165, 233, 0.95);
          }

          .product-management-row--drop-after {
            box-shadow: inset 0 -3px 0 rgba(14, 165, 233, 0.95);
          }
        }
      `}
    </style>
  );

  const createProductMenu = useMemo(() => {
    return (
      <Dropdown.Menu>
        <Dropdown.Item onClick={openCreateTokensProduct}>
          {t('Tokens订阅')}
        </Dropdown.Item>
        <Dropdown.Item onClick={openCreateSubscriptionProduct}>
          {t('额度订阅')}
        </Dropdown.Item>
        <Dropdown.Item onClick={openCreateRequestProduct}>
          {t('次数订阅')}
        </Dropdown.Item>
        <Dropdown.Item onClick={openCreatePaygProduct}>
          {t('按量付费')}
        </Dropdown.Item>
        <Dropdown.Item onClick={openCreatePayRequestProduct}>
          {t('按次付费')}
        </Dropdown.Item>
        <Dropdown.Item onClick={openCreatePayTokenProduct}>
          {t('按token付费')}
        </Dropdown.Item>
      </Dropdown.Menu>
    );
  }, [
    openCreatePaygProduct,
    openCreatePayRequestProduct,
    openCreatePayTokenProduct,
    openCreateRequestProduct,
    openCreateSubscriptionProduct,
    openCreateTokensProduct,
    t,
  ]);

  if (!actualRootUser && permissionsLoading) {
    return (
      <ConsolePage>
        <div className='app-surface-solid p-4 md:p-6'>
          <Spin spinning />
        </div>
      </ConsolePage>
    );
  }

  if (!isRootUser) {
    return <Navigate to='/forbidden' replace />;
  }

  return (
    <ConsolePage>
      {productManagementTableStyles}
      <div className='app-surface-solid p-4 md:p-6 space-y-4'>
        <div className='flex items-start justify-between gap-4'>
          <div className='min-w-0'>
            <Title heading={4} style={{ margin: 0 }}>
              {t('商品管理')}
            </Title>
            <Text type='tertiary' size='small'>
              {t(
                '可选择新增额度订阅（美元/次数/tokens）或按量付费（美元/次数/token）商品，并为商品配置可用分组',
              )}
            </Text>
          </div>
          <div className='shrink-0 flex flex-wrap items-center justify-end gap-2'>
            <div className='inline-flex items-center gap-2 rounded-xl border border-gray-200 px-3 py-2'>
              <Switch
                checked={hideArchivedProducts}
                disabled={
                  productManagementOptionsLoading ||
                  hideArchivedProductsSaving ||
                  productsReordering
                }
                onChange={updateHideArchivedProducts}
              />
              <Text size='small'>{t('隐藏停用商品')}</Text>
              <Text type='tertiary' size='small'>
                {t('显示 {{shown}} / {{total}}', {
                  shown: visibleProducts.length,
                  total: Array.isArray(mergedProducts)
                    ? mergedProducts.length
                    : 0,
                })}
              </Text>
            </div>
            {isRootUser ? (
              <Button
                type='tertiary'
                icon={<IconSetting />}
                disabled={paygLoading || paygSaving || productsReordering}
                onClick={() => setPaygSettingSheetVisible(true)}
              >
                {t('付费设置')}
              </Button>
            ) : null}
            <Dropdown
              trigger='click'
              position='bottomRight'
              render={createProductMenu}
            >
              <Button
                type='primary'
                theme='solid'
                icon={<IconPlus />}
                disabled={!isRootUser || productsReordering}
              >
                <span className='inline-flex items-center gap-1'>
                  {t('新增预置商品')}
                  <IconTreeTriangleDown size={14} />
                </span>
              </Button>
            </Dropdown>
            <Button
              onClick={() => {
                void loadProductManagementOptions();
                void loadGroups();
                void loadPaygOptions();
                void loadSubscriptionProducts();
              }}
              disabled={
                productManagementOptionsLoading ||
                hideArchivedProductsSaving ||
                paygLoading ||
                paygSaving ||
                paygProductsSaving ||
                payRequestProductsSaving ||
                payTokenProductsSaving ||
                subscriptionProductsLoading ||
                productsReordering
              }
            >
              {t('刷新')}
            </Button>
          </div>
        </div>

        <Card
          className='!rounded-2xl !border-0 !shadow-none'
          bodyStyle={{ padding: 20, width: '100%' }}
        >
          <CardTable
            columns={mergedProductColumns}
            dataSource={visibleProducts}
            loading={
              productManagementOptionsLoading ||
              paygLoading ||
              paygProductsSaving ||
              payRequestProductsSaving ||
              payTokenProductsSaving ||
              subscriptionProductsLoading ||
              productsReordering
            }
            rowKey='row_key'
            onRow={getProductRowProps}
            className='product-management-table w-full'
            style={{ width: '100%' }}
            scroll={{ x: 'max-content' }}
            hidePagination
          />
        </Card>
      </div>
      {productDragOverlay}

      <Modal
        title={
          revisionHistoryPreset?.name
            ? `${t('版本历史')} · ${revisionHistoryPreset.name}`
            : t('版本历史')
        }
        visible={revisionHistoryVisible}
        onCancel={closeSubscriptionRevisionHistory}
        footer={
          <Button onClick={closeSubscriptionRevisionHistory}>
            {t('关闭')}
          </Button>
        }
        width={820}
        centered
      >
        <Spin spinning={revisionHistoryLoading}>
          <div className='space-y-3'>
            <div className='text-sm text-gray-500'>
              {t('查看该商品每次保存生成的历史快照，并可选择恢复')}
            </div>
            {revisionHistoryItems.length === 0 ? (
              <div className='py-10 text-center text-sm text-gray-500'>
                {t('暂无版本记录')}
              </div>
            ) : (
              revisionHistoryItems.map((revision) => {
                const revisionId = Number(revision?.id ?? 0) || 0;
                const revisionType = inferPresetMode(revision);
                const meta = buildProductMetaLabels(revisionType, revision);
                const groupIds = normalizeGroupIds(revision?.allowed_group_ids);
                const groupDailyLimitLabels = buildGroupDailyLimitLabels(
                  revisionType,
                  revision,
                );
                return (
                  <Card
                    key={`preset-revision-${revisionId}`}
                    className='!rounded-2xl shadow-sm border-0'
                  >
                    <div className='flex items-start justify-between gap-3'>
                      <div className='min-w-0'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <Text strong>
                            {`${t('版本')} #${
                              Number(revision?.revision_no ?? 0) || '-'
                            }`}
                          </Text>
                          {renderProductTypeTag(
                            isSubscriptionProductType(
                              revisionHistoryProductType,
                            )
                              ? revisionType
                              : revisionHistoryProductType,
                          )}
                          {revision?.is_current ? (
                            <Tag color='green' shape='circle'>
                              {t('当前版本')}
                            </Tag>
                          ) : null}
                        </div>
                        <div className='mt-1 text-xs text-gray-500'>
                          {`${t('快照时间')}: ${formatTimestampLabel(
                            revision?.snapshot_time,
                          )}`}
                        </div>
                        <div className='mt-1 text-xs text-gray-500'>
                          {`${t('更新时间')}: ${formatTimestampLabel(
                            revision?.preset_updated_time,
                          )}`}
                        </div>
                      </div>
                      <Button
                        size='small'
                        theme='light'
                        type='primary'
                        disabled={
                          Boolean(revision?.is_current) ||
                          restoringRevisionId > 0
                        }
                        loading={restoringRevisionId === revisionId}
                        onClick={() => openRestoreRevisionDialog(revision)}
                      >
                        {t('恢复此版本')}
                      </Button>
                    </div>

                    {revision?.description ? (
                      <div className='mt-3 whitespace-pre-wrap text-sm leading-6 text-gray-600'>
                        {revision.description}
                      </div>
                    ) : null}

                    {meta.length > 0 ? (
                      <div className='mt-3 flex flex-wrap gap-1'>
                        {meta.map((item, index) => (
                          <Tag
                            key={`preset-revision-meta-${revisionId}-${index}`}
                            color='grey'
                            shape='circle'
                          >
                            {item}
                          </Tag>
                        ))}
                      </div>
                    ) : null}

                    {groupIds.length > 0 ? (
                      <div className='mt-3'>
                        <div className='mb-1 text-xs text-gray-500'>
                          {t('可用分组')}
                        </div>
                        <div className='flex flex-wrap gap-1'>
                          {groupIds.map((gid) => (
                            <Text
                              key={`preset-revision-group-${revisionId}-${gid}`}
                              code
                            >
                              {groupLabelById[gid] || t('未知分组')}
                            </Text>
                          ))}
                        </div>
                      </div>
                    ) : null}

                    {groupDailyLimitLabels.length > 0 ? (
                      <div className='mt-3'>
                        <div className='mb-1 text-xs text-gray-500'>
                          {t('分组日限额')}
                        </div>
                        <div className='flex flex-wrap gap-1'>
                          {groupDailyLimitLabels.map((item, index) => (
                            <Text
                              key={`preset-revision-daily-${revisionId}-${index}`}
                              code
                            >
                              {item}
                            </Text>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </Card>
                );
              })
            )}
          </div>
        </Spin>
      </Modal>

      <Modal
        title={t('恢复商品版本')}
        visible={restoreRevisionVisible}
        onCancel={closeRestoreRevisionDialog}
        onOk={handleRestoreSubscriptionRevision}
        confirmLoading={restoringRevisionId > 0}
        okText={t('恢复')}
        cancelText={t('取消')}
        okButtonProps={{
          disabled: !restoreRevisionTarget || restoringRevisionId > 0,
        }}
        centered
      >
        <div className='space-y-3'>
          <div className='text-sm text-gray-700'>
            {t('将当前商品恢复到版本 #{{revision}}', {
              revision: Number(restoreRevisionTarget?.revision_no ?? 0) || '-',
            })}
          </div>
          <div className='text-xs text-gray-500'>
            {isSubscriptionProductType(revisionHistoryProductType)
              ? t('恢复后会生成一个新的当前版本；默认仅影响后续售出商品')
              : t('恢复后会生成一个新的当前版本；默认仅影响后续购买')}
          </div>
          {isSubscriptionProductType(revisionHistoryProductType) ? (
            <div className='rounded-xl border border-gray-200 px-3 py-3'>
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='font-medium'>{t('同步调整已售商品')}</div>
                  <div className='mt-1 text-xs text-gray-500'>
                    {t(
                      '默认关闭；开启后会将本次商品变更同步到该商品已售出的订阅资产',
                    )}
                  </div>
                </div>
                <Switch
                  checked={restoreRevisionSyncSoldAssets}
                  disabled={restoringRevisionId > 0}
                  onChange={setRestoreRevisionSyncSoldAssets}
                />
              </div>
            </div>
          ) : null}
        </div>
      </Modal>

      <EditRedemptionPresetModal
        key={`${subscriptionEditVisible ? 'open' : 'closed'}-${
          editingSubscriptionProduct?.id ?? 'new'
        }-${editingSubscriptionProduct?.mode || 'subscription'}`}
        visible={subscriptionEditVisible}
        editingPreset={editingSubscriptionProduct}
        allowedModes={['subscription', 'tokens', 'request']}
        modeLocked={editingSubscriptionProduct?.mode}
        presetApiBase={PRODUCT_MANAGEMENT_PRESET_API_BASE}
        showSortOrderField={false}
        showMultiQuantityFields={false}
        onClose={closeSubscriptionEdit}
        onSuccess={handleSubscriptionProductSaved}
      />

      <SideSheet
        title={t('付费设置')}
        placement='right'
        visible={paygSettingSheetVisible}
        closeIcon={null}
        onCancel={() => setPaygSettingSheetVisible(false)}
        width={420}
        footer={
          <div className='flex justify-end gap-2'>
            <Button
              icon={<IconClose />}
              disabled={paygSaving}
              onClick={() => setPaygSettingSheetVisible(false)}
            >
              {t('取消')}
            </Button>
            <Button
              type='primary'
              icon={<IconSave />}
              loading={paygSaving}
              disabled={!isRootUser || paygSaving || paygLoading}
              onClick={savePaygOptions}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Spin spinning={paygLoading || paygSaving}>
          <div className='p-2'>
            <Form
              key={paygFormKey}
              layout='vertical'
              initValues={paygFormInitValues}
              getFormApi={(api) => (paygFormApiRef.current = api)}
            >
              <Form.InputNumber
                field='credit_usd_per_cny'
                label={t('按量付费兑换比例')}
                precision={4}
                min={0.0001}
                extraText={t('美元额度 = 人民币 × 兑换比例')}
                disabled={!isRootUser || paygSaving || paygLoading}
              />
              <Form.InputNumber
                field='credit_requests_per_cny'
                label={t('按次付费兑换比例')}
                precision={0}
                min={0}
                extraText={t('次数 = 人民币 × 兑换比例')}
                disabled={!isRootUser || paygSaving || paygLoading}
              />
              <Form.InputNumber
                field='credit_tokens_per_cny'
                label={t('按token付费兑换比例')}
                precision={0}
                min={0}
                extraText={t('tokens = 人民币 × 兑换比例')}
                disabled={!isRootUser || paygSaving || paygLoading}
              />
            </Form>
          </div>
        </Spin>
      </SideSheet>

      <SideSheet
        title={
          (Array.isArray(paygProducts) ? paygProducts : []).some(
            (p) => Number(p?.id ?? 0) === Number(editingPaygProduct?.id ?? 0),
          )
            ? t('编辑按量商品')
            : t('新增按量商品')
        }
        placement='right'
        visible={paygProductSheetVisible}
        closeIcon={null}
        onCancel={closePaygProductSheet}
        width={420}
        footer={
          <div className='flex justify-end gap-2'>
            <Button icon={<IconClose />} onClick={closePaygProductSheet}>
              {t('取消')}
            </Button>
            <Button
              type='primary'
              icon={<IconSave />}
              loading={paygProductsSaving}
              disabled={!isRootUser || paygProductsSaving}
              onClick={savePaygProduct}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Spin spinning={paygProductsSaving}>
          <div className='p-2'>
            {editingPaygProduct?.id ? (
              <Text type='tertiary' size='small'>
                {t('ID')}：{editingPaygProduct.id}
              </Text>
            ) : null}

            <div className='mt-2'>
              <Form
                key={editingPaygProduct?.id || 'new'}
                layout='vertical'
                initValues={{
                  name: editingPaygProduct?.name || '',
                  description: editingPaygProduct?.description || '',
                  enabled: editingPaygProduct?.enabled !== false,
                  archived: editingPaygProduct?.archived === true,
                  sort_order: Number(editingPaygProduct?.sort_order ?? 0) || 0,
                  stock: editingPaygProduct?.stock ?? null,
                  allowed_group_ids: normalizeGroupIds(
                    editingPaygProduct?.allowed_group_ids,
                  ),
                }}
                getFormApi={(api) => (paygProductFormApiRef.current = api)}
              >
                <Form.Input
                  field='name'
                  label={t('名称')}
                  placeholder={t('请输入名称')}
                  rules={[{ required: true, message: t('请输入名称') }]}
                  showClear
                  disabled={!isRootUser || paygProductsSaving}
                />
                <Form.TextArea
                  field='description'
                  label={t('描述')}
                  placeholder={t('可选：展示在订阅购买页按量商品卡片上')}
                  rows={3}
                  showClear
                  disabled={!isRootUser || paygProductsSaving}
                />
                <Form.InputNumber
                  field='stock'
                  label={t('库存')}
                  min={0}
                  precision={0}
                  step={1}
                  placeholder={t('留空表示无限制')}
                  extraText={t('留空表示无限制；0 表示售罄')}
                  disabled={!isRootUser || paygProductsSaving}
                />
                <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
                  <Form.Switch
                    field='enabled'
                    label={t('是否上架')}
                    disabled={!isRootUser || paygProductsSaving}
                  />
                  <Form.Switch
                    field='archived'
                    label={t('停用商品')}
                    disabled={!isRootUser || paygProductsSaving}
                    onChange={(checked) => {
                      if (checked) {
                        paygProductFormApiRef.current?.setValue(
                          'enabled',
                          false,
                        );
                      }
                    }}
                  />
                </div>
                <div className='mt-2 text-xs text-gray-500'>
                  {t('停用后会自动下架，且可通过“隐藏停用商品”从列表中收起')}
                </div>
                <Form.Select
                  field='allowed_group_ids'
                  label={t('可用分组')}
                  placeholder={t('请选择可用分组')}
                  optionList={groupIdOptions}
                  loading={groupsLoading}
                  multiple
                  search
                  disabled={!isRootUser || paygProductsSaving}
                  extraText={t('上架商品必须配置可用分组')}
                  style={{ width: '100%' }}
                />
              </Form>
            </div>
          </div>
        </Spin>
      </SideSheet>

      <SideSheet
        title={
          (Array.isArray(payRequestProducts) ? payRequestProducts : []).some(
            (p) =>
              Number(p?.id ?? 0) === Number(editingPayRequestProduct?.id ?? 0),
          )
            ? t('编辑按次商品')
            : t('新增按次商品')
        }
        placement='right'
        visible={payRequestProductSheetVisible}
        closeIcon={null}
        onCancel={closePayRequestProductSheet}
        width={420}
        footer={
          <div className='flex justify-end gap-2'>
            <Button icon={<IconClose />} onClick={closePayRequestProductSheet}>
              {t('取消')}
            </Button>
            <Button
              type='primary'
              icon={<IconSave />}
              loading={payRequestProductsSaving}
              disabled={!isRootUser || payRequestProductsSaving}
              onClick={savePayRequestProduct}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Spin spinning={payRequestProductsSaving}>
          <div className='p-2'>
            {editingPayRequestProduct?.id ? (
              <Text type='tertiary' size='small'>
                {t('ID')}：{editingPayRequestProduct.id}
              </Text>
            ) : null}

            <div className='mt-2'>
              <Form
                key={editingPayRequestProduct?.id || 'new'}
                layout='vertical'
                initValues={{
                  name: editingPayRequestProduct?.name || '',
                  description: editingPayRequestProduct?.description || '',
                  enabled: editingPayRequestProduct?.enabled !== false,
                  archived: editingPayRequestProduct?.archived === true,
                  sort_order:
                    Number(editingPayRequestProduct?.sort_order ?? 0) || 0,
                  stock: editingPayRequestProduct?.stock ?? null,
                  allowed_group_ids: normalizeGroupIds(
                    editingPayRequestProduct?.allowed_group_ids,
                  ),
                }}
                getFormApi={(api) =>
                  (payRequestProductFormApiRef.current = api)
                }
              >
                <Form.Input
                  field='name'
                  label={t('名称')}
                  placeholder={t('请输入名称')}
                  rules={[{ required: true, message: t('请输入名称') }]}
                  showClear
                  disabled={!isRootUser || payRequestProductsSaving}
                />
                <Form.TextArea
                  field='description'
                  label={t('描述')}
                  placeholder={t('可选：展示在订阅购买页按次商品卡片上')}
                  rows={3}
                  showClear
                  disabled={!isRootUser || payRequestProductsSaving}
                />
                <Form.InputNumber
                  field='stock'
                  label={t('库存')}
                  min={0}
                  precision={0}
                  step={1}
                  placeholder={t('留空表示无限制')}
                  extraText={t('留空表示无限制；0 表示售罄')}
                  disabled={!isRootUser || payRequestProductsSaving}
                />
                <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
                  <Form.Switch
                    field='enabled'
                    label={t('是否上架')}
                    disabled={!isRootUser || payRequestProductsSaving}
                  />
                  <Form.Switch
                    field='archived'
                    label={t('停用商品')}
                    disabled={!isRootUser || payRequestProductsSaving}
                    onChange={(checked) => {
                      if (checked) {
                        payRequestProductFormApiRef.current?.setValue(
                          'enabled',
                          false,
                        );
                      }
                    }}
                  />
                </div>
                <div className='mt-2 text-xs text-gray-500'>
                  {t('停用后会自动下架，且可通过“隐藏停用商品”从列表中收起')}
                </div>
                <Form.Select
                  field='allowed_group_ids'
                  label={t('可用分组')}
                  placeholder={t('请选择可用分组')}
                  optionList={groupIdOptions}
                  loading={groupsLoading}
                  multiple
                  search
                  disabled={!isRootUser || payRequestProductsSaving}
                  extraText={t('上架商品必须配置可用分组')}
                  style={{ width: '100%' }}
                />
              </Form>
            </div>
          </div>
        </Spin>
      </SideSheet>

      <SideSheet
        title={
          (Array.isArray(payTokenProducts) ? payTokenProducts : []).some(
            (p) =>
              Number(p?.id ?? 0) === Number(editingPayTokenProduct?.id ?? 0),
          )
            ? t('编辑按token商品')
            : t('新增按token商品')
        }
        placement='right'
        visible={payTokenProductSheetVisible}
        closeIcon={null}
        onCancel={closePayTokenProductSheet}
        width={420}
        footer={
          <div className='flex justify-end gap-2'>
            <Button icon={<IconClose />} onClick={closePayTokenProductSheet}>
              {t('取消')}
            </Button>
            <Button
              type='primary'
              icon={<IconSave />}
              loading={payTokenProductsSaving}
              disabled={!isRootUser || payTokenProductsSaving}
              onClick={savePayTokenProduct}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Spin spinning={payTokenProductsSaving}>
          <div className='p-2'>
            {editingPayTokenProduct?.id ? (
              <Text type='tertiary' size='small'>
                {t('ID')}：{editingPayTokenProduct.id}
              </Text>
            ) : null}

            <div className='mt-2'>
              <Form
                key={editingPayTokenProduct?.id || 'new'}
                layout='vertical'
                initValues={{
                  name: editingPayTokenProduct?.name || '',
                  description: editingPayTokenProduct?.description || '',
                  enabled: editingPayTokenProduct?.enabled !== false,
                  archived: editingPayTokenProduct?.archived === true,
                  sort_order:
                    Number(editingPayTokenProduct?.sort_order ?? 0) || 0,
                  stock: editingPayTokenProduct?.stock ?? null,
                  allowed_group_ids: normalizeGroupIds(
                    editingPayTokenProduct?.allowed_group_ids,
                  ),
                }}
                getFormApi={(api) => (payTokenProductFormApiRef.current = api)}
              >
                <Form.Input
                  field='name'
                  label={t('名称')}
                  placeholder={t('请输入名称')}
                  rules={[{ required: true, message: t('请输入名称') }]}
                  showClear
                  disabled={!isRootUser || payTokenProductsSaving}
                />
                <Form.TextArea
                  field='description'
                  label={t('描述')}
                  placeholder={t('可选：展示在订阅购买页按token商品卡片上')}
                  rows={3}
                  showClear
                  disabled={!isRootUser || payTokenProductsSaving}
                />
                <Form.InputNumber
                  field='stock'
                  label={t('库存')}
                  min={0}
                  precision={0}
                  step={1}
                  placeholder={t('留空表示无限制')}
                  extraText={t('留空表示无限制；0 表示售罄')}
                  disabled={!isRootUser || payTokenProductsSaving}
                />
                <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
                  <Form.Switch
                    field='enabled'
                    label={t('是否上架')}
                    disabled={!isRootUser || payTokenProductsSaving}
                  />
                  <Form.Switch
                    field='archived'
                    label={t('停用商品')}
                    disabled={!isRootUser || payTokenProductsSaving}
                    onChange={(checked) => {
                      if (checked) {
                        payTokenProductFormApiRef.current?.setValue(
                          'enabled',
                          false,
                        );
                      }
                    }}
                  />
                </div>
                <div className='mt-2 text-xs text-gray-500'>
                  {t('停用后会自动下架，且可通过“隐藏停用商品”从列表中收起')}
                </div>
                <Form.Select
                  field='allowed_group_ids'
                  label={t('可用分组')}
                  placeholder={t('请选择可用分组')}
                  optionList={groupIdOptions}
                  loading={groupsLoading}
                  multiple
                  search
                  disabled={!isRootUser || payTokenProductsSaving}
                  extraText={t('上架商品必须配置可用分组')}
                  style={{ width: '100%' }}
                />
              </Form>
            </div>
          </div>
        </Spin>
      </SideSheet>
    </ConsolePage>
  );
};

export default ProductManagement;
