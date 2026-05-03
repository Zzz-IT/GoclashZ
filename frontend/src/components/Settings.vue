<template>
  <div class="settings-container">

    <Transition name="slide-fade" mode="out-in">
      <div :key="view" class="settings-view-wrapper">

        <div v-if="view === 'main'" class="settings-page">
          <div class="glass-card setting-group">
            <h3>网络设置</h3>

            <div class="setting-item clickable" @click="view = 'network'">
              <div class="info">
                <h4>基础网络设置</h4>
                <p>配置内核底层的 TCP 并发、超时以及连接测速逻辑。</p>
              </div>
              <span class="arrow">➔</span>
            </div>

            <div class="setting-item clickable" @click="view = 'dns'">
              <div class="info">
                <h4>DNS 服务器配置</h4>
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
                <h4>应用行为设置</h4>
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
                <h4>组件与库更新</h4>
                <p>管理并同步 Mihomo 内核、Wintun 驱动以及 GeoIP/GeoSite 规则数据库。</p>
              </div>
              <span class="arrow">➔</span>
            </div>

            <div class="setting-item clickable" @click="view = 'about'">
              <div class="info">
                <h4>关于应用</h4>
                <p>查看软件版本、进行配置备份还原以及访问 GitHub 开源仓库。</p>
              </div>
              <span class="arrow">➔</span>
            </div>
          </div>
        </div>

        <div v-else-if="view === 'update'" class="settings-page">
          <div class="sub-header">
            <button class="back-btn" @click="view = 'main'">
              <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
            </button>
            <h3>组件与库更新</h3>
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
              <button class="action-btn" @click="handleUpdateCore" :disabled="checkingCoreUpdate || updatingCore">
                {{ checkingCoreUpdate ? '正在检查...' : (updatingCore ? '正在处理...' : '检查更新') }}
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
                  {{ isInstalling ? '处理中...' : '重新安装' }}
                </button>
              </div>
            </div>
            
            <div class="divider"></div>

            <div class="setting-item col-item" style="flex-direction: row; justify-content: space-between; align-items: center; padding-bottom: 0; margin-top: 10px;">
              <div class="info">
                <h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">路由规则数据库</h3>
              </div>
              <button class="action-btn primary-btn accent-btn" @click="handleUpdateAllDbs" :disabled="updatingAllDbs">
                {{ updatingAllDbs ? '处理中...' : '更新全部' }}
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
                  <button class="action-btn" @click="openDbEditModal(db.key, behavior[db.behaviorKey])" :disabled="updatingDbs[db.key]">编辑链接</button>
                  <button class="action-btn" @click="handleUpdateDb(db.key)" :disabled="updatingDbs[db.key]">
                    {{ updatingDbs[db.key] ? '同步中...' : '更新同步' }}
                  </button>
                </div>
              </div>
              <div class="divider" v-if="idx < dbList.length - 1"></div>
            </template>
          </div>
        </div>

        <div v-else-if="view === 'tun'" class="settings-page">
          <div class="sub-header section-header">
            <button class="back-btn" @click="view = 'main'">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
            </button>
            <h3>虚拟网卡配置</h3>
            <button class="action-btn accent-btn mini-btn-reset" @click="confirmReset('tun')">
              <span class="btn-icon" v-html="ICONS.refresh"></span> 重置
            </button>
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
              <button class="action-btn" @click="installDriver(true)" :disabled="isInstalling || tunStatus.hasWintun">
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
              <ModernNumberInput 
                v-model="tunConfig.mtu" 
                :min="576" 
                :max="1500" 
                @change="saveTun" 
                :disabled="!tunStatus.hasWintun" 
              />
            </div>

          </div>
        </div>

        <div v-else-if="view === 'dns'" class="settings-page">
          <div class="sub-header section-header">
            <button class="back-btn" @click="view = 'main'">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
            </button>
            <h3>DNS 服务器配置</h3>
            <button class="action-btn accent-btn mini-btn-reset" @click="confirmReset('dns')">
              <span class="btn-icon" v-html="ICONS.refresh"></span> 重置
            </button>
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

        <div v-else-if="view === 'network'" class="settings-page">
          <div class="sub-header section-header">
            <button class="back-btn" @click="view = 'main'">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
            </button>
            <h3>基础网络配置</h3>
            <button class="action-btn accent-btn mini-btn-reset" @click="confirmReset('network')">
              <span class="btn-icon" v-html="ICONS.refresh"></span> 重置
            </button>
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
                <h4>允许局域网连接 (Allow LAN)</h4>
                <p>开启后将允许局域网内其他设备通过此代理上网。</p>
              </div>
              <label class="modern-switch">
                <input type="checkbox" v-model="netConfig.allowLan" @change="saveNet">
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

            <Transition name="dropdown">
              <div v-if="netConfig.tcpKeepAlive" class="tcp-keep-alive-sub-items">
                <div class="divider"></div>
                <div class="setting-item">
                  <div class="info">
                    <h4>发送时间间隔 (Interval)</h4>
                    <p>单位为秒，建议值 15-30s</p>
                  </div>
                  <div class="input-with-unit">
                    <ModernNumberInput 
                      v-model="netConfig.tcpKeepAliveInterval" 
                      :min="1" 
                      :max="3600" 
                      @change="saveNet" 
                    />
                    <span class="unit">s</span>
                  </div>
                </div>
              </div>
            </Transition>

            <div class="divider"></div>

            <div class="setting-item col-item">
              <div class="info">
                <h4>延迟测试网址 (Delay Test URL)</h4>
                <p>内核进行连接可用性测试时使用的 URL。建议使用 Google 或 Cloudflare 的测速地址。</p>
              </div>
              <input 
                type="text" 
                class="modern-input" 
                style="text-align: left; width: 100%; margin-top: 12px; font-size: 0.95rem; padding: 12px 16px;" 
                v-model="netConfig.testUrl" 
                @blur="saveNet" 
                placeholder="http://www.gstatic.com/generate_204" 
              />
            </div>
            <div class="divider"></div>

            <div class="setting-item col-item">
              <div class="info">
                <h4>外部控制地址 (External Controller)</h4>
                <p>内核 REST API 的监听地址。默认只允许本机 127.0.0.1 访问，不建议修改。</p>
              </div>
              <input 
                type="text" 
                class="modern-input" 
                style="text-align: left; width: 100%; margin-top: 12px; font-size: 0.95rem; padding: 12px 16px;" 
                v-model="netConfig.externalController" 
                @blur="saveNet" 
                placeholder="127.0.0.1:9090" 
              />
            </div>

            <div class="divider"></div>

            <div class="setting-item col-item">
              <div class="info">
                <h4>本地 Hosts 映射 (Hosts)</h4>
                <p>手动指定域名与 IP 的映射关系。对接 DNS 设置中的「使用 Hosts」选项。</p>
              </div>
              <div class="hosts-input-container">
                <textarea 
                  class="modern-textarea" 
                  v-model="netConfig.hosts" 
                  @blur="saveNet" 
                  rows="6" 
                  placeholder="'example.com': 127.0.0.1 (请遵循 YAML 键值对格式)"
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
          </div>
        </div>

        <div v-else-if="view === 'behavior'" class="settings-page">
          <div class="sub-header section-header">
            <button class="back-btn" @click="view = 'main'">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
            </button>
            <h3>应用行为设置</h3>
            <button class="action-btn accent-btn mini-btn-reset" @click="confirmReset('behavior')">
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
                <h4>自动延迟测速</h4>
                <p>启用后，将按设定的时间间隔在后台自动更新节点延迟。</p>
              </div>
              <label class="modern-switch">
                <input type="checkbox" v-model="behavior.autoDelayTest" @change="saveBehavior">
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
                      @change="saveBehavior"
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
                <input type="checkbox" v-model="behavior.colorDelay" @change="saveBehavior">
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
                <input type="checkbox" v-model="behavior.delayRetention" @change="saveBehavior">
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
                    @change="saveBehavior" 
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

        <div v-else-if="view === 'about'" class="settings-page">
          <div class="sub-header section-header">
            <button class="back-btn" @click="view = 'main'">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
            </button>
            <h3>关于应用</h3>
          </div>

          <div class="glass-card setting-group scrollable">
            <!-- 软件图标与名称展示行 -->
            <div class="setting-item" style="padding: 20px 0;">
              <div class="info" style="display: flex; align-items: center; gap: 18px;">
                <img :src="appLogo" style="width: 52px; height: 52px; border-radius: 12px;" />
                <h4 style="margin: 0; font-weight: 800; font-size: 1.6rem; letter-spacing: -0.01em;">GoclashZ</h4>
              </div>
            </div>

            <div class="divider"></div>
            <div class="setting-item">
              <div class="info">
                <h4>软件版本</h4>
                <p>{{ globalState.appVersion || '获取中...' }}</p>
              </div>
              <button class="action-btn accent-btn" @click="handleCheckUpdate" :disabled="globalState.appUpdateChecking">
                {{ globalState.appUpdateChecking ? '检查中...' : '检查更新' }}
              </button>
            </div>

            <div class="divider"></div>

            <div class="setting-item">
              <div class="info">
                <h4>自动更新</h4>
                <p>允许软件自动检查并提示新版本。</p>
              </div>
              <label class="modern-switch">
                <input type="checkbox" v-model="behavior.autoUpdate" @change="saveBehavior" />
                <span class="slider"></span>
              </label>
            </div>

            <Transition name="dropdown">
              <div v-if="behavior.autoUpdate" class="auto-update-sub-items">
                <div class="divider"></div>
                <div class="setting-item">
                  <div class="info">
                    <h4>检查更新方式</h4>
                  </div>
                  <ModernSelect 
                    v-model="behavior.updateMethod" 
                    :options="[
                      { label: '每次启动', value: 'startup' }, 
                      { label: '定时', value: 'scheduled' }
                    ]" 
                    @change="saveBehavior" 
                  />
                </div>

                <div class="divider"></div>
                <div class="setting-item" :class="{ 'disabled-fade': behavior.updateMethod !== 'scheduled' }">
                  <div class="info">
                    <h4>检查间隔时间</h4>
                  </div>
                  <div class="input-with-unit">
                    <ModernNumberInput 
                      v-model="behavior.updateInterval" 
                      :min="1"
                      :max="365"
                      :disabled="behavior.updateMethod !== 'scheduled'" 
                      @change="saveBehavior"
                    />
                    <span class="unit">天</span>
                  </div>
                </div>
              </div>
            </Transition>

            <div class="divider"></div>

            <div class="setting-item">
              <div class="info">
                <h4>本地配置备份</h4>
                <p>将订阅、应用设置及主题打包导出为 .gocz 文件</p>
              </div>
              <button class="action-btn accent-btn" @click="handleExportBackup">导出备份</button>
            </div>

            <div class="divider"></div>

            <div class="setting-item">
              <div class="info">
                <h4>还原备份</h4>
                <p>从 .gocz 文件恢复数据，订阅配置将采用智能合并模式</p>
              </div>
              <button class="action-btn accent-btn" @click="openRestoreModal">还原备份</button>
            </div>

            <div class="divider"></div>

            <div class="setting-item">
              <div class="info">
                <h4>GitHub 仓库</h4>
                <a href="javascript:void(0)" @click="openLink('https://github.com/Zzz-IT/GoclashZ')" class="link-item">https://github.com/Zzz-IT/GoclashZ</a>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="view === 'uwp'" class="settings-page">

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
    </Transition>

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
    <!-- 统一模态框系统 -->
    <Transition name="pop">
      <div v-if="showResetConfirm" class="modal-overlay" @click="showResetConfirm = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3 class="danger-text">确认重置</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">确定要将 <strong>{{ resetModuleName }}</strong> 恢复为默认设置吗？此操作不可撤销，程序将重新加载配置。</p>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showResetConfirm = false">取消</button>
              <button class="primary-btn accent-btn red-text-btn flex-1" @click="handleReset">确认重置</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    
    <Transition name="pop">
      <div v-if="showCoreUpdateConfirm" class="modal-overlay" @click="showCoreUpdateConfirm = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>发现新版本</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">
              检测到 Mihomo 内核新版本 <strong>{{ coreUpdateInfo.remote }}</strong>，当前版本为 <strong>{{ coreUpdateInfo.local }}</strong>。<br/><br/>
              更新内核将会短暂断开代理连接。是否立即更新？
            </p>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showCoreUpdateConfirm = false">取消</button>
              <button class="primary-btn accent-btn flex-1" @click="executeCoreUpdate">立即更新</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 还原备份弹窗 (复用订阅管理的卡片样式) -->
    <Transition name="pop">
      <div v-if="showRestoreModal" class="modal-overlay" @click.self="showRestoreModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>还原本地数据</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">请选择备份文件并设置还原模式：</p>
            
            <div class="restore-actions" style="width: 100%; display: flex; flex-direction: column; gap: 4px;">
              <button class="action-btn w-full-btn hover-accent" @click="handleSelectFile" :class="{'active-border': selectedPath}" style="width: 100%; box-sizing: border-box;">
                <span class="btn-icon" v-html="ICONS.folder" style="margin-right: 4px;"></span>
                <span class="truncate" style="flex: 1; text-align: center;">
                  {{ selectedPath ? '已选择: ' + selectedPath.split('\\').pop() : '浏览备份文件 (.gocz)' }}
                </span>
              </button>
              
              <div class="divider-text" style="margin: 12px 0">配置还原模式</div>
              
              <div class="mode-selector-group" style="width: 100%;">
                <ModernSelect 
                  v-model="restoreMode" 
                  :options="[
                    { label: '全部恢复 (包含订阅与软件设置)', value: 'all' },
                    { label: '仅恢复订阅配置 (合并入现有列表)', value: 'subs' },
                    { label: '仅恢复软件设置 (包含主题/日志)', value: 'settings' }
                  ]"
                  class="w-full"
                  style="height: 44px;"
                />
              </div>
            </div>

            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showRestoreModal = false">取消</button>
              <button class="primary-btn accent-btn flex-1" :disabled="!selectedPath" @click="confirmRestore">执行还原</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { BrowserOpenURL, EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { showAlert, globalState } from '../store';
import { ICONS } from '../utils/icons';
import appLogo from '../assets/logo.ico';
import ModernSelect from './ModernSelect.vue';
import ModernNumberInput from './ModernNumberInput.vue';

const openLink = (url: string) => {
  BrowserOpenURL(url);
};

const showResetConfirm = ref(false);
const resetModule = ref('');
const resetModuleName = ref('');
const hostsError = ref('');

const checkingCoreUpdate = ref(false);
const showCoreUpdateConfirm = ref(false);
const coreUpdateInfo = ref({ local: '', remote: '', releaseUrl: '' });

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
      hostsError.value = `第 ${i + 1} 行格式错误：请使用 "域名: IP" 的格式 (注意冒号为英文且要有值)`;
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


// --- 备份与还原状态 ---
const showRestoreModal = ref(false);
const selectedPath = ref("");
const restoreMode = ref("all");

const modules: Record<string, string> = {
  'network': '基础网络设置',
  'dns': 'DNS 服务器设置',
  'tun': '虚拟网卡设置',
  'behavior': '应用行为设置'
};

const confirmReset = (mod: string) => {
  resetModule.value = mod;
  resetModuleName.value = modules[mod];
  showResetConfirm.value = true;
};

const handleReset = async () => {
  try {
    await API.ResetComponentSettings(resetModule.value);
    showResetConfirm.value = false;
    
    // 重新拉取对应数据以更新 UI
    if (resetModule.value === 'network') {
        const netConf = await (API.GetNetworkConfig as any)();
        if (netConf) netConfig.value = netConf;
    }
    if (resetModule.value === 'dns') {
        const dnsConf = await (API.GetDNSConfig as any)();
        if (dnsConf) dnsConfig.value = dnsConf;
    }
    if (resetModule.value === 'tun') {
        const tunConf = await API.GetTunConfig();
        if (tunConf) tunConfig.value = tunConf;
    }
    if (resetModule.value === 'behavior') {
        const bh = await API.GetAppBehavior();
        if (bh) behavior.value = bh;
    }
    showAlert(`${resetModuleName.value} 已重置为默认值`, "成功");
  } catch (e) {
    console.error("重置失败:", e);
    showAlert("重置失败: " + e, "错误");
  }
};

const props = defineProps({
  initialView: {
    type: String,
    default: 'main'
  }
});

const view = ref(props.initialView as 'main' | 'uwp' | 'tun' | 'dns' | 'network' | 'behavior' | 'update' | 'about');
watch(() => props.initialView, (newVal) => { view.value = newVal as any; });

watch(view, async (v) => {
  if (v === 'update') {
    await refreshComponentInfo();
  }
});

const coreVersion = ref('读取中...');
const wintunVersion = ref('读取中...');
const isInstalling = ref(false);
const updatingCore = ref(false);

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
  { label: '调试', value: 'debug' },
  { label: '信息', value: 'info' },
  { label: '警告', value: 'warning' },
  { label: '错误', value: 'error' },
  { label: '静默', value: 'silent' }
];

