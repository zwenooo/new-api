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
  Space,
  SplitButtonGroup,
  Tag,
  AvatarGroup,
  Avatar,
  Tooltip,
  Progress,
  Popover,
  Typography,
  Input,
  Modal,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  renderQuotaToUSD,
  getModelCategories,
  showError,
} from '../../../helpers';
import {
  IconTreeTriangleDown,
  IconCopy,
  IconEyeOpened,
  IconEyeClosed,
} from '@douyinfe/semi-icons';

const renderOverlayTrigger = (node) => (
  <span className='inline-flex'>{node}</span>
);

// progress color helper
const getProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

// Render functions
function renderTimestamp(timestamp) {
  return <>{timestamp2string(timestamp)}</>;
}

// Render status column only (no usage)
const renderStatus = (text, record, t) => {
  const enabled = text === 1;

  let tagColor = 'black';
  let tagText = t('未知状态');
  if (enabled) {
    tagColor = 'green';
    tagText = t('已启用');
  } else if (text === 2) {
    tagColor = 'red';
    tagText = t('已禁用');
  } else if (text === 3) {
    tagColor = 'yellow';
    tagText = t('已过期');
  } else if (text === 4) {
    tagColor = 'grey';
    tagText = t('已耗尽');
  }

  return (
    <Tag color={tagColor} shape='circle' size='small'>
      {tagText}
    </Tag>
  );
};

const renderTokenAllowedGroupsColumn = (record, groupLabelById, t) => {
  const allowedGroupIds = Array.isArray(record?.allowed_group_ids)
    ? record.allowed_group_ids
        .map((v) => Number(v))
        .filter((v) => Number.isFinite(v) && v > 0)
        .map((v) => Math.floor(v))
    : [];
  if (allowedGroupIds.length > 0) {
    const seen = new Set();
    const labels = allowedGroupIds
      .map((gid) => groupLabelById?.[gid] || t('未知分组'))
      .filter((label) => {
        const normalized = String(label || '').trim();
        if (!normalized) return false;
        if (seen.has(normalized)) return false;
        seen.add(normalized);
        return true;
      });
    return renderGroup(labels.join(','));
  }
  const primaryGroupId = Number(record?.group_id ?? 0);
  if (Number.isFinite(primaryGroupId) && primaryGroupId > 0) {
    const gid = Math.floor(primaryGroupId);
    return renderGroup(groupLabelById?.[gid] || t('未知分组'));
  }
  return renderGroup('');
};

// Render token key column with show/hide and copy functionality
const renderTokenKey = (text, record, showKeys, setShowKeys, copyText) => {
  const fullKey = 'sk-' + record.key;
  const maskedKey =
    'sk-' + record.key.slice(0, 4) + '**********' + record.key.slice(-4);
  const revealed = !!showKeys[record.id];

  return (
    <div className='w-[200px]'>
      <Input
        readOnly
        value={revealed ? fullKey : maskedKey}
        size='small'
        suffix={
          <div className='flex items-center'>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={revealed ? <IconEyeClosed /> : <IconEyeOpened />}
              aria-label='toggle token visibility'
              onClick={(e) => {
                e.stopPropagation();
                setShowKeys((prev) => ({ ...prev, [record.id]: !revealed }));
              }}
            />
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={<IconCopy />}
              aria-label='copy token key'
              onClick={async (e) => {
                e.stopPropagation();
                await copyText(fullKey);
              }}
            />
          </div>
        }
      />
    </div>
  );
};

// Render model limits column
const renderModelLimits = (text, record, t) => {
  if (record.model_limits_enabled && text) {
    const models = text.split(',').filter(Boolean);
    const categories = getModelCategories(t);

    const vendorAvatars = [];
    const matchedModels = new Set();
    Object.entries(categories).forEach(([key, category]) => {
      if (key === 'all') return;
      if (!category.icon || !category.filter) return;
      const vendorModels = models.filter((m) =>
        category.filter({ model_name: m }),
      );
      if (vendorModels.length > 0) {
        vendorAvatars.push(
          <Tooltip
            key={key}
            content={vendorModels.join(', ')}
            position='top'
            showArrow
          >
            {renderOverlayTrigger(
              <Avatar
                size='extra-extra-small'
                alt={category.label}
                color='transparent'
              >
                {category.icon}
              </Avatar>,
            )}
          </Tooltip>,
        );
        vendorModels.forEach((m) => matchedModels.add(m));
      }
    });

    const unmatchedModels = models.filter((m) => !matchedModels.has(m));
    if (unmatchedModels.length > 0) {
      vendorAvatars.push(
        <Tooltip
          key='unknown'
          content={unmatchedModels.join(', ')}
          position='top'
          showArrow
        >
          {renderOverlayTrigger(
            <Avatar size='extra-extra-small' alt='unknown'>
              {t('其他')}
            </Avatar>,
          )}
        </Tooltip>,
      );
    }

    return <AvatarGroup size='extra-extra-small'>{vendorAvatars}</AvatarGroup>;
  } else {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }
};

