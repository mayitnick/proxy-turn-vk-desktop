//go:build !windows
// +build !windows

package clientengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

func setupPlatformTUN(ctx context.Context, disp *Dispatcher, tunFdSock, ip, dnsCSV, mtuStr, peerAddr string, turnURLs []string) (func(), error) {
	if tunFdSock == "" {
		return nil, fmt.Errorf("-tun-fd-sock не указан")
	}
	log.Println("[RAW] Ожидание TUN fd от Android...")
	var tunFile *os.File
	var fdErr error
	attempt := 0
	for {
		attempt++
		tunFile, fdErr = recvTunFD(tunFdSock)
		if fdErr == nil {
			break
		}
		rawDiagf("recvTunFD попытка #%d неудачна: %v (повтор через 200мс)", attempt, fdErr)
		select {
		case <-ctx.Done():
			rawDiagf("recvTunFD: ctx отменён, прекращаю попытки")
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	rawDiagf("recvTunFD успешен на попытке #%d, fd=%v", attempt, tunFile.Fd())
	disp.AttachTUN(tunFile)
	log.Println("[RAW] TUN подключён, трафик пошёл")
	cleanup := func() {
		_ = tunFile.Close()
	}
	return cleanup, nil
}
