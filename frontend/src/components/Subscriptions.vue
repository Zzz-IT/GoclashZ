<template>
  <div class="subs-view" @click="closeMenus">
    <div class="page-header">
      <div class="header-text">
        <h2 class="main-title">本地配置库</h2>
        <span class="sub-text">点击卡片切换</span>
      </div>
      
      <div class="header-actions">
        <button class="action-btn" @click.stop="showImportModal = true">
          <span class="btn-icon" v-html="ICONS.plus"></span> 导入配置
        </button>
        <button class="primary-btn" @click="handleUpdateAll" :disabled="loading">
          <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': loading }"></span>
          {{ loading ? '更新中...' : '更新全部订阅' }}
        </button>
      </div>
    </div>

    <div class="subs-list">
      <div v-if="localConfigs.length === 0" class="empty-state">
        暂无本地配置文件。
      </div>

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
      <div v-if="showImportModal" class="modal-overlay" @click="showImportModal = false">
        <div class="import-card glass-panel" @click.stop>
          <div class="modal-header">
            <h3>导入配置文件</h3>
            <button class="close-x" @click="showImportModal = false">&times;</button>
          </div>

          <div class="modal-body">
            <div class="section">
              <label>订阅链接</label>
              <div class="input-row">
                <input v-model="url" placeholder="https://..." class="modern-input" />
                <button class="primary-btn mini" @click="handleDownload" :disabled="!url || loading">下载</button>
              </div>
            </div>

            <div class="divider-text">或者</div>

            <div class="section">
              <label>本地导入</label>
              <button class="action-btn w-full-btn" @click="handleImportLocal">
                <span class="btn-icon" v-html="ICONS.folder"></span> 浏览本地 YAML 文件
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
  more: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="2"></circle><circle cx="12" cy="5" r="2"></circle><circle cx="12" cy="19" r="2"></circle></svg>`,
  plus: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>`,
  folder: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>`
};

const showImportModal = ref(false);
const url = ref('');
const loading = ref(false);
const selecting = ref<string | null>(null);
const currentPath = ref('');
const localConfigs = ref<string[]>([]);
const activeMenu = ref<string | null>(null);

const isCurrentConfig = (name: string) => {
  if (!currentPath.value) return false;
  const currentFile = currentPath.value.split(/[\\/]/).pop();
  return currentFile === name;
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

const handleSelectConfig = async (name: string) => {
  if (isCurrentConfig(name) || selecting.value) return;
  selecting.value = name;
  try {
    await API.SelectLocalConfig(name);
    currentPath.value = name;
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
    alert("所有订阅更新尝试完成");
    await fetchConfigs();
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleUpdateSingle = async (filename: string) => {
  activeMenu.value = null;
  loading.value = true;
  try {
    await (API as any).UpdateSingleSub(filename);
    alert(`"${filename}" 更新成功`);
    await fetchConfigs();
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleDownload = async () => {
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    await fetchConfigs();
    showImportModal.value = false;
    url.value = '';
    alert("下载成功！");
  } catch (e) {
    alert("下载失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleImportLocal = async () => {
  showImportModal.value = false;
  try {
    await API.ImportLocalConfig();
    await fetchConfigs();
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

const toggleMenu = (name: string) => {
  activeMenu.value = activeMenu.value === name ? null : name;
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

/* 头部样式调整 */
.page-header { display: flex; justify-content: space-between; align-items: flex-end; margin-bottom: 32px; }
.main-title { font-size: 1.6rem; font-weight: 600; margin-bottom: 4px; }
.sub-text { font-size: 0.85rem; color: var(--text-sub); }
.header-actions { display: flex; gap: 12px; }

/* 按钮样式 */
.primary-btn { 
  display: flex; align-items: center; gap: 8px; padding: 10px 20px; 
  border-radius: 8px; border: none; background: var(--text-main); 
  color: var(--accent-fg); font-weight: 600; cursor: pointer; transition: 0.2s; 
}
.primary-btn:hover:not(:disabled) { opacity: 0.85; transform: translateY(-1px); }
.action-btn { 
  display: flex; align-items: center; gap: 8px; padding: 10px 20px; 
  border-radius: 8px; border: none; background: var(--surface); 
  color: var(--text-main); cursor: pointer; font-weight: 500; transition: 0.2s; 
}
.action-btn:hover { background: var(--surface-hover); }

/* 列表部分 */
.subs-list { flex: 1; overflow-y: auto; padding-right: 8px; }
.sub-card { position: relative; padding: 20px; border-radius: 12px; background: var(--surface); margin-bottom: 16px; transition: 0.2s; }
.sub-card:hover { background: var(--surface-hover); }
.active-card { background: var(--surface-hover) !important; }

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

/* 弹窗遮罩 */
.modal-overlay { 
  position: fixed; top: 0; left: 0; width: 100%; height: 100%; 
  background: rgba(0,0,0,0.4); backdrop-filter: blur(4px); 
  display: flex; align-items: center; justify-content: center; z-index: 2000; 
}
.import-card { width: 440px; background: var(--glass-panel); padding: 24px; border-radius: 16px; box-shadow: 0 20px 50px rgba(0,0,0,0.3); }
.modal-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
.close-x { background: none; border: none; font-size: 28px; cursor: pointer; color: var(--text-sub); }

.section label { display: block; font-size: 0.85rem; color: var(--text-sub); margin-bottom: 8px; font-weight: 600; }
.input-row { display: flex; gap: 8px; background: var(--surface-hover); padding: 4px 4px 4px 12px; border-radius: 10px; align-items: center; }
.modern-input { flex: 1; background: transparent; border: none; color: inherit; outline: none; padding: 8px 0; }
.mini { padding: 8px 16px; font-size: 0.85rem; }

.divider-text { text-align: center; margin: 20px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 42%; height: 1px; background: var(--surface-hover); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }

.w-full-btn { width: 100%; padding: 12px; font-weight: 600; border-radius: 10px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; display: flex; align-items: center; justify-content: center; gap: 8px; }

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: scale(0.95); }

.dropdown-menu { position: absolute; right: 0; top: 30px; width: 150px; border-radius: 8px; z-index: 10; overflow: hidden; }
.menu-item { width: 100%; padding: 10px 16px; border: none; background: transparent; text-align: left; cursor: pointer; font-size: 0.85rem; color: var(--text-main); }
.menu-item:hover { background: var(--surface-hover); }
.menu-item.danger { color: var(--text-main) !important; font-weight: 600; }
.menu-divider { height: 1px; margin: 4px 0; }

.empty-state { padding: 30px; text-align: center; color: var(--text-muted); border: none; border-radius: 10px; background: var(--surface); }
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
</style>