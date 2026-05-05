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

import React, { useEffect, useState, useRef } from 'react';
import { Banner, Button, Col, Form, Row, Spin, Modal } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const defaultInputs = {
  TopUpLink: '',
  'general_setting.docs_link': '',
  QuotaPerUnit: '',
  USDExchangeRate: '',
  SubscriptionInviteCommissionFirstPercent: 0,
  SubscriptionInviteCommissionRepeatPercent: 0,
  DisplayInCurrencyEnabled: false,
  DisplayTokenStatEnabled: false,
  StompKingRankMode: 'quota',
  DefaultCollapseSidebar: false,
  DemoSiteEnabled: false,
  SelfUseModeEnabled: false,
  ChatCompletionsEnabled: false,
  'channel_allocation_setting.user_sticky_exclusive_enabled': false,
};

export default function GeneralSettings(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [showQuotaWarning, setShowQuotaWarning] = useState(false);
  const [inputs, setInputs] = useState(() => structuredClone(defaultInputs));
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(() => structuredClone(defaultInputs));

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((inputs) => ({ ...inputs, [fieldName]: value }));
    };
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    for (const item of updateArray) {
      if (
        item.key === 'SubscriptionInviteCommissionFirstPercent' ||
        item.key === 'SubscriptionInviteCommissionRepeatPercent'
      ) {
        const value = inputs[item.key];
        if (value === undefined || value === null || value === '') {
          return showError(t('分佣比例不能为空'));
        }
        if (!Number.isFinite(value)) {
          return showError(t('分佣比例必须为数字'));
        }
        if (!Number.isInteger(value)) {
          return showError(t('分佣比例必须为整数'));
        }
        if (value < 0 || value > 100) {
          return showError(t('分佣比例必须在 0-100 之间'));
        }
      }
    }

    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else if (typeof inputs[item.key] === 'number') {
        value = inputs[item.key].toString();
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = structuredClone(defaultInputs);
    Object.keys(defaultInputs).forEach((key) => {
      if (props.options && props.options[key] !== undefined) {
        if (
          key === 'SubscriptionInviteCommissionFirstPercent' ||
          key === 'SubscriptionInviteCommissionRepeatPercent'
        ) {
          const parsed = parseInt(props.options[key], 10);
          currentInputs[key] = Number.isFinite(parsed) ? parsed : 0;
        } else if (
          key === 'StompKingRankMode' &&
          props.options[key] === 'visible_quota'
        ) {
          currentInputs[key] = 'cost_quota';
        } else {
          currentInputs[key] = props.options[key];
        }
      }
    });
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('通用设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'TopUpLink'}
                  label={t('充值链接')}
                  initValue={''}
                  placeholder={t('例如发卡网站的购买链接')}
                  onChange={handleFieldChange('TopUpLink')}
                  showClear
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'general_setting.docs_link'}
                  label={t('文档地址')}
                  initValue={''}
                  placeholder={t('例如 https://docs.example.com')}
                  onChange={handleFieldChange('general_setting.docs_link')}
                  showClear
                />
              </Col>
              {inputs.QuotaPerUnit !== '500000' && inputs.QuotaPerUnit !== 500000 && (
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field={'QuotaPerUnit'}
                    label={t('单位美元额度')}
                    initValue={''}
                    placeholder={t('一单位货币能兑换的额度')}
                    onChange={handleFieldChange('QuotaPerUnit')}
                    showClear
                    onClick={() => setShowQuotaWarning(true)}
                  />
                </Col>
              )}
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'USDExchangeRate'}
                  label={t('美元汇率（非充值汇率，仅用于定价页面换算）')}
                  initValue={''}
                  placeholder={t('美元汇率')}
                  onChange={handleFieldChange('USDExchangeRate')}
                  showClear
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'SubscriptionInviteCommissionFirstPercent'}
                  label={t('订阅分佣比例（首次 %）')}
                  initValue={0}
                  min={0}
                  max={100}
                  precision={0}
                  onChange={handleFieldChange(
                    'SubscriptionInviteCommissionFirstPercent',
                  )}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'SubscriptionInviteCommissionRepeatPercent'}
                  label={t('订阅分佣比例（后续 %）')}
                  initValue={0}
                  min={0}
                  max={100}
                  precision={0}
                  onChange={handleFieldChange(
                    'SubscriptionInviteCommissionRepeatPercent',
                  )}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'DisplayInCurrencyEnabled'}
                  label={t('以货币形式显示额度')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('DisplayInCurrencyEnabled')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'DisplayTokenStatEnabled'}
                  label={t('额度查询接口返回令牌额度而非用户额度')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('DisplayTokenStatEnabled')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Select
                  field={'StompKingRankMode'}
                  label={t('蹬王榜单排序')}
                  optionList={[
                    { label: t('按标准费用'), value: 'cost_quota' },
                    { label: t('按实际费用'), value: 'quota' },
                    { label: t('按成功请求次数'), value: 'success_count' },
                  ]}
                  style={{ width: 240 }}
                  onChange={handleFieldChange('StompKingRankMode')}
                  extraText={t('用于控制 /console/stomp_king 榜单展示方式')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'DefaultCollapseSidebar'}
                  label={t('默认折叠侧边栏')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('DefaultCollapseSidebar')}
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'DemoSiteEnabled'}
                  label={t('演示站点模式')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('DemoSiteEnabled')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'SelfUseModeEnabled'}
                  label={t('自用模式')}
                  extraText={t('开启后不限制：必须设置模型倍率')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('SelfUseModeEnabled')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ChatCompletionsEnabled'}
                  label={t('启用 /v1/chat/completions 接口')}
                  extraText={t('关闭后该端点返回 403：chat_completions_disabled')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange('ChatCompletionsEnabled')}
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'channel_allocation_setting.user_sticky_exclusive_enabled'}
                  label={t('同用户请求固定渠道（尽量独占）')}
                  extraText={t(
                    '开启后：尽量将同一用户的请求分配到同一渠道；渠道充足时尽量做到用户独占渠道',
                  )}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={handleFieldChange(
                    'channel_allocation_setting.user_sticky_exclusive_enabled',
                  )}
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存通用设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>

      <Modal
        title={t('警告')}
        visible={showQuotaWarning}
        onOk={() => setShowQuotaWarning(false)}
        onCancel={() => setShowQuotaWarning(false)}
        closeOnEsc={true}
        width={500}
      >
        <Banner
          type='warning'
          description={t(
            '此设置用于系统内部计算，默认值500000是为了精确到6位小数点设计，不推荐修改。',
          )}
          bordered
          fullMode={false}
          closeIcon={null}
        />
      </Modal>
    </>
  );
}
