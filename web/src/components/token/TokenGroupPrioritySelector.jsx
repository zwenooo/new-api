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

import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Button, Empty, Tag } from '@douyinfe/semi-ui';
import {
  Check,
  ChevronDown,
  ChevronUp,
  GripVertical,
  Plus,
  Trash2,
} from 'lucide-react';

export const normalizeTokenGroupIds = (rawIds) => {
  let source = rawIds;
  if (typeof source === 'string') {
    const trimmed = source.trim();
    if (!trimmed) return [];
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      try {
        source = JSON.parse(trimmed);
      } catch (_) {
        source = trimmed.split(',');
      }
    } else {
      source = trimmed.split(',');
    }
  } else if (source === null || source === undefined) {
    return [];
  } else if (!Array.isArray(source)) {
    source = [source];
  }

  if (!Array.isArray(source)) return [];
  const seen = new Set();
  const out = [];
  source.forEach((raw) => {
    const num = Number(raw);
    if (!Number.isFinite(num) || num <= 0) return;
    const id = Math.floor(num);
    if (id <= 0 || seen.has(id)) return;
    seen.add(id);
    out.push(id);
  });
  return out;
};

export const mergeTokenGroupOptions = (
  options,
  selectedIds,
  getFallbackLabel,
) => {
  const merged = [];
  const seen = new Set();

  const pushOption = (option) => {
    const raw = Number(option?.value ?? 0);
    const value = Number.isFinite(raw) ? Math.floor(raw) : 0;
    if (value <= 0 || seen.has(value)) return;
    seen.add(value);
    merged.push({
      ...option,
      value,
      label: String(option?.label ?? '').trim() || `#${value}`,
      desc: String(option?.desc ?? '').trim(),
    });
  };

  (Array.isArray(options) ? options : []).forEach(pushOption);

  normalizeTokenGroupIds(selectedIds).forEach((groupId) => {
    if (seen.has(groupId)) return;
    seen.add(groupId);
    const fallbackLabel =
      typeof getFallbackLabel === 'function'
        ? String(getFallbackLabel(groupId) ?? '').trim()
        : '';
    merged.push({
      value: groupId,
      label: fallbackLabel || `#${groupId}`,
      desc: '',
      ratio: null,
      disabled: true,
    });
  });

  return merged;
};

const normalizeSelectableGroupOptions = (options) => {
  const normalized = [];
  const seen = new Set();

  (Array.isArray(options) ? options : []).forEach((option) => {
    const raw = Number(option?.value ?? 0);
    const value = Number.isFinite(raw) ? Math.floor(raw) : 0;
    if (value <= 0 || seen.has(value)) return;
    const label = String(option?.label ?? '').trim();
    if (!label) return;
    seen.add(value);
    normalized.push({
      ...option,
      value,
      label,
      desc: String(option?.desc ?? '').trim(),
      billable: Boolean(option?.billable),
      no_billing: Boolean(option?.no_billing),
    });
  });

  return normalized;
};

const hasRatio = (ratio) => {
  if (ratio === null || ratio === undefined) return false;
  return String(ratio).trim() !== '';
};

const areGroupListsEqual = (left, right) => {
  const l = normalizeTokenGroupIds(left);
  const r = normalizeTokenGroupIds(right);
  if (l.length !== r.length) return false;
  for (let i = 0; i < l.length; i += 1) {
    if (l[i] !== r[i]) return false;
  }
  return true;
};

const reorderGroupIdsBefore = (ids, sourceId, targetId) => {
  const normalized = normalizeTokenGroupIds(ids);
  const src = Number(sourceId);
  const dst = Number(targetId);
  if (!Number.isFinite(src) || !Number.isFinite(dst) || src <= 0 || dst <= 0) {
    return normalized;
  }
  if (src === dst) return normalized;

  const next = normalized.filter((item) => item !== src);
  const targetIndex = next.findIndex((item) => item === dst);
  if (targetIndex < 0) return normalized;
  next.splice(targetIndex, 0, src);
  return next;
};

const moveGroupIdsToEnd = (ids, sourceId) => {
  const normalized = normalizeTokenGroupIds(ids);
  const src = Number(sourceId);
  if (!Number.isFinite(src) || src <= 0) return normalized;
  const next = normalized.filter((item) => item !== src);
  if (next.length === normalized.length) return normalized;
  next.push(src);
  return next;
};

const panelClassName =
  'relative flex min-h-[240px] md:min-h-0 flex-col overflow-hidden rounded-[14px] bg-slate-200/70 shadow-sm dark:bg-[#262b33]';

