<template>
  <div class="connections-view">
    <Transition name="slide-fade" mode="out-in">
      <div v-if="!selectedConn" class="connections-list-wrapper">
        <Teleport v-if="titleTargetReady" to="#title-extra-target">
          <span v-if="connections.length > 0" class="conn-title-count">{{ t('connections.activeCount', { count: connections.length }) }}</span>
        </Teleport>
        <div class="action-bar card-panel page-sticky-mask">
        <div class="conn-tabs-viewport">
          <div class="conn-tabs-track" ref="tabsTrackRef">
            <button :ref="(el) => { if (filterMode === 'all') connTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: filterMode === 'all' }]" @click="filterMode = 'all'">{{ t('connections.tabAll') }}</button>
            <button :ref="(el) => { if (filterMode === 'proxy') connTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: filterMode === 'proxy' }]" @click="filterMode = 'proxy'">{{ t('connections.tabProxy') }}</button>
            <button :ref="(el) => { if (filterMode === 'direct') connTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: filterMode === 'direct' }]" @click="filterMode = 'direct'">{{ t('connections.tabDirect') }}</button>
            <div class="conn-tab-slider" :class="{ animated: connSliderReady }" v-show="connSliderVisible" :style="connSliderStyle"></div>
          </div>
        </div>
        <div class="global-actions">
          <button class="action-btn" @click="isPaused = !isPaused">
            <span class="btn-icon" v-html="isPaused ? ICONS.play : ICONS.pause"></span>
            {{ isPaused ? t('connections.resumeRefresh') : t('connections.pauseRefresh') }}
          </button>
          <button v-if="connections.length > 0" class="primary-btn accent-btn" @click="closeAll">
            <span class="btn-icon" style="color: #ff5a5a;" v-html="ICONS.xCircle"></span>
            {{ t('connections.closeAll') }}
          </button>
        </div>
      </div>

      <div class="scroll-content">
        <div v-if="isLoading" class="empty-state">
          <p>{{ t('connections.loadingConns') }}</p>
        </div>
        <div class="conn-grid" v-else-if="filteredConnections.length > 0">
          <div v-for="conn in filteredConnections" :key="conn.id" class="conn-card" @click="openDetail(conn)">
            <div class="conn-header">
              <span class="host" :title="conn.metadata.host || conn.metadata.destinationIP">
                {{ conn.metadata.host || conn.metadata.destinationIP }}
              </span>
              <span class="network">{{ conn.metadata.network }}</span>
            </div>
            
            <div class="conn-body">
              <div class="tags">
                <span v-if="getProxyName(conn)" :class="['tag', isDirect(conn) ? 'tag-direct' : 'tag-proxy']">
                  {{ getProxyName(conn) }}
                </span>
                <span v-if="conn.rule" class="tag tag-rule">{{ conn.rule }}</span>
              </div>
            </div>
            
            <div class="conn-footer font-mono">
              <div class="time-info">
                <span class="icon-svg" v-html="ICONS.clock"></span>
                <span>{{ conn.durationStr }}</span>
              </div>
              <div class="traffic-info">
                <span class="transfer up">
                  <span class="icon-svg" v-html="ICONS.upload"></span>
                  <span>{{ conn.uploadStr }}</span>
                </span>
                <span class="transfer down">
                  <span class="icon-svg" v-html="ICONS.download"></span>
                  <span>{{ conn.downloadStr }}</span>
                </span>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="empty-state">
          <p v-if="connections.length > 0">{{ t('connections.noCategoryConns') }}</p>
          <p v-else>{{ t('connections.noTraffic') }}</p>
        </div>
      </div>
      </div>

      <div v-else class="detail-page card-panel">
        <div class="detail-header">
          <h3>{{ t('connections.detailTitle') }}</h3>
          <button class="action-btn back-btn" @click="closeDetail">
            {{ t('common.back') }}
          </button>
        </div>

        <div class="detail-body scroll-content">
          <div class="detail-row"><span>ID:</span> <span class="font-mono">{{ selectedConn.id }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailNetwork') }}:</span> <span>{{ selectedConn.metadata.network }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailSource') }}:</span> <span class="font-mono">{{ selectedConn.metadata.sourceIP }}:{{ selectedConn.metadata.sourcePort }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailDest') }}:</span> <span class="font-mono">{{ selectedConn.metadata.destinationIP || selectedConn.metadata.host }}:{{ selectedConn.metadata.destinationPort }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailRule') }}:</span> <span>{{ selectedConn.rule }} {{ selectedConn.rulePayload ? `(${selectedConn.rulePayload})` : '' }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailChain') }}:</span> <span class="path-chain">{{ selectedConn.chains.join(' ➔ ') }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailUpload') }}:</span> <span class="font-mono">{{ selectedConn.uploadStr }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailDownload') }}:</span> <span class="font-mono">{{ selectedConn.downloadStr }}</span></div>
          <div class="detail-row"><span>{{ t('connections.detailTime') }}:</span> <span>{{ new Date(selectedConn.start).toLocaleString() }} ({{ selectedConn.durationStr }})</span></div>
        </div>

        <div class="detail-footer">
          <button class="action-btn red-text-btn" @click="closeSingleConnection(selectedConn.id)">{{ t('connections.closeSingle') }}</button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed, watch, nextTick } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { showConfirm, showAlert } from '../store';
