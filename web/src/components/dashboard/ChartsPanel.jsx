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

import React from 'react';
import { Button, DatePicker, Select } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';

const ChartsPanel = ({
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_model_line,
  spec_pie,
  spec_rank_bar,
  spec_cache_hit_rate,
  spec_token_quota_bar,
  CHART_CONFIG,
  hasApiInfoPanel,
  filterConfig,
  showTokenQuotaChart,
  t,
}) => {
  const tabOptions = [
    { value: '1', label: t('消耗分布') },
    { value: '2', label: t('消耗趋势') },
    { value: '3', label: t('调用次数分布') },
    { value: '4', label: t('调用次数排行') },
    { value: '5', label: t('缓存命中率') },
    showTokenQuotaChart ? { value: '6', label: t('令牌消耗统计') } : null,
  ].filter(Boolean);

  return (
    <section
      className={`dashboard-chart-panel flex min-h-0 flex-1 flex-col rounded-[26px] bg-[var(--app-card)] shadow-[var(--app-shadow)] ${hasApiInfoPanel ? 'lg:col-span-3' : ''}`.trim()}
    >
      <div className='dashboard-panel-header p-5 pb-4'>
        <h3 className='dashboard-panel-title text-sm font-medium text-neutral-600 dark:text-neutral-300'>
          {t('使用统计')}
        </h3>
        <div className='dashboard-chart-toolbar mt-4 flex flex-wrap items-center gap-2'>
          <div className='flex w-full flex-wrap items-center gap-1 rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-1)] p-1 md:w-auto'>
            {tabOptions.map((option) => {
              const active = activeChartTab === option.value;
              return (
                <Button
                  key={option.value}
                  size='small'
                  type={active ? 'primary' : 'tertiary'}
                  theme={active ? 'solid' : 'borderless'}
                  onClick={() => setActiveChartTab(option.value)}
                  className='!rounded-lg'
                >
                  {option.label}
                </Button>
              );
            })}
          </div>

          {filterConfig ? (
            <div className='flex w-full flex-wrap items-center gap-2 pt-2 md:ml-auto md:w-auto md:pt-0'>
              <DatePicker
                type='dateTime'
                size='small'
                value={filterConfig.startTimestamp}
                onChange={(val) =>
                  filterConfig.handleInputChange(val, 'start_timestamp')
                }
                placeholder={t('起始时间')}
                className='w-full md:w-[190px]'
              />
              <DatePicker
                type='dateTime'
                size='small'
                value={filterConfig.endTimestamp}
                onChange={(val) =>
                  filterConfig.handleInputChange(val, 'end_timestamp')
                }
                placeholder={t('结束时间')}
                className='w-full md:w-[190px]'
              />
              <Select
                size='small'
                value={filterConfig.dataExportDefaultTime}
                optionList={filterConfig.timeOptions}
                onChange={(val) =>
                  filterConfig.handleInputChange(
                    val,
                    'data_export_default_time',
                  )
                }
                placeholder={t('时间粒度')}
                className='w-full md:w-[120px]'
              />
              <Button
                type='primary'
                theme='solid'
                size='small'
                onClick={filterConfig.handleFilterConfirm}
                className='!rounded-lg'
              >
                {t('确认')}
              </Button>
            </div>
          ) : null}
        </div>
      </div>

      <div className='flex min-h-0 flex-1 flex-col px-5 pb-5'>
        <div className='dashboard-chart-surface flex min-h-0 flex-1 flex-col rounded-[22px] bg-[var(--app-card)]'>
          <div className='dashboard-chart-canvas min-h-0 flex-1 p-4'>
            {activeChartTab === '1' && (
              <VChart spec={spec_line} option={CHART_CONFIG} />
            )}
            {activeChartTab === '2' && (
              <VChart spec={spec_model_line} option={CHART_CONFIG} />
            )}
            {activeChartTab === '3' && (
              <VChart spec={spec_pie} option={CHART_CONFIG} />
            )}
            {activeChartTab === '4' && (
              <VChart spec={spec_rank_bar} option={CHART_CONFIG} />
            )}
            {activeChartTab === '5' && (
              <VChart spec={spec_cache_hit_rate} option={CHART_CONFIG} />
            )}
            {showTokenQuotaChart && activeChartTab === '6' && (
              <VChart spec={spec_token_quota_bar} option={CHART_CONFIG} />
            )}
          </div>
        </div>
      </div>
    </section>
  );
};

export default ChartsPanel;
