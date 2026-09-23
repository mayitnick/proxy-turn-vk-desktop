# agents.md — Руководство разработчика и агентов по проекту VK TURN Proxy (Windows / Android)

## 1. Архитектура системы

Проект представляет собой клиент-серверную систему для обхода блокировок и цензуры через инфраструктуру **VK Calls (звонки ВКонтакте)** с использованием протокола **TURN Relay**.

```
[Пользователь / ОС] 
       ↓ (WinTUN / TUN interface)
[Локальный Client (Go)]
       ↓ (RTP-obfs AEAD / DTLS)
[VK TURN Relays (calls.okcdn.ru / 91.231.x.x, 90.156.x.x)]
       ↓ (TURN UDP / TCP Data Channels)
[VPS Server (Go - wdtt-server)]
       ↓ (iptables NAT)
[Внешний Интернет]
```

---

## 2. Ключевые протоколы и режимы

1. **Режим WireGuard + DTLS (`-mode vpn`)**:
   - Клиент соединяется с VPS по порту `56000` через DTLS поверх TURN релеев.
   - Запрашивает конфигурацию: `GETCONF:<local_port>|<device_id>|<password>`.
   - Сервер выделяет IP-адрес из пула `10.66.0.0/24` и возвращает конфигурацию WireGuard.
   - Клиент поднимает виртуальный адаптер **WinTUN** (`VKTurnWG`) через встроенный `golang.zx2c4.com/wireguard`, настраивает IP, MTU (1280) и DNS.
   - Трафик системы заворачивается в туннель префиксами `0.0.0.0/1` и `128.0.0.0/1`.

2. **Прямой режим Raw-IP (`-mode rawtun`, `-notls`)**:
   - Клиент отправляет: `GETCONF_RAW:<device_id>|<password>`.
   - Сервер отвечает: `RAWCONF:<client_ip>|<dns_csv>|<mtu>`.
   - Сырые IP-пакеты ядра Windows читаются из кольцевого буфера WinTUN (8 МБ) и шифруются **RTP-obfs ChaCha20-Poly1305 AEAD** без оверхеда DTLS.
   - Ключ шифрования выводится из пароля подключения через **HKDF-SHA256**:
     - Salt: `"WDTT-WRAP-v1"`
     - Info: `"rtp-obfs/chacha20poly1305"`

3. **Режим SOCKS5 (`-mode socks`)**:
   - Клиент поднимает локальный SOCKS5 прокси (`127.0.0.1:1080`), гоняя трафик через userspace WireGuard (netstack) без создания TUN адаптера.

---

## 3. Критически важные правила маршрутизации (Routing Exceptions)

Чтобы предотвратить **петлю маршрутизации (Routing Loop)** в Windows:
- До применения маршрутов `0.0.0.0/1` и `128.0.0.0/1` определяются:
  1. Физический шлюз по умолчанию (`Default Gateway`) и интерфейс через `Get-NetRoute`.
  2. IP-адрес VPS сервера (`peerAddr`).
  3. Все IP-адреса серверов VK TURN (`GetDiscoveredTurnIPs()`).
- Для каждого из этих IP прописываются **строгие маршруты `/32` через физический шлюз**:
  `route add <IP> mask 255.255.255.255 <GW_IP> metric 1`
- Маршруты по умолчанию в WinTUN добавляются с явным указанием индекса интерфейса (`IF <ifIndex>`):
  `route add 0.0.0.0 mask 128.0.0.0 <clientIP> IF <ifIndex> metric 1`
  `route add 128.0.0.0 mask 128.0.0.0 <clientIP> IF <ifIndex> metric 1`
- При завершении процесса (`Ctrl+C`) деструктор обязан удалить добавленные маршруты и закрыть сессию WinTUN.

---

## 4. Сборка проекта для Windows

### Требования к среде:
- **Go 1.24.1+** (строго 1.24.1 или выше из-за зависимостей `fhttp` и `quic-go-utls`).
- Файл драйвера **`wintun.dll`** (64-бит) из официального репозитория WireGuard / Wintun.net.

### Команды компиляции:
```powershell
$env:GOTOOLCHAIN = "local"
$env:GOSUMDB = "off"
cd go_client
go build -ldflags="-s -w" -o "..\portable_client\vk-turn-client.exe" .\cmd\vk-turn-client
```

---

## 5. Структура Portable-билда (`portable_client`)

```
portable_client/
 ├── vk-turn-client.exe   # Скомпилированный бинарник клиента
 ├── wintun.dll           # 64-битный драйвер WinTUN (рядом с .exe)
 ├── config.ini           # Конфигурационный файл (PEER, PASSWORD, VK_HASH, WORKERS)
 ├── wg-turn.conf         # WireGuard-конфигурация (создается автоматически сервером)
 ├── fwdtt_gui.py         # Легковесный графический интерфейс (CustomTkinter)
 └── start.bat            # Запуск от имени Администратора
```

---

## 6. Важные файлы исходного кода

- `go_client/cmd/vk-turn-client/main.go` — точка входа CLI.
- `go_client/engine.go` — программный интерфейс ядра туннеля (`RunTunnelEngine`) для CLI и Wails GUI.
- `go_client/link_parser.go` — парсер конфигурационных ссылок `qwdtt://` и `wdtt://`.
- `go_client/wg_wintun_windows.go` — встроенный движок WireGuard поверх WinTUN для Windows.
- `go_client/tun_windows.go` — создание адаптера WinTUN, управление сессией, ring buffer (8 МБ), маршруты.
- `go_client/turn_registry.go` — реестр обнаруженных TURN релеев для динамических исключений.
- `go_client/pipe.go` — внутренняя реализация in-memory `AsyncPacketPipe` (замена внешней библиотеки `connutil`).
- `go_client/listen_windows.go` — реализация `SO_REUSEADDR` для Windows через `syscall.Handle`.
- `go_client/dispatcher.go` — многопоточный диспетчер пакетов и воркеров (адаптирован под `io.ReadWriteCloser`).
- `gui/` — полнофункциональное приложение на Wails v2 (Go + HTML/JS/CSS) с мониторингом трафика в реальном времени, графиками и определением GeoIP.
