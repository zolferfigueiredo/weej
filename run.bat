@echo off
rem Builds WeeJ and runs it here, quitting any running copy first so the new build takes over.
setlocal
cd /d "%~dp0"

if not exist ".git\" goto hooks_done
if not exist ".githooks\" goto hooks_done
set "HOOKSPATH="
for /f "delims=" %%h in ('git config --get core.hooksPath 2^>nul') do set "HOOKSPATH=%%h"
if "%HOOKSPATH%"==".githooks" goto hooks_done
git config core.hooksPath .githooks
echo Enabled the repository git hooks.
:hooks_done

if exist "build\WeeJ.exe" "build\WeeJ.exe" --quit

call build.bat --console
if errorlevel 1 exit /b 1

"build\WeeJ.exe" %*
