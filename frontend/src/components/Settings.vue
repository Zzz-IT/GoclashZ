<template>
  <div class="settings-container">

    <div v-if="view === 'main'" class="settings-page">
      <div class="glass-card setting-group">
        <h3>系统与网络</h3>

        <div class="setting-item clickable" @click="enterUwpManager">
          <div class="info">
            <h4>UWP 环回免除工具</h4>
            <p>管理 Windows UWP 应用（如 Microsoft Store）的代理访问权限。</p>
          </div>
          <span class="arrow">➔</span>
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
            @blur="saveBehavior" 
            placeholder="默认 UA" 
          />
        </div>
      </div>
    </div>

    <div v-else-if="view === 'uwp'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
        </button>
        <h3>UWP 环回管理</h3>
      </div>

      <div class="uwp-toolbar">
        <div class="uwp-search">
          <span class="search-icon" v-html="ICONS.search"></span>
          <input v-model="uwpSearch" placeholder="搜索应用名称或包名..." />
          <span v-if="uwpSearch" class="clear-icon" @click="uwpSearch = ''" v-html="ICONS.close"></span>
        </div>
        <div class="uwp-batch">
          <button class="batch-btn" @click="toggleAllUwp(true)">全选</button>
          <button class="batch-btn" @click="toggleAllUwp(false)">反选</button>
        </div>
      </div>

      <div class="uwp-list-wrapper scrollable">
        <div 
          v-for="app in filteredUwpApps" 
          :key="app.sid" 
          class="uwp-app-item"
          :class="{ 'active': app.isEnabled }"
          @click="app.isEnabled = !app.isEnabled"
        >
          <div class="app-main-content">
            <div class="app-avatar">
              {{ app.displayName?.[0]?.toUpperCase() || '?' }}
            </div>
            <div class="app-details">
              <span class="app-name">{{ app.displayName || '未命名应用' }}</span>
              <span class="app-pkg">{{ app.packageFamilyName }}</span>
            </div>
          </div>

          <div class="app-status-wrapper">
            <div class="uwp-status-tag">
              {{ app.isEnabled ? '已豁免' : '受限' }}
            </div>
          </div>
        </div>
      </div>

      <div class="uwp-footer">
        <button class="apply-btn" :disabled="savingUwp" @click="saveUwpChanges">
          <span v-if="!savingUwp">应用更改 (需要管理员权限)</span>
          <span v-else class="loading-spinner">正在保存...</span>
        </button>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert } from '../store';
import { ICONS } from '../utils/icons';

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
  testUrl: 'http://www.gstatic.com/generate_204'
});

const behavior = ref({
  silentStart: false,
  closeToTray: true,
  logLevel: 'info',
  hideLogs: false,
  subUA: '',
  activeConfig: '',
  activeMode: ''
});


const uwpApps = ref<any[]>([]);
const uwpSearch = ref('');
const savingUwp = ref(false);

const enterUwpManager = async () => {
  view.value = 'uwp';
  try {
    uwpApps.value = await (API as any).GetUwpApps();
  } catch (e) {
    showAlert('获取 UWP 列表失败: ' + e, '错误');
  }
};

const filteredUwpApps = computed(() => {
  const q = uwpSearch.value.toLowerCase();
  return uwpApps.value.filter(app => 
    (app.displayName || '').toLowerCase().includes(q) || 
    (app.packageFamilyName || '').toLowerCase().includes(q)
  );
});

const toggleAllUwp = (val: boolean) => {
  if (val) {
    uwpApps.value.forEach(app => app.isEnabled = true);
  } else {
    uwpApps.value.forEach(app => app.isEnabled = !app.isEnabled);
  }
};

const saveUwpChanges = async () => {
  savingUwp.value = true;
  try {
    const sids = uwpApps.value.filter(a => a.isEnabled).map(a => a.sid);
    await (API as any).SaveUwpExemptions(sids);
    await showAlert('豁免配置已成功更新！', '完成');
  } catch (e) {
    await showAlert('保存失败: ' + e, '错误');
  } finally {
    savingUwp.value = false;
  }
};

