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
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, timestamp2string } from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Avatar,
  Button,
  Card,
  Col,
  Form,
  Radio,
  Row,
  SideSheet,
  Space,
  Spin,
  Tag,
  Typography,
  Modal,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconSave,
  IconSearch,
  IconSetting,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const parseUserIds = (raw) => {
  const input = String(raw || '').trim();
  if (!input) return [];
  const parts = input.split(/[\s,，;；]+/).filter(Boolean);
  if (parts.length === 0) return [];

  const ids = [];
  const seen = new Set();
  for (const part of parts) {
    if (!/^\d+$/.test(part)) {
      throw new Error(`user_ids 中包含无效值：${part}`);
    }
    const id = parseInt(part, 10);
    if (!Number.isFinite(id) || id <= 0) {
      throw new Error(`user_ids 中包含无效用户ID：${part}`);
    }
    if (!seen.has(id)) {
      seen.add(id);
      ids.push(id);
    }
  }
  return ids;
};

const formatTs = (ts) => {
  const v = parseInt(ts, 10) || 0;
  if (v <= 0) return '-';
  return timestamp2string(v);
};

const canonicalizePayload = (payload) => {
  const types = Array.isArray(payload?.subscription_types)
    ? [...payload.subscription_types].sort()
    : [];
  const userIds = Array.isArray(payload?.user_ids)
    ? [...payload.user_ids].sort((a, b) => a - b)
    : [];
  return {
    min_remaining_days: payload?.min_remaining_days,
    max_remaining_days:
      payload?.max_remaining_days === undefined ? null : payload.max_remaining_days,
    subscription_types: types,
    user_ids: userIds.length > 0 ? userIds : null,
    add_days: payload?.add_days === undefined ? null : payload.add_days,
    set_expire_at: payload?.set_expire_at === undefined ? null : payload.set_expire_at,
  };
};

