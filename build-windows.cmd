@echo off
setlocal

echo Skapar Windows-resurs med programikonen...
go run github.com/akavel/rsrc@latest -arch amd64 -ico assets\icon.ico -o resource.syso
if errorlevel 1 exit /b 1

echo Bygger lokal-fildelning.exe...
go build -ldflags="-H=windowsgui" -o lokal-fildelning.exe .
if errorlevel 1 exit /b 1

echo Klar: lokal-fildelning.exe
