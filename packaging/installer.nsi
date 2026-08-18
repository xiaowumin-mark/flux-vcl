Unicode True

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef TARGET
  !error "TARGET is required"
!endif
!ifndef APP_EXE
  !error "APP_EXE is required"
!endif
!ifndef RUNTIME_DLL
  !error "RUNTIME_DLL is required"
!endif
!ifndef STAGE_DIR
  !error "STAGE_DIR is required"
!endif
!ifndef OUTPUT_FILE
  !error "OUTPUT_FILE is required"
!endif

!include "MUI2.nsh"
!include "Win\RestartManager.nsh"

Name "FluxVCL ${TARGET} example ${VERSION}"
OutFile "${OUTPUT_FILE}"
InstallDir "$LocalAppData\Programs\FluxVCL\${TARGET}"
InstallDirRegKey HKCU "Software\FluxVCL\Examples\${TARGET}" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
SetCompressorDictSize 32
ManifestDPIAware true
BrandingText "FluxVCL ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=1033 "CompanyName" "FluxVCL"
VIAddVersionKey /LANG=1033 "FileDescription" "FluxVCL ${TARGET} example installer"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}.0"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Copyright (c) 2026 FluxVCL contributors"
VIAddVersionKey /LANG=1033 "ProductName" "FluxVCL"
VIAddVersionKey /LANG=1033 "ProductVersion" "${VERSION}"

!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Run the ${TARGET} example"

Function DirectoryPagePre
  SetShellVarContext current
  StrCpy $0 ""
  ClearErrors
  ReadRegStr $0 HKCU "Software\FluxVCL\Examples\${TARGET}" "InstallDir"
  ClearErrors
  StrCmp $0 "" directory_page_show
  StrCpy $INSTDIR $0
  Abort
directory_page_show:
FunctionEnd

!macro DefineCheckTargetProcess UN PREFIX
Function ${UN}CheckTargetProcess
  ; Restart Manager reports consumers by exact path and works across process
  ; architectures without spawning PowerShell or depending on file-lock rules.
  !insertmacro RestartManager_StartSession $5
  StrCmp $5 "" ${PREFIX}_target_process_error

  !insertmacro RestartManager_RegisterFile $5 "$INSTDIR\${APP_EXE}"
  StrCmp $0 "0" 0 ${PREFIX}_target_process_session_error
  !insertmacro RestartManager_RegisterFile $5 "$INSTDIR\${RUNTIME_DLL}"
  StrCmp $0 "0" 0 ${PREFIX}_target_process_session_error

  StrCpy $2 "0"
  StrCpy $3 "0"
  System::Call 'RSTRTMGR::RmGetList(i$5,*i .r2,*i r3,p0,*i .r4)i.r0'
  StrCpy $6 $0
  !insertmacro RestartManager_EndSession $5
  StrCmp $6 "0" ${PREFIX}_target_process_count_ready
  StrCmp $6 "234" ${PREFIX}_target_process_count_ready ${PREFIX}_target_process_error

${PREFIX}_target_process_count_ready:
  IntCmp $2 0 ${PREFIX}_target_process_not_running ${PREFIX}_target_process_running ${PREFIX}_target_process_running

${PREFIX}_target_process_session_error:
  !insertmacro RestartManager_EndSession $5
${PREFIX}_target_process_error:
  Push "error"
  Return
${PREFIX}_target_process_running:
  Push "running"
  Return
${PREFIX}_target_process_not_running:
  Push "stopped"
FunctionEnd
!macroend

!insertmacro DefineCheckTargetProcess "" "install"
!insertmacro DefineCheckTargetProcess "un." "uninstall"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${STAGE_DIR}\LICENSE.txt"
!define MUI_PAGE_CUSTOMFUNCTION_PRE DirectoryPagePre
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "FluxVCL ${TARGET} example" SecMain
  SetShellVarContext current

  ; Reinstall in place. Moving an existing target requires uninstalling first,
  ; otherwise changing the directory would orphan the previous files.
  StrCpy $0 ""
  StrCpy $1 "0"
  ClearErrors
  ReadRegStr $0 HKCU "Software\FluxVCL\Examples\${TARGET}" "InstallDir"
  StrCmp $0 "" install_path_ready
  StrCpy $1 "1"
  StrCpy $INSTDIR $0
install_path_ready:
  StrCmp $1 "0" install_target_stopped
  Call CheckTargetProcess
  Pop $2
  StrCmp $2 "stopped" install_target_stopped
  Goto install_failed

