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

import React, { useEffect, useRef } from 'react';
import { Typography } from '@douyinfe/semi-ui';
import MarkdownRenderer from '../common/markdown/MarkdownRenderer';
import { ChevronRight, ChevronUp, Brain, Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const ThinkingContent = ({
  message,
  finalExtractedThinkingContent,
  thinkingSource,
  styleState,
  onToggleReasoningExpansion,
}) => {
  const { t } = useTranslation();
  const scrollRef = useRef(null);
  const lastContentRef = useRef('');

  const isThinkingStatus =
    message.status === 'loading' || message.status === 'incomplete';
  const headerText =
    isThinkingStatus && !message.isThinkingComplete
      ? t('思考中...')
      : t('思考过程');

  useEffect(() => {
    if (
      scrollRef.current &&
      finalExtractedThinkingContent &&
      message.isReasoningExpanded
    ) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [finalExtractedThinkingContent, message.isReasoningExpanded]);

  useEffect(() => {
    if (!isThinkingStatus) {
      lastContentRef.current = '';
    }
  }, [isThinkingStatus]);

  if (!finalExtractedThinkingContent) return null;

  let prevLength = 0;
  if (isThinkingStatus && lastContentRef.current) {
    if (finalExtractedThinkingContent.startsWith(lastContentRef.current)) {
      prevLength = lastContentRef.current.length;
    }
  }

  if (isThinkingStatus) {
    lastContentRef.current = finalExtractedThinkingContent;
  }

  return (
    <div
      className='mb-2 overflow-hidden rounded-[18px] sm:mb-4'
      style={{
        border: '1px solid var(--app-border)',
        background: 'var(--app-card)',
        boxShadow: 'var(--app-shadow)',
      }}
    >
      <div
        className='flex cursor-pointer items-center justify-between p-3 transition-colors sm:p-4'
        style={{
          background:
            'linear-gradient(135deg, color-mix(in srgb, var(--app-accent) 13%, var(--app-card) 87%) 0%, color-mix(in srgb, var(--app-accent) 5%, var(--app-card) 95%) 100%)',
          position: 'relative',
          borderBottom: message.isReasoningExpanded
            ? '1px solid var(--app-border)'
            : 'none',
        }}
        onClick={() => onToggleReasoningExpansion(message.id)}
      >
        <div className='absolute inset-0 overflow-hidden'>
          <div
            className='absolute -right-10 -top-10 h-40 w-40 rounded-full opacity-60 blur-3xl'
            style={{ background: 'var(--app-accent-light)' }}
          />
        </div>
        <div className='flex items-center gap-2 sm:gap-4 relative'>
          <div
            className='flex h-6 w-6 items-center justify-center rounded-full shadow-lg sm:h-8 sm:w-8'
            style={{ background: 'var(--app-accent)' }}
          >
            <Brain
              style={{ color: '#ffffff' }}
              size={styleState.isMobile ? 12 : 16}
            />
          </div>
          <div className='flex flex-col'>
            <Typography.Text
              strong
              style={{ color: 'var(--semi-color-text-0)' }}
              className='text-sm sm:text-base'
            >
              {headerText}
            </Typography.Text>
            {thinkingSource && (
              <Typography.Text
                style={{ color: 'var(--semi-color-text-2)' }}
                className='mt-0.5 hidden text-xs sm:block'
              >
                来源: {thinkingSource}
              </Typography.Text>
            )}
          </div>
        </div>
        <div className='flex items-center gap-2 sm:gap-3 relative'>
          {isThinkingStatus && !message.isThinkingComplete && (
            <div className='flex items-center gap-1 sm:gap-2'>
              <Loader2
                style={{ color: 'var(--app-accent)' }}
                className='animate-spin'
                size={styleState.isMobile ? 14 : 18}
              />
              <Typography.Text
                style={{ color: 'var(--semi-color-text-1)' }}
                className='text-xs font-medium sm:text-sm'
              >
                思考中
              </Typography.Text>
            </div>
          )}
          {(!isThinkingStatus || message.isThinkingComplete) && (
            <div
              className='flex h-5 w-5 items-center justify-center rounded-full sm:h-6 sm:w-6'
              style={{ background: 'var(--app-control-bg)' }}
            >
              {message.isReasoningExpanded ? (
                <ChevronUp
                  size={styleState.isMobile ? 12 : 16}
                  style={{ color: 'var(--semi-color-text-1)' }}
                />
              ) : (
                <ChevronRight
                  size={styleState.isMobile ? 12 : 16}
                  style={{ color: 'var(--semi-color-text-1)' }}
                />
              )}
            </div>
          )}
        </div>
      </div>
      <div
        className={`transition-all duration-500 ease-out ${
          message.isReasoningExpanded
            ? 'max-h-96 opacity-100'
            : 'max-h-0 opacity-0'
        } overflow-hidden`}
      >
        {message.isReasoningExpanded && (
          <div className='p-3 sm:p-5 pt-2 sm:pt-4'>
            <div
              ref={scrollRef}
              className='thinking-content-scroll overflow-x-auto overflow-y-auto rounded-lg p-2 shadow-inner sm:rounded-xl'
              style={{
                maxHeight: '200px',
                border: '1px solid var(--app-border)',
                background:
                  'color-mix(in srgb, var(--app-control-bg) 82%, var(--app-card) 18%)',
                scrollbarWidth: 'thin',
                scrollbarColor: 'rgba(0, 0, 0, 0.3) transparent',
              }}
            >
              <div className='prose prose-xs sm:prose-sm max-w-none text-xs sm:text-sm'>
                <MarkdownRenderer
                  content={finalExtractedThinkingContent}
                  className=''
                  animated={isThinkingStatus}
                  previousContentLength={prevLength}
                />
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default ThinkingContent;
