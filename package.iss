; =========================================================
; GoclashZ - 鏅鸿兘閫傞厤瀹夎鑴氭湰 (閫傞厤 paths.go 閫昏緫)
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
; 鍩虹淇℃伅
AppName={#MyAppName}
AppVerName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}

; --- 馃幆 鏍稿績淇敼 1锛氶噰鐢ㄧ幇浠ｂ€滃綋鍓嶇敤鎴封€濆畨瑁呮ā寮?---
; 榛樿瀹夎鍒?C:\Users\鐢ㄦ埛鍚峔AppData\Local\Programs\GoclashZ
; 杩欐牱鍋氱洰褰曟案杩滄嫢鏈夊啓鍏ユ潈闄愶紝paths.go 浼氬畬缇庡惎鐢?{app}\data 渚挎惡妯″紡锛?
DefaultDirName={localappdata}\Programs\{#MyAppName}
AlwaysShowDirOnReadyPage=yes
DisableDirPage=no

; --- 馃幆 鏍稿績淇敼 2锛氶檷绾ф潈闄愯姹?---
; 杞欢瀹夎鍜屾棩甯歌繍琛屼笉闇€瑕佺鐞嗗憳鏉冮檺銆?
; (寮€鍚?TUN 铏氭嫙缃戝崱鏃讹紝浣犱唬鐮侀噷鐨?sys.CheckAdmin 浼氳嚜鍔ㄥ脊鍑?UAC 鎻愭潈锛屼綋楠屾洿濂?
PrivilegesRequired=lowest

; 杈撳嚭璁剧疆
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
; 1. 鎵撳寘涓荤▼搴?
Source: ".\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

; 2. --- 馃幆 鏍稿績淇敼 3锛氫慨姝ｆ墦鍖呮簮璺緞 ---
; 婧愮爜涓唴鏍稿瓨鏀惧湪 .\data\core\bin锛屾墦鍖呮椂鎴戜滑鎶婂畠濉炶繘瀹夎鐩綍鐨?{app}\data\core\bin 涓?
; 鎺掗櫎鍦ㄥ紑鍙戣繍琛屾椂浜х敓鐨勪复鏃朵笅杞芥枃浠跺拰鍐呮牳缂撳瓨鏁版嵁搴?(濡?cache.db, geoip.metadb)
Source: ".\data\core\bin\*"; DestDir: "{app}\data\core\bin"; Excludes: "*.tmp,*.zip,*.old,*.txt,*.json,*.db,*.metadb"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent
