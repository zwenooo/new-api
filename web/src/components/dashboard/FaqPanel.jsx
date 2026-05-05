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
import { Card, Empty } from '@douyinfe/semi-ui';
import { HelpCircle } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import MarkdownRenderer from '../common/markdown/MarkdownRenderer';

const FaqPanel = ({
  faqData,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  maxHeight = '24rem',
  t,
}) => {
  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-xl !shadow-none lg:col-span-1'
      title={
        <div className={FLEX_CENTER_GAP2}>
          <HelpCircle size={16} />
          {t('常见问答')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight={maxHeight}>
        {faqData.length > 0 ? (
          <div className='flex flex-col'>
            {faqData.map((item, index) => (
              <div
                key={index}
                className='px-6 py-4 border-b border-[color:var(--app-border)] last:border-b-0'
              >
                <div className='text-lg font-semibold text-semi-color-text-0'>
                  {item.question}
                </div>
                <div className='mt-2'>
                  <MarkdownRenderer
                    content={item.answer || ''}
                    className='docs-markdown'
                    enableBreaks={true}
                  />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className='flex justify-center items-center py-8'>
            <Empty
              image={<IllustrationConstruction style={ILLUSTRATION_SIZE} />}
              darkModeImage={
                <IllustrationConstructionDark style={ILLUSTRATION_SIZE} />
              }
              title={t('暂无常见问答')}
              description={t('请联系管理员在系统设置中配置常见问答')}
            />
          </div>
        )}
      </ScrollableContainer>
    </Card>
  );
};

export default FaqPanel;
