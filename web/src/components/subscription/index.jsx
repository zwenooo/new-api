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
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  API,
  inferPresetMode,
  renderCnyFen,
  renderQuotaToUSD,
  showError,
  showSuccess,
} from '../../helpers';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router-dom';
import {
  Button,
  Card,
  InputNumber,
  Modal,
  Radio,
  Spin,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import { LayoutGrid, List } from 'lucide-react';
import ConsolePage from '../layout/ConsolePage';
import { UserContext } from '../../context/User';

const { Text, Title } = Typography;

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

const isOutOfStock = (rawStock) => {
  const stock = normalizeStockValue(rawStock);
  return typeof stock === 'number' && stock <= 0;
};

const normalizeNonNegativeNumberValue = (raw) => {
  const num = Number(raw);
  if (!Number.isFinite(num) || num < 0) {
    return 0;
  }
  return num;
};

const normalizeGroupDailyLimits = (rawLimits) => {
  const list = Array.isArray(rawLimits) ? rawLimits : [];
  const seen = new Set();
  const out = [];
  list.forEach((item) => {
    const rawId = Number(item?.group_id ?? 0);
    const groupId = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
    if (groupId <= 0) return;
    if (seen.has(groupId)) return;
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

const normalizePaygProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const seen = new Set();
  const out = [];
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const enabled = item?.enabled !== false;

    const sortOrderRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortOrderRaw)
      ? Math.max(0, Math.floor(sortOrderRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);
    if (allowedGroupIds.length === 0) return;

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const normalizePayRequestProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const seen = new Set();
  const out = [];
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const enabled = item?.enabled !== false;

    const sortOrderRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortOrderRaw)
      ? Math.max(0, Math.floor(sortOrderRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);
    if (allowedGroupIds.length === 0) return;

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const normalizePayTokenProducts = (rawProducts) => {
  if (!Array.isArray(rawProducts)) return [];
  const seen = new Set();
  const out = [];
  rawProducts.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0) return;
    if (seen.has(id)) return;

    const name = String(item?.name ?? '').trim();
    if (!name) return;

    const description = String(item?.description ?? '').trim();
    const enabled = item?.enabled !== false;

    const sortOrderRaw = Number(item?.sort_order ?? 0);
    const sortOrder = Number.isFinite(sortOrderRaw)
      ? Math.max(0, Math.floor(sortOrderRaw))
      : 0;

    const stock = normalizeStockValue(item?.stock);
    if (stock === undefined) return;

    const allowedGroupIds = normalizeGroupIds(item?.allowed_group_ids);
    if (allowedGroupIds.length === 0) return;

    seen.add(id);
    out.push({
      id,
      name,
      description,
      enabled,
      sort_order: sortOrder,
      stock,
      allowed_group_ids: allowedGroupIds,
    });
  });
  return out;
};

const submitEpayForm = (url, params) => {
  if (!url || !params) return;
  const form = document.createElement('form');
  form.action = url;
  form.method = 'POST';
  const isSafari =
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1;
  if (!isSafari) {
    form.target = '_blank';
  }
  Object.keys(params).forEach((key) => {
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = key;
    input.value = params[key];
    form.appendChild(input);
  });
  document.body.appendChild(form);
  form.submit();
  document.body.removeChild(form);
};

const openEpayPayPage = (url) => {
  const target = String(url || '').trim();
  if (!target) return;
  const isSafari =
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1;
  if (isSafari) {
    window.location.assign(target);
    return;
  }
  const nextWindow = window.open(target, '_blank', 'noopener,noreferrer');
  if (!nextWindow) {
    window.location.assign(target);
  }
};

const getEpayScanTip = (methodType, t) => {
  if (methodType === 'alipay') return t('请使用支付宝扫码完成支付');
  if (methodType === 'wxpay') return t('请使用微信扫码完成支付');
  return t('请使用手机扫码完成支付');
};

const normalizeEpayMethods = (rawMethods) => {
  const methods = Array.isArray(rawMethods) ? rawMethods : [];
  return methods
    .filter((m) => m && typeof m === 'object')
    .map((m) => ({
      ...m,
      type: String(m.type || '').trim(),
      name: String(m.name || '').trim(),
    }))
    .filter((m) => m.type && m.type !== 'stripe' && m.type !== 'custom');
};

const getDefaultEpayMethod = (methods) => {
  if (!Array.isArray(methods) || methods.length === 0) return '';
  const alipay = methods.find((m) => m?.type === 'alipay');
  return alipay?.type || methods[0].type;
};

const escapeHtmlText = (rawText) =>
  String(rawText || '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');

const isProbablyHtml = (raw) => /<\/?[a-z][\s\S]*>/i.test(String(raw || ''));

const sanitizeInlineStyle = (rawStyle) => {
  const styleText = String(rawStyle || '').trim();
  if (!styleText) return '';

  const allowedProps = new Set([
    'color',
    'background-color',
    'font-size',
    'font-weight',
    'font-style',
    'text-decoration',
  ]);

  const out = [];
  styleText.split(';').forEach((decl) => {
    const idx = decl.indexOf(':');
    if (idx === -1) return;
    const prop = decl.slice(0, idx).trim().toLowerCase();
    const value = decl.slice(idx + 1).trim();
    if (!prop || !value) return;
    if (!allowedProps.has(prop)) return;

    const valueLower = value.toLowerCase();
    if (
      valueLower.includes('expression') ||
      valueLower.includes('url(') ||
      valueLower.includes('@import')
    ) {
      return;
    }

    if (typeof CSS !== 'undefined' && typeof CSS.supports === 'function') {
      try {
        if (!CSS.supports(prop, value)) return;
      } catch {
        return;
      }
    } else if (prop === 'font-weight') {
      if (!/^(normal|bold|bolder|lighter|[1-9]00)$/i.test(value)) return;
    }

    out.push(`${prop}: ${value}`);
  });

  return out.join('; ');
};

const sanitizeSubscriptionStoreNoticeHtml = (unsafeHtml) => {
  if (typeof DOMParser === 'undefined') return '';

  const allowedTags = new Set([
    'A',
    'B',
    'BR',
    'CODE',
    'DIV',
    'EM',
    'I',
    'LI',
    'OL',
    'P',
    'SPAN',
    'STRONG',
    'U',
    'UL',
  ]);
  const dropTags = new Set([
    'SCRIPT',
    'STYLE',
    'IFRAME',
    'OBJECT',
    'EMBED',
    'LINK',
    'META',
  ]);
  const allowedGlobalAttrs = new Set(['class']);
  const allowedAttrsByTag = {
    A: new Set(['href', 'target', 'rel', 'class']),
  };

  const normalizeAnchor = (el) => {
    const rawHref = String(el.getAttribute('href') || '').trim();
    const hrefLower = rawHref.toLowerCase();
    const hrefUnsafe =
      hrefLower.startsWith('javascript:') ||
      hrefLower.startsWith('vbscript:') ||
      hrefLower.startsWith('data:');
    if (!rawHref || hrefUnsafe) {
      el.removeAttribute('href');
    }

    const target = String(el.getAttribute('target') || '').trim();
    if (target && target !== '_blank' && target !== '_self') {
      el.removeAttribute('target');
    }

    if (String(el.getAttribute('target') || '').trim() === '_blank') {
      const rel = String(el.getAttribute('rel') || '');
      const parts = new Set(rel.split(/\s+/).filter(Boolean));
      parts.add('noopener');
      parts.add('noreferrer');
      el.setAttribute('rel', [...parts].join(' '));
    }
  };

  const unwrapNode = (el) => {
    const parent = el.parentNode;
    if (!parent) return;
    while (el.firstChild) parent.insertBefore(el.firstChild, el);
    parent.removeChild(el);
  };

  const parser = new DOMParser();
  const doc = parser.parseFromString(String(unsafeHtml || ''), 'text/html');
  const elements = Array.from(doc.body.querySelectorAll('*'));

  elements.forEach((el) => {
    if (!el.parentNode) return;

    const tag = String(el.tagName || '').toUpperCase();
    if (!tag) return;

    if (dropTags.has(tag)) {
      el.remove();
      return;
    }

    if (!allowedTags.has(tag)) {
      unwrapNode(el);
      return;
    }

    const allowedAttrs = allowedAttrsByTag[tag] || null;
    Array.from(el.attributes || []).forEach((attr) => {
      const name = String(attr?.name || '').toLowerCase();
      if (!name) return;
      if (name.startsWith('on')) {
        el.removeAttribute(name);
        return;
      }
      if (name === 'style') {
        const safeStyle = sanitizeInlineStyle(attr?.value);
        if (safeStyle) {
          el.setAttribute('style', safeStyle);
        } else {
          el.removeAttribute('style');
        }
        return;
      }
      if (allowedAttrs) {
        if (!allowedAttrs.has(name)) el.removeAttribute(name);
        return;
      }
      if (!allowedGlobalAttrs.has(name)) el.removeAttribute(name);
    });

    if (tag === 'A') normalizeAnchor(el);
  });

  return doc.body.innerHTML;
};

const Subscription = () => {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const [userState, userDispatch] = useContext(UserContext);
  const payInitRef = useRef(false);
  const epayPaidTipRef = useRef(false);
  const paygPaidTipRef = useRef(false);
  const payRequestPaidTipRef = useRef(false);
  const payTokenPaidTipRef = useRef(false);
  const paygAutoOpenRef = useRef(false);

  const [plansLoading, setPlansLoading] = useState(false);
  const [plans, setPlans] = useState([]);

  const [payConfigLoading, setPayConfigLoading] = useState(false);
  const [epayEnabled, setEpayEnabled] = useState(false);
  const [epayMethods, setEpayMethods] = useState([]);

  const [subscriptionCheckoutMode, setSubscriptionCheckoutMode] =
    useState('payment');
  const [subscriptionTrafficMessage, setSubscriptionTrafficMessage] =
    useState('');
  const [subscriptionTrafficQRCode, setSubscriptionTrafficQRCode] =
    useState('');
  const [subscriptionStoreNotice, setSubscriptionStoreNotice] = useState('');

  const [orderModalOpen, setOrderModalOpen] = useState(false);
  const [orderingPlan, setOrderingPlan] = useState(null);
  const [orderQuantity, setOrderQuantity] = useState(1);
  const [applyMode, setApplyMode] = useState('stack');
  const [payMethod, setPayMethod] = useState('balance');
  const [epayMethod, setEpayMethod] = useState('');
  const [epayCheckout, setEpayCheckout] = useState(null);
  const [epayPayStatus, setEpayPayStatus] = useState('pending');
  const [orderSubmitting, setOrderSubmitting] = useState(false);

  const [paygProducts, setPaygProducts] = useState([]);
  const [selectedPaygProduct, setSelectedPaygProduct] = useState(null);
  const [paygCreditUsdPerCny, setPaygCreditUsdPerCny] = useState(0);
  const [paygModalOpen, setPaygModalOpen] = useState(false);
  const [paygAmountYuan, setPaygAmountYuan] = useState(1);
  const [paygPayMethod, setPaygPayMethod] = useState('balance');
  const [paygEpayMethod, setPaygEpayMethod] = useState('');
  const [paygCheckout, setPaygCheckout] = useState(null);
  const [paygPayStatus, setPaygPayStatus] = useState('pending');
  const [paygSubmitting, setPaygSubmitting] = useState(false);

  const [payRequestProducts, setPayRequestProducts] = useState([]);
  const [selectedPayRequestProduct, setSelectedPayRequestProduct] =
    useState(null);
  const [payRequestCreditRequestsPerCny, setPayRequestCreditRequestsPerCny] =
    useState(0);
  const [payRequestModalOpen, setPayRequestModalOpen] = useState(false);
  const [payRequestAmountYuan, setPayRequestAmountYuan] = useState(1);
  const [payRequestPayMethod, setPayRequestPayMethod] = useState('balance');
  const [payRequestEpayMethod, setPayRequestEpayMethod] = useState('');
  const [payRequestCheckout, setPayRequestCheckout] = useState(null);
  const [payRequestPayStatus, setPayRequestPayStatus] = useState('pending');
  const [payRequestSubmitting, setPayRequestSubmitting] = useState(false);

  const [payTokenProducts, setPayTokenProducts] = useState([]);
  const [selectedPayTokenProduct, setSelectedPayTokenProduct] = useState(null);
  const [payTokenCreditTokensPerCny, setPayTokenCreditTokensPerCny] =
    useState(0);
  const [payTokenModalOpen, setPayTokenModalOpen] = useState(false);
  const [payTokenAmountYuan, setPayTokenAmountYuan] = useState(1);
  const [payTokenPayMethod, setPayTokenPayMethod] = useState('balance');
  const [payTokenEpayMethod, setPayTokenEpayMethod] = useState('');
  const [payTokenCheckout, setPayTokenCheckout] = useState(null);
  const [payTokenPayStatus, setPayTokenPayStatus] = useState('pending');
  const [payTokenSubmitting, setPayTokenSubmitting] = useState(false);
  const [viewMode, setViewMode] = useState('list');

  const balanceFen = userState?.user?.balance_fen || 0;
  const [groupRatios, setGroupRatios] = useState({});
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);
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
  const userSelectableGroupIdSet = useMemo(() => {
    const ids = new Set();
    (Array.isArray(availableGroups) ? availableGroups : []).forEach((g) => {
      const rawId = Number(g?.id ?? 0);
      const id = Number.isFinite(rawId) ? Math.floor(rawId) : 0;
      if (id <= 0) return;
      if (g?.user_selectable !== true) return;
      ids.add(id);
    });
    return ids;
  }, [availableGroups]);
  const getGroupLabel = useCallback(
    (rawId) => {
      const idRaw = Number(rawId ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return t('未知分组');
      return groupLabelById?.[id] || t('未知分组');
    },
    [groupLabelById, t],
  );
  const getVisibleOptionalGroupIds = useCallback(
    (rawIds) =>
      normalizeGroupIds(rawIds).filter((gid) =>
        userSelectableGroupIdSet.has(gid),
      ),
    [userSelectableGroupIdSet],
  );
  const shouldAddSpaceBeforeView = String(i18n?.language || '')
    .toLowerCase()
    .startsWith('en');
  const subscriptionStoreNoticeIsHtml = useMemo(() => {
    const trimmed = String(subscriptionStoreNotice || '').trim();
    if (!trimmed) return false;
    return isProbablyHtml(trimmed);
  }, [subscriptionStoreNotice]);
  const subscriptionStoreNoticeLines = useMemo(() => {
    const trimmed = String(subscriptionStoreNotice || '').trim();
    if (!trimmed) return [];
    if (isProbablyHtml(trimmed)) return [];
    return trimmed.split(/\r?\n/);
  }, [subscriptionStoreNotice]);
  const subscriptionStoreNoticeHtml = useMemo(() => {
    if (!subscriptionStoreNoticeIsHtml) return '';
    const trimmed = String(subscriptionStoreNotice || '').trim();
    if (!trimmed) return '';
    const pricingLinkHtml = `<a href="/console/pricing" class="text-white underline decoration-white underline-offset-2 hover:opacity-90">${escapeHtmlText(t('模型广场'))}</a>`;
    const replaced = trimmed.split('{{pricing}}').join(pricingLinkHtml);
    return sanitizeSubscriptionStoreNoticeHtml(replaced);
  }, [subscriptionStoreNotice, subscriptionStoreNoticeIsHtml, t]);

  const closeOrderModal = () => {
    setOrderModalOpen(false);
    setEpayCheckout(null);
    setEpayPayStatus('pending');
    setOrderQuantity(1);
    epayPaidTipRef.current = false;
  };

  const renderStoreNoticeLine = useCallback(
    (line) => {
      const parts = String(line || '').split('{{pricing}}');
      if (parts.length <= 1) return line;
      return parts.map((part, index) => (
        <React.Fragment key={`store-notice-${index}`}>
          {part}
          {index < parts.length - 1 ? (
            <Link
              to='/console/pricing'
              className='text-white underline decoration-white underline-offset-2 hover:opacity-90'
            >
              {t('模型广场')}
            </Link>
          ) : null}
        </React.Fragment>
      ));
    },
    [t],
  );

  const loadPlans = async () => {
    setPlansLoading(true);
    try {
      const res = await API.get('/api/subscription/plans');
      const { success, message, data } = res.data;
      if (success) {
        setPlans(Array.isArray(data) ? data : []);
      } else {
        showError(message || t('获取套餐失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取套餐失败'));
    } finally {
      setPlansLoading(false);
    }
  };

  const loadPayConfig = async () => {
    setPayConfigLoading(true);
    try {
      const res = await API.get('/api/user/topup/info');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取支付配置失败'));
        return;
      }
      const enabled = Boolean(data?.enable_online_topup);
      setEpayEnabled(enabled);

      const checkoutMode = String(
        data?.subscription_checkout_mode || 'payment',
      ).trim();
      setSubscriptionCheckoutMode(checkoutMode);
      setSubscriptionTrafficMessage(
        String(data?.subscription_traffic_message || ''),
      );
      setSubscriptionTrafficQRCode(
        String(data?.subscription_traffic_qrcode || ''),
      );
      setSubscriptionStoreNotice(String(data?.subscription_store_notice || ''));
      setPaygProducts(normalizePaygProducts(data?.payg_products));
      const paygRate = Number(data?.payg_credit_usd_per_cny ?? 0);
      setPaygCreditUsdPerCny(
        Number.isFinite(paygRate) && paygRate > 0 ? paygRate : 0,
      );
      setPayRequestProducts(
        normalizePayRequestProducts(data?.pay_request_products),
      );
      const payRequestRate = Number(
        data?.pay_request_credit_requests_per_cny ?? 0,
      );
      setPayRequestCreditRequestsPerCny(
        Number.isFinite(payRequestRate) && payRequestRate > 0
          ? payRequestRate
          : 0,
      );

      setPayTokenProducts(normalizePayTokenProducts(data?.pay_token_products));
      const payTokenRate = Number(data?.pay_token_credit_tokens_per_cny ?? 0);
      setPayTokenCreditTokensPerCny(
        Number.isFinite(payTokenRate) && payTokenRate > 0 ? payTokenRate : 0,
      );

      let methods = data?.pay_methods || [];
      try {
        if (typeof methods === 'string') {
          methods = JSON.parse(methods);
        }
      } catch {
        methods = [];
      }
      methods = normalizeEpayMethods(methods);
      setEpayMethods(methods);
      if (methods.length > 0) {
        setEpayMethod((prev) => prev || getDefaultEpayMethod(methods));
      }
    } catch (e) {
      showError(e?.message || t('获取支付配置失败'));
    } finally {
      setPayConfigLoading(false);
    }
  };

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

  const loadAvailableGroups = useCallback(async () => {
    if (groupsLoading) return true;
    setGroupsLoading(true);
    try {
      const res = await API.get('/api/group/resolve');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        return false;
      }
      setAvailableGroups(Array.isArray(data) ? data : []);
      return true;
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
      return false;
    } finally {
      setGroupsLoading(false);
    }
  }, [groupsLoading, t]);

  const refreshUser = useCallback(async () => {
    try {
      const res = await API.get('/api/user/self');
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
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

  const refreshViewerPricingState = useCallback(async () => {
    await Promise.all([
      refreshUser(),
      loadGroupRatios(),
      loadAvailableGroups(),
    ]);
  }, [loadAvailableGroups, loadGroupRatios, refreshUser]);

  const openOrderModal = (plan) => {
    if (isOutOfStock(plan?.stock)) {
      showError(t('已售罄'));
      return;
    }
    setOrderingPlan(plan || null);
    setOrderQuantity(1);
    setApplyMode(plan?.multi_quantity_defer_only !== false ? 'defer' : 'stack');
    setEpayCheckout(null);
    payInitRef.current = false;
    setEpayPayStatus('pending');
    epayPaidTipRef.current = false;

    if (subscriptionCheckoutMode === 'traffic') {
      setOrderModalOpen(true);
      return;
    }

    if (epayEnabled && epayMethods.length > 0) {
      setPayMethod('epay');
      setEpayMethod(getDefaultEpayMethod(epayMethods));
    } else {
      setPayMethod('balance');
      setEpayMethod('');
    }
    setOrderModalOpen(true);
  };

  useEffect(() => {
    if (!orderModalOpen || subscriptionCheckoutMode === 'traffic') {
      return;
    }
    if (payConfigLoading) {
      return;
    }
    if (payInitRef.current) {
      return;
    }

    if (epayEnabled && epayMethods.length > 0) {
      setPayMethod('epay');
      setEpayMethod(getDefaultEpayMethod(epayMethods));
    } else {
      setPayMethod('balance');
      setEpayMethod('');
    }

    payInitRef.current = true;
  }, [
    orderModalOpen,
    subscriptionCheckoutMode,
    payConfigLoading,
    epayEnabled,
    epayMethods,
  ]);

  const buildEpayPayLink = (url, params) => {
    try {
      const u = new URL(url);
      Object.entries(params || {}).forEach(([key, value]) => {
        if (!key) return;
        if (value === undefined || value === null) return;
        u.searchParams.set(key, String(value));
      });
      return u.toString();
    } catch {
      return '';
    }
  };

  const buildGatewayCheckoutState = (data, method) => ({
    tradeNo: String(data?.trade_no || '').trim(),
    payPageUrl: String(
      data?.pay_page_url || data?.checkout_url || data?.payurl || '',
    ).trim(),
    qrCode: String(data?.qr_code || data?.qrcode || '').trim(),
    qrImageUrl: String(
      data?.qr_image_url || data?.qrcode_img || data?.img || '',
    ).trim(),
    method,
  });

  const formatUsd = (usdValue, digits = 2) => {
    const num = Number(usdValue);
    if (!Number.isFinite(num) || num <= 0) return '$0';
    return `$${num.toFixed(digits).replace(/\.?0+$/, '')}`;
  };

  const paygAmountFen = useMemo(() => {
    const num = Number(paygAmountYuan);
    if (!Number.isFinite(num) || num <= 0) return 0;
    return Math.round(num * 100);
  }, [paygAmountYuan]);

  const paygCreditUsd = useMemo(() => {
    const amount = Number(paygAmountYuan);
    if (!Number.isFinite(amount) || amount <= 0) return 0;
    const rate = Number(paygCreditUsdPerCny);
    if (!Number.isFinite(rate) || rate <= 0) return 0;
    return amount * rate;
  }, [paygAmountYuan, paygCreditUsdPerCny]);

  const payRequestAmountFen = useMemo(() => {
    const num = Number(payRequestAmountYuan);
    if (!Number.isFinite(num) || num <= 0) return 0;
    return Math.round(num * 100);
  }, [payRequestAmountYuan]);

  const payRequestCreditRequests = useMemo(() => {
    const amountFen = Number(payRequestAmountFen);
    if (!Number.isFinite(amountFen) || amountFen <= 0) return 0;
    const rateRaw = Number(payRequestCreditRequestsPerCny);
    const rate = Number.isFinite(rateRaw) ? Math.floor(rateRaw) : 0;
    if (rate <= 0) return 0;
    return Math.floor((amountFen * rate) / 100);
  }, [payRequestAmountFen, payRequestCreditRequestsPerCny]);

  const payTokenAmountFen = useMemo(() => {
    const num = Number(payTokenAmountYuan);
    if (!Number.isFinite(num) || num <= 0) return 0;
    return Math.round(num * 100);
  }, [payTokenAmountYuan]);

  const payTokenCreditTokens = useMemo(() => {
    const amountFen = Number(payTokenAmountFen);
    if (!Number.isFinite(amountFen) || amountFen <= 0) return 0;
    const rateRaw = Number(payTokenCreditTokensPerCny);
    const rate = Number.isFinite(rateRaw) ? Math.floor(rateRaw) : 0;
    if (rate <= 0) return 0;
    return Math.floor((amountFen * rate) / 100);
  }, [payTokenAmountFen, payTokenCreditTokensPerCny]);

  const openPaygModal = useCallback(
    (product) => {
      if (isOutOfStock(product?.stock)) {
        showError(t('已售罄'));
        return;
      }
      setPaygCheckout(null);
      setPaygPayStatus('pending');
      paygPaidTipRef.current = false;
      setSelectedPaygProduct(product || null);

      if (epayEnabled && epayMethods.length > 0) {
        setPaygPayMethod('epay');
        setPaygEpayMethod(getDefaultEpayMethod(epayMethods));
      } else {
        setPaygPayMethod('balance');
        setPaygEpayMethod('');
      }
      setPaygModalOpen(true);
    },
    [epayEnabled, epayMethods, t],
  );

  const closePaygModal = () => {
    setPaygModalOpen(false);
    setPaygCheckout(null);
    setPaygPayStatus('pending');
    paygPaidTipRef.current = false;
    setSelectedPaygProduct(null);
  };

  const openPayRequestModal = useCallback(
    (product) => {
      if (isOutOfStock(product?.stock)) {
        showError(t('已售罄'));
        return;
      }
      setPayRequestCheckout(null);
      setPayRequestPayStatus('pending');
      payRequestPaidTipRef.current = false;
      setSelectedPayRequestProduct(product || null);

      if (epayEnabled && epayMethods.length > 0) {
        setPayRequestPayMethod('epay');
        setPayRequestEpayMethod(getDefaultEpayMethod(epayMethods));
      } else {
        setPayRequestPayMethod('balance');
        setPayRequestEpayMethod('');
      }
      setPayRequestModalOpen(true);
    },
    [epayEnabled, epayMethods, t],
  );

  const closePayRequestModal = () => {
    setPayRequestModalOpen(false);
    setPayRequestCheckout(null);
    setPayRequestPayStatus('pending');
    payRequestPaidTipRef.current = false;
    setSelectedPayRequestProduct(null);
  };

  const openPayTokenModal = useCallback(
    (product) => {
      if (isOutOfStock(product?.stock)) {
        showError(t('已售罄'));
        return;
      }
      setPayTokenCheckout(null);
      setPayTokenPayStatus('pending');
      payTokenPaidTipRef.current = false;
      setSelectedPayTokenProduct(product || null);

      if (epayEnabled && epayMethods.length > 0) {
        setPayTokenPayMethod('epay');
        setPayTokenEpayMethod(getDefaultEpayMethod(epayMethods));
      } else {
        setPayTokenPayMethod('balance');
        setPayTokenEpayMethod('');
      }
      setPayTokenModalOpen(true);
    },
    [epayEnabled, epayMethods, t],
  );

  const closePayTokenModal = () => {
    setPayTokenModalOpen(false);
    setPayTokenCheckout(null);
    setPayTokenPayStatus('pending');
    payTokenPaidTipRef.current = false;
    setSelectedPayTokenProduct(null);
  };

  const createPayRequestOrder = async () => {
    if (payRequestCheckout) return;
    if (!selectedPayRequestProduct?.id) {
      showError(t('请选择按次付费产品'));
      return;
    }
    if (isOutOfStock(selectedPayRequestProduct?.stock)) {
      showError(t('已售罄'));
      return;
    }

    const amount = Number(payRequestAmountYuan);
    if (!Number.isFinite(amount) || amount <= 0) {
      showError(t('请输入充值金额'));
      return;
    }
    if (payRequestPayMethod !== 'balance' && payRequestPayMethod !== 'epay') {
      showError(t('支付方式无效'));
      return;
    }
    if (payRequestPayMethod === 'epay') {
      if (!epayEnabled || !payRequestEpayMethod) {
        showError(t('请选择支付方式'));
        return;
      }
    }

    setPayRequestSubmitting(true);
    try {
      const money = amount.toFixed(2);
      const res = await API.post('/api/pay_request/order', {
        money,
        pay_method: payRequestPayMethod,
        epay_method: payRequestPayMethod === 'epay' ? payRequestEpayMethod : '',
        product_id: selectedPayRequestProduct.id,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('创建订单失败'));
        return;
      }

      const tradeNo = data?.trade_no;
      if (payRequestPayMethod === 'balance') {
        showSuccess(t('充值成功'));
        void refreshViewerPricingState();
        closePayRequestModal();
        void loadPayConfig();
        return;
      }

      const url = data?.url;
      const params = data?.params;
      const payLink = buildEpayPayLink(url, params);
      setPayRequestCheckout({
        tradeNo,
        url,
        params,
        payLink,
        method: payRequestEpayMethod,
      });
      setPayRequestPayStatus('pending');
    } catch (e) {
      showError(e?.message || t('创建订单失败'));
    } finally {
      setPayRequestSubmitting(false);
    }
  };

  const createPayTokenOrder = async () => {
    if (payTokenCheckout) return;
    if (!selectedPayTokenProduct?.id) {
      showError(t('请选择按token付费产品'));
      return;
    }
    if (isOutOfStock(selectedPayTokenProduct?.stock)) {
      showError(t('已售罄'));
      return;
    }

    const amount = Number(payTokenAmountYuan);
    if (!Number.isFinite(amount) || amount <= 0) {
      showError(t('请输入充值金额'));
      return;
    }
    if (payTokenPayMethod !== 'balance' && payTokenPayMethod !== 'epay') {
      showError(t('支付方式无效'));
      return;
    }
    if (payTokenPayMethod === 'epay') {
      if (!epayEnabled || !payTokenEpayMethod) {
        showError(t('请选择支付方式'));
        return;
      }
    }

    setPayTokenSubmitting(true);
    try {
      const money = amount.toFixed(2);
      const res = await API.post('/api/pay_token/order', {
        money,
        pay_method: payTokenPayMethod,
        epay_method: payTokenPayMethod === 'epay' ? payTokenEpayMethod : '',
        product_id: selectedPayTokenProduct.id,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('创建订单失败'));
        return;
      }

      const tradeNo = data?.trade_no;
      if (payTokenPayMethod === 'balance') {
        showSuccess(t('充值成功'));
        void refreshViewerPricingState();
        closePayTokenModal();
        void loadPayConfig();
        return;
      }

      const url = data?.url;
      const params = data?.params;
      const payLink = buildEpayPayLink(url, params);
      setPayTokenCheckout({
        tradeNo,
        url,
        params,
        payLink,
        method: payTokenEpayMethod,
      });
      setPayTokenPayStatus('pending');
    } catch (e) {
      showError(e?.message || t('创建订单失败'));
    } finally {
      setPayTokenSubmitting(false);
    }
  };

  const createPaygOrder = async () => {
    if (paygCheckout) return;
    if (!selectedPaygProduct?.id) {
      showError(t('请选择按量商品'));
      return;
    }
    if (isOutOfStock(selectedPaygProduct?.stock)) {
      showError(t('已售罄'));
      return;
    }

    const amount = Number(paygAmountYuan);
    if (!Number.isFinite(amount) || amount <= 0) {
      showError(t('请输入充值金额'));
      return;
    }
    if (paygPayMethod !== 'balance' && paygPayMethod !== 'epay') {
      showError(t('支付方式无效'));
      return;
    }
    if (paygPayMethod === 'epay') {
      if (!epayEnabled || !paygEpayMethod) {
        showError(t('请选择支付方式'));
        return;
      }
    }

    setPaygSubmitting(true);
    try {
      const money = amount.toFixed(2);
      const res = await API.post('/api/payg/order', {
        money,
        pay_method: paygPayMethod,
        epay_method: paygPayMethod === 'epay' ? paygEpayMethod : '',
        product_id: selectedPaygProduct.id,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('创建订单失败'));
        return;
      }

      if (paygPayMethod === 'balance') {
        showSuccess(t('充值成功'));
        void refreshViewerPricingState();
        closePaygModal();
        void loadPayConfig();
        return;
      }

      const checkout = buildGatewayCheckoutState(data, paygEpayMethod);
      if (!checkout.payPageUrl && !checkout.qrCode && !checkout.qrImageUrl) {
        showError(t('后台没有返回支付页或二维码'));
        return;
      }
      setPaygCheckout(checkout);
      setPaygPayStatus('pending');
    } catch (e) {
      showError(e?.message || t('创建订单失败'));
    } finally {
      setPaygSubmitting(false);
    }
  };

  const createOrder = async () => {
    if (epayCheckout) return;
    if (subscriptionCheckoutMode === 'traffic') {
      setOrderModalOpen(false);
      return;
    }

    if (!orderingPlan?.id) return;

    const qtyRaw = Number(orderQuantity);
    const qty = Number.isFinite(qtyRaw) ? Math.max(1, Math.floor(qtyRaw)) : 1;
    const stock = normalizeStockValue(orderingPlan?.stock);
    if (typeof stock === 'number') {
      if (stock <= 0) {
        showError(t('已售罄'));
        return;
      }
      if (qty > stock) {
        showError(t('库存不足'));
        setOrderQuantity(Math.max(1, stock));
        return;
      }
    }

    if (applyMode !== 'stack' && applyMode !== 'defer') {
      showError(t('请选择叠加或顺延'));
      return;
    }
    const multiDeferOnly = orderingPlan?.multi_quantity_defer_only !== false;
    if (multiDeferOnly && applyMode !== 'defer') {
      showError(t('仅支持顺延'));
      return;
    }
    if (payMethod !== 'epay' && payMethod !== 'balance') {
      showError(t('请选择支付方式'));
      return;
    }
    if (payMethod === 'epay') {
      if (!epayEnabled) {
        showError(t('管理员未配置易支付'));
        return;
      }
      if (!epayMethod) {
        showError(t('请选择易支付支付方式'));
        return;
      }
    }
    const unitFen = Number(orderingPlan?.price_fen || 0);
    const totalFen =
      Number.isFinite(unitFen) && unitFen > 0 ? unitFen * qty : 0;
    if (payMethod === 'balance' && balanceFen < totalFen) {
      showError(t('余额不足'));
      return;
    }

    setOrderSubmitting(true);
    try {
      const payload = {
        plan_id: orderingPlan.id,
        pay_method: payMethod,
        epay_method: payMethod === 'epay' ? epayMethod : '',
        apply_mode: applyMode,
        quantity: qty,
      };
      const res = await API.post('/api/subscription/order', payload);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('下单失败'));
        return;
      }

      if (payMethod === 'balance') {
        showSuccess(t('订阅购买成功'));
        closeOrderModal();
        await refreshViewerPricingState();
        void loadPlans();
        void loadPayConfig();
        return;
      }

      if (!data?.url || !data?.params) {
        showError(t('拉起支付失败'));
        return;
      }
      const payLink = buildEpayPayLink(data.url, data.params);
      setEpayCheckout({
        tradeNo: data.trade_no || '',
        url: data.url,
        params: data.params,
        payLink,
        method: epayMethod,
      });
      setEpayPayStatus('pending');
      epayPaidTipRef.current = false;
      showSuccess(t('支付订单已创建，请扫码或打开支付页完成支付'));
    } catch (e) {
      showError(e?.message || t('下单失败'));
    } finally {
      setOrderSubmitting(false);
    }
  };

  const planList = Array.isArray(plans) ? plans : [];
  const maxOrderQuantity = useMemo(() => {
    if (!orderingPlan) return 1;
    if (!orderingPlan?.multi_quantity_enabled) return 1;
    const purchaseLimit = Number(orderingPlan?.purchase_limit || 0);
    const purchasedCount = Number(orderingPlan?.purchased_count || 0);
    let max = 100;
    if (Number.isFinite(purchaseLimit) && purchaseLimit > 0) {
      max = Math.max(
        1,
        purchaseLimit - (Number.isFinite(purchasedCount) ? purchasedCount : 0),
      );
    }
    const stock = normalizeStockValue(orderingPlan?.stock);
    if (typeof stock === 'number') {
      if (stock <= 0) return 1;
      max = Math.min(max, stock);
    }
    return Math.min(100, max);
  }, [orderingPlan]);

  const orderTotalFen = useMemo(() => {
    const unitFen = Number(orderingPlan?.price_fen || 0);
    const qtyRaw = Number(orderQuantity);
    const qty = Number.isFinite(qtyRaw) ? Math.max(1, Math.floor(qtyRaw)) : 1;
    if (!Number.isFinite(unitFen) || unitFen <= 0) return 0;
    return unitFen * qty;
  }, [orderQuantity, orderingPlan]);

  const paygSaleProducts = useMemo(() => {
    return normalizePaygProducts(paygProducts)
      .filter((p) => p?.enabled !== false)
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      });
  }, [paygProducts]);

  const payRequestSaleProducts = useMemo(() => {
    return normalizePayRequestProducts(payRequestProducts)
      .filter((p) => p?.enabled !== false)
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      });
  }, [payRequestProducts]);

  const payTokenSaleProducts = useMemo(() => {
    return normalizePayTokenProducts(payTokenProducts)
      .filter((p) => p?.enabled !== false)
      .sort((a, b) => {
        const sa = Number(a?.sort_order ?? 0) || 0;
        const sb = Number(b?.sort_order ?? 0) || 0;
        if (sa !== sb) return sb - sa;
        const ia = Number(a?.id ?? 0) || 0;
        const ib = Number(b?.id ?? 0) || 0;
        return ib - ia;
      });
  }, [payTokenProducts]);

  const planCards = (
    <div className='grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 items-stretch'>
      {payRequestSaleProducts.length > 0
        ? payRequestSaleProducts.map((product) => (
            <Card
              key={`pay-request-${product.id}`}
              className='!rounded-2xl !border-0 !shadow-none h-full self-stretch'
              bodyStyle={{
                display: 'flex',
                flexDirection: 'column',
                height: '100%',
              }}
            >
              <div className='space-y-2'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <Title heading={6} style={{ margin: 0 }}>
                      {product.name || t('按次付费')}
                    </Title>
                    {product.description ? (
                      <div className='mt-1 text-xs text-gray-500 leading-tight'>
                        {product.description}
                      </div>
                    ) : null}
                    <div className='mt-1 space-y-0.5 text-xs text-gray-500 leading-tight'>
                      <div>
                        {payRequestCreditRequestsPerCny > 0
                          ? t('¥1 = {{rate}} 次', {
                              rate: payRequestCreditRequestsPerCny,
                            })
                          : t('按次付费兑换比例未配置')}
                      </div>
                      {(() => {
                        const stock = normalizeStockValue(product.stock);
                        if (stock === null) {
                          return (
                            <div>
                              {t('库存')}: {t('无限')}
                            </div>
                          );
                        }
                        if (stock === 0) {
                          return (
                            <div className='text-red-500'>
                              {t('库存')}: {t('售罄')}
                            </div>
                          );
                        }
                        if (typeof stock === 'number') {
                          return (
                            <div>
                              {t('库存')}: {stock}
                            </div>
                          );
                        }
                        return null;
                      })()}
                      {Array.isArray(product.allowed_group_ids) &&
                      getVisibleOptionalGroupIds(product.allowed_group_ids)
                        .length > 0 ? (
                        <div className='flex flex-wrap items-center gap-1'>
                          {t('可选分组')}：
                          <span className='inline-flex flex-wrap gap-1'>
                            {getVisibleOptionalGroupIds(
                              product.allowed_group_ids,
                            ).map((gid) => (
                              <Text
                                key={`${product.id}-${gid}`}
                                code
                                style={{ fontSize: 12 }}
                              >
                                {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                              </Text>
                            ))}
                          </span>
                        </div>
                      ) : null}
                    </div>
                  </div>
                </div>
              </div>
              <div className='mt-auto flex justify-end pt-4'>
                <Button
                  type='primary'
                  theme='solid'
                  className='!rounded-lg'
                  disabled={isOutOfStock(product.stock)}
                  onClick={() => openPayRequestModal(product)}
                >
                  {isOutOfStock(product.stock) ? t('售罄') : t('充值')}
                </Button>
              </div>
            </Card>
          ))
        : null}

      {payTokenSaleProducts.length > 0
        ? payTokenSaleProducts.map((product) => (
            <Card
              key={`pay-token-${product.id}`}
              className='!rounded-2xl !border-0 !shadow-none h-full self-stretch'
              bodyStyle={{
                display: 'flex',
                flexDirection: 'column',
                height: '100%',
              }}
            >
              <div className='space-y-2'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <Title heading={6} style={{ margin: 0 }}>
                      {product.name || t('按token付费')}
                    </Title>
                    {product.description ? (
                      <div className='mt-1 text-xs text-gray-500 leading-tight'>
                        {product.description}
                      </div>
                    ) : null}
                    <div className='mt-1 space-y-0.5 text-xs text-gray-500 leading-tight'>
                      <div>
                        {payTokenCreditTokensPerCny > 0
                          ? t('¥1 = {{rate}} tokens', {
                              rate: payTokenCreditTokensPerCny,
                            })
                          : t('按token付费兑换比例未配置')}
                      </div>
                      {(() => {
                        const stock = normalizeStockValue(product.stock);
                        if (stock === null) {
                          return (
                            <div>
                              {t('库存')}: {t('无限')}
                            </div>
                          );
                        }
                        if (stock === 0) {
                          return (
                            <div className='text-red-500'>
                              {t('库存')}: {t('售罄')}
                            </div>
                          );
                        }
                        if (typeof stock === 'number') {
                          return (
                            <div>
                              {t('库存')}: {stock}
                            </div>
                          );
                        }
                        return null;
                      })()}
                      {Array.isArray(product.allowed_group_ids) &&
                      getVisibleOptionalGroupIds(product.allowed_group_ids)
                        .length > 0 ? (
                        <div className='flex flex-wrap items-center gap-1'>
                          {t('可选分组')}：
                          <span className='inline-flex flex-wrap gap-1'>
                            {getVisibleOptionalGroupIds(
                              product.allowed_group_ids,
                            ).map((gid) => (
                              <Text
                                key={`${product.id}-${gid}`}
                                code
                                style={{ fontSize: 12 }}
                              >
                                {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                              </Text>
                            ))}
                          </span>
                        </div>
                      ) : null}
                    </div>
                  </div>
                </div>
              </div>
              <div className='mt-auto flex justify-end pt-4'>
                <Button
                  type='primary'
                  theme='solid'
                  className='!rounded-lg'
                  disabled={isOutOfStock(product.stock)}
                  onClick={() => openPayTokenModal(product)}
                >
                  {isOutOfStock(product.stock) ? t('售罄') : t('充值')}
                </Button>
              </div>
            </Card>
          ))
        : null}

      {paygSaleProducts.length > 0
        ? paygSaleProducts.map((product) => (
            <Card
              key={`payg-${product.id}`}
              className='!rounded-2xl !border-0 !shadow-none h-full self-stretch'
              bodyStyle={{
                display: 'flex',
                flexDirection: 'column',
                height: '100%',
              }}
            >
              <div className='min-w-0'>
                <Title heading={6} style={{ margin: 0 }}>
                  {product.name || t('按量付费')}
                </Title>
                {product.description ? (
                  <div className='mt-1 text-xs text-gray-500 leading-tight'>
                    {product.description}
                  </div>
                ) : null}
                {(() => {
                  const stock = normalizeStockValue(product.stock);
                  if (stock === null) {
                    return (
                      <div className='mt-1 text-xs text-gray-500 leading-tight'>
                        {t('库存')}: {t('无限')}
                      </div>
                    );
                  }
                  if (stock === 0) {
                    return (
                      <div className='mt-1 text-xs text-red-500 leading-tight'>
                        {t('库存')}: {t('售罄')}
                      </div>
                    );
                  }
                  if (typeof stock === 'number') {
                    return (
                      <div className='mt-1 text-xs text-gray-500 leading-tight'>
                        {t('库存')}: {stock}
                      </div>
                    );
                  }
                  return null;
                })()}
                {Array.isArray(product.allowed_group_ids) &&
                getVisibleOptionalGroupIds(product.allowed_group_ids).length >
                  0 ? (
                  <div className='mt-1 text-xs text-gray-500 leading-tight flex flex-wrap items-center gap-1'>
                    {t('可选分组')}：
                    <span className='inline-flex flex-wrap gap-1'>
                      {getVisibleOptionalGroupIds(
                        product.allowed_group_ids,
                      ).map((gid) => (
                        <Text
                          key={`${product.id}-${gid}`}
                          code
                          style={{ fontSize: 12 }}
                        >
                          {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                        </Text>
                      ))}
                    </span>
                  </div>
                ) : null}
              </div>
              <div className='mt-auto flex justify-end pt-4'>
                <Button
                  type='primary'
                  theme='solid'
                  className='!rounded-lg'
                  disabled={isOutOfStock(product.stock)}
                  onClick={() => openPaygModal(product)}
                >
                  {isOutOfStock(product.stock) ? t('售罄') : t('充值')}
                </Button>
              </div>
            </Card>
          ))
        : null}

      {planList.map((plan) => {
        const mode = inferPresetMode(plan);
        return (
          <Card
            key={plan.id}
            className='!rounded-2xl !border-0 !shadow-none h-full self-stretch'
            bodyStyle={{
              display: 'flex',
              flexDirection: 'column',
              height: '100%',
            }}
          >
            <div className='space-y-2'>
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <Title heading={6} style={{ margin: 0 }}>
                    {plan.name}
                  </Title>
                  {plan.description ? (
                    <div className='mt-1 text-xs text-gray-500 leading-tight'>
                      {plan.description}
                    </div>
                  ) : null}
                  <div className='mt-1 space-y-0.5 text-xs text-gray-500 leading-tight'>
                    {mode === 'request' ? (
                      <>
                        <div>
                          {t('每日次数')}{' '}
                          {Number(plan.daily_request_limit) === 0
                            ? t('无限')
                            : Number(plan.daily_request_limit) || 0}
                        </div>
                        <div>
                          {t('总次数')}{' '}
                          {Number(plan.quota) === 0
                            ? t('无限')
                            : Number(plan.quota) || 0}
                        </div>
                      </>
                    ) : mode === 'tokens' ? (
                      <>
                        <div>
                          {t('Tokens')}{' '}
                          {Number(plan.quota) === 0
                            ? t('无限')
                            : `${Number(plan.quota) || 0} tokens`}
                        </div>
                        {(() => {
                          const groupDailyLimits = normalizeGroupDailyLimits(
                            plan.group_daily_limits,
                          );
                          if (groupDailyLimits.length > 0) {
                            return (
                              <div>
                                <div>{t('日限（按分组）')}</div>
                                <div className='mt-0.5 flex flex-wrap items-center gap-1'>
                                  {groupDailyLimits.map((item) => (
                                    <Text
                                      key={`plan-${plan.id}-${item.group_id}`}
                                      code
                                      style={{ fontSize: 12 }}
                                    >
                                      {`${getGroupLabel(item.group_id)}: ${
                                        item.daily_quota_limit <= 0
                                          ? t('无限')
                                          : `${item.daily_quota_limit || 0} tokens`
                                      }`}
                                    </Text>
                                  ))}
                                </div>
                              </div>
                            );
                          }
                          return (
                            <div>
                              {t('日限')}{' '}
                              {Number(plan.daily_quota_limit) <= 0
                                ? t('无限')
                                : `${Number(plan.daily_quota_limit) || 0} tokens`}
                            </div>
                          );
                        })()}
                      </>
                    ) : (
                      <>
                        <div>
                          {t('额度')}{' '}
                          {Number(plan.quota) === 0
                            ? t('无限')
                            : renderQuotaToUSD(plan.quota || 0, 2)}
                        </div>
                        {(() => {
                          const groupDailyLimits = normalizeGroupDailyLimits(
                            plan.group_daily_limits,
                          );
                          if (groupDailyLimits.length > 0) {
                            return (
                              <div>
                                <div>{t('日限（按分组）')}</div>
                                <div className='mt-0.5 flex flex-wrap items-center gap-1'>
                                  {groupDailyLimits.map((item) => (
                                    <Text
                                      key={`plan-${plan.id}-${item.group_id}`}
                                      code
                                      style={{ fontSize: 12 }}
                                    >
                                      {`${getGroupLabel(item.group_id)}: ${
                                        item.daily_quota_limit <= 0
                                          ? t('无限')
                                          : renderQuotaToUSD(
                                              item.daily_quota_limit || 0,
                                              2,
                                            )
                                      }`}
                                    </Text>
                                  ))}
                                </div>
                              </div>
                            );
                          }
                          return (
                            <div>
                              {t('日限')}{' '}
                              {Number(plan.daily_quota_limit) <= 0
                                ? t('无限')
                                : renderQuotaToUSD(
                                    plan.daily_quota_limit || 0,
                                    2,
                                  )}
                            </div>
                          );
                        })()}
                      </>
                    )}
                    <div>
                      {t('时长')}{' '}
                      {Number(plan.quota_valid_days) === 0
                        ? t('无限')
                        : `${plan.quota_valid_days || 0} ${t('天')}`}
                    </div>
                    {(() => {
                      const stock = normalizeStockValue(plan.stock);
                      if (stock === null) {
                        return (
                          <div>
                            {t('库存')}: {t('无限')}
                          </div>
                        );
                      }
                      if (stock === 0) {
                        return (
                          <div className='text-red-500'>
                            {t('库存')}: {t('售罄')}
                          </div>
                        );
                      }
                      if (typeof stock === 'number') {
                        return (
                          <div>
                            {t('库存')}: {stock}
                          </div>
                        );
                      }
                      return null;
                    })()}
                    {Number(plan.purchase_limit) > 0 ? (
                      <div>
                        {t('限购')} {Number(plan.purchase_limit)} {t('次')}
                      </div>
                    ) : null}
                    {(() => {
                      const groupIds = normalizeGroupIds(
                        plan.allowed_group_ids,
                      );
                      const visibleGroupIds = getVisibleOptionalGroupIds(
                        plan.allowed_group_ids,
                      );
                      if (groupIds.length === 0) {
                        return (
                          <div className='flex flex-wrap items-center gap-1'>
                            {t('可选分组')}：
                            <span className='inline-flex flex-wrap gap-1'>
                              <Text type='danger' style={{ fontSize: 12 }}>
                                {t('未配置')}
                              </Text>
                            </span>
                          </div>
                        );
                      }
                      if (visibleGroupIds.length === 0) {
                        return null;
                      }
                      return (
                        <div className='flex flex-wrap items-center gap-1'>
                          {t('可选分组')}：
                          <span className='inline-flex flex-wrap gap-1'>
                            {visibleGroupIds.map((gid) => (
                              <Text key={gid} code style={{ fontSize: 12 }}>
                                {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                              </Text>
                            ))}
                          </span>
                        </div>
                      );
                    })()}
                  </div>
                </div>
                <div className='shrink-0 text-right'>
                  <div className='text-lg font-semibold text-rose-600'>
                    {renderCnyFen(plan.price_fen)}
                  </div>
                  <div className='text-xs text-gray-500'>
                    {mode === 'request'
                      ? t('次数订阅')
                      : mode === 'tokens'
                        ? t('Tokens订阅')
                        : t('订阅额度')}
                  </div>
                </div>
              </div>
            </div>
            <div className='mt-auto flex justify-end pt-4'>
              <Button
                type='primary'
                theme='solid'
                className='!rounded-lg'
                disabled={
                  isOutOfStock(plan.stock) ||
                  (Number(plan.purchase_limit) > 0 &&
                    Number(plan.purchased_count) >= Number(plan.purchase_limit))
                }
                onClick={() => openOrderModal(plan)}
              >
                {isOutOfStock(plan.stock)
                  ? t('售罄')
                  : Number(plan.purchase_limit) > 0 &&
                      Number(plan.purchased_count) >=
                        Number(plan.purchase_limit)
                    ? t('已达限购')
                    : t('购买')}
              </Button>
            </div>
          </Card>
        );
      })}
    </div>
  );

  const combinedListData = useMemo(() => {
    const payRequestItems = payRequestSaleProducts.map((product) => ({
      ...product,
      _type: 'pay_request',
      _key: `pay_request-${product.id}`,
    }));
    const payTokenItems = payTokenSaleProducts.map((product) => ({
      ...product,
      _type: 'pay_token',
      _key: `pay_token-${product.id}`,
    }));
    const paygItems = paygSaleProducts.map((product) => ({
      ...product,
      _type: 'payg',
      _key: `payg-${product.id}`,
    }));
    const planItems = planList.map((plan) => ({
      ...plan,
      _type: 'plan',
      _key: `plan-${plan.id}`,
    }));
    return [...payRequestItems, ...payTokenItems, ...paygItems, ...planItems];
  }, [
    payRequestSaleProducts,
    payTokenSaleProducts,
    paygSaleProducts,
    planList,
  ]);

  const combinedListColumns = useMemo(
    () => [
      {
        title: t('类型'),
        dataIndex: '_type',
        key: '_type',
        width: 90,
        render: (type, record) => {
          if (type === 'pay_request') {
            return <Text type='success'>{t('按次付费')}</Text>;
          }
          if (type === 'pay_token') {
            return <Text type='success'>{t('按token付费')}</Text>;
          }
          if (type === 'payg') {
            return <Text type='success'>{t('按量付费')}</Text>;
          }
          const mode = inferPresetMode(record);
          if (mode === 'request') return t('次数订阅');
          if (mode === 'tokens') return t('Tokens订阅');
          return t('订阅额度');
        },
      },
      {
        title: t('名称'),
        dataIndex: 'name',
        key: 'name',
        render: (text, record) => (
          <div>
            <div className='font-medium'>
              {text ||
                (record._type === 'payg'
                  ? t('按量付费')
                  : record._type === 'pay_request'
                    ? t('按次付费')
                    : record._type === 'pay_token'
                      ? t('按token付费')
                      : '-')}
            </div>
            {record.description && (
              <div className='text-xs text-gray-500 mt-0.5'>
                {record.description}
              </div>
            )}
          </div>
        ),
      },
      {
        title: t('额度/次数'),
        dataIndex: 'quota',
        key: 'quota',
        width: 100,
        render: (_, record) => {
          if (record._type === 'payg') return '-';
          if (record._type === 'pay_request') return '-';
          if (record._type === 'pay_token') return '-';
          const mode = inferPresetMode(record);
          if (mode === 'request') {
            const total = Number(record.quota) || 0;
            const totalLabel = total === 0 ? t('无限') : total;
            return `${totalLabel} ${t('次')}`;
          }
          if (mode === 'tokens') {
            const quota = Number(record.quota) || 0;
            return quota === 0 ? t('无限') : `${quota} tokens`;
          }
          const quota = Number(record.quota) || 0;
          return quota === 0 ? t('无限') : renderQuotaToUSD(quota, 2);
        },
      },
      {
        title: t('日限'),
        dataIndex: 'daily_quota_limit',
        key: 'daily_quota_limit',
        render: (_, record) => {
          if (
            record._type === 'payg' ||
            record._type === 'pay_request' ||
            record._type === 'pay_token'
          )
            return '-';
          const mode = inferPresetMode(record);
          if (mode === 'request') {
            const daily = Number(record.daily_request_limit) || 0;
            return daily === 0 ? t('无限') : `${daily} ${t('次/天')}`;
          }
          if (mode === 'tokens') {
            const groupDailyLimits = normalizeGroupDailyLimits(
              record.group_daily_limits,
            );
            if (groupDailyLimits.length > 0) {
              return (
                <div className='flex items-center gap-1 flex-wrap'>
                  {groupDailyLimits.map((item) => (
                    <Text
                      key={`list-${record.id}-${item.group_id}`}
                      code
                      style={{ fontSize: 11, whiteSpace: 'nowrap' }}
                    >
                      {`${getGroupLabel(item.group_id)}: ${item.daily_quota_limit <= 0 ? t('无限') : `${item.daily_quota_limit || 0} tokens`}`}
                    </Text>
                  ))}
                </div>
              );
            }
            const limit = Number(record.daily_quota_limit) || 0;
            return limit <= 0 ? t('无限') : `${limit} tokens`;
          }
          const groupDailyLimits = normalizeGroupDailyLimits(
            record.group_daily_limits,
          );
          if (groupDailyLimits.length > 0) {
            return (
              <div className='flex items-center gap-1 flex-wrap'>
                {groupDailyLimits.map((item) => (
                  <Text
                    key={`list-${record.id}-${item.group_id}`}
                    code
                    style={{ fontSize: 11, whiteSpace: 'nowrap' }}
                  >
                    {`${getGroupLabel(item.group_id)}: ${item.daily_quota_limit <= 0 ? t('无限') : renderQuotaToUSD(item.daily_quota_limit || 0, 2)}`}
                  </Text>
                ))}
              </div>
            );
          }
          return Number(record.daily_quota_limit) <= 0
            ? t('无限')
            : renderQuotaToUSD(record.daily_quota_limit || 0, 2);
        },
      },
      {
        title: t('时长'),
        dataIndex: 'quota_valid_days',
        key: 'quota_valid_days',
        width: 70,
        render: (text, record) => {
          if (
            record._type === 'payg' ||
            record._type === 'pay_request' ||
            record._type === 'pay_token'
          )
            return '-';
          const days = Number(text) || 0;
          return days === 0 ? t('无限') : `${days} ${t('天')}`;
        },
      },
      {
        title: t('库存'),
        dataIndex: 'stock',
        key: 'stock',
        width: 70,
        render: (text) => {
          const stock = normalizeStockValue(text);
          if (stock === null) return t('无限');
          if (stock === 0) return <Text type='danger'>{t('售罄')}</Text>;
          if (typeof stock === 'number') return stock;
          return '-';
        },
      },
      {
        title: t('可选分组'),
        dataIndex: 'allowed_group_ids',
        key: 'allowed_group_ids',
        render: (_, record) => {
          const groupIds = normalizeGroupIds(record.allowed_group_ids);
          const visibleGroupIds = getVisibleOptionalGroupIds(
            record.allowed_group_ids,
          );
          if (groupIds.length === 0) {
            return record._type === 'payg' ||
              record._type === 'pay_request' ||
              record._type === 'pay_token' ? (
              '-'
            ) : (
              <Text type='danger' style={{ fontSize: 12 }}>
                {t('未配置')}
              </Text>
            );
          }
          if (visibleGroupIds.length === 0) {
            return '-';
          }
          return (
            <div className='flex items-center gap-1 flex-wrap'>
              {visibleGroupIds.map((gid) => (
                <Text
                  key={`list-group-${record._key}-${gid}`}
                  code
                  style={{ fontSize: 11, whiteSpace: 'nowrap' }}
                >
                  {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                </Text>
              ))}
            </div>
          );
        },
      },
      {
        title: t('价格'),
        dataIndex: 'price_fen',
        key: 'price_fen',
        width: 80,
        render: (text, record) => {
          if (
            record._type === 'payg' ||
            record._type === 'pay_request' ||
            record._type === 'pay_token'
          )
            return '-';
          return (
            <span className='font-semibold text-rose-600 whitespace-nowrap'>
              {renderCnyFen(text)}
            </span>
          );
        },
      },
      {
        title: t('操作'),
        key: 'action',
        width: 90,
        render: (_, record) => {
          if (record._type === 'pay_request') {
            return (
              <Button
                type='primary'
                theme='solid'
                size='small'
                className='!rounded-lg'
                disabled={isOutOfStock(record.stock)}
                onClick={() => openPayRequestModal(record)}
              >
                {isOutOfStock(record.stock) ? t('售罄') : t('充值')}
              </Button>
            );
          }
          if (record._type === 'pay_token') {
            return (
              <Button
                type='primary'
                theme='solid'
                size='small'
                className='!rounded-lg'
                disabled={isOutOfStock(record.stock)}
                onClick={() => openPayTokenModal(record)}
              >
                {isOutOfStock(record.stock) ? t('售罄') : t('充值')}
              </Button>
            );
          }
          if (record._type === 'payg') {
            return (
              <Button
                type='primary'
                theme='solid'
                size='small'
                className='!rounded-lg'
                disabled={isOutOfStock(record.stock)}
                onClick={() => openPaygModal(record)}
              >
                {isOutOfStock(record.stock) ? t('售罄') : t('充值')}
              </Button>
            );
          }
          const isLimitReached =
            Number(record.purchase_limit) > 0 &&
            Number(record.purchased_count) >= Number(record.purchase_limit);
          const outOfStock = isOutOfStock(record.stock);
          return (
            <Button
              type='primary'
              theme='solid'
              size='small'
              className='!rounded-lg'
              disabled={outOfStock || isLimitReached}
              onClick={() => openOrderModal(record)}
            >
              {outOfStock
                ? t('售罄')
                : isLimitReached
                  ? t('已达限购')
                  : t('购买')}
            </Button>
          );
        },
      },
    ],
    [
      t,
      groupRatios,
      openOrderModal,
      openPaygModal,
      openPayRequestModal,
      openPayTokenModal,
    ],
  );

  const planListView = (
    <Card className='!rounded-xl overflow-hidden' bordered={false}>
      <Table
        columns={combinedListColumns}
        dataSource={combinedListData}
        rowKey='_key'
        pagination={false}
        scroll={{ y: 'calc(100vh - 280px)' }}
        size='small'
      />
    </Card>
  );

  useEffect(() => {
    loadPlans();
    loadPayConfig();
    void refreshViewerPricingState();
  }, []);

  useEffect(() => {
    if (paygAutoOpenRef.current) return;
    if (payConfigLoading) return;
    const params = new URLSearchParams(location.search || '');
    if (params.get('payg') !== '1') return;
    if (paygSaleProducts.length === 0) {
      paygAutoOpenRef.current = true;
      return;
    }
    openPaygModal(paygSaleProducts[0]);
    paygAutoOpenRef.current = true;
  }, [location.search, openPaygModal, payConfigLoading, paygSaleProducts]);

  useEffect(() => {
    if (!orderModalOpen) return;
    if (!epayCheckout?.tradeNo) return;

    let cancelled = false;
    let timer = null;

    const poll = async () => {
      if (cancelled) return;
      try {
        const res = await API.get('/api/subscription/order/status', {
          params: { trade_no: epayCheckout.tradeNo },
        });
        const { success, data } = res.data;
        if (!success) {
          timer = window.setTimeout(poll, 2000);
          return;
        }
        const status = String(data?.status || '').trim();
        if (status === 'success') {
          if (!epayPaidTipRef.current) {
            epayPaidTipRef.current = true;
            showSuccess(t('支付成功，订阅已生效'));
          }
          void refreshViewerPricingState();
          closeOrderModal();
          void loadPlans();
          void loadPayConfig();
          return;
        }
        if (status === 'failed') {
          setEpayPayStatus('failed');
          if (!epayPaidTipRef.current) {
            epayPaidTipRef.current = true;
            showError(t('支付失败'));
          }
          return;
        }
        setEpayPayStatus('pending');
      } catch {}
      timer = window.setTimeout(poll, 2000);
    };

    poll();

    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [orderModalOpen, epayCheckout?.tradeNo]);

  useEffect(() => {
    if (!payRequestModalOpen) return;
    if (!payRequestCheckout?.tradeNo) return;

    let cancelled = false;
    let timer = null;

    const poll = async () => {
      if (cancelled) return;
      try {
        const res = await API.get('/api/pay_request/order/status', {
          params: { trade_no: payRequestCheckout.tradeNo },
        });
        const { success, data } = res.data;
        if (!success) {
          timer = window.setTimeout(poll, 2000);
          return;
        }
        const status = String(data?.status || '').trim();
        if (status === 'success') {
          if (!payRequestPaidTipRef.current) {
            payRequestPaidTipRef.current = true;
            showSuccess(t('充值成功'));
          }
          void refreshViewerPricingState();
          closePayRequestModal();
          void loadPayConfig();
          return;
        }
        if (status === 'failed') {
          setPayRequestPayStatus('failed');
          if (!payRequestPaidTipRef.current) {
            payRequestPaidTipRef.current = true;
            showError(t('支付失败'));
          }
          return;
        }
        setPayRequestPayStatus('pending');
      } catch {}
      timer = window.setTimeout(poll, 2000);
    };

    poll();

    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [payRequestModalOpen, payRequestCheckout?.tradeNo]);

  useEffect(() => {
    if (!payTokenModalOpen) return;
    if (!payTokenCheckout?.tradeNo) return;

    let cancelled = false;
    let timer = null;

    const poll = async () => {
      if (cancelled) return;
      try {
        const res = await API.get('/api/pay_token/order/status', {
          params: { trade_no: payTokenCheckout.tradeNo },
        });
        const { success, data } = res.data;
        if (!success) {
          timer = window.setTimeout(poll, 2000);
          return;
        }
        const status = String(data?.status || '').trim();
        if (status === 'success') {
          if (!payTokenPaidTipRef.current) {
            payTokenPaidTipRef.current = true;
            showSuccess(t('充值成功'));
          }
          void refreshViewerPricingState();
          closePayTokenModal();
          void loadPayConfig();
          return;
        }
        if (status === 'failed') {
          setPayTokenPayStatus('failed');
          if (!payTokenPaidTipRef.current) {
            payTokenPaidTipRef.current = true;
            showError(t('支付失败'));
          }
          return;
        }
        setPayTokenPayStatus('pending');
      } catch {}
      timer = window.setTimeout(poll, 2000);
    };

    poll();

    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [payTokenModalOpen, payTokenCheckout?.tradeNo]);

  useEffect(() => {
    if (!paygModalOpen) return;
    if (!paygCheckout?.tradeNo) return;

    let cancelled = false;
    let timer = null;

    const poll = async () => {
      if (cancelled) return;
      try {
        const res = await API.get('/api/payg/order/status', {
          params: { trade_no: paygCheckout.tradeNo },
        });
        const { success, data } = res.data;
        if (!success) {
          timer = window.setTimeout(poll, 2000);
          return;
        }
        const status = String(data?.status || '').trim();
        if (status === 'success') {
          if (!paygPaidTipRef.current) {
            paygPaidTipRef.current = true;
            showSuccess(t('充值成功'));
          }
          void refreshViewerPricingState();
          closePaygModal();
          void loadPayConfig();
          return;
        }
        if (status === 'failed') {
          setPaygPayStatus('failed');
          if (!paygPaidTipRef.current) {
            paygPaidTipRef.current = true;
            showError(t('支付失败'));
          }
          return;
        }
        setPaygPayStatus('pending');
      } catch {}
      timer = window.setTimeout(poll, 2000);
    };

    poll();

    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [paygModalOpen, paygCheckout?.tradeNo]);

  return (
    <ConsolePage hideHeader>
      <div className='relative min-h-screen lg:min-h-0'>
        <Modal
          title={t('按次付费充值')}
          visible={payRequestModalOpen}
          onOk={createPayRequestOrder}
          onCancel={closePayRequestModal}
          maskClosable={false}
          centered
          confirmLoading={payRequestSubmitting}
          okText={
            payRequestPayMethod === 'epay' && !payRequestCheckout
              ? t('去支付')
              : t('确认充值')
          }
          cancelText={t('取消')}
          {...(payRequestCheckout
            ? {
                footer: (
                  <div className='flex justify-end'>
                    <Button
                      type='primary'
                      theme='solid'
                      className='!rounded-lg'
                      onClick={closePayRequestModal}
                    >
                      {t('关闭')}
                    </Button>
                  </div>
                ),
              }
            : {})}
          okButtonProps={{
            disabled:
              Boolean(payRequestCheckout) ||
              payConfigLoading ||
              payRequestSubmitting ||
              payRequestAmountFen <= 0 ||
              payRequestCreditRequests <= 0 ||
              (payRequestPayMethod === 'epay' &&
                (!epayEnabled || !payRequestEpayMethod)) ||
              (payRequestPayMethod === 'balance' &&
                balanceFen < payRequestAmountFen),
          }}
        >
          <div className='space-y-4 pb-6'>
            {selectedPayRequestProduct?.id ? (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='space-y-1'>
                  <div className='flex items-center justify-between gap-3'>
                    <Text strong>{t('商品')}</Text>
                    <Text type='tertiary' size='small'>
                      #{selectedPayRequestProduct.id}
                    </Text>
                  </div>
                  <div className='text-sm font-medium'>
                    {selectedPayRequestProduct.name}
                  </div>
                  {selectedPayRequestProduct.description ? (
                    <div className='text-xs text-gray-500 leading-tight'>
                      {selectedPayRequestProduct.description}
                    </div>
                  ) : null}
                  {Array.isArray(selectedPayRequestProduct.allowed_group_ids) &&
                  getVisibleOptionalGroupIds(
                    selectedPayRequestProduct.allowed_group_ids,
                  ).length > 0 ? (
                    <div className='text-xs text-gray-500 leading-tight flex flex-wrap items-center gap-1 pt-1'>
                      {t('可选分组')}：
                      <span className='inline-flex flex-wrap gap-1'>
                        {getVisibleOptionalGroupIds(
                          selectedPayRequestProduct.allowed_group_ids,
                        ).map((gid) => (
                          <Text
                            key={`${selectedPayRequestProduct.id}-${gid}`}
                            code
                            style={{ fontSize: 12 }}
                          >
                            {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                          </Text>
                        ))}
                      </span>
                    </div>
                  ) : null}
                </div>
              </Card>
            ) : null}
            <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-3'>
                  <Text strong>{t('充值金额')}</Text>
                  <div className='text-xs text-gray-500'>
                    {payRequestCreditRequestsPerCny > 0
                      ? t('¥1 = {{rate}} 次', {
                          rate: payRequestCreditRequestsPerCny,
                        })
                      : t('按次付费兑换比例未配置')}
                  </div>
                </div>
                <div className='flex flex-wrap items-center gap-3'>
                  <InputNumber
                    min={0.01}
                    precision={2}
                    value={payRequestAmountYuan}
                    disabled={Boolean(payRequestCheckout)}
                    onChange={(v) => setPayRequestAmountYuan(v)}
                    className='w-full md:w-60 !rounded-lg'
                  />
                  <div className='text-sm text-gray-600'>
                    {t('预计获得')}:{' '}
                    <Text strong>
                      {payRequestCreditRequests} {t('次')}
                    </Text>
                  </div>
                </div>
                <div className='text-xs text-gray-500'>
                  {t('当前按次付费余额')}:{' '}
                  {userState?.user?.pay_request_quota || 0} {t('次')}
                  {'  ·  '}
                  {t('账户余额')}: {renderCnyFen(balanceFen)}
                </div>
              </div>
            </Card>

            {!payRequestCheckout && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div>
                  <div className='mb-2 flex items-center justify-between gap-3'>
                    <Text strong>{t('支付方式')}</Text>
                    {payRequestPayMethod === 'balance' && (
                      <Text
                        size='small'
                        type='tertiary'
                        className='whitespace-nowrap'
                      >
                        {t('当前余额')}: {renderCnyFen(balanceFen)}
                      </Text>
                    )}
                  </div>
                  <Spin spinning={payConfigLoading}>
                    <Radio.Group
                      type='button'
                      value={
                        payRequestPayMethod === 'epay' && payRequestEpayMethod
                          ? `epay:${payRequestEpayMethod}`
                          : 'balance'
                      }
                      onChange={(val) => {
                        const selected =
                          val && val.target ? val.target.value : val;
                        if (selected === 'balance') {
                          setPayRequestPayMethod('balance');
                          setPayRequestEpayMethod('');
                          setPayRequestCheckout(null);
                          return;
                        }
                        if (
                          typeof selected === 'string' &&
                          selected.startsWith('epay:')
                        ) {
                          setPayRequestPayMethod('epay');
                          setPayRequestEpayMethod(
                            selected.slice('epay:'.length),
                          );
                          setPayRequestCheckout(null);
                        }
                      }}
                    >
                      {epayEnabled &&
                        epayMethods.map((m) => (
                          <Radio key={m.type} value={`epay:${m.type}`}>
                            {m.name || m.type}
                          </Radio>
                        ))}
                      <Radio value='balance'>{t('账户余额')}</Radio>
                    </Radio.Group>
                  </Spin>
                  {!payConfigLoading && !epayEnabled && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('管理员未配置易支付')}
                    </div>
                  )}
                  {epayEnabled &&
                    !payConfigLoading &&
                    epayMethods.length === 0 && (
                      <div className='mt-2 text-xs text-gray-500'>
                        {t('管理员未配置易支付')}
                      </div>
                    )}
                  {payRequestPayMethod === 'balance' &&
                    payRequestAmountFen > 0 && (
                      <div className='mt-2 text-xs text-gray-500'>
                        {t('需要支付')}: {renderCnyFen(payRequestAmountFen)}
                      </div>
                    )}
                  {payRequestPayMethod === 'balance' &&
                    payRequestAmountFen > 0 &&
                    balanceFen < payRequestAmountFen && (
                      <div className='mt-1 text-xs text-rose-600'>
                        {t('余额不足')}
                      </div>
                    )}
                </div>
              </Card>
            )}

            {payRequestCheckout && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='flex flex-col items-center gap-3'>
                  {(() => {
                    const methodType =
                      payRequestCheckout?.method || payRequestEpayMethod;
                    const scanTip = getEpayScanTip(methodType, t);
                    return (
                      <>
                        {scanTip ? (
                          <div className='text-xs text-gray-500'>{scanTip}</div>
                        ) : null}
                        {payRequestCheckout.payLink ? (
                          <QRCodeSVG
                            value={payRequestCheckout.payLink}
                            size={220}
                          />
                        ) : null}
                        <div className='flex items-center gap-2'>
                          <Button
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() =>
                              submitEpayForm(
                                payRequestCheckout.url,
                                payRequestCheckout.params,
                              )
                            }
                          >
                            {t('打开支付页')}
                          </Button>
                          <Button
                            className='!rounded-lg'
                            onClick={() => setPayRequestCheckout(null)}
                          >
                            {t('返回')}
                          </Button>
                        </div>
                        {payRequestCheckout.tradeNo && (
                          <div className='text-xs text-gray-500'>
                            {t('订单号')}: {payRequestCheckout.tradeNo}
                          </div>
                        )}
                      </>
                    );
                  })()}
                </div>
              </Card>
            )}
          </div>
        </Modal>

        <Modal
          title={t('按token付费充值')}
          visible={payTokenModalOpen}
          onOk={createPayTokenOrder}
          onCancel={closePayTokenModal}
          maskClosable={false}
          centered
          confirmLoading={payTokenSubmitting}
          okText={
            payTokenPayMethod === 'epay' && !payTokenCheckout
              ? t('去支付')
              : t('确认充值')
          }
          cancelText={t('取消')}
          {...(payTokenCheckout
            ? {
                footer: (
                  <div className='flex justify-end'>
                    <Button
                      type='primary'
                      theme='solid'
                      className='!rounded-lg'
                      onClick={closePayTokenModal}
                    >
                      {t('关闭')}
                    </Button>
                  </div>
                ),
              }
            : {})}
          okButtonProps={{
            disabled:
              Boolean(payTokenCheckout) ||
              payConfigLoading ||
              payTokenSubmitting ||
              payTokenAmountFen <= 0 ||
              payTokenCreditTokens <= 0 ||
              !selectedPayTokenProduct?.id ||
              (payTokenPayMethod === 'epay' &&
                (!epayEnabled || !payTokenEpayMethod)) ||
              (payTokenPayMethod === 'balance' &&
                balanceFen < payTokenAmountFen),
          }}
        >
          <div className='space-y-4 pb-6'>
            {selectedPayTokenProduct?.id ? (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='space-y-1'>
                  <div className='flex items-center justify-between gap-3'>
                    <Text strong>{t('商品')}</Text>
                    <Text type='tertiary' size='small'>
                      #{selectedPayTokenProduct.id}
                    </Text>
                  </div>
                  <div className='text-sm font-medium'>
                    {selectedPayTokenProduct.name}
                  </div>
                  {selectedPayTokenProduct.description ? (
                    <div className='text-xs text-gray-500 leading-tight'>
                      {selectedPayTokenProduct.description}
                    </div>
                  ) : null}
                  {Array.isArray(selectedPayTokenProduct.allowed_group_ids) &&
                  getVisibleOptionalGroupIds(
                    selectedPayTokenProduct.allowed_group_ids,
                  ).length > 0 ? (
                    <div className='text-xs text-gray-500 leading-tight flex flex-wrap items-center gap-1 pt-1'>
                      {t('可选分组')}：
                      <span className='inline-flex flex-wrap gap-1'>
                        {getVisibleOptionalGroupIds(
                          selectedPayTokenProduct.allowed_group_ids,
                        ).map((gid) => (
                          <Text
                            key={`${selectedPayTokenProduct.id}-${gid}`}
                            code
                            style={{ fontSize: 12 }}
                          >
                            {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                          </Text>
                        ))}
                      </span>
                    </div>
                  ) : null}
                </div>
              </Card>
            ) : null}

            <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-3'>
                  <Text strong>{t('充值金额')}</Text>
                  <div className='text-xs text-gray-500'>
                    {payTokenCreditTokensPerCny > 0
                      ? t('¥1 = {{rate}} tokens', {
                          rate: payTokenCreditTokensPerCny,
                        })
                      : t('按token付费兑换比例未配置')}
                  </div>
                </div>
                <div className='flex flex-wrap items-center gap-3'>
                  <InputNumber
                    min={0.01}
                    precision={2}
                    value={payTokenAmountYuan}
                    disabled={Boolean(payTokenCheckout)}
                    onChange={(v) => setPayTokenAmountYuan(v)}
                    className='w-full md:w-60 !rounded-lg'
                  />
                  <div className='text-sm text-gray-600'>
                    {t('预计获得')}:{' '}
                    <Text strong>{payTokenCreditTokens} tokens</Text>
                  </div>
                </div>
                <div className='text-xs text-gray-500'>
                  {t('当前按token付费余额')}:{' '}
                  {userState?.user?.pay_token_quota || 0} tokens
                  {'  ·  '}
                  {t('账户余额')}: {renderCnyFen(balanceFen)}
                </div>
              </div>
            </Card>

            {!payTokenCheckout && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div>
                  <div className='mb-2 flex items-center justify-between gap-3'>
                    <Text strong>{t('支付方式')}</Text>
                    {payTokenPayMethod === 'balance' && (
                      <Text
                        size='small'
                        type='tertiary'
                        className='whitespace-nowrap'
                      >
                        {t('当前余额')}: {renderCnyFen(balanceFen)}
                      </Text>
                    )}
                  </div>
                  <Spin spinning={payConfigLoading}>
                    <Radio.Group
                      type='button'
                      value={
                        payTokenPayMethod === 'epay' && payTokenEpayMethod
                          ? `epay:${payTokenEpayMethod}`
                          : 'balance'
                      }
                      onChange={(val) => {
                        const selected =
                          val && val.target ? val.target.value : val;
                        if (selected === 'balance') {
                          setPayTokenPayMethod('balance');
                          setPayTokenEpayMethod('');
                          setPayTokenCheckout(null);
                          return;
                        }
                        if (
                          typeof selected === 'string' &&
                          selected.startsWith('epay:')
                        ) {
                          setPayTokenPayMethod('epay');
                          setPayTokenEpayMethod(selected.slice('epay:'.length));
                          setPayTokenCheckout(null);
                        }
                      }}
                    >
                      {epayEnabled &&
                        epayMethods.map((m) => (
                          <Radio key={m.type} value={`epay:${m.type}`}>
                            {m.name || m.type}
                          </Radio>
                        ))}
                      <Radio value='balance'>{t('账户余额')}</Radio>
                    </Radio.Group>
                  </Spin>
                  {!payConfigLoading && !epayEnabled && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('管理员未配置易支付')}
                    </div>
                  )}
                  {epayEnabled &&
                    !payConfigLoading &&
                    epayMethods.length === 0 && (
                      <div className='mt-2 text-xs text-gray-500'>
                        {t('管理员未配置易支付')}
                      </div>
                    )}
                  {payTokenPayMethod === 'balance' && payTokenAmountFen > 0 && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('需要支付')}: {renderCnyFen(payTokenAmountFen)}
                    </div>
                  )}
                  {payTokenPayMethod === 'balance' &&
                    payTokenAmountFen > 0 &&
                    balanceFen < payTokenAmountFen && (
                      <div className='mt-1 text-xs text-rose-600'>
                        {t('余额不足')}
                      </div>
                    )}
                </div>
              </Card>
            )}

            {payTokenCheckout && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='flex flex-col items-center gap-3'>
                  {(() => {
                    const methodType =
                      payTokenCheckout?.method || payTokenEpayMethod;
                    const scanTip = getEpayScanTip(methodType, t);
                    return (
                      <>
                        {scanTip ? (
                          <div className='text-xs text-gray-500'>{scanTip}</div>
                        ) : null}
                        {payTokenCheckout.payLink ? (
                          <QRCodeSVG
                            value={payTokenCheckout.payLink}
                            size={220}
                          />
                        ) : null}
                        <div className='flex items-center gap-2'>
                          <Button
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() =>
                              submitEpayForm(
                                payTokenCheckout.url,
                                payTokenCheckout.params,
                              )
                            }
                          >
                            {t('打开支付页')}
                          </Button>
                          <Button
                            className='!rounded-lg'
                            onClick={() => setPayTokenCheckout(null)}
                          >
                            {t('返回')}
                          </Button>
                        </div>
                        {payTokenCheckout.tradeNo && (
                          <div className='text-xs text-gray-500'>
                            {t('订单号')}: {payTokenCheckout.tradeNo}
                          </div>
                        )}
                      </>
                    );
                  })()}
                </div>
              </Card>
            )}
          </div>
        </Modal>

        <Modal
          title={t('按量付费充值')}
          visible={paygModalOpen}
          onOk={createPaygOrder}
          onCancel={closePaygModal}
          maskClosable={false}
          centered
          confirmLoading={paygSubmitting}
          okText={
            paygPayMethod === 'epay' && !paygCheckout
              ? t('去支付')
              : t('确认充值')
          }
          cancelText={t('取消')}
          {...(paygCheckout
            ? {
                footer: (
                  <div className='flex justify-end'>
                    <Button
                      type='primary'
                      theme='solid'
                      className='!rounded-lg'
                      onClick={closePaygModal}
                    >
                      {t('关闭')}
                    </Button>
                  </div>
                ),
              }
            : {})}
          okButtonProps={{
            disabled:
              Boolean(paygCheckout) ||
              payConfigLoading ||
              paygSubmitting ||
              paygAmountFen <= 0 ||
              !selectedPaygProduct?.id ||
              (paygPayMethod === 'epay' && (!epayEnabled || !paygEpayMethod)) ||
              (paygPayMethod === 'balance' && balanceFen < paygAmountFen),
          }}
        >
          <div className='space-y-4 pb-6'>
            {selectedPaygProduct?.id ? (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='space-y-1'>
                  <div className='flex items-center justify-between gap-3'>
                    <Text strong>{t('商品')}</Text>
                    <Text type='tertiary' size='small'>
                      #{selectedPaygProduct.id}
                    </Text>
                  </div>
                  <div className='text-sm font-medium'>
                    {selectedPaygProduct.name}
                  </div>
                  {selectedPaygProduct.description ? (
                    <div className='text-xs text-gray-500 leading-tight'>
                      {selectedPaygProduct.description}
                    </div>
                  ) : null}
                  {Array.isArray(selectedPaygProduct.allowed_group_ids) &&
                  getVisibleOptionalGroupIds(
                    selectedPaygProduct.allowed_group_ids,
                  ).length > 0 ? (
                    <div className='text-xs text-gray-500 leading-tight flex flex-wrap items-center gap-1 pt-1'>
                      {t('可选分组')}：
                      <span className='inline-flex flex-wrap gap-1'>
                        {getVisibleOptionalGroupIds(
                          selectedPaygProduct.allowed_group_ids,
                        ).map((gid) => (
                          <Text
                            key={`${selectedPaygProduct.id}-${gid}`}
                            code
                            style={{ fontSize: 12 }}
                          >
                            {`${getGroupLabel(gid)} * ${formatRatio(groupRatios?.[gid])}`}
                          </Text>
                        ))}
                      </span>
                    </div>
                  ) : null}
                </div>
              </Card>
            ) : null}
            <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-3'>
                  <Text strong>{t('充值金额')}</Text>
                  <div className='text-xs text-gray-500'>
                    {paygCreditUsdPerCny > 0
                      ? t('¥1 = ${{rate}} 额度', { rate: paygCreditUsdPerCny })
                      : t('按量付费兑换比例未配置')}
                  </div>
                </div>
                <div className='flex flex-wrap items-center gap-3'>
                  <InputNumber
                    min={0.01}
                    precision={2}
                    value={paygAmountYuan}
                    disabled={Boolean(paygCheckout)}
                    onChange={(v) => setPaygAmountYuan(v)}
                    className='w-full md:w-60 !rounded-lg'
                  />
                  <div className='text-sm text-gray-600'>
                    {t('预计获得')}:{' '}
                    <Text strong>{formatUsd(paygCreditUsd, 2)}</Text>
                  </div>
                </div>
                <div className='text-xs text-gray-500'>
                  {t('当前按量付费余额')}:{' '}
                  {renderQuotaToUSD(userState?.user?.payg_quota || 0, 2)}
                  {'  ·  '}
                  {t('账户余额')}: {renderCnyFen(balanceFen)}
                </div>
              </div>
            </Card>

            {!paygCheckout && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div>
                  <div className='mb-2 flex items-center justify-between gap-3'>
                    <Text strong>{t('支付方式')}</Text>
                    {paygPayMethod === 'balance' && (
                      <Text
                        size='small'
                        type='tertiary'
                        className='whitespace-nowrap'
                      >
                        {t('当前余额')}: {renderCnyFen(balanceFen)}
                      </Text>
                    )}
                  </div>
                  <Spin spinning={payConfigLoading}>
                    <Radio.Group
                      type='button'
                      value={
                        paygPayMethod === 'epay' && paygEpayMethod
                          ? `epay:${paygEpayMethod}`
                          : 'balance'
                      }
                      onChange={(val) => {
                        const selected =
                          val && val.target ? val.target.value : val;
                        if (selected === 'balance') {
                          setPaygPayMethod('balance');
                          setPaygEpayMethod('');
                          setPaygCheckout(null);
                          return;
                        }
                        if (
                          typeof selected === 'string' &&
                          selected.startsWith('epay:')
                        ) {
                          setPaygPayMethod('epay');
                          setPaygEpayMethod(selected.slice('epay:'.length));
                          setPaygCheckout(null);
                        }
                      }}
                    >
                      {epayEnabled &&
                        epayMethods.map((m) => (
                          <Radio key={m.type} value={`epay:${m.type}`}>
                            {m.name || m.type}
                          </Radio>
                        ))}
                      <Radio value='balance'>{t('账户余额')}</Radio>
                    </Radio.Group>
                  </Spin>
                  {!payConfigLoading && !epayEnabled && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('管理员未配置易支付')}
                    </div>
                  )}
                  {epayEnabled &&
                    !payConfigLoading &&
                    epayMethods.length === 0 && (
                      <div className='mt-2 text-xs text-gray-500'>
                        {t('管理员未配置易支付')}
                      </div>
                    )}
                  {paygPayMethod === 'balance' && paygAmountFen > 0 && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('需要支付')}: {renderCnyFen(paygAmountFen)}
                    </div>
                  )}
                  {paygPayMethod === 'balance' &&
                    paygAmountFen > 0 &&
                    balanceFen < paygAmountFen && (
                      <div className='mt-1 text-xs text-rose-600'>
                        {t('余额不足')}
                      </div>
                    )}
                </div>
              </Card>
            )}

            {paygCheckout &&
              (() => {
                const methodType = paygCheckout?.method || paygEpayMethod;
                const scanTip = getEpayScanTip(methodType, t);
                return (
                  <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                    <div className='flex flex-col items-center gap-3'>
                      {(() => {
                        if (paygPayStatus === 'success') {
                          return (
                            <div className='text-sm font-medium text-emerald-600'>
                              {t('充值成功')}
                            </div>
                          );
                        }
                        if (paygPayStatus === 'failed') {
                          return (
                            <div className='text-sm font-medium text-rose-600'>
                              {t('支付失败')}
                            </div>
                          );
                        }
                        return (
                          <div className='text-sm text-gray-600'>{scanTip}</div>
                        );
                      })()}
                      {paygCheckout.qrImageUrl ? (
                        <div className='rounded-xl bg-white p-3'>
                          <img
                            src={paygCheckout.qrImageUrl}
                            alt={scanTip}
                            className='h-[220px] w-[220px] rounded-lg object-contain'
                          />
                        </div>
                      ) : paygCheckout.qrCode ? (
                        <div className='rounded-xl bg-white p-3'>
                          <QRCodeSVG value={paygCheckout.qrCode} size={220} />
                        </div>
                      ) : (
                        <div className='text-sm text-gray-500'>
                          {t('后台没有返回可展示的支付二维码')}
                        </div>
                      )}
                      {paygPayStatus !== 'success' && (
                        <div className='flex flex-wrap justify-center gap-2'>
                          <Button
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            disabled={!paygCheckout.payPageUrl}
                            onClick={() =>
                              openEpayPayPage(paygCheckout.payPageUrl)
                            }
                          >
                            {t('打开支付页')}
                          </Button>
                          <Button
                            theme='light'
                            className='!rounded-lg'
                            onClick={() => setPaygCheckout(null)}
                          >
                            {t('返回修改')}
                          </Button>
                        </div>
                      )}
                      {paygCheckout.tradeNo && (
                        <div className='text-xs text-gray-500'>
                          {t('订单号')}: {paygCheckout.tradeNo}
                        </div>
                      )}
                      {paygPayStatus === 'pending' && (
                        <div className='flex items-center gap-2 text-xs text-gray-500'>
                          <Spin size='small' />
                          <span>{t('等待支付')}</span>
                        </div>
                      )}
                    </div>
                  </Card>
                );
              })()}
          </div>
        </Modal>

        <Modal
          title={t('购买订阅')}
          visible={orderModalOpen}
          onOk={createOrder}
          onCancel={closeOrderModal}
          maskClosable={false}
          centered
          confirmLoading={
            subscriptionCheckoutMode === 'traffic' ? false : orderSubmitting
          }
          okText={
            subscriptionCheckoutMode === 'traffic'
              ? undefined
              : payMethod === 'epay' && !epayCheckout
                ? t('去支付')
                : t('确认购买')
          }
          cancelText={
            subscriptionCheckoutMode === 'traffic' ? undefined : t('取消')
          }
          {...(subscriptionCheckoutMode === 'traffic'
            ? {
                footer: (
                  <div className='flex justify-end'>
                    <Button
                      type='primary'
                      theme='solid'
                      className='!rounded-lg'
                      onClick={() => setOrderModalOpen(false)}
                    >
                      {t('关闭')}
                    </Button>
                  </div>
                ),
              }
            : {})}
          {...(subscriptionCheckoutMode !== 'traffic' && epayCheckout
            ? {
                footer: (
                  <div className='flex justify-end'>
                    <Button
                      type='primary'
                      theme='solid'
                      className='!rounded-lg'
                      onClick={closeOrderModal}
                    >
                      {t('关闭')}
                    </Button>
                  </div>
                ),
              }
            : {})}
          {...(subscriptionCheckoutMode === 'traffic'
            ? {}
            : {
                okButtonProps: {
                  disabled:
                    Boolean(epayCheckout) ||
                    !orderingPlan ||
                    payConfigLoading ||
                    orderSubmitting ||
                    (payMethod === 'epay' && (!epayEnabled || !epayMethod)) ||
                    (payMethod === 'balance' && balanceFen < orderTotalFen),
                },
              })}
        >
          <div className='space-y-4 pb-6'>
            {subscriptionCheckoutMode !== 'traffic' && (
              <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                <div className='flex items-center justify-between gap-3'>
                  <div className='min-w-0'>
                    <Text strong>{orderingPlan?.name || '-'}</Text>
                    <div className='mt-1 space-y-0.5 text-xs text-gray-500 leading-tight'>
                      {orderingPlan?.description ? (
                        <div style={{ whiteSpace: 'pre-wrap' }}>
                          {orderingPlan.description}
                        </div>
                      ) : null}
                      {inferPresetMode(orderingPlan) === 'request' ? (
                        <>
                          <div>
                            {t('每日次数')}{' '}
                            {Number(orderingPlan?.daily_request_limit) === 0
                              ? t('无限')
                              : Number(orderingPlan?.daily_request_limit) || 0}
                          </div>
                          <div>
                            {t('总次数')}{' '}
                            {Number(orderingPlan?.quota) === 0
                              ? t('无限')
                              : Number(orderingPlan?.quota) || 0}
                          </div>
                        </>
                      ) : inferPresetMode(orderingPlan) === 'tokens' ? (
                        <>
                          <div>
                            {t('Tokens')}{' '}
                            {Number(orderingPlan?.quota) === 0
                              ? t('无限')
                              : `${Number(orderingPlan?.quota) || 0} tokens`}
                          </div>
                          {(() => {
                            const groupDailyLimits = normalizeGroupDailyLimits(
                              orderingPlan?.group_daily_limits,
                            );
                            if (groupDailyLimits.length > 0) {
                              return (
                                <div>
                                  <div>{t('日限（按分组）')}</div>
                                  <div className='mt-0.5 flex flex-wrap items-center gap-1'>
                                    {groupDailyLimits.map((item) => (
                                      <Text
                                        key={`order-${orderingPlan?.id}-${item.group_id}`}
                                        code
                                        style={{ fontSize: 12 }}
                                      >
                                        {`${getGroupLabel(item.group_id)}: ${
                                          item.daily_quota_limit <= 0
                                            ? t('无限')
                                            : `${item.daily_quota_limit || 0} tokens`
                                        }`}
                                      </Text>
                                    ))}
                                  </div>
                                </div>
                              );
                            }
                            return (
                              <div>
                                {t('日限')}{' '}
                                {Number(orderingPlan?.daily_quota_limit) <= 0
                                  ? t('无限')
                                  : `${Number(orderingPlan?.daily_quota_limit) || 0} tokens`}
                              </div>
                            );
                          })()}
                        </>
                      ) : (
                        <>
                          <div>
                            {t('额度')}{' '}
                            {Number(orderingPlan?.quota) === 0
                              ? t('无限')
                              : renderQuotaToUSD(orderingPlan?.quota || 0, 2)}
                          </div>
                          {(() => {
                            const groupDailyLimits = normalizeGroupDailyLimits(
                              orderingPlan?.group_daily_limits,
                            );
                            if (groupDailyLimits.length > 0) {
                              return (
                                <div>
                                  <div>{t('日限（按分组）')}</div>
                                  <div className='mt-0.5 flex flex-wrap items-center gap-1'>
                                    {groupDailyLimits.map((item) => (
                                      <Text
                                        key={`order-${orderingPlan?.id}-${item.group_id}`}
                                        code
                                        style={{ fontSize: 12 }}
                                      >
                                        {`${getGroupLabel(item.group_id)}: ${
                                          item.daily_quota_limit <= 0
                                            ? t('无限')
                                            : renderQuotaToUSD(
                                                item.daily_quota_limit || 0,
                                                2,
                                              )
                                        }`}
                                      </Text>
                                    ))}
                                  </div>
                                </div>
                              );
                            }
                            return (
                              <div>
                                {t('日限')}{' '}
                                {Number(orderingPlan?.daily_quota_limit) <= 0
                                  ? t('无限')
                                  : renderQuotaToUSD(
                                      orderingPlan?.daily_quota_limit || 0,
                                      2,
                                    )}
                              </div>
                            );
                          })()}
                        </>
                      )}
                      <div>
                        {t('时长')}{' '}
                        {Number(orderingPlan?.quota_valid_days) === 0
                          ? t('无限')
                          : `${orderingPlan?.quota_valid_days || 0} ${t('天')}`}
                      </div>
                    </div>
                  </div>
                  <div className='shrink-0 text-right'>
                    <div className='text-base font-semibold text-rose-600'>
                      {renderCnyFen(orderTotalFen)}
                    </div>
                    {orderingPlan?.multi_quantity_enabled ? (
                      <div className='mt-0.5 text-xs text-gray-500'>
                        {t('单价')}:{' '}
                        {renderCnyFen(orderingPlan?.price_fen || 0)} ×{' '}
                        {(() => {
                          const qtyRaw = Number(orderQuantity);
                          return Number.isFinite(qtyRaw)
                            ? Math.max(1, Math.floor(qtyRaw))
                            : 1;
                        })()}
                      </div>
                    ) : null}
                  </div>
                </div>
              </Card>
            )}

            {subscriptionCheckoutMode === 'traffic' ? (
              <div className='space-y-3'>
                {subscriptionTrafficMessage && (
                  <div
                    className='text-sm text-gray-600'
                    style={{ whiteSpace: 'pre-wrap' }}
                  >
                    {subscriptionTrafficMessage}
                  </div>
                )}

                {subscriptionTrafficQRCode && (
                  <div className='flex justify-center'>
                    <img
                      src={subscriptionTrafficQRCode}
                      alt='qrcode'
                      style={{
                        maxWidth: 260,
                        maxHeight: 260,
                        borderRadius: 12,
                      }}
                    />
                  </div>
                )}

                {!subscriptionTrafficMessage && !subscriptionTrafficQRCode && (
                  <div className='text-sm text-gray-500'>
                    {t('暂未配置引流内容')}
                  </div>
                )}
              </div>
            ) : (
              <>
                {orderingPlan?.multi_quantity_enabled ? (
                  <div>
                    <div className='mb-2 flex items-center justify-between gap-3'>
                      <Text strong>{t('购买数量')}</Text>
                    </div>
                    <InputNumber
                      value={orderQuantity}
                      min={1}
                      max={maxOrderQuantity}
                      precision={0}
                      disabled={
                        Boolean(epayCheckout) ||
                        orderSubmitting ||
                        payConfigLoading
                      }
                      onChange={(v) => {
                        const raw = Number(v);
                        const next = Number.isFinite(raw)
                          ? Math.min(
                              Math.max(1, Math.floor(raw)),
                              maxOrderQuantity,
                            )
                          : 1;
                        setOrderQuantity(next);
                        if (
                          next > 1 &&
                          orderingPlan?.multi_quantity_defer_only !== false
                        ) {
                          setApplyMode('defer');
                        }
                      }}
                      style={{ width: '100%' }}
                    />
                    {(() => {
                      const hasPurchaseLimit =
                        Number(orderingPlan?.purchase_limit) > 0;
                      const stock = normalizeStockValue(orderingPlan?.stock);
                      const hasStockLimit = typeof stock === 'number';
                      if (!hasPurchaseLimit && !hasStockLimit) return null;
                      return (
                        <div className='mt-2 text-xs text-gray-500'>
                          {t('最多可购买')} {maxOrderQuantity} {t('份')}
                        </div>
                      );
                    })()}
                  </div>
                ) : null}
                <div>
                  <div className='mb-2 flex items-center justify-between gap-3'>
                    <Text strong>{t('生效方式')}</Text>
                  </div>
                  <Radio.Group
                    type='button'
                    value={applyMode}
                    onChange={(val) => {
                      const selected =
                        val && val.target ? val.target.value : val;
                      if (
                        selected === 'stack' &&
                        orderingPlan?.multi_quantity_defer_only !== false
                      ) {
                        setApplyMode('defer');
                        return;
                      }
                      setApplyMode(selected);
                    }}
                  >
                    <Radio
                      value='stack'
                      disabled={
                        orderingPlan?.multi_quantity_defer_only !== false
                      }
                    >
                      {t('叠加（立即生效）')}
                    </Radio>
                    <Radio value='defer'>{t('顺延（到期后生效）')}</Radio>
                  </Radio.Group>
                  <div className='mt-2 text-xs text-gray-500'>
                    {applyMode === 'defer'
                      ? t(
                          '若当前仍有有效订阅额度（不含自由额度），新购买的额度包将从当前订阅到期后开始计算有效期',
                        )
                      : t('订阅额度购买后将立即生效')}
                  </div>
                  {orderingPlan?.multi_quantity_defer_only !== false ? (
                    <div className='mt-1 text-xs text-gray-500'>
                      {t('仅支持顺延')}
                    </div>
                  ) : null}
                </div>

                <div>
                  <div className='mb-2 flex items-center justify-between gap-3'>
                    <Text strong>{t('支付方式')}</Text>
                    {payMethod === 'balance' && (
                      <Text
                        size='small'
                        type='tertiary'
                        className='whitespace-nowrap'
                      >
                        {t('当前余额')}: {renderCnyFen(balanceFen)}
                      </Text>
                    )}
                  </div>
                  <Spin spinning={payConfigLoading}>
                    <Radio.Group
                      type='button'
                      value={
                        payMethod === 'epay' && epayMethod
                          ? `epay:${epayMethod}`
                          : 'balance'
                      }
                      onChange={(val) => {
                        const selected =
                          val && val.target ? val.target.value : val;
                        if (selected === 'balance') {
                          setPayMethod('balance');
                          setEpayMethod('');
                          setEpayCheckout(null);
                          return;
                        }
                        if (
                          typeof selected === 'string' &&
                          selected.startsWith('epay:')
                        ) {
                          setPayMethod('epay');
                          setEpayMethod(selected.slice('epay:'.length));
                          setEpayCheckout(null);
                        }
                      }}
                    >
                      {epayEnabled &&
                        epayMethods.map((m) => (
                          <Radio key={m.type} value={`epay:${m.type}`}>
                            {m.name || m.type}
                          </Radio>
                        ))}
                      <Radio value='balance'>{t('账户余额')}</Radio>
                    </Radio.Group>
                  </Spin>
                  {!payConfigLoading && !epayEnabled && (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('管理员未配置易支付')}
                    </div>
                  )}
                  {epayEnabled &&
                    !payConfigLoading &&
                    epayMethods.length === 0 && (
                      <div className='mt-2 text-xs text-gray-500'>
                        {t('管理员未配置易支付')}
                      </div>
                    )}
                </div>

                {epayCheckout && (
                  <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
                    <div className='flex flex-col items-center gap-3'>
                      {(() => {
                        const methodType = epayCheckout?.method || epayMethod;
                        const scanTip = getEpayScanTip(methodType, t);
                        if (epayPayStatus === 'success') {
                          return (
                            <div className='text-sm font-medium text-emerald-600'>
                              {t('支付成功，订阅已生效')}
                            </div>
                          );
                        }
                        if (epayPayStatus === 'failed') {
                          return (
                            <div className='text-sm font-medium text-rose-600'>
                              {t('支付失败')}
                            </div>
                          );
                        }
                        return (
                          <div className='text-sm text-gray-600'>{scanTip}</div>
                        );
                      })()}
                      {epayCheckout.payLink ? (
                        <div className='rounded-xl bg-white p-3'>
                          <QRCodeSVG value={epayCheckout.payLink} size={220} />
                        </div>
                      ) : (
                        <div className='text-sm text-gray-500'>
                          {t('生成二维码失败，请尝试打开支付页')}
                        </div>
                      )}
                      {epayPayStatus !== 'success' && (
                        <div className='flex flex-wrap justify-center gap-2'>
                          <Button
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() =>
                              submitEpayForm(
                                epayCheckout.url,
                                epayCheckout.params,
                              )
                            }
                          >
                            {t('打开支付页')}
                          </Button>
                          <Button
                            theme='light'
                            className='!rounded-lg'
                            onClick={() => setEpayCheckout(null)}
                          >
                            {t('返回修改')}
                          </Button>
                        </div>
                      )}
                      {epayCheckout.tradeNo && (
                        <div className='text-xs text-gray-500'>
                          {t('订单号')}: {epayCheckout.tradeNo}
                        </div>
                      )}
                      {epayPayStatus === 'pending' && (
                        <div className='flex items-center gap-2 text-xs text-gray-500'>
                          <Spin size='small' />
                          <span>{t('等待支付')}</span>
                        </div>
                      )}
                    </div>
                  </Card>
                )}
              </>
            )}
          </div>
        </Modal>

        <div className='space-y-6'>
          <div className='space-y-4 w-full'>
            <Card className='!rounded-2xl !border-0 !shadow-none'>
              <div className='flex items-center justify-between'>
                <div>
                  <Text className='text-lg font-medium'>{t('订阅商城')}</Text>
                  <div className='text-xs text-gray-500 space-y-1'>
                    {subscriptionStoreNoticeIsHtml &&
                    subscriptionStoreNoticeHtml ? (
                      <div
                        style={{ whiteSpace: 'pre-wrap' }}
                        dangerouslySetInnerHTML={{
                          __html: subscriptionStoreNoticeHtml,
                        }}
                      />
                    ) : subscriptionStoreNoticeLines.length > 0 ? (
                      subscriptionStoreNoticeLines.map((line, index) => (
                        <div key={`subscription-store-notice-${index}`}>
                          {renderStoreNoticeLine(line)}
                        </div>
                      ))
                    ) : (
                      <>
                        <div>
                          {t('分组倍率')}，{t('模型价格请移步至')}
                          <Link
                            to='/console/pricing'
                            className='text-white underline decoration-white underline-offset-2 hover:opacity-90'
                          >
                            {t('模型广场')}
                          </Link>
                          {shouldAddSpaceBeforeView ? ' ' : ''}
                          {t('查看')}
                        </div>
                        <div>
                          {t(
                            '创建令牌时需选择已购订阅内所包含的分组，否则会导致无法消费',
                          )}
                        </div>
                      </>
                    )}
                  </div>
                </div>
                <div className='flex items-center rounded-lg overflow-hidden bg-semi-color-fill-0 dark:bg-semi-color-fill-1'>
                  <button
                    className={`p-2 transition-colors ${viewMode === 'card' ? 'bg-semi-color-fill-2 dark:bg-semi-color-fill-2' : 'hover:bg-semi-color-fill-1 dark:hover:bg-semi-color-fill-2'}`}
                    onClick={() => setViewMode('card')}
                    title={t('卡片视图')}
                  >
                    <LayoutGrid size={18} />
                  </button>
                  <button
                    className={`p-2 transition-colors ${viewMode === 'list' ? 'bg-semi-color-fill-2 dark:bg-semi-color-fill-2' : 'hover:bg-semi-color-fill-1 dark:hover:bg-semi-color-fill-2'}`}
                    onClick={() => setViewMode('list')}
                    title={t('列表视图')}
                  >
                    <List size={18} />
                  </button>
                </div>
              </div>
            </Card>
            <Spin spinning={plansLoading}>
              {viewMode === 'card' ? planCards : planListView}
            </Spin>
          </div>
        </div>
      </div>
    </ConsolePage>
  );
};

export default Subscription;
