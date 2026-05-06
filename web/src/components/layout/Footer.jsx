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

import React, { useContext, useEffect, useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { getFooterHTML, getLogo, getSystemName } from '../../helpers';
import { StatusContext } from '../../context/Status';

const UPSTREAM_SOURCE_URL = 'https://github.com/QuantumNous/new-api';
const PUBLIC_SOURCE_URL = 'https://github.com/zwenooo/new-api';
const AGPL_LICENSE_URL = 'https://www.gnu.org/licenses/agpl-3.0.html';

const FooterBar = () => {
  const { t } = useTranslation();
  const [footer, setFooter] = useState(getFooterHTML());
  const [statusState] = useContext(StatusContext);
  const systemName = getSystemName();
  const logo = getLogo() || '/logo.png';
  const version = String(statusState?.status?.version || '').trim();
  const sourceUrl =
    version && version !== 'v0.0.0'
      ? `${PUBLIC_SOURCE_URL}/tree/${encodeURIComponent(version)}`
      : PUBLIC_SOURCE_URL;

  const loadFooter = () => {
    let footer_html = localStorage.getItem('footer_html');
    if (footer_html) {
      setFooter(footer_html);
    }
  };

  const currentYear = new Date().getFullYear();

  const attribution = useMemo(
    () => (
      <div className='app-footer-attribution'>
        <span>{t('基于 New API 修改发布')}</span>
        <span className='app-footer-separator'>/</span>
        <span>
          {t('设计与开发由')}{' '}
          <a
            href={UPSTREAM_SOURCE_URL}
            target='_blank'
            rel='noopener noreferrer'
            className='app-footer-link app-footer-link--primary'
          >
            New API
          </a>
        </span>
        {version && (
          <>
            <span className='app-footer-separator'>/</span>
            <span>{version}</span>
          </>
        )}
      </div>
    ),
    [t, version],
  );

  const customFooter = useMemo(
    () => (
      <footer className='w-full py-10'>
        <div className='app-container'>
          <div className='app-footer-shell'>
            <div className='app-footer-card'>
              <div className='app-footer-brand'>
                <div className='app-footer-mark'>
                  <img
                    src={logo}
                    alt={systemName}
                    className='h-full w-full object-contain'
                  />
                </div>
                <div className='min-w-0'>
                  <div className='app-footer-title truncate'>{systemName}</div>
                  <div className='app-footer-meta'>
                    © {currentYear} · {t('版权所有')}
                  </div>
                </div>
              </div>

              <div className='app-footer-content'>
                {attribution}
                <div className='app-footer-links'>
                  <Link to='/console' className='app-footer-link'>
                    {t('控制台')}
                  </Link>
                  <Link to='/console/pricing' className='app-footer-link'>
                    {t('模型')}
                  </Link>
                  <Link to='/about' className='app-footer-link'>
                    {t('关于')}
                  </Link>
                  <a
                    href={sourceUrl}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='app-footer-link'
                  >
                    {t('对应源码')}
                  </a>
                  <a
                    href={AGPL_LICENSE_URL}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='app-footer-link'
                  >
                    AGPL-3.0
                  </a>
                </div>
              </div>
            </div>
          </div>
        </div>
      </footer>
    ),
    [logo, systemName, t, currentYear, attribution, sourceUrl],
  );

  useEffect(() => {
    loadFooter();
  }, []);

  return (
    <div className='w-full'>
      {footer ? (
        <div className='app-footer-custom'>
          <div
            className='custom-footer'
            dangerouslySetInnerHTML={{ __html: footer }}
          ></div>
          <div className='app-footer-custom-attribution'>
            {attribution}
            <a
              href={sourceUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='app-footer-link'
            >
              {t('对应源码')}
            </a>
            <a
              href={AGPL_LICENSE_URL}
              target='_blank'
              rel='noopener noreferrer'
              className='app-footer-link'
            >
              AGPL-3.0
            </a>
          </div>
        </div>
      ) : (
        customFooter
      )}
    </div>
  );
};

export default FooterBar;
