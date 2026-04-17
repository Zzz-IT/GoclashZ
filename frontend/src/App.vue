<script setup lang="ts">
import { ref, onMounted, onUnmounted, reactive } from 'vue'
import {
  RunProxy, StopProxy, GetProxyStatus, GetProxyNodes,
  SelectProxy, SetConfigMode, GetInitialData,
  UpdateSubscription, GetNodeDelay, GetOfflineDelay
} from '../wailsjs/go/main/App'

// ================== 状态定义 ==================
const currentTab = ref('dashboard')
const isRunning = ref(false)
const statusMessage = ref('就绪')
const yamlError = ref('')
const upSpeed = ref('0 B/s')
const downSpeed = ref('0 B/s')
let trafficWs: WebSocket | null = null

// 设置项
const subUrl = ref(localStorage.getItem('sub_url') || '')
const customUA = ref(localStorage.getItem('custom_ua') || 'clash-verge/1.0')
const testUrl = ref(localStorage.getItem('test_url') || 'http://www.gstatic.com/generate_204')

// 节点数据
const currentMode = ref('rule')
const proxyGroups = ref<any[]>([])
const selectedGroup = ref<string>('')
const nodeDelays = reactive<Record<string, number>>({})
const isTesting = ref(false)

// ================== 初始化 ==================
onMounted(async () => {
  await loadOfflineData()
  const status = await GetProxyStatus()
  isRunning.value = status
  if (status) {
    statusMessage.value = '✅ 运行中'
    startTrafficMonitor()
    await loadOnlineNodes()
  }
})

// ================== 测速核心逻辑 (重点) ==================
const runTest = async () => {
  if (isTesting.value) return
  isTesting.value = true

  const group = proxyGroups.value.find(g => g.name === selectedGroup.value)
  if (!group) return

  // 并行探测当前策略组下的所有节点
  const testPromises = group.proxies.map(async (nodeName: string) => {
    let delay = -1
    if (isRunning.value) {
      // 在线：走 Clash 内核 API (HTTP Delay)
      delay = await GetNodeDelay(nodeName, testUrl.value)
    } else {
      // 离线：走 Go 原生 TCP 握手 (TCP Ping)
      delay = await GetOfflineDelay(nodeName)
    }
    nodeDelays[nodeName] = delay
  })

  await Promise.all(testPromises)
  isTesting.value = false
}

// ================== 基础交互 ==================
const loadOfflineData = async () => {
  const initData: any = await GetInitialData()
  if (initData.error) { yamlError.value = initData.error; return }
  yamlError.value = ''
  if (initData.mode) currentMode.value = initData.mode.toLowerCase()
  if (initData.groups) {
    proxyGroups.value = initData.groups.map((g: any) => ({
      name: g.name, type: g.type, now: '等待启动...', proxies: g.proxies || []
    }))
    if (!selectedGroup.value) selectedGroup.value = proxyGroups.value[0].name
  }
}

const loadOnlineNodes = async () => {
  if (!isRunning.value) return
  const nodes = await GetProxyNodes()
  if (nodes) proxyGroups.value = nodes
}

const handleStart = async () => {
  statusMessage.value = '启动中...'
  const offlineSels = proxyGroups.value.filter(g => g.now !== '等待启动...').map(g => ({ name: g.name, selected: g.now }))
  try {
    await RunProxy()
    isRunning.value = true
    startTrafficMonitor()
    setTimeout(async () => {
      await SetConfigMode(currentMode.value.charAt(0).toUpperCase() + currentMode.value.slice(1))
      for (const s of offlineSels) { await SelectProxy(s.name, s.selected) }
      await loadOnlineNodes()
    }, 1000)
  } catch (e) { statusMessage.value = '启动失败' }
}

const handleStop = async () => {
  await StopProxy()
  isRunning.value = false
  stopTrafficMonitor()
  await loadOfflineData()
}

