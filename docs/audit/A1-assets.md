# A1 资产自检与自愈链路审计（`core/runtimeassets/**` + `core/utils/{paths,atomic,pe}.go` 及其全部调用方）

> 生成：2026-11-16 ｜ 审计范围：`core/runtimeassets/**`（6 文件 1178 行，全读）、`core/utils/{paths,atomic,pe}.go`（416 行，全读）、全部调用方（`app.go`/`main.go`/`core/appcore/{controller,supervisor,control_coordinator,updater_orchestrator,diagnostics,env}.go`/`core/clash/setup.go`/`core/clash/runner.go`）相关段落、前端 5 个调用点。
> 证据等级：**A** = 代码/实测可确证；**B** = 代码 + 运行时状态一致推断；**C** = 假设，未实测。
> 只读审计：未修改任何源码，未运行应用、未构建。唯一写入文件即本报告。

---

## 0 覆盖范围与方法

### 0.1 已读文件与覆盖度

| 文件 | 行数 | 读法 | 覆盖度 |
|---|---|---|---|
| `core/runtimeassets/status.go` | 125 | 全读 | 100% |
| `core/runtimeassets/check.go` | 321 | 全读 | 100% |
| `core/runtimeassets/sync.go` | 277 | 全读 | 100% |
| `core/runtimeassets/catalog.go` | 206 | 全读 | 100% |
| `core/runtimeassets/migrate.go` | 172 | 全读 | 100% |
| `core/runtimeassets/types.go` | 77 | 全读 | 100% |
| `core/utils/paths.go` | 277 | 全读 | 100% |
| `core/utils/atomic.go` | 99 | 全读 | 100% |
| `core/utils/pe.go` | 40 | 全读 | 100% |
| `core/utils/process_windows.go` | 22 | 全读 | 100%（`HideCommandWindow` 唯一被 `ReadCoreVersion` 使用） |
| `app.go` | 1472 | 读资产相关段：60-100、130-210、196-280、640-920 | 约 15%（资产/UI 面全覆盖） |
| `core/appcore/supervisor.go` | 251 | 全读 | 100% |
| `core/appcore/controller.go` | 2484 | 读 620-700、886-963、1740-1800 | 相关函数全覆盖 |
| `core/appcore/control_coordinator.go` | — | 读 40-260（setLocked/prepareTunEnable/回滚） | 资产相关段 100% |
| `core/clash/setup.go` | 528 | 读 1-150、420-473 | 资产相关段 100% |
| `core/clash/runner.go` | — | 读 95-155 | `Start` 段 100% |
| `core/clash/component_replace.go` | 85 | 全读 | 100%（用于对照"已有安全替换设施"） |
| `core/clash/wintun.go` | — | 读 1-70 | 写者面 100% |
| `core/appcore/{updater_orchestrator,diagnostics,env}.go` | — | 读 150-395 / 49-97 / 1-51 | 调用面 100% |
| `main.go` | — | 读 70-110 | `--repair-core-layout` 入口 100% |
| `core/sys/{tun_win,data_dir,file_version_windows}.go` | — | 读相关段 | 对照面 100% |
| `frontend/src/{App.vue,components/SettingsTun.vue,components/SettingsUpdate.vue,components/SettingsAbout.vue,store.ts}` | — | 读调用点 | 调用点 100% |
| `docs/SELF_HEAL_AND_LOGGING_PLAN.md` | 618 | 全读 | 100%（用于 §3 对账） |

**总体覆盖度**：审计范围内的 Go 源码 100%；调用方"资产写读/判定"相关函数 100%；调用方的其它逻辑（内核启停细节、helper RPC 实现）未读，已在 §4 声明。

### 0.2 检索手段

1. **调用点穷举**：`grep` 了 `runtimeassets\.`、`GetStatus(`、`EnsureReady(`、`RepairFromSeed(`、`MigrateLegacyAssets(`、`CleanupLegacyAssets(`、`GetSeedCatalogStatus(`、`MergeStatus(`、`RecordSeedCoreCopied(`、`LoadSeedCatalog(`、`CopyFile(`、`shouldCopy|RepairInvalid|VersionProbeOK`、`GetSeedCoreBinDir|GetSeedAssetManifestPath|GetCoreBinDir`、`RepairRuntimeAssets|GetRuntimeAssetStatus|...`（前端 + 绑定 + 生成服务面）。
2. **实机对账（只读）**：`Get-ChildItem` 列出 `bin/seed/core/bin`、`build/runtime-assets/core/bin`、仓库根 `data/core/bin`、`%LOCALAPPDATA%\GoclashZ\core\bin`；`Get-FileHash` 实测 manifest、seed 与 runtime 的 SHA256；`Get-Content` 读 `asset-manifest.json` 与 `asset-state.json`。
3. **口径**：只统计仓库源文件；`frontend/dist`、`node_modules` 不计入调用点（dist 是构建产物）。

### 0.3 实测基准（本次审计的硬数字）

| 项 | 值 | 来源 |
|---|---|---|
| runtime 目录 6 个资产总字节 | **95,336,775 B ≈ 90.9 MiB** | `%LOCALAPPDATA%\GoclashZ\core\bin` 实测 |
| clash.exe | 61,105,152 B | 同上（seed 同尺寸，sha 相同） |
| wintun.dll | 427,552 B | 同上 |
| geoip.metadb / geosite.dat / country.mmdb / asn.dat | 9,031,662 / 4,211,125 / 8,250,535 / 12,310,749 | 同上 |
| 一次 `GetStatus` 的读盘量 | **95.3 MB**（6 次 SHA256）+ 1 次 `clash.exe -v` | `check.go:128,268,302` + `check.go:132` |
| 热缓存哈希吞吐参考 | 61 MB ≈ 0.06 s | 引用 `docs/audit/A0-crosscutting.md:10` 实测（本机 NVMe）；冷缓存/杀软首扫未测（C） |
| DataDir 实际落点（dev） | `C:\Users\<u>\AppData\Local\GoclashZ` | `paths.go:115-118` + `Get-ChildItem $env:LOCALAPPDATA` 实测 |
| AppDir 实际落点（dev） | `D:\GOLANG\GoclashZv3\bin` | `paths.go:66-91` + 实测 exe 位置 |

> 关键：**AppDir 在 D: 卷、DataDir 在 C: 卷**（本机实测）。这一条直接决定了 `migrate.go:135-141` 的跨卷 `os.Rename` 必然失败 → 走 `os.Remove` 分支（见 F-A1-16）。

### 0.4 本次限制

- 未运行应用、未构建、未 kill 进程（硬性规则），因此**所有耗时都是按字节量与已知吞吐推算**，标注 B/C；进程内锁竞争、杀软扫描放大倍数均未实测。
- 未读 helper 侧 `ReplaceCoreFile` 的实现，跨进程写同一文件的影响范围只到调用边界（C）。

---

## 1 链路清单表

> "严格度"：极严 = PE 头 + SHA256 + 子进程探测；严 = 上述部分；中 = stat/PE；弱 = 仅 stat。
> "单次成本"中的 MB 数为实测字节（§0.3），耗时为 B 级推算；spawn 数按 `-v` 缓存未命中计。

