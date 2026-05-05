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

import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Card,
  Avatar,
  Tag,
  Divider,
  Empty,
  Button,
  Modal,
  Radio,
  Input,
} from '@douyinfe/semi-ui';
import { Server, Gauge, ExternalLink } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  renderQuota,
  renderNumber,
  setUserData,
  timestamp2string,
} from '../../helpers';
import { UserContext } from '../../context/User';

export default function ApiInfoPanel({
  apiInfoData,
  handleCopyUrl,
  handleSpeedTest,
  accountGroup,
  isAdminUser,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  t,
}) {
  const [userState, userDispatch] = useContext(UserContext);
  const [redeemCode, setRedeemCode] = useState('');
  const [redeemLoading, setRedeemLoading] = useState(false);
  const [redeemApplyMode, setRedeemApplyMode] = useState('stack');
  const [redeemConfirmOpen, setRedeemConfirmOpen] = useState(false);
  const [showShadow, setShowShadow] = useState(false);
  const scrollRef = useRef(null);

  const accountItems = useMemo(
    () => (Array.isArray(accountGroup?.items) ? accountGroup.items : []),
    [accountGroup],
  );
  // 普通用户：始终在此面板展示兑换码入口与账户信息
  // （无订阅时也需要能看到兑换码入口）
  const showQuotaInfo = !isAdminUser;

  const handleInlineRedeem = () => {
    const code = redeemCode.trim();
    if (!code) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setRedeemApplyMode('stack');
    setRedeemConfirmOpen(true);
  };

  const confirmInlineRedeem = async () => {
    const code = redeemCode.trim();
    if (!code) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    if (redeemApplyMode !== 'stack' && redeemApplyMode !== 'defer') {
      showError(t('请选择叠加或顺延'));
      return;
    }
    setRedeemLoading(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: code,
        apply_mode: redeemApplyMode,
      });
      const { success, message, data } = res.data;
      if (success) {
        const addedQuota = data?.added_quota ?? data ?? 0;
        const refreshedUser = data?.user;
        const redeem = data?.redeem;

        showSuccess(t('兑换成功！'));
        let content = t('成功兑换额度：') + renderQuota(addedQuota);
        if (redeem?.mode === 'tokens') {
          const tokens = Number(addedQuota) || 0;
          content =
            tokens === 0
              ? t('Tokens订阅已开通')
              : `${t('成功兑换tokens：')}${renderNumber(tokens)} tokens`;
        } else if (redeem?.mode === 'pay_token') {
          const tokens = Number(addedQuota) || 0;
          content = `${t('成功兑换tokens：')}${renderNumber(tokens)} tokens`;
        } else if (redeem?.mode === 'request') {
          content = t('次数订阅已开通');
        } else if (redeem?.mode === 'pay_request') {
          const count = Number(addedQuota) || 0;
          content = `${t('成功兑换次数：')}${renderNumber(count)} ${t('次')}`;
        }
        Modal.success({
          title: t('兑换成功！'),
          content,
          centered: true,
        });

        if (userState?.user) {
          if (refreshedUser && Object.keys(refreshedUser).length > 0) {
            userDispatch({ type: 'login', payload: refreshedUser });
            setUserData(refreshedUser);
          } else {
            try {
              const selfRes = await API.get('/api/user/self');
              const {
                success: selfSuccess,
                message: selfMessage,
                data: selfData,
              } = selfRes?.data || {};
              if (selfSuccess && selfData && Object.keys(selfData).length > 0) {
                userDispatch({ type: 'login', payload: selfData });
                setUserData(selfData);
              } else {
                showError(selfMessage || t('获取用户信息失败，请刷新页面'));
              }
            } catch (e) {
              showError(t('获取用户信息失败，请刷新页面'));
            }
          }
        }
        setRedeemCode('');
        setRedeemConfirmOpen(false);
      } else {
        showError(message);
      }
    } catch (err) {
      showError(t('请求失败'));
    } finally {
      setRedeemLoading(false);
    }
  };

  const updateShadow = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const threshold = 4;
    const canScroll = el.scrollHeight - el.clientHeight > threshold;
    const notAtBottom =
      el.scrollTop + el.clientHeight < el.scrollHeight - threshold;
    setShowShadow(canScroll && notAtBottom);
  }, []);

  useEffect(() => {
    updateShadow();
  }, [apiInfoData, accountItems, showQuotaInfo, redeemLoading, updateShadow]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.addEventListener('scroll', updateShadow);
    return () => el.removeEventListener('scroll', updateShadow);
  }, [updateShadow, showQuotaInfo]);

  const renderQuotaItem = (item, idx) => {
    const hasQuotaTable =
      item.quotaTable &&
      Array.isArray(item.quotaTable.headers) &&
      Array.isArray(item.quotaTable.values);
    const isSingleQuotaTable =
      hasQuotaTable &&
      item.quotaTable.headers.length === 1 &&
      item.quotaTable.values.length === 1;

    return (
      <div
        key={idx}
        className='relative overflow-hidden flex items-start justify-between gap-3 rounded-lg bg-[var(--app-card-muted)] p-2'
      >
        <div className='min-w-0'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
            <div className='text-sm font-medium text-neutral-800 dark:text-neutral-100'>
              {item.title}
            </div>
            {item.period ? (
              <div className='text-[11px] leading-tight text-neutral-500 dark:text-neutral-400'>
                ({item.period})
              </div>
            ) : null}
          </div>
          {!hasQuotaTable && item.subtitle && (
            <div className='text-xs text-neutral-500 dark:text-neutral-400 mt-0.5'>
              {item.subtitle}
            </div>
          )}
          {hasQuotaTable && (
            <div className='mt-2 min-h-9 text-xs text-neutral-600 dark:text-neutral-300'>
              {isSingleQuotaTable ? (
                <div className='flex min-h-9 items-center gap-1'>
                  <span>{item.quotaTable.headers[0]}</span>
                  <span className='text-neutral-500 dark:text-neutral-400'> : </span>
                  <span className='font-semibold text-neutral-800 dark:text-neutral-100'>
                    {item.quotaTable.values[0]}
                  </span>
                </div>
              ) : (
                <>
                  <div
                    className='grid gap-x-2 mb-0.5'
                    style={{
                      gridTemplateColumns: `repeat(${Math.max(
                        item.quotaTable.headers.length || 1,
                        item.quotaTable.values.length || 1,
                      )}, minmax(0, 1fr))`,
                    }}
                  >
                    {item.quotaTable.headers.map((header, hIdx) => (
                      <div key={hIdx} className='truncate'>
                        {header}
                      </div>
                    ))}
                  </div>
                  <div
                    className='grid gap-x-2 font-semibold'
                    style={{
                      gridTemplateColumns: `repeat(${Math.max(
                        item.quotaTable.headers.length || 1,
                        item.quotaTable.values.length || 1,
                      )}, minmax(0, 1fr))`,
                    }}
                  >
                    {item.quotaTable.values.map((val, vIdx) => (
                      <div key={vIdx} className='truncate'>
                        {val}
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}
        </div>
        <div className='text-right'>
          {!hasQuotaTable && (
            <div className='text-sm font-semibold text-neutral-800 dark:text-neutral-100'>
              {item.value}
            </div>
          )}
        </div>
        {item.isExpired ? (
          <div className='pointer-events-none absolute inset-0 bg-neutral-200/75 dark:bg-black/45 backdrop-blur-[1px]'>
            <div className='absolute right-3 bottom-2 origin-bottom-right rotate-12 text-lg font-bold text-[#5b1b1b] dark:text-[#7a2a2a]'>
              {item.maskText || t('已失效')}
            </div>
          </div>
        ) : null}
      </div>
    );
  };

  return (
    <>
      <Modal
        title={t('兑换额度')}
        visible={redeemConfirmOpen}
        onOk={confirmInlineRedeem}
        onCancel={() => setRedeemConfirmOpen(false)}
        maskClosable={false}
        centered
        okText={t('确认兑换')}
        cancelText={t('取消')}
        confirmLoading={redeemLoading}
        okButtonProps={{ disabled: redeemLoading }}
      >
        <div className='space-y-3 pb-2'>
          <div>
            <div className='mb-2 font-medium'>{t('生效方式')}</div>
            <Radio.Group
              type='button'
              value={redeemApplyMode}
              onChange={(val) => {
                const selected = val && val.target ? val.target.value : val;
                setRedeemApplyMode(selected);
              }}
            >
              <Radio value='stack'>{t('叠加（立即生效）')}</Radio>
              <Radio value='defer'>{t('顺延（到期后生效）')}</Radio>
            </Radio.Group>
            <div className='mt-2 text-xs text-gray-500'>
              {redeemApplyMode === 'defer'
                ? t(
                    '若当前仍有有效订阅额度（不含自由额度），新兑换的订阅额度包将从当前订阅到期后开始计算有效期',
                  )
                : t('订阅额度兑换后将立即生效')}
            </div>
          </div>
          <div className='text-xs text-gray-500'>
            {t('该选项仅对订阅类兑换码生效，自由额度兑换码不受影响')}
          </div>
        </div>
      </Modal>

      <Card
        {...CARD_PROPS}
        className='!rounded-xl !shadow-none flex min-h-0 flex-1 flex-col'
        headerStyle={{ padding: 0 }}
        header={
          <div className={`${FLEX_CENTER_GAP2} px-3 py-1`.trim()}>
            <Server size={16} />
            {showQuotaInfo ? t('账户订阅') : t('API信息')}
          </div>
        }
        bodyStyle={{
          padding: 0,
          display: 'flex',
          flexDirection: 'column',
          flex: 1,
          minHeight: 0,
        }}
      >
      {showQuotaInfo ? (
        <div className='flex min-h-0 flex-1 flex-col gap-2 px-2 pb-2 pt-0'>
          <div className='shrink-0'>
            <Input
              className='app-inline-action-input'
              placeholder={t('请输入兑换码')}
              value={redeemCode}
              onChange={setRedeemCode}
              onEnterPress={handleInlineRedeem}
              addonAfter={
                <Button
                  type='primary'
                  theme='solid'
                  loading={redeemLoading}
                  onClick={handleInlineRedeem}
                  className='!px-4'
                >
                  {t('兑换额度')}
                </Button>
              }
            />
          </div>

          <div className='relative flex min-h-0 flex-1 flex-col'>
            <div
              ref={scrollRef}
              onScroll={updateShadow}
              className='flex min-h-0 flex-1 flex-col overflow-y-auto card-content-scroll'
            >
              {accountItems.length > 0 ? (
                <div className='space-y-2'>
                  {accountItems.map((item, idx) => renderQuotaItem(item, idx))}
                </div>
              ) : (
                <div className='flex justify-center items-center min-h-[12rem] w-full'>
                  <Empty
                    title={t('暂无订阅')}
                    description={t('可通过兑换码获取订阅')}
                  />
                </div>
              )}
            </div>
            {showShadow && (
              <div className='card-content-fade-indicator opacity-100' />
            )}
          </div>
        </div>
      ) : apiInfoData.length > 0 ? (
        <div className='relative flex min-h-0 flex-1 flex-col'>
          <div
            ref={scrollRef}
            onScroll={updateShadow}
            className='flex min-h-0 flex-1 flex-col overflow-y-auto card-content-scroll p-1'
          >
            {apiInfoData.map((api) => (
              <React.Fragment key={api.id}>
                <div className='flex p-2 hover:bg-white rounded-lg transition-colors cursor-pointer'>
                  <div className='flex-shrink-0 mr-3'>
                    <Avatar size='extra-small' color={api.color}>
                      {api.route.substring(0, 2)}
                    </Avatar>
                  </div>
                  <div className='flex-1'>
                    <div className='flex flex-wrap items-center justify-between mb-1 w-full gap-2'>
                      <span className='text-sm font-medium text-gray-900 !font-bold break-all'>
                        {api.route}
                      </span>
                      <div className='flex items-center gap-1 mt-1 lg:mt-0'>
                        <Tag
                          prefixIcon={<Gauge size={12} />}
                          size='small'
                          color='white'
                          shape='circle'
                          onClick={() => handleSpeedTest(api.url)}
                          className='cursor-pointer hover:opacity-80 text-xs'
                        >
                          {t('测速')}
                        </Tag>
                        <Tag
                          prefixIcon={<ExternalLink size={12} />}
                          size='small'
                          color='white'
                          shape='circle'
                          onClick={() =>
                            window.open(api.url, '_blank', 'noopener,noreferrer')
                          }
                          className='cursor-pointer hover:opacity-80 text-xs'
                        >
                          {t('跳转')}
                        </Tag>
                      </div>
                    </div>
                    <div
                      className='!text-semi-color-primary break-all cursor-pointer hover:underline mb-1'
                      onClick={() => handleCopyUrl(api.url)}
                    >
                      {api.url}
                    </div>
                    <div className='text-gray-500'>{api.description}</div>
                  </div>
                </div>
                <Divider />
              </React.Fragment>
            ))}
          </div>
          {showShadow && (
            <div
              className='pointer-events-none absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-[rgba(0,0,0,0.12)] via-[rgba(0,0,0,0.06)] to-transparent dark:from-[rgba(0,0,0,0.55)] dark:via-[rgba(0,0,0,0.28)]'
            />
          )}
        </div>
      ) : (
        <div className='flex justify-center items-center min-h-[20rem] w-full'>
          <Empty
            image={<IllustrationConstruction style={ILLUSTRATION_SIZE} />}
            darkModeImage={
              <IllustrationConstructionDark style={ILLUSTRATION_SIZE} />
            }
            title={t('暂无API信息')}
            description={t('请联系管理员在系统设置中配置API信息')}
          />
        </div>
      )}
      </Card>
    </>
  );
}
