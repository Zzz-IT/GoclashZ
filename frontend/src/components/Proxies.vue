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
              <span v-if="node.testing" class="testing-text">测试中...</span>
              <span v-else :class="['n-delay font-mono', getDelayColorClass(node.delay)]">
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
    // 👈 这里的 data.groups 现在在离线状态下也会有值了
    if (data && data.groups) {
      const processedGroups: any[] = [];
      Object.keys(data.groups).forEach(name => {
        const item = data.groups[name];
        const isGroupType = ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type);
        const isSystemReserved = ['GLOBAL', 'DIRECT', 'REJECT'].includes(name);

        if (isGroupType && !isSystemReserved) {
          const proxies = (item.all || []).map((memberName: string) => {
            return {
              name: memberName,
              now: item.now,
              delay: null, // 强制重置延迟显示
              testing: false
            };
          });
          processedGroups.push({ name, type: item.type, now: item.now, proxies });
        }
      });

      localGroups.value = processedGroups;

      // 核心修复：验证当前选中的组是否依然存在。如果不存在或为空，则切换到第一个可用组
      const isCurrentValid = processedGroups.some(g => g.name === currentGroup.value);
      if (!isCurrentValid && processedGroups.length > 0) {
        currentGroup.value = processedGroups[0].name;
      }
    }
  } catch (e) {
    console.error("加载失败", e);
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
    API.TestAllProxies(nodesArray).finally(() => {
        setTimeout(() => {
            isTesting.value = false;
            if(activeGroupData.value) {
               activeGroupData.value.proxies.forEach((n:any) => n.testing = false);
            }
        }, 1500);
    });
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
  // 1. 先注册监听器，确保在 loadData 过程中如果触发了事件也能捕获
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
      console.log("检测到配置已更新，正在重新加载节点列表...");
      // 彻底重置状态，强制触发验证逻辑
      currentGroup.value = '';
      await loadData();
  });

  // 2. 初始加载
  await loadData();
});

onUnmounted(() => {
  EventsOff("proxy-delay-update");
  EventsOff("config-changed");
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
  border-color: var(--glass-border);
  box-shadow: 0 2px 4px rgba(0,0,0,0.05);
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
  background: var(--text-main);
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
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  cursor: pointer;
  background: var(--surface);
  transition: all 0.2s ease;
}
.node-item:hover { background: var(--surface-hover); }
.node-item.active { border-color: var(--accent); border-width: 2px; background: var(--glass-panel); padding: 15px 19px; }

.node-info { flex: 1; min-width: 0; }
.n-name { font-size: 0.95rem; font-weight: 500; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-main); }

.n-latency-box {
  margin-left: 12px;
  padding: 6px 10px;
  border-radius: 6px;
  background: rgba(0,0,0,0.03);
  cursor: pointer;
  transition: background 0.2s;
  min-width: 54px;
  text-align: right;
}
.dark .n-latency-box { background: rgba(255,255,255,0.05); }
.n-latency-box:hover { background: rgba(0,0,0,0.08); }
.dark .n-latency-box:hover { background: rgba(255,255,255,0.1); }

.testing-text { font-size: 0.8rem; color: var(--text-muted); }
.n-delay { font-size: 0.85rem; font-weight: 600; }

.t-unknown { color: var(--text-muted); }
.t-fast { color: #10b981; }
.t-mid { color: #f59e0b; }
.t-slow { color: #ef4444; }
.t-fail { color: #dc2626; }

.empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--text-muted);
  font-style: italic;
}
</style>