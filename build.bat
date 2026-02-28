@echo off
echo Building Claude Router (no console window)...

cd /d "%~dp0"

REM 使用 -gui 标志编译，隐藏命令行窗口
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags="-H windowsgui" -o main.exe ./cmd/server

$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o main.exe -ldflags="-s -w" cmd/server/main.go
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o main ./cmd/onlyback
if %errorlevel% neq 0 (
    echo Build failed!
    pause
    exit /b 1
)

echo Build successful: claude-router.exe
echo This executable runs without showing a console window.
pause
