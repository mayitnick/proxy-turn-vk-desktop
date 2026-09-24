//go:build windows
// +build windows

package clientengine

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

type WindowsTunDevice struct {
	adapter   *wintun.Adapter
	session   wintun.Session
	name      string
	routes    []string
	gwIP      string
	gwIface   string
	clientIP  string

	rxPackets uint64
	rxBytes   uint64
	txPackets uint64
	txBytes   uint64

	mu       sync.Mutex
	closed   bool
	stopStat chan struct{}
}

func runCommand(tag, name string, args ...string) error {
	cmdStr := fmt.Sprintf("%s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	if err != nil {
		log.Printf("[%s:ERR] Команда: %s", tag, cmdStr)
		if errStr != "" {
			log.Printf("[%s:ERR] Ошибка: %s", tag, errStr)
		} else if outStr != "" {
			log.Printf("[%s:ERR] Вывод: %s", tag, outStr)
		}
		return fmt.Errorf("%s: %w", cmdStr, err)
	}

	log.Printf("[%s:OK] %s", tag, cmdStr)
	return nil
}

func getDefaultGateway() (string, string, error) {
	log.Printf("[WINTUN] Поиск физического шлюза по умолчанию...")
	psScript := `(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1 | ForEach-Object { "$($_.NextHop)|$($_.InterfaceAlias)" })`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("PowerShell error: %w", err)
	}

	res := strings.TrimSpace(string(out))
	parts := strings.Split(res, "|")
	if len(parts) >= 2 && parts[0] != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
	}
	return "", "", fmt.Errorf("unexpected gateway response: %q", res)
}

func OpenWinTUN(tunName, clientIP, dnsCSV string, mtu int, peerAddr string, turnAddrs []string) (*WindowsTunDevice, error) {
	log.Println("─────────────────────────────────────────────────────────────────")
	log.Printf("[WINTUN] Инициализация WinTUN %q (IP: %s, MTU: %d)...", tunName, clientIP, mtu)

	gw, iface, err := getDefaultGateway()
	if err != nil {
		log.Printf("[ROUTE:WARN] Не удалось определить шлюз: %v", err)
	} else {
		log.Printf("[ROUTE] Обнаружен физический шлюз: %s на интерфейсе %q", gw, iface)
	}

	guid, _ := windows.GenerateGUID()
	adapter, err := wintun.CreateAdapter(tunName, "VKTurnTunnel", &guid)
	if err != nil {
		log.Printf("[WINTUN] CreateAdapter: %v, пробуем OpenAdapter...", err)
		adapter, err = wintun.OpenAdapter(tunName)
		if err != nil {
			return nil, fmt.Errorf("OpenAdapter %q: %w", tunName, err)
		}
	}
	log.Printf("[WINTUN] Адаптер %q готов", tunName)

	session, err := adapter.StartSession(0x800000)
	if err != nil {
		adapter.Close()
		return nil, fmt.Errorf("StartSession: %w", err)
	}
	log.Printf("[WINTUN] Сессия WinTUN запущена (буфер 8 МБ)")

	dev := &WindowsTunDevice{
		adapter:  adapter,
		session:  session,
		name:     tunName,
		gwIP:     gw,
		gwIface:  iface,
		clientIP: clientIP,
		stopStat: make(chan struct{}),
	}

	// Настройка IP
	log.Printf("[WINTUN] Установка IP %s...", clientIP)
	_ = runCommand("NETSH", "netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%q", tunName), "static", clientIP, "255.255.255.0", "none")

	// MTU
	log.Printf("[WINTUN] Установка MTU = %d...", mtu)
	_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "subinterface",
		fmt.Sprintf("%q", tunName), fmt.Sprintf("mtu=%d", mtu), "store=active")

	// DNS
	if dnsCSV != "" {
		parts := strings.Split(dnsCSV, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			primary := strings.TrimSpace(parts[0])
			log.Printf("[WINTUN] DNS первичный: %s", primary)
			_ = runCommand("NETSH", "netsh", "interface", "ip", "set", "dns",
				fmt.Sprintf("name=%q", tunName), "static", primary, "validate=no")
			for i := 1; i < len(parts); i++ {
				sec := strings.TrimSpace(parts[i])
				if sec != "" {
					_ = runCommand("NETSH", "netsh", "interface", "ip", "add", "dns",
						fmt.Sprintf("name=%q", tunName), sec, fmt.Sprintf("index=%d", i+1), "validate=no")
				}
			}
		}
	}

	// Исключения маршрутизации
	if gw != "" {
		log.Println("[ROUTE] Добавление хостовых исключений /32 (VPS и VK TURN)...")
		var targetHosts []string

		if h, _, err := net.SplitHostPort(peerAddr); err == nil {
			targetHosts = append(targetHosts, h)
		} else if peerAddr != "" {
			targetHosts = append(targetHosts, peerAddr)
		}

		for _, rawURL := range turnAddrs {
			if h, _, err := net.SplitHostPort(rawURL); err == nil {
				targetHosts = append(targetHosts, h)
			} else if rawURL != "" {
				targetHosts = append(targetHosts, rawURL)
			}
		}

		// Добавляем DNS Яндекса для прямого зондирования сети
		targetHosts = append(targetHosts, "77.88.8.8", "77.88.8.1")

		seen := make(map[string]bool)
		for _, host := range targetHosts {
			host = strings.TrimSpace(host)
			if host == "" || seen[host] {
				continue
			}
			seen[host] = true

			ips, err := net.LookupIP(host)
			if err == nil && len(ips) > 0 {
				for _, ip := range ips {
					if ipv4 := ip.To4(); ipv4 != nil {
						ipStr := ipv4.String()
						log.Printf("[ROUTE] Исключение: %s/32 -> %s", ipStr, gw)
						_ = runCommand("ROUTE", "route", "add", ipStr, "mask", "255.255.255.255", gw, "metric", "1")
						dev.routes = append(dev.routes, ipStr)
					}
				}
			} else {
				log.Printf("[ROUTE] Исключение: %s/32 -> %s", host, gw)
				_ = runCommand("ROUTE", "route", "add", host, "mask", "255.255.255.255", gw, "metric", "1")
				dev.routes = append(dev.routes, host)
			}
		}
		RegisterActiveRouting(gw, dev.gwIface, dev.routes)

		log.Println("[ROUTE] Направление интернета в WinTUN (0.0.0.0/1 и 128.0.0.0/1)...")
		_ = runCommand("ROUTE", "route", "delete", "0.0.0.0", "mask", "128.0.0.0")
		_ = runCommand("ROUTE", "route", "delete", "128.0.0.0", "mask", "128.0.0.0")
		_ = runCommand("ROUTE", "route", "add", "0.0.0.0", "mask", "128.0.0.0", clientIP, "metric", "1")
		_ = runCommand("ROUTE", "route", "add", "128.0.0.0", "mask", "128.0.0.0", clientIP, "metric", "1")

		// Абсолютный приоритет WinTUN
		_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", tunName), "metric=1")

		// Монополизация: выставляем метрику 500 на физический интерфейс
		if dev.gwIface != "" {
			_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", dev.gwIface), "metric=500")
			_ = runCommand("NETSH", "netsh", "interface", "ipv6", "set", "interface", fmt.Sprintf("%q", dev.gwIface), "metric=500")
		}
		_ = runCommand("IPCONFIG", "ipconfig", "/flushdns")
	}

	go dev.statsLoop()
	log.Println("[WINTUN] Готово! Весь трафик теперь защищённо идёт через туннель.")
	log.Println("─────────────────────────────────────────────────────────────────")
	return dev, nil
}

