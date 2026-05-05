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

import { memo, useCallback } from 'react';
import { Input, Button, Tag } from '@douyinfe/semi-ui';
import { IconSearch, IconCopy } from '@douyinfe/semi-icons';
import { LayoutGrid, Table2 } from 'lucide-react';

const PricingToolbar = memo(
  ({
    searchValue,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    selectedRowKeys = [],
    copyText,
    viewMode,
    setViewMode,
    filteredModels = [],
    isMobile,
    t,
  }) => {
    const handleCopyClick = useCallback(() => {
      if (copyText && selectedRowKeys.length > 0) {
        copyText(selectedRowKeys);
      }
    }, [copyText, selectedRowKeys]);

    return (
      <div className='pricing-toolbar'>
        <div className='flex items-center gap-3 flex-wrap'>
          <div className='flex-1 min-w-[200px] max-w-[320px]'>
            <Input
              prefix={<IconSearch />}
              placeholder={t('搜索模型...')}
              value={searchValue}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onChange={handleChange}
              showClear
              size='default'
            />
          </div>

          <div className='flex items-center gap-2 text-sm text-gray-500'>
            <span>
              {t('共 {{count}} 个模型', { count: filteredModels.length })}
            </span>
            {selectedRowKeys.length > 0 && (
              <Tag color='blue' size='small'>
                {t('已选 {{count}}', { count: selectedRowKeys.length })}
              </Tag>
            )}
          </div>

          <div className='flex items-center gap-2 ml-auto'>
            <Button
              icon={<IconCopy />}
              onClick={handleCopyClick}
              disabled={selectedRowKeys.length === 0}
              size='default'
            >
              {!isMobile && t('复制')}
            </Button>

            <div className='flex items-center rounded-lg overflow-hidden bg-semi-color-fill-0 dark:bg-semi-color-fill-1'>
              <button
                className={`p-2 transition-colors ${viewMode === 'card' ? 'bg-semi-color-fill-2 dark:bg-semi-color-fill-2' : 'hover:bg-semi-color-fill-1 dark:hover:bg-semi-color-fill-2'}`}
                onClick={() => setViewMode('card')}
                title={t('卡片视图')}
              >
                <LayoutGrid size={18} />
              </button>
              <button
                className={`p-2 transition-colors ${viewMode === 'table' ? 'bg-semi-color-fill-2 dark:bg-semi-color-fill-2' : 'hover:bg-semi-color-fill-1 dark:hover:bg-semi-color-fill-2'}`}
                onClick={() => setViewMode('table')}
                title={t('表格视图')}
              >
                <Table2 size={18} />
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  },
);

PricingToolbar.displayName = 'PricingToolbar';

export default PricingToolbar;
