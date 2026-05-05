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

import React, { useMemo } from 'react';
import { Button, Modal, Empty } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { marked } from 'marked';
import {
  IllustrationNoContent,
  IllustrationNoContentDark,
} from '@douyinfe/semi-illustrations';

const NoticeModal = ({
  visible,
  onClose,
  isMobile,
  noticeMarkdown = '',
  loading = false,
}) => {
  const { t } = useTranslation();

  const noticeHtml = useMemo(() => {
    const trimmed = (noticeMarkdown || '').trim();
    if (!trimmed) return '';
    return marked.parse(trimmed);
  }, [noticeMarkdown]);

  const renderNotice = () => {
    if (loading) {
      return (
        <div className='py-12'>
          <Empty description={t('加载中...')} />
        </div>
      );
    }

    if (!noticeHtml) {
      return (
        <div className='py-12'>
          <Empty
            image={
              <IllustrationNoContent style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoContentDark style={{ width: 150, height: 150 }} />
            }
            description={t('暂无公告')}
          />
        </div>
      );
    }

    return (
      <div
        className='notice-content-scroll max-h-[55vh] overflow-y-auto pr-2'
        dangerouslySetInnerHTML={{ __html: noticeHtml }}
      />
    );
  };

  return (
    <Modal
      title={t('公告')}
      visible={visible}
      onCancel={onClose}
      footer={
        <div className='flex justify-end'>
          <Button type='primary' onClick={onClose}>
            {t('关闭公告')}
          </Button>
        </div>
      }
      size={isMobile ? 'full-width' : 'large'}
    >
      {renderNotice()}
    </Modal>
  );
};

export default NoticeModal;