const loadData = async () => {
  try {
    const status = await API.CheckTunEnv();
    tunStatus.value = status;
    const tunConf = await API.GetTunConfig();
    if (tunConf) tunConfig.value = tunConf;

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

const handleTunToggle = async (e: Event) => {
  if (tunConfig.value.enable && !tunStatus.value.hasWintun) {
    e.preventDefault();
    tunConfig.value.enable = false;
    await showAlert('无法开启 TUN 模式：\n请先点击下方的“安装驱动”按钮下载并配置 wintun.dll。', '缺少依赖');
    return;
  }
  
  // 1. 记录操作前的原始状态
  const originalValue = !tunConfig.value.enable;
  
  try {
    // 2. 调用后端 API
    await API.ToggleTunMode(tunConfig.value.enable);
    await saveTun();

    // 同步刷新全局状态
    const newStatus = await API.GetProxyStatus();
    window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: newStatus }));
  } catch (err) {
    // 3. 核心修复：发生错误时回滚 UI 状态
    tunConfig.value.enable = originalValue; 
    await showAlert("操作内核 TUN 失败: " + err, '错误');
  }
};


const installDriver = async () => {
  isInstalling.value = true;
  try {
    await API.InstallTunDriver();
    await loadData();
    if (tunStatus.value.hasWintun) {
       await showAlert('驱动安装成功，现在可以开启 TUN 模式了！', '安装成功');
    } else {
       await showAlert('系统仍未检测到 wintun.dll。请确认网络或以管理员运行。', '安装失败');
    }
  } catch (e) {
    await showAlert('安装提示: ' + e, '发生错误');
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

const updateDnsArray = (e: Event, key: string) => {
  const val = (e.target as HTMLTextAreaElement).value;
  dnsConfig.value[key] = val.split('\n').map(s => s.trim()).filter(s => s);
  saveDns();
};

const updateFallbackFilterIpcidr = (e: Event) => {
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.ipcidr = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

const updateFallbackFilterDomain = (e: Event) => {
    const val = (e.target as HTMLTextAreaElement).value;
    dnsConfig.value.fallbackFilter.domain = val.split('\n').map(s => s.trim()).filter(s => s);
    saveDns();
};

const formatNameserverPolicy = (policy: Record<string, string>) => {
  if (!policy) return '';
  return Object.entries(policy).map(([k, v]) => `${k}: ${v}`).join('\n');
};

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
  max-width: 100%;
  line-height: 1.5;
}

.info { flex: 1; padding-right: 24px; min-width: 0; }

.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.col-item { flex-direction: column; align-items: stretch; gap: 10px; padding: 16px 0; }
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

.slider { position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3);}
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--accent); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--accent-fg); }

.slide-in { animation: slideIn 0.2s ease forwards; }
@keyframes slideIn { from { opacity: 0; transform: translateX(10px); } to { opacity: 1; transform: translateX(0); } }

.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }

.sub-item {
  padding-left: 20px !important;
  border-left: 2px solid var(--surface-hover);
  margin-left: 8px;
  margin-top: -10px;
  margin-bottom: 10px;
}

.sub-label { font-size: 0.9rem !important; font-weight: 500 !important; }
.disabled-fade { opacity: 0.5; pointer-events: none; }
.input-with-unit { display: flex; align-items: center; gap: 8px; }
.unit { font-size: 0.85rem; color: var(--text-muted); font-family: var(--font-mono); }
.status-msg { margin-top: 4px; font-weight: 500; }
.green-text { color: var(--text-main); font-weight: 600; }
.red-text { color: var(--text-muted); }

/* ================================== */
/* UWP 管理器 - 像素级 UI 方案          */
/* ================================== */

/* 工具栏自适应搜索框 */
.uwp-toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
}

