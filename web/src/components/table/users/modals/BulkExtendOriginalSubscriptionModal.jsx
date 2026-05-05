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
import {
  API,
  inferPresetMode,
  showError,
  showSuccess,
  timestamp2string,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Avatar,
  Button,
  Card,
  Col,
  Form,
  Modal,
  Row,
  SideSheet,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconSave,
  IconSearch,
  IconSetting,
} from '@douyinfe/semi-icons';

const { Text } = Typography;

const normalizeOrderedUniquePositiveIds = (rawIds) => {
  if (!Array.isArray(rawIds)) return [];
  const seen = new Set();
  const out = [];
  rawIds.forEach((raw) => {
    const num = Number(raw);
    if (!Number.isFinite(num) || num <= 0) return;
    const id = Math.trunc(num);
    if (id <= 0 || seen.has(id)) return;
    seen.add(id);
    out.push(id);
  });
  return out;
};

const parseUserIds = (raw) => {
  const input = String(raw || '').trim();
  if (!input) return [];
  const parts = input.split(/[\s,，;；]+/).filter(Boolean);
  if (parts.length === 0) return [];

  const ids = [];
  const seen = new Set();
  for (const part of parts) {
    if (!/^\d+$/.test(part)) {
      throw new Error(`excluded_user_ids 中包含无效值：${part}`);
    }
    const id = parseInt(part, 10);
    if (!Number.isFinite(id) || id <= 0) {
      throw new Error(`excluded_user_ids 中包含无效用户ID：${part}`);
    }
    if (!seen.has(id)) {
      seen.add(id);
      ids.push(id);
    }
  }
  return ids;
};

const parseUsernames = (raw) => {
  const input = String(raw || '').trim();
  if (!input) return [];
  const parts = input.split(/[\s,，;；]+/).filter(Boolean);
  if (parts.length === 0) return [];

  const usernames = [];
  const seen = new Set();
  parts.forEach((part) => {
    const value = String(part || '').trim();
    if (!value || seen.has(value)) return;
    seen.add(value);
    usernames.push(value);
  });
  return usernames;
};

const getModeLabel = (t, mode) => {
  switch (mode) {
    case 'subscription':
      return t('额度订阅');
    case 'tokens':
      return t('Token订阅');
    case 'request':
      return t('次数订阅');
    default:
      return mode || t('未知类型');
  }
};

const normalizeSubscriptionPresets = (rawPresets) => {
  const list = Array.isArray(rawPresets) ? rawPresets : [];
  const out = [];
  const seen = new Set();
  list.forEach((item) => {
    const id = Number(item?.id ?? 0);
    if (!Number.isFinite(id) || id <= 0 || seen.has(id)) return;
    const mode = inferPresetMode(item);
    if (!['subscription', 'tokens', 'request'].includes(mode)) return;
    const name = String(item?.name ?? '').trim();
    if (!name) return;
    seen.add(id);
    out.push({
      ...item,
      id,
      mode,
      name,
      enabled: item?.enabled !== false,
      sort_order: Number.isFinite(Number(item?.sort_order ?? 0))
        ? Math.max(0, Math.trunc(Number(item?.sort_order ?? 0)))
        : 0,
    });
  });
  out.sort((a, b) => {
    const ea = a?.enabled !== false;
    const eb = b?.enabled !== false;
    if (ea !== eb) return ea ? -1 : 1;
    const sa = Number(a?.sort_order ?? 0) || 0;
    const sb = Number(b?.sort_order ?? 0) || 0;
    if (sa !== sb) return sb - sa;
    return (Number(b?.id ?? 0) || 0) - (Number(a?.id ?? 0) || 0);
  });
  return out;
};

const canonicalizePayload = (payload) => ({
  fault_start_at: payload?.fault_start_at,
  fault_end_at: payload?.fault_end_at,
  source_preset_ids: normalizeOrderedUniquePositiveIds(payload?.source_preset_ids).sort(
    (a, b) => a - b,
  ),
  excluded_user_ids: normalizeOrderedUniquePositiveIds(payload?.excluded_user_ids).sort(
    (a, b) => a - b,
  ),
  excluded_usernames: Array.isArray(payload?.excluded_usernames)
    ? [...payload.excluded_usernames]
        .map((item) => String(item || '').trim())
        .filter(Boolean)
        .sort((a, b) => a.localeCompare(b))
    : [],
  extend_days: payload?.extend_days,
});

const formatTs = (ts) => {
  const value = Number(ts) || 0;
  if (value <= 0) return '-';
  return timestamp2string(value);
};

