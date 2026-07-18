<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>应用行为设置</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'behavior')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> 重置
      </button>
    </div>

    <div class="glass-card setting-group scrollable">
      <div class="setting-item">
        <div class="info">
          <h4>静默启动</h4>
          <p>启动时直接进入系统托盘，不自动显示主界面。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.silentStart" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>关闭面板时隐藏到托盘</h4>
          <p>点击右上角关闭按钮时，程序将继续在后台运行。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.closeToTray" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>开机自启</h4>
          <p>登录 Windows 时自动启动 GoclashZ。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.startupWithOS" @change="$emit('startup-change')">
          <span class="slider"></span>
        </label>
      </div>

      <!-- 👇 新增：自启模式（当开机自启开启时才显示，附带动画） -->
      <Transition name="dropdown">
        <div v-if="behavior.startupWithOS" class="delay-retention-sub-items">
          <div class="divider"></div>

          <div class="setting-item">
            <div class="info">
              <h4>启动后恢复代理状态</h4>
              <p>开机自启后，自动恢复退出前启用的系统代理或 TUN 模式。</p>
            </div>
            <label class="modern-switch">
              <input type="checkbox" v-model="behavior.restoreOnStartup" @change="$emit('save')">
              <span class="slider"></span>
            </label>
          </div>

        </div>
      </Transition>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>自动延迟测速</h4>
          <p>启用后，将按设定的时间间隔在后台自动更新节点延迟。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.autoDelayTest" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>

      <Transition name="dropdown">
        <div v-if="behavior.autoDelayTest" class="delay-retention-sub-items">
          <div class="divider"></div>
          <div class="setting-item">
            <div class="info">
              <h4>测速间隔</h4>
            </div>
            <div class="input-with-unit">
              <ModernNumberInput
                v-model="behavior.autoDelayTestInterval"
                :min="1"
                :max="1440"
                @change="$emit('save')"
              />
              <span class="unit">min</span>
            </div>
          </div>
        </div>
      </Transition>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>显色彩色延迟数字</h4>
          <p>启用后，节点延迟将以绿黄红三色显示，替代默认的黑白深浅风格。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.colorDelay" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>延迟结果保留</h4>
          <p>开启后将缓存测速结果，可选择定时清空或长时间保留。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.delayRetention" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>

      <Transition name="dropdown">
        <div v-if="behavior.delayRetention" class="delay-retention-sub-items">
          <div class="divider"></div>
          <div class="setting-item">
            <div class="info">
              <h4>保留时间</h4>
            </div>
            <ModernSelect
              v-model="behavior.delayRetentionTime"
              :options="[
                { label: '5 秒', value: '5' },
                { label: '10 秒', value: '10' },
                { label: '30 秒', value: '30' },
                { label: '长时间', value: 'long' }
              ]"
              @change="$emit('save')"
            />
          </div>
        </div>
      </Transition>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>内核日志等级</h4>
          <p>调整核心输出的日志详细程度。如遇到问题无法排查，可改为调试。</p>
        </div>
        <ModernSelect
          v-model="behavior.logLevel"
          :options="logLevelOptions"
          @change="$emit('save')"
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>软件日志等级</h4>
          <p>调整主程序本身的日志输出等级。此设置即时生效，对实时日志页面起过滤作用。</p>
        </div>
        <ModernSelect
          v-model="behavior.appLogLevel"
          :options="appLogLevelOptions"
          @change="$emit('save')"
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>隐藏日志入口</h4>
          <p>隐藏侧边栏中的日志页面入口；后台仍会保留最近日志用于故障诊断。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.hideLogs" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>仅统计代理流量</h4>
          <p>开启后仪表盘流量图将只计算经由代理节点的流量，排除直连 (DIRECT) 流量。</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="behavior.proxyTrafficOnly" @change="$emit('save')">
          <span class="slider"></span>
        </label>
      </div>

      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>订阅更新 User-Agent</h4>
          <p>自定义下载或更新订阅配置时的请求头，留空使用默认值。</p>
        </div>
        <input
          type="text"
          class="modern-input"
          style="width: 200px; text-align: center;"
          v-model="behavior.subUA"
          @blur="$emit('save')"
          placeholder="默认 UA"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue';
import { showAlert } from '../store';
import { ICONS } from '../utils/icons';
import ModernSelect from './ModernSelect.vue';
import ModernNumberInput from './ModernNumberInput.vue';

const props = defineProps<{
  behavior: Record<string, any>;
  resetKey: number;
}>();

const emit = defineEmits(['navigate', 'reset', 'save', 'startup-change']);

const logLevelOptions = [
  { label: '调试', value: 'debug' },
  { label: '信息', value: 'info' },
  { label: '警告', value: 'warn' },
  { label: '错误', value: 'error' },
  { label: '静默', value: 'silent' }
];

const appLogLevelOptions = [
  { label: '调试', value: 'debug' },
  { label: '信息', value: 'info' },
  { label: '警告', value: 'warn' },
  { label: '错误', value: 'error' }
];

// 🚀 核心：监听更新间隔时间，防止用户输入 0 或负数
watch(() => props.behavior.updateInterval, async (newVal) => {
  if (newVal !== undefined && newVal <= 0) {
    props.behavior.updateInterval = 1;

    // 👇 修复：只有在用户实际启用了定时更新的情况下，才弹出警告。
    // 如果是旧版本配置缺失导致的 0，则静默修复并保存，不打扰用户。
    if (props.behavior.autoUpdate && props.behavior.updateMethod === 'scheduled') {
      await showAlert("检查更新间隔不能小于 1 天。", "配置提示");
    }

    emit('save');
  }
});
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 100%;
  overflow: visible;
}
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.setting-group.scrollable {
  padding-bottom: 20px;
  overflow: visible;
  max-height: none;
}

h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-sub);
  max-width: 100%;
  line-height: 1.5;
}

.info { flex: 1; padding-right: 24px; min-width: 0; }

.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

.modern-input {
  background: var(--surface-hover);
  border: none;
  color: var(--text-main);
  padding: 10px 14px;
  border-radius: 8px;
  outline: none;
  font-size: 0.9rem;
  text-align: right;
}

.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }

.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);}
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--accent-fg); }

.sub-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 0 0 12px 0;
  background: transparent;
}
.sub-header.page-sticky-mask {
  --sticky-mask-bleed: 4px;
}
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }

.input-with-unit { display: flex; align-items: center; gap: 8px; }
.unit { font-size: 0.85rem; color: var(--text-sub); font-family: var(--font-mono); font-weight: 500; }

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sub-header.section-header h3 {
  flex: 1;
  margin-left: 12px;
}

.mini-btn-reset {
  height: 36px !important;
  padding: 0 14px !important;
  font-size: 0.85rem !important;
  border-radius: 8px !important;
}

.mini-btn-reset :deep(.btn-icon) svg {
  width: 16px;
  height: 16px;
}

.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.5s cubic-bezier(0.4, 0, 0.2, 1);
  max-height: 250px;
  overflow: hidden;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  max-height: 0;
  transform: translateY(-8px);
}
</style>
