import { ref } from 'vue';
import zhCN from './zh-CN';
import zhTW from './zh-TW';
import zhHK from './zh-HK';
import enUS from './en-US';
import jaJP from './ja-JP';
import ruRU from './ru-RU';
import frFR from './fr-FR';
import deDE from './de-DE';
import { SupportedLocale, SUPPORTED_LOCALES } from './types';

export * from './types';

const messages: Record<SupportedLocale, any> = {
  'zh-CN': zhCN,
  'zh-TW': zhTW,
  'zh-HK': zhHK,
  'en-US': enUS,
  'ja-JP': jaJP,
  'ru-RU': ruRU,
  'fr-FR': frFR,
  'de-DE': deDE,
};

const STORAGE_KEY = 'goclashz_language';

// 1. 同步预热读取本地缓存，杜绝页面初次渲染时的闪烁
function getInitialLocale(): SupportedLocale {
  const cached = localStorage.getItem(STORAGE_KEY) as SupportedLocale | null;
  if (cached && messages[cached]) {
    return cached;
  }
  return 'zh-CN';
}

export const currentLocale = ref<SupportedLocale>(getInitialLocale());

function getNestedValue(obj: any, keys: string[]): any {
  let curr = obj;
  for (const k of keys) {
    if (!curr || typeof curr !== 'object') return undefined;
    curr = curr[k];
  }
  return curr;
}

/**
 * 翻译并解析插值
 * @param path 嵌套路径，例如 'settings.about.language'
 * @param params 可选的动态插值对象，例如 { name: 'DNS', error: '404' }
 */
export function t(path: string, params?: Record<string, string | number>): string {
  const keys = path.split('.');
  const locale = currentLocale.value;

  // 1. 查当前选定语言
  let result = getNestedValue(messages[locale], keys);

  // 2. 优雅降级：若当前语言无此 key，回退到 zh-CN 基准包
  if (result === undefined && locale !== 'zh-CN') {
    result = getNestedValue(messages['zh-CN'], keys);
  }

  // 3. 兜底返回 path 自身，防止白屏
  if (result === undefined) {
    return path;
  }

  // 4. 替换参数插值，例如 "剩余 {time}"
  if (params && typeof result === 'string') {
    return result.replace(/\{(\w+)\}/g, (match, key) => {
      return params[key] !== undefined ? String(params[key]) : match;
    });
  }

  return String(result);
}

/**
 * 设置全局语言
 */
export function setLocale(locale: SupportedLocale) {
  if (!messages[locale]) return;
  currentLocale.value = locale;
  localStorage.setItem(STORAGE_KEY, locale);
}
