<template>
  <div class="settings-page">
    <div class="glass-card setting-group">
      <h3>网络设置</h3>

      <div class="setting-item clickable" @click="$emit('navigate', 'network')">
        <div class="info">
          <h4>基础网络设置</h4>
          <p>配置内核底层的 TCP 并发、超时以及连接测速逻辑。</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'dns')">
        <div class="info">
          <h4>DNS 服务器配置</h4>
          <p>管理防污染解析、Fake-IP 策略以及分流专用的 DNS 群组。</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'tun')">
        <div class="info">
          <h4>虚拟网卡设置 (TUN 模式)</h4>
          <p>管理 Wintun 驱动并开启全局透明代理，接管所有软件流量。</p>
        </div>
        <span class="arrow">➔</span>
      </div>
    </div>

    <div class="glass-card setting-group">
      <h3>应用设置</h3>

      <div class="setting-item clickable" @click="$emit('navigate', 'behavior')">
        <div class="info">
          <h4>应用行为设置</h4>
          <p>定制软件启动模式、托盘图标逻辑及订阅请求 User-Agent。</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('enter-uwp')">
        <div class="info">
          <h4>UWP 环回免除工具</h4>
          <p>赋予 Windows UWP 应用（如微软商店、邮件）访问本地代理的权限。</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'update')">
        <div class="info">
          <h4>组件与库更新</h4>
          <p>管理并同步 Mihomo 内核、Wintun 驱动以及 GeoIP/GeoSite 规则数据库。</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'about')">
        <div class="info">
          <h4>关于应用</h4>
          <p>查看软件版本、进行配置备份还原以及访问 GitHub 开源仓库。</p>
        </div>
        <span class="arrow">➔</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  navigate: [view: 'network' | 'dns' | 'tun' | 'behavior' | 'update' | 'about'];
  'enter-uwp': [];
}>();
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
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }
.arrow { color: var(--text-sub); font-size: 1.2rem; }
</style>
