<script setup lang="ts">
import { ref, onMounted, onUnmounted, reactive, nextTick } from 'vue'
// 引入绑定的 Go 方法
import {
  RunProxy, StopProxy, GetProxyStatus, GetProxyNodes,
  SelectProxy, SetConfigMode, GetInitialData,
  UpdateSubscription, StartAsyncTest, StartStreamingLogs,
  UpdateClashSettings, CheckTunEnv
} from '../wailsjs/go/main/App'
// 引入 Wails 运行时
import { EventsOn } from '../wailsjs/runtime/runtime'

// ================== 类型定义 ==================
interface ProxyNode {
  name: string
  type: string
  now: string
  proxies: string[]
}

// ================== 核心状态 ==================
const currentTab = ref('dashboard') // dashboard | logs | settings
const isRunning = ref(false)
const statusMessage = ref('等待点火')
const yamlError = ref('')

// 流量数据
const upSpeed = ref('0 B/s')
const downSpeed = ref('0 B/s')
let trafficWs: WebSocket | null = null

// 设置项 (持久化)
const subUrl = ref(localStorage.getItem('sub_url') || '')
const customUA = ref(localStorage.getItem('custom_ua') || 'clash-verge/1.0')
const testUrl = ref(localStorage.getItem('test_url') || 'http://www.gstatic.com/generate_204')

// 节点与模式
const currentMode = ref('rule')
const proxyGroups = ref<ProxyNode[]>([])
const selectedGroup = ref('')
const nodeDelays = reactive<Record<string, number>>({})
const isTesting = ref(false)

// 日志系统
const logs = ref<any[]>([])
const maxLogs = 200

// 特性开关
const features = reactive({
  "allow-lan": false,
  "ipv6": false,
  "tun": { "enable": false }
})

// ================== 生命周期 ==================
onMounted(async () => {
  // 1. 监听后端推过来的高并发测速结果 (Stelliberty 风格)
  EventsOn("node_delay_update", (data: any) => {
    nodeDelays[data.name] = data.delay
  })

  // 2. 监听内核实时日志
  EventsOn("clash_log", (log: any) => {
    logs.value.unshift(log)
    if (logs.value.length > maxLogs) logs.value.pop()
  })

  // 3. 初始化加载
  await initApp()
})

onUnmounted(() => {
  stopTrafficMonitor()
})

const initApp = async () => {
  // 加载离线节点预览
  const initData: any = await GetInitialData()
  if (initData.error) {
    yamlError.value = initData.error
  } else {
    yamlError.value = ''
    if (initData.mode) currentMode.value = initData.mode.toLowerCase()
    if (initData.groups) {
      proxyGroups.value = initData.groups.map((g: any) => ({
        ...g, now: '等待启动...'
      }))
      selectedGroup.value = proxyGroups.value[0]?.name || ''
    }
  }

  // 检查运行状态
  isRunning.value = await GetProxyStatus()
  if (isRunning.value) {
    statusMessage.value = '✅ 代理运行中'
    startTrafficMonitor()
    StartStreamingLogs() // 开启日志流
    await refreshOnlineNodes()
  }
}

// ================== 核心交互逻辑 ==================

// 启动
const handleStart = async () => {
  if (yamlError.value) return alert("配置文件有误")
  statusMessage.value = '正在点火...'

  // 提取预选
  const offlineSelections = proxyGroups.value
    .filter(g => g.now !== '等待启动...')
    .map(g => ({ name: g.name, selected: g.now }))

  try {
    const res = await RunProxy()
    isRunning.value = true
    statusMessage.value = res
    startTrafficMonitor()
    StartStreamingLogs()

    // 延迟同步状态
    setTimeout(async () => {
      // 同步模式
      const mode = currentMode.value.charAt(0).toUpperCase() + currentMode.value.slice(1)
      await SetConfigMode(mode)
      // 同步预选节点
      for (const s of offlineSelections) {
        await SelectProxy(s.name, s.selected)
      }
      await refreshOnlineNodes()
    }, 1000)
  } catch (e) {
    statusMessage.value = '启动失败'
  }
}

// 停止
const handleStop = async () => {
  await StopProxy()
  isRunning.value = false
  statusMessage.value = '🛑 已停止'
  stopTrafficMonitor()
  await initApp()
}

