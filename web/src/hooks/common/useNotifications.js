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

import { useCallback, useEffect, useMemo, useState } from 'react';
import { API, showError } from '../../helpers';

const NOTICE_READ_KEY = 'notice_read_key';

const hashString = (value) => {
  let hash = 0;
  for (let i = 0; i < value.length; i++) {
    hash = (hash << 5) - hash + value.charCodeAt(i);
    hash |= 0;
  }
  return (hash >>> 0).toString(16);
};

export const useNotifications = () => {
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [noticeMarkdown, setNoticeMarkdown] = useState('');
  const [noticeLoading, setNoticeLoading] = useState(false);

  const noticeKey = useMemo(() => {
    const trimmed = (noticeMarkdown || '').trim();
    if (!trimmed) return '';
    return `notice:${hashString(trimmed)}`;
  }, [noticeMarkdown]);

  const updateUnreadCount = useCallback((key) => {
    if (!key) {
      setUnreadCount(0);
      return;
    }
    const readKey = localStorage.getItem(NOTICE_READ_KEY) || '';
    setUnreadCount(readKey === key ? 0 : 1);
  }, []);

  const refreshNotice = useCallback(
    async ({ silent = false } = {}) => {
      setNoticeLoading(true);
      try {
        const res = await API.get('/api/notice', { skipErrorHandler: true });
        const { success, message, data } = res.data || {};
        if (success && typeof data === 'string') {
          setNoticeMarkdown(data);
        } else {
          setNoticeMarkdown('');
          if (!silent) showError(message || '获取公告失败');
        }
      } catch (error) {
        setNoticeMarkdown('');
        if (!silent) showError(error?.message || error);
      } finally {
        setNoticeLoading(false);
      }
    },
    [setNoticeLoading],
  );

  useEffect(() => {
    refreshNotice({ silent: true }).catch(() => null);
  }, [refreshNotice]);

  useEffect(() => {
    updateUnreadCount(noticeKey);
  }, [noticeKey, updateUnreadCount]);

  const handleNoticeOpen = () => {
    setNoticeVisible(true);
    refreshNotice({ silent: false }).catch(() => null);
  };

  const handleNoticeClose = () => {
    setNoticeVisible(false);
    if (noticeKey) {
      localStorage.setItem(NOTICE_READ_KEY, noticeKey);
    }
    setUnreadCount(0);
  };

  return {
    noticeVisible,
    unreadCount,
    noticeMarkdown,
    noticeLoading,
    handleNoticeOpen,
    handleNoticeClose,
  };
};
