package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	clientengine "wg-turn-client"
)

// DiagStep — один шаг в пайплайне самодиагностики
type DiagStep struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // "pending", "running", "ok", "warn", "error"
	Message   string `json:"message"`
	Hint      string `json:"hint"`
	LatencyMs int64  `json:"latency_ms"`
}

// DiagReport — итоговый отчёт диагностики
type DiagReport struct {
	OverallStatus string     `json:"overall_status"` // "ok", "warn", "error"
	Timestamp     string     `json:"timestamp"`
	Summary       string     `json:"summary"`
	Steps         []DiagStep `json:"steps"`
}

// RunDiagnostics выполняет пошаговую самодиагностику туннеля и окружения
func (a *App) RunDiagnostics() DiagReport {
	cfg := a.LoadConfig()
	isConnected := (a.state == "connected")

	report := DiagReport{
		OverallStatus: "ok",
		Timestamp:     time.Now().Format("02.01.2006 15:04:05"),
		Steps:         make([]DiagStep, 0, 7),
	}

	hasError := false
	hasWarn := false

	addStep := func(step DiagStep) {
		if step.Status == "error" {
			hasError = true
		} else if step.Status == "warn" {
			hasWarn = true
		}
		report.Steps = append(report.Steps, step)
	}

	// ─── ШАГ 1: Физический интернет и сетевой шлюз ───
	{
		step := DiagStep{
			ID:    "physical_net",
			Title: "1. Физическая сеть и интернет (LAN / Wi-Fi)",
		}
		start := time.Now()
		gw, iface, _ := clientengine.GetDefaultGateway()
		online := clientengine.ProbePhysicalInternet()
		step.LatencyMs = time.Since(start).Milliseconds()

		if online {
			step.Status = "ok"
			gwInfo := ""
			if gw != "" {
				gwInfo = fmt.Sprintf(", шлюз %s [%s]", gw, iface)
			}
			step.Message = fmt.Sprintf("Физический интернет доступен (пинг %d мс%s)", step.LatencyMs, gwInfo)
			step.Hint = "Базовое сетевое подключение в полном порядке."
		} else {
			step.Status = "error"
			step.Message = "Нет физического доступа в интернет (DNS Яндекс 77.88.8.8 не отвечает)"
			step.Hint = "Проверьте сетевой кабель или подключение к Wi-Fi. Убедитесь, что сетевой адаптер включен."
		}
		addStep(step)
	}

	// ─── ШАГ 2: DNS резолвинг доменов VK ───
	{
		step := DiagStep{
			ID:    "dns_resolve",
			Title: "2. DNS резолвинг доменов VK и инфраструктуры",
		}
		start := time.Now()
		domains := []string{"vk.com", "api.vk.me", "calls.okcdn.ru"}
		allResolved := true
		var failedDomains []string
		totalIPs := 0

		for _, d := range domains {
			ips, err := net.LookupIP(d)
			if err != nil || len(ips) == 0 {
				allResolved = false
				failedDomains = append(failedDomains, d)
			} else {
				totalIPs += len(ips)
			}
		}
		step.LatencyMs = time.Since(start).Milliseconds()

		if allResolved {
			step.Status = "ok"
			step.Message = fmt.Sprintf("Все домены VK разрешены (найдено %d IP, %d мс)", totalIPs, step.LatencyMs)
			step.Hint = "Системный DNS работает штатно."
		} else {
			step.Status = "error"
			step.Message = fmt.Sprintf("Не удалось разрешить домены: %s", strings.Join(failedDomains, ", "))
			step.Hint = "Провайдер может блокировать DNS-запросы. Рекомендуется сменить DNS на Yandex или Cloudflare в Настройках."
		}
		addStep(step)
	}

	// ─── ШАГ 3: Валидность хешей звонков VK (до 4 шт.) ───
	var activeTurnUrls []string
	{
		step := DiagStep{
			ID:    "vk_hashes",
			Title: "3. Проверка комнат звонков VK (активность хешей)",
		}

		hashes := cfg.GetAllHashes()
		if len(hashes) == 0 {
			step.Status = "error"
			step.Message = "Не указан ни один хеш или ссылка звонка VK"
			step.Hint = "Вставьте ссылку на звонок VK в Настройках (например: https://vk.com/call/join/...)."
		} else {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()

			results := clientengine.CheckMultipleVKCallHashes(ctx, hashes)
			step.LatencyMs = time.Since(start).Milliseconds()

			okCount := 0
			expiredCount := 0
			var details []string

			for i, r := range results {
				shortHash := r.Hash
				if len(shortHash) > 24 {
					shortHash = shortHash[:24] + "..."
				}
				if r.Status == "ok" {
					okCount++
					activeTurnUrls = append(activeTurnUrls, r.TurnURLs...)
					details = append(details, fmt.Sprintf("#%d: 🟢 OK (%d TURN)", i+1, r.TurnCount))
				} else if r.Status == "expired" {
					expiredCount++
					details = append(details, fmt.Sprintf("#%d: 🔴 Протух", i+1))
				} else {
					details = append(details, fmt.Sprintf("#%d: 🔴 Ошибка", i+1))
				}
			}

			if okCount == len(hashes) {
				step.Status = "ok"
				step.Message = fmt.Sprintf("Все хеши активны (%d из %d): %s", okCount, len(hashes), strings.Join(details, ", "))
				step.Hint = "Комнаты звонков VK открыты, TURN-креды успешно генерируются."
			} else if okCount > 0 {
				step.Status = "warn"
				step.Message = fmt.Sprintf("Часть хешей протухла (активно %d из %d): %s", okCount, len(hashes), strings.Join(details, ", "))
				step.Hint = "Обновите неактивные хеши в Настройках, чтобы задействовать все воркеры."
			} else {
				step.Status = "error"
				step.Message = fmt.Sprintf("Все хеши протухли (%d шт.): %s", len(hashes), strings.Join(details, ", "))
				step.Hint = "Звонки в VK завершены или комнаты удалены. Создайте новую ссылку в VK Звонки (vk.com/calls) и обновите слоты."
			}
		}
		addStep(step)
	}

	// ─── ШАГ 4: Доступность TURN-релеев VK ───
	{
		step := DiagStep{
			ID:    "turn_relays",
			Title: "4. Доступность TURN-релеев VK (calls.okcdn.ru)",
		}
		start := time.Now()

		targetTurn := "calls.okcdn.ru:3478"
		if len(activeTurnUrls) > 0 {
			targetTurn = activeTurnUrls[0]
		}

		network := "udp"
		if cfg.TurnTCP {
			network = "tcp"
		}

		d := net.Dialer{Timeout: 2500 * time.Millisecond}
		conn, err := d.Dial(network, targetTurn)
		step.LatencyMs = time.Since(start).Milliseconds()

		if err == nil {
			_ = conn.Close()
			step.Status = "ok"
			step.Message = fmt.Sprintf("Связь с TURN-релеем %s (%s) успешна (%d мс)", targetTurn, strings.ToUpper(network), step.LatencyMs)
			step.Hint = "Порты WebRTC TURN открыты и отвечают."
		} else {
			step.Status = "error"
			step.Message = fmt.Sprintf("Не удалось подключиться к TURN-релею %s: %v", targetTurn, err)
			step.Hint = "Провайдер может блокировать TURN UDP. Попробуйте включить опцию 'Использовать TCP вместо UDP' в Настройках."
		}
		addStep(step)
	}

	// ─── ШАГ 5: Доступность сервера VPS (PEER) ───
	{
		step := DiagStep{
			ID:    "vps_peer",
			Title: "5. Доступность VPS сервера (PEER:PORT)",
		}
		start := time.Now()

		if cfg.PeerAddr == "" {
			step.Status = "error"
			step.Message = "Адрес сервера (PEER) не указан в настройках"
			step.Hint = "Заполните поле 'Адрес сервера (PEER:PORT)' в Настройках."
		} else {
			host, port, err := net.SplitHostPort(cfg.PeerAddr)
			if err != nil {
				host = cfg.PeerAddr
				port = "56000"
			}

			// Проверяем UDP сокет и резолвинг адреса VPS
			udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
			step.LatencyMs = time.Since(start).Milliseconds()

			if err != nil {
				step.Status = "error"
				step.Message = fmt.Sprintf("Не удалось разрешить адрес VPS: %v", err)
				step.Hint = "Проверьте правильность IP адреса или домена сервера в Настройках."
			} else {
				conn, err := net.DialUDP("udp", nil, udpAddr)
				if err != nil {
					step.Status = "error"
					step.Message = fmt.Sprintf("Ошибка сокета к VPS: %v", err)
					step.Hint = "Проверьте сетевой адаптер и правила локального файрвола."
				} else {
					_ = conn.Close()
					step.Status = "ok"
					step.Message = fmt.Sprintf("Сокет к VPS (%s) открывается штатно", udpAddr.String())
					step.Hint = "Сетевой маршрут до VPS готов к передаче трафика."
				}
			}
		}
		addStep(step)
	}

	// ─── ШАГ 6: WinTUN адаптер и системные привилегии ───
	{
		step := DiagStep{
			ID:    "wintun_driver",
			Title: "6. Системные права и драйвер WinTUN",
		}
		isAdmin := a.CheckAdmin()
		exeDir := filepath.Dir(os.Args[0])
		wintunPath := filepath.Join(exeDir, "wintun.dll")
		_, errDll := os.Stat(wintunPath)

		if isAdmin && errDll == nil {
			step.Status = "ok"
			step.Message = "Права Администратора активны, wintun.dll присутствует"
			step.Hint = "Виртуальный сетевой адаптер полностью готов к работе."
		} else if !isAdmin {
			step.Status = "warn"
			step.Message = "Приложение запущено БЕЗ прав Администратора"
			step.Hint = "Для создания WinTUN адаптера и управления маршрутами Windows требуются права Администратора. Нажмите 'Перезапустить' в желтой плашке."
		} else {
			step.Status = "error"
			step.Message = fmt.Sprintf("Файл wintun.dll не найден в %s", exeDir)
			step.Hint = "Поместите 64-битный файл wintun.dll рядом с программой."
		}
		addStep(step)
	}

	// ─── ШАГ 7: Сквозная проверка выхода в интернет (Egress Health) ───
	{
		step := DiagStep{
			ID:    "tunnel_egress",
			Title: "7. Сквозной выход в интернет через туннель (Egress Health)",
		}

		if isConnected {
			start := time.Now()
			client := &http.Client{Timeout: 3500 * time.Millisecond}
			resp, err := client.Get("http://api.ipify.org?format=text")
			step.LatencyMs = time.Since(start).Milliseconds()

			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				ip := strings.TrimSpace(string(body))
				step.Status = "ok"
				step.Message = fmt.Sprintf("Сквозной трафик идёт отлично! Внешний IP: %s (%d мс)", ip, step.LatencyMs)
				step.Hint = "Туннель полностью исправен, трафик успешно покидает VPS и идёт в интернет."
			} else {
				step.Status = "error"
				step.Message = "Первые байты ушли, но интернет не отвечает (Zero Egress)!"
				step.Hint = "Туннель соединён с VPS, но сервер не маршрутизирует пакеты в интернет! Решение на сервере: 1) включите IP Forwarding: 'sysctl -w net.ipv4.ip_forward=1'; 2) добавьте NAT: 'iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE'; 3) перезапустите wdtt-server."
			}
		} else {
			step.Status = "pending"
			step.Message = "Туннель сейчас отключен (проверка доступна при подключении)"
			step.Hint = "Подключите туннель и запустите диагностику снова для проверки сквозного выхода в сеть."
		}
		addStep(step)
	}

	// Формируем общее резюме
	if hasError {
		report.OverallStatus = "error"
		report.Summary = "Обнаружены проблемы, блокирующие работу туннеля! Ознакомьтесь с подсказками ниже."
	} else if hasWarn {
		report.OverallStatus = "warn"
		report.Summary = "Туннель может работать, но обнаружены предупреждения, требующие внимания."
	} else {
		report.OverallStatus = "ok"
		report.Summary = "Все компоненты системы работают безупречно! Туннель полностью готов к использованию 🦊✨"
	}

	return report
}
