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

const ConsolePage = ({
  children,
  className = '',
  innerClassName = '',
  maxWidth,
  fillHeight = false,
}) => {
  const innerStyle = maxWidth
    ? { maxWidth, marginLeft: 'auto', marginRight: 'auto' }
    : undefined;

  const outerLayoutClass = fillHeight
    ? 'flex flex-1 min-h-0 flex-col'
    : 'flex shrink-0 flex-col';
  const innerLayoutClass = fillHeight
    ? 'flex flex-1 min-h-0 flex-col'
    : 'flex flex-col';

  return (
    <div
      className={`console-page ${outerLayoutClass} ${className}`.trim()}
    >
      <div
        className={`console-inner ${innerLayoutClass} ${innerClassName}`.trim()}
        style={innerStyle}
      >
        {children}
      </div>
    </div>
  );
};

export default ConsolePage;
