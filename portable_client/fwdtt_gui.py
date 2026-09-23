import os
import sys
import json
import re
import urllib.parse
import subprocess
import threading
import queue
import customtkinter as ctk

# ═════════════════════════════════════════════════════════════════════
#  FWDTT — Fox WireGuard over TURN Tunnel (Windows GUI)
# ═════════════════════════════════════════════════════════════════════

ctk.set_appearance_mode("Dark")
ctk.set_default_color_theme("dark-blue")

# Лисья тёплая, но ненавязчивая палитра
COLOR_BG = "#121316"
COLOR_CARD = "#1A1C23"
COLOR_INPUT = "#222530"
COLOR_FOX_ORANGE = "#E57A38"
COLOR_FOX_HOVER = "#F08A4B"
COLOR_GREEN = "#2ECC71"
COLOR_RED = "#E74C3C"
COLOR_TEXT_DIM = "#8A8F9E"
COLOR_BORDER = "#2B2E3C"

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "config.ini")
CLIENT_EXE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "vk-turn-client.exe")

class FwdttApp(ctk.CTk):
    def __init__(self):
        super().__init__()

        self.title("FWDTT — Fox WireGuard over TURN Tunnel")
        self.geometry("640x720")
        self.minsize(580, 650)
        self.configure(fg_color=COLOR_BG)

        self.process = None
        self.log_queue = queue.Queue()
        self.is_running = False

        self._build_ui()
        self._load_config()
        self._poll_logs()

    def _build_ui(self):
        # Верхняя панель (Header)
        header_frame = ctk.CTkFrame(self, fg_color="transparent")
        header_frame.pack(fill="x", padx=24, pady=(20, 10))

        title_box = ctk.CTkFrame(header_frame, fg_color="transparent")
        title_box.pack(side="left")

        title_label = ctk.CTkLabel(
            title_box,
            text="🦊 FWDTT",
            font=ctk.CTkFont(family="Segoe UI", size=22, weight="bold"),
            text_color="#FFFFFF"
        )
        title_label.pack(anchor="w")

        subtitle_label = ctk.CTkLabel(
            title_box,
            text="Fox WireGuard over TURN Tunnel • WinTUN Client",
            font=ctk.CTkFont(family="Segoe UI", size=12),
            text_color=COLOR_TEXT_DIM
        )
        subtitle_label.pack(anchor="w")

        self.status_badge = ctk.CTkLabel(
            header_frame,
            text="● Отключено",
            font=ctk.CTkFont(family="Segoe UI", size=13, weight="bold"),
            text_color=COLOR_TEXT_DIM,
            fg_color=COLOR_CARD,
            corner_radius=8,
            padx=12,
            pady=6
        )
        self.status_badge.pack(side="right")

        # Карточка быстрого импорта (qwdtt:// / wdtt://)
        import_card = ctk.CTkFrame(self, fg_color=COLOR_CARD, corner_radius=12, border_width=1, border_color=COLOR_BORDER)
        import_card.pack(fill="x", padx=24, pady=8)

        import_title = ctk.CTkLabel(
            import_card,
            text="Быстрый импорт ссылки",
            font=ctk.CTkFont(family="Segoe UI", size=12, weight="bold"),
            text_color=COLOR_FOX_ORANGE
        )
        import_title.pack(anchor="w", padx=16, pady=(12, 4))

        import_box = ctk.CTkFrame(import_card, fg_color="transparent")
        import_box.pack(fill="x", padx=16, pady=(0, 12))

        self.uri_entry = ctk.CTkEntry(
            import_box,
            placeholder_text="Вставьте qwdtt:// или wdtt:// ссылку...",
            font=ctk.CTkFont(family="Segoe UI", size=12),
            fg_color=COLOR_INPUT,
            border_color=COLOR_BORDER,
            height=36
        )
        self.uri_entry.pack(side="left", fill="x", expand=True, padx=(0, 8))

        import_btn = ctk.CTkButton(
            import_box,
            text="Импорт",
            width=85,
            height=36,
            fg_color=COLOR_FOX_ORANGE,
            hover_color=COLOR_FOX_HOVER,
            font=ctk.CTkFont(family="Segoe UI", size=12, weight="bold"),
            command=self._apply_uri
        )
        import_btn.pack(side="right")

        # Основные настройки (Settings Card)
        settings_card = ctk.CTkFrame(self, fg_color=COLOR_CARD, corner_radius=12, border_width=1, border_color=COLOR_BORDER)
        settings_card.pack(fill="x", padx=24, pady=8)

        # Peer
        ctk.CTkLabel(settings_card, text="Адрес сервера (Peer:Port)", font=ctk.CTkFont(family="Segoe UI", size=12), text_color=COLOR_TEXT_DIM).pack(anchor="w", padx=16, pady=(12, 2))
        self.peer_entry = ctk.CTkEntry(settings_card, placeholder_text="87.232.123.42:56000", fg_color=COLOR_INPUT, border_color=COLOR_BORDER, height=34)
        self.peer_entry.pack(fill="x", padx=16, pady=(0, 8))

        # Password
        ctk.CTkLabel(settings_card, text="Пароль подключения (WRAP Key)", font=ctk.CTkFont(family="Segoe UI", size=12), text_color=COLOR_TEXT_DIM).pack(anchor="w", padx=16, pady=(2, 2))
        self.pass_entry = ctk.CTkEntry(settings_card, placeholder_text="Пароль сервера...", show="•", fg_color=COLOR_INPUT, border_color=COLOR_BORDER, height=34)
        self.pass_entry.pack(fill="x", padx=16, pady=(0, 8))

        # VK Hash
        ctk.CTkLabel(settings_card, text="Хеши VK-звонков (через запятую)", font=ctk.CTkFont(family="Segoe UI", size=12), text_color=COLOR_TEXT_DIM).pack(anchor="w", padx=16, pady=(2, 2))
        self.hash_entry = ctk.CTkEntry(settings_card, placeholder_text="2WYt7qKS05mwBKis...", fg_color=COLOR_INPUT, border_color=COLOR_BORDER, height=34)
        self.hash_entry.pack(fill="x", padx=16, pady=(0, 8))

        # Workers & Mode row
        row_frame = ctk.CTkFrame(settings_card, fg_color="transparent")
        row_frame.pack(fill="x", padx=16, pady=(2, 14))

        workers_box = ctk.CTkFrame(row_frame, fg_color="transparent")
        workers_box.pack(side="left", fill="x", expand=True, padx=(0, 8))
        ctk.CTkLabel(workers_box, text="Воркеры (потоки)", font=ctk.CTkFont(family="Segoe UI", size=12), text_color=COLOR_TEXT_DIM).pack(anchor="w")
        self.workers_entry = ctk.CTkEntry(workers_box, fg_color=COLOR_INPUT, border_color=COLOR_BORDER, height=34)
        self.workers_entry.insert(0, "18")
        self.workers_entry.pack(fill="x", pady=(2, 0))

        mode_box = ctk.CTkFrame(row_frame, fg_color="transparent")
        mode_box.pack(side="right", fill="x", expand=True, padx=(8, 0))
        ctk.CTkLabel(mode_box, text="Режим работы", font=ctk.CTkFont(family="Segoe UI", size=12), text_color=COLOR_TEXT_DIM).pack(anchor="w")
        self.mode_menu = ctk.CTkOptionMenu(
            mode_box,
            values=["WireGuard (VPN)", "Raw-IP (no-TLS)"],
            fg_color=COLOR_INPUT,
            button_color=COLOR_BORDER,
            button_hover_color=COLOR_FOX_ORANGE,
            height=34
        )
        self.mode_menu.pack(fill="x", pady=(2, 0))

        # Кнопка включения / выключения (Большая кнопка)
        self.btn_toggle = ctk.CTkButton(
            self,
            text="Подключить туннель",
            height=46,
            fg_color=COLOR_FOX_ORANGE,
            hover_color=COLOR_FOX_HOVER,
            font=ctk.CTkFont(family="Segoe UI", size=15, weight="bold"),
            corner_radius=10,
            command=self._toggle_tunnel
        )
        self.btn_toggle.pack(fill="x", padx=24, pady=10)

        # Консоль логов (Log Box)
        log_frame = ctk.CTkFrame(self, fg_color=COLOR_CARD, corner_radius=12, border_width=1, border_color=COLOR_BORDER)
        log_frame.pack(fill="both", expand=True, padx=24, pady=(4, 16))

        log_header = ctk.CTkFrame(log_frame, fg_color="transparent")
        log_header.pack(fill="x", padx=12, pady=(8, 4))
        ctk.CTkLabel(log_header, text="Журнал событий", font=ctk.CTkFont(family="Segoe UI", size=12, weight="bold"), text_color=COLOR_TEXT_DIM).pack(side="left")

        clear_btn = ctk.CTkButton(log_header, text="Очистить", width=60, height=22, fg_color="transparent", text_color=COLOR_TEXT_DIM, hover_color=COLOR_INPUT, font=ctk.CTkFont(family="Segoe UI", size=11), command=self._clear_logs)
        clear_btn.pack(side="right")

        self.log_textbox = ctk.CTkTextbox(
            log_frame,
            fg_color=COLOR_BG,
            text_color="#CBD5E1",
            font=ctk.CTkFont(family="Consolas", size=11),
            corner_radius=8,
            wrap="word"
        )
        self.log_textbox.pack(fill="both", expand=True, padx=12, pady=(0, 12))

    def _load_config(self):
        if not os.path.exists(CONFIG_FILE):
            return
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#") or "=" not in line:
                        continue
                    k, v = line.split("=", 1)
                    k, v = k.strip(), v.strip()
                    if k == "PEER":
                        self.peer_entry.delete(0, "end")
                        self.peer_entry.insert(0, v)
                    elif k == "PASSWORD":
                        self.pass_entry.delete(0, "end")
                        self.pass_entry.insert(0, v)
                    elif k == "VK_HASH":
                        self.hash_entry.delete(0, "end")
                        self.hash_entry.insert(0, v)
                    elif k == "WORKERS":
                        self.workers_entry.delete(0, "end")
                        self.workers_entry.insert(0, v)
        except Exception as e:
            self._append_log(f"[GUI] Ошибка чтения config.ini: {e}\n")

    def _save_config(self):
        try:
            with open(CONFIG_FILE, "w", encoding="utf-8") as f:
                f.write("# FWDTT Configuration\n")
                f.write(f"PEER={self.peer_entry.get().strip()}\n")
                f.write(f"PASSWORD={self.pass_entry.get().strip()}\n")
                f.write(f"VK_HASH={self.hash_entry.get().strip()}\n")
                f.write(f"WORKERS={self.workers_entry.get().strip()}\n")
        except Exception as e:
            self._append_log(f"[GUI] Ошибка сохранения config.ini: {e}\n")

    def _apply_uri(self):
        uri = self.uri_entry.get().strip()
        if not uri:
            return

        # Парсинг qwdtt://config?...
        if uri.startswith("qwdtt://"):
            try:
                clean_url = "http://" + uri.replace("qwdtt://", "")
                parsed = urllib.parse.urlparse(clean_url)
                params = urllib.parse.parse_qs(parsed.query)

                if "peer" in params:
                    self.peer_entry.delete(0, "end")
                    self.peer_entry.insert(0, params["peer"][0])
                if "pass" in params:
                    self.pass_entry.delete(0, "end")
                    self.pass_entry.insert(0, params["pass"][0])
                elif "password" in params:
                    self.pass_entry.delete(0, "end")
                    self.pass_entry.insert(0, params["password"][0])
                if "hashes" in params:
                    self.hash_entry.delete(0, "end")
                    self.hash_entry.insert(0, params["hashes"][0])
                elif "hash" in params:
                    self.hash_entry.delete(0, "end")
                    self.hash_entry.insert(0, params["hash"][0])
                if "workers" in params:
                    self.workers_entry.delete(0, "end")
                    self.workers_entry.insert(0, params["workers"][0])

                self._append_log("🦊 Конфигурация успешно импортирована из qwdtt:// ссылки!\n")
                self._save_config()
                self.uri_entry.delete(0, "end")
            except Exception as e:
                self._append_log(f"[!] Ошибка разбора qwdtt://: {e}\n")

        # Парсинг wdtt://host:port:wgPort:localPort:password:vkHash
        elif uri.startswith("wdtt://"):
            try:
                parts = uri.replace("wdtt://", "").split(":")
                if len(parts) >= 6:
                    host, dtls_port, _, _, password, vk_hash = parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]
                    self.peer_entry.delete(0, "end")
                    self.peer_entry.insert(0, f"{host}:{dtls_port}")
                    self.pass_entry.delete(0, "end")
                    self.pass_entry.insert(0, password)
                    self.hash_entry.delete(0, "end")
                    self.hash_entry.insert(0, vk_hash)

                    self._append_log("🦊 Конфигурация успешно импортирована из wdtt:// ссылки!\n")
                    self._save_config()
                    self.uri_entry.delete(0, "end")
            except Exception as e:
                self._append_log(f"[!] Ошибка разбора wdtt://: {e}\n")

    def _toggle_tunnel(self):
        if self.is_running:
            self._stop_tunnel()
        else:
            self._start_tunnel()

    def _start_tunnel(self):
        if not os.path.exists(CLIENT_EXE):
            self._append_log(f"[!] Ошибка: файл клиента не найден: {CLIENT_EXE}\n")
            return

        self._save_config()

        peer = self.peer_entry.get().strip()
        pwd = self.pass_entry.get().strip()
        vkhash = self.hash_entry.get().strip()
        workers = self.workers_entry.get().strip() or "18"
        mode_val = self.mode_menu.get()

        mode = "vpn"
        extra_args = []
        if "Raw-IP" in mode_val:
            mode = "rawtun"
            extra_args = ["-notls"]

        cmd = [
            CLIENT_EXE,
            "-mode", mode,
            "-peer", peer,
            "-password", pwd,
            "-vk", vkhash,
            "-n", workers
        ] + extra_args

        self._append_log(f"🦊 Запуск FWDTT туннеля (потоков: {workers})...\n")

        try:
            self.process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                stdin=subprocess.PIPE,
                text=True,
                bufsize=1,
                encoding="utf-8",
                errors="replace",
                creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
            )
            self.is_running = True
            self.status_badge.configure(text="● Защищено", text_color=COLOR_GREEN)
            self.btn_toggle.configure(text="Отключить туннель", fg_color=COLOR_RED, hover_color="#C0392B")

            threading.Thread(target=self._reader_thread, daemon=True).start()
        except Exception as e:
            self._append_log(f"[!] Не удалось запустить процесс: {e}\n")

    def _stop_tunnel(self):
        if self.process:
            self._append_log("\n🦊 Завершение работы туннеля и очистка маршрутов...\n")
            try:
                self.process.terminate()
            except Exception:
                pass
            self.process = None

        self.is_running = False
        self.status_badge.configure(text="● Отключено", text_color=COLOR_TEXT_DIM)
        self.btn_toggle.configure(text="Подключить туннель", fg_color=COLOR_FOX_ORANGE, hover_color=COLOR_FOX_HOVER)

    def _reader_thread(self):
        while self.process and self.process.poll() is None:
            line = self.process.stdout.readline()
            if line:
                self.log_queue.put(line)
        self.log_queue.put("__TUNNEL_STOPPED__")

    def _poll_logs(self):
        while not self.log_queue.empty():
            msg = self.log_queue.get_nowait()
            if msg == "__TUNNEL_STOPPED__":
                self._stop_tunnel()
            else:
                self._append_log(msg)
        self.after(100, self._poll_logs)

    def _append_log(self, text):
        self.log_textbox.insert("end", text)
        self.log_textbox.see("end")

    def _clear_logs(self):
        self.log_textbox.delete("1.0", "end")

if __name__ == "__main__":
    app = FwdttApp()
    app.mainloop()
