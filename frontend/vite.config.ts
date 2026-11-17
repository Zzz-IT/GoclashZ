import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import wails from '@wailsio/runtime/plugins/vite'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails('./bindings')],
  server: {
    // wails3 dev 会通过 WAILS_VITE_PORT 注入端口；独立运行 dev 时回退到 9245
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
    watch: {
      // 编辑器/工具链的原子写会先落一个临时目录再 rename；
      // Windows 上 chokidar 去 watch 这个瞬时目录会 EBUSY 直接把 dev server 打死。
      ignored: ['**/*.tmpdir/**', '**/*.tmpdir', '**/*.tmp']
    }
  }
})
