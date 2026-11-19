@echo off
setlocal DisableDelayedExpansion
pushd "%~dp0" || goto directory_error
if not exist ".\ai-interview-arena.exe" goto missing_binary

echo AI Interview Arena - local startup
echo Keep this window open. Choose Settings ^> Stop the app in your browser or press Ctrl+C to quit.
".\ai-interview-arena.exe" --root . %*
set "arena_status=%errorlevel%"
popd
if "%arena_status%"=="0" exit /b 0
if "%arena_status%"=="130" exit /b 130
if "%arena_status%"=="-1073741510" exit /b -1073741510
echo.
echo The program exited with error code %arena_status%. See the message above and README.md.
pause
exit /b %arena_status%

:missing_binary
popd
echo AI Interview Arena: the executable is missing.
echo Extract the complete ZIP package into a writable folder and try again.
pause
exit /b 1

:directory_error
echo AI Interview Arena: could not open the kit folder.
pause
exit /b 1