const handleNodeSelect = async (groupName: string, nodeName: string) => {
  const g = proxyGroups.value.find(x => x.name === groupName)
  if (!isRunning.value) { if (g) g.now = nodeName; return }
  await SelectProxy(groupName, nodeName)
  await loadOnlineNodes()
}

const handleUpdateSub = async () => {
  const res = await UpdateSubscription(subUrl.value, customUA.value)
  alert(res); await loadOfflineData()
}

// ================== 辅助函数 ==================
const startTrafficMonitor = () => {
  if (trafficWs) return
  trafficWs = new WebSocket('ws://127.0.0.1:9090/traffic')
  trafficWs.onmessage = (e) => {
    const d = JSON.parse(e.data)
    upSpeed.value = formatBytes(d.up) + '/s'
    downSpeed.value = formatBytes(d.down) + '/s'
  }
}
const stopTrafficMonitor = () => { if (trafficWs) { trafficWs.close(); trafficWs = null } }
const formatBytes = (b: number) => {
  if (b === 0) return '0 B'
  const k = 1024; const s = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(b) / Math.log(k))
  return parseFloat((b / Math.pow(k, i)).toFixed(2)) + ' ' + s[i]
}
</script>

<template>
  <div class="app-container">
    <aside class="sidebar">
      <div class="logo">GoclashZ</div>
      <nav>
        <button :class="{ active: currentTab === 'dashboard' }" @click="currentTab = 'dashboard'">📊 仪表盘</button>
        <button :class="{ active: currentTab === 'settings' }" @click="currentTab = 'settings'">⚙️ 设置</button>
      </nav>
    </aside>

    <main class="main-content">
      <div v-if="currentTab === 'dashboard'">
        <header class="header">
          <div class="status-wrapper">
            <div class="mode-selector glass">
              <button :class="{active: currentMode === 'rule'}" @click="currentMode='rule'; isRunning && SetConfigMode('Rule')">规则</button>
              <button :class="{active: currentMode === 'global'}" @click="currentMode='global'; isRunning && SetConfigMode('Global')">全局</button>
              <button :class="{active: currentMode === 'direct'}" @click="currentMode='direct'; isRunning && SetConfigMode('Direct')">直连</button>
            </div>
            <p class="status-text"><span class="dot" :class="{ active: isRunning }"></span> {{ statusMessage }}</p>
          </div>
          <div class="actions">
            <button class="btn-test" @click="runTest" :disabled="isTesting">{{ isTesting ? '探测中...' : '⚡ 测速' }}</button>
            <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动</button>
            <button v-else class="btn-stop" @click="handleStop">■ 停止</button>
          </div>
        </header>

        <section class="traffic-card glass">
          <div class="t-item"><span class="label">UP</span><span class="value up">{{ upSpeed }}</span></div>
          <div class="t-item"><span class="label">DOWN</span><span class="value down">{{ downSpeed }}</span></div>
        </section>

        <section v-if="proxyGroups.length > 0">
          <div class="group-tabs">
            <button v-for="g in proxyGroups" :key="g.name" :class="{ active: selectedGroup === g.name }" @click="selectedGroup = g.name">{{ g.name }}</button>
          </div>
          <div class="node-list glass">
            <div v-for="g in proxyGroups" v-show="selectedGroup === g.name" :key="g.name">
              <div class="node-grid">
                <button v-for="node in g.proxies" :key="node" :class="{ active: g.now === node }" @click="handleNodeSelect(g.name, node)" class="node-btn">
                  <span class="n-name">{{ node }}</span>
                  <span v-if="nodeDelays[node]" :class="['n-delay', { slow: nodeDelays[node] > 500, err: nodeDelays[node] === -1 }]">
                    {{ nodeDelays[node] === -1 ? '超时' : nodeDelays[node] + 'ms' }}
                  </span>
                </button>
              </div>
            </div>
          </div>
        </section>
      </div>

      <div v-else class="settings-view">
        <h2 class="title">订阅与更新</h2>
        <div class="card glass">
          <div class="field"><label>订阅链接</label><input v-model="subUrl" /></div>
          <div class="field"><label>测速地址</label><input v-model="testUrl" /></div>
          <button class="btn-update" @click="handleUpdateSub">🚀 更新订阅</button>
        </div>
      </div>
    </main>
  </div>
