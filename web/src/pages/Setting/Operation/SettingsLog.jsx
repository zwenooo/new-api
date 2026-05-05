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
import { Button, Col, Form, Row, Spin, Typography, DatePicker } from '@douyinfe/semi-ui';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
} from '../../../helpers';

export default function SettingsLog(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [loadingCleanHistoryLog, setLoadingCleanHistoryLog] = useState(false);
  const [inputs, setInputs] = useState({
    LogConsumeEnabled: false,
    'request_trace.enabled': false,
    'request_trace.retention_minutes': 0,
    historyTimestamp: dayjs().subtract(1, 'month').toDate(),
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow).filter(
      (item) => item.key !== 'historyTimestamp',
    );

    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
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
  async function onCleanHistoryLog() {
    try {
      setLoadingCleanHistoryLog(true);
      if (!inputs.historyTimestamp) throw new Error(t('请选择日志记录时间'));
      const res = await API.delete(
        `/api/log/?target_timestamp=${Date.parse(inputs.historyTimestamp) / 1000}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(`${data} ${t('条日志已清理！')}`);
        return;
      } else {
        throw new Error(t('日志清理失败：') + message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoadingCleanHistoryLog(false);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        if (typeof inputs[key] === 'boolean') {
          currentInputs[key] = toBoolean(props.options[key]);
        } else if (typeof inputs[key] === 'number') {
          const parsed = parseInt(props.options[key], 10);
          currentInputs[key] = Number.isFinite(parsed) ? parsed : 0;
        } else {
          currentInputs[key] = props.options[key];
        }
      }
    }
    setInputs((prev) => {
      const merged = { ...prev, ...currentInputs };
      // Keep the local-only historyTimestamp.
      merged.historyTimestamp = prev.historyTimestamp;
      setInputsRow(structuredClone(merged));
      refForm.current?.setValues(merged);
      return merged;
    });
  }, [props.options]);
  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('日志设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'LogConsumeEnabled'}
                  label={t('启用额度消费日志记录')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      LogConsumeEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'request_trace.enabled'}
                  label={t('启用完整请求链路持久化（request_id 点查）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      'request_trace.enabled': value,
                    });
                  }}
                />
                <Typography.Text type='tertiary'>
                  {t(
                    '开启后会持久化请求/响应 Headers 与 Body（可能包含敏感信息），用于按 request_id 点查排障；请确保日志目录有持久化卷',
                  )}
                </Typography.Text>
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'request_trace.retention_minutes'}
                  label={t('请求链路保留（分钟，0=永久）')}
                  initValue={0}
                  min={0}
                  max={5256000}
                  precision={0}
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      'request_trace.retention_minutes': value,
                    });
                  }}
                />
                <Typography.Text type='tertiary'>
                  {t('仅影响 request_trace：后台定时清理超过 N 分钟的索引与落盘对象（例如 60=1小时，1440=1天）。')}
                </Typography.Text>
                <Typography.Text type='tertiary'>
                  {t('磁盘保护：系统会保证 request_trace 落盘目录所在磁盘至少保留 10GiB 空闲空间；低于阈值会从最旧 request_trace 开始自动清理（即使保留分钟为 0）。')}
                </Typography.Text>
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Spin spinning={loadingCleanHistoryLog}>
                  <Form.DatePicker
                    label={t('日志记录时间')}
                    field={'historyTimestamp'}
                    type='dateTime'
                    inputReadOnly={true}
                    onChange={(value) => {
                      setInputs({
                        ...inputs,
                        historyTimestamp: value,
                      });
                    }}
                  />
                  <Button size='default' onClick={onCleanHistoryLog}>
                    {t('清除历史日志')}
                  </Button>
                </Spin>
              </Col>
            </Row>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存日志设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
