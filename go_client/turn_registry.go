package clientengine

import (
	"net"
	"sync"
)

var (
	discoveredTurnMu  sync.Mutex
	discoveredTurnIPs = make(map[string]struct{})
)

// RegisterDiscoveredTurnAddr регистрирует IP-адрес или хост релея TURN
func RegisterDiscoveredTurnAddr(addr string) {
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	discoveredTurnMu.Lock()
	discoveredTurnIPs[host] = struct{}{}
	discoveredTurnMu.Unlock()
}

// GetDiscoveredTurnIPs возвращает список всех обнаруженных IP TURN
func GetDiscoveredTurnIPs() []string {
	discoveredTurnMu.Lock()
	defer discoveredTurnMu.Unlock()
	list := make([]string, 0, len(discoveredTurnIPs))
	for ip := range discoveredTurnIPs {
		list = append(list, ip)
	}
	return list
}
