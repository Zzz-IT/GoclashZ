<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import {
  RunProxy, StopProxy, GetProxyStatus, StartAsyncTest,
  GetInitialData, StartStreamingLogs, UpdateClashSettings
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// ================== 状态变量 ==================
const currentTab = ref('dashboard') // dashboard | logs | settings
const isRunning = ref(false)
const statusMessage = ref('等待启动')

// 节点数据
const proxyGroups = ref<any[]>([])
const selectedGroup = ref('')
const nodeDelays = reactive<Record<string, number>>({})

// 实时日志数据
const logs = ref<any[]>([])
const maxLogs = 150

// 特性开关状态 (Reactive)
const features = reactive({
  "allow-lan": false,
  "ipv6": false,
  "tun": { "enable": false, "stack": "system" }
})

// ================== 初始化 ==================
onMounted(async () => {
  // 1. 监听后端推过来的日志
  EventsOn("clash_log", (log: any) => {
    logs.value.unshift(log)
    if (logs.value.length > maxLogs) logs.value.pop()
  })

  // 2. 监听测速更新
  EventsOn("node_delay_update", (data: any) => {
    nodeDelays[data.name] = data.delay
  })

  // 3. 初始加载
  await loadInitial()
})

const loadInitial = async () => {
  const initData: any = await GetInitialData()
  if (initData.groups) {
    proxyGroups.value = initData.groups
    selectedGroup.value = initData.groups[0].name
  }
  isRunning.value = await GetProxyStatus()
  if (isRunning.value) {
    statusMessage.value = '✅ 代理运行中'
    StartStreamingLogs() // 如果启动中，开启日志流
  }
}

// ================== 交互方法 ==================

const handleStart = async () => {
  statusMessage.value = '正在启动...'
  const res = await RunProxy()
  isRunning.value = true
  statusMessage.value = res
  setTimeout(() => StartStreamingLogs(), 1000) // 延迟启动日志流
}

const handleStop = async () => {
  await StopProxy()
  isRunning.value = false
  statusMessage.value = '🛑 已停止'
}

const runTest = async () => {
  if (!selectedGroup.value) return
  await StartAsyncTest(selectedGroup.value)
}

// ✨ 模仿 Stelliberty 的特性切换逻辑
const toggleFeature = async (key: string, value: any) => {
  if (!isRunning.value) {
    alert("请先启动代理后再修改特性")
    return
  }
  const payload: any = {}
  payload[key] = value
  const res = await UpdateClashSettings(payload)
  console.log(res)
}
</script>

<template>
  <div class="app-container">
    <aside class="sidebar">
      <div class="logo">GoclashZ</div>
      <nav>
        <button :class="{active: currentTab==='dashboard'}" @click="currentTab='dashboard'">📊 仪表盘</button>
        <button :class="{active: currentTab==='logs'}" @click="currentTab='logs'">📜 实时日志</button>
        <button :class="{active: currentTab==='settings'}" @click="currentTab='settings'">⚙️ 特性设置</button>
      </nav>
    </aside>

    <main class="main-content">
      <div v-if="currentTab === 'dashboard'" class="tab-content">
        <header class="header">
          <div class="status-indicator">
            <span class="dot" :class="{active: isRunning}"></span>
            <span class="status-text">{{ statusMessage }}</span>
          </div>
          <div class="actions">
            <button class="btn-test" @click="runTest">⚡ 测速</button>
            <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动</button>
            <button v-else class="btn-stop" @click="handleStop">■ 停止</button>
          </div>
        </header>

        <section class="node-area glass">
          <div class="group-tabs">
            <button v-for="g in proxyGroups" :key="g.name" :class="{active: selectedGroup === g.name}" @click="selectedGroup=g.name">{{ g.name }}</button>
          </div>
          <div class="node-grid">
             <div v-for="n in proxyGroups.find(x=>x.name===selectedGroup)?.proxies" :key="n" class="node-card">
                <span class="n-name">{{ n }}</span>
                <span v-if="nodeDelays[n]" :class="['n-delay', {err: nodeDelays[n]==-1}]">
                   {{ nodeDelays[n] == -1 ? 'Error' : nodeDelays[n]+'ms' }}
                </span>
             </div>
          </div>
        </section>
      </div>

      <div v-if="currentTab === 'logs'" class="tab-content animate-in">
        <div class="view-header">
           <h2>内核实时输出</h2>
           <button class="btn-clear" @click="logs = []">🗑️ 清空日志</button>
        </div>
        <div class="log-viewer">
           <div v-for="(log, i) in logs" :key="i" class="log-line">
              <span :class="['log-label', log.type.toLowerCase()]">[{{ log.type.toUpperCase() }}]</span>
              <span class="log-msg">{{ log.payload }}</span>
           </div>
           <div v-if="logs.length === 0" class="empty-hint">等待内核输出日志...</div>
        </div>
      </div>

      <div v-if="currentTab === 'settings'" class="tab-content animate-in">
         <h2 class="view-header">Clash 高级特性</h2>
         <div class="feature-list">
            <div class="feature-item glass">
               <div class="f-text">
                  <h3>允许局域网 (Allow LAN)</h3>
                  <p>开启后，同一 WiFi 下的其他手机/电脑可连接此代理</p>
               </div>
               <label class="switch">
                  <input type="checkbox" v-model="features['allow-lan']" @change="toggleFeature('allow-lan', features['allow-lan'])">
                  <span class="slider"></span>
               </label>
            </div>

            <div class="feature-item glass">
               <div class="f-text">
                  <h3>IPv6 支持</h3>
                  <p>是否允许代理 IPv6 网络流量</p>
               </div>
               <label class="switch">
                  <input type="checkbox" v-model="features['ipv6']" @change="toggleFeature('ipv6', features['ipv6'])">
                  <span class="slider"></span>
               </label>
            </div>

            <div class="feature-item glass">
               <div class="f-text">
                  <h3>TUN 模式 (全虚拟网卡)</h3>
                  <p>真正的全局代理，接管不支持代理设置的游戏或软件</p>
               </div>
               <label class="switch">
                  <input type="checkbox" v-model="features.tun.enable" @change="toggleFeature('tun', features.tun)">
                  <span class="slider"></span>
               </label>
            </div>
         </div>
      </div>
    </main>
  </div>
</template>

<style>
/* 核心布局与侧边栏样式同前，以下为新增样式 */

/* 日志浏览器 */
.log-viewer {
  height: 75vh;
  background: #020617;
  border-radius: 12px;
  padding: 20px;
  overflow-y: auto;
  font-family: 'Fira Code', 'Consolas', monospace;
  font-size: 0.85rem;
  border: 1px solid var(--border-color);
}
.log-line { margin-bottom: 6px; white-space: pre-wrap; word-break: break-all; line-height: 1.4; border-bottom: 1px solid #1e293b; padding-bottom: 4px;}
.log-label { font-weight: bold; margin-right: 10px; min-width: 60px; display: inline-block;}
.log-label.info { color: #3b82f6; }
.log-label.warning { color: #f59e0b; }
.log-label.error { color: #ef4444; }
.log-msg { color: #cbd5e1; }

/* 特性列表 */
.feature-list { display: flex; flex-direction: column; gap: 15px; }
.feature-item { display: flex; justify-content: space-between; align-items: center; padding: 25px; }
.f-text h3 { margin: 0 0 5px 0; font-size: 1.1rem; color: #f8fafc; }
.f-text p { margin: 0; font-size: 0.85rem; color: #94a3b8; }

/* 现代开关 (CSS Switch) */
.switch { position: relative; display: inline-block; width: 46px; height: 24px; }
.switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: #334155; transition: .4s; border-radius: 34px; }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .4s; border-radius: 50%; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(22px); }

.animate-in { animation: fadeIn 0.4s ease-out; }
@keyframes fadeIn { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
</style>