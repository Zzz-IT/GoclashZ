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
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
};

export const formatDurationCn = (seconds: number) => {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0秒';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  
  const parts = [];
  if (h > 0) parts.push(`${h}小时`);
  if (m > 0) parts.push(`${m}分`);
  if (s > 0) parts.push(`${s}秒`);
  
  return parts.join('');
};

export const formatRelativeTime = (timestamp: number) => {
  if (!timestamp) return '从未';
  
  const now = new Date().getTime();
  // Ensure timestamp is in milliseconds
  const tsMs = timestamp > 9999999999 ? timestamp : timestamp * 1000;
  const diff = now - tsMs;
  
  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return Math.floor(diff / 60000) + ' 分钟前';
  if (diff < 86400000) return Math.floor(diff / 3600000) + ' 小时前';
  if (diff < 2592000000) return Math.floor(diff / 86400000) + ' 天前';
  return new Date(tsMs).toLocaleDateString();
};
