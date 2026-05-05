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

import React, { useEffect, useState } from 'react';
import { API, showError } from '../../helpers';
import { marked } from 'marked';
import { Empty } from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { useTranslation } from 'react-i18next';

const About = () => {
  const { t } = useTranslation();
  const [about, setAbout] = useState('');
  const [aboutLoaded, setAboutLoaded] = useState(false);
  const currentYear = new Date().getFullYear();

  const displayAbout = async () => {
    setAbout(localStorage.getItem('about') || '');
    const res = await API.get('/api/about');
    const { success, message, data } = res.data;
    if (success) {
      let aboutContent = data;
      if (!data.startsWith('https://')) {
        aboutContent = marked.parse(data);
      }
      setAbout(aboutContent);
      localStorage.setItem('about', aboutContent);
    } else {
      showError(message);
      setAbout(t('加载关于内容失败...'));
    }
    setAboutLoaded(true);
  };

  useEffect(() => {
    displayAbout().then();
  }, []);

  const emptyStyle = {
    padding: '24px',
  };

  const customDescription = (
    <div style={{ textAlign: 'center' }}>
      <div className='text-sm font-medium text-semi-color-text-0'>
        {t('管理员暂时未设置任何关于内容')}
      </div>
      <div className='mt-2 text-xs text-semi-color-text-2'>
        {t('可在后台设置「关于」内容，支持 HTML 与 Markdown')}
      </div>
    </div>
  );

  return (
    <div className='pt-24 pb-16'>
      <div className='app-container'>
        {aboutLoaded && about === '' ? (
          <div className='flex justify-center items-center min-h-[60vh] p-8'>
            <Empty
              image={
                <IllustrationConstruction style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationConstructionDark
                  style={{ width: 150, height: 150 }}
                />
              }
              description={t('管理员暂时未设置任何关于内容')}
              style={emptyStyle}
            >
              {customDescription}
            </Empty>
          </div>
        ) : (
          <>
            {about.startsWith('https://') ? (
              <iframe
                src={about}
                className='w-full h-[calc(100vh-120px)] border-none'
              />
            ) : (
              <div
                className='app-surface-solid p-6 md:p-8'
                style={{ fontSize: 'larger' }}
                dangerouslySetInnerHTML={{ __html: about }}
              ></div>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default About;
