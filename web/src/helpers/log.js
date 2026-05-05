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

export function getLogOther(otherStr) {
  if (otherStr === undefined || otherStr === '') {
    otherStr = '{}';
  }
  let other = JSON.parse(otherStr);
  return other;
}

export function normalizeManageLogContent(text, t) {
  if (!text) {
    return text;
  }

  const normalized = text.trim();
  if (normalized === '') {
    return text;
  }

  const quotaMatches = normalized.match(/(?:[＄$]\s*)?[0-9]+(?:\.[0-9]+)?\s*额度/g);
  if (quotaMatches && quotaMatches.length >= 2) {
    const fromQuota = quotaMatches[0].trim();
    const toQuota = quotaMatches[1].trim();
    const isDaily = normalized.includes('每日');
    const translationKey = isDaily
      ? '管理员将用户每日额度从 {{from}} 修改为 {{to}}'
      : '管理员将用户额度从 {{from}} 修改为 {{to}}';
    const fallbackText = isDaily
      ? `管理员将用户每日额度从 ${fromQuota} 修改为 ${toQuota}`
      : `管理员将用户额度从 ${fromQuota} 修改为 ${toQuota}`;
    if (typeof t === 'function') {
      return t(translationKey, {
        from: fromQuota,
        to: toQuota,
        defaultValue: fallbackText,
      });
    }
    return fallbackText;
  }

  const adminIdMatch = normalized.match(/ID\s*:?\s*(\d+)/i);
  if (adminIdMatch) {
    const adminId = adminIdMatch[1];
    const translationKey = '管理员(ID:{{id}})强制禁用了用户的两步验证';
    const fallbackText = `管理员(ID:${adminId})强制禁用了用户的两步验证`;
    if (typeof t === 'function') {
      return t(translationKey, {
        id: adminId,
        defaultValue: fallbackText,
      });
    }
    return fallbackText;
  }

  return text;
}

export function formatBytes(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) {
    return '0 B';
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = n;
  let i = 0;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i += 1;
  }

  const digits = i === 0 ? 0 : size >= 10 ? 1 : 2;
  return `${size.toFixed(digits)} ${units[i]}`;
}

export function formatBytesWithExact(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) {
    return '0 B';
  }

  const exact = `${Math.round(n).toLocaleString()} bytes`;
  if (n < 1024) {
    return exact;
  }

  return `${formatBytes(n)} (${exact})`;
}
