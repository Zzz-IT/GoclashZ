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
const currentGroup = ref<string>(''); // 记录当前选中的组名
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
      Object.keys(data.groups).forEach(name => {
        const item = data.groups[name];
        const isGroupType = ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type);
        const isSystemReserved = ['GLOBAL', 'DIRECT', 'REJECT'].includes(name);

        if (isGroupType && !isSystemReserved) {
          const proxies = (item.all || []).map((memberName: string) => {
            return {
              name: memberName,
              now: item.now,
              // 关键修改：每次软件启动/重新加载页面时，无视内核缓存的历史记录，强制设为 null
              // 这样界面上永远默认显示 "--"，直到你手动测速
              delay: null,
              testing: false
            };
          });
          processedGroups.push({ name, type: item.type, now: item.now, proxies });
        }
      });

      localGroups.value = processedGroups;
      // 默认选中第一个组
      if (processedGroups.length > 0 && !currentGroup.value) {
        currentGroup.value = processedGroups[0].name;
      }
    }
  } catch (e) { console.error("数据加载失败", e); }
};

// 选中节点
const selectNode = async (groupName: string, nodeName: string) => {
  const targetGroup = localGroups.value.find(g => g.name === groupName);
  if (targetGroup && targetGroup.type !== 'Selector') return;
  try {
    await API.SelectProxy(groupName, nodeName);
    if (targetGroup) {
        targetGroup.now = nodeName; // 立即更新UI状态
    }
  } catch (e) { alert("切换失败: " + e); }
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
            // 如果后端没有返回结果，重置 testing 状态为 false 避免卡住
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
    node.delay = 0; // 0 表示失败
  } finally {
      setTimeout(() => { node.testing = false }, 5000);
  }
};

// 格式化延迟显示
const formatDelay = (delay: number | null) => {
  if (delay === null) return '--';
  if (delay === 0) return '-1ms';
  return `${delay}ms`;
};

// 获取颜色
const getDelayColorClass = (delay: number | null) => {
  if (delay === null) return 't-unknown';
  if (delay === 0) return 't-fail';
  if (delay < 250) return 't-fast';
  if (delay < 600) return 't-mid';
  return 't-slow';
};

// 找到脚本末尾的 onMounted 逻辑并替换
onMounted(async () => {
  // 1. 初始加载数据
  await loadData();

  // 2. 监听测速更新 (原有逻辑)
  EventsOn("proxy-delay-update", (data: any) => {
    localGroups.value.forEach(g => {
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) {
        node.delay = data.delay;
        node.testing = false;
      }
    });
  });

  // 3. 监听配置切换事件 (新增：解决切换配置后节点不刷新)
  EventsOn("config-changed", () => {
    console.log("检测到内核配置已更新，正在重新加载节点...");
    loadData(); // 使用你代码中定义好的加载函数
  });
});

// 记得在 onUnmounted 中销毁监听
onUnmounted(() => {
  EventsOff("proxy-delay-update");
  EventsOff("config-changed");
});
</script>

<style scoped>
.proxies-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

/* 顶部动作栏 */
.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  margin-bottom: 24px;
  background: var(--surface);
  border-radius: 12px;
}

/* 代理组选项卡 */
.group-tabs {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 4px; /* 防止滚动条遮挡 */
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
  font-size: 0.95rem; /* 稍微放大一点标签字号 */
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

/* 测速按钮 */
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

/* 滚动区 */
.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

.group-section { margin-bottom: 24px; }

/* 节点网格：加大了最小宽度 minmax 从 200px 提到 260px，间距从 12px 提到 16px */
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }

/* 节点项：增加了 padding 让盒子更大 */
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
/* 激活状态调整 padding 以抵消边框增粗带来的抖动 */
.node-item.active { border-color: var(--text-main); border-width: 2px; background: var(--glass-panel); padding: 15px 19px; }

.node-info { flex: 1; min-width: 0; }
/* 调大了节点名称的字体 */
.n-name { font-size: 0.95rem; font-weight: 500; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-main); }

/* 测速点击区：让点击区域更舒适 */
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
/* 调大了延迟数字的字体 */
.n-delay { font-size: 0.85rem; font-weight: 600; }

.t-unknown { color: var(--text-muted); }
.t-fast { color: #10b981; }
.t-mid { color: #f59e0b; }
.t-slow { color: #ef4444; }
.t-fail { color: #dc2626; }
</style>