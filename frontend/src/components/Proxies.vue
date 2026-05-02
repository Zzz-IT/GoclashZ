<template>
  <div class="proxies-view">
    <div class="action-bar">
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
        <button class="primary-btn accent-btn" @click="testAllDelays" :disabled="isTesting || !activeGroupData">
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

            <div class="node-main-area">
              <div class="node-info">
                <span class="n-name" :title="node.name">{{ node.name }}</span>
              </div>
              <div class="node-meta">
                <span class="n-protocol">{{ node.type }}</span>
              </div>
            </div>

            <div 
              class="n-latency-box" 
              :class="{ disabled: isTesting || singleTesting }"
              @click.stop="testSingleDelay(node)"
            >
              <div v-if="node.testing" class="scanner-container">
                <svg class="scanner-svg" viewBox="0 0 24 24">
                  <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
                  <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
                </svg>
              </div>
              
              <div v-else-if="!globalState.proxyDelays[node.name] || globalState.proxyDelays[node.name].delay === null" class="ping-idle">
                <svg class="idle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
              
              <span v-else :class="['n-delay font-mono', getDelayColorClass(globalState.proxyDelays[node.name]?.delay ?? null)]">
                {{ formatDelay(globalState.proxyDelays[node.name]?.delay ?? null) }}
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
import { showAlert, globalState, updateProxyDelay } from '../store';
import { ICONS } from '../utils/icons';

const localGroups = ref<any[]>([]);
const currentGroup = ref<string>('');
const isTesting = ref(false);
const singleTesting = ref(false); // 🚀 新增：单点测速全局锁

// 👇 新增：引用用户设置与计时器控制
const isColorMode = ref(false);
const delayRetention = ref(true);
// 存储全局倒计时 ID，不放在 reactive 中防止不必要的响应式开销
const delayTimers: Record<string, number> = {};

// 🎯 修复：声明各个监听器的精准取消函数，防止 EventsOff 误杀全局 Store 监听
let unsubStart: () => void;
let unsubUpdate: () => void;
let unsubChanged: () => void;
let unsubFinish: () => void;
const delayRetentionTime = ref('long');

// 计算当前要显示的组的数据
const activeGroupData = computed(() => {
  return localGroups.value.find(g => g.name === currentGroup.value);
});

// 加载数据
const loadData = async () => {
  // 👇 新增：预加载配置以决定样式和保留逻辑
  try {
    const bh = await (API.GetAppBehavior as any)();
    if (bh) {
      isColorMode.value = bh.colorDelay === true;
      delayRetention.value = bh.delayRetention === true;
      delayRetentionTime.value = bh.delayRetentionTime || 'long';
    }
  } catch (e) {}

  try {
    const data: any = await API.GetInitialData();
    if (data && data.groups) {
      const processedGroups: any[] = [];
      
      const keys = (data.groupOrder && data.groupOrder.length > 0) 
                   ? data.groupOrder 
                   : Object.keys(data.groups);

      keys.forEach((name: string) => {
        const item = data.groups[name];
        if (!item) return;

        const isGroupType = ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type);
        const isSystemReserved = ['GLOBAL', 'DIRECT', 'REJECT'].includes(name);

        if (isGroupType && !isSystemReserved) {
          const proxies = (item.all || []).map((memberName: string) => {
            // 【核心修改】：同时去 data.proxies 和 data.groups 中查找节点详情
            const detail = (data.proxies && data.proxies[memberName]) || (data.groups && data.groups[memberName]);
            
            return {
              name: memberName,
              // 如果找到了真实详情，就取它的 type (如 vless, hysteria2)，否则回退到 PROXY
              type: detail && detail.type ? detail.type.toUpperCase() : 'PROXY',
              now: item.now,
              // 移除 delay: null, 交由 globalState.proxyDelays 全局接管
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

    // 🚀 核心修改：视觉无感穿透同步
    if (globalState.mode === 'global') {
        // 使用 catch 忽略潜在错误，不阻断主流程
        API.SelectProxy('GLOBAL', nodeName).catch(console.error);
    }
  } catch (e) {
    await showAlert("切换失败: " + e, '错误');
  }
};

// 测速当前选中的组
const testAllDelays = async () => {
  if (!activeGroupData.value || isTesting.value) return;

  isTesting.value = true;
  const nodesArray = activeGroupData.value.proxies.map((n: any) => {
      n.testing = true;
      return n.name;
  });

  if (nodesArray.length > 0) {
    try {
      await API.TestAllProxies(nodesArray);
    } catch (e) {
      isTesting.value = false;
      activeGroupData.value.proxies.forEach((n: any) => {
          n.testing = false;
      });
    }
  } else {
    isTesting.value = false;
  }
};

// 单点测速
const testSingleDelay = async (node: any) => {
  if (node.testing || isTesting.value || singleTesting.value) return;
  
  node.testing = true;
  singleTesting.value = true;

  try {
    const delay = await API.TestProxy(node.name);
    const retention = globalState.delayRetention ? globalState.delayRetentionTime : 'long';
    updateProxyDelay(node.name, delay, retention);
  } catch (e) {
    const msg = String(e);
    // 🛡️ 核心修复：如果是 busy 状态（说明已在批量测速中），则静默退出，不要覆盖已有结果为 0
    if (msg.includes('DELAY_TEST_BUSY') || msg.includes('已有测速任务') || msg.includes('busy')) {
      return;
    }

    console.error("单点测速失败:", e);
    const retention = globalState.delayRetention ? globalState.delayRetentionTime : 'long';
    updateProxyDelay(node.name, 0, retention);
  } finally {
    node.testing = false;
    singleTesting.value = false;
  }
};

const formatDelay = (delay: number | null) => {
  if (delay === null) return '--';
  if (delay <= 0) return '超时'; 
  return `${delay}ms`;
};

// 👇 改写颜色计算逻辑
const getDelayColorClass = (delay: number | null) => {
  if (delay === null) return isColorMode.value ? 'c-unknown' : 't-unknown';
  if (delay <= 0) return isColorMode.value ? 'c-fail' : 't-fail'; 
  
  if (delay <= 300) return isColorMode.value ? 'c-fast' : 't-fast';
  if (delay <= 600) return isColorMode.value ? 'c-mid' : 't-mid';
  return isColorMode.value ? 'c-slow' : 't-slow';
};

onMounted(async () => {
  // 🎯 接收并保存取消函数
  unsubStart = EventsOn("proxy-test-start", (nodeName: string) => {
    if (!localGroups.value) return;
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      const node = g.proxies.find((n: any) => n.name === nodeName);
      if (node) {
        node.testing = true;
      }
    });
  });

  // 接收到“结果”信号，仅负责关掉 UI 上的转圈动画
  unsubUpdate = EventsOn("proxy-delay-update", (data: any) => {
    if (!data || !localGroups.value) return;
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) {
        node.testing = false;
      }
    });
  });

  unsubChanged = EventsOn("config-changed", async () => {
      currentGroup.value = '';
      await loadData();
  });

  unsubFinish = EventsOn("proxy-test-finished", () => {
    isTesting.value = false;
  });

  await loadData();
});

onUnmounted(() => {
  // 🎯 组件销毁时，精准反注册当前组件的监听，绝不误伤 store.ts 的全局监听
  if (unsubStart) unsubStart();
  if (unsubUpdate) unsubUpdate();
  if (unsubChanged) unsubChanged();
  if (unsubFinish) unsubFinish();
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
  align-items: center;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 4px;
  margin-bottom: -4px;
  flex: 1;
  min-width: 0;
  margin-right: 16px;
  user-select: none;
  -webkit-user-select: none;
}

.global-actions {
  flex-shrink: 0;
  white-space: nowrap;
}

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
  user-select: none;
  -webkit-user-select: none;
  flex-shrink: 0;
}
.group-tab-btn:hover {
  color: var(--text-main);
  background: var(--surface-hover);
}
.group-tab-btn.active {
  background: var(--surface-panel);
  color: var(--text-main);
  font-weight: 600;
}
.group-tab-btn .count {
  font-size: 0.8rem;
  opacity: 0.6;
  margin-left: 6px;
}

.btn-icon { width: 14px; height: 14px; }

.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

.group-section { margin-bottom: 24px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }

.node-item {
  display: flex;
  justify-content: space-between;
  align-items: stretch;
  padding: 16px 20px;
  height: 84px;
  border: none;
  border-radius: 12px;
  cursor: pointer;
  background: var(--surface);
  transition: all 0.2s ease;
}
.node-item:hover { background: var(--surface-hover); }
.node-item.active { 
  background: var(--accent); 
  color: var(--accent-fg);
  font-weight: 600; 
}

.node-item.active .n-name { 
  color: var(--accent-fg); 
}

/* 🎯 修复：保持字重一致，仅通过不透明度拉开深浅，避免黑色模式下字体显得过细 */
.node-item.active .t-fast { color: var(--accent-fg) !important; opacity: 1; font-weight: 700; }
.node-item.active .t-mid { color: var(--accent-fg) !important; opacity: 0.6; font-weight: 700; }
.node-item.active .t-slow { color: var(--accent-fg) !important; opacity: 0.3; font-weight: 700; }
.node-item.active .t-fail { color: var(--accent-fg) !important; opacity: 1; font-weight: 700; }
.node-item.active .t-unknown { color: var(--accent-fg) !important; opacity: 0.3; font-weight: 700; }

/* 确保选中节点（active）的图标完全不透明且使用反色（白色） */
.node-item.active .ping-idle {
  color: var(--accent-fg) !important; /* 在日间模式选中时为白色 */
  opacity: 1 !important;
  transform: none !important;        /* 选中后禁止悬停缩放，保持静止 */
}

/* 彻底移除选中状态下延迟框的背景色块 */
.node-item.active .n-latency-box,
.node-item.active .n-latency-box:hover {
  background: transparent !important;
  transform: none !important;
}

/* 确保测速中的动画在选中态也是反色 */
.node-item.active .scanner-track {
  stroke: var(--accent-fg) !important;
  stroke-opacity: 0.15; /* 🚀 核心：让底圈更浅、更通透 */
}
.node-item.active .scanner-bar {
  stroke: var(--accent-fg) !important;
}

.node-main-area { flex: 1; display: flex; flex-direction: column; justify-content: space-between; min-width: 0; }

.node-info { flex: 1; min-width: 0; }
.n-name { font-size: 0.95rem; font-weight: 500; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-main); }

