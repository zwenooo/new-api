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
import { Modal, TextArea, Typography } from '@douyinfe/semi-ui';

const BatchBindUsersModal = ({
  showBatchBindUsers,
  setShowBatchBindUsers,
  batchBindChannelUsers,
  batchBindUsersValue,
  setBatchBindUsersValue,
  selectedChannels,
  t,
}) => {
  return (
    <Modal
      title={t('批量绑定用户')}
      visible={showBatchBindUsers}
      onOk={batchBindChannelUsers}
      onCancel={() => setShowBatchBindUsers(false)}
      maskClosable={false}
      centered={true}
      size='small'
      className='!rounded-lg'
    >
      <div className='mb-3'>
        <Typography.Text>
          {t('请输入要绑定的用户ID（逗号/空格/换行分隔，留空表示清空绑定）')}
        </Typography.Text>
      </div>
      <TextArea
        placeholder={t('例如：1,2,3')}
        value={batchBindUsersValue}
        onChange={(v) => setBatchBindUsersValue(v)}
        autosize={{ minRows: 3, maxRows: 8 }}
        showClear
      />
      <div className='mt-4'>
        <Typography.Text type='secondary'>
          {t('已选择 ${count} 个渠道').replace(
            '${count}',
            selectedChannels.length,
          )}
        </Typography.Text>
      </div>
    </Modal>
  );
};

export default BatchBindUsersModal;