// Render IP restrictions column
const renderAllowIps = (text, t) => {
  if (!text || text.trim() === '') {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }

  const ips = text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean);

  const displayIps = ips.slice(0, 1);
  const extraCount = ips.length - displayIps.length;

  const ipTags = displayIps.map((ip, idx) => (
    <Tag key={idx} shape='circle'>
      {ip}
    </Tag>
  ));

  if (extraCount > 0) {
    ipTags.push(
      <Tooltip
        key='extra'
        content={ips.slice(1).join(', ')}
        position='top'
        showArrow
      >
        {renderOverlayTrigger(<Tag shape='circle'>{'+' + extraCount}</Tag>)}
      </Tooltip>,
    );
  }

  return <Space wrap>{ipTags}</Space>;
};

// Render separate quota usage column
const renderQuotaUsage = (text, record, t) => {
  const { Paragraph } = Typography;
  const used = parseInt(record.used_quota) || 0;
  const standardUsed = parseInt(record.cost_used_quota) || 0;
  const hasStandardUsedQuota =
    record.cost_used_quota !== undefined && record.cost_used_quota !== null;
  const remain = parseInt(record.remain_quota) || 0;
  const total = used + remain;
  const quotaDetailsHidden =
    record.quota_details_hidden === true ||
    record.remain_quota === undefined ||
    record.remain_quota === null;

  if (quotaDetailsHidden) {
    const showDualQuota = hasStandardUsedQuota && standardUsed !== used;
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('实际费用')}: {renderQuota(used)}
        </Paragraph>
        {showDualQuota ? (
          <Paragraph copyable={{ content: renderQuota(standardUsed) }}>
            {t('标准费用')}: {renderQuota(standardUsed)}
          </Paragraph>
        ) : null}
        <div className='text-[var(--semi-color-text-2)]'>
          {t('剩余额度暂不公开')}
        </div>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        {renderOverlayTrigger(
          <Tag color='white' shape='circle'>
            <div className='flex flex-col items-end'>
              <span className='text-xs leading-none'>{renderQuota(used)}</span>
              {showDualQuota ? (
                <span className='mt-1 text-[11px] leading-none text-[var(--semi-color-text-2)]'>
                  {t('标准费用')}: {renderQuota(standardUsed)}
                </span>
              ) : (
                <span className='mt-1 text-[11px] leading-none text-[var(--semi-color-text-2)]'>
                  {t('剩余额度已隐藏')}
                </span>
              )}
            </div>
          </Tag>,
        )}
      </Popover>
    );
  }

  if (record.unlimited_quota) {
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('实际费用')}: {renderQuota(used)}
        </Paragraph>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        {renderOverlayTrigger(
          <Tag color='white' shape='circle'>
            {t('无限额度')}
          </Tag>,
        )}
      </Popover>
    );
  }
  const percent = total > 0 ? (remain / total) * 100 : 0;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(used) }}>
        {t('实际费用')}: {renderQuota(used)}
      </Paragraph>
      {hasStandardUsedQuota && standardUsed !== used ? (
        <Paragraph copyable={{ content: renderQuota(standardUsed) }}>
          {t('标准费用')}: {renderQuota(standardUsed)}
        </Paragraph>
      ) : null}
      <Paragraph copyable={{ content: renderQuota(remain) }}>
        {t('剩余额度')}: {renderQuota(remain)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(total) }}>
        {t('总额度')}: {renderQuota(total)}
      </Paragraph>
    </div>
  );
  return (
    <Popover content={popoverContent} position='top'>
      {renderOverlayTrigger(
        <Tag color='white' shape='circle'>
          <div className='flex flex-col items-end'>
            <span className='text-xs leading-none'>{`${renderQuota(remain)} / ${renderQuota(total)}`}</span>
            <Progress
              percent={percent}
              stroke={getProgressColor(percent)}
              aria-label='quota usage'
              format={() => `${percent.toFixed(0)}%`}
              style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
            />
          </div>
        </Tag>,
      )}
    </Popover>
  );
};

