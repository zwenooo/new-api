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
import { Button, Checkbox, Form, Row, Col, Typography, Spin } from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGateway(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const payMethodTemplates = [
    {
      name: t('支付宝'),
      color: 'rgba(var(--semi-blue-5), 1)',
      type: 'alipay',
    },
    {
      name: t('微信'),
      color: 'rgba(var(--semi-green-5), 1)',
      type: 'wxpay',
    },
  ];
  const [inputs, setInputs] = useState({
    PayAddress: '',
    EpayId: '',
    EpayKey: '',
    Price: 7.3,
    MinTopUp: 1,
    TopupGroupRatio: '',
    CustomCallbackAddress: '',
    PayMethods: '',
    AmountOptions: '',
    AmountDiscount: '',
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  const parsePayMethods = (raw) => {
    const trimmed = String(raw || '').trim();
    if (trimmed === '') {
      return { methods: [], error: '' };
    }
    try {
      const parsed = JSON.parse(trimmed);
      if (!Array.isArray(parsed)) {
        return { methods: [], error: t('充值方式设置必须为 JSON 数组') };
      }
      const methods = parsed
        .filter((m) => m && typeof m === 'object')
        .map((m) => ({
          ...m,
          type: String(m.type || '').trim(),
          name: String(m.name || '').trim(),
        }))
        .filter((m) => m.type);
      return { methods, error: '' };
    } catch (e) {
      return { methods: [], error: t('充值方式设置不是合法的 JSON') };
    }
  };

  const buildPayMethodsJSON = (selectedTypes) => {
    const { methods: currentMethods, error } = parsePayMethods(inputs.PayMethods);
    if (error) {
      showError(error);
      return;
    }
    const selectedSet = new Set(selectedTypes || []);
    const byType = new Map(currentMethods.map((m) => [m.type, m]));
    const builtinTypes = new Set(payMethodTemplates.map((m) => m.type));

    const out = [];
    payMethodTemplates.forEach((tpl) => {
      if (!selectedSet.has(tpl.type)) return;
      const existing = byType.get(tpl.type);
      if (existing) {
        out.push({
          ...tpl,
          ...existing,
          type: tpl.type,
          name: existing.name || tpl.name,
          color: existing.color || tpl.color,
        });
      } else {
        out.push({ ...tpl });
      }
    });
    currentMethods.forEach((m) => {
      if (builtinTypes.has(m.type)) return;
      if (!selectedSet.has(m.type)) return;
      out.push(m);
    });

    const next = JSON.stringify(out, null, 2);
    formApiRef.current?.setValue('PayMethods', next);
    setInputs((prev) => ({ ...prev, PayMethods: next }));
  };

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        PayAddress: props.options.PayAddress || '',
        EpayId: props.options.EpayId || '',
        EpayKey: props.options.EpayKey || '',
        Price:
          props.options.Price !== undefined
            ? parseFloat(props.options.Price)
            : 7.3,
        MinTopUp:
          props.options.MinTopUp !== undefined
            ? parseFloat(props.options.MinTopUp)
            : 1,
        TopupGroupRatio: props.options.TopupGroupRatio || '',
        CustomCallbackAddress: props.options.CustomCallbackAddress || '',
        PayMethods: props.options.PayMethods || '',
        AmountOptions: props.options.AmountOptions || '',
        AmountDiscount: props.options.AmountDiscount || '',
      };

      // 美化 JSON 展示
      try {
        if (currentInputs.PayMethods) {
          currentInputs.PayMethods = JSON.stringify(
            JSON.parse(currentInputs.PayMethods),
            null,
            2,
          );
        }
      } catch {}
      try {
        if (currentInputs.AmountOptions) {
          currentInputs.AmountOptions = JSON.stringify(
            JSON.parse(currentInputs.AmountOptions),
            null,
            2,
          );
        }
      } catch {}
      try {
        if (currentInputs.AmountDiscount) {
          currentInputs.AmountDiscount = JSON.stringify(
            JSON.parse(currentInputs.AmountDiscount),
            null,
            2,
          );
        }
      } catch {}

      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const { methods: parsedPayMethods, error: payMethodsError } = parsePayMethods(
    inputs.PayMethods,
  );
  const payMethodToggleOptions = [
    ...payMethodTemplates.map((m) => ({
      type: m.type,
      label: m.name || m.type,
    })),
    ...parsedPayMethods
      .filter((m) => !payMethodTemplates.some((tpl) => tpl.type === m.type))
      .map((m) => ({
        type: m.type,
        label: m.name || m.type,
      })),
  ];
  const payMethodEnabledTypes = parsedPayMethods.map((m) => m.type);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitPayAddress = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    const callbackAddress = removeTrailingSlash(inputs.CustomCallbackAddress);
    if (callbackAddress.endsWith('/v1')) {
      showError(
        t('回调地址不要包含 /v1，请将 /v1 配置在「基址」中，例如：https://yourdomain.com/v1'),
      );
      return;
    }

    if (originInputs['TopupGroupRatio'] !== inputs.TopupGroupRatio) {
      if (!verifyJSON(inputs.TopupGroupRatio)) {
        showError(t('充值分组倍率不是合法的 JSON 字符串'));
        return;
      }
      try {
        const parsed = JSON.parse(inputs.TopupGroupRatio);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          showError(t('充值分组倍率必须为 {"group_id": ratio} 的 JSON 对象'));
          return;
        }
        const ok = Object.entries(parsed).every(([rawKey, ratio]) => {
          const key = String(rawKey || '').trim();
          if (!/^[1-9]\d*$/.test(key)) return false;
          return (
            typeof ratio === 'number' &&
            Number.isFinite(ratio) &&
            ratio >= 0
          );
        });
        if (!ok) {
          showError(t('充值分组倍率必须为 {"group_id": ratio} 的 JSON 对象'));
          return;
        }
      } catch (error) {
        showError(t('充值分组倍率必须为 {"group_id": ratio} 的 JSON 对象'));
        return;
      }
    }

    if (originInputs['PayMethods'] !== inputs.PayMethods) {
      if (!verifyJSON(inputs.PayMethods)) {
        showError(t('充值方式设置不是合法的 JSON 字符串'));
        return;
      }
    }

    if (originInputs['AmountOptions'] !== inputs.AmountOptions && inputs.AmountOptions.trim() !== '') {
      if (!verifyJSON(inputs.AmountOptions)) {
        showError(t('自定义充值数量选项不是合法的 JSON 数组'));
        return;
      }
    }

    if (originInputs['AmountDiscount'] !== inputs.AmountDiscount && inputs.AmountDiscount.trim() !== '') {
      if (!verifyJSON(inputs.AmountDiscount)) {
        showError(t('充值金额折扣配置不是合法的 JSON 对象'));
        return;
      }
    }

    setLoading(true);
    try {
      const options = [
        { key: 'PayAddress', value: removeTrailingSlash(inputs.PayAddress) },
      ];

      if (inputs.EpayId !== '') {
        options.push({ key: 'EpayId', value: inputs.EpayId });
      }
      if (inputs.EpayKey !== undefined && inputs.EpayKey !== '') {
        options.push({ key: 'EpayKey', value: inputs.EpayKey });
      }
      if (inputs.Price !== '') {
        options.push({ key: 'Price', value: inputs.Price.toString() });
      }
      if (inputs.MinTopUp !== '') {
        options.push({ key: 'MinTopUp', value: inputs.MinTopUp.toString() });
      }
      if (inputs.CustomCallbackAddress !== '') {
        options.push({
          key: 'CustomCallbackAddress',
          value: callbackAddress,
        });
      }
      if (originInputs['TopupGroupRatio'] !== inputs.TopupGroupRatio) {
        options.push({ key: 'TopupGroupRatio', value: inputs.TopupGroupRatio });
      }
      if (originInputs['PayMethods'] !== inputs.PayMethods) {
        options.push({ key: 'PayMethods', value: inputs.PayMethods });
      }
      if (originInputs['AmountOptions'] !== inputs.AmountOptions) {
        options.push({ key: 'payment_setting.amount_options', value: inputs.AmountOptions });
      }
      if (originInputs['AmountDiscount'] !== inputs.AmountDiscount) {
        options.push({ key: 'payment_setting.amount_discount', value: inputs.AmountDiscount });
      }

      // 发送请求
      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
        setOriginInputs({ ...inputs });
        props.refresh && props.refresh();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('支付设置')}>
          <Text>
            {t(
              '（当前仅支持易支付接口，默认使用上方服务器地址作为回调地址！）',
            )}
          </Text>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='PayAddress'
                label={t('支付地址')}
                placeholder={t('例如：https://yourdomain.com')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='EpayId'
                label={t('易支付商户ID')}
                placeholder={t('例如：0001')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='EpayKey'
                label={t('易支付商户密钥')}
                placeholder={t('敏感信息不会发送到前端显示')}
                type='password'
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='CustomCallbackAddress'
                label={t('回调地址')}
                placeholder={t('例如：https://yourdomain.com')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='Price'
                precision={2}
                label={t('充值价格（x元/美金）')}
                placeholder={t('例如：7，就是7元/美金')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='MinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，就是最低充值2$')}
              />
            </Col>
          </Row>
          <Form.TextArea
            field='TopupGroupRatio'
            label={t('充值分组倍率')}
            placeholder={t('为一个 JSON 文本，键为 group_id，值为倍率，例如：{"4": 1, "8": 0.6}')}
            autosize
            trigger='blur'
            stopValidateWithError
            rules={[
              {
                validator: (rule, value) => verifyJSON(value),
                message: t('不是合法的 JSON 字符串'),
              },
              {
                validator: (rule, value) => {
                  if (!value || value.trim() === '') return true;
                  let parsed;
                  try {
                    parsed = JSON.parse(value);
                  } catch (error) {
                    return false;
                  }
                  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
                    return false;
                  }
                  return Object.entries(parsed).every(([rawKey, ratio]) => {
                    const key = String(rawKey || '').trim();
                    if (!/^[1-9]\d*$/.test(key)) return false;
                    return typeof ratio === 'number' && Number.isFinite(ratio) && ratio >= 0;
                  });
                },
                message: t('必须为 {"group_id": ratio} 的 JSON 对象，例如：{"4": 1}'),
              },
            ]}
          />
          <Form.Slot label={t('启用充值方式')}>
            <div className='flex flex-wrap gap-3'>
              {payMethodToggleOptions.map((opt) => (
                <Checkbox
                  key={opt.type}
                  checked={payMethodEnabledTypes.includes(opt.type)}
                  disabled={Boolean(payMethodsError)}
                  onChange={(e) => {
                    const next = e.target.checked
                      ? Array.from(new Set([...payMethodEnabledTypes, opt.type]))
                      : payMethodEnabledTypes.filter((t0) => t0 !== opt.type);
                    buildPayMethodsJSON(next);
                  }}
                >
                  {opt.label}
                </Checkbox>
              ))}
            </div>
            {payMethodsError ? (
              <div className='mt-2 text-xs text-red-500'>{payMethodsError}</div>
            ) : (
              <div className='mt-2 text-xs text-gray-500'>
                {t('关闭后，对应支付方式在用户侧将不可选')}
              </div>
            )}
          </Form.Slot>
          <Form.TextArea
            field='PayMethods'
            label={t('充值方式设置')}
            placeholder={t('为一个 JSON 文本')}
            autosize
          />
          
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col span={24}>
              <Form.TextArea
                field='AmountOptions'
                label={t('自定义充值数量选项')}
                placeholder={t('为一个 JSON 数组，例如：[10, 20, 50, 100, 200, 500]')}
                autosize
                extraText={t('设置用户可选择的充值数量选项，例如：[10, 20, 50, 100, 200, 500]')}
              />
            </Col>
          </Row>
          
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col span={24}>
              <Form.TextArea
                field='AmountDiscount'
                label={t('充值金额折扣配置')}
                placeholder={t('为一个 JSON 对象，例如：{"100": 0.95, "200": 0.9, "500": 0.85}')}
                autosize
                extraText={t('设置不同充值金额对应的折扣，键为充值金额，值为折扣率，例如：{"100": 0.95, "200": 0.9, "500": 0.85}')}
              />
            </Col>
          </Row>
          
          <Button onClick={submitPayAddress}>{t('更新支付设置')}</Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