func (d *WindowsTunDevice) statsLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastTx, lastRx uint64
	for {
		select {
		case <-ticker.C:
			curTx := atomic.LoadUint64(&d.txBytes)
			curTxP := atomic.LoadUint64(&d.txPackets)
			curRx := atomic.LoadUint64(&d.rxBytes)
			curRxP := atomic.LoadUint64(&d.rxPackets)

			diffTx := curTx - lastTx
			diffRx := curRx - lastRx
			lastTx = curTx
			lastRx = curRx

			speedTx := float64(diffTx) / 1024.0 / 5.0
			speedRx := float64(diffRx) / 1024.0 / 5.0

			if curTxP > 0 || curRxP > 0 {
				log.Printf("[TRAFFIC] ↑ Uplink: %.1f KB/s (всего: %.2f MB) | ↓ Downlink: %.1f KB/s (всего: %.2f MB)",
					speedTx, float64(curTx)/(1024*1024),
					speedRx, float64(curRx)/(1024*1024))
			}
		case <-d.stopStat:
			return
		}
	}
}

func (d *WindowsTunDevice) Read(b []byte) (int, error) {
	pkt, err := d.session.ReceivePacket()
	if err != nil {
		return 0, err
	}
	defer d.session.ReleaseReceivePacket(pkt)

	n := copy(b, pkt)
	c := atomic.AddUint64(&d.txPackets, 1)
	atomic.AddUint64(&d.txBytes, uint64(n))
	if c == 1 {
		log.Printf("[WINTUN:FLOW] Первый исходящий пакет перехвачен (%d байт)!", n)
	}
	return n, nil
}

func (d *WindowsTunDevice) Write(b []byte) (int, error) {
	pkt, err := d.session.AllocateSendPacket(len(b))
	if err != nil {
		return 0, err
	}
	copy(pkt, b)
	d.session.SendPacket(pkt)

	c := atomic.AddUint64(&d.rxPackets, 1)
	atomic.AddUint64(&d.rxBytes, uint64(len(b)))
	if c == 1 {
		log.Printf("[WINTUN:FLOW] Первый входящий пакет передан системе (%d байт)!", len(b))
	}
	return len(b), nil
}

func (d *WindowsTunDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	close(d.stopStat)

	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("[WINTUN] Очистка маршрутов и закрытие адаптера...")
	_ = runCommand("ROUTE", "route", "delete", "0.0.0.0", "mask", "128.0.0.0")
	_ = runCommand("ROUTE", "route", "delete", "128.0.0.0", "mask", "128.0.0.0")
	for _, host := range d.routes {
		_ = runCommand("ROUTE", "route", "delete", host)
	}
	if d.gwIface != "" {
		_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", d.gwIface), "metric=25")
		_ = runCommand("NETSH", "netsh", "interface", "ipv6", "set", "interface", fmt.Sprintf("%q", d.gwIface), "metric=25")
	}
	_ = runCommand("IPCONFIG", "ipconfig", "/flushdns")

	d.session.End()
	d.adapter.Close()
	log.Println("[WINTUN] Маршруты удалены, адаптер закрыт.")
	log.Println("─────────────────────────────────────────────────────────────────")
	return nil
}
