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

const CustomInputRender = (props) => {
  const { detailProps } = props;
  const { clearContextNode, uploadNode, inputNode, sendNode, onClick } =
    detailProps;

  const styledInputNode = React.cloneElement(inputNode, {
    className: `w-full max-w-full ${inputNode.props.className || ''}`,
    style: {
      ...inputNode.props.style,
      width: '100%',
      maxWidth: '100%',
      boxSizing: 'border-box',
    },
  });

  // 清空按钮
  const styledClearNode = clearContextNode
    ? React.cloneElement(clearContextNode, {
        className: `!rounded-full !bg-gray-100 hover:!bg-red-500 hover:!text-white flex-shrink-0 transition-all ${clearContextNode.props.className || ''}`,
        style: {
          ...clearContextNode.props.style,
          width: '32px',
          height: '32px',
          minWidth: '32px',
          padding: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        },
      })
    : null;

  // 发送按钮
  const styledSendNode = React.cloneElement(sendNode, {
    className: `!rounded-full !bg-purple-500 hover:!bg-purple-600 flex-shrink-0 transition-all ${sendNode.props.className || ''}`,
    style: {
      ...sendNode.props.style,
      width: '32px',
      height: '32px',
      minWidth: '32px',
      padding: 0,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    },
  });

  return (
    <div className='playground-chat-inputBox box-border w-full max-w-full px-3 pb-3 pt-2 sm:px-9 sm:pb-4 sm:pt-3'>
      <div
        className='box-border flex w-full max-w-full min-w-0 items-center gap-2 overflow-hidden rounded-xl bg-gray-50 p-2 shadow-sm transition-shadow hover:shadow-md sm:gap-3 sm:rounded-2xl'
        style={{
          border: '1px solid var(--semi-color-border)',
        }}
        onClick={onClick}
      >
        {/* 清空对话按钮 - 左边 */}
        {styledClearNode}
        <div className='min-w-0 flex-1 overflow-hidden'>{styledInputNode}</div>
        {/* 发送按钮 - 右边 */}
        {styledSendNode}
      </div>
    </div>
  );
};

export default CustomInputRender;
