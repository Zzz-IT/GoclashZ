<template>
  <div class="proxies-view">
    <div class="action-bar glass-panel">
      <div class="group-tabs">
        <button
          v-for="group in localGroups"
          :key="group.name"
          :class="['group-tab-btn', { active: currentGroup === group.name }]"
          @click="currentGroup = group.name"
        >
          {{ group.name }}
          <span class="count">({{ group.proxies?.length || 0 }})</span>
        </button>
      </div>

      <div class="global-actions">
        <button class="primary-action-btn" @click="testAllDelays" :disabled="isTesting || !activeGroupData">
          <span class="btn-icon" v-html="ICONS.zap"></span>
          {{ isTesting ? '测速中...' : '测速当前组' }}
        </button>
      </div>
    </div>

    <div class="scroll-content">
      <div v-if="activeGroupData" class="group-section">
        <div class="node-grid">
          <div v-for="node in activeGroupData.proxies" :key="node.name"
               :class="['node-item', { active: activeGroupData.now === node.name }]"
               @click="selectNode(activeGroupData.name, node.name)">

            <div class="node-info">
              <span class="n-name" :title="node.name">{{ node.name }}</span>
            </div>

            <div class="n-latency-box" @click.stop="testSingleDelay(node)">
              <div v-if="node.testing" class="scanner-container">
                <svg class="scanner-svg" viewBox="0 0 24 24">
                  <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
                  <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
                </svg>
              </div>
              
              <div v-else-if="node.delay === null" class="ping-idle">
                <svg class="idle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
              
              <span v-else :class="['n-delay font-mono', getDelayColorClass(node.delay)]">
                <span class="status-dot"></span>
                {{ formatDelay(node.delay) }}
              </span>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">
        <p>暂无代理组数据，请检查内核状态或订阅配置。</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

