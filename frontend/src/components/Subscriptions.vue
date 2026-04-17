<template>
  <div class="subs-view" @click="activeMenu = null">
    <div class="page-header">
      <div class="header-text">
        <span class="micro-title">配置源管理</span>
        <h2 class="main-title">订阅与节点配置</h2>
      </div>
      <button class="primary-btn" @click="handleUpdate" :disabled="loading">
        <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': loading }"></span>
        {{ loading ? '正在同步数据...' : '更新当前订阅' }}
      </button>
    </div>

    <div class="action-card glass-panel">
      <div class="card-info">
        <h3 class="card-title">导入外部订阅</h3>
        <p class="card-desc">支持标准的 Clash YAML 格式订阅链接。导入后将自动覆盖并重载内核。</p>
      </div>
      <div class="input-group">
        <div class="input-wrapper">
          <span class="input-icon" v-html="ICONS.link"></span>
          <input v-model="url" placeholder="https://example.com/api/v1/client/subscribe?token=..." class="modern-input" />
        </div>
        <button class="action-btn" @click="handleUpdate" :disabled="!url || loading">
          立即导入
        </button>
      </div>
    </div>

    <div class="action-card glass-panel" style="margin-bottom: 24px;">
      <div class="card-info">
        <h3 class="card-title">导入本地配置</h3>
        <p class="card-desc">选择本地的 YAML 配置文件并将其导入到应用的配置库中。</p>
      </div>
      <div class="input-group">
        <button class="action-btn" style="width: 100%; padding: 12px 0;" @click="handleImportLocal">
          <span style="margin-right: 8px;">+</span> 浏览并导入本地文件
        </button>
      </div>
    </div>

    <div class="subs-list">
      <h4 class="list-title">本地可用配置</h4>

      <div v-if="localConfigs.length === 0" class="empty-state">
        暂无本地配置文件，请导入。
      </div>

      <div
        v-for="config in localConfigs"
        :key="config"
        class="sub-card"
        :class="{'active-card': isCurrentConfig(config)}"
      >
        <div class="sub-header">
          <div class="sub-info">
            <h4 class="sub-name">{{ config }}</h4>
            <span class="sub-url font-mono">core/bin/{{ config }}</span>
          </div>
          <div class="sub-status" v-if="isCurrentConfig(config)">
            <span class="status-badge online">● 正在使用</span>
          </div>
        </div>

        <div class="sub-footer">
          <span class="sub-time">状态: {{ isCurrentConfig(config) ? '运行中' : '已就绪' }}</span>

          <div class="sub-actions relative-menu">
            <button class="icon-btn" title="操作" @click.stop="toggleMenu(config)" v-html="ICONS.edit"></button>

            <div v-if="activeMenu === config" class="dropdown-menu">
              <button class="menu-item" @click.stop="handleRename(config)">重命名</button>
              <button class="menu-item" @click.stop="handleEditFile(config)">编辑文件</button>
              <div class="menu-divider"></div>
              <button class="menu-item danger" @click.stop="handleDelete(config)">删除</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const ICONS = {
  refresh: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"></polyline><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path></svg>`,
  link: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>`,
  edit: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="1"></circle><circle cx="12" cy="5" r="1"></circle><circle cx="12" cy="19" r="1"></circle></svg>` // 改为了更通用的三点菜单图标
};

const url = ref('');
const loading = ref(false);
const currentPath = ref('core/bin/config.yaml'); // 默认选中路径
const localConfigs = ref<string[]>([]);
const activeMenu = ref<string | null>(null);

// 初始化获取数据
const fetchConfigs = async () => {
  try {
    const list = await API.GetLocalConfigs();
    localConfigs.value = list || [];
  } catch (error) {
    console.error("获取本地配置失败", error);
  }
};

const isCurrentConfig = (filename: string) => {
  return currentPath.value.endsWith(filename);
};

const toggleMenu = (filename: string) => {
  activeMenu.value = activeMenu.value === filename ? null : filename;
};

