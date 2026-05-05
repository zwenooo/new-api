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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getLucideIcon } from '../../helpers/render';
import { ChevronDown } from 'lucide-react';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useSidebar } from '../../hooks/common/useSidebar';
import {
  API,
  isEffectiveAdmin,
  isEffectiveRoot,
  showError,
  showSuccess,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useViewerMode } from '../../context/ViewerMode';

const routerMap = {
  home: '/',
  channel: '/console/channel',
  token: '/console/token',
  redemption: '/console/redemption',
  product_management: '/console/product_management',
  subscription: '/console/subscription',
  my_subscription: '/console/my_subscription',
  invitation: '/console/invitation',
  topup: '/console/topup',
  order: '/console/order',
  user: '/console/user',
  log: '/console/log',
  stomp_king: '/console/stomp_king',
  midjourney: '/console/midjourney',
  setting: '/console/setting',
  about: '/about',
  detail: '/console',
  pricing: '/console/pricing',
  service_status: '/console/status',
  task: '/console/task',
  models: '/console/models',
  playground: '/console/playground',
  personal: '/console/personal',
  faq: '/console/faq',
};

const SiderBar = ({ onNavigate = () => {} }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [collapsed] = useSidebarCollapsed();
  const { isModuleVisible, hasSectionVisibleModules } = useSidebar();
  const { mode: viewerMode } = useViewerMode();
  const isAdminUser = isEffectiveAdmin();
  const isRootUser = isEffectiveRoot();
  const personalSetting = statusState?.status?.personal_setting;
  const showInvitationMenu =
    isAdminUser || personalSetting?.invitation_page_visible !== false;

  const [selectedKeys, setSelectedKeys] = useState(['home']);
  const [chatItems, setChatItems] = useState([]);
  const [openedKeys, setOpenedKeys] = useState([]);
  const location = useLocation();
  const [routerMapState, setRouterMapState] = useState(routerMap);

  const consoleItems = useMemo(() => {
    const items = [
      {
        text: t('数据看板'),
        itemKey: 'detail',
        to: '/detail',
        className:
          localStorage.getItem('enable_data_export') === 'true'
            ? ''
            : 'tableHiddle',
      },
      {
        text: t('令牌管理'),
        itemKey: 'token',
        to: '/token',
      },
      {
        text: t('调用日志'),
        itemKey: 'log',
        to: '/log',
      },
      {
        text: t('绘图日志'),
        itemKey: 'midjourney',
        to: '/midjourney',
        className:
          localStorage.getItem('enable_drawing') === 'true'
            ? ''
            : 'tableHiddle',
      },
      {
        text: t('任务日志'),
        itemKey: 'task',
        to: '/task',
        className:
          localStorage.getItem('enable_task') === 'true' ? '' : 'tableHiddle',
      },
    ];

    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible('console', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [
    localStorage.getItem('enable_data_export'),
    localStorage.getItem('enable_drawing'),
    localStorage.getItem('enable_task'),
    t,
    isAdminUser,
    isModuleVisible,
    viewerMode,
  ]);

  const personalItems = useMemo(() => {
    const items = [
      {
        text: t('我的订阅'),
        itemKey: 'my_subscription',
        to: '/console/my_subscription',
      },
      {
        text: t('订阅商城'),
        itemKey: 'subscription',
        to: '/console/subscription',
      },
      ...(showInvitationMenu
        ? [
            {
              text: t('我的邀请'),
              itemKey: 'invitation',
              to: '/invitation',
            },
          ]
        : []),
    ];

    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible('personal', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [t, showInvitationMenu, isAdminUser, isModuleVisible, viewerMode]);

  const discoverItems = useMemo(() => {
    const items = [
      {
        text: t('谁是蹬王'),
        itemKey: 'stomp_king',
        sectionKey: 'console',
        to: '/console/stomp_king',
      },
      {
        text: t('模型广场'),
        itemKey: 'pricing',
        sectionKey: 'personal',
        to: '/console/pricing',
      },
      {
        text: t('服务状态'),
        itemKey: 'service_status',
        sectionKey: 'personal',
        to: '/service_status',
      },
      {
        text: t('常见问答'),
        itemKey: 'faq',
        sectionKey: 'personal',
        to: '/faq',
      },
    ];

    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible(item.sectionKey, item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [t, isAdminUser, isModuleVisible, viewerMode]);

  const adminItems = useMemo(() => {
    const items = [
      {
        text: t('渠道管理'),
        itemKey: 'channel',
        to: '/channel',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('模型管理'),
        itemKey: 'models',
        to: '/console/models',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('兑换码管理'),
        itemKey: 'redemption',
        to: '/redemption',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('用户管理'),
        itemKey: 'user',
        to: '/user',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('商品管理'),
        itemKey: 'product_management',
        to: '/console/product_management',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('订单管理'),
        itemKey: 'order',
        to: '/console/order',
        className: isAdminUser ? '' : 'tableHiddle',
      },
      {
        text: t('系统设置'),
        itemKey: 'setting',
        to: '/setting',
        className: isRootUser ? '' : 'tableHiddle',
      },
    ];

    // 根据配置过滤项目
    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible('admin', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [isAdminUser, isRootUser, t, isModuleVisible, viewerMode]);

  const chatMenuItems = useMemo(() => {
    const items = [
      {
        text: t('操练场'),
        itemKey: 'playground',
        to: '/playground',
      },
      {
        text: t('聊天'),
        itemKey: 'chat',
        items: chatItems,
      },
    ];

    // 根据配置过滤项目
    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible('chat', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [chatItems, t, isModuleVisible]);

  // 更新路由映射，添加聊天路由
  const updateRouterMapWithChats = (chats) => {
    const newRouterMap = { ...routerMap };

    if (Array.isArray(chats) && chats.length > 0) {
      for (let i = 0; i < chats.length; i++) {
        newRouterMap['chat' + i] = '/console/chat/' + i;
      }
    }

    setRouterMapState(newRouterMap);
    return newRouterMap;
  };

  // 加载聊天项
  useEffect(() => {
    let chats = localStorage.getItem('chats');
    if (chats) {
      try {
        chats = JSON.parse(chats);
        if (Array.isArray(chats)) {
          let chatItems = [];
          for (let i = 0; i < chats.length; i++) {
            let shouldSkip = false;
            let chat = {};
            for (let key in chats[i]) {
              let link = chats[i][key];
              if (typeof link !== 'string') continue; // 确保链接是字符串
              if (link.startsWith('fluent')) {
                shouldSkip = true;
                break; // 跳过 Fluent Read
              }
              chat.text = key;
              chat.itemKey = 'chat' + i;
              chat.to = '/console/chat/' + i;
            }
            if (shouldSkip || !chat.text) continue; // 避免推入空项
            chatItems.push(chat);
          }
          setChatItems(chatItems);
          updateRouterMapWithChats(chats);
        }
      } catch (e) {
        showError('聊天数据解析失败');
      }
    }
  }, []);

  // 根据当前路径设置选中的菜单项
  useEffect(() => {
    const currentPath = location.pathname;
    let matchingKey = Object.keys(routerMapState).find(
      (key) => routerMapState[key] === currentPath,
    );

    // 处理聊天路由
    if (!matchingKey && currentPath.startsWith('/console/chat/')) {
      const chatIndex = currentPath.split('/').pop();
      if (!isNaN(chatIndex)) {
        matchingKey = 'chat' + chatIndex;
      } else {
        matchingKey = 'chat';
      }
    }

    // 兼容旧的服务状态路径
    if (!matchingKey && currentPath === '/console/service_status') {
      matchingKey = 'service_status';
    }

    // 如果找到匹配的键，更新选中的键
    if (matchingKey) {
      setSelectedKeys([matchingKey]);
    }
  }, [location.pathname, routerMapState]);

  // 监控折叠状态变化以更新 body class
  useEffect(() => {
    if (collapsed) {
      document.body.classList.add('sidebar-collapsed');
    } else {
      document.body.classList.remove('sidebar-collapsed');
    }
  }, [collapsed]);

  const resolveTo = (itemKey) => routerMapState[itemKey] || routerMap[itemKey];

  const logout = async () => {
    await API.get('/api/user/logout');
    showSuccess(t('注销成功!'));
    userDispatch({ type: 'logout' });
    localStorage.removeItem('user');
    navigate('/login');
    onNavigate();
  };

  const renderLinkItem = (item) => {
    if (item.className === 'tableHiddle') return null;
    const to = resolveTo(item.itemKey);
    if (!to) return null;

    const isSelected = selectedKeys.includes(item.itemKey);

    return (
      <Link
        key={item.itemKey}
        to={to}
        onClick={() => {
          setSelectedKeys([item.itemKey]);
          onNavigate();
        }}
        className={`rail-item ${isSelected ? 'rail-item-active' : ''}`.trim()}
      >
        <span className='rail-icon'>
          {getLucideIcon(item.itemKey, isSelected)}
        </span>
        {!collapsed && <span className='rail-label'>{item.text}</span>}
      </Link>
    );
  };

  const renderChatGroup = () => {
    const isChatSelected = selectedKeys.some(
      (key) => key === 'chat' || key.startsWith('chat'),
    );
    const groupOpen = openedKeys.includes('chat');

    return (
      <div className='rail-group'>
        <button
          type='button'
          className={`rail-item rail-group-trigger ${isChatSelected ? 'rail-item-active' : ''}`.trim()}
          onClick={() => {
            if (collapsed) return;
            setOpenedKeys((prev) =>
              prev.includes('chat')
                ? prev.filter((k) => k !== 'chat')
                : [...prev, 'chat'],
            );
          }}
        >
          <span className='rail-icon'>
            {getLucideIcon('chat', isChatSelected)}
          </span>
          {!collapsed && (
            <>
              <span className='rail-label'>{t('聊天')}</span>
              <span
                className='rail-chevron'
                style={{
                  transform: groupOpen ? 'rotate(180deg)' : 'rotate(0deg)',
                }}
              >
                <ChevronDown size={16} strokeWidth={2} />
              </span>
            </>
          )}
        </button>

        {!collapsed && groupOpen && (
          <div className='rail-sublist'>
            {chatItems.map((subItem) => {
              const to = resolveTo(subItem.itemKey);
              if (!to) return null;
              const isSelected = selectedKeys.includes(subItem.itemKey);
              return (
                <Link
                  key={subItem.itemKey}
                  to={to}
                  onClick={() => {
                    setSelectedKeys([subItem.itemKey]);
                    onNavigate();
                  }}
                  className={`rail-subitem ${isSelected ? 'rail-subitem-active' : ''}`.trim()}
                >
                  <span className='rail-subdot' />
                  <span className='rail-sublabel'>{subItem.text}</span>
                </Link>
              );
            })}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className='sidebar-container'>
      <div className='sidebar-nav'>
        <div className='sidebar-nav-content'>
          {/* 主导航不再渲染小标题，避免把入口误导为固定分类 */}
          {hasSectionVisibleModules('chat') && (
            <div className='rail-section'>
              {!collapsed && (
                <div className='rail-section-label'>{t('对话')}</div>
              )}
              {chatMenuItems.map((item) => {
                if (item.itemKey === 'chat')
                  return (
                    <React.Fragment key='chat'>
                      {renderChatGroup()}
                    </React.Fragment>
                  );
                return renderLinkItem(item);
              })}
            </div>
          )}

          {hasSectionVisibleModules('console') && (
            <div className='rail-section'>
              {!collapsed && (
                <div className='rail-section-label'>{t('控制台')}</div>
              )}
              {consoleItems.map((item) => renderLinkItem(item))}
            </div>
          )}

          {hasSectionVisibleModules('personal') && personalItems.length > 0 && (
            <div className='rail-section'>
              {!collapsed && (
                <div className='rail-section-label'>{t('个人中心')}</div>
              )}
              {personalItems.map((item) => renderLinkItem(item))}
            </div>
          )}

          {discoverItems.length > 0 && (
            <div className='rail-section'>
              {!collapsed && (
                <div className='rail-section-label'>{t('发现')}</div>
              )}
              {discoverItems.map((item) => renderLinkItem(item))}
            </div>
          )}

          {isAdminUser && hasSectionVisibleModules('admin') && (
            <div className='rail-section'>
              {!collapsed && (
                <div className='rail-section-label'>{t('管理')}</div>
              )}
              {adminItems.map((item) => renderLinkItem(item))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default SiderBar;
