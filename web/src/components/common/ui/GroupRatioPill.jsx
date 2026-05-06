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
import { Tag } from '@douyinfe/semi-ui';
import { formatRatioValue } from './RatioTag';

const GroupRatioPill = ({ label, ratio, className = '' }) => (
  <Tag
    size='small'
    shape='square'
    type='light'
    color='blue'
    className={`max-w-full ${className}`.trim()}
  >
    <span className='inline-flex max-w-full min-w-0 items-center gap-1 font-mono'>
      <span className='min-w-0 truncate'>{label}</span>
      <span className='shrink-0'>x{formatRatioValue(ratio)}</span>
    </span>
  </Tag>
);

export default GroupRatioPill;
