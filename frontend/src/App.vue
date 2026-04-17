<template>
  <div class="app-shell" :class="{ dark: isDark }">
    <div class="drag-bar" style="--wails-draggable:drag">
      <div class="brand">🚀 GoclashZ</div>
      <div class="top-actions" style="--wails-draggable:none">
        <span class="traffic-mon">↑ {{ traffic.up }} / ↓ {{ traffic.down }}</span>
        <button @click="isDark = !isDark" class="icon-btn">{{ isDark ? '🌙' : '☀️' }}</button>
      </div>
    </div>

    <div class="main-layout">
      <aside class="sidebar glass-panel">
        <nav class="nav-list">
          <div v-for="item in menu" :key="item.id"
               :class="['nav-item', { active: currentTab === item.id }]"
               @click="currentTab = item.id">
            <span class="icon">{{ item.icon }}</span>
            <span>{{ item.label }}</span>
          </div>
        </nav>
        <div class="sidebar-footer">
          <div :class="['status-dot', { online: isRunning }]"></div>
          <span>{{ isRunning ? '内核运行中' : '服务已停止' }}</span>
        </div>
      </aside>

      <main class="content glass-panel">
        <header class="content-header">
          <h1>{{ activeMenuLabel }}</h1>
        </header>

        <div class="view-scroller">
          <div v-if="currentTab === 'home'" class="view-home">
            <div class="hero-card">
              <div class="hero-info">
                <h2>系统状态</h2>
                <p>Mixed Port: 7890 | Mode: {{ currentMode.toUpperCase() }}</p>
              </div>
              <button class="power-btn" :class="{ stop: isRunning }" @click="toggleProxy">
                {{ isRunning ? '停止代理' : '启动代理' }}
              </button>
            </div>

            <div class="card-grid">
              <div class="info-card">
                <h3>路由模式</h3>
                <div class="mode-group">
                  <button v-for="m in ['rule', 'global', 'direct']" :key="m"
                          :class="{ active: currentMode === m }" @click="changeMode(m)">
                    {{ m.toUpperCase() }}
                  </button>
                </div>
              </div>
              <div class="info-card">
                <h3>TUN 状态</h3>
                <p v-if="tunStatus.hasWintun" class="text-success">已安装 Wintun 驱动</p>
                <p v-else class="text-danger">未发现驱动</p>
                <button class="mini-btn" @click="checkTun">重新检查</button>
              </div>
            </div>
          </div>

          <div v-else-if="currentTab === 'proxies'" class="view-proxies">
            <div v-for="group in proxyGroups" :key="group.name" class="group-box">
              <h3>{{ group.name }}</h3>
              <div class="node-grid">
                <div v-for="node in group.proxies" :key="node.name"
                     :class="['node-item', { active: node.now === node.name }]"
                     @click="selectNode(group.name, node.name)">
                  <span class="n-name">{{ node.name }}</span>
                  <span class="n-delay">{{ getDelay(node.history) }}ms</span>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="currentTab === 'logs'" class="view-logs">
            <div class="log-console" ref="logBox">
              <div v-for="(log, i) in logLines" :key="i" :class="['log-line', log.type]">
                <span class="l-time">[{{ log.time }}]</span>
                <span class="l-type">{{ log.type.toUpperCase() }}</span>
                <span class="l-msg">{{ log.payload }}</span>
              </div>
            </div>
            <button class="mini-btn clear-btn" @click="logLines = []">清空日志</button>
          </div>

          <div v-else-if="currentTab === 'settings'" class="view-settings">
            <div class="setting-row">
              <div class="s-info">
                <h4>UWP 环回免除</h4>
                <p>修复 Windows 应用商店无法联网的问题</p>
              </div>
              <button class="action-btn" @click="fixUWP">一键修复</button>
            </div>
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, nextTick } from 'vue';
// 导入 Wails 绑定
import * as API from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const isDark = ref(false);
const currentTab = ref('home');
const isRunning = ref(false);
const traffic = ref({ up: '0 B/s', down: '0 B/s' });
const currentMode = ref('rule');
const proxyGroups = ref<any[]>([]);
const tunStatus = ref<any>({ hasWintun: false });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);

const menu = [
  { id: 'home', label: '控制台', icon: '🏠' },
  { id: 'proxies', label: '节点', icon: '⚡' },
  { id: 'logs', label: '日志', icon: '📄' },
  { id: 'settings', label: '设置', icon: '⚙️' }
];

const activeMenuLabel = computed(() => menu.find(m => m.id === currentTab.value)?.label);

