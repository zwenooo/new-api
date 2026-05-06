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

import { useState, useCallback, useLayoutEffect } from 'react';
import {
  generateVChartSemiTheme,
  initVChartSemiTheme,
  switchVChartSemiTheme,
} from '@visactor/vchart-semi-theme';
import {
  modelColorMap,
  renderNumber,
  renderQuota,
  modelToColor,
  getQuotaWithUnit,
} from '../../helpers';
import {
  processRawData,
  calculateTrendData,
  aggregateDataByTimeAndModel,
  generateChartTimePoints,
  updateChartSpec,
  updateMapValue,
  initializeMaps,
} from '../../helpers/dashboard';

export const useDashboardCharts = (
  dataExportDefaultTime,
  setTrendData,
  setConsumeQuota,
  setTimes,
  setConsumeTokens,
  setPieData,
  setLineData,
  setModelColors,
  t,
  groupLabelMaps = {},
) => {
  // ========== 图表规格状态 ==========
  const [spec_pie, setSpecPie] = useState({
    type: 'pie',
    data: [
      {
        id: 'id0',
        values: [{ type: 'null', value: '0' }],
      },
    ],
    outerRadius: 0.8,
    innerRadius: 0.5,
    padAngle: 0.6,
    valueField: 'value',
    categoryField: 'type',
    pie: {
      style: {
        cornerRadius: 10,
      },
      state: {
        hover: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
        selected: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    title: {
      visible: false,
    },
    legends: {
      visible: true,
      orient: 'left',
    },
    label: {
      visible: true,
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['type'],
            value: (datum) => renderNumber(datum['value']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_line, setSpecLine] = useState({
    type: 'bar',
    data: [
      {
        id: 'barData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Usage',
    seriesField: 'Model',
    stack: true,
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: false,
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            if (array[i].key == '其他') {
              continue;
            }
            let value = parseFloat(array[i].value);
            if (isNaN(value)) {
              value = 0;
            }
            if (array[i].datum && array[i].datum.TimeSum) {
              sum = array[i].datum.TimeSum;
            }
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // 模型消耗趋势折线图
  const [spec_model_line, setSpecModelLine] = useState({
    type: 'line',
    data: [
      {
        id: 'lineData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: false,
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // 模型调用次数排行柱状图
  const [spec_rank_bar, setSpecRankBar] = useState({
    type: 'bar',
    data: [
      {
        id: 'rankData',
        values: [],
      },
    ],
    xField: 'Model',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: false,
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_cache_hit_rate, setSpecCacheHitRate] = useState({
    type: 'bar',
    data: [
      {
        id: 'cacheHitRateData',
        values: [],
      },
    ],
    xField: ['Dimension', 'Series'],
    yField: 'Rate',
    seriesField: 'Series',
    stack: false,
    barGapInGroup: 4,
    barMaxWidth: 34,
    legends: {
      visible: true,
    },
    title: {
      visible: false,
    },
    label: {
      visible: true,
      position: 'top',
      formatMethod: (value, datum) =>
        `${Number(datum?.Rate ?? value ?? 0).toFixed(2)}%`,
    },
    axes: [
      {
        orient: 'bottom',
        sampling: false,
        label: {
          autoHide: false,
          autoRotate: true,
          autoLimit: false,
          autoRotateAngle: [35],
        },
      },
      {
        orient: 'left',
        label: {
          formatMethod: (value) => `${value}%`,
        },
      },
    ],
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Series'],
            value: (datum) => `${Number(datum['Rate'] || 0).toFixed(2)}%`,
          },
          {
            key: t('分组'),
            value: (datum) => datum['GroupLabel'] || '-',
          },
          {
            key: 'UA',
            value: (datum) => datum['RawUA'] || datum['Dimension'] || '-',
          },
          {
            key: t('缓存 Tokens'),
            value: (datum) => renderNumber(datum['CacheHitTokens'] || 0),
          },
          {
            key: 'Prompt Tokens',
            value: (datum) => renderNumber(datum['PromptTokensTotal'] || 0),
          },
        ],
      },
    },
    color: {
      specified: {
        [t('全站均值')]: '#2563eb',
        [t('自身缓存率')]: '#16a34a',
        [t('缓存率')]: '#2563eb',
      },
    },
  });

  const [spec_token_quota_bar, setSpecTokenQuotaBar] = useState({
    type: 'bar',
    data: [
      {
        id: 'tokenQuotaData',
        values: [],
      },
    ],
    xField: 'TokenName',
    yField: 'Quota',
    seriesField: 'TokenName',
    barMaxWidth: 28,
    padding: {
      top: 12,
      right: 24,
      bottom: 76,
      left: 64,
    },
    legends: {
      visible: false,
    },
    title: {
      visible: false,
    },
    label: {
      visible: false,
    },
    axes: [
      {
        orient: 'bottom',
        sampling: false,
        label: {
          autoHide: false,
          autoRotate: true,
          autoLimit: false,
          autoRotateAngle: [35],
        },
      },
      {
        orient: 'left',
        label: {
          formatMethod: (value) => renderQuota(Number(value) || 0, 2),
        },
      },
    ],
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['TokenName'],
            value: (datum) => renderQuota(datum['Quota'] || 0, 4),
          },
          {
            key: t('调用次数'),
            value: (datum) => renderNumber(datum['Count'] || 0),
          },
          {
            key: t('用户显示消耗'),
            value: (datum) => renderQuota(datum['VisibleQuota'] || 0, 4),
          },
          {
            key: t('标准费用'),
            value: (datum) => renderQuota(datum['CostQuota'] || 0, 4),
          },
        ],
      },
    },
    color: {
      specified: {},
    },
  });

  // ========== 数据处理函数 ==========
  const generateModelColors = useCallback((uniqueModels, modelColors) => {
    const newModelColors = {};
    Array.from(uniqueModels).forEach((modelName) => {
      newModelColors[modelName] =
        modelColorMap[modelName] ||
        modelColors[modelName] ||
        modelToColor(modelName);
    });
    return newModelColors;
  }, []);

  const updateChartData = useCallback(
    (data) => {
      const processedData = processRawData(
        data,
        dataExportDefaultTime,
        initializeMaps,
        updateMapValue,
      );

      const {
        totalQuota,
        totalTimes,
        totalTokens,
        uniqueModels,
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
      } = processedData;

      const trendDataResult = calculateTrendData(
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
        dataExportDefaultTime,
      );
      setTrendData(trendDataResult);

      const newModelColors = generateModelColors(uniqueModels, {});
      setModelColors(newModelColors);

      const aggregatedData = aggregateDataByTimeAndModel(
        data,
        dataExportDefaultTime,
      );

      const modelTotals = new Map();
      for (let [_, value] of aggregatedData) {
        updateMapValue(modelTotals, value.model, value.count);
      }

      const newPieData = Array.from(modelTotals)
        .map(([model, count]) => ({
          type: model,
          value: count,
        }))
        .sort((a, b) => b.value - a.value);

      const chartTimePoints = generateChartTimePoints(
        aggregatedData,
        data,
        dataExportDefaultTime,
      );

      let newLineData = [];

      chartTimePoints.forEach((time) => {
        let timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            rawQuota: aggregated?.quota || 0,
            Usage: aggregated?.quota
              ? getQuotaWithUnit(aggregated.quota, 4)
              : 0,
          };
        });

        const timeSum = timeData.reduce((sum, item) => sum + item.rawQuota, 0);
        timeData.sort((a, b) => b.rawQuota - a.rawQuota);
        timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
        newLineData.push(...timeData);
      });

      newLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      updateChartSpec(
        setSpecPie,
        newPieData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'id0',
      );

      updateChartSpec(
        setSpecLine,
        newLineData,
        `${t('总计')}：${renderQuota(totalQuota, 2)}`,
        newModelColors,
        'barData',
      );

      // ===== 模型调用次数折线图 =====
      let modelLineData = [];
      chartTimePoints.forEach((time) => {
        const timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            Count: aggregated?.count || 0,
          };
        });
        modelLineData.push(...timeData);
      });
      modelLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      // ===== 模型调用次数排行柱状图 =====
      const rankData = Array.from(modelTotals)
        .map(([model, count]) => ({
          Model: model,
          Count: count,
        }))
        .sort((a, b) => b.Count - a.Count);

      updateChartSpec(
        setSpecModelLine,
        modelLineData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'lineData',
      );

      updateChartSpec(
        setSpecRankBar,
        rankData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'rankData',
      );

      setPieData(newPieData);
      setLineData(newLineData);
      setConsumeQuota(totalQuota);
      setTimes(totalTimes);
      setConsumeTokens(totalTokens);
    },
    [
      dataExportDefaultTime,
      setTrendData,
      generateModelColors,
      setModelColors,
      setPieData,
      setLineData,
      setConsumeQuota,
      setTimes,
      setConsumeTokens,
      t,
    ],
  );

  const updateCacheHitRateData = useCallback(
    (data, labelMaps = groupLabelMaps) => {
      const resolveGroupLabel = (group) => {
        const raw = String(group || '').trim();
        if (!raw) {
          return t('未知分组');
        }
        const groupId = Number(raw);
        if (Number.isFinite(groupId) && groupId > 0) {
          const normalizedId = Math.floor(groupId);
          return (
            labelMaps.byId?.[normalizedId] || labelMaps.byCode?.[raw] || raw
          );
        }
        return labelMaps.byCode?.[raw] || raw;
      };

      const normalizeStat = (item, series) => {
        const cacheHitTokens = Number(item?.cache_hit_tokens || 0);
        const promptTokensTotal = Number(item?.prompt_tokens_total || 0);
        const rate =
          promptTokensTotal > 0
            ? (cacheHitTokens / promptTokensTotal) * 100
            : 0;
        const rawGroup = String(item?.group || '').trim();
        const groupLabel = resolveGroupLabel(rawGroup);
        const rawUA = String(item?.ua || '').trim() || t('未知');
        const dimension = `${groupLabel} / ${rawUA}`;
        return {
          Dimension: dimension,
          UA: dimension,
          RawUA: rawUA,
          Group: rawGroup,
          GroupLabel: groupLabel,
          Series: series,
          Rate: Number(rate.toFixed(4)),
          CacheHitTokens: cacheHitTokens,
          PromptTokensTotal: promptTokensTotal,
        };
      };

      const isComparisonData =
        data &&
        !Array.isArray(data) &&
        (Array.isArray(data.global) || Array.isArray(data.self));
      const values = isComparisonData
        ? [
            ...(Array.isArray(data.global)
              ? data.global.map((item) => normalizeStat(item, t('全站均值')))
              : []),
            ...(Array.isArray(data.self)
              ? data.self.map((item) => normalizeStat(item, t('自身缓存率')))
              : []),
          ]
        : (Array.isArray(data) ? data : []).map((item) =>
            normalizeStat(item, t('缓存率')),
          );
      const series = Array.from(new Set(values.map((item) => item.Series)));
      const seriesSummary = series.map((name) => {
        const items = values.filter((item) => item.Series === name);
        const totalHit = items.reduce(
          (sum, item) => sum + Number(item.CacheHitTokens || 0),
          0,
        );
        const totalPrompt = items.reduce(
          (sum, item) => sum + Number(item.PromptTokensTotal || 0),
          0,
        );
        return {
          name,
          rate: totalPrompt > 0 ? (totalHit / totalPrompt) * 100 : null,
        };
      });

      setSpecCacheHitRate((prev) => ({
        ...prev,
        data: [{ id: 'cacheHitRateData', values }],
        legends: {
          ...prev.legends,
          visible: series.length > 1,
        },
        color: {
          specified: {
            ...prev.color?.specified,
            [t('全站均值')]: '#2563eb',
            [t('自身缓存率')]: '#16a34a',
            [t('缓存率')]: '#2563eb',
          },
        },
        title: {
          ...prev.title,
          subtext:
            seriesSummary.length > 0
              ? seriesSummary
                  .map((item) =>
                    item.rate == null
                      ? `${item.name}：-`
                      : `${item.name}：${item.rate.toFixed(2)}%`,
                  )
                  .join(' · ')
              : `${t('总计')}：-`,
        },
      }));
    },
    [groupLabelMaps.byCode, groupLabelMaps.byId, t],
  );

  const updateTokenQuotaData = useCallback(
    (data) => {
      const values = (Array.isArray(data) ? data : [])
        .map((item) => {
          const tokenName = String(item?.token_name || '').trim();
          const quota = Number(item?.quota || 0);
          return {
            TokenName: tokenName || t('操练场'),
            RawTokenName: tokenName,
            Quota: quota,
            VisibleQuota: Number(item?.visible_quota || 0),
            CostQuota: Number(item?.cost_quota || 0),
            Count: Number(item?.count || 0),
          };
        })
        .filter((item) => item.Quota > 0 || item.Count > 0)
        .sort((a, b) => b.Quota - a.Quota);

      const totalQuota = values.reduce(
        (sum, item) => sum + Number(item.Quota || 0),
        0,
      );
      const totalCount = values.reduce(
        (sum, item) => sum + Number(item.Count || 0),
        0,
      );
      const tokenColors = {};
      values.forEach((item) => {
        tokenColors[item.TokenName] = modelToColor(item.TokenName);
      });

      setSpecTokenQuotaBar((prev) => ({
        ...prev,
        data: [{ id: 'tokenQuotaData', values }],
        title: {
          ...prev.title,
          subtext:
            values.length > 0
              ? `${t('总计')}：${renderQuota(totalQuota, 2)} · ${t('调用次数')}：${renderNumber(totalCount)}`
              : `${t('总计')}：-`,
        },
        color: {
          specified: tokenColors,
        },
      }));
    },
    [t],
  );

  // ========== 初始化图表主题 ==========
  useLayoutEffect(() => {
    const getCssVar = (styles, name, fallback) =>
      styles.getPropertyValue(name).trim() || fallback;

    // 根据当前 CSS 变量同步图表主题，避免图表背景与卡片面板割裂。
    const buildChartTheme = (mode) => {
      const styles = window.getComputedStyle(document.documentElement);
      const theme = generateVChartSemiTheme(mode);
      const palette = theme.colorScheme.default.palette;
      const surfaceBg = getCssVar(
        styles,
        '--dashboard-chart-bg-color',
        mode === 'dark' ? '#111318' : '#f8fafc',
      );
      const popupBg = getCssVar(
        styles,
        '--dashboard-chart-popup-bg',
        mode === 'dark' ? '#1a1d24' : '#ffffff',
      );
      const textPrimary = getCssVar(
        styles,
        '--dashboard-chart-text-primary',
        mode === 'dark' ? 'rgba(248,250,252,0.96)' : 'rgba(17,24,39,0.92)',
      );
      const textSecondary = getCssVar(
        styles,
        '--dashboard-chart-text-secondary',
        mode === 'dark' ? 'rgba(203,213,225,0.72)' : 'rgba(71,85,105,0.72)',
      );
      const gridColor = getCssVar(
        styles,
        '--dashboard-chart-grid',
        mode === 'dark' ? 'rgba(148,163,184,0.14)' : 'rgba(148,163,184,0.18)',
      );
      const axisMarkerBg = getCssVar(
        styles,
        '--dashboard-chart-axis-marker-bg',
        mode === 'dark' ? 'rgba(248,250,252,0.92)' : 'rgba(15,23,42,0.92)',
      );
      const axisMarkerFont = getCssVar(
        styles,
        '--dashboard-chart-axis-marker-font',
        mode === 'dark' ? '#111827' : '#ffffff',
      );

      palette.backgroundColor = surfaceBg;
      palette.popupBackgroundColor = popupBg;
      palette.primaryFontColor = textPrimary;
      palette.secondaryFontColor = textSecondary;
      palette.tertiaryFontColor = textSecondary;
      palette.axisLabelFontColor = textSecondary;
      palette.axisGridColor = gridColor;
      palette.axisDomainColor = gridColor;
      palette.markLabelBackgroundColor = gridColor;
      palette.axisMarkerBackgroundColor = axisMarkerBg;
      palette.axisMarkerFontColor = axisMarkerFont;

      return theme;
    };

    const syncChartTheme = () => {
      const mode = document.documentElement.classList.contains('dark')
        ? 'dark'
        : 'light';
      switchVChartSemiTheme(true, mode, buildChartTheme(mode));
    };

    initVChartSemiTheme({
      isWatchingMode: false,
      isWatchingThemeSwitch: false,
    });
    syncChartTheme();

    const observer = new MutationObserver(() => {
      syncChartTheme();
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });

    return () => {
      observer.disconnect();
    };
  }, []);

  return {
    // 图表规格
    spec_pie,
    spec_line,
    spec_model_line,
    spec_rank_bar,
    spec_cache_hit_rate,
    spec_token_quota_bar,

    // 函数
    updateChartData,
    updateCacheHitRateData,
    updateTokenQuotaData,
    generateModelColors,
  };
};