install_target_stopped:
  ClearErrors
  SetOutPath "$INSTDIR"
  IfErrors install_failed
  SetOverwrite on

  File /oname=${APP_EXE} "${STAGE_DIR}\${APP_EXE}"
  IfErrors install_failed
  File /oname=${RUNTIME_DLL} "${STAGE_DIR}\${RUNTIME_DLL}"
  IfErrors install_failed
  File /oname=LICENSE.txt "${STAGE_DIR}\LICENSE.txt"
  IfErrors install_failed
  File /oname=THIRD-PARTY-NOTICES.txt "${STAGE_DIR}\THIRD-PARTY-NOTICES.txt"
  IfErrors install_failed
  File /oname=energye-Apache-2.0.txt "${STAGE_DIR}\energye-Apache-2.0.txt"
  IfErrors install_failed
  File /oname=Go-LICENSE.txt "${STAGE_DIR}\Go-LICENSE.txt"
  IfErrors install_failed
  File /oname=Go-PATENTS.txt "${STAGE_DIR}\Go-PATENTS.txt"
  IfErrors install_failed
  File /oname=dependencies.lock.json "${STAGE_DIR}\dependencies.lock.json"
  IfErrors install_failed

  WriteUninstaller "$INSTDIR\uninstall.exe"
  IfErrors install_failed

  CreateDirectory "$SMPROGRAMS\FluxVCL"
  IfErrors install_failed
  CreateShortcut "$SMPROGRAMS\FluxVCL\${TARGET} example.lnk" "$INSTDIR\${APP_EXE}"
  IfErrors install_failed
  CreateShortcut "$SMPROGRAMS\FluxVCL\Uninstall ${TARGET} example.lnk" "$INSTDIR\uninstall.exe"
  IfErrors install_failed

  WriteRegStr HKCU "Software\FluxVCL\Examples\${TARGET}" "InstallDir" "$INSTDIR"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "DisplayName" "FluxVCL ${TARGET} example"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "DisplayVersion" "${VERSION}"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "Publisher" "FluxVCL"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "InstallLocation" "$INSTDIR"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  IfErrors install_failed
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  IfErrors install_failed
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "NoModify" 1
  IfErrors install_failed
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}" "NoRepair" 1
  IfErrors install_failed
  ClearErrors
  SetErrorLevel 0
  Goto install_done

install_failed:
  SetErrors
  SetErrorLevel 1
  StrCmp $1 "1" install_failed_notice

  ; This was a fresh installation into a previously unregistered directory.
  ; Roll back only the files and entries owned by this package. A failed
  ; in-place reinstall keeps the old registration and uninstaller for retry.
  Delete "$SMPROGRAMS\FluxVCL\${TARGET} example.lnk"
  Delete "$SMPROGRAMS\FluxVCL\Uninstall ${TARGET} example.lnk"
  RMDir "$SMPROGRAMS\FluxVCL"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}"
  DeleteRegKey HKCU "Software\FluxVCL\Examples\${TARGET}"
  DeleteRegKey /ifempty HKCU "Software\FluxVCL\Examples"
  DeleteRegKey /ifempty HKCU "Software\FluxVCL"
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\${RUNTIME_DLL}"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\energye-Apache-2.0.txt"
  Delete "$INSTDIR\Go-LICENSE.txt"
  Delete "$INSTDIR\Go-PATENTS.txt"
  Delete "$INSTDIR\dependencies.lock.json"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  RMDir "$LocalAppData\Programs\FluxVCL"

install_failed_notice:
  SetErrors
  SetErrorLevel 1
  IfSilent install_done
  MessageBox MB_OK|MB_ICONSTOP "FluxVCL ${TARGET} could not be installed. Close the running example and retry."
  Quit

install_done:
SectionEnd

Section "Uninstall"
  SetShellVarContext current

  ; Keep shortcuts and registry entries until every installed file is gone.
  ; Restart Manager checks exact EXE/DLL paths before any file or entry is removed.
  Call un.CheckTargetProcess
  Pop $2
  StrCmp $2 "stopped" remove_installed_files
  Goto uninstall_failed

remove_installed_files:
  ClearErrors
  Delete "$INSTDIR\${APP_EXE}"
  IfFileExists "$INSTDIR\${APP_EXE}" uninstall_failed
  Delete "$INSTDIR\${RUNTIME_DLL}"
  IfFileExists "$INSTDIR\${RUNTIME_DLL}" uninstall_failed
  Delete "$INSTDIR\LICENSE.txt"
  IfFileExists "$INSTDIR\LICENSE.txt" uninstall_failed
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  IfFileExists "$INSTDIR\THIRD-PARTY-NOTICES.txt" uninstall_failed
  Delete "$INSTDIR\energye-Apache-2.0.txt"
  IfFileExists "$INSTDIR\energye-Apache-2.0.txt" uninstall_failed
  Delete "$INSTDIR\Go-LICENSE.txt"
  IfFileExists "$INSTDIR\Go-LICENSE.txt" uninstall_failed
  Delete "$INSTDIR\Go-PATENTS.txt"
  IfFileExists "$INSTDIR\Go-PATENTS.txt" uninstall_failed
  Delete "$INSTDIR\dependencies.lock.json"
  IfFileExists "$INSTDIR\dependencies.lock.json" uninstall_failed
  Delete "$INSTDIR\uninstall.exe"
  IfFileExists "$INSTDIR\uninstall.exe" uninstall_failed
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\FluxVCL\${TARGET} example.lnk"
  Delete "$SMPROGRAMS\FluxVCL\Uninstall ${TARGET} example.lnk"
  RMDir "$SMPROGRAMS\FluxVCL"

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-${TARGET}"
  DeleteRegKey HKCU "Software\FluxVCL\Examples\${TARGET}"
  DeleteRegKey /ifempty HKCU "Software\FluxVCL\Examples"
  DeleteRegKey /ifempty HKCU "Software\FluxVCL"
  RMDir "$LocalAppData\Programs\FluxVCL"
  ClearErrors
  SetErrorLevel 0
  Goto uninstall_done

uninstall_failed:
  SetErrors
  SetErrorLevel 1
  IfSilent uninstall_done
  MessageBox MB_OK|MB_ICONSTOP "Close the FluxVCL ${TARGET} example and retry uninstalling."

uninstall_done:
SectionEnd