// 核心逻辑：启动/停止
const toggleProxy = async () => {
  try {
    isRunning.value ? await API.StopProxy() : await API.RunProxy();
    isRunning.value = await API.GetProxyStatus();
  } catch (e) { alert(e); }
};

// 切换模式
const changeMode = async (mode: string) => {
  await API.SetConfigMode(mode);
  currentMode.value = mode;
};

// 切换节点
const selectNode = async (group: string, node: string) => {
  await API.SelectProxy(group, node);
  loadInitialData();
};

// UWP 修复
const fixUWP = async () => {
  try {
    await API.FixUWPNetwork();
    alert("✅ 修复成功");
  } catch (e) { alert(e); }
};

// 加载基础数据
const loadInitialData = async () => {
  const data: any = await API.GetInitialData();
  if (data) {
    currentMode.value = data.mode;
    proxyGroups.value = data.groups || [];
  }
};

const checkTun = async () => { tunStatus.value = await API.CheckTunEnv(); };
const getDelay = (history: any[]) => history?.length ? history[history.length-1].delay : '--';

onMounted(async () => {
  isRunning.value = await API.GetProxyStatus();
  loadInitialData();
  checkTun();

  // 监听流量数据
  EventsOn("traffic-data", (data: any) => { traffic.value = data; });

  // 监听日志输出
  API.StartStreamingLogs();
  EventsOn("log-message", (log: any) => {
    logLines.value.push({ ...log, time: new Date().toLocaleTimeString() });
    if (logLines.value.length > 300) logLines.value.shift();
    nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight; });
  });

  // 监听内核退出
  EventsOn("clash-exited", () => { isRunning.value = false; });
});
</script>

<style scoped>
.app-shell { display: flex; flex-direction: column; height: 100vh; color: var(--text-main); }
.drag-bar { height: 40px; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; font-weight: bold; font-size: 0.8rem; }
.traffic-mon { font-family: monospace; color: var(--accent); margin-right: 15px; }

.main-layout { display: flex; flex: 1; padding: 0 15px 15px 15px; gap: 15px; overflow: hidden; }

/* 侧边栏 */
.sidebar { width: 200px; border-radius: 18px; display: flex; flex-direction: column; padding: 15px 10px; }
.nav-item { display: flex; align-items: center; gap: 12px; padding: 12px; margin-bottom: 5px; border-radius: 12px; cursor: pointer; transition: 0.2s; color: var(--text-sub); }
.nav-item:hover { background: rgba(255,255,255,0.15); }
.nav-item.active { background: var(--accent); color: white; box-shadow: 0 4px 10px rgba(92, 99, 237, 0.3); }

/* 内容区 */
.content { flex: 1; border-radius: 18px; display: flex; flex-direction: column; padding: 20px; overflow: hidden; }
.view-scroller { flex: 1; overflow-y: auto; padding-right: 5px; }

/* 仪表盘 */
.hero-card { background: var(--accent); color: white; padding: 25px; border-radius: 20px; display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.power-btn { padding: 12px 25px; border-radius: 12px; border: none; background: white; color: var(--accent); font-weight: 800; cursor: pointer; }
.power-btn.stop { background: var(--danger); color: white; }

.card-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 15px; }
.info-card { background: rgba(0,0,0,0.03); padding: 15px; border-radius: 15px; }
.dark .info-card { background: rgba(255,255,255,0.03); }

/* 节点列表 */
.group-box h3 { margin: 15px 0 10px; font-size: 1rem; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 10px; }
.node-item { background: rgba(0,0,0,0.03); padding: 10px 15px; border-radius: 10px; display: flex; justify-content: space-between; cursor: pointer; font-size: 0.9rem; }
.node-item.active { border: 1px solid var(--accent); color: var(--accent); background: rgba(92,99,237,0.1); }

/* 日志 */
.log-console { background: rgba(0,0,0,0.8); color: #adff2f; font-family: Consolas, monospace; padding: 15px; border-radius: 10px; height: 400px; overflow-y: auto; font-size: 0.8rem; }
.log-line.error { color: #ff6b6b; }
.log-line.warning { color: #ffd93d; }

.icon-btn { background: none; border: none; cursor: pointer; font-size: 1.2rem; color: var(--text-main); }
.mini-btn { margin-top: 10px; padding: 5px 12px; border-radius: 6px; border: 1px solid var(--accent); background: none; color: var(--accent); cursor: pointer; }
.text-success { color: var(--success); }
.text-danger { color: var(--danger); }
</style>