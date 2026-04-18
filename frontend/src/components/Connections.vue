<template>
  <div class="connections-view">
    <div class="action-bar glass-panel">
      <div class="stats">
        <span class="count">活跃连接: {{ connections.length }}</span>
      </div>
      <div class="global-actions">
        <button class="secondary-btn" @click="isPaused = !isPaused">
          <span class="btn-icon" v-html="isPaused ? ICONS.play : ICONS.pause"></span>
          {{ isPaused ? '继续刷新' : '暂停刷新' }}
        </button>
        <button class="primary-action-btn stop-btn" @click="closeAll">
          <span class="btn-icon" v-html="ICONS.xCircle"></span>
          断开全部
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

    <div v-if="showModal" class="modal-overlay" @click="closeDetail">
      <div class="modal-content glass-panel" @click.stop>
        <div class="modal-header">
          <h3>连接详情</h3>
          <button class="close-btn" @click="closeDetail" v-html="ICONS.x"></button>
        </div>
        <div class="modal-body" v-if="selectedConn">
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
        <div class="modal-footer">
          <button class="danger-btn" @click="closeSingleConnection(selectedConn.id)">强行断开此连接</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const ICONS = {
  pause: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="4" width="4" height="16"></rect><rect x="14" y="4" width="4" height="16"></rect></svg>`,
  play: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>`,
  xCircle: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`,
  x: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`
};

const connections = ref<any[]>([]);
const isPaused = ref(false);
const showModal = ref(false);
const selectedConn = ref<any>(null);
let timer: ReturnType<typeof setInterval> | null = null;

const fetchConnections = async () => {
  if (isPaused.value) return;
  try {
    const data: any = await (API as any).GetConnections();
    
    // 增强健壮性：严格判断 data.connections 必须存在且为数组
    if (data && Array.isArray(data.connections)) {
      connections.value = data.connections;
      // 同步更新打开的详情弹窗数据
      if (showModal.value && selectedConn.value) {
        const updated = data.connections.find((c: any) => c.id === selectedConn.value.id);
        if (updated) selectedConn.value = updated;
        else showModal.value = false; // 如果在后台已经被关闭，则退出弹窗
      }
    } else {
      // 容错处理：如果内核尚未完全启动或数据异常，清空列表
      connections.value = [];
    }
  } catch (e) {
    console.error("加载连接数据失败", e);
  }
};

const startTimer = () => {
  fetchConnections();
  timer = setInterval(fetchConnections, 1000); // 1秒一次的心跳轮询
};

onMounted(() => { startTimer(); });

onUnmounted(() => { if (timer) clearInterval(timer); });

// 判断是否直连
const isDirect = (conn: any) => {
  const chains = conn.chains || [];
  return chains.includes('DIRECT') || chains.includes('REJECT');
};

// 获取最外层的代理名称
const getProxyName = (conn: any) => {
  const chains = conn.chains || [];
  if (chains.length > 0) return chains[0];
  return 'Unknown';
};

// 字节单位格式化
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const openDetail = (conn: any) => {
  selectedConn.value = conn;
  showModal.value = true;
};

const closeDetail = () => {
  showModal.value = false;
  selectedConn.value = null;
};

// 断开所有
const closeAll = async () => {
  if (confirm('确定要强行切断当前所有的网络连接吗？')) {
    try {
      await (API as any).CloseAllConnections();
      connections.value = [];
      if (showModal.value) closeDetail();
    } catch (e) { alert("操作失败: " + e); }
  }
};

// 断开单一
const closeSingleConnection = async (id: string) => {
  try {
    await (API as any).CloseConnection(id);
    closeDetail();
    fetchConnections();
  } catch (e) { alert("断开失败: " + e); }
};
</script>

<style scoped>
.connections-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

.action-bar { display: flex; justify-content: space-between; align-items: center; padding: 12px 16px; margin-bottom: 24px; border-radius: 12px; }
.stats .count { font-weight: 600; font-size: 0.95rem; color: var(--text-main); }

