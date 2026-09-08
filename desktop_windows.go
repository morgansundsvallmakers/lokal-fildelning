//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/gogpu/systray"
)

//go:embed assets/tray-icon.png
var trayIconPNG []byte

func init() {
	go startWindowsTray()
	go openBrowserWhenReady()
}

func startWindowsTray() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tray := systray.New()
	menu := systray.NewMenu()
	menu.Add("Öppna Lokal fildelning", func() {
		_ = openBrowser(localURL())
	})
	menu.AddSeparator()
	menu.Add("Avsluta", func() {
		tray.Remove()
		os.Exit(0)
	})

	tray.SetIcon(trayIconPNG).
		SetTooltip("Lokal fildelning").
		SetMenu(menu)
	tray.OnClick(func() {
		_ = openBrowser(localURL())
	})
	tray.Show()

	_ = tray.Run()
}

func openBrowserWhenReady() {
	url := localURL()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 500 {
				_ = openBrowser(url)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func localURL() string {
	port := defaultPort
	if value := os.Getenv("PORT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 65535 {
			port = parsed
		}
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
