@echo off
rem Installs WeeJ to start at login and keeps it running. install.bat --uninstall removes it.
setlocal
cd /d "%~dp0"

set "LOG=%%TEMP%%\weej.log"

if "%~1"=="--uninstall" goto do_uninstall

if exist "build\WeeJ.exe" goto have_exe
call build.bat
if errorlevel 1 exit /b 1
:have_exe
"build\WeeJ.exe" --quit

set "ARGS=--keep-alive"
if not "%~1"=="" set "ARGS=%ARGS% %~1"
set "ARGS=%ARGS% --log %LOG%"

"build\WeeJ.exe" --login on %ARGS%
if errorlevel 1 exit /b 1
"build\WeeJ.exe" --detach %ARGS%
if errorlevel 1 exit /b 1

echo Installed and running. It will start again at every login.
echo Log: %LOG%
echo Remove it with: install.bat --uninstall
exit /b 0

:do_uninstall
if exist "build\WeeJ.exe" goto have_exe_u
call build.bat
if errorlevel 1 exit /b 1
:have_exe_u
"build\WeeJ.exe" --quit
"build\WeeJ.exe" --login off
if errorlevel 1 exit /b 1
echo Uninstalled.
exit /b 0
