<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.network.title') }}</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'network')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> {{ t('settings.network.resetBtn') }}
      </button>
    </div>

    <div class="glass-card setting-group scrollable">
      <div class="setting-item col-item" style="padding-bottom: 0; align-items: flex-start;">
        <h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">{{ t('settings.network.basicGroup') }}</h3>
      </div>
      <div class="divider" style="margin-top: 10px;"></div>
      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.ipv6') }}</h4>
          <p>{{ t('settings.network.ipv6Desc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.ipv6" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.allowLan') }}</h4>
          <p>{{ t('settings.network.allowLanDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.allowLan" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.unifiedDelay') }}</h4>
          <p>{{ t('settings.network.unifiedDelayDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.unifiedDelay" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.tcpConcurrent') }}</h4>
          <p>{{ t('settings.network.tcpConcurrentDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.tcpConcurrent" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.tcpKeepAlive') }}</h4>
          <p>{{ t('settings.network.tcpKeepAliveDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.tcpKeepAlive" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>

      <Transition name="dropdown">
        <div v-if="netConfig.tcpKeepAlive" class="tcp-keep-alive-sub-items">
          <div class="divider"></div>
          <div class="setting-item">
            <div class="info">
              <h4>{{ t('settings.network.keepAliveInterval') }}</h4>
              <p>{{ t('settings.network.keepAliveIntervalDesc') }}</p>
            </div>
            <div class="input-with-unit">
              <ModernNumberInput 
                v-model="netConfig.tcpKeepAliveInterval" 
                :min="1" 
                :max="3600" 
                @change="saveNet" 
                :disabled="loading"
              />
              <span class="unit">s</span>
            </div>
          </div>
        </div>
      </Transition>

      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info">
          <h4>{{ t('settings.network.delayTestUrl') }}</h4>
          <p>{{ t('settings.network.delayTestUrlDesc') }}</p>
        </div>
        <input 
          type="text" 
          class="modern-input" 
          style="text-align: left; width: 100%; margin-top: 12px; font-size: 0.95rem; padding: 12px 16px;" 
          v-model="netConfig.testUrl" 
          @blur="saveNet" 
          :disabled="loading"
          placeholder="http://www.gstatic.com/generate_204" 
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info">
          <h4>{{ t('settings.network.hosts') }}</h4>
          <p>{{ t('settings.network.hostsDesc') }}</p>
        </div>
        <div class="hosts-input-container">
          <textarea
            class="modern-textarea"
            v-model="netConfig.hosts"
            @blur="saveNet"
            :disabled="loading"
            rows="6"
            :placeholder="t('settings.network.hostsPlaceholder')"
            style="margin-top: 10px; font-family: var(--font-mono); font-size: 0.85rem; width: 100%;"
          ></textarea>
          <div v-show="hostsError" class="validation-error">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="warn-icon" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
            <span>{{ hostsError }}</span>
          </div>
        </div>
      </div>

      <!-- 端口设置 -->
      <div class="setting-item col-item section-group-header">
        <h3 class="section-group-title">{{ t('settings.network.portGroup') }}</h3>
        <p class="section-group-sub">{{ t('settings.network.portGroupDesc') }}</p>
      </div>
      <div class="divider" style="margin-top: 10px;"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.mixedPort') }}</h4>
          <p>{{ t('settings.network.mixedPortDesc') }}</p>
        </div>
        <ModernNumberInput
          v-model="netConfig.mixedPort"
          :min="1"
          :max="65535"
          @change="saveNet"
          :disabled="loading"
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.socksPort') }}</h4>
          <p>{{ t('settings.network.socksPortDesc') }}</p>
        </div>
        <ModernNumberInput
          v-model="netConfig.socksPort"
          :min="0"
          :max="65535"
          @change="saveNet"
          :disabled="loading"
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.httpPort') }}</h4>
          <p>{{ t('settings.network.httpPortDesc') }}</p>
        </div>
        <ModernNumberInput
          v-model="netConfig.port"
          :min="0"
          :max="65535"
          @change="saveNet"
          :disabled="loading"
        />
      </div>

      <!-- 外部控制器 -->
      <div class="setting-item col-item section-group-header">
        <h3 class="section-group-title">{{ t('settings.network.extControllerGroup') }}</h3>
        <p class="section-group-sub">{{ t('settings.network.extControllerGroupDesc') }}</p>
      </div>
      <div class="divider" style="margin-top: 10px;"></div>

      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.network.customExtController') }}</h4>
          <p>{{ t('settings.network.customExtControllerDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="netConfig.externalControllerEnabled" @change="saveNet" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info">
          <h4>{{ t('settings.network.extControllerAddress') }}</h4>
          <p>{{ t('settings.network.extControllerAddressDesc') }}</p>
        </div>
        <input
          type="text"
          class="modern-input"
          style="text-align: left; width: 100%; margin-top: 12px; font-size: 0.95rem; padding: 12px 16px;"
          v-model="netConfig.externalController"
          @blur="saveNet"
          :disabled="loading || !netConfig.externalControllerEnabled"
          placeholder="127.0.0.1:9090"
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info">
          <h4>{{ t('settings.network.extControllerSecret') }}</h4>
          <p>{{ t('settings.network.extControllerSecretDesc') }}</p>
        </div>
        <input
          type="text"
          class="modern-input"
          style="text-align: left; width: 100%; margin-top: 12px; font-size: 0.95rem; padding: 12px 16px;"
          v-model="netConfig.externalControllerSecret"
          @blur="saveNet"
          :disabled="loading || !netConfig.externalControllerEnabled"
          :placeholder="t('settings.network.secretPlaceholder')"
          autocomplete="off"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { ICONS } from '../utils/icons';
import ModernNumberInput from './ModernNumberInput.vue';
import { t } from '../locales';

const props = defineProps<{ resetKey: number }>();

defineEmits<{ navigate: [view: 'main']; reset: [module: 'network'] }>();

const loading = ref(true);
const netConfig = ref({
  ipv6: false,
  allowLan: false,
  externalController: '127.0.0.1:9090',
  externalControllerEnabled: false,
  externalControllerSecret: '',
  unifiedDelay: true,
  tcpConcurrent: true,
  tcpKeepAlive: true,
  tcpKeepAliveInterval: 15,
  testUrl: 'http://www.gstatic.com/generate_204',
  hosts: '',
  mixedPort: 7890,
  socksPort: 0,
  port: 0,
});

const hostsError = ref('');

// 👇 新增：校验 Hosts 是否符合 YAML 字典基础格式
const validateHosts = (val: string) => {
  if (!val || val.trim() === '') {
    hostsError.value = ''; // 为空是合法的（代表不配置）
    return true;
  }

  const lines = val.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    
    // 跳过空行和以 # 开头的注释行
    if (line === '' || line.startsWith('#')) continue;

    // 正则解析：必须是 "键: 值" 的形式 (至少包含一个冒号，且冒号后面要有内容)
    if (!/^[^:]+:\s*.+$/.test(line)) {
      hostsError.value = t('settings.network.hostsLineError', { line: i + 1 });
      return false;
    }
  }
  
  hostsError.value = ''; // 校验通过，清空错误
  return true;
};

// 👇 实时监听用户的输入
watch(() => netConfig.value.hosts, (newVal) => {
  validateHosts(newVal || '');
});

const loadData = async () => {
  loading.value = true;
  try {
    const netConf = await (API.GetNetworkConfig as any)();
    if (netConf) netConfig.value = netConf;
  } catch (e) {
    console.error('加载配置失败', e);
  } finally {
    loading.value = false;
  }
};

const saveNet = async () => {
  if (loading.value) return;
  try {
    await (API.SaveNetworkConfig as any)(netConfig.value);
  } catch (e) {
    console.error('网络配置保存失败', e);
  }
};

onMounted(() => { void loadData(); });
watch(() => props.resetKey, () => { void loadData(); });
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

.section-group-header {
  padding-top: 24px !important;
  padding-bottom: 0 !important;
  align-items: flex-start !important;
  gap: 4px !important;
}
.section-group-title {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 600;
  color: var(--text-main);
}
.section-group-sub {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-sub);
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
.col-item { flex-direction: column; align-items: stretch; gap: 10px; padding: 16px 0; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

.modern-input, .modern-textarea { 
  background: var(--surface-hover); 
  border: none; 
  color: var(--text-main); 
  padding: 10px 14px; 
  border-radius: 8px; 
  outline: none; 
  font-size: 0.9rem;
}

.modern-input { text-align: right; }
.modern-textarea { resize: vertical; font-family: monospace; font-size: 0.85rem; line-height: 1.5; text-align: left; }
.modern-input:disabled, .modern-textarea:disabled { opacity: 0.5; cursor: not-allowed; }

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

/* ================================ */
/* 验证错误提示样式                    */
/* ================================ */
.hosts-input-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.validation-error {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #f59e0b; /* 橙色警告色 */
  font-size: 0.85rem;
  font-weight: 500;
  animation: fadeIn 0.3s ease;
  margin-top: 4px;
}

.warn-icon {
  width: 16px;
  height: 16px;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
