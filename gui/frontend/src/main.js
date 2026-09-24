// FWDTT Minimal Controller
import './style.css';

// DOM элементы
const btnPower = document.getElementById('btnPower');
const powerTextLabel = document.getElementById('powerTextLabel');
const summarySecurity = document.getElementById('summarySecurity');
const btnCheckIP = document.getElementById('btnCheckIP');
const uptimeDisplay = document.getElementById('uptimeDisplay');
const errorRow = document.getElementById('errorRow');
const errorText = document.getElementById('errorText');

// Шапка
const serverPill = document.getElementById('serverPill');
const pillServerIP = document.getElementById('pillServerIP');
const pillPing = document.getElementById('pillPing');
const btnOpenSettings = document.getElementById('btnOpenSettings');
const btnOpenLogs = document.getElementById('btnOpenLogs');
const logBadge = document.getElementById('logBadge');
const adminNotice = document.getElementById('adminNotice');
const btnRelaunchAdmin = document.getElementById('btnRelaunchAdmin');

// Fox Sleep Mode
const foxSleepBanner = document.getElementById('foxSleepBanner');
const foxSleepText = document.getElementById('foxSleepText');
const btnWakeupFox = document.getElementById('btnWakeupFox');

// Подвал (статистика)
const lblSpeedDown = document.getElementById('lblSpeedDown');
const lblSpeedUp = document.getElementById('lblSpeedUp');
const lblSessionTraffic = document.getElementById('lblSessionTraffic');
const lblLifetimeTraffic = document.getElementById('lblLifetimeTraffic');
const lblWorkers = document.getElementById('lblWorkers');

// Модалки
const modalSettings = document.getElementById('modalSettings');
const btnCloseSettings = document.getElementById('btnCloseSettings');
const btnSaveConfig = document.getElementById('btnSaveConfig');
const modalLogs = document.getElementById('modalLogs');
const btnCloseLogs = document.getElementById('btnCloseLogs');
const rawLogsConsole = document.getElementById('rawLogsConsole');
const btnCopyRawLogs = document.getElementById('btnCopyRawLogs');
const btnClearRawLogs = document.getElementById('btnClearRawLogs');

// Форма
const inPeer = document.getElementById('inPeer');
const inPassword = document.getElementById('inPassword');
const inVkHash = document.getElementById('inVkHash');
const inMode = document.getElementById('inMode');
const inDNS = document.getElementById('inDNS');
const inWorkers = document.getElementById('inWorkers');
const workersSliderVal = document.getElementById('workersSliderVal');
const inTurnTCP = document.getElementById('inTurnTCP');

// Состояние
let isConnected = false;
let isConnecting = false;
let statsTimer = null;
let logCount = 0;
let triggerBoom = false;
let particlesList = [];

