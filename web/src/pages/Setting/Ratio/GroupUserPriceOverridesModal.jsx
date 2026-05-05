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
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

import { API, showError, showSuccess } from '../../../helpers';

const { Text } = Typography;

const normalizeEntries = (raw) => {
  const list = Array.isArray(raw) ? raw : [];
  const seen = new Set();
  return list
    .map((item) => {
      const userId = Number(item?.user_id ?? item?.userId ?? 0);
      const factor = Number(item?.factor ?? 0);
      if (
        !Number.isInteger(userId) ||
        userId <= 0 ||
        !Number.isFinite(factor)
      ) {
        return null;
      }
      if (seen.has(userId)) return null;
      seen.add(userId);
      return {
        user_id: userId,
        username: String(item?.username || '').trim(),
        display_name: String(item?.display_name || '').trim(),
        email: String(item?.email || '').trim(),
        factor,
      };
    })
    .filter(Boolean)
    .sort((a, b) => a.user_id - b.user_id);
};

const displayUserLabel = (record) =>
  String(
    record?.display_name || record?.username || record?.email || '',
  ).trim() || `#${record?.user_id ?? 0}`;

const formatRatio = (value) => {
  const factor = Number(value);
  if (!Number.isFinite(factor)) return '1';
  return factor.toFixed(6).replace(/\.?0+$/, '');
};

export default function GroupUserPriceOverridesModal({
  visible,
  group,
  onClose,
  onSaved,
}) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [entries, setEntries] = useState([]);
  const [originEntries, setOriginEntries] = useState([]);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searching, setSearching] = useState(false);
  const [searchOptions, setSearchOptions] = useState([]);
  const [selectedUserId, setSelectedUserId] = useState(0);
  const [selectedUserRecord, setSelectedUserRecord] = useState(null);
  const [newFactor, setNewFactor] = useState();

  const groupId = Number(group?.id || 0);

  const isDirty = useMemo(
    () => JSON.stringify(entries) !== JSON.stringify(originEntries),
    [entries, originEntries],
  );

  const loadEntries = async () => {
    if (!Number.isInteger(groupId) || groupId <= 0) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/group/${groupId}/user_price_overrides`);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('加载用户专属倍率失败'));
        return;
      }
      const normalized = normalizeEntries(data);
      setEntries(normalized);
      setOriginEntries(normalized);
    } catch (error) {
      showError(error?.message || t('加载用户专属倍率失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) return;
    setSearchKeyword('');
    setSearchOptions([]);
    setSelectedUserId(0);
    setSelectedUserRecord(null);
    setNewFactor(undefined);
    void loadEntries();
  }, [groupId, visible]);

  const searchUsers = async () => {
    const keyword = String(searchKeyword || '').trim();
    if (!keyword) {
      setSearchOptions([]);
      return;
    }
    setSearching(true);
    try {
      const res = await API.get(
        `/api/user/search?keyword=${encodeURIComponent(
          keyword,
        )}&group_id=&p=1&page_size=10`,
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('搜索用户失败'));
        return;
      }
      const items = Array.isArray(data?.items) ? data.items : [];
      const options = items
        .map((item) => {
          const userId = Number(item?.id || 0);
          if (!Number.isInteger(userId) || userId <= 0) return null;
          const label = String(
            item?.display_name || item?.username || item?.email || '',
          ).trim();
          if (!label) return null;
          return {
            value: userId,
            label: `${label} (#${userId})`,
            record: item,
          };
        })
        .filter(Boolean);
      setSearchOptions(options);
    } catch (error) {
      showError(error?.message || t('搜索用户失败'));
    } finally {
      setSearching(false);
    }
  };

  const addOrUpdateEntry = () => {
    if (!selectedUserRecord || selectedUserId <= 0) {
      showError(t('请选择用户'));
      return;
    }
    const factor = Number(newFactor ?? 0);
    if (!Number.isFinite(factor) || factor <= 0) {
      showError(t('专属倍率必须大于 0'));
      return;
    }

    setEntries((prev) => {
      const next = Array.isArray(prev) ? [...prev] : [];
      const index = next.findIndex((item) => item.user_id === selectedUserId);
      const record = {
        user_id: selectedUserId,
        username: String(selectedUserRecord?.username || '').trim(),
        display_name: String(selectedUserRecord?.display_name || '').trim(),
        email: String(selectedUserRecord?.email || '').trim(),
        factor,
      };
      if (index >= 0) {
        next[index] = record;
      } else {
        next.push(record);
      }
      return normalizeEntries(next);
    });

    setSearchKeyword('');
    setSearchOptions([]);
    setSelectedUserId(0);
    setSelectedUserRecord(null);
    setNewFactor(undefined);
  };

  const updateFactor = (userId, value) => {
    const factor = Number(value ?? 0);
    setEntries((prev) =>
      normalizeEntries(
        (Array.isArray(prev) ? prev : []).map((item) =>
          item.user_id === userId
            ? {
                ...item,
                factor: Number.isFinite(factor) ? factor : item.factor,
              }
            : item,
        ),
      ),
    );
  };

  const removeEntry = (userId) => {
    setEntries((prev) =>
      (Array.isArray(prev) ? prev : []).filter(
        (item) => item.user_id !== userId,
      ),
    );
  };

  const handleSave = async () => {
    if (!Number.isInteger(groupId) || groupId <= 0) {
      showError(t('分组 id 无效'));
      return;
    }
    for (const entry of entries) {
      const factor = Number(entry?.factor ?? 0);
      if (!Number.isFinite(factor) || factor <= 0) {
        showError(
          t('用户 {{user}} 的专属倍率必须大于 0', {
            user: displayUserLabel(entry),
          }),
        );
        return;
      }
    }

    setSaving(true);
    try {
      const res = await API.put(`/api/group/${groupId}/user_price_overrides`, {
        entries: entries.map((item) => ({
          user_id: item.user_id,
          factor: Number(item.factor),
        })),
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('保存用户专属倍率失败'));
        return;
      }
      const normalized = normalizeEntries(entries);
      setOriginEntries(normalized);
      showSuccess(
        t('已保存 {{count}} 条专属倍率', {
          count: Number(data?.count ?? normalized.length),
        }),
      );
      if (typeof onSaved === 'function') {
        onSaved(normalized);
      }
      onClose?.();
    } catch (error) {
      showError(error?.message || t('保存用户专属倍率失败'));
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    {
      title: t('用户'),
      dataIndex: 'email',
      key: 'user',
      render: (_, record) => (
        <div>
          <Text strong>{displayUserLabel(record)}</Text>
          <Text
            type='tertiary'
            style={{ display: 'block', marginTop: 2, fontSize: 12 }}
          >
            {record?.email || `#${record?.user_id}`}
          </Text>
        </div>
      ),
    },
    {
      title: t('默认倍率'),
      key: 'base_factor',
      width: 120,
      render: () => <Text>{formatRatio(group?.ratio)}x</Text>,
    },
    {
      title: t('专属倍率'),
      dataIndex: 'factor',
      key: 'factor',
      width: 160,
      render: (value, record) => (
        <InputNumber
          value={value}
          min={0.000001}
          step={0.1}
          precision={6}
          style={{ width: '100%' }}
          onChange={(nextValue) => updateFactor(record.user_id, nextValue)}
        />
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 100,
      render: (_, record) => (
        <Button
          icon={<IconDelete />}
          type='danger'
          theme='borderless'
          onClick={() => removeEntry(record.user_id)}
        />
      ),
    },
  ];

  return (
    <Modal
      title={t('分组 {{group}} 用户专属倍率', {
        group: String(group?.name || '').trim() || t('未命名分组'),
      })}
      visible={visible}
      centered
      okText={t('保存')}
      cancelText={t('关闭')}
      confirmLoading={saving}
      onOk={handleSave}
      onCancel={onClose}
      size='large'
    >
      <Spin spinning={loading}>
        <Text type='tertiary'>
          {t(
            '这里管理“当前分组”的用户专属倍率。保存后会直接覆盖该分组的默认倍率，不再需要单独维护客户价格模板。',
          )}
        </Text>

        <div
          style={{
            marginTop: 16,
            padding: 16,
            borderRadius: 16,
            border: '1px solid var(--semi-color-border)',
            background: 'var(--semi-color-fill-0)',
          }}
        >
          <Space align='end' wrap>
            <div style={{ minWidth: 220 }}>
              <Text
                type='tertiary'
                style={{ display: 'block', marginBottom: 6 }}
              >
                {t('搜索用户')}
              </Text>
              <Input
                value={searchKeyword}
                placeholder={t('输入用户名 / 显示名 / 邮箱')}
                onChange={setSearchKeyword}
                onEnterPress={searchUsers}
              />
            </div>
            <Button loading={searching} onClick={searchUsers}>
              {t('搜索')}
            </Button>
            <div style={{ minWidth: 260 }}>
              <Text
                type='tertiary'
                style={{ display: 'block', marginBottom: 6 }}
              >
                {t('选择用户')}
              </Text>
              <Select
                value={selectedUserId || undefined}
                optionList={searchOptions.map((item) => ({
                  value: item.value,
                  label: item.label,
                }))}
                placeholder={t('先搜索，再选择')}
                search
                filter
                showClear
                style={{ width: '100%' }}
                onChange={(value) => {
                  const nextValue = Number(value || 0);
                  const option = searchOptions.find(
                    (item) => item.value === nextValue,
                  );
                  setSelectedUserId(
                    Number.isInteger(nextValue) && nextValue > 0
                      ? nextValue
                      : 0,
                  );
                  setSelectedUserRecord(option?.record || null);
                }}
              />
            </div>
            <div style={{ width: 160 }}>
              <Text
                type='tertiary'
                style={{ display: 'block', marginBottom: 6 }}
              >
                {t('专属倍率')}
              </Text>
              <InputNumber
                value={newFactor}
                min={0.000001}
                step={0.1}
                precision={6}
                placeholder={formatRatio(group?.ratio)}
                style={{ width: '100%' }}
                onChange={setNewFactor}
              />
            </div>
            <Button
              type='primary'
              icon={<IconPlus />}
              onClick={addOrUpdateEntry}
            >
              {t('添加')}
            </Button>
          </Space>
        </div>

        <div style={{ marginTop: 16 }}>
          {entries.length === 0 ? (
            <Empty image={null} description={t('当前分组还没有用户专属倍率')} />
          ) : (
            <Table
              columns={columns}
              dataSource={entries}
              pagination={false}
              rowKey='user_id'
            />
          )}
        </div>

        {isDirty ? (
          <Text
            type='warning'
            style={{ display: 'block', marginTop: 12, fontSize: 12 }}
          >
            {t('当前有未保存的专属倍率改动')}
          </Text>
        ) : null}
      </Spin>
    </Modal>
  );
}
