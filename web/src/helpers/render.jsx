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

import i18next from 'i18next';
import { Modal, Tag, Typography, Avatar } from '@douyinfe/semi-ui';
import { copy, showSuccess } from './utils';
import { MOBILE_BREAKPOINT } from '../hooks/common/useIsMobile';
import { visit } from 'unist-util-visit';
import * as LobeIcons from '@lobehub/icons';
import RatioTag from '../components/common/ui/RatioTag';
import {
  OpenAI,
  Claude,
  Gemini,
  Moonshot,
  Zhipu,
  Qwen,
  DeepSeek,
  Minimax,
  Wenxin,
  Spark,
  Midjourney,
  Hunyuan,
  Cohere,
  Cloudflare,
  Ai360,
  Yi,
  Jina,
  Mistral,
  XAI,
  Ollama,
  Doubao,
  Suno,
  Xinference,
  OpenRouter,
  Dify,
  Coze,
  SiliconCloud,
  FastGPT,
  Kling,
  Jimeng,
} from '@lobehub/icons';

import {
  LayoutGrid,
  Sparkles,
  MessagesSquare,
  KeyRound,
  Activity,
  Image as ImageIcon,
  CheckCircle2,
  WalletCards,
  Network,
  Ticket,
  Gift,
  HelpCircle,
  UserRound,
  SlidersHorizontal,
  CircleUserRound,
  Boxes,
  Package,
  Crown,
  ShoppingCart,
  Server,
} from 'lucide-react';

// 获取侧边栏Lucide图标组件
export function getLucideIcon(key, selected = false) {
  const size = 18;
  const strokeWidth = 1.8;
  const SELECTED_COLOR = 'var(--semi-color-primary)';
  const iconColor = selected ? SELECTED_COLOR : 'currentColor';
  const commonProps = {
    size,
    strokeWidth,
    className: `transition-colors duration-200 ${selected ? 'transition-transform duration-200 scale-105' : ''}`,
  };

  // 根据不同的key返回不同的图标
  switch (key) {
    case 'detail':
      return <LayoutGrid {...commonProps} color={iconColor} />;
    case 'playground':
      return <Sparkles {...commonProps} color={iconColor} />;
    case 'chat':
      return <MessagesSquare {...commonProps} color={iconColor} />;
    case 'token':
      return <KeyRound {...commonProps} color={iconColor} />;
    case 'log':
      return <Activity {...commonProps} color={iconColor} />;
    case 'stomp_king':
      return <Crown {...commonProps} color={iconColor} />;
    case 'midjourney':
      return <ImageIcon {...commonProps} color={iconColor} />;
    case 'task':
      return <CheckCircle2 {...commonProps} color={iconColor} />;
    case 'subscription':
      return <ShoppingCart {...commonProps} color={iconColor} />;
    case 'my_subscription':
      return <WalletCards {...commonProps} color={iconColor} />;
    case 'invitation':
      return <Gift {...commonProps} color={iconColor} />;
    case 'topup':
      return <WalletCards {...commonProps} color={iconColor} />;
    case 'order':
      return <WalletCards {...commonProps} color={iconColor} />;
    case 'channel':
      return <Network {...commonProps} color={iconColor} />;
    case 'redemption':
      return <Ticket {...commonProps} color={iconColor} />;
    case 'product_management':
      return <Package {...commonProps} color={iconColor} />;
    case 'user':
    case 'personal':
      return <UserRound {...commonProps} color={iconColor} />;
    case 'models':
      return <Boxes {...commonProps} color={iconColor} />;
    case 'pricing':
      return <Boxes {...commonProps} color={iconColor} />;
    case 'service_status':
      return <Server {...commonProps} color={iconColor} />;
    case 'setting':
      return <SlidersHorizontal {...commonProps} color={iconColor} />;
    case 'faq':
      return <HelpCircle {...commonProps} color={iconColor} />;
    default:
      return <CircleUserRound {...commonProps} color={iconColor} />;
  }
}

// 获取模型分类
export const getModelCategories = (() => {
  let categoriesCache = null;
  let lastLocale = null;

  return (t) => {
    const currentLocale = i18next.language;
    if (categoriesCache && lastLocale === currentLocale) {
      return categoriesCache;
    }

    categoriesCache = {
      all: {
        label: t('全部模型'),
        icon: null,
        filter: () => true,
      },
      openai: {
        label: 'OpenAI',
        icon: <OpenAI />,
        filter: (model) =>
          model.model_name.toLowerCase().includes('gpt') ||
          model.model_name.toLowerCase().includes('dall-e') ||
          model.model_name.toLowerCase().includes('whisper') ||
          model.model_name.toLowerCase().includes('tts') ||
          model.model_name.toLowerCase().includes('text-') ||
          model.model_name.toLowerCase().includes('babbage') ||
          model.model_name.toLowerCase().includes('davinci') ||
          model.model_name.toLowerCase().includes('curie') ||
          model.model_name.toLowerCase().includes('ada') ||
          model.model_name.toLowerCase().includes('o1') ||
          model.model_name.toLowerCase().includes('o3') ||
          model.model_name.toLowerCase().includes('o4'),
      },
      anthropic: {
        label: 'Anthropic',
        icon: <Claude.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('claude'),
      },
      gemini: {
        label: 'Gemini',
        icon: <Gemini.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('gemini'),
      },
      moonshot: {
        label: 'Moonshot',
        icon: <Moonshot />,
        filter: (model) => model.model_name.toLowerCase().includes('moonshot'),
      },
      zhipu: {
        label: t('智谱'),
        icon: <Zhipu.Color />,
        filter: (model) =>
          model.model_name.toLowerCase().includes('chatglm') ||
          model.model_name.toLowerCase().includes('glm-'),
      },
      qwen: {
        label: t('通义千问'),
        icon: <Qwen.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('qwen'),
      },
      deepseek: {
        label: 'DeepSeek',
        icon: <DeepSeek.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('deepseek'),
      },
      minimax: {
        label: 'MiniMax',
        icon: <Minimax.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('abab'),
      },
      baidu: {
        label: t('文心一言'),
        icon: <Wenxin.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('ernie'),
      },
      xunfei: {
        label: t('讯飞星火'),
        icon: <Spark.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('spark'),
      },
      midjourney: {
        label: 'Midjourney',
        icon: <Midjourney />,
        filter: (model) => model.model_name.toLowerCase().includes('mj_'),
      },
      tencent: {
        label: t('腾讯混元'),
        icon: <Hunyuan.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('hunyuan'),
      },
      cohere: {
        label: 'Cohere',
        icon: <Cohere.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('command'),
      },
      cloudflare: {
        label: 'Cloudflare',
        icon: <Cloudflare.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('@cf/'),
      },
      ai360: {
        label: t('360智脑'),
        icon: <Ai360.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('360'),
      },
      yi: {
        label: t('零一万物'),
        icon: <Yi.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('yi'),
      },
      jina: {
        label: 'Jina',
        icon: <Jina />,
        filter: (model) => model.model_name.toLowerCase().includes('jina'),
      },
      mistral: {
        label: 'Mistral AI',
        icon: <Mistral.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('mistral'),
      },
      xai: {
        label: 'xAI',
        icon: <XAI />,
        filter: (model) => model.model_name.toLowerCase().includes('grok'),
      },
      llama: {
        label: 'Llama',
        icon: <Ollama />,
        filter: (model) => model.model_name.toLowerCase().includes('llama'),
      },
      doubao: {
        label: t('豆包'),
        icon: <Doubao.Color />,
        filter: (model) => model.model_name.toLowerCase().includes('doubao'),
      },
    };

    lastLocale = currentLocale;
    return categoriesCache;
  };
})();

/**
 * 根据渠道类型返回对应的厂商图标
 * @param {number} channelType - 渠道类型值
 * @returns {JSX.Element|null} - 对应的厂商图标组件
 */
export function getChannelIcon(channelType) {
  const iconSize = 14;

  switch (channelType) {
    case 1: // OpenAI
    case 3: // Azure OpenAI
      return <OpenAI size={iconSize} />;
    case 2: // Midjourney Proxy
    case 5: // Midjourney Proxy Plus
      return <Midjourney size={iconSize} />;
    case 36: // Suno API
      return <Suno size={iconSize} />;
    case 4: // Ollama
      return <Ollama size={iconSize} />;
    case 14: // Anthropic Claude
    case 33: // AWS Claude
      return <Claude.Color size={iconSize} />;
    case 41: // Vertex AI
      return <Gemini.Color size={iconSize} />;
    case 34: // Cohere
      return <Cohere.Color size={iconSize} />;
    case 39: // Cloudflare
      return <Cloudflare.Color size={iconSize} />;
    case 43: // DeepSeek
      return <DeepSeek.Color size={iconSize} />;
    case 15: // 百度文心千帆
    case 46: // 百度文心千帆V2
      return <Wenxin.Color size={iconSize} />;
    case 17: // 阿里通义千问
      return <Qwen.Color size={iconSize} />;
    case 18: // 讯飞星火认知
      return <Spark.Color size={iconSize} />;
    case 16: // 智谱 ChatGLM
    case 26: // 智谱 GLM-4V
      return <Zhipu.Color size={iconSize} />;
    case 24: // Google Gemini
    case 11: // Google PaLM2
      return <Gemini.Color size={iconSize} />;
    case 47: // Xinference
      return <Xinference.Color size={iconSize} />;
    case 25: // Moonshot
      return <Moonshot size={iconSize} />;
    case 20: // OpenRouter
      return <OpenRouter size={iconSize} />;
    case 19: // 360 智脑
      return <Ai360.Color size={iconSize} />;
    case 23: // 腾讯混元
      return <Hunyuan.Color size={iconSize} />;
    case 31: // 零一万物
      return <Yi.Color size={iconSize} />;
    case 35: // MiniMax
      return <Minimax.Color size={iconSize} />;
    case 37: // Dify
      return <Dify.Color size={iconSize} />;
    case 38: // Jina
      return <Jina size={iconSize} />;
    case 40: // SiliconCloud
      return <SiliconCloud.Color size={iconSize} />;
    case 42: // Mistral AI
      return <Mistral.Color size={iconSize} />;
    case 45: // 字节火山方舟、豆包通用
      return <Doubao.Color size={iconSize} />;
    case 48: // xAI
      return <XAI size={iconSize} />;
    case 49: // Coze
      return <Coze size={iconSize} />;
    case 50: // 可灵 Kling
      return <Kling.Color size={iconSize} />;
    case 51: // 即梦 Jimeng
      return <Jimeng.Color size={iconSize} />;
    case 8: // 自定义渠道
    case 22: // 知识库：FastGPT
      return <FastGPT.Color size={iconSize} />;
    case 21: // 知识库：AI Proxy
    case 44: // 嵌入模型：MokaAI M3E
    default:
      return null; // 未知类型或自定义渠道不显示图标
  }
}

