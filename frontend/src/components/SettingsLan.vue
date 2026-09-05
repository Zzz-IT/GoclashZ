<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="19" y1="12" x2="5" y2="12"></line>
          <polyline points="12 19 5 12 12 5"></polyline>
        </svg>
      </button>
      <h3>{{ t('settings.lan.title') }}</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'network')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> {{ t('settings.lan.resetBtn') }}
      </button>
    </div>

    <div class="glass-card setting-group scrollable">
      <!-- 总开关 -->
      <div class="setting-item">
        <div class="info">
          <h4>{{ t('settings.lan.allowLan') }}</h4>
          <p>{{ t('settings.lan.allowLanDesc') }}</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="lanConfig.allowLan" @change="save" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>

      <div class="divider"></div>

        <!-- 认证配置 -->
        <div class="section-label">
          <span class="section-label-text">{{ t('settings.lan.authSection') }}</span>
          <span class="section-label-sub">{{ t('settings.lan.authSectionSub') }}</span>
        </div>

        <div class="setting-item col-item">
          <div class="info">
            <h4>{{ t('settings.lan.userLabel') }}</h4>
          </div>
          <input
            type="text"
            class="modern-input full-input"
            v-model="lanConfig.lanAuthUser"
            @blur="save"
            :disabled="loading"
            :placeholder="t('settings.lan.authPlaceholder')"
            autocomplete="off"
          />
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info">
            <h4>{{ t('settings.lan.passLabel') }}</h4>
          </div>
          <div class="password-wrap">
            <input
              :type="showPassword ? 'text' : 'password'"
              class="modern-input full-input"
              v-model="lanConfig.lanAuthPass"
              @blur="save"
              :disabled="loading"
              :placeholder="t('settings.lan.authPlaceholder')"
              autocomplete="new-password"
            />
            <button class="eye-btn" @click="showPassword = !showPassword" type="button" tabindex="-1">
              <svg v-if="!showPassword" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
                <circle cx="12" cy="12" r="3"></circle>
              </svg>
              <svg v-else viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"></path>
                <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"></path>
                <line x1="1" y1="1" x2="23" y2="23"></line>
              </svg>
            </button>
          </div>
        </div>

        <div class="divider"></div>

        <!-- 允许连接的 IP -->
        <div class="setting-item col-item">
          <div class="info">
            <h4>{{ t('settings.lan.allowedIPs') }}</h4>
            <p>{{ t('settings.lan.allowedIPsDesc') }}</p>
            <p class="example-hint">{{ t('settings.lan.exampleHint', { example: '192.168.1.0/24' }) }}</p>
          </div>
          <div class="textarea-wrap">
            <textarea
              class="modern-textarea"
              v-model="lanConfig.lanAllowedIPs"
              @blur="saveAndValidate('allowed')"
              @input="validateCIDR(lanConfig.lanAllowedIPs, 'allowed')"
              :disabled="loading"
              rows="4"
              placeholder="192.168.1.0/24"
            ></textarea>
            <div v-if="errors.allowed" class="validation-error">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="warn-icon" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <span>{{ errors.allowed }}</span>
            </div>
          </div>
        </div>
        <div class="divider"></div>

        <!-- 禁止连接的 IP -->
        <div class="setting-item col-item">
          <div class="info">
            <h4>{{ t('settings.lan.disallowedIPs') }}</h4>
            <p>{{ t('settings.lan.disallowedIPsDesc') }}</p>
            <p class="example-hint">{{ t('settings.lan.exampleHint', { example: '192.168.1.100/32' }) }}</p>
          </div>
          <div class="textarea-wrap">
            <textarea
              class="modern-textarea"
              v-model="lanConfig.lanDisallowedIPs"
              @blur="saveAndValidate('disallowed')"
              @input="validateCIDR(lanConfig.lanDisallowedIPs, 'disallowed')"
              :disabled="loading"
              rows="4"
              placeholder="192.168.1.100/32"
            ></textarea>
            <div v-if="errors.disallowed" class="validation-error">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="warn-icon" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <span>{{ errors.disallowed }}</span>
            </div>
          </div>
        </div>
        <div class="divider"></div>

        <!-- 跳过认证的 IP -->
        <div class="setting-item col-item">
          <div class="info">
            <h4>{{ t('settings.lan.skipAuthIPs') }}</h4>
            <p>{{ t('settings.lan.skipAuthIPsDesc') }}</p>
            <p class="example-hint">{{ t('settings.lan.exampleHint', { example: '127.0.0.1/8' }) }}</p>
          </div>
          <div class="textarea-wrap">
            <textarea
              class="modern-textarea"
              v-model="lanConfig.lanSkipAuthIPs"
              @blur="saveAndValidate('skipAuth')"
              @input="validateCIDR(lanConfig.lanSkipAuthIPs, 'skipAuth')"
              :disabled="loading"
              rows="4"
              placeholder="127.0.0.1/8"
            ></textarea>
            <div v-if="errors.skipAuth" class="validation-error">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="warn-icon" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <span>{{ errors.skipAuth }}</span>
            </div>
          </div>
        </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { ICONS } from '../utils/icons';
import { t } from '../locales';

const props = defineProps<{ resetKey: number }>();
defineEmits<{ navigate: [view: 'main']; reset: [module: 'network'] }>();

const loading = ref(true);
const showPassword = ref(false);

const lanConfig = reactive({
  allowLan: false,
  lanAuthUser: '',
  lanAuthPass: '',
  lanAllowedIPs: '',
  lanDisallowedIPs: '',
  lanSkipAuthIPs: '',
});

const errors = reactive<Record<string, string>>({
  allowed: '',
  disallowed: '',
  skipAuth: '',
});

