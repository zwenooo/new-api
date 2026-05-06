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

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { API, modelToColor, renderQuotaToUSD, showError } from '../../helpers';
import ConsolePage from '../../components/layout/ConsolePage';
import { Avatar, Card, Empty, Select, Spin, Typography } from '@douyinfe/semi-ui';
import { getDiceBearAvatarUrl } from '../../helpers/avatar';

const { Title, Text } = Typography;

const podiumOrder = [0, 1, 2];

const renderUsdUsage = (quota) => renderQuotaToUSD(quota || 0);

const pad2 = (value) => String(value).padStart(2, '0');

const formatDateYMD = (date) => {
  const d = date instanceof Date ? date : new Date(date);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
};

const normalizeRankMode = (value) => {
  if (value === 'visible_quota') {
    return 'cost_quota';
  }
  return value || 'quota';
};

const RING_RADIUS = 40;
const RING_STROKE_WIDTH = 14;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;
const RING_SEGMENT_GAP = 1.5;
const modelQuotaRingHoverTarget = new EventTarget();
const MODEL_QUOTA_RING_HOVER_EVENT = 'model-quota-ring-hover';

const formatModelQuotaPercent = (ratio) => `${(ratio * 100).toFixed(2)}%`;

const formatSegmentTooltip = (segment, metric, countUnitLabel) => {
  if (metric === 'success_count') {
    return `${segment.modelName} · ${segment.successCount || 0} ${countUnitLabel} (${formatModelQuotaPercent(segment.ratio)})`;
  }
  return `${segment.modelName} · ${renderUsdUsage(segment.quota)} (${formatModelQuotaPercent(segment.ratio)})`;
};

const buildModelQuotaSegments = (modelQuota, metric) => {
  if (!Array.isArray(modelQuota) || modelQuota.length === 0) {
    return [];
  }

  const valueKey = metric === 'success_count' ? 'success_count' : 'quota';
  const rawSegments = modelQuota.filter(
    (item) => item?.model_name && (item?.[valueKey] || 0) > 0,
  );
  if (rawSegments.length === 0) {
    return [];
  }

  const totalValue = rawSegments.reduce((sum, item) => sum + (item?.[valueKey] || 0), 0);
  if (totalValue <= 0) {
    return [];
  }

  let rawOffset = 0;
  return rawSegments.map((item, index) => {
    const quota = item.quota || 0;
    const successCount = item.success_count || 0;
    const value = valueKey === 'success_count' ? successCount : quota;
    const ratio = value / totalValue;
    const rawLength = ratio * RING_CIRCUMFERENCE;
    const length = Math.max(0, rawLength - RING_SEGMENT_GAP);
    const segment = {
      segmentKey: `${item.model_name}-${index}`,
      modelName: item.model_name,
      quota,
      successCount,
      ratio,
      length,
      offset: rawOffset,
      color: modelToColor(item.model_name),
    };
    rawOffset += rawLength;
    return segment;
  });
};

const ModelQuotaRing = ({
  modelQuota,
  metric = 'quota',
  countUnitLabel = '',
  ringClassName = '',
  children,
}) => {
  const segments = useMemo(() => buildModelQuotaSegments(modelQuota, metric), [modelQuota, metric]);
  const ringRef = useRef(null);
  const ringIdRef = useRef(`model-quota-ring-${Math.random().toString(36).slice(2)}`);
  const [hoveredSegment, setHoveredSegment] = useState(null);
  const [tooltipPos, setTooltipPos] = useState(null);
  const [isRingHovered, setIsRingHovered] = useState(false);
  const hoveredSegmentKey = hoveredSegment?.segmentKey || '';
  const isSegmentActive = hoveredSegmentKey !== '';

  useEffect(() => {
    const handleRingHover = (event) => {
      if (event?.detail?.ringId === ringIdRef.current) {
        return;
      }
      setHoveredSegment(null);
      setIsRingHovered(false);
    };

    modelQuotaRingHoverTarget.addEventListener(
      MODEL_QUOTA_RING_HOVER_EVENT,
      handleRingHover,
    );

    return () => {
      modelQuotaRingHoverTarget.removeEventListener(
        MODEL_QUOTA_RING_HOVER_EVENT,
        handleRingHover,
      );
    };
  }, []);

  const activateRingHover = () => {
    modelQuotaRingHoverTarget.dispatchEvent(
      new CustomEvent(MODEL_QUOTA_RING_HOVER_EVENT, {
        detail: { ringId: ringIdRef.current },
      }),
    );
    setIsRingHovered(true);
  };
  const renderSegments = useMemo(() => {
    if (!hoveredSegmentKey) {
      return segments;
    }
    const normalSegments = segments.filter(
      (segment) => segment.segmentKey !== hoveredSegmentKey,
    );
    const activeSegments = segments.filter(
      (segment) => segment.segmentKey === hoveredSegmentKey,
    );
    return [...normalSegments, ...activeSegments];
  }, [segments, hoveredSegmentKey]);

  useLayoutEffect(() => {
    if (!hoveredSegment || !ringRef.current || typeof window === 'undefined') {
      setTooltipPos(null);
      return;
    }

    const rect = ringRef.current.getBoundingClientRect();
    setTooltipPos({
      x: rect.left + rect.width / 2,
      y: rect.top,
    });
  }, [hoveredSegmentKey]);

  return (
    <div
      ref={ringRef}
      className={`model-quota-ring relative inline-flex rounded-full box-content ${ringClassName}`.trim()}
      style={{
        transform: isSegmentActive || isRingHovered ? 'scale(1.1)' : 'scale(1)',
        transition: 'transform 200ms ease',
      }}
      onMouseEnter={activateRingHover}
      onMouseLeave={() => {
        setHoveredSegment(null);
        setIsRingHovered(false);
      }}
    >
      {typeof document !== 'undefined' &&
        hoveredSegment &&
        tooltipPos &&
        createPortal(
          <div
            className='pointer-events-none fixed z-[99999] whitespace-nowrap rounded-md border border-[color:var(--app-border)] bg-[color:var(--semi-color-bg-2)] px-2 py-1 text-[10px] leading-none text-semi-color-text-0 shadow-lg'
            style={{
              left: `${tooltipPos.x}px`,
              top: `${tooltipPos.y}px`,
              transform: 'translate(-50%, calc(-100% - 10px))',
            }}
          >
            {formatSegmentTooltip(hoveredSegment, metric, countUnitLabel)}
          </div>,
          document.body,
        )}
      <svg
        className='pointer-events-auto absolute inset-0 z-[2] h-full w-full overflow-visible'
        viewBox='0 0 100 100'
        aria-hidden='true'
        onMouseLeave={() => setHoveredSegment(null)}
      >
        <circle
          cx='50'
          cy='50'
          r={RING_RADIUS}
          fill='none'
          stroke='rgba(148, 163, 184, 0.16)'
          strokeWidth={RING_STROKE_WIDTH}
          style={{ pointerEvents: 'none' }}
        />
        {renderSegments.map((segment) => {
          const isHovered = hoveredSegmentKey === segment.segmentKey;
          return (
          <circle
            key={`${segment.segmentKey}-${segment.offset}`}
            className='cursor-pointer'
            cx='50'
            cy='50'
            r={RING_RADIUS}
            fill='none'
            stroke={segment.color}
            strokeWidth={isHovered ? RING_STROKE_WIDTH + 4 : RING_STROKE_WIDTH}
            strokeDasharray={`${segment.length} ${RING_CIRCUMFERENCE}`}
            strokeDashoffset={-segment.offset}
            strokeLinecap='butt'
            opacity={hoveredSegmentKey && !isHovered ? 0.72 : 1}
            transform='rotate(-90 50 50)'
            onMouseEnter={() => setHoveredSegment(segment)}
            onMouseLeave={() => setHoveredSegment(null)}
            style={{
              transition: 'stroke-width 180ms ease, opacity 180ms ease, filter 180ms ease',
              filter: isHovered
                ? `drop-shadow(0 0 4px ${segment.color}) drop-shadow(0 0 10px ${segment.color})`
                : 'none',
            }}
          >
            <title>{formatSegmentTooltip(segment, metric, countUnitLabel)}</title>
          </circle>
          );
        })}
      </svg>
      <div className='pointer-events-none relative z-[1] inline-flex h-full w-full items-center justify-center'>
        {children}
      </div>
    </div>
  );
};

