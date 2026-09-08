//go:build windows

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func init() {
	go openBrowserWhenReady()
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
