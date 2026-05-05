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

import React, { useEffect, useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { getFooterHTML, getLogo, getSystemName } from '../../helpers';

const FooterBar = () => {
  const { t } = useTranslation();
  const [footer, setFooter] = useState(getFooterHTML());
  const systemName = getSystemName();
  const logo = getLogo() || '/logo.png';

  const loadFooter = () => {
    let footer_html = localStorage.getItem('footer_html');
    if (footer_html) {
      setFooter(footer_html);
    }
  };

  const currentYear = new Date().getFullYear();

  const customFooter = useMemo(
    () => (
      <footer className='w-full py-12'>
        <div className='app-container'>
          <div className='app-footer-shell'>
            <div className='app-footer-card'>
              <div className='flex items-center gap-3'>
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
              </div>
            </div>
          </div>
        </div>
      </footer>
    ),
    [logo, systemName, t, currentYear],
  );

  useEffect(() => {
    loadFooter();
  }, []);

  return (
    <div className='w-full'>
      {footer ? (
        <div
          className='custom-footer'
          dangerouslySetInnerHTML={{ __html: footer }}
        ></div>
      ) : (
        customFooter
      )}
    </div>
  );
};

export default FooterBar;
