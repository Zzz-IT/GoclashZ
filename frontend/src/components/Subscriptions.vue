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
        <h3 class="card-title">链接导入</h3>
        <p class="card-desc">输入 Clash 订阅链接，下载后的文件将存入本地配置库。</p>
      </div>
      <div class="input-group">
        <div class="input-wrapper">
          <span class="input-icon" v-html="ICONS.link"></span>
          <input v-model="url" placeholder="https://..." class="modern-input" />
        </div>
        <button class="action-btn" @click="handleUpdate" :disabled="!url || loading">
          立即下载
        </button>
      </div>
    </div>

    <div class="action-card glass-panel" style="margin-bottom: 24px;">
      <div class="card-info">
        <h3 class="card-title">本地导入</h3>
        <p class="card-desc">从你的电脑选择一个 .yaml 文件导入。</p>
      </div>
      <div class="input-group">
        <button class="action-btn w-full-btn" @click="handleImportLocal">
          <span class="btn-plus">+</span> 浏览本地文件
        </button>
      </div>
    </div>

    <div class="subs-list">
      <h4 class="list-title">本地配置库 (点击卡片即可切换并应用)</h4>

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
            <button class="icon-btn menu-trigger" @click.stop="toggleMenu(config)" v-html="ICONS.more"></button>

            <div v-if="activeMenu === config" class="dropdown-menu">
              <button class="menu-item" @click.stop="handleRename(config)">重命名</button>
              <button class="menu-item" @click.stop="handleEditFile(config)">记事本编辑</button>
              <div class="menu-divider"></div>
              <button class="menu-item danger" @click.stop="handleDelete(config)">彻底删除</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 1. 修改：补全 onUnmounted 的导入
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const ICONS = {
  refresh: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"></polyline><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path></svg>`,
  link: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>`,
  more: `<svg viewBox="0 0 24 24" fill="currentColor" stroke="none"><circle cx="12" cy="12" r="2"></circle><circle cx="12" cy="5" r="2"></circle><circle cx="12" cy="19" r="2"></circle></svg>`
};

const url = ref('');
const loading = ref(false);
const selecting = ref<string | null>(null);
const currentPath = ref('');
const localConfigs = ref<string[]>([]);
const activeMenu = ref<string | null>(null);

// 2. 统一判断逻辑：解决 TS2451 重复声明问题
const isCurrentConfig = (filename: string) => {
  if (!currentPath.value) return false;
  // 提取文件名进行对比，忽略路径差异
  const currentFile = currentPath.value.split(/[\\/]/).pop();
  return currentFile === filename;
};

const fetchConfigs = async () => {
  try {
    // 获取本地文件列表
    const list = await API.GetLocalConfigs();
    // 过滤掉运行时的 config.yaml，避免干扰
    localConfigs.value = (list || []).filter(name => name !== 'config.yaml');

    const data: any = await API.GetInitialData();
    if (data && data.activeConfig) {
      currentPath.value = data.activeConfig;
    } else if (data && data.configPath) {
      currentPath.value = data.configPath;
    }
  } catch (e) {
    console.error("同步状态失败:", e);
  }
};

const toggleMenu = (filename: string) => {
  activeMenu.value = activeMenu.value === filename ? null : filename;
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

const handleUpdate = async () => {
  if (!url.value) return;
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    await fetchConfigs();
    alert("订阅已更新！");
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

const handleImportLocal = async () => {
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

  // 判断是否是正在使用的配置
  if (isCurrentConfig(filename)) {
    const confirmClose = confirm(`【警告】"${filename}" 正在使用中！\n删除前需要强制关闭代理和虚拟网卡服务，是否继续？`);
    if (!confirmClose) return;

    // 确定删除正在使用的配置，先调用后端停止代理
    await API.StopProxy();
    currentPath.value = ''; // 前端状态清空
  } else {
    // 删除非正在使用的配置
    if (!confirm(`确定彻底删除 ${filename} 吗？`)) return;
  }

  try {
    await API.DeleteConfig(filename);
    await fetchConfigs(); // 重新拉取列表

    // 核心逻辑：如果删除后列表空了，同步清空 config.yaml
    if (localConfigs.value.length === 0) {
      await (API as any).ClearBaseConfig(); // 调用我们刚才在后端写的新方法
      alert("已删除所有订阅，并清空了运行环境。");
    }
  } catch (e) {
    alert("删除失败: " + e);
  }
};

// 3. 补全监听逻辑：修复 TS2552 找不到 onUnmounted 问题
onMounted(() => {
  fetchConfigs();
  (window as any).runtime.EventsOn("config-changed", (newName: string) => {
    currentPath.value = newName;
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
.micro-title { font-size: 0.7rem; color: var(--text-muted); text-transform: uppercase; font-weight: 700; }
.main-title { font-size: 1.4rem; margin-top: 4px; }

/* 按钮通用 */
.primary-btn { display: flex; align-items: center; gap: 8px; padding: 8px 16px; border-radius: 6px; border: none; background: var(--text-main); color: var(--accent-fg); font-weight: 600; cursor: pointer; }
.btn-icon { width: 14px; height: 14px; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { 100% { transform: rotate(360deg); } }

/* 卡片布局 */
.action-card { padding: 20px; border-radius: 12px; border: 1px solid var(--glass-border); background: var(--surface); margin-bottom: 20px; }
.card-title { font-size: 1rem; margin-bottom: 4px; }
.card-desc { font-size: 0.8rem; color: var(--text-sub); margin-bottom: 16px; }

.input-group { display: flex; gap: 10px; }
.input-wrapper { flex: 1; display: flex; align-items: center; background: var(--surface-hover); border: 1px solid var(--glass-border); border-radius: 8px; padding: 0 12px; }
.modern-input { flex: 1; background: transparent; border: none; color: inherit; padding: 10px 0; outline: none; font-size: 0.85rem; }
.action-btn { padding: 0 16px; border-radius: 8px; border: 1px solid var(--glass-border); background: transparent; color: var(--text-main); cursor: pointer; transition: 0.2s; }
.action-btn:hover { background: var(--surface-hover); }
.w-full-btn { width: 100%; padding: 12px; font-weight: 600; }

/* 列表部分 */
.list-title { font-size: 0.85rem; color: var(--text-sub); margin-bottom: 12px; }
.sub-card {
  position: relative; padding: 16px; border-radius: 10px; border: 1px solid var(--glass-border);
  background: var(--surface); margin-bottom: 12px; transition: all 0.2s ease;
}
.clickable { cursor: pointer; }
.clickable:hover { border-color: var(--text-muted); transform: translateY(-1px); box-shadow: 0 4px 12px rgba(0,0,0,0.05); }

/* 状态样式 */
.active-card { border-color: #10b981 !important; background: rgba(16, 185, 129, 0.02); }
.selecting-card { opacity: 0.7; pointer-events: none; border-style: dashed; }

.sub-header { display: flex; justify-content: space-between; margin-bottom: 12px; }
.sub-name { font-size: 0.95rem; font-weight: 600; }
.sub-path { font-size: 0.7rem; color: var(--text-muted); }

.status-badge { font-size: 0.7rem; font-weight: 700; padding: 3px 8px; border-radius: 4px; }
.status-badge.online { color: #10b981; background: rgba(16, 185, 129, 0.1); }
.loading-tag { color: var(--text-muted); background: var(--surface-hover); }

.sub-footer { display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--glass-border); pt: 12px; margin-top: 10px; padding-top: 10px; }
.sub-hint { font-size: 0.75rem; color: var(--text-sub); font-style: italic; }

/* 菜单项 */
.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: 4px; }
.icon-btn:hover { background: var(--surface-hover); color: var(--text-main); }

/* 修改原有的 sub-actions 防止被遮挡 */
.sub-actions { position: relative; }

/* 彻底修复菜单透明问题 */
.dropdown-menu {
  position: absolute;
  right: 0;
  top: 32px; /* 从 bottom 改为 top，向下弹出，防止在卡片底部被遮挡 */
  width: 140px;
  /* 强制设定背景色，加上 rgba 后备颜色防止变量丢失导致透明 */
  background: var(--surface, rgba(30, 30, 30, 0.95));
  border: 1px solid var(--glass-border, #444);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.6);
  z-index: 999; /* 强制拉高层级 */
  overflow: hidden;
  /* 如果系统支持毛玻璃，给菜单加上毛玻璃 */
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

.menu-item {
  width: 100%; text-align: left; padding: 10px 12px; border: none;
  background: transparent; /* 让背景透明，透出 .dropdown-menu 的颜色 */
  font-size: 0.8rem; color: var(--text-main, #eee); cursor: pointer;
  transition: background 0.2s;
}

/* 悬浮时加一个半透明的白底高亮 */
.menu-item:hover { background: rgba(255, 255, 255, 0.1); }
.menu-item.danger { color: #ef4444; }
.menu-divider { height: 1px; background: var(--glass-border); margin: 4px 0; }

.empty-state { padding: 30px; text-align: center; color: var(--text-muted); border: 1px dashed var(--glass-border); border-radius: 10px; }
</style>