.node-meta { display: flex; align-items: center; }

/* ================================ */
/* 协议标签：精确视觉对齐方案          */
/* ================================ */
.n-protocol {
  font-size: 0.65rem; 
  font-weight: 800;
  /* 修正圆角：从 Pill 形状改为 4px 小圆角，与 12px 的卡片外框形成视觉嵌套感 */
  border-radius: 4px; 
  padding: 1px 6px; 
  width: fit-content;
  text-transform: uppercase;
  display: flex;
  align-items: center;
}

/* 情况 1：未选中的节点卡片 */
/* 照抄顶部“已选中的代理组 Tab”的高亮逻辑 */
.node-item:not(.active) .n-protocol {
  background: var(--surface-panel); 
  color: var(--text-main);
}

/* 情况 2：已选中的节点卡片 (反色态) */
/* 严格照抄 Overview.vue 中 .on .icon-ring 的夜间/激活模式处理逻辑 */
.node-item.active .n-protocol { 
  /* 这里的 rgba(128, 128, 128, 0.25) 是 Overview 中定义的标准半透明遮罩 */
  background: rgba(128, 128, 128, 0.25) !important; 
  color: var(--accent-fg) !important;
  /* 确保完全没有边框或阴影干扰，保持通透感 */
  box-shadow: none; 
  border: none;
}

.n-latency-box {
  flex-shrink: 0 !important;
  white-space: nowrap;
  min-width: max-content;
  margin-left: 12px;
  padding: 6px 10px;
  border-radius: 6px;
  background: transparent !important;
  cursor: pointer;
  min-width: 54px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: opacity 0.2s;
}
.n-latency-box:hover {
  opacity: 0.6;
}
.n-latency-box.disabled {
  pointer-events: none;
}

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

.ping-idle {
  color: var(--text-sub);      /* 使用比 muted 更深的颜色 */
  opacity: 0.7;                /* 提高初始不透明度，确保可见 */
  transition: all 0.2s ease;
  display: flex;
}
.n-latency-box:hover .ping-idle {
  opacity: 1;
  color: var(--text-main);
  transform: scale(1.1);
}
.idle-icon { width: 14px; height: 14px; }

/* ================================ */
/* 延迟颜色逻辑: 默认深浅 vs 绿黄红      */
/* ================================ */
.n-delay {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8rem;
  font-weight: 600;
}

/* 默认：三级深浅逻辑 */
.t-fast { color: var(--text-main); font-weight: 700; }   /* 0-300ms 最深 */
.t-mid { color: var(--text-sub); }                      /* 300-600ms 中等 */
.t-slow { color: var(--text-muted); opacity: 0.7; }     /* >600ms 最浅 */
.t-fail { color: var(--text-main); font-weight: 700; }  /* 超时 最深 */
.t-unknown { color: var(--text-muted); }

/* 彩色逻辑：必须加更高权重，防止被 .active 覆盖 */
.c-fast { color: #10b981 !important; font-weight: 700; } /* 绿 */
.c-mid { color: #f59e0b !important; font-weight: 700; }  /* 黄 */
.c-slow { color: #ef4444 !important; font-weight: 700; } /* 红 */
.c-fail { color: #ef4444 !important; font-weight: 800; } /* 红，加粗 */
.c-unknown { color: var(--text-muted); }


@keyframes rotate {
  100% { transform: rotate(360deg); }
}

.empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--text-muted);
  font-style: italic;
}
</style>