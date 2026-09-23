//go:build !windows
// +build !windows

package clientengine

import "context"

func startWindowsWireGuardTUN(ctx context.Context, conf, peerAddr string) (func(), error) {
	return nil, nil
}
