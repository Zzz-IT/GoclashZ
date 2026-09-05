; =========================================================
; GoclashZ - 智能适配安装脚本 (适配 paths.go 逻辑)
; =========================================================

#define MyAppName "GoclashZ"
#define MyAppVersion "1.3.0"
#define MyAppPublisher "Zzz"
#define MyAppExeName "GoclashZ.exe"

[Setup]
AppMutex=Global\GoclashZ_Single_Instance_Mutex
CloseApplications=yes
RestartApplications=no
WizardStyle=modern dynamic includetitlebar
VersionInfoVersion=1.3.0.0
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

; CI 拉取的最新资产只作为 seed
Source: ".\build\runtime-assets\core\bin\*"; DestDir: "{app}\seed\core\bin"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: ".\build\runtime-assets\core\asset-manifest.json"; DestDir: "{app}\seed\core"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Dirs]
Name: "{app}"; Permissions: users-readexec
Name: "{app}\seed"; Permissions: users-readexec
Name: "{app}\seed\core"; Permissions: users-readexec
Name: "{app}\seed\core\bin"; Permissions: users-readexec

[INI]
Filename: "{app}\.installed"; Section: "Install"; Key: "Status"; String: "Installed"

[Run]
Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent unchecked runasoriginaluser

[Code]
function EscapeString(S: string): string;
begin
  StringChangeEx(S, '\', '\\', True);
  StringChangeEx(S, '"', '\"', True);
  Result := S;
end;

function IsDefaultInstallDir(): Boolean;
var
  DefaultDir: string;
begin
  DefaultDir := ExpandConstant('{localappdata}\Programs\GoclashZ');
  Result := CompareText(RemoveBackslashUnlessRoot(WizardDirValue), RemoveBackslashUnlessRoot(DefaultDir)) = 0;
end;

function GetInstallMode(): string;
begin
  if IsDefaultInstallDir() then
    Result := 'standard'
  else
    Result := 'self-contained';
end;

function GetResolvedDataDir(): string;
begin
  if IsDefaultInstallDir() then
    Result := ExpandConstant('{localappdata}\GoclashZ')
  else
    Result := AddBackslash(WizardDirValue) + 'data';
end;

procedure WriteInstallProfile();
var
  Mode, DataDir, Json, Path: string;
begin
  Mode := GetInstallMode();
  DataDir := GetResolvedDataDir();

  Json :=
    '{' + #13#10 +
    '  "mode": "' + Mode + '",' + #13#10 +
    '  "appDir": "' + EscapeString(WizardDirValue) + '",' + #13#10 +
    '  "dataDir": "' + EscapeString(DataDir) + '",' + #13#10 +
    '  "createdBy": "installer"' + #13#10 +
    '}';

  Path := AddBackslash(WizardDirValue) + 'install-profile.json';
  SaveStringToFile(Path, Json, False);
end;

procedure CreateDataDirs();
var
  DataDir: string;
  ResultCode: Integer;
begin
  DataDir := GetResolvedDataDir();

  ForceDirectories(DataDir);
  ForceDirectories(AddBackslash(DataDir) + 'Settings');
  ForceDirectories(AddBackslash(DataDir) + 'Subscriptions');
  ForceDirectories(AddBackslash(DataDir) + 'profiles');
  ForceDirectories(AddBackslash(DataDir) + 'runtime');
  ForceDirectories(AddBackslash(DataDir) + 'core\bin');
  ForceDirectories(AddBackslash(DataDir) + 'updates');
  ForceDirectories(AddBackslash(DataDir) + 'logs');
  ForceDirectories(AddBackslash(DataDir) + 'backups');

  // 确保数据目录对当前用户完全可写
  Exec('icacls', '"' + DataDir + '" /grant Users:(OI)(CI)F /T /Q', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function IsDirWritable(Dir: string): Boolean;
var
  TestFile: string;
begin
  Result := False;
  ForceDirectories(Dir);
  TestFile := AddBackslash(Dir) + '.goclashz_write_test';
  if SaveStringToFile(TestFile, 'test', False) then begin
    DeleteFile(TestFile);
    Result := True;
  end;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  DataDir: string;
begin
  Result := True;

  if CurPageID = wpSelectDir then begin
    DataDir := GetResolvedDataDir();

    if not IsDefaultInstallDir() then begin
      if not IsDirWritable(WizardDirValue) then begin
        MsgBox('当前自定义安装目录不可写。请选择普通用户可写目录，或使用默认安装目录。', mbError, MB_OK);
        Result := False;
        Exit;
      end;
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssPostInstall then begin
    CreateDataDirs();
    WriteInstallProfile();
  end;
end;