const showDbModal = ref(false);
const editingDb = ref({ type: '', link: '' });
const updatingDbs = ref<Record<string, boolean>>({});
const componentFileInfo = ref<Record<string, any>>({});
const dbFileInfo = ref<Record<string, any>>({});

const updatingAllDbs = ref(false);

const refreshComponentFileInfo = async () => {
  const info = await (API as any).GetComponentFileInfo();
  componentFileInfo.value = info || {};
  dbFileInfo.value = {
    geoip: info?.geoip || {},
    geosite: info?.geosite || {},
    mmdb: info?.mmdb || {},
    asn: info?.asn || {},
  };
};

const refreshComponentInfo = async () => {
  coreVersion.value = await (API as any).GetCoreVersion();
  wintunVersion.value = await (API as any).GetWintunVersion();
  await refreshComponentFileInfo();
};

const isUpdatingAnyDb = computed(() => {
  return updatingAllDbs.value || Object.values(updatingDbs.value).some(Boolean);
});

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

const formatUpdateError = (err: any) => {
  let msg = String(err || '');
  // 清洗超长 GitHub Release 资产 Signed URL
  msg = msg.replace(/https:\/\/release-assets\.githubusercontent\.com\/\S+/g, 'GitHub Release 资产下载地址');
  
  // 清洗普通 GitHub Release 下载地址，移除 query
  msg = msg.replace(/https:\/\/github\.com\/\S+\/releases\/download\/\S+/g, (match) => {
    try {
      const url = new URL(match);
      url.search = '';
      return url.toString();
    } catch (e) {
      return 'GitHub Release 下载地址';
    }
  });

  // 移除常见的签名敏感参数
  msg = msg.replace(/([?&](sp|sv|se|sr|sig|skoid|sktid|skt|ske|sks|skv)=[^\\s]+)/g, '');

  if (msg.length > 360) {
    msg = msg.slice(0, 360) + '...';
  }
  return msg;
};

