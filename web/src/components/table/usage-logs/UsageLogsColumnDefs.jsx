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
  Avatar,
  Button,
  Space,
  Tag,
  Tooltip,
  Popover,
  Typography,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  stringToColor,
  getLogOther,
  renderModelTag,
  renderClaudeLogContent,
  renderLogContent,
} from '../../../helpers';
import { IconHelpCircle, IconLink } from '@douyinfe/semi-icons';
import { Route } from 'lucide-react';

const colors = [
  'amber',
  'blue',
  'cyan',
  'green',
  'grey',
  'indigo',
  'light-blue',
  'lime',
  'orange',
  'pink',
  'purple',
  'red',
  'teal',
  'violet',
  'yellow',
];

const resolveGroupDisplay = (rawGroup, groupLabelById, t) => {
  const text = String(rawGroup ?? '').trim();
  if (!text) return '';

  const parts = text
    .split(',')
    .map((part) => String(part || '').trim())
    .filter(Boolean);

  const resolvePart = (part) => {
    const num = Number(part);
    if (Number.isFinite(num) && num > 0) {
      const id = Math.floor(num);
      const label = groupLabelById?.[id];
      if (label) return String(label);
      return typeof t === 'function' ? t('未知分组') : '未知分组';
    }
    return part;
  };

  return parts.map(resolvePart).join(',');
};

