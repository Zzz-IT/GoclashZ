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
        <SettingsTheme v-else-if="view === 'theme'" :behavior="behavior" :loading="behaviorLoading" @navigate="onNavigate" @save="saveBehavior" />
        <SettingsBehavior v-else-if="view === 'behavior'" :behavior="behavior" :loading="behaviorLoading" :reset-key="resetVersions.behavior" @navigate="onNavigate" @reset="confirmReset" @save="saveBehavior" @startup-change="handleStartupWithOSChange" />
        <SettingsAbout v-else-if="view === 'about'" :behavior="behavior" :loading="behaviorLoading" @navigate="onNavigate" @save="saveBehavior" @reload-behavior="loadBehavior" />
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
/* resetVersions 只覆盖「走后端 ResetComponentSettings 的段落」：Go 侧 switch 只有
   tun/dns/network/behavior 四个分支，没有 theme —— 主题重置由 SettingsTheme 自己在前端完成。
   原来还留了 theme: 0 与 modules.theme，但对应的 emit('reset','theme') 从没被发出过（G-15）。 */
const resetVersions = ref({ tun: 0, dns: 0, network: 0, lan: 0, behavior: 0 });
const modules = computed<Record<string, string>>(() => ({
  network: t('settings.home.network'),
  dns: t('settings.home.dns'),
  tun: t('settings.home.tun'),
  lan: t('settings.home.lan'),
  behavior: t('settings.home.behavior'),
}));
/* 兜底默认值必须与 Go core/appcore/behavior.go 的 Default() 逐字段对齐（G-06 / U-20）：
   GetAppBehavior 走 utils.LoadSetting，永不返回错误，这些兜底值正常环境是死代码；
   但只要它们与 Go 默认值不同，一旦真被写盘就会把 logLevel(error→info)、
   delayRetention(true→false)、updateInterval(1→3) 悄悄改成兜底值。
   4 个分级玻璃强度字段（glassBlurStrength / Content / Overlay / Control）前端已无消费者，不再列出（G-23）。 */