| # | 入口 / 场景 | file:line | 严格度 | 单次成本 | 是否阻塞用户 | 误报面 | 触发频率 |
|---|---|---|---|---|---|---|---|
| 1 | 启动资产维护 `runStartupRuntimeAssetMaintenance` | `app.go:166-191`（由 `app.go:204` 调用） | 极严 | Migrate 哈希 95.3 MB + EnsureReady×2（2~3 次 GetStatus ≈190~285 MB）+ 1~2 次 `-v`（60 s 缓存内不重复） | **是**（UI 出现前，同步） | 任何一次 IO/子进程异常 → 启动变慢或被日志刷屏；geo 失败只 warn | 每次启动 |
| 2 | `MigrateLegacyAssets` | `migrate.go:14-91`；入口 `app.go:167`、`app.go:720`、`main.go:92` | 中（哈希 + rename/write） | 6×`calculateSHA256`（`migrate.go:36-41`）≈95.3 MB；跨卷时退化为读+写全量 | 是 | `os.Rename` 跨卷失败 → `os.ReadFile`+`os.WriteFile` 兜底（内存中整份 61 MB）；失败静默 | 每次启动 + 手动按钮 |
| 3 | `CleanupLegacyAssets` | `migrate.go:94-171`；入口 `app.go:190`（`go`） | 中 | 最多 12×哈希对比（`migrate.go:121-125`） | 否（goroutine） | 与 #7/#6 抢同一批目录；跨卷 rename 失败 → **直接删文件**（`migrate.go:140`） | 每次启动异步 |
| 4 | `GetStatus` | `status.go:19-42` | 极严 | 95.3 MB SHA256 + 1 次 `-v`（≤5 s，60 s 缓存）+ 版本 API | 是（纯同步） | 任何 IO 异常只降级为字段，不阻断；但**成本必然发生** | 每次 EnsureReady + 6 个 UI 方法 |
| 5 | `CheckCore` → `ReadCoreVersion` | `check.go:91-142`、`check.go:144-197` | 极严 | stat + PE + 61.1 MB SHA256 + spawn（≤5 s） | 是 | 探针失败只把 Version 降级、`Ready` 仍 true（`check.go:125-137`）→ 掩盖"跑不起来"；**全局互斥锁跨子进程** | 每次 GetStatus |
| 6 | `CheckWintun` | `check.go:199-273` | 严 | stat×N + PE + `GetFileVersion` + 427 KB SHA256 | 是 | 主目录不可用时改用 alt 目录并**返回它 Ready**（`check.go:207-225`），但内核只从 `core/bin` 加载 → 乐观误判 | 每次 GetStatus |
| 7 | `adoptAssetFromFallback` | `check.go:40-63`、`check.go:66-89` | —（**写操作**） | 命中则 0；缺失则同步 `CopyFile`（≤61.1 MB） | **是** | 来源无哈希/签名校验，只看 MZ+大小（`pe.go:22-37`）；候选含 `<appDir>/../data/core/bin` 等旧目录 | 每次 `CheckCore`/`CheckWintun` 且主路径缺失 |
| 8 | `CheckDataFile`（4 个 geo） | `check.go:275-307` | 弱（仅 stat）+ 哈希 | 33.1 MB SHA256 | 是 | **0 字节/截断文件也 `Ready=true`**（`check.go:296-300` 无任何内容校验） | 每次 GetStatus |
| 9 | `EnsureReady` | `status.go:45-73` | 严 | 满足：1×GetStatus；不满足：2×GetStatus + 修复 | 是 | 修复失败即返回 error（`status.go:55-56`）→ 上层可能升级为硬失败 | 每层调用 |
| 10 | `RepairFromSeed` / `repairFromSeed` | `sync.go:118-134`、`sync.go:136-246` | 中（判定有缺陷） | 逐资产：seed 侧 `checkAssetByPath`（含 seed 的 `-v` 与哈希）+ runtime 侧 `Check*` + 复制（≤95.3 MB） | 是 | 包级 60 s 冷却跨资产污染；wintun 恒 `shouldCopy`；无单飞 | 每次 EnsureReady 不满足时 |
| 11 | TUN 前置 `prepareTunEnable` | `controller.go:1760-1787`（入口 `control_coordinator.go:75-80`） | 严 | 1×EnsureReady + 1×helper ping | 是 | 只对 `ErrMissing` 硬拦（`controller.go:1777`，设计正确）；其余只 warn | 每次开 TUN |
| 12 | watchdog `Reconcile` | `supervisor.go:142-221`，watchdog `supervisor.go:105-137` | 严 | `supervisor.go:155` → 1×EnsureReady = 1~2×GetStatus（95.3~190 MB） | 是（阻塞 watchdog goroutine） | 每 5 s 一次全量哈希；`ctx` 8 s 预算无法中断（见 F-A1-08） | TUN 期望开启期间 **5 s/次** |
| 13 | 起内核 `ensureCoreRunningWithDesiredState` | `controller.go:626-662`（入口 `controller.go:886-896`、`904-921`） | **最严** | 1×EnsureReady（`controller.go:655`） | 是 | **任何**修复错误 → `运行组件不可用: %w`（`controller.go:661`）硬失败 | 每次起内核 |
| 14 | `clash.Start` → `PrepareEnv` | `runner.go:97-125`（`:114`）→ `setup.go:26-42`（`:28`） | 中 | 1×EnsureReady（每次重试也跑） | 是 | 与 #13 重复；错误被包装成"内核不可用" | 每次起内核 + 每次启动重试 |
| 15 | 6 个 UI 同步方法 | `app.go:667`(GetDataDirInfo)、`728`(CheckTunEnv)、`745`(GetWintunVersion)、`757`(GetComponentFileInfo)、`886`(GetCoreVersion)、`898`(GetRuntimeAssetStatus) | 极严 | 各 1×GetStatus（≤95.3 MB + 1 spawn） | **是**（Wails 同步 RPC） | 调页即卡；`CheckTunEnv` 还叠 helper ping(≤10 s) + `CoreSupportsMips` | 打开设置页 / 首屏 / 诊断弹窗 |
| 16 | `GetDataDirInfo` → `GetSeedCatalogStatus` | `app.go:681` → `catalog.go:119-151`（`:134-147`） | 极严 | 再对 **seed** 侧 6 资产跑一遍：`checkCoreByPath(seed)` → seed 的 `clash.exe -v`（≤5 s）+ 95.3 MB 哈希 | 是 | 诊断接口本身很贵；seed 缺 clash.exe 时整块信息为 false | 点"数据目录诊断" |
| 17 | `RepairRuntimeAssets` | `app.go:901-907`（`:902` Force） | Force（跳过冷却） | 重写全部 6 资产 95.3 MB + 复制期间的 rename | 是 | 覆盖运行中的 clash.exe/wintun.dll 必失败；错误只写日志（`app.go:904`） | 无前端调用方（死接口，见 F-A1-17） |
| 18 | 更新后记账 `RecordSeedCoreCopied` | `sync.go:256-277`；调用 `updater_orchestrator.go:378` | —（写状态） | 1×`LoadSeedCatalog` + 可能 1×61 MB 哈希（`sync.go:265-266`） | 否 | 用 manifest 的 sha256 记账（`sync.go:261-263`），而 manifest 该条已过期 → 记错哈希链（F-A1-06） | 每次内核替换 |

**链路合流结论**：一次"开 TUN"在正常路径上是 **4 次 EnsureReady**（#11 → #12 → #13 → #14），即 **4 次（最坏 8 次）全量 `GetStatus` ≈ 381 MB（最坏 762 MB）读盘 + 0~4 次 `-v`（受 60 s 版本缓存影响）**；TUN 开启后 watchdog 每 5 s 再跑 #12；打开"设置→TUN"再并发 2 次 `GetStatus`（`SettingsTun.vue:118`，`refreshRuntimeAssets()` 与 `CheckTunEnv()` 并发）。

---

## 2 问题清单

> 按严重度排序。每条给出 严重度 / 证据等级 / file:line / 现象 / 影响 / 修法。

### P0