// --- UI 增强助手：队列、去抖、通知聚合 ---

// 1. 弹窗队列：防止多个弹窗冲突或卡死事件循环
type ModalJob = {
  title: string;
  message: string;
  danger?: boolean;
};
const modalQueue: ModalJob[] = [];
let modalShowing = false;

const enqueueModal = async (job: ModalJob) => {
  modalQueue.push(job);
  if (modalShowing) return;
  modalShowing = true;
  while (modalQueue.length > 0) {
    const current = modalQueue.shift()!;
    await showAlert(current.message, current.title, !!current.danger);
  }
  modalShowing = false;
};

// 2. Geo 通知聚合：连续单点更新时，合并为一个通知
type GeoNotice = {
  key: string;
  status: 'success' | 'error';
  error?: string;
};
const geoKeys = ["geoip", "geosite", "mmdb", "asn"];
const geoNameMap: Record<string, string> = {
  geoip: 'GeoIP',
  geosite: 'GeoSite',
  mmdb: 'MMDB',
  asn: 'ASN',
};
const pendingGeoNotices: GeoNotice[] = [];
let geoNoticeTimer: number | undefined;

const pushGeoNotice = (notice: GeoNotice) => {
  pendingGeoNotices.push(notice);
  if (geoNoticeTimer) window.clearTimeout(geoNoticeTimer);
  geoNoticeTimer = window.setTimeout(() => {
    flushGeoNotices();
  }, 350);
};