/**
 * 根据图标名称动态获取 LobeHub 图标组件
 * 支持：
 * - 基础："OpenAI"、"OpenAI.Color" 等
 * - 额外属性（点号链式）："OpenAI.Avatar.type={'platform'}"、"OpenRouter.Avatar.shape={'square'}"
 * - 继续兼容第二参数 size；若字符串里有 size=，以字符串为准
 * @param {string} iconName - 图标名称/描述
 * @param {number} size - 图标大小，默认为 14
 * @returns {JSX.Element} - 对应的图标组件或 Avatar
 */
export function getLobeHubIcon(iconName, size = 14) {
  if (typeof iconName === 'string') iconName = iconName.trim();
  // 如果没有图标名称，返回 Avatar
  if (!iconName) {
    return <Avatar size='extra-extra-small'>?</Avatar>;
  }

  // 解析组件路径与点号链式属性
  const segments = String(iconName).split('.');
  const baseKey = segments[0];
  const BaseIcon = LobeIcons[baseKey];

  let IconComponent = undefined;
  let propStartIndex = 1;

  if (BaseIcon && segments.length > 1 && BaseIcon[segments[1]]) {
    IconComponent = BaseIcon[segments[1]];
    propStartIndex = 2;
  } else {
    IconComponent = LobeIcons[baseKey];
    propStartIndex = 1;
  }

  // 失败兜底
  if (
    !IconComponent ||
    (typeof IconComponent !== 'function' && typeof IconComponent !== 'object')
  ) {
    const firstLetter = String(iconName).charAt(0).toUpperCase();
    return <Avatar size='extra-extra-small'>{firstLetter}</Avatar>;
  }

  // 解析点号链式属性，形如：key={...}、key='...'、key="..."、key=123、key、key=true/false
  const props = {};

  const parseValue = (raw) => {
    if (raw == null) return true;
    let v = String(raw).trim();
    // 去除一层花括号包裹
    if (v.startsWith('{') && v.endsWith('}')) {
      v = v.slice(1, -1).trim();
    }
    // 去除引号
    if (
      (v.startsWith('"') && v.endsWith('"')) ||
      (v.startsWith("'") && v.endsWith("'"))
    ) {
      return v.slice(1, -1);
    }
    // 布尔
    if (v === 'true') return true;
    if (v === 'false') return false;
    // 数字
    if (/^-?\d+(?:\.\d+)?$/.test(v)) return Number(v);
    // 其他原样返回字符串
    return v;
  };

  for (let i = propStartIndex; i < segments.length; i++) {
    const seg = segments[i];
    if (!seg) continue;
    const eqIdx = seg.indexOf('=');
    if (eqIdx === -1) {
      props[seg.trim()] = true;
      continue;
    }
    const key = seg.slice(0, eqIdx).trim();
    const valRaw = seg.slice(eqIdx + 1).trim();
    props[key] = parseValue(valRaw);
  }

  // 兼容第二参数 size，若字符串中未显式指定 size，则使用函数入参
  if (props.size == null && size != null) props.size = size;

  return <IconComponent {...props} />;
}

// 颜色列表
const colors = [
  'amber',
  'blue',
  'cyan',
  'green',
  'grey',
  'indigo',
  'light-blue',
  'lime',
  'orange',
  'pink',
  'purple',
  'red',
  'teal',
  'violet',
  'yellow',
];

// 基础10色色板 (N ≤ 10)
const baseColors = [
  '#1664FF',
  '#3B82F6',
  '#0EA5E9',
  '#1AC6FF',
  '#22D3EE',
  '#06B6D4',
  '#14B8A6',
  '#2DD4BF',
  '#10B981',
  '#22C55E',
];

// 扩展20色色板 (10 < N ≤ 20)
const extendedColors = [
  '#1664FF',
  '#2F54EB',
  '#3B82F6',
  '#60A5FA',
  '#0EA5E9',
  '#38BDF8',
  '#1AC6FF',
  '#22D3EE',
  '#06B6D4',
  '#0891B2',
  '#13C2C2',
  '#14B8A6',
  '#2DD4BF',
  '#34D399',
  '#10B981',
  '#059669',
  '#3CC780',
  '#22C55E',
  '#4ADE80',
  '#A3E635',
];

// 模型颜色映射
export const modelColorMap = {
  // GPT-5
  'gpt-5': '#1664FF',
  'gpt-5-2025-08-07': '#3B82F6',
  'gpt-5-chat-latest': '#1AC6FF',
  'gpt-5-high': '#0EA5E9',
  'gpt-5-mini': '#3CC780',
  'gpt-5-mini-2025-08-07': '#22C55E',
  'gpt-5-nano': '#14B8A6',
  'gpt-5-nano-2025-08-07': '#2DD4BF',
  'gpt-5-codex': '#06B6D4',
  'gpt-5.1': '#60A5FA',
  'gpt-5.1-codex': '#13C2C2',
  'gpt-5.1-codex-mini': '#0891B2',
  'gpt-5.1-codex-max': '#22D3EE',
  'gpt-5.2': '#A3E635',
  'gpt-5.2-codex': '#10B981',

  // GPT-4
  'gpt-4': '#2F54EB',
  'gpt-4-0613': '#60A5FA',
  'gpt-4-1106-preview': '#1664FF',
  'gpt-4-0125-preview': '#38BDF8',
  'gpt-4-turbo-preview': '#22D3EE',
  'gpt-4-32k': '#14B8A6',
  'gpt-4-32k-0613': '#2DD4BF',
  'gpt-4-all': '#3CC780',
  'gpt-4-gizmo-*': '#06B6D4',
  'gpt-4-vision-preview': '#10B981',

  // GPT-3.5
  'gpt-3.5-turbo': '#22C55E',
  'gpt-3.5-turbo-0613': '#4ADE80',
  'gpt-3.5-turbo-1106': '#14B8A6',
  'gpt-3.5-turbo-16k': '#0EA5E9',
  'gpt-3.5-turbo-16k-0613': '#38BDF8',
  'gpt-3.5-turbo-instruct': '#1AC6FF',

  // OpenAI - Images/Speech/Text
  'dall-e': '#22D3EE',
  'dall-e-3': '#06B6D4',
  'whisper-1': '#10B981',
  'tts-1': '#38BDF8',
  'tts-1-1106': '#1AC6FF',
  'tts-1-hd': '#60A5FA',
  'tts-1-hd-1106': '#3B82F6',
  'text-ada-001': '#2DD4BF',
  'text-babbage-001': '#14B8A6',
  'text-curie-001': '#0EA5E9',
  'text-davinci-003': '#1664FF',
  'text-davinci-edit-001': '#38BDF8',
  'text-embedding-ada-002': '#22C55E',
  'text-embedding-v1': '#3CC780',
  'text-moderation-latest': '#06B6D4',
  'text-moderation-stable': '#0891B2',

  // Anthropic
  'claude-3-opus-20240229': '#14B8A6',
  'claude-3-sonnet-20240229': '#2DD4BF',
  'claude-3-haiku-20240307': '#34D399',
  'claude-2.1': '#A3E635',
};

export function modelToColor(modelName) {
  // 1. 如果模型在预定义的 modelColorMap 中，使用预定义颜色
  if (modelColorMap[modelName]) {
    return modelColorMap[modelName];
  }

  // 2. 生成一个稳定的数字作为索引
  let hash = 0;
  for (let i = 0; i < modelName.length; i++) {
    hash = (hash << 5) - hash + modelName.charCodeAt(i);
    hash = hash & hash; // Convert to 32-bit integer
  }
  hash = Math.abs(hash);

  // 3. 根据模型名称长度选择不同的色板
  const colorPalette = modelName.length > 10 ? extendedColors : baseColors;

  // 4. 使用hash值选择颜色
  const index = hash % colorPalette.length;
  return colorPalette[index];
}

export function stringToColor(str) {
  let sum = 0;
  for (let i = 0; i < str.length; i++) {
    sum += str.charCodeAt(i);
  }
  let i = sum % colors.length;
  return colors[i];
}

// 渲染带有模型图标的标签
export function renderModelTag(modelName, options = {}) {
  const {
    color,
    size = 'default',
    shape = 'circle',
    onClick,
    suffixIcon,
  } = options;

  const categories = getModelCategories(i18next.t);
  let icon = null;

  for (const [key, category] of Object.entries(categories)) {
    if (key !== 'all' && category.filter({ model_name: modelName })) {
      icon = category.icon;
      break;
    }
  }

  return (
    <Tag
      color={color || stringToColor(modelName)}
      prefixIcon={icon}
      suffixIcon={suffixIcon}
      size={size}
      shape={shape}
      onClick={onClick}
    >
      {modelName}
    </Tag>
  );
}