const renderQuotaConsumption = (text, record, t) => {
  const usedQuota = Number(record?.used_quota ?? 0) || 0;
  const usdText = renderQuotaToUSD(usedQuota, 6);
  const popoverContent = (
    <div className='text-xs p-2'>
      <Typography.Paragraph copyable={{ content: usdText }}>
        {t('额度消耗($)')}: {usdText}
      </Typography.Paragraph>
      <Typography.Paragraph copyable={{ content: renderQuota(usedQuota) }}>
        {t('已用额度')}: {renderQuota(usedQuota)}
      </Typography.Paragraph>
    </div>
  );

  return (
    <Popover content={popoverContent} position='top'>
      {renderOverlayTrigger(
        <Tag color='white' shape='circle'>
          {usdText}
        </Tag>,
      )}
    </Popover>
  );
};

// Render operations column
const renderOperations = (
  text,
  record,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  manageToken,
  refresh,
  t,
) => {
  let chatsArray = [];
  try {
    const raw = localStorage.getItem('chats');
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      for (let i = 0; i < parsed.length; i++) {
        const item = parsed[i];
        const name = Object.keys(item)[0];
        if (!name) continue;
        chatsArray.push({
          node: 'item',
          key: i,
          name,
          value: item[name],
          onClick: () => onOpenLink(name, item[name], record),
        });
      }
    }
  } catch (_) {
    showError(t('聊天链接配置错误，请联系管理员'));
  }

  return (
    <Space wrap>
      <SplitButtonGroup
        className='overflow-hidden'
        aria-label={t('项目操作按钮组')}
      >
        <Button
          size='small'
          type='tertiary'
          onClick={() => {
            if (chatsArray.length === 0) {
              showError(t('请联系管理员配置聊天链接'));
            } else {
              const first = chatsArray[0];
              onOpenLink(first.name, first.value, record);
            }
          }}
        >
          {t('聊天')}
        </Button>
        <Dropdown trigger='click' position='bottomRight' menu={chatsArray}>
          {renderOverlayTrigger(
            <Button
              type='tertiary'
              icon={<IconTreeTriangleDown />}
              size='small'
            ></Button>,
          )}
        </Dropdown>
      </SplitButtonGroup>

      {record.status === 1 ? (
        <Button
          type='danger'
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'disable', record);
            await refresh();
          }}
        >
          {t('禁用')}
        </Button>
      ) : (
        <Button
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'enable', record);
            await refresh();
          }}
        >
          {t('启用')}
        </Button>
      )}

      <Button
        type='tertiary'
        size='small'
        onClick={() => {
          setEditingToken(record);
          setShowEdit(true);
        }}
      >
        {t('编辑')}
      </Button>

      <Button
        type='danger'
        size='small'
        onClick={() => {
          Modal.confirm({
            title: t('确定是否要删除此令牌？'),
            content: t('此修改将不可逆'),
            onOk: () => {
              (async () => {
                await manageToken(record.id, 'delete', record);
                await refresh();
              })();
            },
          });
        }}
      >
        {t('删除')}
      </Button>
    </Space>
  );
};

export const getTokensColumns = ({
  t,
  showKeys,
  setShowKeys,
  copyText,
  manageToken,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  refresh,
  groupLabelById,
}) => {
  return [
    {
      title: t('名称'),
      dataIndex: 'name',
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (text, record) => renderStatus(text, record, t),
    },
    {
      title: t('额度消耗($)'),
      key: 'quota_consumption_usd',
      render: (text, record) => renderQuotaConsumption(text, record, t),
    },
    {
      title: t('额度信息'),
      key: 'quota_usage',
      render: (text, record) => renderQuotaUsage(text, record, t),
    },
    {
      title: t('可用分组'),
      key: 'allowed_group_ids',
      render: (text, record) =>
        renderTokenAllowedGroupsColumn(record, groupLabelById, t),
    },
    {
      title: t('密钥'),
      key: 'token_key',
      render: (text, record) =>
        renderTokenKey(text, record, showKeys, setShowKeys, copyText),
    },
    {
      title: t('可用模型'),
      dataIndex: 'model_limits',
      render: (text, record) => renderModelLimits(text, record, t),
    },
    {
      title: t('IP限制'),
      dataIndex: 'allow_ips',
      render: (text) => renderAllowIps(text, t),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      render: (text, record, index) => {
        return <div>{renderTimestamp(text)}</div>;
      },
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        return (
          <div>
            {record.expired_time === -1 ? t('永不过期') : renderTimestamp(text)}
          </div>
        );
      },
    },
    {
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) =>
        renderOperations(
          text,
          record,
          onOpenLink,
          setEditingToken,
          setShowEdit,
          manageToken,
          refresh,
          t,
        ),
    },
  ];
};
