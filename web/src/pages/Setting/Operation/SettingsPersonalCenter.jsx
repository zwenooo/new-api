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

import React, { useEffect, useMemo, useState } from 'react';
import { Card, Switch, Typography, Space, Button, Divider } from '@douyinfe/semi-ui';
import { Mail, MessageCircle, Github, Shield, Send, Cpu } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const { Text, Title } = Typography;

const DEFAULT_BINDING_VISIBILITY = {
  email: true,
  wechat: true,
  github: true,
  oidc: true,
  telegram: true,
  linuxdo: true,
};

const bindingOptionMeta = (t) => [
  {
    key: 'email',
    icon: <Mail size={16} />,
    title: t('邮箱'),
    description: t('允许用户在个人设置中绑定邮箱地址'),
  },
  {
    key: 'wechat',
    icon: <MessageCircle size={16} />,
    title: t('微信'),
    description: t('允许用户绑定微信用于通知或登录'),
  },
  {
    key: 'github',
    icon: <Github size={16} />,
    title: 'GitHub',
    description: t('允许用户绑定 GitHub 账户'),
  },
  {
    key: 'oidc',
    icon: <Shield size={16} />,
    title: 'OIDC',
    description: t('允许用户绑定企业或自建的 OIDC 身份'),
  },
  {
    key: 'telegram',
    icon: <Send size={16} />,
    title: 'Telegram',
    description: t('允许用户绑定 Telegram 账户'),
  },
  {
    key: 'linuxdo',
    icon: <Cpu size={16} />,
    title: 'Linux DO',
    description: t('允许用户绑定 Linux DO 账户'),
  },
];

const SettingsPersonalCenter = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [bindingVisibility, setBindingVisibility] = useState(
    DEFAULT_BINDING_VISIBILITY,
  );
  const [walletVisible, setWalletVisible] = useState(true);
  const [invitationPageVisible, setInvitationPageVisible] = useState(true);
  const [otherSettingsVisible, setOtherSettingsVisible] = useState(true);

  const parsedBindingVisibility = useMemo(() => {
    const raw = options?.PersonalSettingAccountBindingVisibility;
    if (!raw) {
      return DEFAULT_BINDING_VISIBILITY;
    }
    try {
      const parsed = JSON.parse(raw);
      return {
        ...DEFAULT_BINDING_VISIBILITY,
        ...parsed,
      };
    } catch (error) {
      console.warn('Failed to parse binding visibility config', error);
      return DEFAULT_BINDING_VISIBILITY;
    }
  }, [options?.PersonalSettingAccountBindingVisibility]);

  useEffect(() => {
    setBindingVisibility(parsedBindingVisibility);
    const rawVisibility = options?.PersonalSettingWalletInvitationVisible;
    const computedWalletVisible = rawVisibility !== 'false';
    setWalletVisible(computedWalletVisible);

    const rawInvitationPageVisibility =
      options?.PersonalSettingInvitationPageVisible;
    if (
      rawInvitationPageVisibility === undefined ||
      rawInvitationPageVisibility === ''
    ) {
      setInvitationPageVisible(computedWalletVisible);
    } else {
      setInvitationPageVisible(rawInvitationPageVisibility !== 'false');
    }
    const rawOtherSettingsVisibility =
      options?.PersonalSettingOtherSettingsVisible;
    setOtherSettingsVisible(rawOtherSettingsVisibility !== 'false');
  }, [
    parsedBindingVisibility,
    options?.PersonalSettingWalletInvitationVisible,
    options?.PersonalSettingInvitationPageVisible,
    options?.PersonalSettingOtherSettingsVisible,
  ]);

  const handleBindingToggle = (key, value) => {
    setBindingVisibility((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await API.put('/api/option/', {
        key: 'PersonalSettingAccountBindingVisibility',
        value: JSON.stringify(bindingVisibility),
      });
      await API.put('/api/option/', {
        key: 'PersonalSettingWalletInvitationVisible',
        value: walletVisible,
      });
      await API.put('/api/option/', {
        key: 'PersonalSettingInvitationPageVisible',
        value: invitationPageVisible,
      });
      await API.put('/api/option/', {
        key: 'PersonalSettingOtherSettingsVisible',
        value: otherSettingsVisible,
      });
      showSuccess(t('设置保存成功'));
      refresh();
    } catch (error) {
      console.error(error);
      showError(t('设置保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const bindingOptions = useMemo(() => bindingOptionMeta(t), [t]);

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='mb-4'>
        <Title heading={5} className='mb-1'>
          {t('个人中心显示设置')}
        </Title>
        <Text type='tertiary' className='text-sm'>
          {t('管理员可以控制普通用户在个人中心可见的绑定方式与邀请奖励')}
        </Text>
      </div>

      <div className='mb-3'>
        <Text strong className='text-sm'>
          {t('账户绑定可见性')}
        </Text>
        <Text type='tertiary' className='text-xs block mt-1'>
          {t('关闭后，对应的账户绑定入口将从普通用户的个人设置中隐藏')}
        </Text>
      </div>

      <Space direction='vertical' style={{ width: '100%' }} size='small'>
        {bindingOptions.map((item) => (
          <div
            key={item.key}
            className='flex items-center justify-between px-3 py-2 rounded-xl border border-gray-200'
          >
            <div className='flex items-center gap-3'>
              <div className='flex items-center justify-center w-8 h-8 rounded-full bg-gray-100 text-gray-700'>
                {item.icon}
              </div>
              <div>
                <Text strong>{item.title}</Text>
                <Text type='tertiary' className='block text-xs mt-0.5'>
                  {item.description}
                </Text>
              </div>
            </div>
            <Switch
              checked={bindingVisibility[item.key] !== false}
              onChange={(value) => handleBindingToggle(item.key, value)}
            />
          </div>
        ))}
      </Space>

      <Divider margin='16px 0' />

      <div className='flex items-center justify-between px-3 py-2 rounded-xl border border-gray-200 mb-4'>
        <div>
          <Text strong>{t('我的邀请页面')}</Text>
          <Text type='tertiary' className='block text-xs mt-0.5'>
            {t('决定是否向普通用户展示订阅购买下方的我的邀请入口')}
          </Text>
        </div>
        <Switch
          checked={invitationPageVisible}
          onChange={setInvitationPageVisible}
        />
      </div>

      <div className='flex items-center justify-between px-3 py-2 rounded-xl border border-gray-200 mb-4'>
        <div>
          <Text strong>{t('邀请奖励卡片')}</Text>
          <Text type='tertiary' className='block text-xs mt-0.5'>
            {t('决定是否向普通用户展示个人中心钱包中的邀请奖励模块')}
          </Text>
        </div>
        <Switch checked={walletVisible} onChange={setWalletVisible} />
      </div>

      <div className='flex items-center justify-between px-3 py-2 rounded-xl border border-gray-200 mb-4'>
        <div>
          <Text strong>{t('个人中心其他设置卡片')}</Text>
          <Text type='tertiary' className='block text-xs mt-0.5'>
            {t('决定是否向普通用户展示个人中心中的其他设置卡片')}
          </Text>
        </div>
        <Switch
          checked={otherSettingsVisible}
          onChange={setOtherSettingsVisible}
        />
      </div>

      <Button
        type='primary'
        theme='solid'
        loading={saving}
        onClick={handleSave}
      >
        {t('保存设置')}
      </Button>
    </Card>
  );
};

export default SettingsPersonalCenter;