export function renderText(text, limit) {
  if (text.length > limit) {
    return text.slice(0, limit - 3) + '...';
  }
  return text;
}

/**
 * Render group tags based on the input group string
 * @param {string} group - The input group string
 * @returns {JSX.Element} - The rendered group tags
 */
export function renderGroup(group) {
  if (group === '') {
    return (
      <Tag key='default' color='white' shape='circle'>
        {i18next.t('用户分组')}
      </Tag>
    );
  }

  const tagColors = {
    vip: 'yellow',
    pro: 'yellow',
    svip: 'red',
    premium: 'red',
  };

  const groups = group.split(',').sort();

  return (
    <span key={group}>
      {groups.map((group) => (
        <Tag
          color={tagColors[group] || stringToColor(group)}
          key={group}
          shape='circle'
          onClick={async (event) => {
            event.stopPropagation();
            if (await copy(group)) {
              showSuccess(i18next.t('已复制：') + group);
            } else {
              Modal.error({
                title: i18next.t('无法复制到剪贴板，请手动复制'),
                content: group,
              });
            }
          }}
        >
          {group}
        </Tag>
      ))}
    </span>
  );
}

export function renderRatio(ratio) {
  return <RatioTag value={ratio} />;
}

const measureTextWidth = (
  text,
  style = {
    fontSize: '14px',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  },
  containerWidth,
) => {
  const span = document.createElement('span');

  span.style.visibility = 'hidden';
  span.style.position = 'absolute';
  span.style.whiteSpace = 'nowrap';
  span.style.fontSize = style.fontSize;
  span.style.fontFamily = style.fontFamily;

  span.textContent = text;

  document.body.appendChild(span);
  const width = span.offsetWidth;

  document.body.removeChild(span);

  return width;
};

export function truncateText(text, maxWidth = 200) {
  const isMobileScreen = window.matchMedia(
    `(max-width: ${MOBILE_BREAKPOINT - 1}px)`,
  ).matches;
  if (!isMobileScreen) {
    return text;
  }
  if (!text) return text;

  try {
    // Handle percentage-based maxWidth
    let actualMaxWidth = maxWidth;
    if (typeof maxWidth === 'string' && maxWidth.endsWith('%')) {
      const percentage = parseFloat(maxWidth) / 100;
      // Use window width as fallback container width
      actualMaxWidth = window.innerWidth * percentage;
    }

    const width = measureTextWidth(text);
    if (width <= actualMaxWidth) return text;

    let left = 0;
    let right = text.length;
    let result = text;

    while (left <= right) {
      const mid = Math.floor((left + right) / 2);
      const truncated = text.slice(0, mid) + '...';
      const currentWidth = measureTextWidth(truncated);

      if (currentWidth <= actualMaxWidth) {
        result = truncated;
        left = mid + 1;
      } else {
        right = mid - 1;
      }
    }

    return result;
  } catch (error) {
    console.warn(
      'Text measurement failed, falling back to character count',
      error,
    );
    if (text.length > 20) {
      return text.slice(0, 17) + '...';
    }
    return text;
  }
}

export const renderGroupOption = (item) => {
  const {
    disabled,
    selected,
    label,
    focused,
    className,
    style,
    onMouseEnter,
    onClick,
    empty,
    emptyContent,
    ...rest
  } = item;

  const baseStyle = {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '8px 16px',
    cursor: disabled ? 'not-allowed' : 'pointer',
    backgroundColor: focused ? 'var(--semi-color-fill-0)' : 'transparent',
    opacity: disabled ? 0.5 : 1,
    ...(selected && {
      backgroundColor: 'var(--semi-color-primary-light-default)',
    }),
    '&:hover': {
      backgroundColor: !disabled && 'var(--semi-color-fill-1)',
    },
  };

  const handleClick = () => {
    if (!disabled && onClick) {
      onClick();
    }
  };

  const handleMouseEnter = (e) => {
    if (!disabled && onMouseEnter) {
      onMouseEnter(e);
    }
  };

  return (
    <div
      style={baseStyle}
      onClick={handleClick}
      onMouseEnter={handleMouseEnter}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
        <Typography.Text strong type={disabled ? 'tertiary' : undefined}>
          {label}
        </Typography.Text>
        {(() => {
          const primary =
            typeof label === 'string' ? String(label || '').trim() : '';
          const secondaryRaw = String(
            item?.desc ?? item?.fullLabel ?? '',
          ).trim();
          const secondary =
            secondaryRaw && secondaryRaw !== primary ? secondaryRaw : '';
          if (!secondary) return null;
          return (
            <Typography.Text type='secondary' size='small'>
              {secondary}
            </Typography.Text>
          );
        })()}
      </div>
      {item.ratio && renderRatio(item.ratio)}
    </div>
  );
};

export function renderNumber(num) {
  if (num >= 1000000000) {
    return (num / 1000000000).toFixed(1) + 'B';
  } else if (num >= 1000000) {
    return (num / 1000000).toFixed(1) + 'M';
  } else if (num >= 10000) {
    return (num / 1000).toFixed(1) + 'k';
  } else {
    return num;
  }
}

export function renderQuotaNumberWithDigit(num, digits = 2) {
  if (typeof num !== 'number' || isNaN(num)) {
    return 0;
  }
  let displayInCurrency = localStorage.getItem('display_in_currency');
  num = num.toFixed(digits);
  if (displayInCurrency) {
    return '$' + num;
  }
  return num;
}

export function renderNumberWithPoint(num) {
  if (num === undefined) return '';
  num = num.toFixed(2);
  if (num >= 100000) {
    // Convert number to string to manipulate it
    let numStr = num.toString();
    // Find the position of the decimal point
    let decimalPointIndex = numStr.indexOf('.');

    let wholePart = numStr;
    let decimalPart = '';

    // If there is a decimal point, split the number into whole and decimal parts
    if (decimalPointIndex !== -1) {
      wholePart = numStr.slice(0, decimalPointIndex);
      decimalPart = numStr.slice(decimalPointIndex);
    }

    // Take the first two and last two digits of the whole number part
    let shortenedWholePart = wholePart.slice(0, 2) + '..' + wholePart.slice(-2);

    // Return the formatted number
    return shortenedWholePart + decimalPart;
  }

  // If the number is less than 100,000, return it unmodified
  return num;
}

export function getQuotaPerUnit() {
  let quotaPerUnit = localStorage.getItem('quota_per_unit');
  quotaPerUnit = parseFloat(quotaPerUnit);
  return quotaPerUnit;
}

const DEFAULT_QUOTA_PER_UNIT = 500000;

function getUsdPerMillionTokens() {
  const quotaPerUnit = getQuotaPerUnit();
  if (!Number.isFinite(quotaPerUnit) || quotaPerUnit <= 0) {
    return 1000000 / DEFAULT_QUOTA_PER_UNIT;
  }
  return 1000000 / quotaPerUnit;
}

function getRatioUsdPerMillionTokens(modelRatio) {
  return modelRatio * getUsdPerMillionTokens();
}

function normalizeLongContextModelName(modelName) {
  return String(modelName || '')
    .trim()
    .toLowerCase();
}

function getLongContextPricing(modelName, inputTokens, cacheTokens) {
  const normalizedModelName = normalizeLongContextModelName(modelName);
  const totalInputTokens =
    Math.max(Number(inputTokens) || 0, 0) +
    Math.max(Number(cacheTokens) || 0, 0);
  if (normalizedModelName !== 'gpt-5.4' || totalInputTokens <= 272000) {
    return {
      inputMultiplier: 1,
      outputMultiplier: 1,
      enabled: false,
    };
  }
  return {
    inputMultiplier: 2,
    outputMultiplier: 1.5,
    enabled: true,
  };
}

export function renderUnitWithQuota(quota) {
  let quotaPerUnit = localStorage.getItem('quota_per_unit');
  quotaPerUnit = parseFloat(quotaPerUnit);
  quota = parseFloat(quota);
  return quotaPerUnit * quota;
}

export function getQuotaWithUnit(quota, digits = 6) {
  let quotaPerUnit = localStorage.getItem('quota_per_unit');
  quotaPerUnit = parseFloat(quotaPerUnit);
  return (quota / quotaPerUnit).toFixed(digits);
}

export function renderQuotaWithAmount(amount) {
  let displayInCurrency = localStorage.getItem('display_in_currency');
  displayInCurrency = displayInCurrency === 'true';
  if (displayInCurrency) {
    return '$' + amount;
  } else {
    return renderNumber(renderUnitWithQuota(amount));
  }
}

export function renderQuota(quota, digits = 2) {
  let quotaPerUnit = localStorage.getItem('quota_per_unit');
  let displayInCurrency = localStorage.getItem('display_in_currency');
  quotaPerUnit = parseFloat(quotaPerUnit);
  displayInCurrency = displayInCurrency === 'true';
  if (displayInCurrency) {
    const result = quota / quotaPerUnit;
    const fixedResult = result.toFixed(digits);

    // 如果 toFixed 后结果为 0 但原始值不为 0，显示最小值
    if (parseFloat(fixedResult) === 0 && quota > 0 && result > 0) {
      const minValue = Math.pow(10, -digits);
      return '$' + minValue.toFixed(digits);
    }

    return '$' + fixedResult;
  }
  return renderNumber(quota);
}

export function renderMoneyFen(fen) {
  if (fen === undefined || fen === null || Number.isNaN(fen)) {
    return `0.00 ${i18next.t('元')}`;
  }
  const fenNumber = typeof fen === 'string' ? parseInt(fen, 10) : Number(fen);
  if (!Number.isFinite(fenNumber)) {
    return `0.00 ${i18next.t('元')}`;
  }
  const sign = fenNumber < 0 ? '-' : '';
  const absFen = Math.abs(Math.trunc(fenNumber));
  const yuan = Math.floor(absFen / 100);
  const cents = absFen % 100;
  return `${sign}${yuan}.${String(cents).padStart(2, '0')} ${i18next.t('元')}`;
}

