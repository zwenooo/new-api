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
import {
  Button,
  Dropdown,
  InputNumber,
  Modal,
  Space,
  SplitButtonGroup,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderQuota,
  getChannelIcon,
  renderQuotaWithAmount,
  showSuccess,
  showError,
  showInfo,
} from '../../../helpers';
import {
  CHANNEL_OPTIONS,
  MODEL_FETCHABLE_CHANNEL_TYPES,
} from '../../../constants';
import { parseUpstreamUpdateMeta } from '../../../hooks/channels/upstreamUpdateUtils';
import { IconTreeTriangleDown, IconMore } from '@douyinfe/semi-icons';
import { FaRandom } from 'react-icons/fa';

const formatCny = (value, digits = 2) => {
  const n = Number(value);
  if (!Number.isFinite(n)) return '--';
  const sign = n < 0 ? '-' : '';
  return `${sign}￥${Math.abs(n).toFixed(digits)}`;
};

const formatUsdSuffix = (value, digits = 2) => {
  const n = Number(value);
  if (!Number.isFinite(n)) return '--';
  const sign = n < 0 ? '-' : '';
  return `${sign}${Math.abs(n).toFixed(digits)}$`;
};

const formatMultiplier = (value, digits = 6) => {
  const n = Number(value);
  if (!Number.isFinite(n)) return '--';
  let str = n.toFixed(digits);
  str = str.replace(/\.?0+$/, '');
  return str;
};

const safeParseOtherInfo = (record) => {
  if (!record) return {};
  const raw = typeof record.other_info === 'string' ? record.other_info : '';
  if (!raw) return {};
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
};

const getUpstreamUpdateMeta = (record) => {
  const supported =
    !!record &&
    record.children === undefined &&
    MODEL_FETCHABLE_CHANNEL_TYPES.has(record.type);
  if (!record || record.children !== undefined) {
    return {
      supported: false,
      enabled: false,
      pendingAddModels: [],
      pendingRemoveModels: [],
    };
  }
  const parsed =
    record?.upstreamUpdateMeta && typeof record.upstreamUpdateMeta === 'object'
      ? record.upstreamUpdateMeta
      : parseUpstreamUpdateMeta(record?.settings);
  return {
    supported,
    enabled: parsed?.enabled === true,
    pendingAddModels: Array.isArray(parsed?.pendingAddModels)
      ? parsed.pendingAddModels
      : [],
    pendingRemoveModels: Array.isArray(parsed?.pendingRemoveModels)
      ? parsed.pendingRemoveModels
      : [],
  };
};

const formatBoundUserLabel = (user) => {
  const id = Number(user?.id || 0);
  const username = String(user?.username || '').trim();
  const displayName = String(user?.display_name || '').trim();
  const name = displayName || username;
  if (Number.isFinite(id) && id > 0) {
    if (name) return `${id} (${name})`;
    return String(id);
  }
  return name || '';
};

const renderBoundUsersTag = (record, t) => {
  if (!record || record.children !== undefined) return null;
  const count = Number(record.bound_user_count || 0);
  if (!Number.isFinite(count) || count <= 0) return null;

  const users = Array.isArray(record.bound_users) ? record.bound_users : [];
  const tooltipContent =
    users.length > 0 ? (
      <div className='flex flex-col gap-1 max-w-xs'>
        <Typography.Text strong>{t('已绑定用户')}</Typography.Text>
        {users.map((u) => {
          const label = formatBoundUserLabel(u);
          return (
            <Typography.Text key={u?.id} size='small'>
              {label || String(u?.id || '')}
            </Typography.Text>
          );
        })}
      </div>
    ) : (
      t('已绑定 ${count} 个用户').replace('${count}', String(count))
    );

  return (
    <Tooltip content={tooltipContent} trigger='hover' position='topLeft'>
      <Tag color='cyan' shape='circle' type='light'>
        {t('绑定')} {count}
      </Tag>
    </Tooltip>
  );
};

