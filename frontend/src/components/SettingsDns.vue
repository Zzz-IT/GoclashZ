<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>DNS 服务器配置</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'dns')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> 重置
      </button>
    </div>

    <div class="glass-card setting-group scrollable">

      <div class="setting-item">
        <div class="info"><h4>启用 DNS 覆写 (Enable DNS)</h4></div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.enable" @change="saveDns" :disabled="loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>DNS 监听端口 (Listen)</h4></div>
        <input type="text" class="modern-input" v-model="dnsConfig.listen" @blur="saveDns" :disabled="!dnsConfig.enable || loading" placeholder="如 0.0.0.0:1053" />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>开启 IPv6 解析 (IPv6 Resolution)</h4></div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.ipv6" @change="saveDns" :disabled="!dnsConfig.enable || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>偏好 HTTP/3 (Prefer HTTP/3)</h4>
          <p>支持 DoH3 的服务器优先使用 HTTP/3 连接</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.preferH3" @change="saveDns" :disabled="!dnsConfig.enable || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>增强模式 (Enhanced Mode)</h4></div>
        <ModernSelect 
          v-model="dnsConfig.enhancedMode" 
          :options="enhancedModeOptions" 
          @change="saveDns" 
          :disabled="!dnsConfig.enable || loading" 
        />
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info">
          <h4>遵守规则 (Respect Rules)</h4>
          <p>Fake-IP 模式下，匹配路由规则以决定是否返回真实 IP</p>
        </div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.respectRules" @change="saveDns" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip' || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>Fake-IP 范围 (Fake-IP Range)</h4></div>
        <input type="text" class="modern-input" v-model="dnsConfig.fakeIpRange" @blur="saveDns" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip' || loading" />
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>Fake-IP 缓存过滤器 (Fake-IP Filter)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.fakeIpFilter || []).join('\n')" @blur="updateDnsArray($event, 'fakeIpFilter')" rows="3" placeholder="如 *.lan" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip' || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>使用系统 Hosts (Use System Hosts)</h4></div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.useSystemHosts" @change="saveDns" :disabled="!dnsConfig.enable || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>使用 Hosts (Use Hosts)</h4></div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.useHosts" @change="saveDns" :disabled="!dnsConfig.enable || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>默认名称服务器 (Default Nameservers)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.defaultNameserver || []).join('\n')" @blur="updateDnsArray($event, 'defaultNameserver')" rows="2" placeholder="纯IP服务器，如 114.114.114.114" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>主名称服务器 (Nameservers)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.nameserver || []).join('\n')" @blur="updateDnsArray($event, 'nameserver')" rows="3" placeholder="推荐使用 DoH / DoT" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>备用名称服务器 (Fallback)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.fallback || []).join('\n')" @blur="updateDnsArray($event, 'fallback')" rows="3" placeholder="用于解析境外域名" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>直连名称服务器 (Direct Nameservers)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.directNameserver || []).join('\n')" @blur="updateDnsArray($event, 'directNameserver')" rows="2" placeholder="专用于直连规则的 DNS" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>代理节点解析服务器 (Proxy Server Nameserver)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.proxyServerNameserver || []).join('\n')" @blur="updateDnsArray($event, 'proxyServerNameserver')" rows="2" placeholder="用于解析代理节点的域名" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>指定域名解析服务器 (Nameserver Policy)</h4></div>
        <textarea class="modern-textarea" :value="formatNameserverPolicy(dnsConfig.nameserverPolicy)" @blur="updateNameserverPolicy" rows="4" placeholder="geosite:cn: https://doh.pub/dns-query" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>启用 GeoIP 回退 (Fallback Filter GeoIP)</h4></div>
        <label class="modern-switch">
          <input type="checkbox" v-model="dnsConfig.fallbackFilter.geoip" @change="saveDns" :disabled="!dnsConfig.enable || loading">
          <span class="slider"></span>
        </label>
      </div>
      <div class="divider"></div>

      <div class="setting-item">
        <div class="info"><h4>GeoIP 代码 (GeoIP Code)</h4></div>
        <input type="text" class="modern-input" v-model="dnsConfig.fallbackFilter.geoipCode" @blur="saveDns" :disabled="!dnsConfig.enable || !dnsConfig.fallbackFilter.geoip || loading" placeholder="默认 CN" />
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>IPCIDR 过滤 (Fallback Filter IPCIDR)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.fallbackFilter.ipcidr || []).join('\n')" @blur="updateFallbackFilterIpcidr" rows="3" placeholder="如 240.0.0.0/4" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>
      <div class="divider"></div>

      <div class="setting-item col-item">
        <div class="info"><h4>域名过滤 (Fallback Filter Domain)</h4></div>
        <textarea class="modern-textarea" :value="(dnsConfig.fallbackFilter.domain || []).join('\n')" @blur="updateFallbackFilterDomain" rows="3" placeholder="匹配的域名将强制走 Fallback" :disabled="!dnsConfig.enable || loading"></textarea>
      </div>

    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { ICONS } from '../utils/icons';
