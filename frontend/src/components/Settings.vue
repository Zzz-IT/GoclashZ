<template>
  <div class="settings-container">

    <div v-if="view === 'main'" class="settings-page">
      <div class="glass-card setting-group">
        <h3>网络设置</h3>

        <div class="setting-item clickable" @click="view = 'network'">
          <div class="info">
            <h4>基础 network 设置</h4>
            <p>配置内核底层的 TCP 并发、超时以及连接测速逻辑。</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="setting-item clickable" @click="view = 'dns'">
          <div class="info">
            <h4>DNS 服务器配置 (DNS Config)</h4>
            <p>管理防污染解析、Fake-IP 策略以及分流专用的 DNS 群组。</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="setting-item clickable" @click="view = 'tun'">
          <div class="info">
            <h4>虚拟网卡设置 (TUN 模式)</h4>
            <p>管理 Wintun 驱动并开启全局透明代理，接管所有软件流量。</p>
          </div>
          <span class="arrow">➔</span>
        </div>
      </div>

      <div class="glass-card setting-group">
        <h3>应用设置</h3>

        <div class="setting-item clickable" @click="view = 'behavior'">
          <div class="info">
            <h4>应用行为设置 (App Behavior)</h4>
            <p>定制软件启动模式、托盘图标逻辑及订阅请求 User-Agent。</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="setting-item clickable" @click="enterUwpManager">
          <div class="info">
            <h4>UWP 环回免除工具</h4>
            <p>赋予 Windows UWP 应用（如微软商店、邮件）访问本地代理的权限。</p>
          </div>
          <span class="arrow">➔</span>
        </div>

        <div class="setting-item clickable" @click="view = 'update'">
          <div class="info">
            <h4>内核与驱动更新 (Update Center)</h4>
            <p>检查并更新 Mihomo 内核二进制文件及 Wintun 驱动组件。</p>
          </div>
          <span class="arrow">➔</span>
        </div>
      </div>
    </div>

    <div v-else-if="view === 'update'" class="settings-page slide-in">
      <div class="sub-header">
        <button class="back-btn" @click="view = 'main'">
          <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
        </button>
        <h3>组件更新中心</h3>
      </div>

      <div class="glass-card setting-group scrollable">
        <div class="setting-item col-item" style="padding-bottom: 0; align-items: flex-start;">
          <h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">内核与驱动</h3>
        </div>
        <div class="divider" style="margin-top: 10px;"></div>

        <div class="setting-item">
          <div class="info">
            <h4>Mihomo 内核 <span style="color: var(--accent); margin-left: 8px; font-style: italic; font-size: 0.8rem; font-weight: normal;">(更新会短暂断开代理)</span></h4>
            <p>当前版本: {{ coreVersion }}</p>
          </div>
          <button class="action-btn" @click="handleUpdateCore" :disabled="updatingCore">
            {{ updatingCore ? '正在处理...' : '检查更新' }}
          </button>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info">
            <h4>Wintun 驱动 (DLL)</h4>
            <p>当前版本: {{ wintunVersion || '获取中...' }}</p>
          </div>
          <div class="btn-group">
            <button class="action-btn" @click="installDriver(true)" :disabled="isInstalling">
              {{ isReinstallingDriver ? '处理中...' : '重新安装' }}
            </button>
            <button class="action-btn" @click="installDriver(false)" :disabled="isInstalling">
              {{ isCheckingUpdate ? '处理中...' : '检查更新' }}
            </button>
          </div>
        </div>
        
        <div class="divider"></div>

        <div class="setting-item col-item" style="flex-direction: row; justify-content: space-between; align-items: center; padding-bottom: 0; margin-top: 10px;">
          <div class="info">
             <h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">路由规则数据库</h3>
          </div>
          <button class="action-btn primary-btn accent-btn" @click="handleUpdateAllDbs" :disabled="isUpdatingAnyDb">
            {{ updatingAllDbs ? '并发处理中...' : '一键更新全部' }}
          </button>
        </div>
        <div class="divider" style="margin-top: 14px;"></div>

        <template v-for="(db, idx) in dbList" :key="db.key">
          <div class="setting-item">
            <div class="info" style="overflow: hidden;">
              <h4>{{ db.title }} 文件</h4>
              <p class="link-text" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                {{ behavior[db.behaviorKey] || '未配置下载链接' }}
              </p>
              <p v-if="dbFileInfo[db.key]?.exists" style="font-size: 0.75rem; color: var(--text-muted); margin-top: 2px;">
                大小: {{ formatSize(dbFileInfo[db.key].size) }} | 更新于: {{ formatRelativeTime(dbFileInfo[db.key].modTime) }}
              </p>
              <p v-else style="font-size: 0.75rem; color: var(--red-text); margin-top: 2px;">文件不存在，请点击更新同步</p>
            </div>
            <div class="btn-group" style="flex-shrink: 0;">
              <button class="action-btn" @click="openDbEditModal(db.key, behavior[db.behaviorKey])" :disabled="isUpdatingAnyDb">编辑链接</button>
              <button class="action-btn" @click="handleUpdateDb(db.key)" :disabled="isUpdatingAnyDb">
                 {{ updatingDbs[db.key] ? '同步中...' : '更新同步' }}
              </button>
            </div>
          </div>
          <div class="divider" v-if="idx < dbList.length - 1"></div>
        </template>
      </div>
    </div>

    <div class="modal-overlay" v-if="showDbModal">
      <div class="custom-modal-card">
        <div class="modal-header">
          <h3>编辑 {{ dbTitles[editingDb.type] }} 下载链接</h3>
        </div>
        <div class="modal-body">
          <input type="text" class="modal-input" v-model="editingDb.link" style="text-align: left;" @keyup.enter="saveDbLink" />
          <div class="modal-footer">
            <button class="action-btn flex-1" @click="showDbModal = false">取消</button>
            <button class="primary-btn accent-btn flex-1" @click="saveDbLink">保存更改</button>
          </div>
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
          <button class="action-btn" @click="installDriver(false)" :disabled="isInstalling || tunStatus.hasWintun">
            {{ isInstalling ? '处理中...' : (tunStatus.hasWintun ? '已安装' : '安装驱动') }}
          </button>
        </div>

        <div class="divider"></div>

        <div class="setting-item">
          <div class="info"><h4>堆栈 (Stack)</h4></div>
          <ModernSelect 
            v-model="tunConfig.stack" 
            :options="stackOptions" 
            @change="saveTun" 
            :disabled="!tunStatus.hasWintun" 
          />
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
          <ModernSelect 
            v-model="dnsConfig.enhancedMode" 
            :options="enhancedModeOptions" 
            @change="saveDns" 
            :disabled="!dnsConfig.enable" 
          />
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

        <div class="divider"></div>

        <div class="setting-item col-item">
          <div class="info">
            <h4>本地 Hosts 映射 (Hosts)</h4>
            <p>手动指定域名与 IP 的映射关系。对接 DNS 设置中的「使用 Hosts」选项。</p>
          </div>
          <textarea 
            class="modern-textarea" 
            v-model="netConfig.hosts" 
            @blur="saveNet" 
            rows="6" 
            placeholder="'example.com': 127.0.0.1 (请遵循 YAML 键值对格式)"
            style="margin-top: 10px; font-family: var(--font-mono); font-size: 0.85rem;"
          ></textarea>
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
          <ModernSelect 
            v-model="behavior.logLevel" 
            :options="logLevelOptions" 
            @change="saveBehavior" 
          />
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
import ModernSelect from './ModernSelect.vue';

