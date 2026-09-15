//go:build !windows
// +build !windows

package main

import "context"

func startWindowsWireGuardTUN(ctx context.Context, conf, peerAddr string) (func(), error) {
	return nil, nil
}
