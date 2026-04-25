; =========================================================
; GoclashZ - 智能适配安装脚本 (适配 paths.go 逻辑)
; =========================================================

#define MyAppName "GoclashZ"
#define MyAppVersion "1.1.1"
#define MyAppPublisher "Zzz"
#define MyAppExeName "GoclashZ.exe"

[Setup]
WizardStyle=modern dynamic includetitlebar
VersionInfoVersion=1.1.1.0
VersionInfoCompany=Zzz
VersionInfoDescription=GoclashZ Installer
VersionInfoCopyright=Copyright (C) 2026 Zzz
; 基础信息
AppName={#MyAppName}
AppVerName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}

; --- 🎯 核心修改 1：采用现代“当前用户”安装模式 ---
; 默认安装到 C:\Users\用户名\AppData\Local\Programs\GoclashZ
; 这样做目录永远拥有写入权限，paths.go 会完美启用 {app}\data 便携模式！
DefaultDirName={localappdata}\Programs\{#MyAppName}
AlwaysShowDirOnReadyPage=yes
DisableDirPage=no

; --- 🎯 核心修改 2：降级权限要求 ---
; 软件安装和日常运行不需要管理员权限。
; (开启 TUN 虚拟网卡时，你代码里的 sys.CheckAdmin 会自动弹出 UAC 提权，体验更好)
PrivilegesRequired=lowest

; 输出设置
OutputDir=.\build\installer
OutputBaseFilename=GoclashZ_Setup
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

; 2. --- 🎯 核心修改 3：修正打包源路径 ---
; 源码中内核存放在 .\core\bin，打包时我们把它塞进安装目录的 {app}\data\core\bin 下
Source: ".\data\core\bin\*"; DestDir: "{app}\data\core\bin"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent