<!-- markdownlint-disable MD033 -->
# <img src="docs/assets/logo.png" width="45" alt="GoclashZ Logo" style="vertical-align: middle; margin-right: 10px;"> GoclashZ
<!-- markdownlint-enable MD033 -->

基於 Wails 構建的高性能、工業級實色美學 Mihomo (Clash Meta) 桌面控制端

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square) ![Wails](https://img.shields.io/badge/Wails-v2.12-red?style=flat-square) ![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js&style=flat-square) ![License](https://img.shields.io/badge/License-MIT-black?style=flat-square)

<p align="center">
  <a href="README.md">简体中文</a> | <a href="README_zh-TW.md">繁體中文（台灣）</a> | <b>繁體中文（香港）</b> | <a href="README_en.md">English</a> | <a href="README_ja.md">日本語</a> | <a href="README_ru.md">Русский</a>
</p>

---

GoclashZ 誕生於對現代桌面應用過度臃腫的抗拒。本項目摒棄傳統 Electron 架構，利用 Go 語言的系統級並發能力與 Wails 的原生網頁視圖渲染，將內存佔用與系統資源消耗壓縮至極限。視覺層面堅持高對比度、黑白實色的極簡工業美學，剔除一切無意義的漸變與裝飾。它不只是一個控制界面，更是一套經過嚴苛加固的網絡狀態管理系統。

## 界面預覽

<!-- markdownlint-disable MD033 -->
| 深色模式 (Dark) | 淺色模式 (Light) |
| :---: | :---: |
| <img src="docs/assets/控制台-黑.png" width="400" alt="控制台深色模式"> | <img src="docs/assets/控制台-白.png" width="400" alt="控制台淺色模式"> |
| <img src="docs/assets/代理节点-黑.png" width="400" alt="代理節點深色模式"> | <img src="docs/assets/代理节点-白.png" width="400" alt="代理節點淺色模式"> |
| <img src="docs/assets/订阅-黑.png" width="400" alt="訂閱管理深色模式"> | <img src="docs/assets/订阅-白.png" width="400" alt="訂閱管理淺色模式"> |
<!-- markdownlint-enable MD033 -->

## 核心功能

### 網絡接管與控制

* **獨立 Helper 提權架構**：引入完全隔離的本地後台服務（Named Pipe IPC），負責安全管控高權限操作（TUN 模式、UWP 解除、服務啟閉），主程式始終保持在普通用戶權限下運行。
* **智能 TUN 引擎**：內置 Wintun 虛擬網卡驅動的自動化部署與自愈機制，支援全系統網絡流量透明接管。
* **UWP 回環免除**：一鍵解除 Universal Windows Platform 應用的本地網絡隔離限制。
* **局域網共享與管控**：全面支援局域網代理共享，內置 SOCKS5 專屬端口管理、身份驗證及 IP 訪問控制列表 (ACL)。
* **系統代理管控**：精準控制 Windows 註冊表級代理設定，提供毫無延遲的路由切換體驗。

### 全面國際化與多語系支援 (i18n)

* **自研零依賴響應式引擎**：基於 Vue 3 響應式系統與本地儲存預熱，實現秒級無刷新切換，零第三方肥大套件負擔。
* **支援 8 大常用國際語系**：簡體中文、繁體中文（台灣）、繁體中文（香港）、English、日本語、Русский、Français、Deutsch 達成 100% 完整鍵值對齊。
* **智能自適應排版防禦**：外語長詞條下側邊欄寬度自動延展至 205px，搭配 Windows 原生字體回退鏈與表單防擠壓彈性佈局，徹底根絕文本截斷與組件變形。
* **全鏈路系統盤連動**：Windows 任務欄系統盤右鍵菜單即時同步多語言切換。

### 性能與並發探測

* **路由感知與雙棧探測**：出站測試強制通過內部代理端口，避免 TUN 模式下直連誤判。原生支援 IPv4 / IPv6 並發探測，提供極速真實出口地址偵測。
* **全局並發節流**：節點測速與更新鏈路引入信號量管理，防止通訊端耗盡與系統 I/O 阻塞。
* **流式監控引擎**：通過 WebSocket 與 Stream API 即時傳輸核心狀態，實現零延遲的連接拓撲與流量圖表展示。

### 配置與狀態管控

* **統一狀態協調器 (ControlCoordinator)**：消除多入口（UI、系統盤、看門狗）並發控制時的狀態覆蓋衝突。
* **配置自愈體系**：版本升級或匯入舊版備份時，自動修復缺失的新版欄位。
* **交易級防呆備份**：專有 `.gocz` 封裝格式，支援一鍵還原與自動快照備份。
* **原生註冊表自啟整合**：通過 Windows 原生 Run 註冊表鍵實現穩定開機自啟，告別 Task Scheduler 超時困擾。

## 部署與安裝

訪問 [Releases](https://github.com/Zzz-IT/GoclashZ/releases) 頁面下載最新版本。

## 開發者指南

```bash
# 啟動熱重載開發服務器
wails dev

# 編譯 Windows 發行版可執行文件
wails build
```

## 開源許可

本項目遵循 **MIT** 許可協議發布。
