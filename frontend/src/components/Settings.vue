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

        <div class="setting-item clickable" @click="view = 'dns'">
          <div class="info">
            <h4>DNS 服务器配置 (DNS Config)</h4>
            <p>配置防污染解析、Fake-IP 以及策略路由所用的名称服务器。</p>
          </div>
          <span class="arrow">➔</span>
        </div>
        
        <div class="setting-item clickable" @click="view = 'network'">
          <div class="info">
            <h4>基础 network 设置</h4>
            <p>管理内核底层的连接行为、IPv6 栈以及测速逻辑。</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="setting-item clickable" @click="view = 'behavior'">
          <div class="info">
            <h4>应用行为设置 (App Behavior)</h4>
            <p>配置软件启动行为、关闭逻辑以及系统托盘相关设置。</p>
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
              检测状态: <span :class="tunStatus.hasWintun ? 'green-text' : 'red-text'">{{ tunStatus.hasWintun ? 'wintun 已就绪' : '缺失驱动文件' }}</span>
            </p>
          </div>
          <button class="action-btn" @click="installDriver" :disabled="isInstalling || tunStatus.hasWintun">
            {{ isInstalling ? '处理中...' : (tunStatus.hasWintun ? '已安装' : '安装驱动') }}
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
          <input type="text" class="modern-input" :value="tunConfig.dnsHijack.join(', ')" @blur="updateTunDnsHijack" placeholder="如 any:53" :disabled="!tunStatus.hasWintun" />
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

    <div v-else-if="view === 'dns'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
        </button>
        <h3>DNS 服务器配置</h3>
      </div>

      <div class="glass-card setting-group scrollable">

        <div class="setting-item">
          <div class="info"><h4>启用 DNS 解析 (Enable DNS)</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.enable" @change="saveDns">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>DNS 监听端口 (Listen)</h4></div>
          <input type="text" class="modern-input" v-model="dnsConfig.listen" @blur="saveDns" :disabled="!dnsConfig.enable" placeholder="如 0.0.0.0:1053" />
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>开启 IPv6 解析 (IPv6 Resolution)</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.ipv6" @change="saveDns" :disabled="!dnsConfig.enable">
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
            <input type="checkbox" v-model="dnsConfig.preferH3" @change="saveDns" :disabled="!dnsConfig.enable">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>增强模式 (Enhanced Mode)</h4></div>
          <select class="modern-select" v-model="dnsConfig.enhancedMode" @change="saveDns" :disabled="!dnsConfig.enable">
            <option value="fake-ip">Fake-IP</option>
            <option value="redir-host">Redir-Host</option>
            <option value="normal">Normal</option>
          </select>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
             <h4>遵守规则 (Respect Rules)</h4>
             <p>Fake-IP 模式下，匹配路由规则以决定是否返回真实 IP</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.respectRules" @change="saveDns" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip'">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>Fake-IP 范围 (Fake-IP Range)</h4></div>
          <input type="text" class="modern-input" v-model="dnsConfig.fakeIpRange" @blur="saveDns" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip'" />
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>Fake-IP 缓存过滤器 (Fake-IP Filter)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.fakeIpFilter || []).join('\n')" @blur="updateDnsArray($event, 'fakeIpFilter')" rows="3" placeholder="如 *.lan" :disabled="!dnsConfig.enable || dnsConfig.enhancedMode !== 'fake-ip'"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>使用系统 Hosts (Use System Hosts)</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.useSystemHosts" @change="saveDns" :disabled="!dnsConfig.enable">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>使用 Hosts (Use Hosts)</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.useHosts" @change="saveDns" :disabled="!dnsConfig.enable">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>默认名称服务器 (Default Nameservers)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.defaultNameserver || []).join('\n')" @blur="updateDnsArray($event, 'defaultNameserver')" rows="2" placeholder="纯IP服务器，如 114.114.114.114" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>主名称服务器 (Nameservers)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.nameserver || []).join('\n')" @blur="updateDnsArray($event, 'nameserver')" rows="3" placeholder="推荐使用 DoH / DoT" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>备用名称服务器 (Fallback)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.fallback || []).join('\n')" @blur="updateDnsArray($event, 'fallback')" rows="3" placeholder="用于解析境外域名" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>直连名称服务器 (Direct Nameservers)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.directNameserver || []).join('\n')" @blur="updateDnsArray($event, 'directNameserver')" rows="2" placeholder="专用于直连规则的 DNS" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>代理节点解析服务器 (Proxy Server Nameserver)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.proxyServerNameserver || []).join('\n')" @blur="updateDnsArray($event, 'proxyServerNameserver')" rows="2" placeholder="用于解析代理节点的域名" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>指定域名解析服务器 (Nameserver Policy)</h4></div>
          <textarea class="modern-textarea" :value="formatNameserverPolicy(dnsConfig.nameserverPolicy)" @blur="updateNameserverPolicy" rows="4" placeholder="geosite:cn: https://doh.pub/dns-query" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>启用 GeoIP 回退 (Fallback Filter GeoIP)</h4></div>
          <label class="modern-switch">
            <input type="checkbox" v-model="dnsConfig.fallbackFilter.geoip" @change="saveDns" :disabled="!dnsConfig.enable">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>GeoIP 代码 (GeoIP Code)</h4></div>
          <input type="text" class="modern-input" v-model="dnsConfig.fallbackFilter.geoipCode" @blur="saveDns" :disabled="!dnsConfig.enable || !dnsConfig.fallbackFilter.geoip" placeholder="默认 CN" />
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>IPCIDR 过滤 (Fallback Filter IPCIDR)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.fallbackFilter.ipcidr || []).join('\n')" @blur="updateFallbackFilterIpcidr" rows="3" placeholder="如 240.0.0.0/4" :disabled="!dnsConfig.enable"></textarea>
        </div>
        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info"><h4>域名过滤 (Fallback Filter Domain)</h4></div>
          <textarea class="modern-textarea" :value="(dnsConfig.fallbackFilter.domain || []).join('\n')" @blur="updateFallbackFilterDomain" rows="3" placeholder="匹配的域名将强制走 Fallback" :disabled="!dnsConfig.enable"></textarea>
        </div>

      </div>
    </div>

    <div v-else-if="view === 'network'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
        </button>
        <h3>基础网络配置</h3>
      </div>

      <div class="glass-card setting-group scrollable">
        <div class="setting-item">
          <div class="info">
            <h4>IPv6 支持</h4>
            <p>开启后内核将解析并接管 IPv6 流量。若网络环境不支持可能导致卡顿。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="netConfig.ipv6" @change="saveNet">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>统一延迟测试 (Unified Delay)</h4>
            <p>开启后将去除握手损耗，显示更真实的节点响应延迟。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="netConfig.unifiedDelay" @change="saveNet">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>TCP 并发连接</h4>
            <p>同时向所有解析出的 IP 发起连接，取最快响应者。大幅提升首屏加载速度。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="netConfig.tcpConcurrent" @change="saveNet">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>TCP 保持活动 (Keep Alive)</h4>
            <p>降低在某些防火墙下的断连概率，保持长连接存活。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="netConfig.tcpKeepAlive" @change="saveNet">
            <span class="slider"></span>
          </label>
        </div>

        <div class="setting-item sub-item" :class="{ 'disabled-fade': !netConfig.tcpKeepAlive }">
          <div class="info">
            <h4 class="sub-label">发送时间间隔 (Interval)</h4>
            <p>单位为秒，建议值 15-30s</p>
          </div>
          <div class="input-with-unit">
            <input 
              type="number" 
              class="modern-input num-input" 
              v-model.number="netConfig.tcpKeepAliveInterval" 
              :disabled="!netConfig.tcpKeepAlive"
              @blur="saveNet"
            />
            <span class="unit">s</span>
          </div>
        </div>

        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info">
            <h4>延迟测试网址 (Delay Test URL)</h4>
            <p>内核进行连接可用性测试时使用的 URL。建议使用 Google 或 Cloudflare 的测速地址。</p>
          </div>
          <input 
            type="text" 
            class="modern-input" 
            style="text-align: left; width: 100%; margin-top: 10px;" 
            v-model="netConfig.testUrl" 
            @blur="saveNet" 
            placeholder="http://www.gstatic.com/generate_204" 
          />
        </div>
      </div>
    </div>

    <div v-else-if="view === 'behavior'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
        </button>
        <h3>应用行为设置</h3>
      </div>

      <div class="glass-card setting-group scrollable">
        <div class="setting-item">
          <div class="info">
            <h4>静默启动</h4>
            <p>启动时直接进入系统托盘，不自动显示主界面。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="behavior.silentStart" @change="saveBehavior">
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
            <input type="checkbox" v-model="behavior.closeToTray" @change="saveBehavior">
            <span class="slider"></span>
          </label>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>内核日志等级 (Log Level)</h4>
            <p>调整核心输出的日志详细程度。如遇到问题无法排查，可改为 debug。</p>
          </div>
          <select class="modern-select" v-model="behavior.logLevel" @change="saveBehavior">
            <option value="info">Info (默认)</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
            <option value="debug">Debug</option>
            <option value="silent">Silent</option>
          </select>
        </div>
        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>隐藏日志显示</h4>
            <p>开启后，左侧导航栏的“实时日志”入口将被隐藏。</p>
          </div>
          <label class="modern-switch">
            <input type="checkbox" v-model="behavior.hideLogs" @change="saveBehavior">
            <span class="slider"></span>
          </label>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const props = defineProps({
  initialView: {
    type: String,
    default: 'main'
  }
});