const StompKing = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState([]);
  const [selectedDate, setSelectedDate] = useState(() => formatDateYMD(new Date()));
  const [rankMode, setRankMode] = useState('cost_quota');

  const fetchRank = async () => {
    setLoading(true);
    try {
      const normalizedRankMode = normalizeRankMode(rankMode);
      const res = await API.get('/api/log/king_rank', {
        params: { date: selectedDate, rank_mode: normalizedRankMode },
      });
      if (!res.data?.success) {
        showError(res.data?.message || t('获取排行失败'));
        setItems([]);
        return;
      }
      setRankMode(normalizeRankMode(String(res.data?.rank_mode || 'cost_quota')));
      const list = Array.isArray(res.data.data) ? res.data.data : [];
      setItems(list);
    } catch (error) {
      showError(error?.message || t('获取排行失败'));
      setItems([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRank();
  }, [selectedDate, rankMode]);

  const dateOptions = useMemo(() => {
    const now = new Date();
    const options = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date(now);
      d.setDate(now.getDate() - i);
      const ymd = formatDateYMD(d);
      const label =
        i === 0
          ? `${t('今日')} (${ymd})`
          : i === 1
            ? `${t('昨天')} (${ymd})`
            : ymd;
      options.push({ value: ymd, label });
    }
    return options;
  }, [t]);

  const podiumItems = useMemo(() => items.slice(0, 3), [items]);

  const listItems = useMemo(() => items.slice(3, 10), [items]);
  const hasData = items.length > 0;
  const isRankBySuccessCount = rankMode === 'success_count';
  const isRankByActualFee = rankMode === 'quota';
  const isRankByStandardFee = rankMode === 'cost_quota';
  const countUnitLabel = t('次');
  const rankModeOptions = useMemo(
    () => [
      { value: 'cost_quota', label: t('标准费用') },
      { value: 'quota', label: t('实际费用') },
      { value: 'success_count', label: t('成功次数') },
    ],
    [t],
  );
  const metricText = useMemo(() => {
    if (isRankBySuccessCount) {
      return t('当日全站成功请求次数前十名');
    }
    if (isRankByStandardFee) {
      return t('当日全站标准费用前十名');
    }
    if (isRankByActualFee) {
      return t('当日全站实际费用前十名');
    }
    return t('当日全站标准费用前十名');
  }, [isRankByActualFee, isRankByStandardFee, isRankBySuccessCount, t]);

  const renderMetricValue = (item) => {
    if (!item) {
      return '';
    }
    if (isRankBySuccessCount) {
      return `${item.success_count || 0} ${countUnitLabel}`;
    }
    return renderUsdUsage(item.quota);
  };

  return (
    <ConsolePage fillHeight>
      <div className='flex min-h-0 flex-1 flex-col gap-4'>
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-2'>
            <Title heading={4} className='!mb-0'>
              {t('谁是蹬王')}
            </Title>
            <Text type='secondary'>{metricText}</Text>
          </div>
          <div className='flex items-center gap-2'>
            <Text type='secondary'>{t('口径')}:</Text>
            <Select
              value={rankMode}
              onChange={setRankMode}
              size='small'
              className='w-[140px]'
              showClear={false}
              optionList={rankModeOptions}
            />
            <Text type='secondary'>{t('日期')}:</Text>
            <Select
              value={selectedDate}
              onChange={setSelectedDate}
              size='small'
              className='w-[160px]'
              showClear={false}
            >
              {dateOptions.map((opt) => (
                <Select.Option key={opt.value} value={opt.value}>
                  {opt.label}
                </Select.Option>
              ))}
            </Select>
          </div>
        </div>

        <Card
          className='!rounded-2xl !border-0 !shadow-none !bg-transparent'
          style={{ background: 'transparent' }}
          bodyStyle={{ background: 'transparent' }}
        >
          <div className='flex items-center justify-end'>
            {loading && <Spin size='small' />}
          </div>
          {!hasData && !loading ? (
            <Empty description={t('暂无排行数据')} className='py-10' />
          ) : podiumItems.length > 0 ? (
            <div className='relative mt-4 stomp-king-podium'>
              <style>{`
                  .stomp-king-podium {
                    color: #0f172a;
                  }
                  html.dark .stomp-king-podium {
                    color: #e2e8f0;
                  }
                  .stomp-king-podium .float-anim {
                    animation: float 4s ease-in-out infinite;
                  }
                  @keyframes float {
                    0%,
                    100% {
                      transform: translateY(0);
                    }
                    50% {
                      transform: translateY(-12px);
                    }
                  }
                  .stomp-king-podium .gold-glow {
                    box-shadow: none;
                  }
                  .stomp-king-podium .silver-glow {
                    box-shadow: none;
                  }
                  .stomp-king-podium .bronze-glow {
                    box-shadow: none;
                  }
                  .stomp-king-podium .podium-user-slot:hover .float-anim {
                    animation-play-state: paused;
                  }
              `}</style>
              <div className='flex items-end justify-center w-full max-w-5xl mx-auto mb-3 px-4 gap-2 md:gap-4'>
                {podiumItems[1] && (
                  <div className='podium-user-slot flex flex-col items-center w-1/3 max-w-[180px]'>
                    <div className='float-anim mb-1 text-center' style={{ animationDelay: '-0.5s' }}>
                      <div className='mb-1 font-bold text-slate-700 dark:text-slate-300 text-[11px] md:text-xs'>
                        {podiumItems[1].username}
                      </div>
                      <div className='relative inline-block group'>
                        <ModelQuotaRing
                          modelQuota={podiumItems[1].model_quota}
                          metric={rankMode}
                          countUnitLabel={countUnitLabel}
                          ringClassName='p-[2px] md:p-[3px] silver-glow w-8 h-8 md:w-12 md:h-12'
                        >
                          <div className='w-full h-full rounded-full overflow-hidden bg-slate-800'>
                            <img
                              src={getDiceBearAvatarUrl(podiumItems[1].avatar_seed, { size: 200 })}
                              className='w-full h-full object-cover'
                              alt='Silver'
                            />
                          </div>
                        </ModelQuotaRing>
                      </div>
                      <div className='text-[10px] text-slate-500 dark:text-slate-400'>
                        {renderMetricValue(podiumItems[1])}
                      </div>
                    </div>
                    <div className='w-full h-12 md:h-16 bg-slate-500 dark:bg-slate-700 rounded-t-xl flex flex-col items-center pt-2 border-t border-x border-slate-300/20'>
                      <span className='text-2xl md:text-3xl font-black text-slate-100/70 dark:text-white/10 select-none'>
                        2
                      </span>
                    </div>
                  </div>
                )}

                {podiumItems[0] && (
                  <div className='podium-user-slot flex flex-col items-center w-1/3 max-w-[200px] relative'>
                    <div className='float-anim mb-2 text-center'>
                      <div className='mb-2 font-bold text-amber-700 dark:text-yellow-100 text-xs md:text-sm'>
                        {podiumItems[0].username}
                      </div>
                      <div className='relative inline-block group'>
                        <ModelQuotaRing
                          modelQuota={podiumItems[0].model_quota}
                          metric={rankMode}
                          countUnitLabel={countUnitLabel}
                          ringClassName='p-[3px] md:p-[4px] w-10 h-10 md:w-16 md:h-16'
                        >
                          <div className='w-full h-full rounded-full overflow-hidden bg-slate-800'>
                            <img
                              src={getDiceBearAvatarUrl(podiumItems[0].avatar_seed, { size: 240 })}
                              className='w-full h-full object-cover'
                              alt='Gold'
                            />
                          </div>
                        </ModelQuotaRing>
                      </div>
                      <div className='text-[10px] text-amber-600 dark:text-yellow-100/80'>
                        {renderMetricValue(podiumItems[0])}
                      </div>
                    </div>
                    <div className='w-full h-18 md:h-24 bg-amber-500 dark:bg-amber-700 rounded-t-xl flex flex-col items-center pt-3 border-t border-x border-yellow-300/30 relative'>
                      <span className='text-4xl md:text-5xl font-black text-amber-50/70 dark:text-white/10 select-none'>
                        1
                      </span>
                    </div>
                  </div>
                )}

                {podiumItems[2] && (
                  <div className='podium-user-slot flex flex-col items-center w-1/3 max-w-[180px]'>
                    <div className='float-anim mb-1 text-center' style={{ animationDelay: '-1.2s' }}>
                      <div className='mb-1 font-bold text-amber-700 dark:text-amber-200 text-[10px] md:text-[11px]'>
                        {podiumItems[2].username}
                      </div>
                      <div className='relative inline-block group'>
                        <ModelQuotaRing
                          modelQuota={podiumItems[2].model_quota}
                          metric={rankMode}
                          countUnitLabel={countUnitLabel}
                          ringClassName='p-[2px] md:p-[3px] bronze-glow w-7 h-7 md:w-11 md:h-11'
                        >
                          <div className='w-full h-full rounded-full overflow-hidden bg-slate-800'>
                            <img
                              src={getDiceBearAvatarUrl(podiumItems[2].avatar_seed, { size: 200 })}
                              className='w-full h-full object-cover'
                              alt='Bronze'
                            />
                          </div>
                        </ModelQuotaRing>
                      </div>
                      <div className='text-[10px] text-amber-600 dark:text-amber-200/80'>
                        {renderMetricValue(podiumItems[2])}
                      </div>
                    </div>
                    <div className='w-full h-10 md:h-14 bg-amber-700 dark:bg-amber-900 rounded-t-xl flex flex-col items-center pt-1.5 border-t border-x border-amber-600/20'>
                      <span className='text-xl md:text-2xl font-black text-amber-50/65 dark:text-white/5 select-none'>
                        3
                      </span>
                    </div>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <Empty description={t('暂无前三名数据')} className='py-10' />
          )}



        </Card>

        <Card
          className='!rounded-2xl !border-0 !shadow-none !bg-transparent flex min-h-0 flex-1 flex-col'
          style={{ background: 'transparent' }}
          bodyStyle={{
            background: 'transparent',
            display: 'flex',
            flexDirection: 'column',
            flex: 1,
            minHeight: 0,
          }}
        >
          <div className='flex items-center justify-between'>
            <Text className='text-sm text-semi-color-text-2'>
              {t('第 4-10 名')}
            </Text>
            {loading && <Spin size='small' />}
          </div>
          {!hasData && !loading ? (
            <div className='mt-4 flex min-h-0 flex-1 items-center justify-center'>
              <Empty description={t('暂无排行数据')} />
            </div>
          ) : (
            <div className='mt-4 min-h-0 flex-1 overflow-y-auto card-content-scroll pr-1'>
              <div className='flex flex-col gap-2'>
	                {listItems.map((item, idx) => (
	                  <div
	                    key={item.user_id}
	                    className='flex justify-center rounded-xl px-3 py-1'
	                  >
	                    <div className='flex w-full max-w-2xl items-center justify-between'>
	                      <div className='flex items-center gap-1.5'>
	                        <div className='flex h-6 w-6 items-center justify-center rounded-full bg-[color:var(--semi-color-fill-0)] text-[11px] font-semibold text-semi-color-text-1'>
	                          {idx + 4}
	                        </div>
                        <ModelQuotaRing
                          modelQuota={item.model_quota}
                          metric={rankMode}
                          countUnitLabel={countUnitLabel}
                          ringClassName='p-[2px]'
                        >
                          <Avatar
                            size='small'
                            src={getDiceBearAvatarUrl(item.avatar_seed, { size: 64 })}
                          >
                            {item.username?.[0]?.toUpperCase() || '?'}
                          </Avatar>
                        </ModelQuotaRing>
                        <Text className='font-medium'>{item.username}</Text>
                      </div>
                      <Text type='secondary'>{renderMetricValue(item)}</Text>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </Card>
      </div>
    </ConsolePage>
  );
};

export default StompKing;
