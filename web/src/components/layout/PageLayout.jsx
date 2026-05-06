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

import HeaderBar from './headerbar';
import SiderBar from './SiderBar';
import App from '../../App';
import FooterBar from './Footer';
import { ToastContainer } from 'react-toastify';
import React, { useContext, useEffect, useState } from 'react';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useTranslation } from 'react-i18next';
import {
  API,
  getLogo,
  getSystemName,
  showError,
  setStatusData,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useLocation } from 'react-router-dom';

const PageLayout = () => {
  const [, userDispatch] = useContext(UserContext);
  const [, statusDispatch] = useContext(StatusContext);
  const isMobile = useIsMobile();
  const [collapsed, , setCollapsed] = useSidebarCollapsed();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { i18n } = useTranslation();
  const location = useLocation();

  const shouldInnerPadding =
    location.pathname.includes('/console') &&
    !location.pathname.startsWith('/console/chat') &&
    location.pathname !== '/console/playground' &&
    location.pathname !== '/console/pricing';
  const isAuthRoute =
    location.pathname === '/' ||
    location.pathname === '/login' ||
    location.pathname === '/register' ||
    location.pathname === '/reset' ||
    location.pathname === '/user/reset' ||
    location.pathname.startsWith('/oauth');

  const hasUser = Boolean(localStorage.getItem('user'));
  const isConsoleRoute = hasUser && location.pathname.startsWith('/console');
  const isLogsRoute = location.pathname === '/console/log';
  const showDesktopSider = isConsoleRoute && !isMobile;
  const showMobileDrawer = isConsoleRoute && isMobile && drawerOpen;
  const showFooter = !isConsoleRoute;

  useEffect(() => {
    if (!isConsoleRoute) {
      setDrawerOpen(false);
    }
  }, [isConsoleRoute]);

  useEffect(() => {
    if (isMobile && drawerOpen && collapsed) {
      setCollapsed(false);
    }
  }, [isMobile, drawerOpen, collapsed, setCollapsed]);

  const loadUser = () => {
    let user = localStorage.getItem('user');
    if (user) {
      let data = JSON.parse(user);
      userDispatch({ type: 'login', payload: data });
    }
  };

  const loadStatus = async () => {
    try {
      const res = await API.get('/api/status');
      const { success, data } = res.data;
      if (success) {
        statusDispatch({ type: 'set', payload: data });
        setStatusData(data);
      } else {
        showError('Unable to connect to server');
      }
    } catch (error) {
      showError('Failed to load status');
    }
  };

  useEffect(() => {
    loadUser();
    loadStatus().catch(console.error);
    let systemName = getSystemName();
    if (systemName) {
      document.title = systemName;
    }
    let logo = getLogo();
    if (logo) {
      let linkElement = document.querySelector("link[rel~='icon']");
      if (linkElement) {
        linkElement.href = logo;
      }
    }
    const savedLang = localStorage.getItem('i18nextLng');
    if (savedLang) {
      i18n.changeLanguage(savedLang);
    }
  }, [i18n]);

  return (
    <div
      className={`app-shell flex flex-col ${
        isConsoleRoute ? 'h-screen overflow-hidden' : 'min-h-screen'
      } ${isAuthRoute ? 'app-shell--auth' : ''}`.trim()}
    >
      <HeaderBar
        onMobileMenuToggle={() => setDrawerOpen((prev) => !prev)}
        drawerOpen={drawerOpen}
      />

      {showMobileDrawer && (
        <div className='fixed inset-0 z-[200] md:hidden'>
          <div
            className='absolute inset-0 bg-black/30 backdrop-blur-sm'
            onClick={() => setDrawerOpen(false)}
          />
          <div className='app-console-drawer-panel absolute left-0 top-[var(--app-header-height)] h-[calc(100vh-var(--app-header-height))] w-[280px] p-4'>
            <SiderBar onNavigate={() => setDrawerOpen(false)} />
          </div>
        </div>
      )}

      <div
        className={`${isConsoleRoute ? 'flex flex-1 min-h-0 flex-col' : 'flex flex-col'}`.trim()}
      >
        {isConsoleRoute ? (
          <div className='flex flex-1 min-h-0 px-4 pb-8'>
            <div className='app-console-frame flex w-full flex-1 min-h-0 flex-col gap-4 md:flex-row md:gap-5'>
              {showDesktopSider && (
                <aside className='app-console-sidebar-shell hidden h-full w-[var(--sidebar-current-width)] shrink-0 justify-between gap-10 px-4 py-4 md:flex md:flex-col'>
                  <SiderBar />
                </aside>
              )}

              <div className='flex flex-1 min-w-0'>
                <div
                  className={`app-console-main flex w-full flex-1 flex-col gap-2 min-h-0 overflow-hidden ${
                    shouldInnerPadding ? 'app-console-main--padded' : ''
                  } ${isLogsRoute ? 'app-console-main--logs' : ''}`.trim()}
                >
                  <div className='console-scroll flex flex-1 min-h-0 flex-col overflow-y-auto scrollbar-hide'>
                    <App />
                  </div>
                </div>
              </div>
            </div>
          </div>
        ) : (
          <>
            <div className='flex-1 min-h-0'>
              <App />
            </div>
            {showFooter && <FooterBar />}
          </>
        )}
      </div>

      <ToastContainer />
    </div>
  );
};

export default PageLayout;
