@echo off
setlocal

REM Build frontend (web/dist) for new-api on Windows.
REM See: scripts\windows\build-web.ps1

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-web.ps1" %*

