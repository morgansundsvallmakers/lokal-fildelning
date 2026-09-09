//go:build windows

package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

func init() {
	go func() {
		port := <-browserPortReady
		openBrowserWhenReady(port)
	}()
}

func openBrowserWhenReady(port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
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

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
