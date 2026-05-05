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

import React, { useCallback, useContext, useEffect, useRef, useState } from 'react';
import { Button, Card } from '@douyinfe/semi-ui';
import { Link2 } from 'lucide-react';
import { copy, showSuccess } from '../../helpers';
import { StatusContext } from '../../context/Status';

const BaseUrlPanel = ({ CARD_PROPS, t }) => {
  const [statusState] = useContext(StatusContext);
  const baseUrls = statusState?.status?.base_urls;
  const baseUrlList = Array.isArray(baseUrls)
    ? baseUrls
        .map((it) => (typeof it === 'string' ? it.trim() : ''))
        .filter((it) => it)
    : [];

  const parseBaseUrlEntry = (raw) => {
    const text = String(raw || '').trim();
    if (!text) return { label: '', value: '' };

    // Prefer unambiguous delimiter: label|value
    const pipeIndex = text.indexOf('|');
    if (pipeIndex !== -1) {
      const label = text.slice(0, pipeIndex).trim();
      const value = text.slice(pipeIndex + 1).trim();
      return { label: label || value, value };
    }

    // Support label,value and label，value (copy will only use the value part)
    const asciiCommaIndex = text.indexOf(',');
    const cnCommaIndex = text.indexOf('，');
    const commaIndex =
      asciiCommaIndex !== -1 && cnCommaIndex !== -1
        ? Math.min(asciiCommaIndex, cnCommaIndex)
        : Math.max(asciiCommaIndex, cnCommaIndex);
    if (commaIndex !== -1) {
      const label = text.slice(0, commaIndex).trim();
      const value = text.slice(commaIndex + 1).trim();
      const valueLower = value.toLowerCase();
      if (valueLower.startsWith('http://') || valueLower.startsWith('https://')) {
        return { label: label || value, value };
      }
    }

    return { label: text, value: text };
  };

  const [showShadow, setShowShadow] = useState(false);
  const scrollRef = useRef(null);

  const updateShadow = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const threshold = 2;
    const canScroll = el.scrollHeight - el.clientHeight > threshold;
    const notAtBottom = el.scrollTop + el.clientHeight < el.scrollHeight - threshold;
    setShowShadow(canScroll && notAtBottom);
  }, []);

  useEffect(() => {
    updateShadow();
  }, [baseUrlList, updateShadow]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.addEventListener('scroll', updateShadow);
    return () => el.removeEventListener('scroll', updateShadow);
  }, [updateShadow]);

  const handleCopy = async (value) => {
    const text = String(value || '').trim();
    if (!text || text === '-') return;
    if (await copy(text)) {
      showSuccess(t('已复制到剪贴板！'));
    }
  };

  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-xl !shadow-none w-full flex min-h-0 flex-1 flex-col overflow-hidden'
      headerStyle={{ padding: 0 }}
      header={
        <div className='flex items-center gap-2 px-4 py-1.5'>
          <Link2 size={16} />
          {t('基址')}
        </div>
      }
      bodyStyle={{
        padding: 0,
        display: 'flex',
        flexDirection: 'column',
        flex: 1,
        minHeight: 0,
      }}
    >
      <div className='flex min-h-0 flex-1 flex-col px-4 pb-4 pt-1'>
        <div className='relative flex min-h-0 flex-1 flex-col'>
          <div
            ref={scrollRef}
            onScroll={updateShadow}
            className={`space-y-2 min-h-0 flex-1${
              baseUrlList.length > 1 ? ' overflow-y-auto card-content-scroll pr-1' : ''
            }`}
          >
            {(baseUrlList.length > 0 ? baseUrlList : ['-']).map((raw, idx) => {
              const { label, value } = parseBaseUrlEntry(raw);
              return (
                <div
                  key={`${raw}_${idx}`}
                  className='dashboard-info-card dashboard-info-card--static flex items-start justify-between gap-4 rounded-2xl px-4 py-3.5'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='truncate text-sm font-semibold tracking-[0.01em] text-neutral-700 dark:text-neutral-100'>
                      {label || '-'}
                    </div>
                    {value && value !== label && (
                      <div className='mt-2 truncate text-xs font-mono leading-5 tracking-[0.02em] text-neutral-500 dark:text-neutral-400'>
                        {value}
                      </div>
                    )}
                  </div>
                  <Button
                    size='small'
                    type='primary'
                    theme='solid'
                    className='!rounded-lg'
                    disabled={!value || value === '-'}
                    onClick={() => handleCopy(value)}
                  >
                    {t('复制')}
                  </Button>
                </div>
              );
            })}
          </div>
          {showShadow && <div className='card-content-fade-indicator opacity-100' />}

        </div>
      </div>
    </Card>
  );
};

export default BaseUrlPanel;
