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

import React, { useRef } from 'react';
import { Avatar, Dropdown, Typography } from '@douyinfe/semi-ui';
import { ChevronDown } from 'lucide-react';
import { IconExit, IconUserSetting } from '@douyinfe/semi-icons';
import { getDiceBearAvatarUrl, stringToColor } from '../../../helpers';
import SkeletonWrapper from '../components/SkeletonWrapper';

const UserArea = ({
  userState,
  isLoading,
  isMobile,
  dropdownPosition = 'bottomRight',
  getPopupContainer,
  logout,
  navigate,
  t,
}) => {
  const dropdownRef = useRef(null);
  if (isLoading) {
    return (
      <SkeletonWrapper
        loading={true}
        type='userArea'
        width={50}
        isMobile={isMobile}
      />
    );
  }

  if (userState.user) {
    return (
      <div className='relative' ref={dropdownRef}>
        <Dropdown
          position={dropdownPosition}
          getPopupContainer={
            getPopupContainer ? getPopupContainer : () => dropdownRef.current
          }
          render={
            <Dropdown.Menu className='app-user-menu'>
              <Dropdown.Item onClick={() => navigate('/console/personal')}>
                <div className='flex items-center gap-2'>
                  <IconUserSetting
                    size='small'
                    className='app-user-menu__icon'
                  />
                  <span>{t('个人设置')}</span>
                </div>
              </Dropdown.Item>
              <Dropdown.Item onClick={logout}>
                <div className='flex items-center gap-2'>
                  <IconExit size='small' className='app-user-menu__icon' />
                  <span>{t('退出')}</span>
                </div>
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <button
            type='button'
            className='app-header-user-trigger'
            aria-haspopup='menu'
          >
            <Avatar
              size='extra-small'
              src={getDiceBearAvatarUrl(userState.user.avatar_seed, {
                size: 64,
              })}
              color={stringToColor(userState.user.username)}
            >
              {userState.user.username[0].toUpperCase()}
            </Avatar>
            <span className='hidden md:inline'>
              <Typography.Text className='app-user-name !mb-0'>
                {userState.user.username}
              </Typography.Text>
            </span>
            <ChevronDown size={14} className='text-semi-color-text-2' />
          </button>
        </Dropdown>
      </div>
    );
  } else {
    return null;
  }
};

export default UserArea;
