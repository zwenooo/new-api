export const DICEBEAR_AVATAR_STYLE = 'open-peeps';
export const DICEBEAR_AVATAR_API_BASE = 'https://api.dicebear.com/7.x';

export function getDiceBearAvatarUrl(seed, { size = 64 } = {}) {
  if (!seed) return '';
  const normalizedSeed = String(seed).trim();
  if (!normalizedSeed) return '';

  const base = DICEBEAR_AVATAR_API_BASE.replace(/\/$/, '');
  const style = encodeURIComponent(DICEBEAR_AVATAR_STYLE);
  const url = new URL(`${base}/${style}/svg`);
  url.searchParams.set('seed', normalizedSeed);
  if (size) url.searchParams.set('size', String(size));
  return url.toString();
}

