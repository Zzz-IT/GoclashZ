<script setup lang="ts">
import { ref, onMounted } from 'vue'
// 引入绑定的 Go 方法
import { RunProxy, StopProxy, GetProxyStatus } from '../wailsjs/go/main/App'

// 状态变量
const isRunning = ref(false)
const statusMessage = ref('初始化中...')

// 流量数据变量
const upSpeed = ref('0 B/s')
const downSpeed = ref('0 B/s')

// WebSocket 实例
let trafficWs: WebSocket | null = null

// 挂载时检查状态
onMounted(async () => {
  const status = await GetProxyStatus()
  isRunning.value = status
  if (status) {
    statusMessage.value = '✅ 代理运行中'
    startTrafficMonitor() // 如果启动着，就直接连上流量监控
  } else {
    statusMessage.value = '🛑 代理已停止'
  }
})

// 启动代理
const handleStart = async () => {
  statusMessage.value = '正在启动...'
  try {
    const result = await RunProxy()
    statusMessage.value = result
    isRunning.value = true
    startTrafficMonitor() // 启动后，开启流量监控
  } catch (error) {
    statusMessage.value = '发生错误: ' + error
  }
}

// 停止代理
const handleStop = async () => {
  try {
    const result = await StopProxy()
    statusMessage.value = result
    isRunning.value = false
    stopTrafficMonitor() // 停止后，掐断流量监控
  } catch (error) {
    statusMessage.value = '发生错误: ' + error
  }
}

// ============== 核心：流量监控功能 ==============

// 连接 Clash 的 WebSocket 获取实时流量
const startTrafficMonitor = () => {
  if (trafficWs) return

  // 连接我们在 config.yaml 里配置的 9090 API 端口
  trafficWs = new WebSocket('ws://127.0.0.1:9090/traffic')

  trafficWs.onmessage = (event) => {
    // 返回的数据格式是: {"up": 1024, "down": 2048} (单位是字节)
    const data = JSON.parse(event.data)
    upSpeed.value = formatBytes(data.up) + '/s'
    downSpeed.value = formatBytes(data.down) + '/s'
  }

  trafficWs.onerror = () => {
    console.error('流量接口连接失败')
  }
}

// 断开 WebSocket
const stopTrafficMonitor = () => {
  if (trafficWs) {
    trafficWs.close()
    trafficWs = null
  }
  upSpeed.value = '0 B/s'
  downSpeed.value = '0 B/s'
}

// 格式化字节为 KB/MB 工具函数
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
</script>

<template>
  <div class="container">
    <h1>🚀 GoclashZ DashBoard</h1>

    <div class="card">
      <div class="status-indicator">
        <span class="dot" :class="{ 'active': isRunning }"></span>
        <p class="status-text">{{ statusMessage }}</p>
      </div>

      <div class="traffic-board">
        <div class="traffic-item">
          <span class="label">↑ 上传速度</span>
          <span class="value up-color">{{ upSpeed }}</span>
        </div>
        <div class="divider"></div>
        <div class="traffic-item">
          <span class="label">↓ 下载速度</span>
          <span class="value down-color">{{ downSpeed }}</span>
        </div>
      </div>

      <div class="actions">
        <button v-if="!isRunning" class="btn-start" @click="handleStart">▶ 启动代理</button>
        <button v-else class="btn-stop" @click="handleStop">■ 停止代理</button>
      </div>
    </div>
  </div>
</template>

<style>
/* 更新了一版更有科技感的样式 */
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: #1e1e2e;
  color: #fff;
}

.container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
}

h1 {
  margin-bottom: 30px;
  font-weight: 800;
  letter-spacing: 1px;
}

.card {
  background: #282a36;
  padding: 40px;
  border-radius: 16px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  text-align: center;
  width: 350px;
}

.status-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-bottom: 30px;
}

.dot {
  width: 12px;
  height: 12px;
  background-color: #ef4444;
  border-radius: 50%;
  transition: all 0.3s;
}
.dot.active {
  background-color: #10b981;
  box-shadow: 0 0 10px #10b981;
}

.status-text {
  font-size: 1.1rem;
  font-weight: bold;
  margin: 0;
}

.traffic-board {
  display: flex;
  background: #1e1e2e;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 30px;
  justify-content: space-around;
  align-items: center;
}

.traffic-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.divider {
  width: 1px;
  height: 40px;
  background-color: #44475a;
}

.label {
  font-size: 0.85rem;
  color: #a1a1aa;
}

.value {
  font-size: 1.3rem;
  font-weight: bold;
  font-family: monospace;
}
.up-color { color: #f59e0b; }
.down-color { color: #3b82f6; }

button {
  width: 100%;
  padding: 14px;
  font-size: 1.1rem;
  font-weight: bold;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  transition: opacity 0.2s, transform 0.1s;
}

button:hover { opacity: 0.9; }
button:active { transform: scale(0.98); }

.btn-start { background-color: #10b981; color: white; }
.btn-stop { background-color: #ef4444; color: white; }
</style>