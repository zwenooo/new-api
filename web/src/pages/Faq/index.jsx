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

import React, { useContext } from 'react';
import { useTranslation } from 'react-i18next';

import ConsolePage from '../../components/layout/ConsolePage';
import FaqPanel from '../../components/dashboard/FaqPanel';
import { StatusContext } from '../../context/Status';
import {
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
} from '../../constants/dashboard.constants';

const Faq = () => {
  const [statusState] = useContext(StatusContext);
  const { t } = useTranslation();

  const faqEnabled = statusState?.status?.faq_enabled ?? true;
  const faqData = faqEnabled ? statusState?.status?.faq || [] : [];

  return (
    <ConsolePage>
      <div className='grid grid-cols-1 gap-4'>
        <FaqPanel
          faqData={faqData}
          CARD_PROPS={CARD_PROPS}
          FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
          ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
          maxHeight='none'
          t={t}
        />
      </div>
    </ConsolePage>
  );
};

export default Faq;

