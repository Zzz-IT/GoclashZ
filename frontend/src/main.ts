import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import './styles/tokens.css'
import './styles/surfaces.css'
import './styles/layout.css'
import { initStore } from './store'
import { bootstrapTheme } from './utils/theme'

// 同步应用主题缓存：必须早于 createApp，否则首帧会先渲染默认主题再闪一下
bootstrapTheme()
initStore(); // 启动全局监听

createApp(App).mount('#app')
