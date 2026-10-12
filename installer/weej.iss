; Installer for WeeJ. installer.bat and .github/workflows/release.yml compile this with /DAppVersion,
; and the release once per architecture with /DArch=x64 or /DArch=x86 and that build's /DSourceExe.
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef Arch
  #define Arch "x64"
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
#if Arch == "x64"
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#else
ArchitecturesAllowed=x86compatible
#endif
; No AppMutex: [Code] closes a running copy itself instead of asking the user to.
CloseApplications=yes
RestartApplications=no
OutputBaseFilename=WeeJ-{#AppVersion}-{#Arch}-setup
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

[Tasks]
; Only on a first install, so installing over WeeJ keeps what was chosen in its menu since.
Name: "login"; Description: "{cm:LaunchAtLogin}"; Flags: unchecked; Check: not IsUpgrade

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

[Code]
const
  // mutexName and MainWindowClass in internal/platform/sys/instance.go.
  AppMutexName = 'Local\com.zolfer.weej';
  AppWindowClass = 'WeeJ.Main';
  UninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{82E4CB37-0DB9-4E44-B883-8C04E719EA7D}_is1';

function RegisterWindowMessage(Name: String): Longint;
  external 'RegisterWindowMessageW@user32.dll stdcall';

// Quits a running WeeJ the way WeeJ.exe --quit does (the WeeJ.Quit message to its main window),
// so nobody has to find it in the notification area first.
function CloseRunningApp(): Boolean;
var
  Wnd: HWND;
  Waited: Integer;
begin
  if not CheckForMutexes(AppMutexName) then
  begin
    Result := True;
    Exit;
  end;
  Wnd := FindWindowByClassName(AppWindowClass);
  if Wnd <> 0 then
    PostMessage(Wnd, RegisterWindowMessage('WeeJ.Quit'), 0, 0);
  Waited := 0;
  while CheckForMutexes(AppMutexName) and (Waited < 10000) do
  begin
    Sleep(100);
    Waited := Waited + 100;
  end;
  Result := not CheckForMutexes(AppMutexName);
end;

function IsUpgrade(): Boolean;
begin
  Result := RegKeyExists(HKCU, UninstallKey);
end;

// Runs when installing starts, so cancelling the wizard leaves WeeJ running.
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  if CloseRunningApp() then
    Result := ''
  else
    Result := FmtMessage(SetupMessage(msgSetupAppRunningError), ['WeeJ']);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    CloseRunningApp();
end;