const props = defineProps({
  initialView: {
    type: String,
    default: 'main'
  }
});

const view = ref(props.initialView as 'main' | 'uwp' | 'tun' | 'dns' | 'network' | 'behavior' | 'update');
watch(() => props.initialView, (newVal) => { view.value = newVal as any; });

const coreVersion = ref('读取中...');
const wintunVersion = ref('读取中...');
const isCheckingUpdate = ref(false);
const isReinstallingDriver = ref(false);
const isInstalling = computed(() => isCheckingUpdate.value || isReinstallingDriver.value);
const updatingCore = ref(false);

// 新增数据库更新所需要的变量
const dbList = [
  { key: 'geoip', title: 'GeoIP', behaviorKey: 'geoIpLink' },
  { key: 'geosite', title: 'GeoSite', behaviorKey: 'geoSiteLink' },
  { key: 'mmdb', title: 'MMDB', behaviorKey: 'mmdbLink' },
  { key: 'asn', title: 'ASN', behaviorKey: 'asnLink' },
];
const dbTitles: Record<string, string> = { geoip: 'GeoIP', geosite: 'GeoSite', mmdb: 'MMDB', asn: 'ASN' };

const stackOptions = [
  { label: 'gVisor', value: 'gvisor' },
  { label: 'Mixed', value: 'mixed' },
  { label: 'System', value: 'system' },
  { label: 'LWIP', value: 'lwip' }
];

