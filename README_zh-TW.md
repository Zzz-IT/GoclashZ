<!-- markdownlint-disable MD033 -->
# <img src="docs/assets/logo.png" width="45" alt="GoclashZ Logo" style="vertical-align: middle; margin-right: 10px;"> GoclashZ

基於 Wails 建置的高效能、工業級實色美學 Mihomo (Clash Meta) 桌面控制端

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square) ![Wails](https://img.shields.io/badge/Wails-v2.12-red?style=flat-square) ![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js&style=flat-square) ![License](https://img.shields.io/badge/License-MIT-black?style=flat-square)

<p align="center">
  <a href="README.md">简体中文</a> | <b>繁體中文（台灣）</b> | <a href="README_zh-HK.md">繁體中文（香港）</a> | <a href="README_en.md">English</a> | <a href="README_ja.md">日本語</a> | <a href="README_ru.md">Русский</a>
</p>

---

GoclashZ 誕生於對現代桌面應用程式過度臃腫的反思。本專案捨棄傳統 Electron 架構，運用 Go 語言的系統級並行處理能力與 Wails 的原生網頁檢視渲染，將記憶體佔用與系統資源消耗壓縮至極限。視覺風格堅持高對比度、黑白實色的極簡工業美學，剔除多餘的漸層與裝飾。它不只是一個控制介面，更是一套經過嚴謹加固的網路狀態管理平台。

## 介面預覽

| 深色模式 (Dark) | 淺色模式 (Light) |
| :---: | :---: |
| <img src="docs/assets/控制台-黑.png" width="400" alt="控制台深色模式"> | <img src="docs/assets/控制台-白.png" width="400" alt="控制台淺色模式"> |
| <img src="docs/assets/代理节点-黑.png" width="400" alt="代理節點深色模式"> | <img src="docs/assets/代理节点-白.png" width="400" alt="代理節點淺色模式"> |
| <img src="docs/assets/订阅-黑.png" width="400" alt="訂閱管理深色模式"> | <img src="docs/assets/订阅-白.png" width="400" alt="訂閱管理淺色模式"> |
<!-- markdownlint-enable MD033 -->

## 核心功能

### 網路接管與控制

* **獨立 Helper 提權架構**：引進完全隔離的本機背景服務（Named Pipe IPC），專門管控高權限操作（TUN 模式、UWP 限制解除、服務啟閉），主程式始終以普通使用者權限安全執行。
* **智慧 TUN 引擎**：內建 Wintun 虛擬網卡驅動程式自動化部署與自我修復機制，支援全系統網路流量透明接管。
* **UWP 回環免除**：一鍵解除 Universal Windows Platform 應用程式的本機網路隔離限制。
* **區域網路代理分享**：支援區域網路共享代理，具備專用 SOCKS5 連接埠管理、身分驗證與細緻的 IP 存取控制清單 (ACL)。
* **系統代理管控**：精準控制 Windows 機碼級代理設定，零延遲切換路由。

### 全面國際化與多語系支援 (i18n)

* **自研零依賴響應式引擎**：基於 Vue 3 響應式系統與本機快取預載，實現毫秒級無刷新切換，零第三方肥大套件負擔。
* **支援 8 大常用國際語系**：簡體中文、繁體中文（台灣）、繁體中文（香港）、English、日本語、Русский、Français、Deutsch 達成 100% 完整鍵值對齊。
* **智慧自適應排版防禦**：外語長單字情境下側邊欄寬度自動延展至 205px，搭配 Windows 原生字型回退鏈與表單防擠壓彈性版面，根絕文字截斷與元件變形。
* **全鏈路系統匣功能連動**：Windows 工作列系統匣右鍵選單即時同步多語言切換。

### 效能與連線偵測

* **路由感知與雙堆疊偵測**：出站測試強制透過內部代理連接埠，避免 TUN 模式下直連誤判。原生支援 IPv4 / IPv6 同步偵測，提供極速的真實出口位址回傳與防跳爍展示。
* **全域並行節流**：節點測速與更新管線實施信號量控制，防止通訊端耗盡與系統 I/O 阻塞。
* **串流即時監控**：以 WebSocket 與 Stream API 即時傳輸核心狀態，提供流暢的連線拓撲與流量統計圖表。

### 設定與狀態管控

* **統一狀態協調器 (ControlCoordinator)**：消除多重觸發入口（UI、系統匣、看門狗）並行時的狀態覆蓋衝突。
* **設定自我修復體系**：版本升級或匯入舊版備份時，自動修復並持久化缺失的新版欄位。
* **交易級防呆備份**：專有 `.gocz` 封裝格式，支援一鍵還原與自動快照備份。
* **原生機碼自啟整合**：透過 Windows 原生 Run 機碼實現穩定開機自啟，告別 Task Scheduler 呼叫逾時困擾。

## 部署與安裝

造訪 [Releases](https://github.com/Zzz-IT/GoclashZ/releases) 頁面下載最新版本。

## 開發者指南

```bash
# 啟動即時熱重載開發伺服器
wails dev

# 建置 Windows 發行版可執行檔
wails build
```

## 聲明與後續計畫

### 社群致謝與免責聲明

* **衷心感謝**：由衷感謝所有使用 GoclashZ 以及在 GitHub 上積極提交 Issue、提出寶貴建言與參與技術討論的開發者與使用者。每一則回饋與關注都是驅使本專案持續精進、追求極致體驗的重要動力。
* **獨立開源與免責聲明**：
  1. 本專案（GoclashZ）純屬個人基於對 Go 語言並行架構、系統級網路程式設計及桌面端輕量化美學的學術研究與技術探索所開發的自由開源工具，按 MIT 授權條款原樣提供。
  2. **開發者從未在任何第三方平台（包含但不限於各類社群媒體、影音平台、論壇社群、即時通訊群組或電商管道）進行過任何形式的商業宣傳、推廣引流或背書，亦未授權任何機構或個人代表本專案進行宣傳與轉載分發**。
  3. 網路上任何第三方發布關於本軟體的介紹影片、圖文、分發載點、使用教學或商業行為，**均屬第三方獨立個人或機構的單方面行為，概與本軟體開發者無任何關聯**。
  4. 使用者應在恪遵當地法律規範的前提下合法使用本軟體。因使用、分發或基於第三方宣傳使用本軟體所產生之任何風險、責任與後果，**均由相關主體及使用者自行承擔，開發者不承擔任何直接、間接或連帶法律責任**。

### 並行計畫：轉向 Wails v3 架構，探索徹底擺脫 WebView2

為了追求更純粹的桌面原生體驗並徹底根絕環境相依困擾，專案近期計畫建立並行實驗分支（全面轉向 **Wails v3** 架構）：

* **擺脫 WebView2 依賴**：目前版本相依於 Windows 系統的 Microsoft Edge WebView2 執行階段，部分精簡系統或老舊環境中常因缺少元件而影響開箱即用體驗。轉向 Wails v3 將深入探索解耦外部執行階段、輕量化嵌入或原生渲染的技術可行性。
* **架構演進與系統深度整合**：善用 Wails v3 全新重構的跨平台原生視窗排程、無周邊背景通訊與系統整合能力，進一步壓低記憶體佔用與啟動耗時，打造真正開箱即用、完全零外部相依的極簡控制端。

## 開源授權

本專案遵循 **MIT** 授權協議發布。