// 节点选择
const handleNodeSelect = async (groupName: string, nodeName: string) => {
  const group = proxyGroups.value.find(g => g.name === groupName)
  if (!isRunning.value) {
    if (group) group.now = nodeName // 离线记忆
    return
  }
  try {
    await SelectProxy(groupName, nodeName)
    await refreshOnlineNodes()
  } catch (e) { alert("切换失败") }
}

// 测速 (调用异步引擎)
const runTest = async () => {
  if (isTesting.value) return
  isTesting.value = true
  await StartAsyncTest(selectedGroup.value)
  setTimeout(() => { isTesting.value = false }, 2000)
}

// 特性切换 (含 TUN 环境检查)
const toggleFeature = async (key: string, value: any) => {
  if (key === 'tun' && value === true) {
    const env = await CheckTunEnv()
    if (!env.isAdmin) {
      alert("❌ 需要管理员权限才能开启 TUN 模式")
      features.tun.enable = false; return
    }
    if (!env.hasWintun) {
      alert("❌ 缺少 wintun.dll")
      features.tun.enable = false; return
    }
  }

  const payload: any = {}
  if (key === 'tun') {
    payload['tun'] = { enable: value, stack: 'system', "auto-route": true }
  } else {
    payload[key] = value
  }

  if (isRunning.value) {
    await UpdateClashSettings(payload)
  }
}

// ================== 辅助功能 ==================
const refreshOnlineNodes = async () => {
  const nodes = await GetProxyNodes()
  if (nodes) proxyGroups.value = nodes
}

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
const handleUpdateSub = async () => {
  const res = await UpdateSubscription(subUrl.value, customUA.value)
  alert(res); localStorage.setItem('sub_url', subUrl.value); await initApp()
}
</script>

<template>
  <div class="app-container">
    <aside class="sidebar">
      <div class="logo">GoclashZ</div>
      <nav>
        <button :class="{active: currentTab==='dashboard'}" @click="currentTab='dashboard'">📊 仪表盘</button>
        <button :class="{active: currentTab==='logs'}" @click="currentTab='logs'">📜 实时日志</button>
        <button :class="{active: currentTab==='settings'}" @click="currentTab='settings'">⚙️ 设置</button>
      </nav>
      <div class="sidebar-footer">v1.0.0</div>
    </aside>

    <main class="main-content">

      <div v-if="currentTab === 'dashboard'" class="tab-content animate-in">
        <header class="header">
          <div class="mode-selector glass">
            <button :class="{active: currentMode==='rule'}" @click="currentMode='rule'; isRunning && SetConfigMode('Rule')">规则</button>
            <button :class="{active: currentMode==='global'}" @click="currentMode='global'; isRunning && SetConfigMode('Global')">全局</button>
            <button :class="{active: currentMode==='direct'}" @click="currentMode='direct'; isRunning && SetConfigMode('Direct')">直连</button>
          </div>
          <div class="status-box">
             <span class="dot" :class="{active: isRunning}"></span>
             <span class="status-msg">{{ statusMessage }}</span>
          </div>
          <div class="actions">
            <button class="btn-test" @click="runTest" :disabled="isTesting">{{ isTesting ? '探测中' : '⚡ 测速' }}</button>
            <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动</button>
            <button v-else class="btn-stop" @click="handleStop">■ 停止</button>
          </div>
        </header>

        <section class="traffic-card glass">
          <div class="t-item"><span class="label">上传</span><span class="value up">{{ upSpeed }}</span></div>
          <div class="divider"></div>
          <div class="t-item"><span class="label">下载</span><span class="value down">{{ downSpeed }}</span></div>
        </section>

        <section v-if="proxyGroups.length > 0" class="node-section">
           <div class="group-tabs">
             <button v-for="g in proxyGroups" :key="g.name" :class="{active: selectedGroup === g.name}" @click="selectedGroup=g.name">{{ g.name }}</button>
           </div>
           <div class="node-list-container glass">
              <div v-for="g in proxyGroups" :key="g.name" v-show="selectedGroup === g.name">
                <div class="node-grid">
                  <button v-for="n in g.proxies" :key="n" :class="{active: g.now === n}" @click="handleNodeSelect(g.name, n)" class="node-btn">
                    <span class="n-name">{{ n }}</span>
                    <span v-if="nodeDelays[n]" :class="['n-delay', {slow: nodeDelays[n]>500, err: nodeDelays[n]==-1}]">
                       {{ nodeDelays[n] == -1 ? 'Error' : nodeDelays[n]+'ms' }}
                    </span>
                  </button>
                </div>
              </div>
           </div>
        </section>
      </div>

      <div v-if="currentTab === 'logs'" class="tab-content animate-in">
        <div class="view-header">
           <h2>内核实时日志</h2>
           <button class="btn-clear" @click="logs = []">🗑️ 清空</button>
        </div>
        <div class="log-viewer">
           <div v-for="(log, i) in logs" :key="i" class="log-line">
              <span :class="['log-label', log.type.toLowerCase()]">[{{ log.type.toUpperCase() }}]</span>
              <span class="log-msg">{{ log.payload }}</span>
           </div>
           <div v-if="logs.length === 0" class="empty-hint">等待数据流入...</div>
        </div>
      </div>

      <div v-if="currentTab === 'settings'" class="tab-content animate-in">
         <h2 class="view-header">订阅管理</h2>
         <div class="settings-card glass">
            <div class="field"><label>订阅链接</label><input v-model="subUrl" placeholder="https://..." /></div>
            <div class="field"><label>User-Agent</label><input v-model="customUA" /></div>
            <button class="btn-update" @click="handleUpdateSub">🚀 更新订阅</button>
         </div>

         <h2 class="view-header" style="margin-top: 30px;">Clash 高级特性</h2>
         <div class="feature-list">
            <div class="feature-item glass">
               <div class="f-text"><h3>允许局域网 (LAN)</h3><p>共享代理给同一 WiFi 下的设备</p></div>
               <label class="switch">
                  <input type="checkbox" v-model="features['allow-lan']" @change="toggleFeature('allow-lan', features['allow-lan'])">
                  <span class="slider"></span>
               </label>
            </div>
            <div class="feature-item glass">
               <div class="f-text"><h3>IPv6 支持</h3><p>处理 IPv6 流量</p></div>
               <label class="switch">
                  <input type="checkbox" v-model="features['ipv6']" @change="toggleFeature('ipv6', features['ipv6'])">
                  <span class="slider"></span>
               </label>
            </div>
            <div class="feature-item glass">
               <div class="f-text"><h3>TUN 模式 (全虚拟网卡)</h3><p>真正的全局接管 (需管理员权限)</p></div>
               <label class="switch">
                  <input type="checkbox" v-model="features.tun.enable" @change="toggleFeature('tun', features.tun.enable)">
                  <span class="slider"></span>
               </label>
            </div>
         </div>
      </div>
    </main>
  </div>