const panelHeaderClassName = 'bg-transparent px-3.5 py-2.5 dark:bg-transparent';

const dividerClassName = 'w-px self-stretch rounded-full bg-slate-300/90 dark:bg-slate-700';

const scrollAreaClassName = 'scrollbar-hide min-h-0 flex-1 overflow-y-auto px-3.5 py-3';

const priorityBadgeClassName =
  'flex h-9 w-9 shrink-0 items-center justify-center rounded-[10px] bg-white text-sm font-semibold text-slate-700 dark:bg-[#4b5563] dark:text-slate-100';

const selectedActionClassName =
  'flex shrink-0 items-center gap-0.5 rounded-[10px] bg-white px-1 py-1 dark:bg-[#4b5563]';

const dragHandleClassName =
  'flex h-7 w-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-md text-slate-500 active:cursor-grabbing dark:text-slate-300';

const overlayActionIconClassName =
  'flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-slate-400 dark:text-slate-400';

const TokenGroupPrioritySelector = ({
  t,
  options = [],
  value = [],
  onChange,
  loading = false,
  disabled = false,
  title,
  description,
  availableTitle,
  availableDescription,
  selectedTitle,
  selectedDescription,
  availableEmptyText,
  selectedEmptyText,
  getFallbackLabel,
  className,
  preferSideBySide = false,
  heightVariant = 'default',
}) => {
  const propSelectedIds = useMemo(() => normalizeTokenGroupIds(value), [value]);
  const [localSelectedIds, setLocalSelectedIds] = useState(propSelectedIds);
  const [dragState, setDragState] = useState(null);
  const [dropTargetId, setDropTargetId] = useState(0);

  const selectedIdsRef = useRef(propSelectedIds);
  const pendingCommitRef = useRef(null);
  const dragStateRef = useRef(null);
  const dragPointerRef = useRef(null);
  const overlayRef = useRef(null);
  const overlayFrameRef = useRef(0);
  const cardRefs = useRef(new Map());
  const layoutSnapshotRef = useRef(new Map());

  const selectedSet = useMemo(() => new Set(localSelectedIds), [localSelectedIds]);

  const selectableOptions = useMemo(
    () => normalizeSelectableGroupOptions(options),
    [options],
  );

  const mergedOptions = useMemo(
    () =>
      mergeTokenGroupOptions(options, localSelectedIds, (groupId) => {
        const fallbackLabel =
          typeof getFallbackLabel === 'function' ? getFallbackLabel(groupId) : '';
        return String(fallbackLabel ?? '').trim() || `${t('未知分组')} (#${groupId})`;
      }),
    [getFallbackLabel, localSelectedIds, options, t],
  );

  const groupMetaById = useMemo(() => {
    const map = new Map();
    mergedOptions.forEach((option) => {
      map.set(Number(option.value), option);
    });
    return map;
  }, [mergedOptions]);

  const availableGroups = useMemo(() => selectableOptions, [selectableOptions]);

  const selectedGroups = useMemo(
    () =>
      localSelectedIds.map((groupId) =>
        groupMetaById.get(groupId) || {
          value: groupId,
          label: `${t('未知分组')} (#${groupId})`,
          desc: '',
          ratio: null,
          disabled: true,
        },
      ),
    [groupMetaById, localSelectedIds, t],
  );

  const draggingGroup = dragState ? groupMetaById.get(dragState.id) : null;
  const draggingIndex = dragState
    ? localSelectedIds.findIndex((groupId) => groupId === dragState.id)
    : -1;

  const syncLocalSelection = (nextIds) => {
    const normalized = normalizeTokenGroupIds(nextIds);
    selectedIdsRef.current = normalized;
    setLocalSelectedIds(normalized);
    return normalized;
  };

  const emitSelection = (nextIds) => {
    onChange?.(normalizeTokenGroupIds(nextIds));
  };

  const updateSelection = (nextIds, { emit = true } = {}) => {
    const normalized = syncLocalSelection(nextIds);
    if (emit) {
      emitSelection(normalized);
    }
    return normalized;
  };

  const setCardRef = (groupId, node) => {
    if (node) {
      cardRefs.current.set(groupId, node);
    } else {
      cardRefs.current.delete(groupId);
    }
  };

  const clearOverlayFrame = () => {
    if (overlayFrameRef.current) {
      cancelAnimationFrame(overlayFrameRef.current);
      overlayFrameRef.current = 0;
    }
  };

  const applyOverlayTransform = () => {
    const overlayNode = overlayRef.current;
    const currentDrag = dragStateRef.current;
    const pointer = dragPointerRef.current;
    if (!overlayNode || !currentDrag || !pointer) return;

    const x = Math.round(pointer.x - currentDrag.offsetX);
    const y = Math.round(pointer.y - currentDrag.offsetY);

    overlayNode.style.transform = `translate3d(${x}px, ${y}px, 0) scale(1.01)`;
  };

  const scheduleOverlayTransform = () => {
    if (overlayFrameRef.current) return;
    overlayFrameRef.current = requestAnimationFrame(() => {
      overlayFrameRef.current = 0;
      applyOverlayTransform();
    });
  };

  const clearDragState = () => {
    clearOverlayFrame();
    dragStateRef.current = null;
    dragPointerRef.current = null;
    setDragState(null);
    setDropTargetId(0);
  };

  const addGroup = (groupId) => {
    if (disabled || loading) return;
    const id = Number(groupId);
    if (!Number.isFinite(id) || id <= 0) return;
    const normalizedId = Math.floor(id);
    const current = selectedIdsRef.current;
    if (current.includes(normalizedId)) return;
    updateSelection([...current, normalizedId]);
  };

  const removeGroup = (groupId) => {
    if (disabled || loading || dragStateRef.current) return;
    const id = Number(groupId);
    if (!Number.isFinite(id) || id <= 0) return;
    updateSelection(selectedIdsRef.current.filter((item) => item !== Math.floor(id)));
  };

  const moveByOffset = (groupId, offset) => {
    if (disabled || loading || dragStateRef.current) return;
    const id = Number(groupId);
    const current = selectedIdsRef.current;
    const index = current.findIndex((item) => item === id);
    if (index < 0) return;
    const targetIndex = index + offset;
    if (targetIndex < 0 || targetIndex >= current.length) return;
    const next = [...current];
    const [item] = next.splice(index, 1);
    next.splice(targetIndex, 0, item);
    updateSelection(next);
  };

  const previewReorder = (clientY) => {
    const activeDragState = dragStateRef.current;
    if (!activeDragState) return;
    const current = selectedIdsRef.current;
    const others = current.filter((item) => item !== activeDragState.id);

    let targetId = 0;
    for (const candidateId of others) {
      const node = cardRefs.current.get(candidateId);
      if (!node) continue;
      const rect = node.getBoundingClientRect();
      if (clientY < rect.top + rect.height / 2) {
        targetId = candidateId;
        break;
      }
    }

    const next = targetId
      ? reorderGroupIdsBefore(current, activeDragState.id, targetId)
      : moveGroupIdsToEnd(current, activeDragState.id);

    setDropTargetId(targetId);

    if (!areGroupListsEqual(current, next)) {
      syncLocalSelection(next);
    }
  };

  const startPointerDrag = (event, groupId) => {
    if (disabled || loading || event.button !== 0) return;

    const id = Number(groupId);
    if (!Number.isFinite(id) || id <= 0) return;

    const node = cardRefs.current.get(id);
    if (!node) return;

    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture?.(event.pointerId);

    const rect = node.getBoundingClientRect();
    const nextDragState = {
      id,
      pointerId: event.pointerId,
      width: rect.width,
      height: rect.height,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
    };

    pendingCommitRef.current = selectedIdsRef.current;
    dragStateRef.current = nextDragState;
    dragPointerRef.current = { x: event.clientX, y: event.clientY };
    setDropTargetId(id);
    setDragState(nextDragState);
  };

  useEffect(() => {
    dragStateRef.current = dragState;
    if (dragState) {
      scheduleOverlayTransform();
    }
  }, [dragState]);

  useEffect(() => {
    if (dragState || areGroupListsEqual(propSelectedIds, selectedIdsRef.current)) return;
    syncLocalSelection(propSelectedIds);
  }, [dragState, propSelectedIds]);

  useEffect(() => {
    if (!dragState) return undefined;

    const previousUserSelect = document.body.style.userSelect;
    const previousCursor = document.body.style.cursor;
    document.body.style.userSelect = 'none';
    document.body.style.cursor = 'grabbing';

    const handlePointerMove = (event) => {
      const activeDragState = dragStateRef.current;
      if (!activeDragState) return;
      if (
        activeDragState.pointerId !== undefined &&
        event.pointerId !== undefined &&
        activeDragState.pointerId !== event.pointerId
      ) {
        return;
      }

      dragPointerRef.current = { x: event.clientX, y: event.clientY };
      scheduleOverlayTransform();
      previewReorder(event.clientY);
    };

    const finishDrag = (event) => {
      const activeDragState = dragStateRef.current;
      if (
        activeDragState?.pointerId !== undefined &&
        event?.pointerId !== undefined &&
        activeDragState.pointerId !== event.pointerId
      ) {
        return;
      }

      const next = normalizeTokenGroupIds(selectedIdsRef.current);
      const prev = normalizeTokenGroupIds(pendingCommitRef.current || propSelectedIds);
      if (!areGroupListsEqual(next, prev)) {
        emitSelection(next);
      }
      pendingCommitRef.current = null;
      clearDragState();
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', finishDrag);
    window.addEventListener('pointercancel', finishDrag);

    return () => {
      document.body.style.userSelect = previousUserSelect;
      document.body.style.cursor = previousCursor;
      clearOverlayFrame();
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', finishDrag);
      window.removeEventListener('pointercancel', finishDrag);
    };
  }, [dragState, propSelectedIds]);

  useLayoutEffect(() => {
    const nextSnapshot = new Map();
    localSelectedIds.forEach((groupId) => {
      const node = cardRefs.current.get(groupId);
      if (!node) return;
      nextSnapshot.set(groupId, node.getBoundingClientRect());
    });

    nextSnapshot.forEach((nextRect, groupId) => {
      if (dragStateRef.current?.id === groupId) return;
      const prevRect = layoutSnapshotRef.current.get(groupId);
      const node = cardRefs.current.get(groupId);
      if (!prevRect || !node) return;

      const deltaY = prevRect.top - nextRect.top;
      if (Math.abs(deltaY) < 1) return;

      node.style.transition = 'none';
      node.style.transform = `translate3d(0, ${deltaY}px, 0) scale(0.992)`;
      node.style.willChange = 'transform';
      node.getBoundingClientRect();
      requestAnimationFrame(() => {
        node.style.transition =
          'transform 500ms cubic-bezier(0.16, 1, 0.3, 1), box-shadow 260ms ease, opacity 220ms ease';
        node.style.transform = 'translate3d(0, 0, 0) scale(1)';
      });
    });

    layoutSnapshotRef.current = nextSnapshot;
  }, [localSelectedIds]);

  const dragOverlay =
    dragState && draggingGroup && typeof document !== 'undefined'
      ? createPortal(
          <div
            ref={overlayRef}
            className='pointer-events-none fixed left-0 top-0 z-[9999] will-change-transform'
            style={{ width: dragState.width }}
          >
            <div className='relative overflow-hidden rounded-[14px] bg-white p-3 shadow-[0_18px_42px_-24px_rgba(15,23,42,0.28)] dark:bg-[#3b4350]'>
              <div className='flex items-start gap-3'>
                <div className={priorityBadgeClassName}>
                  {String((draggingIndex >= 0 ? draggingIndex : 0) + 1).padStart(2, '0')}
                </div>

                <div className='min-w-0 flex-1'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0 flex-1'>
                      <div className='flex flex-wrap items-center gap-2'>
                        <div className='truncate text-sm font-medium text-slate-900 dark:text-slate-50'>
                          {draggingGroup.label}
                        </div>
                        {hasRatio(draggingGroup?.ratio) ? (
                          <Tag size='small' color='blue'>
                            x{draggingGroup.ratio}
                          </Tag>
                        ) : null}
                        {draggingGroup.disabled ? (
                          <Tag size='small' color='grey'>
                            {t('历史分组')}
                          </Tag>
                        ) : null}
                      </div>

                      {draggingGroup?.desc ? (
                        <div className='mt-0.5 line-clamp-1 text-xs leading-5 text-slate-500 dark:text-slate-300'>
                          {draggingGroup.desc}
                        </div>
                      ) : (
                        <div className='mt-0.5 text-xs text-slate-400 dark:text-slate-500'>
                          {t('暂无描述')}
                        </div>
                      )}
                    </div>

                    <div className={selectedActionClassName}>
                      <div className={overlayActionIconClassName}>
                        <ChevronUp size={15} />
                      </div>
                      <div className={overlayActionIconClassName}>
                        <ChevronDown size={15} />
                      </div>
                      <div className={`${overlayActionIconClassName} text-rose-400 dark:text-rose-300`}>
                        <Trash2 size={15} />
                      </div>
                      <div className={dragHandleClassName}>
                        <GripVertical size={15} />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>,
          document.body,
        )
      : null;

  const heightClassName =
    heightVariant === 'fill'
      ? 'h-full flex-1'
      : heightVariant === 'compact'
        ? 'h-[min(520px,calc(100vh-300px))]'
        : 'h-[min(620px,calc(100vh-260px))]';

  const rootClassName = [
    'flex max-h-full min-h-0 flex-col overflow-hidden',
    heightClassName,
    className,
  ]
    .filter(Boolean)
    .join(' ');

  const showShellHeader = Boolean(title || description);

  const gridClassName = preferSideBySide
    ? 'grid min-h-0 flex-1 gap-4 md:grid-cols-[minmax(0,0.98fr)_1px_minmax(0,1.08fr)]'
    : 'grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(0,0.98fr)_1px_minmax(0,1.08fr)]';

  const dividerVisibilityClassName = preferSideBySide ? 'hidden md:block' : 'hidden xl:block';

  return (
    <div className={rootClassName}>
      {showShellHeader ? (
        <div className='mb-4'>
          {title ? (
            <div className='text-[15px] font-semibold tracking-[0.01em] text-slate-900 dark:text-slate-50'>
              {title}
            </div>
          ) : null}
          {description ? (
            <div className={`${title ? 'mt-1 ' : ''}text-xs leading-5 text-slate-500 dark:text-slate-300`}>
              {description}
            </div>
          ) : null}
        </div>
      ) : null}

      <div className={gridClassName}>
        <div className={panelClassName}>
          <div className={panelHeaderClassName}>
            <div>
              <div className='text-sm font-semibold text-slate-900 dark:text-slate-50'>
                {availableTitle || t('可选分组')}
              </div>
              {availableDescription ? (
                <div className='mt-1 text-xs text-slate-500 dark:text-slate-300'>
                  {availableDescription}
                </div>
              ) : null}
            </div>
          </div>

          <div className={scrollAreaClassName}>
            {availableGroups.length === 0 ? (
              <div className='flex h-full min-h-[230px] items-center justify-center rounded-[14px] bg-white dark:bg-[#343b46]'>
                <Empty
                  title={loading ? t('加载中') : t('没有更多可选分组')}
                  description={
                    availableEmptyText || t('暂无可选分组')
                  }
                />
              </div>
            ) : (
              <div className='space-y-2.5'>
                {availableGroups.map((group) => {
                  const groupId = Number(group.value);
                  const isAlreadySelected = selectedSet.has(groupId);
                  return (
                    <div
                      key={groupId}
                      className={`rounded-[14px] p-3 shadow-sm ${
                        isAlreadySelected
                          ? 'bg-emerald-100 dark:bg-[#214a35]'
                          : 'bg-white dark:bg-[#39414e]'
                      }`}
                    >
                      <div className='flex items-start justify-between gap-3'>
                        <div className='min-w-0 flex-1'>
                          <div className='flex flex-wrap items-center gap-2'>
                            <div className='truncate text-sm font-semibold text-slate-900 dark:text-slate-50'>
                              {group.label}
                            </div>
                            {hasRatio(group?.ratio) ? (
                              <Tag size='small' color='blue'>
                                x{group.ratio}
                              </Tag>
                            ) : null}
                            {group.no_billing ? (
                              <Tag size='small' color='orange'>
                                {t('不计费')}
                              </Tag>
                            ) : (
                              <Tag size='small' color={group.billable ? 'green' : 'grey'}>
                                {group.billable ? t('可消费') : t('暂无资费')}
                              </Tag>
                            )}
                          </div>

                          {group?.desc ? (
                            <div className='mt-1 line-clamp-1 text-xs leading-5 text-slate-500 dark:text-slate-300'>
                              {group.desc}
                            </div>
                          ) : (
                            <div className='mt-1 text-xs text-slate-400 dark:text-slate-500'>
                              {t('暂无描述')}
                            </div>
                          )}
                        </div>

                        <Button
                          size='small'
                          theme='light'
                          type='primary'
                          className={`!rounded-lg !px-3 ${
                            isAlreadySelected
                              ? '!bg-emerald-50 !text-emerald-600 dark:!bg-emerald-400/10 dark:!text-emerald-200'
                              : ''
                          }`}
                          disabled={disabled || loading || isAlreadySelected}
                          onClick={() => addGroup(groupId)}
                        >
                          <span className='inline-flex items-center gap-1'>
                            {isAlreadySelected ? <Check size={14} /> : <Plus size={14} />}
                            {isAlreadySelected ? t('已添加') : t('加入')}
                          </span>
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        <div className={dividerVisibilityClassName}>
          <div className={dividerClassName} />
        </div>

        <div className={panelClassName}>
          <div className={panelHeaderClassName}>
            <div>
              <div className='flex items-center gap-2 text-sm'>
                <div className='font-semibold text-slate-900 dark:text-slate-50'>
                  {selectedTitle || t('选中分组')}
                </div>
                <div className='text-xs text-slate-500 dark:text-slate-300'>
                  {t('（越靠前消费优先级越高）')}
                </div>
              </div>
              {selectedDescription ? (
                <div className='mt-1 text-xs text-slate-500 dark:text-slate-300'>
                  {selectedDescription}
                </div>
              ) : null}
            </div>
          </div>

          <div className={scrollAreaClassName}>
            {selectedGroups.length === 0 ? (
              <div className='flex h-full min-h-[230px] items-center justify-center rounded-[14px] bg-slate-50/70 dark:bg-[#343b46]'>
                <Empty
                  title={t('尚未选择分组')}
                  description={
                    selectedEmptyText || t('从左侧添加分组')
                  }
                />
              </div>
            ) : (
              <div className='space-y-3'>
                {selectedGroups.map((group, index) => {
                  const groupId = Number(group.value);
                  const isDragging = dragState?.id === groupId;
                  const isDropTarget =
                    Boolean(dragState) && dragState.id !== groupId && dropTargetId === groupId;

                  return (
                    <div
                      key={groupId}
                      ref={(node) => setCardRef(groupId, node)}
                      className={`relative overflow-hidden rounded-[14px] p-3 transition-[box-shadow,background-color,opacity,transform] duration-200 ${
                        isDropTarget
                          ? 'bg-sky-100 shadow-sm dark:bg-[#21546d]'
                          : 'bg-slate-50 shadow-sm dark:bg-[#3b4350]'
                      } ${isDragging ? 'opacity-0' : 'opacity-100'}`}
                    >
                      <div className='flex items-start gap-3'>
                        <div className={priorityBadgeClassName}>
                          {String(index + 1).padStart(2, '0')}
                        </div>

                        <div className='min-w-0 flex-1'>
                          <div className='flex items-start justify-between gap-3'>
                            <div className='min-w-0 flex-1'>
                              <div className='flex flex-wrap items-center gap-2'>
                                <div className='truncate text-sm font-medium text-slate-900 dark:text-slate-50'>
                                  {group.label}
                                </div>
                                {hasRatio(group?.ratio) ? (
                                  <Tag size='small' color='blue'>
                                    x{group.ratio}
                                  </Tag>
                                ) : null}
                                {group.disabled ? (
                                  <Tag size='small' color='grey'>
                                    {t('历史分组')}
                                  </Tag>
                                ) : null}
                              </div>

                              {group?.desc ? (
                                <div className='mt-0.5 line-clamp-1 text-xs leading-5 text-slate-500 dark:text-slate-300'>
                                  {group.desc}
                                </div>
                              ) : (
                                <div className='mt-0.5 text-xs text-slate-400 dark:text-slate-500'>
                                  {t('暂无描述')}
                                </div>
                              )}
                            </div>

                            <div className={selectedActionClassName}>
                              <Button
                                size='small'
                                theme='borderless'
                                className='!rounded-md !p-1.5'
                                disabled={disabled || loading || dragState?.id === groupId || index === 0}
                                onClick={() => moveByOffset(groupId, -1)}
                              >
                                <ChevronUp size={15} />
                              </Button>

                              <Button
                                size='small'
                                theme='borderless'
                                className='!rounded-md !p-1.5'
                                disabled={
                                  disabled ||
                                  loading ||
                                  dragState?.id === groupId ||
                                  index === selectedGroups.length - 1
                                }
                                onClick={() => moveByOffset(groupId, 1)}
                              >
                                <ChevronDown size={15} />
                              </Button>

                              <Button
                                size='small'
                                theme='borderless'
                                type='danger'
                                className='!rounded-md !p-1.5'
                                disabled={disabled || loading || dragState?.id === groupId}
                                onClick={() => removeGroup(groupId)}
                              >
                                <Trash2 size={15} />
                              </Button>

                              <button
                                type='button'
                                onPointerDown={(event) => startPointerDrag(event, groupId)}
                                className={dragHandleClassName}
                              >
                                <GripVertical size={15} />
                              </button>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>
      {dragOverlay}
    </div>
  );
};

export default TokenGroupPrioritySelector;
