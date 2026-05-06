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
import { Skeleton } from '@douyinfe/semi-ui';

// 统一卡片色调，保证不同指标在浅色/暗黑模式下都有辨识度。
const getMetricTone = (key) => {
  const toneMap = {
    usage: 'emerald',
    'today-calls': 'sky',
    cache: 'indigo',
    resource: 'amber',
    history: 'violet',
    performance: 'sky',
    account: 'slate',
  };

  return toneMap[key] || 'slate';
};

const CardShell = ({ title, tone, children }) => (
  <section
    className='dashboard-stat-card dashboard-stat-card--plain rounded-[22px] bg-[var(--app-card)] shadow-[var(--app-shadow)]'
    data-tone={tone}
  >
    <div className='p-5 pb-3'>
      <h3 className='dashboard-stat-card__title text-sm font-medium text-neutral-600 dark:text-neutral-300'>
        {title}
      </h3>
    </div>
    <div className='px-5 pb-5'>{children}</div>
  </section>
);

const StatsCards = ({ groupedStatsData, loading }) => {
  const metricGroups = groupedStatsData;

  const allSingleMetricGroups =
    metricGroups.length > 0 &&
    metricGroups.every((group) => group.items.length === 1);

  if (allSingleMetricGroups) {
    const gridCols =
      {
        3: 'md:grid-cols-3',
        4: 'md:grid-cols-4',
        5: 'md:grid-cols-5',
      }[metricGroups.length] || 'md:grid-cols-4';

    return (
      <div className={`grid grid-cols-1 gap-3 ${gridCols}`.trim()}>
        {metricGroups.map((group) => {
          const item = group.items[0];

          return (
            <CardShell
              key={group.key}
              title={item.title}
              tone={getMetricTone(group.key)}
            >
              <Skeleton
                loading={loading}
                active
                placeholder={
                  <Skeleton.Paragraph
                    active
                    rows={1}
                    style={{ width: '140px', height: '30px' }}
                  />
                }
              >
                <div className='dashboard-stat-card__value text-4xl font-semibold text-neutral-800 dark:text-neutral-100'>
                  {item.value}
                </div>
              </Skeleton>

              {item.subtitle ? (
                <p
                  className={`dashboard-stat-card__meta mt-3 ${
                    item.subtitleClassName || 'text-xs'
                  } text-neutral-500 dark:text-neutral-400`.trim()}
                >
                  {item.subtitle}
                </p>
              ) : null}

              {item.footer ? (
                <p className='mt-1 text-xs text-neutral-500 dark:text-neutral-400'>
                  {item.footer}
                </p>
              ) : null}
            </CardShell>
          );
        })}
      </div>
    );
  }

  const xlGridCols =
    groupedStatsData.length >= 5 ? 'xl:grid-cols-5' : 'xl:grid-cols-4';

  return (
    <div
      className={`grid grid-cols-1 gap-3 md:grid-cols-2 ${xlGridCols}`.trim()}
    >
      {groupedStatsData.map((group) => (
        <section
          key={group.key}
          className={`dashboard-stat-card rounded-[22px] bg-[var(--app-card)] p-5 shadow-[var(--app-shadow)]${
            group.key === 'account'
              ? ' max-h-[150px] overflow-hidden flex flex-col'
              : ''
          }`}
          data-tone={getMetricTone(group.key)}
        >
          <div className='dashboard-stat-card__title text-sm font-medium text-neutral-600 dark:text-neutral-300'>
            {group.title}
          </div>

          <div
            className={`mt-4 space-y-3${
              group.key === 'account'
                ? ' min-h-0 flex-1 overflow-y-auto card-content-scroll pr-1'
                : ''
            }`}
          >
            {group.items.map((item, itemIdx) => {
              const hasQuotaTable =
                item.quotaTable &&
                Array.isArray(item.quotaTable.headers) &&
                Array.isArray(item.quotaTable.values);
              const isSingleQuotaTable =
                hasQuotaTable &&
                item.quotaTable.headers.length === 1 &&
                item.quotaTable.values.length === 1;

              return (
                <div
                  key={itemIdx}
                  className='dashboard-stat-card__subitem relative overflow-hidden rounded-2xl bg-[var(--app-card-muted)] p-4'
                  role={item.onClick ? 'button' : undefined}
                  tabIndex={item.onClick ? 0 : undefined}
                  onClick={item.onClick}
                >
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
                        <div className='text-sm font-medium text-neutral-800 dark:text-neutral-100'>
                          {item.title}
                        </div>
                        {item.period ? (
                          <div className='text-[11px] leading-tight text-neutral-500 dark:text-neutral-400'>
                            ({item.period})
                          </div>
                        ) : null}
                      </div>
                      {item.subtitle ? (
                        <div
                          className={`mt-1 ${
                            item.subtitleClassName || 'text-xs'
                          } text-neutral-500 dark:text-neutral-400`.trim()}
                        >
                          {item.subtitle}
                        </div>
                      ) : null}
                    </div>

                    <Skeleton
                      loading={loading}
                      active
                      placeholder={
                        <Skeleton.Paragraph
                          active
                          rows={1}
                          style={{
                            width: hasQuotaTable ? '180px' : '90px',
                            height: '18px',
                          }}
                        />
                      }
                    >
                      {hasQuotaTable ? (
                        <div className='text-right'>
                          {isSingleQuotaTable ? (
                            <div className='min-h-9 flex items-center justify-end gap-1'>
                              <span className='text-[11px] text-neutral-500 dark:text-neutral-400'>
                                {item.quotaTable.headers[0]}
                              </span>
                              <span className='text-[11px] text-neutral-500 dark:text-neutral-400'>
                                {' : '}
                              </span>
                              <span className='text-xs font-medium text-neutral-800 dark:text-neutral-100'>
                                {item.quotaTable.values[0]}
                              </span>
                            </div>
                          ) : (
                            <>
                              <div
                                className='grid gap-2 text-[11px] text-neutral-500 dark:text-neutral-400'
                                style={{
                                  gridTemplateColumns: `repeat(${Math.max(
                                    item.quotaTable.headers.length || 1,
                                    item.quotaTable.values.length || 1,
                                  )}, minmax(0, 1fr))`,
                                }}
                              >
                                {item.quotaTable.headers.map((header, hIdx) => (
                                  <div key={hIdx} className='truncate'>
                                    {header}
                                  </div>
                                ))}
                              </div>
                              <div
                                className='mt-1 grid gap-2 text-xs font-medium text-neutral-800 dark:text-neutral-100'
                                style={{
                                  gridTemplateColumns: `repeat(${Math.max(
                                    item.quotaTable.headers.length || 1,
                                    item.quotaTable.values.length || 1,
                                  )}, minmax(0, 1fr))`,
                                }}
                              >
                                {item.quotaTable.values.map((val, vIdx) => (
                                  <div key={vIdx} className='truncate'>
                                    {val}
                                  </div>
                                ))}
                              </div>
                            </>
                          )}
                        </div>
                      ) : (
                        <div className='dashboard-stat-card__subvalue text-right text-sm font-semibold text-neutral-800 dark:text-neutral-100'>
                          {item.value}
                        </div>
                      )}
                    </Skeleton>
                  </div>
                  {item.isExpired ? (
                    <div className='pointer-events-none absolute inset-0 bg-neutral-200/75 dark:bg-black/45 backdrop-blur-[1px]'>
                      <div className='absolute right-3 bottom-2 origin-bottom-right rotate-12 text-lg font-bold text-[#5b1b1b] dark:text-[#7a2a2a]'>
                        {item.maskText || '已失效'}
                      </div>
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
};

export default StatsCards;
