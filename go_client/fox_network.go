package clientengine

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// WakeupFoxChan позволяет мгновенно разбудить лисёнка (по нажатию Enter в CLI или кнопке в GUI)
	WakeupFoxChan = make(chan struct{}, 1)
	
	// Хранилище активных маршрутов-исключений для динамического обновления при смене сети
	activeRoutingMu    sync.Mutex
	activeRoutesList   []string
	activeCurrentGW    string
	activeCurrentIface string
)

// WakeupFox отправляет сигнал немедленного пробуждения спящего лисёнка
func WakeupFox() {
	select {
	case WakeupFoxChan <- struct{}{}:
	default:
	}
}

// RegisterActiveRouting регистрирует текущие маршруты-исключения для динамической миграции
func RegisterActiveRouting(gw, iface string, routes []string) {
	activeRoutingMu.Lock()
	defer activeRoutingMu.Unlock()
	activeCurrentGW = gw
	activeCurrentIface = iface
	activeRoutesList = append([]string(nil), routes...)
}

// ProbePhysicalInternet проверяет доступность интернета через прямой маршрут к DNS Яндекса (77.88.8.8:53)
func ProbePhysicalInternet() bool {
	// 1. Быстрый TCP probe на 77.88.8.8:53 (разрешён и работает всегда при наличии физической сети)
	d := net.Dialer{Timeout: 1200 * time.Millisecond}
	conn, err := d.Dial("tcp", "77.88.8.8:53")
	if err == nil {
		_ = conn.Close()
		return true
	}

	// 2. Fallback на резервный IP Яндекса 77.88.8.1:53
	conn2, err2 := d.Dial("tcp", "77.88.8.1:53")
	if err2 == nil {
		_ = conn2.Close()
		return true
	}

	// 3. UDP DNS запрос (запасной вариант, если TCP 53 заблокирован провайдером)
	udpAddr, err := net.ResolveUDPAddr("udp", "77.88.8.8:53")
	if err != nil {
		return false
	}
	uConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return false
	}
	defer uConn.Close()

	_ = uConn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	// Минимальный DNS query для '.' (root)
	dummyDNSQuery := []byte{
		0xaa, 0xbb, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01,
	}
	if _, err := uConn.Write(dummyDNSQuery); err != nil {
		return false
	}
	buf := make([]byte, 512)
	n, err := uConn.Read(buf)
	return err == nil && n > 0
}

// ProbeInTunnel проверяет реальное прохождение данных внутри туннеля (L7 Healthcheck)
func ProbeInTunnel() bool {
	// Проверяем доступность публичных узлов (1.1.1.1:53 или Cloudflare / Google)
	d := net.Dialer{Timeout: 2000 * time.Millisecond}
	conn, err := d.Dial("tcp", "1.1.1.1:53")
	if err == nil {
		_ = conn.Close()
		return true
	}
	conn2, err2 := d.Dial("tcp", "8.8.8.8:53")
	if err2 == nil {
		_ = conn2.Close()
		return true
	}
	return false
}

