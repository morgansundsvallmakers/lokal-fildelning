@echo off
setlocal

if exist resource.syso del /q resource.syso
if exist rsrc_windows_amd64.syso del /q rsrc_windows_amd64.syso

echo Skapar Windows-resurs med programikonen...
go run github.com/tc-hib/go-winres@latest simply --arch amd64 --icon assets\tray-icon.png --manifest gui --file-description "Lokal fildelning" --product-name "Lokal fildelning" --original-filename "lokal-fildelning.exe"
if errorlevel 1 exit /b 1

echo Bygger lokal-fildelning.exe...
go build -ldflags="-H=windowsgui" -o lokal-fildelning.exe .
if errorlevel 1 exit /b 1

echo Klar: lokal-fildelning.exe
