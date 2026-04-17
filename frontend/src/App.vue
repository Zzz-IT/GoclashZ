<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
// 引入绑定的 Go 方法
import { RunProxy, StopProxy, GetProxyStatus, GetProxyNodes, SelectProxy, SetConfigMode, GetInitialData } from '../wailsjs/go/main/App'

// ================== 类型定义 ==================
interface ProxyNode {
  name: string
  type: string
  now: string
  proxies: string[]
}

// ================== 状态变量 ==================
const isRunning = ref(false)
const statusMessage = ref('初始化中...')
const yamlError = ref('')
const upSpeed = ref('0 B/s')
const downSpeed = ref('0 B/s')
let trafficWs: WebSocket | null = null

const currentMode = ref('rule') // 存储当前选中的模式 (Rule/Global/Direct)
const proxyGroups = ref<ProxyNode[]>([])
const selectedGroup = ref<string>('')

// ================== 生命周期 ==================
onMounted(async () => {
  // 1. 加载离线数据
  await loadOfflineData()

  // 2. 检测实时状态
  const status = await GetProxyStatus()
  isRunning.value = status
  if (status) {
    statusMessage.value = '✅ 代理运行中'
    startTrafficMonitor()
    await loadOnlineNodes()
  } else {
    statusMessage.value = '🛑 代理已停止'
  }
})

onUnmounted(() => {
  stopTrafficMonitor()
})

// ================== 数据加载逻辑 ==================

const loadOfflineData = async () => {
  try {
    const initData: any = await GetInitialData()
    if (initData.error) {
      yamlError.value = initData.error
      return
    }

    yamlError.value = ''
    // 离线时读取 yaml 默认模式
    if (initData.mode) currentMode.value = initData.mode.toLowerCase()

    if (initData.groups) {
      proxyGroups.value = initData.groups.map((g: any) => ({
        name: g.name,
        type: g.type,
        now: '等待启动...',
        proxies: g.proxies || []
      }))
      if (proxyGroups.value.length > 0 && !selectedGroup.value) {
        selectedGroup.value = proxyGroups.value[0].name
      }
    }
  } catch (e) {
    console.error('离线解析失败', e)
  }
}

const loadOnlineNodes = async () => {
  if (!isRunning.value) return
  try {
    const nodes = await GetProxyNodes()
    if (nodes && nodes.length > 0) {
      yamlError.value = ''
      proxyGroups.value = nodes as ProxyNode[]
    }
  } catch (err) {
    console.error('API 获取失败', err)
  }
}

// ================== 交互逻辑 ==================

// 1. 节点选择逻辑 (支持离线记忆)
const handleNodeSelect = async (groupName: string, nodeName: string) => {
  const targetGroup = proxyGroups.value.find(g => g.name === groupName)

  if (!isRunning.value) {
    // 离线：记录在内存里
    if (targetGroup) targetGroup.now = nodeName
    return
  }

  // 在线：真实切换
  try {
    await SelectProxy(groupName, nodeName)
    await loadOnlineNodes()
  } catch (err) {
    alert('切换失败: ' + err)
  }
}

// 2. 模式切换逻辑 (支持离线预览/记忆)
const changeMode = async (mode: string) => {
  // 更新 UI 高亮状态（无论是否启动都先更新 UI）
  currentMode.value = mode.toLowerCase()

  if (isRunning.value) {
    // 如果已经启动，则立即同步给后端
    try {
      await SetConfigMode(mode)
    } catch (e) {
      alert("模式切换失败")
    }
  }
}

// 3. 启动逻辑 (启动瞬间同步所有离线选择)
const handleStart = async () => {
  if (yamlError.value !== '') {
    alert("YAML 配置错误，无法启动！")
    return
  }

  statusMessage.value = '启动中...'
  try {
    // 【记忆点 A】: 提取预选节点
    const offlineSelections = proxyGroups.value
      .filter(g => g.now && g.now !== '等待启动...')
      .map(g => ({ name: g.name, selected: g.now }))

    // 【记忆点 B】: 提取当前 UI 选中的模式 (首字母大写)
    const pendingMode = currentMode.value.charAt(0).toUpperCase() + currentMode.value.slice(1)

    // 启动代理
    const result = await RunProxy()
    statusMessage.value = result
    isRunning.value = true
    startTrafficMonitor()

    // 🚀 核心同步步骤
    setTimeout(async () => {
      // 1. 同步运行模式 (Rule/Global/Direct)
      await SetConfigMode(pendingMode)

      // 2. 同步每一个预选节点
      for (const sel of offlineSelections) {
         try {
            await SelectProxy(sel.name, sel.selected)
         } catch(e) { console.error("同步节点失败", e) }
      }

      // 3. 刷新最终在线状态
      await loadOnlineNodes()
    }, 1000)

  } catch (error) {
    statusMessage.value = '启动失败: ' + error
  }
}