const behavior = ref<any>({ language: currentLocale.value, silentStart: false, closeToTray: true, startupWithOS: false, restoreOnStartup: false, portProxyMode: false, colorDelay: false, delayRetention: true, delayRetentionTime: 'long', proxyTrafficOnly: false, logLevel: 'error', appLogLevel: 'info', hideLogs: false, subUA: 'clash-verge', activeConfig: '', activeMode: 'rule', geoIpLink: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb', geoSiteLink: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat', mmdbLink: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb', asnLink: 'https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb', autoUpdate: true, updateMethod: 'startup', updateInterval: 1, lastUpdateCheck: 0, autoDelayTest: false, autoDelayTestInterval: 60, coreComparisonPrompt: true, longConnectionProtection: true, deferRestartWhenActive: true, longConnectionMinSeconds: 60, themeMode: 'light', themePreset: 'graphite', themeAccent: '', themeRadius: 'soft', themeAura: true, glassBlurEnabled: false, glassTintStrength: 58, glassDepth: 50 });

const behaviorLoading = ref(true);

/* ===== G-04：behavior 的「读-改-写」约定 =====
   后端 SaveAppBehavior 是整份结构覆盖（BehaviorStore.SetAndSave），不是字段合并；
   而同一份 user_behavior.json 还有别的写入者：侧栏明暗 setThemeMode（单字段读改写）、
   备份还原后端的 Behavior.Load()、启动项状态同步。
   所以保存前必须重读后端最新值、只把自己真正改过的字段盖上去，否则会把别人刚写的字段回滚。
   loadedSnapshot 就是「上一次从后端读到的基线」，用它与当前 behavior 做差得到脏字段。 */
let loadedSnapshot: Record<string, any> | null = null;

/** 字段级浅比较：AppBehavior 目前只有标量与字符串，数组/对象按 JSON 值比较以防后端将来加结构字段 */
const sameFieldValue = (a: any, b: any) => {
  if (a === b) return true;
  if (a && b && typeof a === 'object' && typeof b === 'object') return JSON.stringify(a) === JSON.stringify(b);
  return false;
};

/** 只挑出相对基线真正改过的字段；没有基线时退回整份（等价于修复前的行为） */
const pickDirtyFields = (base: Record<string, any> | null, next: Record<string, any>): Record<string, any> => {
  if (!base) return { ...next };
  const patch: Record<string, any> = {};
  for (const key of Object.keys(next)) {
    /* revision 是乐观锁版本号，必须排除：
       它由服务端独占推进，本地基线在保存后会「前进」，若把它当成脏字段，
       提交时就会用本地旧版本覆盖 latest.revision，CAS 永远失败（自锁）。 */
    if (key === 'revision') continue;
    if (!sameFieldValue(base[key], next[key])) patch[key] = next[key];
  }
  return patch;
};

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
      // 基线 = 刚读到的后端值（此后对 behavior 的改动都算「本地脏字段」）
      loadedSnapshot = { ...value };
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
/**
 * 串行化：连点两个开关时，后一次必须在前一次落盘之后再去读基线，
 * 否则两次都基于旧快照、后写的会把先写的脏字段覆盖回去（G-04）。
 */
let behaviorSaveChain: Promise<void> = Promise.resolve();
const saveBehavior = (): Promise<void> => {
  behaviorSaveChain = behaviorSaveChain.then(runSaveBehavior, runSaveBehavior);
  return behaviorSaveChain;
};

const runSaveBehavior = async () => {
  // 加载未完成时控件本应禁用，这里再兜一层：别把兜底对象里的假值整份写盘（G-13）
  if (behaviorLoading.value) return;
  // 先拍下本地快照再 await：等在 Get/SAVE 期间又改了字段，那些改动仍然算脏，
  // 交给链上的下一次保存处理，不会被"基线前进"吞掉（G-04）
  const localSnapshot: Record<string, any> = { ...behavior.value };
  const dirty = pickDirtyFields(loadedSnapshot, localSnapshot);
  // payload 是「后端最新值 + 本地脏字段」的普通对象，最终会整份交给 SaveAppBehavior；
  // 用 any 承接是为了给 AppBehavior 的字段做增量合并（本页 behavior 本身也是宽松类型）。
  let payload: any = { ...localSnapshot };
  if (loadedSnapshot) {
    try {
      // 只把本地脏字段合并到后端最新值上，而不是整份覆盖（G-04）
      const latest: any = await App.GetAppBehavior();
      if (latest) payload = { ...latest, ...dirty };
    } catch (error) {
      // 读不到最新值就退回整份写回：宁可暂时冒覆盖风险，也别让用户的改动丢失
      reportError(error, 'settings.saveBehavior.reread');
    }
  }
  try {
    /* 乐观锁重试（BEHAVIOR_CONFLICT）：我们拿到 latest 到真正落盘之间，别的入口
       （侧栏明暗 / 托盘 / 自启同步）可能又写了一次 —— 此时后端用 revision 拒绝我们的提交。
       这不是错误，重新读最新版本、把同一批脏字段再合并一次即可，用户完全无感。 */
    try {
      await App.SaveAppBehavior(payload);
    } catch (conflict: any) {
      const msg = String((conflict && (conflict.message || conflict.error)) || conflict || '');
      if (!msg.includes('BEHAVIOR_CONFLICT')) throw conflict;
      const latest: any = await App.GetAppBehavior();
      if (!latest) throw conflict;
      payload = { ...latest, ...dirty };
      await App.SaveAppBehavior(payload);
      // 重试成功：基线跟到最新（本地 revision 由下面统一推进）
    }
    // 保存成功后基线跟到「本次提交的那份」：脏字段清零，下次只提交新的改动。
    // 不把 payload 反写回 behavior —— 非脏字段（别人改的）保持界面原样，等下次 loadBehavior 再对齐。
    // revision 由服务端独占推进，这里按「提交的版本 + 1」跟进；若仍有跨入口写入，
    // 下一次保存会被 CAS 拦下并走上面的无感重试，所以不需要为它多打一次 RPC。
    loadedSnapshot = { ...localSnapshot, revision: (Number(payload?.revision) || 0) + 1 };
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
