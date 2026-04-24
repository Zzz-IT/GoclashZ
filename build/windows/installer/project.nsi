Unicode true

; --- Compression Settings ---
; Use LZMA solid compression
SetCompressor /SOLID lzma
SetCompressorDictSize 64

; Request user execution level (no UAC prompt)
!define REQUEST_EXECUTION_LEVEL "user"

!include "wails_tools.nsh"

# Version Information
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"

!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

; Setup application auto-run after install
; Flags: nowait postinstall skipifsilent
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES

; Set language to Simplified Chinese
!insertmacro MUI_LANGUAGE "SimpChinese"

Name "${INFO_PRODUCTNAME}"
; Output installer to build/installer directory
OutFile "..\..\..\build\installer\${INFO_PROJECTNAME}_Setup.exe" 

; Core Change 1: Modern "Current User" installation mode
; DefaultDirName={localappdata}\Programs\{#MyAppName}
; Install to AppData/Local/Programs for user permissions
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"

ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section "MainSection" SEC01
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files

    ; Core Change 3: Package extra core binaries
    ; Source: ".\data\core\bin\*"; DestDir: "{app}\data\core\bin"
    ; NSIS script path is build/windows/installer/, go back 3 levels
    SetOutPath "$INSTDIR\data\core\bin"
    File /a /r "..\..\..\data\core\bin\*"
    
    ; Restore default output path to install root
    SetOutPath $INSTDIR

    ; Create desktop and start menu shortcuts
    CreateDirectory "$SMPROGRAMS\${INFO_PRODUCTNAME}"
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    ; Clean WebView2 user data cache on uninstall
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" 

    ; Recursively delete installation directory
    RMDir /r "$INSTDIR"

    ; Clean up shortcuts
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk"
    RMDir "$SMPROGRAMS\${INFO_PRODUCTNAME}"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

