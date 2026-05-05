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
  useEffect,
  useState,
  useRef,
  useCallback,
  useMemo,
} from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  downloadTextAsFile,
  inferRedemptionMode,
  showError,
  showSuccess,
  renderQuota,
  renderNumber,
  yuanToFen,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  Modal,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Form,
  Avatar,
  Row,
  Col,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconSave,
  IconClose,
  IconGift,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const EditRedemptionModal = (props) => {
  const { t } = useTranslation();
  const isEdit = props.editingRedemption.id !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);

  const DEFAULT_QUOTA_TOKENS = 100000;
  const DEFAULT_QUOTA_PER_USD = 500000; // 本地存储缺失时的美元兑额度后备比例

  // 从本地缓存读取额度换算比例，保证在页面无需二次请求时也能换算额度
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
  const redemptionMode = useMemo(
    () => inferRedemptionMode(props.editingRedemption),
    [props.editingRedemption],
  );
  const isSubscription = redemptionMode === 'subscription';
  const isActivation = redemptionMode === 'activation';

  const getInitValues = useCallback(
    (mode = 'subscription') => {
      const defaultQuotaUSD = convertQuotaToUSD(DEFAULT_QUOTA_TOKENS);
      const baseValues = {
        name: '',
        price_yuan: 0,
        quota_usd: mode === 'activation' ? 0 : defaultQuotaUSD,
        daily_quota_limit_usd: mode === 'subscription' ? null : 0,
        quota_valid_days: mode === 'subscription' ? 30 : 0,
        allowed_group_ids: [],
        count: 1,
        expired_time: null,
      };
      if (mode === 'activation') {
        return {
          ...baseValues,
          daily_quota_limit_usd: 0,
          quota_valid_days: 0,
        };
      }
      return baseValues;
    },
    [convertQuotaToUSD],
  );

  const formInitValues = useMemo(
    () => getInitValues(redemptionMode),
    [getInitValues, redemptionMode],
  );

  const handleCancel = () => {
    props.handleClose();
  };

  const normalizeGroups = useCallback((raw) => {
    const list = Array.isArray(raw) ? raw : [];
    return list
      .map((g) => {
        if (typeof g === 'string') {
          const code = String(g || '').trim();
          return code ? { code, display_name: code } : null;
        }
        const id = Number(g?.id || 0);
        const code = String(g?.code || '').trim();
        if (!code) return null;
        const displayName =
          String(g?.display_name || g?.name || code).trim() || code;
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
        const normalized = normalizeGroups(data);
        setAvailableGroups(normalized);
        return normalized;
      } else {
        showError(message || t('获取分组失败'));
      }
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
    } finally {
      setGroupsLoading(false);
    }
    return [];
  }, [normalizeGroups, t]);

  const groupOptions = useMemo(() => {
    return (Array.isArray(availableGroups) ? availableGroups : [])
      .map((g) => ({
        label: g?.display_name || g?.code,
        value: g?.id,
      }))
      .filter((opt) => opt.value);
  }, [availableGroups]);

  const loadRedemption = useCallback(async () => {
    setLoading(true);
    try {
      const [, res] = await Promise.all([
        loadGroups(),
        API.get(`/api/redemption/${props.editingRedemption.id}`),
      ]);

      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      const recordMode = inferRedemptionMode(data);
      const baseValues = getInitValues(recordMode);
      const expireValue =
        data.expired_time === 0 || !data.expired_time
          ? null
          : new Date(data.expired_time * 1000);

      const allowedGroupValues = Array.isArray(data?.allowed_group_ids)
        ? data.allowed_group_ids
            .map((v) => parseInt(v, 10))
            .filter((v) => Number.isFinite(v) && v > 0)
        : [];

      const initValues = {
        ...baseValues,
        name: data.name || baseValues.name,
        price_yuan: (data.price_fen || 0) / 100,
        quota_usd: convertQuotaToUSD(data.quota),
        daily_quota_limit_usd:
          recordMode === 'subscription'
            ? convertQuotaToUSD(data.daily_quota_limit || 0)
            : baseValues.daily_quota_limit_usd,
        expired_time: expireValue,
        quota_valid_days:
          recordMode === 'subscription'
            ? data.quota_valid_days || 0
            : baseValues.quota_valid_days,
        allowed_group_ids:
          recordMode === 'subscription'
            ? allowedGroupValues
            : baseValues.allowed_group_ids,
        count: data.count || baseValues.count,
      };
      formApiRef.current?.setValues(initValues);
    } finally {
      setLoading(false);
    }
  }, [
    convertQuotaToUSD,
    getInitValues,
    inferRedemptionMode,
    loadGroups,
    props.editingRedemption.id,
  ]);

  useEffect(() => {
    if (!props.visiable) return;
    if (!formApiRef.current) return;
    if (isEdit) {
      loadRedemption();
    } else {
      loadGroups();
      formApiRef.current.setValues(formInitValues);
    }
  }, [formInitValues, isEdit, loadGroups, loadRedemption, props.visiable]);

  const submit = async (values) => {
    const mode = redemptionMode;
    if (isEdit && mode !== 'subscription') {
      showError(t('当前仅支持编辑订阅额度兑换码'));
      return;
    }
    let name = values.name;
    let localInputs = { ...values, mode };

    if (mode === 'activation') {
      if (!isEdit && (!name || name === '')) {
        name = 'ClawBox 激活码';
      }
      localInputs = {
        ...localInputs,
        name,
        quota: 0,
        daily_quota_limit: 0,
        daily_request_limit: 0,
        quota_valid_days: 0,
      };
      delete localInputs.allowed_group_ids;
    } else {
      const quotaTokens = convertUSDToQuota(values.quota_usd);
      if (quotaTokens <= 0) {
        showError(t('额度必须大于0'));
        return;
      }
      if (!isEdit && (!name || name === '')) {
        name = renderQuota(quotaTokens);
      }

      localInputs = { ...localInputs, name, quota: quotaTokens };

      if (mode === 'subscription') {
        const dailyLimitUSD = parseFloat(values.daily_quota_limit_usd);
        if (!Number.isFinite(dailyLimitUSD) || dailyLimitUSD <= 0) {
          showError(t('每日限额必须大于0'));
          return;
        }
        localInputs.daily_quota_limit = convertUSDToQuota(dailyLimitUSD);
        const quotaValidDays = parseInt(values.quota_valid_days, 10);
        if (!Number.isFinite(quotaValidDays) || quotaValidDays < 0) {
          showError(t('额度有效期不能小于0天'));
          return;
        }
        localInputs.quota_valid_days = quotaValidDays;

        const rawAllowedGroups = Array.isArray(values.allowed_group_ids)
          ? values.allowed_group_ids
          : [];
        const seen = new Set();
        const allowedGroupIds = rawAllowedGroups
          .map((v) => {
            const id = typeof v === 'number' ? v : parseInt(v, 10);
            return Number.isFinite(id) ? id : 0;
          })
          .map((v) => Math.trunc(v))
          .filter((v) => v > 0)
          .filter((v) => {
            if (seen.has(v)) return false;
            seen.add(v);
            return true;
          });
        if (allowedGroupIds.length === 0) {
          showError(t('请选择可用分组'));
          return;
        }
        localInputs.allowed_group_ids = allowedGroupIds;
      } else {
        localInputs.daily_quota_limit = 0;
        localInputs.quota_valid_days = 0;
        delete localInputs.allowed_group_ids;
      }
    }

    let priceFen = 0;
    try {
      if (
        localInputs.price_yuan !== undefined &&
        localInputs.price_yuan !== null
      ) {
        priceFen = yuanToFen(localInputs.price_yuan);
      }
    } catch (e) {
      showError(e?.message || t('金额格式错误'));
      return;
    }
    localInputs.price_fen = priceFen;
    delete localInputs.price_yuan;

    localInputs.count = parseInt(localInputs.count, 10) || 0;

    delete localInputs.daily_quota_limit_usd;
    delete localInputs.quota_usd;

    setLoading(true);

    if (!localInputs.expired_time) {
      localInputs.expired_time = 0;
    } else {
      localInputs.expired_time = Math.floor(
        localInputs.expired_time.getTime() / 1000,
      );
    }

    let res;
    if (isEdit) {
      res = await API.put(`/api/redemption/`, {
        ...localInputs,
        id: parseInt(props.editingRedemption.id, 10),
      });
    } else {
      res = await API.post(`/api/redemption/`, {
        ...localInputs,
      });
    }

    const { success, message, data } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('兑换码更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showSuccess(t('兑换码创建成功！'));
        props.refresh();
        const initValues = getInitValues(redemptionMode);
        formApiRef.current?.setValues(initValues);
        props.handleClose();
      }
    } else {
      showError(message);
    }
    setLoading(false);
    if (!isEdit && data) {
      let text = '';
      for (let i = 0; i < data.length; i++) {
        text += data[i] + '\n';
      }
      Modal.confirm({
        title: t('兑换码创建成功'),
        content: (
          <div>
            <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
            <p>{t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}</p>
          </div>
        ),
        onOk: () => {
          downloadTextAsFile(text, `${localInputs.name}.txt`);
        },
      });
    }
    setLoading(false);
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            {isEdit ? (
              <Tag color='blue' shape='circle'>
                {t('更新')}
              </Tag>
            ) : (
              <Tag color='green' shape='circle'>
                {t('新建')}
              </Tag>
            )}
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新兑换码信息') : t('创建新的兑换码')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visiable}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleCancel}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
      >
        <Spin spinning={loading}>
          <Form
            initValues={formInitValues}
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Basic Info */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconGift size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的基本信息')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='name'
                        label={t('名称')}
                        placeholder={t('请输入名称')}
                        style={{ width: '100%' }}
                        rules={
                          !isEdit
                            ? []
                            : [{ required: true, message: t('请输入名称') }]
                        }
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.InputNumber
                        field='price_yuan'
                        label={t('结算价格')}
                        placeholder={t('请输入价格')}
                        prefix='￥'
                        precision={2}
                        min={0}
                        step={1}
                        style={{ width: '100%' }}
                        extraText={t('用于邀请返佣结算')}
                      />
                    </Col>
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
                  {isSubscription ? (
                    <Row gutter={12} className='mt-2'>
                      <Col span={24}>
                        <Form.InputNumber
                          field='daily_quota_limit_usd'
                          label={t('日限额（USD）')}
                          placeholder={t('请输入日限额（美元）')}
                          precision={2}
                          min={0}
                          step={1}
                          prefix='$'
                          rules={[
                            {
                              required: true,
                              message: t('请输入日限额（美元）'),
                            },
                            {
                              validator: (rule, v) => {
                                const num = parseFloat(v);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('日限额必须大于0'));
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
                      <Col span={24}>
                        <Form.InputNumber
                          field='quota_valid_days'
                          label={t('额度有效期（天）')}
                          placeholder={t('请输入额度有效期（天）')}
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
                                return Number.isFinite(num) && num >= 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度有效期不能小于0天'));
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Select
                          field='allowed_group_ids'
                          label={t('可用分组')}
                          placeholder={t('请选择可用分组')}
                          optionList={groupOptions}
                          loading={groupsLoading}
                          multiple
                          search
                          rules={[
                            { required: true, message: t('请选择可用分组') },
                          ]}
                          style={{ width: '100%' }}
                          extraText={t('限制该订阅额度可消费的渠道分组')}
                        />
                      </Col>
                      <Col span={24}>
                        <div className='text-xs text-gray-500'>
                          {(() => {
                            const days = Number(values.quota_valid_days) || 0;
                            if (days === 0) {
                              return t(
                                '订阅额度仅在兑换当日有效，过期剩余额度自动失效。',
                              );
                            }
                            return t(
                              '订阅额度将在兑换成功后 {{days}} 天内有效，过期剩余额度自动失效。',
                              {
                                days,
                              },
                            );
                          })()}
                        </div>
                      </Col>
                    </Row>
                  ) : isActivation ? (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('激活码仅用于启用 ClawBox 首次使用资格，不发放额度。')}
                    </div>
                  ) : (
                    <div className='mt-2 text-xs text-gray-500'>
                      {t('当前兑换码类型不展示额外的订阅限制项。')}
                    </div>
                  )}
                </Card>

                <Card className='!rounded-2xl shadow-sm border-0'>
                  {/* Header: Quota Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='green'
                      className='mr-2 shadow-md'
                    >
                      <IconCreditCard size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('额度设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的额度和数量')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    {!isActivation && (
                      <Col span={12}>
                        <Form.AutoComplete
                          field='quota_usd'
                          label={`${t('额度')} ($)`}
                          placeholder={`${t('请输入额度')} ($)`}
                          style={{ width: '100%' }}
                          type='number'
                          rules={[
                            { required: true, message: t('请输入额度') },
                            {
                              validator: (rule, v) => {
                                const num = parseFloat(v);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度必须大于0'));
                              },
                            },
                          ]}
                          extraText={t('折算额度：{{amount}} tokens', {
                            amount: renderNumber(
                              convertUSDToQuota(values.quota_usd),
                            ),
                          })}
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
                    )}
                    {!isEdit && (
                      <Col span={isActivation ? 24 : 12}>
                        <Form.InputNumber
                          field='count'
                          label={t('生成数量')}
                          min={1}
                          rules={[
                            { required: true, message: t('请输入生成数量') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('生成数量必须大于0'));
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                          showClear
                        />
                      </Col>
                    )}
                  </Row>
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default EditRedemptionModal;
