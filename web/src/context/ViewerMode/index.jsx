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
import {
  canUseUserView,
  getViewerMode,
  resetViewerMode,
  setViewerMode,
  subscribeViewerMode,
  toggleViewerMode,
  VIEWER_MODE_ADMIN,
  VIEWER_MODE_USER,
} from '../../helpers/viewerMode';

export const ViewerModeContext = React.createContext({
  mode: VIEWER_MODE_ADMIN,
  isUserView: false,
  isAdminView: true,
  setMode: () => VIEWER_MODE_ADMIN,
  toggleMode: () => VIEWER_MODE_ADMIN,
  resetMode: () => VIEWER_MODE_ADMIN,
});

export const ViewerModeProvider = ({ children }) => {
  const [mode, setModeState] = React.useState(() => getViewerMode());

  React.useEffect(() => subscribeViewerMode(setModeState), []);

  const setMode = React.useCallback((nextMode) => {
    if (nextMode === VIEWER_MODE_USER && !canUseUserView()) {
      return resetViewerMode();
    }
    return setViewerMode(nextMode);
  }, []);

  const handleToggleMode = React.useCallback(() => {
    if (!canUseUserView()) {
      return resetViewerMode();
    }
    return toggleViewerMode();
  }, []);

  const handleResetMode = React.useCallback(() => resetViewerMode(), []);

  const value = React.useMemo(
    () => ({
      mode,
      isUserView: mode === VIEWER_MODE_USER,
      isAdminView: mode === VIEWER_MODE_ADMIN,
      setMode,
      toggleMode: handleToggleMode,
      resetMode: handleResetMode,
    }),
    [handleResetMode, handleToggleMode, mode, setMode],
  );

  return (
    <ViewerModeContext.Provider value={value}>
      {children}
    </ViewerModeContext.Provider>
  );
};

export const useViewerMode = () => React.useContext(ViewerModeContext);
