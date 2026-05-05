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

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Typography } from '@douyinfe/semi-ui';
import {
  API,
  getTodayStartTimestamp,
  isEffectiveAdmin,
  showError,
  showSuccess,
  timestamp2string,
  renderQuota,
  renderNumber,
  getLogOther,
  copy,
  renderClaudeLogContent,
  renderLogContent,
  renderAudioModelPrice,
  renderClaudeModelPrice,
  renderModelPrice,
  formatBytesWithExact,
  normalizeManageLogContent,
} from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useLogsData = () => {
  const { t } = useTranslation();

  // User and admin
  const isAdminUser = isEffectiveAdmin();

  // Define column keys for selection
  const COLUMN_KEYS = {
    TIME: 'time',
    CHANNEL: 'channel',
    USERNAME: 'username',
    PROMPT_CACHE_KEY: 'prompt_cache_key',
    SESSION_ID: 'session_id',
    CONVERSATION_ID: 'conversation_id',
    TOKEN: 'token',
    GROUP: 'group',
    QUOTA_BUCKET: 'quota_bucket',
    TYPE: 'type',
    MODEL: 'model',
    USE_TIME: 'use_time',
    PROMPT: 'prompt',
    COMPLETION: 'completion',
    COST: 'cost',
    RETRY: 'retry',
    IP: 'ip',
  };

  // Basic state
  const [logs, setLogs] = useState([]);
  const [expandData, setExpandData] = useState({});
  const [showStat, setShowStat] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingStat, setLoadingStat] = useState(false);
  const [groupLabelById, setGroupLabelById] = useState({});
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(0);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [logType, setLogType] = useState(isAdminUser ? 0 : 2);
  // Role-specific storage key to prevent different roles from overwriting each other
  const STORAGE_KEY = isAdminUser
    ? 'logs-table-columns-admin'
    : 'logs-table-columns-user';

  // Statistics state
  const [stat, setStat] = useState({
    quota: 0,
    token: 0,
  });

  // Form state
  const [formApi, setFormApi] = useState(null);
  let now = new Date();
  const formInitValues = {
    username: '',
    token_name: '',
    model_name: '',
    request_id: '',
    channel: '',
    group_id: '',
    start_timestamp: timestamp2string(getTodayStartTimestamp()),
    end_timestamp: timestamp2string(now.getTime() / 1000 + 3600),
    logType: isAdminUser ? '0' : '2',
  };

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [storedCompactMode, setStoredCompactMode] = useTableCompactMode('logs');
  const compactMode = isAdminUser ? storedCompactMode : true;
  const setCompactMode = isAdminUser ? setStoredCompactMode : () => {};

  // User info modal state
  const [showUserInfo, setShowUserInfoModal] = useState(false);
  const [userInfoData, setUserInfoData] = useState(null);

  useEffect(() => {
    (async () => {
      try {
        const res = await API.get('/api/group/resolve');
        const { success, message, data } = res.data || {};
        if (!success) {
          showError(message || t('获取分组失败'));
          return;
        }
        const map = {};
        (Array.isArray(data) ? data : []).forEach((g) => {
          const idRaw = Number(g?.id ?? 0);
          const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
          if (id <= 0) return;
          const label = String(g?.display_name || g?.code || '').trim();
          if (!label) return;
          map[id] = label;
        });
        setGroupLabelById(map);
      } catch (e) {
        showError(e?.message || t('获取分组失败'));
      }
    })();
  }, [t]);

  // Load saved column preferences from localStorage
  useEffect(() => {
    const savedColumns = localStorage.getItem(STORAGE_KEY);
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults };
        Object.keys(defaults).forEach((key) => {
          if (Object.prototype.hasOwnProperty.call(parsed, key)) {
            merged[key] = !!parsed[key];
          }
        });

        // For non-admin users, force-hide admin-only columns (does not touch admin settings)
        if (!isAdminUser) {
          merged[COLUMN_KEYS.CHANNEL] = false;
          merged[COLUMN_KEYS.USERNAME] = false;
          merged[COLUMN_KEYS.RETRY] = false;
        }
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Get default column visibility based on user role
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.TIME]: true,
      [COLUMN_KEYS.CHANNEL]: isAdminUser,
      [COLUMN_KEYS.USERNAME]: isAdminUser,
      [COLUMN_KEYS.PROMPT_CACHE_KEY]: isAdminUser,
      [COLUMN_KEYS.SESSION_ID]: isAdminUser,
      [COLUMN_KEYS.CONVERSATION_ID]: isAdminUser,
      [COLUMN_KEYS.TOKEN]: true,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.QUOTA_BUCKET]: isAdminUser,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.MODEL]: true,
      [COLUMN_KEYS.USE_TIME]: true,
      [COLUMN_KEYS.PROMPT]: true,
      [COLUMN_KEYS.COMPLETION]: true,
      [COLUMN_KEYS.COST]: true,
      [COLUMN_KEYS.RETRY]: isAdminUser,
      [COLUMN_KEYS.IP]: true,
    };
  };

  // Initialize default column visibility
  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
  };

  // Handle column visibility change
  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  // Handle "Select All" checkbox
  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};

    allKeys.forEach((key) => {
      if (
        (key === COLUMN_KEYS.CHANNEL ||
          key === COLUMN_KEYS.USERNAME ||
          key === COLUMN_KEYS.RETRY) &&
        !isAdminUser
      ) {
        updatedColumns[key] = false;
      } else {
        updatedColumns[key] = checked;
      }
    });

    setVisibleColumns(updatedColumns);
  };

  // Persist column settings to the role-specific STORAGE_KEY
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(visibleColumns));
    }
  }, [visibleColumns]);

  // 获取表单值的辅助函数，确保所有值都是字符串
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};

    const start_timestamp =
      formValues.start_timestamp || timestamp2string(getTodayStartTimestamp());
    const end_timestamp =
      formValues.end_timestamp || timestamp2string(now.getTime() / 1000 + 3600);

    return {
      username: formValues.username || '',
      token_name: formValues.token_name || '',
      model_name: formValues.model_name || '',
      request_id: formValues.request_id || '',
      start_timestamp,
      end_timestamp,
      channel: formValues.channel || '',
      group_id: formValues.group_id || '',
      logType: isAdminUser
        ? formValues.logType
          ? parseInt(formValues.logType)
          : 0
        : 2,
    };
  };

  // Statistics functions
  const getLogSelfStat = async () => {
    const {
      token_name,
      model_name,
      start_timestamp,
      end_timestamp,
      group_id,
      logType: formLogType,
    } = getFormValues();
    const currentLogType = formLogType !== undefined ? formLogType : logType;
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let url = `/api/log/self/stat?type=${currentLogType}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&group_id=${group_id}`;
    url = encodeURI(url);
    let res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const getLogStat = async () => {
    const {
      username,
      token_name,
      model_name,
      request_id,
      start_timestamp,
      end_timestamp,
      channel,
      group_id,
      logType: formLogType,
    } = getFormValues();
    const currentLogType = formLogType !== undefined ? formLogType : logType;
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let url = `/api/log/stat?type=${currentLogType}&username=${username}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&channel=${channel}&group_id=${group_id}`;
    url = encodeURI(url);
    let res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const handleEyeClick = async () => {
    if (loadingStat) {
      return;
    }
    setLoadingStat(true);
    if (isAdminUser) {
      await getLogStat();
    } else {
      await getLogSelfStat();
    }
    setShowStat(true);
    setLoadingStat(false);
  };

  // User info function
  const showUserInfoFunc = async (userId) => {
    if (!isAdminUser) {
      return;
    }
    const res = await API.get(`/api/user/${userId}`);
    const { success, message, data } = res.data;
    if (success) {
      setUserInfoData(data);
      setShowUserInfoModal(true);
    } else {
      showError(message);
    }
  };

  // Format logs data
  const setLogsFormat = (logs) => {
    const parsedOtherList = logs.map((log) => getLogOther(log.other));

    const formatHeaderSnapshot = (headers) => {
      if (!headers || typeof headers !== 'object') return '';
      const keys = Object.keys(headers).sort((a, b) => a.localeCompare(b));
      const lines = [];
      keys.forEach((key) => {
        const values = headers[key];
        const arr = Array.isArray(values) ? values : [values];
        arr.forEach((value) => {
          if (value === undefined || value === null) return;
          const str = String(value).trim();
          if (!str) return;
          lines.push(`${key}: ${str}`);
        });
      });
      return lines.join('\n');
    };

    let expandDatesLocal = {};

    const buildLogContentText = (logRecord) => {
      if (!logRecord) return '';
      const text =
        typeof logRecord.content === 'string'
          ? logRecord.content.trim()
          : String(logRecord.content || '').trim();
      if (!text) return '';
      const type = Number(logRecord.type) || 0;
      return type === 3 ? normalizeManageLogContent(text, t) : text;
    };

    for (let i = 0; i < logs.length; i++) {
      logs[i].timestamp2string = timestamp2string(logs[i].created_at);
      logs[i].key = logs[i].id;
      let other = parsedOtherList[i] || null;

      const promptCacheKey =
        typeof other?.prompt_cache_key === 'string'
          ? other.prompt_cache_key.trim()
          : '';
      const sessionId =
        typeof other?.session_id === 'string' ? other.session_id.trim() : '';
      const conversationId =
        typeof other?.conversation_id === 'string'
          ? other.conversation_id.trim()
          : '';
      const requestPath =
        typeof other?.request_path === 'string'
          ? other.request_path.trim()
          : '';
      const requestMethod =
        typeof other?.request_method === 'string'
          ? other.request_method.trim()
          : '';

      logs[i].prompt_cache_key = promptCacheKey;
      logs[i].session_id = sessionId;
      logs[i].conversation_id = conversationId;
      logs[i].request_path = requestPath;
      logs[i].request_method = requestMethod;
      logs[i].quota_bucket =
        typeof other?.quota_bucket === 'string'
          ? other.quota_bucket.trim()
          : '';
      const requestIdFromRecord =
        typeof logs[i].request_id === 'string'
          ? logs[i].request_id.trim()
          : String(logs[i].request_id || '').trim();
      const requestIdFromOther =
        typeof other?.request_id === 'string' ? other.request_id.trim() : '';
      logs[i].request_id = requestIdFromOther || requestIdFromRecord;
      logs[i].request_ua =
        typeof other?.request_ua === 'string' ? other.request_ua.trim() : '';
      let expandDataLocal = [];

      if (!isAdminUser) {
        expandDatesLocal[logs[i].key] = expandDataLocal;
        continue;
      }

      if (Number(logs[i].channel) > 0) {
        expandDataLocal.push({
          key: t('渠道信息'),
          value: `${logs[i].channel} - ${logs[i].channel_name || '[未知]'}`,
        });
      }
      const endpoint = [requestMethod, requestPath].filter(Boolean).join(' ');
      if (endpoint) {
        expandDataLocal.push({
          key: t('接口'),
          value: endpoint,
        });
      }
      if (logs[i].request_id) {
        expandDataLocal.push({
          key: 'request_id',
          value: logs[i].request_id,
        });
      }
      if (logs[i].request_ua) {
        expandDataLocal.push({
          key: t('请求UA'),
          value: logs[i].request_ua,
        });
      }

      const logContentText =
        logs[i].type !== 2 ? buildLogContentText(logs[i]) : '';
      if (logContentText) {
        expandDataLocal.push({
          key: t('日志内容'),
          value: (
            <Typography.Text
              type='tertiary'
              style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}
            >
              {logContentText}
            </Typography.Text>
          ),
        });
      }
      const requestContentLength = Number(
        other?.admin_info?.request_content_length || 0,
      );
      if (Number.isFinite(requestContentLength) && requestContentLength > 0) {
        expandDataLocal.push({
          key: t('请求体大小'),
          value: formatBytesWithExact(requestContentLength),
        });
      }
      const headerText = formatHeaderSnapshot(
        other?.admin_info?.request_headers,
      );
      if (headerText) {
        expandDataLocal.push({
          key: t('请求Headers'),
          value: (
            <Typography.Text
              type='tertiary'
              style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}
            >
              {headerText}
            </Typography.Text>
          ),
        });
      }
      if (other?.stream_exit_reason) {
        expandDataLocal.push({
          key: 'stream_exit_reason',
          value: String(other.stream_exit_reason),
        });
      }
      if (other?.stream_exit_error) {
        expandDataLocal.push({
          key: 'stream_exit_error',
          value: String(other.stream_exit_error),
        });
      }
      if (other?.ws || other?.audio) {
        expandDataLocal.push({
          key: t('语音输入'),
          value: other.audio_input,
        });
        expandDataLocal.push({
          key: t('语音输出'),
          value: other.audio_output,
        });
        expandDataLocal.push({
          key: t('文字输入'),
          value: other.text_input,
        });
        expandDataLocal.push({
          key: t('文字输出'),
          value: other.text_output,
        });
      }
      if (other?.cache_tokens > 0) {
        expandDataLocal.push({
          key: t('缓存 Tokens'),
          value: other.cache_tokens,
        });
      }
      if (other?.cache_creation_tokens > 0) {
        expandDataLocal.push({
          key: t('缓存创建 Tokens'),
          value: other.cache_creation_tokens,
        });
      }
      if (other?.cache_creation_tokens_5m > 0) {
        expandDataLocal.push({
          key: t('5m 缓存创建 Tokens'),
          value: other.cache_creation_tokens_5m,
        });
      }
      if (other?.cache_creation_tokens_1h > 0) {
        expandDataLocal.push({
          key: t('1h 缓存创建 Tokens'),
          value: other.cache_creation_tokens_1h,
        });
      }
      if (logs[i].type === 2) {
        expandDataLocal.push({
          key: t('日志详情'),
          value: other?.claude
            ? renderClaudeLogContent(
                other?.model_ratio,
                other.completion_ratio,
                other.model_price,
                other.group_ratio,
                other?.user_group_ratio,
                other?.group_ratio_source,
                other.cache_ratio || 1.0,
                other.cache_creation_ratio || 1.0,
                other.cache_creation_tokens_5m || 0,
                other.cache_creation_ratio_5m ||
                  other.cache_creation_ratio ||
                  1.0,
                other.cache_creation_tokens_1h || 0,
                other.cache_creation_ratio_1h ||
                  other.cache_creation_ratio ||
                  1.0,
              )
            : renderLogContent(
                logs[i].model_name,
                logs[i].prompt_tokens,
                other.cache_tokens || 0,
                other?.model_ratio,
                other.completion_ratio,
                other.model_price,
                other.group_ratio,
                other?.user_group_ratio,
                other?.group_ratio_source,
                other.cache_ratio || 1.0,
                false,
                1.0,
                other.web_search || false,
                other.web_search_call_count || 0,
                other.file_search || false,
                other.file_search_call_count || 0,
                other?.base_multiplier || 1,
                other?.base_multiplier_applied,
              ),
        });
      }
      if (logs[i].type === 2) {
        let modelMapped =
          other?.is_model_mapped &&
          other?.upstream_model_name &&
          other?.upstream_model_name !== '';
        if (modelMapped) {
          expandDataLocal.push({
            key: t('请求并计费模型'),
            value: logs[i].model_name,
          });
          expandDataLocal.push({
            key: t('实际模型'),
            value: other.upstream_model_name,
          });
        }
        let content = '';
        if (other?.ws || other?.audio) {
          content = renderAudioModelPrice(
            other?.text_input,
            other?.text_output,
            other?.model_ratio,
            other?.model_price,
            other?.completion_ratio,
            other?.audio_input,
            other?.audio_output,
            other?.audio_ratio,
            other?.audio_completion_ratio,
            other?.group_ratio,
            other?.user_group_ratio,
            other?.group_ratio_source,
            other?.cache_tokens || 0,
            other?.cache_ratio || 1.0,
            other?.base_multiplier || 1,
            other?.base_multiplier_applied,
          );
        } else if (other?.claude) {
          content = renderClaudeModelPrice(
            logs[i].prompt_tokens,
            logs[i].completion_tokens,
            other.model_ratio,
            other.model_price,
            other.completion_ratio,
            other.group_ratio,
            other?.user_group_ratio,
            other?.group_ratio_source,
            other.cache_tokens || 0,
            other.cache_ratio || 1.0,
            other.cache_creation_tokens || 0,
            other.cache_creation_ratio || 1.0,
            other?.base_multiplier || 1,
            other?.base_multiplier_applied,
            other.cache_creation_tokens_5m || 0,
            other.cache_creation_ratio_5m || other.cache_creation_ratio || 1.0,
            other.cache_creation_tokens_1h || 0,
            other.cache_creation_ratio_1h || other.cache_creation_ratio || 1.0,
            other?.service_tier || '',
            other?.service_tier_multiplier || 1,
          );
        } else {
          content = renderModelPrice(
            logs[i].model_name,
            logs[i].prompt_tokens,
            logs[i].completion_tokens,
            other?.model_ratio,
            other?.model_price,
            other?.completion_ratio,
            other?.group_ratio,
            other?.user_group_ratio,
            other?.group_ratio_source,
            other?.cache_tokens || 0,
            other?.cache_ratio || 1.0,
            other?.image || false,
            other?.image_ratio || 0,
            other?.image_output || 0,
            other?.web_search || false,
            other?.web_search_call_count || 0,
            other?.web_search_price || 0,
            other?.file_search || false,
            other?.file_search_call_count || 0,
            other?.file_search_price || 0,
            other?.audio_input_seperate_price || false,
            other?.audio_input_token_count || 0,
            other?.audio_input_price || 0,
            other?.image_generation_call || false,
            other?.image_generation_call_price || 0,
            other?.base_multiplier || 1,
            other?.base_multiplier_applied,
            other?.service_tier || '',
            other?.service_tier_multiplier || 1,
          );
        }
        expandDataLocal.push({
          key: t('计费过程'),
          value: content,
        });
        if (other?.service_tier) {
          const multiplier = Number(other?.service_tier_multiplier);
          const suffix =
            Number.isFinite(multiplier) && multiplier > 0 && multiplier !== 1
              ? ` (x${multiplier})`
              : '';
          expandDataLocal.push({
            key: t('Service Tier'),
            value: `${other.service_tier}${suffix}`,
          });
        }
        if (other?.reasoning_effort) {
          expandDataLocal.push({
            key: t('Reasoning Effort'),
            value: other.reasoning_effort,
          });
        }
      }
      expandDatesLocal[logs[i].key] = expandDataLocal;
    }

    setExpandData(expandDatesLocal);
    setLogs(logs);
  };

  // Load logs function
  const loadLogs = async (startIdx, pageSize, customLogType = null) => {
    setLoading(true);

    let url = '';
    const {
      username,
      token_name,
      model_name,
      request_id,
      start_timestamp,
      end_timestamp,
      channel,
      group_id,
      logType: formLogType,
    } = getFormValues();

    const currentLogType =
      customLogType !== null
        ? customLogType
        : formLogType !== undefined
          ? formLogType
          : logType;

    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    if (isAdminUser) {
      url = `/api/log/?p=${startIdx}&page_size=${pageSize}&type=${currentLogType}&username=${username}&token_name=${token_name}&model_name=${model_name}&request_id=${request_id}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&channel=${channel}&group_id=${group_id}`;
    } else {
      url = `/api/log/self/?p=${startIdx}&page_size=${pageSize}&type=${currentLogType}&token_name=${token_name}&model_name=${model_name}&request_id=${request_id}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&group_id=${group_id}`;
    }
    url = encodeURI(url);
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      const newPageData = data.items;
      setActivePage(data.page);
      setPageSize(data.page_size);
      setLogCount(data.total);

      setLogsFormat(newPageData);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Page handlers
  const handlePageChange = (page) => {
    setActivePage(page);
    loadLogs(page, pageSize).then((r) => {});
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    loadLogs(activePage, size)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  // Refresh function
  const refresh = async () => {
    setActivePage(1);
    handleEyeClick();
    await loadLogs(1, pageSize);
  };

  // Copy text function
  const copyText = async (e, text) => {
    e.stopPropagation();
    if (await copy(text)) {
      showSuccess('已复制：' + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // Initialize data
  useEffect(() => {
    const localPageSize =
      parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(activePage, localPageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, []);

  // Initialize statistics when formApi is available
  useEffect(() => {
    if (formApi) {
      handleEyeClick();
    }
  }, [formApi]);

  // Check if any record has expandable content
  const hasExpandableRows = () => {
    if (!isAdminUser) {
      return false;
    }
    return logs.length > 0;
  };

  return {
    // Basic state
    logs,
    expandData,
    showStat,
    loading,
    loadingStat,
    groupLabelById,
    activePage,
    logCount,
    pageSize,
    logType,
    stat,
    isAdminUser,

    // Form state
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // Column visibility
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    COLUMN_KEYS,

    // Compact mode
    compactMode,
    setCompactMode,

    // User info modal
    showUserInfo,
    setShowUserInfoModal,
    userInfoData,
    showUserInfoFunc,

    // Functions
    loadLogs,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    copyText,
    handleEyeClick,
    setLogsFormat,
    hasExpandableRows,
    setLogType,

    // Translation
    t,
  };
};