// 导入外部订阅 (原有)
const handleUpdate = async () => {
  if (!url.value) return;
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    alert("订阅更新并应用成功！");
    await fetchConfigs(); // 刷新列表
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

// 导入本地文件 (新增)
const handleImportLocal = async () => {
  try {
    await API.ImportLocalConfig();
    await fetchConfigs();
  } catch (error) {
    if (error) console.error("导入已取消或失败:", error);
  }
};

// 重命名配置 (新增)
const handleRename = async (filename: string) => {
  activeMenu.value = null;
  const newName = prompt(`请输入 [${filename}] 的新名称:`, filename);
  if (newName && newName.trim() !== "" && newName !== filename) {
    try {
      await API.RenameConfig(filename, newName.trim());
      // 如果重命名的是当前正在使用的配置文件，可以选择更新 currentPath
      if (isCurrentConfig(filename)) currentPath.value = `core/bin/${newName.trim()}`;
      await fetchConfigs();
    } catch (error) {
      alert("重命名失败: " + error);
    }
  }
};

// 用系统编辑器打开文件 (新增)
const handleEditFile = async (filename: string) => {
  activeMenu.value = null;
  try {
    await API.OpenConfigFile(filename);
  } catch (error) {
    alert("打开文件失败: " + error);
  }
};

// 删除配置 (新增)
const handleDelete = async (filename: string) => {
  activeMenu.value = null;
  if (isCurrentConfig(filename)) {
    alert("不能删除正在使用的默认配置文件！");
    return;
  }
  if (confirm(`确定要永久删除配置文件 ${filename} 吗？`)) {
    try {
      await API.DeleteConfig(filename);
      await fetchConfigs();
    } catch (error) {
      alert("删除失败: " + error);
    }
  }
};

onMounted(async () => {
  await fetchConfigs();
  const data: any = await API.GetInitialData();
  if (data && data.configPath) {
    currentPath.value = data.configPath;
  }
});
</script>

<style scoped>
.subs-view { display: flex; flex-direction: column; height: 100%; }
.page-header { display: flex; justify-content: space-between; align-items: flex-end; margin-bottom: 24px; }
.micro-title { font-size: 0.75rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
.main-title { font-size: 1.5rem; font-weight: 600; margin-top: 4px; color: var(--text-main); }
.primary-btn { display: flex; align-items: center; gap: 8px; padding: 10px 20px; border-radius: 8px; border: none; background: var(--text-main); color: var(--accent-fg); font-size: 0.85rem; font-weight: 500; cursor: pointer; transition: 0.2s; }
.primary-btn:hover:not(:disabled) { transform: translateY(-1px); box-shadow: 0 4px 12px rgba(0,0,0,0.1); }
.primary-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.btn-icon { width: 14px; height: 14px; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { 100% { transform: rotate(360deg); } }

.action-card { padding: 24px; border-radius: 12px; border: 1px solid var(--glass-border); background: var(--surface); margin-bottom: 32px; }
.card-title { font-size: 1.1rem; font-weight: 500; margin-bottom: 6px; }
.card-desc { font-size: 0.85rem; color: var(--text-sub); margin-bottom: 20px; }
.input-group { display: flex; gap: 12px; }
.input-wrapper { flex: 1; display: flex; align-items: center; background: var(--surface-hover); border: 1px solid var(--glass-border); border-radius: 8px; padding: 0 12px; transition: 0.2s border-color; }
.input-wrapper:focus-within { border-color: var(--text-sub); }
.input-icon { width: 16px; height: 16px; color: var(--text-muted); margin-right: 8px; }
.modern-input { flex: 1; background: transparent; border: none; outline: none; padding: 12px 0; color: var(--text-main); font-size: 0.9rem; }
.modern-input::placeholder { color: var(--text-muted); }
.action-btn { padding: 0 24px; border-radius: 8px; border: 1px solid var(--glass-border); background: transparent; color: var(--text-main); font-weight: 500; cursor: pointer; transition: 0.2s; }
.action-btn:hover:not(:disabled) { background: var(--surface-hover); }
.action-btn:disabled { opacity: 0.5; cursor: not-allowed; }

.list-title { font-size: 0.9rem; font-weight: 600; color: var(--text-sub); margin-bottom: 12px; }
.sub-card { padding: 20px; border-radius: 12px; border: 1px solid var(--glass-border); background: var(--surface); transition: 0.2s; margin-bottom: 12px; }
.active-card { border-color: #10b981; box-shadow: 0 4px 12px rgba(16, 185, 129, 0.05); }
.sub-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.sub-name { font-size: 1.05rem; font-weight: 500; margin-bottom: 4px; }
.sub-url { font-size: 0.75rem; color: var(--text-muted); word-break: break-all; }
.status-badge.online { background: rgba(16, 185, 129, 0.1); color: #10b981; padding: 4px 10px; border-radius: 20px; font-size: 0.75rem; font-weight: 600; }
.sub-footer { display: flex; justify-content: space-between; align-items: center; padding-top: 16px; border-top: 1px solid var(--glass-border); }
.sub-time { font-size: 0.8rem; color: var(--text-sub); }

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; transition: 0.2s; }
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }
.empty-state { padding: 20px; text-align: center; color: var(--text-muted); font-size: 0.9rem; border: 1px dashed var(--glass-border); border-radius: 8px; }

/* 下拉菜单样式 */
.relative-menu { position: relative; }
.dropdown-menu {
  position: absolute; right: 0; bottom: 32px; width: 120px;
  background: var(--surface); border: 1px solid var(--glass-border);
  border-radius: 8px; box-shadow: 0 10px 25px rgba(0,0,0,0.1);
  overflow: hidden; z-index: 100;
}
.menu-item {
  width: 100%; text-align: left; padding: 10px 16px; border: none;
  background: none; font-size: 0.85rem; color: var(--text-main); cursor: pointer;
}
.menu-item:hover { background: var(--surface-hover); }
.menu-item.danger { color: #ef4444; }
.menu-item.danger:hover { background: rgba(239, 68, 68, 0.1); }
.menu-divider { height: 1px; background: var(--glass-border); margin: 4px 0; }
</style>