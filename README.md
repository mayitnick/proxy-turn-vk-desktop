# VK TURN Proxy (Desktop)

Настольный клиент для обхода блокировок и цензуры через распределённую WebRTC TURN-инфраструктуру сервиса VK Calls (звонки ВКонтакте) с поддержкой виртуального сетевого адаптера **WinTUN**, **WireGuard** и локального **SOCKS5**-прокси.

---

## 📜 Происхождение и лицензия (Attribution & License)

Проект является форком оригинального проекта [SpaceNeuroX/proxy-turn-vk-android](https://github.com/SpaceNeuroX/proxy-turn-vk-android) (который, в свою очередь, базировался на наработках [WDTT](https://github.com/amurcanov/proxy-turn-vk-android)).

Проект распространяется под свободной лицензией **GNU General Public License v3.0 (GPL v3)** — см. файл [LICENSE](LICENSE).

### Ключевые изменения в десктопной версии:
- **Удалена кодовая база Android:** проект полностью освобождён от Gradle, Android SDK/NDK и мобильных зависимостей.
- **Поддержка WinTUN для Windows:** встроен высокоскоростной движок WinTUN (WireGuard C/Go bindings) с 8-мегабайтным кольцевым буфером для перехвата и маршрутизации всего системного IP-трафика.
- **Бесшовное управление маршрутами:** скрытый запуск системных утилит (`route`, `netsh`, PowerShell) без мерцания консольных окон и с корректным приоритетом метрик интерфейса.
- **Поддержка конфигурационных ссылок:** быстрый импорт параметров из ссылок `qwdtt://` и `wdtt://`.
- **Два варианта GUI:**
  - **Wails v2 Desktop GUI:** современный десктопный клиент на Go + Web-стеке с мониторингом скорости в реальном времени, графиками, проверкой GeoIP и UAC-элевацией.
  - **CustomTkinter GUI (`fwdtt_gui.py`):** легковесный интерфейс для portable-сборки.
- **Автоматическое предотвращение петель маршрутизации (Routing Exceptions):** динамическое определение физического шлюза и создание точечных `/32` маршрутов-исключений для VPS и пула TURN-релеев VK.
- **Портативная сборка (CLI):** готовая структура `portable_client/` для быстрого развёртывания.

---

## 🚀 Архитектура решения

```
[Пользователь / Приложения Windows]
       │
       ▼ (Виртуальный адаптер WinTUN)
[vk-turn-client (Go)]
       │
       ▼ (RTP-obfs ChaCha20-Poly1305 AEAD / DTLS)
[VK Calls TURN Relays (calls.okcdn.ru / 91.231.x.x, 90.156.x.x)]
       │
       ▼ (TURN UDP / TCP Data Channels)
[Ваш VPS сервер (wdtt-server)]
       │
       ▼ (iptables NAT)
[Внешний Интернет]
```

---

## 🛠️ Сборка и запуск CLI

### Требования:
- **Go 1.24+** (64-бит)
- Windows 10 / 11 с правами Администратора (для создания адаптера WinTUN)

### Сборка клиента:
```powershell
cd go_client
go build -ldflags="-s -w" -o "..\portable_client\vk-turn-client.exe" .\cmd\vk-turn-client
```

### Запуск по ссылке:
```powershell
vk-turn-client.exe -uri "qwdtt://config?peer=1.2.3.4:56000&pass=secret&hashes=..."
# или просто передав ссылку первым аргументом:
vk-turn-client.exe "qwdtt://config?..."
```

### Настройка через config.ini:
1. Скопируйте файл `portable_client/config.example.ini` в `portable_client/config.ini`:
   ```powershell
   copy portable_client\config.example.ini portable_client\config.ini
   ```
2. Откройте `config.ini` и укажите:
   - `PEER` — IP и порт вашего сервера (например: `1.2.3.4:56000`).
   - `PASSWORD` — пароль подключения к серверу.
   - `VK_HASH` — ссылка или хеш группового звонка ВКонтакте.
   - `WORKERS` — количество потоков (рекомендуется от `18` до `27`).
3. Запустите `start.bat` (CLI) или `start_gui.bat` (Python GUI) от имени **Администратора**.

---

## 🖥️ Сборка Wails v2 GUI

### Требования:
- Node.js 18+ & npm
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Сборка:
```powershell
cd gui
wails build -clean -ldflags="-s -w"
```
Скомпилированное приложение будет доступно в `gui/build/bin/vk-turn-desktop.exe`.