import ModernSelect from './ModernSelect.vue';

const props = defineProps<{ resetKey: number }>();

defineEmits<{ navigate: [view: 'main']; reset: [module: 'dns'] }>();

const enhancedModeOptions = [
  { label: 'Fake-IP', value: 'fake-ip' },
  { label: 'Redir-Host', value: 'redir-host' },
  { label: 'Normal', value: 'normal' }
];

const loading = ref(true);

const dnsConfig = ref<any>({
  enable: true, 
  listen: '0.0.0.0:1053',
  ipv6: false, 
  preferH3: false,
  enhancedMode: 'fake-ip', 
  respectRules: false,
  fakeIpRange: '198.18.0.1/16',
  fakeIpFilter: ['*.lan', '*.localdomain'],
  useSystemHosts: true,
  useHosts: true,
  defaultNameserver: ['223.5.5.5', '114.114.114.114'],
  nameserver: ['https://doh.pub/dns-query'],
  fallback: ['https://doh.dns.sb/dns-query'],
  directNameserver: ['https://dns.alidns.com/dns-query'],
  proxyServerNameserver: ['https://doh.pub/dns-query'],
  nameserverPolicy: { 'geosite:cn': 'https://doh.pub/dns-query' },
  fallbackFilter: {
      geoip: true,
      geoipCode: 'CN',
      ipcidr: ['240.0.0.0/4', '0.0.0.0/32'],
      domain: ['+.google.com', '+.facebook.com', '+.twitter.com']
  }
});

const loadData = async () => {
  loading.value = true;
  try {
    const dnsConf = await (API.GetDNSConfig as any)();
    if (dnsConf) dnsConfig.value = dnsConf;
  } catch (e) {
    console.error('加载配置失败', e);
  } finally {
    loading.value = false;
  }
};

const saveDns = async () => {
  if (loading.value) return;
  try { await (API.SaveDNSConfig as any)(dnsConfig.value); } catch (e) { console.error('DNS 保存失败', e); }
};

const updateDnsArray = (e: Event, key: string) => {
  if (loading.value) return;
  const val = (e.target as HTMLTextAreaElement).value;
  dnsConfig.value[key] = val.split('\n').map(s => s.trim()).filter(s => s);
  saveDns();
};

const updateFallbackFilterIpcidr = (e: Event) => {
    if (loading.value) return;
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.ipcidr = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

const updateFallbackFilterDomain = (e: Event) => {
    if (loading.value) return;
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.domain = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

const formatNameserverPolicy = (policy: Record<string, string>) => {
  if (!policy) return '';
  return Object.entries(policy).map(([k, v]) => `${k}: ${v}`).join('\n');
};

const updateNameserverPolicy = (e: Event) => {
  if (loading.value) return;
  const val = (e.target as HTMLTextAreaElement).value;
  const policy: Record<string, string> = {};

  val.split('\n').forEach(line => {
    line = line.trim();
    if (!line) return;

    let idx = line.indexOf(': ');
    if (idx === -1) idx = line.lastIndexOf(':');

    if (idx > 0) {
      const k = line.substring(0, idx).trim();
      const v = line.substring(idx + 1).trim();
      if (k && v) policy[k] = v;
    }
  });

  dnsConfig.value.nameserverPolicy = policy;
  saveDns();
};

onMounted(() => { void loadData(); });
watch(() => props.resetKey, () => { void loadData(); });
</script>

<style scoped>
.settings-page { display: flex; flex-direction: column; flex: 1; min-height: 100%; overflow: visible; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.setting-group.scrollable { padding-bottom: 20px; overflow: visible; max-height: none; }
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.col-item { flex-direction: column; align-items: stretch; gap: 10px; padding: 16px 0; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }
.modern-input, .modern-textarea { background: var(--surface-hover); border: none; color: var(--text-main); padding: 10px 14px; border-radius: 8px; outline: none; font-size: 0.9rem; }
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
.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }
.sub-header.page-sticky-mask { --sticky-mask-bleed: 4px; }
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }
.section-header { display: flex; justify-content: space-between; align-items: center; }
.sub-header.section-header h3 { flex: 1; margin-left: 12px; }
.mini-btn-reset { height: 36px !important; padding: 0 14px !important; font-size: 0.85rem !important; border-radius: 8px !important; }
.mini-btn-reset :deep(.btn-icon) svg { width: 16px; height: 16px; }
</style>
