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
import { NavLink } from 'react-router-dom';
import SkeletonWrapper from '../components/SkeletonWrapper';

const Navigation = ({
  mainNavLinks,
  isMobile,
  isLoading,
  pricingRequireAuth,
}) => {
  const renderNavLinks = () => {
    const isLoggedIn = Boolean(localStorage.getItem('user'));
    const linksToRender = isLoggedIn
      ? mainNavLinks
      : mainNavLinks.filter(
          (link) =>
            link.itemKey !== 'home' &&
            link.itemKey !== 'console' &&
            link.itemKey !== 'pricing' &&
            link.itemKey !== 'docs',
        );

    const commonLinkClasses = `app-nav-link${isMobile ? ' !h-8 !px-2.5' : ''}`;

    return linksToRender.map((link) => {
      const linkContent = <span>{link.text}</span>;

      if (link.isExternal) {
        return (
          <a
            key={link.itemKey}
            href={link.externalLink}
            target='_blank'
            rel='noopener noreferrer'
            className={commonLinkClasses}
          >
            {linkContent}
          </a>
        );
      }

      let targetPath = link.to;
      if (link.itemKey === 'console' && !isLoggedIn) {
        targetPath = '/login';
      }
      if (link.itemKey === 'pricing' && pricingRequireAuth && !isLoggedIn) {
        targetPath = '/login';
      }

      return (
        <NavLink
          key={link.itemKey}
          to={targetPath}
          end
          className={({ isActive }) =>
            `${commonLinkClasses}${isActive ? ' is-active' : ''}`
          }
        >
          {linkContent}
        </NavLink>
      );
    });
  };

  return (
    <nav className='flex flex-1 items-center gap-1 lg:gap-2 mx-2 md:mx-4 overflow-x-auto whitespace-nowrap scrollbar-hide'>
      <SkeletonWrapper
        loading={isLoading}
        type='navigation'
        count={4}
        width={60}
        height={16}
        isMobile={isMobile}
      >
        {renderNavLinks()}
      </SkeletonWrapper>
    </nav>
  );
};

export default Navigation;
