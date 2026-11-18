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
            @lostpointercapture="onThumbPointerUp"
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
        <!-- P-16：区分「真的没有分组」与「读取失败」，后者不能再显示成"请先添加配置" -->
        <p>{{ loadError ? t('proxies.loadFailedHint') : t('proxies.noGroupsHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick, TransitionGroup } from 'vue';
import { App } from '../api';
import { Events } from '@wailsio/runtime';
import { showAlert, globalState, scheduleOutboundIPRefresh, setProxyCardMode, ProxyCardMode } from '../store';
import { ICONS } from '../utils/icons';
import { toUserError } from '../utils/errors';
import { safeGetString, safeSetString } from '../utils/storage';
import { reportError } from '../utils/report';
import { t } from '../locales';

/* P-34：localStorage 在 WebView2 隐私模式/配额异常时会直接抛错，而这些读写都在 setup 顶层与 watch 里，
   一抛就是整页白屏。统一走薄包装：读失败退回默认值，写失败静默。
   R-1 等价重构：原局部 readStorage/writeStorage 抽到 utils/storage（逐行同义），本文件不再各留一份。 */

const localGroups = ref<any[]>([]);
const currentGroup = ref<string>(safeGetString('goclashz_proxyGroup') || '');
const isTesting = ref(false);
// P-16：后端把"读取失败"折成空 groups，前端需要单独的失败态才能区分「真空」与「读失败」
const loadError = ref(false);

const isColorMode = ref(false);

const tabsTrackRef = ref<HTMLElement | null>(null);
const activeTabEl = ref<HTMLElement | null>(null);
const sliderStyle = ref({ left: '0px', width: '0px' });
const sliderReady = ref(false);
const sliderVisible = ref(false);

/* ------------------------------------------------------------------
 * 分组条横向溢出处理
 * 轨道不再显示原生滚动条（它会把轨道撑高 8px，导致组名与测速按钮不居中），
 * 所以这里补两件事：滚轮横向滚动、按住拖动下方自绘指示条滚动。
 * ---------------------------------------------------------------- */
/* P-11：这里原有 canScrollLeft / canScrollRight 两个"左右边缘渐隐"状态，只写不读（渐隐遮罩早已删掉），已移除。 */

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
  if (!tabsTrackRef.value) return;
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
  const thumb = e.currentTarget as HTMLElement;
  // P-12：lostpointercapture 走到这里时捕获已经丢了，再 release 会抛 NotFoundError；
  // 元素被隐藏等原因提前丢捕获时，也必须在这条路径上复位 dragging，否则 .is-dragging 常驻。
  if (thumb.hasPointerCapture?.(e.pointerId)) thumb.releasePointerCapture?.(e.pointerId);
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

let sliderRaf = 0;   // P-13：rAF 原先没有取消逻辑，卸载后回调仍会写 sliderVisible/sliderReady
const resetSlider = () => {
  if (localGroups.value.length === 0) return;
  sliderReady.value = false;
  sliderVisible.value = false;
  nextTick(() => {
    updateSlider();
    // 强制触发一次重排，让浏览器“记住”此时无动画的起始位置
    if (tabsTrackRef.value) void tabsTrackRef.value.offsetHeight;
    
    cancelAnimationFrame(sliderRaf);
    sliderRaf = requestAnimationFrame(() => {
      sliderVisible.value = true;
      sliderReady.value = true;
    });
  });
};

watch(currentGroup, (v) => { 
  if (v) safeSetString('goclashz_proxyGroup', v); 
  frozenSortedNodes.value = null;
  nextTick(updateSlider); 
});
watch(() => localGroups.value.length, () => {
  resetSlider();
  nextTick(updateTrackOverflow);
});
let trackResizeObserver: ResizeObserver | null = null;

/* R-1：审计建议把本钩子与下方 async onMounted 合并，核对后**不合并**：
   本钩子是纯同步的布局测量（resetSlider / updateTrackOverflow / ResizeObserver），
   下方钩子要 await loadBehavior/loadData 并注册事件。合并后这两个钩子只能取 async 形式，
   而同步段一旦抛错会变成该 async 函数的 Promise 拒绝（Vue 的 mounted 钩子由
   flushPostFlushCbs 直接调用，不会把返回值拒绝交给全局 errorHandler），错误上报路径
   与现状（同步错误交给 Vue 错误处理器）不同。收益只是少一个钩子，按「行为不变」保留两处。 */
onMounted(() => {
  resetSlider();
  nextTick(updateTrackOverflow);
  // 窗口/轨道尺寸变化或分组数量变化时更新自绘滚动指示条
  if (typeof ResizeObserver !== 'undefined' && tabsTrackRef.value) {
    trackResizeObserver = new ResizeObserver(() => updateTrackOverflow());
    trackResizeObserver.observe(tabsTrackRef.value);
  }
});

let unsubStart: (() => void) | null = null;
let unsubUpdate: (() => void) | null = null;
// P-08：config-changed 监听已删除（不可达），保留注释说明来源。
let unsubFinish: (() => void) | null = null;
let unsubProxyState: (() => void) | null = null;

const frozenSortedNodes = ref<any[] | null>(null);

const storedSort = parseInt(safeGetString('goclashz_proxySortState') || '0');
const sortState = ref(isNaN(storedSort) ? 0 : storedSort);

watch(sortState, (newVal) => {
  safeSetString('goclashz_proxySortState', newVal.toString());
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
/* P-14：adaptive 宽度依赖 filteredAndSortedNodes，而它依赖 proxyDelays —— 测速期间每个节点一条事件，
   等于每帧都对全部节点重测一次文本。同一「字体+文本」的宽度是常量，直接缓存即可。 */
const measuredWidthCache = new Map<string, number>();
function measureNodeTextWidth(text: string, font = '500 15px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'): number {
  const cacheKey = font + '\u0000' + text;
  const cached = measuredWidthCache.get(cacheKey);
  if (cached !== undefined) return cached;

  let width = 80;
  if (typeof document !== 'undefined') {
    if (!measureCanvas) {
      measureCanvas = document.createElement('canvas');
    }
    const context = measureCanvas.getContext('2d');
    if (context) {
      context.font = font;
      width = context.measureText(text).width;
    }
  }
  measuredWidthCache.set(cacheKey, width);
  return width;
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

/* 排序结果（不含搜索过滤）。
   P-10：冻结快照如果存"已过滤"的数组，测速期间清空搜索就会让不匹配的节点整个消失；
   而过滤对每个元素独立，先排序后过滤与先过滤后排序结果一致，所以把过滤从这层挪到渲染层。 */
const sortedNodes = computed(() => {
  if (!activeGroupData.value || !activeGroupData.value.proxies) return [];

  const nodes = [...activeGroupData.value.proxies];

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

const applyNodeFilter = (nodes: any[]) => {
  if (!searchQuery.value) return nodes;
  const q = searchQuery.value.toLowerCase();
  return nodes.filter(n => n.name.toLowerCase().includes(q));
};

const filteredAndSortedNodes = computed(() => {
  if (!activeGroupData.value || !activeGroupData.value.proxies) return [];
  // 测速进行时冻结排序快照，避免频繁重排致使卡片剧烈抖动与错位
  if (isTesting.value && frozenSortedNodes.value) {
    return applyNodeFilter(frozenSortedNodes.value);
  }
  return applyNodeFilter(sortedNodes.value);
});

// 计算当前要显示的组的数据
const activeGroupData = computed(() => {
  return localGroups.value.find(g => g.name === currentGroup.value);
});

/* P-15：行为配置只决定延迟颜色的阈值，与拓扑无关。原先它挂在 loadData 里，
   于是 6 个 loadData 入口（每次点节点都会走一次）都多一次 RPC；改为挂载时拉一次。 */
const loadBehavior = async () => {
  try {
    const bh = await App.GetAppBehavior();
    if (bh) {
      isColorMode.value = bh.colorDelay === true;
    }
  } catch (e) {
    // 取不到行为配置不影响节点列表，但必须留痕（F-A4-11）
    reportError(e, 'proxies.loadBehavior');
  }
};

/* P1-G（P-03）：测速期间 loadData 会把 localGroups 整个换成新对象，而网格此刻渲染的是
   frozenSortedNodes 里的旧对象 —— 旧对象留在快照里，proxy-test-start / proxy-delay-update
   写在"新对象"上，转圈与进度从此与界面失联（表现为"数字在变、转圈不转"）。
   所以重建数据后立刻把快照换成新对象：顺序沿用旧快照（测速期间不重排），
   testing 状态按名字迁移，配置里新出现的节点追加到末尾，避免测速期间节点凭空增减。 */
const refreshFrozenNodes = (prev: any[] | null) => {
  const group = activeGroupData.value;
  if (!isTesting.value || !prev || !group) {
    frozenSortedNodes.value = null;
    return;
  }
  const fresh = new Map<string, any>();
  (group.proxies || []).forEach((n: any) => fresh.set(n.name, n));

  const next: any[] = [];
  prev.forEach((old: any) => {
    const node = fresh.get(old.name);
    if (!node) return;
    node.testing = old.testing === true;
    next.push(node);
  });
  const kept = new Set(prev.map((o: any) => o.name));
  (group.proxies || []).forEach((n: any) => {
    if (!kept.has(n.name)) next.push(n);
  });
  frozenSortedNodes.value = next;
};

// 把一次 GetInitialData 的结果落成 localGroups
const applyInitialData = (data: any) => {
  if (!data || !data.groups) {
    // 返回结构异常：当前本来就有列表时不要把它打成错误态；后端给了 loadError 就直接采信。
    loadError.value = !!data?.loadError || localGroups.value.length === 0;
    return;
  }

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
          // R-1：原 `now: item.now` 已删 —— 节点对象上的 now 全仓零读取（选中态读的是组的 now，见模板 :class）
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

  const prevFrozen = frozenSortedNodes.value;
  localGroups.value = processedGroups;

  /* P-16：优先用后端给的确定信号 loadError（app.go 的两个失败分支都会带上原因）。
     只有旧后端没有该字段时才退回启发式判定 —— 启发式的问题是会误报：
     纯规则 + DIRECT 的配置本来就一个 Selector 组都没有，会被当成"读取失败"。 */
  loadError.value = data.loadError
    ? true
    : processedGroups.length === 0 &&
      !data.loadError &&
      (!!String(data.activeConfig || '') || (data.isOffline === true && globalState.isRunning === true));

  const isCurrentValid = processedGroups.some(g => g.name === currentGroup.value);
  if (!isCurrentValid && processedGroups.length > 0) {
    currentGroup.value = processedGroups[0].name;
  }

  refreshFrozenNodes(prevFrozen);
};

/* P-09：loadData 有 6 个入口（mount/选节点/isRunning/activeConfigId/config-changed/proxy-state-sync 未知组），
   原先既无互斥也无序号，先发后到的旧响应能把新结果覆盖回去。现在：
   在飞时只登记"结束后补跑一次"（既不发重复请求，也不丢掉这次刷新），请求序号作双保险。 */
let loadSeq = 0;
let loadRunning = false;
let loadAgain = false;

// 加载数据
const loadData = async () => {
  if (loadRunning) {
    loadAgain = true;
    return;
  }
  loadRunning = true;
  try {
    do {
      loadAgain = false;
      const seq = ++loadSeq;
      try {
        const data: any = await App.GetInitialData();
        if (seq !== loadSeq) return;   // 双保险：已有更新的一次请求在外
        applyInitialData(data);
      } catch (e) {
        if (seq === loadSeq) loadError.value = true;
        reportError(e, 'proxies.load');
      }
    } while (loadAgain);
  } finally {
    loadRunning = false;
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

/* P1-E/C-06：测速批次隔离（后端不提供 batch id，只能在前端用"本批是否已经开测"来判定）。
   - testBatchStarted：本批是否已经收到过 proxy-test-start；
   - testBatchNodes：本批真正开测过的节点。
   后端 cancel 时不会把 ok 置 false（delay_manager.go 只在启动失败分支置 false），所以"停止 → 立刻重测"
   时上一批迟到的 finished 与正常完成同形，只能靠"本批还没开始测"这个事实把它作废。 */
let testBatchStarted = false;
const testBatchNodes = new Set<string>();
/* proxy-test-finished 的 payload 有两种形态，这里做双兼容（P-02）：
   - 新：map{"ok": bool, "message": string}（v3_events.go 已按 map[string]any 注册，delay_manager.go 已改结构化）；
   - 旧：string —— 成功恒为"测速完成"，其余都是失败原因（delay_manager.go 原 461/503/510/522）。
   先读结构化字段，读不到再回退到文案比较，后端回滚或中间态都不会把成功当失败。 */
const TEST_FINISH_OK_TEXT = '测速完成';
const readTestFinish = (payload: unknown): { ok: boolean; message: string } => {
  if (payload && typeof payload === 'object') {
    const rec = payload as { ok?: unknown; message?: unknown; reason?: unknown };
    const message = typeof rec.message === 'string' ? rec.message.trim()
      : (typeof rec.reason === 'string' ? rec.reason.trim() : '');
    // ok 缺省时按文案兜底，避免只给 message 的旧 payload 被误判成失败
    const ok = typeof rec.ok === 'boolean' ? rec.ok : (!message || message === TEST_FINISH_OK_TEXT);
    return { ok, message };
  }
  if (typeof payload === 'string') {
    const message = payload.trim();
    return { ok: !message || message === TEST_FINISH_OK_TEXT, message };
  }
  return { ok: true, message: '' };
};

const stopTesting = async () => {
  // 让在飞/迟到的事件立刻失效：这一批已经由用户中止
  testBatchStarted = false;
  testBatchNodes.clear();
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
  testBatchStarted = false;
  testBatchNodes.clear();
  const proxies = activeGroupData.value.proxies || [];
  // 冻结当前节点顺序，防止测速期间卡片位置频繁跳动造成视觉混乱与点击错位。
  // P-10：冻结的是"排好序但未过滤"的列表，搜索过滤放到渲染时做，测速中清空搜索才不会丢节点。
  frozenSortedNodes.value = [...sortedNodes.value];

  const nodesArray = proxies.map((n: any) => n.name);

  if (nodesArray.length > 0) {
    try {
      await App.TestAllProxies(nodesArray);
    } catch (e) {
      // 这条 catch 只兜传输层故障：App.TestAllProxies 是 go 异步启动（app.go:1289），
      // 真正的"启动失败/无有效节点"走 proxy-test-finished 的 payload（P-02，见下方监听器）
      reportError(e, 'proxies.testAll');
      isTesting.value = false;
      frozenSortedNodes.value = null;
      proxies.forEach((n: any) => {
        n.testing = false;
      });
      /* 提示策略（用户口径：无感自愈优先，只报无法解决/严重问题）：
         测速失败属"可重试、无数据损失"的一类 —— 只记录日志，不弹 toast。
         节点卡片自己的延迟/超时/失败字样就是结果反馈，再弹一条等于用系统噪音告诉用户"没测出来"。 */
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
    // 忙 = 上一次还没跑完（用户连点），静默即可；其它失败同样只记日志，节点格会显示超时/失败。
    reportError(e, 'proxies.testSingle');
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
    // P1-E：不在本轮测速里的 start 是上一批的迟到事件，直接丢弃（自动测速是 SilentUI，不会发这个事件）
    if (!isTesting.value || typeof nodeName !== 'string') return;
    // 本批确实开测了：此后到达的"测速完成"才属于本批（见 finished 监听器里的作废判据）
    testBatchStarted = true;
    testBatchNodes.add(nodeName);
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
    // P1-E：只认本轮真正开测过的节点 —— 上一批迟到的结果不能把本轮同一节点的转圈关掉
    if (isTesting.value && !testBatchNodes.has(data.name)) return;
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) {
        node.testing = false;
      }
    });
  });

  /* P-08：这里原来监听 config-changed → loadData()。全仓唯一的 emit 点是
     core/appcore/config_editor_controller.go:35（保存"当前激活配置"的文本），
     而保存入口在 YamlEditor 页、与 Proxies 互斥挂载（v-if 整块销毁），所以这条监听永远不可达。
     删除以免让人误以为"配置变更会自动刷新代理列表"；代理拓扑的真实变更走 proxy-state-sync。 */

  unsubFinish = Events.On("proxy-test-finished", ({ data }: { data?: unknown }) => {
    const { ok, message } = readTestFinish(data);

    /* P1-E（C-06）：上一批被取消后仍会发一条 finished；若它落在新一轮测速开始之后，
       旧实现会直接把新一轮的 isTesting 清成 false（按钮弹回"测速本组"、转圈消失）。
       本批还没收到过 proxy-test-start 时收到的"成功"通知必然是上一批的，整条丢弃。
       注意真正的"启动失败"（忙/内核/无有效节点）ok=false，下面照常处理。 */
    if (isTesting.value && !testBatchStarted && ok) return;

    isTesting.value = false;
    frozenSortedNodes.value = null;
    testBatchStarted = false;
    testBatchNodes.clear();
    
    // 🚀 核心修复：清理所有组内节点的转圈状态，防止长尾节点卡死
    localGroups.value.forEach(g => {
      if (!g.proxies) return;
      g.proxies.forEach((n: any) => {
        n.testing = false;
      });
    });

    /* P-02 的结论保留（payload 是后端唯一失败通道，必须解析），但**展示层改为静默**：
       批量测速的失败在界面上由三点体现——按钮回到"测速本组"、转圈全部停止、节点延迟仍是旧值/失败字样。
       这已足够"无感自愈"；再补一条 toast 只是噪音（用户原话：像测速这样的不需要 toast）。
       失败原因只进日志（reportError），便于排查。 */
    if (ok) return;
    reportError(new Error('proxy-test-finished: ' + (message || 'failed')), 'proxies.testFinished');
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

  await loadBehavior();
  await loadData();
});

onUnmounted(() => {
  trackResizeObserver?.disconnect();
  trackResizeObserver = null;
  // P-13：resetSlider 的 rAF 在卸载后仍会回调并写状态，必须取消
  cancelAnimationFrame(sliderRaf);
  sliderRaf = 0;
  unsubStart?.();
  unsubUpdate?.();

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
  border-radius: var(--ui-radius-lg);
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
  border-radius: var(--ui-radius-sm);
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
  /* P-12：触屏上自绘滚动条必须自己接管指针，否则浏览器会先把手势当滚动抢走 */
  touch-action: none;
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



/* R-1：原 ::-webkit-scrollbar-button / -track / -thumb / -thumb:hover 四条规则已删 ——
   轨道已是 scrollbar-width:none（Firefox）+ ::-webkit-scrollbar{height:0;display:none}（Chromium），
   滚动条根本不渲染，这四条永不生效（审计 §4.1「死 CSS」；grep 仅命中本块）。 */

.proxy-tab-btn {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  padding: 0 16px;
  border: none;
  border-radius: var(--ui-radius-sm);
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
  border-radius: var(--ui-radius-sm);
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
  border-radius: var(--ui-radius-sm);
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
  border-radius: var(--ui-radius-sm);
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

/* 1. 标准卡片模式 (默认)：与基础 .node-grid 完全同值，原 .size-standard 重复声明已删（R-1） */

/* 2. 紧凑小卡模式 */
.node-grid.size-compact {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.node-grid.size-compact .node-item {
  height: 60px;
  padding: 10px 14px;
  border-radius: var(--ui-radius-md);
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
  border-radius: var(--ui-radius-md);
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