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

import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Input,
  Row,
  Select,
  Spin,
  Tabs,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

import {
  API,
  compareObjects,
  selectFilter,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
} from '../../helpers';

const RESPONSES_DIRECT_PASS_UA_DEFAULT =
  'codex_vscode,codex_exec,Codex Desktop,codex_cli_rs';
const RESPONSES_DIRECT_PASS_UA_LEGACY_DEFAULT = 'codex_cli_rs/';

const DEFAULT_INPUTS = {
  'cx_compat.opencode.instructions': '',
  'cx_compat.opencode.instructions_meta': '',
  'cx_compat.opencode.pinned_instructions': '',
  'cx_compat.opencode.pinned_meta': '',
  'cx_compat.responses.codex_cli_rs_ua_contains':
    RESPONSES_DIRECT_PASS_UA_DEFAULT,
  'cx_compat.responses.override_instructions': false,
  'cx_compat.responses.body_patch_json': '',
  'codex.prompt.chat_completions.instructions': '',
};

const BOOLEAN_KEYS = [
  'cx_compat.responses.override_instructions',
];

const EDITABLE_KEYS = [
  'codex.prompt.chat_completions.instructions',
  'cx_compat.responses.codex_cli_rs_ua_contains',
  'cx_compat.responses.override_instructions',
  'cx_compat.responses.body_patch_json',
];

const CxCompatSetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({ ...DEFAULT_INPUTS });
  const [inputsRow, setInputsRow] = useState({ ...DEFAULT_INPUTS });
  const refForm = useRef();
  const [activeTab, setActiveTab] = useState('cx_instructions');

  const [gitHubLoading, setGitHubLoading] = useState(false);
  const [gitHubRepo, setGitHubRepo] = useState('');
  const [gitHubPath, setGitHubPath] = useState('');
  const [gitHubDefaultBranch, setGitHubDefaultBranch] = useState('');
  const [gitHubConfiguredRef, setGitHubConfiguredRef] = useState('');
  const [gitHubBranches, setGitHubBranches] = useState([]);
  const [gitHubRef, setGitHubRef] = useState('');
  const [gitHubCommits, setGitHubCommits] = useState([]);
  const [gitHubCommit, setGitHubCommit] = useState('');
  const [gitHubCommitManual, setGitHubCommitManual] = useState('');

  const parseMeta = (raw) => {
    if (!raw) return null;
    try {
      return JSON.parse(raw);
    } catch {
      return null;
    }
  };

  const loadOptions = async () => {
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return false;
      }

      const next = { ...DEFAULT_INPUTS };
      data.forEach((item) => {
        if (!Object.prototype.hasOwnProperty.call(next, item.key)) return;
        if (
          item.key.endsWith('Enabled') ||
          item.key.endsWith('enabled') ||
          BOOLEAN_KEYS.includes(item.key)
        ) {
          next[item.key] = toBoolean(item.value);
          return;
        }
        next[item.key] = item.value;
      });

      const responsesDirectPassUAMatch = String(
        next['cx_compat.responses.codex_cli_rs_ua_contains'] || '',
      )
        .trim()
        .toLowerCase();
      if (
        !responsesDirectPassUAMatch ||
        responsesDirectPassUAMatch ===
          RESPONSES_DIRECT_PASS_UA_LEGACY_DEFAULT.toLowerCase()
      ) {
        next['cx_compat.responses.codex_cli_rs_ua_contains'] =
          RESPONSES_DIRECT_PASS_UA_DEFAULT;
      }

      setInputs(next);
      setInputsRow(structuredClone(next));
      refForm.current?.setValues(next);
      return true;
    } catch (e) {
      showError(t('加载失败，请重试'));
      return false;
    }
  };

  useEffect(() => {
    loadOptions();
  }, []);

  const loadGitHubBranches = async () => {
    setGitHubLoading(true);
    try {
      const res = await API.get(
        '/api/cx_compat/opencode/instructions/github/branches',
        { skipErrorHandler: true },
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return false;
      }
      setGitHubRepo(data.repo || '');
      setGitHubPath(data.path || '');
      setGitHubDefaultBranch(data.default_branch || '');
      setGitHubConfiguredRef(data.configured_ref || '');
      setGitHubBranches(
        (data.branches || []).map((x) => ({ label: x, value: x })),
      );

      const initialRef =
        data.configured_ref || data.default_branch || (data.branches || [])[0];
      if (initialRef) setGitHubRef(initialRef);
      return true;
    } catch (e) {
      if (e?.name === 'AxiosError' && e?.response?.status === 404) {
        showError(
          t(
            '后端未包含 GitHub 分支接口（404）。如果你在 Vite 开发模式（:5173）下访问，也可能是 Vite 代理未指向正确的后端（检查 VITE_PROXY_TARGET，例如 http://localhost:3000），请重启前端 dev server 后再试。',
          ),
        );
      } else {
        showError(t('加载 GitHub 分支失败'));
      }
      return false;
    } finally {
      setGitHubLoading(false);
    }
  };

  const loadGitHubCommits = async (ref) => {
    if (!ref) return false;
    setGitHubLoading(true);
    try {
      const res = await API.get(
        '/api/cx_compat/opencode/instructions/github/commits',
        {
          params: { ref },
          skipErrorHandler: true,
        },
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return false;
      }
      setGitHubCommits(
        (data.commits || []).map((item) => {
          const shortSha = item.sha ? item.sha.slice(0, 8) : '';
          const msg = (item.message || '').split('\n')[0];
          const label = [shortSha, msg, item.date ? `(${item.date})` : '']
            .filter(Boolean)
            .join(' ');
          return {
            label,
            value: item.sha,
          };
        }),
      );
      return true;
    } catch (e) {
      if (e?.name === 'AxiosError' && e?.response?.status === 404) {
        showError(
          t(
            '后端未包含 GitHub commits 接口（404）。如果你在 Vite 开发模式（:5173）下访问，也可能是 Vite 代理未指向正确的后端（检查 VITE_PROXY_TARGET，例如 http://localhost:3000），请重启前端 dev server 后再试。',
          ),
        );
      } else {
        showError(t('加载 GitHub commits 失败'));
      }
      return false;
    } finally {
      setGitHubLoading(false);
    }
  };

  useEffect(() => {
    loadGitHubBranches();
  }, []);

  useEffect(() => {
    if (!gitHubRef) return;
    setGitHubCommit('');
    setGitHubCommitManual('');
    loadGitHubCommits(gitHubRef);
  }, [gitHubRef]);

  const onSubmit = async () => {
    const updateArray = compareObjects(inputs, inputsRow).filter((x) =>
      EDITABLE_KEYS.includes(x.key),
    );
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    setLoading(true);
    try {
      const requestQueue = updateArray.map((item) =>
        API.put('/api/option/', {
          key: item.key,
          value: String(inputs[item.key]),
        }),
      );
      const res = await Promise.all(requestQueue);
      if (requestQueue.length === 1) {
        if (res.includes(undefined)) return;
      } else if (requestQueue.length > 1) {
        if (res.includes(undefined)) return showError(t('部分保存失败，请重试'));
      }
      showSuccess(t('保存成功'));
      await loadOptions();
    } catch (e) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const onSyncOpenCodeInstructionsFromLocal = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/cx_compat/opencode/instructions/sync');
      const { success, message } = res.data;
      if (!success) return showError(message);
      showSuccess(message || t('同步成功'));
      await loadOptions();
    } catch (e) {
      showError(t('同步失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const onSyncOpenCodeInstructionsFromGitHub = async () => {
    setLoading(true);
    try {
      const payload = {};
      if (gitHubRef) payload.ref = gitHubRef;
      const commitToUse = (gitHubCommitManual || '').trim() || gitHubCommit;
      if (commitToUse) payload.commit = commitToUse;

      const res =
        Object.keys(payload).length > 0
          ? await API.post('/api/cx_compat/opencode/instructions/sync/github', {
              ...payload,
            })
          : await API.post('/api/cx_compat/opencode/instructions/sync/github');
      const { success, message } = res.data;
      if (!success) return showError(message);
      showSuccess(message || t('同步成功'));
      await loadOptions();
    } catch (e) {
      showError(t('同步失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const onPinOpenCodeInstructionsDefault = async () => {
    setLoading(true);
    try {
      const res = await API.post(
        '/api/cx_compat/opencode/instructions/pin_default',
      );
      const { success, message } = res.data;
      if (!success) return showError(message);
      showSuccess(message || t('操作成功'));
      await loadOptions();
    } catch (e) {
      showError(t('操作失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const onRestoreOpenCodeInstructionsDefault = async () => {
    setLoading(true);
    try {
      const res = await API.post(
        '/api/cx_compat/opencode/instructions/restore_default',
      );
      const { success, message } = res.data;
      if (!success) return showError(message);
      showSuccess(message || t('操作成功'));
      await loadOptions();
    } catch (e) {
      showError(t('操作失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const openCodeMeta = parseMeta(inputs['cx_compat.opencode.instructions_meta']);
  const openCodePinnedMeta = parseMeta(inputs['cx_compat.opencode.pinned_meta']);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Tabs type='button' activeKey={activeTab} onChange={setActiveTab}>
          <Tabs.TabPane tab={t('通用适配')} itemKey='cx_instructions'>
            <div className='mt-4'>
              <Row style={{ marginTop: 10 }}>
                <Col span={24}>
                  <Banner
                    type='info'
                    description={
                      <div>
                        <div>{t('对所有 /v1/responses 请求统一生效')}</div>
                        <div>{t('普通渠道 /v1/responses，以及渠道级 cx2cc 转 /responses 都会复用这套规则')}</div>
                        <div>{t('命中直通 UA 列表时直接放行；其余请求按「客户端适配 -> 通用适配」顺序处理')}</div>
                      </div>
                    }
                  />
                </Col>
              </Row>
              <Row>
                <Col span={24}>
                  <Form.TextArea
                    label={t('通用 instructions（/v1/responses）')}
                    field={'codex.prompt.chat_completions.instructions'}
                    autosize={{ minRows: 8, maxRows: 18 }}
                    trigger='blur'
                    extraText={t(
                      '用于 /v1/responses 通用适配：普通渠道 /responses、渠道级 cx2cc 转 /responses 都会使用；命中直通 UA 列表时不注入；留空则使用内置 Codex CLI prompt',
                    )}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'codex.prompt.chat_completions.instructions': value,
                      })
                    }
                  />
                </Col>
              </Row>
              <Row gutter={12}>
                <Col xs={24} sm={12} md={12} lg={12} xl={12}>
                  <Form.Input
                    label={t(
                      '直通 UA 匹配串（默认 codex_vscode, codex_exec, Codex Desktop, codex_cli_rs）',
                    )}
                    field={'cx_compat.responses.codex_cli_rs_ua_contains'}
                    trigger='blur'
                    extraText={t(
                      '命中则对应 /v1/responses 请求直接放行，跳过注入与翻译；普通渠道 /responses、渠道级 cx2cc 都适用；多个关键字用逗号/换行/分号分隔',
                    )}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'cx_compat.responses.codex_cli_rs_ua_contains': value,
                      })
                    }
                  />
                </Col>
              </Row>
              <Row gutter={12}>
                <Col xs={24} sm={12} md={12} lg={12} xl={12}>
                  <Form.Switch
                    label={t('覆盖 /responses instructions')}
                    field={'cx_compat.responses.override_instructions'}
                    extraText={t(
                      '仅影响通用适配：开启后强制覆盖为「通用 instructions」；关闭则仅在缺失/为空时注入',
                    )}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'cx_compat.responses.override_instructions': value,
                      })
                    }
                  />
                </Col>
              </Row>
              <Row>
                <Col span={24}>
                  <Form.TextArea
                    label={t('/responses 请求体篡改（JSON）')}
                    field={'cx_compat.responses.body_patch_json'}
                    autosize={{ minRows: 4, maxRows: 12 }}
                    trigger='blur'
                    extraText={t(
                      '对命中规范化规则的 /v1/responses 请求生效；普通渠道 /responses、渠道级 cx2cc 都适用；提供 JSON 对象，会以「字段存在则覆盖，不存在则新增」的方式合并到 REQ BODY。示例：{"stream":true}',
                    )}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'cx_compat.responses.body_patch_json': value,
                      })
                    }
                  />
                </Col>
              </Row>
            </div>
          </Tabs.TabPane>

          <Tabs.TabPane tab={t('客户端适配（OpenCode/OpenClaw）')} itemKey='opencode'>
            <div className='mt-4'>
              <Row style={{ marginTop: 10 }}>
                <Col span={24}>
                  <Banner
                    type='info'
                    description={
                      <div>
                        <div>{t('OpenCode 指令管理只负责维护 OpenCode/OpenClaw 相关提示词内容')}</div>
                        <div>{t('/v1/responses 的基础规范化在「通用适配」里统一处理')}</div>
                        <div>{t('下方同步/恢复默认/设为默认版本操作仅影响 OpenCode instructions 本身')}</div>
                      </div>
                    }
                  />
                </Col>
              </Row>

              <Spin spinning={gitHubLoading}>
                <Row style={{ marginTop: 10 }}>
                  <Col span={24}>
                    <Banner
                      type='info'
                      description={`${t('GitHub 源')}：${gitHubRepo || '-'}${
                        gitHubPath ? ` (${gitHubPath})` : ''
                      }；${t('默认分支')}：${gitHubDefaultBranch || '-'}；${t(
                        '配置默认 ref',
                      )}：${gitHubConfiguredRef || '-'}`}
                    />
                  </Col>
                </Row>

                <Row gutter={12} style={{ marginTop: 10 }}>
                  <Col xs={24} sm={12} md={10} lg={10} xl={10}>
                    <div className='semi-form-field'>
                      <div className='semi-form-field-label'>{t('GitHub 分支/ref')}</div>
                      <Select
                        placeholder={t('请选择分支或 ref')}
                        optionList={gitHubBranches}
                        value={gitHubRef}
                        onChange={(value) => setGitHubRef(value)}
                        filter={selectFilter}
                        autoClearSearchValue={false}
                        searchPosition='dropdown'
                        showClear
                        style={{ width: '100%' }}
                      />
                    </div>
                  </Col>
                  <Col xs={24} sm={12} md={14} lg={14} xl={14}>
                    <div className='semi-form-field'>
                      <div className='semi-form-field-label'>
                        {t('Commit（可选，锁定版本）')}
                      </div>
                      <Select
                        placeholder={t('留空则拉取分支最新；可筛选并选择最近 commits')}
                        optionList={gitHubCommits}
                        value={gitHubCommit}
                        onChange={(value) => setGitHubCommit(value)}
                        filter={selectFilter}
                        autoClearSearchValue={false}
                        searchPosition='dropdown'
                        showClear
                        style={{ width: '100%' }}
                      />
                      <Input
                        placeholder={t('也可手动输入 commit SHA（优先于上方选择）')}
                        value={gitHubCommitManual}
                        onChange={(value) => setGitHubCommitManual(value)}
                        showClear
                        style={{ marginTop: 8, width: '100%' }}
                      />
                    </div>
                  </Col>
                </Row>
              </Spin>

              <Row style={{ marginTop: 10 }}>
                <Col span={24}>
                  <Banner
                    type='warning'
                    description={
                      <div>
                        <div>
                          {t('当前版本')}：
                          {openCodeMeta
                            ? `${openCodeMeta.source || t('未知')}${
                                openCodeMeta.repo
                                  ? ` ${openCodeMeta.repo}@${openCodeMeta.ref || ''}`
                                  : ''
                              }${openCodeMeta.commit ? `#${openCodeMeta.commit}` : ''}${
                                openCodeMeta.synced_at ? `（${openCodeMeta.synced_at}）` : ''
                              }`
                            : t('未知')}
                        </div>
                        <div>
                          {t('恢复默认')}：{t('优先恢复到「已固定默认版本」，否则恢复到内置默认')}
                        </div>
                        <div>
                          {t('已固定默认版本')}：
                          {openCodePinnedMeta
                            ? `${openCodePinnedMeta.source || t('未知')}${
                                openCodePinnedMeta.pinned_at
                                  ? `（${openCodePinnedMeta.pinned_at}）`
                                  : ''
                              }`
                            : t('未设置')}
                        </div>
                      </div>
                    }
                  />
                </Col>
              </Row>

              <Row>
                <Col span={24}>
                  <Form.TextArea
                    label={t('OpenCode instructions（只读）')}
                    field={'cx_compat.opencode.instructions'}
                    autosize={{ minRows: 6, maxRows: 14 }}
                    disabled
                    extraText={t(
                      '点击下方按钮同步：可从 GitHub 拉取（支持选择分支/ref 与 commit），或从本机/挂载路径拉取；也可设为默认版本并随时恢复默认',
                    )}
                  />
                </Col>
              </Row>
            </div>
          </Tabs.TabPane>

          <Tabs.TabPane tab={t('Droid 兼容')} itemKey='droid'>
            <div className='mt-4 p-2'>
              <Banner
                type='warning'
                description={t('暂未实现 Droid 兼容，后续会在此处补齐配置项与注入逻辑')}
              />
            </div>
          </Tabs.TabPane>

          <Tabs.TabPane tab={t('更多兼容')} itemKey='more'>
            <div className='mt-4 p-2'>
              <Banner type='info' description={t('预留：后续扩展更多客户端兼容配置')} />
            </div>
          </Tabs.TabPane>
        </Tabs>

        <Row>
          {activeTab === 'cx_instructions' || activeTab === 'cx_trace' ? (
            <Button size='default' onClick={onSubmit}>
              {t('保存')}
            </Button>
          ) : activeTab === 'opencode' ? (
            <div className='flex gap-3 flex-wrap'>
              <Button size='default' onClick={onSubmit}>
                {t('保存')}
              </Button>
              <Button type='primary' onClick={onSyncOpenCodeInstructionsFromGitHub}>
                {t('从 GitHub 同步')}
              </Button>
              <Button type='tertiary' onClick={onSyncOpenCodeInstructionsFromLocal}>
                {t('从本机同步')}
              </Button>
              <Button type='secondary' onClick={onPinOpenCodeInstructionsDefault}>
                {t('设为默认版本')}
              </Button>
              <Button type='warning' onClick={onRestoreOpenCodeInstructionsDefault}>
                {t('恢复默认')}
              </Button>
            </div>
          ) : null}
        </Row>
      </Form>
    </Spin>
  );
};

export default CxCompatSetting;