import { ICONS } from '../utils/icons';
import { t } from '../locales';

const connections = ref<any[]>([]);
const isPaused = ref(false);
const selectedConn = ref<any>(null);
const filterMode = ref(localStorage.getItem('goclashz_connFilter') || 'all');

const tabsTrackRef = ref<HTMLElement | null>(null);
const connTabEl = ref<HTMLElement | null>(null);
const connSliderStyle = ref({ left: '0px', width: '0px' });
const connSliderReady = ref(false);
const connSliderVisible = ref(false);

const updateConnSlider = () => {
  const track = tabsTrackRef.value;
  const btn = connTabEl.value;
  if (track && btn) {
    connSliderStyle.value = {
      left: `${btn.offsetLeft}px`,
      width: `${btn.offsetWidth}px`,
    };
  }
};

const resetConnSlider = () => {
  connSliderReady.value = false;
  connSliderVisible.value = false;
  nextTick(() => {
    updateConnSlider();
    nextTick(() => {
      connSliderVisible.value = true;
      connSliderReady.value = true;
    });
  });
};

watch(filterMode, (v) => { localStorage.setItem('goclashz_connFilter', v); nextTick(updateConnSlider); });

const isMonitoring = ref(false); // 增加一个状态锁，防止重复注册监听
const isLoading = ref(true); // 🚀 新增：控制初始加载状态

// 🚀 新增：保存专属退订函数
let unsubMonitor: (() => void) | null = null; 

const startMonitor = async () => {
  if (isMonitoring.value) return;
  isMonitoring.value = true;
  
  // ✅ 修改后：接住返回的专属销毁函数
  unsubMonitor = EventsOn("connections-update", (data: any) => {
    if (isPaused.value) return;

    if (data && Array.isArray(data.connections)) {
      connections.value = data.connections;
      if (selectedConn.value) {
        const updated = data.connections.find((c: any) => c.id === selectedConn.value.id);
        if (updated) selectedConn.value = updated;
        else selectedConn.value = null;
      }
    } else {
      connections.value = [];
    }
  });

  try {
    isLoading.value = true;
    
    // 🚀 核心修复：在建立监听后，立即主动请求一次连接快照，消除初始延迟
    const initData = await API.GetConnections();
    if (initData && Array.isArray(initData.connections)) {
      connections.value = initData.connections;
    }

    await API.StartConnectionMonitor();
  } catch (e) {
    console.error("启动连接监控失败:", e);
  } finally {
    isLoading.value = false;
  }
};