const view = ref(props.initialView);
watch(() => props.initialView, (newVal) => { view.value = newVal; });

const isInstalling = ref(false);
const tunStatus = ref<Record<string, boolean>>({ hasWintun: false, isAdmin: false });

const tunConfig = ref({
  enable: false, stack: 'gvisor', device: '', autoRoute: true, autoDetect: true,
  dnsHijack: ['any:53'], strictRoute: true, mtu: 1500
});

// DNS 配置响应式对象
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

const netConfig = ref({
  ipv6: false,
  unifiedDelay: true,
  tcpConcurrent: true,
  tcpKeepAlive: true,
  tcpKeepAliveInterval: 15,
  testUrl: 'http://www.gstatic.com/generate_204' // 👈 1. 响应式绑定
});

const behavior = ref({
  silentStart: false,
  closeToTray: true,
  logLevel: 'info',
  hideLogs: false
});

const loadData = async () => {
  try {
    const status = await API.CheckTunEnv();
    tunStatus.value = status;
    const tunConf = await API.GetTunConfig();
    if (tunConf) tunConfig.value = tunConf;

    // 👉 [新增] 每次进入设置页，强制拉取当前真实运行状态，覆盖 UI 显示
    const realStatus: any = await API.GetProxyStatus();
    if (realStatus) {
      tunConfig.value.enable = realStatus.tun;
    }

    const dnsConf = await (API.GetDNSConfig as any)();
    if (dnsConf) dnsConfig.value = dnsConf;

    const netConf = await (API.GetNetworkConfig as any)();
    if (netConf) netConfig.value = netConf;

    const behaviorConf = await (API.GetAppBehavior as any)();
    if (behaviorConf) behavior.value = behaviorConf;
  } catch (e) {
    console.error('加载配置失败', e);
  }
};