const enhancedModeOptions = [
  { label: 'Fake-IP', value: 'fake-ip' },
  { label: 'Redir-Host', value: 'redir-host' },
  { label: 'Normal', value: 'normal' }
];

const logLevelOptions = [
  { label: '调试 (Debug)', value: 'debug' },
  { label: '信息 (Info)', value: 'info' },
  { label: '警告 (Warning)', value: 'warning' },
  { label: '错误 (Error)', value: 'error' },
  { label: '静默 (Silent)', value: 'silent' }
];

const showDbModal = ref(false);
const editingDb = ref({ type: '', link: '' });
const updatingDbs = ref<Record<string, boolean>>({});
const dbFileInfo = ref<Record<string, any>>({});

const updatingAllDbs = ref(false);
const isUpdatingAnyDb = computed(() => Object.keys(updatingDbs.value).length > 0 || updatingAllDbs.value);

const formatSize = (bytes: number) => {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const formatRelativeTime = (timestamp: number) => {
  if (!timestamp) return '未知';
  const now = Math.floor(Date.now() / 1000);
  const diff = now - timestamp;

  if (diff < 60) return '刚刚';
  if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前';
  if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前';
  if (diff < 2592000) return Math.floor(diff / 86400) + ' 天前';
  return new Date(timestamp * 1000).toLocaleDateString();
};

const handleUpdateCore = async () => {
  updatingCore.value = true;
  try {
    const res = await (API as any).UpdateCoreComponent();
    // 拦截“已经是最新”的状态码
    if (res === "ALREADY_LATEST") {
      await showAlert(`当前内核 (${coreVersion.value}) 已是最新版本，无需更新！`, "通知");
    } else {
      await showAlert("内核跨版本更新成功！", "通知");
      // 更新成功后刷新显示的版本号
      coreVersion.value = await (API as any).GetCoreVersion();
    }
  } catch (e) {
    await showAlert("更新异常: " + e, "错误");
  } finally {
    updatingCore.value = false;
  }
};
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
  testUrl: 'http://www.gstatic.com/generate_204',
  hosts: ''
});

