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
import { useSetTheme, useTheme } from '../../../context/Theme';

const ThemeSwitcher = () => {
  const { t } = useTranslation();
  const theme = useTheme();
  const setTheme = useSetTheme();
  const themeOptions = [
    { key: 'light', label: t('浅色'), title: t('始终使用浅色主题') },
    { key: 'dark', label: t('深色'), title: t('始终使用深色主题') },
    { key: 'system', label: t('跟随系统'), title: t('跟随系统主题设置') },
  ];

  return (
    <div className='app-theme-switcher' role='group' aria-label={t('切换主题')}>
      {themeOptions.map((option) => (
        <button
          key={option.key}
          type='button'
          title={option.title}
          aria-pressed={theme === option.key}
          className={`app-theme-option ${theme === option.key ? 'is-active' : ''}`}
          onClick={() => setTheme(option.key)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
};

export default ThemeSwitcher;
