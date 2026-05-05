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
import { Link } from 'react-router-dom';
import { Typography } from '@douyinfe/semi-ui';
import SkeletonWrapper from '../components/SkeletonWrapper';

const HeaderLogo = ({
  logo,
  logoLoaded,
  isLoading,
  systemName,
  isSelfUseMode,
  isDemoSiteMode,
  t,
}) => {
  return (
    <Link to='/' className='app-brand-link group'>
      <div className='app-brand-mark'>
        <SkeletonWrapper loading={!logoLoaded} type='image' />
        <img
          src={logo}
          alt='logo'
          className={`absolute inset-0 h-full w-full object-contain transition-opacity duration-200 ${logoLoaded ? 'opacity-100' : 'opacity-0'}`}
        />
      </div>
      <div className='app-brand-copy'>
        <SkeletonWrapper
          loading={isLoading}
          type='title'
          width={120}
          height={24}
        >
          <Typography.Title
            heading={4}
            className='app-brand-title !mb-0 !min-w-0 !truncate'
          >
            {systemName}
          </Typography.Title>
        </SkeletonWrapper>
        {(isSelfUseMode || isDemoSiteMode) && !isLoading && (
          <span className='app-brand-badge'>
            {isSelfUseMode ? t('自用模式') : t('演示站点')}
          </span>
        )}
      </div>
    </Link>
  );
};

export default HeaderLogo;
