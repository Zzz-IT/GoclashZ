<template>
  <div class="connections-view">
    <template v-if="!selectedConn">
      <div class="action-bar card-panel">
        <div class="stats">
          <span class="count">活跃连接: {{ connections.length }}</span>
        </div>
        <div class="global-actions">
          <button class="action-btn" @click="isPaused = !isPaused">
            <span class="btn-icon" v-html="isPaused ? ICONS.play : ICONS.pause"></span>
            {{ isPaused ? '继续刷新' : '暂停刷新' }}
          </button>
          <button v-if="connections.length > 0" class="primary-btn accent-btn" @click="closeAll">
            <span class="btn-icon" style="color: #ff5a5a;" v-html="ICONS.xCircle"></span>
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
          <p>当前没有流量经过</p>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="detail-page card-panel">
        <div class="detail-header">
          <h3>连接详情</h3>
          <button class="action-btn back-btn" @click="closeDetail">
            返回
          </button>
        </div>

        <div class="detail-body scroll-content">
          <div class="detail-row"><span>ID:</span> <span class="font-mono">{{ selectedConn.id }}</span></div>
          <div class="detail-row"><span>网络:</span> <span>{{ selectedConn.metadata.network }}</span></div>
          <div class="detail-row"><span>源地址:</span> <span class="font-mono">{{ selectedConn.metadata.sourceIP }}:{{ selectedConn.metadata.sourcePort }}</span></div>
          <div class="detail-row"><span>目标地址:</span> <span class="font-mono">{{ selectedConn.metadata.destinationIP || selectedConn.metadata.host }}:{{ selectedConn.metadata.destinationPort }}</span></div>
          <div class="detail-row"><span>匹配规则:</span> <span>{{ selectedConn.rule }} {{ selectedConn.rulePayload ? `(${selectedConn.rulePayload})` : '' }}</span></div>
          <div class="detail-row"><span>代理链路:</span> <span class="path-chain">{{ selectedConn.chains.join(' ➔ ') }}</span></div>
          <div class="detail-row"><span>上传流量:</span> <span class="font-mono">{{ selectedConn.uploadStr }}</span></div>
          <div class="detail-row"><span>下载流量:</span> <span class="font-mono">{{ selectedConn.downloadStr }}</span></div>
          <div class="detail-row"><span>连接时间:</span> <span>{{ new Date(selectedConn.start).toLocaleString() }} ({{ selectedConn.durationStr }})</span></div>
        </div>

        <div class="detail-footer">
          <button class="action-btn red-text-btn" @click="closeSingleConnection(selectedConn.id)">强行断开此连接</button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { showConfirm, showAlert } from '../store';
import { ICONS } from '../utils/icons';

const connections = ref<any[]>([]);
const isPaused = ref(false);
const selectedConn = ref<any>(null);

const isMonitoring = ref(false); // 增加一个状态锁，防止重复注册监听

const startMonitor = async () => {
  if (isMonitoring.value) return;
  isMonitoring.value = true;
  
  EventsOn("connections-update", (data: any) => {
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
    await (API as any).StartConnectionMonitor();
  } catch (e) {
    console.error("启动连接监控失败:", e);
  }
};

const stopMonitor = () => {
  if (!isMonitoring.value) return;
  isMonitoring.value = false;
  EventsOff("connections-update");
  (API as any).StopConnectionMonitor();
};

// 配合 KeepAlive 的生命周期控制
onMounted(() => startMonitor());
onActivated(() => startMonitor());       // 再次切回连接页时恢复更新
onDeactivated(() => stopMonitor());      // 切到别的页面时暂停后台请求，节省性能
onUnmounted(() => stopMonitor());

const isDirect = (conn: any) => {
  const chains = conn.chains || [];
  return chains.includes('DIRECT') || chains.includes('REJECT');
};

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
  const ok = await showConfirm('确定要强行切断当前所有的网络连接吗？', '强行断开全部', true);
  if (ok) {
    try {
      await (API as any).CloseAllConnections();
      if (selectedConn.value) closeDetail();
    } catch (e) { 
      await showAlert("操作失败: " + e, '错误'); 
    }
  }
};

const closeSingleConnection = async (id: string) => {
  const ok = await showConfirm(`确定要强行断开连接吗？`, '断开连接', true);
  if (!ok) return;
  
  try {
    await (API as any).CloseConnection(id);
    closeDetail();
  } catch (e) { 
    await showAlert("断开失败: " + e, '错误'); 
  }
};
</script>

<style scoped>
.connections-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

.action-bar { display: flex; justify-content: space-between; align-items: center; padding: 12px 16px; margin-bottom: 24px; }
.stats .count { font-weight: 600; font-size: 0.95rem; color: var(--text-main); }

.global-actions { display: flex; gap: 12px; }
.btn-icon { width: 14px; height: 14px; display: inline-flex; align-items: center;}

.scroll-content { flex: 1; overflow-y: auto; padding-right: 8px; }

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
.icon-svg :deep(svg) { width: 12px; height: 12px; }

.empty-state { height: 200px; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-style: italic; }

/* --- 详情子页样式 --- */
.detail-page { display: flex; flex-direction: column; height: 100%; border: none; padding: 24px; box-sizing: border-box; }

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
