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

const UsersActions = ({
  setShowAddUser,
  setShowUserGroupManagement,
  setShowBulkUpdateSubscription,
  setShowBulkCompensateSubscription,
  setShowBulkExtendOriginalSubscription,
  t,
}) => {
  // Add new user
  const handleAddUser = () => {
    setShowAddUser(true);
  };

  const handleBulkUpdateSubscription = () => {
    setShowBulkUpdateSubscription(true);
  };

  const handleBulkCompensateSubscription = () => {
    setShowBulkCompensateSubscription(true);
  };

  const handleBulkExtendOriginalSubscription = () => {
    setShowBulkExtendOriginalSubscription(true);
  };

  const handleUserGroupManagement = () => {
    setShowUserGroupManagement(true);
  };

  return (
    <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
      <Button className='w-full md:w-auto' onClick={handleAddUser} size='small'>
        {t('添加用户')}
      </Button>
      <Button
        className='w-full md:w-auto'
        onClick={handleUserGroupManagement}
        size='small'
        theme='light'
      >
        {t('用户分组')}
      </Button>
      <Button
        className='w-full md:w-auto'
        onClick={handleBulkUpdateSubscription}
        size='small'
        theme='light'
        type='primary'
      >
        {t('批量调整订阅时长')}
      </Button>
      <Button
        className='w-full md:w-auto'
        onClick={handleBulkCompensateSubscription}
        size='small'
        theme='light'
        type='secondary'
      >
        {t('批量补偿订阅')}
      </Button>
      <Button
        className='w-full md:w-auto'
        onClick={handleBulkExtendOriginalSubscription}
        size='small'
        theme='light'
        type='tertiary'
      >
        {t('批量延长原订阅')}
      </Button>
    </div>
  );
};

export default UsersActions;
