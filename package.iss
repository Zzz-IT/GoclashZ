; =========================================================
; GoclashZ - 閺呴缚鍏橀柅鍌炲帳鐎瑰顥婇懘姘拱 (闁倿鍘?paths.go 闁槒绶?
; =========================================================

#define MyAppName "GoclashZ"
#define MyAppVersion "1.2.1"
#define MyAppPublisher "Zzz"
#define MyAppExeName "GoclashZ.exe"

[Setup]
WizardStyle=modern dynamic includetitlebar
VersionInfoVersion=1.2.1.0
VersionInfoCompany=Zzz
VersionInfoDescription=GoclashZ Installer
VersionInfoCopyright=Copyright (C) 2026 Zzz
; 閸╄櫣顢呮穱鈩冧紖
AppName={#MyAppName}
AppVerName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}

; --- 閺嶇绺炬穱顔芥暭 1閿涙岸鍣伴悽銊у箛娴狅絺鈧粌缍嬮崜宥囨暏閹村皝鈧繂鐣ㄧ憗鍛佸?---
; 姒涙顓荤€瑰顥婇崚?C:\Users\閻劍鍩涢崥宄擜ppData\Local\Programs\GoclashZ
; 鏉╂瑦鐗遍崑姘辨窗瑜版洘妗堟潻婊勫閺堝鍟撻崗銉︽綀闂勬劧绱漰aths.go 娴兼艾鐣紘搴℃儙閻?{app}\data 娓氭寧鎯″Ο鈥崇础閿?DefaultDirName={localappdata}\Programs\{#MyAppName}
AlwaysShowDirOnReadyPage=yes
DisableDirPage=no

; --- 閺嶇绺炬穱顔芥暭 2閿涙岸妾风痪褎娼堥梽鎰洣濮?---
; 鏉烆垯娆㈢€瑰顥婇崪灞炬）鐢瓕绻嶇悰灞肩瑝闂団偓鐟曚胶顓搁悶鍡楁喅閺夊啴妾洪妴?; TUN 濡€崇础闁俺绻?GoclashZHelper 閸氬骸褰撮張宥呭閹绘劒绶垫妯绘綀闂勬劘鍏橀崝娑崇礉閺冪娀娓?UI 閹绘劖娼堥妴?PrivilegesRequired=lowest

; 鏉堟挸鍤拋鍓х枂
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
; 1. 閹垫挸瀵樻稉鑽も柤鎼?Source: ".\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

; 2. 閹垫挸瀵?Helper 閺堝秴濮熺粙瀣碍 (閻劋绨?TUN閵嗕礁鍞撮弽鍛婃纯閺傛壆鐡戞妯绘綀闂勬劖鎼锋担?
Source: ".\build\bin\GoclashZHelper.exe"; DestDir: "{app}"; Flags: ignoreversion

; 3. --- 閺嶇绺炬穱顔芥暭 3閿涙矮鎱ㄥ锝嗗ⅵ閸栧懏绨捄顖氱窞 ---
; 濠ф劗鐖滄稉顓炲敶閺嶇鐡ㄩ弨鎯ф躬 .\data\core\bin閿涘本澧﹂崠鍛閹存垳婊戦幎濠傜暊婵夌偠绻樼€瑰顥婇惄顔肩秿閻?{app}\core\bin 娑?; 閹烘帡娅庨崷銊ョ磻閸欐垼绻嶇悰灞炬娴溠呮晸閻ㄥ嫪澶嶉弮鏈电瑓鏉炶姤鏋冩禒璺烘嫲閸愬懏鐗崇紓鎾崇摠閺佺増宓佹惔?(婵?cache.db, geoip.metadb)
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