// IPv4/CIDR 或 IPv6/CIDR 基础验证
const CIDR_RE = /^((\d{1,3}\.){3}\d{1,3}(\/(\d|[1-2]\d|3[0-2]))?|[0-9a-fA-F:]+\/\d{1,3})$/;

function validateCIDR(text: string, key: string): boolean {
  if (!text || text.trim() === '') {
    errors[key] = '';
    return true;
  }
  const lines = text.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (line === '' || line.startsWith('#')) continue;
    if (!CIDR_RE.test(line)) {
      errors[key] = t('settings.lan.cidrError', { line: i + 1, text: line });
      return false;
    }
  }
  errors[key] = '';
  return true;
}

function validateAll(): boolean {
  const a = validateCIDR(lanConfig.lanAllowedIPs, 'allowed');
  const b = validateCIDR(lanConfig.lanDisallowedIPs, 'disallowed');
  const c = validateCIDR(lanConfig.lanSkipAuthIPs, 'skipAuth');
  return a && b && c;
}

const loadData = async () => {
  loading.value = true;
  try {
    const cfg = await (API.GetNetworkConfig as any)();
    if (cfg) {
      lanConfig.allowLan       = cfg.allowLan       ?? false;
      lanConfig.lanAuthUser    = cfg.lanAuthUser     ?? '';
      lanConfig.lanAuthPass    = cfg.lanAuthPass     ?? '';
      lanConfig.lanAllowedIPs  = cfg.lanAllowedIPs   ?? '';
      lanConfig.lanDisallowedIPs = cfg.lanDisallowedIPs ?? '';
      lanConfig.lanSkipAuthIPs = cfg.lanSkipAuthIPs  ?? '';
    }
  } catch (e) {
    console.error('加载局域网代理配置失败', e);
  } finally {
    loading.value = false;
  }
};

const save = async () => {
  if (loading.value) return;
  try {
    // Merge LAN fields into the full network config to avoid overwriting
    // other network settings that may be managed in SettingsNetwork.
    const current = await (API.GetNetworkConfig as any)();
    const merged = {
      ...current,
      allowLan:         lanConfig.allowLan,
      lanAuthUser:      lanConfig.lanAuthUser,
      lanAuthPass:      lanConfig.lanAuthPass,
      lanAllowedIPs:    lanConfig.lanAllowedIPs,
      lanDisallowedIPs: lanConfig.lanDisallowedIPs,
      lanSkipAuthIPs:   lanConfig.lanSkipAuthIPs,
    };
    await (API.SaveNetworkConfig as any)(merged);
  } catch (e) {
    console.error('局域网代理配置保存失败', e);
  }
};

const saveAndValidate = async (key: string) => {
  validateCIDR(
    key === 'allowed' ? lanConfig.lanAllowedIPs :
    key === 'disallowed' ? lanConfig.lanDisallowedIPs : lanConfig.lanSkipAuthIPs,
    key
  );
  await save();
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
.setting-group.scrollable { padding-bottom: 20px; overflow: visible; max-height: none; }

h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 4px 0; color: var(--text-main); font-size: 1rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); line-height: 1.5; }

.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.col-item { flex-direction: column; align-items: stretch; gap: 10px; padding: 16px 0; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

/* section label */
.section-label {
  padding: 18px 0 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.section-label-text {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--text-main);
}
.section-label-sub {
  font-size: 0.8rem;
  color: var(--text-sub);
}

.example-hint {
  margin-top: 4px !important;
  font-family: var(--font-mono);
  font-size: 0.8rem !important;
  color: var(--text-muted) !important;
}

/* inputs */
.full-input {
  width: 100%;
  background: var(--surface-hover);
  border: none;
  color: var(--text-main);
  padding: 10px 14px;
  border-radius: 8px;
  outline: none;
  font-size: 0.9rem;
  box-sizing: border-box;
}
.full-input:disabled { opacity: 0.5; cursor: not-allowed; }

.password-wrap {
  position: relative;
  width: 100%;
}
.password-wrap .full-input { padding-right: 40px; }
.eye-btn {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-sub);
  padding: 4px;
  display: flex;
  align-items: center;
}
.eye-btn:hover { color: var(--text-main); }

.modern-textarea {
  background: var(--surface-hover);
  border: none;
  color: var(--text-main);
  padding: 10px 14px;
  border-radius: 8px;
  outline: none;
  resize: vertical;
  font-family: var(--font-mono);
  font-size: 0.85rem;
  line-height: 1.6;
  width: 100%;
  box-sizing: border-box;
}
.modern-textarea:disabled { opacity: 0.5; cursor: not-allowed; }

.textarea-wrap { display: flex; flex-direction: column; gap: 6px; width: 100%; }

/* switch */
.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; flex-shrink: 0; }
.modern-switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3); }
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--accent-fg); }

/* validation */
.validation-error {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #f59e0b;
  font-size: 0.82rem;
  font-weight: 500;
  animation: fadeIn 0.2s ease;
}
.warn-icon { width: 15px; height: 15px; flex-shrink: 0; }

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(-3px); }
  to   { opacity: 1; transform: translateY(0); }
}

/* header */
.sub-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 0 0 12px 0;
  background: transparent;
}
.sub-header.section-header h3 { flex: 1; margin: 0; border: none; padding: 0; margin-left: 12px; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; flex-shrink: 0; }
.back-btn:hover { background: var(--surface-hover); }
.mini-btn-reset { height: 36px !important; padding: 0 14px !important; font-size: 0.85rem !important; border-radius: 8px !important; }
.mini-btn-reset :deep(.btn-icon) svg { width: 16px; height: 16px; }
</style>
