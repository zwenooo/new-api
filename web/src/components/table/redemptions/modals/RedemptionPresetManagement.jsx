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
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  API,
  copy,
  downloadTextAsFile,
  inferPresetMode,
  renderCnyFen,
  showError,
  showSuccess,
  timestamp2string,
} from '../../../../helpers';
import {
  Avatar,
  Button,
  Card,
  Empty,
  Modal,
  Popconfirm,
  SideSheet,
  Space,
  Spin,
  Switch,
  Tag,
  Typography,
  InputNumber,
} from '@douyinfe/semi-ui';
import { IconCopy, IconGift, IconPlus } from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardTable from '../../../common/ui/CardTable';
import EditRedemptionPresetModal from './EditRedemptionPresetModal';

const { Text, Title } = Typography;

const samePresetId = (a, b) => {
  const ida = Number(a);
  const idb = Number(b);
  return Number.isFinite(ida) && Number.isFinite(idb) && ida > 0 && ida === idb;
};

const normalizeGroupIds = (rawIds) => {
  const list = Array.isArray(rawIds) ? rawIds : [];
  const out = [];
  const seen = new Set();
  list.forEach((raw) => {
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

const RedemptionPresetManagement = forwardRef(
  (
    {
      visible,
      embedded = false,
      embeddedActionsVisible = true,
      allowedModes,
      modeLocked,
      onClose,
      onGenerated,
    },
    ref,
  ) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [presets, setPresets] = useState([]);
  const [availableGroups, setAvailableGroups] = useState([]);
  const lockedMode = String(modeLocked || '').trim();
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

  const displayPresets = useMemo(() => {
    const list = Array.isArray(presets) ? presets : [];
    if (!allowedModeSet) return list;
    return list.filter((p) => allowedModeSet.has(inferPresetMode(p)));
  }, [allowedModeSet, presets]);

  const DEFAULT_QUOTA_PER_USD = 500000; // 本地存储缺失时的美元兑额度后备比例

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

  const formatUSD = (usdValue, digits = 4) => {
    const num = Number(usdValue);
    if (!Number.isFinite(num) || num <= 0) {
      return '$0';
    }
    return `$${num
      .toFixed(digits)
      .replace(/\.?0+$/, '')}`;
  };

  const renderQuotaUSD = (quotaTokens) => {
    const quotaNumber = Number(quotaTokens);
    if (!Number.isFinite(quotaNumber) || quotaNumber <= 0) {
      return '$0';
    }
    const perUnit = getQuotaPerUnit();
    if (!perUnit) {
      return '$0';
    }
    return formatUSD(quotaNumber / perUnit);
  };

  const [showEdit, setShowEdit] = useState(false);
  const [editingPreset, setEditingPreset] = useState({ id: undefined });

  const loadPresets = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/redemption/presets');
      const { success, message, data } = res.data;
      if (success) {
        setPresets(Array.isArray(data) ? data : []);
      } else {
        showError(message || t('获取失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get('/api/group/');
      const { success, message, data } = res.data;
      if (success) {
        setAvailableGroups(Array.isArray(data) ? data : []);
      } else {
        showError(message || t('获取分组失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
    }
  }, [t]);

  const groupLabelById = useMemo(() => {
    const map = {};
    (Array.isArray(availableGroups) ? availableGroups : []).forEach((g) => {
      const idRaw = Number(g?.id ?? 0);
      const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
      if (id <= 0) return;
      const label = String(g?.display_name || g?.name || '').trim();
      const code = String(g?.code || '').trim();
      const name = label || code;
      if (!name) return;
      map[id] = name;
    });
    return map;
  }, [availableGroups]);

  const deletePreset = async (id) => {
    try {
      const res = await API.delete(`/api/redemption/presets/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('删除成功'));
        loadPresets();
      } else {
        showError(message || t('删除失败'));
      }
    } catch (e) {
      showError(e?.message || t('删除失败'));
    }
  };

  const openCreate = (mode) => {
    const nextMode = lockedMode || String(mode || '').trim();
    setEditingPreset(nextMode ? { id: undefined, mode: nextMode } : { id: undefined });
    setShowEdit(true);
  };

  const openEdit = (preset) => {
    setEditingPreset(preset || { id: undefined });
    setShowEdit(true);
  };

  const closeEdit = () => {
    setShowEdit(false);
    setTimeout(() => setEditingPreset({ id: undefined }), 300);
  };

  useImperativeHandle(ref, () => ({
    openCreate,
    reload: loadPresets,
  }));

  const copyPreset = async (preset) => {
    if (!preset?.name) return;
    const mode = inferPresetMode(preset);
    const payload = {
      name: `${preset.name}-复制`,
      description: String(preset.description || ''),
      mode,
      enabled: preset.enabled !== false,
      multi_quantity_enabled: Boolean(preset.multi_quantity_enabled),
      multi_quantity_defer_only: preset.multi_quantity_defer_only !== false,
      sort_order: Number(preset.sort_order) || 0,
      price_fen: Number(preset.price_fen) || 0,
      purchase_limit: Number(preset.purchase_limit) || 0,
      quota: Number(preset.quota) || 0,
      daily_quota_limit: Number(preset.daily_quota_limit) || 0,
      quota_valid_days: Number(preset.quota_valid_days) || 0,
      plan_valid_days: Number(preset.plan_valid_days) || 0,
      channel_ids: Array.isArray(preset.channel_ids) ? preset.channel_ids : [],
      allowed_group_ids: Array.isArray(preset.allowed_group_ids)
        ? preset.allowed_group_ids
        : [],
      expired_time: Number(preset.expired_time) || 0,
    };

    try {
      const res = await API.post('/api/redemption/presets', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('复制成功'));
        loadPresets();
      } else {
        showError(message || t('复制失败'));
      }
    } catch (e) {
      showError(e?.message || t('复制失败'));
    }
  };

  const updatePresetEnabled = async (preset, enabled) => {
    if (!preset?.id || !preset?.name) return;
    const mode = inferPresetMode(preset);
    const payload = {
      id: preset.id,
      name: preset.name,
      description: String(preset.description || ''),
      mode,
      enabled: Boolean(enabled),
      multi_quantity_enabled: Boolean(preset.multi_quantity_enabled),
      multi_quantity_defer_only: preset.multi_quantity_defer_only !== false,
      sort_order: Number(preset.sort_order) || 0,
      price_fen: Number(preset.price_fen) || 0,
      purchase_limit: Number(preset.purchase_limit) || 0,
      quota: Number(preset.quota) || 0,
      daily_quota_limit: Number(preset.daily_quota_limit) || 0,
      quota_valid_days: Number(preset.quota_valid_days) || 0,
      plan_valid_days: Number(preset.plan_valid_days) || 0,
      channel_ids: Array.isArray(preset.channel_ids) ? preset.channel_ids : [],
      allowed_group_ids: Array.isArray(preset.allowed_group_ids)
        ? preset.allowed_group_ids
        : [],
      expired_time: Number(preset.expired_time) || 0,
    };
    setLoading(true);
    try {
      const res = await API.post('/api/redemption/presets', payload);
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        if (data) {
          setPresets((prev) =>
            (Array.isArray(prev) ? prev : []).map((item) =>
              samePresetId(item?.id, preset.id) ? data : item,
            ),
          );
        }
      } else {
        showError(message || t('保存失败'));
      }
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setLoading(false);
    }
  };

  const updatePresetMultiQuantity = async (preset, enabled) => {
    if (!preset?.id || !preset?.name) return;
    setPresets((prev) =>
      (Array.isArray(prev) ? prev : []).map((item) =>
        samePresetId(item?.id, preset.id)
          ? { ...item, multi_quantity_enabled: Boolean(enabled) }
          : item,
      ),
    );
    const mode = inferPresetMode(preset);
    const payload = {
      id: preset.id,
      name: preset.name,
      description: String(preset.description || ''),
      mode,
      enabled: preset.enabled !== false,
      multi_quantity_enabled: Boolean(enabled),
      multi_quantity_defer_only: preset.multi_quantity_defer_only !== false,
      sort_order: Number(preset.sort_order) || 0,
      price_fen: Number(preset.price_fen) || 0,
      purchase_limit: Number(preset.purchase_limit) || 0,
      quota: Number(preset.quota) || 0,
      daily_quota_limit: Number(preset.daily_quota_limit) || 0,
      quota_valid_days: Number(preset.quota_valid_days) || 0,
      plan_valid_days: Number(preset.plan_valid_days) || 0,
      channel_ids: Array.isArray(preset.channel_ids) ? preset.channel_ids : [],
      allowed_group_ids: Array.isArray(preset.allowed_group_ids)
        ? preset.allowed_group_ids
        : [],
      expired_time: Number(preset.expired_time) || 0,
    };
    setLoading(true);
    try {
      const res = await API.post('/api/redemption/presets', payload);
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        if (data) {
          setPresets((prev) =>
            (Array.isArray(prev) ? prev : []).map((item) =>
              samePresetId(item?.id, preset.id) ? data : item,
            ),
          );
        }
      } else {
        showError(message || t('保存失败'));
        loadPresets();
      }
    } catch (e) {
      showError(e?.message || t('保存失败'));
      loadPresets();
    } finally {
      setLoading(false);
    }
  };

  const updatePresetMultiQuantityDeferOnly = async (preset, enabled) => {
    if (!preset?.id || !preset?.name) return;
    setPresets((prev) =>
      (Array.isArray(prev) ? prev : []).map((item) =>
        samePresetId(item?.id, preset.id)
          ? { ...item, multi_quantity_defer_only: Boolean(enabled) }
          : item,
      ),
    );
    const mode = inferPresetMode(preset);
    const payload = {
      id: preset.id,
      name: preset.name,
      description: String(preset.description || ''),
      mode,
      enabled: preset.enabled !== false,
      multi_quantity_enabled: Boolean(preset.multi_quantity_enabled),
      multi_quantity_defer_only: Boolean(enabled),
      sort_order: Number(preset.sort_order) || 0,
      price_fen: Number(preset.price_fen) || 0,
      purchase_limit: Number(preset.purchase_limit) || 0,
      quota: Number(preset.quota) || 0,
      daily_quota_limit: Number(preset.daily_quota_limit) || 0,
      quota_valid_days: Number(preset.quota_valid_days) || 0,
      plan_valid_days: Number(preset.plan_valid_days) || 0,
      channel_ids: Array.isArray(preset.channel_ids) ? preset.channel_ids : [],
      allowed_group_ids: Array.isArray(preset.allowed_group_ids)
        ? preset.allowed_group_ids
        : [],
      expired_time: Number(preset.expired_time) || 0,
    };
    setLoading(true);
    try {
      const res = await API.post('/api/redemption/presets', payload);
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        if (data) {
          setPresets((prev) =>
            (Array.isArray(prev) ? prev : []).map((item) =>
              samePresetId(item?.id, preset.id) ? data : item,
            ),
          );
        }
      } else {
        showError(message || t('保存失败'));
        loadPresets();
      }
    } catch (e) {
      showError(e?.message || t('保存失败'));
      loadPresets();
    } finally {
      setLoading(false);
    }
  };

  const generateByPreset = (preset) => {
    if (!preset?.name) return;
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
        const res = await API.post('/api/redemption/presets/generate', {
          name: preset.name,
          count,
        });
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('生成失败'));
          return;
        }

        const keys = Array.isArray(data) ? data : [];
        showSuccess(t('生成成功'));
        onGenerated && onGenerated();

        if (keys.length > 0) {
          const text = keys.join('\n') + '\n';
          Modal.confirm({
            title: t('兑换码创建成功'),
            content: (
              <div>
                <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
                <p>{t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}</p>
              </div>
            ),
            onOk: () => {
              downloadTextAsFile(text, `${preset.name}.txt`);
            },
          });
        }
      },
    });
  };

  const columns = useMemo(() => {
    return [
      {
        title: t('名称'),
        dataIndex: 'name',
        key: 'name',
        render: (text) => (
          <Space spacing={6}>
            <Text strong>{text}</Text>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={<IconCopy />}
              onClick={async (e) => {
                e.stopPropagation();
                const ok = await copy(text);
                if (ok) {
                  showSuccess(t('复制成功'));
                } else {
                  showError(t('复制失败'));
                }
              }}
            />
          </Space>
        ),
      },
      {
        title: t('价格'),
        dataIndex: 'price_fen',
        key: 'price_fen',
        render: (text) => {
          const fen = parseInt(text, 10) || 0;
          return fen > 0 ? (
            <Tag color='grey' shape='circle'>
              {renderCnyFen(fen)}
            </Tag>
          ) : (
            <Text type='tertiary'>-</Text>
          );
        },
      },
      {
        title: t('排序'),
        dataIndex: 'sort_order',
        key: 'sort_order',
        width: 90,
        render: (v) => (
          <Tag color='grey' shape='circle'>
            {Number(v) || 0}
          </Tag>
        ),
      },
      {
        title: t('规格'),
        key: 'spec',
        render: (text, record) => {
          const mode = inferPresetMode(record);
          if (mode === 'subscription') {
            const limit = Number(record.purchase_limit) || 0;
            const dailyLimit = Number(record.daily_quota_limit) || 0;
            const groupIds = normalizeGroupIds(record.allowed_group_ids);
            const groups = groupIds.map(
              (id) => groupLabelById[id] || t('未知分组'),
            );
            return (
              <Text type='tertiary'>
                {t('额度')} {renderQuotaUSD(record.quota || 0)} / {t('日限')}{' '}
                {dailyLimit <= 0 ? t('无限') : renderQuotaUSD(dailyLimit)} /{' '}
                {t('有效期')} {record.quota_valid_days || 0} {t('天')} /{' '}
                {t('可用分组')} {groups.length > 0 ? groups.join(', ') : t('未配置')}
                {limit > 0 ? ` / ${t('限购')} ${limit} ${t('次')}` : ''}
              </Text>
            );
          }
          if (mode === 'tokens') {
            const limit = Number(record.purchase_limit) || 0;
            const dailyLimit = Number(record.daily_quota_limit) || 0;
            const groupIds = normalizeGroupIds(record.allowed_group_ids);
            const groups = groupIds.map(
              (id) => groupLabelById[id] || t('未知分组'),
            );
            const total = Number(record.quota) || 0;
            return (
              <Text type='tertiary'>
                {t('Tokens')}{' '}
                {total <= 0 ? t('无限') : total.toLocaleString()} / {t('日限')}{' '}
                {dailyLimit <= 0 ? t('无限') : dailyLimit.toLocaleString()} /{' '}
                {t('有效期')} {record.quota_valid_days || 0} {t('天')} /{' '}
                {t('可用分组')} {groups.length > 0 ? groups.join(', ') : t('未配置')}
                {limit > 0 ? ` / ${t('限购')} ${limit} ${t('次')}` : ''}
              </Text>
            );
          }
          if (mode === 'payg') {
            const groupIds = normalizeGroupIds(record.allowed_group_ids);
            const groups = groupIds.map(
              (id) => groupLabelById[id] || t('未知分组'),
            );
            return (
              <Text type='tertiary'>
                {t('额度')} {renderQuotaUSD(record.quota || 0)} / {t('可用分组')}{' '}
                {groups.length > 0 ? groups.join(', ') : '-'}
              </Text>
            );
          }
          return (
            <Text type='tertiary'>
              {t('额度')} {renderQuotaUSD(record.quota || 0)}
            </Text>
          );
        },
      },
      {
        title: t('出售'),
        key: 'enabled',
        width: 90,
        render: (_, record) => {
          const mode = inferPresetMode(record);
          if (mode !== 'subscription' && mode !== 'tokens') {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Switch
              checked={record?.enabled !== false}
              disabled={loading}
              onChange={(v) => updatePresetEnabled(record, v)}
            />
          );
        },
      },
      {
        title: t('多数量'),
        key: 'multi_quantity_enabled',
        width: 110,
        render: (_, record) => {
          const mode = inferPresetMode(record);
          if (mode !== 'subscription' && mode !== 'tokens') {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Switch
              checked={Boolean(record?.multi_quantity_enabled)}
              disabled={loading}
              onChange={(v) => updatePresetMultiQuantity(record, v)}
            />
          );
        },
      },
      {
        title: t('仅顺延'),
        key: 'multi_quantity_defer_only',
        width: 110,
        render: (_, record) => {
          const mode = inferPresetMode(record);
          if (mode !== 'subscription' && mode !== 'tokens') {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Switch
              checked={record?.multi_quantity_defer_only !== false}
              disabled={loading}
              onChange={(v) => updatePresetMultiQuantityDeferOnly(record, v)}
            />
          );
        },
      },
      {
        title: t('过期时间'),
        dataIndex: 'expired_time',
        key: 'expired_time',
        render: (text) => {
          const ts = parseInt(text, 10) || 0;
          return <div>{ts === 0 ? t('永不过期') : timestamp2string(ts)}</div>;
        },
      },
      {
        title: t('更新时间'),
        dataIndex: 'updated_time',
        key: 'updated_time',
        render: (text) => {
          const ts = parseInt(text, 10) || 0;
          return <div>{ts > 0 ? timestamp2string(ts) : '-'}</div>;
        },
      },
      {
        title: '',
        key: 'action',
        fixed: 'right',
        width: 200,
        render: (_, record) => (
          <Space>
            <Button size='small' onClick={() => generateByPreset(record)}>
              {t('生成兑换码')}
            </Button>
            <Button size='small' onClick={() => copyPreset(record)}>
              {t('复制')}
            </Button>
            <Button size='small' onClick={() => openEdit(record)}>
              {t('编辑')}
            </Button>
            <Popconfirm
              title={t('确定删除？')}
              onConfirm={() => deletePreset(record.id)}
            >
              <Button size='small' type='danger'>
                {t('删除')}
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];
  }, [groupLabelById, loading, t]);

  useEffect(() => {
    if (visible || embedded) {
      loadPresets();
      loadGroups();
    }
  }, [embedded, loadGroups, loadPresets, visible]);

  const embeddedHeader = (
    <div className='flex items-start justify-between gap-4'>
      <div className='min-w-0'>
        <Title heading={5} style={{ margin: 0 }}>
          {t('预置商品')}
        </Title>
        <Text type='tertiary' size='small'>
          {t('维护固定规格与价格，一键生成兑换码')}
        </Text>
      </div>
      {embeddedActionsVisible ? (
        <div className='shrink-0 flex items-center gap-2'>
          <Button icon={<IconPlus />} onClick={() => openCreate()}>
            {t('新增预置商品')}
          </Button>
          <Button loading={loading} onClick={loadPresets}>
            {t('刷新')}
          </Button>
        </div>
      ) : null}
    </div>
  );

  const embeddedBody = (
    <Spin spinning={loading}>
      <div className='mt-4'>
        <Card className='!rounded-2xl shadow-sm border-0'>
          {displayPresets.length === 0 ? (
            <Empty
              image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无预置商品')}
              style={{ padding: 30 }}
            />
          ) : (
            <CardTable
              columns={columns}
              dataSource={displayPresets}
              rowKey='id'
              pagination={false}
              hidePagination
              size='small'
              scroll={{ x: 'max-content' }}
            />
          )}
        </Card>
      </div>
    </Spin>
  );

  return (
    <>
      <EditRedemptionPresetModal
        visible={showEdit}
        editingPreset={editingPreset}
        allowedModes={allowedModes}
        modeLocked={modeLocked}
        onClose={closeEdit}
        onSuccess={loadPresets}
      />

      {embedded ? (
        <div>
          {embeddedHeader}
          {embeddedBody}
        </div>
      ) : null}

      <SideSheet
        title={
          <div className='flex items-center gap-3'>
            <Avatar size='small' color='green' className='shadow-md'>
              <IconGift size={16} />
            </Avatar>
            <div>
              <Title heading={5} style={{ margin: 0 }}>
                {t('预置商品')}
              </Title>
              <Text type='tertiary' size='small'>
                {t('维护固定规格与价格，一键生成兑换码')}
              </Text>
            </div>
          </div>
        }
        visible={visible && !embedded}
        placement={isMobile ? 'bottom' : 'right'}
        onCancel={onClose}
        closeIcon={null}
        footer={
          <div className='flex justify-between items-center'>
            <Button icon={<IconPlus />} onClick={() => openCreate()}>
              {t('新增预置商品')}
            </Button>
            <Button onClick={onClose}>{t('关闭')}</Button>
          </div>
        }
        size={isMobile ? 'large' : 980}
      >
        <Spin spinning={loading}>
          <div className='p-2'>
            <Card className='!rounded-2xl shadow-sm border-0'>
              {displayPresets.length === 0 ? (
                <Empty
                  image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
                  darkModeImage={
                    <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
                  }
                  description={t('暂无预置商品')}
                  style={{ padding: 30 }}
                />
              ) : (
                <CardTable
                  columns={columns}
                  dataSource={displayPresets}
                  rowKey='id'
                  pagination={false}
                  hidePagination
                  size='small'
                  scroll={{ x: 'max-content' }}
                />
              )}
            </Card>
          </div>
        </Spin>
      </SideSheet>
    </>
  );
  },
);

export default RedemptionPresetManagement;
