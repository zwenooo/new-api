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

import React, { useContext, useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Form,
  Input,
  Modal,
  Row,
  Spin,
  Tabs,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

import {
  API,
  showError,
  showSuccess,
  setStatusData,
  toBoolean,
  verifyJSON,
} from '../../helpers';
import { StatusContext } from '../../context/Status';
import SettingsClawBoxCustomerService from '../../pages/Setting/Operation/SettingsClawBoxCustomerService';

const { Text } = Typography;
const DEFAULT_PORTABLE_REPO = 'zwenooo/ClawBox';
const DEFAULT_PORTABLE_CHANNEL = 'stable';
const DEFAULT_MANAGED_ANTHROPIC_PROVIDER_ID = 'clawbox-anthropic';
const DEFAULT_MANAGED_ANTHROPIC_MODEL_MAPPINGS = [
  { actual: 'claude-haiku-4-5-20251001', display: 'gpt-5.4' },
  { actual: 'claude-opus-4-5-20251101', display: 'gpt-5.4' },
  { actual: 'claude-opus-4-6', display: 'gpt-5.4' },
  { actual: 'claude-sonnet-4-5-20250929', display: 'gpt-5.4' },
  { actual: 'claude-sonnet-4-6', display: 'gpt-5.4' },
  { actual: 'claude-sonnet-4-7', display: 'gpt-5.4' },
  { actual: 'claude-opus-4-7', display: 'gpt-5.4' },
];
const DEFAULT_MANAGED_CONFIG_TEMPLATE = JSON.stringify(
  {
    agents: {
      defaults: {
        model: {
          primary: `${DEFAULT_MANAGED_ANTHROPIC_PROVIDER_ID}/claude-sonnet-4-6`,
          fallbacks: [`${DEFAULT_MANAGED_ANTHROPIC_PROVIDER_ID}/claude-opus-4-6`],
        },
        models: Object.fromEntries(
          DEFAULT_MANAGED_ANTHROPIC_MODEL_MAPPINGS.map(({ actual }) => [
            `${DEFAULT_MANAGED_ANTHROPIC_PROVIDER_ID}/${actual}`,
            {},
          ]),
        ),
      },
    },
    models: {
      providers: {
        [DEFAULT_MANAGED_ANTHROPIC_PROVIDER_ID]: {
          baseUrl: '${baseurl}',
          apiKey: '${apikey}',
          api: 'anthropic-messages',
          models: DEFAULT_MANAGED_ANTHROPIC_MODEL_MAPPINGS.map(({ actual, display }) => ({
            id: actual,
            name: display,
          })),
        },
      },
    },
    discovery: {
      mdns: {
        mode: 'off',
      },
    },
    tools: {
      profile: 'full',
      allow: [
        'group:fs',
        'group:runtime',
        'group:web',
        'group:memory',
        'group:sessions',
        'image',
        'pdf',
      ],
    },
  },
  null,
  2,
);
const PAGE_GUTTER = { xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 };
const OVERVIEW_TILE_STYLE = {
  height: '100%',
  padding: 16,
  border: '1px solid var(--semi-color-border)',
  borderRadius: 12,
  background:
    'linear-gradient(180deg, var(--semi-color-fill-0) 0%, var(--semi-color-bg-0) 100%)',
  boxSizing: 'border-box',
};
const SURFACE_PANEL_STYLE = {
  height: '100%',
  padding: 16,
  border: '1px solid var(--semi-color-border)',
  borderRadius: 12,
  background: 'var(--semi-color-fill-0)',
  boxSizing: 'border-box',
};

const ClawBoxSetting = () => {
  const { t } = useTranslation();
  const [, statusDispatch] = useContext(StatusContext);
  const formApiRef = useRef(null);
  const [activeTab, setActiveTab] = useState('basic');

  const [inputs, setInputs] = useState({
    ClawBoxActivationEnabled: false,
    ClawBoxRegisterEnabled: false,
    ClawBoxProductModeEnabled: false,
    ClawBoxProductId: '',
    ClawBoxInitialShrimp: '0',
    ClawBoxManagedOpenClawConfig: '{}',
    ClawBoxPortableRepo: DEFAULT_PORTABLE_REPO,
    ClawBoxPortableChannel: DEFAULT_PORTABLE_CHANNEL,
    ClawBoxPortableUpdateEnabled: true,
    'general_setting.clawbox_customer_service_qrcode': '',
  });
  const [originInputs, setOriginInputs] = useState({});
  const [loading, setLoading] = useState(false);
  const [managedConfigLoading, setManagedConfigLoading] = useState(false);
  const [isLoaded, setIsLoaded] = useState(false);

  const [bundledUpdateLoading, setBundledUpdateLoading] = useState(false);
  const [bundledUpdate, setBundledUpdate] = useState({
    version: '',
    bundledUrl: '',
    bundledSha256: '',
    windowsUrl: '',
    windowsSha256: '',
    macosUrl: '',
    macosSha256: '',
    releaseNotes: '',
    minAppVersion: '',
    nodeVersion: '',
    nodeUrl: '',
    nodeSha256: '',
  });
  const [portableSyncLoading, setPortableSyncLoading] = useState(false);
  const [portableSaveLoading, setPortableSaveLoading] = useState(false);
  const [portableGitHubTokenLoading, setPortableGitHubTokenLoading] = useState(false);
  const [portableGitHubTokenClearLoading, setPortableGitHubTokenClearLoading] = useState(false);
  const [portableGitHubToken, setPortableGitHubToken] = useState('');
  const [portableGitHubAuth, setPortableGitHubAuth] = useState({
    configured: false,
    source: 'none',
    option_key: 'ClawBoxPortableGitHubToken',
  });
  const [portableActivateId, setPortableActivateId] = useState(null);
  const [portableDeleteId, setPortableDeleteId] = useState(null);
  const [portableManualLoading, setPortableManualLoading] = useState(false);
  const [portableSyncVersion, setPortableSyncVersion] = useState('');
  const [portableLatest, setPortableLatest] = useState({
    enabled: true,
    version: '',
    mode: '',
    platform: '',
    arch: '',
    channel: '',
    downloadUrl: '',
    downloadSha256: '',
    releaseNotes: '',
    minAppVersion: '',
    message: '',
  });
  const [portableReleases, setPortableReleases] = useState([]);
  const [portableManual, setPortableManual] = useState({
    version: '',
    tag: '',
    assetName: 'ClawBox-Portable-windows-x64.zip',
    downloadUrl: '',
    downloadSha256: '',
    releaseNotes: '',
    minAppVersion: '0.1.0',
  });

  const getApiErrorMessage = (error, fallbackMessage) => {
    const responseMessage = error?.response?.data?.message;
    if (typeof responseMessage === 'string' && responseMessage.trim()) {
      return `${fallbackMessage}: ${responseMessage.trim()}`;
    }
    if (typeof error?.message === 'string' && error.message.trim()) {
      return `${fallbackMessage}: ${error.message.trim()}`;
    }
    return fallbackMessage;
  };

  const loadClawBoxManifests = async (channelOverride) => {
    try {
      const portableChannel = stringifyOptionValue(
        channelOverride ?? inputs.ClawBoxPortableChannel,
        DEFAULT_PORTABLE_CHANNEL,
      ).toLowerCase();
      const [bundledRes, portableStatusRes, portableRes, releasesRes] = await Promise.all([
        API.get('/api/clawbox/update/bundled-latest.json', { skipErrorHandler: true }),
        API.get('/api/clawbox/update/portable/status', {
          skipErrorHandler: true,
          params: { channel: portableChannel },
        }),
        API.get('/api/clawbox/update/portable-latest.json', {
          skipErrorHandler: true,
          params: { channel: portableChannel },
        }),
        API.get('/api/clawbox/update/portable/releases', {
          skipErrorHandler: true,
          params: { channel: portableChannel },
        }),
      ]);
      const bundledManifest = bundledRes.data;
      setBundledUpdate({
        version: bundledManifest.version || '',
        bundledUrl: bundledManifest.bundledUrl || '',
        bundledSha256: bundledManifest.bundledSha256 || '',
        windowsUrl: bundledManifest.downloads?.windows?.url || '',
        windowsSha256: bundledManifest.downloads?.windows?.sha256 || '',
        macosUrl: bundledManifest.downloads?.macos?.url || '',
        macosSha256: bundledManifest.downloads?.macos?.sha256 || '',
        releaseNotes: bundledManifest.releaseNotes || '',
        minAppVersion: bundledManifest.minAppVersion || '',
        nodeVersion: bundledManifest.node?.version || '',
        nodeUrl: bundledManifest.node?.url || '',
        nodeSha256: bundledManifest.node?.sha256 || '',
      });

      const portableStatus = portableStatusRes?.data || {};
      const portableManifest = portableRes.data || {};
      setPortableLatest({
        enabled:
          typeof portableStatus.enabled === 'boolean'
            ? portableStatus.enabled
            : !!portableManifest.enabled,
        version: portableManifest.version || '',
        mode: portableManifest.mode || '',
        platform: portableManifest.platform || '',
        arch: portableManifest.arch || '',
        channel: portableManifest.channel || '',
        downloadUrl: portableManifest.downloadUrl || '',
        downloadSha256: portableManifest.downloadSha256 || '',
        releaseNotes: portableManifest.releaseNotes || '',
        minAppVersion: portableManifest.minAppVersion || '',
        message: portableStatus.message || portableManifest.message || '',
      });

      const releaseData = releasesRes?.data?.data || [];
      setPortableReleases(Array.isArray(releaseData) ? releaseData : []);
    } catch (error) {
      showError(getApiErrorMessage(error, t('更新清单刷新失败')));
    }
  };

  const loadPortableGitHubAuth = async () => {
    try {
      const res = await API.get('/api/clawbox/update/portable/github-token', {
        skipErrorHandler: true,
      });
      const payload = res?.data?.data || {};
      setPortableGitHubAuth({
        configured: !!payload.configured,
        source: payload.source || 'none',
        option_key: payload.option_key || 'ClawBoxPortableGitHubToken',
      });
    } catch (error) {
      showError(getApiErrorMessage(error, t('GitHub Token 状态加载失败')));
    }
  };

  const getOptions = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      const nextInputs = {
        ClawBoxActivationEnabled: false,
        ClawBoxRegisterEnabled: false,
        ClawBoxProductModeEnabled: false,
        ClawBoxProductId: '',
        ClawBoxInitialShrimp: '0',
        ClawBoxManagedOpenClawConfig: '{}',
        ClawBoxPortableRepo: DEFAULT_PORTABLE_REPO,
        ClawBoxPortableChannel: DEFAULT_PORTABLE_CHANNEL,
        ClawBoxPortableUpdateEnabled: true,
        'general_setting.clawbox_customer_service_qrcode': '',
      };

      data.forEach((item) => {
        switch (item.key) {
          case 'ClawBoxActivationEnabled':
          case 'ClawBoxRegisterEnabled':
          case 'ClawBoxProductModeEnabled':
          case 'ClawBoxPortableUpdateEnabled':
            nextInputs[item.key] = toBoolean(item.value);
            break;
          case 'ClawBoxProductId':
          case 'ClawBoxInitialShrimp':
          case 'ClawBoxManagedOpenClawConfig':
          case 'ClawBoxPortableRepo':
          case 'ClawBoxPortableChannel':
          case 'general_setting.clawbox_customer_service_qrcode':
            nextInputs[item.key] = item.value || '';
            break;
          default:
            break;
        }
      });

      if (!nextInputs.ClawBoxInitialShrimp) {
        nextInputs.ClawBoxInitialShrimp = '0';
      }
      nextInputs.ClawBoxManagedOpenClawConfig = prettifyJSON(
        nextInputs.ClawBoxManagedOpenClawConfig,
      );

      setInputs(nextInputs);
      setOriginInputs(nextInputs);
      if (formApiRef.current) {
        formApiRef.current.setValues(nextInputs);
      }
      await loadClawBoxManifests(nextInputs.ClawBoxPortableChannel);
      await loadPortableGitHubAuth();
      setIsLoaded(true);
    } catch (error) {
      showError(t('加载失败，请刷新页面'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    getOptions();
  }, []);

  useEffect(() => {
    if (activeTab === 'support') {
      formApiRef.current = null;
    }
  }, [activeTab]);

  const refreshStatus = async () => {
    try {
      const res = await API.get('/api/status');
      const { success, data } = res.data;
      if (success) {
        statusDispatch({ type: 'set', payload: data });
        setStatusData(data);
      }
    } catch (error) {
      showError(t('刷新状态失败，请刷新页面'));
    }
  };

  const handleFormChange = (values) => {
    setInputs((prev) => ({
      ...prev,
      ...values,
    }));
  };

  const stringifyOptionValue = (value, emptyValue = '') => {
    if (value === null || typeof value === 'undefined' || value === '') {
      return emptyValue;
    }
    return String(value).trim();
  };

  const prettifyJSON = (raw, fallback = '{}') => {
    const trimmed = String(raw || '').trim();
    if (!trimmed) {
      return fallback;
    }
    if (!verifyJSON(trimmed)) {
      return raw;
    }
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  };

  const normalizeManagedConfigTemplate = (raw) => {
    const trimmed = String(raw || '').trim();
    const nextRaw = trimmed || '{}';
    if (!verifyJSON(nextRaw)) {
      throw new Error(t('ClawBox 配置模板不是合法的 JSON'));
    }
    const parsed = JSON.parse(nextRaw);
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
      throw new Error(t('ClawBox 配置模板必须为 JSON 对象'));
    }
    return {
      compact: JSON.stringify(parsed),
      pretty: JSON.stringify(parsed, null, 2),
    };
  };

  const getPortableGitHubTokenSourceText = () => {
    switch (portableGitHubAuth.source) {
      case 'option':
        return t('后台设置');
      case 'env:ONEAPI_CLAWBOX_PORTABLE_GITHUB_TOKEN':
        return 'ONEAPI_CLAWBOX_PORTABLE_GITHUB_TOKEN';
      case 'env:ONEAPI_CX_COMPAT_GITHUB_TOKEN':
        return 'ONEAPI_CX_COMPAT_GITHUB_TOKEN';
      case 'env:ONEAPI_CX_COMPAT_OPENCODE_GITHUB_TOKEN':
        return 'ONEAPI_CX_COMPAT_OPENCODE_GITHUB_TOKEN';
      default:
        return t('未配置');
    }
  };

  const submitClawBoxSettings = async () => {
    const nextValues = {
      ClawBoxActivationEnabled: !!inputs.ClawBoxActivationEnabled,
      ClawBoxRegisterEnabled: !!inputs.ClawBoxRegisterEnabled,
      ClawBoxProductModeEnabled: !!inputs.ClawBoxProductModeEnabled,
      ClawBoxProductId: stringifyOptionValue(inputs.ClawBoxProductId),
      ClawBoxInitialShrimp: stringifyOptionValue(inputs.ClawBoxInitialShrimp, '0'),
    };
    const requests = [];
    const originProductModeEnabled = !!originInputs.ClawBoxProductModeEnabled;

    if (originProductModeEnabled && !nextValues.ClawBoxProductModeEnabled) {
      requests.push({
        key: 'ClawBoxProductModeEnabled',
        value: nextValues.ClawBoxProductModeEnabled,
      });
    }
    if (originInputs.ClawBoxProductId !== nextValues.ClawBoxProductId) {
      requests.push({
        key: 'ClawBoxProductId',
        value: nextValues.ClawBoxProductId,
      });
    }
    if (
      stringifyOptionValue(originInputs.ClawBoxInitialShrimp, '0') !==
      nextValues.ClawBoxInitialShrimp
    ) {
      requests.push({
        key: 'ClawBoxInitialShrimp',
        value: nextValues.ClawBoxInitialShrimp,
      });
    }
    if (
      originInputs.ClawBoxActivationEnabled !== nextValues.ClawBoxActivationEnabled
    ) {
      requests.push({
        key: 'ClawBoxActivationEnabled',
        value: nextValues.ClawBoxActivationEnabled,
      });
    }
    if (
      originInputs.ClawBoxRegisterEnabled !== nextValues.ClawBoxRegisterEnabled
    ) {
      requests.push({
        key: 'ClawBoxRegisterEnabled',
        value: nextValues.ClawBoxRegisterEnabled,
      });
    }
    if (!originProductModeEnabled && nextValues.ClawBoxProductModeEnabled) {
      requests.push({
        key: 'ClawBoxProductModeEnabled',
        value: nextValues.ClawBoxProductModeEnabled,
      });
    }

    if (requests.length === 0) {
      return;
    }

    setLoading(true);
    try {
      for (const request of requests) {
        const res = await API.put('/api/option/', {
          key: request.key,
          value:
            typeof request.value === 'boolean'
              ? request.value.toString()
              : request.value,
        });
        if (!res.data.success) {
          showError(res.data.message);
          return;
        }
      }

      const nextInputs = {
        ...inputs,
        ClawBoxActivationEnabled: nextValues.ClawBoxActivationEnabled,
        ClawBoxRegisterEnabled: nextValues.ClawBoxRegisterEnabled,
        ClawBoxProductModeEnabled: nextValues.ClawBoxProductModeEnabled,
        ClawBoxProductId: nextValues.ClawBoxProductId,
        ClawBoxInitialShrimp: nextValues.ClawBoxInitialShrimp,
      };
      setInputs(nextInputs);
      setOriginInputs((prev) => ({
        ...prev,
        ClawBoxActivationEnabled: nextValues.ClawBoxActivationEnabled,
        ClawBoxRegisterEnabled: nextValues.ClawBoxRegisterEnabled,
        ClawBoxProductModeEnabled: nextValues.ClawBoxProductModeEnabled,
        ClawBoxProductId: nextValues.ClawBoxProductId,
        ClawBoxInitialShrimp: nextValues.ClawBoxInitialShrimp,
      }));
      showSuccess(t('更新成功'));
      await refreshStatus();
    } catch (error) {
      showError(t('更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const submitManagedConfigTemplate = async () => {
    let normalized;
    try {
      normalized = normalizeManagedConfigTemplate(inputs.ClawBoxManagedOpenClawConfig);
    } catch (error) {
      showError(error.message || t('ClawBox 配置模板校验失败'));
      return;
    }

    const originPretty = prettifyJSON(originInputs.ClawBoxManagedOpenClawConfig, '{}');
    if (originPretty === normalized.pretty) {
      showSuccess(t('没有需要保存的内容'));
      return;
    }

    setManagedConfigLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ClawBoxManagedOpenClawConfig',
        value: normalized.compact,
      });
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      const nextInputs = {
        ...inputs,
        ClawBoxManagedOpenClawConfig: normalized.pretty,
      };
      setInputs(nextInputs);
      setOriginInputs((prev) => ({
        ...prev,
        ClawBoxManagedOpenClawConfig: normalized.pretty,
      }));
      if (formApiRef.current) {
        formApiRef.current.setValue('ClawBoxManagedOpenClawConfig', normalized.pretty);
      }
      showSuccess(t('ClawBox 配置模板已保存'));
    } catch (error) {
      showError(getApiErrorMessage(error, t('ClawBox 配置模板保存失败')));
    } finally {
      setManagedConfigLoading(false);
    }
  };

  const submitClawBoxBundledUpdate = async () => {
    if (!bundledUpdate.version.trim()) {
      showError(t('版本号不能为空'));
      return;
    }

    setBundledUpdateLoading(true);
    try {
      const payload = {
        version: bundledUpdate.version.trim(),
        bundledUrl: bundledUpdate.bundledUrl.trim(),
        bundledSha256: bundledUpdate.bundledSha256.trim(),
        releaseNotes: bundledUpdate.releaseNotes.trim(),
        minAppVersion: bundledUpdate.minAppVersion.trim(),
      };
      const downloads = {};
      if (
        bundledUpdate.windowsUrl.trim() ||
        bundledUpdate.windowsSha256.trim()
      ) {
        downloads.windows = {
          url: bundledUpdate.windowsUrl.trim(),
          sha256: bundledUpdate.windowsSha256.trim(),
        };
      }
      if (bundledUpdate.macosUrl.trim() || bundledUpdate.macosSha256.trim()) {
        downloads.macos = {
          url: bundledUpdate.macosUrl.trim(),
          sha256: bundledUpdate.macosSha256.trim(),
        };
      }
      if (Object.keys(downloads).length > 0) {
        payload.downloads = downloads;
      }
      if (
        bundledUpdate.nodeVersion.trim() &&
        bundledUpdate.nodeUrl.trim() &&
        bundledUpdate.nodeSha256.trim()
      ) {
        payload.node = {
          version: bundledUpdate.nodeVersion.trim(),
          url: bundledUpdate.nodeUrl.trim(),
          sha256: bundledUpdate.nodeSha256.trim(),
        };
      }

      const res = await API.put('/api/clawbox/update/bundled-latest.json', payload);
      if (res.data.success) {
        showSuccess(t('更新成功'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('更新失败'));
    } finally {
      setBundledUpdateLoading(false);
    }
  };

  const submitPortableSettings = async () => {
    const requests = [];
    const nextRepo = stringifyOptionValue(inputs.ClawBoxPortableRepo, DEFAULT_PORTABLE_REPO);
    const nextChannel = stringifyOptionValue(
      inputs.ClawBoxPortableChannel,
      DEFAULT_PORTABLE_CHANNEL,
    ).toLowerCase();

    if (
      stringifyOptionValue(originInputs.ClawBoxPortableRepo, DEFAULT_PORTABLE_REPO) !== nextRepo
    ) {
      requests.push({ key: 'ClawBoxPortableRepo', value: nextRepo });
    }
    if (
      stringifyOptionValue(
        originInputs.ClawBoxPortableChannel,
        DEFAULT_PORTABLE_CHANNEL,
      ).toLowerCase() !== nextChannel
    ) {
      requests.push({ key: 'ClawBoxPortableChannel', value: nextChannel });
    }
    if (
      !!originInputs.ClawBoxPortableUpdateEnabled !==
      !!inputs.ClawBoxPortableUpdateEnabled
    ) {
      requests.push({
        key: 'ClawBoxPortableUpdateEnabled',
        value: !!inputs.ClawBoxPortableUpdateEnabled,
      });
    }

    if (requests.length === 0) {
      showSuccess(t('没有需要保存的内容'));
      return;
    }

    setPortableSaveLoading(true);
    try {
      for (const request of requests) {
        const res = await API.put('/api/option/', request);
        if (!res.data.success) {
          showError(res.data.message);
          return;
        }
      }
      const nextInputs = {
        ...inputs,
        ClawBoxPortableRepo: nextRepo,
        ClawBoxPortableChannel: nextChannel,
        ClawBoxPortableUpdateEnabled: !!inputs.ClawBoxPortableUpdateEnabled,
      };
      setInputs(nextInputs);
      setOriginInputs((prev) => ({
        ...prev,
        ClawBoxPortableRepo: nextRepo,
        ClawBoxPortableChannel: nextChannel,
        ClawBoxPortableUpdateEnabled: !!inputs.ClawBoxPortableUpdateEnabled,
      }));
      showSuccess(t('Portable 同步配置已保存'));
      await loadClawBoxManifests(nextChannel);
    } catch (error) {
      showError(t('保存失败'));
    } finally {
      setPortableSaveLoading(false);
    }
  };

  const submitPortableGitHubToken = async () => {
    const nextToken = portableGitHubToken.trim();
    if (!nextToken) {
      showError(t('GitHub Token 不能为空，如需移除请点击清空'));
      return;
    }

    setPortableGitHubTokenLoading(true);
    try {
      const res = await API.put('/api/clawbox/update/portable/github-token', {
        token: nextToken,
      });
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      setPortableGitHubToken('');
      await loadPortableGitHubAuth();
      showSuccess(t('Portable GitHub Token 已保存'));
    } catch (error) {
      showError(t('保存失败'));
    } finally {
      setPortableGitHubTokenLoading(false);
    }
  };

  const clearPortableGitHubToken = async () => {
    setPortableGitHubTokenClearLoading(true);
    try {
      const res = await API.delete('/api/clawbox/update/portable/github-token');
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      setPortableGitHubToken('');
      await loadPortableGitHubAuth();
      showSuccess(t('后台保存的 Portable GitHub Token 已清空'));
    } catch (error) {
      showError(t('清空失败'));
    } finally {
      setPortableGitHubTokenClearLoading(false);
    }
  };

  const syncPortableReleaseFromGitHub = async () => {
    setPortableSyncLoading(true);
    try {
      const payload = {
        repo: stringifyOptionValue(inputs.ClawBoxPortableRepo, DEFAULT_PORTABLE_REPO),
        channel: stringifyOptionValue(
          inputs.ClawBoxPortableChannel,
          DEFAULT_PORTABLE_CHANNEL,
        ).toLowerCase(),
        version: stringifyOptionValue(portableSyncVersion),
        platform: 'windows',
        arch: 'x64',
        set_latest: true,
      };
      const res = await API.post('/api/clawbox/update/portable/releases/sync/github', payload);
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('已从 GitHub 同步 Portable 版本'));
      await loadClawBoxManifests(payload.channel);
    } catch (error) {
      showError(t('同步失败'));
    } finally {
      setPortableSyncLoading(false);
    }
  };

  const activatePortableRelease = async (id) => {
    setPortableActivateId(id);
    try {
      const res = await API.post(`/api/clawbox/update/portable/releases/${id}/activate`);
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('已切换最新版本'));
      await loadClawBoxManifests();
    } catch (error) {
      showError(t('切换失败'));
    } finally {
      setPortableActivateId(null);
    }
  };

  const deletePortableRelease = async (item) => {
    Modal.confirm({
      title: t('删除 Portable 版本'),
      content: t(
        item?.is_latest
          ? '当前记录是 latest。删除后，系统会在同通道下自动选择新的 latest；如果没有剩余记录，则 latest 会被清空。'
          : '删除后将无法再从后台恢复这条 Portable 版本记录。',
      ),
      okText: t('确认删除'),
      okButtonProps: { type: 'danger' },
      cancelText: t('取消'),
      onOk: async () => {
        setPortableDeleteId(item.id);
        try {
          const res = await API.delete(`/api/clawbox/update/portable/releases/${item.id}`);
          if (!res.data.success) {
            showError(res.data.message);
            return;
          }
          showSuccess(t('Portable 版本已删除'));
          await loadClawBoxManifests();
        } catch (error) {
          showError(t('删除失败'));
        } finally {
          setPortableDeleteId(null);
        }
      },
    });
  };

  const submitPortableManualRelease = async () => {
    if (!portableManual.version.trim()) {
      showError(t('手动版本号不能为空'));
      return;
    }
    if (!portableManual.downloadUrl.trim() || !portableManual.downloadSha256.trim()) {
      showError(t('手动发布至少需要下载地址和 SHA256'));
      return;
    }

    setPortableManualLoading(true);
    try {
      const res = await API.post('/api/clawbox/update/portable/releases', {
        version: portableManual.version.trim(),
        tag: portableManual.tag.trim(),
        mode: 'portable',
        platform: 'windows',
        arch: 'x64',
        channel: stringifyOptionValue(
          inputs.ClawBoxPortableChannel,
          DEFAULT_PORTABLE_CHANNEL,
        ).toLowerCase(),
        source: 'manual',
        repo: stringifyOptionValue(inputs.ClawBoxPortableRepo, DEFAULT_PORTABLE_REPO),
        asset_name: portableManual.assetName.trim(),
        download_url: portableManual.downloadUrl.trim(),
        download_sha256: portableManual.downloadSha256.trim(),
        release_notes: portableManual.releaseNotes.trim(),
        min_app_version: portableManual.minAppVersion.trim(),
        set_latest: true,
      });
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('已登记手动 Portable 版本'));
      setPortableManual((prev) => ({
        ...prev,
        version: '',
        tag: '',
        downloadUrl: '',
        downloadSha256: '',
        releaseNotes: '',
      }));
      await loadClawBoxManifests(
        stringifyOptionValue(inputs.ClawBoxPortableChannel, DEFAULT_PORTABLE_CHANNEL).toLowerCase(),
      );
    } catch (error) {
      showError(t('保存失败'));
    } finally {
      setPortableManualLoading(false);
    }
  };

  const managedTemplateSummary = (() => {
    const raw = String(inputs.ClawBoxManagedOpenClawConfig || '').trim();
    if (!raw || raw === '{}') {
      return {
        value: t('空模板'),
        detail: t('仅使用客户端默认初始化逻辑'),
      };
    }
    if (!verifyJSON(raw)) {
      return {
        value: t('JSON 待修复'),
        detail: t('当前编辑内容不是合法 JSON'),
      };
    }
    const parsed = JSON.parse(raw);
    const keys = Object.keys(parsed || {});
    if (!keys.length) {
      return {
        value: t('空模板'),
        detail: t('仅使用客户端默认初始化逻辑'),
      };
    }
    return {
      value: t('{{count}} 个顶层字段', { count: keys.length }),
      detail: keys.join(' / '),
    };
  })();

  const overviewItems = [
    {
      title: t('激活入口'),
      value: inputs.ClawBoxActivationEnabled ? t('已开启') : t('已关闭'),
      detail: t('控制 ClawBox 激活页是否对用户开放'),
    },
    {
      title: t('注册入口'),
      value: inputs.ClawBoxRegisterEnabled ? t('已开启') : t('已关闭'),
      detail: t('控制桌面端注册流程是否可见'),
    },
    {
      title: t('商品模式'),
      value: inputs.ClawBoxProductModeEnabled ? t('已开启') : t('已关闭'),
      detail:
        stringifyOptionValue(inputs.ClawBoxProductId) ||
        t('未指定商品 ID，按量商品仅剩 1 个时自动选中'),
    },
    {
      title: t('初始化模板'),
      value: managedTemplateSummary.value,
      detail: managedTemplateSummary.detail,
    },
    {
      title: t('Bundled 版本'),
      value: bundledUpdate.version || t('未配置'),
      detail: bundledUpdate.minAppVersion
        ? `${t('最低应用版本')}: ${bundledUpdate.minAppVersion}`
        : t('尚未设置最低版本'),
    },
    {
      title: t('Portable 发布'),
      value: portableLatest.version || t('未发布'),
      detail: `${stringifyOptionValue(
        inputs.ClawBoxPortableChannel,
        DEFAULT_PORTABLE_CHANNEL,
      ).toLowerCase()} / ${
        inputs.ClawBoxPortableUpdateEnabled ? t('更新开启') : t('更新关闭')
      }`,
    },
  ];

  if (!isLoaded) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '40vh',
        }}
      >
        <Spin size='large' />
      </div>
    );
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        marginTop: '10px',
      }}
    >
      <Card>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div>
            <Text strong style={{ display: 'block', fontSize: 18, marginBottom: 6 }}>
              {t('ClawBox 控制台')}
            </Text>
            <Text type='secondary' style={{ lineHeight: 1.7 }}>
              {t(
                '把接入开关、初始化模板、更新清单和客服素材拆到独立标签页，避免所有配置堆在一张长表单里。',
              )}
            </Text>
          </div>
          <Banner
            type='info'
            description={t(
              '先在上方概览确认当前状态，再按标签页分别维护接入配置、OpenClaw 初始化模板、Bundled 更新、Portable 发布和客服入口。',
            )}
          />
          <Row gutter={PAGE_GUTTER}>
            {overviewItems.map((item) => (
              <Col key={item.title} xs={24} sm={12} lg={8}>
                <div style={OVERVIEW_TILE_STYLE}>
                  <Text type='tertiary' style={{ display: 'block', marginBottom: 8 }}>
                    {item.title}
                  </Text>
                  <Text
                    strong
                    style={{ display: 'block', fontSize: 20, marginBottom: 8 }}
                  >
                    {item.value}
                  </Text>
                  <Text type='secondary' style={{ lineHeight: 1.6 }}>
                    {item.detail}
                  </Text>
                </div>
              </Col>
            ))}
          </Row>
        </div>
      </Card>

      <Card>
        <Tabs type='button' activeKey={activeTab} onChange={setActiveTab}>
          <Tabs.TabPane tab={t('接入配置')} itemKey='basic' />
          <Tabs.TabPane tab={t('初始化模板')} itemKey='template' />
          <Tabs.TabPane tab={t('Bundled 更新')} itemKey='bundled' />
          <Tabs.TabPane tab={t('Portable 发布')} itemKey='portable' />
          <Tabs.TabPane tab={t('客服入口')} itemKey='support' />
        </Tabs>
      </Card>

      {activeTab === 'support' ? (
        <Card>
          <SettingsClawBoxCustomerService options={inputs} refresh={getOptions} />
        </Card>
      ) : (
        <Form
          initValues={inputs}
          onValueChange={handleFormChange}
          getFormApi={(api) => (formApiRef.current = api)}
        >
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {activeTab === 'basic' ? (
              <Card>
                <Form.Section text={t('接入配置')}>
                  <Banner
                    type='info'
                    description={t(
                      '这里负责控制 ClawBox 的用户入口、商品绑定和初始虾粮发放逻辑。',
                    )}
                    style={{ marginBottom: 20 }}
                  />
                  <Row gutter={PAGE_GUTTER}>
                    <Col xs={24} md={10}>
                      <div style={SURFACE_PANEL_STYLE}>
                        <Text strong style={{ display: 'block', marginBottom: 10 }}>
                          {t('入口开关')}
                        </Text>
                        <Form.Checkbox field='ClawBoxActivationEnabled' noLabel>
                          {t('启用 ClawBox 激活入口')}
                        </Form.Checkbox>
                        <Form.Checkbox field='ClawBoxRegisterEnabled' noLabel>
                          {t('启用 ClawBox 注册入口')}
                        </Form.Checkbox>
                        <Form.Checkbox field='ClawBoxProductModeEnabled' noLabel>
                          {t('启用 ClawBox 商品模式')}
                        </Form.Checkbox>
                        <Text
                          type='secondary'
                          style={{ display: 'block', marginTop: 12, lineHeight: 1.7 }}
                        >
                          {t(
                            '激活和注册入口通常面向首次接入用户；商品模式开启后，客户端分组会持续跟商品配置保持一致。',
                          )}
                        </Text>
                      </div>
                    </Col>
                    <Col xs={24} md={14}>
                      <div style={SURFACE_PANEL_STYLE}>
                        <Text strong style={{ display: 'block', marginBottom: 10 }}>
                          {t('商品与额度')}
                        </Text>
                        <Form.Input
                          field='ClawBoxProductId'
                          label={t('ClawBox 商品 ID')}
                          placeholder={t('留空=仅当只有 1 个按量付费商品时自动选中')}
                          extraText={t(
                            '开启商品模式后，ClawBox 用户的唯一 API key 分组会始终与该商品的分组保持同步',
                          )}
                        />
                        <Form.InputNumber
                          field='ClawBoxInitialShrimp'
                          label={t('ClawBox 初始虾粮')}
                          min={0}
                          step={1}
                          precision={0}
                          placeholder={t('0=不赠送')}
                          extraText={t(
                            '这里填写的是美元额度；ClawBox 客户端展示时会按 token 口径换算虾粮，默认 1 美元 = 500000 token = 500000 虾粮。发放时会挂到 ClawBox 商品分组上',
                          )}
                        />
                      </div>
                    </Col>
                  </Row>
                  <Button
                    loading={loading}
                    onClick={submitClawBoxSettings}
                    style={{ marginTop: 16 }}
                  >
                    {t('保存 ClawBox 配置')}
                  </Button>
                </Form.Section>
              </Card>
            ) : null}

            {activeTab === 'template' ? (
              <Card>
                <Form.Section text={t('OpenClaw 初始化模板')}>
                  <Banner
                    type='info'
                    description={t(
                      '这里只维护服务端下发的模板片段。客户端仍会在本地补齐 relay token、gateway token、端口和控制台来源，再 merge 到 openclaw.json。',
                    )}
                    style={{ marginBottom: 20 }}
                  />
                  <Row gutter={PAGE_GUTTER}>
                    <Col xs={24} lg={16}>
                      <Form.TextArea
                        field='ClawBoxManagedOpenClawConfig'
                        label={t('ClawBox 配置模板（JSON）')}
                        rows={18}
                        placeholder={t('为一个 JSON 对象')}
                        extraText={t(
                          '适合在这里统一控制默认模型、thinking、工具白名单和 discovery 行为。',
                        )}
                      />
                      <div
                        style={{
                          display: 'flex',
                          gap: 12,
                          flexWrap: 'wrap',
                          marginTop: 8,
                        }}
                      >
                        <Button
                          onClick={() => {
                            const pretty = prettifyJSON(DEFAULT_MANAGED_CONFIG_TEMPLATE);
                            setInputs((prev) => ({
                              ...prev,
                              ClawBoxManagedOpenClawConfig: pretty,
                            }));
                            if (formApiRef.current) {
                              formApiRef.current.setValue(
                                'ClawBoxManagedOpenClawConfig',
                                pretty,
                              );
                            }
                          }}
                        >
                          {t('填入示例 JSON')}
                        </Button>
                        <Button
                          onClick={() => {
                            try {
                              const normalized = normalizeManagedConfigTemplate(
                                inputs.ClawBoxManagedOpenClawConfig,
                              );
                              setInputs((prev) => ({
                                ...prev,
                                ClawBoxManagedOpenClawConfig: normalized.pretty,
                              }));
                              if (formApiRef.current) {
                                formApiRef.current.setValue(
                                  'ClawBoxManagedOpenClawConfig',
                                  normalized.pretty,
                                );
                              }
                            } catch (error) {
                              showError(error.message || t('ClawBox 配置模板格式化失败'));
                            }
                          }}
                        >
                          {t('格式化 JSON')}
                        </Button>
                        <Button
                          loading={managedConfigLoading}
                          onClick={submitManagedConfigTemplate}
                        >
                          {t('保存初始化模板')}
                        </Button>
                      </div>
                    </Col>
                    <Col xs={24} lg={8}>
                      <div style={SURFACE_PANEL_STYLE}>
                        <Text strong style={{ display: 'block', marginBottom: 10 }}>
                          {t('使用边界')}
                        </Text>
                        <Text type='secondary' style={{ display: 'block', lineHeight: 1.8 }}>
                          {t(
                            '仅支持顶层字段：agents / discovery / models / plugins / tools。',
                          )}
                        </Text>
                        <Text
                          type='secondary'
                          style={{ display: 'block', marginTop: 12, lineHeight: 1.8 }}
                        >
                          {t(
                            '这里适合放服务端托管的默认模型策略，不要写依赖本机环境的 token、端口和本地路径。',
                          )}
                        </Text>
                        <Text
                          type='secondary'
                          style={{ display: 'block', marginTop: 12, lineHeight: 1.8 }}
                        >
                          {t(
                            '如果想统一调整客户端默认模型或工具可见性，优先改这份模板而不是手工修改每个用户本地文件。',
                          )}
                        </Text>
                      </div>
                    </Col>
                  </Row>
                </Form.Section>
              </Card>
            ) : null}

            {activeTab === 'bundled' ? (
              <>
                <Card>
                  <Form.Section text={t('Bundled 更新基础信息')}>
                    <Banner
                      type='info'
                      description={t(
                        'Bundled 更新清单用于描述运行时包版本；你可以在这里配置默认下载地址、最低版本和平台专属资源。',
                      )}
                      style={{ marginBottom: 20 }}
                    />
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('版本号')} *
                          </Text>
                          <Input
                            value={bundledUpdate.version}
                            onChange={(value) =>
                              setBundledUpdate((prev) => ({ ...prev, version: value }))
                            }
                            placeholder='2026.03.29.1'
                          />
                        </div>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('默认下载地址')}
                          </Text>
                          <Input
                            value={bundledUpdate.bundledUrl}
                            onChange={(value) =>
                              setBundledUpdate((prev) => ({ ...prev, bundledUrl: value }))
                            }
                            placeholder='https://...'
                          />
                        </div>
                        <div style={{ marginBottom: 0 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('默认 SHA256')}
                          </Text>
                          <Input
                            value={bundledUpdate.bundledSha256}
                            onChange={(value) =>
                              setBundledUpdate((prev) => ({
                                ...prev,
                                bundledSha256: value,
                              }))
                            }
                            placeholder='sha256 哈希值'
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('更新说明')}
                          </Text>
                          <Input
                            value={bundledUpdate.releaseNotes}
                            onChange={(value) =>
                              setBundledUpdate((prev) => ({
                                ...prev,
                                releaseNotes: value,
                              }))
                            }
                            placeholder='Bundled runtime update 2026.03.29.1'
                          />
                        </div>
                        <div style={{ marginBottom: 0 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('最低应用版本')}
                          </Text>
                          <Input
                            value={bundledUpdate.minAppVersion}
                            onChange={(value) =>
                              setBundledUpdate((prev) => ({
                                ...prev,
                                minAppVersion: value,
                              }))
                            }
                            placeholder='0.1.0'
                          />
                        </div>
                      </Col>
                    </Row>
                  </Form.Section>
                </Card>

                <Card>
                  <Form.Section text={t('平台下载与 Node 运行时')}>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={SURFACE_PANEL_STYLE}>
                          <Text strong style={{ display: 'block', marginBottom: 10 }}>
                            Windows
                          </Text>
                          <div style={{ marginBottom: 16 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>
                              {t('下载地址')}
                            </Text>
                            <Input
                              value={bundledUpdate.windowsUrl}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({ ...prev, windowsUrl: value }))
                              }
                              placeholder='https://.../clawbox-windows.zip'
                            />
                          </div>
                          <div style={{ marginBottom: 0 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>SHA256</Text>
                            <Input
                              value={bundledUpdate.windowsSha256}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({
                                  ...prev,
                                  windowsSha256: value,
                                }))
                              }
                              placeholder='sha256 哈希值'
                            />
                          </div>
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={SURFACE_PANEL_STYLE}>
                          <Text strong style={{ display: 'block', marginBottom: 10 }}>
                            macOS
                          </Text>
                          <div style={{ marginBottom: 16 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>
                              {t('下载地址')}
                            </Text>
                            <Input
                              value={bundledUpdate.macosUrl}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({ ...prev, macosUrl: value }))
                              }
                              placeholder='https://.../clawbox-macos.zip'
                            />
                          </div>
                          <div style={{ marginBottom: 0 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>SHA256</Text>
                            <Input
                              value={bundledUpdate.macosSha256}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({
                                  ...prev,
                                  macosSha256: value,
                                }))
                              }
                              placeholder='sha256 哈希值'
                            />
                          </div>
                        </div>
                      </Col>
                    </Row>
                    <div style={{ ...SURFACE_PANEL_STYLE, marginTop: 16 }}>
                      <Text strong style={{ display: 'block', marginBottom: 10 }}>
                        Node.js {t('（可选，留空则不更新 Node）')}
                      </Text>
                      <Row gutter={PAGE_GUTTER}>
                        <Col xs={24} md={8}>
                          <div style={{ marginBottom: 0 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>
                              {t('Node 版本')}
                            </Text>
                            <Input
                              value={bundledUpdate.nodeVersion}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({ ...prev, nodeVersion: value }))
                              }
                              placeholder='20.11.0'
                            />
                          </div>
                        </Col>
                        <Col xs={24} md={8}>
                          <div style={{ marginBottom: 0 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>
                              {t('Node 下载地址')}
                            </Text>
                            <Input
                              value={bundledUpdate.nodeUrl}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({ ...prev, nodeUrl: value }))
                              }
                              placeholder='https://...'
                            />
                          </div>
                        </Col>
                        <Col xs={24} md={8}>
                          <div style={{ marginBottom: 0 }}>
                            <Text style={{ display: 'block', marginBottom: 4 }}>
                              Node SHA256
                            </Text>
                            <Input
                              value={bundledUpdate.nodeSha256}
                              onChange={(value) =>
                                setBundledUpdate((prev) => ({ ...prev, nodeSha256: value }))
                              }
                              placeholder='sha256 哈希值'
                            />
                          </div>
                        </Col>
                      </Row>
                    </div>
                    <Button
                      loading={bundledUpdateLoading}
                      onClick={submitClawBoxBundledUpdate}
                      style={{ marginTop: 16 }}
                    >
                      {t('保存 Bundled 更新清单')}
                    </Button>
                  </Form.Section>
                </Card>
              </>
            ) : null}

            {activeTab === 'portable' ? (
              <>
                <Card>
                  <Form.Section text={t('Portable 同步配置')}>
                    <Banner
                      type='info'
                      description={t(
                        '客户端固定读取 portable-latest.json；这里负责维护同步源、默认通道、更新开关，以及从 GitHub 拉取正式发布记录。',
                      )}
                      style={{ marginBottom: 20 }}
                    />
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('GitHub 仓库')}
                          </Text>
                          <Input
                            value={inputs.ClawBoxPortableRepo}
                            onChange={(value) =>
                              setInputs((prev) => ({
                                ...prev,
                                ClawBoxPortableRepo: value,
                              }))
                            }
                            placeholder={DEFAULT_PORTABLE_REPO}
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('默认通道')}
                          </Text>
                          <Input
                            value={inputs.ClawBoxPortableChannel}
                            onChange={(value) =>
                              setInputs((prev) => ({
                                ...prev,
                                ClawBoxPortableChannel: value,
                              }))
                            }
                            placeholder={DEFAULT_PORTABLE_CHANNEL}
                          />
                        </div>
                      </Col>
                    </Row>
                    <div style={{ ...SURFACE_PANEL_STYLE, marginBottom: 16 }}>
                      <Form.Checkbox field='ClawBoxPortableUpdateEnabled' noLabel>
                        {t('启用 ClawBox Portable 客户端更新检查')}
                      </Form.Checkbox>
                      <Text type='secondary' style={{ display: 'block', marginTop: 6 }}>
                        {t(
                          '关闭后，客户端会先读到“更新已关闭”状态，并直接把当前版本视为最新版本。',
                        )}
                      </Text>
                    </div>
                    <div
                      style={{
                        display: 'flex',
                        gap: 12,
                        flexWrap: 'wrap',
                        marginBottom: 20,
                      }}
                    >
                      <Button loading={portableSaveLoading} onClick={submitPortableSettings}>
                        {t('保存 Portable 同步配置')}
                      </Button>
                    </div>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={16}>
                        <div style={{ marginBottom: 0 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('同步指定版本（留空=GitHub 最新 release）')}
                          </Text>
                          <Input
                            value={portableSyncVersion}
                            onChange={setPortableSyncVersion}
                            placeholder='2026.03.31.3'
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={8}>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'flex-end',
                            height: '100%',
                          }}
                        >
                          <Button
                            loading={portableSyncLoading}
                            onClick={syncPortableReleaseFromGitHub}
                          >
                            {t('从 GitHub 同步 Portable')}
                          </Button>
                        </div>
                      </Col>
                    </Row>
                  </Form.Section>
                </Card>

                <Card>
                  <Form.Section text={t('GitHub 访问凭证')}>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={16}>
                        <Text
                          type='secondary'
                          style={{ display: 'block', marginBottom: 16, lineHeight: 1.8 }}
                        >
                          {t(
                            '后台保存的 Portable GitHub Token 会优先于环境变量生效，用于同步 GitHub release 和代理私有资产下载。这里不会回显旧 token，重新输入并保存即可覆盖。',
                          )}
                        </Text>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('Portable GitHub Token')}
                          </Text>
                          <Input
                            type='password'
                            value={portableGitHubToken}
                            onChange={setPortableGitHubToken}
                            placeholder='github_pat_xxx'
                          />
                        </div>
                        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
                          <Button
                            loading={portableGitHubTokenLoading}
                            onClick={submitPortableGitHubToken}
                          >
                            {t('保存 Portable GitHub Token')}
                          </Button>
                          <Button
                            type='tertiary'
                            loading={portableGitHubTokenClearLoading}
                            onClick={clearPortableGitHubToken}
                          >
                            {t('清空后台 Token')}
                          </Button>
                        </div>
                      </Col>
                      <Col xs={24} md={8}>
                        <div style={SURFACE_PANEL_STYLE}>
                          <Text strong style={{ display: 'block', marginBottom: 10 }}>
                            {t('当前状态')}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {portableGitHubAuth.configured ? t('已配置') : t('未配置')}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('来源')}: {getPortableGitHubTokenSourceText()}
                          </Text>
                          <Text style={{ display: 'block' }}>
                            {t('后台字段')}:{' '}
                            {portableGitHubAuth.option_key || 'ClawBoxPortableGitHubToken'}
                          </Text>
                        </div>
                      </Col>
                    </Row>
                  </Form.Section>
                </Card>

                <Card>
                  <Form.Section text={t('当前 latest 与版本记录')}>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} lg={8}>
                        <div style={SURFACE_PANEL_STYLE}>
                          <Text strong style={{ display: 'block', marginBottom: 10 }}>
                            {t('当前 latest manifest')}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('更新开关')}: {portableLatest.enabled ? t('已开启') : t('已关闭')}
                          </Text>
                          {!portableLatest.enabled && portableLatest.message ? (
                            <Text type='danger' style={{ display: 'block', marginBottom: 6 }}>
                              {portableLatest.message}
                            </Text>
                          ) : null}
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('版本')}: {portableLatest.version || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('模式')}: {portableLatest.mode || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('平台')}: {portableLatest.platform || '-'} /{' '}
                            {portableLatest.arch || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('通道')}: {portableLatest.channel || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('下载地址')}: {portableLatest.downloadUrl || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            SHA256: {portableLatest.downloadSha256 || '-'}
                          </Text>
                          <Text style={{ display: 'block', marginBottom: 6 }}>
                            {t('最低应用版本')}: {portableLatest.minAppVersion || '-'}
                          </Text>
                          <Text style={{ display: 'block' }}>
                            {t('更新说明')}: {portableLatest.releaseNotes || '-'}
                          </Text>
                        </div>
                      </Col>
                      <Col xs={24} lg={16}>
                        <Text strong style={{ display: 'block', marginBottom: 8 }}>
                          {t('已同步版本')}
                        </Text>
                        <div
                          style={{
                            display: 'flex',
                            flexDirection: 'column',
                            gap: 12,
                          }}
                        >
                          {portableReleases.length === 0 ? (
                            <div style={SURFACE_PANEL_STYLE}>
                              <Text type='secondary'>{t('当前没有 Portable 版本记录')}</Text>
                            </div>
                          ) : (
                            portableReleases.map((item) => (
                              <div
                                key={item.id}
                                style={{
                                  border: '1px solid var(--semi-color-border)',
                                  borderRadius: 12,
                                  padding: 16,
                                }}
                              >
                                <div
                                  style={{
                                    display: 'flex',
                                    justifyContent: 'space-between',
                                    gap: 12,
                                    flexWrap: 'wrap',
                                    marginBottom: 8,
                                  }}
                                >
                                  <Text strong>
                                    {item.version}{' '}
                                    {item.is_latest ? `(${t('当前 latest')})` : ''}
                                  </Text>
                                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                                    <Button
                                      size='small'
                                      disabled={item.is_latest}
                                      loading={portableActivateId === item.id}
                                      onClick={() => activatePortableRelease(item.id)}
                                    >
                                      {item.is_latest ? t('当前生效中') : t('设为 latest')}
                                    </Button>
                                    <Button
                                      size='small'
                                      type='danger'
                                      loading={portableDeleteId === item.id}
                                      onClick={() => deletePortableRelease(item)}
                                    >
                                      {t('删除')}
                                    </Button>
                                  </div>
                                </div>
                                <Text style={{ display: 'block', marginBottom: 4 }}>
                                  Tag: {item.tag || '-'}
                                </Text>
                                <Text style={{ display: 'block', marginBottom: 4 }}>
                                  Source: {item.source || '-'} / {item.repo || '-'}
                                </Text>
                                <Text style={{ display: 'block', marginBottom: 4 }}>
                                  {t('代理下载地址')}: {item.proxy_download_url || '-'}
                                </Text>
                                <Text style={{ display: 'block', marginBottom: 4 }}>
                                  {t('上游下载地址')}: {item.download_url || '-'}
                                </Text>
                                <Text style={{ display: 'block', marginBottom: 4 }}>
                                  SHA256: {item.download_sha256 || '-'}
                                </Text>
                                <Text style={{ display: 'block' }}>
                                  {t('更新说明')}: {item.release_notes || '-'}
                                </Text>
                              </div>
                            ))
                          )}
                        </div>
                      </Col>
                    </Row>
                  </Form.Section>
                </Card>

                <Card>
                  <Form.Section text={t('手动登记备用版本')}>
                    <Text
                      type='secondary'
                      style={{ display: 'block', marginBottom: 16, lineHeight: 1.8 }}
                    >
                      {t(
                        '这个入口用于你已经手动上传过包文件，只需要把 URL 和 SHA256 登记进来；正常发布仍然推荐走 GitHub 自动同步。',
                      )}
                    </Text>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('版本号')}
                          </Text>
                          <Input
                            value={portableManual.version}
                            onChange={(value) =>
                              setPortableManual((prev) => ({ ...prev, version: value }))
                            }
                            placeholder='2026.03.31.3'
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>Tag</Text>
                          <Input
                            value={portableManual.tag}
                            onChange={(value) =>
                              setPortableManual((prev) => ({ ...prev, tag: value }))
                            }
                            placeholder='v2026.03.31.3'
                          />
                        </div>
                      </Col>
                    </Row>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('下载地址')}
                          </Text>
                          <Input
                            value={portableManual.downloadUrl}
                            onChange={(value) =>
                              setPortableManual((prev) => ({ ...prev, downloadUrl: value }))
                            }
                            placeholder='https://.../ClawBox-Portable-windows-x64.zip'
                          />
                        </div>
                      </Col>
                    </Row>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>SHA256</Text>
                          <Input
                            value={portableManual.downloadSha256}
                            onChange={(value) =>
                              setPortableManual((prev) => ({
                                ...prev,
                                downloadSha256: value,
                              }))
                            }
                            placeholder='sha256 哈希值'
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('资产文件名')}
                          </Text>
                          <Input
                            value={portableManual.assetName}
                            onChange={(value) =>
                              setPortableManual((prev) => ({ ...prev, assetName: value }))
                            }
                            placeholder='ClawBox-Portable-windows-x64.zip'
                          />
                        </div>
                      </Col>
                    </Row>
                    <Row gutter={PAGE_GUTTER}>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('更新说明')}
                          </Text>
                          <Input
                            value={portableManual.releaseNotes}
                            onChange={(value) =>
                              setPortableManual((prev) => ({
                                ...prev,
                                releaseNotes: value,
                              }))
                            }
                            placeholder='Portable package update 2026.03.31.3'
                          />
                        </div>
                      </Col>
                      <Col xs={24} md={12}>
                        <div style={{ marginBottom: 16 }}>
                          <Text style={{ display: 'block', marginBottom: 4 }}>
                            {t('最低应用版本')}
                          </Text>
                          <Input
                            value={portableManual.minAppVersion}
                            onChange={(value) =>
                              setPortableManual((prev) => ({
                                ...prev,
                                minAppVersion: value,
                              }))
                            }
                            placeholder='0.1.0'
                          />
                        </div>
                      </Col>
                    </Row>
                    <Button loading={portableManualLoading} onClick={submitPortableManualRelease}>
                      {t('登记手动 Portable 版本')}
                    </Button>
                  </Form.Section>
                </Card>
              </>
            ) : null}
          </div>
        </Form>
      )}
    </div>
  );
};

export default ClawBoxSetting;
