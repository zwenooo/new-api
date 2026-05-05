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

import React, { useEffect, useMemo, useState, useRef } from 'react';
import {
  API,
  isEffectiveAdmin,
  showError,
  showSuccess,
  timestamp2string,
  renderQuotaWithPrompt,
  renderQuotaToUSD,
  getModelCategories,
  selectFilter,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Form,
  Col,
  Row,
  Modal,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import TokenGroupPrioritySelector, {
  normalizeTokenGroupIds,
} from '../../../token/TokenGroupPrioritySelector';

const { Text, Title } = Typography;

const EditTokenModal = (props) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupLabelsLoading, setGroupLabelsLoading] = useState(false);
  const [groupLabelById, setGroupLabelById] = useState({});
  const [showGroupSelector, setShowGroupSelector] = useState(false);
  const [selectedAllowedGroupIds, setSelectedAllowedGroupIds] = useState([]);
  const [draftAllowedGroupIds, setDraftAllowedGroupIds] = useState([]);
  const isEdit = props.editingToken.id !== undefined;
  const isAdminUser = isEffectiveAdmin();

  const baseInitValues = useMemo(
    () => ({
      name: '',
      remain_quota: 500000,
      change_quota: false,
      daily_quota_limit: 0,
      daily_quota_unlimited: true,
      expired_time: -1,
      unlimited_quota: !isAdminUser,
      model_limits_enabled: false,
      model_limits: [],
      allow_ips: '',
      allowed_group_ids: [],
      tokenCount: 1,
    }),
    [isAdminUser],
  );

  const handleCancel = () => {
    props.handleClose();
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    try {
      let res = await API.get(`/api/user/models`);
      const { success, message, data } = res.data;
      if (success) {
        const categories = getModelCategories(t);
        const modelList = Array.isArray(data) ? data : [];
        let localModelOptions = modelList.map((model) => {
          let icon = null;
          for (const [key, category] of Object.entries(categories)) {
            if (key !== 'all' && category.filter({ model_name: model })) {
              icon = category.icon;
              break;
            }
          }
          return {
            label: (
              <span className='flex items-center gap-1'>
                {icon}
                {model}
              </span>
            ),
            value: model,
          };
        });
        setModels(localModelOptions);
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(error?.message || t('获取模型失败'));
    }
  };

  const loadGroups = async () => {
    setGroupsLoading(true);
    try {
      let res = await API.get(`/api/user/self/groups`);
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message));
        return;
      }

      const list = Array.isArray(data) ? data : [];
      const localGroupOptions = list
        .map((item) => {
          const id = Number(item?.id ?? 0);
          if (!Number.isFinite(id) || id <= 0) return null;
          const label = String(item?.display_name || item?.code || '').trim();
          if (!label) return null;
          const desc = String(item?.desc || '').trim();
          return {
            label,
            value: Math.floor(id),
            ratio: item?.ratio,
            desc,
            billable: Boolean(item?.billable),
            no_billing: Boolean(item?.no_billing),
          };
        })
        .filter(Boolean)
        .sort((a, b) => a.value - b.value);
      setGroups(localGroupOptions);
    } catch (error) {
      showError(error?.message || t('获取分组失败'));
    } finally {
      setGroupsLoading(false);
    }
  };

  const loadGroupLabels = async () => {
    setGroupLabelsLoading(true);
    try {
      const res = await API.get('/api/group/resolve');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(t(message || '获取分组失败'));
        return;
      }

      const labels = {};
      (Array.isArray(data) ? data : []).forEach((item) => {
        const id = Number(item?.id ?? 0);
        if (!Number.isFinite(id) || id <= 0) return;
        const label = String(item?.display_name || item?.code || '').trim();
        if (!label) return;
        labels[Math.floor(id)] = label;
      });
      setGroupLabelById(labels);
    } catch (error) {
      showError(error?.message || t('获取分组失败'));
    } finally {
      setGroupLabelsLoading(false);
    }
  };

  const resolveGroupLabel = (groupId) => {
    const id = Number(groupId);
    if (!Number.isFinite(id) || id <= 0) {
      return t('未知分组');
    }
    const normalizedId = Math.floor(id);
    const optionLabel = groups.find(
      (item) => item.value === normalizedId,
    )?.label;
    return (
      optionLabel ||
      groupLabelById[normalizedId] ||
      `${t('未知分组')} (#${normalizedId})`
    );
  };

  const loadToken = async () => {
    setLoading(true);
    try {
      let res = await API.get(`/api/token/${props.editingToken.id}`);
      const { success, message, data } = res.data;
      if (success) {
        if (data.expired_time !== -1) {
          data.expired_time = timestamp2string(data.expired_time);
        }
        if (data.model_limits !== '') {
          data.model_limits = data.model_limits.split(',');
        } else {
          data.model_limits = [];
        }
        data.allowed_group_ids = normalizeTokenGroupIds(data.allowed_group_ids);
        if (formApiRef.current) {
          const initValues = {
            ...baseInitValues,
            ...data,
            remain_quota: undefined,
            change_quota: false,
            daily_quota_limit: data.daily_quota_limit ?? 0,
            daily_quota_unlimited: data.daily_quota_limit === 0,
          };
          formApiRef.current.setValues(initValues);
        }
        setSelectedAllowedGroupIds(data.allowed_group_ids);
        setDraftAllowedGroupIds(data.allowed_group_ids);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error?.message || t('获取令牌失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        formApiRef.current.setValues(baseInitValues);
      }
    }
    setSelectedAllowedGroupIds(baseInitValues.allowed_group_ids);
    setDraftAllowedGroupIds(baseInitValues.allowed_group_ids);
    loadModels();
    loadGroups();
    loadGroupLabels();
  }, [props.editingToken.id]);

  useEffect(() => {
    if (props.visiable) {
      // Groups can change after user purchases (subscriptions/PAYG); refresh on open to avoid stale options.
      loadGroups();
      loadGroupLabels();
      if (isEdit) {
        loadToken();
      } else {
        formApiRef.current?.setValues(baseInitValues);
        setSelectedAllowedGroupIds(baseInitValues.allowed_group_ids);
        setDraftAllowedGroupIds(baseInitValues.allowed_group_ids);
      }
    } else {
      formApiRef.current?.reset();
      setShowGroupSelector(false);
      setSelectedAllowedGroupIds([]);
      setDraftAllowedGroupIds([]);
    }
  }, [props.visiable, props.editingToken.id]);

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const submit = async (values) => {
    setLoading(true);
    const allowedGroupIds = normalizeTokenGroupIds(selectedAllowedGroupIds);
    const mustResetQuota =
      isEdit &&
      Boolean(props.editingToken?.unlimited_quota) &&
      !values.unlimited_quota;
    const shouldSendQuota = !isEdit || values.change_quota || mustResetQuota;
    if (allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      setLoading(false);
      return;
    }

    if (isEdit) {
      let { tokenCount: _tc, ...localInputs } = values;
      if (shouldSendQuota) {
        localInputs.remain_quota = parseInt(localInputs.remain_quota);
      } else {
        delete localInputs.remain_quota;
      }
      // normalize daily quota on edit
      localInputs.daily_quota_limit =
        parseInt(localInputs.daily_quota_limit) || 0;
      if (localInputs.daily_quota_unlimited) {
        localInputs.daily_quota_limit = 0;
      }
      delete localInputs.daily_quota_unlimited;
      delete localInputs.change_quota;
      if (localInputs.expired_time !== -1) {
        let time = Date.parse(localInputs.expired_time);
        if (isNaN(time)) {
          showError(t('过期时间格式错误！'));
          setLoading(false);
          return;
        }
        localInputs.expired_time = Math.ceil(time / 1000);
      }
      localInputs.model_limits = localInputs.model_limits.join(',');
      localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
      localInputs.allowed_group_ids = allowedGroupIds;
      let res = await API.put(`/api/token/`, {
        ...localInputs,
        id: parseInt(props.editingToken.id),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        let { tokenCount: _tc, ...localInputs } = values;
        const baseName =
          values.name.trim() === '' ? 'default' : values.name.trim();
        if (i !== 0 || values.name.trim() === '') {
          localInputs.name = `${baseName}-${generateRandomSuffix()}`;
        } else {
          localInputs.name = baseName;
        }
        localInputs.remain_quota = parseInt(localInputs.remain_quota);
        localInputs.daily_quota_limit =
          parseInt(localInputs.daily_quota_limit) || 0;
        if (localInputs.daily_quota_unlimited) {
          localInputs.daily_quota_limit = 0;
        }
        delete localInputs.daily_quota_unlimited;
        delete localInputs.change_quota;

        if (localInputs.expired_time !== -1) {
          let time = Date.parse(localInputs.expired_time);
          if (isNaN(time)) {
            showError(t('过期时间格式错误！'));
            setLoading(false);
            break;
          }
          localInputs.expired_time = Math.ceil(time / 1000);
        }
        localInputs.model_limits = localInputs.model_limits.join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        localInputs.allowed_group_ids = allowedGroupIds;
        let res = await API.post(`/api/token/`, localInputs);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
  };

  const openGroupSelector = () => {
    setDraftAllowedGroupIds(normalizeTokenGroupIds(selectedAllowedGroupIds));
    setShowGroupSelector(true);
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
              {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
            </Title>
          </Space>
        }
        bodyStyle={{
          padding: '0',
          maxHeight: 'calc(100vh - 120px)',
          overflow: 'hidden',
        }}
        visible={props.visiable}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                className='!rounded-lg'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                className='!rounded-lg'
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
        <div className='scrollbar-hide max-h-[calc(100vh-120px)] overflow-y-auto'>
          <Spin spinning={loading}>
            <Form
              key={isEdit ? 'edit' : 'new'}
              initValues={baseInitValues}
              getFormApi={(api) => (formApiRef.current = api)}
              onSubmit={submit}
            >
              {({ values }) => {
                const selectedGroupIds = normalizeTokenGroupIds(
                  selectedAllowedGroupIds,
                );
                const mustResetQuota =
                  isEdit &&
                  Boolean(props.editingToken?.unlimited_quota) &&
                  !values.unlimited_quota;

                return (
                  <div className='p-2'>
                    {/* 基本信息 */}
                    <Card className='!rounded-2xl shadow-sm border-0'>
                      <div className='flex items-center mb-2'>
                        <Avatar
                          size='small'
                          color='blue'
                          className='mr-2 shadow-md'
                        >
                          <IconKey size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('基本信息')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('设置令牌的基本信息')}
                          </div>
                        </div>
                      </div>
                      <Row gutter={12}>
                        {isAdminUser && (
                          <Col span={24}>
                            <Form.Input
                              field='name'
                              label={t('名称')}
                              placeholder={t('请输入名称')}
                              rules={[
                                { required: true, message: t('请输入名称') },
                              ]}
                              showClear
                            />
                          </Col>
                        )}
                        <Col span={24}>
                          <Form.Slot label={t('分组优先级')}>
                            <div className='rounded-2xl border border-slate-200 bg-slate-50/80 p-4 dark:border-slate-700 dark:bg-slate-900/40'>
                              <div className='flex flex-wrap items-center justify-between gap-3'>
                                <div>
                                  <div className='text-sm font-medium text-slate-900 dark:text-slate-100'>
                                    {t('分组选择')}
                                  </div>
                                  <div className='mt-1 text-xs text-slate-500 dark:text-slate-300'>
                                    {t(
                                      '打开弹窗后可选择分组，并调整模型命中后的固定分组顺序',
                                    )}
                                  </div>
                                </div>
                                <Button
                                  theme='solid'
                                  type='primary'
                                  className='!rounded-lg'
                                  onClick={openGroupSelector}
                                  loading={groupsLoading || groupLabelsLoading}
                                >
                                  {selectedGroupIds.length > 0
                                    ? t('调整分组')
                                    : t('选择分组')}
                                </Button>
                              </div>

                              {selectedGroupIds.length > 0 ? (
                                <div className='mt-3 flex flex-wrap gap-2'>
                                  {selectedGroupIds.map((groupId, index) => (
                                    <Tag key={groupId} color='blue'>
                                      {`${index + 1}. ${resolveGroupLabel(groupId)}`}
                                    </Tag>
                                  ))}
                                </div>
                              ) : (
                                <div className='mt-3 rounded-xl border border-dashed border-slate-300 px-3 py-2 text-xs text-slate-500 dark:border-slate-600 dark:text-slate-300'>
                                  {t('尚未选择分组')}
                                </div>
                              )}
                            </div>
                          </Form.Slot>
                        </Col>
                        <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                          <Form.DatePicker
                            field='expired_time'
                            label={t('过期时间')}
                            type='dateTime'
                            placeholder={t('请选择过期时间')}
                            rules={[
                              { required: true, message: t('请选择过期时间') },
                              {
                                validator: (rule, value) => {
                                  if (value === -1 || !value)
                                    return Promise.resolve();
                                  const time = Date.parse(value);
                                  if (isNaN(time)) {
                                    return Promise.reject(
                                      t('过期时间格式错误！'),
                                    );
                                  }
                                  if (time <= Date.now()) {
                                    return Promise.reject(
                                      t('过期时间不能早于当前时间！'),
                                    );
                                  }
                                  return Promise.resolve();
                                },
                              },
                            ]}
                            showClear
                            style={{ width: '100%' }}
                          />
                        </Col>
                        <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                          <Form.Slot label={t('过期时间快捷设置')}>
                            <Space wrap>
                              <Button
                                theme='light'
                                type='primary'
                                onClick={() => setExpiredTime(0, 0, 0, 0)}
                              >
                                {t('永不过期')}
                              </Button>
                              <Button
                                theme='light'
                                type='tertiary'
                                onClick={() => setExpiredTime(1, 0, 0, 0)}
                              >
                                {t('一个月')}
                              </Button>
                              <Button
                                theme='light'
                                type='tertiary'
                                onClick={() => setExpiredTime(0, 1, 0, 0)}
                              >
                                {t('一天')}
                              </Button>
                              <Button
                                theme='light'
                                type='tertiary'
                                onClick={() => setExpiredTime(0, 0, 1, 0)}
                              >
                                {t('一小时')}
                              </Button>
                            </Space>
                          </Form.Slot>
                        </Col>
                        {!isEdit && (
                          <Col span={24}>
                            <Form.InputNumber
                              field='tokenCount'
                              label={t('新建数量')}
                              min={1}
                              extraText={t(
                                '批量创建时会在名称后自动添加随机后缀',
                              )}
                              rules={[
                                {
                                  required: true,
                                  message: t('请输入新建数量'),
                                },
                              ]}
                              style={{ width: '100%' }}
                            />
                          </Col>
                        )}
                      </Row>
                    </Card>

                    {/* 额度设置 */}
                    <Card className='!rounded-2xl shadow-sm border-0'>
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
                            {t('设置令牌可用额度和数量')}
                          </div>
                        </div>
                      </div>
                      <Row gutter={12}>
                        {isEdit ? (
                          <Col span={24}>
                            <Form.Switch
                              field='change_quota'
                              label={t('重设额度')}
                              size='large'
                              extraText={t(
                                '当前额度不会公开回显；关闭时保持现有令牌额度不变',
                              )}
                            />
                          </Col>
                        ) : null}
                        <Col span={24}>
                          <Form.AutoComplete
                            field='remain_quota'
                            label={isEdit ? t('新的额度') : t('额度')}
                            placeholder={
                              isEdit && !values.change_quota && !mustResetQuota
                                ? t('保持当前额度不变')
                                : t('请输入额度')
                            }
                            type='number'
                            disabled={
                              values.unlimited_quota ||
                              (isEdit && !values.change_quota && !mustResetQuota)
                            }
                            extraText={
                              isEdit && !values.change_quota && !mustResetQuota
                                ? t('未公开当前额度；如需修改，请开启“重设额度”')
                                : renderQuotaWithPrompt(values.remain_quota || 0)
                            }
                            rules={
                              values.unlimited_quota ||
                              (isEdit && !values.change_quota && !mustResetQuota)
                                ? []
                                : [{ required: true, message: t('请输入额度') }]
                            }
                            data={[
                              { value: 500000, label: '1$' },
                              { value: 5000000, label: '10$' },
                              { value: 25000000, label: '50$' },
                              { value: 50000000, label: '100$' },
                              { value: 250000000, label: '500$' },
                              { value: 500000000, label: '1000$' },
                            ]}
                          />
                        </Col>
                        <Col span={24}>
                          <Form.Switch
                            field='unlimited_quota'
                            label={t('无限额度')}
                            size='large'
                            extraText={t(
                              '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                            )}
                          />
                        </Col>
                      </Row>
                      {isAdminUser && (
                        <Row gutter={12} className='mt-2'>
                          <Col span={24}>
                            <Form.Switch
                              field='daily_quota_unlimited'
                              label={t('每日额度不限')}
                              size='large'
                            />
                          </Col>
                          <Col span={24}>
                            <Form.InputNumber
                              field='daily_quota_limit'
                              label={t('每日额度')}
                              placeholder={t('请输入每日额度')}
                              disabled={values.daily_quota_unlimited}
                              step={500000}
                              extraText={renderQuotaToUSD(
                                values.daily_quota_limit || 0,
                              ).replace('≈ ', '')}
                              style={{ width: '100%' }}
                            />
                          </Col>
                        </Row>
                      )}
                    </Card>

                    {/* 访问限制 */}
                    <Card className='!rounded-2xl shadow-sm border-0'>
                      <div className='flex items-center mb-2'>
                        <Avatar
                          size='small'
                          color='purple'
                          className='mr-2 shadow-md'
                        >
                          <IconLink size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('访问限制')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('设置令牌的访问限制')}
                          </div>
                        </div>
                      </div>
                      <Row gutter={12}>
                        <Col span={24}>
                          <Form.Select
                            field='model_limits'
                            label={t('模型限制列表')}
                            placeholder={t(
                              '请选择该令牌支持的模型，留空支持所有模型',
                            )}
                            multiple
                            optionList={models}
                            extraText={t('非必要，不建议启用模型限制')}
                            filter={selectFilter}
                            autoClearSearchValue={false}
                            searchPosition='dropdown'
                            showClear
                            style={{ width: '100%' }}
                          />
                        </Col>
                        <Col span={24}>
                          <Form.TextArea
                            field='allow_ips'
                            label={t('IP白名单')}
                            placeholder={t(
                              '允许的IP，一行一个，不填写则不限制',
                            )}
                            autosize
                            rows={1}
                            extraText={t('请勿过度信任此功能，IP可能被伪造')}
                            showClear
                            style={{ width: '100%' }}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </div>
                );
              }}
            </Form>
          </Spin>
        </div>
      </SideSheet>

      <Modal
        title={t('分组选择')}
        visible={showGroupSelector}
        centered
        width={960}
        okText={t('确认')}
        cancelText={t('取消')}
        onCancel={() => setShowGroupSelector(false)}
        onOk={() => {
          const nextGroupIds = normalizeTokenGroupIds(draftAllowedGroupIds);
          setSelectedAllowedGroupIds(nextGroupIds);
          formApiRef.current?.setValue('allowed_group_ids', nextGroupIds);
          setShowGroupSelector(false);
        }}
        okButtonProps={{
          theme: 'solid',
          type: 'primary',
          className: '!rounded-lg',
          disabled:
            groupsLoading ||
            groupLabelsLoading ||
            normalizeTokenGroupIds(draftAllowedGroupIds).length === 0,
        }}
        cancelButtonProps={{
          theme: 'tertiary',
          type: 'solid',
          className: '!rounded-lg',
        }}
        bodyStyle={{
          height: 'min(640px, calc(100vh - 220px))',
          overflow: 'hidden',
        }}
      >
        <div className='flex h-full flex-col gap-4 overflow-hidden'>
          <div className='text-sm text-neutral-700 dark:text-neutral-200'>
            {t(
              '选择令牌允许使用的分组，并按顺序决定模型命中时优先固定到哪个分组。命中后，请求不会再切到其他分组。',
            )}
          </div>
          <TokenGroupPrioritySelector
            t={t}
            options={groups}
            value={draftAllowedGroupIds}
            onChange={setDraftAllowedGroupIds}
            loading={groupsLoading || groupLabelsLoading}
            getFallbackLabel={resolveGroupLabel}
            availableEmptyText={
              groups.length === 0 ? t('管理员未设置用户可选分组') : undefined
            }
            selectedEmptyText={t('从左侧添加分组')}
            className='min-h-0'
            preferSideBySide
            heightVariant='fill'
          />
        </div>
      </Modal>
    </>
  );
};

export default EditTokenModal;
