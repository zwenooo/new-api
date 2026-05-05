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
import { Modal, Typography, Input, InputNumber } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';

const TransferModal = ({
  t,
  openTransfer,
  transfer,
  handleTransferCancel,
  userState,
  renderMoneyFen,
  transferAmountYuan,
  setTransferAmountYuan,
}) => {
  const maxYuan = (userState?.user?.aff_quota || 0) / 100;
  const clampTransferAmount = (value) => {
    if (value === undefined || value === null || value === '') {
      setTransferAmountYuan(value);
      return;
    }
    const numericValue = typeof value === 'number' ? value : Number(value);
    if (!Number.isFinite(numericValue)) return;

    const minValue = 0.01;
    const upperBound = Number.isFinite(maxYuan) ? maxYuan : 0;
    const clamped = Math.min(Math.max(numericValue, minValue), upperBound);
    setTransferAmountYuan(clamped);
  };
  return (
    <Modal
      title={
        <div className='flex items-center'>
          <CreditCard className='mr-2' size={18} />
          {t('转入账户余额')}
        </div>
      }
      visible={openTransfer}
      onOk={transfer}
      onCancel={handleTransferCancel}
      maskClosable={false}
      centered
    >
      <div className='space-y-4'>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('返利余额')}
          </Typography.Text>
          <Input
            value={renderMoneyFen(userState?.user?.aff_quota)}
            disabled
            className='!rounded-lg'
          />
        </div>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('账户余额')}
          </Typography.Text>
          <Input
            value={renderMoneyFen(userState?.user?.balance_fen)}
            disabled
            className='!rounded-lg'
          />
        </div>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('转入金额（元）')}
          </Typography.Text>
          <InputNumber
            min={0.01}
            max={maxYuan}
            precision={2}
            value={transferAmountYuan}
            onChange={clampTransferAmount}
            className='w-full !rounded-lg'
          />
        </div>
      </div>
    </Modal>
  );
};

export default TransferModal;
