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
import { Navigate } from 'react-router-dom';
import { Spin } from '@douyinfe/semi-ui';
import ConsolePage from '../../components/layout/ConsolePage';
import SettingsOrderManagement from '../Setting/Payment/SettingsOrderManagement';
import { isRoot } from '../../helpers';
import { useUserPermissions } from '../../hooks/common/useUserPermissions';

const Order = () => {
  const isRootUser = isRoot();
  const { permissions, loading, isSidebarModuleAllowed } = useUserPermissions();

  if (!isRootUser && loading) {
    return (
      <ConsolePage>
        <div className='app-surface-solid p-4 md:p-6'>
          <Spin spinning />
        </div>
      </ConsolePage>
    );
  }

  if (
    !isRootUser &&
    (!permissions?.sidebar_modules ||
      !isSidebarModuleAllowed('admin', 'order'))
  ) {
    return <Navigate to='/forbidden' replace />;
  }

  return (
    <ConsolePage>
      <div className='app-surface-solid p-4 md:p-6'>
        <SettingsOrderManagement />
      </div>
    </ConsolePage>
  );
};

export default Order;
