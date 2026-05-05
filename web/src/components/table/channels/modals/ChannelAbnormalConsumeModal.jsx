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

import React, { useMemo } from 'react';
import {
  Modal,
  Button,
  Switch,
  Table,
  Typography,
  TextArea,
  Tag,
  Space,
} from '@douyinfe/semi-ui';
import { copy, showError, showSuccess, timestamp2string } from '../../../../helpers';

const ChannelAbnormalConsumeModal = ({
  showChannelAbnormalConsumeModal,
  currentAbnormalConsumeChannel,
  channelAbnormalConsumeEnabledMap,
  updateChannelAbnormalConsumeEnabled,
  channelAbnormalConsumeRecords,
  channelAbnormalConsumeLoading,
  loadChannelAbnormalConsumeRecords,
  clearChannelAbnormalConsumeRecords,
  closeChannelAbnormalConsumeModal,
  t,
}) => {
  const channelId = Number(currentAbnormalConsumeChannel?.id || 0);
  const channelName = String(currentAbnormalConsumeChannel?.name || '').trim();
  const enabled = Boolean(channelAbnormalConsumeEnabledMap?.[channelId]);

  const handleCopy = (text) => {
    const content = String(text || '').trim();
    if (!content) {
      showError(t('内容为空'));
      return;
    }
    copy(content).then((ok) => {
      if (ok) {
        showSuccess(t('复制成功'));
      } else {
        showError(t('复制失败，请手动复制'));
      }
    });
  };

  const columns = useMemo(() => {
    return [
      {
        title: t('时间'),
        dataIndex: 'time',
        render: (val) => (val ? timestamp2string(val) : '-'),
        width: 170,
      },
      {
        title: t('耗时'),
        dataIndex: 'duration_ms',
        render: (val) => {
          const ms = Number(val) || 0;
          return `${(ms / 1000).toFixed(2)}s`;
        },
        width: 90,
      },
      {
        title: t('请求IP'),
        dataIndex: 'request_ip',
        width: 130,
      },
      {
        title: t('错误'),
        dataIndex: 'status_code',
        render: (val, record) => {
          const code = String(record.error_code || '').trim();
          const type = String(record.error_type || '').trim();
          const status = Number(val) || 0;
          const parts = [`${status}`];
          if (type) parts.push(type);
          if (code) parts.push(code);
          return parts.join(' / ');
        },
        width: 240,
      },
      {
        title: t('关键词'),
        dataIndex: 'matched_keywords',
        render: (val) => {
          const list = Array.isArray(val) ? val : [];
          if (list.length === 0) return '-';
          return (
            <Space wrap>
              {list.slice(0, 3).map((k) => (
                <Tag key={k} color='yellow' type='light' size='small'>
                  {k}
                </Tag>
              ))}
              {list.length > 3 ? (
                <Tag color='grey' type='light' size='small'>
                  +{list.length - 3}
                </Tag>
              ) : null}
            </Space>
          );
        },
        width: 160,
      },
      {
        title: t('分组'),
        dataIndex: 'group',
        render: (val) => String(val || ''),
        width: 110,
      },
      {
        title: t('请求体'),
        dataIndex: 'request_body',
        render: (val) => (
          <Typography.Text
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 220 }}
          >
            {String(val || '')}
          </Typography.Text>
        ),
        width: 260,
      },
      {
        title: t('响应体'),
        dataIndex: 'response_body',
        render: (val) => (
          <Typography.Text
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 220 }}
          >
            {String(val || '')}
          </Typography.Text>
        ),
        width: 260,
      },
    ];
  }, [t]);

  return (
    <Modal
      title={
        <div className='flex flex-col gap-2 w-full'>
          <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-2'>
            <Typography.Text
              strong
              className='!text-[var(--semi-color-text-0)] !text-base'
            >
              {t('渠道异常统计')}
              {channelId ? `：#${channelId}` : ''}
              {channelName ? ` ${channelName}` : ''}
            </Typography.Text>
            <div className='flex flex-wrap items-center gap-2'>
              <Typography.Text strong>{t('是否记录')}</Typography.Text>
              <Switch
                size='small'
                checked={enabled}
                disabled={!channelId}
                onChange={(v) =>
                  updateChannelAbnormalConsumeEnabled(channelId, v)
                }
              />
              <Button
                size='small'
                type='tertiary'
                disabled={!channelId}
                loading={channelAbnormalConsumeLoading}
                onClick={() => loadChannelAbnormalConsumeRecords(channelId)}
              >
                {t('刷新')}
              </Button>
              <Button
                size='small'
                type='danger'
                disabled={!channelId}
                loading={channelAbnormalConsumeLoading}
                onClick={() => clearChannelAbnormalConsumeRecords(channelId)}
              >
                {t('清空')}
              </Button>
            </div>
          </div>
          <Typography.Text type='tertiary' size='small'>
            {t('仅记录非正常消费的请求与响应（长字符串>100字符会被替换为***）')}
          </Typography.Text>
        </div>
      }
      visible={showChannelAbnormalConsumeModal}
      onCancel={closeChannelAbnormalConsumeModal}
      footer={
        <Button type='tertiary' onClick={closeChannelAbnormalConsumeModal}>
          {t('关闭')}
        </Button>
      }
      maskClosable={false}
      centered
      size='large'
      className='!rounded-lg'
    >
      <Table
        columns={columns}
        dataSource={channelAbnormalConsumeRecords}
        rowKey='id'
        loading={channelAbnormalConsumeLoading}
        pagination={false}
        scroll={{ x: 'max-content' }}
        expandedRowRender={(record) => {
          const requestBody = String(record.request_body || '');
          const responseBody = String(record.response_body || '');
          return (
            <div className='flex flex-col gap-3'>
              <div className='flex flex-wrap gap-3 items-center'>
                <Typography.Text type='tertiary'>
                  request_id: {record.request_id || '-'}
                </Typography.Text>
                <Typography.Text type='tertiary'>
                  path: {record.request_path || '-'}
                </Typography.Text>
              </div>

              <div className='grid grid-cols-1 md:grid-cols-2 gap-3'>
                <div>
                  <div className='flex items-center justify-between mb-1'>
                    <Typography.Text strong>{t('请求体')}</Typography.Text>
                    <Button
                      size='small'
                      type='tertiary'
                      onClick={() => handleCopy(requestBody)}
                    >
                      {t('复制')}
                    </Button>
                  </div>
                  <TextArea
                    rows={10}
                    value={requestBody}
                    readOnly
                    placeholder='-'
                  />
                </div>
                <div>
                  <div className='flex items-center justify-between mb-1'>
                    <Typography.Text strong>{t('响应体')}</Typography.Text>
                    <Button
                      size='small'
                      type='tertiary'
                      onClick={() => handleCopy(responseBody)}
                    >
                      {t('复制')}
                    </Button>
                  </div>
                  <TextArea
                    rows={10}
                    value={responseBody}
                    readOnly
                    placeholder='-'
                  />
                </div>
              </div>
            </div>
          );
        }}
      />
    </Modal>
  );
};

export default ChannelAbnormalConsumeModal;
