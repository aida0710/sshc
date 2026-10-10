// The demo bridge runs inside the disposable browser VM. It never opens a host port.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sys/unix"
)

const (
	engineOrigin = "http://127.0.0.1:60001"
	// Base64 encoding adds overhead to the 2 MiB payload limit.
	maxBridgeMessageBytes = 4 << 20
	maxBridgeBodyBytes    = 2 << 20
	// Initialization can take longer under CPU emulation than on a native machine.
	engineStartupTimeout = 90 * time.Second
)

type browserBridge struct {
	serial      *os.File
	client      *http.Client
	outputMutex sync.Mutex
	socketMutex sync.Mutex
	sockets     map[string]*websocket.Conn
}

func main() {
	if err := runBridge(); err != nil {
		// The URL from sshc open contains a bootstrap token; never include it in logs.
		fmt.Fprintln(os.Stderr, "Demo bridge initialization failed.")
		os.Exit(1)
	}
}

func runBridge() error {
	serial, err := os.OpenFile("/dev/ttyS1", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer serial.Close()
	settings, err := unix.IoctlGetTermios(int(serial.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	settings.Iflag, settings.Oflag, settings.Lflag = 0, 0, 0
	settings.Cflag = unix.B115200 | unix.CS8 | unix.CREAD | unix.CLOCAL
	settings.Cc[unix.VMIN], settings.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(int(serial.Fd()), unix.TCSETS, settings); err != nil {
		return err
	}
	jar, _ := cookiejar.New(nil)
	bridge := &browserBridge{serial: serial, client: &http.Client{Jar: jar, Timeout: engineStartupTimeout}, sockets: make(map[string]*websocket.Conn)}
	startup, cancel := context.WithTimeout(context.Background(), engineStartupTimeout)
	defer cancel()
	if err := awaitEngine(startup, bridge.client); err != nil {
		return err
	}
	if err := initialiseDemoVault(startup, bridge.client); err != nil {
		return err
	}
	bootstrap, err := mintBootstrap(startup)
	if err != nil {
		return err
	}
	bridge.reply(browserReply{Kind: "ready", Bootstrap: bootstrap})
	scanner := bufio.NewScanner(serial)
	scanner.Buffer(make([]byte, 4096), maxBridgeMessageBytes)
	for scanner.Scan() {
		var request browserRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		switch request.Kind {
		case "fetch":
			go bridge.fetch(request)
		case "open":
			go bridge.openSocket(request)
		case "send", "close":
			bridge.routeSocketInput(request)
		case "cli-resize":
			resizeConsole(request.Cols, request.Rows)
		}
	}
	return scanner.Err()
}

func awaitEngine(ctx context.Context, client *http.Client) error {
	// Poll a real readiness signal rather than hiding startup behind a fixed delay.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, engineOrigin+"/api/v1/health", nil)
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			// Health needs a session cookie even before Vault initialization. A 401
			// confirms the engine is listening and has installed its authentication gate.
			if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
