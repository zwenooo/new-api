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
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Space,
  Spin,
  Switch,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

export default function UserGroupManagementModal({
  visible,
  onClose,
  onSaved,
}) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [items, setItems] = useState([]);
  const [editorVisible, setEditorVisible] = useState(false);
  const [editingItem, setEditingItem] = useState(null);

  const loadItems = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user_group/');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取用户分组失败'));
        return;
      }
      setItems(Array.isArray(data) ? data : []);
    } catch (error) {
      showError(error?.message || t('获取用户分组失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) return;
    void loadItems();
  }, [visible]);

  const closeEditor = () => {
    setEditorVisible(false);
    setEditingItem(null);
  };

  const openCreate = () => {
    setEditingItem(null);
    setEditorVisible(true);
  };

  const openEdit = (item) => {
    setEditingItem(item);
    setEditorVisible(true);
  };

  const submit = async (values) => {
    setSaving(true);
    try {
      const payload = {
        code: String(values?.code || '').trim(),
        name: String(values?.name || '').trim(),
        description: String(values?.description || '').trim(),
        sort_order: Number(values?.sort_order ?? 0) || 0,
        enabled: values?.enabled !== false,
      };
      let res;
      if (editingItem?.id) {
        res = await API.put('/api/user_group/', {
          id: editingItem.id,
          ...payload,
        });
      } else {
        res = await API.post('/api/user_group/', payload);
      }
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('保存用户分组失败'));
        return;
      }
      showSuccess(t('保存成功'));
      closeEditor();
      await loadItems();
      onSaved?.();
    } catch (error) {
      showError(error?.message || t('保存用户分组失败'));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/user_group/${id}`);
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('删除用户分组失败'));
        return;
      }
      showSuccess(t('删除成功'));
      await loadItems();
      onSaved?.();
    } catch (error) {
      showError(error?.message || t('删除用户分组失败'));
    }
  };

  const columns = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        width: 80,
      },
      {
        title: t('名称'),
        dataIndex: 'name',
        render: (text, record) => (
          <div>
            <Text strong>{text}</Text>
            <Text
              type='tertiary'
              style={{ display: 'block', marginTop: 2, fontSize: 12 }}
            >
              {record?.code || '-'}
            </Text>
          </div>
        ),
      },
      {
        title: t('说明'),
        dataIndex: 'description',
        render: (text) => String(text || '').trim() || '-',
      },
      {
        title: t('启用'),
        dataIndex: 'enabled',
        width: 100,
        render: (value) => <Text>{value === false ? t('否') : t('是')}</Text>,
      },
      {
        title: t('操作'),
        key: 'action',
        width: 140,
        render: (_, record) => (
          <Space>
            <Button
              theme='borderless'
              icon={<IconEdit />}
              onClick={() => openEdit(record)}
            />
            <Popconfirm
              title={t('确定删除该用户分组吗？')}
              content={t('删除后已绑定用户会被清空该用户分组')}
              okType='danger'
              onConfirm={() => handleDelete(record.id)}
            >
              <Button theme='borderless' type='danger' icon={<IconDelete />} />
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [t],
  );

  return (
    <>
      <Modal
        title={t('用户分组管理')}
        visible={visible}
        centered
        footer={
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button icon={<IconPlus />} onClick={openCreate}>
              {t('新增用户分组')}
            </Button>
            <Button onClick={onClose}>{t('关闭')}</Button>
          </div>
        }
        onCancel={onClose}
        onOk={onClose}
        size='large'
      >
        <Spin spinning={loading}>
          <Table
            columns={columns}
            dataSource={items}
            pagination={false}
            rowKey='id'
          />
        </Spin>
      </Modal>

      <Modal
        title={editingItem?.id ? t('编辑用户分组') : t('新增用户分组')}
        visible={editorVisible}
        centered
        confirmLoading={saving}
        onCancel={closeEditor}
        footer={null}
      >
        <Form
          initValues={{
            code: editingItem?.code || '',
            name: editingItem?.name || '',
            description: editingItem?.description || '',
            sort_order: Number(editingItem?.sort_order ?? 0) || 0,
            enabled: editingItem?.enabled !== false,
          }}
          onSubmit={submit}
        >
          <Form.Input
            field='code'
            label={t('编码')}
            rules={[{ required: true, message: t('请输入编码') }]}
          />
          <Form.Input
            field='name'
            label={t('名称')}
            rules={[{ required: true, message: t('请输入名称') }]}
          />
          <Form.Input field='description' label={t('说明')} />
          <Form.InputNumber field='sort_order' label={t('排序')} min={0} />
          <Form.Switch field='enabled' label={t('启用')} />
          <div
            style={{
              display: 'flex',
              justifyContent: 'flex-end',
              marginTop: 16,
            }}
          >
            <Space>
              <Button onClick={closeEditor}>{t('取消')}</Button>
              <Button htmlType='submit' type='primary' loading={saving}>
                {t('保存')}
              </Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </>
  );
}
