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

import React, { useMemo, useRef } from 'react';
import {
  SideSheet,
  Button,
  Collapse,
  Modal,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconClose, IconCopy } from '@douyinfe/semi-icons';
import RequestTraceInline from './RequestTraceInline';
import { copy, showSuccess } from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const SummaryRow = ({ label, value, onCopy, mono }) => (
  <div className='log-detail-summary-row'>
    <span className='log-detail-summary-label'>{label}</span>
    <span
      className={`log-detail-summary-value ${mono ? 'font-mono text-[11px]' : ''} ${onCopy ? 'cursor-pointer hover:opacity-80' : ''}`}
      title={typeof value === 'string' ? value : undefined}
      onClick={onCopy || undefined}
    >
      {value || '--'}
    </span>
  </div>
);

const LogDetailSideSheet = ({
  visible,
  onClose,
  record,
  expandData,
  isAdminUser,
  copyText,
  t,
}) => {
  const isMobile = useIsMobile();
  const detailsRef = useRef(null);

  const expandItems = useMemo(() => {
    if (!record) return [];
    return Array.isArray(expandData?.[record?.key]) ? expandData[record.key] : [];
  }, [record, expandData]);

  const requestId = String(record?.request_id || '').trim();
  const requestUA = String(record?.request_ua || '').trim();
  const endpoint = [record?.request_method, record?.request_path]
    .filter(Boolean)
    .join(' ');

  const channelInfoKey = t('渠道信息');
  const requestUAKey = t('请求UA');
  const interfaceKey = t('接口');

  const channelInfoText = useMemo(() => {
    const item = expandItems.find((i) => i?.key === channelInfoKey);
    if (!item) return '';
    const v = item.value;
    return typeof v === 'string' || typeof v === 'number' ? String(v).trim() : '';
  }, [expandItems, channelInfoKey]);

  const streamExitReasonText = useMemo(() => {
    const item = expandItems.find((i) => i?.key === 'stream_exit_reason');
    if (!item) return '';
    const v = item.value;
    return typeof v === 'string' || typeof v === 'number' ? String(v).trim() : '';
  }, [expandItems]);

  const detailsItems = useMemo(() => {
    const excludeKeys = new Set([
      interfaceKey,
      'request_id',
      requestUAKey,
      channelInfoKey,
      'stream_exit_reason',
    ]);
    return expandItems.filter((item) => item && item.key && !excludeKeys.has(item.key));
  }, [expandItems, interfaceKey, requestUAKey, channelInfoKey]);

  const copyDetails = async (e) => {
    if (e?.stopPropagation) e.stopPropagation();
    const text = String(detailsRef.current?.innerText || '').trim();
    if (!text) return;
    if (await copy(text)) {
      showSuccess(t('已复制'));
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制') });
    }
  };

  if (!record) return null;

  const title = (
    <div className='flex items-center gap-2 min-w-0'>
      <Typography.Text strong className='truncate'>
        {record.model_name || t('日志详情')}
      </Typography.Text>
      {record.type !== undefined && (
        <Tag size='small' color='blue' shape='circle'>
          {record.timestamp2string}
        </Tag>
      )}
    </div>
  );

  return (
    <SideSheet
      placement='right'
      title={title}
      visible={visible}
      width={isMobile ? '100%' : 720}
      onCancel={onClose}
      closeIcon={
        <Button
          className='semi-button-tertiary semi-button-size-small semi-button-borderless'
          type='button'
          icon={<IconClose />}
          onClick={onClose}
        />
      }
      bodyStyle={{
        padding: 0,
        display: 'flex',
        flexDirection: 'column',
      }}
      className='log-detail-sidesheet'
    >
      <div className='log-detail-sidesheet-body' ref={detailsRef}>
        {/* Summary section */}
        <div className='log-detail-section'>
          <div className='log-detail-section-header'>
            <Typography.Text type='tertiary' size='small'>
              {t('摘要')}
            </Typography.Text>
            <Button
              size='small'
              theme='light'
              icon={<IconCopy size='small' />}
              onClick={copyDetails}
            >
              {t('复制')}
            </Button>
          </div>
          <div className='log-detail-summary-grid'>
            <SummaryRow
              label={interfaceKey}
              value={endpoint}
              onCopy={endpoint ? (e) => copyText(e, endpoint) : null}
            />
            <SummaryRow
              label='request_id'
              value={requestId}
              onCopy={requestId ? (e) => copyText(e, requestId) : null}
              mono
            />
            <SummaryRow
              label={requestUAKey}
              value={requestUA}
              onCopy={requestUA ? (e) => copyText(e, requestUA) : null}
            />
            <SummaryRow
              label={channelInfoKey}
              value={channelInfoText}
              onCopy={channelInfoText ? (e) => copyText(e, channelInfoText) : null}
            />
            {streamExitReasonText ? (
              <SummaryRow
                label='stream_exit_reason'
                value={streamExitReasonText}
                onCopy={(e) => copyText(e, streamExitReasonText)}
              />
            ) : null}
          </div>
        </div>

        {/* Detail items */}
        {detailsItems.length > 0 && (
          <div className='log-detail-section'>
            <Collapse accordion defaultActiveKey='0'>
              {detailsItems.map((item, idx) => (
                <Collapse.Panel
                  header={
                    <Typography.Text strong size='small'>
                      {item.key}
                    </Typography.Text>
                  }
                  itemKey={String(idx)}
                  key={String(idx)}
                >
                  <div className='log-detail-collapse-content select-text'>
                    {item.value}
                  </div>
                </Collapse.Panel>
              ))}
            </Collapse>
          </div>
        )}

        {/* Request trace */}
        {isAdminUser && requestId ? (
          <div className='log-detail-section'>
            <RequestTraceInline requestId={requestId} t={t} />
          </div>
        ) : null}
      </div>
    </SideSheet>
  );
};

export default LogDetailSideSheet;

