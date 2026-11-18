// U-03：可见文案改走 i18n（key 见 docs/audit-frontend/i18n-needs-fix-7-utils.md）。
// 语言包补齐之前由 tr() 回退到原来的中文，保证本 Wave 行为不变。
import { t } from '../locales';

/** t() 在 key 缺失时原样返回 key 本身（locales/index.ts:64-67），据此回退到内置文案 */
const tr = (key: string, fallback: string, params?: Record<string, string | number>) => {
  const value = t(key, params);
  return value === key ? fallback : value;
};

export const formatBytes = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';

  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);

  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

export const formatSpeed = (bps: number) => {
  if (!Number.isFinite(bps) || bps <= 0) return '0 B/s';
  return formatBytes(bps) + '/s';
};

export const formatEtaTime = (seconds: number) => {
  if (!Number.isFinite(seconds) || seconds < 0) return '--';
  if (seconds === 0) return '--';
  // U-06：补小时档。慢速链路下内核/组件包任务的 ETA 经常超过 3600s，
  // 旧实现会一路显示 60m 0s … 1440m 0s。
  if (seconds >= 3600) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return `${h}h ${m}m`;
  }
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
};

export const formatRelativeTime = (timestamp: number) => {
  // U-03：原来这套中文（从未 / 刚刚 / N 分钟前…）在 8 语言下固定显示中文。
  if (!timestamp) return tr('common.never', '从未');

  const now = new Date().getTime();
  // Ensure timestamp is in milliseconds
  const tsMs = timestamp > 9999999999 ? timestamp : timestamp * 1000;
  const diff = now - tsMs;

  if (diff < 60000) return tr('common.justNow', '刚刚');
  if (diff < 3600000) {
    const n = Math.floor(diff / 60000);
    return tr('common.minutesAgo', `${n} 分钟前`, { n });
  }
  if (diff < 86400000) {
    const n = Math.floor(diff / 3600000);
    return tr('common.hoursAgo', `${n} 小时前`, { n });
  }
  if (diff < 2592000000) {
    const n = Math.floor(diff / 86400000);
    return tr('common.daysAgo', `${n} 天前`, { n });
  }
  return new Date(tsMs).toLocaleDateString();
};
