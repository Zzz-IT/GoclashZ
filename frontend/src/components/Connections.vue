<template>
  <div class="connections-view">
    <template v-if="!selectedConn">
      <div class="action-bar glass-panel">
        <div class="stats">
          <span class="count">活跃连接: {{ connections.length }}</span>
        </div>
        <div class="global-actions">
          <button class="action-btn" @click="isPaused = !isPaused">
            <span class="btn-icon" v-html="isPaused ? ICONS.play : ICONS.pause"></span>
            {{ isPaused ? '继续刷新' : '暂停刷新' }}
          </button>
          <button class="primary-btn accent-btn red-text-btn" @click="closeAll">
            <span class="btn-icon" v-html="ICONS.xCircle"></span>
            断开全部连接
          </button>
        </div>
      </div>

      <div class="scroll-content">
        <div class="conn-grid" v-if="connections.length > 0">
          <div v-for="conn in connections" :key="conn.id" class="conn-card" @click="openDetail(conn)">
            <div class="conn-header">
              <span class="host" :title="conn.metadata.host || conn.metadata.destinationIP">
                {{ conn.metadata.host || conn.metadata.destinationIP }}
              </span>
              <span class="network">{{ conn.metadata.network }}</span>
            </div>
            <div class="conn-body">
              <div class="tags">
                <span :class="['tag', isDirect(conn) ? 'tag-direct' : 'tag-proxy']">
                  {{ getProxyName(conn) }}
                </span>
                <span class="tag tag-rule">{{ conn.rule }}</span>
              </div>
            </div>
            <div class="conn-footer font-mono">
              <span class="transfer">↑ {{ formatBytes(conn.upload) }}</span>
              <span class="transfer">↓ {{ formatBytes(conn.download) }}</span>
            </div>
          </div>
        </div>
        <div v-else class="empty-state">
          <p>当前没有流量经过</p>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="detail-page glass-panel">
        <div class="detail-header">
          <button class="action-btn back-btn" @click="closeDetail">
            <span class="btn-icon" v-html="ICONS.back"></span> 返回列表
          </button>
          <h3>连接详情</h3>
          <div class="header-placeholder"></div>
        </div>

        <div class="detail-body scroll-content">
          <div class="detail-row"><span>ID:</span> <span class="font-mono">{{ selectedConn.id }}</span></div>
          <div class="detail-row"><span>网络:</span> <span>{{ selectedConn.metadata.network }}</span></div>
          <div class="detail-row"><span>源地址:</span> <span class="font-mono">{{ selectedConn.metadata.sourceIP }}:{{ selectedConn.metadata.sourcePort }}</span></div>
          <div class="detail-row"><span>目标地址:</span> <span class="font-mono">{{ selectedConn.metadata.destinationIP || selectedConn.metadata.host }}:{{ selectedConn.metadata.destinationPort }}</span></div>
          <div class="detail-row"><span>匹配规则:</span> <span>{{ selectedConn.rule }} {{ selectedConn.rulePayload ? `(${selectedConn.rulePayload})` : '' }}</span></div>
          <div class="detail-row"><span>代理链路:</span> <span class="path-chain">{{ selectedConn.chains.join(' ➔ ') }}</span></div>
          <div class="detail-row"><span>上传流量:</span> <span class="font-mono">{{ formatBytes(selectedConn.upload) }}</span></div>
          <div class="detail-row"><span>下载流量:</span> <span class="font-mono">{{ formatBytes(selectedConn.download) }}</span></div>
          <div class="detail-row"><span>连接时间:</span> <span>{{ new Date(selectedConn.start).toLocaleString() }}</span></div>
        </div>

        <div class="detail-footer">
          <button class="action-btn red-text-btn" @click="closeSingleConnection(selectedConn.id)">强行断开此连接</button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

const ICONS = {
  pause: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="4" width="4" height="16"></rect><rect x="14" y="4" width="4" height="16"></rect></svg>`,
  play: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>`,
  xCircle: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`,
  x: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`,
  back: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"></polyline></svg>`
};

const connections = ref<any[]>([]);
const isPaused = ref(false);
const selectedConn = ref<any>(null);