.global-actions { display: flex; gap: 12px; }
.secondary-btn, .primary-action-btn { display: flex; align-items: center; gap: 6px; padding: 8px 16px; border-radius: 8px; border: none; font-size: 0.85rem; font-weight: 500; cursor: pointer; transition: 0.2s; }
.secondary-btn { background: var(--surface-hover); color: var(--text-main); border: 1px solid var(--glass-border); }
.secondary-btn:hover { background: var(--glass-panel); }
.stop-btn { background: #ef4444; color: white; }
.stop-btn:hover { opacity: 0.85; }
.btn-icon { width: 14px; height: 14px; display: inline-flex; }

.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

.conn-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }

.conn-card { background: var(--surface); border: 1px solid var(--glass-border); border-radius: 12px; padding: 16px; cursor: pointer; transition: all 0.2s ease; display: flex; flex-direction: column; gap: 12px; }
.conn-card:hover { background: var(--surface-hover); border-color: var(--text-sub); transform: translateY(-2px); }

.conn-header { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.host { font-weight: 600; font-size: 0.95rem; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
.network { font-size: 0.7rem; font-family: var(--font-mono); background: var(--surface-hover); padding: 2px 6px; border-radius: 4px; color: var(--text-sub); text-transform: uppercase; }

.conn-body { display: flex; flex-direction: column; gap: 8px; }
.tags { display: flex; flex-wrap: wrap; gap: 6px; }
.tag { font-size: 0.75rem; padding: 4px 8px; border-radius: 6px; font-weight: 500; }
.tag-direct { background: rgba(16, 185, 129, 0.1); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.2); }
.tag-proxy { background: rgba(79, 70, 229, 0.1); color: #6366f1; border: 1px solid rgba(79, 70, 229, 0.2); }
.dark .tag-proxy { color: #818cf8; }
.tag-rule { background: var(--surface-hover); color: var(--text-sub); border: 1px solid var(--glass-border); }

.conn-footer { display: flex; justify-content: space-between; font-size: 0.8rem; color: var(--text-muted); }
.transfer { background: var(--surface-hover); padding: 4px 8px; border-radius: 6px; }

.empty-state { height: 200px; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-style: italic; }

/* 详情弹窗样式 */
.modal-overlay { position: fixed; top: 0; left: 0; width: 100vw; height: 100vh; background: rgba(0,0,0,0.4); backdrop-filter: blur(4px); display: flex; align-items: center; justify-content: center; z-index: 100; }
.modal-content { width: 440px; max-width: 90%; background: var(--glass-bg); padding: 24px; border-radius: 16px; display: flex; flex-direction: column; gap: 20px; box-shadow: 0 10px 25px rgba(0,0,0,0.2); }
.modal-header { display: flex; justify-content: space-between; align-items: center; }
.modal-header h3 { margin: 0; font-size: 1.1rem; color: var(--text-main); }
.close-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; transition: 0.2s; }
.close-btn:hover { color: var(--text-main); }

.modal-body { display: flex; flex-direction: column; gap: 12px; }
.detail-row { display: flex; justify-content: space-between; align-items: flex-start; font-size: 0.85rem; border-bottom: 1px solid var(--glass-border); padding-bottom: 8px; }
.detail-row span:first-child { color: var(--text-muted); width: 80px; flex-shrink: 0; }
.detail-row span:last-child { color: var(--text-main); text-align: right; word-break: break-all; flex: 1; }
.path-chain { color: #6366f1 !important; font-weight: 500; }

.modal-footer { display: flex; justify-content: flex-end; margin-top: 8px; }
.danger-btn { background: rgba(239, 68, 68, 0.1); color: #ef4444; border: 1px solid rgba(239, 68, 68, 0.2); padding: 8px 16px; border-radius: 8px; cursor: pointer; font-weight: 500; font-size: 0.85rem; transition: 0.2s; }
.danger-btn:hover { background: #ef4444; color: white; }
</style>
