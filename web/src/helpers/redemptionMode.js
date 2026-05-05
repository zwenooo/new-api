const normalizeMode = (value) => String(value ?? '').trim();

const hasElements = (value) => Array.isArray(value) && value.length > 0;

const inferLegacyMode = (record) => {
  if (!record) return '';
  if (Number(record?.plan_valid_days ?? 0) > 0 || hasElements(record?.channel_ids)) {
    return 'xiaotuan';
  }
  if (Number(record?.daily_request_limit ?? 0) > 0) {
    return 'request';
  }
  if (
    Number(record?.daily_quota_limit ?? 0) > 0 ||
    Number(record?.quota_valid_days ?? 0) > 0 ||
    hasElements(record?.group_daily_limits)
  ) {
    return 'subscription';
  }
  if (hasElements(record?.allowed_group_ids)) {
    return 'payg';
  }
  if (Number(record?.quota ?? 0) > 0) {
    return 'free';
  }
  return '';
};

export const inferBillingMode = (record, defaultMode = '') => {
  const mode = normalizeMode(record?.mode);
  if (mode) return mode;
  return inferLegacyMode(record) || defaultMode;
};

export const inferPresetMode = (record) => inferBillingMode(record, 'subscription');

export const inferRedemptionMode = (record) =>
  inferBillingMode(record, 'subscription');