onMounted(async () => {
  EventsOn("connections-update", (data: any) => {
    if (isPaused.value) return;

    if (data && Array.isArray(data.connections)) {
      connections.value = data.connections;
      if (selectedConn.value) {
        const updated = data.connections.find((c: any) => c.id === selectedConn.value.id);
        if (updated) selectedConn.value = updated;
        else selectedConn.value = null; // 连接消失时自动退回列表
      }
    } else {
      connections.value = [];
    }
  });

  try {
    await (API as any).StartConnectionMonitor();
  } catch (e) {
    console.error("启动连接监控失败:", e);
  }
});

onUnmounted(() => {
  EventsOff("connections-update");
  (API as any).StopConnectionMonitor();
});

const isDirect = (conn: any) => {
  const chains = conn.chains || [];
  return chains.includes('DIRECT') || chains.includes('REJECT');
};

const getProxyName = (conn: any) => {
  const chains = conn.chains || [];
  if (chains.length > 0) return chains[0];
  return 'Unknown';
};

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const openDetail = (conn: any) => {
  selectedConn.value = conn;
};

const closeDetail = () => {
  selectedConn.value = null;
};

const closeAll = async () => {
  if (confirm('确定要强行切断当前所有的网络连接吗？')) {
    try {
      await (API as any).CloseAllConnections();
      if (selectedConn.value) closeDetail();
    } catch (e) { alert("操作失败: " + e); }
  }
};

const closeSingleConnection = async (id: string) => {
  try {
    await (API as any).CloseConnection(id);
    closeDetail();
  } catch (e) { alert("断开失败: " + e); }
};
</script>

<style scoped>
.connections-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

.action-bar { display: flex; justify-content: space-between; align-items: center; padding: 12px 16px; margin-bottom: 24px; border-radius: 12px; }
.stats .count { font-weight: 600; font-size: 0.95rem; color: var(--text-main); }

.global-actions { display: flex; gap: 12px; }
.btn-icon { width: 14px; height: 14px; display: inline-flex; }

.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

.conn-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }

.conn-card { background: var(--surface); border: none; border-radius: 12px; padding: 16px; cursor: pointer; transition: all 0.2s ease; display: flex; flex-direction: column; gap: 12px; }
.conn-card:hover { background: var(--surface-hover); }

.conn-header { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.host { font-weight: 600; font-size: 0.95rem; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
.network { font-size: 0.7rem; font-family: var(--font-mono); background: var(--surface-hover); padding: 2px 6px; border-radius: 4px; color: var(--text-sub); text-transform: uppercase; }

.conn-body { display: flex; flex-direction: column; gap: 8px; }
.tags { display: flex; flex-wrap: wrap; gap: 6px; }
.tag { font-size: 0.75rem; padding: 4px 8px; border-radius: 6px; font-weight: 500; border: none; }
.tag-direct { background: var(--surface-hover); color: var(--text-main); }
.tag-proxy { background: var(--surface-hover); color: var(--text-sub); }
.dark .tag-proxy { color: var(--text-sub); }
.tag-rule { background: var(--surface-hover); color: var(--text-muted); }

.conn-footer { display: flex; justify-content: space-between; font-size: 0.8rem; color: var(--text-muted); }
.transfer { background: var(--surface-hover); padding: 4px 8px; border-radius: 6px; }

.empty-state { height: 200px; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-style: italic; }

/* 详情子页样式 */
.detail-page { display: flex; flex-direction: column; height: 100%; border-radius: 12px; background: var(--surface); border: none; padding: 24px; box-sizing: border-box; }
.detail-header { display: flex; justify-content: space-between; align-items: center; padding-bottom: 16px; margin-bottom: 24px; }
.detail-header h3 { margin: 0; font-size: 1.2rem; color: var(--text-main); font-weight: 600; }
.back-btn { width: 100px; justify-content: center; }
.header-placeholder { width: 100px; } /* 占位保证标题绝对居中 */

.detail-body { display: flex; flex-direction: column; gap: 16px; flex: 1; padding-right: 12px; }
.detail-row { display: flex; align-items: flex-start; font-size: 0.9rem; padding-bottom: 12px; }
.detail-row span:first-child { color: var(--text-muted); width: 100px; flex-shrink: 0; font-weight: 500; }
.detail-row span:last-child { color: var(--text-main); word-break: break-all; flex: 1; }
.path-chain { color: var(--text-main) !important; font-weight: 500; }

.detail-footer { display: flex; justify-content: flex-end; margin-top: 24px; padding-top: 16px; }
</style>