export function renderCnyFen(fen) {
  if (fen === undefined || fen === null || Number.isNaN(fen)) {
    return `￥0.00`;
  }
  const fenNumber = typeof fen === 'string' ? parseInt(fen, 10) : Number(fen);
  if (!Number.isFinite(fenNumber)) {
    return `￥0.00`;
  }
  const sign = fenNumber < 0 ? '-' : '';
  const absFen = Math.abs(Math.trunc(fenNumber));
  const yuan = Math.floor(absFen / 100);
  const cents = absFen % 100;
  return `${sign}￥${yuan}.${String(cents).padStart(2, '0')}`;
}

export function yuanToFen(value) {
  if (value === undefined || value === null) {
    throw new Error(i18next.t('金额不能为空'));
  }
  const str = String(value).trim();
  if (str === '') {
    throw new Error(i18next.t('金额不能为空'));
  }
  if (str.startsWith('-')) {
    throw new Error(i18next.t('金额不能为负数'));
  }
  const match = str.match(/^(\d+)(?:\.(\d{0,2}))?$/);
  if (!match) {
    throw new Error(i18next.t('金额格式错误'));
  }
  const intPart = match[1];
  const fracPartRaw = match[2] || '';
  const fracPart = (fracPartRaw + '00').slice(0, 2);
  const yuan = Number(intPart);
  const fen = Number(fracPart);
  if (!Number.isFinite(yuan) || !Number.isFinite(fen)) {
    throw new Error(i18next.t('金额格式错误'));
  }
  const amountFen = yuan * 100 + fen;
  if (!Number.isSafeInteger(amountFen)) {
    throw new Error(i18next.t('金额过大'));
  }
  return amountFen;
}

function isValidGroupRatio(ratio) {
  return Number.isFinite(ratio) && ratio !== -1;
}

/**
 * Helper function to get effective ratio and label
 * @param {number} groupRatio - The default group ratio
 * @param {number} user_group_ratio - The user-specific group ratio
 * @returns {Object} - Object containing { ratio, label, useUserGroupRatio }
 */
function getEffectiveRatio(groupRatio, user_group_ratio, group_ratio_source) {
  const useUserGroupRatio = isValidGroupRatio(user_group_ratio);
  const normalizedSource = String(group_ratio_source || '')
    .trim()
    .toLowerCase();
  let ratioLabel = i18next.t('分组倍率');
  if (useUserGroupRatio) {
    switch (normalizedSource) {
      case 'legacy':
        ratioLabel = i18next.t('旧专属倍率');
        break;
      case 'profile':
        ratioLabel = i18next.t('模板倍率');
        break;
      case 'base_multiplier':
        ratioLabel = i18next.t('基础倍率');
        break;
      default:
        ratioLabel = i18next.t('专属倍率');
        break;
    }
  }
  const effectiveRatio = useUserGroupRatio ? user_group_ratio : groupRatio;

  return {
    ratio: effectiveRatio,
    label: ratioLabel,
    useUserGroupRatio: useUserGroupRatio,
    source: normalizedSource || 'public',
  };
}

// Shared core for simple price rendering (used by OpenAI-like and Claude-like variants)
function renderPriceSimpleCore({
  modelRatio,
  modelPrice = -1,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheTokens = 0,
  cacheRatio = 1.0,
  cacheCreationTokens = 0,
  cacheCreationRatio = 1.0,
  cacheCreationTokens5m = 0,
  cacheCreationRatio5m = 1.0,
  cacheCreationTokens1h = 0,
  cacheCreationRatio1h = 1.0,
  image = false,
  imageRatio = 1.0,
  isSystemPromptOverride = false,
  baseMultiplier = 1.0,
  baseMultiplierApplied = true,
}) {
  const {
    ratio: effectiveGroupRatio,
    label: ratioLabel,
    useUserGroupRatio,
  } = getEffectiveRatio(groupRatio, user_group_ratio, groupRatioSource);
  const finalGroupRatio = effectiveGroupRatio;
  const parsedBaseMultiplier = parseFloat(baseMultiplier);
  const normalizedBaseMultiplier =
    Number.isFinite(parsedBaseMultiplier) && parsedBaseMultiplier > 0
      ? parsedBaseMultiplier
      : 1;
  const effectiveBaseMultiplier =
    baseMultiplierApplied === false ? 1 : normalizedBaseMultiplier;

  if (modelPrice !== -1) {
    return i18next.t('价格：${{price}} * {{ratioType}}：{{ratio}}', {
      price: modelPrice,
      ratioType: ratioLabel,
      ratio: finalGroupRatio,
    });
  }

  const parts = [];
  // base: model ratio
  parts.push(i18next.t('模型: {{ratio}}'));

  // cache part (label differs when with image)
  if (cacheTokens !== 0) {
    parts.push(i18next.t('缓存: {{cacheRatio}}'));
  }

  const hasSplitCacheCreation =
    cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;

  const shouldShowLegacyCacheCreation =
    !hasSplitCacheCreation && cacheCreationTokens !== 0;

  const shouldShowCacheCreation5m =
    hasSplitCacheCreation && cacheCreationTokens5m > 0;
  const shouldShowCacheCreation1h =
    hasSplitCacheCreation && cacheCreationTokens1h > 0;

  if (hasSplitCacheCreation) {
    if (shouldShowCacheCreation5m && shouldShowCacheCreation1h) {
      parts.push(
        i18next.t(
          '缓存创建: 5m {{cacheCreationRatio5m}} / 1h {{cacheCreationRatio1h}}',
        ),
      );
    } else if (shouldShowCacheCreation5m) {
      parts.push(i18next.t('缓存创建: 5m {{cacheCreationRatio5m}}'));
    } else if (shouldShowCacheCreation1h) {
      parts.push(i18next.t('缓存创建: 1h {{cacheCreationRatio1h}}'));
    }
  } else if (shouldShowLegacyCacheCreation) {
    parts.push(i18next.t('缓存创建: {{cacheCreationRatio}}'));
  }

  // image part
  if (image) {
    parts.push(i18next.t('图片输入: {{imageRatio}}'));
  }

  parts.push(`{{ratioType}}: {{groupRatio}}`);
  if (effectiveBaseMultiplier !== 1) {
    parts.push(i18next.t('基础倍率: {{baseMultiplier}}'));
  }

  let result = i18next.t(parts.join(' * '), {
    ratio: modelRatio,
    ratioType: ratioLabel,
    groupRatio: finalGroupRatio,
    cacheRatio: cacheRatio,
    cacheCreationRatio: cacheCreationRatio,
    cacheCreationRatio5m: cacheCreationRatio5m,
    cacheCreationRatio1h: cacheCreationRatio1h,
    imageRatio: imageRatio,
    baseMultiplier: effectiveBaseMultiplier,
  });

  if (isSystemPromptOverride) {
    result += '\n\r' + i18next.t('系统提示覆盖');
  }

  return result;
}

function normalizeServiceTierInfo(serviceTier, serviceTierMultiplier) {
  const normalizedServiceTier =
    typeof serviceTier === 'string' ? serviceTier.trim() : '';
  const parsedMultiplier = parseFloat(serviceTierMultiplier);
  const normalizedServiceTierMultiplier =
    Number.isFinite(parsedMultiplier) && parsedMultiplier > 0
      ? parsedMultiplier
      : 1;
  const hasServiceTierMultiplier =
    normalizedServiceTier !== '' && normalizedServiceTierMultiplier !== 1;
  return {
    normalizedServiceTier,
    normalizedServiceTierMultiplier,
    hasServiceTierMultiplier,
  };
}

