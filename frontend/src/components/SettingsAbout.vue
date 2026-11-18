<template>
  <div class="settings-page ui-page-shell" :class="{ 'no-anim': loading }">
    <div class="sub-header section-header ui-page-header">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.about.title') }}</h3>
    </div>

    <div class="ui-page-body">
      <div class="glass-card setting-group ui-surface-group scrollable">
        <!-- 软件图标与名称展示行 -->
        <div class="setting-item" style="padding: 20px 0; display: flex; justify-content: space-between; align-items: center;">
          <div class="info" style="display: flex; align-items: center; gap: 18px;">
            <img :src="appLogo" style="width: 52px; height: 52px; border-radius: 12px;" />
            <h4 style="margin: 0; font-weight: 800; font-size: 1.6rem; letter-spacing: -0.01em;">GoclashZ</h4>
          </div>

          <!-- 新增：右侧空间显示后台静默下载进度 -->
          <!-- 新增：右侧空间显示后台静默下载进度 -->
          <div v-if="globalState.appUpdateProgress" class="app-update-progress-container"
               :class="{ 'clickable-progress': globalState.appUpdateProgress.isDownloaded }"
               @click="globalState.appUpdateProgress.isDownloaded ? promptInstallApp(globalState.appUpdateProgress) : null">
            <div class="progress-info">
              <span v-if="globalState.appUpdateProgress.isDownloaded" class="speed" style="color: var(--ui-accent); font-weight: 600;">新版本已就绪，点击安装</span>
              <template v-else>
                <span class="speed">{{ formatSpeed(globalState.appUpdateProgress.speedBps) }}</span>
                <span class="divider-dot">·</span>
                <span class="eta">剩余 {{ formatEtaTime(globalState.appUpdateProgress.etaSec) }}</span>
              </template>
            </div>
            <div class="progress-bar-wrap">
              <div class="progress-bar-fill" :style="{ width: appUpdatePercent + '%', backgroundColor: globalState.appUpdateProgress.isDownloaded ? 'var(--ui-accent)' : '' }"></div>
            </div>
            <div class="progress-size">
              <span v-if="globalState.appUpdateProgress.isDownloaded">{{ globalState.appUpdateProgress.version }} 下载完成</span>
              <span v-else>{{ formatBytes(globalState.appUpdateProgress.bytesDone) }} / {{ formatBytes(globalState.appUpdateProgress.totalBytes) }}</span>
            </div>
          </div>
        </div>

        <div class="divider"></div>
        <div class="setting-item" id="set-about-language">
          <div class="info">
            <h4>{{ t('settings.about.language') }}</h4>
            <p>{{ t('settings.about.languageDesc') }}</p>
          </div>
          <!-- G-19：走 v-model + @change（用户点选才触发），语言由 Settings.saveBehavior 成功后统一 setLocale -->
          <ModernSelect
            v-model="behavior.language"
            :options="SUPPORTED_LOCALES"
            :disabled="loading"
            @change="handleLanguageChange"
          />
        </div>

        <div class="divider"></div>
        <div class="setting-item" id="set-about-version">
          <div class="info">
            <h4>{{ t('settings.about.version') }}</h4>
            <p>{{ globalState.appVersion || t('settings.about.fetchingVersion') }}</p>
          </div>
          <button class="action-btn accent-btn" @click="handleCheckUpdate" :disabled="globalState.appUpdateChecking">
            {{ globalState.appUpdateChecking ? t('settings.about.checking') : t('settings.about.checkUpdate') }}
          </button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-about-autoUpdate">
          <div class="info">
            <h4>{{ t('settings.about.autoUpdate') }}</h4>
            <p>{{ t('settings.about.autoUpdateDesc') }}</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="behavior.autoUpdate" :disabled="loading" @change="$emit('save')" />
            <span class="slider"></span>
          </label>
        </div>

        <Transition :name="isInitialized ? 'dropdown' : ''">
          <div v-if="behavior.autoUpdate" class="auto-update-sub-items">
            <div class="divider"></div>
            <div class="setting-item" id="set-about-updateMethod">
              <div class="info">
                <h4>{{ t('settings.about.updateMethod') }}</h4>
              </div>
              <ModernSelect
                v-model="behavior.updateMethod"
                :options="[
                  { label: t('settings.about.startup'), value: 'startup' },
                  { label: t('settings.about.scheduled'), value: 'scheduled' }
                ]"
                :disabled="loading"
                @change="$emit('save')"
              />
            </div>

            <Transition :name="isInitialized ? 'dropdown' : ''">
              <div v-if="behavior.updateMethod === 'scheduled'" class="update-interval-sub-items">
                <div class="divider"></div>
                <div class="setting-item" id="set-about-updateInterval">
                  <div class="info">
                    <h4>{{ t('settings.about.updateInterval') }}</h4>
                  </div>
                  <div class="input-with-unit">
                    <ModernNumberInput
                      v-model="behavior.updateInterval"
                      :min="1"
                      :max="365"
                      :disabled="loading"
                      @change="$emit('save')"
                    />
                    <span class="unit">{{ t('settings.about.daysUnit') }}</span>
                  </div>
                </div>
              </div>
            </Transition>
          </div>
        </Transition>

        <div class="divider"></div>

        <div class="setting-item" id="set-about-exportBackup">
          <div class="info">
            <h4>{{ t('settings.about.exportBackup') }}</h4>
            <p>{{ t('settings.about.exportBackupDesc') }}</p>
          </div>
          <button class="action-btn accent-btn" @click="handleExportBackup">{{ t('settings.about.exportBtn') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-about-restoreBackup">
          <div class="info">
            <h4>{{ t('settings.about.restoreBackup') }}</h4>
            <p>{{ t('settings.about.restoreBackupDesc') }}</p>
          </div>
          <button class="action-btn accent-btn" @click="openRestoreModal">{{ t('settings.about.restoreBtn') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item clickable" id="set-about-diagnostics" @click="emit('navigate', 'diagnostics')">
          <div class="info">
            <h4>{{ t('settings.about.diagnostics') }}</h4>
            <p>{{ t('settings.about.diagnosticsDesc') }}</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-about-githubRepo">
          <div class="info">
            <h4>{{ t('settings.about.githubRepo') }}</h4>
            <a href="javascript:void(0)" @click="openLink('https://github.com/Zzz-IT/GoclashZ')" class="link-item">https://github.com/Zzz-IT/GoclashZ</a>
          </div>
        </div>
      </div>
    </div>

    <!-- 还原备份弹窗 -->
    <Teleport to="body">
    <Transition name="pop">
      <div v-if="showRestoreModal" class="modal-overlay" @click.self="showRestoreModal = false">
        <div class="custom-modal-card" style="overflow: visible;" @click.stop>
          <div class="modal-header">
            <h3>{{ t('settings.about.restoreTitle') }}</h3>
          </div>
          <div class="modal-body" style="overflow: visible;">
            <p class="global-modal-msg">{{ t('settings.about.restorePrompt') }}</p>

            <div class="restore-actions" style="width: 100%; display: flex; flex-direction: column; gap: 4px;">
              <button class="action-btn w-full-btn hover-accent" @click="handleSelectFile" :class="{'active-border': selectedPath}" style="width: 100%; box-sizing: border-box;">
                <span class="btn-icon" v-html="ICONS.folder" style="margin-right: 4px;"></span>
                <span class="truncate" style="flex: 1; text-align: center;">
                  {{ selectedPath ? t('settings.about.selectedFile', { name: selectedPath.split('\\').pop() || '' }) : t('settings.about.browseBackupFile') }}
                </span>
              </button>

              <div class="divider-text" style="margin: 12px 0">{{ t('settings.about.restoreModeTitle') }}</div>

              <div class="mode-selector-group" style="width: 100%;">
                <ModernSelect
                  class="w-full"
                  v-model="restoreMode"
                  :options="restoreModeOptions"
                />
              </div>
            </div>

            <div class="modal-footer">
              <button class="action-btn flex-1" :disabled="isRestoring" @click="showRestoreModal = false">{{ t('common.cancel') }}</button>
              <button class="primary-btn accent-btn flex-1" :disabled="!selectedPath || isRestoring" @click="confirmRestore">{{ isRestoring ? t('common.loading') : t('settings.about.executeRestore') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import { App } from '../api';
import { Browser, Events } from '@wailsio/runtime';
import { globalState, showAlert, showToast, showConfirm } from '../store';
import { formatBytes, formatEtaTime, formatSpeed } from '../utils/format';
import { ICONS } from '../utils/icons';
import { toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import appLogo from '../assets/logo.ico';
import ModernNumberInput from './ModernNumberInput.vue';
import ModernSelect from './ModernSelect.vue';
import { t, SUPPORTED_LOCALES } from '../locales';

defineProps<{ behavior: Record<string, any>; loading?: boolean }>();
const emit = defineEmits<{
  navigate: [view: 'main' | 'diagnostics'];
  save: [];
  /** 还原备份后请父级重新 GetAppBehavior（G-05）：否则前端快照会把还原结果覆盖回旧值 */
  'reload-behavior': [];
}>();

const isInitialized = ref(false);

onMounted(async () => {
  await nextTick();
  isInitialized.value = true;
});

/**
 * 语言选择（G-19）：这里只通知父级保存，不立刻 setLocale。
 * 由 Settings.saveBehavior 在 SaveAppBehavior 成功后才统一 setLocale；保存失败时它会
 * loadBehavior()，界面语言随之回退到后端值 —— 原来的写法先切语言再保存，失败时用户会看到语言当面跳回去。
 */
const handleLanguageChange = () => {
  emit('save');
};

const showRestoreModal = ref(false);
const selectedPath = ref("");
const restoreMode = ref("all");
const isRestoring = ref(false);

const restoreModeOptions = computed(() => [
  {
    label: t('settings.about.restoreModeAll'),
    value: 'all',
    description: t('settings.about.restoreModeAllDesc')
  },
  {
    label: t('settings.about.restoreModeSubs'),
    value: 'subs',
    description: t('settings.about.restoreModeSubsDesc')
  },
  {
    label: t('settings.about.restoreModeSubsMerge'),
    value: 'subs-merge',
    description: t('settings.about.restoreModeSubsMergeDesc')
  },
  {
    label: t('settings.about.restoreModeSettings'),
    value: 'settings',
    description: t('settings.about.restoreModeSettingsDesc')
  }
]);

const openLink = (url: string) => {
  // Browser.OpenURL 返回 Promise：不接拒绝会产生未处理的 Promise 拒绝（G-20）
  Browser.OpenURL(url).catch((e: unknown) => reportError(e, 'about.openLink'));
};

const handleCheckUpdate = async () => {
  if (globalState.appUpdateChecking) return;
  try {
    // 🚀 核心改进：调用异步静默下载流，将通知权交给全局监听器 (App.vue)
    await App.CheckAndDownloadAppUpdateAsync();
  } catch (e) {
    const u = toUserError(e);
    await showAlert(u.message, t('common.error'), true, u.detail);
  }
};

const promptInstallApp = async (progress: any) => {
  const version = progress.version || "";
  const fullPath = progress.path || "";

  const ok = await showConfirm(
      t('settings.about.installAppConfirmMsg', { version }),
      t('settings.about.installAppConfirmTitle'),
      false
  );

  if (ok) {
      if (!fullPath) {
        await showAlert(t('settings.about.installAppPathEmpty'), t('common.error'), true);
        return;
      }
      try {
        await App.ApplyAppUpdate(fullPath);
      } catch (e: any) {
        await showAlert(String(e?.message || e || t('common.error')), t('common.error'), true);
      }
  }
};

// 导出备份
const handleExportBackup = async () => {
  try {
    const res = await App.ExportBackup();
    if (res === "SUCCESS") {
      showToast(t('settings.about.exportSuccess'), 'success');
    } else if (res !== "CANCELLED") {
      // G-20：只认 SUCCESS 字面量会让其它返回值（含将来新增的状态）静默失败，
      // 用户不知道备份其实没生成。用户主动取消仍按静默处理。
      showToast(t('settings.about.exportFailed', { error: String(res) }), 'error');
    }
  } catch (e) {
    const u = toUserError(e);
    await showAlert(u.message, t('common.error'), true, u.detail);
  }
};

// 打开还原面板
const openRestoreModal = () => {
  selectedPath.value = "";
  restoreMode.value = "all";
  showRestoreModal.value = true;
};

// 选择还原文件
const handleSelectFile = async () => {
  try {
    const path = await App.SelectBackupFile();
    if (path) {
      selectedPath.value = path;
    }
  } catch (e) {
    // 选择文件对话框的取消也走 reject，这里只记录不弹窗
    reportError(e, 'about.selectBackupFile');
  }
};

// 执行还原
const confirmRestore = async () => {
  if (!selectedPath.value || isRestoring.value) return;

  const warnings: Record<string, string> = {
    all: t('settings.about.restoreModeAllDesc'),
    subs: t('settings.about.restoreModeSubsDesc'),
    'subs-merge': t('settings.about.restoreModeSubsMergeDesc'),
    settings: t('settings.about.restoreModeSettingsDesc')
  };

  /* t() 在键缺失时返回键路径本身（见 locales/index.ts），所以原来的「|| 中文兜底」
     分支永远不会执行，属死代码（G-20）；这里只保留"模式描述缺失时回落到默认确认语"这一处真兜底。 */
  const confirmMsg = warnings[restoreMode.value] || t('settings.about.restoreConfirmDefault');

  const ok = await showConfirm(
    confirmMsg + "\n\n" + t('settings.about.restoreNotice'),
    t('settings.about.restoreConfirmTitle'),
    true
  );
  if (!ok) return;

  isRestoring.value = true;
  try {
    const res = await App.ExecuteRestore(selectedPath.value, restoreMode.value);
    if (res === "SUCCESS") {
      showRestoreModal.value = false;
      /* G-05：后端 RestoreBackup 成功后会 Behavior.Load() 换掉内存里的 behavior，
         但前端 behavior 快照还是还原前的对象。不请父级重读的话，用户改任意一个开关
         saveBehavior 就会把还原结果整份覆盖回旧值（标题/玻璃/语言/日志级别一起回滚）。
         文案也随之改成"已还原并重载"，不再宣称"即时生效"。 */
      emit('reload-behavior');
      // 文案键不变，Wave 2 把「即时生效」改成「已还原并重载」（见 i18n-needs），避免新增 key 让 check-i18n 变红
      showToast(t('settings.about.restoreSuccess'), 'success');
    } else if (res !== "CANCELLED") {
      // G-20：同导出，非 SUCCESS/CANCELLED 的返回值必须让用户看见
      await showAlert(t('settings.about.restoreFailed', { error: String(res) }), t('common.error'), true);
    }
  } catch (e) {
    const u = toUserError(e);
    await showAlert(u.message, t('common.error'), true, u.detail);
  } finally {
    isRestoring.value = false;
  }
};

const appUpdatePercent = computed(() => {
  const p = globalState.appUpdateProgress;
  if (!p || !p.totalBytes) return 0;
  return Math.min(100, Math.floor((p.bytesDone / p.totalBytes) * 100));
});

const unsubAppUpdateBusy = Events.On("app-update-busy", () => {
  globalState.appUpdateChecking = false;
  void showAlert(t('settings.update.appUpdateBusy'), t('common.confirm'));
});

onUnmounted(() => {
  unsubAppUpdateBusy();
});
</script>

<style scoped>
.clickable-progress {
  cursor: pointer;
  padding: 10px 14px;
  border-radius: var(--ui-radius-lg);
  transition: background 0.2s;
  margin-right: -14px;
  margin-top: -10px;
  margin-bottom: -10px;
}
.clickable-progress:hover {
  background: var(--ui-surface-hover);
}

.settings-page { flex: 1 1 auto; min-width: 0; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.setting-group.scrollable { padding-bottom: 20px; }

h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-sub);
  max-width: 100%;
  line-height: 1.5;
}

.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
/* 可点击行沿用设置首页的入口样式：整行可点，右侧箭头提示会进入子页 */
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: var(--ui-radius-lg); margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--ui-surface-hover); }
.arrow { color: var(--text-sub); font-size: 1.2rem; }
.divider { height: 1px; background: var(--ui-border-subtle); opacity: 0.5; margin: 0; }

.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--ui-surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);}
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--ui-accent-block); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--ui-accent-fg); }