const handleStop = async () => {
  try {
    const result = await StopProxy()
    statusMessage.value = result
    isRunning.value = false
    stopTrafficMonitor()
    await loadOfflineData()
  } catch (error) {
    statusMessage.value = '停止失败: ' + error
  }
}

// ================== 工具函数 ==================
const startTrafficMonitor = () => {
  if (trafficWs) return
  trafficWs = new WebSocket('ws://127.0.0.1:9090/traffic')
  trafficWs.onmessage = (event) => {
    const data = JSON.parse(event.data)
    upSpeed.value = formatBytes(data.up) + '/s'
    downSpeed.value = formatBytes(data.down) + '/s'
  }
}

const stopTrafficMonitor = () => {
  if (trafficWs) { trafficWs.close(); trafficWs = null }
  upSpeed.value = '0 B/s'; downSpeed.value = '0 B/s'
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
</script>

<template>
  <div class="app-container">
    <aside class="sidebar">
      <div class="logo">GoclashZ</div>
      <nav>
        <a href="#" class="active">仪表盘</a>
        <a href="#">订阅管理</a>
      </nav>
    </aside>

    <main class="main-content">
      <header class="header">
        <div class="status-wrapper">
          <div class="mode-selector glass">
            <button :class="{active: currentMode === 'rule'}" @click="changeMode('Rule')">规则</button>
            <button :class="{active: currentMode === 'global'}" @click="changeMode('Global')">全局</button>
            <button :class="{active: currentMode === 'direct'}" @click="changeMode('Direct')">直连</button>
          </div>

          <div class="status-indicator">
            <span class="dot" :class="{ 'active': isRunning, 'error': yamlError !== '' }"></span>
            <p class="status-text">{{ yamlError !== '' ? '配置错误' : statusMessage }}</p>
          </div>
        </div>

        <div class="actions">
          <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动</button>
          <button v-else class="btn-stop" @click="handleStop">■ 停止</button>
        </div>
      </header>

      <div v-if="yamlError !== ''" class="error-banner glass">
        ⚠️ 配置文件语法有误：<br/>{{ yamlError }}
      </div>

      <section class="dashboard-grid">
        <div class="card traffic-card glass">
          <div class="traffic-item">
            <span class="icon">↑</span>
            <div class="info"><span class="label">上传</span><span class="value up">{{ upSpeed }}</span></div>
          </div>
          <div class="divider"></div>
          <div class="traffic-item">
            <span class="icon">↓</span>
            <div class="info"><span class="label">下载</span><span class="value down">{{ downSpeed }}</span></div>
          </div>
        </div>
      </section>

      <section v-if="proxyGroups.length > 0" class="proxy-section">
        <h2 class="section-title">线路选择 (启动前可预选)</h2>

        <div class="group-tabs">
          <button v-for="group in proxyGroups" :key="group.name"
            :class="['tab-btn', { active: selectedGroup === group.name }]"
            @click="selectedGroup = group.name">
            {{ group.name }}
          </button>
        </div>

        <div class="node-list-container glass">
          <div v-for="group in proxyGroups" :key="'content-'+group.name" v-show="selectedGroup === group.name">
            <div class="current-node-info">
              <span class="label">当前出站:</span>
              <span class="highlight">{{ group.now }}</span>
              <span class="type-badge">{{ group.type }}</span>
            </div>

            <div class="node-grid">
              <button
                v-for="node in group.proxies"
                :key="node"
                :class="['node-btn', { active: group.now === node }]"
                @click="handleNodeSelect(group.name, node)"
              >
                {{ node }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <div v-else-if="yamlError === ''" class="empty-state">
        <p>📭 暂无有效配置，请确保 core/bin/config.yaml 正确</p>
      </div>
    </main>
  </div>
</template>

<style>
/* 保持原样，无需改动样式 */
:root { --bg-color: #0f172a; --panel-bg: rgba(30, 41, 59, 0.7); --border-color: rgba(255, 255, 255, 0.1); --text-main: #f8fafc; --text-muted: #94a3b8; --accent: #3b82f6; --success: #10b981; --danger: #ef4444; }
body { margin: 0; font-family: system-ui, sans-serif; background-color: var(--bg-color); color: var(--text-main); background-image: radial-gradient(circle at top right, #1e1b4b, transparent 40%), radial-gradient(circle at bottom left, #064e3b, transparent 40%); background-attachment: fixed; }
.app-container { display: flex; height: 100vh; overflow: hidden; }
.sidebar { width: 200px; background: rgba(15, 23, 42, 0.8); border-right: 1px solid var(--border-color); padding: 20px 0; display: flex; flex-direction: column; }
.logo { font-size: 1.5rem; font-weight: 900; text-align: center; margin-bottom: 40px; background: linear-gradient(to right, #60a5fa, #a78bfa); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
nav a { display: block; padding: 12px 24px; color: var(--text-muted); text-decoration: none; font-weight: 500; }
nav a.active { color: var(--text-main); background: rgba(255, 255, 255, 0.05); border-right: 3px solid var(--accent); }
.main-content { flex: 1; padding: 30px 40px; overflow-y: auto; }
.glass { background: var(--panel-bg); backdrop-filter: blur(12px); border: 1px solid var(--border-color); border-radius: 16px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
.status-wrapper { display: flex; align-items: center; gap: 20px; }
.mode-selector { display: flex; padding: 4px; border-radius: 10px; }
.mode-selector button { padding: 6px 16px; background: transparent; color: var(--text-muted); border: none; border-radius: 8px; cursor: pointer; }
.mode-selector button.active { background: var(--accent); color: white; }
.status-indicator { display: flex; align-items: center; gap: 12px; }
.dot { width: 10px; height: 10px; background-color: var(--danger); border-radius: 50%; }
.dot.active { background-color: var(--success); box-shadow: 0 0 10px var(--success); }
.dot.error { background-color: #f59e0b; box-shadow: 0 0 10px #f59e0b; }
.status-text { font-weight: 600; margin: 0; }
.actions button { padding: 10px 24px; font-weight: 600; border: none; border-radius: 8px; cursor: pointer; }
.btn-start { background-color: var(--success); color: white; }
.btn-stop { background-color: var(--danger); color: white; }
.error-banner { background-color: rgba(239, 68, 68, 0.2); border-color: rgba(239, 68, 68, 0.5); color: #fca5a5; padding: 16px 20px; margin-bottom: 20px; font-family: monospace; line-height: 1.5; }
.traffic-card { display: flex; justify-content: space-around; padding: 24px; margin-bottom: 30px; }
.traffic-item { display: flex; align-items: center; gap: 16px; }
.icon { font-size: 1.5rem; color: var(--text-muted); background: rgba(255,255,255,0.05); width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; border-radius: 10px; }
.info { display: flex; flex-direction: column; }
.label { font-size: 0.85rem; color: var(--text-muted); margin-bottom: 4px; }
.value { font-size: 1.4rem; font-weight: 700; font-family: monospace; }
.up { color: #f59e0b; } .down { color: #3b82f6; }
.divider { width: 1px; background: var(--border-color); }
.section-title { font-size: 1.2rem; margin-bottom: 20px; font-weight: 600; }
.group-tabs { display: flex; gap: 10px; margin-bottom: 20px; overflow-x: auto; padding-bottom: 5px; }
.tab-btn { padding: 8px 16px; background: rgba(255,255,255,0.05); color: var(--text-muted); border: 1px solid var(--border-color); border-radius: 8px; cursor: pointer; }
.tab-btn.active { background: var(--accent); color: white; border-color: var(--accent); }
.node-list-container { padding: 24px; min-height: 150px; }
.current-node-info { margin-bottom: 20px; font-size: 1.1rem; }
.highlight { color: var(--success); font-weight: bold; margin: 0 10px; }
.type-badge { font-size: 0.75rem; background: rgba(255,255,255,0.1); padding: 2px 8px; border-radius: 12px; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; }
.node-btn { background: rgba(0, 0, 0, 0.2); border: 1px solid var(--border-color); color: var(--text-muted); padding: 12px 8px; border-radius: 8px; text-align: center; font-size: 0.9rem; cursor: pointer; transition: all 0.2s ease; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.node-btn:hover { background: rgba(255, 255, 255, 0.1); color: var(--text-main); }
.node-btn.active { border-color: var(--success); color: var(--success); font-weight: bold; }
</style>