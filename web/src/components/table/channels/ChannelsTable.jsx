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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useLocation } from 'react-router-dom';
import CardTable from '../../common/ui/CardTable';
import { getChannelsColumns } from './ChannelsColumnDefs';

const ChannelsTable = (channelsData) => {
  const location = useLocation();
  const highlightTriedRef = useRef(false);
  const [expandedRowKeys, setExpandedRowKeys] = useState([]);
  const {
    channels,
    groupOptions,
    loading,
    searching,
    activePage,
    pageSize,
    channelCount,
    enableBatchDelete,
    compactMode,
    visibleColumns,
    selectedRowKeys,
    handleRowSelectionChange,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    t,
    COLUMN_KEYS,
    // Column functions and data
    updateChannelBalance,
    resetChannelUsedQuota,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    channelAbnormalConsumeEnabledMap,
    updateChannelAbnormalConsumeEnabled,
    openChannelAbnormalConsumeModal,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    // Multi-key management
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    setShowChannelProfitStatsModal,
    setCurrentProfitStatsChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
    enableTagMode,
    formApi,
    searchChannels,
  } = channelsData;

  const groupLabelById = useMemo(() => {
    const map = new Map();
    (Array.isArray(groupOptions) ? groupOptions : []).forEach((opt) => {
      const id = Number(opt?.value ?? 0);
      if (!Number.isFinite(id) || id <= 0) return;
      const label = String(opt?.label ?? '').trim();
      if (!label) return;
      map.set(Math.floor(id), label);
    });
    return map;
  }, [groupOptions]);

  const highlightChannelID = useMemo(() => {
    const params = new URLSearchParams(location?.search || '');
    return Number(params.get('highlight_channel_id') || 0);
  }, [location?.search]);

  useEffect(() => {
    highlightTriedRef.current = false;
  }, [highlightChannelID]);

  useEffect(() => {
    if (!enableTagMode) {
      setExpandedRowKeys([]);
    }
  }, [enableTagMode]);

  useEffect(() => {
    if (!Number.isFinite(highlightChannelID) || highlightChannelID <= 0) return;
    if (loading || searching) return;

    const rowKey = String(highlightChannelID);

    const highlightNow = () => {
      const el = document.querySelector(`[data-row-key="${rowKey}"]`);
      if (!el) return false;
      el.scrollIntoView({ block: 'center' });
      el.classList.add('cx-nav-highlight');
      window.setTimeout(() => {
        el.classList.remove('cx-nav-highlight');
      }, 3000);
      return true;
    };

    requestAnimationFrame(() => {
      if (enableTagMode) {
        const list = Array.isArray(channels) ? channels : [];
        const group = list.find((it) => {
          const children = Array.isArray(it?.children) ? it.children : [];
          return children.some((c) => String(c?.key || '') === rowKey);
        });
        const groupKey = group?.key;
        if (
          groupKey !== undefined &&
          groupKey !== null &&
          !expandedRowKeys.includes(groupKey)
        ) {
          setExpandedRowKeys((prev) => {
            const cur = Array.isArray(prev) ? prev : [];
            if (cur.includes(groupKey)) return cur;
            return [...cur, groupKey];
          });
          return;
        }
      }

      if (highlightNow()) return;
      if (highlightTriedRef.current) return;
      if (!formApi || typeof formApi.setValues !== 'function') return;
      if (typeof searchChannels !== 'function') return;

      highlightTriedRef.current = true;
      formApi.setValues({
        searchKeyword: rowKey,
        searchGroup: '',
        searchModel: '',
      });
      setTimeout(() => {
        searchChannels(enableTagMode);
      }, 0);
    });
  }, [
    highlightChannelID,
    loading,
    searching,
    channels,
    enableTagMode,
    expandedRowKeys,
    formApi,
    searchChannels,
  ]);

  // Get all columns
  const allColumns = useMemo(() => {
    return getChannelsColumns({
      t,
      COLUMN_KEYS,
      groupLabelById,
      updateChannelBalance,
      resetChannelUsedQuota,
      manageChannel,
      manageTag,
      submitTagEdit,
      testChannel,
      channelAbnormalConsumeEnabledMap,
      updateChannelAbnormalConsumeEnabled,
      openChannelAbnormalConsumeModal,
      setCurrentTestChannel,
      setShowModelTestModal,
      setEditingChannel,
      setShowEdit,
      setShowEditTag,
      setEditingTag,
      copySelectedChannel,
      refresh,
      activePage,
      channels,
      setShowMultiKeyManageModal,
      setCurrentMultiKeyChannel,
      setShowChannelProfitStatsModal,
      setCurrentProfitStatsChannel,
      openUpstreamUpdateModal,
      detectChannelUpstreamUpdates,
    });
  }, [
    t,
    COLUMN_KEYS,
    groupLabelById,
    updateChannelBalance,
    resetChannelUsedQuota,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    channelAbnormalConsumeEnabledMap,
    updateChannelAbnormalConsumeEnabled,
    openChannelAbnormalConsumeModal,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    activePage,
    channels,
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    setShowChannelProfitStatsModal,
    setCurrentProfitStatsChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
  ]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    return allColumns.filter((column) => visibleColumns[column.key]);
  };

  const visibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [visibleColumns, allColumns]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? visibleColumnsList.map(({ fixed, ...rest }) => rest)
      : visibleColumnsList;
  }, [compactMode, visibleColumnsList]);

  const handleExpand = (expanded, record) => {
    if (!enableTagMode) return;
    const key = record?.key ?? record?.groupKey;
    if (key === undefined || key === null) return;
    setExpandedRowKeys((prev) => {
      const cur = Array.isArray(prev) ? prev : [];
      return expanded ? [...new Set([...cur, key])] : cur.filter((k) => k !== key);
    });
  };

  const tagModeTreeProps = enableTagMode
    ? {
        expandedRowKeys,
        onExpand: handleExpand,
      }
    : {};

  return (
    <CardTable
      {...tagModeTreeProps}
      columns={tableColumns}
      dataSource={channels}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: channelCount,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
      expandAllRows={false}
      onRow={handleRow}
      rowSelection={
        enableBatchDelete
          ? {
              selectedRowKeys,
              onChange: handleRowSelectionChange,
            }
          : null
      }
      empty={<></>}
      className='rounded-xl overflow-hidden'
      size='middle'
      loading={loading || searching}
    />
  );
};

export default ChannelsTable;