#### F-A1-01 检查无缓存：每次 `GetStatus` 无条件全量 SHA256 95.3 MB
- **严重度**：P0 ｜ **证据等级**：A（成本）/ B（耗时）
- **位置**：`core/runtimeassets/status.go:19-42`；`core/runtimeassets/check.go:128-130`（core）、`:268-270`（wintun）、`:302-304`（4 个 geo）、`:309-321`（`calculateSHA256`）；`core/runtimeassets/migrate.go:36-41`
- **现象**：`GetStatus` 对 6 个资产逐个 `os.Stat` + `SHA256` + PE 校验，**没有任何 stat/(size,mtime) 缓存**。唯一的缓存是 `ReadCoreVersion` 的 60 s 版本缓存（`check.go:151`），只覆盖"子进程探测"这一项，不覆盖 95.3 MB 哈希。`MigrateLegacyAssets` 每次启动还会**再**对同样 6 个文件算一遍哈希（`migrate.go:36-41`）。
- **影响**：TUN 期望开启期间 watchdog 每 5 s 触发一次 `Reconcile`（`supervisor.go:72`、`:114-129`、`:155`）→ **持续 ≈19 MB/s 的读盘 + 对应 SHA256 CPU**，笔记本功耗/发热/杀软扫描压力长期存在；冷启动时正好撞上杀软首扫窗口，把"检查"变成"几十秒无响应"。
- **修法**：在 `GetStatus`（`status.go:19`）内新增包级 `statCache`（key=`path`，value=`size,mtime,hash,at`，TTL 30~60 s），命中则跳过 `calculateSHA256`；对 watchdog 路径（`supervisor.go:155`）改为调用新增的 `GetStatusFast(ctx)`，只做 stat+PE（`check.go:100-126` 的 L0/L1 部分）；把哈希计算下沉到 `repairFromSeed`（`sync.go:191` 的比较处）按需单文件计算。

#### F-A1-02 `RepairInvalid` 下 wintun 恒 `shouldCopy=true`，配 `CopyFile` 无释放等待 → 运行中覆盖必失败 → 冷却 → 硬失败
- **严重度**：P0 ｜ **证据等级**：A（判定链）/ B（`os.Rename` 的具体 Win32 错误码）
- **位置**：`core/runtimeassets/sync.go:176-188`（`:182` `shouldCopy = !runtimeHealth.Ready || !runtimeHealth.VersionProbeOK`）、`sync.go:252-254`（`isCoreAsset` 含 wintun）、`check.go:230-273`（`checkWintunByPath` **从不设置 `VersionProbeOK`**，全文仅 `check.go:140` 设置它）、`core/utils/atomic.go:59-99`（`CopyFile`：只写 `dst+".tmp"` 再 `os.Rename`，无 `WaitFileReleased`、无重试）
- **现象**：只要 `requirementSatisfied` 为 false 进入修复分支，wintun 的 `shouldCopy` **恒为 true**（`VersionProbeOK` 恒 false）；于是每次自愈都会重写 `wintun.dll`。若 TUN 正开着，`wintun.dll` 已被 clash.exe 以 image 方式映射，`os.Rename`（`atomic.go:93`）必失败 → 本次修复报错。对比同仓已有的安全替换设施：`core/clash/component_replace.go:10-35`（`WaitFileReleased`）+ `:54-84`（`ReplaceFileWithBackup`，备份+10 次重试+回滚）——**自愈路径没有复用它们**。
- **影响**：一次"文件在、但修复失败"的假故障；错误进入包级 60 s 冷却（`sync.go:120-124`），随后 `ensureCoreRunningWithDesiredState` 把这份错误升级成 `运行组件不可用`（`controller.go:655-661`）→ 开 TUN/重启内核直接失败，且 60 s 内连重试机会都没有。这正是"开 TUN 弹缺驱动、重试正常"的机制之一。
- **修法**：① `checkWintunByPath`（`check.go:258-266`）在拿到非空版本时置 `h.VersionProbeOK = true`；② `sync.go:182` 改为 `shouldCopy = !runtimeHealth.Ready || !runtimeHealth.Valid`（去掉 `VersionProbeOK`）；③ `sync.go:211` 的 `utils.CopyFile` 换成 `clash.ReplaceFileWithBackup` 语义（新增 `utils.ReplaceFileBusySafe`：先 `WaitFileReleased` 500 ms，再 rename，失败转"待替换"标记而不报错）。

#### F-A1-03 自愈并发失控：冷却非原子、无单飞、`CopyFile` 用固定 `.tmp` 名
- **严重度**：P0 ｜ **证据等级**：A
- **位置**：`core/runtimeassets/sync.go:118-134`（锁只覆盖"读/写冷却变量"，`repairFromSeed` 在锁外执行）、`sync.go:110-116`（冷却变量是**包级单例**）、`core/utils/atomic.go:70-71`（`tmp := dst + ".tmp"` + `os.OpenFile(tmp, O_CREATE|O_WRONLY|O_TRUNC)`，**无 `O_EXCL`、无随机后缀**）
- **现象**：`RepairFromSeed` 先 `Lock` 判断冷却 → `Unlock` → 执行 `repairFromSeed` → 再 `Lock` 记录结果。两个 goroutine 可同时通过判断并并行修复；而两个 `CopyFile` 写同一个 `wintun.dll.tmp`，后写者会截断前者的临时文件，`os.Rename` 后得到**半截文件**或双方都失败。并发来源真实存在：watchdog（`supervisor.go:155`，无 UI 锁）、UI `GetRuntimeAssetStatus`→`CheckCore`→`adoptAssetFromFallback`（`app.go:898`）、`RepairRuntimeAssets`（`app.go:902`）、设置页两个并发 `GetStatus`（`SettingsTun.vue:118`）。
- **影响**：偶发"资产明明是好的却校验失败"/"复制出半截 DLL"；且冷却变量是包级单例 → **core 修复失败会让紧接着的 geo 阶段（`app.go:181`）直接拿到 core 的错误**，geo 连尝试机会都没有，日志里出现张冠李戴的报错。
- **修法**：① `sync.go:118` 外层加 `singleflight`（key = 资产名集合哈希）或 `sync.Mutex` 覆盖整个 `repairFromSeed` 调用；② `atomic.go:70` 改为 `os.CreateTemp(dir, "."+base+".*.tmp")` 并 `os.Rename` 其 `Name()`（同文件 `WriteFileAtomic:19` 已是正确写法，直接照抄）；③ 冷却改成 `map[AssetKey]{nextAt, err}`，`sync.go:120` 只查当前 `def.Key`。

#### F-A1-04 "检查"里写文件且无来源校验：`adoptAssetFromFallback` 会在只读 UI 方法里复制 61 MB
- **严重度**：P0 ｜ **证据等级**：A
- **位置**：`core/runtimeassets/check.go:66-89`（`:82` `utils.CopyFile(src, primary)`），被 `check.go:92`（`CheckCore`）与 `check.go:200`（`CheckWintun`）调用；`check.go:40-63` 的候选目录包含 `<appDir>/core/bin`、`<appDir>/data/core/bin`、`<parent>/data/core/bin`、`<parent>/core/bin`；校验只到 `pe.go:22-37`（MZ + 大小区间）
- **现象**：`GetStatus` 名义上是只读检查，实际在 `CheckCore` 第一步就调用 `adoptAssetFromFallback`，主路径缺失时**同步复制**最多 61.1 MB。于是所有 6 个 UI 方法（`app.go:667/728/745/757/886/898`）都可能变成"一次 61 MB 写盘"。且来源只看 PE 魔数与体积，**不比对 manifest 的 sha256、不校验来源目录可信度**。
- **影响**：① 用户删掉 clash.exe 后，只要打开设置页就会在 UI 线程上触发 61 MB 复制（配合 F-A1-09 的锁，最坏卡数秒）；② 实测本机仓库根 `D:\GOLANG\GoclashZv3\data\core\bin` 就躺着一套**旧版**资产（geoip 8,769,243 vs 现行 9,031,662），而它正是 `<parent>/data/core/bin` 候选 → 运行目录一旦缺文件，会被**旧资产**静默填回；③ 父目录若可写（便携安装/用户目录安装），任何能写该目录的程序都能投放一个"合法 PE"被当作内核执行。
- **修法**：① `check.go:92` / `:200` 删除 `adoptAssetFromFallback` 调用，迁移动作只保留在 `MigrateLegacyAssets`（`migrate.go:14`）与自愈执行器（`sync.go:209-214`）；② 若必须保留兜底，加 `LoadSeedCatalog()` 比对 `catalog.Items[name].SHA256`（`catalog.go:106-113`）后才复制；③ 候选目录去掉 `filepath.Dir(appDir)` 两处（`check.go:48-49`）。

