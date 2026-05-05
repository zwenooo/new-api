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
import { Modal, Select, Typography } from '@douyinfe/semi-ui';

const BatchGroupModal = ({
  showBatchSetGroup,
  setShowBatchSetGroup,
  batchSetChannelGroup,
  batchSetGroupValue,
  setBatchSetGroupValue,
  groupOptions,
  selectedChannels,
  t,
}) => {
  return (
    <Modal
      title={t('批量设置分组')}
      visible={showBatchSetGroup}
      onOk={batchSetChannelGroup}
      onCancel={() => setShowBatchSetGroup(false)}
      maskClosable={false}
      centered
      size='small'
      className='!rounded-lg'
    >
      <div className='mb-4'>
        <Typography.Text>{t('请选择要设置的分组')}</Typography.Text>
      </div>
      <Select
        placeholder={t('请选择分组')}
        value={batchSetGroupValue}
        style={{ width: '100%' }}
        onChange={(v) => setBatchSetGroupValue(v)}
        optionList={groupOptions}
        multiple
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

export default BatchGroupModal;
