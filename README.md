# Lokal fildelning

Lokal fildelning är en enkel lokal filöverföringstjänst för hem och makerspace. Den körs på en dator i det lokala nätverket och låter enheter på samma LAN ladda upp, se, hämta och radera tillfälliga filer – utan konton eller databas.

## Teknik

Servern är skriven i Go. Frontenden ligger i `public/` i källkoden och bäddas in i den färdiga binären med Go `embed`, så den byggda `.exe`-filen kan köras utan en separat `public/`-mapp.

Uppladdade filer och metadata lagras lokalt i `uploads/` och versionshanteras inte.

## Bygg

Du behöver Go 1.27 eller senare.

```cmd
go mod tidy
go build -o lokal-fildelning.exe .
```

## Starta

För utveckling:

```cmd
go run .
```

Eller kör den byggda filen:

```cmd
lokal-fildelning.exe
```

Servern använder port 3000 som standard. Du kan välja en annan port med miljövariabeln `PORT`.

Terminalen visar både adressen för den egna datorn och en lokal nätverksadress. Öppna `http://localhost:3000` på serverdatorn. Från en telefon eller annan dator på samma LAN öppnar du den utskrivna nätverksadressen eller använder QR-koden i webbgränssnittet.

Windows-brandväggen kan fråga om programmet ska få kommunicera på privata nätverk. Tillåt detta för att andra enheter ska kunna ansluta.

## Funktion

- uppladdning från enheter på samma lokala nätverk
- valfri beskrivning
- vald livslängd, högst 8 timmar
- automatisk radering när livslängden gått ut
- listning, hämtning och manuell radering
- QR-kod till aktuell LAN-adress
- ingen inloggning och ingen databas

## Viktigt att känna till

Alla som når tjänsten på nätverket kan se, hämta och radera alla filer. Lägg därför inte upp känsligt material. Varje fil får en vald livslängd och raderas automatiskt när tiden går ut, senast efter 8 timmar.