const BulkUpdateSubscriptionDurationModal = (props) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);

  const [loading, setLoading] = useState(false);
  const [lastResult, setLastResult] = useState(null);
  const [lastPreviewKey, setLastPreviewKey] = useState('');
  const [executeConfirmVisible, setExecuteConfirmVisible] = useState(false);
  const [pendingExecutePayload, setPendingExecutePayload] = useState(null);

  const subscriptionTypeOptions = useMemo(
    () => [
      { label: t('额度订阅'), value: 'quota' },
      { label: t('次数订阅'), value: 'request' },
    ],
    [t],
  );

  const initValues = useMemo(
    () => ({
      user_ids_raw: '',
      subscription_types: ['quota', 'request'],
      min_remaining_days: undefined,
      max_remaining_days: undefined,
      set_mode: 'add',
      add_days: undefined,
      set_expire_at_date: dayjs().add(30, 'day').endOf('day').toDate(),
    }),
    [],
  );

  useEffect(() => {
    if (!props.visible) return;
    setLastResult(null);
    setLastPreviewKey('');
    setExecuteConfirmVisible(false);
    setPendingExecutePayload(null);
    formApiRef.current?.setValues(initValues);
  }, [props.visible, initValues]);

  const buildNormalizedPayload = (values) => {
    const minRemainingDays = parseInt(values?.min_remaining_days, 10);
    if (!Number.isFinite(minRemainingDays) || minRemainingDays < 0) {
      throw new Error(t('min_remaining_days 必须为非负整数'));
    }

    const payload = {
      min_remaining_days: minRemainingDays,
    };

    const rawTypes = Array.isArray(values?.subscription_types)
      ? values.subscription_types
      : [];
    const allowedTypes = new Set(['quota', 'request']);
    const types = [];
    for (const item of rawTypes) {
      if (!allowedTypes.has(item)) {
        throw new Error(t('无效的订阅类型'));
      }
      if (!types.includes(item)) {
        types.push(item);
      }
    }
    if (types.length === 0) {
      throw new Error(t('请选择至少一个订阅类型'));
    }
    payload.subscription_types = types;

    const maxRaw = values?.max_remaining_days;
    if (maxRaw !== undefined && maxRaw !== null && maxRaw !== '') {
      const maxRemainingDays = parseInt(maxRaw, 10);
      if (!Number.isFinite(maxRemainingDays) || maxRemainingDays < 0) {
        throw new Error(t('max_remaining_days 必须为非负整数'));
      }
      payload.max_remaining_days = maxRemainingDays;
    }

    if (
      payload.max_remaining_days !== undefined &&
      payload.max_remaining_days < payload.min_remaining_days
    ) {
      throw new Error(t('max_remaining_days 必须大于等于 min_remaining_days'));
    }

    const userIds = parseUserIds(values?.user_ids_raw);
    if (userIds.length > 0) {
      payload.user_ids = userIds;
    }

    const setMode = values?.set_mode;
    if (setMode !== 'add' && setMode !== 'date') {
      throw new Error(t('无效的设置方式'));
    }

    if (setMode === 'add') {
      const addDays = parseInt(values?.add_days, 10);
      if (!Number.isFinite(addDays) || addDays <= 0) {
        throw new Error(t('add_days 必须为正整数'));
      }
      payload.add_days = addDays;
      return payload;
    }

    const date = values?.set_expire_at_date;
    if (!date) {
      throw new Error(t('请选择到期时间'));
    }
    const unix = Math.floor(new Date(date).getTime() / 1000);
    if (!Number.isFinite(unix) || unix <= 0) {
      throw new Error(t('无效的到期时间'));
    }
    payload.set_expire_at = unix;
    return payload;
  };

  const makeRequestKey = (normalizedPayload) =>
    JSON.stringify(canonicalizePayload(normalizedPayload));

  const run = async (dryRun, overridePayload = null) => {
    if (!formApiRef.current) return;
    setLoading(true);
    try {
      await formApiRef.current.validate();
      const normalized =
        overridePayload || buildNormalizedPayload(formApiRef.current.getValues());
      const res = await API.post('/api/user/subscriptions/bulk/duration', {
        ...normalized,
        dry_run: Boolean(dryRun),
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setLastResult(data);
      if (dryRun) {
        setLastPreviewKey(makeRequestKey(normalized));
        showSuccess(t('预览完成'));
      } else {
        const quotaUpdated = parseInt(data?.quota_subscription_updated_count) || 0;
        const requestUpdated =
          parseInt(data?.request_subscription_updated_count) || 0;
        showSuccess(
          t('已执行：额度订阅 {{quota}} 条，次数订阅 {{request}} 条', {
            quota: quotaUpdated,
            request: requestUpdated,
          }),
        );
        setLastPreviewKey('');
        props.refresh?.();
      }
    } catch (err) {
      showError(err);
    } finally {
      setLoading(false);
    }
  };

  const prepareExecute = async () => {
    if (!formApiRef.current) return;
    try {
      await formApiRef.current.validate();
      const normalized = buildNormalizedPayload(formApiRef.current.getValues());
      const key = makeRequestKey(normalized);
      if (!lastPreviewKey || lastPreviewKey !== key) {
        showError(t('请先使用相同参数进行一次预览'));
        return;
      }
      setPendingExecutePayload(normalized);
      setExecuteConfirmVisible(true);
    } catch (err) {
      showError(err);
    }
  };

  const confirmExecute = async () => {
    const payload = pendingExecutePayload;
    setExecuteConfirmVisible(false);
    setPendingExecutePayload(null);
    if (!payload) return;
    await run(false, payload);
  };

  const handleClose = () => {
    if (loading) return;
    setExecuteConfirmVisible(false);
    setPendingExecutePayload(null);
    props.handleClose?.();
  };

  return (
    <>
      <Modal
        centered
        visible={executeConfirmVisible}
        title={t('确认执行批量修改？')}
        onCancel={() => setExecuteConfirmVisible(false)}
        onOk={confirmExecute}
        confirmLoading={loading}
        okType='danger'
      >
        {t('将按当前表单参数批量修改到期时间')}
      </Modal>

      <SideSheet
        placement={'left'}
        title={
          <Space>
            <Tag color='orange' shape='circle'>
              {t('批量')}
            </Tag>
            <Title heading={4} className='m-0'>
              {t('批量调整订阅时长')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: 0 }}
        visible={props.visible}
        width={isMobile ? '100%' : 720}
        closeIcon={null}
        onCancel={handleClose}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='light'
                type='primary'
                icon={<IconSearch />}
                loading={loading}
                onClick={() => run(true)}
              >
                {t('预览')}
              </Button>
              <Button
                theme='solid'
                type='danger'
                icon={<IconSave />}
                loading={loading}
                disabled={loading}
                onClick={prepareExecute}
              >
                {t('执行修改')}
              </Button>
              <Button
                theme='light'
                type='primary'
                icon={<IconClose />}
                onClick={handleClose}
                disabled={loading}
              >
                {t('关闭')}
              </Button>
            </Space>
          </div>
        }
      >
        <Spin spinning={loading}>
          <Form
            initValues={initValues}
            getFormApi={(api) => (formApiRef.current = api)}
            onValueChange={() => {
              if (!lastPreviewKey) return;
              try {
                const normalized = buildNormalizedPayload(
                  formApiRef.current?.getValues(),
                );
                const key = makeRequestKey(normalized);
                if (key !== lastPreviewKey) {
                  setLastPreviewKey('');
                  if (lastResult?.dry_run) {
                    setLastResult(null);
                  }
                }
              } catch (e) {
                setLastPreviewKey('');
                if (lastResult?.dry_run) {
                  setLastResult(null);
                }
              }
            }}
          >
          {({ values }) => (
            <div className='p-2 space-y-2'>
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='orange' className='mr-2 shadow-md'>
                    <IconSetting size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>
                      {t('筛选条件')}
                    </Text>
                    <div className='text-xs text-gray-600'>
                      {t(
                        '额度订阅：剩余额度>0 且 expire_at>0 且未过期；次数订阅：总次数不限或剩余次数>0 且 expire_at>0 且未过期',
                      )}
                    </div>
                  </div>
                </div>

                <Row gutter={12}>
                  <Col span={24}>
                    <Form.TextArea
                      field='user_ids_raw'
                      label={t('用户ID（可选）')}
                      placeholder={t('留空表示所有用户；支持逗号/空格/换行分隔')}
                      autosize
                      rows={1}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Select
                      field='subscription_types'
                      label={t('订阅类型')}
                      placeholder={t('请选择订阅类型')}
                      optionList={subscriptionTypeOptions}
                      multiple
                      showClear
                      style={{ width: '100%' }}
                      rules={[{ required: true, message: t('请选择订阅类型') }]}
                    />
                  </Col>
                  <Col span={12}>
                    <Form.InputNumber
                      field='min_remaining_days'
                      label={t('订阅剩余天数 >')}
                      placeholder={t('例如 30')}
                      min={0}
                      step={1}
                      precision={0}
                      rules={[
                        { required: true, message: t('请输入 min_remaining_days') },
                      ]}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={12}>
                    <Form.InputNumber
                      field='max_remaining_days'
                      label={t('且 <=（可选）')}
                      placeholder={t('不填则不限制')}
                      min={0}
                      step={1}
                      precision={0}
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>

              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconSave size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('设置到期')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('对命中的订阅统一增加天数，或直接设置到期时间')}
                    </div>
                  </div>
                </div>

                <Form.RadioGroup field='set_mode' label={t('设置方式')}>
                  <Radio value='add'>{t('统一增加 N 天')}</Radio>
                  <Radio value='date'>{t('指定到期时间')}</Radio>
                </Form.RadioGroup>

                <Row gutter={12}>
                  {values?.set_mode === 'add' ? (
                    <Col span={24}>
                      <Form.InputNumber
                        field='add_days'
                        label={t('增加天数')}
                        placeholder={t('例如 7')}
                        min={1}
                        step={1}
                        precision={0}
                        rules={[
                          {
                            required: true,
                            message: t('请输入 add_days'),
                          },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  ) : (
                    <Col span={24}>
                      <Form.DatePicker
                        field='set_expire_at_date'
                        label={t('新到期时间')}
                        type='dateTime'
                        format='yyyy/MM/dd HH:mm:ss'
                        showClear={false}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {lastResult && (
                <Card className='!rounded-2xl shadow-sm border-0'>
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color={lastResult.dry_run ? 'grey' : 'green'}
                      className='mr-2 shadow-md'
                    >
                      <IconSearch size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('结果')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {lastResult.dry_run
                          ? t('预览结果（未落库）')
                          : t('执行结果（已落库）')}
                      </div>
                    </div>
                  </div>

                  <div className='space-y-1'>
                    <Text className='block'>
                      {t('匹配用户：{{count}}', {
                        count: lastResult.matched_user_count || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配额度订阅记录：{{count}}', {
                        count: lastResult.quota_subscription_matched_count || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配次数订阅记录：{{count}}', {
                        count:
                          lastResult.request_subscription_matched_count || 0,
                      })}
                    </Text>
                    {!lastResult.dry_run && (
                      <>
                        <Text className='block'>
                          {t('已更新额度订阅记录：{{count}}', {
                            count:
                              lastResult.quota_subscription_updated_count || 0,
                          })}
                        </Text>
                        <Text className='block'>
                          {t('已更新次数订阅记录：{{count}}', {
                            count:
                              lastResult.request_subscription_updated_count ||
                              0,
                          })}
                        </Text>
                      </>
                    )}
                    <div className='text-xs text-gray-600'>
                      {t('筛选下限到期时间：{{ts}}', {
                        ts: formatTs(lastResult.min_expire_at),
                      })}
                      {lastResult.max_expire_at ? (
                        <>
                          {' '}
                          {t('上限：{{ts}}', {
                            ts: formatTs(lastResult.max_expire_at),
                          })}
                        </>
                      ) : null}
                    </div>
                    {lastResult.operation === 'add_days' ? (
                      <div className='text-xs text-gray-600'>
                        {t('统一增加：{{days}} 天', {
                          days: lastResult.add_days || 0,
                        })}
                      </div>
                    ) : (
                      <div className='text-xs text-gray-600'>
                        {t('设置到期时间：{{ts}}', {
                          ts: formatTs(lastResult.set_expire_at),
                        })}
                      </div>
                    )}
                    {Array.isArray(lastResult.matched_user_ids_preview) &&
                      lastResult.matched_user_ids_preview.length > 0 && (
                        <div className='text-xs text-gray-600 break-all'>
                          {t('用户预览：')}
                          {lastResult.matched_user_ids_preview.join(', ')}
                          {lastResult.matched_user_ids_preview_more
                            ? t(' …（更多）')
                            : ''}
                        </div>
                      )}
                  </div>
                </Card>
              )}
            </div>
          )}
        </Form>
      </Spin>
      </SideSheet>
    </>
  );
};

export default BulkUpdateSubscriptionDurationModal;
