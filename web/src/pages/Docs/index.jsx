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
import { Tabs } from '@douyinfe/semi-ui';
import MarkdownRenderer from '../../components/common/markdown/MarkdownRenderer';

import codexWindows from './content/codex/windows.md?raw';
import codexMacos from './content/codex/macos.md?raw';
import codexLinux from './content/codex/linux.md?raw';

const Docs = () => {
  return (
    <div className='pt-24 pb-16'>
      <div className='app-container'>
        <div className='mb-6'>
          <div className='text-2xl font-semibold text-semi-color-text-0'>
            使用说明
          </div>
          <div className='mt-2 text-sm text-semi-color-text-2'>
            按产品与操作系统分类的快速指南
          </div>
        </div>

        <div className='app-surface-solid p-4 md:p-6'>
          <Tabs type='button' defaultActiveKey='codex'>
            <Tabs.TabPane tab='CodeX' itemKey='codex'>
              <div className='mt-4'>
                <Tabs type='line' defaultActiveKey='windows'>
                  <Tabs.TabPane tab='Windows' itemKey='windows'>
                    <MarkdownRenderer
                      content={codexWindows}
                      enableBreaks={false}
                      fontSize={15}
                      className='docs-markdown'
                      style={{ lineHeight: '1.75' }}
                    />
                  </Tabs.TabPane>
                  <Tabs.TabPane tab='macOS' itemKey='macos'>
                    <MarkdownRenderer
                      content={codexMacos}
                      enableBreaks={false}
                      fontSize={15}
                      className='docs-markdown'
                      style={{ lineHeight: '1.75' }}
                    />
                  </Tabs.TabPane>
                  <Tabs.TabPane tab='Linux' itemKey='linux'>
                    <MarkdownRenderer
                      content={codexLinux}
                      enableBreaks={false}
                      fontSize={15}
                      className='docs-markdown'
                      style={{ lineHeight: '1.75' }}
                    />
                  </Tabs.TabPane>
                </Tabs>
              </div>
            </Tabs.TabPane>

            <Tabs.TabPane tab='Claude Code' itemKey='claude'>
              <div className='p-6 text-semi-color-text-2'>
                暂未提供该产品的快速指南。
              </div>
            </Tabs.TabPane>

            <Tabs.TabPane tab='GLM' itemKey='glm'>
              <div className='p-6 text-semi-color-text-2'>
                暂未提供该产品的快速指南。
              </div>
            </Tabs.TabPane>

            <Tabs.TabPane tab='OpenAI 企业级代码助手' itemKey='openai_enterprise'>
              <div className='p-6 text-semi-color-text-2'>
                暂未提供该产品的快速指南。
              </div>
            </Tabs.TabPane>
          </Tabs>
        </div>
      </div>
    </div>
  );
};

export default Docs;