export function renderModelPrice(
  modelName,
  inputTokens,
  completionTokens,
  modelRatio,
  modelPrice = -1,
  completionRatio,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheTokens = 0,
  cacheRatio = 1.0,
  image = false,
  imageRatio = 1.0,
  imageOutputTokens = 0,
  webSearch = false,
  webSearchCallCount = 0,
  webSearchPrice = 0,
  fileSearch = false,
  fileSearchCallCount = 0,
  fileSearchPrice = 0,
  audioInputSeperatePrice = false,
  audioInputTokens = 0,
  audioInputPrice = 0,
  imageGenerationCall = false,
  imageGenerationCallPrice = 0,
  baseMultiplier = 1.0,
  baseMultiplierApplied = true,
  serviceTier = '',
  serviceTierMultiplier = 1.0,
) {
  const {
    ratio: effectiveGroupRatio,
    label: ratioLabel,
    useUserGroupRatio,
  } = getEffectiveRatio(groupRatio, user_group_ratio, groupRatioSource);
  groupRatio = effectiveGroupRatio;
  const parsedBaseMultiplier = parseFloat(baseMultiplier);
  const normalizedBaseMultiplier =
    Number.isFinite(parsedBaseMultiplier) && parsedBaseMultiplier > 0
      ? parsedBaseMultiplier
      : 1;
  const effectiveBaseMultiplier =
    baseMultiplierApplied === false ? 1 : normalizedBaseMultiplier;
  const {
    normalizedServiceTier,
    normalizedServiceTierMultiplier,
    hasServiceTierMultiplier,
  } = normalizeServiceTierInfo(serviceTier, serviceTierMultiplier);
  const longContextPricing = getLongContextPricing(
    modelName,
    inputTokens,
    cacheTokens,
  );
  const baseMultiplierDesc =
    effectiveBaseMultiplier !== 1
      ? i18next.t(' * 基础倍率 {{baseMultiplier}}', {
          baseMultiplier: effectiveBaseMultiplier,
        })
      : '';

  if (modelPrice !== -1) {
    const baseModelPrice = hasServiceTierMultiplier
      ? modelPrice / normalizedServiceTierMultiplier
      : modelPrice;
    const serviceTierDesc = hasServiceTierMultiplier
      ? i18next.t(' * 服务层级倍率 {{serviceTier}}: {{multiplier}}', {
          serviceTier: normalizedServiceTier,
          multiplier: normalizedServiceTierMultiplier,
        })
      : normalizedServiceTier
        ? i18next.t(' * 服务层级 {{serviceTier}}', {
            serviceTier: normalizedServiceTier,
          })
        : '';
    const totalModelPrice = modelPrice * groupRatio * effectiveBaseMultiplier;
    return i18next.t(
      '模型价格：${{price}}{{serviceTierDesc}} * {{ratioType}}：{{ratio}}{{baseMultiplierDesc}} = ${{total}}',
      {
        price: baseModelPrice,
        ratio: groupRatio,
        ratioType: ratioLabel,
        serviceTierDesc,
        baseMultiplierDesc,
        total: totalModelPrice,
      },
    );
  } else {
    if (completionRatio === undefined) {
      completionRatio = 0;
    }
    let inputRatioPrice = getRatioUsdPerMillionTokens(modelRatio);
    let completionRatioPrice = inputRatioPrice * completionRatio;
    let cacheRatioPrice = inputRatioPrice * cacheRatio;
    let imageRatioPrice = inputRatioPrice * imageRatio;
    const baseInputRatioPrice = hasServiceTierMultiplier
      ? inputRatioPrice / normalizedServiceTierMultiplier
      : inputRatioPrice;
    const baseCompletionRatioPrice = hasServiceTierMultiplier
      ? completionRatioPrice / normalizedServiceTierMultiplier
      : completionRatioPrice;

    // Calculate effective input tokens (non-cached + cached with ratio applied)
    let effectiveInputTokens =
      inputTokens - cacheTokens + cacheTokens * cacheRatio;
    // Handle image tokens if present
    if (image && imageOutputTokens > 0) {
      effectiveInputTokens =
        inputTokens - imageOutputTokens + imageOutputTokens * imageRatio;
    }
    if (audioInputTokens > 0) {
      effectiveInputTokens -= audioInputTokens;
    }
    const adjustedEffectiveInputTokens =
      effectiveInputTokens * longContextPricing.inputMultiplier;
    const adjustedCompletionTokens =
      completionTokens * longContextPricing.outputMultiplier;
    let price =
      (adjustedEffectiveInputTokens / 1000000) * inputRatioPrice * groupRatio +
      (audioInputTokens / 1000000) * audioInputPrice * groupRatio +
      (adjustedCompletionTokens / 1000000) * completionRatioPrice * groupRatio +
      (webSearchCallCount / 1000) * webSearchPrice * groupRatio +
      (fileSearchCallCount / 1000) * fileSearchPrice * groupRatio +
      imageGenerationCallPrice * groupRatio;
    const totalWithBase = price * effectiveBaseMultiplier;

    return (
      <>
        <article>
          <p>
            {i18next.t('输入价格：${{price}} / 1M tokens{{audioPrice}}', {
              price: inputRatioPrice,
              audioPrice: audioInputSeperatePrice
                ? `，音频 $${audioInputPrice} / 1M tokens`
                : '',
            })}
          </p>
          <p>
            {i18next.t(
              '输出价格：${{price}} * {{completionRatio}} = ${{total}} / 1M tokens (补全倍率: {{completionRatio}})',
              {
                price: inputRatioPrice,
                total: completionRatioPrice,
                completionRatio: completionRatio,
              },
            )}
          </p>
          {cacheTokens > 0 && (
            <p>
              {i18next.t(
                '缓存价格：${{price}} * {{cacheRatio}} = ${{total}} / 1M tokens (缓存倍率: {{cacheRatio}})',
                {
                  price: inputRatioPrice,
                  total: inputRatioPrice * cacheRatio,
                  cacheRatio: cacheRatio,
                },
              )}
            </p>
          )}
          {longContextPricing.enabled && (
            <p>
              {i18next.t(
                '长上下文倍率：输入 {{inputMultiplier}}x，输出 {{outputMultiplier}}x',
                {
                  inputMultiplier: longContextPricing.inputMultiplier,
                  outputMultiplier: longContextPricing.outputMultiplier,
                },
              )}
            </p>
          )}
          {image && imageOutputTokens > 0 && (
            <p>
              {i18next.t(
                '图片输入价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (图片倍率: {{imageRatio}})',
                {
                  price: imageRatioPrice,
                  ratio: groupRatio,
                  total: imageRatioPrice * groupRatio,
                  imageRatio: imageRatio,
                },
              )}
            </p>
          )}
          {webSearch && webSearchCallCount > 0 && (
            <p>
              {i18next.t('Web搜索价格：${{price}} / 1K 次', {
                price: webSearchPrice,
              })}
            </p>
          )}
          {fileSearch && fileSearchCallCount > 0 && (
            <p>
              {i18next.t('文件搜索价格：${{price}} / 1K 次', {
                price: fileSearchPrice,
              })}
            </p>
          )}
          {imageGenerationCall && imageGenerationCallPrice > 0 && (
            <p>
              {i18next.t('图片生成调用：${{price}} / 1次', {
                price: imageGenerationCallPrice,
              })}
            </p>
          )}
          {effectiveBaseMultiplier !== 1 && (
            <p>
              {i18next.t('基础倍率: {{baseMultiplier}}', {
                baseMultiplier: effectiveBaseMultiplier,
              })}
            </p>
          )}
          {normalizedServiceTier && (
            <p>
              {hasServiceTierMultiplier
                ? i18next.t(
                    '服务层级倍率：{{serviceTier}} * {{multiplier}}，输入价格 ${{baseInputPrice}} -> ${{inputPrice}} / 1M tokens，输出价格 ${{baseCompletionPrice}} -> ${{completionPrice}} / 1M tokens',
                    {
                      serviceTier: normalizedServiceTier,
                      multiplier: normalizedServiceTierMultiplier,
                      baseInputPrice: baseInputRatioPrice.toFixed(6),
                      inputPrice: inputRatioPrice.toFixed(6),
                      baseCompletionPrice: baseCompletionRatioPrice.toFixed(6),
                      completionPrice: completionRatioPrice.toFixed(6),
                    },
                  )
                : i18next.t('服务层级：{{serviceTier}}', {
                    serviceTier: normalizedServiceTier,
                  })}
            </p>
          )}
          <p>
            {(() => {
              // 构建输入部分描述
              let inputDesc = '';
              if (image && imageOutputTokens > 0) {
                inputDesc = i18next.t(
                  '(输入 {{nonImageInput}} tokens + 图片输入 {{imageInput}} tokens * {{imageRatio}} / 1M tokens * ${{price}}',
                  {
                    nonImageInput: inputTokens - imageOutputTokens,
                    imageInput: imageOutputTokens,
                    imageRatio: imageRatio,
                    price: inputRatioPrice,
                  },
                );
              } else if (cacheTokens > 0) {
                inputDesc = i18next.t(
                  '(输入 {{nonCacheInput}} tokens / 1M tokens * ${{price}} + 缓存 {{cacheInput}} tokens / 1M tokens * ${{cachePrice}}',
                  {
                    nonCacheInput: inputTokens - cacheTokens,
                    cacheInput: cacheTokens,
                    price: inputRatioPrice,
                    cachePrice: cacheRatioPrice,
                  },
                );
              } else if (audioInputSeperatePrice && audioInputTokens > 0) {
                inputDesc = i18next.t(
                  '(输入 {{nonAudioInput}} tokens / 1M tokens * ${{price}} + 音频输入 {{audioInput}} tokens / 1M tokens * ${{audioPrice}}',
                  {
                    nonAudioInput: inputTokens - audioInputTokens,
                    audioInput: audioInputTokens,
                    price: inputRatioPrice,
                    audioPrice: audioInputPrice,
                  },
                );
              } else {
                inputDesc = i18next.t(
                  '(输入 {{input}} tokens / 1M tokens * ${{price}}',
                  {
                    input: inputTokens,
                    price: inputRatioPrice,
                  },
                );
              }

              // 构建输出部分描述
              const outputDesc = i18next.t(
                '输出 {{completion}} tokens / 1M tokens * ${{compPrice}}) * {{ratioType}} {{ratio}}',
                {
                  completion: completionTokens,
                  compPrice: completionRatioPrice,
                  ratio: groupRatio,
                  ratioType: ratioLabel,
                },
              );

              // 构建额外服务描述
              const extraServices = [
                webSearch && webSearchCallCount > 0
                  ? i18next.t(
                      ' + Web搜索 {{count}}次 / 1K 次 * ${{price}} * {{ratioType}} {{ratio}}',
                      {
                        count: webSearchCallCount,
                        price: webSearchPrice,
                        ratio: groupRatio,
                        ratioType: ratioLabel,
                      },
                    )
                  : '',
                fileSearch && fileSearchCallCount > 0
                  ? i18next.t(
                      ' + 文件搜索 {{count}}次 / 1K 次 * ${{price}} * {{ratioType}} {{ratio}}',
                      {
                        count: fileSearchCallCount,
                        price: fileSearchPrice,
                        ratio: groupRatio,
                        ratioType: ratioLabel,
                      },
                    )
                  : '',
                imageGenerationCall && imageGenerationCallPrice > 0
                  ? i18next.t(
                      ' + 图片生成调用 ${{price}} / 1次 * {{ratioType}} {{ratio}}',
                      {
                        price: imageGenerationCallPrice,
                        ratio: groupRatio,
                        ratioType: ratioLabel,
                      },
                    )
                  : '',
              ].join('');

              return i18next.t(
                '{{inputDesc}} + {{outputDesc}}{{extraServices}}{{baseMultiplierDesc}} = ${{total}}',
                {
                  inputDesc,
                  outputDesc,
                  extraServices,
                  baseMultiplierDesc,
                  total: totalWithBase.toFixed(6),
                },
              );
            })()}
          </p>
          <p>{i18next.t('仅供参考，以实际扣费为准')}</p>
        </article>
      </>
    );
  }
}