const normalizeGroupIDs = (value) => {
  if (!Array.isArray(value)) return [];
  return Array.from(
    new Set(
      value
        .map((v) => Number(v))
        .filter((v) => Number.isFinite(v) && v > 0)
        .map((v) => Math.floor(v)),
    ),
  ).sort((a, b) => a - b);
};

const mapGroupLabels = (ids, groupLabelById, t) => {
  return ids.map((gid) => {
    return (
      (groupLabelById instanceof Map ? groupLabelById.get(gid) : null) ??
      t('未知分组')
    );
  });
};

const renderGroupTooltipContent = (title, labels, color) => {
  if (!Array.isArray(labels) || labels.length === 0) return null;
  return (
    <div className='flex flex-col gap-2 max-w-xs'>
      <Typography.Text strong>{title}</Typography.Text>
      <div className='flex flex-wrap gap-1'>
        {labels.map((label, index) => (
          <Tag
            key={`${title}-${label}-${index}`}
            color={color}
            type='light'
            size='small'
            shape='circle'
          >
            {label}
          </Tag>
        ))}
      </div>
    </div>
  );
};

const renderCompactGroupTags = (title, labels, color) => {
  if (!Array.isArray(labels) || labels.length === 0) return null;
  const tooltipContent = renderGroupTooltipContent(title, labels, color);

  return (
    <Tooltip content={tooltipContent} position='topLeft'>
      <Tag color={color} type='light' size='small' shape='circle'>
        {title} {labels.length}
      </Tag>
    </Tooltip>
  );
};

const renderChannelGroupSummary = (record, groupLabelById, t) => {
  const primaryGroupIDs = normalizeGroupIDs(record?.group_ids);
  const backupGroupIDs = normalizeGroupIDs(record?.backup_group_ids);
  if (primaryGroupIDs.length === 0 && backupGroupIDs.length === 0) {
    return null;
  }

  const primaryLabels = mapGroupLabels(primaryGroupIDs, groupLabelById, t);
  const backupLabels = mapGroupLabels(backupGroupIDs, groupLabelById, t);

  return (
    <div className='flex flex-col gap-1 min-w-0 max-w-[10rem]'>
      {primaryLabels.length > 0 ? (
        <div className='flex items-start gap-2 min-w-0'>
          {renderCompactGroupTags(t('主'), primaryLabels, 'green')}
        </div>
      ) : null}
      {backupLabels.length > 0 ? (
        <div className='flex items-start gap-2 min-w-0'>
          {renderCompactGroupTags(t('备'), backupLabels, 'orange')}
        </div>
      ) : null}
    </div>
  );
};

// Render functions
const renderType = (type, channelInfo = undefined, t) => {
  let type2label = new Map();
  for (let i = 0; i < CHANNEL_OPTIONS.length; i++) {
    type2label[CHANNEL_OPTIONS[i].value] = CHANNEL_OPTIONS[i];
  }
  type2label[0] = { value: 0, label: t('未知类型'), color: 'grey' };

  let icon = getChannelIcon(type);

  if (channelInfo?.is_multi_key) {
    icon =
      channelInfo?.multi_key_mode === 'random' ? (
        <div className='flex items-center gap-1'>
          <FaRandom className='text-blue-500' />
          {icon}
        </div>
      ) : (
        <div className='flex items-center gap-1'>
          <IconTreeTriangleDown className='text-blue-500' />
          {icon}
        </div>
      );
  }

  return (
    <Tag color={type2label[type]?.color} shape='circle' prefixIcon={icon}>
      {type2label[type]?.label}
    </Tag>
  );
};

const renderTagType = (t) => {
  return (
    <Tag color='light-blue' shape='circle' type='light'>
      {t('标签聚合')}
    </Tag>
  );
};