### P1

#### F-A1-05 `Ready` 与 `VersionProbeOK` 解耦：损坏内核 / 0 字节 geo 在快路径上永不触发自愈
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`check.go:118-137`（PE 通过就 `h.Valid=true; h.Ready=true`（`:125-126`），随后版本探测失败只改 `Version/Hint`，**不回退 `Ready`**）；`check.go:296-300`（geo 只 `stat` 成功即 `Valid/Ready=true`）；`status.go:50` 与 `status.go:75-90`（`requirementSatisfied` **只看 Ready**）；`sync.go:180-188`（`RepairInvalid` 的 `VersionProbeOK` 分支因此几乎走不到）
- **现象**：`EnsureReady` 的第一道闸门是 `requirementSatisfied`（`status.go:50`），它只读 `Ready`；而 `Ready` 在"格式像 PE 但根本跑不起来"（被截断、架构错、被 AV 隔离成占位文件）和"0 字节 geoip.metadb"时**仍为 true**。于是 `EnsureReady` 直接 return，`RepairInvalid` 的判定逻辑成为死代码，用户看到的是后续 `clash.Start`/`ValidateConfig` 报出的莫名错误。
- **影响**：最需要自愈的两类损坏（内核不可执行、规则库被截断）恰好绕过了自愈；`check.go:135` 的提示"版本探测失败或超时，但内核文件格式有效；可尝试直接启动"会把用户引到错误方向。
- **修法**：① `check.go:125` 改为 `h.Valid = true` 但 `h.Ready = false`，等版本探测成功（`:139-140`）再置 `h.Ready = true`；探针超时时引入第三态（如 `Ready=true, Degraded=true`）而不是现在这样混同。② `checkDataFileByPath`（`check.go:283`）增加最小体积/魔数校验（metadb/mdb 头、`geosite.dat` 头），`Size==0` 直接 `ErrInvalidPE` 语义的新错误码。③ `requirementSatisfied`（`status.go:75`）对 core 额外要求 `VersionProbeOK || Version=="已安装，版本未知"` 的显式降级策略，而不是无条件信任 `Ready`。

#### F-A1-06 manifest 的 SHA256 只写不校验，且实测清单与二进制已不一致 → 内核升级被永久判成"用户魔改"
- **严重度**：P1 ｜ **证据等级**：A（manifest 与二进制实测不符、代码只写不读）/ B（对升级流程的最终影响）
- **位置**：`catalog.go:97-114`（读 manifest 建 `Items`）、`sync.go:219-222`（复制后用 **manifest 的 sha256** 记账）、`sync.go:256-277`（`RecordSeedCoreCopied` 同样用 `catalog.Items["clash.exe"].SHA256`）、`sync.go:191-203`（升级时用 `runtimeHealth.SHA256 == copied.SHA256` 判断"是否被用户改过"）
- **实测证据**：`bin/seed/core/asset-manifest.json:8-10` 声明 `clash.exe` `size=47213056, sha256=58a8136f…, version=v1.19.27`；而 `bin/seed/core/bin/clash.exe` 实际 **61,105,152 B / `1fa8055e0359…`**（`Get-FileHash` 实测，runtime 副本同哈希）。同理全仓检索确认：manifest 的 `sha256` 从未与任何真实文件比对（唯一用途是被抄进 `asset-state.json`）。
- **影响**：只要发生过一次 core 复制/替换，`asset-state.json` 里 `clash.exe` 的 sha 就是 `58a8136f…`；而运行目录的真实文件是 `1fa8055e…` → 下次 app 升级（`appUpdated=true`）时 `sync.go:193-197` 判定"已被用户手动修改"→ **跳过覆盖**，用户永远拿不到内置新内核，且日志只留一句"已被用户手动修改，升级时跳过覆盖"，极具误导性。
- **修法**：① `sync.go:219-222` 删除"优先用 manifest sha"的覆盖，一律用实际文件的 `seedHealth.SHA256`；② `RecordSeedCoreCopied`（`sync.go:261-266`）同样改为对目标文件实测哈希（或复制后返回 hash 由调用方传入）；③ 在 `LoadSeedCatalog`（`catalog.go:82`）加载后加一次"元数据 vs 实测"一致性校验，不一致就打 warn 并改用实测值；④ 修 manifest 生成脚本，让 `size/sha256` 与产物同步（当前差异说明清单是旧构建残留）。

#### F-A1-07 `CheckWintun` 的 alt 目录回退把"不在运行目录"的 DLL 判成 Ready → 自检乐观误判，且与 `CleanupLegacyAssets` 互拆
- **严重度**：P1 ｜ **证据等级**：A（代码路径）/ B（TUN 实际失败）
- **位置**：`check.go:207-225`（主路径不 Ready 时遍历 `<appDir>/core/bin/wintun.dll`、`<appDir>/data/core/bin/wintun.dll`，命中就**把该 alt 的健康度原样返回**）；对照 `core/clash/runner.go:105-120`（内核工作目录 = `utils.GetCoreBinDir()`）与 `core/sys/tun_win.go:21-23`（Wintun 路径固定 `core/bin`）
- **现象**：`CheckWintun` 可以在 DLL 并不存在运行目录时返回 `Ready=true`、`Path=<alt>`。而 `EnsureReady`（`status.go:67-70`）据此认为 wintun 可用直接放行；clash.exe 启动时按 `binDir` 只会在 `core/bin` 找 DLL（Windows 加载器先找 exe 目录），于是 TUN 起不来。更糟的是 `CleanupLegacyAssets`（`migrate.go:144-155`）启动时就在**删除** `<appDir>/core/bin/*`，与"把那里当合法来源"直接矛盾——同一份状态在启动后几秒内可能从 Ready 翻成 Missing。
- **影响**：典型的"自检说没问题，功能却不可用"；也解释了历史上"wintun 明明装了也在用却被判缺失"的反向情况（现在被修成另一个方向的误判）。
- **修法**：① `check.go:221-224` 命中 alt 时不要返回 `altHealth`，而是返回 `primary` 的 health 并附 `Hint="检测到旁路副本 %s，请点击修复迁入运行目录"`（即"发现"与"就绪"分离）；② 或先尝试把 alt 复制进 primary 再复检（复用 `adoptAssetFromFallback` 的复制逻辑），复制成功才 Ready；③ `migrate.go:144-149` 的清理清单不要把 `wintun.dll` 列进"可删除"，或改为"仅当 primary 存在且 hash 相同才删"。

