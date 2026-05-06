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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Form,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

import { API, isRoot, showError, showSuccess } from '../../../helpers';
import { PRICING_PROFILE_REQUEST_VERSION } from '../../../constants/common.constant';
import RatioTag from '../../../components/common/ui/RatioTag';
import GroupRatioPill from '../../../components/common/ui/GroupRatioPill';

const { Text } = Typography;

const audienceTagColor = {
  retail: 'blue',
  reseller: 'green',
};

const normalizeProfileGroupFactors = (rawFactors) => {
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

const getProfileInitValues = (profile) => ({
  code: profile?.code || '',
  name: profile?.name || '',
  audience: profile?.audience || 'retail',
  default_factor:
    Number(profile?.default_factor ?? 0) > 0
      ? Number(profile.default_factor)
      : 1,
  enabled: profile?.enabled !== false,
  description: profile?.description || '',
});

const pricingProfileRequestConfig = {
  disableDuplicate: true,
  params: {
    _v: PRICING_PROFILE_REQUEST_VERSION,
  },
};

export default function CustomerPricingSettings() {
  const { t } = useTranslation();
  const rootUser = isRoot();
  const formApiRef = useRef(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [profiles, setProfiles] = useState([]);
  const [groups, setGroups] = useState([]);
  const [legacyUsers, setLegacyUsers] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingProfile, setEditingProfile] = useState(null);
  const [editingGroupFactors, setEditingGroupFactors] = useState([]);
  const [formKey, setFormKey] = useState(0);

  const groupOptions = useMemo(
    () =>
      (Array.isArray(groups) ? groups : [])
        .map((item) => {
          const id = Number(item?.id ?? 0);
          if (!Number.isFinite(id) || id <= 0) return null;
          const label = String(
            item?.display_name || item?.name || item?.code || '',
          ).trim();
          if (!label) return null;
          return {
            label,
            value: id,
          };
        })
        .filter(Boolean)
        .sort((a, b) => Number(a.value) - Number(b.value)),
    [groups],
  );

  const groupLabelById = useMemo(() => {
    const map = {};
    groupOptions.forEach((item) => {
      map[item.value] = item.label;
    });
    return map;
  }, [groupOptions]);

  const loadProfiles = async () => {
    const res = await API.get(
      '/api/pricing_profiles/',
      pricingProfileRequestConfig,
    );
    const { success, message, data } = res.data || {};
    if (!success) {
      throw new Error(message || t('获取价格模板失败'));
    }
    setProfiles(Array.isArray(data) ? data : []);
  };

  const loadGroups = async () => {
    const res = await API.get('/api/group/');
    const { success, message, data } = res.data || {};
    if (!success) {
      throw new Error(message || t('获取分组失败'));
    }
    setGroups(Array.isArray(data) ? data : []);
  };

  const loadLegacyUsers = async () => {
    if (!rootUser) {
      setLegacyUsers([]);
      return;
    }
    const res = await API.get(
      '/api/pricing_profiles/legacy_users',
      pricingProfileRequestConfig,
    );
    const { success, message, data } = res.data || {};
    if (!success) {
      throw new Error(message || t('获取历史用户失败'));
    }
    setLegacyUsers(Array.isArray(data) ? data : []);
  };

  const refreshData = async () => {
    setLoading(true);
    try {
      await Promise.all([loadProfiles(), loadGroups(), loadLegacyUsers()]);
    } catch (error) {
      showError(error?.message || t('刷新失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refreshData();
  }, []);

  const openCreateModal = () => {
    setEditingProfile(null);
    setEditingGroupFactors([]);
    setFormKey((prev) => prev + 1);
    setModalVisible(true);
  };

  const openEditModal = (profile) => {
    setEditingProfile(profile || null);
    setEditingGroupFactors(
      normalizeProfileGroupFactors(profile?.group_factors),
    );
    setFormKey((prev) => prev + 1);
    setModalVisible(true);
  };

  const closeModal = () => {
    setModalVisible(false);
    setEditingProfile(null);
    setEditingGroupFactors([]);
  };

  const addGroupFactor = () => {
    setEditingGroupFactors((prev) => [
      ...(Array.isArray(prev) ? prev : []),
      { group_id: 0, factor: 1 },
    ]);
  };

  const updateGroupFactor = (index, key, value) => {
    setEditingGroupFactors((prev) =>
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

  const removeGroupFactor = (index) => {
    setEditingGroupFactors((prev) =>
      (Array.isArray(prev) ? prev : []).filter(
        (_, itemIndex) => itemIndex !== index,
      ),
    );
  };

  const submitProfile = async (values) => {
    const payload = {
      code: String(values?.code || '').trim(),
      name: String(values?.name || '').trim(),
      audience: String(values?.audience || 'retail')
        .trim()
        .toLowerCase(),
      default_factor: Number(values?.default_factor ?? 0),
      enabled: values?.enabled !== false,
      description: String(values?.description || '').trim(),
      group_factors: [],
    };

    if (!payload.code) {
      showError(t('模板编码不能为空'));
      return;
    }
    if (!payload.name) {
      payload.name = payload.code;
    }
    if (
      !Number.isFinite(payload.default_factor) ||
      payload.default_factor <= 0
    ) {
      showError(t('默认倍率必须大于0'));
      return;
    }

    const normalizedFactors = [];
    const seen = new Set();
    for (const item of Array.isArray(editingGroupFactors)
      ? editingGroupFactors
      : []) {
      const rawGroupId = Number(item?.group_id ?? 0);
      const groupId = Number.isFinite(rawGroupId) ? Math.floor(rawGroupId) : 0;
      if (groupId <= 0) {
        showError(t('分组倍率中的分组无效'));
        return;
      }
      if (seen.has(groupId)) {
        showError(
          t('分组 {{group}} 的倍率重复', {
            group: groupLabelById[groupId] || String(groupId),
          }),
        );
        return;
      }
      const factor = Number(item?.factor ?? 0);
      if (!Number.isFinite(factor) || factor <= 0) {
        showError(
          t('分组 {{group}} 的倍率必须大于0', {
            group: groupLabelById[groupId] || String(groupId),
          }),
        );
        return;
      }
      seen.add(groupId);
      normalizedFactors.push({
        group_id: groupId,
        factor,
      });
    }
    payload.group_factors = normalizedFactors.sort(
      (a, b) => a.group_id - b.group_id,
    );

    setSaving(true);
    try {
      const res = editingProfile?.id
        ? await API.put(`/api/pricing_profiles/${editingProfile.id}`, payload)
        : await API.post('/api/pricing_profiles/', payload);
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('保存失败'));
        return;
      }
      showSuccess(t('保存成功'));
      closeModal();
      await refreshData();
    } catch (error) {
      showError(error?.message || t('保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async (profileId) => {
    try {
      const res = await API.delete(`/api/pricing_profiles/${profileId}`);
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('删除失败'));
        return;
      }
      showSuccess(t('删除成功'));
      await refreshData();
    } catch (error) {
      showError(error?.message || t('删除失败'));
    }
  };

  const profileColumns = [
    {
      title: t('模板'),
      dataIndex: 'name',
      key: 'name',
      render: (_, record) => (
        <div>
          <div className='font-medium'>
            {record?.name || record?.code || '-'}
          </div>
          <div className='text-xs text-gray-500'>{record?.code || '-'}</div>
        </div>
      ),
    },
    {
      title: t('客户类型'),
      dataIndex: 'audience',
      key: 'audience',
      render: (value) => {
        const audience = String(value || '')
          .trim()
          .toLowerCase();
        return (
          <Tag color={audienceTagColor[audience] || 'grey'}>
            {audience === 'reseller' ? t('B端') : t('C端')}
          </Tag>
        );
      },
    },
    {
      title: t('默认倍率'),
      dataIndex: 'default_factor',
      key: 'default_factor',
      render: (value) => <RatioTag value={value} />,
    },
    {
      title: t('分组覆写'),
      dataIndex: 'group_factors',
      key: 'group_factors',
      render: (value) => {
        const factors = normalizeProfileGroupFactors(value);
        if (factors.length === 0) {
          return '-';
        }
        return (
          <Space wrap>
            {factors.slice(0, 3).map((item) => (
              <GroupRatioPill
                key={`${item.group_id}-${item.factor}`}
                label={groupLabelById[item.group_id] || item.group_id}
                ratio={item.factor}
              />
            ))}
            {factors.length > 3 && (
              <Tag color='blue'>{`+${factors.length - 3}`}</Tag>
            )}
          </Space>
        );
      },
    },
    {
      title: t('用户数'),
      dataIndex: 'user_count',
      key: 'user_count',
      render: (value) => Number(value ?? 0) || 0,
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      key: 'enabled',
      render: (value) =>
        value === false ? (
          <Tag color='grey'>{t('停用')}</Tag>
        ) : (
          <Tag color='green'>{t('启用')}</Tag>
        ),
    },
    {
      title: t('操作'),
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button
            icon={<IconEdit />}
            theme='light'
            disabled={!rootUser}
            onClick={() => openEditModal(record)}
          >
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确认删除该价格模板？')}
            content={t('删除后不可恢复')}
            disabled={!rootUser}
            onConfirm={() => deleteProfile(record.id)}
          >
            <Button
              icon={<IconDelete />}
              theme='light'
              type='danger'
              disabled={!rootUser}
            >
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const legacyUserColumns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: t('用户'),
      key: 'user',
      render: (_, record) => (
        <div>
          <div className='font-medium'>
            {record?.display_name || record?.username || '-'}
          </div>
          <div className='text-xs text-gray-500'>{record?.username || '-'}</div>
        </div>
      ),
    },
    {
      title: t('服务分组'),
      dataIndex: 'group_label',
      key: 'group_label',
      render: (value) => value || '-',
    },
    {
      title: t('基础倍率'),
      dataIndex: 'base_multiplier',
      key: 'base_multiplier',
      render: (value) => <RatioTag value={value} />,
    },
    {
      title: t('旧特殊倍率目标数'),
      dataIndex: 'legacy_target_count',
      key: 'legacy_target_count',
      render: (value) => Number(value ?? 0) || 0,
    },
    {
      title: t('当前生效来源'),
      dataIndex: 'effective_source',
      key: 'effective_source',
      render: (value, record) => {
        const source = String(value || 'public')
          .trim()
          .toLowerCase();
        switch (source) {
          case 'legacy':
            return (
              <div>
                <Tag color='orange'>{t('旧专属倍率')}</Tag>
                {!record?.base_multiplier_applied &&
                  Number(record?.base_multiplier ?? 1) !== 1 && (
                    <div className='text-xs text-gray-500'>
                      {t('旧专属倍率优先')}
                    </div>
                  )}
              </div>
            );
          case 'base_multiplier':
            return <Tag color='cyan'>{t('基础倍率')}</Tag>;
          default:
            return <Tag>{t('公开倍率')}</Tag>;
        }
      },
    },
    {
      title: t('当前模板'),
      dataIndex: 'pricing_profile_label',
      key: 'pricing_profile_label',
      render: (value) => value || '-',
    },
  ];

  return (
    <Spin spinning={loading}>
      <Card
        title={t('客户价格规则')}
        headerExtraContent={
          <Space>
            <Button onClick={() => void refreshData()}>{t('刷新')}</Button>
            <Button
              theme='solid'
              icon={<IconPlus />}
              disabled={!rootUser}
              onClick={openCreateModal}
            >
              {t('新建模板')}
            </Button>
          </Space>
        }
      >
        <Table
          rowKey='id'
          pagination={false}
          columns={profileColumns}
          dataSource={profiles}
          empty={<Empty image={null} description={t('暂无价格模板')} />}
        />
      </Card>

      {rootUser && (
        <Card title={t('兼容迁移清单')} style={{ marginTop: 16 }}>
          <Table
            rowKey='id'
            pagination={false}
            columns={legacyUserColumns}
            dataSource={legacyUsers}
            empty={<Empty image={null} description={t('暂无历史用户')} />}
          />
        </Card>
      )}

      <Modal
        title={editingProfile?.id ? t('编辑价格模板') : t('新建价格模板')}
        visible={modalVisible}
        onCancel={closeModal}
        onOk={() => formApiRef.current?.submitForm()}
        confirmLoading={saving}
        okText={t('保存')}
        cancelText={t('取消')}
        width={720}
      >
        <Form
          key={formKey}
          initValues={getProfileInitValues(editingProfile)}
          getFormApi={(api) => {
            formApiRef.current = api;
          }}
          onSubmit={submitProfile}
          onSubmitFail={(errors) => {
            const firstError = Object.values(errors || {})[0];
            if (firstError) {
              showError(Array.isArray(firstError) ? firstError[0] : firstError);
            }
          }}
        >
          <Form.Input
            field='code'
            label={t('模板编码')}
            placeholder={t('请输入模板编码')}
            rules={[{ required: true, message: t('请输入模板编码') }]}
          />
          <Form.Input
            field='name'
            label={t('模板名称')}
            placeholder={t('请输入模板名称')}
          />
          <Form.Select
            field='audience'
            label={t('客户类型')}
            optionList={[
              { label: t('C端'), value: 'retail' },
              { label: t('B端'), value: 'reseller' },
            ]}
          />
          <Form.InputNumber
            field='default_factor'
            label={t('默认倍率')}
            min={0.000001}
            step={0.1}
            precision={6}
            style={{ width: '100%' }}
          />
          <Form.Switch field='enabled' label={t('启用')} />
          <Form.TextArea
            field='description'
            label={t('备注')}
            autosize={{ minRows: 2, maxRows: 4 }}
            showClear
          />

          <div className='rounded-xl border border-gray-200 p-3'>
            <div className='mb-3 flex items-center justify-between gap-3'>
              <Text className='font-medium'>{t('分组覆写')}</Text>
              <Button
                icon={<IconPlus />}
                htmlType='button'
                theme='light'
                onClick={addGroupFactor}
              >
                {t('新增')}
              </Button>
            </div>

            {editingGroupFactors.length === 0 ? (
              <Empty image={null} description={t('暂无分组覆写')} />
            ) : (
              <div className='space-y-2'>
                {editingGroupFactors.map((item, index) => (
                  <div
                    key={`pricing-profile-factor-${index}`}
                    className='flex flex-col gap-2 rounded-lg border border-gray-100 p-3 md:flex-row md:items-center'
                  >
                    <Select
                      value={item?.group_id || undefined}
                      optionList={groupOptions}
                      placeholder={t('选择分组')}
                      search
                      style={{ flex: 1 }}
                      onChange={(value) =>
                        updateGroupFactor(index, 'group_id', value)
                      }
                    />
                    <InputNumber
                      value={item?.factor}
                      min={0.000001}
                      step={0.1}
                      precision={6}
                      placeholder={t('倍率')}
                      style={{ width: 180 }}
                      onChange={(value) =>
                        updateGroupFactor(index, 'factor', value)
                      }
                    />
                    <Button
                      icon={<IconDelete />}
                      htmlType='button'
                      theme='light'
                      type='danger'
                      onClick={() => removeGroupFactor(index)}
                    >
                      {t('删除')}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Form>
      </Modal>
    </Spin>
  );
}