const stopMonitor = () => {
  if (!isMonitoring.value) return;
  isMonitoring.value = false;
  
  // ✅ 修改后：精准狙击，只销毁当前组件刚注册的那个监听器
  if (unsubMonitor) {
    unsubMonitor();
    unsubMonitor = null;
  }
  
  API.StopConnectionMonitor();
};

// 配合 KeepAlive 的生命周期控制
const titleTargetReady = ref(false);

const refreshTitleTarget = async () => {
  await nextTick();
  titleTargetReady.value = !!document.querySelector('#title-extra-target');
};

onMounted(() => {
  startMonitor(); 
  resetConnSlider(); 
  refreshTitleTarget();
});
onActivated(() => { 
  startMonitor(); 
  resetConnSlider(); 
  refreshTitleTarget();
});       // 再次切回连接页时恢复更新
onDeactivated(() => stopMonitor());      // 切到别的页面时暂停后台请求，节省性能
onUnmounted(() => {
  stopMonitor();
  selectedConn.value = null;
});

const isDirect = (conn: any) => {
  const chains = conn.chains || [];
  return chains.includes('DIRECT') || chains.includes('REJECT');
};

const filteredConnections = computed(() => {
  if (filterMode.value === 'all') return connections.value;
  if (filterMode.value === 'proxy') return connections.value.filter(c => !isDirect(c));
  if (filterMode.value === 'direct') return connections.value.filter(c => isDirect(c));
  return connections.value;
});

const getProxyName = (conn: any) => {
  const chains = conn.chains || [];
  if (chains.length > 0) return chains[0];
  return 'Unknown';
};

const openDetail = (conn: any) => {
  selectedConn.value = conn;
};

const closeDetail = () => {
  selectedConn.value = null;
};

const closeAll = async () => {
  const ok = await showConfirm(t('connections.closeAllConfirmMsg'), t('connections.closeAllConfirmTitle'), true);
  if (ok) {
    try {
      await API.CloseAllConnections();
      if (selectedConn.value) closeDetail();
    } catch (e) { 
      await showAlert(t('connections.closeError', { error: String(e) }), t('common.error')); 
    }
  }
};

const closeSingleConnection = async (id: string) => {
  const ok = await showConfirm(t('connections.closeSingleConfirmMsg'), t('connections.closeSingleConfirmTitle'), true);
  if (!ok) return;
  
  try {
    await API.CloseConnection(id);
    closeDetail();
  } catch (e) { 
    await showAlert(t('connections.closeError', { error: String(e) }), t('common.error')); 
  }
};
</script>

<style scoped>
.connections-view { 
  display: flex; 
  flex-direction: column; 
  min-height: 100%; 
  overflow: visible; 
}

.connections-list-wrapper {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  width: 100%;
}

.slide-fade-enter-active,
.slide-fade-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  width: 100%;
}

.slide-fade-enter-from {
  opacity: 0;
  transform: translateX(12px); 
}

.slide-fade-leave-to {
  opacity: 0;
  transform: translateX(-12px); 
}

.action-bar { 
  display: flex; 
  justify-content: space-between; 
  align-items: center; 
  padding: 12px 16px; 
  margin-bottom: 24px; 
  background: var(--surface);
  border-radius: 12px;
}
.action-bar.page-sticky-mask {
  --sticky-mask-bleed: 6px;
}
.global-actions { display: flex; gap: 12px; }

/* 注入到大标题右侧的计数器样式 (被 Teleport 使用) */
.conn-title-count {
  font-size: 0.95rem;
  font-weight: 500;
  color: var(--text-muted);
  font-family: var(--font-mono);
  transform: translateY(-1px);
}

/* 选项卡样式 (移植自 Proxies.vue) */
.conn-tabs-viewport {
  flex: 1;
  min-width: 0;
  border-radius: 8px;
}

.conn-tabs-track {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--surface-hover);
  padding: 6px;
  border-radius: 12px;
  user-select: none;
  -webkit-user-select: none;
  position: relative;
}

