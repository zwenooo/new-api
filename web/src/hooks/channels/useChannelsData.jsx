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

import { useState, useEffect, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  loadChannelModels,
  copy,
} from '../../helpers';
import {
  CHANNEL_OPTIONS,
  ITEMS_PER_PAGE,
  MODEL_TABLE_PAGE_SIZE,
} from '../../constants';
import { useIsMobile } from '../common/useIsMobile';
import { useTableCompactMode } from '../common/useTableCompactMode';
import { useChannelUpstreamUpdates } from './useChannelUpstreamUpdates';
import { parseUpstreamUpdateMeta } from './upstreamUpdateUtils';
import { Modal } from '@douyinfe/semi-ui';

export const useChannelsData = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  // Basic states
  const [channels, setChannels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [idSort, setIdSort] = useState(false);
  const [searching, setSearching] = useState(false);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [channelCount, setChannelCount] = useState(0);
  const [groupOptions, setGroupOptions] = useState([]);

  // UI states
  const [showEdit, setShowEdit] = useState(false);
  const [enableBatchDelete, setEnableBatchDelete] = useState(false);
  const [editingChannel, setEditingChannel] = useState({ id: undefined });
  const [showEditTag, setShowEditTag] = useState(false);
  const [editingTag, setEditingTag] = useState('');
  const [selectedChannels, setSelectedChannels] = useState([]);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [enableTagMode, setEnableTagMode] = useState(false);
  const [showBatchSetTag, setShowBatchSetTag] = useState(false);
  const [batchSetTagValue, setBatchSetTagValue] = useState('');
  const [showBatchSetGroup, setShowBatchSetGroup] = useState(false);
  const [batchSetGroupValue, setBatchSetGroupValue] = useState([]);
  const [showBatchUpdateModels, setShowBatchUpdateModels] = useState(false);
  const [batchAddModelsValue, setBatchAddModelsValue] = useState('');
  const [batchRemoveModelsValue, setBatchRemoveModelsValue] = useState('');
  const [showBatchBindUsers, setShowBatchBindUsers] = useState(false);
  const [batchBindUsersValue, setBatchBindUsersValue] = useState('');
  const [compactMode, setCompactMode] = useTableCompactMode('channels');

  // Channel-level abnormal consume collection
  const [channelAbnormalConsumeEnabledMap, setChannelAbnormalConsumeEnabledMap] =
    useState({});
  const [showChannelAbnormalConsumeModal, setShowChannelAbnormalConsumeModal] =
    useState(false);
  const [currentAbnormalConsumeChannel, setCurrentAbnormalConsumeChannel] =
    useState(null);
  const [channelAbnormalConsumeRecords, setChannelAbnormalConsumeRecords] =
    useState([]);
  const [channelAbnormalConsumeLoading, setChannelAbnormalConsumeLoading] =
    useState(false);

  // Channel profit stats modal
  const [showChannelProfitStatsModal, setShowChannelProfitStatsModal] =
    useState(false);
  const [currentProfitStatsChannel, setCurrentProfitStatsChannel] =
    useState(null);

  // Column visibility states
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Status filter
  const [statusFilter, setStatusFilter] = useState(
    localStorage.getItem('channel-status-filter') || 'all',
  );

  // Type tabs states
  const [activeTypeKey, setActiveTypeKey] = useState('all');
  const [typeCounts, setTypeCounts] = useState({});

  // Model test states
  const [showModelTestModal, setShowModelTestModal] = useState(false);
  const [currentTestChannel, setCurrentTestChannel] = useState(null);
  const [modelSearchKeyword, setModelSearchKeyword] = useState('');
  const [modelTestResults, setModelTestResults] = useState({});
  const [testingModels, setTestingModels] = useState(new Set());
  const [selectedModelKeys, setSelectedModelKeys] = useState([]);
  const [isBatchTesting, setIsBatchTesting] = useState(false);
  const [testQueue, setTestQueue] = useState([]);
  const [isProcessingQueue, setIsProcessingQueue] = useState(false);
  const [modelTablePage, setModelTablePage] = useState(1);

  // Multi-key management states
  const [showMultiKeyManageModal, setShowMultiKeyManageModal] = useState(false);
  const [currentMultiKeyChannel, setCurrentMultiKeyChannel] = useState(null);

  // Refs
  const requestCounter = useRef(0);
  const allSelectingRef = useRef(false);
  const [formApi, setFormApi] = useState(null);

  const formInitValues = {
    searchKeyword: '',
    searchGroup: '',
    searchModel: '',
  };

  // Column keys
  const COLUMN_KEYS = {
    ID: 'id',
    NAME: 'name',
    GROUP: 'group',
    TYPE: 'type',
    STATUS: 'status',
    RESPONSE_TIME: 'response_time',
    BALANCE: 'balance',
    BUY_PRICE: 'buy_price',
    PROFIT: 'profit',
    PRIORITY: 'priority',
    WEIGHT: 'weight',
    OPERATE: 'operate',
  };

  // Initialize from localStorage
  useEffect(() => {
    const localIdSort = localStorage.getItem('id-sort') === 'true';
    const localPageSize =
      parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    const localEnableTagMode =
      localStorage.getItem('enable-tag-mode') === 'true';
    const localEnableBatchDelete =
      localStorage.getItem('enable-batch-delete') === 'true';

    setIdSort(localIdSort);
    setPageSize(localPageSize);
    setEnableTagMode(localEnableTagMode);
    setEnableBatchDelete(localEnableBatchDelete);

    loadChannels(1, localPageSize, localIdSort, localEnableTagMode)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    fetchGroups().then();
    fetchChannelAbnormalConsumeConfig().then();
    loadChannelModels().then();
  }, []);

  // Column visibility management
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.ID]: true,
      [COLUMN_KEYS.NAME]: true,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.STATUS]: true,
      [COLUMN_KEYS.RESPONSE_TIME]: true,
      [COLUMN_KEYS.BALANCE]: true,
      [COLUMN_KEYS.BUY_PRICE]: true,
      [COLUMN_KEYS.PROFIT]: true,
      [COLUMN_KEYS.PRIORITY]: true,
      [COLUMN_KEYS.WEIGHT]: true,
      [COLUMN_KEYS.OPERATE]: true,
    };
  };

  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
  };

  // Load saved column preferences
  useEffect(() => {
    const savedColumns = localStorage.getItem('channels-table-columns');
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults, ...parsed };
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Save column preferences
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem(
        'channels-table-columns',
        JSON.stringify(visibleColumns),
      );
    }
  }, [visibleColumns]);

  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};
    allKeys.forEach((key) => {
      updatedColumns[key] = checked;
    });
    setVisibleColumns(updatedColumns);
  };

  // Data formatting
  const setChannelFormat = (channels, enableTagMode) => {
    let channelDates = [];
    let channelTags = {};

    for (let i = 0; i < channels.length; i++) {
      channels[i].upstreamUpdateMeta = parseUpstreamUpdateMeta(
        channels[i].settings,
      );
      channels[i].key = '' + channels[i].id;
      if (!enableTagMode) {
        channelDates.push(channels[i]);
      } else {
        let tag = channels[i].tag ? channels[i].tag : '';
        let tagIndex = channelTags[tag];
        let tagChannelDates = undefined;

        if (tagIndex === undefined) {
          channelTags[tag] = 1;
          tagChannelDates = {
            key: tag,
            id: tag,
            tag: tag,
            name: '标签：' + tag,
            group_ids: [],
            backup_group_ids: [],
            used_quota: 0,
            cost_used_quota: 0,
            response_time: 0,
            priority: -1,
            weight: -1,
          };
          tagChannelDates.children = [];
          channelDates.push(tagChannelDates);
        } else {
          tagChannelDates = channelDates.find((item) => item.key === tag);
        }

        if (tagChannelDates.priority === -1) {
          tagChannelDates.priority = channels[i].priority;
        } else {
          if (tagChannelDates.priority !== channels[i].priority) {
            tagChannelDates.priority = '';
          }
        }

        if (tagChannelDates.weight === -1) {
          tagChannelDates.weight = channels[i].weight;
        } else {
          if (tagChannelDates.weight !== channels[i].weight) {
            tagChannelDates.weight = '';
          }
        }

        const channelGroupIds = Array.isArray(channels[i]?.group_ids)
          ? channels[i].group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v))
          : [];
        const existingGroupIds = Array.isArray(tagChannelDates?.group_ids)
          ? tagChannelDates.group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v))
          : [];
        const channelBackupGroupIds = Array.isArray(channels[i]?.backup_group_ids)
          ? channels[i].backup_group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v))
          : [];
        const existingBackupGroupIds = Array.isArray(
          tagChannelDates?.backup_group_ids,
        )
          ? tagChannelDates.backup_group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v))
          : [];
        if (channelGroupIds.length > 0) {
          tagChannelDates.group_ids = Array.from(
            new Set([...existingGroupIds, ...channelGroupIds]),
          ).sort((a, b) => a - b);
        } else if (existingGroupIds.length > 0) {
          tagChannelDates.group_ids = existingGroupIds;
        }
        if (channelBackupGroupIds.length > 0) {
          tagChannelDates.backup_group_ids = Array.from(
            new Set([...existingBackupGroupIds, ...channelBackupGroupIds]),
          ).sort((a, b) => a - b);
        } else if (existingBackupGroupIds.length > 0) {
          tagChannelDates.backup_group_ids = existingBackupGroupIds;
        }

        tagChannelDates.children.push(channels[i]);
        if (channels[i].status === 1) {
          tagChannelDates.status = 1;
        }
        tagChannelDates.used_quota += channels[i].used_quota;
        tagChannelDates.cost_used_quota += Number(channels[i].cost_used_quota) || 0;
        tagChannelDates.response_time += channels[i].response_time;
        tagChannelDates.response_time = tagChannelDates.response_time / 2;
      }
    }
    setChannels(channelDates);
  };

  // Get form values helper
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
      searchGroup: formValues.searchGroup || '',
      searchModel: formValues.searchModel || '',
    };
  };

  // Load channels
  const loadChannels = async (
    page,
    pageSize,
    idSort,
    enableTagMode,
    typeKey = activeTypeKey,
    statusF,
  ) => {
    if (statusF === undefined) statusF = statusFilter;

    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    if (searchKeyword !== '' || searchGroup !== '' || searchModel !== '') {
      setLoading(true);
      await searchChannels(
        enableTagMode,
        typeKey,
        statusF,
        page,
        pageSize,
        idSort,
      );
      setLoading(false);
      return;
    }

    const reqId = ++requestCounter.current;
    setLoading(true);
    const typeParam = typeKey !== 'all' ? `&type=${typeKey}` : '';
    const statusParam = statusF !== 'all' ? `&status=${statusF}` : '';
    const res = await API.get(
      `/api/channel/?p=${page}&page_size=${pageSize}&id_sort=${idSort}&tag_mode=${enableTagMode}${typeParam}${statusParam}`,
    );

    if (res === undefined || reqId !== requestCounter.current) {
      return;
    }

    const { success, message, data } = res.data;
    if (success) {
      const { items, total, type_counts } = data;
      if (type_counts) {
        const sumAll = Object.values(type_counts).reduce(
          (acc, v) => acc + v,
          0,
        );
        setTypeCounts({ ...type_counts, all: sumAll });
      }
      setChannelFormat(items, enableTagMode);
      setChannelCount(total);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Search channels
  const searchChannels = async (
    enableTagMode,
    typeKey = activeTypeKey,
    statusF = statusFilter,
    page = 1,
    pageSz = pageSize,
    sortFlag = idSort,
  ) => {
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    setSearching(true);
    try {
      if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
        await loadChannels(
          page,
          pageSz,
          sortFlag,
          enableTagMode,
          typeKey,
          statusF,
        );
        return;
      }

      const typeParam = typeKey !== 'all' ? `&type=${typeKey}` : '';
      const statusParam = statusF !== 'all' ? `&status=${statusF}` : '';
      const res = await API.get(
        `/api/channel/search?keyword=${searchKeyword}&group_id=${searchGroup}&model=${searchModel}&id_sort=${sortFlag}&tag_mode=${enableTagMode}&p=${page}&page_size=${pageSz}${typeParam}${statusParam}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const { items = [], total = 0, type_counts = {} } = data;
        const sumAll = Object.values(type_counts).reduce(
          (acc, v) => acc + v,
          0,
        );
        setTypeCounts({ ...type_counts, all: sumAll });
        setChannelFormat(items, enableTagMode);
        setChannelCount(total);
        setActivePage(page);
      } else {
        showError(message);
      }
    } finally {
      setSearching(false);
    }
  };

  // Refresh
  const refresh = async (page = activePage) => {
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      await loadChannels(page, pageSize, idSort, enableTagMode);
    } else {
      await searchChannels(
        enableTagMode,
        activeTypeKey,
        statusFilter,
        page,
        pageSize,
        idSort,
      );
    }
  };

  const upstreamUpdates = useChannelUpstreamUpdates({ t, refresh });

  // Channel management
  const manageChannel = async (id, action, record, value) => {
    let data = { id };
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(`/api/channel/${id}/`);
        break;
      case 'enable':
        data.status = 1;
        res = await API.put('/api/channel/', data);
        break;
      case 'disable':
        data.status = 2;
        res = await API.put('/api/channel/', data);
        break;
      case 'priority':
        if (value === '' || value === undefined || value === null) return;
        data.priority = parseInt(value, 10);
        if (!Number.isFinite(data.priority)) {
          showInfo(t('优先级必须是整数！'));
          return;
        }
        res = await API.put('/api/channel/', data);
        break;
      case 'weight':
        if (value === '' || value === undefined || value === null) return;
        data.weight = parseInt(value, 10);
        if (!Number.isFinite(data.weight)) {
          showInfo(t('权重必须是非负整数！'));
          return;
        }
        if (data.weight < 0) data.weight = 0;
        res = await API.put('/api/channel/', data);
        break;
      case 'enable_all':
        data.channel_info = record.channel_info;
        data.channel_info.multi_key_status_list = {};
        res = await API.put('/api/channel/', data);
        break;
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('操作成功完成！'));
      let channel = res.data.data;
      let newChannels = [...channels];
      if (action !== 'delete') {
        record.status = channel.status;
        if (action === 'priority') {
          record.priority = channel.priority;
        }
        if (action === 'weight') {
          record.weight = channel.weight;
        }
        if (action === 'enable_all') {
          record.channel_info = channel.channel_info;
        }
        if (
          enableTagMode &&
          (action === 'priority' || action === 'weight') &&
          typeof record?.tag === 'string' &&
          record.tag
        ) {
          const tagRow = newChannels.find((it) => it?.key === record.tag);
          const children = Array.isArray(tagRow?.children) ? tagRow.children : [];
          if (children.length > 0) {
            const firstPriority = children[0]?.priority;
            const allSamePriority = children.every(
              (ch) => ch?.priority === firstPriority,
            );
            tagRow.priority =
              allSamePriority && firstPriority !== undefined && firstPriority !== null
                ? firstPriority
                : '';

            const firstWeight = children[0]?.weight;
            const allSameWeight = children.every((ch) => ch?.weight === firstWeight);
            tagRow.weight =
              allSameWeight && firstWeight !== undefined && firstWeight !== null
                ? firstWeight
                : '';
          }
        }
      }
      setChannels(newChannels);
    } else {
      showError(message);
    }
  };

  // Tag management
  const manageTag = async (tag, action) => {
    let res;
    switch (action) {
      case 'enable':
        res = await API.post('/api/channel/tag/enabled', { tag: tag });
        break;
      case 'disable':
        res = await API.post('/api/channel/tag/disabled', { tag: tag });
        break;
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess('操作成功完成！');
      let newChannels = [...channels];
      for (let i = 0; i < newChannels.length; i++) {
        if (newChannels[i].tag === tag) {
          let status = action === 'enable' ? 1 : 2;
          newChannels[i]?.children?.forEach((channel) => {
            channel.status = status;
          });
          newChannels[i].status = status;
        }
      }
      setChannels(newChannels);
    } else {
      showError(message);
    }
  };

  // Page handlers
  const handlePageChange = (page) => {
    const nextPage =
      typeof page === 'number'
        ? page
        : Number(page?.currentPage ?? page?.page ?? page);
    if (!Number.isFinite(nextPage) || nextPage < 1) {
      return;
    }
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    setActivePage(nextPage);
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      loadChannels(nextPage, pageSize, idSort, enableTagMode).then(() => {});
    } else {
      searchChannels(
        enableTagMode,
        activeTypeKey,
        statusFilter,
        nextPage,
        pageSize,
        idSort,
      );
    }
  };

  const handlePageSizeChange = async (size) => {
    const nextSize =
      typeof size === 'number' ? size : Number(size?.pageSize ?? size);
    if (!Number.isFinite(nextSize) || nextSize <= 0) {
      return;
    }
    localStorage.setItem('page-size', nextSize + '');
    setPageSize(nextSize);
    setActivePage(1);
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      loadChannels(1, nextSize, idSort, enableTagMode)
        .then()
        .catch((reason) => {
          showError(reason);
        });
    } else {
      searchChannels(
        enableTagMode,
        activeTypeKey,
        statusFilter,
        1,
        nextSize,
        idSort,
      );
    }
  };

  // Fetch groups
  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      if (res === undefined) return;
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        return;
      }
      const list = Array.isArray(data) ? data : [];
      const normalized = list
        .map((g) => {
          if (typeof g === 'string') {
            const code = String(g || '').trim();
            return code ? { id: 0, code, display_name: code } : null;
          }
          const id = Number(g?.id ?? 0);
          if (!Number.isFinite(id) || id <= 0) return null;
          const code = String(g?.code || '').trim();
          const displayName = String(g?.display_name || '').trim();
          const name = displayName || code;
          if (!name) return null;
          return { id: Math.floor(id), code, display_name: name };
        })
        .filter(Boolean);
      setGroupOptions(
        normalized.map((g) => ({
          label: g.display_name || g.code,
          value: g.id,
        })),
      );
    } catch (error) {
      showError(error.message);
    }
  };

  const fetchChannelAbnormalConsumeConfig = async () => {
    try {
      const res = await API.get('/api/channel/abnormal_consume/config');
      if (!res?.data) return;
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setChannelAbnormalConsumeEnabledMap(data?.enabled || {});
    } catch (error) {
      showError(error.message);
    }
  };

  const updateChannelAbnormalConsumeEnabled = async (channelId, enabled) => {
    const id = Number(channelId);
    if (!Number.isFinite(id) || id <= 0) {
      showError(t('渠道ID无效'));
      return;
    }
    try {
      const res = await API.put('/api/channel/abnormal_consume/config', {
        channel_id: id,
        enabled,
      });
      if (!res?.data) return;
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      setChannelAbnormalConsumeEnabledMap((prev) => ({
        ...prev,
        [id]: enabled,
      }));
      showSuccess(enabled ? t('已开启异常统计') : t('已关闭异常统计'));
    } catch (error) {
      showError(
        error?.response?.data?.message || error?.message || t('操作失败'),
      );
    }
  };

  const loadChannelAbnormalConsumeRecords = async (channelId) => {
    const id = Number(
      channelId || currentAbnormalConsumeChannel?.id || 0,
    );
    if (!Number.isFinite(id) || id <= 0) {
      showError(t('渠道ID无效'));
      return;
    }
    setChannelAbnormalConsumeLoading(true);
    try {
      const res = await API.get(
        `/api/channel/abnormal_consume?channel_id=${encodeURIComponent(id)}`,
      );
      if (!res?.data) return;
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取异常统计失败'));
        return;
      }
      setChannelAbnormalConsumeRecords(data?.items || []);
      if (typeof data?.enabled === 'boolean') {
        setChannelAbnormalConsumeEnabledMap((prev) => ({
          ...prev,
          [id]: data.enabled,
        }));
      }
    } catch (error) {
      showError(
        error?.response?.data?.message ||
          error?.message ||
          t('获取异常统计失败'),
      );
    } finally {
      setChannelAbnormalConsumeLoading(false);
    }
  };

  const clearChannelAbnormalConsumeRecords = async (channelId) => {
    const id = Number(
      channelId || currentAbnormalConsumeChannel?.id || 0,
    );
    if (!Number.isFinite(id) || id <= 0) {
      showError(t('渠道ID无效'));
      return;
    }
    setChannelAbnormalConsumeLoading(true);
    try {
      const res = await API.delete(
        `/api/channel/abnormal_consume?channel_id=${encodeURIComponent(id)}`,
      );
      if (!res?.data) return;
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('清空失败'));
        return;
      }
      setChannelAbnormalConsumeRecords([]);
      showSuccess(t('已清空异常统计'));
    } catch (error) {
      showError(
        error?.response?.data?.message || error?.message || t('清空失败'),
      );
    } finally {
      setChannelAbnormalConsumeLoading(false);
    }
  };

  const openChannelAbnormalConsumeModal = async (channel) => {
    const id = Number(channel?.id || 0);
    if (!Number.isFinite(id) || id <= 0) {
      showError(t('渠道ID无效'));
      return;
    }
    setCurrentAbnormalConsumeChannel(channel);
    setShowChannelAbnormalConsumeModal(true);
    await loadChannelAbnormalConsumeRecords(id);
  };

  const closeChannelAbnormalConsumeModal = () => {
    setShowChannelAbnormalConsumeModal(false);
  };

  // Copy channel
  const copySelectedChannel = async (record) => {
    try {
      const res = await API.post(`/api/channel/copy/${record.id}`);
      if (res?.data?.success) {
        showSuccess(t('渠道复制成功'));
        await refresh();
      } else {
        showError(res?.data?.message || t('渠道复制失败'));
      }
    } catch (error) {
      showError(
        t('渠道复制失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
  };

  // Update channel property
  const updateChannelProperty = (channelId, updateFn) => {
    const newChannels = [...channels];
    let updated = false;

    newChannels.forEach((channel) => {
      if (channel.children !== undefined) {
        channel.children.forEach((child) => {
          if (child.id === channelId) {
            updateFn(child);
            updated = true;
          }
        });
      } else if (channel.id === channelId) {
        updateFn(channel);
        updated = true;
      }
    });

    if (updated) {
      setChannels(newChannels);
    }
  };

  // Tag edit
  const submitTagEdit = async (type, data) => {
    switch (type) {
      case 'priority':
        if (data.priority === undefined || data.priority === '') {
          showInfo('优先级必须是整数！');
          return;
        }
        data.priority = parseInt(data.priority, 10);
        break;
      case 'weight':
        if (
          data.weight === undefined ||
          data.weight < 0 ||
          data.weight === ''
        ) {
          showInfo('权重必须是非负整数！');
          return;
        }
        data.weight = parseInt(data.weight, 10);
        break;
    }

    try {
      const res = await API.put('/api/channel/tag', data);
      const { success, message } = res?.data || {};
      if (success) {
        showSuccess('更新成功！');
        await refresh();
      } else {
        showError(message || t('操作失败'));
      }
    } catch (error) {
      showError(error);
    }
  };

  // Close edit
  const closeEdit = () => {
    setShowEdit(false);
  };

  // 行选择（桌面端表格勾选逻辑）
  // - 普通模式：直接使用 Semi Table 提供的选中结果
  // - 标签聚合模式：勾选/取消勾选「标签汇总行」时，自动联动其所有子渠道
  const handleRowSelectionChange = (nextSelectedRowKeys, nextSelectedRows) => {
    // 未开启批量操作时不处理，避免无意义的状态更新
    if (!enableBatchDelete) {
      return;
    }

    // 普通模式：直接采用表格返回的选中结果
    if (!enableTagMode) {
      setSelectedRowKeys(nextSelectedRowKeys || []);
      setSelectedChannels(nextSelectedRows || []);
      return;
    }

    const safeNextKeys = Array.isArray(nextSelectedRowKeys)
      ? nextSelectedRowKeys
      : [];
    const prevKeys = Array.isArray(selectedRowKeys) ? selectedRowKeys : [];

    // 构建 key -> 记录 的映射，方便后续展开/收缩标签行
    const keyRecordMap = new Map();
    const traverse = (items) => {
      if (!Array.isArray(items)) return;
      items.forEach((item) => {
        if (!item) return;
        if (item.key !== undefined && item.key !== null) {
          keyRecordMap.set(item.key, item);
        }
        if (Array.isArray(item.children) && item.children.length > 0) {
          traverse(item.children);
        }
      });
    };
    traverse(channels);

    const prevKeySet = new Set(prevKeys);
    const nextKeySet = new Set(safeNextKeys);
    const finalKeySet = new Set(safeNextKeys);

    const addedKeys = [];
    const removedKeys = [];

    safeNextKeys.forEach((key) => {
      if (!prevKeySet.has(key)) {
        addedKeys.push(key);
      }
    });

    prevKeys.forEach((key) => {
      if (!nextKeySet.has(key)) {
        removedKeys.push(key);
      }
    });

    // 选中标签汇总行时，自动选中其所有子渠道
    addedKeys.forEach((key) => {
      const record = keyRecordMap.get(key);
      if (
        record &&
        Array.isArray(record.children) &&
        record.children.length > 0
      ) {
        record.children.forEach((child) => {
          if (!child) return;
          const childKey =
            child.key !== undefined && child.key !== null
              ? child.key
              : String(child.id);
          finalKeySet.add(childKey);
        });
      }
    });

    // 取消选择标签汇总行时，自动取消其所有子渠道
    removedKeys.forEach((key) => {
      const record = keyRecordMap.get(key);
      if (
        record &&
        Array.isArray(record.children) &&
        record.children.length > 0
      ) {
        record.children.forEach((child) => {
          if (!child) return;
          const childKey =
            child.key !== undefined && child.key !== null
              ? child.key
              : String(child.id);
          finalKeySet.delete(childKey);
        });
      }
    });

    const finalKeys = Array.from(finalKeySet);
    const finalRows = finalKeys
      .map((key) => keyRecordMap.get(key))
      .filter((item) => item !== undefined);

    setSelectedRowKeys(finalKeys);
    setSelectedChannels(finalRows);
  };

  // Row style
  const handleRow = (record, index) => {
    if (record.status !== 1) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)',
        },
      };
    } else {
      return {};
    }
  };

  // 根据选中的行生成后端需要的渠道 ID 列表
  // - 普通模式：直接使用选中渠道的 id
  // - 标签聚合模式：如果选中的是「标签汇总行」（带 children），则展开为其子渠道 id
  //   同时自动去重，并过滤掉非数字 id（例如标签行自身的字符串 id）
  const getSelectedChannelIds = () => {
    const idSet = new Set();

    selectedChannels.forEach((channel) => {
      if (
        enableTagMode &&
        channel &&
        Array.isArray(channel.children) &&
        channel.children.length > 0
      ) {
        channel.children.forEach((child) => {
          if (child && Number.isInteger(child.id)) {
            idSet.add(child.id);
          }
        });
      } else if (channel && Number.isInteger(channel.id)) {
        idSet.add(channel.id);
      }
    });

    return Array.from(idSet);
  };

  // Batch operations
  const batchSetChannelTag = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要设置标签的渠道！'));
      return;
    }
    if (batchSetTagValue === '') {
      showError(t('标签不能为空！'));
      return;
    }
    const ids = getSelectedChannelIds();
    if (ids.length === 0) {
      showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
      return;
    }
    const res = await API.post('/api/channel/batch/tag', {
      ids,
      tag: batchSetTagValue === '' ? null : batchSetTagValue,
    });
    if (res.data.success) {
      showSuccess(
        t('已为 ${count} 个渠道设置标签！').replace('${count}', res.data.data),
      );
      await refresh();
      setShowBatchSetTag(false);
    } else {
      showError(res.data.message);
    }
  };

  const batchResetChannelUsedQuota = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要重置的渠道！'));
      return;
    }
    setLoading(true);
    try {
      const ids = getSelectedChannelIds();
      if (ids.length === 0) {
        showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
        setLoading(false);
        return;
      }
      const res = await API.post('/api/channel/batch/reset_used_quota', {
        ids,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('批量重置已用额度失败'));
      } else {
        showSuccess(
          t('已重置 ${count} 个渠道的已用额度！').replace(
            '${count}',
            data ?? ids.length,
          ),
        );
        await refresh();
      }
    } catch (error) {
      showError(
        t('批量重置已用额度失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
    setLoading(false);
  };

  const batchSetChannelGroup = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要设置分组的渠道！'));
      return;
    }
    if (!batchSetGroupValue || batchSetGroupValue.length === 0) {
      showError(t('分组名称不能为空！'));
      return;
    }
    setLoading(true);
    try {
      const ids = getSelectedChannelIds();
      if (ids.length === 0) {
        showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
        setLoading(false);
        return;
      }
      const res = await API.post('/api/channel/batch/group', {
        ids,
        groups: batchSetGroupValue,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('批量设置分组失败'));
      } else {
        showSuccess(
          t('已为 ${count} 个渠道设置分组！').replace(
            '${count}',
            data ?? ids.length,
          ),
        );
        await refresh();
        setShowBatchSetGroup(false);
      }
    } catch (error) {
      showError(
        t('批量设置分组失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
    setLoading(false);
  };

  const parseModelsInput = (raw) => {
    if (!raw) return [];
    return raw
      .split(/[\n,]/)
      .map((item) => String(item).trim())
      .filter((item) => item.length > 0);
  };

  const parseUserIdsInput = (raw) => {
    if (!raw) return [];
    const parts = String(raw)
      .split(/[\s,]+/)
      .map((item) => item.trim())
      .filter(Boolean);
    const ids = [];
    const seen = new Set();
    parts.forEach((p) => {
      const id = parseInt(p, 10);
      if (!Number.isFinite(id) || id <= 0) return;
      if (seen.has(id)) return;
      seen.add(id);
      ids.push(id);
    });
    return ids;
  };

  const batchUpdateChannelModels = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要调整模型的渠道！'));
      return;
    }
    const addModels = parseModelsInput(batchAddModelsValue);
    const removeModels = parseModelsInput(batchRemoveModelsValue);
    if (addModels.length === 0 && removeModels.length === 0) {
      showError(t('请至少输入要添加或移除的模型！'));
      return;
    }
    setLoading(true);
    try {
      const ids = getSelectedChannelIds();
      if (ids.length === 0) {
        showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
        setLoading(false);
        return;
      }
      const res = await API.post('/api/channel/batch/models', {
        ids,
        add_models: addModels,
        remove_models: removeModels,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('批量调整模型失败'));
      } else {
        showSuccess(
          t('已更新 ${count} 个渠道的模型列表！').replace(
            '${count}',
            data ?? ids.length,
          ),
        );
        await refresh();
        setShowBatchUpdateModels(false);
      }
    } catch (error) {
      showError(
        t('批量调整模型失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
    setLoading(false);
  };

  const batchBindChannelUsers = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要绑定用户的通道！'));
      return;
    }
    const ids = getSelectedChannelIds();
    if (ids.length === 0) {
      showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
      return;
    }

    const raw = String(batchBindUsersValue || '').trim();
    const userIds = raw === '' ? [] : parseUserIdsInput(raw);
    if (raw !== '' && userIds.length === 0) {
      showError(t('用户ID格式错误，请检查输入内容！'));
      return;
    }

    setLoading(true);
    try {
      const res = await API.post('/api/channel/batch/bind_users', {
        ids,
        user_ids: userIds,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('批量绑定用户失败'));
      } else {
        showSuccess(
          t('已更新 ${count} 个渠道的绑定用户！').replace(
            '${count}',
            data ?? ids.length,
          ),
        );
        await refresh();
        setShowBatchBindUsers(false);
        setBatchBindUsersValue('');
      }
    } catch (error) {
      showError(
        t('批量绑定用户失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
    setLoading(false);
  };

  const batchDeleteChannels = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要删除的通道！'));
      return;
    }
    setLoading(true);
    const ids = getSelectedChannelIds();
    if (ids.length === 0) {
      showError(t('未找到有效的渠道 ID，请检查选择的渠道！'));
      setLoading(false);
      return;
    }
    const res = await API.post(`/api/channel/batch`, { ids });
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(t('已删除 ${data} 个通道！').replace('${data}', data));
      await refresh();
      setTimeout(() => {
        if (channels.length === 0 && activePage > 1) {
          refresh(activePage - 1);
        }
      }, 100);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Channel operations
  const testAllChannels = async () => {
    const res = await API.get(`/api/channel/test`);
    const { success, message } = res.data;
    if (success) {
      showInfo(t('已成功开始测试所有已启用通道，请刷新页面查看结果。'));
    } else {
      showError(message);
    }
  };

  const deleteAllDisabledChannels = async () => {
    const res = await API.delete(`/api/channel/disabled`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(
        t('已删除所有禁用渠道，共计 ${data} 个').replace('${data}', data),
      );
      await refresh();
    } else {
      showError(message);
    }
  };

  const updateAllChannelsBalance = async () => {
    const res = await API.get(`/api/channel/update_balance`);
    const { success, message } = res.data;
    if (success) {
      showInfo(t('已更新完毕所有已启用通道余额！'));
    } else {
      showError(message);
    }
  };

  const updateChannelBalance = async (record) => {
    const res = await API.get(`/api/channel/update_balance/${record.id}/`);
    const { success, message, balance } = res.data;
    if (success) {
      updateChannelProperty(record.id, (channel) => {
        channel.balance = balance;
        channel.balance_updated_time = Date.now() / 1000;
      });
      showInfo(
        t('通道 ${name} 余额更新成功！').replace('${name}', record.name),
      );
    } else {
      showError(message);
    }
  };

  const resetChannelUsedQuota = async (record) => {
    try {
      const res = await API.post(
        `/api/channel/reset_used_quota/${record.id}`,
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      updateChannelProperty(record.id, (channel) => {
        channel.used_quota = data?.used_quota ?? 0;
        channel.cost_used_quota = data?.cost_used_quota ?? 0;
        channel.request_success_count = data?.request_success_count ?? 0;
      });

      if (enableTagMode) {
        await refresh();
      }

      showSuccess(
        t('渠道 ${name} 的已用额度已重置！').replace('${name}', record.name),
      );
    } catch (error) {
      console.error(error);
      showError(t('重置渠道已用额度失败'));
    }
  };

  const fixChannelsAbilities = async () => {
    const res = await API.post(`/api/channel/fix`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(
        t('已修复 ${success} 个通道，失败 ${fails} 个通道。')
          .replace('${success}', data.success)
          .replace('${fails}', data.fails),
      );
      await refresh();
    } else {
      showError(message);
    }
  };

  // Test channel
  const testChannel = async (record, model) => {
    setTestQueue((prev) => [...prev, { channel: record, model }]);
    if (!isProcessingQueue) {
      setIsProcessingQueue(true);
    }
  };

  // Process test queue
  const processTestQueue = async () => {
    if (!isProcessingQueue || testQueue.length === 0) return;

    const { channel, model, indexInFiltered } = testQueue[0];

    if (currentTestChannel && currentTestChannel.id === channel.id) {
      let pageNo;
      if (indexInFiltered !== undefined) {
        pageNo = Math.floor(indexInFiltered / MODEL_TABLE_PAGE_SIZE) + 1;
      } else {
        const filteredModelsList = currentTestChannel.models
          .split(',')
          .filter((m) =>
            m.toLowerCase().includes(modelSearchKeyword.toLowerCase()),
          );
        const modelIdx = filteredModelsList.indexOf(model);
        pageNo =
          modelIdx !== -1
            ? Math.floor(modelIdx / MODEL_TABLE_PAGE_SIZE) + 1
            : 1;
      }
      setModelTablePage(pageNo);
    }

    try {
      setTestingModels((prev) => new Set([...prev, model]));
      const res = await API.get(
        `/api/channel/test/${channel.id}?model=${model}`,
      );
      const { success, message, time } = res.data;

      setModelTestResults((prev) => ({
        ...prev,
        [`${channel.id}-${model}`]: { success, time },
      }));

      if (success) {
        updateChannelProperty(channel.id, (ch) => {
          ch.response_time = time * 1000;
          ch.test_time = Date.now() / 1000;
        });
        if (!model) {
          showInfo(
            t('通道 ${name} 测试成功，耗时 ${time.toFixed(2)} 秒。')
              .replace('${name}', channel.name)
              .replace('${time.toFixed(2)}', time.toFixed(2)),
          );
        }
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setTestingModels((prev) => {
        const newSet = new Set(prev);
        newSet.delete(model);
        return newSet;
      });
    }

    setTestQueue((prev) => prev.slice(1));
  };

  // Monitor queue changes
  useEffect(() => {
    if (testQueue.length > 0 && isProcessingQueue) {
      processTestQueue();
    } else if (testQueue.length === 0 && isProcessingQueue) {
      setIsProcessingQueue(false);
      setIsBatchTesting(false);
    }
  }, [testQueue, isProcessingQueue]);

  // Batch test models
  const batchTestModels = async () => {
    if (!currentTestChannel) return;

    setIsBatchTesting(true);
    setModelTablePage(1);

    const filteredModels = currentTestChannel.models
      .split(',')
      .filter((model) =>
        model.toLowerCase().includes(modelSearchKeyword.toLowerCase()),
      );

    setTestQueue(
      filteredModels.map((model, idx) => ({
        channel: currentTestChannel,
        model,
        indexInFiltered: idx,
      })),
    );
    setIsProcessingQueue(true);
  };

  // Handle close modal
  const handleCloseModal = () => {
    if (isBatchTesting) {
      setTestQueue([]);
      setIsProcessingQueue(false);
      setIsBatchTesting(false);
      showSuccess(t('已停止测试'));
    } else {
      setShowModelTestModal(false);
      setModelSearchKeyword('');
      setSelectedModelKeys([]);
      setModelTablePage(1);
    }
  };

  // Type counts
  const channelTypeCounts = useMemo(() => {
    if (Object.keys(typeCounts).length > 0) return typeCounts;
    const counts = { all: channels.length };
    channels.forEach((channel) => {
      const collect = (ch) => {
        const type = ch.type;
        counts[type] = (counts[type] || 0) + 1;
      };
      if (channel.children !== undefined) {
        channel.children.forEach(collect);
      } else {
        collect(channel);
      }
    });
    return counts;
  }, [typeCounts, channels]);

  const availableTypeKeys = useMemo(() => {
    const keys = ['all'];
    Object.entries(channelTypeCounts).forEach(([k, v]) => {
      if (k !== 'all' && v > 0) keys.push(String(k));
    });
    return keys;
  }, [channelTypeCounts]);

  return {
    // Basic states
    channels,
    loading,
    searching,
    activePage,
    pageSize,
    channelCount,
    groupOptions,
    idSort,
    enableTagMode,
    enableBatchDelete,
    statusFilter,
    compactMode,

    // UI states
    showEdit,
    setShowEdit,
    editingChannel,
    setEditingChannel,
    showEditTag,
    setShowEditTag,
    editingTag,
    setEditingTag,
    selectedChannels,
    setSelectedChannels,
    selectedRowKeys,
    showBatchSetTag,
    setShowBatchSetTag,
    batchSetTagValue,
    setBatchSetTagValue,
    showBatchSetGroup,
    setShowBatchSetGroup,
    batchSetGroupValue,
    setBatchSetGroupValue,
    showBatchUpdateModels,
    setShowBatchUpdateModels,
    batchAddModelsValue,
    setBatchAddModelsValue,
    batchRemoveModelsValue,
    setBatchRemoveModelsValue,
    showBatchBindUsers,
    setShowBatchBindUsers,
    batchBindUsersValue,
    setBatchBindUsersValue,

    // Channel abnormal consume collection
    channelAbnormalConsumeEnabledMap,
    showChannelAbnormalConsumeModal,
    setShowChannelAbnormalConsumeModal,
    currentAbnormalConsumeChannel,
    setCurrentAbnormalConsumeChannel,
    channelAbnormalConsumeRecords,
    channelAbnormalConsumeLoading,

    // Channel profit stats
    showChannelProfitStatsModal,
    setShowChannelProfitStatsModal,
    currentProfitStatsChannel,
    setCurrentProfitStatsChannel,

    // Column states
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    COLUMN_KEYS,

    // Type tab states
    activeTypeKey,
    setActiveTypeKey,
    typeCounts,
    channelTypeCounts,
    availableTypeKeys,

    // Model test states
    showModelTestModal,
    setShowModelTestModal,
    currentTestChannel,
    setCurrentTestChannel,
    modelSearchKeyword,
    setModelSearchKeyword,
    modelTestResults,
    testingModels,
    selectedModelKeys,
    setSelectedModelKeys,
    isBatchTesting,
    modelTablePage,
    setModelTablePage,
    allSelectingRef,

    // Multi-key management states
    showMultiKeyManageModal,
    setShowMultiKeyManageModal,
    currentMultiKeyChannel,
    setCurrentMultiKeyChannel,

    // Form
    formApi,
    setFormApi,
    formInitValues,

    // Helpers
    t,
    isMobile,
    ...upstreamUpdates,

    // Functions
    loadChannels,
    searchChannels,
    refresh,
    manageChannel,
    manageTag,
    handlePageChange,
    handlePageSizeChange,
    copySelectedChannel,
    updateChannelProperty,
    submitTagEdit,
    closeEdit,
    handleRow,
    batchSetChannelTag,
    batchDeleteChannels,
    batchResetChannelUsedQuota,
    batchSetChannelGroup,
    batchUpdateChannelModels,
    batchBindChannelUsers,
    testAllChannels,
    deleteAllDisabledChannels,
    updateAllChannelsBalance,
    updateChannelBalance,
    resetChannelUsedQuota,
    fixChannelsAbilities,
    testChannel,
    batchTestModels,
    handleCloseModal,
    getFormValues,
    fetchChannelAbnormalConsumeConfig,
    updateChannelAbnormalConsumeEnabled,
    openChannelAbnormalConsumeModal,
    closeChannelAbnormalConsumeModal,
    loadChannelAbnormalConsumeRecords,
    clearChannelAbnormalConsumeRecords,

    // Column functions
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    getDefaultColumnVisibility,

    // Setters
    setIdSort,
    setEnableTagMode,
    setEnableBatchDelete,
    setStatusFilter,
    setCompactMode,
    setActivePage,
    handleRowSelectionChange,
  };
};