const renderStatus = (status, channelInfo = undefined, t) => {
  if (channelInfo) {
    if (channelInfo.is_multi_key) {
      let keySize = channelInfo.multi_key_size;
      let enabledKeySize = keySize;
      if (channelInfo.multi_key_status_list) {
        enabledKeySize =
          keySize - Object.keys(channelInfo.multi_key_status_list).length;
      }
      return renderMultiKeyStatus(status, keySize, enabledKeySize, t);
    }
  }
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')}
        </Tag>
      );
  }
};

const renderMultiKeyStatus = (status, keySize, enabledKeySize, t) => {
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')} {enabledKeySize}/{keySize}
        </Tag>
      );
  }
};

const renderResponseTime = (responseTime, t) => {
  let time = responseTime / 1000;
  time = time.toFixed(2) + t(' 秒');
  if (responseTime === 0) {
    return (
      <Tag color='grey' shape='circle'>
        {t('未测试')}
      </Tag>
    );
  } else if (responseTime <= 1000) {
    return (
      <Tag color='green' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 3000) {
    return (
      <Tag color='lime' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 5000) {
    return (
      <Tag color='yellow' shape='circle'>
        {time}
      </Tag>
    );
  } else {
    return (
      <Tag color='red' shape='circle'>
        {time}
      </Tag>
    );
  }
};

export const getChannelsColumns = ({
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
}) => {
  return [
    {
      key: COLUMN_KEYS.ID,
      title: t('ID'),
      dataIndex: 'id',
    },
    {
      key: COLUMN_KEYS.NAME,
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record, index) => {
        const upstreamUpdateMeta = getUpstreamUpdateMeta(record);
        const pendingAddCount = upstreamUpdateMeta.pendingAddModels.length;
        const pendingRemoveCount = upstreamUpdateMeta.pendingRemoveModels.length;
        const showUpstreamUpdateTag =
          upstreamUpdateMeta.supported &&
          upstreamUpdateMeta.enabled &&
          (pendingAddCount > 0 || pendingRemoveCount > 0);
        const nameNode =
          record.remark && record.remark.trim() !== '' ? (
            <Tooltip
              content={
                <div className='flex flex-col gap-2 max-w-xs'>
                  <div className='text-sm'>{record.remark}</div>
                  <Button
                    size='small'
                    type='primary'
                    theme='outline'
                    onClick={(e) => {
                      e.stopPropagation();
                      navigator.clipboard
                        .writeText(record.remark)
                        .then(() => {
                          showSuccess(t('复制成功'));
                        })
                        .catch(() => {
                          showError(t('复制失败'));
                        });
                    }}
                  >
                    {t('复制')}
                  </Button>
                </div>
              }
              trigger='hover'
              position='topLeft'
            >
              <span>{text}</span>
            </Tooltip>
          ) : (
            <span>{text}</span>
          );

        return (
          <Space spacing={4}>
            {nameNode}
            {showUpstreamUpdateTag ? (
              <Space spacing={4} align='center'>
                {pendingAddCount > 0 ? (
                  <Tooltip content={t('点击处理新增模型')} position='top'>
                    <Tag
                      color='green'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal?.(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'add',
                        );
                      }}
                    >
                      +{pendingAddCount}
                    </Tag>
                  </Tooltip>
                ) : null}
                {pendingRemoveCount > 0 ? (
                  <Tooltip content={t('点击处理删除模型')} position='top'>
                    <Tag
                      color='red'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal?.(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'remove',
                        );
                      }}
                    >
                      -{pendingRemoveCount}
                    </Tag>
                  </Tooltip>
                ) : null}
              </Space>
            ) : null}
            {renderBoundUsersTag(record, t)}
          </Space>
        );
      },
    },
    {
      key: COLUMN_KEYS.GROUP,
      title: t('分组'),
      dataIndex: 'group_ids',
      width: 180,
      render: (text, record) => renderChannelGroupSummary(record, groupLabelById, t),
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'type',
      render: (text, record, index) => {
        if (record.children === undefined) {
          if (record.channel_info) {
            if (record.channel_info.is_multi_key) {
              return <>{renderType(text, record.channel_info, t)}</>;
            }
          }
          return <>{renderType(text, undefined, t)}</>;
        } else {
          return <>{renderTagType(t)}</>;
        }
      },
    },
    {
      key: COLUMN_KEYS.STATUS,
      title: t('状态'),
      dataIndex: 'status',
      render: (text, record, index) => {
        let baseStatusNode = renderStatus(text, record.channel_info, t);

        if (text === 3) {
          const otherInfo = safeParseOtherInfo(record);
          const reason =
            typeof otherInfo?.status_reason === 'string'
              ? otherInfo.status_reason.trim()
              : '';
          const timeNum = Number(otherInfo?.status_time || 0);
          const hasTime = Number.isFinite(timeNum) && timeNum > 0;

          let tooltipContent = '';
          if (reason && hasTime) {
            tooltipContent =
              t('原因：') + reason + t('，时间：') + timestamp2string(timeNum);
          } else if (reason) {
            tooltipContent = t('原因：') + reason;
          } else if (record?.remark && record.remark.trim() !== '') {
            tooltipContent = record.remark;
          }

          if (tooltipContent) {
            baseStatusNode = (
              <Tooltip content={tooltipContent}>
                {renderStatus(text, record.channel_info, t)}
              </Tooltip>
            );
          }
        }

        return baseStatusNode;
      },
    },
    {
      key: COLUMN_KEYS.RESPONSE_TIME,
      title: t('响应时间'),
      dataIndex: 'response_time',
      render: (text, record, index) => <div>{renderResponseTime(text, t)}</div>,
    },
    {
      key: COLUMN_KEYS.BALANCE,
      title: t('已用/剩余'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <Space spacing={1}>
                <Tooltip content={t('已用额度')}>
                  <Tag color='white' type='ghost' shape='circle'>
                    {renderQuota(record.used_quota)}
                  </Tag>
                </Tooltip>
                <Tooltip
                  content={t('剩余额度$') + record.balance + t('，点击更新')}
                >
                  <Tag
                    color='white'
                    type='ghost'
                    shape='circle'
                    onClick={() => updateChannelBalance(record)}
                  >
                    {renderQuotaWithAmount(record.balance)}
                  </Tag>
                </Tooltip>
              </Space>
            </div>
          );
        } else {
          return (
            <Tooltip content={t('已用额度')}>
              <Tag color='white' type='ghost' shape='circle'>
                {renderQuota(record.used_quota)}
              </Tag>
            </Tooltip>
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.BUY_PRICE,
      title: t('进价'),
      dataIndex: 'buy_cny_per_usd',
      render: (text, record) => {
        if (record.children !== undefined) {
          return (
            <Tag color='grey' shape='circle'>
              --
            </Tag>
          );
        }

        const billingMode = String(record?.billing_mode || '').trim();
        if (billingMode === 'request') {
          const buyRate = Number(record?.buy_requests_per_cny);
          if (!Number.isFinite(buyRate) || buyRate <= 0) {
            return (
              <Tag color='grey' shape='circle'>
                --
              </Tag>
            );
          }
          return (
            <Tooltip content={t('¥1 = {{n}} 次', { n: buyRate })} position='top'>
              <Tag color='light-blue' shape='circle'>
                ¥1={buyRate}次
              </Tag>
            </Tooltip>
          );
        }

        const buy = Number(record.buy_cny_per_usd);
        if (!Number.isFinite(buy) || buy <= 0) {
          return (
            <Tag color='grey' shape='circle'>
              --
            </Tag>
          );
        }

        const inv = 1 / buy;
        const invText = Number.isFinite(inv) ? inv.toFixed(4) : '--';
        return (
          <Tooltip content={`￥${buy}/$1（1￥=${invText}$）`} position='top'>
            <Tag color='light-blue' shape='circle'>
              ￥{buy}/$
            </Tag>
          </Tooltip>
        );
      },
    },
    {
      key: COLUMN_KEYS.PROFIT,
      title: t('利润'),
      dataIndex: 'profit_cny',
      render: (text, record) => {
        const profit = Number(record.profit_cny);
        const revenue = Number(record.revenue_cny);
        const cost = Number(record.cost_cny);

        if (!Number.isFinite(profit)) {
          return (
            <Tag color='grey' shape='circle'>
              --
            </Tag>
          );
        }

        const billingMode = String(record?.billing_mode || '').trim();
        if (billingMode === 'request') {
          const successCount = Number(record?.request_success_count || 0);
          const buyRate = Number(record?.buy_requests_per_cny);
          const sellRate = Number(record?.sell_requests_per_cny);

          const tooltipContent =
            Number.isFinite(revenue) && Number.isFinite(cost) ? (
              <Space vertical align='start' spacing={4}>
                <Typography.Text strong size='small'>
                  {t('按次口径')}
                </Typography.Text>
                <Typography.Text size='small'>
                  {t('成功次数')}：{Number(successCount || 0)}
                </Typography.Text>
                <Typography.Text size='small'>
                  {t('销售额')}：{Number(successCount || 0)} ÷{' '}
                  {Number.isFinite(sellRate) && sellRate > 0 ? sellRate : '--'} ={' '}
                  {formatCny(revenue)}
                </Typography.Text>
                <Typography.Text size='small'>
                  {t('成本')}：{Number(successCount || 0)} ÷{' '}
                  {Number.isFinite(buyRate) && buyRate > 0 ? buyRate : '--'} ={' '}
                  {formatCny(cost)}
                </Typography.Text>
                <Typography.Text strong size='small'>
                  {t('利润')}：{formatCny(profit)}
                </Typography.Text>
              </Space>
            ) : (
              formatCny(profit)
            );

          return (
            <Tooltip content={tooltipContent} position='top'>
              <Tag color={profit >= 0 ? 'green' : 'red'} shape='circle'>
                {formatCny(profit)}
              </Tag>
            </Tooltip>
          );
        }

        const quotaPerUnit = Number(localStorage.getItem('quota_per_unit'));
        const quotaPerUnitValid =
          Number.isFinite(quotaPerUnit) && quotaPerUnit > 0;
        const salesUsd = quotaPerUnitValid
          ? Number(record.used_quota) / quotaPerUnit
          : NaN;
        const costUsd = quotaPerUnitValid
          ? Number(record.cost_used_quota) / quotaPerUnit
          : NaN;
        const revenueCnyPerUsd =
          Number.isFinite(revenue) && Number.isFinite(salesUsd) && salesUsd !== 0
            ? revenue / salesUsd
            : NaN;

        const buyCnyPerUsd = Number(record.buy_cny_per_usd);

        const tooltipContent =
          Number.isFinite(revenue) && Number.isFinite(cost) ? (
            <Space vertical align='start' spacing={4}>
              <Typography.Text strong size='small'>
                {t('销售口径')}
              </Typography.Text>
              <Typography.Text size='small'>
                {formatUsdSuffix(salesUsd)} × {formatMultiplier(revenueCnyPerUsd)}{' '}
                = {formatCny(revenue)}
              </Typography.Text>
              <Typography.Text strong size='small'>
                {t('成本口径')}
              </Typography.Text>
              <Typography.Text size='small'>
                {formatUsdSuffix(costUsd)} × {formatMultiplier(buyCnyPerUsd)} ={' '}
                {formatCny(cost)}
              </Typography.Text>
              <Typography.Text strong size='small'>
                {t('利润')}：{formatCny(profit)}
              </Typography.Text>
            </Space>
          ) : (
            formatCny(profit)
          );

        return (
          <Tooltip content={tooltipContent} position='top'>
            <Tag color={profit >= 0 ? 'green' : 'red'} shape='circle'>
              {formatCny(profit)}
            </Tag>
          </Tooltip>
        );
      },
    },
    {
      key: COLUMN_KEYS.PRIORITY,
      title: t('优先级'),
      dataIndex: 'priority',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='priority'
                onBlur={(e) => {
                  manageChannel(record.id, 'priority', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.priority}
                min={-999}
                size='small'
              />
            </div>
          );
        } else {
          const tagKey = String(record?.key ?? '').trim();
          const disabled = tagKey === '';
          return (
            <InputNumber
              style={{ width: 70 }}
              name='priority'
              keepFocus={true}
              disabled={disabled}
              onBlur={(e) => {
                if (disabled) return;
                const nextValue = e?.target?.value;
                Modal.warning({
                  title: t('修改子渠道优先级'),
                  content:
                    t('确定要修改所有子渠道优先级为 ') +
                    nextValue +
                    t(' 吗？'),
                  onOk: () => {
                    if (nextValue === '' || nextValue === undefined) {
                      return;
                    }
                    submitTagEdit('priority', {
                      tag: tagKey,
                      priority: nextValue,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.priority}
              min={-999}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.WEIGHT,
      title: t('权重'),
      dataIndex: 'weight',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='weight'
                onBlur={(e) => {
                  manageChannel(record.id, 'weight', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.weight}
                min={0}
                size='small'
              />
            </div>
          );
        } else {
          const tagKey = String(record?.key ?? '').trim();
          const disabled = tagKey === '';
          return (
            <InputNumber
              style={{ width: 70 }}
              name='weight'
              keepFocus={true}
              disabled={disabled}
              onBlur={(e) => {
                if (disabled) return;
                const nextValue = e?.target?.value;
                Modal.warning({
                  title: t('修改子渠道权重'),
                  content:
                    t('确定要修改所有子渠道权重为 ') +
                    nextValue +
                    t(' 吗？'),
                  onOk: () => {
                    if (nextValue === '' || nextValue === undefined) {
                      return;
                    }
                    submitTagEdit('weight', {
                      tag: tagKey,
                      weight: nextValue,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.weight}
              min={0}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.OPERATE,
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) => {
        if (record.children === undefined) {
          const abnormalEnabled = Boolean(
            channelAbnormalConsumeEnabledMap?.[record.id],
          );
          const usedQuotaValue = Number(record.used_quota) || 0;
          const costUsedQuotaValue = Number(record.cost_used_quota) || 0;
          const requestSuccessCountValue =
            Number(record.request_success_count) || 0;
          const upstreamUpdateMeta = getUpstreamUpdateMeta(record);

          const moreMenuItems = [
            {
              node: 'item',
              name: t('重置已用额度'),
              type: 'tertiary',
              disabled:
                !resetChannelUsedQuota ||
                (usedQuotaValue === 0 &&
                  costUsedQuotaValue === 0 &&
                  requestSuccessCountValue === 0),
              onClick: () => {
                if (!resetChannelUsedQuota) {
                  return;
                }
                Modal.confirm({
                  title: t('确定重置渠道已用额度吗？'),
                  content: t(
                    '该操作会将此渠道的已用数据（销售口径 + 成本口径 + 按次口径成功次数）清零，不会影响剩余额度。',
                  ),
                  onOk: () => resetChannelUsedQuota(record),
                });
              },
            },
            {
              node: 'item',
              name: t('删除'),
              type: 'danger',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要删除此渠道？'),
                  content: t('此修改将不可逆'),
                  onOk: () => {
                    (async () => {
                      await manageChannel(record.id, 'delete', record);
                      await refresh();
                      setTimeout(() => {
                        if (channels.length === 0 && activePage > 1) {
                          refresh(activePage - 1);
                        }
                      }, 100);
                    })();
                  },
                });
              },
            },
            {
              node: 'item',
              name: t('复制'),
              type: 'tertiary',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要复制此渠道？'),
                  content: t('复制渠道的所有信息'),
                  onOk: () => copySelectedChannel(record),
                });
              },
            },
          ];

          if (upstreamUpdateMeta.supported) {
            moreMenuItems.push({
              node: 'item',
              name: t('仅检测上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                detectChannelUpstreamUpdates?.(record);
              },
            });
            moreMenuItems.push({
              node: 'item',
              name: t('处理上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                if (!upstreamUpdateMeta.enabled) {
                  showInfo(t('该渠道未开启上游模型更新检测'));
                  return;
                }
                if (
                  upstreamUpdateMeta.pendingAddModels.length === 0 &&
                  upstreamUpdateMeta.pendingRemoveModels.length === 0
                ) {
                  showInfo(t('该渠道暂无可处理的上游模型更新'));
                  return;
                }
                openUpstreamUpdateModal?.(
                  record,
                  upstreamUpdateMeta.pendingAddModels,
                  upstreamUpdateMeta.pendingRemoveModels,
                  upstreamUpdateMeta.pendingAddModels.length > 0
                    ? 'add'
                    : 'remove',
                );
              },
            });
          }

          return (
            <Space wrap>
              <SplitButtonGroup
                className='overflow-hidden'
                aria-label={t('测试单个渠道操作项目组')}
              >
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() => testChannel(record, '')}
                >
                  {t('测试')}
                </Button>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconTreeTriangleDown />}
                  onClick={() => {
                    setCurrentTestChannel(record);
                    setShowModelTestModal(true);
                  }}
                />
              </SplitButtonGroup>

              {record.status === 1 ? (
                <Button
                  type='danger'
                  size='small'
                  onClick={() => manageChannel(record.id, 'disable', record)}
                >
                  {t('禁用')}
                </Button>
              ) : (
                <Button
                  size='small'
                  onClick={() => manageChannel(record.id, 'enable', record)}
                >
                  {t('启用')}
                </Button>
              )}

              <SplitButtonGroup aria-label={t('异常统计')}>
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() =>
                    updateChannelAbnormalConsumeEnabled(
                      record.id,
                      !abnormalEnabled,
                    )
                  }
                >
                  {abnormalEnabled ? t('关闭统计') : t('启动统计')}
                </Button>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconTreeTriangleDown />}
                  onClick={() => openChannelAbnormalConsumeModal(record)}
                />
              </SplitButtonGroup>

              {record.channel_info?.is_multi_key ? (
                <SplitButtonGroup aria-label={t('多密钥渠道操作项目组')}>
                  <Button
                    type='tertiary'
                    size='small'
                    onClick={() => {
                      setEditingChannel(record);
                      setShowEdit(true);
                    }}
                  >
                    {t('编辑')}
                  </Button>
                  <Dropdown
                    trigger='click'
                    position='bottomRight'
                    menu={[
                      {
                        node: 'item',
                        name: t('多密钥管理'),
                        onClick: () => {
                          setCurrentMultiKeyChannel(record);
                          setShowMultiKeyManageModal(true);
                        },
                      },
                    ]}
                  >
                    <Button
                      type='tertiary'
                      size='small'
                      icon={<IconTreeTriangleDown />}
                    />
                  </Dropdown>
                </SplitButtonGroup>
              ) : (
                <Button
                  type='tertiary'
                  size='small'
                  onClick={() => {
                    setEditingChannel(record);
                    setShowEdit(true);
                  }}
                >
                  {t('编辑')}
                </Button>
              )}

              <Button
                type='tertiary'
                size='small'
                onClick={() => {
                  setCurrentProfitStatsChannel(record);
                  setShowChannelProfitStatsModal(true);
                }}
              >
                {t('利润统计')}
              </Button>

              {moreMenuItems.length > 0 ? (
                <Dropdown
                  trigger='click'
                  position='bottomRight'
                  menu={moreMenuItems}
                >
                  <Button icon={<IconMore />} type='tertiary' size='small' />
                </Dropdown>
              ) : null}
            </Space>
          );
        } else {
          return (
            <Space wrap>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'enable')}
              >
                {t('启用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'disable')}
              >
                {t('禁用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => {
                  setShowEditTag(true);
                  setEditingTag(record.key);
                }}
              >
                {t('编辑')}
              </Button>
            </Space>
          );
        }
      },
    },
  ];
};
