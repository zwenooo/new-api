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
import { useTranslation } from 'react-i18next';
import TokensTable from '../../components/table/tokens';
import ConsolePage from '../../components/layout/ConsolePage';
import { isEffectiveAdmin } from '../../helpers';
import UserTokensPanel from '../../components/dashboard/UserTokensPanel';
import BaseUrlPanel from '../../components/dashboard/BaseUrlPanel';
import {
  CARD_PROPS,
  FLEX_CENTER_GAP2,
} from '../../constants/dashboard.constants';

const Token = () => {
  const { t } = useTranslation();
  const isAdminUser = isEffectiveAdmin();

  return (
    <ConsolePage fillHeight>
      {isAdminUser ? (
        <TokensTable />
      ) : (
        <div className='grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-12 lg:grid-rows-1'>
          <div className='flex min-h-0 flex-col lg:col-span-8'>
            <div className='flex min-h-0 flex-1 overflow-hidden'>
              <UserTokensPanel
                CARD_PROPS={CARD_PROPS}
                FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
                t={t}
              />
            </div>
          </div>

          <div className='flex min-h-0 flex-col lg:col-span-4'>
            <div className='flex min-h-0 flex-1 overflow-hidden'>
              <BaseUrlPanel CARD_PROPS={CARD_PROPS} t={t} />
            </div>
          </div>
        </div>
      )}
    </ConsolePage>
  );
};

export default Token;
