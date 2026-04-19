<template>
  <div class="subs-view" @click="closeMenus">
    <div class="page-header">
      <div class="header-text">
        <span class="micro-title">配置源管理</span>
        <h2 class="main-title">订阅与本地配置</h2>
      </div>
      <div class="header-actions">
        <button class="action-btn" @click.stop="showImportCard = true">
          <span class="btn-plus">+</span> 导入配置
        </button>
        <button class="primary-btn" @click="handleUpdateAll" :disabled="loading">
          <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': loading }"></span>
          {{ loading ? '更新中...' : '更新全部订阅' }}
        </button>
      </div>
    </div>

    <div class="subs-list">
      <h4 class="list-title">本地配置库 (点击卡片切换)</h4>
      <div v-if="localConfigs.length === 0" class="empty-state">暂无配置文件</div>

      <div
        v-for="config in localConfigs"
        :key="config"
        class="sub-card clickable"
        :class="{
          'active-card': isCurrentConfig(config),
          'selecting-card': selecting === config
        }"
        @click="handleSelectConfig(config)"
      >
        <div class="sub-header">
          <div class="sub-info">
            <h4 class="sub-name">{{ config }}</h4>
            <span class="sub-path font-mono">core/bin/{{ config }}</span>
          </div>
          <div class="sub-status">
            <span v-if="isCurrentConfig(config)" class="status-badge online">● 正在使用</span>
            <span v-else-if="selecting === config" class="status-badge loading-tag">应用中...</span>
          </div>
        </div>

        <div class="sub-footer">
          <span class="sub-hint">{{ isCurrentConfig(config) ? '内核已加载此配置' : '点击切换' }}</span>
          <div class="sub-actions">
            <button class="icon-btn" @click.stop="toggleMenu(config)" v-html="ICONS.more"></button>
            <div v-if="activeMenu === config" class="dropdown-menu">
              <button class="menu-item" @click.stop="handleUpdateSingle(config)">更新订阅</button>
              <div class="menu-divider"></div>
              <button class="menu-item" @click.stop="handleRename(config)">重命名</button>
              <button class="menu-item" @click.stop="handleEditFile(config)">记事本编辑</button>
              <button class="menu-item danger" @click.stop="handleDelete(config)">彻底删除</button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 导入弹窗 -->
    <Transition name="fade">
      <div v-if="showImportCard" class="modal-overlay" @click="showImportCard = false">
        <div class="import-card glass-panel" @click.stop>
          <div class="card-header">
            <h3>导入配置文件</h3>
            <button class="close-x" @click="showImportCard = false">&times;</button>
          </div>

          <div class="import-options">
            <div class="option-section">
              <label>通过链接订阅</label>
              <div class="input-row">
                <input v-model="url" placeholder="请输入订阅地址 https://..." class="modern-input" />
                <button class="primary-btn mini" @click="handleDownloadLink" :disabled="!url || loading">
                  下载
                </button>
              </div>
            </div>

            <div class="divider-text">或者</div>

            <div class="option-section">
              <label>本地 YAML 文件</label>
              <button class="action-btn w-full-btn" @click="handleImportLocal">
                选择本地文件并导入
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const ICONS = {
  refresh: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"></polyline><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path></svg>`,
  more: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="2"></circle><circle cx="12" cy="5" r="2"></circle><circle cx="12" cy="19" r="2"></circle></svg>`
};

const showImportCard = ref(false);
const loading = ref(false);
const url = ref('');
const localConfigs = ref<string[]>([]);
const currentPath = ref('');
const activeMenu = ref<string | null>(null);
const selecting = ref<string | null>(null);

const isCurrentConfig = (filename: string) => {
  if (!currentPath.value) return false;
  const currentFile = currentPath.value.split(/[\\/]/).pop();
  return currentFile === filename;
};

const fetchConfigs = async () => {
  try {
    const list = await API.GetLocalConfigs();
    localConfigs.value = (list || []).filter(name => !name.endsWith('config.yaml'));
    const data: any = await API.GetInitialData();
    if (data && data.activeConfig) {
      currentPath.value = data.activeConfig;
    }
  } catch (e) {
    console.error("同步状态失败:", e);
  }
};

const handleSelectConfig = async (filename: string) => {
  if (isCurrentConfig(filename) || selecting.value) return;
  selecting.value = filename;
  try {
    await API.SelectLocalConfig(filename);
    currentPath.value = filename;
  } catch (error) {
    alert("切换失败: " + error);
  } finally {
    selecting.value = null;
  }
};

const handleUpdateAll = async () => {
  loading.value = true;
  try {
    await (API as any).UpdateAllSubs();
    alert("所有订阅更新任务已提交");
    await fetchConfigs();
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleUpdateSingle = async (name: string) => {
  activeMenu.value = null;
  loading.value = true;
  try {
    await (API as any).UpdateSingleSub(name);
    alert(`订阅 ${name} 更新成功`);
    await fetchConfigs();
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleDownloadLink = async () => {
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    await fetchConfigs();
    showImportCard.value = false;
    url.value = '';
    alert("订阅下载成功！");
  } catch (e) {
    alert("下载失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleImportLocal = async () => {
  try {
    await API.ImportLocalConfig();
    await fetchConfigs();
    showImportCard.value = false;
  } catch (e) {
    console.log("Import cancelled");
  }
};

const handleRename = async (filename: string) => {
  activeMenu.value = null;
  const newName = prompt("请输入新名称:", filename);
  if (newName && newName !== filename) {
    try {
      await API.RenameConfig(filename, newName);
      await fetchConfigs();
    } catch (e) {
      alert(e);
    }
  }
};

const handleEditFile = async (filename: string) => {
  activeMenu.value = null;
  await API.OpenConfigFile(filename);
};

const handleDelete = async (filename: string) => {
  activeMenu.value = null;
  if (isCurrentConfig(filename)) {
    const confirmClose = confirm(`【警告】"${filename}" 正在使用中！\n删除前需要强制关闭代理和虚拟网卡服务，是否继续？`);
    if (!confirmClose) return;
    await API.StopProxy();
    currentPath.value = '';
  } else {
    if (!confirm(`确定彻底删除 ${filename} 吗？`)) return;
  }

  try {
    await API.DeleteConfig(filename);
    await fetchConfigs();
    if (localConfigs.value.length === 0) {
      await (API as any).ClearBaseConfig();
    }
  } catch (e) {
    alert("删除失败: " + e);
  }
};

const toggleMenu = (filename: string) => {
  activeMenu.value = activeMenu.value === filename ? null : filename;
};

const closeMenus = () => {
  activeMenu.value = null;
};

onMounted(() => {
  fetchConfigs();
  (window as any).runtime.EventsOn("config-changed", (newName: string) => {
    if (newName && newName !== 'config.yaml') {
      currentPath.value = newName;
    }
  });
});

onUnmounted(() => {
  (window as any).runtime.EventsOff("config-changed");
});
</script>

<style scoped>
.subs-view { display: flex; flex-direction: column; height: 100%; color: var(--text-main); }

/* 页面头部 */
.page-header { display: flex; justify-content: space-between; align-items: flex-end; margin-bottom: 24px; }
.header-actions { display: flex; gap: 12px; }
.micro-title { font-size: 0.7rem; color: var(--text-muted); text-transform: uppercase; font-weight: 700; }
.main-title { font-size: 1.4rem; margin-top: 4px; }

/* 按钮通用 */
.primary-btn { display: flex; align-items: center; gap: 8px; padding: 8px 16px; border-radius: 6px; border: none; background: var(--text-main); color: var(--accent-fg); font-weight: 600; cursor: pointer; }
.btn-icon { width: 14px; height: 14px; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { 100% { transform: rotate(360deg); } }

/* 列表部分 */
.list-title { font-size: 0.85rem; color: var(--text-sub); margin-bottom: 12px; }
.sub-card {
  position: relative; padding: 16px; border-radius: 10px; border: none;
  background: var(--surface); margin-bottom: 12px; transition: all 0.2s ease;
}
.clickable { cursor: pointer; }
.clickable:hover { background: var(--surface-hover); }

/* 状态样式 */
.active-card { background: var(--surface-hover) !important; }
.selecting-card { opacity: 0.7; pointer-events: none; }

.sub-header { display: flex; justify-content: space-between; margin-bottom: 12px; }
.sub-name { font-size: 0.95rem; font-weight: 600; }
.sub-path { font-size: 0.7rem; color: var(--text-muted); }

.status-badge { font-size: 0.7rem; font-weight: 700; padding: 3px 8px; border-radius: 4px; }
.status-badge.online { color: var(--text-main); background: var(--surface-hover); }
.loading-tag { color: var(--text-muted); background: var(--surface-hover); }

.sub-footer { display: flex; justify-content: space-between; align-items: center; margin-top: 10px; padding-top: 10px; }
.sub-hint { font-size: 0.75rem; color: var(--text-sub); font-style: italic; }

.icon-btn {
  width: 24px !important; height: 24px !important;
  padding: 0; display: flex; align-items: center; justify-content: center;
  background: none; border: none; cursor: pointer; color: var(--text-sub); border-radius: 4px;
}
.icon-btn :deep(svg) { width: 14px !important; height: 14px !important; }
.icon-btn:hover { background: var(--surface-hover); color: var(--text-main); }

/* 下拉菜单 */
.dropdown-menu {
  position: absolute; right: 0; top: 30px; width: 140px; border-radius: 8px; z-index: 999; overflow: hidden;
}
.menu-item { padding: 10px 14px; font-size: 0.85rem; border: none; width: 100%; text-align: left; cursor: pointer; transition: background 0.2s; }
.menu-item.danger { color: var(--text-main) !important; font-weight: 600; }
.menu-divider { height: 1px; margin: 4px 0; }

.empty-state { padding: 30px; text-align: center; color: var(--text-muted); border: none; border-radius: 10px; background: var(--surface); }

/* 弹窗遮罩 */
.modal-overlay {
  position: fixed; top: 0; left: 0; width: 100%; height: 100%;
  background: rgba(0,0,0,0.4); backdrop-filter: blur(4px);
  display: flex; align-items: center; justify-content: center; z-index: 2000;
}
.import-card { width: 420px; padding: 24px; border-radius: 16px; box-shadow: 0 20px 40px rgba(0,0,0,0.2); }
.card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.close-x { background: none; border: none; font-size: 24px; cursor: pointer; color: var(--text-sub); }

.option-section label { display: block; font-size: 0.85rem; color: var(--text-sub); margin-bottom: 8px; font-weight: 600; }
.input-row { display: flex; gap: 8px; }
.modern-input { flex: 1; background: var(--surface-hover); border: none; color: inherit; padding: 10px 12px; border-radius: 8px; outline: none; font-size: 0.85rem; }
.divider-text { text-align: center; margin: 16px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 35%; height: 1px; background: var(--surface-hover); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }
.w-full-btn { width: 100%; padding: 12px; font-weight: 600; border-radius: 8px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; }
.mini { padding: 0 16px; }

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: scale(0.95); }
</style>

<style>
/* ☀️ 日间模式 (白底黑字，纯实色) */
.app-shell:not(.dark) .dropdown-menu { background: #ffffff !important; border: 1px solid #e4e4e7; box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1); }
.app-shell:not(.dark) .menu-item { background: transparent; color: #18181b; }
.app-shell:not(.dark) .menu-item:hover { background: #f4f4f5; }
.app-shell:not(.dark) .menu-divider { background: #e4e4e7; }

/* 🌙 夜间模式 (黑底白字，纯实色) */
.app-shell.dark .dropdown-menu { background: #242427 !important; border: 1px solid #3f3f46; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6); }
.app-shell.dark .menu-item { background: transparent; color: #f4f4f5; }
.app-shell.dark .menu-item:hover { background: #3f3f46; }
.app-shell.dark .menu-divider { background: #3f3f46; }

<style>
/* * 新增一个不带 scoped 的 style 标签！
 * 绕过 Vue 的作用域限制，直接根据外层的 .app-shell 状态强制应用纯实色
 */

/* ☀️ 日间模式 (白底黑字，纯实色) */
.app-shell:not(.dark) .dropdown-menu {
  background: #ffffff !important;
  border: 1px solid #e4e4e7;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}
.app-shell:not(.dark) .menu-item { background: transparent; color: #18181b; }
.app-shell:not(.dark) .menu-item:hover { background: #f4f4f5; }
.app-shell:not(.dark) .menu-divider { background: #e4e4e7; }

/* 🌙 夜间模式 (黑底白字，纯实色) */
.app-shell.dark .dropdown-menu {
  background: #242427 !important; /* 深黑灰纯色 */
  border: 1px solid #3f3f46;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
}
.app-shell.dark .menu-item { background: transparent; color: #f4f4f5; }
.app-shell.dark .menu-item:hover { background: #3f3f46; }
.app-shell.dark .menu-divider { background: #3f3f46; }
</style>