.sub-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 0 0 12px 0;
  background: transparent;
}
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--ui-surface-hover); }
.section-header { display: flex; justify-content: space-between; align-items: center; }
.sub-header.section-header h3 { flex: 1; margin-left: 12px; }

.disabled-fade { opacity: 0.5; pointer-events: none; }
.input-with-unit { display: flex; align-items: center; gap: 8px; }
.unit { font-size: 0.85rem; color: var(--text-sub); font-family: var(--font-mono); font-weight: 500; }
.link-text { font-family: monospace; font-size: 0.8rem; color: var(--text-muted); margin-top: 4px; }

.modal-footer .action-btn,
.modal-footer .primary-btn {
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.w-full-btn { width: 100%; justify-content: center; }
.divider-text {
  display: flex; align-items: center; text-align: center; color: var(--text-sub); font-size: 0.75rem;
  font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; margin: 15px 0;
}
.divider-text::before, .divider-text::after { content: ''; flex: 1; border-bottom: 1px solid var(--ui-surface-hover); }
.divider-text::before { margin-right: 10px; }
.divider-text::after { margin-left: 10px; }
.restore-actions { display: flex; flex-direction: column; }
.active-border { border: 1px solid var(--ui-accent) !important; }

/* 关于页面的超链接样式 */
.link-item {
  color: var(--ui-accent);
  font-size: 0.85rem;
  text-decoration: none;
  transition: opacity 0.2s;
  cursor: pointer;
}
.link-item:hover {
  opacity: 0.8;
  text-decoration: underline;
}
.app-update-progress-container {
  flex: 1;
  max-width: 240px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: var(--ui-surface-hover);
  padding: 10px 14px;
  border-radius: var(--ui-radius-sm);
}
.progress-info {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-main);
}
.divider-dot {
  color: var(--text-muted);
  font-weight: bold;
}
.progress-bar-wrap {
  width: 100%;
  height: 6px;
  background: var(--ui-surface-card);
  border-radius: 3px;
  overflow: hidden;
}
.progress-bar-fill {
  height: 100%;
  background: var(--ui-accent-block);
  transition: width 0.3s ease;
}
.progress-size {
  font-size: 0.75rem;
  color: var(--text-muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