export function renderLogContent(
  modelName,
  inputTokens = 0,
  cacheTokens = 0,
  modelRatio,
  completionRatio,
  modelPrice = -1,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheRatio = 1.0,
  image = false,
  imageRatio = 1.0,
  webSearch = false,
  webSearchCallCount = 0,
  fileSearch = false,
  fileSearchCallCount = 0,
  baseMultiplier = 1.0,
  baseMultiplierApplied = true,
) {
  const {
    ratio,
    label: ratioLabel,
    useUserGroupRatio: useUserGroupRatio,
  } = getEffectiveRatio(groupRatio, user_group_ratio, groupRatioSource);
  const longContextPricing = getLongContextPricing(
    modelName,
    inputTokens,
    cacheTokens,
  );
  const parsedBaseMultiplier = parseFloat(baseMultiplier);
  const normalizedBaseMultiplier =
    Number.isFinite(parsedBaseMultiplier) && parsedBaseMultiplier > 0
      ? parsedBaseMultiplier
      : 1;
  const effectiveBaseMultiplier =
    baseMultiplierApplied === false ? 1 : normalizedBaseMultiplier;
  const baseMultiplierSuffix =
    effectiveBaseMultiplier !== 1
      ? i18next.t('，基础倍率 {{baseMultiplier}}', {
          baseMultiplier: effectiveBaseMultiplier,
        })
      : '';

  if (modelPrice !== -1) {
    const baseText = i18next.t('模型价格 ${{price}}，{{ratioType}} {{ratio}}', {
      price: modelPrice,
      ratioType: ratioLabel,
      ratio,
    });
    return baseText + baseMultiplierSuffix;
  } else {
    if (image) {
      const baseText = i18next.t(
        '模型倍率 {{modelRatio}}，缓存倍率 {{cacheRatio}}，输出倍率 {{completionRatio}}，图片输入倍率 {{imageRatio}}，{{ratioType}} {{ratio}}',
        {
          modelRatio: modelRatio,
          cacheRatio: cacheRatio,
          completionRatio: completionRatio,
          imageRatio: imageRatio,
          ratioType: ratioLabel,
          ratio,
        },
      );
      return baseText + baseMultiplierSuffix;
    } else if (webSearch) {
      const baseText = i18next.t(
        '模型倍率 {{modelRatio}}，缓存倍率 {{cacheRatio}}，输出倍率 {{completionRatio}}，{{ratioType}} {{ratio}}，Web 搜索调用 {{webSearchCallCount}} 次',
        {
          modelRatio: modelRatio,
          cacheRatio: cacheRatio,
          completionRatio: completionRatio,
          ratioType: ratioLabel,
          ratio,
          webSearchCallCount,
        },
      );
      return baseText + baseMultiplierSuffix;
    } else {
      const baseText = i18next.t(
        '模型倍率 {{modelRatio}}，缓存倍率 {{cacheRatio}}，输出倍率 {{completionRatio}}，{{ratioType}} {{ratio}}',
        {
          modelRatio: modelRatio,
          cacheRatio: cacheRatio,
          completionRatio: completionRatio,
          ratioType: ratioLabel,
          ratio,
        },
      );
      if (!longContextPricing.enabled) {
        return baseText + baseMultiplierSuffix;
      }
      return (
        baseText +
        i18next.t(
          '，长上下文倍率 输入 {{inputMultiplier}}x / 输出 {{outputMultiplier}}x',
          {
            inputMultiplier: longContextPricing.inputMultiplier,
            outputMultiplier: longContextPricing.outputMultiplier,
          },
        ) +
        baseMultiplierSuffix
      );
    }
  }
}

export function renderModelPriceSimple(
  modelRatio,
  modelPrice = -1,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheTokens = 0,
  cacheRatio = 1.0,
  cacheCreationTokens = 0,
  cacheCreationRatio = 1.0,
  image = false,
  imageRatio = 1.0,
  isSystemPromptOverride = false,
  provider = 'openai',
  baseMultiplier = 1.0,
  cacheCreationTokens5m = 0,
  cacheCreationRatio5m = 1.0,
  cacheCreationTokens1h = 0,
  cacheCreationRatio1h = 1.0,
) {
  return renderPriceSimpleCore({
    modelRatio,
    modelPrice,
    groupRatio,
    user_group_ratio,
    groupRatioSource,
    cacheTokens,
    cacheRatio,
    cacheCreationTokens,
    cacheCreationRatio,
    cacheCreationTokens5m,
    cacheCreationRatio5m,
    cacheCreationTokens1h,
    cacheCreationRatio1h,
    image,
    imageRatio,
    isSystemPromptOverride,
    baseMultiplier,
  });
}

export function renderAudioModelPrice(
  inputTokens,
  completionTokens,
  modelRatio,
  modelPrice = -1,
  completionRatio,
  audioInputTokens,
  audioCompletionTokens,
  audioRatio,
  audioCompletionRatio,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheTokens = 0,
  cacheRatio = 1.0,
  baseMultiplier = 1.0,
  baseMultiplierApplied = true,
) {
  const {
    ratio: effectiveGroupRatio,
    label: ratioLabel,
    useUserGroupRatio,
  } = getEffectiveRatio(groupRatio, user_group_ratio, groupRatioSource);
  groupRatio = effectiveGroupRatio;
  const parsedBaseMultiplier = parseFloat(baseMultiplier);
  const normalizedBaseMultiplier =
    Number.isFinite(parsedBaseMultiplier) && parsedBaseMultiplier > 0
      ? parsedBaseMultiplier
      : 1;
  const effectiveBaseMultiplier =
    baseMultiplierApplied === false ? 1 : normalizedBaseMultiplier;
  const baseMultiplierDesc =
    effectiveBaseMultiplier !== 1
      ? i18next.t(' * 基础倍率 {{baseMultiplier}}', {
          baseMultiplier: effectiveBaseMultiplier,
        })
      : '';
  if (modelPrice !== -1) {
    const totalModelPrice = modelPrice * groupRatio * effectiveBaseMultiplier;
    return i18next.t(
      '模型价格：${{price}} * {{ratioType}}：{{ratio}}{{baseMultiplierDesc}} = ${{total}}',
      {
        price: modelPrice,
        ratio: groupRatio,
        total: totalModelPrice,
        ratioType: ratioLabel,
        baseMultiplierDesc,
      },
    );
  } else {
    if (completionRatio === undefined) {
      completionRatio = 0;
    }

    // try toFixed audioRatio
    audioRatio = parseFloat(audioRatio).toFixed(6);
    // 按当前 quota_per_unit 动态换算 1M tokens 的美元价格。
    let inputRatioPrice = getRatioUsdPerMillionTokens(modelRatio);
    let completionRatioPrice = inputRatioPrice * completionRatio;
    let cacheRatioPrice = inputRatioPrice * cacheRatio;

    // Calculate effective input tokens (non-cached + cached with ratio applied)
    const effectiveInputTokens =
      inputTokens - cacheTokens + cacheTokens * cacheRatio;

    let textPrice =
      (effectiveInputTokens / 1000000) * inputRatioPrice * groupRatio +
      (completionTokens / 1000000) * completionRatioPrice * groupRatio;
    let audioPrice =
      (audioInputTokens / 1000000) * inputRatioPrice * audioRatio * groupRatio +
      (audioCompletionTokens / 1000000) *
        inputRatioPrice *
        audioRatio *
        audioCompletionRatio *
        groupRatio;
    let price = textPrice + audioPrice;
    const totalWithBase = price * effectiveBaseMultiplier;
    return (
      <>
        <article>
          <p>
            {i18next.t('提示价格：${{price}} / 1M tokens', {
              price: inputRatioPrice,
            })}
          </p>
          <p>
            {i18next.t(
              '补全价格：${{price}} * {{completionRatio}} = ${{total}} / 1M tokens (补全倍率: {{completionRatio}})',
              {
                price: inputRatioPrice,
                total: completionRatioPrice,
                completionRatio: completionRatio,
              },
            )}
          </p>
          {cacheTokens > 0 && (
            <p>
              {i18next.t(
                '缓存价格：${{price}} * {{cacheRatio}} = ${{total}} / 1M tokens (缓存倍率: {{cacheRatio}})',
                {
                  price: inputRatioPrice,
                  total: inputRatioPrice * cacheRatio,
                  cacheRatio: cacheRatio,
                },
              )}
            </p>
          )}
          <p>
            {i18next.t(
              '音频提示价格：${{price}} * {{audioRatio}} = ${{total}} / 1M tokens (音频倍率: {{audioRatio}})',
              {
                price: inputRatioPrice,
                total: inputRatioPrice * audioRatio,
                audioRatio: audioRatio,
              },
            )}
          </p>
          <p>
            {i18next.t(
              '音频补全价格：${{price}} * {{audioRatio}} * {{audioCompRatio}} = ${{total}} / 1M tokens (音频补全倍率: {{audioCompRatio}})',
              {
                price: inputRatioPrice,
                total: inputRatioPrice * audioRatio * audioCompletionRatio,
                audioRatio: audioRatio,
                audioCompRatio: audioCompletionRatio,
              },
            )}
          </p>
          <p>
            {cacheTokens > 0
              ? i18next.t(
                  '文字提示 {{nonCacheInput}} tokens / 1M tokens * ${{price}} + 缓存 {{cacheInput}} tokens / 1M tokens * ${{cachePrice}} + 文字补全 {{completion}} tokens / 1M tokens * ${{compPrice}} = ${{total}}',
                  {
                    nonCacheInput: inputTokens - cacheTokens,
                    cacheInput: cacheTokens,
                    cachePrice: inputRatioPrice * cacheRatio,
                    price: inputRatioPrice,
                    completion: completionTokens,
                    compPrice: completionRatioPrice,
                    total: textPrice.toFixed(6),
                  },
                )
              : i18next.t(
                  '文字提示 {{input}} tokens / 1M tokens * ${{price}} + 文字补全 {{completion}} tokens / 1M tokens * ${{compPrice}} = ${{total}}',
                  {
                    input: inputTokens,
                    price: inputRatioPrice,
                    completion: completionTokens,
                    compPrice: completionRatioPrice,
                    total: textPrice.toFixed(6),
                  },
                )}
          </p>
          <p>
            {i18next.t(
              '音频提示 {{input}} tokens / 1M tokens * ${{audioInputPrice}} + 音频补全 {{completion}} tokens / 1M tokens * ${{audioCompPrice}} = ${{total}}',
              {
                input: audioInputTokens,
                completion: audioCompletionTokens,
                audioInputPrice: audioRatio * inputRatioPrice,
                audioCompPrice:
                  audioRatio * audioCompletionRatio * inputRatioPrice,
                total: audioPrice.toFixed(6),
              },
            )}
          </p>
          {effectiveBaseMultiplier !== 1 && (
            <p>
              {i18next.t('基础倍率: {{baseMultiplier}}', {
                baseMultiplier: effectiveBaseMultiplier,
              })}
            </p>
          )}
          <p>
            {i18next.t(
              '总价：文字价格 {{textPrice}} + 音频价格 {{audioPrice}}{{baseMultiplierDesc}} = ${{total}}',
              {
                total: totalWithBase.toFixed(6),
                textPrice: textPrice.toFixed(6),
                audioPrice: audioPrice.toFixed(6),
                baseMultiplierDesc,
              },
            )}
          </p>
          <p>{i18next.t('仅供参考，以实际扣费为准')}</p>
        </article>
      </>
    );
  }
}

