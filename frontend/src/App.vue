<template>
  <div class="layout" :class="{ dark: isDarkMode }">
    <div class="drag-handle" style="--wails-draggable:drag"></div>

    <Sidebar v-model:activeId="currentTab" />

    <main class="content">
      <header class="top-bar">
        <h2>{{ pageTitle }}</h2>
        <button @click="isDarkMode = !isDarkMode" class="theme-toggle">
          {{ isDarkMode ? '🌙' : '☀️' }}
        </button>
      </header>

      <div class="view-container">
        <div v-if="currentTab === 'home'" class="dashboard">
          <ModernCard class="status-card">
            <div class="status-info">
              <div class="indicator" :class="{ online: isRunning }"></div>
              <div>
                <h3>{{ isRunning ? '服务已启动' : '服务已停止' }}</h3>
                <p>Mixed Port: 7890</p>
              </div>
            </div>
            <button @click="handleToggleProxy" :class="['action-btn', { stop: isRunning }]">
              {{ isRunning ? '停止服务' : '开启服务' }}
            </button>
          </ModernCard>

          <div class="grid">
            <ModernCard>
              <h4>实时速度</h4>
              <div class="speed-box">
                <span>↑ 0 KB/s</span>
                <span>↓ 0 KB/s</span>
              </div>
            </ModernCard>
            <ModernCard>
              <h4>UWP 网络修复</h4>
              <p class="desc">一键解除沙箱回环限制</p>
              <button @click="handleFixUWP" class="secondary-btn">开始修复</button>
            </ModernCard>
          </div>
        </div>

        <div v-else class="placeholder">
          <p>{{ pageTitle }} 功能开发中...</p>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import Sidebar from './components/Sidebar.vue';
import ModernCard from './components/ModernCard.vue';

// ⚠️ 核心：导入 Wails 自动生成的 JS 绑定
// 注意路径：App.vue 在 src 下，wailsjs 在 src 的同级目录下
import { RunProxy, StopProxy, GetProxyStatus, FixUWPNetwork } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const currentTab = ref('home');
const isRunning = ref(false);
const isDarkMode = ref(false);

const pageTitle = computed(() => {
  const titles: Record<string, string> = { home: '系统概览', proxies: '节点选择', logs: '实时日志', settings: '设置' };
  return titles[currentTab.value];
});

// 处理代理开关逻辑
const handleToggleProxy = async () => {
  try {
    if (isRunning.value) {
      await StopProxy();
    } else {
      await RunProxy();
    }
    isRunning.value = await GetProxyStatus();
  } catch (err: any) {
    alert("操作失败: " + err);
  }
};

// 修复 UWP 网络逻辑
const handleFixUWP = async () => {
  try {
    await FixUWPNetwork();
    alert("✅ 修复完成，所有 UWP 应用现可访问代理。");
  } catch (err: any) {
    alert("修复失败: " + err);
  }
};

onMounted(async () => {
  // 初始化状态检查
  isRunning.value = await GetProxyStatus();

  // 监听后端发出的内核退出事件
  EventsOn("clash-exited", (msg: string) => {
    isRunning.value = false;
    console.warn("内核状态异常:", msg);
  });
});
</script>

<style scoped>
.layout { display: flex; height: 100vh; overflow: hidden; }
.drag-handle { position: fixed; top: 0; left: 0; right: 0; height: 30px; z-index: 999; }
.content { flex: 1; display: flex; flex-direction: column; background: var(--bg-primary); }
.top-bar { display: flex; justify-content: space-between; align-items: center; padding: 24px 40px; }
.view-container { padding: 0 40px 40px; overflow-y: auto; }

.status-card { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
.status-info { display: flex; align-items: center; }
.indicator { width: 12px; height: 12px; border-radius: 50%; background: #ef4444; margin-right: 16px; box-shadow: 0 0 8px #ef4444; }
.indicator.online { background: #10b981; box-shadow: 0 0 8px #10b981; }

.action-btn { background: var(--accent); color: white; border: none; padding: 12px 32px; border-radius: var(--radius-lg); cursor: pointer; font-weight: 600; }
.action-btn.stop { background: #ef4444; }

.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.speed-box { display: flex; gap: 24px; color: var(--accent); font-weight: 700; font-size: 1.1rem; margin-top: 12px; }
.desc { color: var(--text-muted); font-size: 0.9rem; margin-bottom: 16px; }
.secondary-btn { background: var(--accent-soft); color: var(--accent); border: none; padding: 10px 20px; border-radius: var(--radius-lg); cursor: pointer; font-weight: 600; }
.theme-toggle { background: none; border: none; font-size: 1.5rem; cursor: pointer; }
</style>