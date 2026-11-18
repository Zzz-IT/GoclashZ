// 剪贴板复制的唯一实现（U-18）。
//
// 原来 App.vue（日志复制 / 日志目录）、SettingsDiagnostics.vue（各路径复制）、
// Subscriptions.vue（分享链接）各写了一套，失败文案还停在
// `t('common.copy') + ' ✗'` —— 把按钮名（"复制"）当失败提示，而且那个 ✗ 是硬编码的，
// 8 语言下都长一样。统一到这里后，调用方只需决定「要不要 toast」。
//
// 注意：本文件依赖 store.showToast，所以禁止 store.ts 反向 import 本文件（会成环）。
import { t } from '../locales';
import { showToast } from '../store';
import { reportError } from './report';

/** t() 在 key 缺失时原样返回 key（locales/index.ts:64-67），据此回退到内置文案 */
const tr = (key: string, fallback: string) => {
  const value = t(key);
  return value === key ? fallback : value;
};

/**
 * 只复制、不弹提示：给需要自己决定反馈形式的调用方
 * （例如订阅页把结果写进行内状态 / 受管模态）。
 * @returns 是否成功；失败时已经通过 reportError 留痕
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (e) {
    // WebView2 在窗口未聚焦（NotAllowedError）或权限策略禁用剪贴板时会抛错，
    // 属于可预期失败：留痕后由调用方决定怎么告诉用户。
    reportError(e, 'clipboard.write');
    return false;
  }
}

/**
 * 复制 + toast 反馈：给「点一下就要看到结果」的场景（日志页 / 诊断页）。
 * @param okMsg 成功文案，省略时用 common.copied
 */
export async function copyText(text: string, okMsg?: string): Promise<boolean> {
  const ok = await copyToClipboard(text);
  if (ok) {
    showToast(okMsg ?? tr('common.copied', '已复制到剪贴板'), 'success');
  } else {
    // 失败文案必须说清"复制失败"，而不是复用按钮名 + 一个硬编码符号
    showToast(tr('common.copyFailed', '复制失败，请手动复制'), 'error');
  }
  return ok;
}
