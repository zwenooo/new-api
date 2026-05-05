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

export const VIEWER_MODE_STORAGE_KEY = 'console_view_mode';
export const VIEWER_MODE_CHANGE_EVENT = 'new-api:viewer-mode-change';

export const VIEWER_MODE_ADMIN = 'admin';
export const VIEWER_MODE_USER = 'user';

export const ROLE_GUEST = 0;
export const ROLE_COMMON_USER = 1;
export const ROLE_ADMIN_USER = 10;
export const ROLE_ROOT_USER = 100;

const getStoredUser = () => {
  try {
    const raw = localStorage.getItem('user');
    return raw ? JSON.parse(raw) : null;
  } catch (error) {
    return null;
  }
};

export const getActualRole = (user = getStoredUser()) => {
  const role = Number(user?.role ?? ROLE_GUEST);
  return Number.isFinite(role) ? role : ROLE_GUEST;
};

export const hasActualAdminRole = (user = getStoredUser()) =>
  getActualRole(user) >= ROLE_ADMIN_USER;

export const hasActualRootRole = (user = getStoredUser()) =>
  getActualRole(user) >= ROLE_ROOT_USER;

export const canUseUserView = (user = getStoredUser()) =>
  hasActualAdminRole(user);

export const getViewerMode = () => {
  const stored = localStorage.getItem(VIEWER_MODE_STORAGE_KEY);
  return stored === VIEWER_MODE_USER ? VIEWER_MODE_USER : VIEWER_MODE_ADMIN;
};

const emitViewerModeChange = () => {
  window.dispatchEvent(
    new CustomEvent(VIEWER_MODE_CHANGE_EVENT, {
      detail: { mode: getViewerMode() },
    }),
  );
};

export const setViewerMode = (mode) => {
  const nextMode =
    mode === VIEWER_MODE_USER ? VIEWER_MODE_USER : VIEWER_MODE_ADMIN;
  if (nextMode === VIEWER_MODE_ADMIN) {
    localStorage.removeItem(VIEWER_MODE_STORAGE_KEY);
  } else {
    localStorage.setItem(VIEWER_MODE_STORAGE_KEY, nextMode);
  }
  emitViewerModeChange();
  return nextMode;
};

export const resetViewerMode = () => setViewerMode(VIEWER_MODE_ADMIN);

export const toggleViewerMode = () =>
  setViewerMode(
    getViewerMode() === VIEWER_MODE_USER ? VIEWER_MODE_ADMIN : VIEWER_MODE_USER,
  );

export const isUserViewMode = () => getViewerMode() === VIEWER_MODE_USER;

export const getEffectiveRole = (user = getStoredUser()) => {
  const actualRole = getActualRole(user);
  if (actualRole < ROLE_ADMIN_USER) {
    return actualRole;
  }
  if (getViewerMode() === VIEWER_MODE_USER) {
    return ROLE_COMMON_USER;
  }
  return actualRole;
};

export const isEffectiveAdmin = (user = getStoredUser()) =>
  getEffectiveRole(user) >= ROLE_ADMIN_USER;

export const isEffectiveRoot = (user = getStoredUser()) =>
  getEffectiveRole(user) >= ROLE_ROOT_USER;

export const subscribeViewerMode = (callback) => {
  const handleViewerModeChange = (event) => {
    callback(event?.detail?.mode ?? getViewerMode());
  };
  const handleStorageChange = (event) => {
    if (
      event.key === VIEWER_MODE_STORAGE_KEY ||
      event.key === null ||
      event.key === 'user'
    ) {
      callback(getViewerMode());
    }
  };

  window.addEventListener(VIEWER_MODE_CHANGE_EVENT, handleViewerModeChange);
  window.addEventListener('storage', handleStorageChange);

  return () => {
    window.removeEventListener(
      VIEWER_MODE_CHANGE_EVENT,
      handleViewerModeChange,
    );
    window.removeEventListener('storage', handleStorageChange);
  };
};
