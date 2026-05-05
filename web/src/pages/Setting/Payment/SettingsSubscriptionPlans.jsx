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
import { Button, Form, Modal, Switch, Table, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, renderMoneyFen, showError, showSuccess, yuanToFen } from '../../../helpers';

const { Text } = Typography;

const metaExample = {
  version: 1,
  note: '这里是套餐权益的自定义配置（目前仅保存，不参与购买/计费逻辑）',
  features: ['权益1', '权益2'],
  limits: {
    models: [],
    daily_quota_limit: null,
    channel_limit: null,
  },
};

const emptyPlanForm = {
  name: '',
  description: '',
  price_yuan: 0,
  duration_days: 30,
  meta: '',
  enabled: true,
  sort_order: 0,
};

export default function SettingsSubscriptionPlans() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [plans, setPlans] = useState([]);
  const modalFormApiRef = useRef();

  const [modalOpen, setModalOpen] = useState(false);
  const [modalLoading, setModalLoading] = useState(false);
  const [editingPlan, setEditingPlan] = useState(null);
  const [planForm, setPlanForm] = useState(emptyPlanForm);

  const loadPlans = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/subscription/plans/all');
      const { success, message, data } = res.data;
      if (success) {
        setPlans(Array.isArray(data) ? data : []);
      } else {
        showError(message || t('获取套餐失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取套餐失败'));
    } finally {
      setLoading(false);
    }
  };

  const openCreateModal = () => {
    setEditingPlan(null);
    setPlanForm({ ...emptyPlanForm });
    setModalOpen(true);
  };

  const openEditModal = (plan) => {
    setEditingPlan(plan);
    setPlanForm({
      name: plan?.name || '',
      description: plan?.description || '',
      price_yuan: (plan?.price_fen || 0) / 100,
      duration_days: plan?.duration_days || 30,
      meta: plan?.meta || '',
      enabled: Boolean(plan?.enabled),
      sort_order: plan?.sort_order || 0,
    });
    setModalOpen(true);
  };

  useEffect(() => {
    if (!modalOpen) return;
    if (!modalFormApiRef.current) return;
    modalFormApiRef.current.setValues(planForm, { isOverride: true });
  }, [modalOpen]);

  const savePlan = async () => {
    let priceFen = 0;
    try {
      priceFen = yuanToFen(planForm.price_yuan);
    } catch (e) {
      showError(e?.message || t('金额格式错误'));
      return;
    }

    const payload = {
      name: planForm.name,
      description: planForm.description,
      price_fen: priceFen,
      duration_days: planForm.duration_days,
      meta: planForm.meta,
      enabled: planForm.enabled,
      sort_order: planForm.sort_order,
    };

    setModalLoading(true);
    try {
      let res;
      if (editingPlan?.id) {
        res = await API.put(`/api/subscription/plans/${editingPlan.id}`, payload);
      } else {
        res = await API.post('/api/subscription/plans', payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        setModalOpen(false);
        await loadPlans();
      } else {
        showError(message || t('保存失败'));
      }
    } catch (e) {
      showError(e?.message || t('保存失败'));
    } finally {
      setModalLoading(false);
    }
  };

  const fillMetaExample = () => {
    const meta = JSON.stringify(metaExample, null, 2);
    setPlanForm((p) => ({
      ...p,
      meta,
    }));
    if (modalFormApiRef.current) {
      modalFormApiRef.current.setValue('meta', meta);
    }
  };

  const formatMetaJson = () => {
    const raw = (planForm.meta || '').trim();
    if (!raw) {
      showError(t('Meta 为空，无法格式化'));
      return;
    }
    try {
      const parsed = JSON.parse(raw);
      const meta = JSON.stringify(parsed, null, 2);
      setPlanForm((p) => ({
        ...p,
        meta,
      }));
      if (modalFormApiRef.current) {
        modalFormApiRef.current.setValue('meta', meta);
      }
    } catch (e) {
      showError(t('Meta 不是合法的 JSON'));
    }
  };

  const deletePlan = (plan) => {
    if (!plan?.id) return;
    Modal.confirm({
      title: t('确认删除该套餐？'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/subscription/plans/${plan.id}`);
          const { success, message } = res.data;
          if (success) {
            showSuccess(t('删除成功'));
            await loadPlans();
          } else {
            showError(message || t('删除失败'));
          }
        } catch (e) {
          showError(e?.message || t('删除失败'));
        }
      },
    });
  };

  useEffect(() => {
    loadPlans();
  }, []);

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      { title: t('名称'), dataIndex: 'name' },
      {
        title: t('价格'),
        dataIndex: 'price_fen',
        render: (v) => renderMoneyFen(v),
        width: 140,
      },
      {
        title: t('有效期(天)'),
        dataIndex: 'duration_days',
        width: 110,
      },
      {
        title: t('启用'),
        dataIndex: 'enabled',
        width: 90,
        render: (v) => <Switch checked={Boolean(v)} disabled />,
      },
      {
        title: t('排序'),
        dataIndex: 'sort_order',
        width: 90,
      },
      {
        title: t('操作'),
        dataIndex: 'actions',
        width: 160,
        render: (_, record) => (
          <div className='flex gap-2'>
            <Button size='small' onClick={() => openEditModal(record)}>
              {t('编辑')}
            </Button>
            <Button size='small' type='danger' theme='solid' onClick={() => deletePlan(record)}>
              {t('删除')}
            </Button>
          </div>
        ),
      },
    ],
    [t],
  );

  return (
    <>
      <div className='flex items-center justify-between mb-3'>
        <div className='space-y-1'>
          <Text strong>{t('订阅套餐')}</Text>
          <div className='text-xs text-gray-500'>
            {t('价格单位为人民币，系统内部按「分」存储')}
          </div>
        </div>
        <Button type='primary' theme='solid' onClick={openCreateModal}>
          {t('新增套餐')}
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={plans}
        rowKey='id'
        size='middle'
        loading={loading}
        scroll={{ x: 'max-content' }}
        pagination={{
          pageSize: 10,
        }}
      />

      <Modal
        title={editingPlan ? t('编辑套餐') : t('新增套餐')}
        visible={modalOpen}
        onOk={savePlan}
        onCancel={() => setModalOpen(false)}
        maskClosable={false}
        centered
        confirmLoading={modalLoading}
        width={720}
      >
        <Form
          layout='vertical'
          initValues={planForm}
          getFormApi={(formAPI) => (modalFormApiRef.current = formAPI)}
        >
          <Form.Input
            field='name'
            label={t('名称')}
            onChange={(v) => setPlanForm((p) => ({ ...p, name: v }))}
            rules={[{ required: true, message: t('请输入名称') }]}
            extraText={t('展示给用户看的套餐名称，例如：月卡 / 年卡')}
          />
          <Form.Input
            field='description'
            label={t('描述')}
            onChange={(v) => setPlanForm((p) => ({ ...p, description: v }))}
            extraText={t('可选：展示给用户看的说明文案')}
          />
          <div className='grid grid-cols-1 md:grid-cols-3 gap-3'>
            <Form.InputNumber
              field='price_yuan'
              label={t('价格（元）')}
              precision={2}
              min={0.01}
              onChange={(v) => setPlanForm((p) => ({ ...p, price_yuan: v }))}
              extraText={t('按人民币元填写，后端会换算为「分」存储')}
            />
            <Form.InputNumber
              field='duration_days'
              label={t('有效期（天）')}
              precision={0}
              min={1}
              onChange={(v) => setPlanForm((p) => ({ ...p, duration_days: v }))}
              extraText={t('例如：月卡填 30')}
            />
            <Form.InputNumber
              field='sort_order'
              label={t('排序')}
              precision={0}
              onChange={(v) => setPlanForm((p) => ({ ...p, sort_order: v }))}
              extraText={t('数值越大越靠前')}
            />
          </div>

          <div className='flex items-center gap-2 mb-2'>
            <Switch
              checked={planForm.enabled}
              onChange={(checked) => setPlanForm((p) => ({ ...p, enabled: checked }))}
            />
            <Text>{t('启用')}</Text>
          </div>

          <Form.TextArea
            field='meta'
            label={t('权益配置（Meta）')}
            onChange={(v) => setPlanForm((p) => ({ ...p, meta: v }))}
            placeholder={t('可填任意文本；也可填 JSON（建议）')}
            autosize
            extraText={t('当前后端仅原样保存该字段，不参与购买逻辑；建议用 JSON 便于后续扩展')}
          />

          <div className='flex flex-wrap gap-2'>
            <Button size='small' onClick={fillMetaExample}>
              {t('填入示例 JSON')}
            </Button>
            <Button size='small' onClick={formatMetaJson}>
              {t('格式化 JSON')}
            </Button>
          </div>
        </Form>
      </Modal>
    </>
  );
}