// 1. Пассивный залипательный фон (Ambient Canvas с интерактивным ускорением)
function initAmbientCanvas() {
  const canvas = document.getElementById('ambientCanvas');
  const ctx = canvas.getContext('2d');
  let width, height;

  function resize() {
    width = canvas.width = window.innerWidth;
    height = canvas.height = window.innerHeight;
  }
  window.addEventListener('resize', resize);
  resize();

  const count = 45;
  particlesList = [];
  for (let i = 0; i < count; i++) {
    particlesList.push({
      x: Math.random() * width,
      y: Math.random() * height,
      vx: (Math.random() - 0.5) * 0.3,
      vy: (Math.random() - 0.5) * 0.3,
      r: Math.random() * 2.2 + 0.8,
      alpha: Math.random() * 0.4 + 0.15,
      dAlpha: (Math.random() * 0.008 + 0.002) * (Math.random() > 0.5 ? 1 : -1)
    });
  }

  function draw() {
    ctx.clearRect(0, 0, width, height);

    if (triggerBoom) {
      triggerBoom = false;
      // Взрывной разлёт звёздочек от центра кнопки
      const cx = width / 2;
      const cy = height * 0.45;
      for (let p of particlesList) {
        const dx = p.x - cx;
        const dy = p.y - cy;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        p.vx = (dx / dist) * (Math.random() * 6 + 3);
        p.vy = (dy / dist) * (Math.random() * 6 + 3);
        p.alpha = 1.0;
      }
    }

    for (let p of particlesList) {
      p.x += p.vx;
      p.y += p.vy;
      p.alpha += p.dAlpha;

      // Плавное торможение после разлёта
      if (Math.abs(p.vx) > 0.4) p.vx *= 0.96;
      if (Math.abs(p.vy) > 0.4) p.vy *= 0.96;

      if (p.alpha <= 0.1 || p.alpha >= 0.7) p.dAlpha = -p.dAlpha;
      if (p.x < 0) p.x = width;
      if (p.x > width) p.x = 0;
      if (p.y < 0) p.y = height;
      if (p.y > height) p.y = 0;

      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(56, 189, 248, ${p.alpha * (isConnected ? 1.0 : 0.45)})`;
      ctx.shadowBlur = isConnected ? 14 : 6;
      ctx.shadowColor = isConnected ? '#34d399' : '#38bdf8';
      ctx.fill();
    }
    requestAnimationFrame(draw);
  }
  draw();
}

// 2. Форматирование
function formatBytes(bytes) {
  if (!bytes || bytes <= 0) return '0.0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatSpeed(bytesPerSec) {
  return formatBytes(bytesPerSec) + '/s';
}

function formatUptime(seconds) {
  if (!seconds || seconds <= 0) return '00:00:00';
  const h = Math.floor(seconds / 3600).toString().padStart(2, '0');
  const m = Math.floor((seconds % 3600) / 60).toString().padStart(2, '0');
  const s = Math.floor(seconds % 60).toString().padStart(2, '0');
  return `${h}:${m}:${s}`;
}

// 3. Обработчики интерфейса
function setupUIHandlers() {
  btnPower.addEventListener('click', handlePowerToggle);

  // Настройки
  const openSettings = () => modalSettings.classList.add('open');
  const closeSettings = () => modalSettings.classList.remove('open');
  btnOpenSettings.addEventListener('click', openSettings);
  serverPill.addEventListener('click', openSettings);
  btnCloseSettings.addEventListener('click', closeSettings);
  modalSettings.querySelector('.drawer-backdrop').addEventListener('click', closeSettings);

  // Логи
  const openLogs = () => {
    modalLogs.classList.add('open');
    logBadge.classList.remove('active');
  };
  const closeLogs = () => modalLogs.classList.remove('open');
  btnOpenLogs.addEventListener('click', openLogs);
  btnCloseLogs.addEventListener('click', closeLogs);
  modalLogs.querySelector('.drawer-backdrop').addEventListener('click', closeLogs);

  btnCopyRawLogs.addEventListener('click', () => {
    navigator.clipboard.writeText(rawLogsConsole.innerText);
  });
  btnClearRawLogs.addEventListener('click', () => {
    rawLogsConsole.innerHTML = '';
    logCount = 0;
  });

  // Проверка IP
  btnCheckIP.addEventListener('click', () => {
    summarySecurity.textContent = 'Определение внешнего IP...';
    if (window.go?.main?.App?.CheckIP) {
      window.go.main.App.CheckIP();
    }
  });

  // Права админа
  btnRelaunchAdmin.addEventListener('click', () => {
    if (window.go?.main?.App?.RelaunchAsAdmin) {
      window.go.main.App.RelaunchAsAdmin();
    }
  });

  // Пробуждение лисёнка
  if (btnWakeupFox) {
    btnWakeupFox.addEventListener('click', () => {
      if (window.go?.main?.App?.WakeupFox) {
        window.go.main.App.WakeupFox();
      }
    });
  }

  // Ползунок воркеров
  inWorkers.addEventListener('input', (e) => {
    workersSliderVal.textContent = e.target.value;
  });

  // Сохранить настройки
  btnSaveConfig.addEventListener('click', async () => {
    const cfg = collectFormConfig();
    if (window.go?.main?.App?.SaveConfig) {
      await window.go.main.App.SaveConfig(cfg);
    }
    if (cfg.peer_addr) pillServerIP.textContent = cfg.peer_addr;
    closeSettings();
  });
}

function collectFormConfig() {
  return {
    peer_addr: inPeer.value.trim(),
    password: inPassword.value.trim(),
    vk_hash: inVkHash.value.trim(),
    num_workers: parseInt(inWorkers.value, 10) || 27,
    conn_mode: inMode.value || 'vpn',
    socks_addr: '127.0.0.1:1080',
    go_dns: inDNS.value,
    obfs_mode: 'audio',
    turn_tcp: inTurnTCP.checked,
    auto_connect: false
  };
}

// 4. Подключение и отключение
async function handlePowerToggle() {
  if (isConnecting) return;

  if (isConnected) {
    setDisconnectedUI();
    if (window.go?.main?.App?.Disconnect) {
      await window.go.main.App.Disconnect();
    }
  } else {
    const cfg = collectFormConfig();
    if (!cfg.peer_addr || !cfg.password || !cfg.vk_hash) {
      modalSettings.classList.add('open');
      return;
    }

    setConnectingUI();
    try {
      if (window.go?.main?.App?.Connect) {
        await window.go.main.App.Connect(cfg);
      }
    } catch (err) {
      setDisconnectedUI(err.toString());
    }
  }
}

function setConnectingUI() {
  isConnecting = true;
  isConnected = false;
  document.getElementById('app').className = 'widget-layout state-connecting';
  btnPower.className = 'power-button connecting';
  powerTextLabel.textContent = 'ПОИСК...';
  summarySecurity.textContent = 'Установка туннеля и поиск релеев...';
  errorRow.style.display = 'none';
}

function setConnectedUI(country, ip) {
  const wasNotConnected = !isConnected;
  isConnecting = false;
  isConnected = true;
  document.getElementById('app').className = 'widget-layout state-connected';
  btnPower.className = 'power-button connected';
  powerTextLabel.textContent = 'ПОДКЛЮЧЕНО';

  // Тот самый ВЗРЫВНОЙ ЭФФЕКТ: разлёт частиц и ударная волна!
  if (wasNotConnected) {
    triggerBoom = true;
    const shockwave = document.getElementById('powerShockwave');
    if (shockwave) {
      shockwave.classList.remove('trigger');
      void shockwave.offsetWidth; // сброс reflow
      shockwave.classList.add('trigger');
    }
  }

  if (ip && ip !== '—') {
    const loc = (country && country !== '—') ? ` (${country})` : '';
    summarySecurity.textContent = `Защищено • ${ip}${loc}`;
  } else {
    summarySecurity.textContent = 'Защищено (VK TURN)';
  }
  errorRow.style.display = 'none';
}

function setDisconnectedUI(err) {
  isConnecting = false;
  isConnected = false;
  document.getElementById('app').className = 'widget-layout';
  btnPower.className = 'power-button idle';
  powerTextLabel.textContent = 'ОТКЛЮЧЕНО';
  summarySecurity.textContent = 'Соединение не защищено';

  lblSpeedDown.textContent = '0.0 B/s';
  lblSpeedUp.textContent = '0.0 B/s';

  if (err) {
    errorText.textContent = err;
    errorRow.style.display = 'flex';
  } else {
    errorRow.style.display = 'none';
  }
}

// 5. Опрос состояния
function startStatsLoop() {
  if (statsTimer) clearInterval(statsTimer);
  statsTimer = setInterval(async () => {
    if (!window.go?.main?.App?.GetStats) return;
    try {
      const s = await window.go.main.App.GetStats();
      updateUI(s);
    } catch (e) {}
  }, 1000);
}

function updateUI(s) {
  if (!s) return;

  if (!s.is_admin && inMode.value === 'vpn') {
    adminNotice.classList.add('show');
  } else {
    adminNotice.classList.remove('show');
  }

  if (s.fox_sleeping || s.state === 'sleeping') {
    if (foxSleepBanner) foxSleepBanner.style.display = 'flex';
    if (foxSleepText && s.state_msg) foxSleepText.textContent = s.state_msg;
  } else {
    if (foxSleepBanner) foxSleepBanner.style.display = 'none';
  }

  if (s.state === 'connected') {
    setConnectedUI(s.exit_country, s.exit_ip);
  } else if (s.state === 'connecting') {
    if (!isConnecting) setConnectingUI();
  } else if (s.state === 'idle') {
    if (isConnected || isConnecting) setDisconnectedUI(s.last_error);
  }

  uptimeDisplay.textContent = formatUptime(s.uptime_sec);
  pillPing.textContent = s.ping_ms > 0 ? `${s.ping_ms} ms` : '— ms';
  lblWorkers.textContent = `${s.active_workers} / ${s.total_workers}`;
  lblSpeedDown.textContent = formatSpeed(s.current_down_bps);
  lblSpeedUp.textContent = formatSpeed(s.current_up_bps);
  lblSessionTraffic.textContent = formatBytes(s.session_down + s.session_up);
  lblLifetimeTraffic.textContent = formatBytes(s.lifetime_down + s.lifetime_up);
}

// 6. Логи
function setupLogStream() {
  if (window.runtime?.EventsOn) {
    window.runtime.EventsOn('log_entry', (msg) => {
      appendLog(msg);
    });
  }
}

function appendLog(text) {
  const line = document.createElement('div');
  const clean = text.trim();
  let level = 'info';

  if (clean.includes('[ERROR]') || clean.includes('Ошибка')) level = 'error';
  else if (clean.includes('[WARN]') || clean.includes('warning')) level = 'warn';

  line.className = `log-line ${level}`;
  line.textContent = clean;
  rawLogsConsole.appendChild(line);

  logCount++;
  if (!modalLogs.classList.contains('open')) {
    logBadge.classList.add('active');
  }

  while (rawLogsConsole.children.length > 300) {
    rawLogsConsole.removeChild(rawLogsConsole.firstChild);
  }
  rawLogsConsole.scrollTop = rawLogsConsole.scrollHeight;
}

// Загрузка сохранённого конфига
async function loadSavedConfig() {
  if (!window.go?.main?.App?.LoadConfig) return;
  try {
    const cfg = await window.go.main.App.LoadConfig();
    if (cfg) {
      inPeer.value = cfg.peer_addr || '';
      inPassword.value = cfg.password || '';
      inVkHash.value = cfg.vk_hash || '';
      inMode.value = cfg.conn_mode || 'vpn';
      inDNS.value = cfg.go_dns || 'yandex';
      inWorkers.value = cfg.num_workers || 27;
      workersSliderVal.textContent = inWorkers.value;
      inTurnTCP.checked = !!cfg.turn_tcp;
      if (cfg.peer_addr) pillServerIP.textContent = cfg.peer_addr;
    }
  } catch (e) {}
}

document.addEventListener('DOMContentLoaded', async () => {
  initAmbientCanvas();
  setupUIHandlers();
  await loadSavedConfig();
  startStatsLoop();
  setupLogStream();
});
