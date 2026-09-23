//go:build windows
// +build windows

package clientengine

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// startWindowsWireGuardTUN поднимает нативный WinTUN интерфейс WireGuard на Windows
func startWindowsWireGuardTUN(ctx context.Context, conf, peerAddr string) (func(), error) {
	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("[WINTUN-WG] Запуск встроенного WireGuard WinTUN туннеля...")

	// 1. Парсим параметры из WireGuard конфига
	var clientIP string
	var clientPrivKey string
	var peerPubKey string
	var peerEndpoint string
	var peerAllowedIPs string
	var mtuVal int = 1280
	var dnsCSV string

	lines := strings.Split(conf, "\n")
	isPeerSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[Peer]" {
			isPeerSection = true
			continue
		}
		if line == "[Interface]" {
			isPeerSection = false
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])

		if !isPeerSection {
			switch k {
			case "Address":
				clientIP = strings.Split(v, "/")[0]
			case "PrivateKey":
				clientPrivKey = v
			case "MTU":
				if m, err := strconv.Atoi(v); err == nil {
					mtuVal = m
				}
			case "DNS":
				dnsCSV = v
			}
		} else {
			switch k {
			case "PublicKey":
				peerPubKey = v
			case "Endpoint":
				peerEndpoint = v
			case "AllowedIPs":
				peerAllowedIPs = v
			}
		}
	}

	if clientIP == "" || clientPrivKey == "" || peerPubKey == "" || peerEndpoint == "" {
		return nil, fmt.Errorf("неполный WireGuard конфиг (IP=%s, Endpoint=%s)", clientIP, peerEndpoint)
	}

	log.Printf("[WINTUN-WG] Конфигурация: ClientIP=%s, Endpoint=%s, MTU=%d", clientIP, peerEndpoint, mtuVal)

	// 2. Создаем WinTUN адаптер через golang.zx2c4.com/wireguard/tun
	tunName := "VKTurnWG"
	guid, _ := windows.GenerateGUID()
	tunDev, err := tun.CreateTUNWithRequestedGUID(tunName, &guid, mtuVal)
	if err != nil {
		log.Printf("[WINTUN-WG] CreateTUN ошибка: %v, пробуем открыть...", err)
		tunDev, err = tun.CreateTUN(tunName, mtuVal)
		if err != nil {
			return nil, fmt.Errorf("создание WinTUN адаптера: %w", err)
		}
	}

	// 3. Создаем WireGuard устройство
	logger := device.NewLogger(device.LogLevelSilent, "[WG] ")
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// 4. Генерируем UAPI конфигурацию для устройства
	ipcConfig := fmt.Sprintf("private_key=%s\npublic_key=%s\nendpoint=%s\nallowed_ip=%s\npersistent_keepalive_interval=25\n",
		decodeBase64ToHex(clientPrivKey),
		decodeBase64ToHex(peerPubKey),
		peerEndpoint,
		peerAllowedIPs,
	)

	if err := dev.IpcSet(ipcConfig); err != nil {
		dev.Close()
		return nil, fmt.Errorf("IpcSet: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("Device Up: %w", err)
	}
	log.Printf("[WINTUN-WG] Устройство WireGuard запущено и поднято!")

	// 5. Назначаем IP-адрес интерфейсу через netsh
	log.Printf("[WINTUN-WG] Назначение IP-адреса %s/24...", clientIP)
	_ = runCommand("NETSH", "netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%q", tunName), "static", clientIP, "255.255.255.0", "none")

	// 6. Назначаем MTU
	_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "subinterface",
		fmt.Sprintf("%q", tunName), fmt.Sprintf("mtu=%d", mtuVal), "store=active")

	// 7. Назначаем DNS
	if dnsCSV != "" {
		dnsParts := strings.Split(dnsCSV, ",")
		if len(dnsParts) > 0 && strings.TrimSpace(dnsParts[0]) != "" {
			_ = runCommand("NETSH", "netsh", "interface", "ip", "set", "dns",
				fmt.Sprintf("name=%q", tunName), "static", strings.TrimSpace(dnsParts[0]), "validate=no")
			for i := 1; i < len(dnsParts); i++ {
				sec := strings.TrimSpace(dnsParts[i])
				if sec != "" {
					_ = runCommand("NETSH", "netsh", "interface", "ip", "add", "dns",
						fmt.Sprintf("name=%q", tunName), sec, fmt.Sprintf("index=%d", i+1), "validate=no")
				}
			}
		}
	}

	// 8. Маршрутизация исключений: физический шлюз
	gw, iface, gwErr := getDefaultGateway()
	var routesAdded []string

	if gwErr == nil && gw != "" {
		log.Printf("[WINTUN-WG] Физический шлюз: %s (интерфейс: %s)", gw, iface)
		log.Println("[WINTUN-WG] Настройка исключений /32 (VPS и VK TURN)...")

		var targetHosts []string
		if h, _, err := net.SplitHostPort(peerAddr); err == nil {
			targetHosts = append(targetHosts, h)
		} else if peerAddr != "" {
			targetHosts = append(targetHosts, peerAddr)
		}

		// Добавляем все динамически обнаруженные TURN релеи VK
		turnIPs := GetDiscoveredTurnIPs()
		log.Printf("[WINTUN-WG] Найдено релеев VK TURN для исключений: %d %v", len(turnIPs), turnIPs)
		targetHosts = append(targetHosts, turnIPs...)

		// Добавляем домены инфраструктуры и API ВКонтакте для исключений
		vkDomains := []string{
			"api.vk.me",
			"api.vk.com",
			"api.vk.ru",
			"id.vk.ru",
			"vk.com",
			"oauth.vk.com",
			"calls.okcdn.ru",
			"ok.ru",
		}
		log.Printf("[WINTUN-WG] Добавление доменов VK в исключения: %v", vkDomains)
		targetHosts = append(targetHosts, vkDomains...)

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
						routesAdded = append(routesAdded, ipStr)
					}
				}
			} else {
				log.Printf("[ROUTE] Исключение: %s/32 -> %s", host, gw)
				_ = runCommand("ROUTE", "route", "add", host, "mask", "255.255.255.255", gw, "metric", "1")
				routesAdded = append(routesAdded, host)
			}
		}

		// Получаем точный индекс WinTUN интерфейса (IF <ifIndex>) для надёжной маршрутизации
		tunIfIndex := 0
		if tunIf, err := net.InterfaceByName(tunName); err == nil {
			tunIfIndex = tunIf.Index
			log.Printf("[WINTUN-WG] Интерфейс %s найден, индекс: %d", tunName, tunIfIndex)
		}

		// Заворачиваем интернет в WinTUN интерфейс WireGuard
		log.Println("[ROUTE] Направление интернета в WinTUN (0.0.0.0/1 и 128.0.0.0/1)...")
		// Сначала очищаем возможные старые маршруты от предыдущих сессий
		_ = runCommand("ROUTE", "route", "delete", "0.0.0.0", "mask", "128.0.0.0")
		_ = runCommand("ROUTE", "route", "delete", "128.0.0.0", "mask", "128.0.0.0")

		if tunIfIndex > 0 {
			_ = runCommand("ROUTE", "route", "add", "0.0.0.0", "mask", "128.0.0.0", clientIP, "IF", strconv.Itoa(tunIfIndex), "metric", "1")
			_ = runCommand("ROUTE", "route", "add", "128.0.0.0", "mask", "128.0.0.0", clientIP, "IF", strconv.Itoa(tunIfIndex), "metric", "1")
		} else {
			_ = runCommand("ROUTE", "route", "add", "0.0.0.0", "mask", "128.0.0.0", clientIP, "metric", "1")
			_ = runCommand("ROUTE", "route", "add", "128.0.0.0", "mask", "128.0.0.0", clientIP, "metric", "1")
		}

		// Выставляем абсолютный приоритет (metric=1) на WinTUN интерфейс
		_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", tunName), "metric=1")

		// Монополизация трафика: временно повышаем метрику физического адаптера (IPv4 и IPv6) до 500,
		// чтобы Windows не маршрутизировала трафик в обход WinTUN через физический шлюз
		if iface != "" {
			log.Printf("[ROUTE] Монополизация трафика: выставление метрики 500 на физический интерфейс %q...", iface)
			_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", iface), "metric=500")
			_ = runCommand("NETSH", "netsh", "interface", "ipv6", "set", "interface", fmt.Sprintf("%q", iface), "metric=500")
		}

		// Сбрасываем кэш DNS, чтобы система сразу резолвила через WinTUN DNS
		_ = runCommand("IPCONFIG", "ipconfig", "/flushdns")
	}

	log.Println("[WINTUN-WG] Туннель WinTUN полностью активен! Трафик защищён.")
	log.Println("─────────────────────────────────────────────────────────────────")

	cleanup := func() {
		log.Println("[WINTUN-WG] Завершение работы: очистка маршрутов...")
		_ = runCommand("ROUTE", "route", "delete", "0.0.0.0", "mask", "128.0.0.0")
		_ = runCommand("ROUTE", "route", "delete", "128.0.0.0", "mask", "128.0.0.0")
		for _, r := range routesAdded {
			_ = runCommand("ROUTE", "route", "delete", r)
		}
		if iface != "" {
			_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", iface), "metric=25")
			_ = runCommand("NETSH", "netsh", "interface", "ipv6", "set", "interface", fmt.Sprintf("%q", iface), "metric=25")
		}
		_ = runCommand("IPCONFIG", "ipconfig", "/flushdns")
		dev.Close()
		log.Println("[WINTUN-WG] Интерфейс WireGuard WinTUN закрыт.")
	}

	return cleanup, nil
}

// decodeBase64ToHex преобразует base64 ключ WireGuard в hex для IPC
func decodeBase64ToHex(b64 string) string {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return b64
	}
	return hex.EncodeToString(b)
}
