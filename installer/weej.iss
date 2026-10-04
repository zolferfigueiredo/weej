; Installer for WeeJ. .github/workflows/release.yml compiles this with /DAppVersion and /DSourceExe.
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceExe
  #define SourceExe "..\build\WeeJ.exe"
#endif

[Setup]
AppId={{82E4CB37-0DB9-4E44-B883-8C04E719EA7D}
AppName=WeeJ
AppVersion={#AppVersion}
AppVerName=WeeJ {#AppVersion}
AppPublisher=Zolfer Figueiredo
AppPublisherURL=https://zolfer.com
AppSupportURL=https://github.com/zolferfigueiredo/weej/issues
AppUpdatesURL=https://github.com/zolferfigueiredo/weej/releases
DefaultDirName={autopf}\WeeJ
DisableDirPage=yes
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
AppMutex=Local\com.zolfer.weej
CloseApplications=yes
RestartApplications=no
OutputBaseFilename=WeeJ-{#AppVersion}-x64-setup
OutputDir=..\dist
SetupIconFile=..\winres\weej.ico
UninstallDisplayIcon={app}\WeeJ.exe
UninstallDisplayName=WeeJ
VersionInfoVersion={#AppVersion}.0
WizardStyle=modern
Compression=lzma2
SolidCompression=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "german"; MessagesFile: "compiler:Languages\German.isl"
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "french"; MessagesFile: "compiler:Languages\French.isl"
Name: "italian"; MessagesFile: "compiler:Languages\Italian.isl"
Name: "polish"; MessagesFile: "compiler:Languages\Polish.isl"
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "ukrainian"; MessagesFile: "compiler:Languages\Ukrainian.isl"
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[CustomMessages]
english.LaunchAtLogin=Launch WeeJ at login
german.LaunchAtLogin=WeeJ bei der Anmeldung starten
spanish.LaunchAtLogin=Iniciar WeeJ al iniciar sesión
french.LaunchAtLogin=Lancer WeeJ à la connexion
italian.LaunchAtLogin=Avvia WeeJ all'accesso
polish.LaunchAtLogin=Uruchamiaj WeeJ przy logowaniu
brazilianportuguese.LaunchAtLogin=Iniciar o WeeJ ao fazer login
russian.LaunchAtLogin=Запускать WeeJ при входе в систему
ukrainian.LaunchAtLogin=Запускати WeeJ під час входу в систему
japanese.LaunchAtLogin=ログイン時に WeeJ を起動する
korean.LaunchAtLogin=로그인할 때 WeeJ 실행
chinesesimplified.LaunchAtLogin=登录时启动 WeeJ

[Tasks]
Name: "login"; Description: "{cm:LaunchAtLogin}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "WeeJ.exe"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\WeeJ"; Filename: "{app}\WeeJ.exe"; AppUserModelID: "Zolfer.WeeJ"

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "WeeJ"; ValueData: """{app}\WeeJ.exe"""; Tasks: login; Flags: uninsdeletevalue
; Covers a Run value the app itself created later via --login on, so uninstall always clears it.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "WeeJ"; Flags: uninsdeletevalue

[Run]
Filename: "{app}\WeeJ.exe"; Description: "{cm:LaunchProgram,WeeJ}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: files; Name: "{app}\WeeJ.exe.old"