#### F-A1-08 `ctx` 全链路无效：超时与取消无法中断复制/探针，watchdog 的 8 s 预算形同虚设
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`status.go:19`（`GetStatus(ctx)`）、`status.go:45`（`EnsureReady(ctx,...)`）、`check.go:97`（`checkCoreByPath(ctx,...)`）——**函数体内没有一处引用 `ctx`**；`sync.go:118`/`:136` 只把 `ctx` 透传给同样忽略它的 `checkAssetByPath`/`checkRuntimeAsset`（`catalog.go:153-173`）；`atomic.go:59` 的 `CopyFile` 无 ctx 参数；`check.go:157` 内部自建 `context.Background()`；调用方预算 `supervisor.go:124`（8 s，watchdog）/ `supervisor.go:94`（45 s，异步）。
- **现象**：所有 `runtimeassets` 的公开函数都"看起来支持取消"，实际不检查 `ctx.Done()`：61 MB 复制、5 s `-v` 探针、95 MB 哈希都会跑到结束。`supervisor.go:124-128` 的 `context.WithTimeout(8s)` 只能让 `Reconcile` 在**事后**返回失败，无法避免超时。
- **影响**：出现"explain 超时但实际还在写盘"的并行叠加；watchdog 周期被单次检查拖长（5 s ticker 会丢 tick），把偶发慢变成持续抖动。
- **修法**：① `check.go:309` 的 `calculateSHA256` 增加 `ctx` 并在 `io.Copy` 外层用 `select` 检查（或用 `io.Copy` 分块 + `ctx.Err()` 判断）；② `atomic.go:59` 新增 `CopyFileContext(ctx, src, dst)`（分块 1 MB 检查 ctx），`sync.go:211` 与 `check.go:82` 改用它；③ `checkCoreByPath`（`check.go:97`）在每个耗时步骤前加 `if err := ctx.Err(); err != nil { ... }`；④ 若无 ctx 的调用方（`CheckWintun` 无 ctx 入参，`check.go:199`）需要一起改签名。

#### F-A1-09 `ReadCoreVersion` 持全局互斥锁执行子进程（≤5 s），失败不缓存
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`check.go:144-197`（`:145-147` 全函数持 `coreVersionCacheMu`；`:157-164` 在锁内 `exec.CommandContext(...).CombinedOutput()`，超时 5 s；`:187-194` 只有成功才写缓存）
- **现象**：锁的粒度是**整个进程的版本探测**（不是按 path），且覆盖子进程执行期。任何并发 `GetStatus`（watchdog、UI、`GetSeedCatalogStatus` 的 seed 版本探测）都会排队等这把锁；探针失败时**不写缓存**，于是每次调用都重新等 5 s。
- **影响**：冷启动杀软扫描窗口里一次 `-v` 卡住 → 所有 `GetStatus` 串行堆积（"设置页点开没反应"的直接原因之一）；seed 与 runtime 两个路径互相排队。
- **修法**：① 锁只保护 map 读写（`:145-155` 与 `:187-194`），子进程执行放锁外，用 `singleflight.Group`（key=path）保证同一 path 只跑一次；② 失败也写一条短 TTL（如 5 s）的 negative cache，避免雪崩重试；③ 把 5 s 超时降到 1.5 s（`check.go:157`），并把结果标成 `VersionProbeTimeout` 交给后台刷新。

#### F-A1-10 一次开 TUN 实际是 **4 次** `EnsureReady`（方案说 3 次），最坏 8 次全量扫描
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：① `control_coordinator.go:75-80` → `controller.go:1770`；② `supervisor.go:155`（`Reconcile` 中 `desired.Tun` 分支）；③ `controller.go:655`（经 `:886-896`、`:904-921`）；④ `core/clash/runner.go:114` → `setup.go:28`（`clash.PrepareEnv`）
- **现象**：`setLocked` 先在 `prepareTunEnable` 跑一次；随后 `Supervisor.Reconcile` 再跑一次；起内核前 `ensureCoreRunningWithDesiredState` 第三次；`clash.Start` 内部 `PrepareEnv` 第四次。任一环节 `requirementSatisfied=false` 时，`EnsureReady` 会变成 2 次 `GetStatus` + 一次修复（`status.go:47/56/60`）。
- **影响**：单次用户动作 ≈381 MB 读盘、4 次 `-v`（最坏 762 MB / 8 次）；TUN 冷启动的"25 s 预算"被检查吃掉大半，剩下的时间不够等适配器，最终表现为"开 TUN 失败"。
- **修法**：① 删除 `setup.go:28` 的 `EnsureReady`（`clash.Start` 是纯执行层，不应做资产门禁），改为 `PrepareEnv` 只做 `os.Stat` 存在性断言；② `controller.go:655` 改为复用 `prepareTunEnable` 已得到的 status（把 status 通过 `DesiredState`/`RuntimePlan` 传下去），或在 `Controller` 上加一个 30 s 的 `lastAssetCheck` 单次缓存；③ `supervisor.go:155` 改为 `GetStatusFast`（只 stat），深检交后台（与 F-A1-01 同一修法）。

#### F-A1-11 seed 目录探测只看"存在"、manifest 路径有两套约定、候选含父目录
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`paths.go:156-164`（4 个候选，含 `filepath.Dir(appDir)` 两个）、`paths.go:166-174`（返回**第一个存在的目录**，否则 `candidates[0]`）、`paths.go:176-191`（manifest 候选第 1 条由 `Dir(Dir(seedDir))` 推导，与真实布局不一致，靠硬编码列表兜底）、`catalog.go:175-181`
- **现象**：① 只判目录存在，不判"里面有没有资产"——一个空的 `seed/core/bin` 会遮蔽后面真正有资产的候选，导致 `RepairFromSeed` 全部失败（`sync.go:161-171`）；② manifest 推导出的是 `<seedRoot>/asset-manifest.json`，而实际文件在 `<seedRoot>/core/asset-manifest.json`（实测 `bin/seed/core/asset-manifest.json` 存在，`bin/seed/asset-manifest.json` 不存在），现在靠 `candidates[1]` 硬编码救回，任何新布局都会静默失去 manifest（版本/哈希元数据全丢，但不阻塞复制）；③ 候选包含 appDir 的父目录，任意可写的父目录都能成为资产源。
- **影响**：自愈"有时灵有时不灵"，取决于残留目录；manifest 丢失时 `appUpdated` 恒 false（`sync.go:141-146`），升级覆盖逻辑整段失效。
- **修法**：① `GetSeedCoreBinDir` 改为"目录存在**且** `clash.exe` 或 `asset-manifest.json` 存在"才算命中；② 删除 `paths.go:161-162` 的父目录候选（与 `check.go:48-49` 一并删）；③ `GetSeedAssetManifestPath` 只用显式列表，删掉 `Dir(Dir(seedDir))` 推导（`paths.go:179`）。

#### F-A1-12 启动自愈结论无人消费：事件在 UI 订阅前发出，前端从未订阅
- **严重度**：P1 ｜ **证据等级**：A（无订阅者）/ B（事件丢失时序）
- **位置**：`app.go:188`（`emitEvent("runtime-assets-status", status)`）、`v3_events.go:52`（注册类型）、`app.go:204`（在 `ServiceStartup` 里同步执行，早于窗口/前端加载）、`wails_event_sink.go:37-39`（`ctx==nil` 直接丢弃）、`frontend/src/store.ts:237`（`assetStatus` 字段）
- **现象**：全前端检索 `runtime-assets-status` **零订阅**（只有 `store.ts:237` 的字段声明与 `SettingsUpdate.vue:74` 的赋值）；事件本身在 `ServiceStartup` 中同步发出，此刻 WebView 尚未加载 JS。于是"启动时资产自愈到底成功没有"这个最关键的信息，既不进 UI 也没有落盘日志（只有 `logger.Errorf/Warnf` 几行自然语言）。
- **影响**：事故复盘时无法回答"启动时判定成了什么"；用户也看不到"已自动修复"的正向反馈，只会感觉"上次能开这次不能"。
- **修法**：① 保留 status 到 `App`/Controller 字段（如 `a.startupAssetStatus`），由 `GetRuntimeAssetStatus` 首次调用时返回；② 前端在 `App.vue:614` 的 `onMounted` 里补 `Events.On('runtime-assets-status', ...)`；③ 按主方案 §6.3 落 `boot.phase` / `asset.repair.*` 结构化日志。

