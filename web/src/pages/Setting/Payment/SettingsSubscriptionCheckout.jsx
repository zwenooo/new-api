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

import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Row,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

const { Text } = Typography;

const loadDataUrl = (file) =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(new Error('读取文件失败'));
    reader.readAsDataURL(file);
  });

export default function SettingsSubscriptionCheckout(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    'subscription.checkout_mode': 'payment',
    'subscription.traffic_message': '',
    'subscription.traffic_qrcode': '',
    'subscription.store_notice': '',
  });
  const [inputsRow, setInputsRow] = useState(inputs);
  const [qrFileList, setQrFileList] = useState([]);
  const refForm = useRef();

  function onSubmit() {
    const mode = String(inputs['subscription.checkout_mode'] || '').trim();
    if (mode !== 'payment' && mode !== 'traffic') {
      return showError(t('订阅购买模式无效'));
    }
    if (mode === 'traffic') {
      const message = String(inputs['subscription.traffic_message'] || '').trim();
      const qrcode = String(inputs['subscription.traffic_qrcode'] || '').trim();
      if (message === '' && qrcode === '') {
        return showError(t('引流模式需要配置描述或图片'));
      }
    }

    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      const value = String(inputs[item.key] ?? '');
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });

    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        const failedResponse = res.find(
          (item) => !item || item?.data?.success !== true,
        );
        if (failedResponse) {
          return showError(
            failedResponse?.data?.message || t('保存失败，请重试'),
          );
        }
        showSuccess(t('保存成功'));
        props.refresh && props.refresh();
      })
      .catch((error) => {
        showError(error?.response?.data?.message || t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = { ...inputs };
    for (let key in props.options) {
      if (Object.keys(currentInputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);

    const trafficQRCode = String(currentInputs['subscription.traffic_qrcode'] || '').trim();
    if (trafficQRCode) {
      setQrFileList([
        {
          uid: 'subscription_traffic_image',
          name: t('已上传图片'),
          status: 'success',
          url: trafficQRCode,
        },
      ]);
    } else {
      setQrFileList([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.options]);

  const handleQRCodeUploadChange = ({ fileList }) => {
    (async () => {
      let nextFileList = Array.isArray(fileList) ? fileList : [];
      if (nextFileList.length > 1) {
        nextFileList = [nextFileList[nextFileList.length - 1]];
      }
      setQrFileList(nextFileList);

      const fileObj = nextFileList?.[0]?.fileInstance;
      if (!fileObj) return;
      try {
        const dataUrl = await loadDataUrl(fileObj);
        setInputs((prev) => ({
          ...prev,
          'subscription.traffic_qrcode': String(dataUrl || ''),
        }));
        refForm.current?.setValue(
          'subscription.traffic_qrcode',
          String(dataUrl || ''),
        );
      } catch (e) {
        showError(e?.message || t('读取图片失败'));
      }
    })();
  };

  const clearQRCode = () => {
    setQrFileList([]);
    setInputs((prev) => ({
      ...prev,
      'subscription.traffic_qrcode': '',
    }));
    refForm.current?.setValue('subscription.traffic_qrcode', '');
  };

  const mode = String(inputs['subscription.checkout_mode'] || 'payment').trim();
  const message = String(inputs['subscription.traffic_message'] || '').trim();
  const qrcode = String(inputs['subscription.traffic_qrcode'] || '').trim();

  return (
    <Spin spinning={loading}>
      <Form values={inputs} getFormApi={(formAPI) => (refForm.current = formAPI)}>
        <Form.Section text={t('订阅支付/引流设置')}>
          <Row>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Select
                label={t('订阅购买模式')}
                field={'subscription.checkout_mode'}
                optionList={[
                  { label: t('支付（余额/易支付）'), value: 'payment' },
                  { label: t('引流（展示图片/描述）'), value: 'traffic' },
                ]}
                style={{ width: 240 }}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'subscription.checkout_mode': String(value),
                  })
                }
                extraText={t('用于控制用户购买订阅时的支付流程')}
              />
            </Col>
          </Row>

          {mode === 'traffic' && (
            <>
              <Row style={{ marginTop: 10 }}>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.TextArea
                    field='subscription.traffic_message'
                    label={t('描述')}
                    autosize
                    placeholder={t('可选：展示给用户看的描述（支持换行，可直接写链接文本）')}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'subscription.traffic_message': String(value),
                      })
                    }
                  />
                </Col>
              </Row>

              <Row style={{ marginTop: 10 }}>
                <Col span={24}>
                  <Banner
                    type='info'
                    description={t(
                      '引流模式仅用于展示描述/图片，不走余额/易支付支付流程',
                    )}
                  />
                </Col>
              </Row>

              <Row style={{ marginTop: 10 }}>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Upload
                    field='subscription_traffic_qrcode_file'
                    label={t('展示图片')}
                    accept='image/*'
                    draggable
                    uploadTrigger='custom'
                    beforeUpload={() => false}
                    onChange={handleQRCodeUploadChange}
                    fileList={qrFileList}
                    extraText={t('可选：上传一张用于展示的图片（PNG/JPG 均可）')}
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <div style={{ marginTop: 30 }}>
                    <Text strong>{t('预览')}</Text>
                    <div style={{ marginTop: 8 }}>
                      {message ? (
                        <div style={{ whiteSpace: 'pre-wrap' }}>{message}</div>
                      ) : null}
                      {qrcode ? (
                        <img
                          src={qrcode}
                          alt='qrcode'
                          style={{
                            maxWidth: 220,
                            maxHeight: 220,
                            borderRadius: 8,
                          }}
                        />
                      ) : (
                        <Text type='tertiary'>{t('尚未上传图片')}</Text>
                      )}
                    </div>
                    <div style={{ marginTop: 8 }}>
                      <Button
                        size='small'
                        type='danger'
                        theme='solid'
                        onClick={clearQRCode}
                      >
                        {t('清除图片')}
                      </Button>
                    </div>
                  </div>
                </Col>
              </Row>
            </>
          )}

          <Row style={{ marginTop: 16 }}>
            <Col xs={24} sm={12} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='subscription.store_notice'
                label={t('订阅商城提示文案')}
                autosize
                placeholder={t('可选：显示在订阅商城顶部的提示文本（支持换行）')}
                extraText={
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <span>
                      {t('可使用占位符插入模型广场链接：')}{' '}
                      <Text code>{'{{pricing}}'}</Text>
                    </span>
                    <span>
                      {t(
                        '支持 HTML 标签，如 <a>、<br>（将过滤脚本等不安全内容）',
                      )}
                    </span>
                    <span>
                      {t(
                        '支持内联样式：color、font-weight、font-size（将过滤不安全内容）',
                      )}
                    </span>
                  </div>
                }
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'subscription.store_notice': String(value),
                  })
                }
              />
            </Col>
          </Row>

          <Row style={{ marginTop: 10 }}>
            <Button size='default' onClick={onSubmit}>
              {t('保存')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