export function renderQuotaWithPrompt(quota, digits) {
  let displayInCurrency = localStorage.getItem('display_in_currency');
  displayInCurrency = displayInCurrency === 'true';
  if (displayInCurrency) {
    return i18next.t('等价金额：') + renderQuota(quota, digits);
  }
  return '';
}

export function renderQuotaToUSD(quota, digits = 2) {
  const rawPerUnit = localStorage.getItem('quota_per_unit');
  const quotaPerUnit = parseFloat(rawPerUnit);
  if (!quotaPerUnit || Number.isNaN(quotaPerUnit) || quotaPerUnit <= 0) {
    return `$${(0).toFixed(digits)}`;
  }
  const dollars = quota / quotaPerUnit;
  if (!Number.isFinite(dollars)) {
    return `$${(0).toFixed(digits)}`;
  }
  return `$${dollars.toFixed(digits)}`;
}

export function renderClaudeModelPrice(
  inputTokens,
  completionTokens,
  modelRatio,
  modelPrice = -1,
  completionRatio,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheTokens = 0,
  cacheRatio = 1.0,
  cacheCreationTokens = 0,
  cacheCreationRatio = 1.0,
  baseMultiplier = 1.0,
  baseMultiplierApplied = true,
  cacheCreationTokens5m = 0,
  cacheCreationRatio5m = 1.0,
  cacheCreationTokens1h = 0,
  cacheCreationRatio1h = 1.0,
  serviceTier = '',
  serviceTierMultiplier = 1.0,
) {
  const {
    ratio: effectiveGroupRatio,
    label: ratioLabel,
    useUserGroupRatio,
  } = getEffectiveRatio(groupRatio, user_group_ratio, groupRatioSource);
  groupRatio = effectiveGroupRatio;
  const parsedBaseMultiplier = parseFloat(baseMultiplier);
  const normalizedBaseMultiplier =
    Number.isFinite(parsedBaseMultiplier) && parsedBaseMultiplier > 0
      ? parsedBaseMultiplier
      : 1;
  const effectiveBaseMultiplier =
    baseMultiplierApplied === false ? 1 : normalizedBaseMultiplier;
  const {
    normalizedServiceTier,
    normalizedServiceTierMultiplier,
    hasServiceTierMultiplier,
  } = normalizeServiceTierInfo(serviceTier, serviceTierMultiplier);
  const baseMultiplierDesc =
    effectiveBaseMultiplier !== 1
      ? i18next.t(' * 基础倍率 {{baseMultiplier}}', {
          baseMultiplier: effectiveBaseMultiplier,
        })
      : '';

  if (modelPrice !== -1) {
    const baseModelPrice = hasServiceTierMultiplier
      ? modelPrice / normalizedServiceTierMultiplier
      : modelPrice;
    const serviceTierDesc = hasServiceTierMultiplier
      ? i18next.t(' * 服务层级倍率 {{serviceTier}}: {{multiplier}}', {
          serviceTier: normalizedServiceTier,
          multiplier: normalizedServiceTierMultiplier,
        })
      : normalizedServiceTier
        ? i18next.t(' * 服务层级 {{serviceTier}}', {
            serviceTier: normalizedServiceTier,
          })
        : '';
    const totalModelPrice = modelPrice * groupRatio * effectiveBaseMultiplier;
    return i18next.t(
      '模型价格：${{price}}{{serviceTierDesc}} * {{ratioType}}：{{ratio}}{{baseMultiplierDesc}} = ${{total}}',
      {
        price: baseModelPrice,
        ratioType: ratioLabel,
        ratio: groupRatio,
        serviceTierDesc,
        baseMultiplierDesc,
        total: totalModelPrice,
      },
    );
  } else {
    if (completionRatio === undefined) {
      completionRatio = 0;
    }

    const completionRatioValue = completionRatio || 0;
    const inputRatioPrice = getRatioUsdPerMillionTokens(modelRatio);
    const completionRatioPrice = inputRatioPrice * completionRatioValue;
    const cacheRatioPrice = inputRatioPrice * cacheRatio;
    const cacheCreationRatioPrice = inputRatioPrice * cacheCreationRatio;
    const cacheCreationRatioPrice5m = inputRatioPrice * cacheCreationRatio5m;
    const cacheCreationRatioPrice1h = inputRatioPrice * cacheCreationRatio1h;
    const baseInputRatioPrice = hasServiceTierMultiplier
      ? inputRatioPrice / normalizedServiceTierMultiplier
      : inputRatioPrice;
    const baseCompletionRatioPrice = hasServiceTierMultiplier
      ? completionRatioPrice / normalizedServiceTierMultiplier
      : completionRatioPrice;

    const hasSplitCacheCreation =
      cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;

    const remainingCacheCreationTokens = Math.max(
      cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h,
      0,
    );

    const shouldShowCache = cacheTokens > 0;
    const shouldShowLegacyCacheCreation =
      !hasSplitCacheCreation && cacheCreationTokens > 0;
    const shouldShowCacheCreation5m =
      hasSplitCacheCreation && cacheCreationTokens5m > 0;
    const shouldShowCacheCreation1h =
      hasSplitCacheCreation && cacheCreationTokens1h > 0;
    const shouldShowRemainingCacheCreation =
      hasSplitCacheCreation && remainingCacheCreationTokens > 0;

    // Calculate effective input tokens (non-cached + cached with ratio applied + cache creation with ratio applied)
    const nonCachedTokens = inputTokens;
    const effectiveInputTokens =
      nonCachedTokens +
      cacheTokens * cacheRatio +
      remainingCacheCreationTokens * cacheCreationRatio +
      cacheCreationTokens5m * cacheCreationRatio5m +
      cacheCreationTokens1h * cacheCreationRatio1h;

    let price =
      (effectiveInputTokens / 1000000) * inputRatioPrice * groupRatio +
      (completionTokens / 1000000) * completionRatioPrice * groupRatio;
    const totalWithBase = price * effectiveBaseMultiplier;

    const breakdownSegments = [
      i18next.t('提示 {{input}} tokens / 1M tokens * ${{price}}', {
        input: nonCachedTokens,
        price: inputRatioPrice.toFixed(6),
      }),
    ];
    if (shouldShowCache) {
      breakdownSegments.push(
        i18next.t(
          '缓存 {{tokens}} tokens / 1M tokens * ${{price}} (倍率: {{ratio}})',
          {
            tokens: cacheTokens,
            price: cacheRatioPrice.toFixed(6),
            ratio: cacheRatio,
          },
        ),
      );
    }
    if (shouldShowLegacyCacheCreation) {
      breakdownSegments.push(
        i18next.t(
          '缓存创建 {{tokens}} tokens / 1M tokens * ${{price}} (倍率: {{ratio}})',
          {
            tokens: cacheCreationTokens,
            price: cacheCreationRatioPrice.toFixed(6),
            ratio: cacheCreationRatio,
          },
        ),
      );
    }
    if (shouldShowCacheCreation5m) {
      breakdownSegments.push(
        i18next.t(
          '5m缓存创建 {{tokens}} tokens / 1M tokens * ${{price}} (倍率: {{ratio}})',
          {
            tokens: cacheCreationTokens5m,
            price: cacheCreationRatioPrice5m.toFixed(6),
            ratio: cacheCreationRatio5m,
          },
        ),
      );
    }
    if (shouldShowCacheCreation1h) {
      breakdownSegments.push(
        i18next.t(
          '1h缓存创建 {{tokens}} tokens / 1M tokens * ${{price}} (倍率: {{ratio}})',
          {
            tokens: cacheCreationTokens1h,
            price: cacheCreationRatioPrice1h.toFixed(6),
            ratio: cacheCreationRatio1h,
          },
        ),
      );
    }
    if (shouldShowRemainingCacheCreation) {
      breakdownSegments.push(
        i18next.t(
          '缓存创建 {{tokens}} tokens / 1M tokens * ${{price}} (倍率: {{ratio}})',
          {
            tokens: remainingCacheCreationTokens,
            price: cacheCreationRatioPrice.toFixed(6),
            ratio: cacheCreationRatio,
          },
        ),
      );
    }
    breakdownSegments.push(
      i18next.t('补全 {{completion}} tokens / 1M tokens * ${{price}}', {
        completion: completionTokens,
        price: completionRatioPrice.toFixed(6),
      }),
    );
    const breakdownText = breakdownSegments.join(' + ');

    return (
      <>
        <article>
          <p>
            {i18next.t('提示价格：${{price}} / 1M tokens', {
              price: inputRatioPrice,
            })}
          </p>
          <p>
            {i18next.t(
              '补全价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens',
              {
                price: inputRatioPrice,
                ratio: completionRatio,
                total: completionRatioPrice,
              },
            )}
          </p>
          {cacheTokens > 0 && (
            <p>
              {i18next.t(
                '缓存价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (缓存倍率: {{cacheRatio}})',
                {
                  price: inputRatioPrice,
                  ratio: cacheRatio,
                  total: cacheRatioPrice.toFixed(6),
                  cacheRatio: cacheRatio,
                },
              )}
            </p>
          )}
          {shouldShowLegacyCacheCreation && (
            <p>
              {i18next.t(
                '缓存创建价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (缓存创建倍率: {{cacheCreationRatio}})',
                {
                  price: inputRatioPrice,
                  ratio: cacheCreationRatio,
                  total: cacheCreationRatioPrice.toFixed(6),
                  cacheCreationRatio: cacheCreationRatio,
                },
              )}
            </p>
          )}
          {shouldShowCacheCreation5m && (
            <p>
              {i18next.t(
                '5m缓存创建价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (5m缓存创建倍率: {{cacheCreationRatio5m}})',
                {
                  price: inputRatioPrice,
                  ratio: cacheCreationRatio5m,
                  total: cacheCreationRatioPrice5m.toFixed(6),
                  cacheCreationRatio5m: cacheCreationRatio5m,
                },
              )}
            </p>
          )}
          {shouldShowCacheCreation1h && (
            <p>
              {i18next.t(
                '1h缓存创建价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (1h缓存创建倍率: {{cacheCreationRatio1h}})',
                {
                  price: inputRatioPrice,
                  ratio: cacheCreationRatio1h,
                  total: cacheCreationRatioPrice1h.toFixed(6),
                  cacheCreationRatio1h: cacheCreationRatio1h,
                },
              )}
            </p>
          )}
          {shouldShowRemainingCacheCreation && (
            <p>
              {i18next.t(
                '缓存创建价格：${{price}} * {{ratio}} = ${{total}} / 1M tokens (缓存创建倍率: {{cacheCreationRatio}})',
                {
                  price: inputRatioPrice,
                  ratio: cacheCreationRatio,
                  total: cacheCreationRatioPrice.toFixed(6),
                  cacheCreationRatio: cacheCreationRatio,
                },
              )}
            </p>
          )}
          <p></p>
          {effectiveBaseMultiplier !== 1 && (
            <p>
              {i18next.t('基础倍率: {{baseMultiplier}}', {
                baseMultiplier: effectiveBaseMultiplier,
              })}
            </p>
          )}
          {normalizedServiceTier && (
            <p>
              {hasServiceTierMultiplier
                ? i18next.t(
                    '服务层级倍率：{{serviceTier}} * {{multiplier}}，提示价格 ${{baseInputPrice}} -> ${{inputPrice}} / 1M tokens，补全价格 ${{baseCompletionPrice}} -> ${{completionPrice}} / 1M tokens',
                    {
                      serviceTier: normalizedServiceTier,
                      multiplier: normalizedServiceTierMultiplier,
                      baseInputPrice: baseInputRatioPrice.toFixed(6),
                      inputPrice: inputRatioPrice.toFixed(6),
                      baseCompletionPrice: baseCompletionRatioPrice.toFixed(6),
                      completionPrice: completionRatioPrice.toFixed(6),
                    },
                  )
                : i18next.t('服务层级：{{serviceTier}}', {
                    serviceTier: normalizedServiceTier,
                  })}
            </p>
          )}
          <p>
            {i18next.t(
              '{{breakdown}} * {{ratioType}} {{ratio}}{{baseMultiplierDesc}} = ${{total}}',
              {
                breakdown: breakdownText,
                ratio: groupRatio,
                ratioType: ratioLabel,
                baseMultiplierDesc,
                total: totalWithBase.toFixed(6),
              },
            )}
          </p>
          <p>{i18next.t('仅供参考，以实际扣费为准')}</p>
        </article>
      </>
    );
  }
}

