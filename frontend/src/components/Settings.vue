<template>
  <div class="settings-container">
    <div class="glass-card setting-group">
      <h3>系统与网络</h3>

      <div class="setting-item">
        <div class="info">
          <h4>UWP 环回免除 (Loopback Exemption)</h4>
          <p>解决 Windows 10/11 应用商店、邮件等自带 UWP 软件在开启代理后无法联网的问题。</p>
        </div>
        <button class="action-btn" @click="fixUWP">🔧 一键修复</button>
      </div>

    </div>
  </div>
</template>

<script setup lang="ts">
import { FixUWPNetwork } from '../../wailsjs/go/main/App';

const fixUWP = async () => {
  try {
    await FixUWPNetwork();
    alert('✅ UWP 环回限制已成功解除！');
  } catch (e) {
    alert('修复失败，请尝试右键以管理员身份运行本软件。\n错误信息: ' + e);
  }
};
</script>

<style scoped>
.settings-container { display: flex; flex-direction: column; gap: 20px; }
.setting-group { padding: 20px 24px; }
h3 { margin: 0 0 20px 0; color: var(--text-main); font-size: 1.1rem; border-bottom: 1px solid var(--glass-border); padding-bottom: 12px; }
.setting-item { display: flex; justify-content: space-between; align-items: center; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem;}
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 80%; }
.action-btn { padding: 10px 20px; border-radius: 10px; border: none; background: rgba(79, 70, 229, 0.1); color: var(--accent); font-weight: bold; cursor: pointer; transition: 0.2s; white-space: nowrap;}
.action-btn:hover { background: var(--accent); color: white; }
</style>