const flushGeoNotices = () => {
  geoNoticeTimer = undefined;
  const notices = pendingGeoNotices.splice(0);
  if (!notices.length) return;

  const errors = notices.filter(n => n.status === 'error');
  const successes = notices.filter(n => n.status === 'success');

  if (errors.length > 0) {
    const message = errors
      .map(n => `${geoNameMap[n.key] || n.key}: ${n.error || '更新失败'}`)
      .join('\n');
    void enqueueModal({ title: '数据库更新失败', message, danger: true });
    return;
  }

  if (successes.length > 0) {
    const names = successes.map(n => geoNameMap[n.key] || n.key).join('、');
    void enqueueModal({ title: '通知', message: `${names} 更新完成。`, danger: false });
  }
};

// 3. 文件信息刷新去抖
let componentInfoRefreshTimer: number | undefined;
let componentInfoRefreshing = false;
let componentInfoRefreshPending = false;

const queueComponentInfoRefresh = () => {
  if (componentInfoRefreshTimer) window.clearTimeout(componentInfoRefreshTimer);
  componentInfoRefreshTimer = window.setTimeout(() => {
    void refreshComponentInfoSafely();
  }, 200);
};

const refreshComponentInfoSafely = async () => {
  if (componentInfoRefreshing) {
    componentInfoRefreshPending = true;
    return;
  }
  componentInfoRefreshing = true;
  try {
    await refreshComponentInfo();
  } finally {
    componentInfoRefreshing = false;
    if (componentInfoRefreshPending) {
      componentInfoRefreshPending = false;
      queueComponentInfoRefresh();
    }
  }
};

