@echo off
rem Set AppVersion in internal\core\version.go first. Tags the current commit and pushes it;
rem .github\workflows\release.yml builds the exe, zip and installer and publishes the GitHub release.
setlocal EnableDelayedExpansion
cd /d "%~dp0"

set "VALUEPART="
for /f "tokens=1,* delims==" %%a in ('findstr /r /c:"^const AppVersion = " "internal\core\version.go"') do set "VALUEPART=%%b"
set VERSION=%VALUEPART:"=%
set VERSION=%VERSION: =%
if not "%VERSION%"=="" goto have_version
echo Couldn't read AppVersion from internal\core\version.go.
exit /b 1
:have_version

set "BRANCH="
for /f "delims=" %%b in ('git symbolic-ref --short HEAD 2^>nul') do set "BRANCH=%%b"
if "%BRANCH%"=="main" goto branch_ok
echo Switch to main first.
exit /b 1
:branch_ok

git diff --quiet
if errorlevel 1 goto dirty
git diff --cached --quiet
if errorlevel 1 goto dirty
goto clean
:dirty
echo Commit or stash your changes first.
exit /b 1
:clean

git fetch --quiet origin main
if errorlevel 1 exit /b 1

set "HEADREV="
set "ORIGINREV="
for /f "delims=" %%h in ('git rev-parse HEAD') do set "HEADREV=%%h"
for /f "delims=" %%o in ('git rev-parse origin/main') do set "ORIGINREV=%%o"
if "%HEADREV%"=="%ORIGINREV%" goto up_to_date
echo Push or pull main first.
exit /b 1
:up_to_date

set "TAG=v%VERSION%"
git rev-parse -q --verify "refs/tags/%TAG%" >nul 2>nul
if not errorlevel 1 goto tag_exists
git ls-remote --exit-code origin "refs/tags/%TAG%" >nul 2>nul
if not errorlevel 1 goto tag_exists
goto tag_ok
:tag_exists
echo %TAG% already exists. Raise AppVersion in internal\core\version.go first.
exit /b 1
:tag_ok

git tag -a "%TAG%" -m "WeeJ %VERSION%"
if errorlevel 1 exit /b 1
git push origin "%TAG%"
if errorlevel 1 exit /b 1

set "RELURL=https://github.com/zolferfigueiredo/weej/releases/download/%TAG%/WeeJ-%VERSION%-x64-setup.exe"

echo Pushed %TAG%. GitHub Actions builds and publishes the release:
echo https://github.com/zolferfigueiredo/weej/actions
echo The installer, once it's published:
echo %RELURL%

if not "%~1"=="--url" exit /b 0

set "TMPFILE=%TEMP%\weej_release_loc.txt"
curl.exe -fsS -o nul -w "%%{redirect_url}" --data-urlencode "url=%RELURL%" https://url.zolfer.com/dmg > "%TMPFILE%"
if errorlevel 1 exit /b 1
set "LOC="
for /f "usebackq delims=" %%l in ("%TMPFILE%") do set "LOC=%%l"
del /f /q "%TMPFILE%" >nul 2>nul

set "S=%LOC%"
set "LASTPOS=-1"
set "LEN=0"
:measure_loc
if not "!S:~%LEN%,1!"=="" (
    set /a LEN+=1
    goto measure_loc
)
for /l %%i in (0,1,%LEN%) do (
    set "PAIR=!S:~%%i,2!"
    if "!PAIR!"=="c=" set /a LASTPOS=%%i
)
if %LASTPOS% lss 0 goto no_match
set /a START=LASTPOS+2
call set "CODE=%%S:~%START%%%"
goto got_code
:no_match
set "CODE=%S%"
:got_code
echo Download link: https://url.zolfer.com/%CODE%
