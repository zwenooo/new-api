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
  Card,
  Tag,
  Divider,
  Typography,
  Badge,
} from '@douyinfe/semi-ui';
import {
  isEffectiveAdmin,
  isEffectiveRoot,
  getDiceBearAvatarUrl,
  renderQuota,
  stringToColor,
} from '../../../../helpers';
import { Coins, BarChart2, Users, RefreshCw } from 'lucide-react';

const UserInfoHeader = ({ t, userState, onChangeAvatar, avatarUpdating }) => {
  const showExtendedInfo = isEffectiveAdmin() || isEffectiveRoot();
  const actualUsedQuota = Number(userState?.user?.used_quota ?? 0) || 0;
  const standardUsedQuota =
    userState?.user?.cost_used_quota !== undefined &&
    userState?.user?.cost_used_quota !== null
      ? Number(userState.user.cost_used_quota) || 0
      : 0;
  const showDualUsedQuota =
    standardUsedQuota > 0 && standardUsedQuota !== actualUsedQuota;

  const getUsername = () => {
    if (userState.user) {
      return userState.user.username;
    } else {
      return 'null';
    }
  };

  const getAvatarText = () => {
    const username = getUsername();
    if (username && username.length > 0) {
      return username.slice(0, 2).toUpperCase();
    }
    return 'NA';
  };

  return (
    <section className='rounded-2xl overflow-hidden bg-[var(--app-card)] shadow-[var(--app-shadow)]'>
      <div
        className='relative h-32'
        style={{
          '--palette-primary-darkerChannel': '0 75 80',
          backgroundImage: `linear-gradient(0deg, rgba(var(--palette-primary-darkerChannel) / 80%), rgba(var(--palette-primary-darkerChannel) / 80%))`,
        }}
      >
        {/* 用户信息内容 */}
        <div className='relative z-10 h-full flex flex-col justify-end p-6'>
          <div className='flex items-center'>
            <div className='flex items-stretch gap-3 sm:gap-4 flex-1 min-w-0'>
              <Avatar
                size='large'
                src={getDiceBearAvatarUrl(userState?.user?.avatar_seed, {
                  size: 128,
                })}
                color={stringToColor(getUsername())}
              >
                {getAvatarText()}
              </Avatar>
              <div className='flex-1 min-w-0 flex flex-col justify-between'>
                <div
                  className='text-3xl font-bold truncate'
                  style={{ color: 'white' }}
                >
                  {getUsername()}
                </div>
                <div className='flex flex-wrap items-center gap-2'>
                  {isEffectiveRoot() ? (
                    <Tag size='large' shape='circle' style={{ color: 'white' }}>
                      {t('超级管理员')}
                    </Tag>
                  ) : isEffectiveAdmin() ? (
                    <Tag size='large' shape='circle' style={{ color: 'white' }}>
                      {t('管理员')}
                    </Tag>
                  ) : null}
                  <Tag size='large' shape='circle' style={{ color: 'white' }}>
                    ID: {userState?.user?.id}
                  </Tag>
                  {typeof onChangeAvatar === 'function' && (
                    <Button
                      size='small'
                      theme='borderless'
                      type='tertiary'
                      icon={<RefreshCw size={14} />}
                      loading={avatarUpdating}
                      onClick={onChangeAvatar}
                      className='!text-white/90 hover:!text-white hover:!bg-white/10 !rounded-full'
                    >
                      {t('更换头像')}
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {showExtendedInfo && (
        <div className='p-6'>
          {/* 当前余额和桌面版统计信息 */}
          <div className='flex items-start justify-between gap-6'>
            {/* 当前余额显示 */}
            <Badge count={t('当前余额')} position='rightTop' type='danger'>
              <div className='text-2xl sm:text-3xl md:text-4xl font-bold tracking-wide'>
                {renderQuota(userState?.user?.quota)}
              </div>
            </Badge>

            {/* 桌面版统计信息（Semi UI 卡片） */}
            <div className='hidden lg:block flex-shrink-0'>
              <Card
                size='small'
                className='!rounded-xl'
                bodyStyle={{ padding: '12px 16px' }}
              >
                <div className='flex items-center gap-4'>
                  <div className='flex items-center gap-2'>
                    <Coins size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('历史消耗')}
                    </Typography.Text>
                    <Typography.Text size='small' type='tertiary' strong>
                      {renderQuota(actualUsedQuota)}
                    </Typography.Text>
                    {showDualUsedQuota ? (
                      <Typography.Text size='small' type='tertiary'>
                        {t('标准费用')}: {renderQuota(standardUsedQuota)}
                      </Typography.Text>
                    ) : null}
                  </div>
                  <Divider layout='vertical' />
                  <div className='flex items-center gap-2'>
                    <BarChart2 size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('请求次数')}
                    </Typography.Text>
                    <Typography.Text size='small' type='tertiary' strong>
                      {userState.user?.request_count || 0}
                    </Typography.Text>
                  </div>
                  <Divider layout='vertical' />
                  <div className='flex items-center gap-2'>
                    <Users size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('用户分组')}
                    </Typography.Text>
                    <Typography.Text size='small' type='tertiary' strong>
                      {userState?.user?.group || t('默认')}
                    </Typography.Text>
                  </div>
                </div>
              </Card>
            </div>
          </div>

          {/* 移动端和中等屏幕统计信息卡片 */}
          <div className='lg:hidden mt-2'>
            <Card
              size='small'
              className='!rounded-xl'
              bodyStyle={{ padding: '12px 16px' }}
            >
              <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <Coins size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('历史消耗')}
                    </Typography.Text>
                  </div>
                  <Typography.Text size='small' type='tertiary' strong>
                    {renderQuota(actualUsedQuota)}
                  </Typography.Text>
                </div>
                {showDualUsedQuota ? (
                  <>
                    <Divider margin='8px' />
                    <div className='flex items-center justify-between'>
                      <div className='flex items-center gap-2'>
                        <Coins size={16} />
                        <Typography.Text size='small' type='tertiary'>
                          {t('标准费用')}
                        </Typography.Text>
                      </div>
                      <Typography.Text size='small' type='tertiary' strong>
                        {renderQuota(standardUsedQuota)}
                      </Typography.Text>
                    </div>
                  </>
                ) : null}
                <Divider margin='8px' />
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <BarChart2 size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('请求次数')}
                    </Typography.Text>
                  </div>
                  <Typography.Text size='small' type='tertiary' strong>
                    {userState.user?.request_count || 0}
                  </Typography.Text>
                </div>
                <Divider margin='8px' />
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <Users size={16} />
                    <Typography.Text size='small' type='tertiary'>
                      {t('用户分组')}
                    </Typography.Text>
                  </div>
                  <Typography.Text size='small' type='tertiary' strong>
                    {userState?.user?.group || t('默认')}
                  </Typography.Text>
                </div>
              </div>
            </Card>
          </div>
        </div>
      )}
    </section>
  );
};

export default UserInfoHeader;