</template>

<style>
:root { --bg: #0f172a; --panel: rgba(30, 41, 59, 0.7); --border: rgba(255,255,255,0.1); --accent: #3b82f6; --success: #10b981; --danger: #ef4444; }
body { margin: 0; background: var(--bg); color: #f8fafc; font-family: 'Segoe UI', system-ui, sans-serif; overflow: hidden; }
.app-container { display: flex; height: 100vh; }
.animate-in { animation: fadeIn 0.3s ease-out; }
@keyframes fadeIn { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }

/* Sidebar */
.sidebar { width: 200px; background: rgba(15,23,42,0.9); border-right: 1px solid var(--border); padding: 25px 15px; display: flex; flex-direction: column; }
.logo { font-size: 1.5rem; font-weight: 900; text-align: center; margin-bottom: 40px; color: var(--accent); letter-spacing: 1px; }
.sidebar nav button { width: 100%; padding: 12px 15px; margin-bottom: 8px; background: transparent; border: none; color: #94a3b8; text-align: left; cursor: pointer; border-radius: 10px; transition: 0.2s; font-weight: 600; }
.sidebar nav button:hover { background: rgba(255,255,255,0.05); color: white; }
.sidebar nav button.active { background: var(--accent); color: white; box-shadow: 0 4px 12px rgba(59,130,246,0.3); }
.sidebar-footer { margin-top: auto; font-size: 0.75rem; color: #475569; text-align: center; }

/* Content */
.main-content { flex: 1; padding: 30px; overflow-y: auto; background: radial-gradient(circle at top right, #1e293b, transparent); }
.glass { background: var(--panel); backdrop-filter: blur(12px); border: 1px solid var(--border); border-radius: 16px; }

/* Header */
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
.mode-selector { display: flex; padding: 4px; border-radius: 12px; }
.mode-selector button { padding: 8px 18px; background: transparent; border: none; color: #94a3b8; cursor: pointer; border-radius: 10px; font-weight: bold; transition: 0.2s; }
.mode-selector button.active { background: var(--accent); color: white; }
.status-box { display: flex; align-items: center; gap: 10px; font-weight: 600; }
.dot { width: 10px; height: 10px; background: var(--danger); border-radius: 50%; box-shadow: 0 0 8px var(--danger); }
.dot.active { background: var(--success); box-shadow: 0 0 10px var(--success); }

.actions button { padding: 10px 22px; border-radius: 10px; border: none; cursor: pointer; font-weight: bold; transition: 0.2s; }
.btn-start { background: var(--success); color: white; }
.btn-stop { background: var(--danger); color: white; }
.btn-test { background: #6366f1; color: white; margin-right: 10px; }
.btn-test:disabled { opacity: 0.5; cursor: not-allowed; }

/* Dashboard Cards */
.traffic-card { display: flex; justify-content: space-around; padding: 25px; margin-bottom: 30px; }
.t-item { display: flex; flex-direction: column; align-items: center; gap: 5px; }
.label { font-size: 0.85rem; color: #94a3b8; }
.value { font-size: 1.6rem; font-weight: 800; font-family: 'JetBrains Mono', monospace; }
.up { color: #f59e0b; } .down { color: #3b82f6; }
.divider { width: 1px; height: 40px; background: var(--border); }

/* Nodes */
.group-tabs { display: flex; gap: 10px; margin-bottom: 15px; overflow-x: auto; padding-bottom: 5px; }
.tab-btn, .group-tabs button { padding: 8px 16px; background: rgba(255,255,255,0.03); border: 1px solid var(--border); border-radius: 8px; color: #94a3b8; cursor: pointer; white-space: nowrap; font-size: 0.9rem; }
.group-tabs button.active { background: var(--accent); color: white; border-color: var(--accent); }

.node-list-container { padding: 20px; min-height: 200px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
.node-btn { background: rgba(0,0,0,0.2); border: 1px solid var(--border); padding: 12px 10px; border-radius: 10px; color: #94a3b8; cursor: pointer; display: flex; flex-direction: column; align-items: center; gap: 4px; transition: 0.2s; }
.node-btn:hover { background: rgba(255,255,255,0.05); color: white; }
.node-btn.active { border-color: var(--success); color: var(--success); background: rgba(16, 185, 129, 0.05); font-weight: bold; }
.n-name { font-size: 0.9rem; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; }
.n-delay { font-size: 0.75rem; color: var(--success); font-weight: bold; }
.n-delay.slow { color: #f59e0b; }
.n-delay.err { color: var(--danger); }

/* Logs */
.view-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.log-viewer { height: 70vh; background: #020617; border-radius: 12px; padding: 15px; overflow-y: auto; font-family: 'Consolas', monospace; font-size: 0.8rem; border: 1px solid var(--border); }
.log-line { margin-bottom: 4px; border-bottom: 1px solid #1e293b; padding-bottom: 2px; }
.log-label { font-weight: bold; margin-right: 8px; display: inline-block; width: 50px; }
.log-label.info { color: #3b82f6; }
.log-label.warning { color: #f59e0b; }
.log-label.error { color: #ef4444; }
.log-msg { color: #cbd5e1; }

/* Settings */
.settings-card { padding: 25px; display: flex; flex-direction: column; gap: 15px; margin-bottom: 20px; }
.field { display: flex; flex-direction: column; gap: 8px; }
.field label { font-size: 0.9rem; color: #94a3b8; font-weight: 600; }
.field input { background: rgba(0,0,0,0.3); border: 1px solid var(--border); padding: 12px; border-radius: 10px; color: white; outline: none; }
.field input:focus { border-color: var(--accent); }
.btn-update { background: var(--accent); color: white; padding: 12px; border: none; border-radius: 10px; cursor: pointer; font-weight: bold; }

.feature-list { display: flex; flex-direction: column; gap: 12px; }
.feature-item { display: flex; justify-content: space-between; align-items: center; padding: 20px; }
.f-text h3 { margin: 0 0 4px 0; font-size: 1rem; }
.f-text p { margin: 0; font-size: 0.8rem; color: #94a3b8; }

/* Switch */
.switch { position: relative; width: 44px; height: 24px; }
.switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background: #334155; transition: .4s; border-radius: 34px; }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background: white; transition: .4s; border-radius: 50%; }
input:checked + .slider { background: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); }
</style>