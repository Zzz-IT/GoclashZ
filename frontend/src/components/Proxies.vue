<template>
  <div class="proxies-view">
    <div class="proxy-toolbar page-sticky-mask">
      <div class="proxy-toolbar-card">
        <div class="proxy-tabs-viewport">
          <div class="proxy-tabs-track" ref="tabsTrackRef">
            <button
              v-for="group in localGroups"
              :key="group.name"
              :ref="(el) => { if (group.name === currentGroup) activeTabEl = el as HTMLElement | null }"
              :class="['proxy-tab-btn', { active: currentGroup === group.name }]"
              @click="currentGroup = group.name"
            >
              {{ group.name }}
              <span class="count">({{ group.proxies?.length || 0 }})</span>
            </button>
            <div class="proxy-tab-slider" :class="{ animated: sliderReady }" :style="{ ...sliderStyle, visibility: sliderVisible ? 'visible' : 'hidden' }"></div>
          </div>
        </div>

        <div class="proxy-toolbar-actions">
          <button
            class="primary-btn accent-btn proxy-test-btn"
            @click="testAllDelays"
            :disabled="isTesting || !activeGroupData"
          >
            <span class="btn-icon" v-html="ICONS.zap"></span>
            {{ isTesting ? '测速中...' : '测速当前组' }}
          </button>
        </div>
      </div>

      <div v-if="activeGroupData" class="proxy-sub-toolbar">
        <div class="sub-toolbar-actions">
          <button class="tool-btn" :class="{ 'active': sortState === 1 }" @click="toggleSort" title="按延迟排序">
            <span class="btn-icon" v-html="ICONS.sort"></span>
          </button>
          <button class="tool-btn" @click="locateActiveNode" title="定位当前节点">
            <span class="btn-icon" v-html="ICONS.target"></span>
          </button>
          <button class="tool-btn" :class="{ 'active': showSearch || searchQuery }" @click="toggleSearch" title="搜索节点">
            <span class="btn-icon" v-html="ICONS.search"></span>
          </button>
          <div class="search-wrapper" :class="{ 'is-active': showSearch }">
            <input 
              ref="searchInputRef"
              v-model="searchQuery" 
              type="text" 
              class="search-input" 
              placeholder="搜索节点..." 
              @keyup.esc="toggleSearch"
            />
          </div>
        </div>
        <div class="sub-toolbar-spacer"></div>
      </div>
    </div>

    <div class="scroll-content">
      <div v-if="activeGroupData" :key="activeGroupData.name" class="group-section">
        <div class="node-grid">
            <div v-for="node in filteredAndSortedNodes" :key="node.name"
                 :class="['node-item', { active: activeGroupData.now === node.name, readonly: !activeGroupData.selectable }]"
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
              :class="{ disabled: isTesting }"
              :title="globalState.proxyDelays[node.name]?.message || ''"
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
              
              <span v-else :class="['n-delay font-mono', getDelayColorClass(globalState.proxyDelays[node.name])]">
                {{ formatDelay(globalState.proxyDelays[node.name]) }}
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
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed, watch, nextTick } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { showAlert, globalState, refreshOutboundIP } from '../store';
import { ICONS } from '../utils/icons';

const localGroups = ref<any[]>([]);
const currentGroup = ref<string>(localStorage.getItem('goclashz_proxyGroup') || '');
const isTesting = ref(false);

const isColorMode = ref(false);

const tabsTrackRef = ref<HTMLElement | null>(null);
const activeTabEl = ref<HTMLElement | null>(null);
const sliderStyle = ref({ left: '0px', width: '0px' });
const sliderReady = ref(false);
const sliderVisible = ref(false);

const updateSlider = () => {
  if (localGroups.value.length === 0) return;
  const track = tabsTrackRef.value;
  const btn = activeTabEl.value;
  if (track && btn) {
    sliderStyle.value = {
      left: `${btn.offsetLeft}px`,
      width: `${btn.offsetWidth}px`,
    };
  }
};

const resetSlider = () => {
  if (localGroups.value.length === 0) return;
  sliderReady.value = false;
  sliderVisible.value = false;
  nextTick(() => {
    updateSlider();
    // 强制触发一次重排，让浏览器“记住”此时无动画的起始位置
    if (tabsTrackRef.value) void tabsTrackRef.value.offsetHeight;
    
    requestAnimationFrame(() => {
      sliderVisible.value = true;
      sliderReady.value = true;
    });
  });
};

watch(currentGroup, (v) => { if (v) localStorage.setItem('goclashz_proxyGroup', v); nextTick(updateSlider); });
watch(() => localGroups.value.length, () => { resetSlider(); });
onMounted(() => resetSlider());
onActivated(() => {
  // Give browser time to restore DOM and scroll positions before re-enabling animation
  setTimeout(() => { sliderReady.value = true; }, 50);
});
onDeactivated(() => {
  sliderReady.value = false;
});

let unsubStart: (() => void) | null = null;
let unsubUpdate: (() => void) | null = null;
let unsubChanged: (() => void) | null = null;
let unsubFinish: (() => void) | null = null;
let unsubProxyState: (() => void) | null = null;

const storedSort = parseInt(localStorage.getItem('goclashz_proxySortState') || '0');
const sortState = ref(isNaN(storedSort) ? 0 : storedSort);

watch(sortState, (newVal) => {
  localStorage.setItem('goclashz_proxySortState', newVal.toString());
});
const searchQuery = ref('');
const showSearch = ref(false);
const searchInputRef = ref<HTMLInputElement | null>(null);

const toggleSearch = () => {
  showSearch.value = !showSearch.value;
  if (!showSearch.value) {
    searchQuery.value = '';
  } else {
    nextTick(() => {
      searchInputRef.value?.focus();
    });
  }
};

const toggleSort = () => {
  sortState.value = sortState.value === 0 ? 1 : 0;
};

const locateActiveNode = () => {
  nextTick(() => {
    const activeEl = document.querySelector('.node-item.active');
    if (activeEl) {
      activeEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  });
};

const filteredAndSortedNodes = computed(() => {
  if (!activeGroupData.value || !activeGroupData.value.proxies) return [];
  let nodes = [...activeGroupData.value.proxies];
  
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    nodes = nodes.filter(n => n.name.toLowerCase().includes(q));
  }

  if (sortState.value === 1) {
    nodes.sort((a, b) => {
      const delayA = globalState.proxyDelays[a.name]?.delay;
      const delayB = globalState.proxyDelays[b.name]?.delay;
      const vA = (delayA !== undefined && delayA !== null && delayA > 0) ? delayA : 999999;
      const vB = (delayB !== undefined && delayB !== null && delayB > 0) ? delayB : 999999;
      return vA - vB;
    });
  }
  
  return nodes;
});

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
          processedGroups.push({ 
            name, 
            type: item.type, 
            now: item.now, 
            selectable: isManuallySelectableGroup(item.type),
            proxies 
          });
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

const isManuallySelectableGroup = (type: string) => {
  return ['Selector', 'Fallback'].includes(type);
};

// 选中节点
const selectNode = async (groupName: string, nodeName: string) => {
  const targetGroup = localGroups.value.find(g => g.name === groupName);
  
  if (targetGroup && !isManuallySelectableGroup(targetGroup.type)) {
    return;
  }

  try {
    await API.SelectProxy(groupName, nodeName);
    
    // 🚀 核心修复：以内核返回的真实 now 为准，不再本地乐观修改
    // 重新拉取一次数据确保同步
    await loadData();
    
    // 触发刷新出站 IP
    setTimeout(() => refreshOutboundIP(), 1500);
  } catch (e) {
    await showAlert("切换失败: " + e, '错误');
  }
};

// 🎯 修复：增加对运行状态和配置变化的响应式刷新，打通离线态与运行态的数据源切换
const stopRunningWatch = watch(
  () => globalState.isRunning,
  async (running) => {
    // 只有当状态发生变化时才 reload，避免不必要的请求
    await loadData();
  }
);

const stopActiveConfigWatch = watch(
  () => globalState.activeConfigId,
  async () => {
    await loadData();
  }
);

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
  if (node.testing || isTesting.value) return;
  
  node.testing = true;

  try {
    await API.TestProxy(node.name);
    // 🛡️ 核心修复：不再手动调用 updateProxyDelay
    // 因为后端会通过 proxy-delay-update 事件发送归一化后的 leaf 和 group 派生结果
    // 这样能确保主页面和组页面数据百分之百同步，且处理了嵌套组的派生逻辑
  } catch (e) {
    const msg = String(e);
    // 🛡️ 核心修复：如果是 busy 状态（说明已在批量测速中），则静默退出，不要覆盖已有结果为 0
    if (msg.includes('DELAY_TEST_BUSY') || msg.includes('已有测速任务') || msg.includes('busy')) {
      return;
    }

    console.error("单点测速失败:", e);
  } finally {
    node.testing = false;
  }
};

const formatDelay = (delayInfo: any) => {
  if (!delayInfo) return '--';
  const { delay, status } = delayInfo;
  
  // 🚀 核心优化：只要存在大于 0 的历史有效延迟，就优先展示数字，避免测速波动导致的结果瞬间消失
  if (delay > 0) return `${delay}ms`;
  
  if (status === 'timeout') return '超时';
  if (status === 'connect-error') return '连接失败';
  return '失败';
};

