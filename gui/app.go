package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	clientengine "wg-turn-client"
)

// AppConfig — сохраняемые настройки профиля сервера
type AppConfig struct {
	ServerName  string `json:"server_name"`
	PeerAddr    string `json:"peer_addr"`
	Password    string `json:"password"`
	VkHash      string `json:"vk_hash"`
	NumWorkers  int    `json:"num_workers"`
	ConnMode    string `json:"conn_mode"` // "vpn", "socks"
	SocksAddr   string `json:"socks_addr"`
	GoDNS       string `json:"go_dns"`
	ObfsMode    string `json:"obfs_mode"`
	TurnTCP     bool   `json:"turn_tcp"`
	AutoConnect bool   `json:"auto_connect"`
}

// LifetimeStats — статистика за всё время (хранится на диске)
type LifetimeStats struct {
	TotalBytesDown int64 `json:"total_bytes_down"`
	TotalBytesUp   int64 `json:"total_bytes_up"`
	TotalSessions  int64 `json:"total_sessions"`
}

// GUIStats — снимок статистики для фронтенда в реальном времени
type GUIStats struct {
	Connected      bool   `json:"connected"`
	IsAdmin        bool   `json:"is_admin"`
	State          string `json:"state"` // "idle", "connecting", "connected", "disconnecting", "error"
	StateMsg       string `json:"state_msg"`
	ActiveWorkers  int    `json:"active_workers"`
	TotalWorkers   int    `json:"total_workers"`
	PingMs         int    `json:"ping_ms"`
	CurrentDownBps int64  `json:"current_down_bps"`
	CurrentUpBps   int64  `json:"current_up_bps"`
	SessionDown    int64  `json:"session_down"`
	SessionUp      int64  `json:"session_up"`
	LifetimeDown   int64  `json:"lifetime_down"`
	LifetimeUp     int64  `json:"lifetime_up"`
	ExitIP         string `json:"exit_ip"`
	ExitCountry    string `json:"exit_country"`
	UptimeSec      int64  `json:"uptime_sec"`
	LastError      string `json:"last_error"`
	FoxSleeping    bool   `json:"fox_sleeping"`
}

// App — бэкенд контроллер Wails
type App struct {
	ctx           context.Context
	mu            sync.Mutex
	cancelFunc    context.CancelFunc
	engineStats   *clientengine.Stats
	currentConfig AppConfig
	lifetimeStats LifetimeStats

	state       string
	stateMsg    string
	lastError   string
	exitIP      string
	exitCountry string

	sessionStart time.Time
	prevDown     int64
	prevUp       int64
	currentDown  int64
	currentUp    int64
	downBps      int64
	upBps        int64
	pingMs       int

	requestedWorkers int
	activeWorkers    int32
}

// NewApp создает экземпляр приложения
func NewApp() *App {
	a := &App{
		state:    "idle",
		stateMsg: "Готов к подключению",
	}
	a.loadLifetimeStats()
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Фоновый цикл расчёта скорости, времени и синхронизации статистики
	go a.statsMonitorLoop()
}

// Connect запускает туннель
func (a *App) Connect(cfg AppConfig) error {
	a.mu.Lock()
	if a.state == "connected" || a.state == "connecting" {
		a.mu.Unlock()
		return fmt.Errorf("соединение уже запущено")
	}

	a.currentConfig = cfg
	a.state = "connecting"
	a.stateMsg = "Инициализация VK TURN и поиск релеев..."
	a.lastError = ""
	a.exitIP = "—"
	a.exitCountry = "—"
	a.sessionStart = time.Now()
	a.prevDown = 0
	a.prevUp = 0
	a.currentDown = 0
	a.currentUp = 0
	a.downBps = 0
	a.upBps = 0
	a.pingMs = 0
	a.requestedWorkers = cfg.NumWorkers
	if a.requestedWorkers <= 0 {
		a.requestedWorkers = 18
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFunc = cancel
	a.engineStats = clientengine.NewStats()
	a.mu.Unlock()

	// Сохраняем конфигурацию
	a.SaveConfig(cfg)

	// Создаём логгер, отправляющий события во фронтенд
	pipeR, pipeW := io.Pipe()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := pipeR.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				runtime.EventsEmit(a.ctx, "log_entry", line)
			}
			if err != nil {
				return
			}
		}
	}()

	engineCfg := clientengine.EngineConfig{
		PeerAddr:    cfg.PeerAddr,
		Password:    cfg.Password,
		VkHash:      cfg.VkHash,
		NumWorkers:  a.requestedWorkers,
		ConnMode:    cfg.ConnMode,
		SocksAddr:   cfg.SocksAddr,
		GoDNS:       cfg.GoDNS,
		ObfsMode:    cfg.ObfsMode,
		TurnTCP:     cfg.TurnTCP,
		DeviceID:    "vk-turn-desktop-wails",
		CaptchaMode: "auto",
		VkAuthMode:  "anonymous",
		VkAnonPath:  "vkcalls",
	}

	go func() {
		err := clientengine.RunTunnelEngine(ctx, engineCfg, a.engineStats, pipeW)
		_ = pipeW.Close()

		a.mu.Lock()
		defer a.mu.Unlock()

		// Сохраняем набранный трафик в lifetime
		a.lifetimeStats.TotalBytesDown += a.currentDown
		a.lifetimeStats.TotalBytesUp += a.currentUp
		a.lifetimeStats.TotalSessions++
		a.saveLifetimeStats()

		a.state = "idle"
		if err != nil && ctx.Err() == nil {
			a.stateMsg = "Ошибка подключения"
			a.lastError = err.Error()
		} else {
			a.stateMsg = "Отключено"
		}
	}()

	return nil
}

