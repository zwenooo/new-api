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
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const defaultInputs = {
  ModelRequestRateLimitEnabled: false,
  ModelRequestRateLimitCount: 0,
  ModelRequestRateLimitSuccessCount: 1000,
  ModelRequestRateLimitDurationMinutes: 1,
  ModelRequestRateLimitGroup: '{}',
};

export default function RequestRateLimit(props) {
  const { t } = useTranslation();
  const hideGroupOverrides = Boolean(props.hideGroupOverrides);

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(() => structuredClone(defaultInputs));
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(() => structuredClone(defaultInputs));

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    for (const item of updateArray) {
      if (
        item.key === 'ModelRequestRateLimitDurationMinutes' ||
        item.key === 'ModelRequestRateLimitCount' ||
        item.key === 'ModelRequestRateLimitSuccessCount'
      ) {
        const value = inputs[item.key];
        if (!Number.isFinite(value) || !Number.isInteger(value)) {
          return showError(t('请输入整数'));
        }
        if (item.key === 'ModelRequestRateLimitDurationMinutes' && value < 1) {
          return showError(t('限制周期必须大于等于 1'));
        }
        if (item.key === 'ModelRequestRateLimitCount' && value < 0) {
          return showError(t('用户每周期最多请求次数必须大于等于 0'));
        }
        if (item.key === 'ModelRequestRateLimitSuccessCount' && value < 0) {
          return showError(t('用户每周期最多请求完成次数必须大于等于 0'));
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

        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
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
        if (key === 'ModelRequestRateLimitEnabled') {
          currentInputs[key] = toBoolean(props.options[key]);
          return;
        }
        if (
          key === 'ModelRequestRateLimitDurationMinutes' ||
          key === 'ModelRequestRateLimitCount' ||
          key === 'ModelRequestRateLimitSuccessCount'
        ) {
          const parsed = parseInt(props.options[key], 10);
          currentInputs[key] = Number.isFinite(parsed) ? parsed : defaultInputs[key];
          return;
        }
        currentInputs[key] = props.options[key];
      }
    });
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    if (refForm.current) {
      refForm.current.setValues(currentInputs);
    }
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('模型请求速率限制')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ModelRequestRateLimitEnabled'}
                  label={t('启用用户模型请求速率限制（可能会影响高并发性能）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs((prev) => ({
                      ...prev,
                      ModelRequestRateLimitEnabled: value,
                    }));
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('限制周期')}
                  step={1}
                  min={1}
                  suffix={t('分钟')}
                  extraText={t('频率限制的周期（分钟）')}
                  field={'ModelRequestRateLimitDurationMinutes'}
                  onChange={(value) =>
                    setInputs((prev) => ({
                      ...prev,
                      ModelRequestRateLimitDurationMinutes: value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求次数')}
                  step={1}
                  min={0}
                  max={2147483647}
                  suffix={t('次')}
                  extraText={t('包括失败请求的次数，0代表不限制')}
                  field={'ModelRequestRateLimitCount'}
                  onChange={(value) =>
                    setInputs((prev) => ({
                      ...prev,
                      ModelRequestRateLimitCount: value,
                    }))
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求完成次数')}
                  step={1}
                  min={0}
                  max={2147483647}
                  suffix={t('次')}
                  extraText={t('只包括请求成功的次数，0代表不限制')}
                  field={'ModelRequestRateLimitSuccessCount'}
                  onChange={(value) =>
                    setInputs((prev) => ({
                      ...prev,
                      ModelRequestRateLimitSuccessCount: value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row>
              {!hideGroupOverrides && (
                <Col xs={24} sm={16}>
                  <Form.TextArea
                    label={t('分组速率限制')}
                    placeholder={t(
                      '{\n  "4": [200, 100],\n  "8": [0, 1000]\n}',
                    )}
                    field={'ModelRequestRateLimitGroup'}
                    autosize={{ minRows: 5, maxRows: 15 }}
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
                          if (
                            !parsed ||
                            typeof parsed !== 'object' ||
                            Array.isArray(parsed)
                          ) {
                            return false;
                          }
                          return Object.entries(parsed).every(
                            ([rawKey, rawValue]) => {
                              const key = String(rawKey || '').trim();
                              if (!/^[1-9]\d*$/.test(key)) return false;
                              if (!Array.isArray(rawValue) || rawValue.length !== 2)
                                return false;
                              const [total, success] = rawValue;
                              if (
                                !Number.isFinite(total) ||
                                !Number.isInteger(total) ||
                                total < 0
                              )
                                return false;
                              if (
                                !Number.isFinite(success) ||
                                !Number.isInteger(success) ||
                                success < 0
                              )
                                return false;
                              return true;
                            },
                          );
                        },
                        message: t(
                          '必须为 {"group_id": [total, success]} 的 JSON 对象，例如：{"4":[200,100]}',
                        ),
                      },
                    ]}
                    extraText={
                      <div>
                        <p>{t('说明：')}</p>
                        <ul>
                          <li>
                            {t(
                              '使用 JSON 对象格式，格式为：{"group_id": [最多请求次数, 最多请求完成次数]}',
                            )}
                          </li>
                          <li>
                            {t('示例：{"4": [200, 100], "8": [0, 1000]}。')}
                          </li>
                          <li>
                            {t(
                              '[最多请求次数]必须大于等于0，[最多请求完成次数]必须大于等于0。',
                            )}
                          </li>
                          <li>{t('group_id 必须为正整数。')}</li>
                          <li>
                            {t(
                              '[最多请求次数]和[最多请求完成次数]的最大值为2147483647。',
                            )}
                          </li>
                          <li>{t('分组速率配置优先级高于全局速率限制。')}</li>
                          <li>{t('限制周期统一使用上方配置的「限制周期」值。')}</li>
                        </ul>
                      </div>
                    }
                    onChange={(value) => {
                      setInputs((prev) => ({
                        ...prev,
                        ModelRequestRateLimitGroup: value,
                      }));
                    }}
                  />
                </Col>
              )}
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存模型速率限制')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