// Render functions
function renderType(type, t) {
  switch (type) {
    case 1:
      return (
        <Tag color='cyan' shape='circle'>
          {t('充值')}
        </Tag>
      );
    case 2:
      return (
        <Tag color='lime' shape='circle'>
          {t('消费')}
        </Tag>
      );
    case 3:
      return (
        <Tag color='orange' shape='circle'>
          {t('管理')}
        </Tag>
      );
    case 4:
      return (
        <Tag color='purple' shape='circle'>
          {t('系统')}
        </Tag>
      );
    case 5:
      return (
        <Tag color='red' shape='circle'>
          {t('错误')}
        </Tag>
      );
    case 6:
      return (
        <Tag color='blue' shape='circle'>
          {t('消费进行中')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知')}
        </Tag>
      );
  }
}

function renderIsStream(bool, t) {
  if (bool) {
    return (
      <Tag color='blue' shape='circle'>
        {t('流')}
      </Tag>
    );
  } else {
    return (
      <Tag color='purple' shape='circle'>
        {t('非流')}
      </Tag>
    );
  }
}

function renderQuotaBucket(bucket, t) {
  const value = typeof bucket === 'string' ? bucket.trim() : '';
  switch (value) {
    case 'request':
      return (
        <Tag color='cyan' shape='circle'>
          {t('次数')}
        </Tag>
      );
    case 'subscription':
      return (
        <Tag color='green' shape='circle'>
          {t('订阅')}
        </Tag>
      );
    case 'payg':
      return (
        <Tag color='orange' shape='circle'>
          {t('按量付费')}
        </Tag>
      );
    case 'free':
      return (
        <Tag color='grey' shape='circle'>
          {t('自由')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          --
        </Tag>
      );
  }
}

function renderUseTime(type, t) {
  const time = parseInt(type);
  if (time < 101) {
    return (
      <Tag color='green' shape='circle'>
        {' '}
        {time} s{' '}
      </Tag>
    );
  } else if (time < 300) {
    return (
      <Tag color='orange' shape='circle'>
        {' '}
        {time} s{' '}
      </Tag>
    );
  } else {
    return (
      <Tag color='red' shape='circle'>
        {' '}
        {time} s{' '}
      </Tag>
    );
  }
}

function renderFirstUseTime(type, t) {
  const ms = parseFloat(type);
  if (!Number.isFinite(ms) || ms < 0) {
    return (
      <Tag color='grey' shape='circle'>
        {' '}
        --{' '}
      </Tag>
    );
  }

  const timeText = (ms / 1000.0).toFixed(1);
  const time = parseFloat(timeText);
  if (time < 3) {
    return (
      <Tag color='green' shape='circle'>
        {' '}
        {timeText} s{' '}
      </Tag>
    );
  } else if (time < 10) {
    return (
      <Tag color='orange' shape='circle'>
        {' '}
        {timeText} s{' '}
      </Tag>
    );
  } else {
    return (
      <Tag color='red' shape='circle'>
        {' '}
        {timeText} s{' '}
      </Tag>
    );
  }
}

function renderModelName(record, copyText, t) {
  let other = getLogOther(record.other);
  const modelName = formatDisplayModelName(record.model_name, other);
  let modelMapped =
    other?.is_model_mapped &&
    other?.upstream_model_name &&
    other?.upstream_model_name !== '';
  if (!modelMapped) {
    return renderModelTag(modelName, {
      onClick: (event) => {
        copyText(event, record.model_name).then((r) => { });
      },
    });
  } else {
    return (
      <>
        <Space vertical align={'start'}>
          <Popover
            content={
              <div style={{ padding: 10 }}>
                <Space vertical align={'start'}>
                  <div className='flex items-center'>
                    <Typography.Text strong style={{ marginRight: 8 }}>
                      {t('请求并计费模型')}:
                    </Typography.Text>
                    {renderModelTag(modelName, {
                      onClick: (event) => {
                        copyText(event, record.model_name).then((r) => { });
                      },
                    })}
                  </div>
                  <div className='flex items-center'>
                    <Typography.Text strong style={{ marginRight: 8 }}>
                      {t('实际模型')}:
                    </Typography.Text>
                    {renderModelTag(other.upstream_model_name, {
                      onClick: (event) => {
                        copyText(event, other.upstream_model_name).then(
                          (r) => { },
                        );
                      },
                    })}
                  </div>
                </Space>
              </div>
            }
          >
            {renderModelTag(modelName, {
              onClick: (event) => {
                copyText(event, record.model_name).then((r) => { });
              },
              suffixIcon: (
                <Route
                  style={{ width: '0.9em', height: '0.9em', opacity: 0.75 }}
                />
              ),
            })}
          </Popover>
        </Space>
      </>
    );
  }
}

function formatDisplayModelName(modelName, other) {
  const baseName = String(modelName || '').trim();
  if (!baseName) {
    return '';
  }
  const suffixParts = [];
  const serviceTier = String(other?.service_tier || '').trim();
  const reasoningEffort = String(other?.reasoning_effort || '').trim();
  if (serviceTier) {
    suffixParts.push(`service_tier=${serviceTier}`);
  }
  if (reasoningEffort) {
    suffixParts.push(`reasoning_effort=${reasoningEffort}`);
  }
  if (suffixParts.length === 0) {
    return baseName;
  }
  return `${baseName} (${suffixParts.join(', ')})`;
}

function renderCopyableSessionKey(value, copyText) {
  const text = typeof value === 'string' ? value.trim() : '';
  if (!text) {
    return <></>;
  }
  const displayText =
    text.length > 20 ? `${text.slice(0, 10)}...${text.slice(-6)}` : text;
  return (
    <Tooltip content={text}>
      <span>
        <Tag
          color='grey'
          shape='circle'
          onClick={(event) => {
            copyText(event, text);
          }}
        >
          {displayText}
        </Tag>
      </span>
    </Tooltip>
  );
}

function toTokenNumber(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 0;
  }
  return parsed;
}

function formatTokenCount(value) {
  return toTokenNumber(value).toLocaleString();
}

function getPromptCacheSummary(other) {
  if (!other || typeof other !== 'object') {
    return null;
  }

  const cacheReadTokens = toTokenNumber(other.cache_tokens);
  const cacheCreationTokens = toTokenNumber(other.cache_creation_tokens);
  const cacheCreationTokens5m = toTokenNumber(other.cache_creation_tokens_5m);
  const cacheCreationTokens1h = toTokenNumber(other.cache_creation_tokens_1h);

  const hasSplitCacheCreation =
    cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;
  const cacheWriteTokens = hasSplitCacheCreation
    ? cacheCreationTokens5m + cacheCreationTokens1h
    : cacheCreationTokens;

  if (cacheReadTokens <= 0 && cacheWriteTokens <= 0) {
    return null;
  }

  return {
    cacheReadTokens,
    cacheWriteTokens,
  };
}

export const getLogsColumns = ({
  t,
  COLUMN_KEYS,
  copyText,
  groupLabelById,
  showUserInfoFunc,
  isAdminUser,
  navigateToChannel,
}) => {
  const columns = [
    {
      key: COLUMN_KEYS.TIME,
      title: t('时间'),
      dataIndex: 'timestamp2string',
    },
    {
      key: COLUMN_KEYS.CHANNEL,
      title: t('渠道'),
      dataIndex: 'channel',
      render: (text, record, index) => {
        let isMultiKey = false;
        let multiKeyIndex = -1;
        let other = getLogOther(record.other);
        if (other?.admin_info) {
          let adminInfo = other.admin_info;
          if (adminInfo?.is_multi_key) {
            isMultiKey = true;
            multiKeyIndex = adminInfo.multi_key_index;
          }
        }

        return isAdminUser &&
          (record.type === 0 || record.type === 2 || record.type === 5) ? (
          <Space>
            <Tooltip content={record.channel_name || t('未知渠道')}>
              <span>
                <Tag
                  color={colors[parseInt(text) % colors.length]}
                  shape='circle'
                >
                  {text}
                </Tag>
              </span>
            </Tooltip>
            {typeof navigateToChannel === 'function' ? (
              <Tooltip content={t('跳转到渠道')} position='top'>
                <Button
                  size='small'
                  theme='borderless'
                  type='tertiary'
                  icon={<IconLink />}
                  onClick={(e) => {
                    e.stopPropagation();
                    navigateToChannel(record.channel);
                  }}
                />
              </Tooltip>
            ) : null}
            {isMultiKey && (
              <Tag color='white' shape='circle'>
                {multiKeyIndex}
              </Tag>
            )}
          </Space>
        ) : null;
      },
    },
    {
      key: COLUMN_KEYS.USERNAME,
      title: t('用户'),
      dataIndex: 'username',
      render: (text, record, index) => {
        return isAdminUser ? (
          <div>
            <Avatar
              size='extra-small'
              color={stringToColor(text)}
              style={{ marginRight: 4 }}
              onClick={(event) => {
                event.stopPropagation();
                showUserInfoFunc(record.user_id);
              }}
            >
              {typeof text === 'string' && text.slice(0, 1)}
            </Avatar>
            {text}
          </div>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.PROMPT_CACHE_KEY,
      title: t('prompt_cache_key'),
      dataIndex: 'prompt_cache_key',
      render: (text) => renderCopyableSessionKey(text, copyText),
    },
    {
      key: COLUMN_KEYS.SESSION_ID,
      title: t('session_id'),
      dataIndex: 'session_id',
      render: (text) => renderCopyableSessionKey(text, copyText),
    },
    {
      key: COLUMN_KEYS.CONVERSATION_ID,
      title: t('conversation_id'),
      dataIndex: 'conversation_id',
      render: (text) => renderCopyableSessionKey(text, copyText),
    },
    {
      key: COLUMN_KEYS.TOKEN,
      title: t('令牌'),
      dataIndex: 'token_name',
      render: (text, record, index) => {
        return record.type === 0 || record.type === 2 || record.type === 5 ? (
          <div>
            <Tag
              color='grey'
              shape='circle'
              onClick={(event) => {
                copyText(event, text);
              }}
            >
              {' '}
              {t(text)}{' '}
            </Tag>
          </div>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.GROUP,
      title: t('模型分组'),
      dataIndex: 'group',
      render: (text, record, index) => {
        if (record.type === 0 || record.type === 2 || record.type === 5) {
          const groupRaw = record.group || getLogOther(record.other)?.group;
          const display = resolveGroupDisplay(groupRaw, groupLabelById, t);
          return display ? <>{renderGroup(display)}</> : <></>;
        } else {
          return <></>;
        }
      },
    },
    {
      key: COLUMN_KEYS.QUOTA_BUCKET,
      title: t('计费桶'),
      dataIndex: 'quota_bucket',
      render: (text) => <>{renderQuotaBucket(text, t)}</>,
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'type',
      render: (text, record, index) => {
        return <>{renderType(text, t)}</>;
      },
    },
    {
      key: COLUMN_KEYS.MODEL,
      title: t('模型'),
      dataIndex: 'model_name',
      render: (text, record, index) => {
        return record.type === 0 || record.type === 2 || record.type === 5 ? (
          <>{renderModelName(record, copyText, t)}</>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.USE_TIME,
      title: t('用时/首字'),
      dataIndex: 'use_time',
      render: (text, record, index) => {
        if (!(record.type === 2 || record.type === 5)) {
          return <></>;
        }
        if (record.is_stream) {
          let other = getLogOther(record.other);
          return (
            <>
              <Space>
                {renderUseTime(text, t)}
                {renderFirstUseTime(other?.frt, t)}
                {renderIsStream(record.is_stream, t)}
              </Space>
            </>
          );
        } else {
          return (
            <>
              <Space>
                {renderUseTime(text, t)}
                {renderIsStream(record.is_stream, t)}
              </Space>
            </>
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.PROMPT,
      title: (
        <div className='flex items-center gap-1'>
          {t('输入')}
          <Tooltip
            content={t(
              '根据 Anthropic 协定，/v1/messages 的输入 tokens 仅统计非缓存输入，不包含缓存读取与缓存写入 tokens。',
            )}
          >
            <IconHelpCircle className='text-gray-400 cursor-help' />
          </Tooltip>
        </div>
      ),
      dataIndex: 'prompt_tokens',
      align: 'center',
      render: (text, record, index) => {
        if (!(record.type === 0 || record.type === 2 || record.type === 5)) {
          return <></>;
        }
        const other = getLogOther(record.other);
        const cacheSummary = getPromptCacheSummary(other);
        const hasCacheRead = (cacheSummary?.cacheReadTokens || 0) > 0;
        const hasCacheWrite = (cacheSummary?.cacheWriteTokens || 0) > 0;
        let cacheText = '';
        if (hasCacheRead && hasCacheWrite) {
          cacheText = `${t('缓存读')} ${formatTokenCount(cacheSummary.cacheReadTokens)} · ${t('写')} ${formatTokenCount(cacheSummary.cacheWriteTokens)}`;
        } else if (hasCacheRead) {
          cacheText = `${t('缓存读')} ${formatTokenCount(cacheSummary.cacheReadTokens)}`;
        } else if (hasCacheWrite) {
          cacheText = `${t('缓存写')} ${formatTokenCount(cacheSummary.cacheWriteTokens)}`;
        }

        return (
          <div
            style={{
              display: 'inline-flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              lineHeight: 1.2,
            }}
          >
            <span>{text}</span>
            {cacheText ? (
              <span
                style={{
                  marginTop: 2,
                  fontSize: 11,
                  color: 'var(--semi-color-text-2)',
                  whiteSpace: 'nowrap',
                }}
              >
                {cacheText}
              </span>
            ) : null}
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.COMPLETION,
      title: t('输出'),
      dataIndex: 'completion_tokens',
      align: 'center',
      render: (text, record, index) => {
        const completionTokens = Number(text) || 0;
        return record.type === 0 || record.type === 2 || record.type === 5 ? (
          <span>{completionTokens}</span>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.COST,
      title: t('花费'),
      dataIndex: 'quota',
      render: (text, record, index) => {
        return record.type === 0 || record.type === 2 || record.type === 5 ? (
          <>{renderQuota(text, 6)}</>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.IP,
      title: (
        <div className='flex items-center gap-1'>
          {t('IP')}
          <Tooltip
            content={t(
              '只有当用户设置开启IP记录时，才会进行请求和错误类型日志的IP记录',
            )}
          >
            <IconHelpCircle className='text-gray-400 cursor-help' />
          </Tooltip>
        </div>
      ),
      dataIndex: 'ip',
      render: (text, record, index) => {
        return (record.type === 2 || record.type === 5) && text ? (
          <Tooltip content={text}>
            <span>
              <Tag
                color='orange'
                shape='circle'
                onClick={(event) => {
                  copyText(event, text);
                }}
              >
                {text}
              </Tag>
            </span>
          </Tooltip>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.RETRY,
      title: t('重试'),
      dataIndex: 'retry',
      render: (text, record, index) => {
        if (!(record.type === 2 || record.type === 5)) {
          return <></>;
        }
        let content = t('渠道') + `：${record.channel}`;
        if (record.other !== '') {
          let other = JSON.parse(record.other);
          if (other === null) {
            return <></>;
          }
          if (other.admin_info !== undefined) {
            if (
              other.admin_info.use_channel !== null &&
              other.admin_info.use_channel !== undefined &&
              other.admin_info.use_channel !== ''
            ) {
              let useChannel = other.admin_info.use_channel;
              let useChannelStr = useChannel.join('->');
              content = t('渠道') + `：${useChannelStr}`;
            }
          }
        }
        return isAdminUser ? <div>{content}</div> : <></>;
      },
    },
  ];

  return columns.map((column) => ({
    ...column,
    align: column.align || 'center',
  }));
};

export const getUserLogsColumns = ({ t, copyText, groupLabelById }) => {
  const columns = [
    {
      key: 'time',
      title: t('请求时间'),
      dataIndex: 'timestamp2string',
    },
    {
      key: 'token',
      title: t('令牌'),
      dataIndex: 'token_name',
      render: (text) => {
        return (
          <Tag
            color='grey'
            shape='circle'
            onClick={(event) => {
              copyText(event, text);
            }}
          >
            {t(text)}
          </Tag>
        );
      },
    },
    {
      key: 'model_group',
      title: t('模型分组'),
      dataIndex: 'group',
      render: (text, record) => {
        const groupRaw = record?.group || getLogOther(record.other)?.group;
        const display = resolveGroupDisplay(groupRaw, groupLabelById, t);
        return display ? <>{renderGroup(display)}</> : <></>;
      },
    },
    {
      key: 'model',
      title: t('模型'),
      dataIndex: 'model_name',
      render: (text, record) => <>{renderModelName(record, copyText, t)}</>,
    },
    {
      key: 'use_time',
      title: t('用时'),
      dataIndex: 'use_time',
      render: (text) => <>{renderUseTime(text, t)}</>,
    },
    {
      key: 'prompt_tokens',
      title: t('输入token'),
      dataIndex: 'prompt_tokens',
      render: (text) => <span>{Number(text) || 0}</span>,
    },
    {
      key: 'completion_tokens',
      title: t('输出token'),
      dataIndex: 'completion_tokens',
      render: (text) => <span>{Number(text) || 0}</span>,
    },
    {
      key: 'cache_read',
      title: t('缓存读取'),
      render: (text, record) => {
        const other = getLogOther(record.other);
        const summary = getPromptCacheSummary(other);
        return <span>{summary?.cacheReadTokens || 0}</span>;
      },
    },
    {
      key: 'cache_create',
      title: t('缓存创建'),
      render: (text, record) => {
        const other = getLogOther(record.other);
        const summary = getPromptCacheSummary(other);
        return <span>{summary?.cacheWriteTokens || 0}</span>;
      },
    },
    {
      key: 'cost',
      title: t('费用'),
      dataIndex: 'quota',
      render: (text, record) => {
        const rawQuota = Number(text) || 0;
        const quotaLegacy = record?.quota_legacy === true;
        const standardQuota = Number(record?.cost_quota ?? 0) || 0;
        const showDualQuota =
          !quotaLegacy && standardQuota > 0 && standardQuota !== rawQuota;
        if (quotaLegacy) {
          return (
            <div className='inline-flex flex-col items-center leading-tight'>
              <span>{renderQuota(rawQuota, 6)}</span>
              <span className='mt-1 text-[11px] text-[var(--semi-color-text-2)]'>
                {t('旧口径')}
              </span>
            </div>
          );
        }
        return (
          <div className='inline-flex flex-col items-center leading-tight'>
            <span>{renderQuota(rawQuota, 6)}</span>
            {showDualQuota ? (
              <span className='mt-1 text-[11px] text-[var(--semi-color-text-2)]'>
                {t('标准费用')}: {renderQuota(standardQuota, 6)}
              </span>
            ) : null}
          </div>
        );
      },
    },
  ];

  return columns.map((column) => ({
    ...column,
    align: column.align || 'center',
  }));
};
