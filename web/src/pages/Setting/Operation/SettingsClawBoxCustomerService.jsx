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

const OPTION_KEY = 'general_setting.clawbox_customer_service_qrcode';

const loadDataUrl = (file) =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(new Error('读取文件失败'));
    reader.readAsDataURL(file);
  });

export default function SettingsClawBoxCustomerService(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    [OPTION_KEY]: '',
  });
  const [inputsRow, setInputsRow] = useState(inputs);
  const [qrFileList, setQrFileList] = useState([]);
  const refForm = useRef();

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: String(inputs[item.key] ?? ''),
      }),
    );

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
    const currentInputs = {
      [OPTION_KEY]: String(props.options?.[OPTION_KEY] || '').trim(),
    };
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);

    const qrcode = currentInputs[OPTION_KEY];
    if (qrcode) {
      setQrFileList([
        {
          uid: 'clawbox_customer_service_qrcode',
          name: t('已上传图片'),
          status: 'success',
          url: qrcode,
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

      if (nextFileList.length === 0) {
        setInputs((prev) => ({
          ...prev,
          [OPTION_KEY]: '',
        }));
        refForm.current?.setValue(OPTION_KEY, '');
        return;
      }

      const fileObj = nextFileList?.[0]?.fileInstance;
      if (!fileObj) return;
      try {
        const dataUrl = await loadDataUrl(fileObj);
        setInputs((prev) => ({
          ...prev,
          [OPTION_KEY]: String(dataUrl || ''),
        }));
        refForm.current?.setValue(OPTION_KEY, String(dataUrl || ''));
      } catch (e) {
        showError(e?.message || t('读取图片失败'));
      }
    })();
  };

  const clearQRCode = () => {
    setQrFileList([]);
    setInputs((prev) => ({
      ...prev,
      [OPTION_KEY]: '',
    }));
    refForm.current?.setValue(OPTION_KEY, '');
  };

  const qrcode = String(inputs[OPTION_KEY] || '').trim();

  return (
    <Spin spinning={loading}>
      <Form values={inputs} getFormApi={(formAPI) => (refForm.current = formAPI)}>
        <Form.Section text={t('ClawBox 客服设置')}>
          <Row>
            <Col span={24}>
              <Banner
                type='info'
                description={t(
                  '这里上传的图片会展示在 ClawBox 桌面端的“联系客服”页面，保存后客户端会直接读取后台配置。',
                )}
              />
            </Col>
          </Row>

          <Row style={{ marginTop: 10 }}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Upload
                field='clawbox_customer_service_qrcode_file'
                label={t('客服二维码')}
                accept='image/*'
                draggable
                uploadTrigger='custom'
                beforeUpload={() => false}
                onChange={handleQRCodeUploadChange}
                fileList={qrFileList}
                extraText={t('支持 PNG/JPG/WebP；建议上传正方形二维码图片')}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <div style={{ marginTop: 30 }}>
                <Text strong>{t('预览')}</Text>
                <div style={{ marginTop: 8 }}>
                  {qrcode ? (
                    <img
                      src={qrcode}
                      alt='ClawBox customer service qrcode'
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
