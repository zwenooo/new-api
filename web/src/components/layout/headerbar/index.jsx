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
import { useHeaderBar } from '../../../hooks/common/useHeaderBar';
import { useNotifications } from '../../../hooks/common/useNotifications';
import { useNavigation } from '../../../hooks/common/useNavigation';
import NoticeModal from '../NoticeModal';
import MobileMenuButton from './MobileMenuButton';
import HeaderLogo from './HeaderLogo';
import Navigation from './Navigation';
import ActionButtons from './ActionButtons';

const HeaderBar = ({ onMobileMenuToggle, drawerOpen }) => {
  const [isScrolled, setIsScrolled] = useState(false);
  const {
    userState,
    isMobile,
    collapsed,
    logoLoaded,
    isLoading,
    systemName,
    logo,
    isNewYear,
    isSelfUseMode,
    docsLink,
    isDemoSiteMode,
    isConsoleRoute,
    location,
    headerNavModules,
    pricingRequireAuth,
    logout,
    handleMobileMenuToggle,
    navigate,
    t,
  } = useHeaderBar({ onMobileMenuToggle, drawerOpen });

  const hasLocalUser = Boolean(localStorage.getItem('user'));
  const isPricingRoute = location.pathname === '/console/pricing';
  const isAuthRoute =
    location.pathname === '/' ||
    location.pathname === '/login' ||
    location.pathname === '/register' ||
    location.pathname === '/reset' ||
    location.pathname === '/user/reset' ||
    location.pathname.startsWith('/oauth');

  const {
    noticeVisible,
    unreadCount,
    noticeMarkdown,
    noticeLoading,
    handleNoticeOpen,
    handleNoticeClose,
  } = useNotifications();

  const { mainNavLinks } = useNavigation(t, docsLink, headerNavModules);
  const containerClassName =
    isConsoleRoute || isPricingRoute ? 'app-header-container' : 'app-container';

  useEffect(() => {
    const handleScroll = () => {
      setIsScrolled(window.scrollY > 0);
    };

    handleScroll();
    window.addEventListener('scroll', handleScroll, { passive: true });
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <header
      className={`app-header ${isAuthRoute ? 'app-header--auth' : ''} ${
        isConsoleRoute || isPricingRoute ? 'app-header--console' : ''
      } ${isScrolled ? 'app-header--scrolled' : ''}`.trim()}
    >
      <NoticeModal
        visible={noticeVisible}
        onClose={handleNoticeClose}
        isMobile={isMobile}
        noticeMarkdown={noticeMarkdown}
        loading={noticeLoading}
      />

      <div className={containerClassName}>
        <div className='app-header-shell'>
          <div className='app-header-left'>
            <MobileMenuButton
              isConsoleRoute={isConsoleRoute}
              isMobile={isMobile}
              drawerOpen={drawerOpen}
              collapsed={collapsed}
              onToggle={handleMobileMenuToggle}
              t={t}
            />

            <HeaderLogo
              isMobile={isMobile}
              isConsoleRoute={isConsoleRoute}
              logo={logo}
              logoLoaded={logoLoaded}
              isLoading={isLoading}
              systemName={systemName}
              isSelfUseMode={isSelfUseMode}
              isDemoSiteMode={isDemoSiteMode}
              t={t}
            />
          </div>

          {!isConsoleRoute &&
            !isPricingRoute &&
            !(hasLocalUser && isAuthRoute) && (
              <Navigation
                mainNavLinks={mainNavLinks}
                isMobile={isMobile}
                isLoading={isLoading}
                pricingRequireAuth={pricingRequireAuth}
              />
            )}

          <ActionButtons
            isNewYear={isNewYear}
            showThemeSwitcher={isAuthRoute || isConsoleRoute}
            unreadCount={unreadCount}
            onNoticeOpen={handleNoticeOpen}
            userState={userState}
            isLoading={isLoading}
            isMobile={isMobile}
            isSelfUseMode={isSelfUseMode}
            isConsoleRoute={isConsoleRoute}
            logout={logout}
            navigate={navigate}
            t={t}
          />
        </div>
      </div>
    </header>
  );
};

export default HeaderBar;
