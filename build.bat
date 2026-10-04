@echo off
rem Builds build\WeeJ.exe. Pass --console for a build that keeps its console window (run.bat uses this).
setlocal
cd /d "%~dp0"

set "GO=go"
where go >nul 2>nul
if not errorlevel 1 goto have_go
set "GO=%ProgramFiles%\Go\bin\go.exe"
if exist "%GO%" goto have_go
echo Go isn't installed: winget install GoLang.Go (then open a new terminal).
exit /b 1
:have_go

set "CONSOLE=0"
if "%~1"=="--console" set "CONSOLE=1"

set "VALUEPART="
for /f "tokens=1,* delims==" %%a in ('findstr /r /c:"^const AppVersion = " "internal\core\version.go"') do set "VALUEPART=%%b"
set VERSION=%VALUEPART:"=%
set VERSION=%VERSION: =%
if not "%VERSION%"=="" goto have_version
echo Couldn't read AppVersion from internal\core\version.go.
exit /b 1
:have_version

"%GO%" test ./...
if errorlevel 1 exit /b 1

"%GO%" run ./tools/mkicon winres
if errorlevel 1 exit /b 1

set "GOOS="
set "GOARCH="
"%GO%" tool go-winres make --in winres\winres.json --arch amd64 --product-version %VERSION%.0 --file-version %VERSION%.0
if errorlevel 1 exit /b 1

if not exist "build" mkdir "build"
if exist "build\WeeJ.exe" del /f /q "build\WeeJ.exe"

set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=amd64"

set "LDFLAGS=-s -w"
if "%CONSOLE%"=="0" set "LDFLAGS=%LDFLAGS% -H windowsgui"

"%GO%" build -trimpath -ldflags "%LDFLAGS%" -o build\WeeJ.exe .
if errorlevel 1 exit /b 1

echo Built: build\WeeJ.exe (%VERSION%)
if "%CONSOLE%"=="1" echo Console build: the slider line prints in the terminal.
