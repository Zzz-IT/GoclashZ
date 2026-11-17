<template>
  <div class="proxies-view ui-page-shell">
    <div class="proxy-toolbar ui-page-header">
      <div class="proxy-toolbar-card">
        <div class="proxy-tabs-viewport">
          <!-- 自绘滚动指示条：画在按钮行下方，不覆盖任何标签；只有它能拖动 -->
          <i
            v-show="railThumbVisible"
            class="proxy-rail-thumb"
            :class="{ 'is-dragging': thumbDragging }"
            :style="railThumbStyle"
            role="scrollbar"
            aria-orientation="horizontal"
            :aria-valuenow="Math.round(thumbScrollLeft)"
            aria-valuemin="0"
            :aria-valuemax="thumbScrollMax"
            @pointerdown="onThumbPointerDown"
            @pointermove="onThumbPointerMove"
            @pointerup="onThumbPointerUp"
            @pointercancel="onThumbPointerUp"
          ></i>
          <div
            class="proxy-tabs-track"
            ref="tabsTrackRef"
            @wheel="onTrackWheel"
            @scroll.passive="updateTrackOverflow"
          >
            <button
              v-for="group in localGroups"
              :key="group.name"
              :ref="(el) => { if (group.name === currentGroup) activeTabEl = el as HTMLElement | null }"
              :class="['proxy-tab-btn', { active: currentGroup === group.name }]"
              @click="onTabClick(group.name)"
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
            @click="toggleTestAllDelays"
            :disabled="!activeGroupData"
            :title="isTesting ? t('proxies.stopTesting') : t('proxies.speedTestGroup')"
          >
            <span class="btn-icon" v-html="isTesting ? ICONS.x : ICONS.zap"></span>
            {{ isTesting ? t('proxies.stopTesting') : t('proxies.speedTestGroup') }}
          </button>
        </div>
      </div>

      <div v-if="activeGroupData" class="proxy-sub-toolbar">
        <div class="sub-toolbar-actions">
          <button class="tool-btn" :class="{ 'active': sortState === 1 }" @click="toggleSort" :title="t('proxies.sortByLatency')">
            <span class="btn-icon" v-html="ICONS.sort"></span>
          </button>
          <button class="tool-btn" @click="locateActiveNode" :title="t('proxies.locateActiveNode')">
            <span class="btn-icon" v-html="ICONS.target"></span>
          </button>
          <button class="tool-btn" :class="{ 'active': showSearch || searchQuery }" @click="toggleSearch" :title="t('proxies.searchProxies')">
            <span class="btn-icon" v-html="ICONS.search"></span>
          </button>
          <div class="search-wrapper" :class="{ 'is-active': showSearch }">
            <input 
              ref="searchInputRef"
              v-model="searchQuery" 
              type="text" 
              class="search-input" 
              :placeholder="t('proxies.searchPlaceholder')" 
              @keyup.esc="toggleSearch"
            />
          </div>
        </div>
        <div class="sub-toolbar-spacer"></div>
        <div class="sub-toolbar-layout">
          <button class="tool-btn layout-toggle-btn" @click="cycleCardMode" :title="currentCardModeTitle">
            <span class="btn-icon" :key="cardMode" v-html="currentCardModeIcon"></span>
          </button>
        </div>
      </div>
    </div>

    <div class="scroll-content ui-page-body">
      <div v-if="activeGroupData" :key="activeGroupData.name" class="group-section">
        <TransitionGroup 
          tag="div" 
          class="node-grid" 
          :class="`size-${cardMode}`" 
          :style="gridStyle"
          name="node-card"
        >
            <div v-for="node in filteredAndSortedNodes" :key="node.name"
                 :class="['node-item', 'ui-surface-card', 'ui-hover-surface', { active: activeGroupData.now === node.name, readonly: !activeGroupData.selectable }]"
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
              :class="{ disabled: node.testing }"
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
        </TransitionGroup>
      </div>
      <div v-else class="empty-state">
        <p>{{ t('proxies.noGroupsHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed, watch, nextTick, TransitionGroup } from 'vue';
import { App } from '../api';
import { Events } from '@wailsio/runtime';
import { showAlert, showToast, globalState, scheduleOutboundIPRefresh, setProxyCardMode, ProxyCardMode } from '../store';
import { ICONS } from '../utils/icons';
import { toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import { t } from '../locales';

const localGroups = ref<any[]>([]);
const currentGroup = ref<string>(localStorage.getItem('goclashz_proxyGroup') || '');
const isTesting = ref(false);

const isColorMode = ref(false);

const tabsTrackRef = ref<HTMLElement | null>(null);
const activeTabEl = ref<HTMLElement | null>(null);
const sliderStyle = ref({ left: '0px', width: '0px' });
const sliderReady = ref(false);
const sliderVisible = ref(false);

/* ------------------------------------------------------------------
 * 分组条横向溢出处理
 * 轨道不再显示原生滚动条（它会把轨道撑高 8px，导致组名与测速按钮不居中），
 * 所以这里补三件事：滚轮横向滚动、按住拖动滚动、左右边缘渐隐提示。
 * ---------------------------------------------------------------- */
const canScrollLeft = ref(false);
const canScrollRight = ref(false);

/* 自绘横向滚动指示条（原生滚动条会压住选项卡底边，所以改成画在按钮行下方） */
const railThumbVisible = ref(false);
const railThumbStyle = ref<Record<string, string>>({ left: '0px', width: '0px' });

const updateRailThumb = () => {
  const el = tabsTrackRef.value;
  if (!el) return;
  const overflow = el.scrollWidth - el.clientWidth;
  if (overflow <= 4) {
    railThumbVisible.value = false;
    return;
  }
  const track = el.clientWidth;
  const width = Math.max(28, Math.round((track / el.scrollWidth) * track));
  const left = Math.round((el.scrollLeft / overflow) * (track - width));
  railThumbVisible.value = true;
  railThumbStyle.value = { left: left + 'px', width: width + 'px' };
  thumbScrollLeft.value = el.scrollLeft;
  thumbScrollMax.value = overflow;
};

const updateTrackOverflow = () => {
  const el = tabsTrackRef.value;
  if (!el) return;
  canScrollLeft.value = el.scrollLeft > 2;
  canScrollRight.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 2;
  updateRailThumb();
};

/** 竖向滚轮也用于横向浏览分组（触控板横向手势照旧生效） */
const onTrackWheel = (e: WheelEvent) => {
  const el = tabsTrackRef.value;
  if (!el || el.scrollWidth <= el.clientWidth) return;
  const delta = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
  if (!delta) return;
  e.preventDefault();
  el.scrollLeft += delta;
};

/* 只有「滚动指示条」可以拖动（像真正的滚动条那样）；
   选项卡区域只负责点击切换分组，不再被拖拽逻辑抢占，避免"想点分组却把条拖走"。 */
const thumbDragging = ref(false);
const thumbScrollLeft = ref(0);
const thumbScrollMax = ref(0);
let thumbStartX = 0;
let thumbStartScroll = 0;

const onThumbPointerDown = (e: PointerEvent) => {
  const el = tabsTrackRef.value;
  if (!el || e.button !== 0) return;
  e.preventDefault();
  thumbDragging.value = true;
  thumbStartX = e.clientX;
  thumbStartScroll = el.scrollLeft;
  (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
};

const onThumbPointerMove = (e: PointerEvent) => {
  const el = tabsTrackRef.value;
  const thumb = e.currentTarget as HTMLElement;
  if (!thumbDragging.value || !el) return;
  const maxScroll = el.scrollWidth - el.clientWidth;
  const thumbWidth = thumb.offsetWidth;
  const trackWidth = el.clientWidth;
  const travel = Math.max(1, trackWidth - thumbWidth);
  const dx = e.clientX - thumbStartX;
  el.scrollLeft = thumbStartScroll + (dx / travel) * maxScroll;
};

const onThumbPointerUp = (e: PointerEvent) => {
  if (!thumbDragging.value) return;
  thumbDragging.value = false;
  (e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId);
};

const onTabClick = (name: string) => {
  currentGroup.value = name;
};

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

watch(currentGroup, (v) => { 
  if (v) localStorage.setItem('goclashz_proxyGroup', v); 
  frozenSortedNodes.value = null;
  nextTick(updateSlider); 
});
watch(() => localGroups.value.length, () => {
  resetSlider();
  nextTick(updateTrackOverflow);
});
let trackResizeObserver: ResizeObserver | null = null;

onMounted(() => {
  resetSlider();
  nextTick(updateTrackOverflow);
  // 窗口/轨道尺寸变化或分组数量变化时更新左右渐隐提示
  if (typeof ResizeObserver !== 'undefined' && tabsTrackRef.value) {
    trackResizeObserver = new ResizeObserver(() => updateTrackOverflow());
    trackResizeObserver.observe(tabsTrackRef.value);
  }
});
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

const frozenSortedNodes = ref<any[] | null>(null);

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
  frozenSortedNodes.value = null;
};

const locateActiveNode = () => {
  nextTick(() => {
    const activeEl = document.querySelector('.node-item.active');
    if (activeEl) {
      activeEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  });
};

const CARD_MODES: ProxyCardMode[] = ['standard', 'compact', 'list', 'adaptive'];
const cardMode = computed<ProxyCardMode>({
  get: () => globalState.proxyCardMode || 'standard',
  set: (val: ProxyCardMode) => setProxyCardMode(val),
});



const cycleCardMode = () => {
  // 1. 视口锚定预备：记录当前被选中节点或视口内核心元素的垂直相对位置
  const scrollContainer = document.querySelector('.scroll-content') as HTMLElement | null;
  const activeEl = document.querySelector('.node-item.active') as HTMLElement | null;
  
  let anchorOffsetFromTop = 0;
  let hasAnchor = false;

  if (scrollContainer && activeEl) {
    const containerRect = scrollContainer.getBoundingClientRect();
    const activeRect = activeEl.getBoundingClientRect();
    
    // 如果选中的节点当前在视口中（或接近视口范围内）
    const isVisibleInViewport = (
      activeRect.bottom >= containerRect.top &&
      activeRect.top <= containerRect.bottom
    );

    if (isVisibleInViewport) {
      anchorOffsetFromTop = activeRect.top - containerRect.top;
      hasAnchor = true;
    }
  }

  // 2. 切换模式
  const currentIndex = CARD_MODES.indexOf(cardMode.value);
  const nextIndex = (currentIndex + 1) % CARD_MODES.length;
  cardMode.value = CARD_MODES[nextIndex];

  // 4. 模式切换渲染后，精准维持被选中节点的视口相对位置，杜绝任何顺序跳变与视线丢失
  if (hasAnchor) {
    nextTick(() => {
      requestAnimationFrame(() => {
        if (!activeEl || !scrollContainer) return;
        const containerRect = scrollContainer.getBoundingClientRect();
        const newActiveRect = activeEl.getBoundingClientRect();
        const currentOffset = newActiveRect.top - containerRect.top;
        const diff = currentOffset - anchorOffsetFromTop;
        if (Math.abs(diff) > 1) {
          scrollContainer.scrollTop += diff;
        }
      });
    });
  }
};

const currentCardModeIcon = computed(() => {
  switch (cardMode.value) {
    case 'compact':
      return ICONS.layoutCompact;
    case 'list':
      return ICONS.layoutList;
    case 'adaptive':
      return ICONS.layoutAdaptive;
    case 'standard':
    default:
      return ICONS.layoutStandard;
  }
});

const currentCardModeTitle = computed(() => {
  switch (cardMode.value) {
    case 'compact':
      return t('proxies.layoutCompact');
    case 'list':
      return t('proxies.layoutList');
    case 'adaptive':
      return t('proxies.layoutAdaptive');
    case 'standard':
    default:
      return t('proxies.layoutStandard');
  }
});

let measureCanvas: HTMLCanvasElement | null = null;
function measureNodeTextWidth(text: string, font = '500 15px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'): number {
  if (typeof document === 'undefined') return 80;
  if (!measureCanvas) {
    measureCanvas = document.createElement('canvas');
  }
  const context = measureCanvas.getContext('2d');
  if (!context) return 80;
  context.font = font;
  return context.measureText(text).width;
}

const adaptiveCardWidth = computed(() => {
  if (cardMode.value !== 'adaptive') return 130;
  const nodes = filteredAndSortedNodes.value || [];
  if (nodes.length === 0) return 130;
  let maxW = 0;
  for (const n of nodes) {
    if (n) {
      if (n.name) {
        const nw = measureNodeTextWidth(n.name, '500 15px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif');
        if (nw > maxW) maxW = nw;
      }
      if (n.type) {
        const tw = measureNodeTextWidth(n.type, '800 11px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif');
        if (tw > maxW) maxW = tw;
      }
    }
  }
  // 基础预留空间：卡片左右内边距(28px) + 测速区宽度与外边距(54px) + 呼吸安全间距(18px) = 100px
  // 最小自适应宽度设为 130px（支持短名称节点更紧凑排列，远小于默认标准卡片的 260px）
  return Math.max(130, Math.ceil(maxW + 100));
});

const gridStyle = computed(() => {
  if (cardMode.value === 'adaptive') {
    return {
      gridTemplateColumns: `repeat(auto-fill, minmax(${adaptiveCardWidth.value}px, 1fr))`
    };
  }
  return {};
});

const filteredAndSortedNodes = computed(() => {
  if (!activeGroupData.value || !activeGroupData.value.proxies) return [];

  // 测速进行时冻结排序快照，避免频繁重排致使卡片剧烈抖动与错位
  if (isTesting.value && frozenSortedNodes.value) {
    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase();
      return frozenSortedNodes.value.filter(n => n.name.toLowerCase().includes(q));
    }
    return frozenSortedNodes.value;
  }

  let nodes = [...activeGroupData.value.proxies];
  
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    nodes = nodes.filter(n => n.name.toLowerCase().includes(q));
  }

  if (sortState.value === 1) {
    const measured: any[] = [];
    const unmeasured: any[] = [];

    for (const node of nodes) {
      const delay = globalState.proxyDelays[node.name]?.delay;
      if (delay !== undefined && delay !== null && delay > 0) {
        measured.push(node);
      } else {
        unmeasured.push(node);
      }
    }

    // 测出速度的节点按延迟升序排序（同延迟自动维持原相对顺序）
    measured.sort((a, b) => {
      const delayA = globalState.proxyDelays[a.name]?.delay ?? 0;
      const delayB = globalState.proxyDelays[b.name]?.delay ?? 0;
      return delayA - delayB;
    });

    // 测出速度的放在前面，未测速/超时的按原相对顺序继续排在后面
    return [...measured, ...unmeasured];
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
    const bh = await App.GetAppBehavior();
    if (bh) {
      isColorMode.value = bh.colorDelay === true;
    }
  } catch (e) {
    // 取不到行为配置不影响节点列表，但必须留痕（F-A4-11）
    reportError(e, 'proxies.loadBehavior');
  }

  try {
    const data: any = await App.GetInitialData();
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
    reportError(e, 'proxies.load');
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
    await App.SelectProxy(groupName, nodeName);
    
    // 🚀 核心修复：以内核返回的真实 now 为准，不再本地乐观修改
    // 重新拉取一次数据确保同步
    await loadData();
    
    // 触发刷新出站 IP
    scheduleOutboundIPRefresh('node-switch', {
      force: true,
      delay: 1200,
      clearBeforeStart: true,
      reason: 'node-switch',
    });
  } catch (e) {
    await showAlert(t('proxies.switchError', { error: String(e) }), t('common.error'));
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

// 测速当前选中的组 / 中止测速
const toggleTestAllDelays = async () => {
  if (isTesting.value) {
    await stopTesting();
    return;
  }
  await testAllDelays();
};

const stopTesting = async () => {
  try {
    await App.StopDelayTest();
  } catch (e) {
    reportError(e, 'proxies.stopTest');
  } finally {
    isTesting.value = false;
    frozenSortedNodes.value = null;
    if (localGroups.value) {
      localGroups.value.forEach(g => {
        if (!g.proxies) return;
        g.proxies.forEach((n: any) => {
          n.testing = false;
        });
      });
    }
  }
};

const testAllDelays = async () => {
  if (!activeGroupData.value || isTesting.value) return;

  isTesting.value = true;
  const proxies = activeGroupData.value.proxies || [];
  // 冻结当前节点顺序，防止测速期间卡片位置频繁跳动造成视觉混乱与点击错位
  frozenSortedNodes.value = [...filteredAndSortedNodes.value];

  const nodesArray = proxies.map((n: any) => n.name);

  if (nodesArray.length > 0) {
    try {
      await App.TestAllProxies(nodesArray);
    } catch (e) {
      // F-A4-10：以前这里完全静默，用户点「测速全部」失败后毫无反馈
      reportError(e, 'proxies.testAll');
      isTesting.value = false;
      frozenSortedNodes.value = null;
      proxies.forEach((n: any) => {
        n.testing = false;
      });
      showToast(t('proxies.speedTestFailed', { error: toUserError(e).message }), 'error');
    }
  } else {
    isTesting.value = false;
    frozenSortedNodes.value = null;
  }
};

// 单点测速
const testSingleDelay = async (node: any) => {
  if (node.testing) return;
  
  node.testing = true;

  try {
    await App.TestProxy(node.name);
  } catch (e) {
    // 只认后端确定会产出的 code（core/appcore/delay_manager.go: ErrDelayTestBusy）；
    // 以前还匹配 'busy' 这类泛化子串，会把「内核忙」之类的真实错误一起吞掉。
    if (String(e).includes('DELAY_TEST_BUSY')) {
      showToast(t('proxies.speedTestBusy'), 'info');
      return;
    }
    reportError(e, 'proxies.testSingle');
    showToast(t('proxies.speedTestFailed', { error: toUserError(e).message }), 'error');
  } finally {
    node.testing = false;
  }
};

const formatDelay = (delayInfo: any) => {
  if (!delayInfo) return '--';
  const { delay, status } = delayInfo;
  
  // 🚀 核心优化：只要存在大于 0 的历史有效延迟，就优先展示数字，避免测速波动导致的结果瞬间消失
  if (delay > 0) return `${delay}ms`;
  
  if (status === 'timeout') return t('proxies.timeout');
  if (status === 'connect-error') return t('proxies.testError');
  return t('proxies.testError');
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
  unsubStart = Events.On("proxy-test-start", ({ data: nodeName }: { data: string }) => {
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
  unsubUpdate = Events.On("proxy-delay-update", ({ data }: { data: any }) => {
    if (!data || !localGroups.value) return;
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) {
        node.testing = false;
      }
    });
  });

  unsubChanged = Events.On("config-changed", async () => {
      await loadData();
  });

  unsubFinish = Events.On("proxy-test-finished", () => {
    isTesting.value = false;
    frozenSortedNodes.value = null;
    
    // 🚀 核心修复：清理所有组内节点的转圈状态，防止长尾节点卡死
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      g.proxies.forEach((n: any) => {
        n.testing = false;
      });
    });
  });

  // 🚀 新增：轻量化策略组状态同步，处理 Fallback/URLTest 的自动切换
  unsubProxyState = Events.On("proxy-state-sync", async ({ data: states }: { data: any[] }) => {
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
  trackResizeObserver?.disconnect();
  trackResizeObserver = null;
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
}

.proxy-toolbar {
  width: 100%;
}

/* 分组名 + 测速按钮共用一条"滑轨"（与连接页的选项卡轨道同一套观感）：
   圆角底板 + 内缩 6px，选项卡的高亮胶囊就落在轨道里，不再悬空。 */
/* 滑轨比按钮高"一圈"：上 6px + 按钮 36px + 滚动条 4px + 下 2px = 48px。
   这样既容得下原生横向滚动条，组名与测速按钮的垂直中轴又都落在 24px 处（视觉居中）。 */
/* 滑轨比按钮行高一圈：上下各 10px 留白里容纳"按钮行 + 下方 3px 滚动指示条"，
   按钮中心 = 滑轨中心（10 + 18 = 28），指示条画在 bottom -7px 处，不覆盖任何标签。 */
.proxy-toolbar-card {
  display: flex;
  align-items: flex-start;
  width: 100%;
  padding: 10px 6px;
  gap: 10px;
  border-radius: 12px;
  background: var(--surface-hover);
}

html[data-glass="on"] .proxy-toolbar-card {
  background: var(--glass-fill-card);
}

.proxy-tabs-viewport {
  position: relative;
  flex: 1;
  min-width: 0;
  height: 36px;
  border-radius: 8px;
}

/* 原生横向滚动条已经回到轨道内，这里不再需要边缘渐隐遮罩 */

.proxy-tabs-track {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  min-width: 0;
  width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  padding: 0;
  /* 轨道 = 36px 按钮 + 4px 滚动条；滚动条占的是轨道自己的高度，
     所以按钮仍然贴着轨道顶部，与右侧测速按钮同轴。 */
  scrollbar-width: none;
  scroll-behavior: auto;
  user-select: none;
  -webkit-user-select: none;
  overscroll-behavior-x: contain;
  position: relative;
}

.proxy-tabs-track::-webkit-scrollbar {
  height: 0;
  display: none;
}

/* 自绘滚动指示条：位于按钮行下方 6px，3px 高，不覆盖标签 */
.proxy-rail-thumb {
  position: absolute;
  bottom: -7px;
  height: 3px;
  border-radius: 2px;
  background: var(--text-muted);
  opacity: 0.75;
  /* 只有指示条本身可以拖（和原生滚动条一致），选项卡区域保持点击切换。
     光标刻意用 default：系统滚动条移上去不会变手，自绘条跟它保持一致。 */
  pointer-events: auto;
  cursor: default;
  transition: left 0.08s linear, width 0.08s linear, opacity 0.2s ease, background-color 0.15s ease;
}

/* 视觉上仍是 3px，但上下各扩 4px 命中区，方便按住 */
.proxy-rail-thumb::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: -4px;
  bottom: -4px;
}

.proxy-rail-thumb:hover {
  background: var(--text-sub);
  opacity: 1;
}

.proxy-rail-thumb.is-dragging {
  background: var(--ui-accent-block);
  opacity: 1;
}

html[data-glass="on"] .proxy-rail-thumb {
  background: var(--text-sub);
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
  color: var(--ui-accent-fg);
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
  background: var(--ui-accent-block);
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
  align-items: center;
  height: 36px;
}

.proxy-test-btn {
  height: 36px;
  min-width: 132px;
  padding: 0 18px;
  border-radius: 8px;
  white-space: nowrap;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
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

.sub-toolbar-layout {
  display: flex;
  align-items: center;
}

.layout-toggle-btn {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.layout-toggle-btn:active {
  transform: scale(0.9);
}

.layout-toggle-btn .btn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  animation: iconRotateIn 0.38s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes iconRotateIn {
  0% {
    opacity: 0.3;
    transform: scale(0.7) rotate(-25deg);
  }
  100% {
    opacity: 1;
    transform: scale(1) rotate(0deg);
  }
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
  background: var(--ui-surface-hover);
  color: var(--ui-text-primary);
}

.tool-btn.active {
  color: var(--ui-accent);
  background: var(--ui-accent-soft);
}

.tool-btn .btn-icon {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 唯一滚动区：节点网格自己滚动，工具栏固定 */
.scroll-content { 
  flex: 1; 
  min-height: 0;
  padding-top: 4px;
  display: flex;
  flex-direction: column;
}

.group-section { margin-bottom: 24px; }
.node-grid { 
  display: grid; 
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); 
  gap: 16px; 
  position: relative;
}

/* TransitionGroup FLIP 排序与模式切换平滑优雅动画 */
.node-card-move,
.node-grid .node-item.node-card-move {
  transition: 
    transform 0.46s cubic-bezier(0.25, 1, 0.5, 1),
    height 0.42s cubic-bezier(0.25, 1, 0.5, 1),
    padding 0.42s cubic-bezier(0.25, 1, 0.5, 1),
    border-radius 0.42s cubic-bezier(0.25, 1, 0.5, 1) !important;
  z-index: 2;
}

/* 1. 标准卡片模式 (默认) */
.node-grid.size-standard {
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
}

/* 2. 紧凑小卡模式 */
.node-grid.size-compact {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.node-grid.size-compact .node-item {
  height: 60px;
  padding: 10px 14px;
  border-radius: 10px;
}
.node-grid.size-compact .n-name {
  font-size: 0.85rem;
}
.node-grid.size-compact .n-protocol {
  font-size: 0.6rem;
  padding: 0 4px;
}
.node-grid.size-compact .n-latency-box {
  padding: 4px 6px;
  min-width: 44px;
  margin-left: 8px;
}
.node-grid.size-compact .n-delay {
  font-size: 0.78rem;
}

/* 3. 纵向列表模式 */
.node-grid.size-list {
  grid-template-columns: 1fr;
  gap: 8px;
}
.node-grid.size-list .node-item {
  height: 48px;
  padding: 8px 18px;
  border-radius: 10px;
  align-items: center;
}
.node-grid.size-list .node-main-area {
  flex-direction: row;
  align-items: center;
  justify-content: flex-start;
  gap: 16px;
}
.node-grid.size-list .node-info {
  flex: 1;
}
.node-grid.size-list .node-meta {
  flex-shrink: 0;
}
.node-grid.size-list .n-name {
  font-size: 0.9rem;
}
.node-grid.size-list .n-protocol {
  font-size: 0.65rem;
  padding: 1px 6px;
}
.node-grid.size-list .n-latency-box {
  padding: 4px 8px;
  min-width: 50px;
}

/* 4. 自适应宽度模式 */
.node-grid.size-adaptive {
  gap: 12px;
}
.node-grid.size-adaptive .node-item {
  height: 76px;
  padding: 12px 14px;
  border-radius: 11px;
}
.node-grid.size-adaptive .n-name {
  font-size: 0.9rem;
  white-space: nowrap;
}
.node-grid.size-adaptive .n-protocol {
  font-size: 0.6rem;
  padding: 1px 5px;
}
.node-grid.size-adaptive .n-latency-box {
  min-width: 46px;
  padding: 4px 6px;
  margin-left: 8px;
}
.node-grid.size-adaptive .n-delay {
  font-size: 0.76rem;
}

.node-item {
  display: flex;
  justify-content: space-between;
  align-items: stretch;
  padding: 16px 20px;
  height: 84px;
  min-width: 0;
  box-sizing: border-box;
  border: none;
  cursor: pointer;
  position: relative;
  overflow: hidden;
  transition: 
    transform 0.46s cubic-bezier(0.25, 1, 0.5, 1),
    height 0.42s cubic-bezier(0.25, 1, 0.5, 1),
    padding 0.42s cubic-bezier(0.25, 1, 0.5, 1),
    border-radius 0.42s cubic-bezier(0.25, 1, 0.5, 1),
    background 0.2s ease,
    color 0.2s ease,
    box-shadow 0.2s ease;
  will-change: transform, height;
}
.node-item.readonly {
  cursor: default;
}
.node-item.active { 
  background: var(--ui-accent-block); 
  color: var(--ui-accent-fg);
  font-weight: 600; 
  position: relative;
  z-index: 2;
  box-shadow: var(--ui-shadow-2);
}

.node-item.active .n-name { 
  color: var(--ui-accent-fg); 
}

/* 🎯 修复：保持字重一致，仅通过不透明度拉开深浅，避免黑色模式下字体显得过细 */
.node-item.active .t-fast { color: var(--ui-accent-fg) !important; opacity: 1; font-weight: 700; }
.node-item.active .t-mid { color: var(--ui-accent-fg) !important; opacity: 0.6; font-weight: 700; }
.node-item.active .t-slow { color: var(--ui-accent-fg) !important; opacity: 0.3; font-weight: 700; }
.node-item.active .t-fail { color: var(--ui-accent-fg) !important; opacity: 1; font-weight: 700; }
.node-item.active .t-unknown { color: var(--ui-accent-fg) !important; opacity: 0.3; font-weight: 700; }

/* 确保选中节点（active）的图标完全不透明且使用反色（白色） */
.node-item.active .ping-idle {
  color: var(--ui-accent-fg) !important; /* 在日间模式选中时为白色 */
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
  stroke: var(--ui-accent-fg) !important;
  stroke-opacity: 0.15; /* 🚀 核心：让底圈更浅、更通透 */
}
.node-item.active .scanner-bar {
  stroke: var(--ui-accent-fg) !important;
}

.node-main-area { 
  flex: 1; 
  display: flex; 
  flex-direction: column; 
  justify-content: space-between; 
  min-width: 0; 
  transition: gap 0.42s cubic-bezier(0.25, 1, 0.5, 1);
}

.node-info { flex: 1; min-width: 0; }
.n-name { 
  font-size: 0.95rem; 
  font-weight: 500; 
  display: block; 
  white-space: nowrap; 
  overflow: hidden; 
  text-overflow: ellipsis; 
  color: var(--text-main);
  transition: color 0.2s ease;
}

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
  transition: background-color 0.2s ease, color 0.2s ease;
}

/* 情况 1：未选中的节点卡片 */
/* 照抄顶部“已选中的代理组 Tab”的高亮逻辑 */
.node-item:not(.active) .n-protocol {
  background: var(--surface); 
  color: var(--ui-text-primary);
}

/* 情况 2：已选中的节点卡片 (反色态) */
/* 严格照抄 Overview.vue 中 .on .icon-ring 的夜间/激活模式处理逻辑 */
.node-item.active .n-protocol { 
  /* 选中态标签：accent-fg 半透明遮罩，与 Overview 的 on 态保持一致，不再用色值字面量 */
  background: color-mix(in srgb, var(--ui-accent-fg) 25%, transparent) !important; 
  color: var(--ui-accent-fg) !important;
  /* 确保完全没有边框或阴影干扰，保持通透感 */
  box-shadow: none; 
  border: none;
}

.n-latency-box {
  flex-shrink: 0 !important;
  white-space: nowrap;
  min-width: 54px;
  margin-left: 12px;
  padding: 6px 10px;
  border-radius: 6px;
  background: transparent !important;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: min-width 0.42s cubic-bezier(0.25, 1, 0.5, 1), padding 0.42s cubic-bezier(0.25, 1, 0.5, 1), opacity 0.2s;
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
  stroke: var(--ui-surface-hover);
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
  flex: 1 1 auto;
  min-height: 240px;
  color: var(--text-muted);
  font-style: italic;
  text-align: center;
}
.empty-state p { margin: 0; }
</style>