</template>

<style>
/* 核心样式 */
:root { --bg: #0f172a; --panel: rgba(30, 41, 59, 0.7); --border: rgba(255,255,255,0.1); --accent: #3b82f6; --success: #10b981; --danger: #ef4444; }
body { margin: 0; background: var(--bg); color: #f8fafc; font-family: sans-serif; }
.app-container { display: flex; height: 100vh; }
.sidebar { width: 200px; background: rgba(15,23,42,0.8); border-right: 1px solid var(--border); padding: 20px; }
.sidebar button { width: 100%; padding: 12px; margin-bottom: 10px; background: transparent; border: none; color: #94a3b8; text-align: left; cursor: pointer; border-radius: 8px; }
.sidebar button.active { background: var(--accent); color: white; }
.main-content { flex: 1; padding: 30px; overflow-y: auto; }
.glass { background: var(--panel); backdrop-filter: blur(10px); border: 1px solid var(--border); border-radius: 12px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
.mode-selector { display: flex; padding: 4px; }
.mode-selector button { padding: 6px 15px; background: transparent; border: none; color: #94a3b8; cursor: pointer; border-radius: 6px; }
.mode-selector button.active { background: var(--accent); color: white; }
.status-text { display: flex; align-items: center; gap: 8px; font-weight: bold; }
.dot { width: 10px; height: 10px; background: var(--danger); border-radius: 50%; }
.dot.active { background: var(--success); box-shadow: 0 0 8px var(--success); }
.actions button { padding: 10px 20px; border-radius: 8px; border: none; cursor: pointer; font-weight: bold; }
.btn-start { background: var(--success); color: white; }
.btn-stop { background: var(--danger); color: white; }
.btn-test { background: #6366f1; color: white; margin-right: 10px; }
.traffic-card { display: flex; justify-content: space-around; padding: 20px; margin-bottom: 30px; }
.t-item { display: flex; flex-direction: column; align-items: center; }
.value { font-size: 1.5rem; font-weight: bold; font-family: monospace; }
.up { color: #f59e0b; } .down { color: #3b82f6; }
.group-tabs { display: flex; gap: 8px; margin-bottom: 20px; overflow-x: auto; }
.tab-btn { padding: 8px 15px; background: rgba(255,255,255,0.05); border: 1px solid var(--border); border-radius: 6px; color: #94a3b8; cursor: pointer; white-space: nowrap; }
.tab-btn.active { background: var(--accent); color: white; }
.node-list { padding: 20px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
.node-btn { background: rgba(0,0,0,0.2); border: 1px solid var(--border); padding: 12px 8px; border-radius: 8px; color: #94a3b8; cursor: pointer; display: flex; flex-direction: column; align-items: center; }
.node-btn.active { border-color: var(--success); color: var(--success); font-weight: bold; }
.n-name { font-size: 0.9rem; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; }
.n-delay { font-size: 0.75rem; color: var(--success); margin-top: 4px; }
.n-delay.slow { color: #f59e0b; }
.n-delay.err { color: var(--danger); }
.settings-view .card { padding: 25px; display: flex; flex-direction: column; gap: 15px; }
.field { display: flex; flex-direction: column; gap: 8px; }
.field input { background: rgba(0,0,0,0.3); border: 1px solid var(--border); padding: 10px; border-radius: 6px; color: white; }
.btn-update { background: var(--accent); color: white; padding: 12px; border: none; border-radius: 6px; cursor: pointer; font-weight: bold; }
</style>