#### F-A1-13 同一批文件有 6 个写者、3 套锁、0 个跨路径互斥
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：写 `core/bin/*` 的路径：`check.go:82`（`CopyFile`）、`sync.go:211`（`CopyFile`，唯一受"冷却锁"影响但不受互斥保护）、`migrate.go:60`（`os.Rename` 备份）、`migrate.go:76-88`（`os.Rename` / `os.WriteFile` 迁移）、`component_replace.go:54-84`（`ReplaceFileWithBackup`，无全局锁）、`clash/wintun.go:21-24`（`wintunBinaryMu` 只保护驱动安装自身）、`clash/setup.go:140+`（内核 zip 解包）、`core/appcore/updater_orchestrator.go:353-365`（helper RPC 或 `ReplaceFileWithBackup`）、`core/appcore/env.go:38`（geo 原子下载）
- **现象**：更新器有 `wintunBinaryMu`（`wintun.go:21`）、有 `WaitFileReleased` 重试（`component_replace.go:10`），但自愈路径（`sync.go:211`）与迁移路径（`migrate.go:76`）**完全不加锁**；三者没有共享的 `%DATA%\runtime\.asset.lock`（全仓 grep 无该文件）。
- **影响**：驱动重装 / 内核更新 / 自愈复制 / 启动迁移可同时改同一批文件 → `.tmp`/`.bak` 残留、半数损坏、"替换后版本不对"。
- **修法**：新增 `core/runtimeassets/lock.go`：`AcquireAssetLock(ctx, name)`（进程内 `sync.Mutex` 按名分片 + 跨进程锁文件 `%DATA%\runtime\.asset-<name>.lock`），要求 `sync.go:211`、`check.go:82`、`migrate.go:76-88`、`component_replace.go:54`、`clash/wintun.go:23` 全部先取锁。

#### F-A1-14 `SHA256` 字段零消费者：非修复路径的 95 MB 哈希是纯浪费
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`types.go:35`（`SHA256` 字段）；生产点 `check.go:128/268/302`；消费点仅 `sync.go:182-203`（升级比较）与 `sync.go:219`（记账）
- **现象**：`grep` 确认 —— `frontend/src` 中 `sha256` **0 命中**（`AssetHealth` 的其它字段有 UI 使用），`core/appcore/*` 中 0 命中；也就是说 `GetStatus` 为 6 个资产算出的哈希，在 6 个 UI 方法与 watchdog 路径上**没有任何读者**。
- **影响**：这是"检查太重"里最容易立刻摘掉的一块 —— 70% 以上的读盘量可以零风险删除（修复路径按需单文件计算即可）。
- **修法**：`status.go:19` 增加 `deep bool` 参数（或拆成 `ProbeFast`/`ProbeDeep`），UI/watchdog 调 Fast（不填 SHA256），只有 `repairFromSeed`（`sync.go:191`）与诊断导出走 Deep。

#### F-A1-15 `ErrIsDir` 资产永远不会被修复，且 `EnsureReady` 对 geo 缺失不报错 → 静默永久失败
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`check.go:289-294`（数据文件是目录 → `Exists=true, ErrorCode=ErrIsDir`）；`sync.go:180-188`（`RepairInvalid`/`RepairMissingOnly` 对非 core 资产只看 `Exists` → `shouldCopy=false`）；`status.go:62-70`（只对 Core/Wintun 报错，geo 不检查）
- **现象**：若 `geoip.metadb` 位置被误建成目录（或某次半途失败留下目录），`Exists=true` 让自愈跳过，`EnsureReady` 也不会返回错误 → `RepairFromSeed` 返回 nil → 上层认为"已就绪"。
- **影响**：规则库永久不可用，内核可能因缺少 geo 而规则退化，但所有日志都显示正常。
- **修法**：① `sync.go:184/187` 改为 `shouldCopy = !runtimeHealth.Exists || runtimeHealth.ErrorCode == ErrIsDir`（core 分支同理由 `Ready` 覆盖）；② 复制前若目标是目录，先 `os.RemoveAll`（仅限已知 6 个文件名）；③ `requirementSatisfied`（`status.go:82-88`）的 geo 分支在 `ErrIsDir` 时返回 false，让 EnsureReady 走报错路径。

#### F-A1-16 `CleanupLegacyAssets` 跨卷 rename 失败即删文件，且与前台自检/自愈并发
- **严重度**：P1 ｜ **证据等级**：A（本机卷分布实测）
- **位置**：`migrate.go:111`（`backupDir` 在 `DataDir` 下）、`migrate.go:135-141`（`os.Rename` 失败 → `_ = os.Remove(path)`）、`migrate.go:121-132`（先比哈希再删）、`app.go:190`（`go CleanupLegacyAssets()`）
- **现象**：备份目标是 `<DataDir>\backups\...`，源是 `<AppDir>\core\bin\*`。本机实测 AppDir 在 **D:**、DataDir 在 **C:** → `os.Rename` 跨卷必失败（`ERROR_NOT_SAME_DEVICE`）→ 直接 `os.Remove`，最多 61 MB 的文件被**删除而非备份**（`migrate.go:140`）；且它在 goroutine 里跑，与用户刚触发的 `adoptAssetFromFallback`（`check.go:73-88`）、`CheckWintun` 的 alt 探测同时操作同一批路径。
- **影响**：① 用户放在旧布局里的自定义/魔改内核会被静默删除（无备份）；② 清理与自检互为对手，出现"刚判 Ready 又变 Missing"的抖动。
- **修法**：① `migrate.go:137` 失败分支改为 `os.ReadFile`+`WriteFileAtomic` 到 `backupDir`（跨卷安全），再删源；② 清理改为在 `runStartupRuntimeAssetMaintenance` **之后**串行执行（`app.go:190` 去掉 `go`）或至少先 `AcquireAssetLock`；③ `legacyAssets` 列表里的 `wintun.dll` 与 `clash.exe` 走"仅备份不删除"。

#### F-A1-17 Hint 指向不存在的入口；`RepairRuntimeAssets` 是死接口
- **严重度**：P1 ｜ **证据等级**：A
- **位置**：`check.go:104`（`Hint = "请使用"恢复内置组件"或"更新内核"修复。"`）、`check.go:237`（同类 wintun 文案）；`app.go:901-907`（`RepairRuntimeAssets`）；前端检索：`frontend/src` 中 `RepairRuntimeAssets` **0 命中**、`恢复内置组件` **0 命中**（`frontend/dist` 中仅绑定模块自身的打包产物）
- **现象**：错误提示引导用户去点一个 UI 上不存在的按钮；而唯一能执行重写全部资产的接口没有任何调用方（`app.go:901` 只被生成的 `services_v3_generated.go:330` 暴露）。
- **影响**：用户"看见指引却无路可走"，只能卸载重装；同时 §主方案把这句"按钮假死"当成既有问题，前提不成立（见 §3.3）。
- **修法**：① 二选一——把 `check.go:104/237` 的 Hint 改为已存在的"更新内核"`UpdateCoreComponentAsync`/`InstallTunDriverAsync`；或在 `SettingsUpdate.vue` 增加"恢复内置组件"按钮并调用 `RepairRuntimeAssets`（同时按 §3 建议异步化 + 进度事件）；② 若保留接口，必须补进度与可取消，否则 `RepairForce` 95 MB 同步重写在 Wails RPC 上就是一次卡死。

### P2

#### F-A1-18 `resolveAppDir` 的临时目录判断参数颠倒
- **严重度**：P2 ｜ **证据等级**：A
- **位置**：`core/utils/paths.go:76-78`：`strings.Contains(os.TempDir(), dir)`
- **现象**：语义应为"exe 位于临时目录内"，实际判断"临时目录字符串包含 appDir"。当 appDir 是 temp 的祖先（如安装在 `C:\Users\<u>`）时成立，会用 `os.Getwd()` 覆盖 appDir → AppDir/DataDir 双错位。
- **修法**：改为 `strings.HasPrefix(strings.ToLower(dir), strings.ToLower(filepath.Clean(os.TempDir())))`，或直接删掉该分支（`go-build` 与 `wails-dev` 两个判断已覆盖 dev 场景）。