const behavior = ref<any>({
  silentStart: false,
  closeToTray: true,
  logLevel: 'info',
  hideLogs: false,
  subUA: '',
  activeConfig: '',
  activeMode: '',
  geoIpLink: '',
  geoSiteLink: '',
  mmdbLink: '',
  asnLink: ''
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
    // 动态拉取实际版本号
    const cv = await (API as any).GetCoreVersion();
    if (cv) coreVersion.value = cv;

    const wv = await (API as any).GetWintunVersion();
    if (wv) wintunVersion.value = wv;

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

    // 获取数据库文件信息
    const dbInfo = await (API as any).GetGeoDatabaseInfo();
    if (dbInfo) dbFileInfo.value = dbInfo;
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


const installDriver = async (force: boolean = false) => {
  if (force) isReinstallingDriver.value = true;
  else isCheckingUpdate.value = true;

  try {
    const res = await (API as any).InstallTunDriver(force);
    await loadData();
    
    // 刷新版本号显示
    const wv = await (API as any).GetWintunVersion();
    if (wv) wintunVersion.value = wv;

    if (res === "ALREADY_LATEST") {
       await showAlert(`当前 Wintun 驱动 (${wintunVersion.value}) 已是最新版本！`, '通知');
    } else {
       const msg = force ? "驱动已强制重新安装并覆盖成功！" : `Wintun 驱动 (${wintunVersion.value}) 安装成功，现在可以开启 TUN 模式了！`;
       await showAlert(msg, '安装成功');
    }
  } catch (e) {
    await showAlert('安装提示: ' + e, '发生错误');
  } finally {
    isReinstallingDriver.value = false;
    isCheckingUpdate.value = false;
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

// ------------------------------
// 新增操作函数
// ------------------------------
const openDbEditModal = (type: string, currentLink: string) => {
  editingDb.value = { type, link: currentLink };
  showDbModal.value = true;
};

const saveDbLink = async () => {
  const t = editingDb.value.type;
  const match = dbList.find(d => d.key === t);
  if (match) {
    behavior.value[match.behaviorKey] = editingDb.value.link;
  }
  showDbModal.value = false;
  await saveBehavior();
};

const handleUpdateDb = async (type: string) => {
  if (updatingDbs.value[type] || updatingAllDbs.value) return; // 防止重复点击
  
  updatingDbs.value[type] = true;
  try {
    await (API as any).UpdateGeoDatabase(type);
    await showAlert(`${dbTitles[type]} 文件同步成功！`, "完成");
    // 刷新文件信息
    const dbInfo = await (API as any).GetGeoDatabaseInfo();
    if (dbInfo) dbFileInfo.value = dbInfo;
  } catch (e) {
    await showAlert(`同步异常: ${e}`, "错误");
  } finally {
    delete updatingDbs.value[type];
  }
};

// 终极的一键并发更新方法
const handleUpdateAllDbs = async () => {
  updatingAllDbs.value = true;
  try {
    // 传空数组，代表告诉后端并发更新全部 4 个
    await (API as any).UpdateAllGeoDatabases([]);
    await showAlert("所有规则数据库已极速并发同步至最新！", "完成");
    // 刷新显示的时间和大小
    const dbInfo = await (API as any).GetGeoDatabaseInfo();
    if (dbInfo) dbFileInfo.value = dbInfo;
  } catch (e) {
    await showAlert(`一键同步发生异常:\n${e}`, "错误");
  } finally {
    updatingAllDbs.value = false;
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
.settings-page { display: flex; flex-direction: column; flex: 1; overflow-y: auto; padding-right: 4px; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.scrollable { overflow-y: auto; padding-right: 12px; padding-bottom: 20px; }

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
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }

.arrow { color: var(--text-sub); font-size: 1.2rem; }
.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }

.btn-group {
  display: flex;
  gap: 8px;
}

.modern-input, .modern-textarea { background: var(--surface-hover); border: none; color: var(--text-main); padding: 8px 12px; border-radius: 8px; outline: none; }

/* 专属的极简黑白风 Select 样式 */
.modern-select {
  background-color: var(--surface-hover);
  border: 1px solid transparent;
  color: var(--text-main);
  /* 右侧 padding 设置为 32px，专门给箭头腾出位置，防止文字被遮挡 */
  padding: 8px 32px 8px 12px; 
  border-radius: 8px;
  outline: none;
  cursor: pointer;
  font-size: 0.9rem;
  font-family: inherit;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  
  /* 🗡️ 核心杀招：扒掉系统的原生默认皮肤与灰色方块箭头 */
  appearance: none;
  -webkit-appearance: none;
  
  /* 🎨 注入自定义极简纯色 SVG 箭头 (自适应你主题的深灰色) */
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' viewBox='0 0 24 24' fill='none' stroke='%23777777' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 10px center;
  background-size: 16px;
}

/* 鼠标悬停交互反馈 */
.modern-select:hover:not(:disabled) {
  background-color: var(--surface-panel);
}

/* 点击展开下拉框时的边框反馈，适配黑白高对比度 */
.modern-select:focus {
  border: 1px solid var(--text-sub);
  background-color: var(--surface);
}

/* 禁用状态 */
.modern-select:disabled { 
  opacity: 0.5; 
  cursor: not-allowed; 
}

.modern-input { text-align: right; }
.modern-textarea { resize: vertical; font-family: monospace; font-size: 0.85rem; line-height: 1.5; text-align: left; }
.modern-input:disabled, .modern-textarea:disabled { opacity: 0.5; cursor: not-allowed; }
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

/* 针对新链接和小字 */
.link-text { font-family: monospace; font-size: 0.8rem; color: var(--text-muted); margin-top: 4px; }

/* 悬浮弹窗基础覆盖 */
.modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.4); z-index: 1000; display: flex; align-items: center; justify-content: center; backdrop-filter: blur(5px); }
</style>