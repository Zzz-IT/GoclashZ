<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
// 引入后端绑定的方法
import {
  RunProxy, StopProxy, GetProxyStatus, GetProxyNodes,
  SelectProxy, SetConfigMode, GetInitialData,
  UpdateSubscription, StartAsyncTest, UpdateClashSettings, CheckTunEnv
} from '../wailsjs/go/main/App'
// 引入 Wails 事件监听工具
import { EventsOn } from '../wailsjs/runtime/runtime'

// ================== 状态定义 ==================
const currentTab = ref('dashboard') // dashboard | logs | settings
const isRunning = ref(false)
const statusMessage = ref('就绪')
const yamlError = ref('')

// 流量数据（由 monitor.go 推送）
const upSpeed = ref('0 B/s')
const downSpeed = ref('0 B/s')

// 设置项
const subUrl = ref(localStorage.getItem('sub_url') || '')
const customUA = ref(localStorage.getItem('custom_ua') || 'clash-verge/1.0')
const testUrl = ref(localStorage.getItem('test_url') || 'http://www.gstatic.com/generate_204')

// 节点与模式
const currentMode = ref('rule')
const proxyGroups = ref<any[]>([])
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

// ================== 生命周期与事件监听 ==================
onMounted(async () => {
  // 1. ✨ 监听后端流量推送 (来自 monitor.go)
  EventsOn("clash_traffic", (data: any) => {
    upSpeed.value = formatBytes(data.up) + '/s'
    downSpeed.value = formatBytes(data.down) + '/s'
  })

  // 2. 监听内核实时日志 (来自 api_extra.go)
  EventsOn("clash_log", (log: any) => {
    logs.value.unshift(log)
    if (logs.value.length > maxLogs) logs.value.pop()
  })

  // 3. 监听异步高并发测速结果 (来自 tester.go)
  EventsOn("node_delay_update", (data: any) => {
    nodeDelays[data.name] = data.delay
  })

  // 4. 初始化应用状态
  await initApp()
})

const initApp = async () => {
  // 获取离线配置预览
  const initData: any = await GetInitialData()
  if (initData.error) {
    yamlError.value = initData.error
  } else {
    yamlError.value = ''
    if (initData.mode) currentMode.value = initData.mode.toLowerCase()
    if (initData.groups) {
      proxyGroups.value = initData.groups.map((g: any) => ({ ...g, now: '等待启动...' }))
      selectedGroup.value = proxyGroups.value[0]?.name || ''
    }
  }

  // 同步内核运行状态
  isRunning.value = await GetProxyStatus()
  if (isRunning.value) {
    statusMessage.value = '✅ 代理运行中'
    await refreshOnlineNodes()
  }
}

// ================== 核心交互逻辑 ==================

const handleStart = async () => {
  if (yamlError.value) return alert("YAML 配置有误，无法启动")
  statusMessage.value = '正在启动内核...'

  // 记录离线时的节点选择，用于启动后同步
  const pendingSels = proxyGroups.value
    .filter(g => g.now !== '等待启动...')
    .map(g => ({ name: g.name, selected: g.now }))

  try {
    const res = await RunProxy() // 后端已整合 StartTrafficStream
    isRunning.value = true
    statusMessage.value = res

    // 启动后 1 秒执行热同步
    setTimeout(async () => {
      const mode = currentMode.value.charAt(0).toUpperCase() + currentMode.value.slice(1)
      await SetConfigMode(mode)
      for (const s of pendingSels) {
        await SelectProxy(s.name, s.selected)
      }
      await refreshOnlineNodes()
    }, 1000)
  } catch (e) {
    statusMessage.value = '启动失败'
  }
}

const handleStop = async () => {
  await StopProxy() // 后端已整合 StopTrafficStream
  isRunning.value = false
  statusMessage.value = '🛑 代理已停止'
  upSpeed.value = '0 B/s'; downSpeed.value = '0 B/s'
  await initApp()
}

const handleNodeSelect = async (groupName: string, nodeName: string) => {
  const group = proxyGroups.value.find(g => g.name === groupName)
  if (!isRunning.value) {
    if (group) group.now = nodeName // 离线状态下仅 UI 记忆
    return
  }
  try {
    await SelectProxy(groupName, nodeName)
    await refreshOnlineNodes()
  } catch (e) { alert("切换失败") }
}

const runTest = async () => {
  if (isTesting.value) return
  isTesting.value = true
  await StartAsyncTest(selectedGroup.value)
  setTimeout(() => { isTesting.value = false }, 2000)
}

