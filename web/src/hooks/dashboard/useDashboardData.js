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

import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  API,
  getTodayStartTimestamp,
  isEffectiveAdmin,
  showError,
  setUserData,
  timestamp2string,
} from '../../helpers';
import { getDefaultTime, getInitialTimestamp } from '../../helpers/dashboard';
import { TIME_OPTIONS } from '../../constants/dashboard.constants';
import { useIsMobile } from '../common/useIsMobile';
import { useMinimumLoadingTime } from '../common/useMinimumLoadingTime';

export const useDashboardData = (userState, userDispatch, statusState) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const initialized = useRef(false);

  // ========== 基础状态 ==========
  const [loading, setLoading] = useState(false);
  const [searchModalVisible, setSearchModalVisible] = useState(false);
  const showLoading = useMinimumLoadingTime(loading);

  // ========== 输入状态 ==========
  const [inputs, setInputs] = useState({
    username: '',
    token_name: '',
    model_name: '',
    start_timestamp: getInitialTimestamp(),
    end_timestamp: timestamp2string(new Date().getTime() / 1000 + 3600),
    channel: '',
    data_export_default_time: '',
  });

  const [dataExportDefaultTime, setDataExportDefaultTime] =
    useState(getDefaultTime());

  // ========== 数据状态 ==========
  const [quotaData, setQuotaData] = useState([]);
  const [consumeQuota, setConsumeQuota] = useState(0);
  const [standardConsumeQuota, setStandardConsumeQuota] = useState(0);
  const [consumeTokens, setConsumeTokens] = useState(0);
  const [times, setTimes] = useState(0);
  const [todayUsedQuota, setTodayUsedQuota] = useState(0);
  const [todayStandardUsedQuota, setTodayStandardUsedQuota] = useState(0);
  const [todayUsedTimes, setTodayUsedTimes] = useState(0);
  const [todayCacheHitTokens, setTodayCacheHitTokens] = useState(0);
  const [todayPromptTokensTotal, setTodayPromptTokensTotal] = useState(0);
  const [todayGlobalCacheHitTokens, setTodayGlobalCacheHitTokens] = useState(0);
  const [todayGlobalPromptTokensTotal, setTodayGlobalPromptTokensTotal] =
    useState(0);
  const [todayUsedQuotaLoading, setTodayUsedQuotaLoading] = useState(false);
  const [quotaLegacyNotice, setQuotaLegacyNotice] = useState(false);
  const [todayQuotaLegacyNotice, setTodayQuotaLegacyNotice] = useState(false);
  const [pieData, setPieData] = useState([{ type: 'null', value: '0' }]);
  const [lineData, setLineData] = useState([]);
  const [modelColors, setModelColors] = useState({});

  // ========== 图表状态 ==========
  const [activeChartTab, setActiveChartTab] = useState('1');

  // ========== 趋势数据 ==========
  const [trendData, setTrendData] = useState({
    balance: [],
    usedQuota: [],
    requestCount: [],
    times: [],
    consumeQuota: [],
    tokens: [],
    rpm: [],
    tpm: [],
  });

  // ========== Uptime 数据 ==========
  const [uptimeData, setUptimeData] = useState([]);
  const [uptimeLoading, setUptimeLoading] = useState(false);
  const [activeUptimeTab, setActiveUptimeTab] = useState('');

  // ========== 常量 ==========
  const now = new Date();
  const isAdminUser = isEffectiveAdmin();

  // ========== Panel enable flags ==========
  const apiInfoEnabled = statusState?.status?.api_info_enabled ?? true;
  const announcementsEnabled =
    statusState?.status?.announcements_enabled ?? true;
  const faqEnabled = statusState?.status?.faq_enabled ?? true;
  const uptimeEnabled = statusState?.status?.uptime_kuma_enabled ?? true;

  const hasApiInfoPanel = apiInfoEnabled;
  const hasInfoPanels = announcementsEnabled || faqEnabled || uptimeEnabled;

  // ========== Memoized Values ==========
  const timeOptions = useMemo(
    () =>
      TIME_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const performanceMetrics = useMemo(() => {
    const { start_timestamp, end_timestamp } = inputs;
    const timeDiff =
      (Date.parse(end_timestamp) - Date.parse(start_timestamp)) / 60000;
    const avgRPM = isNaN(times / timeDiff)
      ? '0'
      : (times / timeDiff).toFixed(3);
    const avgTPM = isNaN(consumeTokens / timeDiff)
      ? '0'
      : (consumeTokens / timeDiff).toFixed(3);

    return { avgRPM, avgTPM, timeDiff };
  }, [times, consumeTokens, inputs.start_timestamp, inputs.end_timestamp]);

  const todayCacheHitRate = useMemo(() => {
    const total = Number(todayPromptTokensTotal) || 0;
    const hit = Number(todayCacheHitTokens) || 0;
    if (total <= 0) return null;
    return hit / total;
  }, [todayCacheHitTokens, todayPromptTokensTotal]);

  const todayGlobalCacheHitRate = useMemo(() => {
    const total = Number(todayGlobalPromptTokensTotal) || 0;
    const hit = Number(todayGlobalCacheHitTokens) || 0;
    if (total <= 0) return null;
    return hit / total;
  }, [todayGlobalCacheHitTokens, todayGlobalPromptTokensTotal]);

  // ========== 回调函数 ==========
  const handleInputChange = useCallback((value, name) => {
    if (name === 'data_export_default_time') {
      setDataExportDefaultTime(value);
      localStorage.setItem('data_export_default_time', value);
      setInputs((inputs) => ({ ...inputs, [name]: value }));
      return;
    }
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }, []);

  const showSearchModal = useCallback(() => {
    setSearchModalVisible(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setSearchModalVisible(false);
  }, []);

  // ========== API 调用函数 ==========
  const loadQuotaData = useCallback(async () => {
    setLoading(true);
    try {
      let url = '';
      const { start_timestamp, end_timestamp, username } = inputs;
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;

      if (isAdminUser) {
        url = `/api/data/?username=${username}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      } else {
        url = `/api/data/self/?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      }

      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (success) {
        const normalizedData = Array.isArray(data)
          ? data.map((item) => {
              const settledQuota = Number(item?.quota) || 0;
              const costQuota = Number(item?.cost_quota ?? 0) || 0;
              const hasExplicitLegacyFlag =
                typeof item?.quota_legacy === 'boolean';
              const quotaLegacy = hasExplicitLegacyFlag
                ? Boolean(item?.quota_legacy)
                : !inputs?.username && settledQuota > 0 && costQuota <= 0;
              return {
                ...item,
                settled_quota: settledQuota,
                quota_legacy: quotaLegacy,
                cost_quota: costQuota,
                quota: settledQuota,
              };
            })
          : [];
        setQuotaData(normalizedData);
        setQuotaLegacyNotice(
          normalizedData.some((item) => Boolean(item?.quota_legacy)),
        );
        setStandardConsumeQuota(
          normalizedData.reduce(
            (sum, item) => sum + (Number(item?.cost_quota) || 0),
            0,
          ),
        );
        if (normalizedData.length === 0) {
          normalizedData.push({
            count: 0,
            model_name: '无数据',
            quota: 0,
            visible_quota: 0,
            cost_quota: 0,
            created_at: now.getTime() / 1000,
          });
        }
        normalizedData.sort((a, b) => a.created_at - b.created_at);
        return normalizedData;
      } else {
        showError(message);
        setStandardConsumeQuota(0);
        setQuotaLegacyNotice(false);
        return [];
      }
    } finally {
      setLoading(false);
    }
  }, [inputs, dataExportDefaultTime, isAdminUser, now]);

  const loadUptimeData = useCallback(async () => {
    setUptimeLoading(true);
    try {
      const res = await API.get('/api/uptime/status');
      const { success, message, data } = res.data;
      if (success) {
        setUptimeData(data || []);
        if (data && data.length > 0 && !activeUptimeTab) {
          setActiveUptimeTab(data[0].categoryName);
        }
      } else {
        showError(message);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setUptimeLoading(false);
    }
  }, [activeUptimeTab]);

  const loadTodayUsedQuota = useCallback(async () => {
    setTodayUsedQuotaLoading(true);
    try {
      const startTimestamp = getTodayStartTimestamp();
      const endTimestamp = Math.floor(Date.now() / 1000) + 3600;
      const url = `/api/log/self/stat?type=2&start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`;
      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (success) {
        const todaySettledQuota = Number(data?.quota ?? 0) || 0;
        const todayStandardQuota = Number(data?.cost_quota ?? 0) || 0;
        setTodayUsedQuota(todaySettledQuota);
        setTodayStandardUsedQuota(todayStandardQuota);
        setTodayUsedTimes(data?.count ?? 0);
        setTodayQuotaLegacyNotice(Boolean(data?.quota_legacy));
        try {
          const cacheUrl = `/api/log/self/cache_stat?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`;
          const cacheRes = await API.get(cacheUrl);
          const {
            success: cacheSuccess,
            message: cacheMessage,
            data: cacheData,
          } = cacheRes.data;
          if (cacheSuccess) {
            setTodayCacheHitTokens(cacheData?.cache_hit_tokens ?? 0);
            setTodayPromptTokensTotal(cacheData?.prompt_tokens_total ?? 0);
          } else {
            setTodayCacheHitTokens(0);
            setTodayPromptTokensTotal(0);
            showError(cacheMessage);
          }
        } catch (error) {
          setTodayCacheHitTokens(0);
          setTodayPromptTokensTotal(0);
          showError(t('请求失败'));
        }

        try {
          const globalCacheUrl = isAdminUser
            ? `/api/log/cache_stat?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`
            : `/api/log/global/cache_stat?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`;
          const globalCacheRes = await API.get(globalCacheUrl);
          const {
            success: globalSuccess,
            message: globalMessage,
            data: globalData,
          } = globalCacheRes.data;
          if (globalSuccess) {
            setTodayGlobalCacheHitTokens(globalData?.cache_hit_tokens ?? 0);
            setTodayGlobalPromptTokensTotal(
              globalData?.prompt_tokens_total ?? 0,
            );
          } else {
            setTodayGlobalCacheHitTokens(0);
            setTodayGlobalPromptTokensTotal(0);
            showError(globalMessage);
          }
        } catch (error) {
          setTodayGlobalCacheHitTokens(0);
          setTodayGlobalPromptTokensTotal(0);
          showError(t('请求失败'));
        }
      } else {
        setTodayUsedQuota(0);
        setTodayStandardUsedQuota(0);
        setTodayCacheHitTokens(0);
        setTodayPromptTokensTotal(0);
        setTodayGlobalCacheHitTokens(0);
        setTodayGlobalPromptTokensTotal(0);
        setTodayQuotaLegacyNotice(false);
        showError(message);
      }
    } catch (error) {
      setTodayUsedQuota(0);
      setTodayStandardUsedQuota(0);
      setTodayCacheHitTokens(0);
      setTodayPromptTokensTotal(0);
      setTodayGlobalCacheHitTokens(0);
      setTodayGlobalPromptTokensTotal(0);
      setTodayQuotaLegacyNotice(false);
      showError(t('请求失败'));
    } finally {
      setTodayUsedQuotaLoading(false);
    }
  }, [isAdminUser, t]);

  const getUserData = useCallback(async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
      setUserData(data);
    } else {
      showError(message);
    }
  }, [userDispatch]);

  const refresh = useCallback(async () => {
    const data = await loadQuotaData();
    return data;
  }, [loadQuotaData]);

  const handleSearchConfirm = useCallback(
    async (updateChartDataCallback) => {
      const data = await refresh();
      if (data && data.length > 0 && updateChartDataCallback) {
        updateChartDataCallback(data);
      }
      setSearchModalVisible(false);
    },
    [refresh],
  );

  useEffect(() => {
    if (!initialized.current) {
      getUserData();
      initialized.current = true;
    }
  }, [getUserData]);

  return {
    // 基础状态
    loading: showLoading,
    searchModalVisible,

    // 输入状态
    inputs,
    dataExportDefaultTime,

    // 数据状态
    quotaData,
    consumeQuota,
    standardConsumeQuota,
    setConsumeQuota,
    consumeTokens,
    setConsumeTokens,
    times,
    setTimes,
    todayUsedQuota,
    todayStandardUsedQuota,
    todayUsedTimes,
    todayCacheHitRate,
    todayGlobalCacheHitRate,
    todayUsedQuotaLoading,
    pieData,
    setPieData,
    lineData,
    setLineData,
    modelColors,
    setModelColors,

    // 图表状态
    activeChartTab,
    setActiveChartTab,

    // 趋势数据
    trendData,
    setTrendData,

    // Uptime 数据
    uptimeData,
    uptimeLoading,
    activeUptimeTab,
    setActiveUptimeTab,

    // 计算值
    timeOptions,
    performanceMetrics,
    todayCacheHitTokens,
    todayPromptTokensTotal,
    todayGlobalCacheHitTokens,
    todayGlobalPromptTokensTotal,
    isAdminUser,
    quotaLegacyNotice,
    todayQuotaLegacyNotice,
    hasApiInfoPanel,
    hasInfoPanels,
    apiInfoEnabled,
    announcementsEnabled,
    faqEnabled,
    uptimeEnabled,

    // 函数
    handleInputChange,
    showSearchModal,
    handleCloseModal,
    loadQuotaData,
    loadUptimeData,
    loadTodayUsedQuota,
    getUserData,
    refresh,
    handleSearchConfirm,

    // 导航和翻译
    navigate,
    t,
    isMobile,
  };
};
