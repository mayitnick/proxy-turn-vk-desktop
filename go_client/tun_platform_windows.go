//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

func setupPlatformTUN(ctx context.Context, disp *Dispatcher, tunFdSock, ip, dnsCSV, mtuStr, peerAddr string, turnURLs []string) (func(), error) {
	mtuVal, err := strconv.Atoi(strings.TrimSpace(mtuStr))
	if err != nil || mtuVal <= 0 {
		mtuVal = 1360
	}
	winTun, err := OpenWinTUN("VKTurn", ip, dnsCSV, mtuVal, peerAddr, turnURLs)
	if err != nil {
		return nil, fmt.Errorf("OpenWinTUN: %w", err)
	}
	disp.AttachTUN(winTun)
	log.Println("[RAW] WinTUN подключён к диспетчеру, обмен пакетами активен")
	cleanup := func() {
		_ = winTun.Close()
	}
	return cleanup, nil
}