// RunFoxNetworkWatcher запускает цикл наблюдения за физической сетью и управления режимом сна
func RunFoxNetworkWatcher(ctx context.Context, pauseFlag *int32, stats *Stats, peerAddr string) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var wasSleeping bool
	lastGW := ""
	lastIface := ""

	// Первичная инициализация состояния
	if gw, iface, err := getDefaultGateway(); err == nil {
		lastGW = gw
		lastIface = iface
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-WakeupFoxChan:
			log.Println("[FWDTT:FOX] 🦊 Ручное пробуждение лисёнка! Немедленная проверка сети...")
		case <-ticker.C:
		}

		isOnline := ProbePhysicalInternet()
		stats.PhysicalOnline.Store(isOnline)

		currentGW, currentIface, _ := getDefaultGateway()

		// ─── СИТУАЦИЯ 1: Сеть пропала (спящий режим / оффлайн) ───
		if !isOnline {
			if !wasSleeping {
				wasSleeping = true
				stats.FoxSleeping.Store(true)
				atomic.StoreInt32(pauseFlag, 1)

				msg := "Интернета пока не вижу, посплю, пока ты его не включишь... 🦊💤"
				stats.FoxStatusMsg.Store(&msg)

				log.Println()
				log.Println("    /\\_/\\    *свернулся пушистым клубочком на ковре*")
				log.Println("   ( -.- )   Интернета пока не вижу, посплю, пока ты его не включишь... 🦊💤")
				log.Println("    > ^ <    (Нажмите Enter в консоли или кнопку в GUI, чтобы разбудить меня)")
				log.Println()
			}
			continue
		}

		// ─── СИТУАЦИЯ 2: Сеть появилась или сменилась ───
		if isOnline && wasSleeping {
			wasSleeping = false
			stats.FoxSleeping.Store(false)
			atomic.StoreInt32(pauseFlag, 0)

			msg := "Проснулся, потянулся! Интернет вернулся, восстанавливаю связь... 🦊✨"
			stats.FoxStatusMsg.Store(&msg)

			log.Println()
			log.Println("    /\\_/\\    *радостно виляет хвостом и потягивается*")
			log.Println("   ( o.o )   Проснулся, потянулся! Интернет вернулся, восстанавливаю связь... 🦊✨")
			log.Println("    > ^ <    Проверяю маршруты и запускаю воркеры!")
			log.Println()

			// Мгновенная адаптация шлюза при выходе из сна
			if currentGW != "" && (currentGW != lastGW || currentIface != lastIface) {
				handleGatewayMigration(lastGW, currentGW, currentIface)
				lastGW = currentGW
				lastIface = currentIface
			}
			continue
		}

		// ─── СИТУАЦИЯ 3: Смена Wi-Fi сети на лету (онлайн сохраняется, но шлюз изменился) ───
		if currentGW != "" && lastGW != "" && (currentGW != lastGW || currentIface != lastIface) {
			log.Printf("[FWDTT:ROAMING] Обнаружена смена сети (шлюз: %s -> %s, интерфейс: %s -> %s)!", lastGW, currentGW, lastIface, currentIface)
			handleGatewayMigration(lastGW, currentGW, currentIface)
			lastGW = currentGW
			lastIface = currentIface
		} else if lastGW == "" && currentGW != "" {
			lastGW = currentGW
			lastIface = currentIface
		}
	}
}

// handleGatewayMigration бесшовно переносит точечные маршруты-исключения на новый физический шлюз
func handleGatewayMigration(oldGW, newGW, newIface string) {
	activeRoutingMu.Lock()
	defer activeRoutingMu.Unlock()

	log.Printf("[ROUTE:MIGRATE] Бесшовный перенос маршрутов на новый шлюз %s (адаптер %s)...", newGW, newIface)

	// 1. Удаляем старые маршруты через старый шлюз
	if oldGW != "" {
		for _, ip := range activeRoutesList {
			_ = runCommand("ROUTE", "route", "delete", ip)
		}
	}

	// 2. Добавляем маршруты через новый шлюз
	for _, ip := range activeRoutesList {
		_ = runCommand("ROUTE", "route", "add", ip, "mask", "255.255.255.255", newGW, "metric", "1")
	}

	// 3. Устанавливаем приоритет 500 на новый физический интерфейс, чтобы WinTUN оставался монопольным
	if newIface != "" {
		_ = runCommand("NETSH", "netsh", "interface", "ipv4", "set", "interface", fmt.Sprintf("%q", newIface), "metric=500")
		_ = runCommand("NETSH", "netsh", "interface", "ipv6", "set", "interface", fmt.Sprintf("%q", newIface), "metric=500")
	}

	_ = runCommand("IPCONFIG", "ipconfig", "/flushdns")
	activeCurrentGW = newGW
	activeCurrentIface = newIface
	log.Println("[ROUTE:MIGRATE] Маршруты успешно перенесены! Туннель продолжает работу без разрыва.")
}