const handleTunToggle = async () => {
  if (features.tun.enable) {
    const env = await CheckTunEnv()
    if (!env.isAdmin) {
      alert("❌ 开启失败：需要管理员权限，请右键选择“以管理员身份运行”")
      features.tun.enable = false; return
    }
    if (!env.hasWintun) {
      alert("❌ 开启失败：缺少 wintun.dll，请检查 core/bin 目录")
      features.tun.enable = false; return
    }
  }

  const payload = { "tun": { "enable": features.tun.enable, "stack": "system", "auto-route": true } }
  if (isRunning.value) {
    await UpdateClashSettings(payload)
  }
}

const toggleFeature = async (key: string, value: any) => {
  const payload: any = {}
  payload[key] = value
  if (isRunning.value) await UpdateClashSettings(payload)
}

// ================== 辅助工具 ==================
const refreshOnlineNodes = async () => {
  const nodes = await GetProxyNodes()
  if (nodes) proxyGroups.value = nodes
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024; const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const handleUpdateSub = async () => {
  statusMessage.value = '正在下载订阅...'
  const res = await UpdateSubscription(subUrl.value, customUA.value)
  alert(res)
  localStorage.setItem('sub_url', subUrl.value)
  localStorage.setItem('custom_ua', customUA.value)
  await initApp()
}
</script>

<template>
  <div class="app-container">
    <aside class="sidebar">
      <div class="logo">GoclashZ</div>
      <nav>
        <button :class="{ active: currentTab === 'dashboard' }" @click="currentTab = 'dashboard'">📊 仪表盘</button>
        <button :class="{ active: currentTab === 'logs' }" @click="currentTab = 'logs'">📜 实时日志</button>
        <button :class="{ active: currentTab === 'settings' }" @click="currentTab = 'settings'">⚙️ 设置项</button>
      </nav>
      <div class="version">v1.0.0</div>
    </aside>

    <main class="main-content">

      <div v-if="currentTab === 'dashboard'" class="animate-in">
        <header class="header">
          <div class="mode-selector glass">
            <button :class="{ active: currentMode === 'rule' }" @click="currentMode = 'rule'; isRunning && SetConfigMode('Rule')">规则</button>
            <button :class="{ active: currentMode === 'global' }" @click="currentMode = 'global'; isRunning && SetConfigMode('Global')">全局</button>
            <button :class="{ active: currentMode === 'direct' }" @click="currentMode = 'direct'; isRunning && SetConfigMode('Direct')">直连</button>
          </div>
          <div class="status-box">
            <span class="dot" :class="{ active: isRunning }"></span>
            <span class="msg">{{ statusMessage }}</span>
          </div>
          <div class="actions">
            <button class="btn-test" @click="runTest" :disabled="isTesting">{{ isTesting ? '探测中' : '⚡ 测速' }}</button>
            <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动</button>
            <button v-else class="btn-stop" @click="handleStop">■ 停止</button>
          </div>
        </header>

        <section class="traffic-card glass">
          <div class="t-item">
             <span class="label">上传速率</span>
             <span class="value up">{{ upSpeed }}</span>
          </div>
          <div class="divider"></div>
          <div class="t-item">
             <span class="label">下载速率</span>
             <span class="value down">{{ downSpeed }}</span>
          </div>
        </section>

        <section v-if="proxyGroups.length > 0">
           <div class="group-tabs">
             <button v-for="g in proxyGroups" :key="g.name" :class="{ active: selectedGroup === g.name }" @click="selectedGroup = g.name">
               {{ g.name }}
             </button>
           </div>
           <div class="node-container glass">
              <div v-for="g in proxyGroups" :key="g.name" v-show="selectedGroup === g.name">
                 <div class="node-grid">
                    <button v-for="n in g.proxies" :key="n" :class="{ active: g.now === n }" @click="handleNodeSelect(g.name, n)" class="node-btn">
                       <span class="n-name">{{ n }}</span>
                       <span v-if="nodeDelays[n]" :class="['n-delay', { slow: nodeDelays[n] > 500, err: nodeDelays[n] === -1 }]">
                         {{ nodeDelays[n] === -1 ? 'Fail' : nodeDelays[n] + 'ms' }}
                       </span>
                    </button>
                 </div>
              </div>
           </div>
        </section>
      </div>

      <div v-if="currentTab === 'logs'" class="animate-in">
        <div class="view-header">
           <h2>内核实时输出</h2>
           <button class="btn-clear" @click="logs = []">🗑️ 清空日志</button>
        </div>
        <div class="log-viewer">
           <div v-for="(log, i) in logs" :key="i" class="log-line">
              <span :class="['log-label', log.type.toLowerCase()]">[{{ log.type.toUpperCase() }}]</span>
              <span class="log-msg">{{ log.payload }}</span>
           </div>
           <div v-if="logs.length === 0" class="empty">等待数据流入...</div>
        </div>
      </div>

      <div v-if="currentTab === 'settings'" class="animate-in">
         <h2 class="view-header">订阅设置</h2>
         <div class="card glass settings-card">
            <div class="input-group"><label>订阅 URL</label><input v-model="subUrl" placeholder="https://..." /></div>
            <div class="input-group"><label>User-Agent</label><input v-model="customUA" /></div>
            <button class="btn-update" @click="handleUpdateSub">🚀 立即更新</button>
         </div>

         <h2 class="view-header" style="margin-top: 30px;">Clash 高级特性</h2>
         <div class="feature-list">
            <div class="feature-item glass">
               <div class="f-text"><h3>允许局域网 (LAN)</h3><p>允许其他设备连接你的代理端口</p></div>
               <label class="switch"><input type="checkbox" v-model="features['allow-lan']" @change="toggleFeature('allow-lan', features['allow-lan'])"><span class="slider"></span></label>
            </div>
            <div class="feature-item glass">
               <div class="f-text"><h3>IPv6 支持</h3><p>是否接管 IPv6 流量</p></div>
               <label class="switch"><input type="checkbox" v-model="features['ipv6']" @change="toggleFeature('ipv6', features['ipv6'])"><span class="slider"></span></label>
            </div>
            <div class="feature-item glass highlight">
               <div class="f-text"><h3>TUN 模式</h3><p>真正的全站流量接管 (需管理员权限)</p></div>
               <label class="switch"><input type="checkbox" v-model="features.tun.enable" @change="handleTunToggle"><span class="slider"></span></label>
            </div>
         </div>
      </div>

    </main>
  </div>
</template>

<style>
/* 终极版全套 CSS 样式 */
:root { --bg: #0f172a; --panel: rgba(30, 41, 59, 0.7); --border: rgba(255,255,255,0.1); --accent: #3b82f6; --success: #10b981; --danger: #ef4444; }
body { margin: 0; background: var(--bg); color: #f8fafc; font-family: 'Inter', system-ui, sans-serif; overflow: hidden; }
.app-container { display: flex; height: 100vh; }

/* Sidebar */
.sidebar { width: 220px; background: rgba(15,23,42,0.9); border-right: 1px solid var(--border); padding: 30px 15px; display: flex; flex-direction: column; }
.logo { font-size: 1.5rem; font-weight: 900; text-align: center; margin-bottom: 40px; color: var(--accent); letter-spacing: 2px; }
.sidebar nav button { width: 100%; padding: 14px 18px; margin-bottom: 10px; background: transparent; border: none; color: #94a3b8; text-align: left; cursor: pointer; border-radius: 12px; transition: 0.3s; font-weight: 600; font-size: 0.95rem; }
.sidebar nav button:hover { background: rgba(255,255,255,0.05); color: white; }
.sidebar nav button.active { background: var(--accent); color: white; box-shadow: 0 4px 15px rgba(59,130,246,0.3); }
.version { margin-top: auto; text-align: center; font-size: 0.7rem; color: #475569; }

/* Main Content */
.main-content { flex: 1; padding: 35px; overflow-y: auto; background: radial-gradient(circle at 80% 20%, #1e293b, transparent); }
.glass { background: var(--panel); backdrop-filter: blur(15px); border: 1px solid var(--border); border-radius: 18px; }
.animate-in { animation: fadeIn 0.4s ease-out; }
@keyframes fadeIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }

/* Header & Controls */
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 35px; }
.mode-selector { display: flex; padding: 5px; border-radius: 14px; }
.mode-selector button { padding: 8px 20px; background: transparent; border: none; color: #94a3b8; cursor: pointer; border-radius: 10px; font-weight: 700; transition: 0.2s; font-size: 0.9rem; }
.mode-selector button.active { background: var(--accent); color: white; }
.status-box { display: flex; align-items: center; gap: 12px; font-weight: 600; background: rgba(0,0,0,0.2); padding: 8px 16px; border-radius: 10px; }
.dot { width: 10px; height: 10px; background: var(--danger); border-radius: 50%; box-shadow: 0 0 10px var(--danger); }
.dot.active { background: var(--success); box-shadow: 0 0 10px var(--success); }

.actions button { padding: 12px 24px; border-radius: 12px; border: none; cursor: pointer; font-weight: 800; transition: 0.2s; }
.btn-start { background-color: var(--success); color: white; }
.btn-stop { background-color: var(--danger); color: white; }
.btn-test { background: #6366f1; color: white; margin-right: 12px; }
.btn-test:disabled { opacity: 0.5; cursor: wait; }

/* Traffic Card */
.traffic-card { display: flex; justify-content: space-around; padding: 25px; margin-bottom: 35px; }
.t-item { display: flex; flex-direction: column; align-items: center; gap: 6px; }
.label { font-size: 0.8rem; color: #94a3b8; text-transform: uppercase; letter-spacing: 1px; }
.value { font-size: 1.8rem; font-weight: 800; font-family: 'JetBrains Mono', monospace; }
.up { color: #f59e0b; } .down { color: #3b82f6; }
.divider { width: 1px; height: 45px; background: var(--border); }

/* Node UI */
.group-tabs { display: flex; gap: 12px; margin-bottom: 20px; overflow-x: auto; padding-bottom: 8px; }
.group-tabs button { padding: 10px 20px; background: rgba(255,255,255,0.03); border: 1px solid var(--border); border-radius: 10px; color: #94a3b8; cursor: pointer; white-space: nowrap; font-size: 0.9rem; transition: 0.2s; }
.group-tabs button.active { background: var(--accent); color: white; border-color: var(--accent); }

.node-container { padding: 25px; min-height: 250px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 15px; }
.node-btn { background: rgba(0,0,0,0.25); border: 1px solid var(--border); padding: 16px 12px; border-radius: 12px; color: #94a3b8; cursor: pointer; display: flex; flex-direction: column; align-items: center; gap: 6px; transition: 0.2s; border-left: 4px solid transparent; }
.node-btn:hover { background: rgba(255,255,255,0.06); color: white; transform: translateY(-2px); }
.node-btn.active { border-color: var(--success); color: var(--success); background: rgba(16, 185, 129, 0.08); font-weight: 800; }
.n-name { font-size: 0.9rem; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; }
.n-delay { font-size: 0.75rem; color: var(--success); font-weight: 900; background: rgba(16, 185, 129, 0.1); padding: 2px 8px; border-radius: 6px; }
.n-delay.slow { color: #f59e0b; background: rgba(245, 158, 11, 0.1); }
.n-delay.err { color: var(--danger); background: rgba(239, 68, 68, 0.1); }

/* Logs View */
.view-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 25px; }
.log-viewer { height: 72vh; background: #020617; border-radius: 15px; padding: 20px; overflow-y: auto; font-family: 'Fira Code', 'Consolas', monospace; font-size: 0.8rem; border: 1px solid var(--border); box-shadow: inset 0 0 20px rgba(0,0,0,0.5); }
.log-line { margin-bottom: 6px; border-bottom: 1px solid #1e293b; padding-bottom: 4px; display: flex; gap: 10px; }
.log-label { font-weight: 900; min-width: 60px; text-align: center; border-radius: 4px; font-size: 0.7rem; padding: 2px 4px; }
.log-label.info { color: #3b82f6; background: rgba(59, 130, 246, 0.1); }
.log-label.warning { color: #f59e0b; background: rgba(245, 158, 11, 0.1); }
.log-label.error { color: #ef4444; background: rgba(239, 68, 68, 0.1); }
.log-msg { color: #cbd5e1; line-height: 1.5; word-break: break-all; }

/* Settings View */
.settings-card { padding: 30px; display: flex; flex-direction: column; gap: 20px; margin-bottom: 25px; }
.input-group { display: flex; flex-direction: column; gap: 10px; }
.input-group label { font-size: 0.85rem; color: #94a3b8; font-weight: 700; text-transform: uppercase; }
.input-group input { background: rgba(0,0,0,0.3); border: 1px solid var(--border); padding: 14px; border-radius: 12px; color: white; outline: none; font-size: 1rem; }
.input-group input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2); }
.btn-update { background: var(--accent); color: white; padding: 14px; border: none; border-radius: 12px; cursor: pointer; font-weight: 800; font-size: 1rem; }

.feature-list { display: flex; flex-direction: column; gap: 15px; }
.feature-item { display: flex; justify-content: space-between; align-items: center; padding: 22px 28px; }
.f-text h3 { margin: 0 0 6px 0; font-size: 1.1rem; }
.f-text p { margin: 0; font-size: 0.85rem; color: #94a3b8; }

/* Modern Switch */
.switch { position: relative; width: 50px; height: 26px; }
.switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background: #334155; transition: .4s; border-radius: 34px; }
.slider:before { position: absolute; content: ""; height: 20px; width: 20px; left: 3px; bottom: 3px; background: white; transition: .4s; border-radius: 50%; }
input:checked + .slider { background: var(--accent); }
input:checked + .slider:before { transform: translateX(24px); }
</style>