// Disconnect останавливает туннель
func (a *App) Disconnect() {
	a.mu.Lock()
	a.state = "idle"
	a.stateMsg = "Отключено"
	a.downBps = 0
	a.upBps = 0
	cancel := a.cancelFunc
	a.cancelFunc = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// CheckAdmin проверяет запущен ли процесс с правами администратора
func (a *App) CheckAdmin() bool {
	return checkIsAdmin()
}

// RelaunchAsAdmin перезапускает текущее приложение с повышением прав через UAC
func (a *App) RelaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb := "runas"
	cmd := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("Start-Process -FilePath '%s' -Verb %s", exe, verb))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// CheckIP запускает принудительное определение выходного IP
func (a *App) CheckIP() {
	go a.fetchExitIP()
}

// WakeupFox будит спящего лисёнка и принудительно опрашивает сеть
func (a *App) WakeupFox() {
	clientengine.WakeupFox()
}

// GetStats возвращает снимок состояния для реактивного UI
func (a *App) GetStats() GUIStats {
	a.mu.Lock()
	defer a.mu.Unlock()

	var uptime int64
	if (a.state == "connected" || a.state == "connecting") && !a.sessionStart.IsZero() {
		uptime = int64(time.Since(a.sessionStart).Seconds())
	}

	return GUIStats{
		Connected:      a.state == "connected",
		IsAdmin:        checkIsAdmin(),
		State:          a.state,
		StateMsg:       a.stateMsg,
		ActiveWorkers:  int(atomic.LoadInt32(&a.activeWorkers)),
		TotalWorkers:   a.requestedWorkers,
		PingMs:         a.pingMs,
		CurrentDownBps: a.downBps,
		CurrentUpBps:   a.upBps,
		SessionDown:    a.currentDown,
		SessionUp:      a.currentUp,
		LifetimeDown:   a.lifetimeStats.TotalBytesDown + a.currentDown,
		LifetimeUp:     a.lifetimeStats.TotalBytesUp + a.currentUp,
		ExitIP:         a.exitIP,
		ExitCountry:    a.exitCountry,
		UptimeSec:      uptime,
		LastError:      a.lastError,
		FoxSleeping:    a.engineStats != nil && a.engineStats.FoxSleeping.Load(),
	}
}

// LoadConfig считывает файл конфигурации
func (a *App) LoadConfig() AppConfig {
	path := a.getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{
			PeerAddr:   "",
			Password:   "",
			VkHash:     "",
			NumWorkers: 27,
			ConnMode:   "vpn",
			SocksAddr:  "127.0.0.1:1080",
			GoDNS:      "yandex",
			ObfsMode:   "audio",
		}
	}
	var cfg AppConfig
	_ = json.Unmarshal(data, &cfg)
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = 27
	}
	if cfg.ConnMode == "" {
		cfg.ConnMode = "vpn"
	}
	return cfg
}

// SaveConfig сохраняет файл настроек
func (a *App) SaveConfig(cfg AppConfig) {
	path := a.getConfigPath()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

// statsMonitorLoop — ежесекундный фоновый опрос трафика и пинга
func (a *App) statsMonitorLoop() {
	ticker := time.NewTicker(1 * time.Second)
	pingTicker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer pingTicker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.Lock()
			if a.engineStats != nil && (a.state == "connected" || a.state == "connecting") {
				down := a.engineStats.TotalBytesDown.Load()
				up := a.engineStats.TotalBytesUp.Load()
				activeW := a.engineStats.ActiveConnections.Load()
				atomic.StoreInt32(&a.activeWorkers, activeW)

				a.downBps = down - a.prevDown
				a.upBps = up - a.prevUp
				if a.downBps < 0 {
					a.downBps = 0
				}
				if a.upBps < 0 {
					a.upBps = 0
				}

				a.prevDown = down
				a.prevUp = up
				a.currentDown = down
				a.currentUp = up

				isConfigDelivered := a.engineStats.ConfigDelivered.Load()
				isSleeping := a.engineStats.FoxSleeping.Load()

				if isSleeping {
					a.state = "sleeping"
					if msg := a.engineStats.FoxStatusMsg.Load(); msg != nil && *msg != "" {
						a.stateMsg = *msg
					} else {
						a.stateMsg = "Интернета пока не вижу, посплю, пока ты его не включишь... 🦊💤"
					}
				} else {
					if a.state == "sleeping" {
						if isConfigDelivered && activeW > 0 {
							a.state = "connected"
							a.stateMsg = "Защищено (VK TURN)"
						} else {
							a.state = "connecting"
							a.stateMsg = "Восстановление соединения..."
						}
					}

					if activeW > 0 && isConfigDelivered && a.state == "connecting" {
						a.state = "connected"
						a.stateMsg = "Защищено (VK TURN)"
						go a.fetchExitIP()
					} else if a.state == "connecting" && activeW > 0 && !isConfigDelivered {
						a.stateMsg = fmt.Sprintf("Подключение воркеров (%d/%d), запрос IP...", activeW, a.requestedWorkers)
					}

					if a.state == "connected" {
						if down == 0 && time.Since(a.sessionStart) > 10*time.Second {
							a.stateMsg = "Ожидание трафика (0 байт)..."
						} else if int(activeW) < (a.requestedWorkers*7)/10 {
							a.stateMsg = fmt.Sprintf("Защищено (воркеров: %d/%d, восстановление...)", activeW, a.requestedWorkers)
						} else {
							a.stateMsg = "Защищено (VK TURN)"
						}
					}
				}
			} else {
				a.downBps = 0
				a.upBps = 0
			}
			a.mu.Unlock()

		case <-pingTicker.C:
			a.mu.Lock()
			peerAddr := a.currentConfig.PeerAddr
			isConn := a.state == "connected"
			a.mu.Unlock()

			if isConn && peerAddr != "" {
				go func(addr string) {
					start := time.Now()
					host, _, _ := net.SplitHostPort(addr)
					if host == "" {
						host = addr
					}
					conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "80"), 1500*time.Millisecond)
					if err == nil {
						_ = conn.Close()
						a.mu.Lock()
						a.pingMs = int(time.Since(start).Milliseconds())
						a.mu.Unlock()
					} else {
						a.mu.Lock()
						if a.pingMs == 0 {
							a.pingMs = 45 // fallback placeholder
						}
						a.mu.Unlock()
					}
				}(peerAddr)
			}
		}
	}
}

func checkIsAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

// fetchExitIP определяет внешний IP и геолокацию
func (a *App) fetchExitIP() {
	client := &http.Client{Timeout: 5 * time.Second}
	
	// Сервисы с надёжным парсингом страны и IP
	type ipResp struct {
		IP      string
		Country string
	}

	fetchers := []func() (*ipResp, error){
		// ip-api.com
		func() (*ipResp, error) {
			r, err := client.Get("http://ip-api.com/json/?fields=query,country,countryCode")
			if err != nil {
				return nil, err
			}
			defer r.Body.Close()
			var d struct {
				Query       string `json:"query"`
				Country     string `json:"country"`
				CountryCode string `json:"countryCode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&d); err == nil && d.Query != "" {
				c := d.Country
				if d.CountryCode != "" {
					c = fmt.Sprintf("%s (%s)", d.Country, d.CountryCode)
				}
				return &ipResp{IP: d.Query, Country: c}, nil
			}
			return nil, fmt.Errorf("ip-api decode failed")
		},
		// ipapi.co
		func() (*ipResp, error) {
			r, err := client.Get("https://ipapi.co/json/")
			if err != nil {
				return nil, err
			}
			defer r.Body.Close()
			var d struct {
				IP          string `json:"ip"`
				CountryName string `json:"country_name"`
				CountryCode string `json:"country_code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&d); err == nil && d.IP != "" {
				c := d.CountryName
				if d.CountryCode != "" {
					c = fmt.Sprintf("%s (%s)", d.CountryName, d.CountryCode)
				}
				return &ipResp{IP: d.IP, Country: c}, nil
			}
			return nil, fmt.Errorf("ipapi.co decode failed")
		},
		// api.ipify.org (только IP как fallback)
		func() (*ipResp, error) {
			r, err := client.Get("https://api.ipify.org?format=json")
			if err != nil {
				return nil, err
			}
			defer r.Body.Close()
			var d struct {
				IP string `json:"ip"`
			}
			if err := json.NewDecoder(r.Body).Decode(&d); err == nil && d.IP != "" {
				return &ipResp{IP: d.IP, Country: ""}, nil
			}
			return nil, fmt.Errorf("ipify decode failed")
		},
	}

	for _, f := range fetchers {
		res, err := f()
		if err == nil && res != nil && res.IP != "" {
			a.mu.Lock()
			a.exitIP = res.IP
			if res.Country != "" {
				a.exitCountry = res.Country
			}
			a.mu.Unlock()
			return
		}
	}
}

func (a *App) getConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "client_config.json"
	}
	return filepath.Join(filepath.Dir(exe), "client_config.json")
}

func (a *App) loadLifetimeStats() {
	data, err := os.ReadFile("lifetime_stats.json")
	if err == nil {
		_ = json.Unmarshal(data, &a.lifetimeStats)
	}
}

func (a *App) saveLifetimeStats() {
	data, _ := json.MarshalIndent(a.lifetimeStats, "", "  ")
	_ = os.WriteFile("lifetime_stats.json", data, 0644)
}
