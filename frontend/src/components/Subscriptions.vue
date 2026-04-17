<template>
  <div class="subs-view">
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

    <div class="subs-list">
      <h4 class="list-title">活动配置</h4>

      <div class="sub-card active-card">
        <div class="sub-header">
          <div class="sub-info">
            <h4 class="sub-name">默认本地配置文件</h4>
            <span class="sub-url font-mono">{{ currentPath || 'core/bin/config.yaml' }}</span>
          </div>
          <div class="sub-status">
            <span class="status-badge online">● 正在使用</span>
          </div>
        </div>

        <div class="sub-footer">
          <span class="sub-time">状态: {{ loading ? '同步中' : '就绪' }}</span>
          <div class="sub-actions">
            <button class="icon-btn" title="编辑" v-html="ICONS.edit"></button>
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
  edit: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>`
};

const url = ref('');
const loading = ref(false);
const currentPath = ref('');

const handleUpdate = async () => {
  if (!url.value) return;
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    // 可选：触发一个事件让侧边栏状态刷新，或者弹出一个优美的 Toast
    alert("订阅更新并应用成功！");
  } catch (e) {
    alert("更新失败: " + e);
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  const data: any = await API.GetInitialData();
  // 假设后端能返回当前配置路径
  if (data && data.configPath) {
    currentPath.value = data.configPath;
  }
});
</script>

<style scoped>
.subs-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}

/* 顶部区域 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 24px;
}
.micro-title { font-size: 0.75rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
.main-title { font-size: 1.5rem; font-weight: 600; margin-top: 4px; color: var(--text-main); }

.primary-btn {
  display: flex; align-items: center; gap: 8px; padding: 10px 20px;
  border-radius: 8px; border: none; background: var(--text-main); color: var(--accent-fg);
  font-size: 0.85rem; font-weight: 500; cursor: pointer; transition: 0.2s;
}
.primary-btn:hover:not(:disabled) { transform: translateY(-1px); box-shadow: 0 4px 12px rgba(0,0,0,0.1); }
.primary-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.btn-icon { width: 14px; height: 14px; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { 100% { transform: rotate(360deg); } }

/* 导入卡片 */
.action-card {
  padding: 24px; border-radius: 12px; border: 1px solid var(--glass-border);
  background: var(--surface); margin-bottom: 32px;
}
.card-title { font-size: 1.1rem; font-weight: 500; margin-bottom: 6px; }
.card-desc { font-size: 0.85rem; color: var(--text-sub); margin-bottom: 20px; }

.input-group { display: flex; gap: 12px; }
.input-wrapper {
  flex: 1; display: flex; align-items: center; background: var(--surface-hover);
  border: 1px solid var(--glass-border); border-radius: 8px; padding: 0 12px;
  transition: 0.2s border-color;
}
.input-wrapper:focus-within { border-color: var(--text-sub); }
.input-icon { width: 16px; height: 16px; color: var(--text-muted); margin-right: 8px; }
.modern-input {
  flex: 1; background: transparent; border: none; outline: none; padding: 12px 0;
  color: var(--text-main); font-size: 0.9rem;
}
.modern-input::placeholder { color: var(--text-muted); }

.action-btn {
  padding: 0 24px; border-radius: 8px; border: 1px solid var(--glass-border);
  background: transparent; color: var(--text-main); font-weight: 500;
  cursor: pointer; transition: 0.2s;
}
.action-btn:hover:not(:disabled) { background: var(--surface-hover); }
.action-btn:disabled { opacity: 0.5; cursor: not-allowed; }

/* 列表卡片 */
.list-title { font-size: 0.9rem; font-weight: 600; color: var(--text-sub); margin-bottom: 12px; }
.sub-card {
  padding: 20px; border-radius: 12px; border: 1px solid var(--glass-border);
  background: var(--surface); transition: 0.2s;
}
.active-card { border-color: #10b981; box-shadow: 0 4px 12px rgba(16, 185, 129, 0.05); }

.sub-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.sub-name { font-size: 1.05rem; font-weight: 500; margin-bottom: 4px; }
.sub-url { font-size: 0.75rem; color: var(--text-muted); word-break: break-all; }

.status-badge.online {
  background: rgba(16, 185, 129, 0.1); color: #10b981;
  padding: 4px 10px; border-radius: 20px; font-size: 0.75rem; font-weight: 600;
}

.sub-footer {
  display: flex; justify-content: space-between; align-items: center;
  padding-top: 16px; border-top: 1px solid var(--glass-border);
}
.sub-time { font-size: 0.8rem; color: var(--text-sub); }
.sub-actions { display: flex; gap: 8px; }
.icon-btn {
  background: none; border: none; cursor: pointer; color: var(--text-sub);
  width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; transition: 0.2s;
}
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }
</style>