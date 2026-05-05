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

import React, { useEffect, useState, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  verifyJSON,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  CHANNEL_OPTIONS,
  MODEL_FETCHABLE_CHANNEL_TYPES,
} from '../../../../constants';
import {
  SideSheet,
  Space,
  Spin,
  Button,
  Typography,
  Checkbox,
  Banner,
  Modal,
  ImagePreview,
  Card,
  Tag,
  Avatar,
  Form,
  Row,
  Col,
  Highlight,
  Input,
  InputNumber,
} from '@douyinfe/semi-ui';
import {
  getChannelModels,
  copy,
  getChannelIcon,
  getModelCategories,
  selectFilter,
} from '../../../../helpers';
import ModelSelectModal from './ModelSelectModal';
import JSONEditor from '../../../common/ui/JSONEditor';
import TwoFactorAuthModal from '../../../common/modals/TwoFactorAuthModal';
import ChannelKeyDisplay from '../../../common/ui/ChannelKeyDisplay';
import {
  IconSave,
  IconClose,
  IconServer,
  IconSetting,
  IconCode,
  IconGlobe,
  IconBolt,
  IconPlus,
  IconDelete,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const MODEL_MAPPING_EXAMPLE = {
  'gpt-3.5-turbo': 'gpt-3.5-turbo-0125',
};

const STATUS_CODE_MAPPING_EXAMPLE = {
  400: '500',
};

const REGION_EXAMPLE = {
  default: 'global',
  'gemini-1.5-pro-002': 'europe-west2',
  'gemini-1.5-flash-002': 'europe-west2',
  'claude-3-5-sonnet-20240620': 'europe-west1',
};

function type2secretPrompt(type) {
  // inputs.type === 15 ? '按照如下格式输入：APIKey|SecretKey' : (inputs.type === 18 ? '按照如下格式输入：APPID|APISecret|APIKey' : '请输入渠道对应的鉴权密钥')
  switch (type) {
    case 15:
      return '按照如下格式输入：APIKey|SecretKey';
    case 18:
      return '按照如下格式输入：APPID|APISecret|APIKey';
    case 22:
      return '按照如下格式输入：APIKey-AppId，例如：fastgpt-0sp2gtvfdgyi4k30jwlgwf1i-64f335d84283f05518e9e041';
    case 23:
      return '按照如下格式输入：AppId|SecretId|SecretKey';
    case 33:
      return '按照如下格式输入：Ak|Sk|Region';
    case 50:
      return '按照如下格式输入: AccessKey|SecretKey, 如果上游是Transfer API，则直接输ApiKey';
    case 51:
      return '按照如下格式输入: Access Key ID|Secret Access Key';
    default:
      return '请输入渠道对应的鉴权密钥';
  }
}

const EditChannelModal = (props) => {
  const { t } = useTranslation();
  const channelId = props.editingChannel.id;
  const isEdit = channelId !== undefined;
  const headerOverrideVariables = useMemo(
    () => [
      { key: 'user_id', label: t('用户ID') },
      { key: 'username', label: t('用户名') },
      { key: 'user_email', label: t('用户邮箱') },
      { key: 'user_group', label: t('用户分组') },
      { key: 'token_id', label: t('令牌ID') },
      { key: 'token_group', label: t('令牌分组') },
      { key: 'channel_id', label: t('渠道ID') },
      { key: 'channel_name', label: t('渠道名称') },
      { key: 'channel_type', label: t('渠道类型') },
      { key: 'model', label: t('上游模型') },
      { key: 'origin_model', label: t('原始模型') },
      { key: 'request_id', label: t('请求ID') },
      { key: 'client_ip', label: t('客户端IP') },
    ],
    [t],
  );
  const [loading, setLoading] = useState(isEdit);
  const [proxyTesting, setProxyTesting] = useState(false);
  const isMobile = useIsMobile();
  const handleCancel = () => {
    props.handleClose();
  };
  const originInputs = {
    name: '',
    type: 1,
    key: '',
    openai_organization: '',
    max_input_tokens: 0,
    base_url: '',
    other: '',
    model_mapping: '',
    status_code_mapping: '',
    models: [],
    auto_ban: 1,
    test_model: '',
    group_ids: [],
    backup_group_ids: [],
    priority: 0,
    weight: 0,
    max_concurrency: -1,
    billing_mode: 'quota',
    buy_cny_per_usd: 0,
    buy_requests_per_cny: 0,
    sell_requests_per_cny: 0,
    tag: '',
    multi_key_mode: 'random',
    // 渠道额外设置的默认值
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    messages_to_responses_compat: false,
    system_prompt: '',
    system_prompt_override: false,
    test_enable_max_tokens: true,
    settings: '',
    // 仅 Vertex: 密钥格式（存入 settings.vertex_key_type）
    vertex_key_type: 'json',
    upstream_model_update_check_enabled: false,
    upstream_model_update_auto_sync_enabled: false,
    upstream_model_update_last_check_time: 0,
    upstream_model_update_last_detected_models: [],
    upstream_model_update_ignored_models: '',
  };

  const handleTestProxy = async () => {
    try {
      if (!channelSettings?.proxy || channelSettings.proxy.trim() === '') {
        showError(t('请先填写代理地址'));
        return;
      }
      setProxyTesting(true);
      const res = await API.post('/api/channel/test_proxy', {
        proxy: channelSettings.proxy.trim(),
      });
      const { success, message, data } = res.data || {};
      if (success) {
        const latency = data?.latency_ms !== undefined ? `${data.latency_ms}ms` : '';
        showSuccess(
          `${t('代理可用')} ${latency ? `(${latency})` : ''}`.trim(),
        );
      } else {
        showError(message || t('代理不可用'));
      }
    } catch (e) {
      showError(e?.message || t('测试失败'));
    } finally {
      setProxyTesting(false);
    }
  };
  const [batch, setBatch] = useState(false);
  const [multiToSingle, setMultiToSingle] = useState(false);
  const [multiKeyMode, setMultiKeyMode] = useState('random');
  const [autoBan, setAutoBan] = useState(true);
  const [inputs, setInputs] = useState(originInputs);
  const [originModelOptions, setOriginModelOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [groupOptions, setGroupOptions] = useState([]);
  const [basicModels, setBasicModels] = useState([]);
  const [fullModels, setFullModels] = useState([]);
  const [modelGroups, setModelGroups] = useState([]);
  const [customModel, setCustomModel] = useState('');
  const [modalImageUrl, setModalImageUrl] = useState('');
  const [isModalOpenurl, setIsModalOpenurl] = useState(false);
  const [modelModalVisible, setModelModalVisible] = useState(false);
  const [fetchedModels, setFetchedModels] = useState([]);
  const formApiRef = useRef(null);
  const [vertexKeys, setVertexKeys] = useState([]);
  const [vertexFileList, setVertexFileList] = useState([]);
  const vertexErroredNames = useRef(new Set()); // 避免重复报错
  const [isMultiKeyChannel, setIsMultiKeyChannel] = useState(false);
  const [channelSearchValue, setChannelSearchValue] = useState('');
  const [useManualInput, setUseManualInput] = useState(false); // 是否使用手动输入模式
  const [keyMode, setKeyMode] = useState('append'); // 密钥模式：replace（覆盖）或 append（追加）

  // 2FA验证查看密钥相关状态
  const [twoFAState, setTwoFAState] = useState({
    showModal: false,
    code: '',
    loading: false,
    showKey: false,
    keyData: '',
  });

  // 专门的2FA验证状态（用于TwoFactorAuthModal）
  const [show2FAVerifyModal, setShow2FAVerifyModal] = useState(false);
  const [verifyCode, setVerifyCode] = useState('');
  const [verifyLoading, setVerifyLoading] = useState(false);

  // 2FA状态更新辅助函数
  const updateTwoFAState = (updates) => {
    setTwoFAState((prev) => ({ ...prev, ...updates }));
  };

  // 重置2FA状态
  const resetTwoFAState = () => {
    setTwoFAState({
      showModal: false,
      code: '',
      loading: false,
      showKey: false,
      keyData: '',
    });
  };

  // 重置2FA验证状态
  const reset2FAVerifyState = () => {
    setShow2FAVerifyModal(false);
    setVerifyCode('');
    setVerifyLoading(false);
  };

  // 渠道额外设置状态
  const [channelSettings, setChannelSettings] = useState({
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    messages_to_responses_compat: false,
    system_prompt: '',
    test_enable_max_tokens: true,
  });

  // 渠道按时间段优先级（存储在 setting.service_time_priorities）
  // UI 以 `8-22` 形式编辑，提交时转换为 {start_hour,end_hour,priority}
  const [serviceTimePriorityRules, setServiceTimePriorityRules] = useState([]);

  const showApiConfigCard = inputs.type !== 45; // 控制是否显示 API 配置卡片（仅当渠道类型不是 豆包 时显示）
  const getInitValues = () => ({ ...originInputs });

  const safeParseJSONObject = (raw) => {
    if (typeof raw !== 'string') return {};
    const text = raw.trim();
    if (!text) return {};
    try {
      const obj = JSON.parse(text);
      if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return {};
      return obj;
    } catch {
      return {};
    }
  };

  const formatUnixTime = (timestamp) => {
    const value = Number(timestamp || 0);
    if (!value) {
      return t('暂无');
    }
    return new Date(value * 1000).toLocaleString();
  };

  const normalizeServiceTimePrioritiesForSubmit = () => {
    const rules = Array.isArray(serviceTimePriorityRules)
      ? serviceTimePriorityRules
      : [];
    if (rules.length === 0) return [];

    const coveredBy = new Array(24).fill(-1);
    const markHour = (hour, idx) => {
      if (hour < 0 || hour > 23) return;
      if (coveredBy[hour] !== -1) {
        throw new Error(
          t('时间段重叠：第 {{a}} 项与第 {{b}} 项在 {{hour}}:00-{{next}}:00 重叠', {
            a: coveredBy[hour] + 1,
            b: idx + 1,
            hour: String(hour).padStart(2, '0'),
            next: String((hour + 1) % 24).padStart(2, '0'),
          }),
        );
      }
      coveredBy[hour] = idx;
    };

    const parseRange = (raw, idx) => {
      const text = String(raw || '').trim();
      const match = text.match(/^(\d{1,2})\s*-\s*(\d{1,2})$/);
      if (!match) {
        throw new Error(
          t('时间段格式错误：第 {{idx}} 项请填写如 8-22（按小时）', {
            idx: idx + 1,
          }),
        );
      }
      const start = Number(match[1]);
      const end = Number(match[2]);
      if (!Number.isInteger(start) || start < 0 || start > 23) {
        throw new Error(
          t('开始小时无效：第 {{idx}} 项 start 必须在 0-23', { idx: idx + 1 }),
        );
      }
      if (!Number.isInteger(end) || end < 0 || end > 23) {
        throw new Error(
          t('结束小时无效：第 {{idx}} 项 end 必须在 0-23', { idx: idx + 1 }),
        );
      }
      return { start, end };
    };

    const result = [];
    rules.forEach((row, idx) => {
      const { start, end } = parseRange(row?.range, idx);
      const priorityRaw = Number(row?.priority);
      if (!Number.isFinite(priorityRaw) || priorityRaw < 0) {
        throw new Error(
          t('优先级无效：第 {{idx}} 项 priority 必须大于等于 0', { idx: idx + 1 }),
        );
      }
      const priority = Math.floor(priorityRaw);

      if (start <= end) {
        for (let h = start; h <= end; h++) markHour(h, idx);
      } else {
        for (let h = start; h <= 23; h++) markHour(h, idx);
        for (let h = 0; h <= end; h++) markHour(h, idx);
      }

      result.push({ start_hour: start, end_hour: end, priority });
    });
    return result;
  };

  const addServiceTimePriorityRule = (insertAfterIndex = null) => {
    const basePriorityRaw =
      formApiRef.current && typeof formApiRef.current.getValue === 'function'
        ? formApiRef.current.getValue('priority')
        : inputs.priority;
    const basePriority = Number.isFinite(Number(basePriorityRaw))
      ? Math.max(0, Math.floor(Number(basePriorityRaw)))
      : 0;

    const newRule = { range: '', priority: basePriority };
    setServiceTimePriorityRules((prev) => {
      const cur = Array.isArray(prev) ? prev : [];
      if (insertAfterIndex === null || insertAfterIndex === undefined) {
        return [...cur, newRule];
      }
      const idx = Number(insertAfterIndex);
      if (!Number.isFinite(idx) || idx < 0 || idx >= cur.length) {
        return [...cur, newRule];
      }
      const next = [...cur];
      next.splice(idx + 1, 0, newRule);
      return next;
    });
  };

  const updateServiceTimePriorityRule = (index, patch) => {
    const idx = Number(index);
    if (!Number.isFinite(idx) || idx < 0) return;
    setServiceTimePriorityRules((prev) => {
      const cur = Array.isArray(prev) ? prev : [];
      if (idx >= cur.length) return cur;
      const next = [...cur];
      next[idx] = { ...(next[idx] || {}), ...(patch || {}) };
      return next;
    });
  };

  const removeServiceTimePriorityRule = (index) => {
    const idx = Number(index);
    if (!Number.isFinite(idx) || idx < 0) return;
    setServiceTimePriorityRules((prev) => {
      const cur = Array.isArray(prev) ? prev : [];
      if (idx >= cur.length) return cur;
      return cur.filter((_, i) => i !== idx);
    });
  };

  // 处理渠道额外设置的更新
  const handleChannelSettingsChange = (key, value) => {
    // 更新内部状态
    setChannelSettings((prev) => ({ ...prev, [key]: value }));

    // 同步更新到表单字段
    if (formApiRef.current) {
      formApiRef.current.setValue(key, value);
    }

    // 同步更新inputs状态
    setInputs((prev) => ({ ...prev, [key]: value }));

    // 合并更新 setting JSON，避免覆盖其它字段（例如 service_time_priorities）
    const currentRaw =
      (formApiRef.current && typeof formApiRef.current.getValue === 'function'
        ? formApiRef.current.getValue('setting')
        : inputs.setting) || '';
    const merged = { ...safeParseJSONObject(currentRaw), [key]: value };
    handleInputChange('setting', JSON.stringify(merged));
  };

  const handleChannelOtherSettingsChange = (key, value) => {
    // 更新内部状态
    setChannelSettings((prev) => ({ ...prev, [key]: value }));

    // 同步更新到表单字段
    if (formApiRef.current) {
      formApiRef.current.setValue(key, value);
    }

    // 同步更新inputs状态
    setInputs((prev) => ({ ...prev, [key]: value }));

    // 合并更新 settings JSON，避免覆盖其它字段
    const currentRaw =
      (formApiRef.current && typeof formApiRef.current.getValue === 'function'
        ? formApiRef.current.getValue('settings')
        : inputs.settings) || '';
    const settings = safeParseJSONObject(currentRaw);
    settings[key] = value;
    const settingsJson = JSON.stringify(settings);
    handleInputChange('settings', settingsJson);
  };

  const handleInputChange = (name, value) => {
    if (formApiRef.current) {
      formApiRef.current.setValue(name, value);
    }
    if (name === 'models' && Array.isArray(value)) {
      value = Array.from(new Set(value.map((m) => (m || '').trim())));
    }

    if (name === 'base_url' && value.endsWith('/v1')) {
      Modal.confirm({
        title: '警告',
        content:
          '不需要在末尾加/v1，Transfer API会自动处理，添加后可能导致请求失败，是否继续？',
        onOk: () => {
          setInputs((inputs) => ({ ...inputs, [name]: value }));
        },
      });
      return;
    }
    setInputs((inputs) => ({ ...inputs, [name]: value }));
    if (name === 'type') {
      let localModels = [];
      switch (value) {
        case 2:
          localModels = [
            'mj_imagine',
            'mj_variation',
            'mj_reroll',
            'mj_blend',
            'mj_upscale',
            'mj_describe',
            'mj_uploads',
          ];
          break;
        case 5:
          localModels = [
            'swap_face',
            'mj_imagine',
            'mj_video',
            'mj_edits',
            'mj_variation',
            'mj_reroll',
            'mj_blend',
            'mj_upscale',
            'mj_describe',
            'mj_zoom',
            'mj_shorten',
            'mj_modal',
            'mj_inpaint',
            'mj_custom_zoom',
            'mj_high_variation',
            'mj_low_variation',
            'mj_pan',
            'mj_uploads',
          ];
          break;
        case 36:
          localModels = ['suno_music', 'suno_lyrics'];
          break;
        default:
          localModels = getChannelModels(value);
          break;
      }
      if (inputs.models.length === 0) {
        setInputs((inputs) => ({ ...inputs, models: localModels }));
      }
      setBasicModels(localModels);

      // 重置手动输入模式状态
      setUseManualInput(false);
    }
    //setAutoBan
  };

  const loadChannel = async () => {
    setLoading(true);
    let res = await API.get(`/api/channel/${channelId}`);
    if (res === undefined) {
      return;
    }
    const { success, message, data } = res.data;
    if (success) {
      if (data.models === '') {
        data.models = [];
      } else {
        data.models = data.models.split(',');
      }
      data.group_ids = Array.isArray(data.group_ids)
        ? Array.from(
            new Set(
              data.group_ids
                .map((v) => Number(v))
                .filter((v) => Number.isFinite(v) && v > 0)
                .map((v) => Math.floor(v)),
            ),
          ).sort((a, b) => a - b)
        : [];
      data.backup_group_ids = Array.isArray(data.backup_group_ids)
        ? Array.from(
            new Set(
              data.backup_group_ids
                .map((v) => Number(v))
                .filter((v) => Number.isFinite(v) && v > 0)
                .map((v) => Math.floor(v)),
            ),
          ).sort((a, b) => a - b)
        : [];
      if (data.model_mapping !== '') {
        data.model_mapping = JSON.stringify(
          JSON.parse(data.model_mapping),
          null,
          2,
        );
      }
      const chInfo = data.channel_info || {};
      const isMulti = chInfo.is_multi_key === true;
      setIsMultiKeyChannel(isMulti);
      if (isMulti) {
        setBatch(true);
        setMultiToSingle(true);
        const modeVal = chInfo.multi_key_mode || 'random';
        setMultiKeyMode(modeVal);
        data.multi_key_mode = modeVal;
      } else {
        setBatch(false);
        setMultiToSingle(false);
      }
      // 解析渠道额外设置并合并到data中
      if (data.setting) {
        try {
          const parsedSettings = JSON.parse(data.setting);
          data.force_format = parsedSettings.force_format || false;
          data.thinking_to_content =
            parsedSettings.thinking_to_content || false;
          data.proxy = parsedSettings.proxy || '';
          data.pass_through_body_enabled =
            parsedSettings.pass_through_body_enabled || false;
          data.messages_to_responses_compat =
            parsedSettings.messages_to_responses_compat || false;
          data.system_prompt = parsedSettings.system_prompt || '';
          data.system_prompt_override =
            parsedSettings.system_prompt_override || false;
          data.test_enable_max_tokens =
            parsedSettings.test_enable_max_tokens !== undefined
              ? parsedSettings.test_enable_max_tokens
              : true;

          const rawServiceTimePriorities = Array.isArray(
            parsedSettings.service_time_priorities,
          )
            ? parsedSettings.service_time_priorities
            : [];
          const uiRules = rawServiceTimePriorities
            .map((it) => {
              const start = Number(it?.start_hour);
              const end = Number(it?.end_hour);
              const priority = Number(it?.priority);
              if (
                !Number.isFinite(start) ||
                !Number.isFinite(end) ||
                !Number.isFinite(priority)
              ) {
                return null;
              }
              return {
                range: `${Math.floor(start)}-${Math.floor(end)}`,
                priority: Math.max(0, Math.floor(priority)),
              };
            })
            .filter(Boolean);
          setServiceTimePriorityRules(uiRules);
        } catch (error) {
          console.error('解析渠道设置失败:', error);
          data.force_format = false;
          data.thinking_to_content = false;
          data.proxy = '';
          data.pass_through_body_enabled = false;
          data.messages_to_responses_compat = false;
          data.system_prompt = '';
          data.system_prompt_override = false;
          data.test_enable_max_tokens = true;
          setServiceTimePriorityRules([]);
        }
      } else {
        data.force_format = false;
        data.thinking_to_content = false;
        data.proxy = '';
        data.pass_through_body_enabled = false;
        data.messages_to_responses_compat = false;
        data.system_prompt = '';
        data.system_prompt_override = false;
        data.test_enable_max_tokens = true;
        setServiceTimePriorityRules([]);
      }

      if (data.settings) {
        try {
          const parsedSettings = JSON.parse(data.settings);
          data.azure_responses_version =
            parsedSettings.azure_responses_version || '';
          // 读取 Vertex 密钥格式
          data.vertex_key_type = parsedSettings.vertex_key_type || 'json';
          data.upstream_model_update_check_enabled =
            parsedSettings.upstream_model_update_check_enabled === true;
          data.upstream_model_update_auto_sync_enabled =
            parsedSettings.upstream_model_update_auto_sync_enabled === true;
          data.upstream_model_update_last_check_time =
            Number(parsedSettings.upstream_model_update_last_check_time) || 0;
          data.upstream_model_update_last_detected_models = Array.isArray(
            parsedSettings.upstream_model_update_last_detected_models,
          )
            ? parsedSettings.upstream_model_update_last_detected_models
            : [];
          data.upstream_model_update_ignored_models = Array.isArray(
            parsedSettings.upstream_model_update_ignored_models,
          )
            ? parsedSettings.upstream_model_update_ignored_models.join(',')
            : '';
        } catch (error) {
          console.error('解析其他设置失败:', error);
          data.azure_responses_version = '';
          data.region = '';
          data.vertex_key_type = 'json';
          data.upstream_model_update_check_enabled = false;
          data.upstream_model_update_auto_sync_enabled = false;
          data.upstream_model_update_last_check_time = 0;
          data.upstream_model_update_last_detected_models = [];
          data.upstream_model_update_ignored_models = '';
        }
      } else {
        // 兼容历史数据：老渠道没有 settings 时，默认按 json 展示
        data.vertex_key_type = 'json';
        data.upstream_model_update_check_enabled = false;
        data.upstream_model_update_auto_sync_enabled = false;
        data.upstream_model_update_last_check_time = 0;
        data.upstream_model_update_last_detected_models = [];
        data.upstream_model_update_ignored_models = '';
      }

      const maxConcurrency = Number(data.max_concurrency);
      data.max_concurrency =
        Number.isFinite(maxConcurrency) && Number.isInteger(maxConcurrency)
          ? maxConcurrency
          : -1;

      setInputs(data);
      if (formApiRef.current) {
        formApiRef.current.setValues(data);
      }
      if (data.auto_ban === 0) {
        setAutoBan(false);
      } else {
        setAutoBan(true);
      }
      setBasicModels(getChannelModels(data.type));
      // 同步更新channelSettings状态显示
      setChannelSettings({
        force_format: data.force_format,
        thinking_to_content: data.thinking_to_content,
        proxy: data.proxy,
        pass_through_body_enabled: data.pass_through_body_enabled,
        messages_to_responses_compat: data.messages_to_responses_compat,
        system_prompt: data.system_prompt,
        system_prompt_override: data.system_prompt_override || false,
        test_enable_max_tokens:
          data.test_enable_max_tokens !== undefined
            ? data.test_enable_max_tokens
            : true,
      });
      // console.log(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const fetchUpstreamModelList = async (name) => {
    // if (inputs['type'] !== 1) {
    //   showError(t('仅支持 OpenAI 接口格式'));
    //   return;
    // }
    setLoading(true);
    const models = [];
    let err = false;

    if (isEdit) {
      // 如果是编辑模式，使用已有的 channelId 获取模型列表
      const res = await API.get('/api/channel/fetch_models/' + channelId, {
        skipErrorHandler: true,
      });
      if (res && res.data && res.data.success) {
        models.push(...res.data.data);
      } else {
        err = true;
      }
    } else {
      // 如果是新建模式，通过后端代理获取模型列表
      if (!inputs?.['key']) {
        showError(t('请填写密钥'));
        err = true;
      } else {
        try {
          const res = await API.post(
            '/api/channel/fetch_models',
            {
              base_url: inputs['base_url'],
              type: inputs['type'],
              key: inputs['key'],
              proxy: (channelSettings?.proxy || '').trim(),
            },
            { skipErrorHandler: true },
          );

          if (res && res.data && res.data.success) {
            models.push(...res.data.data);
          } else {
            err = true;
          }
        } catch (error) {
          console.error('Error fetching models:', error);
          err = true;
        }
      }
    }

    if (!err) {
      const uniqueModels = Array.from(new Set(models));
      setFetchedModels(uniqueModels);
      setModelModalVisible(true);
    } else {
      showError(t('获取模型列表失败'));
    }
    setLoading(false);
  };

  const fetchModels = async () => {
    try {
      let res = await API.get(`/api/channel/models`);
      const localModelOptions = res.data.data.map((model) => {
        const id = (model.id || '').trim();
        return {
          key: id,
          label: id,
          value: id,
        };
      });
      setOriginModelOptions(localModelOptions);
      setFullModels(res.data.data.map((model) => model.id));
      setBasicModels(
        res.data.data
          .filter((model) => {
            return model.id.startsWith('gpt-') || model.id.startsWith('text-');
          })
          .map((model) => model.id),
      );
    } catch (error) {
      showError(error.message);
    }
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      if (res === undefined) {
        return;
      }
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取分组失败'));
        return;
      }
      const list = Array.isArray(data) ? data : [];
      const normalized = list
        .map((g) => {
          if (typeof g === 'string') {
            const code = String(g || '').trim();
            return code ? { id: 0, code, display_name: code } : null;
          }
          const id = Number(g?.id ?? 0);
          if (!Number.isFinite(id) || id <= 0) return null;
          const code = String(g?.code || '').trim();
          const displayName = String(g?.display_name || '').trim();
          const name = displayName || code;
          if (!name) return null;
          return { id: Math.floor(id), code, display_name: name };
        })
        .filter(Boolean);
      setGroupOptions(
        normalized.map((g) => ({
          label: g.display_name || g.code,
          value: g.id,
        })),
      );

      // Default selection for new channel: prefer code="default", otherwise the first group.
      if (!isEdit && normalized.length > 0) {
        const defaultGroup =
          normalized.find((g) => String(g?.code || '').trim() === 'default') || normalized[0];
        if (defaultGroup?.id > 0) {
          setInputs((prev) => {
            const existing = Array.isArray(prev?.group_ids) ? prev.group_ids : [];
            if (existing.length > 0) return prev;
            const next = { ...prev, group_ids: [defaultGroup.id] };
            formApiRef.current?.setValue('group_ids', next.group_ids);
            return next;
          });
        }
      }
    } catch (error) {
      showError(error.message);
    }
  };

  const fetchModelGroups = async () => {
    try {
      const res = await API.get('/api/prefill_group?type=model');
      if (res?.data?.success) {
        setModelGroups(res.data.data || []);
      }
    } catch (error) {
      // ignore
    }
  };

  // 使用TwoFactorAuthModal的验证函数
  const handleVerify2FA = async () => {
    if (!verifyCode) {
      showError(t('请输入验证码或备用码'));
      return;
    }

    setVerifyLoading(true);
    try {
      const res = await API.post(`/api/channel/${channelId}/key`, {
        code: verifyCode,
      });
      if (res.data.success) {
        // 验证成功，显示密钥
        updateTwoFAState({
          showModal: true,
          showKey: true,
          keyData: res.data.data.key,
        });
        reset2FAVerifyState();
        showSuccess(t('验证成功'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取密钥失败'));
    } finally {
      setVerifyLoading(false);
    }
  };

  // 显示2FA验证模态框 - 使用TwoFactorAuthModal
  const handleShow2FAModal = () => {
    setShow2FAVerifyModal(true);
  };

  useEffect(() => {
    const modelMap = new Map();

    originModelOptions.forEach((option) => {
      const v = (option.value || '').trim();
      if (!modelMap.has(v)) {
        modelMap.set(v, option);
      }
    });

    inputs.models.forEach((model) => {
      const v = (model || '').trim();
      if (!modelMap.has(v)) {
        modelMap.set(v, {
          key: v,
          label: v,
          value: v,
        });
      }
    });

    const categories = getModelCategories(t);
    const optionsWithIcon = Array.from(modelMap.values()).map((opt) => {
      const modelName = opt.value;
      let icon = null;
      for (const [key, category] of Object.entries(categories)) {
        if (key !== 'all' && category.filter({ model_name: modelName })) {
          icon = category.icon;
          break;
        }
      }
      return {
        ...opt,
        label: (
          <span className='flex items-center gap-1'>
            {icon}
            {modelName}
          </span>
        ),
      };
    });

    setModelOptions(optionsWithIcon);
  }, [originModelOptions, inputs.models, t]);

  useEffect(() => {
    fetchModels().then();
    fetchGroups().then();
    if (!isEdit) {
      setInputs(originInputs);
      setServiceTimePriorityRules([]);
      if (formApiRef.current) {
        formApiRef.current.setValues(originInputs);
      }
      let localModels = getChannelModels(inputs.type);
      setBasicModels(localModels);
      setInputs((inputs) => ({ ...inputs, models: localModels }));
    }
  }, [props.editingChannel.id]);

  useEffect(() => {
    if (formApiRef.current) {
      formApiRef.current.setValues(inputs);
    }
  }, [inputs]);

  useEffect(() => {
    if (props.visible) {
      if (isEdit) {
        loadChannel();
      } else {
        formApiRef.current?.setValues(getInitValues());
      }
      fetchModelGroups();
      // 重置手动输入模式状态
      setUseManualInput(false);
    } else {
      // 统一的模态框关闭重置逻辑
      resetModalState();
    }
  }, [props.visible, channelId]);

  // 统一的模态框重置函数
  const resetModalState = () => {
    formApiRef.current?.reset();
    // 重置渠道设置状态
    setChannelSettings({
      force_format: false,
      thinking_to_content: false,
      proxy: '',
      pass_through_body_enabled: false,
      messages_to_responses_compat: false,
      system_prompt: '',
      system_prompt_override: false,
      test_enable_max_tokens: true,
    });
    setServiceTimePriorityRules([]);
    // 重置密钥模式状态
    setKeyMode('append');
    // 清空表单中的key_mode字段
    if (formApiRef.current) {
      formApiRef.current.setValue('key_mode', undefined);
    }
    // 重置本地输入，避免下次打开残留上一次的 JSON 字段值
    setInputs(getInitValues());
    // 重置2FA状态
    resetTwoFAState();
    // 重置2FA验证状态
    reset2FAVerifyState();
  };

  const handleVertexUploadChange = ({ fileList }) => {
    vertexErroredNames.current.clear();
    (async () => {
      let validFiles = [];
      let keys = [];
      const errorNames = [];
      for (const item of fileList) {
        const fileObj = item.fileInstance;
        if (!fileObj) continue;
        try {
          const txt = await fileObj.text();
          keys.push(JSON.parse(txt));
          validFiles.push(item);
        } catch (err) {
          if (!vertexErroredNames.current.has(item.name)) {
            errorNames.push(item.name);
            vertexErroredNames.current.add(item.name);
          }
        }
      }

      // 非批量模式下只保留一个文件（最新选择的），避免重复叠加
      if (!batch && validFiles.length > 1) {
        validFiles = [validFiles[validFiles.length - 1]];
        keys = [keys[keys.length - 1]];
      }

      setVertexKeys(keys);
      setVertexFileList(validFiles);
      if (formApiRef.current) {
        formApiRef.current.setValue('vertex_files', validFiles);
      }
      setInputs((prev) => ({ ...prev, vertex_files: validFiles }));

      if (errorNames.length > 0) {
        showError(
          t('以下文件解析失败，已忽略：{{list}}', {
            list: errorNames.join(', '),
          }),
        );
      }
    })();
  };

  const submit = async () => {
    const formValues = formApiRef.current ? formApiRef.current.getValues() : {};
    let localInputs = { ...formValues };

    if (localInputs.type === 41) {
      const keyType = localInputs.vertex_key_type || 'json';
      if (keyType === 'api_key') {
        // 直接作为普通字符串密钥处理
        if (!isEdit && (!localInputs.key || localInputs.key.trim() === '')) {
          showInfo(t('请输入密钥！'));
          return;
        }
      } else {
        // JSON 服务账号密钥
        if (useManualInput) {
          if (localInputs.key && localInputs.key.trim() !== '') {
            try {
              const parsedKey = JSON.parse(localInputs.key);
              localInputs.key = JSON.stringify(parsedKey);
            } catch (err) {
              showError(t('密钥格式无效，请输入有效的 JSON 格式密钥'));
              return;
            }
          } else if (!isEdit) {
            showInfo(t('请输入密钥！'));
            return;
          }
        } else {
          // 文件上传模式
          let keys = vertexKeys;
          if (keys.length === 0 && vertexFileList.length > 0) {
            try {
              const parsed = await Promise.all(
                vertexFileList.map(async (item) => {
                  const fileObj = item.fileInstance;
                  if (!fileObj) return null;
                  const txt = await fileObj.text();
                  return JSON.parse(txt);
                }),
              );
              keys = parsed.filter(Boolean);
            } catch (err) {
              showError(t('解析密钥文件失败: {{msg}}', { msg: err.message }));
              return;
            }
          }
          if (keys.length === 0) {
            if (!isEdit) {
              showInfo(t('请上传密钥文件！'));
              return;
            } else {
              delete localInputs.key;
            }
          } else {
            localInputs.key = batch ? JSON.stringify(keys) : JSON.stringify(keys[0]);
          }
        }
      }
    }

    // 如果是编辑模式且 key 为空字符串，避免提交空值覆盖旧密钥
    if (isEdit && (!localInputs.key || localInputs.key.trim() === '')) {
      delete localInputs.key;
    }
    delete localInputs.vertex_files;

    if (!isEdit && (!localInputs.name || !localInputs.key)) {
      showInfo(t('请填写渠道名称和渠道密钥！'));
      return;
    }
    if (!Array.isArray(localInputs.models) || localInputs.models.length === 0) {
      showInfo(t('请至少选择一个模型！'));
      return;
    }
    localInputs.group_ids = Array.isArray(localInputs.group_ids)
      ? Array.from(
          new Set(
            localInputs.group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v)),
          ),
        ).sort((a, b) => a - b)
      : [];
    if (localInputs.group_ids.length === 0) {
      showError(t('请选择可以使用该渠道的分组'));
      return;
    }
    localInputs.backup_group_ids = Array.isArray(localInputs.backup_group_ids)
      ? Array.from(
          new Set(
            localInputs.backup_group_ids
              .map((v) => Number(v))
              .filter((v) => Number.isFinite(v) && v > 0)
              .map((v) => Math.floor(v)),
          ),
        )
          .filter((gid) => !localInputs.group_ids.includes(gid))
          .sort((a, b) => a - b)
      : [];
    if (
      localInputs.model_mapping &&
      localInputs.model_mapping !== '' &&
      !verifyJSON(localInputs.model_mapping)
    ) {
      showInfo(t('模型映射必须是合法的 JSON 格式！'));
      return;
    }
    if (localInputs.base_url && localInputs.base_url.endsWith('/')) {
      localInputs.base_url = localInputs.base_url.slice(
        0,
        localInputs.base_url.length - 1,
      );
    }
    if (localInputs.type === 18 && localInputs.other === '') {
      localInputs.other = 'v2.1';
    }

    const billingMode = String(localInputs.billing_mode || 'quota').trim() || 'quota';
    localInputs.billing_mode = billingMode;
    const maxConcurrency = Number(localInputs.max_concurrency);
    if (
      !Number.isFinite(maxConcurrency) ||
      !Number.isInteger(maxConcurrency) ||
      (maxConcurrency !== -1 && maxConcurrency <= 0)
    ) {
      showError(t('渠道最大并行度仅支持 -1（不限制）或大于 0 的整数'));
      return;
    }
    localInputs.max_concurrency = maxConcurrency;
    if (billingMode === 'request') {
      const buyRate = Number(localInputs.buy_requests_per_cny);
      const sellRate = Number(localInputs.sell_requests_per_cny);
      if (
        !Number.isFinite(buyRate) ||
        buyRate <= 0 ||
        !Number.isFinite(sellRate) ||
        sellRate <= 0
      ) {
        showError(t('按次计费模式下，请正确填写进价与售价（¥1=N次）'));
        return;
      }
      localInputs.buy_requests_per_cny = Math.floor(buyRate);
      localInputs.sell_requests_per_cny = Math.floor(sellRate);
    }

    // 生成渠道额外设置JSON
    let serviceTimePriorities = [];
    try {
      serviceTimePriorities = normalizeServiceTimePrioritiesForSubmit();
    } catch (e) {
      showError(e?.message || t('服务时间段配置无效'));
      return;
    }

    const channelExtraSettings = safeParseJSONObject(localInputs.setting || '');
    channelExtraSettings.force_format = localInputs.force_format || false;
    channelExtraSettings.thinking_to_content =
      localInputs.thinking_to_content || false;
    channelExtraSettings.proxy = localInputs.proxy || '';
    channelExtraSettings.pass_through_body_enabled =
      localInputs.pass_through_body_enabled || false;
    channelExtraSettings.messages_to_responses_compat =
      localInputs.messages_to_responses_compat || false;
    channelExtraSettings.system_prompt = localInputs.system_prompt || '';
    channelExtraSettings.system_prompt_override =
      localInputs.system_prompt_override || false;
    channelExtraSettings.test_enable_max_tokens =
      localInputs.test_enable_max_tokens !== undefined
        ? localInputs.test_enable_max_tokens
        : true;
    if (serviceTimePriorities.length > 0) {
      channelExtraSettings.service_time_priorities = serviceTimePriorities;
    } else {
      delete channelExtraSettings.service_time_priorities;
    }
    localInputs.setting = JSON.stringify(channelExtraSettings);

    const otherSettings = safeParseJSONObject(localInputs.settings || '');
    const upstreamFetchSupported = MODEL_FETCHABLE_CHANNEL_TYPES.has(
      localInputs.type,
    );
    otherSettings.upstream_model_update_check_enabled =
      upstreamFetchSupported &&
      localInputs.upstream_model_update_check_enabled === true;
    otherSettings.upstream_model_update_auto_sync_enabled =
      otherSettings.upstream_model_update_check_enabled &&
      localInputs.upstream_model_update_auto_sync_enabled === true;
    otherSettings.upstream_model_update_ignored_models = Array.from(
      new Set(
        String(localInputs.upstream_model_update_ignored_models || '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean),
      ),
    );
    if (
      !Array.isArray(otherSettings.upstream_model_update_last_detected_models) ||
      !otherSettings.upstream_model_update_check_enabled
    ) {
      otherSettings.upstream_model_update_last_detected_models = [];
    }
    if (!Array.isArray(otherSettings.upstream_model_update_last_removed_models)) {
      otherSettings.upstream_model_update_last_removed_models = [];
    }
    if (typeof otherSettings.upstream_model_update_last_check_time !== 'number') {
      otherSettings.upstream_model_update_last_check_time = 0;
    }
    localInputs.settings = JSON.stringify(otherSettings);

    // 清理不需要发送到后端的字段
    delete localInputs.force_format;
    delete localInputs.thinking_to_content;
    delete localInputs.proxy;
    delete localInputs.pass_through_body_enabled;
    delete localInputs.messages_to_responses_compat;
    delete localInputs.system_prompt;
    delete localInputs.system_prompt_override;
    delete localInputs.test_enable_max_tokens;
    // 顶层的 vertex_key_type 不应发送给后端
    delete localInputs.vertex_key_type;
    delete localInputs.upstream_model_update_check_enabled;
    delete localInputs.upstream_model_update_auto_sync_enabled;
    delete localInputs.upstream_model_update_last_check_time;
    delete localInputs.upstream_model_update_last_detected_models;
    delete localInputs.upstream_model_update_ignored_models;

    let res;
    localInputs.auto_ban = localInputs.auto_ban ? 1 : 0;
    localInputs.models = localInputs.models.join(',');

    let mode = 'single';
    if (batch) {
      mode = multiToSingle ? 'multi_to_single' : 'batch';
    }

    if (isEdit) {
      res = await API.put(`/api/channel/`, {
        ...localInputs,
        id: parseInt(channelId),
        key_mode: isMultiKeyChannel ? keyMode : undefined, // 只在多key模式下传递
      });
    } else {
      res = await API.post(`/api/channel/`, {
        mode: mode,
        multi_key_mode: mode === 'multi_to_single' ? multiKeyMode : undefined,
        channel: localInputs,
      });
    }
    const { success, message } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('渠道更新成功！'));
      } else {
        showSuccess(t('渠道创建成功！'));
        setInputs(originInputs);
      }
      props.refresh();
      props.handleClose();
    } else {
      showError(message);
    }
  };

  const addCustomModels = () => {
    if (customModel.trim() === '') return;
    const modelArray = customModel.split(',').map((model) => model.trim());

    let localModels = [...inputs.models];
    let localModelOptions = [...modelOptions];
    const addedModels = [];

    modelArray.forEach((model) => {
      if (model && !localModels.includes(model)) {
        localModels.push(model);
        localModelOptions.push({
          key: model,
          label: model,
          value: model,
        });
        addedModels.push(model);
      }
    });

    setModelOptions(localModelOptions);
    setCustomModel('');
    handleInputChange('models', localModels);

    if (addedModels.length > 0) {
      showSuccess(
        t('已新增 {{count}} 个模型：{{list}}', {
          count: addedModels.length,
          list: addedModels.join(', '),
        }),
      );
    } else {
      showInfo(t('未发现新增模型'));
    }
  };

  const batchAllowed = !isEdit || isMultiKeyChannel;
  const batchExtra = batchAllowed ? (
    <Space>
      {!isEdit && (
        <Checkbox
          disabled={isEdit}
          checked={batch}
          onChange={(e) => {
            const checked = e.target.checked;

            if (!checked && vertexFileList.length > 1) {
              Modal.confirm({
                title: t('切换为单密钥模式'),
                content: t(
                  '将仅保留第一个密钥文件，其余文件将被移除，是否继续？',
                ),
                onOk: () => {
                  const firstFile = vertexFileList[0];
                  const firstKey = vertexKeys[0] ? [vertexKeys[0]] : [];

                  setVertexFileList([firstFile]);
                  setVertexKeys(firstKey);

                  formApiRef.current?.setValue('vertex_files', [firstFile]);
                  setInputs((prev) => ({ ...prev, vertex_files: [firstFile] }));

                  setBatch(false);
                  setMultiToSingle(false);
                  setMultiKeyMode('random');
                },
                onCancel: () => {
                  setBatch(true);
                },
                centered: true,
              });
              return;
            }

            setBatch(checked);
            if (!checked) {
              setMultiToSingle(false);
              setMultiKeyMode('random');
            } else {
              // 批量模式下禁用手动输入，并清空手动输入的内容
              setUseManualInput(false);
              if (inputs.type === 41) {
                // 清空手动输入的密钥内容
                if (formApiRef.current) {
                  formApiRef.current.setValue('key', '');
                }
                handleInputChange('key', '');
              }
            }
          }}
        >
          {t('批量创建')}
        </Checkbox>
      )}
      {batch && (
        <Checkbox
          disabled={isEdit}
          checked={multiToSingle}
          onChange={() => {
            setMultiToSingle((prev) => !prev);
            setInputs((prev) => {
              const newInputs = { ...prev };
              if (!multiToSingle) {
                newInputs.multi_key_mode = multiKeyMode;
              } else {
                delete newInputs.multi_key_mode;
              }
              return newInputs;
            });
          }}
        >
          {t('密钥聚合模式')}
        </Checkbox>
      )}
    </Space>
  ) : null;

  const channelOptionList = useMemo(
    () =>
      CHANNEL_OPTIONS.map((opt) => ({
        ...opt,
        // 保持 label 为纯文本以支持搜索
        label: opt.label,
      })),
    [],
  );

  const renderChannelOption = (renderProps) => {
    const {
      disabled,
      selected,
      label,
      value,
      focused,
      className,
      style,
      onMouseEnter,
      onClick,
      ...rest
    } = renderProps;

    const searchWords = channelSearchValue ? [channelSearchValue] : [];

    // 构建样式类名
    const optionClassName = [
      'flex items-center gap-3 px-3 py-2 transition-all duration-200 rounded-lg mx-2 my-1',
      focused && 'bg-blue-50 shadow-sm',
      selected &&
        'bg-blue-100 text-blue-700 shadow-lg ring-2 ring-blue-200 ring-opacity-50',
      disabled && 'opacity-50 cursor-not-allowed',
      !disabled && 'hover:bg-gray-50 hover:shadow-md cursor-pointer',
      className,
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <div
        style={style}
        className={optionClassName}
        onClick={() => !disabled && onClick()}
        onMouseEnter={(e) => onMouseEnter()}
      >
        <div className='flex items-center gap-3 w-full'>
          <div className='flex-shrink-0 w-5 h-5 flex items-center justify-center'>
            {getChannelIcon(value)}
          </div>
          <div className='flex-1 min-w-0'>
            <Highlight
              sourceString={label}
              searchWords={searchWords}
              className='text-sm font-medium truncate'
            />
          </div>
          {selected && (
            <div className='flex-shrink-0 text-blue-600'>
              <svg
                width='16'
                height='16'
                viewBox='0 0 16 16'
                fill='currentColor'
              >
                <path d='M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z' />
              </svg>
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            <Tag color='blue' shape='circle'>
              {isEdit ? t('编辑') : t('新建')}
            </Tag>
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新渠道信息') : t('创建新的渠道')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visible}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleCancel}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
      >
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={originInputs}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {() => (
            <Spin spinning={loading}>
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Basic Info */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconServer size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('渠道的基本配置信息')}
                      </div>
                    </div>
                  </div>

                  <Form.Select
                    field='type'
                    label={t('类型')}
                    placeholder={t('请选择渠道类型')}
                    rules={[{ required: true, message: t('请选择渠道类型') }]}
                    optionList={channelOptionList}
                    style={{ width: '100%' }}
                    filter={selectFilter}
                    autoClearSearchValue={false}
                    searchPosition='dropdown'
                    onSearch={(value) => setChannelSearchValue(value)}
                    renderOptionItem={renderChannelOption}
                    onChange={(value) => handleInputChange('type', value)}
                  />

                  <Form.Input
                    field='name'
                    label={t('名称')}
                    placeholder={t('请为渠道命名')}
                    rules={[{ required: true, message: t('请为渠道命名') }]}
                    showClear
                    onChange={(value) => handleInputChange('name', value)}
                    autoComplete='new-password'
                  />

                  {inputs.type === 41 && (
                    <Form.Select
                      field='vertex_key_type'
                      label={t('密钥格式')}
                      placeholder={t('请选择密钥格式')}
                      optionList={[
                        { label: 'JSON', value: 'json' },
                        { label: 'API Key', value: 'api_key' },
                      ]}
                      style={{ width: '100%' }}
                      value={inputs.vertex_key_type || 'json'}
                      onChange={(value) => {
                        // 更新设置中的 vertex_key_type
                        handleChannelOtherSettingsChange('vertex_key_type', value);
                        // 切换为 api_key 时，关闭批量与手动/文件切换，并清理已选文件
                        if (value === 'api_key') {
                          setBatch(false);
                          setUseManualInput(false);
                          setVertexKeys([]);
                          setVertexFileList([]);
                          if (formApiRef.current) {
                            formApiRef.current.setValue('vertex_files', []);
                          }
                        }
                      }}
                      extraText={
                        inputs.vertex_key_type === 'api_key'
                          ? t('API Key 模式下不支持批量创建')
                          : t('JSON 模式支持手动输入或上传服务账号 JSON')
                      }
                    />
                  )}
                  {batch ? (
                    inputs.type === 41 && (inputs.vertex_key_type || 'json') === 'json' ? (
                      <Form.Upload
                        field='vertex_files'
                        label={t('密钥文件 (.json)')}
                        accept='.json'
                        multiple
                        draggable
                        dragIcon={<IconBolt />}
                        dragMainText={t('点击上传文件或拖拽文件到这里')}
                        dragSubText={t('仅支持 JSON 文件，支持多文件')}
                        style={{ marginTop: 10 }}
                        uploadTrigger='custom'
                        beforeUpload={() => false}
                        onChange={handleVertexUploadChange}
                        fileList={vertexFileList}
                        rules={
                          isEdit
                            ? []
                            : [{ required: true, message: t('请上传密钥文件') }]
                        }
                        extraText={batchExtra}
                      />
                    ) : (
                      <Form.TextArea
                        field='key'
                        label={t('密钥')}
                        placeholder={t('请输入密钥，一行一个')}
                        rules={
                          isEdit
                            ? []
                            : [{ required: true, message: t('请输入密钥') }]
                        }
                        autosize
                        autoComplete='new-password'
                        onChange={(value) => handleInputChange('key', value)}
                        extraText={
                          <div className='flex items-center gap-2'>
                            {isEdit &&
                              isMultiKeyChannel &&
                              keyMode === 'append' && (
                                <Text type='warning' size='small'>
                                  {t(
                                    '追加模式：新密钥将添加到现有密钥列表的末尾',
                                  )}
                                </Text>
                              )}
                            {isEdit && (
                              <Button
                                size='small'
                                type='primary'
                                theme='outline'
                                onClick={handleShow2FAModal}
                              >
                                {t('查看密钥')}
                              </Button>
                            )}
                            {batchExtra}
                          </div>
                        }
                        showClear
                      />
                    )
                  ) : (
                    <>
                      {inputs.type === 41 && (inputs.vertex_key_type || 'json') === 'json' ? (
                        <>
                          {!batch && (
                            <div className='flex items-center justify-between mb-3'>
                              <Text className='text-sm font-medium'>
                                {t('密钥输入方式')}
                              </Text>
                              <Space>
                                <Button
                                  size='small'
                                  type={
                                    !useManualInput ? 'primary' : 'tertiary'
                                  }
                                  onClick={() => {
                                    setUseManualInput(false);
                                    // 切换到文件上传模式时清空手动输入的密钥
                                    if (formApiRef.current) {
                                      formApiRef.current.setValue('key', '');
                                    }
                                    handleInputChange('key', '');
                                  }}
                                >
                                  {t('文件上传')}
                                </Button>
                                <Button
                                  size='small'
                                  type={useManualInput ? 'primary' : 'tertiary'}
                                  onClick={() => {
                                    setUseManualInput(true);
                                    // 切换到手动输入模式时清空文件上传相关状态
                                    setVertexKeys([]);
                                    setVertexFileList([]);
                                    if (formApiRef.current) {
                                      formApiRef.current.setValue(
                                        'vertex_files',
                                        [],
                                      );
                                    }
                                    setInputs((prev) => ({
                                      ...prev,
                                      vertex_files: [],
                                    }));
                                  }}
                                >
                                  {t('手动输入')}
                                </Button>
                              </Space>
                            </div>
                          )}

                          {batch && (
                            <Banner
                              type='info'
                              description={t(
                                '批量创建模式下仅支持文件上传，不支持手动输入',
                              )}
                              className='!rounded-lg mb-3'
                            />
                          )}

                          {useManualInput && !batch ? (
                            <Form.TextArea
                              field='key'
                              label={
                                isEdit
                                  ? t('密钥（编辑模式下，保存的密钥不会显示）')
                                  : t('密钥')
                              }
                              placeholder={t(
                                '请输入 JSON 格式的密钥内容，例如：\n{\n  "type": "service_account",\n  "project_id": "your-project-id",\n  "private_key_id": "...",\n  "private_key": "...",\n  "client_email": "...",\n  "client_id": "...",\n  "auth_uri": "...",\n  "token_uri": "...",\n  "auth_provider_x509_cert_url": "...",\n  "client_x509_cert_url": "..."\n}',
                              )}
                              rules={
                                isEdit
                                  ? []
                                  : [
                                      {
                                        required: true,
                                        message: t('请输入密钥'),
                                      },
                                    ]
                              }
                              autoComplete='new-password'
                              onChange={(value) =>
                                handleInputChange('key', value)
                              }
                              extraText={
                                <div className='flex items-center gap-2'>
                                  <Text type='tertiary' size='small'>
                                    {t('请输入完整的 JSON 格式密钥内容')}
                                  </Text>
                                  {isEdit &&
                                    isMultiKeyChannel &&
                                    keyMode === 'append' && (
                                      <Text type='warning' size='small'>
                                        {t(
                                          '追加模式：新密钥将添加到现有密钥列表的末尾',
                                        )}
                                      </Text>
                                    )}
                                  {isEdit && (
                                    <Button
                                      size='small'
                                      type='primary'
                                      theme='outline'
                                      onClick={handleShow2FAModal}
                                    >
                                      {t('查看密钥')}
                                    </Button>
                                  )}
                                  {batchExtra}
                                </div>
                              }
                              autosize
                              showClear
                            />
                          ) : (
                            <Form.Upload
                              field='vertex_files'
                              label={t('密钥文件 (.json)')}
                              accept='.json'
                              draggable
                              dragIcon={<IconBolt />}
                              dragMainText={t('点击上传文件或拖拽文件到这里')}
                              dragSubText={t('仅支持 JSON 文件')}
                              style={{ marginTop: 10 }}
                              uploadTrigger='custom'
                              beforeUpload={() => false}
                              onChange={handleVertexUploadChange}
                              fileList={vertexFileList}
                              rules={
                                isEdit
                                  ? []
                                  : [
                                      {
                                        required: true,
                                        message: t('请上传密钥文件'),
                                      },
                                    ]
                              }
                              extraText={batchExtra}
                            />
                          )}
                        </>
                      ) : (
                        <Form.Input
                          field='key'
                          label={
                            isEdit
                              ? t('密钥（编辑模式下，保存的密钥不会显示）')
                              : t('密钥')
                          }
                          placeholder={t(type2secretPrompt(inputs.type))}
                          rules={
                            isEdit
                              ? []
                              : [{ required: true, message: t('请输入密钥') }]
                          }
                          autoComplete='new-password'
                          onChange={(value) => handleInputChange('key', value)}
                          extraText={
                            <div className='flex items-center gap-2'>
                              {isEdit &&
                                isMultiKeyChannel &&
                                keyMode === 'append' && (
                                  <Text type='warning' size='small'>
                                    {t(
                                      '追加模式：新密钥将添加到现有密钥列表的末尾',
                                    )}
                                  </Text>
                                )}
                              {isEdit && (
                                <Button
                                  size='small'
                                  type='primary'
                                  theme='outline'
                                  onClick={handleShow2FAModal}
                                >
                                  {t('查看密钥')}
                                </Button>
                              )}
                              {batchExtra}
                            </div>
                          }
                          showClear
                        />
                      )}
                    </>
                  )}

                  {isEdit && isMultiKeyChannel && (
                    <Form.Select
                      field='key_mode'
                      label={t('密钥更新模式')}
                      placeholder={t('请选择密钥更新模式')}
                      optionList={[
                        { label: t('追加到现有密钥'), value: 'append' },
                        { label: t('覆盖现有密钥'), value: 'replace' },
                      ]}
                      style={{ width: '100%' }}
                      value={keyMode}
                      onChange={(value) => setKeyMode(value)}
                      extraText={
                        <Text type='tertiary' size='small'>
                          {keyMode === 'replace'
                            ? t('覆盖模式：将完全替换现有的所有密钥')
                            : t('追加模式：将新密钥添加到现有密钥列表末尾')}
                        </Text>
                      }
                    />
                  )}
                  {batch && multiToSingle && (
                    <>
                      <Form.Select
                        field='multi_key_mode'
                        label={t('密钥聚合模式')}
                        placeholder={t('请选择多密钥使用策略')}
                        optionList={[
                          { label: t('随机'), value: 'random' },
                          { label: t('轮询'), value: 'polling' },
                        ]}
                        style={{ width: '100%' }}
                        value={inputs.multi_key_mode || 'random'}
                        onChange={(value) => {
                          setMultiKeyMode(value);
                          handleInputChange('multi_key_mode', value);
                        }}
                      />
                      {inputs.multi_key_mode === 'polling' && (
                        <Banner
                          type='warning'
                          description={t(
                            '轮询模式必须搭配Redis和内存缓存功能使用，否则性能将大幅降低，并且无法实现轮询功能',
                          )}
                          className='!rounded-lg mt-2'
                        />
                      )}
                    </>
                  )}

                  {inputs.type === 18 && (
                    <Form.Input
                      field='other'
                      label={t('模型版本')}
                      placeholder={
                        '请输入星火大模型版本，注意是接口地址中的版本号，例如：v2.1'
                      }
                      onChange={(value) => handleInputChange('other', value)}
                      showClear
                    />
                  )}

                  {inputs.type === 41 && (
                    <JSONEditor
                      key={`region-${isEdit ? channelId : 'new'}`}
                      field='other'
                      label={t('部署地区')}
                      placeholder={t(
                        '请输入部署地区，例如：us-central1\n支持使用模型映射格式\n{\n    "default": "us-central1",\n    "claude-3-5-sonnet-20240620": "europe-west1"\n}',
                      )}
                      value={inputs.other || ''}
                      onChange={(value) => handleInputChange('other', value)}
                      rules={[{ required: true, message: t('请填写部署地区') }]}
                      template={REGION_EXAMPLE}
                      templateLabel={t('填入模板')}
                      editorType='region'
                      formApi={formApiRef.current}
                      extraText={t('设置默认地区和特定模型的专用地区')}
                    />
                  )}

                  {inputs.type === 21 && (
                    <Form.Input
                      field='other'
                      label={t('知识库 ID')}
                      placeholder={'请输入知识库 ID，例如：123456'}
                      onChange={(value) => handleInputChange('other', value)}
                      showClear
                    />
                  )}

                  {inputs.type === 39 && (
                    <Form.Input
                      field='other'
                      label='Account ID'
                      placeholder={
                        '请输入Account ID，例如：d6b5da8hk1awo8nap34ube6gh'
                      }
                      onChange={(value) => handleInputChange('other', value)}
                      showClear
                    />
                  )}

                  {inputs.type === 49 && (
                    <Form.Input
                      field='other'
                      label={t('智能体ID')}
                      placeholder={'请输入智能体ID，例如：7342866812345'}
                      onChange={(value) => handleInputChange('other', value)}
                      showClear
                    />
                  )}

                  {inputs.type === 1 && (
                    <Form.Input
                      field='openai_organization'
                      label={t('组织')}
                      placeholder={t('请输入组织org-xxx')}
                      showClear
                      helpText={t('组织，不填则为默认组织')}
                      onChange={(value) =>
                        handleInputChange('openai_organization', value)
                      }
                    />
                  )}
                </Card>

                {/* API Configuration Card */}
                {showApiConfigCard && (
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    {/* Header: API Config */}
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='green'
                        className='mr-2 shadow-md'
                      >
                        <IconGlobe size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('API 配置')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('API 地址和相关配置')}
                        </div>
                      </div>
                    </div>

                    {inputs.type === 40 && (
                      <Banner
                        type='info'
                        description={
                          <div>
                            <Text strong>{t('邀请链接')}:</Text>
                            <Text
                              link
                              underline
                              className='ml-2 cursor-pointer'
                              onClick={() =>
                                window.open(
                                  'https://cloud.siliconflow.cn/i/hij0YNTZ',
                                )
                              }
                            >
                              https://cloud.siliconflow.cn/i/hij0YNTZ
                            </Text>
                          </div>
                        }
                        className='!rounded-lg'
                      />
                    )}

                    {inputs.type === 3 && (
                      <>
                        <Banner
                          type='warning'
                          description={t(
                            '2025年5月10日后添加的渠道，不需要再在部署的时候移除模型名称中的"."',
                          )}
                          className='!rounded-lg'
                        />
                        <div>
                          <Form.Input
                            field='base_url'
                            label='AZURE_OPENAI_ENDPOINT'
                            placeholder={t(
                              '请输入 AZURE_OPENAI_ENDPOINT，例如：https://docs-test-001.openai.azure.com',
                            )}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            showClear
                          />
                        </div>
                        <div>
                          <Form.Input
                            field='other'
                            label={t('默认 API 版本')}
                            placeholder={t(
                              '请输入默认 API 版本，例如：2025-04-01-preview',
                            )}
                            onChange={(value) =>
                              handleInputChange('other', value)
                            }
                            showClear
                          />
                        </div>
                        <div>
                          <Form.Input
                            field='azure_responses_version'
                            label={t(
                              '默认 Responses API 版本，为空则使用上方版本',
                            )}
                            placeholder={t('例如：preview')}
                            onChange={(value) =>
                              handleChannelOtherSettingsChange(
                                'azure_responses_version',
                                value,
                              )
                            }
                            showClear
                          />
                        </div>
                      </>
                    )}

                    {inputs.type === 8 && (
                      <>
                        <Banner
                          type='warning'
                        description={t(
                          '如果你对接的是上游One API或者Transfer API等转发项目，请使用OpenAI类型，不要使用此类型，除非你知道你在做什么。',
                        )}
                        className='!rounded-lg'
                      />
                        <div>
                          <Form.Input
                            field='base_url'
                            label={t('完整的 Base URL，支持变量{model}')}
                            placeholder={t(
                              '请输入完整的URL，例如：https://api.openai.com/v1/chat/completions',
                            )}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            showClear
                          />
                        </div>
                      </>
                    )}

                    {inputs.type === 37 && (
                      <Banner
                        type='warning'
                        description={t(
                          'Dify渠道只适配chatflow和agent，并且agent不支持图片！',
                        )}
                        className='!rounded-lg'
                      />
                    )}

                    {inputs.type !== 3 &&
                      inputs.type !== 8 &&
                      inputs.type !== 22 &&
                      inputs.type !== 36 &&
                      inputs.type !== 45 && (
                        <div>
                          <Form.Input
                            field='base_url'
                            label={t('API地址')}
                            placeholder={t(
                              '此项可选，用于通过自定义API地址来进行 API 调用，末尾不要带/v1和/',
                            )}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            showClear
                            extraText={t(
                              '对于官方渠道，transfer-api已经内置地址，除非是第三方代理站点或者Azure的特殊接入地址，否则不需要填写',
                            )}
                          />
                        </div>
                      )}

                    {inputs.type === 22 && (
                      <div>
                        <Form.Input
                          field='base_url'
                          label={t('私有部署地址')}
                          placeholder={t(
                            '请输入私有部署地址，格式为：https://fastgpt.run/api/openapi',
                          )}
                          onChange={(value) =>
                            handleInputChange('base_url', value)
                          }
                          showClear
                        />
                      </div>
                    )}

                    {inputs.type === 36 && (
                      <div>
                        <Form.Input
                          field='base_url'
                          label={t(
                            '注意非Chat API，请务必填写正确的API地址，否则可能导致无法使用',
                          )}
                          placeholder={t(
                            '请输入到 /suno 前的路径，通常就是域名，例如：https://api.example.com',
                          )}
                          onChange={(value) =>
                            handleInputChange('base_url', value)
                          }
                          showClear
                        />
                      </div>
                    )}
                  </Card>
                )}

                {/* Model Configuration Card */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Model Config */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='purple'
                      className='mr-2 shadow-md'
                    >
                      <IconCode size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('模型配置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('模型选择和映射设置')}
                      </div>
                    </div>
                  </div>

                  <Form.Select
                    field='models'
                    label={t('模型')}
                    placeholder={t('请选择该渠道所支持的模型')}
                    rules={[{ required: true, message: t('请选择模型') }]}
                    multiple
                    filter={selectFilter}
                    autoClearSearchValue={false}
                    searchPosition='dropdown'
                    optionList={modelOptions}
                    style={{ width: '100%' }}
                    onChange={(value) => handleInputChange('models', value)}
                    renderSelectedItem={(optionNode) => {
                      const modelName = String(optionNode?.value ?? '');
                      return {
                        isRenderInTag: true,
                        content: (
                          <span
                            className='cursor-pointer select-none'
                            role='button'
                            tabIndex={0}
                            title={t('点击复制模型名称')}
                            onClick={async (e) => {
                              e.stopPropagation();
                              const ok = await copy(modelName);
                              if (ok) {
                                showSuccess(
                                  t('已复制：{{name}}', { name: modelName }),
                                );
                              } else {
                                showError(t('复制失败'));
                              }
                            }}
                          >
                            {optionNode.label || modelName}
                          </span>
                        ),
                      };
                    }}
                    extraText={
                      <Space wrap>
                        <Button
                          size='small'
                          type='primary'
                          onClick={() =>
                            handleInputChange('models', basicModels)
                          }
                        >
                          {t('填入相关模型')}
                        </Button>
                        <Button
                          size='small'
                          type='secondary'
                          onClick={() =>
                            handleInputChange('models', fullModels)
                          }
                        >
                          {t('填入所有模型')}
                        </Button>
                        <Button
                          size='small'
                          type='tertiary'
                          onClick={() => fetchUpstreamModelList('models')}
                        >
                          {t('获取模型列表')}
                        </Button>
                        <Button
                          size='small'
                          type='warning'
                          onClick={() => handleInputChange('models', [])}
                        >
                          {t('清除所有模型')}
                        </Button>
                        <Button
                          size='small'
                          type='tertiary'
                          onClick={() => {
                            if (inputs.models.length === 0) {
                              showInfo(t('没有模型可以复制'));
                              return;
                            }
                            try {
                              copy(inputs.models.join(','));
                              showSuccess(t('模型列表已复制到剪贴板'));
                            } catch (error) {
                              showError(t('复制失败'));
                            }
                          }}
                        >
                          {t('复制所有模型')}
                        </Button>
                        {modelGroups &&
                          modelGroups.length > 0 &&
                          modelGroups.map((group) => (
                            <Button
                              key={group.id}
                              size='small'
                              type='primary'
                              onClick={() => {
                                let items = [];
                                try {
                                  if (Array.isArray(group.items)) {
                                    items = group.items;
                                  } else if (typeof group.items === 'string') {
                                    const parsed = JSON.parse(
                                      group.items || '[]',
                                    );
                                    if (Array.isArray(parsed)) items = parsed;
                                  }
                                } catch {}
                                const current =
                                  formApiRef.current?.getValue('models') ||
                                  inputs.models ||
                                  [];
                                const merged = Array.from(
                                  new Set(
                                    [...current, ...items]
                                      .map((m) => (m || '').trim())
                                      .filter(Boolean),
                                  ),
                                );
                                handleInputChange('models', merged);
                              }}
                            >
                              {group.name}
                            </Button>
                          ))}
                      </Space>
                    }
                  />

                  <Form.Input
                    field='custom_model'
                    label={t('自定义模型名称')}
                    placeholder={t('输入自定义模型名称')}
                    onChange={(value) => setCustomModel(value.trim())}
                    value={customModel}
                    suffix={
                      <Button
                        size='small'
                        type='primary'
                        onClick={addCustomModels}
                      >
                        {t('填入')}
                      </Button>
                    }
                  />

                  {MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type) && (
                    <>
                      <Form.Switch
                        field='upstream_model_update_check_enabled'
                        label={t('是否检测上游模型更新')}
                        checkedText={t('开')}
                        uncheckedText={t('关')}
                        onChange={(value) =>
                          handleChannelOtherSettingsChange(
                            'upstream_model_update_check_enabled',
                            value,
                          )
                        }
                        extraText={t(
                          '开启后由后端定时任务检测该渠道上游模型变化',
                        )}
                      />
                      <div className='text-xs text-gray-500 mb-2'>
                        {t('上次检测时间')}:&nbsp;
                        {formatUnixTime(
                          inputs.upstream_model_update_last_check_time,
                        )}
                      </div>
                      <Form.Input
                        field='upstream_model_update_ignored_models'
                        label={t('已忽略模型')}
                        placeholder={t(
                          '例如：gpt-4.1-nano,regex:^claude-.*$,regex:^sora-.*$',
                        )}
                        extraText={t(
                          '支持精确匹配；使用 regex: 开头可按正则匹配。',
                        )}
                        onChange={(value) =>
                          handleInputChange(
                            'upstream_model_update_ignored_models',
                            value,
                          )
                        }
                        showClear
                      />
                    </>
                  )}

                  <Form.Input
                    field='test_model'
                    label={t('默认测试模型')}
                    placeholder={t('不填则为模型列表第一个')}
                    onChange={(value) => handleInputChange('test_model', value)}
                    showClear
                  />

                  <JSONEditor
                    key={`model_mapping-${isEdit ? channelId : 'new'}`}
                    field='model_mapping'
                    label={t('模型重定向')}
                    placeholder={
                      t(
                        '此项可选，用于修改请求体中的模型名称，为一个 JSON 字符串，键为请求中模型名称，值为要替换的模型名称，例如：',
                      ) + `\n${JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2)}`
                    }
                    value={inputs.model_mapping || ''}
                    onChange={(value) =>
                      handleInputChange('model_mapping', value)
                    }
                    template={MODEL_MAPPING_EXAMPLE}
                    templateLabel={t('填入模板')}
                    editorType='keyValue'
                    formApi={formApiRef.current}
                    extraText={t('键为请求中的模型名称，值为要替换的模型名称')}
                  />
                </Card>

                {/* Advanced Settings Card */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Advanced Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='orange'
                      className='mr-2 shadow-md'
                    >
                      <IconSetting size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('高级设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('渠道的高级配置选项')}
                      </div>
                    </div>
                  </div>

                  <Form.Select
                    field='group_ids'
                    label={t('分组')}
                    placeholder={t('请选择可以使用该渠道的分组')}
                    multiple
                    optionList={groupOptions}
                    extraText={t(
                      '仅支持选择已存在的分组（如需新增，请先到「分组管理」创建）',
                    )}
                    style={{ width: '100%' }}
                    onChange={(value) => handleInputChange('group_ids', value)}
                  />

                  <Form.Select
                    field='backup_group_ids'
                    label={t('备用分组')}
                    placeholder={t('可选：请选择该渠道失败后的备用分组')}
                    multiple
                    optionList={groupOptions}
                    extraText={t(
                      '仅在自动重试/自动切换时参与兜底，不影响首次按主分组选路，也不会覆盖当前计费分组',
                    )}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      handleInputChange('backup_group_ids', value)
                    }
                  />

                  <Form.Input
                    field='tag'
                    label={t('渠道标签')}
                    placeholder={t('渠道标签')}
                    showClear
                    onChange={(value) => handleInputChange('tag', value)}
                  />
                  <Form.TextArea
                    field='remark'
                    label={t('备注')}
                    placeholder={t('请输入备注（仅管理员可见）')}
                    maxLength={255}
                    showClear
                    onChange={(value) => handleInputChange('remark', value)}
                  />

                  <Row gutter={12}>
                    <Col span={12}>
                      <Form.InputNumber
                        field='priority'
                        label={t('渠道优先级')}
                        placeholder={t('渠道优先级')}
                        min={0}
                        onNumberChange={(value) =>
                          handleInputChange('priority', value)
                        }
                        style={{ width: '100%' }}
                      />
                    </Col>
                    <Col span={12}>
                      <Form.InputNumber
                        field='weight'
                        label={t('渠道权重')}
                        placeholder={t('渠道权重')}
                        min={0}
                        onNumberChange={(value) =>
                          handleInputChange('weight', value)
                        }
                        style={{ width: '100%' }}
                      />
                    </Col>
                  </Row>

                  <Form.InputNumber
                    field='max_concurrency'
                    label={t('渠道最大并行度')}
                    placeholder={t('默认 -1 表示不限制')}
                    min={-1}
                    onNumberChange={(value) =>
                      handleInputChange('max_concurrency', value)
                    }
                    extraText={t(
                      '默认 -1 不限制；设置为大于 0 的整数后，该渠道同一时刻最多只会承载这么多个请求。',
                    )}
                    style={{ width: '100%' }}
                  />

                  <Form.Slot
                    label={t('服务时间段优先级')}
                    extraText={t(
                      '在指定时间段内，用该时间段的优先级覆盖上方「渠道优先级」；其他时间段仍使用默认优先级。时间段按小时（含起止小时）：8-22 表示 08:00-23:00；支持跨午夜如 22-6 表示 22:00-07:00。',
                    )}
                  >
                    <div className='space-y-2'>
                      {serviceTimePriorityRules.length === 0 ? (
                        <Text type='tertiary' size='small'>
                          {t('未设置：全天使用默认「渠道优先级」')}
                        </Text>
                      ) : (
                        serviceTimePriorityRules.map((row, idx) => (
                          <div
                            key={`service-time-priority-${idx}`}
                            className='flex flex-wrap items-center gap-2'
                          >
                            <Input
                              value={row?.range || ''}
                              placeholder={t('例如 8-22')}
                              onChange={(v) =>
                                updateServiceTimePriorityRule(idx, {
                                  range: v,
                                })
                              }
                              showClear
                              style={{ width: 160 }}
                            />
                            <InputNumber
                              value={row?.priority ?? 0}
                              min={0}
                              precision={0}
                              placeholder={t('优先级')}
                              onChange={(v) =>
                                updateServiceTimePriorityRule(idx, {
                                  priority: v ?? 0,
                                })
                              }
                              style={{ width: 140 }}
                            />
                            <Button
                              icon={<IconPlus />}
                              theme='borderless'
                              onClick={() => addServiceTimePriorityRule(idx)}
                            />
                            <Button
                              icon={<IconDelete />}
                              type='danger'
                              theme='borderless'
                              onClick={() => removeServiceTimePriorityRule(idx)}
                            />
                          </div>
                        ))
                      )}
                      <Button
                        icon={<IconPlus />}
                        type='primary'
                        theme='outline'
                        onClick={() => addServiceTimePriorityRule(null)}
                      >
                        {t('添加时间段')}
                      </Button>
                    </div>
                  </Form.Slot>

                  <Form.Select
                    field='billing_mode'
                    label={t('计费模式')}
                    optionList={[
                      { label: t('额度计费'), value: 'quota' },
                      { label: t('按次计费'), value: 'request' },
                    ]}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      handleInputChange('billing_mode', value)
                    }
                    extraText={t(
                      '额度计费：按 token/额度口径统计；按次计费：按 /console/log 消费成功次数统计',
                    )}
                  />

                  {inputs.billing_mode === 'request' ? (
                    <Row gutter={12}>
                      <Col span={12}>
                        <Form.InputNumber
                          field='buy_requests_per_cny'
                          label={t('进价（¥1=N次）')}
                          placeholder={t('例如 50')}
                          min={1}
                          precision={0}
                          onNumberChange={(value) =>
                            handleInputChange('buy_requests_per_cny', value)
                          }
                          style={{ width: '100%' }}
                          extraText={t('用于计算渠道成本')}
                        />
                      </Col>
                      <Col span={12}>
                        <Form.InputNumber
                          field='sell_requests_per_cny'
                          label={t('售价（¥1=N次）')}
                          placeholder={t('例如 25')}
                          min={1}
                          precision={0}
                          onNumberChange={(value) =>
                            handleInputChange('sell_requests_per_cny', value)
                          }
                          style={{ width: '100%' }}
                          extraText={t('用于计算渠道销售额')}
                        />
                      </Col>
                    </Row>
                  ) : (
                  <Form.InputNumber
                    field='buy_cny_per_usd'
                    label={t('进价（￥/1$）')}
                    placeholder={t('例如 0.2')}
                    min={0}
                    precision={6}
                    onNumberChange={(value) =>
                      handleInputChange('buy_cny_per_usd', value)
                    }
                    style={{ width: '100%' }}
                    extraText={t('用于计算渠道成本与利润；填 0.2 表示 0.2￥/1$（1￥=5$）')}
                  />
                  )}

                  <Form.Switch
                    field='auto_ban'
                    label={t('是否自动禁用')}
                    checkedText={t('开')}
                    uncheckedText={t('关')}
                    onChange={(value) => setAutoBan(value)}
                    extraText={t(
                      '仅当自动禁用开启时有效，关闭后不会自动禁用该渠道',
                    )}
                    initValue={autoBan}
                  />

                  <Form.TextArea
                    field='param_override'
                    label={t('参数覆盖')}
                    placeholder={
                      t('此项可选，用于覆盖请求参数。不支持覆盖 stream 参数') +
                      '\n' +
                      t('旧格式（直接覆盖）：') +
                      '\n{\n  "temperature": 0,\n  "max_tokens": 1000\n}' +
                      '\n\n' +
                      t('新格式（支持条件判断与json自定义）：') +
                      '\n{\n  "operations": [\n    {\n      "path": "temperature",\n      "mode": "set",\n      "value": 0.7,\n      "conditions": [\n        {\n          "path": "model",\n          "mode": "prefix",\n          "value": "gpt"\n        }\n      ]\n    }\n  ]\n}'
                    }
                    autosize
                    onChange={(value) =>
                      handleInputChange('param_override', value)
                    }
                    extraText={
                      <div className='flex gap-2 flex-wrap'>
                        <Text
                          className='!text-semi-color-primary cursor-pointer'
                          onClick={() =>
                            handleInputChange(
                              'param_override',
                              JSON.stringify({ temperature: 0 }, null, 2),
                            )
                          }
                        >
                          {t('旧格式模板')}
                        </Text>
                        <Text
                          className='!text-semi-color-primary cursor-pointer'
                          onClick={() =>
                            handleInputChange(
                              'param_override',
                              JSON.stringify(
                                {
                                  operations: [
                                    {
                                      path: 'temperature',
                                      mode: 'set',
                                      value: 0.7,
                                      conditions: [
                                        {
                                          path: 'model',
                                          mode: 'prefix',
                                          value: 'gpt',
                                        },
                                      ],
                                      logic: 'AND',
                                    },
                                  ],
                                },
                                null,
                                2,
                              ),
                            )
                          }
                        >
                          {t('新格式模板')}
                        </Text>
                      </div>
                    }
                    showClear
                  />

                  <Form.TextArea
                    field='header_override'
                    label={t('请求头覆盖')}
                    placeholder={
                      t('此项可选，用于覆盖请求头参数') +
                      '\n' +
                      t('格式示例：') +
                      '\n{\n  "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 Edg/139.0.0.0"\n}'
                    }
                    autosize
                    onChange={(value) =>
                      handleInputChange('header_override', value)
                    }
                    extraText={
                      <div className='flex flex-col gap-2'>
                        <div className='flex gap-2 flex-wrap'>
                          <Text
                            className='!text-semi-color-primary cursor-pointer'
                            onClick={() =>
                              handleInputChange(
                                'header_override',
                                JSON.stringify(
                                  {
                                    'User-Agent':
                                      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 Edg/139.0.0.0',
                                  },
                                  null,
                                  2,
                                ),
                              )
                            }
                          >
                            {t('格式模板')}
                          </Text>
                        </div>
                        <div className='text-xs text-gray-600 flex flex-wrap gap-x-3 gap-y-1'>
                          <span>{t('可用变量：')}</span>
                          {headerOverrideVariables.map((item) => (
                            <span key={item.key} className='whitespace-nowrap'>
                              <span className='font-mono'>
                                {'${' + item.key + '}'}
                              </span>
                              <span className='ml-1'>{item.label}</span>
                            </span>
                          ))}
                        </div>
                      </div>
                    }
                    showClear
                  />

                  <JSONEditor
                    key={`status_code_mapping-${isEdit ? channelId : 'new'}`}
                    field='status_code_mapping'
                    label={t('状态码复写')}
                    placeholder={
                      t(
                        '此项可选，用于复写返回的状态码，仅影响本地判断，不修改返回到上游的状态码，比如将claude渠道的400错误复写为500（用于重试），请勿滥用该功能，例如：',
                      ) +
                      '\n' +
                      JSON.stringify(STATUS_CODE_MAPPING_EXAMPLE, null, 2)
                    }
                    value={inputs.status_code_mapping || ''}
                    onChange={(value) =>
                      handleInputChange('status_code_mapping', value)
                    }
                    template={STATUS_CODE_MAPPING_EXAMPLE}
                    templateLabel={t('填入模板')}
                    editorType='keyValue'
                    formApi={formApiRef.current}
                    extraText={t(
                      '键为原状态码，值为要复写的状态码，仅影响本地判断',
                    )}
                  />
                </Card>

                {/* Channel Extra Settings Card */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Channel Extra Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='violet'
                      className='mr-2 shadow-md'
                    >
                      <IconBolt size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('渠道额外设置')}
                      </Text>
                    </div>
                  </div>

                  {inputs.type === 1 && (
                    <Form.Switch
                      field='force_format'
                      label={t('强制格式化')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange('force_format', value)
                      }
                      extraText={t(
                        '强制将响应格式化为 OpenAI 标准格式（只适用于OpenAI渠道类型）',
                      )}
                    />
                  )}

                  <Form.Switch
                    field='thinking_to_content'
                    label={t('思考内容转换')}
                    checkedText={t('开')}
                    uncheckedText={t('关')}
                    onChange={(value) =>
                      handleChannelSettingsChange('thinking_to_content', value)
                    }
                    extraText={t(
                      '将 reasoning_content 转换为 <think> 标签拼接到内容中',
                    )}
                  />

                  <Form.Switch
                    field='pass_through_body_enabled'
                    label={t('透传请求体')}
                    checkedText={t('开')}
                    uncheckedText={t('关')}
                    onChange={(value) =>
                      handleChannelSettingsChange(
                        'pass_through_body_enabled',
                        value,
                      )
                    }
                    extraText={t('启用请求体透传功能')}
                  />

                  {(inputs.type === 1 ||
                    inputs.type === 3 ||
                    inputs.type === 8 ||
                    inputs.type === 20 ||
                    inputs.type === 47) && (
                    <Form.Switch
                      field='messages_to_responses_compat'
                      label={t('Messages 转 Responses 兼容')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange(
                          'messages_to_responses_compat',
                          value,
                        )
                      }
                      extraText={t(
                        '将 /v1/messages 内部转换为上游 /v1/responses；若渠道 models 未声明请求模型，请同时配置模型重定向',
                      )}
                    />
                  )}

                  <Form.Switch
                    field='test_enable_max_tokens'
                    label={t('渠道测试使用输出上限')}
                    checkedText={t('开')}
                    uncheckedText={t('关')}
                    onChange={(value) =>
                      handleChannelSettingsChange(
                        'test_enable_max_tokens',
                        value,
                      )
                    }
                    extraText={t('控制渠道【测试】按钮是否附加 max_tokens/max_completion_tokens')}
                  />

                  {MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type) && (
                    <Form.Switch
                      field='upstream_model_update_auto_sync_enabled'
                      label={t('自动同步上游新增模型')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      disabled={!inputs.upstream_model_update_check_enabled}
                      onChange={(value) =>
                        handleChannelOtherSettingsChange(
                          'upstream_model_update_auto_sync_enabled',
                          value,
                        )
                      }
                      extraText={t(
                        '开启后，定时检测到的新增模型会自动并入当前渠道模型列表',
                      )}
                    />
                  )}

                  <Form.Input
                    field='proxy'
                    label={t('代理地址')}
                    placeholder={t('例如: socks5://user:pass@host:port')}
                    onChange={(value) =>
                      handleChannelSettingsChange('proxy', value)
                    }
                    showClear
                    suffix={
                      <Button
                        size='small'
                        theme='solid'
                        icon={<IconGlobe />}
                        loading={proxyTesting}
                        disabled={!channelSettings?.proxy}
                        onClick={handleTestProxy}
                      >
                        {t('测试')}
                      </Button>
                    }
                    extraText={t('用于配置网络代理，支持 socks5 协议')}
                  />

                  <Form.TextArea
                    field='system_prompt'
                    label={t('系统提示词')}
                    placeholder={t(
                      '输入系统提示词，用户的系统提示词将优先于此设置',
                    )}
                    onChange={(value) =>
                      handleChannelSettingsChange('system_prompt', value)
                    }
                    autosize
                    showClear
                    extraText={t(
                      '用户优先：如果用户在请求中指定了系统提示词，将优先使用用户的设置',
                    )}
                  />
                  <Form.Switch
                    field='system_prompt_override'
                    label={t('系统提示词拼接')}
                    checkedText={t('开')}
                    uncheckedText={t('关')}
                    onChange={(value) =>
                      handleChannelSettingsChange(
                        'system_prompt_override',
                        value,
                      )
                    }
                    extraText={t(
                      '如果用户请求中包含系统提示词，则使用此设置拼接到用户的系统提示词前面',
                    )}
                  />
                </Card>
              </div>
            </Spin>
          )}
        </Form>
        <ImagePreview
          src={modalImageUrl}
          visible={isModalOpenurl}
          onVisibleChange={(visible) => setIsModalOpenurl(visible)}
        />
      </SideSheet>
      {/* 使用TwoFactorAuthModal组件进行2FA验证 */}
      <TwoFactorAuthModal
        visible={show2FAVerifyModal}
        code={verifyCode}
        loading={verifyLoading}
        onCodeChange={setVerifyCode}
        onVerify={handleVerify2FA}
        onCancel={reset2FAVerifyState}
        title={t('查看渠道密钥')}
        description={t('为了保护账户安全，请验证您的两步验证码。')}
        placeholder={t('请输入验证码或备用码')}
      />

      {/* 使用ChannelKeyDisplay组件显示密钥 */}
      <Modal
        title={
          <div className='flex items-center'>
            <div className='w-8 h-8 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center mr-3'>
              <svg
                className='w-4 h-4 text-green-600 dark:text-green-400'
                fill='currentColor'
                viewBox='0 0 20 20'
              >
                <path
                  fillRule='evenodd'
                  d='M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z'
                  clipRule='evenodd'
                />
              </svg>
            </div>
            {t('渠道密钥信息')}
          </div>
        }
        visible={twoFAState.showModal && twoFAState.showKey}
        onCancel={resetTwoFAState}
        footer={
          <Button type='primary' onClick={resetTwoFAState}>
            {t('完成')}
          </Button>
        }
        width={700}
        style={{ maxWidth: '90vw' }}
      >
        <ChannelKeyDisplay
          keyData={twoFAState.keyData}
          showSuccessIcon={true}
          successText={t('密钥获取成功')}
          showWarning={true}
          warningText={t(
            '请妥善保管密钥信息，不要泄露给他人。如有安全疑虑，请及时更换密钥。',
          )}
        />
      </Modal>

      <ModelSelectModal
        visible={modelModalVisible}
        models={fetchedModels}
        selected={inputs.models}
        onConfirm={(selectedModels) => {
          handleInputChange('models', selectedModels);
          showSuccess(t('模型列表已更新'));
          setModelModalVisible(false);
        }}
        onCancel={() => setModelModalVisible(false)}
      />
    </>
  );
};

export default EditChannelModal;
