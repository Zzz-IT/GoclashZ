; =========================================================
; GoclashZ - 智能适配安装脚本 (适配 paths.go 逻辑)
; =========================================================

#define MyAppName "GoclashZ"
#define MyAppVersion "1.2.0"
#define MyAppPublisher "Zzz"
#define MyAppExeName "GoclashZ.exe"

[Setup]
WizardStyle=modern dynamic includetitlebar
VersionInfoVersion=1.2.0.0
VersionInfoCompany=Zzz
VersionInfoDescription=GoclashZ Installer
VersionInfoCopyright=Copyright (C) 2026 Zzz
; 基础信息
AppName={#MyAppName}
AppVerName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}

; --- 核心修改 1：采用现代“当前用户”安装模式 ---
; 默认安装到 C:\Users\用户名\AppData\Local\Programs\GoclashZ
; 这样做目录永远拥有写入权限，paths.go 会完美启用 {app}\data 便携模式！
DefaultDirName={localappdata}\Programs\{#MyAppName}
AlwaysShowDirOnReadyPage=yes
DisableDirPage=no

; --- 核心修改 2：降级权限要求 ---
; 软件安装和日常运行不需要管理员权限。
; TUN 模式通过 GoclashZHelper 后台服务提供高权限能力，无需 UI 提权。
PrivilegesRequired=lowest

; 输出设置
OutputDir=.\build\installer
OutputBaseFilename=GoclashZ_win_amd64_Setup
SetupIconFile=.\build\windows\icon.ico
Compression=lzma2/ultra64
SolidCompression=yes

[Languages]
Name: "chinesesimp"; MessagesFile: "ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; 1. 打包主程序
Source: ".\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

; 2. 打包 Helper 服务程序 (用于 TUN、内核更新等高权限操作)
Source: ".\build\bin\GoclashZHelper.exe"; DestDir: "{app}"; Flags: ignoreversion

; 3. --- 核心修改 3：修正打包源路径 ---
; 源码中内核存放在 .\data\core\bin，打包时我们把它塞进安装目录的 {app}\core\bin 中
; 排除在开发运行时产生的临时下载文件和内核缓存数据库 (如 cache.db, geoip.metadb)
Source: ".\data\core\bin\*"; DestDir: "{app}\core\bin"; Excludes: "*.tmp,*.zip,*.old,*.txt,*.json,*.db,*.metadb,*.meta.json"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Dirs]
Name: "{app}\data"; Permissions: users-modify
Name: "{app}\data\Settings"; Permissions: users-modify
Name: "{app}\data\Subscriptions"; Permissions: users-modify
Name: "{app}\data\profiles"; Permissions: users-modify

Name: "{app}\core"; Permissions: users-readexec
Name: "{app}\core\bin"; Permissions: users-readexec

[INI]
Filename: "{app}\.installed"; Section: "Install"; Key: "Status"; String: "Installed"

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent
