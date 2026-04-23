; =========================================================
; GoclashZ - 智能适配安装脚本 (适配 paths.go 逻辑)
; =========================================================

#define MyAppName "GoclashZ"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "Zzz"
#define MyAppExeName "GoclashZ.exe"

[Setup]
; 基础信息
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}

; --- 🎯 自定义安装核心配置 ---
; 默认安装在 C:\Program Files\GoclashZ，但用户可以点击“浏览”更改
DefaultDirName={autopf}\{#MyAppName}
; 允许用户在准备安装页面看到并修改目录
AlwaysShowDirOnReadyPage=yes
; 确保不禁用目录选择页面
DisableDirPage=no

; 权限要求：建议用 Admin，因为如果安装在 C 盘需要权限写入初始内核
PrivilegesRequired=admin

; 输出设置
OutputDir=.\build\installer
OutputBaseFilename=GoclashZ_Setup
SetupIconFile=.\build\windows\icon.ico
Compression=lzma2/ultra64
SolidCompression=yes

[Languages]
; 直接读取当前目录下的语言包
Name: "chinesesimp"; MessagesFile: "ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; 1. 打包主程序
Source: ".\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

; 2. 🎯 关键：适配 paths.go 的 dataDir 逻辑
; 我们将出厂内核直接放入 {app}\data\core\bin
; 这样如果用户安装在 D:\，paths.go 探测到可写，就会直接在这里运行并更新内核。
Source: ".\data\core\bin\*"; DestDir: "{app}\data\core\bin"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent