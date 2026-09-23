package clientengine

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

// EngineConfig содержит параметры запуска туннеля через Wails GUI или внешнее API
type EngineConfig struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	Listen      string `json:"listen"`
	VkHash      string `json:"vk_hash"`
	PeerAddr    string `json:"peer_addr"`
	NumWorkers  int    `json:"num_workers"`
	DeviceID    string `json:"device_id"`
	Password    string `json:"password"`
	CaptchaMode string `json:"captcha_mode"`
	VkAuthMode  string `json:"vk_auth_mode"`
	VkAnonPath  string `json:"vk_anon_path"`
	VkCredsFile string `json:"vk_creds_file"`
	GoDNS       string `json:"go_dns"`
	ObfsMode    string `json:"obfs_mode"`
	ConnMode    string `json:"conn_mode"` // "vpn", "socks", "rawtun"
	SocksAddr   string `json:"socks_addr"`
	SocksAuth   bool   `json:"socks_auth"`
	SocksUser   string `json:"socks_user"`
	SocksPass   string `json:"socks_pass"`
	NoDTLS      bool   `json:"no_dtls"`
	TurnTCP     bool   `json:"turn_tcp"`
	TunFdSock   string `json:"tun_fd_sock"`
}

// RunTunnelEngine запускает сетевой туннель в контексте ctx.
// Функция блокируется до завершения сессии (отмены контекста или фатальной ошибки).
func RunTunnelEngine(ctx context.Context, cfg EngineConfig, stats *Stats, logWriter io.Writer) error {
	if logWriter != nil {
		log.SetOutput(logWriter)
	}

	activeConnMode := strings.ToLower(strings.TrimSpace(cfg.ConnMode))
	if activeConnMode != "socks" && activeConnMode != "rawtun" {
		activeConnMode = "vpn"
	}

	if activeConnMode == "socks" && cfg.SocksAuth {
		if cfg.SocksUser == "" || cfg.SocksPass == "" {
			return fmt.Errorf("для авторизации SOCKS5 нужны логин и пароль")
		}
	}

	dns := cfg.GoDNS
	if dns == "" {
		dns = "yandex"
	}
	setupGlobalResolver(dns)

	cMode := cfg.CaptchaMode
	if cMode == "" {
		cMode = "auto"
	}
	activeCaptchaMode := setCaptchaMode(cMode)

	vAuth := cfg.VkAuthMode
	if vAuth == "" {
		vAuth = "anonymous"
	}
	activeVkAuthMode := setVkAuthMode(vAuth)

	vAnon := cfg.VkAnonPath
	if vAnon == "" {
		vAnon = "vkcalls"
	}
	activeVkAnonPath := setVkAnonPath(vAnon)

	if cfg.VkCredsFile != "" {
		if err := loadVkCredsFile(cfg.VkCredsFile); err != nil {
			log.Printf("[КЛИЕНТ] Ошибка чтения vk-creds-file: %v", err)
		}
	}

	hashes := ParseHashes(cfg.VkHash)
	if len(hashes) == 0 {
		return fmt.Errorf("не указан VK_HASH или не найдено валидных хешей")
	}

	if cfg.PeerAddr == "" {
		return fmt.Errorf("не указан адрес сервера (PEER)")
	}

	peer, err := net.ResolveUDPAddr("udp", cfg.PeerAddr)
	if err != nil {
		return fmt.Errorf("ошибка разбора адреса сервера %s: %w", cfg.PeerAddr, err)
	}

	if cfg.Password == "" {
		return fmt.Errorf("пароль подключения не может быть пустым")
	}

	wrapKey, err := deriveWrapKey(cfg.Password)
	if err != nil {
		return fmt.Errorf("ошибка генерации ключа шифрования: %w", err)
	}

	numW := cfg.NumWorkers
	if numW <= 0 {
		numW = 18
	}
	const maxWorkers = 108
	if numW > maxWorkers {
		numW = maxWorkers
	}

	if getVkAuthMode() == "account" {
		const accountMaxWorkers = 4
		if numW > accountMaxWorkers {
			numW = accountMaxWorkers
		}
	} else {
		if numW < workersPerGroup {
			numW = workersPerGroup
		}
		numW = (numW / workersPerGroup) * workersPerGroup
	}

	obfs := cfg.ObfsMode
	if obfs == "" {
		obfs = "audio"
	}

	tp := &TurnParams{
		Host:         cfg.Host,
		Port:         cfg.Port,
		Hashes:       hashes,
		WrapKey:      wrapKey,
		ObfsMode:     normalizeObfsMode(obfs),
		NoDTLS:       cfg.NoDTLS,
		RawMode:      activeConnMode == "rawtun",
		TCPTransport: cfg.TurnTCP,
	}

	listenAddr := cfg.Listen
	if listenAddr == "" {
		listenAddr = "127.0.0.1:9000"
	}

	localConn, err := listenUDP(listenAddr)
	if err != nil {
		return fmt.Errorf("ошибка слушателя %s: %w", listenAddr, err)
	}
	if uc, ok := localConn.(*net.UDPConn); ok {
		_ = uc.SetReadBuffer(socketBufSize)
		_ = uc.SetWriteBuffer(socketBufSize)
	}
	defer localConn.Close()

	_, localPort, _ := net.SplitHostPort(listenAddr)
	if localPort == "" {
		localPort = "9000"
	}

	numGroups := (numW + workersPerGroup - 1) / workersPerGroup
	log.Println("[КЛИЕНТ] ═══════════════════════════════════════")
	log.Printf("[КЛИЕНТ] Запуск GUI туннеля | Воркеров: %d (групп: %d)", numW, numGroups)
	log.Printf("[КЛИЕНТ] Сервер: %s | Хешей: %d | Режим: %s", cfg.PeerAddr, len(hashes), activeConnMode)
	log.Printf("[КЛИЕНТ] DNS: %s | Captcha: %s | VK Auth: %s", dns, activeCaptchaMode, activeVkAuthMode)
	if activeVkAuthMode == "anonymous" {
		log.Printf("[КЛИЕНТ] VK anon path: %s", activeVkAnonPath)
	}
	log.Println("[КЛИЕНТ] ═══════════════════════════════════════")

	if stats == nil {
		stats = NewStats()
	}

	shutdownCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(shutdownCh)
	}()
	go stats.RunLoop(shutdownCh)

	var disp *Dispatcher
	if activeConnMode == "rawtun" {
		disp = NewDispatcherPendingTUN(ctx, stats)
	} else {
		disp = NewDispatcher(ctx, localConn, stats)
	}
	defer disp.Shutdown()

	configCh := make(chan string, 1)
	configDone := make(chan struct{})

	go func() {
		defer close(configDone)
		select {
		case rawConf, ok := <-configCh:
			if !ok || rawConf == "" {
				return
			}

			if strings.HasPrefix(rawConf, "RAWCONF:") {
				parts := strings.Split(strings.TrimPrefix(rawConf, "RAWCONF:"), "|")
				if len(parts) == 3 {
					ip, dnsCSV, mtuStr := parts[0], parts[1], parts[2]
					cleanup, err := setupPlatformTUN(ctx, disp, cfg.TunFdSock, ip, dnsCSV, mtuStr, cfg.PeerAddr, tp.Hashes)
					if err != nil {
						log.Printf("[RAW] Ошибка настройки TUN: %v", err)
						return
					}
					if cleanup != nil {
						defer cleanup()
					}
					<-ctx.Done()
				}
				return
			}

			finalConf := rawConf
			if !strings.Contains(finalConf, "MTU =") {
				lines := strings.Split(finalConf, "\n")
				var newLines []string
				for _, line := range lines {
					newLines = append(newLines, line)
					if strings.TrimSpace(line) == "[Interface]" {
						newLines = append(newLines, "MTU = 1280")
					}
				}
				finalConf = strings.Join(newLines, "\n")
			}

			_ = os.WriteFile("wg-turn.conf", []byte(finalConf+"\n"), 0600)

			if activeConnMode == "socks" {
				socksA := cfg.SocksAddr
				if socksA == "" {
					socksA = "127.0.0.1:1080"
				}
				dev, tnet, err := startUserspaceWireGuard(finalConf)
				if err != nil {
					log.Printf("[SOCKS] Ошибка userspace WG: %v", err)
					return
				}
				defer dev.Close()
				if err := runSocks5Server(ctx, socksA, tnet, cfg.SocksAuth, cfg.SocksUser, cfg.SocksPass); err != nil {
					log.Printf("[SOCKS] Сервер остановлен: %v", err)
				}
			} else if activeConnMode == "vpn" {
				cleanup, err := startWindowsWireGuardTUN(ctx, finalConf, cfg.PeerAddr)
				if err != nil {
					log.Printf("[VPN] Ошибка запуска WinTUN WireGuard: %v", err)
				} else if cleanup != nil {
					defer cleanup()
				}
				<-ctx.Done()
				return
			}
		case <-ctx.Done():
		}
	}()

	var pauseFlag int32
	var wg sync.WaitGroup
	workerIDCounter := 1
	var prevWaitReady <-chan struct{}

	devID := cfg.DeviceID
	if devID == "" {
		devID = "vk-turn-gui"
	}

	for g := 0; g < numGroups; g++ {
		isFirst := (g == 0)
		var myWaitReady <-chan struct{}
		var mySignalReady chan<- struct{}

		if g > 0 {
			myWaitReady = prevWaitReady
		}
		if g < numGroups-1 {
			ch := make(chan struct{})
			mySignalReady = ch
			prevWaitReady = ch
		}

		startIdx := g * workersPerGroup
		endIdx := startIdx + workersPerGroup
		if endIdx > numW {
			endIdx = numW
		}
		groupSize := endIdx - startIdx
		if groupSize <= 0 {
			continue
		}

		ids := make([]int, groupSize)
		for i := range ids {
			ids[i] = workerIDCounter
			workerIDCounter++
		}

		gID := g + 1
		var cc chan<- string
		if isFirst {
			cc = configCh
		}

		wg.Add(1)
		go func(groupID int, isFirstGroup bool, configChan chan<- string, workerIds []int, startHashIndex int, waitR <-chan struct{}, sigR chan<- struct{}) {
			defer wg.Done()
			WorkerGroup(ctx, groupID, startHashIndex, tp, peer, disp, localPort,
				isFirstGroup, configChan, workerIds, &pauseFlag, devID, cfg.Password, stats, waitR, sigR)
		}(gID, isFirst, cc, ids, g, myWaitReady, mySignalReady)
	}

	wg.Wait()
	close(configCh)
	<-configDone
	log.Println("[КЛИЕНТ] Сессия туннеля остановлена")
	return nil
}