onMounted(() => { loadData(); });

const fixUWP = async () => {
  try {
    await API.FixUWPNetwork();
    alert('✅ UWP 环回限制已成功解除！');
  } catch (e) {
    alert('修复失败，请尝试右键以管理员身份运行本软件。\n错误信息: ' + e);
  }
};

const handleTunToggle = async (e: Event) => {
  if (tunConfig.value.enable && !tunStatus.value.hasWintun) {
    e.preventDefault();
    tunConfig.value.enable = false;
    alert('⚠️ 无法开启 TUN 模式：\n请先点击下方的“安装驱动”按钮下载并配置 wintun.dll。');
    return;
  }
  
  try {
    // 👉 [新增] 设置页拨动开关时，直接调用内核指令同步改变状态
    await API.ToggleTunMode(tunConfig.value.enable);
    await saveTun(); // 保存配置

    // 👉 [新增] 广播给控制台和左下角呼吸灯
    const newStatus = await API.GetProxyStatus();
    window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: newStatus }));
  } catch (err) {
    alert("操作内核 TUN 失败: " + err);
  }
};

const installDriver = async () => {
  isInstalling.value = true;
  try {
    await API.InstallTunDriver();
    await loadData();
    if (tunStatus.value.hasWintun) {
       alert('✅ 驱动安装成功，现在可以开启 TUN 模式了！');
    } else {
       alert('❌ 系统仍未检测到 wintun.dll。请确认网络或以管理员运行。');
    }
  } catch (e) {
    alert('安装提示: ' + e);
  } finally {
    isInstalling.value = false;
  }
};

const saveTun = async () => {
  try { await API.SaveTunConfig(tunConfig.value); } catch (e) { console.error('保存失败', e); }
};

const updateTunDnsHijack = (e: Event) => {
  const val = (e.target as HTMLInputElement).value;
  tunConfig.value.dnsHijack = val.split(',').map(s => s.trim()).filter(s => s);
  saveTun();
};

const saveDns = async () => {
  try { await (API.SaveDNSConfig as any)(dnsConfig.value); } catch (e) { console.error('DNS 保存失败', e); }
};

// 保存基础网络设置
const saveNet = async () => {
  try {
    await (API.SaveNetworkConfig as any)(netConfig.value);
  } catch (e) {
    console.error('网络配置保存失败', e);
  }
};

