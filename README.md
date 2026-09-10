# Lokal fildelning

Lokal fildelning är en enkel lokal filöverföringstjänst för hem och makerspace. Den körs på en dator i det lokala nätverket och låter enheter på samma LAN ladda upp, se, hämta och radera tillfälliga filer – utan konton eller databas.

## Teknik

Servern är skriven i Go. Frontenden ligger i `public/` i källkoden och bäddas in i den färdiga binären med Go `embed`, så den byggda `.exe`-filen kan köras utan en separat `public/`-mapp.

På Windows öppnas `http://localhost:3000` automatiskt i standardwebbläsaren när programmet startar.

Uppladdade filer och metadata lagras lokalt i `uploads/` och versionshanteras inte.

## Bygg

Du behöver Go 1.27 eller senare.

För Windows-versionen utan terminalfönster och med programikonen inbakad i `.exe`-filen:

```cmd
build-windows.cmd
```

Byggskriptet skapar Windows-resursen från `assets\tray-icon.png` med `go-winres` och bygger därefter `lokal-fildelning.exe`. Den genererade resursfilen versionshanteras inte.

För utveckling, med terminal och loggutskrifter:

```cmd
go run .
```

## Starta

Kör:

```cmd
lokal-fildelning.exe
```

Standardwebbläsaren öppnas automatiskt. Servern fortsätter att köra även om webbläsarfönstret stängs. För att avsluta programmet öppnar du sidan på den dator där servern körs och väljer **Stäng servern**.

Servern använder port 3000 som standard. Du kan välja en annan port med miljövariabeln `PORT`.

Från en telefon eller annan dator på samma LAN använder du QR-koden i webbgränssnittet eller datorns lokala nätverksadress.

Windows-brandväggen kan fråga om programmet ska få kommunicera på privata nätverk. Tillåt detta för att andra enheter ska kunna ansluta.

## Funktion

- uppladdning från enheter på samma lokala nätverk
- valfri beskrivning
- vald livslängd, högst 8 timmar
- automatisk radering när livslängden gått ut
- max 500 MB per fil
- max 2 GB total tillfällig lagring
- listning, hämtning och manuell radering
- QR-kod till aktuell LAN-adress
- gemensam programikon i webbsida, favicon och Windows-programfil
- automatisk öppning i standardwebbläsaren på Windows
- säker avstängning från localhost
- ingen inloggning och ingen databas

## Viktigt att känna till

Alla som når tjänsten på nätverket kan se, hämta och radera alla filer. Lägg därför inte upp känsligt material. Varje fil får en vald livslängd och raderas automatiskt när tiden går ut, senast efter 8 timmar.
