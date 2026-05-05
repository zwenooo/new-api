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

import React, { useMemo, useRef, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Descriptions, Modal, Typography } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import { getLogsColumns, getUserLogsColumns } from './UsageLogsColumnDefs';
import RequestTraceInline from './RequestTraceInline';
import LogDetailSideSheet from './LogDetailSideSheet';
import { copy, showSuccess } from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const LogsTable = (logsData) => {
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const [selectedRecord, setSelectedRecord] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const {
    logs,
    expandData,
    loading,
    activePage,
    pageSize,
    logCount,
    compactMode,
    visibleColumns,
    handlePageChange,
    handlePageSizeChange,
    copyText,
    groupLabelById,
    showUserInfoFunc,
    hasExpandableRows,
    isAdminUser,
    t,
    COLUMN_KEYS,
  } = logsData;

  const navigateToChannel = (channelID) => {
    const id = Number(channelID || 0);
    if (!Number.isFinite(id) || id <= 0) return;
    navigate(`/console/channel?highlight_channel_id=${id}`);
  };

  // Get all columns
  const allColumns = useMemo(() => {
    return isAdminUser
      ? getLogsColumns({
          t,
          COLUMN_KEYS,
          copyText,
          groupLabelById,
          showUserInfoFunc,
          isAdminUser,
          navigateToChannel,
        })
      : getUserLogsColumns({
          t,
          copyText,
          groupLabelById,
        });
  }, [
    t,
    COLUMN_KEYS,
    copyText,
    groupLabelById,
    showUserInfoFunc,
    isAdminUser,
    navigateToChannel,
  ]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    if (!isAdminUser) {
      return allColumns;
    }
    return allColumns.filter((column) => visibleColumns[column.key]);
  };

  const visibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [visibleColumns, allColumns, isAdminUser]);

  const hasFixedColumns = useMemo(() => {
    return visibleColumnsList.some((column) => column.fixed);
  }, [visibleColumnsList]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? visibleColumnsList.map(({ fixed, ...rest }) => rest)
      : visibleColumnsList;
  }, [compactMode, visibleColumnsList]);

  const tableScroll = useMemo(() => {
    if (compactMode) {
      return undefined;
    }
    return hasFixedColumns ? { x: 'max-content' } : undefined;
  }, [compactMode, hasFixedColumns]);

  const openDetail = useCallback((record) => {
    setSelectedRecord(record);
    setDetailVisible(true);
  }, []);

  const closeDetail = useCallback(() => {
    setDetailVisible(false);
  }, []);

  // Mobile-only: keep inline expansion for CardTable's card mode
  const ExpandedLogDetails = ({ record }) => {
    const detailsRef = useRef(null);

    const expandItems = Array.isArray(expandData?.[record?.key])
      ? expandData[record.key]
      : [];

    const requestId = String(record?.request_id || '').trim();
    const requestUA = String(record?.request_ua || '').trim();
    const endpoint = [record?.request_method, record?.request_path]
      .filter(Boolean)
      .join(' ');

    const summaryTable = useMemo(() => {
      const interfaceKey = t('接口');
      const requestUAKey = t('请求UA');
      const channelInfoKey = t('渠道信息');

      const getTextValue = (item) => {
        if (!item) return '';
        const value = item.value;
        if (typeof value === 'string' || typeof value === 'number') {
          return String(value).trim();
        }
        return '';
      };

      const channelInfoText = getTextValue(
        expandItems.find((item) => item?.key === channelInfoKey),
      );
      const streamExitReasonText = getTextValue(
        expandItems.find((item) => item?.key === 'stream_exit_reason'),
      );

      const cols = [
        {
          key: 'endpoint',
          title: interfaceKey,
          value: endpoint || '--',
          onCopy: endpoint ? (e) => copyText(e, endpoint) : null,
          className: 'truncate',
          titleText: endpoint || '',
        },
        {
          key: 'request_id',
          title: 'request_id',
          value: requestId || '--',
          onCopy: requestId ? (e) => copyText(e, requestId) : null,
          className: 'break-all font-mono text-[11px]',
          titleText: requestId || '',
        },
        {
          key: 'request_ua',
          title: requestUAKey,
          value: requestUA || '--',
          onCopy: requestUA ? (e) => copyText(e, requestUA) : null,
          className: 'truncate',
          titleText: requestUA || '',
        },
        {
          key: 'channel',
          title: channelInfoKey,
          value: channelInfoText || '--',
          onCopy: channelInfoText ? (e) => copyText(e, channelInfoText) : null,
          className: 'truncate',
          titleText: channelInfoText || '',
        },
        {
          key: 'stream_exit_reason',
          title: 'stream_exit_reason',
          value: streamExitReasonText || '--',
          onCopy: streamExitReasonText
            ? (e) => copyText(e, streamExitReasonText)
            : null,
          className: 'truncate',
          titleText: streamExitReasonText || '',
        },
      ];

      return (
        <div className='w-full max-w-full overflow-x-auto rounded border border-semi-color-border bg-semi-color-bg-0'>
          <table className='w-full table-fixed text-xs'>
            <colgroup>
              <col style={{ width: '16%' }} />
              <col style={{ width: '26%' }} />
              <col style={{ width: '28%' }} />
              <col style={{ width: '22%' }} />
              <col style={{ width: '8%' }} />
            </colgroup>
            <thead>
              <tr className='bg-semi-color-bg-1'>
                {cols.map((col) => (
                  <th
                    key={col.key}
                    className='border-b border-semi-color-border px-2 py-1 text-left font-medium'
                  >
                    <Typography.Text type='tertiary'>{col.title}</Typography.Text>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              <tr>
                {cols.map((col) => (
                  <td
                    key={col.key}
                    className='border-b border-semi-color-border px-2 py-1 align-top'
                  >
                    <div
                      className={`w-full max-w-full select-text ${
                        col.onCopy ? 'cursor-pointer hover:opacity-80' : ''
                      } ${col.className || ''}`}
                      title={col.titleText || undefined}
                      onClick={col.onCopy || undefined}
                    >
                      {col.value}
                    </div>
                  </td>
                ))}
              </tr>
            </tbody>
          </table>
        </div>
      );
    }, [copyText, expandItems, endpoint, requestId, requestUA, t]);

    const detailsItems = useMemo(() => {
      const interfaceKey = t('接口');
      const requestUAKey = t('请求UA');
      const channelInfoKey = t('渠道信息');
      const excludeKeys = new Set([
        interfaceKey,
        'request_id',
        requestUAKey,
        channelInfoKey,
        'stream_exit_reason',
      ]);
      return expandItems.filter(
        (item) => item && item.key && !excludeKeys.has(item.key),
      );
    }, [expandItems, t]);

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

    return (
      <div className='w-full min-w-0 max-w-full overflow-x-auto flex flex-col gap-2'>
        <div className='flex items-center justify-end'>
          <Button size='small' theme='light' onClick={copyDetails}>
            {t('复制')}
          </Button>
        </div>

        <div ref={detailsRef} className='w-full max-w-full overflow-x-auto select-text'>
          <div className='flex flex-col gap-2'>
            {summaryTable}
            <Descriptions data={detailsItems} size='small' column={1} className='w-full max-w-full' />
          </div>
        </div>

        {isAdminUser && requestId ? <RequestTraceInline requestId={requestId} t={t} /> : null}
      </div>
    );
  };

  const expandRowRender = (record) => {
    return <ExpandedLogDetails record={record} />;
  };

  // Desktop: open SideSheet on row click; Mobile: use inline expansion via CardTable cards
  const desktopRowProps = !isMobile && hasExpandableRows()
    ? {
        onRow: (record) => ({
          onClick: () => openDetail(record),
          style: { cursor: 'pointer' },
        }),
      }
    : {};

  const mobileExpandProps = isMobile && hasExpandableRows()
    ? {
        expandedRowRender: expandRowRender,
        expandRowByClick: true,
        rowExpandable: () => true,
      }
    : {};

  return (
    <>
      <CardTable
        columns={tableColumns}
        {...mobileExpandProps}
        {...desktopRowProps}
        dataSource={logs}
        rowKey='key'
        loading={loading}
        scroll={tableScroll}
        className='usage-logs-table rounded-xl overflow-hidden w-full'
        style={{ minWidth: '100%' }}
        size='middle'
        empty={<></>}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: logCount,
          pageSizeOptions: [10, 20, 50, 100],
          showSizeChanger: true,
          onPageSizeChange: (size) => {
            handlePageSizeChange(size);
          },
          onPageChange: handlePageChange,
        }}
        hidePagination={true}
      />
      {!isMobile && (
        <LogDetailSideSheet
          visible={detailVisible}
          onClose={closeDetail}
          record={selectedRecord}
          expandData={expandData}
          isAdminUser={isAdminUser}
          copyText={copyText}
          t={t}
        />
      )}
    </>
  );
};

export default LogsTable;