const BulkExtendOriginalSubscriptionModal = (props) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);

  const [loading, setLoading] = useState(false);
  const [presetsLoading, setPresetsLoading] = useState(false);
  const [presets, setPresets] = useState([]);
  const [lastResult, setLastResult] = useState(null);
  const [lastPreviewKey, setLastPreviewKey] = useState('');
  const [executeConfirmVisible, setExecuteConfirmVisible] = useState(false);
  const [pendingExecutePayload, setPendingExecutePayload] = useState(null);

  const presetOptions = useMemo(
    () =>
      presets.map((preset) => ({
        value: preset.id,
        label: `${preset.name} · ${getModeLabel(t, preset.mode)} · #${preset.id}${
          preset.enabled === false ? ` · ${t('未上架')}` : ''
        }`,
      })),
    [presets, t],
  );

  const initialValues = useMemo(
    () => ({
      fault_start_at_date: dayjs().startOf('day').toDate(),
      fault_end_at_date: dayjs().endOf('day').toDate(),
      source_preset_ids: [],
      excluded_user_ids_raw: '',
      excluded_usernames_raw: '',
      extend_days: 1,
    }),
    [],
  );

  const loadPresets = async () => {
    setPresetsLoading(true);
    try {
      const res = await API.get('/api/redemption/presets', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取商品列表失败'));
        setPresets([]);
        return [];
      }
      const normalized = normalizeSubscriptionPresets(data);
      setPresets(normalized);
      return normalized;
    } catch (error) {
      showError(error?.message || t('获取商品列表失败'));
      setPresets([]);
      return [];
    } finally {
      setPresetsLoading(false);
    }
  };

  useEffect(() => {
    if (!props.visible) return;
    setLastResult(null);
    setLastPreviewKey('');
    setExecuteConfirmVisible(false);
    setPendingExecutePayload(null);
    formApiRef.current?.setValues(initialValues);
    loadPresets();
  }, [props.visible, initialValues]);

  const buildNormalizedPayload = (values) => {
    const sourcePresetIds = normalizeOrderedUniquePositiveIds(values?.source_preset_ids);
    if (sourcePresetIds.length === 0) {
      throw new Error(t('请选择筛选商品范围'));
    }

    const excludedUserIds = parseUserIds(values?.excluded_user_ids_raw);
    const excludedUsernames = parseUsernames(values?.excluded_usernames_raw);

    const startDate = values?.fault_start_at_date;
    const endDate = values?.fault_end_at_date;
    if (!(startDate instanceof Date) || Number.isNaN(startDate.getTime())) {
      throw new Error(t('请选择故障开始时间'));
    }
    if (!(endDate instanceof Date) || Number.isNaN(endDate.getTime())) {
      throw new Error(t('请选择故障结束时间'));
    }
    const faultStartAt = Math.floor(startDate.getTime() / 1000);
    const faultEndAt = Math.floor(endDate.getTime() / 1000);
    if (faultStartAt <= 0 || faultEndAt <= 0) {
      throw new Error(t('故障时间无效'));
    }
    if (faultStartAt > faultEndAt) {
      throw new Error(t('故障开始时间不能晚于结束时间'));
    }

    const extendDaysRaw = Number(values?.extend_days ?? 0);
    const extendDays = Number.isFinite(extendDaysRaw)
      ? Math.max(1, Math.floor(extendDaysRaw))
      : 0;
    if (extendDays <= 0 || extendDays > 3650) {
      throw new Error(t('extend_days 不合法'));
    }

    return {
      fault_start_at: faultStartAt,
      fault_end_at: faultEndAt,
      source_preset_ids: sourcePresetIds,
      excluded_user_ids: excludedUserIds,
      excluded_usernames: excludedUsernames,
      extend_days: extendDays,
    };
  };

  const buildRequestKey = (payload) => JSON.stringify(canonicalizePayload(payload));

  const run = async (dryRun, overridePayload = null) => {
    if (!formApiRef.current) return;
    setLoading(true);
    try {
      await formApiRef.current.validate();
      const payload =
        overridePayload || buildNormalizedPayload(formApiRef.current.getValues());
      const res = await API.post('/api/user/subscriptions/bulk/original-compensation', {
        ...payload,
        dry_run: Boolean(dryRun),
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('请求失败'));
        return;
      }
      setLastResult(data);
      if (dryRun) {
        setLastPreviewKey(buildRequestKey(payload));
        showSuccess(t('预览完成'));
      } else {
        showSuccess(
          t('已完成原订阅补偿：额度 {{quota}} 条，Token {{tokens}} 条，次数 {{request}} 条', {
            quota: Number(data?.extended_quota_subscription_count ?? 0) || 0,
            tokens: Number(data?.extended_token_subscription_count ?? 0) || 0,
            request: Number(data?.extended_request_subscription_count ?? 0) || 0,
          }),
        );
        setLastPreviewKey('');
        props.refresh?.();
      }
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  const handlePreview = async () => {
    await run(true);
  };

  const handleExecute = async () => {
    if (!formApiRef.current) return;
    let payload;
    try {
      payload = buildNormalizedPayload(formApiRef.current.getValues());
    } catch (error) {
      showError(error);
      return;
    }

    if (lastPreviewKey !== buildRequestKey(payload)) {
      showError(t('请先预览当前参数，再执行补偿'));
      return;
    }

    setPendingExecutePayload(payload);
    setExecuteConfirmVisible(true);
  };

  return (
    <>
      <Modal
        title={t('确认执行按原订阅延长补偿？')}
        visible={executeConfirmVisible}
        onCancel={() => {
          setExecuteConfirmVisible(false);
          setPendingExecutePayload(null);
        }}
        onOk={async () => {
          setExecuteConfirmVisible(false);
          await run(false, pendingExecutePayload);
          setPendingExecutePayload(null);
        }}
      >
        <div className='space-y-2'>
          <Text className='block'>
            {t('将按当前预览参数，对命中的原订阅记录逐条延长并补对应天数额度。')}
          </Text>
          <Text className='block text-xs text-gray-600'>
            {t('仅处理有限期、有日限且总量有限的额度/Token/次数订阅。')}
          </Text>
        </div>
      </Modal>

      <SideSheet
        placement='right'
        title={t('批量延长原订阅补偿')}
        visible={props.visible}
        closeIcon={null}
        width={isMobile ? '100%' : 640}
        onCancel={props.handleClose}
        footer={
          <Space>
            <Button icon={<IconClose />} onClick={props.handleClose}>
              {t('取消')}
            </Button>
            <Button
              icon={<IconSearch />}
              loading={loading}
              theme='light'
              type='primary'
              onClick={handlePreview}
            >
              {t('预览')}
            </Button>
            <Button
              icon={<IconSave />}
              loading={loading}
              type='primary'
              onClick={handleExecute}
            >
              {t('执行补偿')}
            </Button>
          </Space>
        }
      >
        <Spin spinning={loading || presetsLoading}>
          <Form
            getFormApi={(api) => {
              formApiRef.current = api;
            }}
            initValues={initialValues}
            onValueChange={() => {
              if (!lastPreviewKey) return;
              try {
                const payload = buildNormalizedPayload(formApiRef.current?.getValues());
                const key = buildRequestKey(payload);
                if (key !== lastPreviewKey) {
                  setLastPreviewKey('');
                  if (lastResult?.dry_run) {
                    setLastResult(null);
                  }
                }
              } catch (error) {
                setLastPreviewKey('');
                if (lastResult?.dry_run) {
                  setLastResult(null);
                }
              }
            }}
          >
            <div className='space-y-4'>
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconSetting size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('筛选范围')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t(
                        '只处理故障时间窗内曾生效、来源商品命中白名单，且原订阅本身为有限期+有日限+总量有限的记录',
                      )}
                    </div>
                  </div>
                </div>

                <Row gutter={12}>
                  <Col span={12}>
                    <Form.DatePicker
                      field='fault_start_at_date'
                      label={t('故障开始时间')}
                      type='dateTime'
                      format='yyyy/MM/dd HH:mm:ss'
                      showClear={false}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={12}>
                    <Form.DatePicker
                      field='fault_end_at_date'
                      label={t('故障结束时间')}
                      type='dateTime'
                      format='yyyy/MM/dd HH:mm:ss'
                      showClear={false}
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>

                <Form.Select
                  field='source_preset_ids'
                  label={t('筛选商品范围')}
                  placeholder={t('请选择需要统计的来源商品')}
                  optionList={presetOptions}
                  multiple
                  search
                  rules={[{ required: true, message: t('请选择筛选商品范围') }]}
                  style={{ width: '100%', marginTop: 12 }}
                  extraText={t('例如可以不勾选 0.1 试用商品')}
                />

                <Row gutter={12} style={{ marginTop: 12 }}>
                  <Col span={12}>
                    <Form.TextArea
                      field='excluded_user_ids_raw'
                      label={t('排除用户ID（可选）')}
                      placeholder={t('支持逗号/空格/换行分隔')}
                      autosize
                      rows={2}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={12}>
                    <Form.TextArea
                      field='excluded_usernames_raw'
                      label={t('排除用户名（可选）')}
                      placeholder={t('支持逗号/空格/换行分隔')}
                      autosize
                      rows={2}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>

              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconSave size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('补偿方式')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('对命中的每一条原订阅直接往后延长 N 天，并补 N 天对应额度')}
                    </div>
                  </div>
                </div>

                <Form.InputNumber
                  field='extend_days'
                  label={t('延长天数')}
                  min={1}
                  max={3650}
                  step={1}
                  precision={0}
                  rules={[{ required: true, message: t('请输入延长天数') }]}
                  style={{ width: '100%' }}
                />
              </Card>

              {lastResult ? (
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
                      <Text className='text-lg font-medium'>{t('结果')}</Text>
                      <div className='text-xs text-gray-600'>
                        {lastResult.dry_run
                          ? t('预览结果（未落库）')
                          : t('执行结果（已落库）')}
                      </div>
                    </div>
                  </div>

                  <div className='space-y-1'>
                    <Text className='block'>
                      {t('故障时间窗：{{start}} ~ {{end}}', {
                        start: formatTs(lastResult.fault_start_at),
                        end: formatTs(lastResult.fault_end_at),
                      })}
                    </Text>
                    <Text className='block'>
                      {t('延长天数：{{count}}', {
                        count: Number(lastResult.extend_days ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配用户：{{count}}', {
                        count: Number(lastResult.matched_user_count ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('排除名单解析用户：{{count}}', {
                        count: Number(lastResult.resolved_excluded_user_count ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('命中后被排除用户：{{count}}', {
                        count: Number(lastResult.excluded_matched_user_count ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配额度订阅：{{count}} 条，预计增加额度：{{amount}}', {
                        count: Number(lastResult.matched_quota_subscription_count ?? 0) || 0,
                        amount:
                          Number(lastResult.matched_quota_compensation_amount ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配Token订阅：{{count}} 条，预计增加Token：{{amount}}', {
                        count: Number(lastResult.matched_token_subscription_count ?? 0) || 0,
                        amount:
                          Number(lastResult.matched_token_compensation_amount ?? 0) || 0,
                      })}
                    </Text>
                    <Text className='block'>
                      {t('匹配次数订阅：{{count}} 条，预计增加次数：{{amount}}', {
                        count:
                          Number(lastResult.matched_request_subscription_count ?? 0) || 0,
                        amount:
                          Number(lastResult.matched_request_compensation_amount ?? 0) || 0,
                      })}
                    </Text>
                    {!lastResult.dry_run ? (
                      <>
                        <Text className='block'>
                          {t('已延长额度订阅：{{count}} 条', {
                            count:
                              Number(lastResult.extended_quota_subscription_count ?? 0) || 0,
                          })}
                        </Text>
                        <Text className='block'>
                          {t('已延长Token订阅：{{count}} 条', {
                            count:
                              Number(lastResult.extended_token_subscription_count ?? 0) || 0,
                          })}
                        </Text>
                        <Text className='block'>
                          {t('已延长次数订阅：{{count}} 条', {
                            count:
                              Number(lastResult.extended_request_subscription_count ?? 0) ||
                              0,
                          })}
                        </Text>
                      </>
                    ) : null}
                  </div>

                  {Array.isArray(lastResult.matched_user_ids_preview) &&
                  lastResult.matched_user_ids_preview.length > 0 ? (
                    <div className='mt-3'>
                      <Text className='block mb-2 text-xs text-gray-600'>
                        {t('命中用户预览')}
                      </Text>
                      <div className='flex flex-wrap gap-2'>
                        {lastResult.matched_user_ids_preview.map((userId) => (
                          <Tag key={`matched-user-${userId}`} color='grey'>
                            #{userId}
                          </Tag>
                        ))}
                        {lastResult.matched_user_ids_preview_more ? (
                          <Tag color='orange'>{t('还有更多')}</Tag>
                        ) : null}
                      </div>
                    </div>
                  ) : null}

                  {Array.isArray(lastResult.resolved_excluded_user_ids_preview) &&
                  lastResult.resolved_excluded_user_ids_preview.length > 0 ? (
                    <div className='mt-3'>
                      <Text className='block mb-2 text-xs text-gray-600'>
                        {t('排除用户预览')}
                      </Text>
                      <div className='flex flex-wrap gap-2'>
                        {lastResult.resolved_excluded_user_ids_preview.map((userId) => (
                          <Tag key={`excluded-user-${userId}`} color='orange'>
                            #{userId}
                          </Tag>
                        ))}
                        {lastResult.resolved_excluded_user_ids_preview_more ? (
                          <Tag color='orange'>{t('还有更多')}</Tag>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </Card>
              ) : null}
            </div>
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default BulkExtendOriginalSubscriptionModal;
