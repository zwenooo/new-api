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

import React, { useContext, useEffect } from 'react';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import DashboardHeader from './DashboardHeader';
import StatsCards from './StatsCards';
import ChartsPanel from './ChartsPanel';
import ApiInfoPanel from './ApiInfoPanel';
import UptimePanel from './UptimePanel';
import SearchModal from './modals/SearchModal';

import { useDashboardData } from '../../hooks/dashboard/useDashboardData';
import { useDashboardStats } from '../../hooks/dashboard/useDashboardStats';
import { useDashboardCharts } from '../../hooks/dashboard/useDashboardCharts';

import {
  CHART_CONFIG,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  UPTIME_STATUS_MAP,
} from '../../constants/dashboard.constants';
import {
  handleCopyUrl,
  handleSpeedTest,
  getUptimeStatusColor,
  getUptimeStatusText,
  renderMonitorList,
} from '../../helpers/dashboard';

const Dashboard = () => {
  // ========== Context ==========
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);

  // ========== 主要数据管理 ==========
  const dashboardData = useDashboardData(userState, userDispatch, statusState);

  // ========== 图表管理 ==========
  const dashboardCharts = useDashboardCharts(
    dashboardData.dataExportDefaultTime,
    dashboardData.setTrendData,
    dashboardData.setConsumeQuota,
    dashboardData.setTimes,
    dashboardData.setConsumeTokens,
    dashboardData.setPieData,
    dashboardData.setLineData,
    dashboardData.setModelColors,
    dashboardData.t,
    dashboardData.groupLabelMaps,
  );

  // ========== 统计数据 ==========
  const { groupedStatsData } = useDashboardStats(
    userState,
    dashboardData.consumeQuota,
    dashboardData.standardConsumeQuota,
    dashboardData.consumeTokens,
    dashboardData.times,
    dashboardData.todayUsedQuota,
    dashboardData.todayUsedTimes,
    dashboardData.trendData,
    dashboardData.performanceMetrics,
    dashboardData.navigate,
    dashboardData.t,
  );

  // ========== 分组数据处理 ==========
  const isAdminUser = dashboardData.isAdminUser;
  const accountGroup = groupedStatsData.find(
    (group) => group.key === 'account',
  );
  const baseGroups = groupedStatsData.filter(
    (group) => group.key !== 'account',
  );

  let statsCardGroups = baseGroups;

  if (!isAdminUser) {
    const usageGroup = baseGroups.find((group) => group.key === 'usage');
    const todayCallsGroup = baseGroups.find(
      (group) => group.key === 'today-calls',
    );
    const tokensGroup = baseGroups.find((group) => group.key === 'tokens');
    const historyGroup = baseGroups.find((group) => group.key === 'history');
    const resourceGroup = baseGroups.find((group) => group.key === 'resource');
    const otherGroups = baseGroups.filter(
      (group) =>
        group.key !== 'usage' &&
        group.key !== 'today-calls' &&
        group.key !== 'tokens' &&
        group.key !== 'history' &&
        group.key !== 'resource',
    );

    // 顺序：今日消费 -> 今日调用次数 -> TOKEN消耗 -> 今日剩余额度 -> 历史消耗
    statsCardGroups = [
      usageGroup,
      todayCallsGroup,
      tokensGroup,
      resourceGroup,
      historyGroup,
      ...otherGroups,
    ].filter(Boolean);
  }

  // ========== 数据处理 ==========
  const loadTokenQuotaDataIfVisible = () =>
    isAdminUser ? Promise.resolve([]) : dashboardData.loadTokenQuotaData();

  const initChart = async () => {
    const [data, cacheHitRateByUA, tokenQuotaData, groupLabelMaps] =
      await Promise.all([
        dashboardData.loadQuotaData(),
        dashboardData.loadCacheHitRateByUA(),
        loadTokenQuotaDataIfVisible(),
        dashboardData.loadGroupLabels(),
        dashboardData.loadTodayUsedQuota(),
      ]);
    if (data && data.length > 0) {
      dashboardCharts.updateChartData(data);
    }
    dashboardCharts.updateCacheHitRateData(cacheHitRateByUA, groupLabelMaps);
    dashboardCharts.updateTokenQuotaData(tokenQuotaData);
    if (isAdminUser && dashboardData.uptimeEnabled) {
      await dashboardData.loadUptimeData();
    }
  };

  const handleRefresh = async () => {
    const [data, cacheHitRateByUA, tokenQuotaData, groupLabelMaps] =
      await Promise.all([
        dashboardData.refresh(),
        dashboardData.loadCacheHitRateByUA(),
        loadTokenQuotaDataIfVisible(),
        dashboardData.loadGroupLabels(),
      ]);
    if (data && data.length > 0) {
      dashboardCharts.updateChartData(data);
    }
    dashboardCharts.updateCacheHitRateData(cacheHitRateByUA, groupLabelMaps);
    dashboardCharts.updateTokenQuotaData(tokenQuotaData);
    if (!isAdminUser) {
      await Promise.all([
        dashboardData.loadTodayUsedQuota(),
        dashboardData.getUserData(),
      ]);
    } else {
      await dashboardData.loadTodayUsedQuota();
    }
    if (isAdminUser && dashboardData.uptimeEnabled) {
      await dashboardData.loadUptimeData();
    }
  };

  const handleSearchConfirm = async () => {
    const [data, cacheHitRateByUA, tokenQuotaData, groupLabelMaps] =
      await Promise.all([
        dashboardData.handleSearchConfirm(),
        dashboardData.loadCacheHitRateByUA(),
        loadTokenQuotaDataIfVisible(),
        dashboardData.loadGroupLabels(),
      ]);
    if (data && data.length > 0) {
      dashboardCharts.updateChartData(data);
    }
    dashboardCharts.updateCacheHitRateData(cacheHitRateByUA, groupLabelMaps);
    dashboardCharts.updateTokenQuotaData(tokenQuotaData);
  };

  // ========== 数据准备 ==========
  const apiInfoData = statusState?.status?.api_info || [];
  const uptimeLegendData = Object.entries(UPTIME_STATUS_MAP).map(
    ([status, info]) => ({
      status: Number(status),
      color: info.color,
      label: dashboardData.t(info.label),
    }),
  );

  // ========== Effects ==========
  useEffect(() => {
    initChart();
  }, []);

  useEffect(() => {
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prevOverflow;
    };
  }, []);

  useEffect(() => {
    if (isAdminUser && dashboardData.activeChartTab === '6') {
      dashboardData.setActiveChartTab('1');
    }
  }, [isAdminUser, dashboardData.activeChartTab]);

  const filterConfig = !isAdminUser
    ? {
        startTimestamp: dashboardData.inputs.start_timestamp,
        endTimestamp: dashboardData.inputs.end_timestamp,
        dataExportDefaultTime: dashboardData.dataExportDefaultTime,
        timeOptions: dashboardData.timeOptions,
        handleInputChange: dashboardData.handleInputChange,
        handleFilterConfirm: handleSearchConfirm,
      }
    : null;

  return (
    <div className='dashboard-shell flex h-full min-h-0 flex-col overflow-hidden'>
      <DashboardHeader
        showSearchModal={dashboardData.showSearchModal}
        refresh={handleRefresh}
        loading={dashboardData.loading}
        isAdminUser={isAdminUser}
        t={dashboardData.t}
      />

      <SearchModal
        searchModalVisible={dashboardData.searchModalVisible}
        handleSearchConfirm={handleSearchConfirm}
        handleCloseModal={dashboardData.handleCloseModal}
        isMobile={dashboardData.isMobile}
        isAdminUser={dashboardData.isAdminUser}
        inputs={dashboardData.inputs}
        dataExportDefaultTime={dashboardData.dataExportDefaultTime}
        timeOptions={dashboardData.timeOptions}
        handleInputChange={dashboardData.handleInputChange}
        t={dashboardData.t}
      />

      <div className='grid flex-1 min-h-0 grid-cols-1 gap-3 overflow-hidden pt-2 lg:grid-cols-12 lg:auto-rows-fr'>
        <div
          className={`flex min-h-0 flex-col gap-2 overflow-hidden ${
            isAdminUser ? 'lg:col-span-8' : 'lg:col-span-12'
          }`.trim()}
        >
          <StatsCards
            groupedStatsData={statsCardGroups}
            loading={
              dashboardData.loading || dashboardData.todayUsedQuotaLoading
            }
          />

          <div className='flex min-h-0 flex-1 flex-col'>
            <ChartsPanel
              activeChartTab={dashboardData.activeChartTab}
              setActiveChartTab={dashboardData.setActiveChartTab}
              spec_line={dashboardCharts.spec_line}
              spec_model_line={dashboardCharts.spec_model_line}
              spec_pie={dashboardCharts.spec_pie}
              spec_rank_bar={dashboardCharts.spec_rank_bar}
              spec_cache_hit_rate={dashboardCharts.spec_cache_hit_rate}
              spec_token_quota_bar={dashboardCharts.spec_token_quota_bar}
              CHART_CONFIG={CHART_CONFIG}
              hasApiInfoPanel={false}
              filterConfig={filterConfig}
              showTokenQuotaChart={!isAdminUser}
              t={dashboardData.t}
            />
          </div>
        </div>

        {isAdminUser ? (
          <div className='flex min-h-0 flex-col overflow-hidden lg:col-span-4'>
            <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-hidden'>
              <ApiInfoPanel
                apiInfoData={apiInfoData}
                handleCopyUrl={(url) => handleCopyUrl(url, dashboardData.t)}
                handleSpeedTest={handleSpeedTest}
                accountGroup={accountGroup}
                isAdminUser={isAdminUser}
                CARD_PROPS={CARD_PROPS}
                FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
                ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
                t={dashboardData.t}
              />

              {dashboardData.uptimeEnabled && (
                <UptimePanel
                  uptimeData={dashboardData.uptimeData}
                  uptimeLoading={dashboardData.uptimeLoading}
                  activeUptimeTab={dashboardData.activeUptimeTab}
                  setActiveUptimeTab={dashboardData.setActiveUptimeTab}
                  loadUptimeData={dashboardData.loadUptimeData}
                  uptimeLegendData={uptimeLegendData}
                  renderMonitorList={(monitors) =>
                    renderMonitorList(
                      monitors,
                      (status) =>
                        getUptimeStatusColor(status, UPTIME_STATUS_MAP),
                      (status) =>
                        getUptimeStatusText(
                          status,
                          UPTIME_STATUS_MAP,
                          dashboardData.t,
                        ),
                      dashboardData.t,
                    )
                  }
                  CARD_PROPS={CARD_PROPS}
                  ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
                  t={dashboardData.t}
                />
              )}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
};

export default Dashboard;
