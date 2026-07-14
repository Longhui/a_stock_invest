@echo off
chcp 65001 >nul
cd /d "%~dp0"
C:/Users/raolh/.workbuddy/binaries/go/go/bin/go run main.go %*
pause