.uwp-search {
  flex: 1;
  display: flex;
  align-items: center;
  background: var(--surface);
  border: 1px solid var(--surface-hover);
  border-radius: 10px;
  padding: 0 12px;
  height: 40px;
  transition: all 0.2s;
}

.uwp-search:focus-within {
  border-color: var(--accent);
  background: var(--surface-panel);
}

.uwp-search input {
  flex: 1;
  border: none;
  background: transparent;
  color: var(--text-main);
  outline: none;
  margin-left: 8px;
  font-size: 0.9rem;
}

.search-icon, .clear-icon {
  display: flex;
  align-items: center;
  color: var(--text-sub);
}

.clear-icon {
  cursor: pointer;
  padding: 4px;
}

.uwp-batch {
  display: flex;
  gap: 8px;
}

.batch-btn {
  background: var(--surface-hover);
  color: var(--text-main);
  border: none;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
}
.batch-btn:hover { background: var(--surface-panel); }

/* 列表与卡片设计 */
.uwp-list-wrapper {
  display: flex;
  flex-direction: column;
  gap: 10px; /* 只有间距，没有分割线 */
  flex: 1;
  padding-right: 4px;
}

.uwp-app-item {
  background: var(--surface);
  border-radius: 12px; /* 12px 容器圆角 */
  padding: 12px 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  border: 1px solid transparent;
}

.uwp-app-item:hover {
  background: var(--surface-hover);
  transform: translateX(4px); /* 悬停时轻微右移 */
}

/* 选中态：背景色直接变为 Accent */
.uwp-app-item.active {
  background: var(--accent);
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

/* 左侧信息栏 */
.app-main-content {
  display: flex;
  align-items: center;
  gap: 16px;
  overflow: hidden;
  flex: 1;
}

.app-avatar {
  width: 42px;
  height: 42px;
  background: var(--surface-panel);
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  font-size: 1.3rem;
  color: var(--text-sub);
  flex-shrink: 0;
}
.uwp-app-item.active .app-avatar {
  background: rgba(255, 255, 255, 0.15);
  color: var(--accent-fg);
}

.app-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow: hidden;
}

.app-name {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.uwp-app-item.active .app-name { color: var(--accent-fg); }

.app-pkg {
  font-size: 0.75rem;
  color: var(--text-sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  opacity: 0.7;
}
.uwp-app-item.active .app-pkg { color: var(--accent-fg); opacity: 0.8; }

/* 状态标签：对齐 Proxies 页面的 4px 圆角风格 */
.uwp-status-tag {
  font-size: 0.7rem; /* 中文稍微大一点点 */
  letter-spacing: 0; /* 中文不需要额外的字母间距 */
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 4px; /* 4px 内部标签圆角 */
  text-transform: uppercase;
  transition: all 0.2s;
  
  /* 未选中：中性面板色 */
  background: var(--surface-panel);
  color: var(--text-main);
}

/* 选中态：照抄 Overview 的磨砂半透明遮罩逻辑 */
.uwp-app-item.active .uwp-status-tag {
  background: rgba(255, 255, 255, 0.25) !important;
  color: var(--accent-fg) !important;
}

/* 底部操作 */
.uwp-footer {
  margin-top: 20px;
  padding-top: 10px;
}

.apply-btn {
  width: 100%;
  padding: 14px;
  background: var(--accent);
  color: var(--accent-fg);
  border: none;
  border-radius: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.2s;
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.1);
}

.apply-btn:hover:not(:disabled) {
  filter: brightness(1.1);
  transform: translateY(-1px);
}

.apply-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.loading-spinner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

/* 确保返回按钮内的图标有尺寸 */
.back-btn .icon svg {
  width: 18px;
  height: 18px;
  display: block;
}

/* 针对 UWP 专属的返回图标额外修正 */
.back-icon-svg :deep(svg) {
  width: 18px;
  height: 18px;
}
</style>