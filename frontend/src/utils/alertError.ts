// 错误模态的唯一入口（R-4 / F-A4-13）。
//
// 审计 R-4 点名的 9 个错误提示点里有 6 处（Rules 3 / Subscriptions 3）是逐字相同的模板：
//   const u = toUserError(e);
//   await showAlert(t(key, { error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
// 其中标题恒为 common.error、恒为危险态、detail 恒取人话转换后的原文，
// 这里收敛成一个函数，调用点只保留「用哪个 key」。其余 3 处模板不同（见 fix-r1-common.md）。
//
// 注意：本文件依赖 store.showAlert，所以禁止 store.ts 反向 import 本文件（会成环）。
import { t } from '../locales';
import { showAlert } from '../store';
import { displayErrorMessage, toUserError } from './errors';

/**
 * 弹「错误」模态：标题固定 common.error、危险态，正文为 t(key, { error: 人话错误 })、
 * 折叠区为原始错误文本。与被替换的模板逐项等价（同样一次 showAlert、同样文案、同样去重行为）。
 *
 * @param e      捕获到的异常（任意类型）
 * @param key    i18n 文案 key，其 {error} 槽位由本函数填充
 * @param params 其它插值（可选，如 { name }）；error 槽位始终以人话错误文本为准
 */
export async function alertError(
  e: unknown,
  key: string,
  params?: Record<string, string | number>
): Promise<void> {
  const u = toUserError(e);
  // params 先铺、error 后写：即使调用方误传 error 也不会覆盖人话错误文本
  await showAlert(t(key, { ...(params || {}), error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
}
