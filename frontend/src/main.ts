import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { initStore } from './store'

initStore(); // 启动全局监听

createApp(App).mount('#app')