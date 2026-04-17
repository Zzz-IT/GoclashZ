<template>
  <div class="settings-container">

    <div v-if="view === 'main'" class="settings-page">
      <div class="glass-card setting-group">
        <h3>系统与网络</h3>

        <div class="setting-item">
          <div class="info">
            <h4>UWP 环回免除 (Loopback Exemption)</h4>
            <p>解决 Windows 10/11 应用商店、邮件等自带 UWP 软件在开启代理后无法联网的问题。</p>
          </div>
          <button class="action-btn" @click="fixUWP">🔧 一键修复</button>
        </div>

        <div class="setting-item clickable" @click="view = 'tun'">
          <div class="info">
            <h4>虚拟网卡设置 (TUN 模式)</h4>
            <p>配置底层驱动并接管系统全量流量，适合不支持代理的游戏或软件。</p>
          </div>
          <span class="arrow">➔</span>
        </div>
      </div>
    </div>

    <div v-else-if="view === 'tun'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
        </button>
        <h3>虚拟网卡配置</h3>
      </div>

      <div class="glass-card setting-group scrollable">

        <div class="setting-item">
          <div class="info"><h4>开启 TUN 模式</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="tunConfig.enable" @change="handleTunToggle">
            <span class="slider"></span>
          </label>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>网卡驱动安装</h4>
            <p class="status-msg">
              检测状态: <span :class="tunStatus.hasWintun ? 'green-text' : 'red-text'">{{ tunStatus.hasWintun ? 'wintun.dll 已就绪' : '缺失驱动文件' }}</span>
            </p>
          </div>
          <button class="action-btn" @click="installDriver" :disabled="isInstalling || tunStatus.hasWintun">
            {{ isInstalling ? '处理中...' : (tunStatus.hasWintun ? '已安装' : '立即安装') }}
          </button>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>堆栈 (Stack)</h4></div>
          <select class="modern-select" v-model="tunConfig.stack" @change="saveTun" :disabled="!tunStatus.hasWintun">
            <option value="gvisor">gVisor</option>
            <option value="mixed">Mixed</option>
            <option value="system">System</option>
            <option value="lwip">LWIP</option>
          </select>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>指定网卡名称 (Device)</h4></div>
          <input type="text" class="modern-input" v-model="tunConfig.device" placeholder="留空则自动" @blur="saveTun" :disabled="!tunStatus.hasWintun" />
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>自动设置路由 (Auto Route)</h4></div>
          <label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoRoute" @change="saveTun" :disabled="!tunStatus.hasWintun"><span class="slider"></span></label>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>自动包含接口 (Auto Detect Interface)</h4></div>
          <label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoDetect" @change="saveTun" :disabled="!tunStatus.hasWintun"><span class="slider"></span></label>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>DNS 劫持 (DNS Hijack)</h4></div>
          <input type="text" class="modern-input" :value="tunConfig.dnsHijack.join(', ')" @blur="updateDnsHijack" placeholder="如 any:53" :disabled="!tunStatus.hasWintun" />
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>严格路由 (Strict Route)</h4></div>
          <label class="modern-switch"><input type="checkbox" v-model="tunConfig.strictRoute" @change="saveTun" :disabled="!tunStatus.hasWintun"><span class="slider"></span></label>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>最大传输单元 (MTU)</h4></div>
          <input type="number" class="modern-input num-input" v-model.number="tunConfig.mtu" @blur="saveTun" :disabled="!tunStatus.hasWintun" />
        </div>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import { FixUWPNetwork, CheckTunEnv, GetTunConfig, SaveTunConfig, InstallTunDriver } from '../../wailsjs/go/main/App';

// 接收来自 App.vue 的跳转参数
const props = defineProps({
  initialView: {
    type: String,
    default: 'main'
  }
});

// 唯一声明一次 view，避免 TS2451 错误
const view = ref(props.initialView);

// 监听从 App.vue 传来的变化，实现点击侧边栏和主页快捷按钮时自由切换
watch(() => props.initialView, (newVal) => {
  view.value = newVal;
});

const isInstalling = ref(false);

// 定义明确的数据类型，解决 Record<string, boolean> 匹配错误
const tunStatus = ref<Record<string, boolean>>({ hasWintun: false, isAdmin: false });

const tunConfig = ref({
  enable: false,
  stack: 'gvisor',
  device: '',
  autoRoute: true,
  autoDetect: true,
  dnsHijack: ['any:53'],
  strictRoute: true,
  mtu: 1500
});

const loadEnv = async () => {
  try {
    const status = await CheckTunEnv();
    tunStatus.value = status;
    const conf = await GetTunConfig();
    if (conf) tunConfig.value = conf;
  } catch (e) {
    console.error('加载失败', e);
  }
};

onMounted(() => { loadEnv(); });

const fixUWP = async () => {
  try {
    await FixUWPNetwork();
    alert('✅ UWP 环回限制已成功解除！');
  } catch (e) {
    alert('修复失败，请尝试右键以管理员身份运行本软件。\n错误信息: ' + e);
  }
};

// 拦截非法开启 TUN
const handleTunToggle = async (e: Event) => {
  if (tunConfig.value.enable && !tunStatus.value.hasWintun) {
    e.preventDefault();
    tunConfig.value.enable = false; // 强制拨回关闭状态
    alert('⚠️ 无法开启 TUN 模式：\n请先点击下方的“安装驱动”按钮下载并配置 wintun.dll。');
    return;
  }
  await saveTun();
};

const installDriver = async () => {
  isInstalling.value = true;
  try {
    await InstallTunDriver();
    await loadEnv();

    if (tunStatus.value.hasWintun) {
       alert('✅ 驱动安装成功，现在可以开启 TUN 模式了！');
    } else {
       alert('❌ 安装命令已执行，但系统仍未检测到 wintun.dll。\n请确认网络是否通畅或尝试以管理员身份运行程序。');
    }
  } catch (e) {
    alert('安装提示: ' + e);
  } finally {
    isInstalling.value = false;
  }
};

const saveTun = async () => {
  try {
    await SaveTunConfig(tunConfig.value);
  } catch (e) {
    console.error('保存失败', e);
  }
};

// 处理数组格式的字符串输入
const updateDnsHijack = (e: Event) => {
  const val = (e.target as HTMLInputElement).value;
  tunConfig.value.dnsHijack = val.split(',').map(s => s.trim()).filter(s => s);
  saveTun();
};
</script>

<style scoped>
.settings-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.settings-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  flex: 1;
}

.setting-group {
  padding: 20px 24px;
  margin-bottom: 20px;
}

.scrollable {
  overflow-y: auto;
  padding-right: 12px;
}

h3 { margin: 0 0 20px 0; color: var(--text-main); font-size: 1.1rem; border-bottom: 1px solid var(--glass-border); padding-bottom: 12px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 80%; }

.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }

.arrow { color: var(--text-sub); font-size: 1.2rem; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

/* 按钮与输入框 */
.action-btn { padding: 8px 16px; border-radius: 8px; border: none; background: rgba(79, 70, 229, 0.1); color: var(--accent); font-weight: bold; cursor: pointer; transition: 0.2s; white-space: nowrap;}
.action-btn:hover:not(:disabled) { background: var(--accent); color: white; }
.action-btn:disabled { opacity: 0.5; cursor: not-allowed; background: var(--surface-hover); color: var(--text-muted); }

.modern-input, .modern-select { background: var(--surface-hover); border: 1px solid var(--glass-border); color: var(--text-main); padding: 8px 12px; border-radius: 8px; outline: none; text-align: right; }
.modern-input:disabled, .modern-select:disabled { opacity: 0.5; cursor: not-allowed; }
.num-input { width: 80px; }

/* 现代开关 (CSS Switch) - 修复夜间模式白条 */
.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }
.slider {
  position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0;
  background-color: var(--glass-border);
  transition: .3s; border-radius: 24px;
}
.dark .slider { background-color: rgba(255, 255, 255, 0.2); }

.slider:before {
  position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px;
  background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);
}

input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); }

/* 子页面动画与样式 */
.slide-in { animation: slideIn 0.2s ease forwards; }
@keyframes slideIn { from { opacity: 0; transform: translateX(10px); } to { opacity: 1; transform: translateX(0); } }

.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: 1px solid var(--glass-border); color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }

.status-msg { margin-top: 4px; font-weight: 500; }
.green-text { color: #10b981; }
.red-text { color: #ef4444; }
</style>