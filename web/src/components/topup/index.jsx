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

import React, { useEffect, useState, useContext, useRef } from 'react';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  isEffectiveAdmin,
  renderQuota,
  renderNumber,
  renderQuotaWithAmount,
  copy,
  renderCnyFen,
  yuanToFen,
  setUserData,
  timestamp2string,
} from '../../helpers';
import { Modal, Radio, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import RechargeCard from './RechargeCard';
import InvitationCard from './InvitationCard';
import TransferModal from './modals/TransferModal';
import PaymentConfirmModal from './modals/PaymentConfirmModal';
import ConsolePage from '../layout/ConsolePage';

const TopUp = () => {
  const { t } = useTranslation();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const isAdminUser = isEffectiveAdmin();

  const [redemptionCode, setRedemptionCode] = useState('');
  const [amount, setAmount] = useState(0.0);
  const [minTopUp, setMinTopUp] = useState(statusState?.status?.min_topup || 1);
  const [topUpCount, setTopUpCount] = useState(
    statusState?.status?.min_topup || 1,
  );
  const [topUpLink, setTopUpLink] = useState(
    statusState?.status?.top_up_link || '',
  );
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(
    statusState?.status?.enable_online_topup || false,
  );
  const [priceRatio, setPriceRatio] = useState(statusState?.status?.price || 1);

  const [enableStripeTopUp, setEnableStripeTopUp] = useState(
    statusState?.status?.enable_stripe_topup || false,
  );
  const [statusLoading, setStatusLoading] = useState(true);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [open, setOpen] = useState(false);
  const [payWay, setPayWay] = useState('');
  const [amountLoading, setAmountLoading] = useState(false);
  const [paymentLoading, setPaymentLoading] = useState(false);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [payMethods, setPayMethods] = useState([]);

  const personalSetting = statusState?.status?.personal_setting;
  const showInvitationCard =
    !isAdminUser || personalSetting?.wallet_invitation_visible !== false;
  const gridLayoutClass = showInvitationCard ? 'lg:grid-cols-12' : '';
  const rechargeColumnClass = showInvitationCard
    ? 'lg:col-span-7'
    : 'lg:col-span-12';

  const affFetchedRef = useRef(false);

  // 邀请相关状态
  const [affLink, setAffLink] = useState('');
  const [openTransfer, setOpenTransfer] = useState(false);
  const [transferAmountYuan, setTransferAmountYuan] = useState(0.01);

  // 预设充值额度选项
  const [presetAmounts, setPresetAmounts] = useState([]);
  const [selectedPreset, setSelectedPreset] = useState(null);

  // 充值配置信息
  const [topupInfo, setTopupInfo] = useState({
    amount_options: [],
    discount: {},
  });

  const [redeemApplyMode, setRedeemApplyMode] = useState('stack');
  const [redeemConfirmOpen, setRedeemConfirmOpen] = useState(false);

  const topUp = () => {
    if (!redemptionCode.trim()) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setRedeemApplyMode('stack');
    setRedeemConfirmOpen(true);
  };

  const confirmRedeem = async () => {
    const code = redemptionCode.trim();
    if (!code) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    if (redeemApplyMode !== 'stack' && redeemApplyMode !== 'defer') {
      showError(t('请选择叠加或顺延'));
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: code,
        apply_mode: redeemApplyMode,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

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
      if (userState.user) {
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
      setRedemptionCode('');
      setRedeemConfirmOpen(false);
    } catch (err) {
      showError(t('请求失败'));
    } finally {
      setIsSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) {
      showError(t('超级管理员未设置充值链接！'));
      return;
    }
    window.open(topUpLink, '_blank');
  };

  const preTopUp = async (payment) => {
    if (payment === 'stripe') {
      if (!enableStripeTopUp) {
        showError(t('管理员未开启Stripe充值！'));
        return;
      }
    } else {
      if (!enableOnlineTopUp) {
        showError(t('管理员未开启在线充值！'));
        return;
      }
    }

    setPayWay(payment);
    setPaymentLoading(true);
    try {
      if (payment === 'stripe') {
        await getStripeAmount();
      } else {
        await getAmount();
      }

      if (topUpCount < minTopUp) {
        showError(t('充值数量不能小于') + minTopUp);
        return;
      }
      setOpen(true);
    } catch (error) {
      showError(t('获取金额失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const onlineTopUp = async () => {
    if (payWay === 'stripe') {
      // Stripe 支付处理
      if (amount === 0) {
        await getStripeAmount();
      }
    } else {
      // 普通支付处理
      if (amount === 0) {
        await getAmount();
      }
    }

    if (topUpCount < minTopUp) {
      showError('充值数量不能小于' + minTopUp);
      return;
    }
    setConfirmLoading(true);
    try {
      let res;
      if (payWay === 'stripe') {
        // Stripe 支付请求
        res = await API.post('/api/user/stripe/pay', {
          amount: parseInt(topUpCount),
          payment_method: 'stripe',
        });
      } else {
        // 普通支付请求
        res = await API.post('/api/user/pay', {
          amount: parseInt(topUpCount),
          payment_method: payWay,
        });
      }

      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          if (payWay === 'stripe') {
            // Stripe 支付回调处理
            window.open(data.pay_link, '_blank');
          } else {
            // 普通支付表单提交
            let params = data;
            let url = res.data.url;
            let form = document.createElement('form');
            form.action = url;
            form.method = 'POST';
            let isSafari =
              navigator.userAgent.indexOf('Safari') > -1 &&
              navigator.userAgent.indexOf('Chrome') < 1;
            if (!isSafari) {
              form.target = '_blank';
            }
            for (let key in params) {
              let input = document.createElement('input');
              input.type = 'hidden';
              input.name = key;
              input.value = params[key];
              form.appendChild(input);
            }
            document.body.appendChild(form);
            form.submit();
            document.body.removeChild(form);
          }
        } else {
          showError(data);
        }
      } else {
        showError(res);
      }
    } catch (err) {
      console.log(err);
      showError(t('支付请求失败'));
    } finally {
      setOpen(false);
      setConfirmLoading(false);
    }
  };

  const getUserQuota = async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
    } else {
      showError(message);
    }
  };

  // 获取充值配置信息
  const getTopupInfo = async () => {
    try {
      const res = await API.get('/api/user/topup/info');
      const { message, data, success } = res.data;
      if (success) {
        setTopupInfo({
          amount_options: data.amount_options || [],
          discount: data.discount || {},
        });

        // 处理支付方式
        let payMethods = data.pay_methods || [];
        try {
          if (typeof payMethods === 'string') {
            payMethods = JSON.parse(payMethods);
          }
          if (payMethods && payMethods.length > 0) {
            // 检查name和type是否为空
            payMethods = payMethods.filter((method) => {
              return method.name && method.type;
            });
            // 如果没有color，则设置默认颜色
            payMethods = payMethods.map((method) => {
              // 规范化最小充值数
              const normalizedMinTopup = Number(method.min_topup);
              method.min_topup = Number.isFinite(normalizedMinTopup)
                ? normalizedMinTopup
                : 0;

              // Stripe 的最小充值从后端字段回填
              if (
                method.type === 'stripe' &&
                (!method.min_topup || method.min_topup <= 0)
              ) {
                const stripeMin = Number(data.stripe_min_topup);
                if (Number.isFinite(stripeMin)) {
                  method.min_topup = stripeMin;
                }
              }

              if (!method.color) {
                if (method.type === 'alipay') {
                  method.color = 'rgba(var(--semi-blue-5), 1)';
                } else if (method.type === 'wxpay') {
                  method.color = 'rgba(var(--semi-green-5), 1)';
                } else if (method.type === 'stripe') {
                  method.color = 'rgba(var(--semi-purple-5), 1)';
                } else {
                  method.color = 'rgba(var(--semi-primary-5), 1)';
                }
              }
              return method;
            });
          } else {
            payMethods = [];
          }

          // 如果启用了 Stripe 支付，添加到支付方法列表
          // 这个逻辑现在由后端处理，如果 Stripe 启用，后端会在 pay_methods 中包含它

          setPayMethods(payMethods);
          const enableStripeTopUp = data.enable_stripe_topup || false;
          const enableOnlineTopUp = data.enable_online_topup || false;
          const minTopUpValue = enableOnlineTopUp
            ? data.min_topup
            : enableStripeTopUp
              ? data.stripe_min_topup
              : 1;
          setEnableOnlineTopUp(enableOnlineTopUp);
          setEnableStripeTopUp(enableStripeTopUp);
          setMinTopUp(minTopUpValue);
          setTopUpCount(minTopUpValue);

          // 如果没有自定义充值数量选项，根据最小充值金额生成预设充值额度选项
          if (topupInfo.amount_options.length === 0) {
            setPresetAmounts(generatePresetAmounts(minTopUpValue));
          }

          // 初始化显示实付金额
          getAmount(minTopUpValue);
        } catch (e) {
          console.log('解析支付方式失败:', e);
          setPayMethods([]);
        }

        // 如果有自定义充值数量选项，使用它们替换默认的预设选项
        if (data.amount_options && data.amount_options.length > 0) {
          const customPresets = data.amount_options.map((amount) => ({
            value: amount,
            discount: data.discount[amount] || 1.0,
          }));
          setPresetAmounts(customPresets);
        }
      } else {
        console.error('获取充值配置失败:', data);
      }
    } catch (error) {
      console.error('获取充值配置异常:', error);
    }
  };

  // 获取邀请链接
  const getAffLink = async () => {
    const res = await API.get('/api/user/aff');
    const { success, message, data } = res.data;
    if (success) {
      let link = `${window.location.origin}/register?aff=${data}`;
      setAffLink(link);
    } else {
      showError(message);
    }
  };

  // 划转邀请额度
  const transfer = async () => {
    let amountFen = 0;
    try {
      amountFen = yuanToFen(transferAmountYuan);
    } catch (e) {
      showError(e?.message || t('金额格式错误'));
      return;
    }
    if (amountFen <= 0) {
      showError(t('转入金额必须大于0'));
      return;
    }
    const maxFen = userState?.user?.aff_quota || 0;
    if (amountFen > maxFen) {
      showError(t('转入金额不能大于返利余额'));
      return;
    }

    const res = await API.post(`/api/user/aff_transfer`, {
      amount_fen: amountFen,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess(message);
      setOpenTransfer(false);
      getUserQuota().then();
    } else {
      showError(message);
    }
  };

  // 复制邀请链接
  const handleAffLinkClick = async () => {
    await copy(affLink);
    showSuccess(t('邀请链接已复制到剪切板'));
  };

  useEffect(() => {
    if (!userState?.user?.id) {
      getUserQuota().then();
    }
    setTransferAmountYuan(0.01);
  }, []);

  useEffect(() => {
    if (affFetchedRef.current) return;
    affFetchedRef.current = true;
    getAffLink().then();
  }, []);

  // 在 statusState 可用时获取充值信息
  useEffect(() => {
    getTopupInfo().then();
  }, []);

  useEffect(() => {
    if (statusState?.status) {
      // const minTopUpValue = statusState.status.min_topup || 1;
      // setMinTopUp(minTopUpValue);
      // setTopUpCount(minTopUpValue);
      setTopUpLink(statusState.status.top_up_link || '');
      setPriceRatio(statusState.status.price || 1);

      setStatusLoading(false);
    }
  }, [statusState?.status]);

  const renderAmount = () => {
    return amount + ' ' + t('元');
  };

  const getAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/amount', {
        amount: parseFloat(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      console.log(err);
    }
    setAmountLoading(false);
  };

  const getStripeAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/stripe/amount', {
        amount: parseFloat(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      console.log(err);
    } finally {
      setAmountLoading(false);
    }
  };

  const handleCancel = () => {
    setOpen(false);
  };

  const handleTransferCancel = () => {
    setOpenTransfer(false);
  };

  // 选择预设充值额度
  const selectPresetAmount = (preset) => {
    setTopUpCount(preset.value);
    setSelectedPreset(preset.value);

    // 计算实际支付金额，考虑折扣
    const discount = preset.discount || topupInfo.discount[preset.value] || 1.0;
    const discountedAmount = preset.value * priceRatio * discount;
    setAmount(discountedAmount);
  };

  // 格式化大数字显示
  const formatLargeNumber = (num) => {
    return num.toString();
  };

  // 根据最小充值金额生成预设充值额度选项
  const generatePresetAmounts = (minAmount) => {
    const multipliers = [1, 5, 10, 30, 50, 100, 300, 500];
    return multipliers.map((multiplier) => ({
      value: minAmount * multiplier,
    }));
  };

  if (!isAdminUser) {
    return (
      <ConsolePage hideHeader>
        <div className='relative min-h-screen lg:min-h-0'>
          <TransferModal
            t={t}
            openTransfer={openTransfer}
            transfer={transfer}
            handleTransferCancel={handleTransferCancel}
            userState={userState}
            renderMoneyFen={renderCnyFen}
            transferAmountYuan={transferAmountYuan}
            setTransferAmountYuan={setTransferAmountYuan}
          />

          <div className='space-y-6'>
            <InvitationCard
              t={t}
              title={t('邀请有礼')}
              userState={userState}
              renderMoneyFen={renderCnyFen}
              setOpenTransfer={setOpenTransfer}
              affLink={affLink}
              handleAffLinkClick={handleAffLinkClick}
            />
          </div>
        </div>
      </ConsolePage>
    );
  }

  return (
    <ConsolePage hideHeader>
      <div className='relative min-h-screen lg:min-h-0'>
        <Modal
          title={t('兑换额度')}
          visible={redeemConfirmOpen}
          onOk={confirmRedeem}
          onCancel={() => setRedeemConfirmOpen(false)}
          maskClosable={false}
          centered
          okText={t('确认兑换')}
          cancelText={t('取消')}
          confirmLoading={isSubmitting}
          okButtonProps={{ disabled: isSubmitting }}
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

        {/* 划转模态框 */}
        <TransferModal
          t={t}
          openTransfer={openTransfer}
          transfer={transfer}
          handleTransferCancel={handleTransferCancel}
          userState={userState}
          renderMoneyFen={renderCnyFen}
          transferAmountYuan={transferAmountYuan}
          setTransferAmountYuan={setTransferAmountYuan}
        />

        {/* 充值确认模态框 */}
        <PaymentConfirmModal
          t={t}
          open={open}
          onlineTopUp={onlineTopUp}
          handleCancel={handleCancel}
          confirmLoading={confirmLoading}
          topUpCount={topUpCount}
          renderQuotaWithAmount={renderQuotaWithAmount}
          amountLoading={amountLoading}
          renderAmount={renderAmount}
          payWay={payWay}
          payMethods={payMethods}
          amountNumber={amount}
          discountRate={topupInfo?.discount?.[topUpCount] || 1.0}
        />

        {/* 用户信息头部 */}
        <div className='space-y-6'>
          <div className={`grid grid-cols-1 ${gridLayoutClass} gap-6`}>
            {/* 左侧充值区域 */}
            <div className={`${rechargeColumnClass} space-y-6 w-full`}>
              <RechargeCard
                t={t}
                enableOnlineTopUp={enableOnlineTopUp}
                enableStripeTopUp={enableStripeTopUp}
                presetAmounts={presetAmounts}
                selectedPreset={selectedPreset}
                selectPresetAmount={selectPresetAmount}
                formatLargeNumber={formatLargeNumber}
                priceRatio={priceRatio}
                topUpCount={topUpCount}
                minTopUp={minTopUp}
                renderQuotaWithAmount={renderQuotaWithAmount}
                getAmount={getAmount}
                setTopUpCount={setTopUpCount}
                setSelectedPreset={setSelectedPreset}
                renderAmount={renderAmount}
                amountLoading={amountLoading}
                payMethods={payMethods}
                preTopUp={preTopUp}
                paymentLoading={paymentLoading}
                payWay={payWay}
                redemptionCode={redemptionCode}
                setRedemptionCode={setRedemptionCode}
                topUp={topUp}
                isSubmitting={isSubmitting}
                topUpLink={topUpLink}
                openTopUpLink={openTopUpLink}
                userState={userState}
                renderQuota={renderQuota}
                statusLoading={statusLoading}
                topupInfo={topupInfo}
              />
            </div>

            {/* 右侧信息区域 */}
            {showInvitationCard && (
              <div className='lg:col-span-5'>
                <InvitationCard
                  t={t}
                  userState={userState}
                  renderMoneyFen={renderCnyFen}
                  setOpenTransfer={setOpenTransfer}
                  affLink={affLink}
                  handleAffLinkClick={handleAffLinkClick}
                />
              </div>
            )}
          </div>
        </div>
      </div>
    </ConsolePage>
  );
};

export default TopUp;