#### F-A1-19 `runtimeassets` 零单测，`shouldCopy` 矩阵无回归网
- **严重度**：P2 ｜ **证据等级**：A
- **位置**：全包无 `_test.go`（仓库仅有 6 个测试文件，无一引用 `runtimeassets`）；判定核心 `sync.go:176-203`、`status.go:75-90`
- **现象**：`shouldCopy` 是 3 种 mode × 6 种资产 × Exists/Ready/Valid/VersionProbeOK × appUpdated 的组合矩阵，完全靠人读代码推断，改一处极易回归。
- **修法**：把 `repairFromSeed` 的判定抽成纯函数 `func decideCopy(mode RepairMode, key AssetKey, h AssetHealth, appUpdated bool, rec *CopyRecord) bool`，对矩阵建表驱动测试（不需要真实文件，全部用 `AssetHealth` 字面量）。

#### F-A1-20 `Ready` 语义只含 core+wintun、`Required` 对 geo 恒 false
- **严重度**：P2 ｜ **证据等级**：A
- **位置**：`status.go:29-30`、`status.go:40`（`Ready = CoreReady && WintunReady`）；`types.go:70-77`（`baseHealth` 的 `required` 参数在 `check.go:281` 对 4 个 geo 传 `false`）；`status.go:123`（`MergeStatus` 同样只算两者）
- **现象**：geo 四个文件缺失时 `RuntimeAssetStatus.Ready` 仍为 true；`AssetHealth.Required` 也无法表达"当前场景需要 geo"。
- **影响**：UI/诊断若用 `Ready` 做"一切就绪"展示会误导；`GetDataDirInfo`（`app.go:702-703`）另起一套 `CanAutoRepair*`，两套语义并存。
- **修法**：`RuntimeAssetStatus` 增加 `ReadyFor Requirement` 字段或 `GeoReady bool`；`baseHealth` 增加 `requiredInScenario` 由 `requiredAssetDefsFor` 填。

---

## 3 与主方案对应关系

> 对照 `docs/SELF_HEAL_AND_LOGGING_PLAN.md`（v2，618 行）。

### 3.1 已覆盖（我独立复核**成立**，并补充证据）

| 方案条目 | 结论 | 我的补充证据 |
|---|---|---|
| §1.1 #1 `GetStatus` 全量、同步、贵 | ✅ 成立 | 实测 95,336,775 B/次（§0.3）；6 个资产逐个哈希 `check.go:128/268/302` |
| §1.1 #4 `adoptAssetFromFallback` 在检查里写文件 | ✅ 成立 | `check.go:66-89`；且**6 个 UI 只读方法都会触发**（F-A1-04） |
| §1.1 #5 修复路径 2~3× `GetStatus` | ✅ 成立（偏保守） | `status.go:47/56/60`；叠加 4 次 EnsureReady 后可达 8 次（F-A1-10） |
| §1.1 #6 包级 60s 冷却 | ✅ 成立 | `sync.go:110-134`；补充：冷却跨越资产，geo 阶段被 core 错误劫持（F-A1-03） |
| §1.1 #9 watchdog 每 5s 全量自检 | ✅ 成立 | `supervisor.go:72` + `:114-129` + `:155`；补充：仅在 `desired.Tun` 为真时（`supervisor.go:154`），系统代理模式不触发 |
| §1.1 #10 第三次 EnsureReady 且任何错误→硬失败 | ✅ 成立 | `controller.go:655-661`，并被 `control_coordinator.go:128-141` 回滚 |
| §1.1 #15/#20 UI 直连深检 | ✅ 成立 | 6 处 `app.go:667/728/745/757/886/898`，前端 `App.vue:641`、`SettingsTun.vue:118`、`SettingsUpdate.vue:73` |
| §1.2-A wintun `VersionProbeOK` 恒 false | ✅ 成立 | `check.go:230-273` 全文无该字段；`check.go:140` 是唯一赋值点 |
| §1.2-B 失败记忆 / 检查夹带写入 | ✅ 成立 | 同上 + `atomic.go:59-99` 无释放等待（新增对比：`component_replace.go:10-84`） |
| §7 #8/#9/#10/#11/#12/#13/#14/#16/#17/#20 改造项 | ✅ 方向正确 | 均已在我的问题清单里有对应修法；额外补充 #16 的"哈希无消费者"前提（F-A1-14）使该改动风险更低 |
| §11.3 ①② UI 直连 / `RepairForce` | ⚠️ ①成立、②前提不成立 | 见 §3.3-5 |

### 3.2 新增（方案未提，本审计独立发现）

| # | 新增发现 | 严重度 | 原因（方案为何漏） |
|---|---|---|---|
| 1 | **manifest 与二进制实测不一致 + 哈希只写不校验 → 内核升级被永久跳过**（F-A1-06） | P1 | 方案只把 manifest 当"元数据来源"，未做实测对账 |
| 2 | **`Ready` 与 `VersionProbeOK` 解耦，导致 `RepairInvalid` 在快路径上是死逻辑**（F-A1-05） | P1 | 方案 §1.2-A 只看到"进去了就一定重拷"，没看"根本进不去" |
| 3 | **`CheckWintun` alt 目录回退 → 自检乐观误判 + 与 `CleanupLegacyAssets` 互拆**（F-A1-07） | P1 | 方案把 alt 回退当成"修好的历史坑"，未验证反向风险 |
| 4 | **`ctx` 全链路无效**（F-A1-08） | P1 | 方案未检查 `runtimeassets` 是否真的响应取消 |
| 5 | **`ReadCoreVersion` 持全局锁跨子进程**（F-A1-09） | P1 | 方案只提到"60s 缓存"，未提锁粒度 |
| 6 | **`CopyFile` 固定 `.tmp` + 无单飞 → 并发复制互相踩**（F-A1-03） | P0 | 方案 §4.2 提出"单飞"但未指出当前 `atomic.go:70` 的具体缺陷 |
| 7 | **一次开 TUN 是 4 次 EnsureReady（方案算 3 次）**（F-A1-10） | P1 | 方案漏了 `runner.go:114 → setup.go:28` 这一条 |
| 8 | **6 个写者 / 3 套锁 / 无跨路径锁**（F-A1-13） | P1 | 方案只提"需要文件锁"，未盘点现有写者 |
| 9 | **`SHA256` 字段零消费者**（F-A1-14） | P1 | 方案建议"缓存哈希"，未验证"能不能不算" |
| 10 | **启动自愈结果无人消费（前端零订阅）**（F-A1-12） | P1 | 方案 §6 关注"落盘"，未查事件是否有订阅者 |
| 11 | **`ErrIsDir` 永不修复 / geo 静默失败**（F-A1-15） | P1 | 方案未覆盖 `ErrIsDir` 分支 |
| 12 | **seed 目录探测只看存在、manifest 路径推导错位、候选含父目录**（F-A1-11） | P1 | 方案 §1.2 未审 `paths.go` 的 seed 布局 |
| 13 | **跨卷 rename 失败 → 直接删文件**（F-A1-16） | P1 | 方案未审 `CleanupLegacyAssets`（只把它当"清理"） |
| 14 | **Hint 指向不存在入口 / `RepairRuntimeAssets` 无调用方**（F-A1-17） | P1 | 方案假设该按钮存在 |
| 15 | **`resolveAppDir` 反向 `Contains`**（F-A1-18） | P2 | 不在方案范围 |
| 16 | **零单测**（F-A1-19）、**`Ready` 不含 geo**（F-A1-20） | P2 | 不在方案范围 |

### 3.3 方案说错的地方（必须修正）