// 👇 改写颜色计算逻辑
const getDelayColorClass = (delayInfo: any) => {
  if (!delayInfo || delayInfo.delay === null) return isColorMode.value ? 'c-unknown' : 't-unknown';
  const { delay, status } = delayInfo;

  if (status !== 'success') return isColorMode.value ? 'c-fail' : 't-fail'; 
  
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
      await loadData();
  });

  unsubFinish = (EventsOn as any)("proxy-test-finished", () => {
    isTesting.value = false;
    
    // 🚀 核心修复：清理所有组内节点的转圈状态，防止长尾节点卡死
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      g.proxies.forEach((n: any) => {
        n.testing = false;
      });
    });
  });

  // 🚀 新增：轻量化策略组状态同步，处理 Fallback/URLTest 的自动切换
  unsubProxyState = (EventsOn as any)("proxy-state-sync", async (states: any[]) => {
    if (!Array.isArray(states)) return;

    // 🛡️ 核心优化：如果收到一个当前 UI 中不存在的组（可能是配置切换后的残余或新发现的组）
    // 则执行全量 reload 以确保拓扑结构百分之百对齐
    const knownNames = new Set(localGroups.value.map(g => g.name));
    const hasUnknown = states.some((s: any) => s && s.name && !knownNames.has(s.name));
    if (hasUnknown) {
        await loadData();
        return;
    }

    const nowMap = new Map<string, string>();
    states.forEach((s: any) => {
      if (s && s.name) {
        nowMap.set(s.name, s.now || '');
      }
    });

    localGroups.value.forEach(g => {
      if (nowMap.has(g.name)) {
        g.now = nowMap.get(g.name);
      }
    });
  });

  await loadData();
});

onUnmounted(() => {
  unsubStart?.();
  unsubUpdate?.();
  unsubChanged?.();
  unsubFinish?.();
  unsubProxyState?.();

  stopRunningWatch();
  stopActiveConfigWatch();
});
</script>

<style scoped>
.proxies-view { 
  display: flex; 
  flex-direction: column; 
  min-height: 100%; 
  overflow: visible; 
}

.proxy-toolbar {
  width: 100%;
  --sticky-mask-bleed: 4px;
}

.proxy-toolbar-card {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: flex-start; /* 改为 flex-start 以精确控制垂直位置 */
  width: 100%;
  min-height: 60px;
  padding: 12px 12px 4px 12px; /* 顶部12px，底部4px。配合 track 的 44px 高度，总高度精确为 60px */
  gap: 12px;
  border-radius: 12px;
  background: var(--surface);
}

.proxy-tabs-viewport {
  flex: 1;
  min-width: 0;
  height: 44px; /* 36px 按钮 + 8px 滚动条空间 */
  border-radius: 8px;
}

.proxy-tabs-track {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  height: 44px;
  min-width: 0;
  width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  padding: 0 0 8px 0;
  user-select: none;
  -webkit-user-select: none;
  overscroll-behavior-x: contain;
  position: relative;
}

.proxy-tabs-track::-webkit-scrollbar {
  height: 4px; /* 与全局竖向滚动条宽度一致 */
  display: block;
}

.proxy-tabs-track::-webkit-scrollbar-button {
  display: none;
}

.proxy-tabs-track::-webkit-scrollbar-track {
  background: transparent;
}

.proxy-tabs-track::-webkit-scrollbar-thumb {
  background: var(--text-muted);
  border-radius: 999px;
}

.proxy-tabs-track::-webkit-scrollbar-thumb:hover {
  background: var(--text-sub);
}

.proxy-tab-btn {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  padding: 0 16px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-sub);
  font-size: 0.95rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  position: relative;
  z-index: 1;
  transition: color 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.proxy-tab-btn:hover {
  color: var(--text-main);
}

.proxy-tab-btn.active {
  color: var(--accent-fg);
  font-weight: 800;
}

.proxy-tab-btn .count {
  margin-left: 6px;
  font-size: 0.8rem;
  opacity: 0.6;
}

.proxy-tab-btn.active .count {
  opacity: 0.8;
}

.proxy-tab-slider {
  position: absolute;
  top: 0;
  height: 36px;
  background: var(--accent);
  border-radius: 8px;
  z-index: 0;
  pointer-events: none;
}

.proxy-tab-slider.animated {
  transition: left 0.3s cubic-bezier(0.4, 0, 0.2, 1), width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.proxy-toolbar-actions {
  flex-shrink: 0;
  display: flex;
  align-items: flex-start;
  height: 36px;
}

.proxy-test-btn {
  height: 36px;
  min-width: 132px;
  padding: 0 18px;
  border-radius: 8px;
  white-space: nowrap;
}

.btn-icon { width: 14px; height: 14px; }

.proxy-sub-toolbar {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  margin-top: 4px;
  margin-bottom: 0;
  padding: 0 4px;
}

.sub-toolbar-spacer {
  flex: 1;
}

.sub-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search-wrapper {
  overflow: hidden;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  width: 0;
  display: flex;
  align-items: center;
  opacity: 0;
}

.search-wrapper.is-active {
  width: 180px;
  opacity: 1;
}

.search-input {
  width: 100%;
  height: 32px;
  padding: 0 14px;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-main);
  font-size: 0.85rem;
  outline: none;
}

.tool-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-sub);
  cursor: pointer;
  transition: all 0.2s ease;
}

.tool-btn:hover {
  background: var(--surface-hover);
  color: var(--text-main);
}

.tool-btn.active {
  color: var(--accent);
  background: var(--surface); /* 启用状态与节点卡片(未选中)背景色一致 */
}

.tool-btn .btn-icon {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.scroll-content { 
  flex: 1; 
  min-height: 0;
  overflow: visible; 
  padding-top: 4px;
}

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
.node-item.readonly {
  cursor: default;
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