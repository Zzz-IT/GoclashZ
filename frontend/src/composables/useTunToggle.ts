// TUN 开关的唯一实现（Overview 首页卡片 / 设置→TUN 开关共用）。
//
// 背景（F-A4-03）：此前首页与设置页各写一套，首页有 helper 引导、驱动引导、
// 自动重试与错误分层，设置页只有一句「开启失败：<原始错误>」，且没有 pending
// 闸门、成功后不回灌后端状态。这里收敛为单一实现，行为对齐首页那套。
//
// 约定：
// - 只负责「开关动作」；UI（卡片/开关控件）自己负责视觉与 disabled 态。
// - pending 复用 globalState.tunPending（store.ts 的看门狗会对它做 60s 兜底复位）。
// - 语义判定走 classifyError；文案走 displayErrorMessage（保留人话映射）。
// - 只有 kind === 'wintun_missing' 才引导安装驱动；其它 wintun 问题一律不装。
import { computed, ref } from 'vue';
import { App } from '../api';
import {
  globalState,
  settleTunIntent,
  setTunIntent,
  showAlert,
  showConfirm,
  updateStateFromBackend,
} from '../store';
import { classifyError, displayErrorMessage, toUserError } from '../utils/errors';
import { t } from '../locales';

export type UseTunToggleOptions = {
  /**
   * 安装成功后的回调（用于刷新驱动状态 / 偏差条）。
   * 注意：安装驱动是异步任务，真正的完成信号是 store 里的 driver-install-success 事件，
   * 这里只做「立即刷新一次」的尽力而为。
   */
  onAfterInstall?: () => void | Promise<void>;
};

export function useTunToggle(options: UseTunToggleOptions = {}) {
  /** 与全局 pending 同步，供 UI 做 disabled / 转圈 */
  const pending = computed(() => globalState.tunPending);
  /** 最近一次失败的原因（已人话化），供页面内联展示（可选） */
  const lastError = ref('');

  const refreshState = async () => {
    const latest = await App.GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
  };

  /** 装完 helper / 驱动后的重试：失败只展示一次，不再递归引导 */
  const retryOnce = async (target: boolean): Promise<boolean> => {
    try {
      await App.ToggleTunMode(target);
      await refreshState();
      return true;
    } catch (e: any) {
      lastError.value = displayErrorMessage(e);
      const u = toUserError(e);
      void showAlert(t('overview.errToggleTun', { error: lastError.value }), t('common.error'), true, u.detail);
      return false;
    }
  };

  /**
   * 切换 TUN。
   * @param target 目标状态；缺省取当前 globalState.tun 的反面
   * @returns 是否最终处于 target 状态（用户取消引导 / 失败均为 false）
   */
  const toggle = async (target?: boolean): Promise<boolean> => {
    if (globalState.tunPending) return false;

    const next = target ?? !globalState.tun;
    lastError.value = '';
    setTunIntent(next);
    globalState.tunPending = true;

    try {
      await App.ToggleTunMode(next);
      await refreshState();
      return true;
    } catch (err: any) {
      const cls = classifyError(err);
      const u = toUserError(err);

      // 1) helper 未装 / 需修复：引导安装并自动重试（成败都给明确出口）
      if (cls.kind === 'helper_install_required' || cls.kind === 'helper_repair_required') {
        const confirmed = await showConfirm(t('overview.tunHelperConfirm'), t('overview.tunHelperTitle'));
        if (!confirmed) {
          lastError.value = displayErrorMessage(err);
          return false;
        }
        try {
          await App.InstallHelperService();
        } catch (e: any) {
          const iu = toUserError(e);
          lastError.value = iu.message;
          void showAlert(t('overview.errInstallHelper', { error: iu.message }), t('common.error'), true, iu.detail);
          return false;
        }
        await options.onAfterInstall?.();
        return await retryOnce(next);
      }

      // 2) 无活动配置：可直接告诉用户去订阅页选一个
      if (cls.kind === 'no_active_config') {
        lastError.value = displayErrorMessage(err);
        void showAlert(`${t('overview.errNoConfigTitle')}\n\n${t('overview.errNoConfigMsg')}`, t('common.notice'));
        return false;
      }

      // 3) 驱动文件确实不存在：引导安装 + 重试
      if (cls.kind === 'wintun_missing') {
        const confirmed = await showConfirm(t('overview.errWintunMissing'), t('common.error'), false);
        if (!confirmed) {
          lastError.value = displayErrorMessage(err);
          return false;
        }
        try {
          await App.InstallTunDriverAsync(true);
          await options.onAfterInstall?.();
          return await retryOnce(next);
        } catch (e: any) {
          const iu = toUserError(e);
          lastError.value = displayErrorMessage(e);
          void showAlert(lastError.value, t('common.error'), true, iu.detail);
          return false;
        }
      }

      // 4) wintun 不可用（被占用/校验失败）与 unknown：只展示，绝不装驱动（无硬编码文案）
      lastError.value = displayErrorMessage(err);
      void showAlert(t('overview.errToggleTun', { error: lastError.value }), t('common.error'), true, u.detail);
      return false;
    } finally {
      globalState.tunPending = false;
      // 关闭失败时清掉「期望关闭」的残留，避免卡片一直显示关闭中
      if (next === false) globalState.desiredTun = false;
      settleTunIntent();
    }
  };

  return { pending, lastError, toggle };
}