| # | 方案原文 | 实际情况（file:line） | 修正建议 |
|---|---|---|---|
| 1 | §1.1 #7 / §1.2-A 代码块 / §7 #8 把 `shouldCopy` 定位在 **`core/runtimeassets/check.go:176-203`**、**`check.go:181-188`** | `check.go` 全文只有 321 行且**不含 `shouldCopy` 标识符**（grep 确认）；判定实为 `core/runtimeassets/sync.go:176-207` | 全文替换为 `sync.go:176-207`；§7 #8 的改法要落到 `sync.go:182` |
| 2 | §1.1 #2："超时 → `VersionProbeOK=false` → 触发 61 MB 重拷" | 探针失败时 `Ready` 仍为 true（`check.go:125-137`），而 `EnsureReady` 只看 `Ready`（`status.go:50/75-90`）→ **不会**进入修复分支。必须另有"其它资产不 Ready"或 `RepairForce` 才会重拷 | 改成"探针超时只降级版本文案；61 MB 重拷需先有其它前置条件（见 §11.4-1）" |
| 3 | §1.2-A（"每次自愈都重拷 wintun"）与 §11.4-1（"只有 `requirementSatisfied=false` 时才进入"）**自相矛盾** | 代码支持 §11.4-1 的说法（`status.go:50`） | 方案内部统一为 §11.4-1；§1.2-A 的措辞加"进入修复分支后"限定 |
| 4 | §1.1 #16："Migrate + EnsureReady + EnsureReady + Cleanup，**同步串行**" | Cleanup 是 `go runtimeassets.CleanupLegacyAssets()`（`app.go:190`），**异步**且与前台自检并发 | 改为"前 3 步同步串行，Cleanup 异步并发（因此引入 F-A1-16 的竞态）" |
| 5 | §11.3 ②："`RepairRuntimeAssets` … **按钮点下去是长时间假死**" | 前端 `frontend/src` **无任何调用**（grep 0 命中，仅 `frontend/bindings/goclashz/coreservice.ts:173` 生成绑定）；按钮不存在 | 改为"接口已暴露但无入口，接入时必须先异步化"；否则该问题的优先级应下调 |
| 6 | §1.1 #3："`CheckWintun` … 427 KB ×3 次 IO" | 至少 6 次 open：`adopt` stat、`checkWintunByPath` stat、PE open、`GetFileVersionSize`+`GetFileVersionInfo`（2 次）、SHA256 open | 改为"≈6 次 open + 1 次 Win32 版本 API"（成本量级不变，但"3 次"会误导优化方向） |
| 7 | §3.2："`GetFileVersion` 失败不再等同'不可用'"（当作待改项） | 现状已是如此：`check.go:261-266` 失败只写"已安装，版本未知"，`Ready` 不受影响 | 标记为"已满足"，从待改清单移除，避免重复施工 |
| 8 | §0.1："4 个 geo 文件哈希（合计 ~30 MB）" | 实测 4 个 geo 合计 **33,804,071 B ≈ 32.2 MiB**，加 wintun 427,552 B 后全量 **95.3 MB**（§0.3） | 更新数字；"61 MB"内核应为 61,105,152 B（与方案一致） |
| 9 | §4.2"不在运行中替换" | 方向正确，但方案未指出仓库**已有**可复用的设施：`core/clash/component_replace.go:10-84`（`WaitFileReleased` + 备份 + 10 次重试 + 回滚） | 补一句"直接复用 `ReplaceFileWithBackup`，不要新写替换器"，否则执行者会重造一套 |
| 10 | §11.2 调用点统计：`EnsureReady` 7 处、`GetStatus` 9 处 | ✅ **核对正确**（`app.go:170/181/902`、`setup.go:28`、`controller.go:655/1770`、`supervisor.go:155`；`status.go:47/56/60` + 6 个 UI 方法） | 无需修改，可作为基线 |

---

## 4 未覆盖与待实测

### 4.1 明确未验证（含原因）

| # | 项 | 等级 | 说明 / 补齐方式 |
|---|---|---|---|
| 1 | 冷缓存 + 杀软首扫下单次 `GetStatus` 的真实耗时 | C | 本次禁止运行应用；热缓存参考值 0.06 s/61 MB 来自 A0。补齐：`Measure-Command` 对 95 MB 冷读（清 `Cache` 或换文件）+ 火绒实时防护开启时对比 |
| 2 | `wintun.dll` 被 clash.exe 映射时 `os.Rename` 的具体错误 | B→C | 代码可证"会失败"（image 映射不允许 replace），但未实测错误码；补齐：独立进程 `LoadLibrary` 锁住 DLL 后跑 `CopyFile` |
| 3 | helper 侧 `ReplaceCoreFile`（`updater_orchestrator.go:355`）与主进程自愈的并发影响 | C | helper 实现不在本审计范围；需读 `cmd/goclashz-helper` |
| 4 | Wails v3 `Event.Emit` 在无监听者时是否丢弃 | B | `runtime-assets-status` 在 UI 加载前发出（`app.go:204 → :188`），是否真丢未验证；补齐：装一个早期 `Events.On` 探针日志 |
| 5 | 安装版（`install-profile.json` standard/portable）下的实际目录布局 | C | 本机是 dev 布局（AppDir=`D:\…\bin`，DataDir=`%LOCALAPPDATA%\GoclashZ`）；standard 下 DataDir 常与 AppDir 同卷，会改变 F-A1-16 的结论 |
| 6 | 内核加载 `wintun.dll` 的搜索顺序（是否也搜 PATH/System32） | C | 决定 F-A1-07 的严重度；需实测（在 `core/bin` 无 DLL、PATH 中有时 TUN 是否可用） |
| 7 | `seed` 与 `runtime` 双目录在打包产物中的真实内容 | B | 本机 `build/runtime-assets/core/bin` **没有 clash.exe**（只有 geo+wintun），若该布局被优先选中则 seed core 恒 Missing；打包脚本未读 |
| 8 | `RepairForce` 在 Wails RPC 上的实际卡顿时长 | C | 由 95 MB 写 + rename 推算（数秒），未实测；且当前无调用入口（F-A1-17） |
| 9 | geo 在线补链路（`core/appcore/env.go:15-51` + `downloader`）的重试/续传 | C | 只读到调用边界，未逐行 |

### 4.2 建议的实测顺序（成本从低到高）

1. **加 3 行临时日志**（不改逻辑）：`GetStatus` 入口/出口打 `dur_ms + caller`，跑 10 分钟 TUN，统计调用次数 → 直接证实 F-A1-01/10 的实际频率。
2. **锁住 DLL 复现**：独立进程 `LoadLibrary(wintun.dll)` 后调用一次 `RepairRuntimeAssets`（或把 `EnsureReady` 的 mode 临时改为 `RepairInvalid` 并删 `wintun.dll` 之外的触发条件）→ 抓 `CopyFile` 的错误码，证实 F-A1-02。
3. **冷启动剖面**：`Clear-RecycleBin` 不适用，改用另一个 95 MB 未缓存文件模拟 + 火绒开启，测 `GetStatus` 耗时分布。
4. **manifest 对账脚本**：把 §0.3 的 `Get-FileHash` 循环产物与 `asset-manifest.json` 比一遍，纳入构建流水线（防 F-A1-06 复发）。

### 4.3 与 A0 报告的一处差异（需主审合并时修正）

`docs/audit/A0-crosscutting.md:35` 记录 `RepairRuntimeAssets` 的调用方是"SettingsUpdate（恢复内置组件）"。本审计独立 grep（含 `frontend/src` 全量、`.vue`/`.ts`/`manifest.ts`）**未找到任何调用方**，也未找到"恢复内置组件"文案（仅 `frontend/dist` 打包产物中的绑定模块本体）。建议 A0 该行改为"无前端调用方（死接口）"，否则会与 A1 的 F-A1-17 结论冲突。

---

*本报告为只读审计产物，未修改任何源码/配置。所有 file:line 均对应审计时的仓库快照。*
