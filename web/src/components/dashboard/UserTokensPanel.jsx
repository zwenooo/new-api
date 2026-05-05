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
  useEffect,
  useMemo,
  useState,
  useRef,
  useCallback,
} from 'react';
import { Button, Card, Empty, Input, Modal, Skeleton } from '@douyinfe/semi-ui';
import { KeyRound } from 'lucide-react';
import { API, copy, showError, showSuccess } from '../../helpers';
import TokenGroupPrioritySelector, {
  normalizeTokenGroupIds,
} from '../token/TokenGroupPrioritySelector';

const maskKey = (key = '') => {
  if (!key) return '';
  if (key.length <= 10) return `sk-${key}`;
  return `sk-${key.slice(0, 4)}…${key.slice(-4)}`;
};

const UserTokensPanel = ({ CARD_PROPS, FLEX_CENTER_GAP2, t }) => {
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showShadow, setShowShadow] = useState(false);
  const scrollRef = useRef(null);

  const [groups, setGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupLabelsLoading, setGroupLabelsLoading] = useState(false);
  const [groupLabels, setGroupLabels] = useState({});

  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createAllowedGroupIds, setCreateAllowedGroupIds] = useState([]);
  const [creating, setCreating] = useState(false);

  const [editGroupOpen, setEditGroupOpen] = useState(false);
  const [editingToken, setEditingToken] = useState(null);
  const [editAllowedGroupIds, setEditAllowedGroupIds] = useState([]);
  const [savingGroup, setSavingGroup] = useState(false);

  const groupLabelById = useMemo(() => {
    const map = new Map();
    Object.entries(groupLabels || {}).forEach(([key, label]) => {
      const id = Number(key ?? 0);
      if (!Number.isFinite(id) || id <= 0) return;
      const normalized = String(label ?? '').trim();
      if (!normalized) return;
      map.set(Math.floor(id), normalized);
    });
    return map;
  }, [groupLabels]);

  const formatGroupIds = useCallback(
    (rawIds) => {
      const ids = normalizeTokenGroupIds(rawIds);
      if (ids.length === 0) return '';
      return ids
        .map((gid) => groupLabelById.get(gid) ?? t('未知分组'))
        .join(', ');
    },
    [groupLabelById, t],
  );

  const loadGroupLabels = async () => {
    setGroupLabelsLoading(true);
    try {
      const res = await API.get('/api/group/resolve');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        return;
      }
      const map = {};
      (Array.isArray(data) ? data : []).forEach((g) => {
        const idRaw = Number(g?.id ?? 0);
        const id = Number.isFinite(idRaw) ? Math.floor(idRaw) : 0;
        if (id <= 0) return;
        const label = String(g?.display_name || g?.code || '').trim();
        if (!label) return;
        map[id] = label;
      });
      setGroupLabels(map);
    } catch (e) {
      showError(e?.message || t('获取分组失败'));
    } finally {
      setGroupLabelsLoading(false);
    }
  };

  const getFallbackGroupLabel = useCallback(
    (groupId) =>
      `${groupLabelById.get(groupId) ?? t('未知分组')} (#${groupId})`,
    [groupLabelById, t],
  );

  const getApiErrorMessage = useCallback(
    (error, fallbackMessage) =>
      error?.response?.data?.message || error?.message || fallbackMessage,
    [],
  );

  const loadTokens = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/token/?p=1&size=100&with_usage=false');
      const { success, message, data } = res.data;
      if (success) {
        const items = Array.isArray(data?.items) ? data.items : [];
        setTokens(
          items.map((item) => ({
            ...item,
            allowed_group_ids: normalizeTokenGroupIds(item?.allowed_group_ids),
          })),
        );
      } else {
        showError(message);
      }
    } catch (error) {
      setTokens([]);
      showError(getApiErrorMessage(error, t('获取密钥失败')));
    } finally {
      setLoading(false);
    }
  };

  const loadGroups = async () => {
    setGroupsLoading(true);
    try {
      const res = await API.get('/api/user/self/groups');
      const { success, message, data } = res.data;
      if (success) {
        const list = Array.isArray(data) ? data : [];
        const optionList = list
          .map((item) => {
            const id = Number(item?.id ?? 0);
            if (!Number.isFinite(id) || id <= 0) return null;
            const label = String(item?.display_name || item?.code || '').trim();
            if (!label) return null;
            const desc = String(item?.desc || '').trim();
            const ratio = item?.ratio;
            const billable = Boolean(item?.billable);
            const no_billing = Boolean(item?.no_billing);
            return {
              value: Math.floor(id),
              label,
              ratio,
              desc,
              billable,
              no_billing,
            };
          })
          .filter(Boolean)
          .sort((a, b) => a.value - b.value);
        setGroups(optionList);
      } else {
        showError(t(message));
      }
    } catch (error) {
      setGroups([]);
      showError(getApiErrorMessage(error, t('获取分组失败')));
    } finally {
      setGroupsLoading(false);
    }
  };

  useEffect(() => {
    loadTokens();
    loadGroups();
    loadGroupLabels();
  }, []);

  useEffect(() => {
    if (!createOpen && !editGroupOpen) return;
    // Groups can change after user purchases (subscriptions/PAYG); refresh on open to avoid stale options.
    loadGroups();
    loadGroupLabels();
  }, [createOpen, editGroupOpen]);

  const updateShadow = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const threshold = 4;
    const canScroll = el.scrollHeight - el.clientHeight > threshold;
    const notAtBottom =
      el.scrollTop + el.clientHeight < el.scrollHeight - threshold;
    setShowShadow(canScroll && notAtBottom);
  }, []);

  const tokenRows = useMemo(
    () =>
      (tokens || []).map((token) => ({
        ...token,
        fullKey: `sk-${token.key}`,
        maskedKey: maskKey(token.key),
      })),
    [tokens],
  );

  useEffect(() => {
    updateShadow();
  }, [tokenRows, loading, updateShadow]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.addEventListener('scroll', updateShadow);
    return () => el.removeEventListener('scroll', updateShadow);
  }, [updateShadow]);

  const handleCopy = async (fullKey) => {
    if (await copy(fullKey)) {
      showSuccess(t('已复制到剪贴板！'));
    }
  };

  const handleToggleStatus = async (token) => {
    const nextStatus = token.status === 1 ? 2 : 1;
    try {
      const res = await API.put('/api/token/?status_only=true', {
        id: token.id,
        status: nextStatus,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        await loadTokens();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(getApiErrorMessage(error, t('操作失败')));
    }
  };

  const handleDelete = (token) => {
    Modal.confirm({
      title: t('删除密钥？'),
      content: t('此操作不可恢复'),
      centered: true,
      okButtonProps: {
        className: '!rounded-lg',
        type: 'danger',
        theme: 'solid',
      },
      cancelButtonProps: {
        className: '!rounded-lg',
        type: 'tertiary',
        theme: 'solid',
      },
      onOk: async () => {
        try {
          const res = await API.delete(`/api/token/${token.id}/`);
          const { success, message } = res.data;
          if (success) {
            showSuccess(t('删除成功！'));
            await loadTokens();
          } else {
            showError(message);
          }
        } catch (error) {
          showError(getApiErrorMessage(error, t('删除失败')));
        }
      },
    });
  };

  const handleCreate = async () => {
    const name = createName.trim();
    if (!name) {
      showError(t('请输入密钥名称'));
      return;
    }
    if (
      !Array.isArray(createAllowedGroupIds) ||
      createAllowedGroupIds.length === 0
    ) {
      showError(t('请选择可用分组'));
      return;
    }
    const allowedGroupIds = normalizeTokenGroupIds(createAllowedGroupIds);
    if (allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      return;
    }
    setCreating(true);
    const payload = {
      name,
      expired_time: -1,
      unlimited_quota: true,
      remain_quota: 500000,
      daily_quota_limit: 0,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      allowed_group_ids: allowedGroupIds,
    };
    try {
      const res = await API.post('/api/token/', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('密钥创建成功！'));
        setCreateOpen(false);
        setCreateName('');
        setCreateAllowedGroupIds([]);
        await loadTokens();
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(getApiErrorMessage(error, t('密钥创建失败')));
    } finally {
      setCreating(false);
    }
  };

  const handleOpenEditGroup = (token) => {
    setEditingToken(token);
    const allowedGroupIds = normalizeTokenGroupIds(token?.allowed_group_ids);
    setEditAllowedGroupIds(allowedGroupIds);
    setEditGroupOpen(true);
  };

  const handleSaveGroup = async () => {
    if (!editingToken) return;
    if (
      !Array.isArray(editAllowedGroupIds) ||
      editAllowedGroupIds.length === 0
    ) {
      showError(t('请选择可用分组'));
      return;
    }
    const allowedGroupIds = normalizeTokenGroupIds(editAllowedGroupIds);
    if (allowedGroupIds.length === 0) {
      showError(t('请选择可用分组'));
      return;
    }
    setSavingGroup(true);
    const payload = {
      id: editingToken.id,
      name: editingToken.name,
      expired_time: editingToken.expired_time,
      unlimited_quota: editingToken.unlimited_quota,
      daily_quota_limit: editingToken.daily_quota_limit ?? 0,
      model_limits_enabled: editingToken.model_limits_enabled,
      model_limits: editingToken.model_limits ?? '',
      allow_ips: editingToken.allow_ips ?? '',
      allowed_group_ids: allowedGroupIds,
    };
    if (Number.isFinite(Number(editingToken.remain_quota))) {
      payload.remain_quota = Number(editingToken.remain_quota);
    }
    try {
      const res = await API.put('/api/token/', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        setEditGroupOpen(false);
        setEditingToken(null);
        setEditAllowedGroupIds([]);
        await loadTokens();
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(getApiErrorMessage(error, t('保存失败')));
    } finally {
      setSavingGroup(false);
    }
  };

  return (
    <>
      <Card
        {...CARD_PROPS}
        className='!rounded-xl !shadow-none flex min-h-0 flex-1 flex-col overflow-hidden'
        headerStyle={{ padding: 0 }}
        header={
          <div className={`${FLEX_CENTER_GAP2} px-4 py-1.5`.trim()}>
            <KeyRound size={16} />
            {t('令牌管理')}
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
        <div className='flex min-h-0 flex-1 flex-col px-4 pb-4 pt-1'>
          <div className='flex items-center justify-between gap-3 pb-3'>
            <div className='text-sm text-neutral-500 dark:text-neutral-400'>
              {t('用于调用 API 的密钥')}
            </div>
            <Button
              type='primary'
              theme='solid'
              size='small'
              onClick={() => setCreateOpen(true)}
              className='!rounded-lg'
            >
              {t('新增密钥')}
            </Button>
          </div>

          <div className='relative flex min-h-0 flex-1 flex-col'>
            <div
              ref={scrollRef}
              onScroll={updateShadow}
              className='flex min-h-0 flex-1 flex-col overflow-y-auto card-content-scroll pr-1'
            >
              {loading ? (
                <div className='space-y-3'>
                  <Skeleton.Paragraph rows={2} style={{ width: '100%' }} />
                  <Skeleton.Paragraph rows={2} style={{ width: '100%' }} />
                </div>
              ) : tokenRows.length > 0 ? (
                <>
                  <div className='space-y-3'>
                    {tokenRows.map((token) => (
                      <div
                        key={token.id}
                        className={`dashboard-info-card dashboard-info-card--static flex items-start justify-between gap-4 rounded-2xl px-4 py-3.5 ${
                          token.status === 1 ? '' : 'opacity-70'
                        }`.trim()}
                      >
                        <div className='min-w-0 flex-1'>
                          <div className='truncate text-sm font-semibold tracking-[0.01em] text-neutral-800 dark:text-neutral-100'>
                            {token.name}
                          </div>
                          {Array.isArray(token.allowed_group_ids) &&
                          token.allowed_group_ids.length > 0 ? (
                            <div className='mt-1 truncate text-xs leading-5 text-neutral-500 dark:text-neutral-300'>
                              {t('可用分组')}:{' '}
                              {formatGroupIds(token.allowed_group_ids)}
                            </div>
                          ) : null}
                          <div className='mt-2 truncate text-xs font-mono tracking-[0.02em] text-neutral-500 dark:text-neutral-300'>
                            {token.maskedKey}
                          </div>
                        </div>
                        <div className='flex shrink-0 flex-wrap items-center justify-end gap-2 self-center'>
                          <Button
                            size='small'
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() => handleCopy(token.fullKey)}
                          >
                            {t('复制')}
                          </Button>
                          <Button
                            size='small'
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() => handleOpenEditGroup(token)}
                          >
                            {t('分组')}
                          </Button>
                          <Button
                            size='small'
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() => handleToggleStatus(token)}
                          >
                            {token.status === 1 ? t('禁用') : t('启用')}
                          </Button>
                          <Button
                            size='small'
                            type='primary'
                            theme='solid'
                            className='!rounded-lg'
                            onClick={() => handleDelete(token)}
                          >
                            {t('删除')}
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              ) : (
                <div className='py-4'>
                  <Empty
                    title={t('暂无密钥')}
                    description={t('点击右上角新增一个密钥')}
                  />
                </div>
              )}
            </div>
            {showShadow && (
              <div className='card-content-fade-indicator opacity-100' />
            )}
          </div>
        </div>
      </Card>

      <Modal
        title={t('新增密钥')}
        className='token-create-modal'
        visible={createOpen}
        centered
        onCancel={() => {
          setCreateOpen(false);
          setCreateName('');
          setCreateAllowedGroupIds([]);
        }}
        onOk={handleCreate}
        confirmLoading={creating}
        okText={t('创建')}
        width={920}
        okButtonProps={{
          type: 'primary',
          theme: 'solid',
          className: '!rounded-lg',
          disabled:
            creating ||
            groupsLoading ||
            groups.length === 0 ||
            !createName.trim() ||
            !Array.isArray(createAllowedGroupIds) ||
            createAllowedGroupIds.length === 0,
        }}
        cancelButtonProps={{
          type: 'tertiary',
          theme: 'solid',
          className: '!rounded-lg',
        }}
        bodyStyle={{
          height: 'min(640px, calc(100vh - 220px))',
          overflow: 'hidden',
          padding: 0,
        }}
      >
        <div className='token-create-modal__body flex h-full flex-col gap-4 overflow-hidden'>
          <div className='space-y-2'>
            <div className='token-create-modal__field-label'>
              {t('密钥名称')}
            </div>
            <Input
              value={createName}
              onChange={setCreateName}
              placeholder={t('例如：生产环境 / 测试环境')}
              showClear
              size='large'
              className='token-create-modal__input'
            />
          </div>

          <div className='token-create-modal__selector-shell min-h-0 flex-1 overflow-hidden'>
            <TokenGroupPrioritySelector
              t={t}
              title={t('可用分组')}
              description={t('左侧挑选可消费分组，右侧可拖拽调整消费优先级。')}
              options={groups}
              getFallbackLabel={getFallbackGroupLabel}
              value={createAllowedGroupIds}
              onChange={setCreateAllowedGroupIds}
              loading={groupsLoading || groupLabelsLoading}
              availableEmptyText={
                groups.length === 0 ? t('管理员未设置用户可选分组') : undefined
              }
              selectedEmptyText={t('从左侧添加分组')}
              className='min-h-0'
              preferSideBySide
              heightVariant='fill'
            />
          </div>
        </div>
      </Modal>

      <Modal
        title={t('配置分组')}
        visible={editGroupOpen}
        centered
        onCancel={() => {
          setEditGroupOpen(false);
          setEditingToken(null);
          setEditAllowedGroupIds([]);
        }}
        onOk={handleSaveGroup}
        confirmLoading={savingGroup}
        okText={t('保存')}
        width={920}
        okButtonProps={{
          type: 'primary',
          theme: 'solid',
          className: '!rounded-lg',
          disabled:
            savingGroup ||
            groupsLoading ||
            !Array.isArray(editAllowedGroupIds) ||
            editAllowedGroupIds.length === 0,
        }}
        cancelButtonProps={{
          type: 'tertiary',
          theme: 'solid',
          className: '!rounded-lg',
        }}
        bodyStyle={{
          height: 'min(640px, calc(100vh - 220px))',
          overflow: 'hidden',
        }}
      >
        <div className='flex h-full flex-col gap-4 overflow-hidden'>
          <TokenGroupPrioritySelector
            t={t}
            options={groups}
            getFallbackLabel={getFallbackGroupLabel}
            value={editAllowedGroupIds}
            onChange={setEditAllowedGroupIds}
            loading={groupsLoading || groupLabelsLoading}
            availableEmptyText={
              groups.length === 0 ? t('管理员未设置用户可选分组') : undefined
            }
            selectedEmptyText={t('从左侧添加分组')}
            className='min-h-0'
            preferSideBySide
            heightVariant='fill'
          />
        </div>
      </Modal>
    </>
  );
};

export default UserTokensPanel;