const syncGeoActiveState = (active: string[] = []) => {
  const activeSet = new Set(active || []);
  geoKeys.forEach((key) => {
    updatingDbs.value[key] = activeSet.has(key);
  });
  if (activeSet.size === 0) {
    updatingAllDbs.value = false;
  }
};

const refreshGeoActiveState = async () => {
  try {
    const active = await (API as any).GetActiveGeoUpdates();
    syncGeoActiveState(active || []);
  } catch {
    syncGeoActiveState([]);
  }
};

const handleCheckUpdate = async () => {
  if (globalState.appUpdateChecking) return;
  try {
    // 🚀 核心改进：调用异步静默下载流，将通知权交给全局监听器 (App.vue)
    await (API as any).CheckAndDownloadAppUpdateAsync();
  } catch (e) {
    await showAlert("检查更新失败: " + e, "错误", true);
  }
};

// 导出备份
const handleExportBackup = async () => {
  try {
    const res = await (API as any).ExportBackup();
    if (res === "SUCCESS") {
      await showAlert("备份成功导出", "通知");
    }
  } catch (e) {
    await showAlert("导出失败: " + String(e), "错误");
  }
};

// 打开还原面板
const openRestoreModal = () => {
  selectedPath.value = "";
  restoreMode.value = "all";
  showRestoreModal.value = true;
};

// 选择还原文件
const handleSelectFile = async () => {
  try {
    const path = await (API as any).SelectBackupFile();
    if (path) {
      selectedPath.value = path;
    }
  } catch (e) {
    console.error("选择文件取消或失败", e);
  }
};

// 执行还原
const confirmRestore = async () => {
  try {
    const res = await (API as any).ExecuteRestore(selectedPath.value, restoreMode.value);
    if (res === "SUCCESS") {
      showRestoreModal.value = false;
      await showAlert("数据还原成功！设置及配置已即时生效。", "成功");
    }
  } catch (e) {
    await showAlert("还原失败: " + String(e), "错误");
  }
};

const handleUpdateCore = async () => {
  if (checkingCoreUpdate.value || updatingCore.value) return;
  (API as any).CheckCoreUpdateAsync();
};