export function renderClaudeLogContent(
  modelRatio,
  completionRatio,
  modelPrice = -1,
  groupRatio,
  user_group_ratio,
  groupRatioSource,
  cacheRatio = 1.0,
  cacheCreationRatio = 1.0,
  cacheCreationTokens5m = 0,
  cacheCreationRatio5m = 1.0,
  cacheCreationTokens1h = 0,
  cacheCreationRatio1h = 1.0,
) {
  const { ratio: effectiveGroupRatio, label: ratioLabel } = getEffectiveRatio(
    groupRatio,
    user_group_ratio,
    groupRatioSource,
  );
  groupRatio = effectiveGroupRatio;

  if (modelPrice !== -1) {
    return i18next.t('模型价格 ${{price}}，{{ratioType}} {{ratio}}', {
      price: modelPrice,
      ratioType: ratioLabel,
      ratio: groupRatio,
    });
  } else {
    const hasSplitCacheCreation =
      cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;
    const shouldShowCacheCreation5m =
      hasSplitCacheCreation && cacheCreationTokens5m > 0;
    const shouldShowCacheCreation1h =
      hasSplitCacheCreation && cacheCreationTokens1h > 0;

    let cacheCreationPart = null;
    if (hasSplitCacheCreation) {
      if (shouldShowCacheCreation5m && shouldShowCacheCreation1h) {
        cacheCreationPart = i18next.t(
          '缓存创建倍率 5m {{cacheCreationRatio5m}} / 1h {{cacheCreationRatio1h}}',
          {
            cacheCreationRatio5m,
            cacheCreationRatio1h,
          },
        );
      } else if (shouldShowCacheCreation5m) {
        cacheCreationPart = i18next.t(
          '缓存创建倍率 5m {{cacheCreationRatio5m}}',
          {
            cacheCreationRatio5m,
          },
        );
      } else if (shouldShowCacheCreation1h) {
        cacheCreationPart = i18next.t(
          '缓存创建倍率 1h {{cacheCreationRatio1h}}',
          {
            cacheCreationRatio1h,
          },
        );
      }
    }

    if (!cacheCreationPart) {
      cacheCreationPart = i18next.t('缓存创建倍率 {{cacheCreationRatio}}', {
        cacheCreationRatio,
      });
    }

    const parts = [
      i18next.t('模型倍率 {{modelRatio}}', { modelRatio }),
      i18next.t('输出倍率 {{completionRatio}}', { completionRatio }),
      i18next.t('缓存倍率 {{cacheRatio}}', { cacheRatio }),
      cacheCreationPart,
      i18next.t('{{ratioType}} {{ratio}}', {
        ratioType: ratioLabel,
        ratio: groupRatio,
      }),
    ];

    return parts.join('，');
  }
}

// 已统一至 renderModelPriceSimple，若仍有遗留引用，请改为传入 provider='claude'

/**
 * rehype 插件：将段落等文本节点拆分为逐词 <span>，并添加淡入动画 class。
 * 仅在流式渲染阶段使用，避免已渲染文字重复动画。
 */
export function rehypeSplitWordsIntoSpans(options = {}) {
  const { previousContentLength = 0 } = options;

  return (tree) => {
    let currentCharCount = 0; // 当前已处理的字符数

    visit(tree, 'element', (node) => {
      if (
        ['p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li', 'strong'].includes(
          node.tagName,
        ) &&
        node.children
      ) {
        const newChildren = [];
        node.children.forEach((child) => {
          if (child.type === 'text') {
            try {
              // 使用 Intl.Segmenter 精准拆分中英文及标点
              const segmenter = new Intl.Segmenter('zh', {
                granularity: 'word',
              });
              const segments = segmenter.segment(child.value);

              Array.from(segments)
                .map((seg) => seg.segment)
                .filter(Boolean)
                .forEach((word) => {
                  const wordStartPos = currentCharCount;
                  const wordEndPos = currentCharCount + word.length;

                  // 判断这个词是否是新增的（在 previousContentLength 之后）
                  const isNewContent = wordStartPos >= previousContentLength;

                  newChildren.push({
                    type: 'element',
                    tagName: 'span',
                    properties: {
                      className: isNewContent ? ['animate-fade-in'] : [],
                    },
                    children: [{ type: 'text', value: word }],
                  });

                  currentCharCount = wordEndPos;
                });
            } catch (_) {
              // Fallback：如果浏览器不支持 Segmenter
              const textStartPos = currentCharCount;
              const isNewContent = textStartPos >= previousContentLength;

              if (isNewContent) {
                // 新内容，添加动画
                newChildren.push({
                  type: 'element',
                  tagName: 'span',
                  properties: {
                    className: ['animate-fade-in'],
                  },
                  children: [{ type: 'text', value: child.value }],
                });
              } else {
                // 旧内容，不添加动画
                newChildren.push(child);
              }

              currentCharCount += child.value.length;
            }
          } else {
            newChildren.push(child);
          }
        });
        node.children = newChildren;
      }
    });
  };
}