const saveBehavior = async () => {
  try {
    await API.SaveAppBehavior(behavior.value);
  } catch (e) {
    console.error('应用行为保存失败', e);
  }
};

// 处理多行文本框的数组更新（换行符分割）
const updateDnsArray = (e: Event, key: string) => {
  const val = (e.target as HTMLTextAreaElement).value;
  dnsConfig.value[key] = val.split('\n').map(s => s.trim()).filter(s => s);
  saveDns();
};

// 专门处理 fallbackFilter 的 IPCIDR 数组
const updateFallbackFilterIpcidr = (e: Event) => {
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.ipcidr = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

// 专门处理 fallbackFilter 的 Domain 数组
const updateFallbackFilterDomain = (e: Event) => {
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.domain = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

// 格式化展示 nameserver-policy 对象
const formatNameserverPolicy = (policy: Record<string, string>) => {
  if (!policy) return '';
  return Object.entries(policy).map(([k, v]) => `${k}: ${v}`).join('\n');
};

// 解析输入框内的内容转为 nameserver-policy 对象
const updateNameserverPolicy = (e: Event) => {
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
</script>

<style scoped>
.settings-container { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
.settings-page { display: flex; flex-direction: column; height: 100%; flex: 1; }
.setting-group { padding: 20px 24px; margin-bottom: 20px; }
.scrollable { overflow-y: auto; padding-right: 12px; padding-bottom: 20px; }

h3 { margin: 0 0 20px 0; color: var(--text-main); font-size: 1.1rem; padding-bottom: 12px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p { 
  margin: 0; 
  font-size: 0.85rem; 
  color: var(--text-sub); 
  max-width: 100%;    /* 修改：允许占据全部可用宽度，不再强制 80% */
  line-height: 1.5;   /* 顺便优化下行高，增加可读性 */
}

/* 1. 为 info 容器增加弹性布局指令 */
.info {
  flex: 1;           /* 占据左侧全部剩余空间 */
  padding-right: 24px; /* 保护右侧间距，防止文字贴着开关 */
  min-width: 0;      /* 防止 flex 容器溢出 */
}

.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.col-item { flex-direction: column; align-items: stretch; gap: 10px; padding: 16px 0; } /* 专为 textarea 设计 */
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }

.arrow { color: var(--text-sub); font-size: 1.2rem; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }


.modern-input, .modern-select, .modern-textarea { background: var(--surface-hover); border: none; color: var(--text-main); padding: 8px 12px; border-radius: 8px; outline: none; }
.modern-input { text-align: right; }
.modern-textarea { resize: vertical; font-family: monospace; font-size: 0.85rem; line-height: 1.5; text-align: left; }
.modern-input:disabled, .modern-select:disabled, .modern-textarea:disabled { opacity: 0.5; cursor: not-allowed; }
.num-input { width: 80px; }

.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }

/* 1. 修复无效的 .dark 匹配，直接使用全局动态变量 var(--surface-hover) 或更明显的背景色 */
.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }

/* 未选中时：保持白色圆形，因为无论白天黑夜，灰色底 + 白圆点都很清晰 */
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);}

input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }

/* 选中时：底色变为 accent（白天黑，黑夜白） */
input:checked + .slider { background-color: var(--accent); }

/* 2. 核心修复：选中时的圆点必须使用 accent-fg 进行反色匹配（白天白，黑夜黑） */
input:checked + .slider:before { 
  transform: translateX(20px); 
  background-color: var(--accent-fg); 
}

.slide-in { animation: slideIn 0.2s ease forwards; }
@keyframes slideIn { from { opacity: 0; transform: translateX(10px); } to { opacity: 1; transform: translateX(0); } }

.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }

/* 子选项缩进，使其看起来隶属于上一个开关 */
.sub-item {
  padding-left: 20px !important;
  border-left: 2px solid var(--surface-hover);
  margin-left: 8px;
  margin-top: -10px; /* 拉近与主开关的距离 */
  margin-bottom: 10px;
}

.sub-label {
  font-size: 0.9rem !important;
  font-weight: 500 !important;
}

/* 禁用状态的淡出效果 */
.disabled-fade {
  opacity: 0.5;
  pointer-events: none;
}

/* 带单位的输入框容器 */
.input-with-unit {
  display: flex;
  align-items: center;
  gap: 8px;
}

.unit {
  font-size: 0.85rem;
  color: var(--text-muted);
  font-family: var(--font-mono);
}

.num-input {
  width: 70px;
  text-align: center;
}

.status-msg { margin-top: 4px; font-weight: 500; }
.green-text { color: var(--text-main); font-weight: 600; }
.red-text { color: var(--text-muted); }
</style>