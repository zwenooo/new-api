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

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from 'react';

const ThemeContext = createContext(null);
export const useTheme = () => useContext(ThemeContext);

const ActualThemeContext = createContext(null);
export const useActualTheme = () => useContext(ActualThemeContext);

const SetThemeContext = createContext(null);
export const useSetTheme = () => useContext(SetThemeContext);

const THEME_STORAGE_KEY = 'theme-preference';
const THEME_OPTIONS = ['light', 'dark', 'system'];

const getSystemTheme = () => {
  if (
    typeof window !== 'undefined' &&
    window.matchMedia('(prefers-color-scheme: dark)').matches
  ) {
    return 'dark';
  }

  return 'light';
};

export const ThemeProvider = ({ children }) => {
  const [theme, setThemeState] = useState(() => {
    if (typeof window === 'undefined') {
      return 'system';
    }

    const savedTheme = localStorage.getItem(THEME_STORAGE_KEY);

    return THEME_OPTIONS.includes(savedTheme) ? savedTheme : 'system';
  });

  const [systemTheme, setSystemTheme] = useState(getSystemTheme);

  const actualTheme = theme === 'system' ? systemTheme : theme;

  useEffect(() => {
    if (typeof window === 'undefined') {
      return undefined;
    }

    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const handleThemeChange = (event) => {
      setSystemTheme(event.matches ? 'dark' : 'light');
    };

    setSystemTheme(media.matches ? 'dark' : 'light');

    if (media.addEventListener) {
      media.addEventListener('change', handleThemeChange);
      return () => media.removeEventListener('change', handleThemeChange);
    }

    media.addListener(handleThemeChange);
    return () => media.removeListener(handleThemeChange);
  }, []);

  useLayoutEffect(() => {
    const body = document.body;
    const root = document.documentElement;

    body.setAttribute('theme-mode', actualTheme);
    root.setAttribute('data-theme', actualTheme);
    root.classList.toggle('dark', actualTheme === 'dark');
  }, [actualTheme]);

  const setTheme = useCallback((nextTheme) => {
    if (!THEME_OPTIONS.includes(nextTheme)) {
      return;
    }

    localStorage.setItem(THEME_STORAGE_KEY, nextTheme);
    setThemeState(nextTheme);
  }, []);

  const themeValue = useMemo(() => theme, [theme]);
  const actualThemeValue = useMemo(() => actualTheme, [actualTheme]);

  return (
    <SetThemeContext.Provider value={setTheme}>
      <ActualThemeContext.Provider value={actualThemeValue}>
        <ThemeContext.Provider value={themeValue}>{children}</ThemeContext.Provider>
      </ActualThemeContext.Provider>
    </SetThemeContext.Provider>
  );
};
