; S-Check Desktop: Installation für den angemeldeten Benutzer, ohne Administratorrechte.
;
;   s-check-setup.exe                                   Assistent
;   s-check-setup.exe /S /SERVER=https://s-check.firma.de   still, Serveradresse vorbelegt
;
; makensis -DVERSION=1.0.0 -DSRC=<Ordner mit s-check.exe> -DOUT=<Datei> s-check.nsi

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!define APP "S-Check"
!define EXE "s-check.exe"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\S-Check"

Name "${APP}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\S-Check"
InstallDirRegKey HKCU "Software\S-Check" "InstallDir"
BrandingText "${APP} ${VERSION}"
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=1031 "ProductName" "${APP}"
VIAddVersionKey /LANG=1031 "FileDescription" "${APP} Desktop Installation"
VIAddVersionKey /LANG=1031 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1031 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=1031 "CompanyName" "${APP}"
VIAddVersionKey /LANG=1031 "LegalCopyright" "${APP}"

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "${SRC}\s-check.ico"
!define MUI_UNICON "${SRC}\s-check.ico"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "S-Check ${VERSION} installieren"
!define MUI_WELCOMEPAGE_TEXT "S-Check prüft S-Series- und S1000D-Dokumentation. Diese Anwendung enthält die Oberfläche und verbindet sich bei der Anmeldung mit dem S-Check-Server Ihrer Organisation. Projektdaten werden nicht auf diesem Rechner gespeichert.$\r$\n$\r$\nFür die Installation sind keine Administratorrechte nötig."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "S-Check jetzt starten"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Verknüpfung auf dem Desktop anlegen"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION DesktopShortcut
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "German"

; A running S-Check keeps its program file open; ask to close it instead of
; replacing a file that is in use.
!macro WaitClosed
  ${Do}
    ClearErrors
    ${IfNot} ${FileExists} "$INSTDIR\${EXE}"
      ${Break}
    ${EndIf}
    FileOpen $0 "$INSTDIR\${EXE}" a
    ${IfNot} ${Errors}
      FileClose $0
      ${Break}
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "S-Check ist noch geöffnet.$\r$\nBitte alle S-Check-Fenster schließen und dann „Wiederholen“ wählen." /SD IDCANCEL IDRETRY +2
    Abort
  ${Loop}
!macroend

Function DesktopShortcut
  CreateShortCut "$DESKTOP\S-Check.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
FunctionEnd

Function .onInit
  ; Edge is part of Windows 10/11; without it S-Check falls back to the default browser
  ReadRegStr $1 HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe" ""
  ${If} $1 == ""
    ReadRegStr $1 HKCU "SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe" ""
  ${EndIf}
  ${If} $1 == ""
    MessageBox MB_OK|MB_ICONINFORMATION "Microsoft Edge wurde auf diesem Rechner nicht gefunden. S-Check öffnet sich dann im Standardbrowser statt in einem eigenen Fenster." /SD IDOK
  ${EndIf}
FunctionEnd

Section "S-Check" SecMain
  SectionIn RO
  !insertmacro WaitClosed
  SetOutPath "$INSTDIR"
  File "${SRC}\${EXE}"
  File "${SRC}\s-check.ico"
  WriteUninstaller "$INSTDIR\Deinstallieren.exe"

  CreateShortCut "$SMPROGRAMS\S-Check.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0 SW_SHOWNORMAL "" "S-Check: Prüfung von S-Series-Dokumentation"

  ; /SERVER=<Adresse> belegt die Serveradresse für diesen Benutzer vor
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/SERVER=" $R1
  ${IfNot} ${Errors}
  ${AndIf} $R1 != ""
    WriteRegStr HKCU "Software\S-Check" "ServerURL" "$R1"
  ${EndIf}
  WriteRegStr HKCU "Software\S-Check" "InstallDir" "$INSTDIR"

  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "S-Check"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "S-Check"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${EXE},0"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Deinstallieren.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\Deinstallieren.exe" /S'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "Uninstall"
  !insertmacro WaitClosed
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\s-check.ico"
  Delete "$INSTDIR\Deinstallieren.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\S-Check.lnk"
  Delete "$DESKTOP\S-Check.lnk"
  ; window profile and log only (cache, cookies); no project data lives here
  RMDir /r "$LOCALAPPDATA\S-Check"
  MessageBox MB_YESNO|MB_ICONQUESTION "Auch die gespeicherte Serveradresse entfernen?" /SD IDNO IDNO keep
    RMDir /r "$APPDATA\S-Check"
    DeleteRegKey HKCU "Software\S-Check"
    Goto done
  keep:
    DeleteRegValue HKCU "Software\S-Check" "InstallDir"
  done:
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
