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
import { Button } from '@douyinfe/semi-ui';
import { Shield, UserRound } from 'lucide-react';
import { useViewerMode } from '../../../context/ViewerMode';
import { canUseUserView } from '../../../helpers/viewerMode';

const ViewModeToggle = ({ userState, isLoading, t }) => {
  const { isUserView, toggleMode } = useViewerMode();

  if (isLoading || !canUseUserView(userState?.user)) {
    return null;
  }

  const buttonLabel = isUserView ? t('回到管理') : t('切到用户');
  const ariaLabel = isUserView
    ? t('切换回管理员视角')
    : t('切换到普通用户视角');
  const Icon = isUserView ? UserRound : Shield;

  return (
    <Button
      theme={isUserView ? 'solid' : 'borderless'}
      type='tertiary'
      size='small'
      icon={<Icon size={14} />}
      aria-label={ariaLabel}
      onClick={toggleMode}
      className='rounded-full'
    >
      {buttonLabel}
    </Button>
  );
};

export default ViewModeToggle;