const executeCoreUpdate = () => {
  showCoreUpdateConfirm.value = false;
  if (updatingCore.value) return;
  (API as any).UpdateCoreComponentAsync();
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
  allowLan: false,
  externalController: '127.0.0.1:9090',
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
  // 👇 新增：显色彩色延迟数字
  colorDelay: false,
  delayRetention: false,          // 👇 移到了这里
  delayRetentionTime: 'long',     // 👇 移到了这里
  logLevel: 'info',
  hideLogs: false,
  subUA: '',
  activeConfig: '',
  activeMode: '',
  geoIpLink: '',
  geoSiteLink: '',
  mmdbLink: '',
  asnLink: '',
  // 👇 新增：自动更新相关
  autoUpdate: true,
  updateMethod: 'startup',
  updateInterval: 3,
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
    const cv = await (API as any).GetCoreVersion();
    if (cv) coreVersion.value = cv;

    const wv = await (API as any).GetWintunVersion();
    if (wv) wintunVersion.value = wv;

    const status = await API.CheckTunEnv();
    tunStatus.value = status;
    const tunConf = await API.GetTunConfig();
    if (tunConf) tunConfig.value = tunConf;

    tunConfig.value.enable = globalState.tun;

    const dnsConf = await (API.GetDNSConfig as any)();
    if (dnsConf) dnsConfig.value = dnsConf;

    const netConf = await (API.GetNetworkConfig as any)();
    if (netConf) netConfig.value = netConf;

    const behaviorConf = await (API.GetAppBehavior as any)();
    if (behaviorConf) behavior.value = behaviorConf;

    const info = await (API as any).GetComponentFileInfo();
    if (info) {
      componentFileInfo.value = info;
      dbFileInfo.value = {
        geoip: info?.geoip || {},
        geosite: info?.geosite || {},
        mmdb: info?.mmdb || {},
        asn: info?.asn || {},
      };
    }
  } catch (e) {
    console.error('加载配置失败', e);
  }
};

onMounted(() => { 
  loadData(); 

  // 🌟 3. 监听手动触发的 Geo 数据库更新事件
  geoKeys.forEach((key) => {
    EventsOn(`geo-update-${key}-start`, () => {
      updatingDbs.value[key] = true;
    });

    EventsOn(`geo-update-${key}-success`, () => {
      updatingDbs.value[key] = false;
      queueComponentInfoRefresh();
      if (updatingAllDbs.value) return;
      pushGeoNotice({ key, status: 'success' });
    });

    EventsOn(`geo-update-${key}-error`, (err: string) => {
      updatingDbs.value[key] = false;
      queueComponentInfoRefresh();
      if (updatingAllDbs.value) {
        console.warn(`[GeoUpdate] ${key} failed during bulk update:`, err);
        return;
      }
      pushGeoNotice({ key, status: 'error', error: formatUpdateError(err) });
    });

    EventsOn(`geo-update-${key}-cancelled`, () => {
      updatingDbs.value[key] = false;
      queueComponentInfoRefresh();
    });

    EventsOn(`geo-update-${key}-busy`, () => {
      // 已在更新中，静默即可
    });
  });

  // 🌟 4. 监听“更新全部”聚合任务
  EventsOn("geo-update-all-start", () => {
    updatingAllDbs.value = true;
    geoKeys.forEach(k => updatingDbs.value[k] = true);
  });

  EventsOn("geo-update-all-success", () => {
    updatingAllDbs.value = false;
    geoKeys.forEach(k => updatingDbs.value[k] = false);
    queueComponentInfoRefresh();
    void enqueueModal({ title: '通知', message: '全部路由规则数据库更新完成。', danger: false });
  });

  EventsOn("geo-update-all-error", (err: string) => {
    updatingAllDbs.value = false;
    geoKeys.forEach(k => updatingDbs.value[k] = false);
    queueComponentInfoRefresh();
    void enqueueModal({
      title: '错误',
      message: '部分数据库更新失败，已保留原有文件：' + formatUpdateError(err),
      danger: true
    });
  });

  EventsOn("geo-update-all-cancelled", () => {
    updatingAllDbs.value = false;
    geoKeys.forEach(k => updatingDbs.value[k] = false);
    queueComponentInfoRefresh();
  });

  // 🌟 5. 后端状态兜底同步：确保 UI 状态始终与后端一致
  EventsOn("geo-update-active-sync", (active: string[]) => {
    const activeSet = new Set(active || []);
    geoKeys.forEach((key) => {
      updatingDbs.value[key] = activeSet.has(key);
    });
    if (activeSet.size === 0) {
      updatingAllDbs.value = false;
    }
  });

  // 监听 Core 检查更新事件
  EventsOn("core-update-check-start", () => { checkingCoreUpdate.value = true; });
  EventsOn("core-update-check-error", (err: string) => {
    checkingCoreUpdate.value = false;
    void enqueueModal({ title: '错误', message: '检查内核更新失败: ' + formatUpdateError(err), danger: true });
  });
  EventsOn("core-update-none", (data: any) => {
    checkingCoreUpdate.value = false;
    void enqueueModal({ title: '检查更新', message: `内核已是最新版本 (${data.local})。`, danger: false });
  });
  EventsOn("core-update-available", (data: any) => {
    checkingCoreUpdate.value = false;
    coreUpdateInfo.value = {
      local: data.local || '',
      remote: data.remote || '',
      releaseUrl: data.releaseUrl || ''
    };
    showCoreUpdateConfirm.value = true;
  });

  // 监听 Core 更新事件 (对应 backend: "core-update")
  EventsOn("core-update-start", () => { updatingCore.value = true; });
  EventsOn("core-version-updated", (payload: any) => {
    coreVersion.value = payload?.version || coreVersion.value;
  });
  EventsOn("core-update-success", () => {
    updatingCore.value = false;
    queueComponentInfoRefresh();
    void enqueueModal({ title: '通知', message: 'Mihomo 内核更新完成。', danger: false });
  });
  EventsOn("core-update-error", (err: string) => {
    updatingCore.value = false;
    queueComponentInfoRefresh();
    void enqueueModal({ title: '错误', message: 'Mihomo 内核更新失败: ' + formatUpdateError(err), danger: true });
  });
  EventsOn("core-update-cancelled", () => {
    updatingCore.value = false;
  });

  // 监听 Tun 驱动安装事件 (对应 backend: "driver-install")
  EventsOn("driver-install-start", () => { 
    isInstalling.value = true;
  });
  EventsOn("wintun-version-updated", (payload: any) => {
    wintunVersion.value = payload?.version || wintunVersion.value;
  });
  EventsOn("driver-install-success", async () => {
    isInstalling.value = false;
    const status = await API.CheckTunEnv();
    tunStatus.value = status as any;
    queueComponentInfoRefresh();
    void enqueueModal({ title: '通知', message: 'Wintun 驱动安装完成。', danger: false });
  });
  EventsOn("driver-install-error", (err: string) => {
    isInstalling.value = false;
    queueComponentInfoRefresh();
    void enqueueModal({ title: '错误', message: 'Wintun 驱动安装失败: ' + formatUpdateError(err), danger: true });
  });
  EventsOn("driver-install-cancelled", () => {
    isInstalling.value = false;
  });
});

