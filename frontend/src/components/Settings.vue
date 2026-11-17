<template>
  <div class="settings-container ui-page-shell" :class="[edgeFadeAxis, { 'edge-fade-on': edgeFadeOn }]">
    <Transition :name="viewDirection === 'back' ? 'pane-right' : 'pane-left'">
      <div :key="view" class="settings-view-wrapper">
        <SettingsHome v-if="view === 'main'" @navigate="onNavigate" @enter-uwp="handleEnterUwp" />
        <SettingsUpdate v-else-if="view === 'update'" :behavior="behavior" @navigate="onNavigate" @save-behavior="saveBehavior" />
        <SettingsTun v-else-if="view === 'tun'" :reset-key="resetVersions.tun" @navigate="onNavigate" @reset="confirmReset" />
        <SettingsDns v-else-if="view === 'dns'" :reset-key="resetVersions.dns" @navigate="onNavigate" @reset="confirmReset" />
        <SettingsNetwork v-else-if="view === 'network'" :reset-key="resetVersions.network" @navigate="onNavigate" @reset="confirmReset" />
        <SettingsLan v-else-if="view === 'lan'" :reset-key="resetVersions.lan" @navigate="onNavigate" @reset="confirmReset" />
        <SettingsTheme v-else-if="view === 'theme'" :behavior="behavior" :loading="behaviorLoading" :reset-key="resetVersions.theme" @navigate="onNavigate" @reset="confirmReset" @save="saveBehavior" />
        <SettingsBehavior v-else-if="view === 'behavior'" :behavior="behavior" :loading="behaviorLoading" :reset-key="resetVersions.behavior" @navigate="onNavigate" @reset="confirmReset" @save="saveBehavior" @startup-change="handleStartupWithOSChange" />
        <SettingsAbout v-else-if="view === 'about'" :behavior="behavior" :loading="behaviorLoading" @navigate="onNavigate" @save="saveBehavior" />
        <SettingsDiagnostics v-else-if="view === 'diagnostics'" @navigate="onNavigate" />
        <SettingsUwp v-else-if="view === 'uwp'" :entry="uwpEntry" @navigate="onNavigate" />
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue';
import { App } from '../api';
import { globalState, showAlert, showConfirm, showToast, syncThemeFromBehavior } from '../store';
import { useEdgeFade } from '../utils/edgeFade';
import { displayErrorMessage, toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import { t, setLocale, currentLocale, SupportedLocale } from '../locales';
import SettingsAbout from './SettingsAbout.vue';
import SettingsBehavior from './SettingsBehavior.vue';
import SettingsDiagnostics from './SettingsDiagnostics.vue';
import SettingsDns from './SettingsDns.vue';
import SettingsHome from './SettingsHome.vue';
import SettingsNetwork from './SettingsNetwork.vue';
import SettingsLan from './SettingsLan.vue';
import SettingsTheme from './SettingsTheme.vue';
import SettingsTun from './SettingsTun.vue';
import SettingsUpdate from './SettingsUpdate.vue';
import SettingsUwp from './SettingsUwp.vue';

const props = defineProps({ initialView: { type: String, default: 'main' } });
const view = ref(props.initialView as 'main' | 'uwp' | 'tun' | 'dns' | 'network' | 'lan' | 'theme' | 'behavior' | 'update' | 'about' | 'diagnostics');
const uwpEntry = ref(0);

/** 设置页过渡方向：进子页=前进（从右来），回上一级=后退（从左来），同级之间按前进处理 */
const viewDirection = ref<'forward' | 'back'>('forward');

/** 二级子页的上一级：诊断页挂在「关于应用」下面，返回要回关于而不是设置首页 */
const PAGE_PARENT: Partial<Record<string, string>> = { diagnostics: 'about' };

/* 设置子页固定横向切换。撑住与收起全部由 useEdgeFade 的计时器驱动，
   不接过渡结束钩子：连切时旧过渡的 after-leave 会把当前的带子提前收掉。
   方向与淡化侧必须合在一个 watch 里：先定方向，再据方向选淡哪一侧 ——
   分成两个 watch 的话，先跑的那个读到的还是上一次的方向。 */
const {
  axis: edgeFadeAxis,
  on: edgeFadeOn,
  begin: edgeFadeBegin,
} = useEdgeFade();

watch(view, (next, prev) => {
  const isBack = (next === 'main' && prev !== 'main') || PAGE_PARENT[prev] === next;
  viewDirection.value = isBack ? 'back' : 'forward';
  edgeFadeBegin('edge-fade-x');
});

const handleEnterUwp = async (anchor?: string) => {
  if (!globalState.isAdmin) {
    const restart = await showConfirm(
      t('settings.uwp.adminPromptMsg'),
      t('settings.uwp.adminPromptTitle'),
      false,
      { confirmText: t('settings.uwp.adminRestartBtn'), cancelText: t('settings.uwp.adminViewBtn') }
    );
    if (restart) {
      try {
        await App.RestartAsAdmin();
      } catch (e: any) {
        const u = toUserError(e);
        await showAlert(displayErrorMessage(e), t('common.error'), true, u.detail);
      }
      return;
    }
  }
  view.value = 'uwp';
  uwpEntry.value += 1;
  // 搜索跳转进入 UWP 时也要定位到具体字段
  if (anchor) {
    pendingAnchor.value = anchor;
    nextTick(() => applyAnchor());
  }
};
// ===== 设置项搜索跳转：切页后定位到具体字段并短暂高亮 =====
const pendingAnchor = ref('');

const applyAnchor = (retry = 0) => {
  const id = pendingAnchor.value;
  if (!id) return;
  const el = document.getElementById(id);
  if (!el) {
    // 目标项可能因 v-if 未渲染：重试几次后放弃，回到顶部并告知用户
    if (retry < 6) {
      setTimeout(() => applyAnchor(retry + 1), 120);
    } else {
      pendingAnchor.value = '';
      document
        .querySelector<HTMLElement>('.settings-container .ui-page-body')
        ?.scrollTo({ top: 0, behavior: 'smooth' });
      showToast(t('diag.searchHiddenItem'), 'info');
    }
    return;
  }

  /* 落位必须直接写 scrollTop，而不是 scrollIntoView：
     scrollIntoView 会连带滚动所有可滚祖先，而且只能"跳"。
     真正的滚动容器是页面的 .ui-page-body，算出目标相对它的偏移再赋值即可。

     retry 0 发生在 nextTick 里、首帧绘制之前，所以页面是"直接出现在目标位置"，
     不会出现"先渲染在页顶、再瞬移过去"的顿挫。
     重试阶段元素是后来才渲染出来的，落位已经晚于首帧，
     这时用平滑滚动把跳跃变成可见的位移，比瞬移好。 */
  const behavior: ScrollBehavior = retry === 0 ? 'auto' : 'smooth';
  const scroller = el.closest<HTMLElement>('.ui-page-body');
  if (scroller) {
    const top =
      el.getBoundingClientRect().top -
      scroller.getBoundingClientRect().top +
      scroller.scrollTop;
    scroller.scrollTo({ top, behavior });
  } else {
    el.scrollIntoView({ block: 'start', behavior });
  }

  // 先移除再强制 reflow：1.9s 内连续跳同一项也能重新播放高亮
  el.classList.remove('setting-flash');
  void el.offsetWidth;
  el.classList.add('setting-flash');
  setTimeout(() => el.classList.remove('setting-flash'), 1900);
  pendingAnchor.value = '';
};

const onNavigate = (next: string, anchor?: string) => {
  // 搜索直达 UWP 也必须走管理员鉴权，不能绕过 handleEnterUwp
  if (next === 'uwp') {
    void handleEnterUwp(anchor);
    return;
  }
  view.value = next as typeof view.value;
  if (!anchor) return;
  pendingAnchor.value = anchor;
  // 等子页挂载完成即可 —— 此时还没绘制第一帧，落位不会被看到
  nextTick(() => applyAnchor());
};

const resetModule = ref('');
const resetModuleName = ref('');
const resetVersions = ref({ tun: 0, dns: 0, network: 0, lan: 0, theme: 0, behavior: 0 });
const modules = computed<Record<string, string>>(() => ({
  network: t('settings.home.network'),
  dns: t('settings.home.dns'),
  tun: t('settings.home.tun'),
  lan: t('settings.home.lan'),
  theme: t('settings.theme.title'),
  behavior: t('settings.home.behavior'),
}));
const behavior = ref<any>({ language: currentLocale.value, silentStart: false, closeToTray: true, startupWithOS: false, restoreOnStartup: false, colorDelay: false, delayRetention: false, delayRetentionTime: 'long', proxyTrafficOnly: false, logLevel: 'info', appLogLevel: 'info', hideLogs: false, subUA: '', activeConfig: '', activeMode: '', geoIpLink: '', geoSiteLink: '', mmdbLink: '', asnLink: '', autoUpdate: true, updateMethod: 'startup', updateInterval: 3, coreComparisonPrompt: true, longConnectionProtection: true, deferRestartWhenActive: true, longConnectionMinSeconds: 60, themeMode: 'light', themePreset: 'graphite', themeAccent: '', themeRadius: 'soft', themeAura: true, glassBlurEnabled: false, glassBlurStrength: 12, glassBlurContentStrength: 12, glassBlurOverlayStrength: 18, glassBlurControlStrength: 8, glassTintStrength: 58, glassDepth: 50 });

const behaviorLoading = ref(true);

const loadBehavior = async () => {
  try {
    const value = await App.GetAppBehavior();
    if (value) {
      if (value.logLevel === 'warn') {
        value.logLevel = 'warning';
      }
      if (value.coreComparisonPrompt === undefined) {
        value.coreComparisonPrompt = true;
      }
      if (value.longConnectionProtection === undefined) {
        value.longConnectionProtection = true;
      }
      behavior.value = value;
      // 后端配置（重置 / 托盘侧改动 / 语言切换触发的 reload）重新加载后，
      // 主题与玻璃必须跟着重算，否则界面会停在旧外观。统一走 syncThemeFromBehavior。
      syncThemeFromBehavior(behavior.value);
      if (value.language) {
        setLocale(value.language as SupportedLocale);
      }
    }
  } catch (error) {
    reportError(error, 'settings.loadBehavior');
  } finally {
    // 必须等浏览器真正绘制一帧（rAF）后再解除 no-anim。
    // 若用 nextTick，解除发生在绘制之前，浏览器首次绘制时 transition 已恢复，
    // 会把"默认值→真实值"当成状态变化播成滑动动画，抑制形同无效。
    requestAnimationFrame(() => {
      requestAnimationFrame(() => { behaviorLoading.value = false; });
    });
  }
};
const saveBehavior = async () => {
  try {
    await App.SaveAppBehavior(behavior.value);
    if (behavior.value.language) {
      setLocale(behavior.value.language as SupportedLocale);
    }
    if (behavior.value.appLogLevel) globalState.appLogLevel = behavior.value.appLogLevel;
    if (behavior.value.logLevel) globalState.logLevel = behavior.value.logLevel;
  } catch (error: any) {
    reportError(error, 'settings.saveBehavior');
    const u = toUserError(error);
    showAlert(t('settings.saveFailed', { error: displayErrorMessage(error) }), t('common.error'), true, u.detail);
    await loadBehavior();
  }
};
const handleStartupWithOSChange = async () => { if (!behavior.value.startupWithOS) behavior.value.restoreOnStartup = false; await saveBehavior(); };
// 重置确认统一走全局模态（危险操作 → isDanger=true），不再自建一份 UI
const confirmReset = async (module: string) => {
  resetModule.value = module;
  resetModuleName.value = modules.value[module] || module;
  const ok = await showConfirm(
    t('settings.resetConfirmMsg', { name: resetModuleName.value }),
    t('settings.resetConfirmTitle'),
    true
  );
  if (!ok) return;
  await handleReset();
};
const handleReset = async () => {
  try {
    // 'lan' settings are stored in NetworkConfig — reuse the network reset endpoint.
    const apiModule = resetModule.value === 'lan' ? 'network' : resetModule.value;
    await App.ResetComponentSettings(apiModule);
    resetVersions.value[resetModule.value as keyof typeof resetVersions.value] += 1;
    if (resetModule.value === 'behavior') await loadBehavior();
    // F-A4-12：成功用 toast
    showToast(t('settings.resetSuccess', { name: resetModuleName.value }), 'success');
  } catch (error) {
    reportError(error, 'settings.reset');
    // F-A4-13：人话进正文、原始文本进 detail
    const u = toUserError(error);
    showAlert(t('settings.resetFailed', { error: displayErrorMessage(error) }), t('common.error'), true, u.detail);
  }
};

void loadBehavior();
watch(() => props.initialView, value => { view.value = value as typeof view.value; });
/* 不再在切页后把滚动容器拉回顶部：
   子页是 :key="view" 整块重建的，新页的 .ui-page-body 天然从 0 开始；
   而原来那句是往 .view-scroller 上写的，它早已不滚动，属于死代码。
   真需要归位时由 applyAnchor 精确处理。 */
</script>

<style scoped>
.settings-container{position:relative;display:flex;flex-direction:column;flex:1 1 auto;min-height:0}.settings-view-wrapper{position:absolute;inset:0;display:flex;flex-direction:column;width:100%;height:100%;flex:1 1 auto;min-height:0}.modal-footer .action-btn,.modal-footer .primary-btn{height:42px;display:flex;align-items:center;justify-content:center}
</style>