const ICONS = {
  zap: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>`
};

const localGroups = ref<any[]>([]);
const currentGroup = ref<string>('');
const isTesting = ref(false);

// 计算当前要显示的组的数据
const activeGroupData = computed(() => {
  return localGroups.value.find(g => g.name === currentGroup.value);
});

// 加载数据
const loadData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data && data.groups) {
      const processedGroups: any[] = [];
      
      // 👉 [新增] 优先使用后端传递的 groupOrder 数组，如果没有则降级使用 Object.keys
      const keys = (data.groupOrder && data.groupOrder.length > 0) 
                   ? data.groupOrder 
                   : Object.keys(data.groups);

      keys.forEach((name: string) => {
        const item = data.groups[name];
        if (!item) return; // 容错处理

        const isGroupType = ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type);
        const isSystemReserved = ['GLOBAL', 'DIRECT', 'REJECT'].includes(name);

        if (isGroupType && !isSystemReserved) {
          const proxies = (item.all || []).map((memberName: string) => {
            return {
              name: memberName,
              now: item.now,
              delay: null,
              testing: false
            };
          });
          processedGroups.push({ name, type: item.type, now: item.now, proxies });
        }
      });

      localGroups.value = processedGroups;

      const isCurrentValid = processedGroups.some(g => g.name === currentGroup.value);
      if (!isCurrentValid && processedGroups.length > 0) {
        currentGroup.value = processedGroups[0].name;
      }
    }
  } catch (e) {
    console.error("加载代理组失败", e);
  }
};

// 选中节点
const selectNode = async (groupName: string, nodeName: string) => {
  const targetGroup = localGroups.value.find(g => g.name === groupName);
  if (targetGroup && targetGroup.type !== 'Selector') return;
  try {
    await API.SelectProxy(groupName, nodeName);
    if (targetGroup) {
        targetGroup.now = nodeName;
    }
  } catch (e) {
    alert("切换失败: " + e);
  }
};

// 测速当前选中的组
const testAllDelays = () => {
  if (!activeGroupData.value) return;

  isTesting.value = true;
  const nodesArray = activeGroupData.value.proxies.map((n: any) => {
      n.delay = null;
      n.testing = true;
      return n.name;
  });

  if (nodesArray.length > 0) {
    // ⚠️ 核心修复：这里只发送触发指令，不依赖返回值结束状态
    API.TestAllProxies(nodesArray);
  } else {
    isTesting.value = false;
  }
};

// 单点测速
const testSingleDelay = async (node: any) => {
  if (node.testing) return;
  node.testing = true;
  node.delay = null;
  try {
    await API.TestAllProxies([node.name]);
  } catch (e) {
    console.error("单点测速失败:", e);
    node.delay = 0;
  } finally {
      setTimeout(() => { node.testing = false }, 5000);
  }
};

const formatDelay = (delay: number | null) => {
  if (delay === null) return '--';
  // 👇 修复：小于等于 0 统称为超时
  if (delay <= 0) return '超时'; 
  return `${delay}ms`;
};

const getDelayColorClass = (delay: number | null) => {
  if (delay === null) return 't-unknown';
  // 👇 修复：把 <= 0 的情况优先拦截，赋予红色失败样式
  if (delay <= 0) return 't-fail'; 
  if (delay < 250) return 't-fast';
  if (delay < 600) return 't-mid';
  return 't-slow';
};

onMounted(async () => {
  EventsOn("proxy-delay-update", (data: any) => {
    localGroups.value.forEach(g => {
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) {
        node.delay = data.delay;
        node.testing = false;
      }
    });
  });

  EventsOn("config-changed", async () => {
      currentGroup.value = '';
      await loadData();
  });

  // ⚠️ 核心修复：统一监听后端 Go 语言 WaitGroup 触发的全局完成事件
  EventsOn("proxy-test-finished", () => {
    isTesting.value = false;
    if(activeGroupData.value) {
        activeGroupData.value.proxies.forEach((n:any) => n.testing = false);
    }
  });

  await loadData();
});

onUnmounted(() => {
  EventsOff("proxy-delay-update");
  EventsOff("config-changed");
  EventsOff("proxy-test-finished"); // 记得注销
});
</script>

<style scoped>
.proxies-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  margin-bottom: 24px;
  background: var(--surface);
  border-radius: 12px;
}

.group-tabs {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 4px;
}
.group-tabs::-webkit-scrollbar { height: 4px; }
.group-tabs::-webkit-scrollbar-thumb { background-color: var(--glass-border); border-radius: 4px; }

.group-tab-btn {
  display: flex;
  align-items: center;
  padding: 8px 16px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-sub);
  border-radius: 8px;
  font-size: 0.95rem;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s ease;
}
.group-tab-btn:hover {
  color: var(--text-main);
  background: var(--surface-hover);
}
.group-tab-btn.active {
  background: var(--glass-panel);
  color: var(--text-main);
  font-weight: 600;
}
.group-tab-btn .count {
  font-size: 0.8rem;
  opacity: 0.6;
  margin-left: 6px;
}

.primary-action-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border: none;
  background: var(--accent);
  color: var(--accent-fg);
  border-radius: 8px;
  cursor: pointer;
  font-weight: 500;
  font-size: 0.9rem;
  transition: opacity 0.2s;
  white-space: nowrap;
}
.primary-action-btn:hover:not(:disabled) { opacity: 0.85; }
.primary-action-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-icon { width: 14px; height: 14px; }

.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

.group-section { margin-bottom: 24px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }

.node-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border: none;
  border-radius: 12px;
  cursor: pointer;
  background: var(--surface);
  transition: all 0.2s ease;
}
.node-item:hover { background: var(--surface-hover); }
.node-item.active { background: var(--glass-panel); font-weight: 600; }

.node-info { flex: 1; min-width: 0; }
.n-name { font-size: 0.95rem; font-weight: 500; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-main); }

.n-latency-box {
  margin-left: 12px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--surface);
  cursor: pointer;
  transition: background 0.2s;
  min-width: 54px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.dark .n-latency-box { background: var(--surface); }
.n-latency-box:hover { background: var(--surface-hover); }
.dark .n-latency-box:hover { background: var(--surface-hover); }

/* 环形扫描器样式 */
.scanner-container {
  width: 18px;
  height: 18px;
}
.scanner-svg {
  animation: rotate 1.5s linear infinite;
}
.scanner-track {
  fill: none;
  stroke: var(--surface-hover);
  stroke-width: 3;
}
.scanner-bar {
  fill: none;
  stroke: var(--text-main);
  stroke-width: 3;
  stroke-dasharray: 30, 100;
  stroke-linecap: round;
}

/* 未测试图标 */
.ping-idle {
  color: var(--text-muted);
  opacity: 0.4;
  transition: all 0.2s;
  display: flex;
}
.n-latency-box:hover .ping-idle {
  opacity: 1;
  color: var(--text-main);
  transform: scale(1.1);
}
.idle-icon { width: 14px; height: 14px; }

/* 状态圆点：摒弃发光效果，使用 JB 风格的实色块 */
.n-delay {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8rem;
  font-weight: 600;
}
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 2px; /* 方形圆角点，更符合 IDE 气质 */
}

/* 延迟颜色：纯灰度明度递进 */
.t-fast { color: var(--text-main); }
.t-fast .status-dot { background: var(--text-main); }

.t-mid { color: var(--text-sub); }
.t-mid .status-dot { background: var(--text-sub); }

.t-slow, .t-fail { color: var(--text-muted); }
.t-slow .status-dot, .t-fail .status-dot { background: var(--text-muted); }

@keyframes rotate {
  100% { transform: rotate(360deg); }
}

.t-unknown { color: var(--text-muted); }

.empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--text-muted);
  font-style: italic;
}
</style>