onUnmounted(() => {
  geoKeys.forEach(t => {
    EventsOff(`geo-update-${t}-start`);
    EventsOff(`geo-update-${t}-success`);
    EventsOff(`geo-update-${t}-error`);
    EventsOff(`geo-update-${t}-cancelled`);
    EventsOff(`geo-update-${t}-busy`);
  });
  EventsOff("geo-update-all-start");
  EventsOff("geo-update-all-success");
  EventsOff("geo-update-all-error");
  EventsOff("geo-update-all-cancelled");
  EventsOff("geo-update-active-sync");

  EventsOff("core-update-check-start");
  EventsOff("core-update-check-error");
  EventsOff("core-update-none");
  EventsOff("core-update-available");
  EventsOff("core-update-start");
  EventsOff("core-version-updated");
  EventsOff("core-update-success");
  EventsOff("core-update-error");
  EventsOff("core-update-cancelled");

  EventsOff("driver-install-start");
  EventsOff("wintun-version-updated");
  EventsOff("driver-install-success");
  EventsOff("driver-install-error");
  EventsOff("driver-install-cancelled");
});

const handleTunToggle = async (e: Event) => {
  if (tunConfig.value.enable && !tunStatus.value.hasWintun) {
    e.preventDefault();
    tunConfig.value.enable = false;
    await showAlert('无法开启 TUN 模式：\n请先点击下方的“安装驱动”按钮下载并配置 wintun.dll。', '缺少依赖');
    return;
  }
  
  const originalValue = !tunConfig.value.enable;
  
  try {
    await API.ToggleTunMode(tunConfig.value.enable);
    await saveTun();
  } catch (err) {
    tunConfig.value.enable = originalValue; 
    await showAlert("操作内核 TUN 失败: " + err, '错误');
  }
};


const installDriver = async (force: boolean = true) => {
  if (isInstalling.value) return;
  (API as any).InstallTunDriverAsync(force);
};
watch(view, async (v) => {
  if (v === 'update') {
    await refreshComponentInfoSafely();
    await refreshGeoActiveState();
  }
});

// 🚀 核心：监听更新间隔时间，防止用户输入 0 或负数
watch(() => behavior.value.updateInterval, async (newVal) => {
  if (newVal !== undefined && newVal <= 0) {
    behavior.value.updateInterval = 1;
    
    // 👇 修复：只有在用户实际启用了定时更新的情况下，才弹出警告。
    // 如果是旧版本配置缺失导致的 0，则静默修复并保存，不打扰用户。
    if (behavior.value.autoUpdate && behavior.value.updateMethod === 'scheduled') {
      await showAlert("检查更新间隔不能小于 1 天。", "配置提示");
    }
    
    saveBehavior();
  }
});

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

