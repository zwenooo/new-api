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
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsMonitoring(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ChannelDisableThreshold: '',
    QuotaRemindThreshold: '',
    AutomaticDisableChannelEnabled: false,
    AutomaticEnableChannelEnabled: false,
    AutomaticDisableKeywords: '',
    RetryTimes: 0,
    AutomaticSwitchStatusCodeWhitelist: '',
    AutomaticSwitchMaxRetries: 5,
    'monitor_setting.auto_test_channel_enabled': false,
    'monitor_setting.auto_test_channel_minutes': 10,
    'monitor_setting.service_status_default_range_days': 30,
    'monitor_setting.service_status_default_range_minutes': 180,
    'monitor_setting.service_status_default_bucket': 'minute',
    'monitor_setting.service_status_ua_filter_mode': 'include',
    'monitor_setting.service_status_ua_contains': '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const uaFilterMode = String(
    inputs?.['monitor_setting.service_status_ua_filter_mode'] || 'include',
  );
  const uaFilterModeIsExclude = uaFilterMode === 'exclude';

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
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
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
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
          <Form.Section text={t('监控设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'monitor_setting.auto_test_channel_enabled'}
                  label={t('定时测试所有通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_enabled': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('自动测试所有通道间隔时间')}
                  step={1}
                  min={1}
                  suffix={t('分钟')}
                  extraText={t('每隔多少分钟测试一次所有通道')}
                  placeholder={''}
                  field={'monitor_setting.auto_test_channel_minutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_minutes': parseInt(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Select
                  label={t('服务状态默认粒度')}
                  field={'monitor_setting.service_status_default_bucket'}
                  optionList={[
                    { label: t('分钟'), value: 'minute' },
                    { label: t('小时'), value: 'hour' },
                    { label: t('天'), value: 'day' },
                  ]}
                  extraText={t('打开服务状态页面默认展示的时间粒度')}
                  placeholder={''}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.service_status_default_bucket': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('服务状态默认查看分钟数')}
                  step={1}
                  min={1}
                  max={259200}
                  suffix={t('分钟')}
                  extraText={t(
                    '打开服务状态页面默认展示近多少分钟的数据（默认粒度为分钟时建议 ≤ 4000）',
                  )}
                  placeholder={''}
                  field={'monitor_setting.service_status_default_range_minutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.service_status_default_range_minutes':
                        parseInt(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('服务状态默认查看天数')}
                  step={1}
                  min={1}
                  max={180}
                  suffix={t('天')}
                  extraText={t(
                    '（旧配置）当默认查看分钟数未配置时使用；建议改用「默认查看分钟数」',
                  )}
                  placeholder={''}
                  field={'monitor_setting.service_status_default_range_days'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.service_status_default_range_days':
                        parseInt(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={24} md={16} lg={16} xl={16}>
                <Form.Select
                  label={t('服务状态UA过滤模式')}
                  field={'monitor_setting.service_status_ua_filter_mode'}
                  optionList={[
                    { label: t('白名单（仅统计包含）'), value: 'include' },
                    { label: t('黑名单（不统计包含）'), value: 'exclude' },
                  ]}
                  extraText={t(
                    '白名单：只统计UA命中关键字的请求；黑名单：排除UA命中关键字的请求',
                  )}
                  placeholder={''}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.service_status_ua_filter_mode': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={24} md={16} lg={16} xl={16}>
                <Form.TextArea
                  label={
                    uaFilterModeIsExclude
                      ? t('服务状态统计UA排除')
                      : t('服务状态统计UA包含')
                  }
                  placeholder={t('一行一个')}
                  extraText={
                    uaFilterModeIsExclude
                      ? t(
                          '不统计 UA 包含以下任意关键字的请求，一行一个（不区分大小写）；留空表示不过滤（统计所有 UA）',
                        )
                      : t(
                          '仅统计 UA 包含以下任意关键字的请求，一行一个（不区分大小写）',
                        )
                  }
                  field={'monitor_setting.service_status_ua_contains'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.service_status_ua_contains': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('测试所有渠道的最长响应时间')}
                  step={1}
                  min={0}
                  suffix={t('秒')}
                  extraText={t(
                    '当运行通道全部测试时，超过此时间将自动禁用通道',
                  )}
                  placeholder={''}
                  field={'ChannelDisableThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelDisableThreshold: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('额度提醒阈值')}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={t('低于此额度时将发送邮件提醒用户')}
                  placeholder={''}
                  field={'QuotaRemindThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaRemindThreshold: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticDisableChannelEnabled'}
                  label={t('失败时自动禁用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      AutomaticDisableChannelEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticEnableChannelEnabled'}
                  label={t('成功时自动启用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      AutomaticEnableChannelEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('自动禁用关键词')}
                  placeholder={t('一行一个，不区分大小写')}
                  extraText={t(
                    '当上游通道返回错误中包含这些关键词时（不区分大小写），自动禁用通道',
                  )}
                  field={'AutomaticDisableKeywords'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableKeywords: value })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('自动切换最大重试次数')}
                  step={1}
                  min={0}
                  extraText={t(
                    '请求失败且允许重试时，若存在其它可用渠道，优先按此处设置的次数切换渠道重试',
                  )}
                  placeholder={''}
                  field={'AutomaticSwitchMaxRetries'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      AutomaticSwitchMaxRetries: parseInt(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('单渠道重试次数')}
                  step={1}
                  min={0}
                  extraText={t(
                    '没有其它可用渠道，或渠道切换重试次数耗尽后，在当前渠道上最多继续重试这么多次',
                  )}
                  placeholder={''}
                  field={'RetryTimes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      RetryTimes: parseInt(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('自动切换状态码白名单')}
                  placeholder={t('一行一个，例如 200')}
                  extraText={t(
                    '仅当上游返回状态码命中白名单时才直接向用户返回；否则自动切换到其它可用通道继续请求（不会禁用当前通道）',
                  )}
                  field={'AutomaticSwitchStatusCodeWhitelist'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      AutomaticSwitchStatusCodeWhitelist: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存监控设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
