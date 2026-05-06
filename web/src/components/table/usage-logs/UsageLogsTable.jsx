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

import React, { useMemo, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Typography } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import { getLogsColumns, getUserLogsColumns } from './UsageLogsColumnDefs';
import LogDetailSideSheet from './LogDetailSideSheet';

const LogsTable = (logsData) => {
  const navigate = useNavigate();
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

  const shouldIgnoreRowClick = (event) => {
    const target = event?.target;
    if (!target || typeof target.closest !== 'function') {
      return false;
    }
    return Boolean(
      target.closest(
        [
          'button',
          'a',
          'input',
          'textarea',
          'select',
          '[role="button"]',
          '.semi-button',
          '.semi-tag',
          '.semi-checkbox',
          '.semi-radio',
          '.semi-switch',
          '.semi-select',
          '.semi-dropdown',
          '.semi-popover',
        ].join(','),
      ),
    );
  };

  const rowDetailProps = hasExpandableRows()
    ? {
        onRow: (record) => ({
          onClickCapture: (event) => {
            if (shouldIgnoreRowClick(event)) {
              return;
            }
            openDetail(record);
          },
          style: { cursor: 'pointer' },
        }),
      }
    : {};

  return (
    <>
      <CardTable
        columns={tableColumns}
        {...rowDetailProps}
        dataSource={logs}
        rowKey='key'
        loading={loading}
        scroll={tableScroll}
        className='usage-logs-table w-full'
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
      <LogDetailSideSheet
        visible={detailVisible}
        onClose={closeDetail}
        record={selectedRecord}
        expandData={expandData}
        isAdminUser={isAdminUser}
        copyText={copyText}
        t={t}
      />
    </>
  );
};

export default LogsTable;
