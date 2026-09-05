<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.about.title') }}</h3>
    </div>

    <div class="glass-card setting-group scrollable">
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
            <span v-if="globalState.appUpdateProgress.isDownloaded" class="speed" style="color: var(--accent); font-weight: 600;">新版本已就绪，点击安装</span>
            <template v-else>
              <span class="speed">{{ formatSpeed(globalState.appUpdateProgress.speedBps) }}</span>
              <span class="divider-dot">·</span>
              <span class="eta">剩余 {{ formatEtaTime(globalState.appUpdateProgress.etaSec) }}</span>
            </template>
          </div>
          <div class="progress-bar-wrap">
            <div class="progress-bar-fill" :style="{ width: appUpdatePercent + '%', backgroundColor: globalState.appUpdateProgress.isDownloaded ? 'var(--accent)' : '' }"></div>
          </div>
          <div class="progress-size">
            <span v-if="globalState.appUpdateProgress.isDownloaded">{{ globalState.appUpdateProgress.version }} 下载完成</span>
            <span v-else>{{ formatBytes(globalState.appUpdateProgress.bytesDone) }} / {{ formatBytes(globalState.appUpdateProgress.totalBytes) }}</span>
          </div>
        </div>
      </div>

      <div class="divider"></div>
      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.language') }}</h4>
          <p>{{ t('settings.about.languageDesc') }}</p>
        </div>
        <ModernSelect
          :modelValue="behavior.language || currentLocale"
          :options="SUPPORTED_LOCALES"
          @update:modelValue="handleLanguageChange"
        />
      </div>

      <div class="divider"></div>
      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.version') }}</h4>
          <p>{{ globalState.appVersion || t('settings.about.fetchingVersion') }}</p>
        </div>
        <button class="action-btn accent-btn" @click="handleCheckUpdate" :disabled="globalState.appUpdateChecking">
          {{ globalState.appUpdateChecking ? t('settings.about.checking') : t('settings.about.checkUpdate') }}
        </button>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.autoUpdate') }}</h4>
          <p>{{ t('settings.about.autoUpdateDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.autoUpdate" @change="$emit('save')" />
          <span class="slider"></span>
        </label>
      </div>

      <Transition name="dropdown">
        <div v-if="behavior.autoUpdate" class="auto-update-sub-items">
          <div class="divider"></div>
          <div class="setting-item">
            <div class="info">
              <h4>{{ t('settings.about.updateMethod') }}</h4>
            </div>
            <ModernSelect
              v-model="behavior.updateMethod"
              :options="[
                { label: t('settings.about.startup'), value: 'startup' },
                { label: t('settings.about.scheduled'), value: 'scheduled' }
              ]"
              @change="$emit('save')"
            />
          </div>

          <div class="divider"></div>
          <div class="setting-item" :class="{ 'disabled-fade': behavior.updateMethod !== 'scheduled' }">
            <div class="info">
              <h4>{{ t('settings.about.updateInterval') }}</h4>
            </div>
            <div class="input-with-unit">
              <ModernNumberInput
                v-model="behavior.updateInterval"
                :min="1"
                :max="365"
                :disabled="behavior.updateMethod !== 'scheduled'"
                @change="$emit('save')"
              />
              <span class="unit">{{ t('settings.about.daysUnit') }}</span>
            </div>
          </div>
        </div>
      </Transition>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.exportBackup') }}</h4>
          <p>{{ t('settings.about.exportBackupDesc') }}</p>
        </div>
        <button class="action-btn accent-btn" @click="handleExportBackup">{{ t('settings.about.exportBtn') }}</button>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.restoreBackup') }}</h4>
          <p>{{ t('settings.about.restoreBackupDesc') }}</p>
        </div>
        <button class="action-btn accent-btn" @click="openRestoreModal">{{ t('settings.about.restoreBtn') }}</button>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.dataDirDiagnostic') }}</h4>
          <p>{{ t('settings.about.dataDirDiagnosticDesc') }}</p>
        </div>
        <button class="action-btn" @click="openDataDirDiagnosticModal">{{ t('settings.about.viewStatusBtn') }}</button>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.appDiagnosticInfo') }}</h4>
          <p>{{ t('settings.about.appDiagnosticDesc') }}</p>
        </div>
        <button class="action-btn accent-btn" @click="handleExportDiagnostics">{{ t('settings.about.diagnosticsExport') }}</button>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.about.githubRepo') }}</h4>
          <a href="javascript:void(0)" @click="openLink('https://github.com/Zzz-IT/GoclashZ')" class="link-item">https://github.com/Zzz-IT/GoclashZ</a>
        </div>
      </div>
    </div>

    <!-- 还原备份弹窗 -->
    <Transition name="pop">
      <div v-if="showRestoreModal" class="modal-overlay" @click.self="showRestoreModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ t('settings.about.restoreTitle') }}</h3>
          </div>
          <div class="modal-body">
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
                  v-model="restoreMode"
                  :options="restoreModeOptions"
                />
              </div>
            </div>

            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showRestoreModal = false">{{ t('common.cancel') }}</button>
              <button class="primary-btn accent-btn flex-1" :disabled="!selectedPath" @click="confirmRestore">{{ t('settings.about.executeRestore') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <Transition name="pop">
      <div v-if="showDataDirDiagnosticModal" class="modal-overlay" @click.self="showDataDirDiagnosticModal = false">
        <div class="custom-modal-card" @click.stop style="max-width: 500px;">
          <div class="modal-header">
            <h3>{{ t('settings.about.diagnosticsTitle') }}</h3>
          </div>
          <div class="modal-body">
            <div v-if="dataDirInfo" class="diagnostic-results" style="max-height: 400px; overflow-y: auto;">
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsAppDir') }}</h4>
                  <p class="link-text" style="word-break: break-all;">{{ dataDirInfo.appDir }}</p>
                </div>
              </div>
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsDataDir') }}</h4>
                  <p class="link-text" style="word-break: break-all;">{{ dataDirInfo.dataDir }}</p>
                </div>
              </div>
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsSeedDir') }}</h4>
                  <p class="link-text" style="word-break: break-all;">{{ dataDirInfo.seedCoreBinDir }}</p>
                  <p v-if="dataDirInfo.seedManifestExists" style="color: var(--green-text); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.seedManifestLabel', { status: t('settings.about.diagnosticsReady') }) }}</p>
                  <p v-else style="color: var(--red-text); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.seedManifestLabel', { status: t('settings.about.diagnosticsMissing') }) }}</p>
                </div>
              </div>
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsCoreDir') }}</h4>
                  <p class="link-text" style="word-break: break-all;">{{ dataDirInfo.coreBinDir }}</p>
                  <p v-if="dataDirInfo.layoutOK" style="color: var(--green-text); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.layoutNormal') }}</p>
                  <p v-else style="color: var(--accent); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.layoutAbnormal') }}</p>
                </div>
              </div>
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsCompStatus') }}</h4>
                  <p :style="{ color: dataDirInfo.coreReady ? 'var(--green-text)' : 'var(--red-text)', fontWeight: 600 }">
                    Mihomo: {{ dataDirInfo.coreReady ? t('settings.about.diagnosticsReady') : (dataDirInfo.coreExists ? t('settings.about.diagnosticsCorrupted') : t('settings.about.diagnosticsMissing')) }}
                  </p>
                  <p :style="{ color: dataDirInfo.wintunReady ? 'var(--green-text)' : 'var(--accent)', fontWeight: 600 }">
                    Wintun: {{ dataDirInfo.wintunReady ? t('settings.about.diagnosticsReady') : (dataDirInfo.wintunExists ? t('settings.about.diagnosticsCorrupted') : t('settings.about.diagnosticsMissing')) }}
                  </p>
                </div>
              </div>
              <div class="setting-item" style="padding: 6px 0; align-items: flex-start; border-bottom: none;">
                <div class="info">
                  <h4>{{ t('settings.about.diagnosticsLegacyDir') }}</h4>
                  <p class="link-text" style="word-break: break-all;">{{ dataDirInfo.legacyDataDir || t('common.empty') }}</p>
                  <p v-if="dataDirInfo.legacyExists" style="color: var(--accent); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.legacyExists') }}</p>
                  <p v-else style="color: var(--green-text); font-size: 0.8rem; margin-top: 4px;">{{ t('settings.about.legacyCleaned') }}</p>
                </div>
              </div>
            </div>

            <div class="modal-footer" style="margin-top: 16px;">
              <button class="action-btn flex-1" @click="openDataDirDiagnosticModal">{{ t('settings.about.diagnosticsRerun') }}</button>
              <button class="action-btn flex-1" @click="handleExportDiagnostics">{{ t('settings.about.diagnosticsExport') }}</button>
              <button class="primary-btn accent-btn flex-1" @click="showDataDirDiagnosticModal = false">{{ t('settings.about.diagnosticsDone') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { BrowserOpenURL, EventsOn } from '../../wailsjs/runtime/runtime';
import { globalState, showAlert, showConfirm } from '../store';
import { formatBytes, formatEtaTime, formatSpeed } from '../utils/format';
import { ICONS } from '../utils/icons';
import appLogo from '../assets/logo.ico';
import ModernNumberInput from './ModernNumberInput.vue';
import ModernSelect from './ModernSelect.vue';
import { t, currentLocale, setLocale, SUPPORTED_LOCALES, SupportedLocale } from '../locales';

const props = defineProps<{ behavior: Record<string, any> }>();
const emit = defineEmits<{
  navigate: [view: 'main'];
  save: [];
}>();

const handleLanguageChange = (val: any) => {
  const lang = String(val) as SupportedLocale;
  props.behavior.language = lang;
  setLocale(lang);
  emit('save');
};

const showRestoreModal = ref(false);
const selectedPath = ref("");
const restoreMode = ref("all");
const showDataDirDiagnosticModal = ref(false);
const dataDirInfo = ref<any>(null);

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
  BrowserOpenURL(url);
};

const handleCheckUpdate = async () => {
  if (globalState.appUpdateChecking) return;
  try {
    // 🚀 核心改进：调用异步静默下载流，将通知权交给全局监听器 (App.vue)
    await (API as any).CheckAndDownloadAppUpdateAsync();
  } catch (e) {
    await showAlert("检查更新失败: " + e, "错误", true);
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
        await (API as any).ApplyAppUpdate(fullPath);
      } catch (e: any) {
        await showAlert(String(e?.message || e || t('common.error')), t('common.error'), true);
      }
  }
};

// 导出备份
const handleExportBackup = async () => {
  try {
    const res = await (API as any).ExportBackup();
    if (res === "SUCCESS") {
      await showAlert("备份成功导出", "通知");
    }
  } catch (e) {
    await showAlert("导出失败: " + String(e), "错误");
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
    const path = await (API as any).SelectBackupFile();
    if (path) {
      selectedPath.value = path;
    }
  } catch (e) {
    console.error("选择文件取消或失败", e);
  }
};

// 执行还原
const confirmRestore = async () => {
  if (!selectedPath.value) return;

  const warnings: Record<string, string> = {
    all: '完整恢复会替换当前软件设置、订阅列表、运行配置和主题设置。恢复过程中会短暂停止内核。',
    subs: '此操作会用备份中的订阅列表替换当前订阅列表，当前多余订阅会被移除。',
    'subs-merge': '此操作会保留当前订阅列表，并将备份中的订阅合并进来。同 ID 订阅可能被备份内容覆盖。',
    settings: '此操作只恢复软件设置和主题设置，不会影响订阅列表。'
  };

  const confirmMsg = warnings[restoreMode.value] || '确定要执行数据还原吗？';

  const ok = await showConfirm(
    confirmMsg + "\n\n还原完成后，部分设置将即时生效。",
    "还原备份确认",
    true
  );
  if (!ok) return;

  try {
    const res = await (API as any).ExecuteRestore(selectedPath.value, restoreMode.value);
    if (res === "SUCCESS") {
      showRestoreModal.value = false;
      await showAlert("数据还原成功！设置及配置已即时生效。", "成功");
    }
  } catch (e) {
    await showAlert("还原失败: " + String(e), "错误");
  }
};

const openDataDirDiagnosticModal = async () => {
  try {
    dataDirInfo.value = await API.GetDataDirInfo();
    showDataDirDiagnosticModal.value = true;
  } catch (error) {
    showAlert('获取数据目录信息失败: ' + error);
  }
};

const handleExportDiagnostics = async () => {
  try {
    await API.ExportDiagnostics();
    showAlert('诊断信息导出成功');
  } catch (error) {
    if (error && String(error).indexOf('User cancelled') === -1) {
      showAlert('导出诊断信息失败: ' + error);
    }
  }
};

const appUpdatePercent = computed(() => {
  const p = globalState.appUpdateProgress;
  if (!p || !p.totalBytes) return 0;
  return Math.min(100, Math.floor((p.bytesDone / p.totalBytes) * 100));
});

const unsubAppUpdateBusy = EventsOn("app-update-busy", () => {
  globalState.appUpdateChecking = false;
  void showAlert("已有软件更新任务正在进行，请稍后再试。", "提示");
});

onUnmounted(() => {
  unsubAppUpdateBusy();
});
</script>

<style scoped>
.clickable-progress {
  cursor: pointer;
  padding: 10px 14px;
  border-radius: 12px;
  transition: background 0.2s;
  margin-right: -14px;
  margin-top: -10px;
  margin-bottom: -10px;
}
.clickable-progress:hover {
  background: var(--surface-hover);
}

.settings-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 100%;
  overflow: visible;
}
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.setting-group.scrollable {
  padding-bottom: 20px;
  overflow: visible;
  max-height: none;
}

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
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);}
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--accent-fg); }

.sub-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 0 0 12px 0;
  background: transparent;
}
.sub-header.page-sticky-mask {
  --sticky-mask-bleed: 4px;
}
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }
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
.divider-text::before, .divider-text::after { content: ''; flex: 1; border-bottom: 1px solid var(--surface-hover); }
.divider-text::before { margin-right: 10px; }
.divider-text::after { margin-left: 10px; }
.restore-actions { display: flex; flex-direction: column; }
.active-border { border: 1px solid var(--accent) !important; }

.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.5s cubic-bezier(0.4, 0, 0.2, 1);
  max-height: 250px;
  overflow: hidden;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  max-height: 0;
  transform: translateY(-8px);
}

/* 关于页面的超链接样式 */
.link-item {
  color: var(--accent);
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
  background: var(--surface-hover);
  padding: 10px 14px;
  border-radius: 8px;
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
  background: var(--surface-panel);
  border-radius: 3px;
  overflow: hidden;
}
.progress-bar-fill {
  height: 100%;
  background: var(--accent);
  transition: width 0.3s ease;
}
.progress-size {
  font-size: 0.75rem;
  color: var(--text-muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