const handleUpdateDb = async (key: string) => {
  if (updatingDbs.value[key]) {
    await refreshGeoActiveState();
    if (updatingDbs.value[key]) return;
  }

  // 乐观置位，确保 UI 响应
  updatingDbs.value[key] = true;

  try {
    await (API as any).UpdateGeoDatabaseAsync(key);
  } catch (e) {
    updatingDbs.value[key] = false;
    void enqueueModal({
      title: '错误',
      message: `${geoNameMap[key] || key} 更新启动失败：${formatUpdateError(e)}`,
      danger: true,
    });
  }
};

const handleUpdateAllDbs = async () => {
  if (updatingAllDbs.value) {
    await refreshGeoActiveState();
    if (updatingAllDbs.value) return;
  }

  updatingAllDbs.value = true;
  geoKeys.forEach((key) => {
    updatingDbs.value[key] = true;
  });

  try {
    await API.UpdateAllGeoDatabasesAsync();
  } catch (e) {
    updatingAllDbs.value = false;
    geoKeys.forEach((key) => {
      updatingDbs.value[key] = false;
    });

    void enqueueModal({
      title: '错误',
      message: '全部更新启动失败：' + formatUpdateError(e),
      danger: true,
    });
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
.settings-container { 
  display: flex; 
  flex-direction: column; 
  height: 100%; 
  overflow: hidden; 
  position: relative; 
}

.settings-view-wrapper {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
}
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

.modern-input, .modern-textarea { 
  background: var(--surface-hover); 
  border: none; 
  color: var(--text-main); 
  padding: 10px 14px; 
  border-radius: 8px; 
  outline: none; 
  font-size: 0.9rem;
}

.modern-select {
  background-color: var(--surface-hover);
  border: 1px solid transparent;
  color: var(--text-main);
  padding: 8px 32px 8px 12px; 
  border-radius: 8px;
  outline: none;
  cursor: pointer;
  font-size: 0.9rem;
  font-family: inherit;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  appearance: none;
  -webkit-appearance: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' viewBox='0 0 24 24' fill='none' stroke='%23777777' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 10px center;
  background-size: 16px;
}

.modern-select:hover:not(:disabled) {
  background-color: var(--surface-panel);
}

.modern-select:focus {
  border: 1px solid var(--text-sub);
  background-color: var(--surface);
}

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
.unit { font-size: 0.85rem; color: var(--text-sub); font-family: var(--font-mono); font-weight: 500; }
.status-msg { margin-top: 4px; font-weight: 500; }
.green-text { color: var(--text-main); font-weight: 600; }
.red-text { color: var(--text-muted); }

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

.uwp-list-wrapper {
  display: flex;
  flex-direction: column;
  gap: 10px; 
  flex: 1;
  padding-right: 4px;
}

.uwp-app-item {
  background: var(--surface);
  border-radius: 12px; 
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
}

.uwp-app-item.active {
  background: var(--accent);
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

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

.uwp-status-tag {
  font-size: 0.7rem; 
  letter-spacing: 0; 
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 4px; 
  text-transform: uppercase;
  transition: all 0.2s;
  background: var(--surface-panel);
  color: var(--text-main);
}

.uwp-app-item.active .uwp-status-tag {
  background: rgba(255, 255, 255, 0.25) !important;
  color: var(--accent-fg) !important;
}

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

.back-btn .icon svg {
  width: 18px;
  height: 18px;
  display: block;
}

.back-icon-svg :deep(svg) {
  width: 18px;
  height: 18px;
}

.link-text { font-family: monospace; font-size: 0.8rem; color: var(--text-muted); margin-top: 4px; }

/* 统一弹窗按钮高度与宽度 */
.modal-footer .action-btn, 
.modal-footer .primary-btn {
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.slide-fade-enter-active,
.slide-fade-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  width: 100%;
  height: 100%;
}

.slide-fade-enter-from {
  opacity: 0;
  transform: translateX(12px); 
}

.slide-fade-leave-to {
  opacity: 0;
  transform: translateX(-12px); 
}
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

.w-full-btn { width: 100%; justify-content: center; }
.divider-text { 
  display: flex; align-items: center; text-align: center; color: var(--text-sub); font-size: 0.75rem; 
  font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; margin: 15px 0;
}
.divider-text::before, .divider-text::after { content: ''; flex: 1; border-bottom: 1px solid var(--surface-hover); }
.divider-text::before { margin-right: 10px; }
.divider-text::after { margin-left: 10px; }

.restore-actions { display: flex; flex-direction: column; }
.active-border { border: 1px solid var(--accent) !important; }
.w-full { width: 100%; }

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

/* 关于页面的超链接样式 */
.link-item {
  color: var(--accent);
  font-size: 0.85rem;
  text-decoration: none;
  transition: opacity 0.2s;
  cursor: pointer;
}
.link-item:hover {
  opacity: 0.8;
  text-decoration: underline;
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