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

const BatchModelsModal = ({
  showBatchUpdateModels,
  setShowBatchUpdateModels,
  batchUpdateChannelModels,
  batchAddModelsValue,
  setBatchAddModelsValue,
  batchRemoveModelsValue,
  setBatchRemoveModelsValue,
  selectedChannels,
  t,
}) => {
  return (
    <Modal
      title={t('批量设置模型')}
      visible={showBatchUpdateModels}
      onOk={batchUpdateChannelModels}
      onCancel={() => setShowBatchUpdateModels(false)}
      maskClosable={false}
      centered
      size='medium'
      className='!rounded-lg'
    >
      <div className='mb-3'>
        <Typography.Text>
          {t('为所选渠道统一添加或移除模型')}
        </Typography.Text>
      </div>
      <div className='mb-3'>
        <Typography.Text strong>
          {t('要添加的模型（用逗号或换行分隔）')}
        </Typography.Text>
        <TextArea
          rows={3}
          value={batchAddModelsValue}
          onChange={(v) => setBatchAddModelsValue(v)}
          placeholder='gpt-4.1,gpt-4o-mini'
        />
      </div>
      <div className='mb-3'>
        <Typography.Text strong>
          {t('要移除的模型（用逗号或换行分隔）')}
        </Typography.Text>
        <TextArea
          rows={3}
          value={batchRemoveModelsValue}
          onChange={(v) => setBatchRemoveModelsValue(v)}
          placeholder='gpt-3.5-turbo'
        />
      </div>
      <div className='mt-2 mb-2'>
        <Typography.Text type='secondary'>
          {t('有则操作，无则不变')}
        </Typography.Text>
      </div>
      <div className='mt-2'>
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

export default BatchModelsModal;