.conn-tab-btn {
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

.conn-tab-btn:hover {
  color: var(--text-main);
}

.conn-tab-btn.active {
  color: var(--accent-fg);
  font-weight: 800;
}

.conn-tab-slider {
  position: absolute;
  top: 6px;
  height: 36px;
  background: var(--accent);
  border-radius: 8px;
  z-index: 0;
  pointer-events: none;
}

.conn-tab-slider.animated {
  transition: left 0.3s cubic-bezier(0.4, 0, 0.2, 1), width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.btn-icon { width: 14px; height: 14px; display: inline-flex; align-items: center;}

.scroll-content { 
  flex: 1; 
  min-height: 0;
  overflow: visible; 
}

.conn-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }

.conn-card { background: var(--surface); border: none; border-radius: 12px; padding: 16px; cursor: pointer; transition: all 0.2s ease; display: flex; flex-direction: column; gap: 12px; }
.conn-card:hover { background: var(--surface-hover); }

.conn-header { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.host { font-weight: 600; font-size: 0.95rem; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
.network { font-size: 0.7rem; font-family: var(--font-mono); background: var(--surface-hover); padding: 2px 6px; border-radius: 4px; color: var(--text-sub); text-transform: uppercase; }

.conn-body { display: flex; flex-direction: column; gap: 8px; flex: 1;}
.tags { display: flex; flex-wrap: wrap; gap: 6px; }
.tag { font-size: 0.75rem; padding: 4px 8px; border-radius: 6px; font-weight: 500; border: none; }
.tag-direct { background: var(--surface-hover); color: var(--text-main); }
.tag-proxy { background: var(--surface-hover); color: var(--text-sub); }
.dark .tag-proxy { color: var(--text-sub); }
.tag-rule { background: var(--surface-hover); color: var(--text-muted); }

/* --- 全新底部排版 --- */
.conn-footer { 
  display: flex; 
  justify-content: space-between; 
  align-items: center; 
  font-size: 0.85rem; 
  color: var(--text-sub); 
  margin-top: 10px; 
}

.time-info { 
  display: flex; 
  align-items: center; 
  gap: 6px;
  font-weight: 500;
}

.traffic-info { 
  display: flex; 
  gap: 8px; 
}

.transfer { 
  display: flex; 
  align-items: center; 
  gap: 5px;
  background: var(--surface-hover); 
  padding: 4px 10px; 
  border-radius: 6px; 
  font-weight: 600;
  font-size: 0.8rem;
}

/* 统一字色，保持纯净感 */
.up, .down { color: var(--text-main); }

.icon-svg { 
  display: flex; 
  align-items: center; 
  justify-content: center; 
  opacity: 0.8;
}
.icon-svg :deep(svg) { width: 12px; height: 12px; }

.empty-state { height: 200px; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-style: italic; }

/* --- 详情子页样式 --- */
.detail-page { 
  display: flex; 
  flex-direction: column; 
  min-height: 100%; 
  border: none; 
  padding: 24px; 
  box-sizing: border-box; 
  overflow: visible;
}

/* 详情头部排版：左标题右按钮 */
.detail-header { 
  display: flex; 
  justify-content: space-between; 
  align-items: center; 
  padding-bottom: 16px; 
  margin-bottom: 24px; 
  border-bottom: 1px solid var(--surface-hover);
}

.detail-header h3 { margin: 0; font-size: 1.2rem; color: var(--text-main); font-weight: 600; }

.back-btn { 
  width: auto; 
  min-width: 60px;
  justify-content: center; 
  padding: 6px 16px;
}

.detail-body { display: flex; flex-direction: column; gap: 16px; flex: 1; padding-right: 12px; }
.detail-row { display: flex; align-items: flex-start; font-size: 0.9rem; padding-bottom: 12px; }
.detail-row span:first-child { color: var(--text-muted); width: 100px; flex-shrink: 0; font-weight: 500; }
.detail-row span:last-child { color: var(--text-main); word-break: break-all; flex: 1; }
.path-chain { color: var(--text-main) !important; font-weight: 500; }

.detail-footer { display: flex; justify-content: flex-end; margin-top: 24px; padding-top: 16px; }
</style>
