// FWDTT Desktop Controller & Self-Doctor
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
const btnOpenDiagnostics = document.getElementById('btnOpenDiagnostics');
const diagBadge = document.getElementById('diagBadge');
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

// Диагностика (Self-Doctor)
const modalDiagnostics = document.getElementById('modalDiagnostics');
const btnCloseDiagnostics = document.getElementById('btnCloseDiagnostics');
const btnRunDiagnostics = document.getElementById('btnRunDiagnostics');
const btnCopyDiagReport = document.getElementById('btnCopyDiagReport');
const diagSummaryCard = document.getElementById('diagSummaryCard');
const diagSummaryIcon = document.getElementById('diagSummaryIcon');
const diagSummaryTitle = document.getElementById('diagSummaryTitle');
const diagSummaryText = document.getElementById('diagSummaryText');
const diagTimestamp = document.getElementById('diagTimestamp');
const diagPipelineContainer = document.getElementById('diagPipelineContainer');

// Форма
const inPeer = document.getElementById('inPeer');
const inPassword = document.getElementById('inPassword');
const inMode = document.getElementById('inMode');
const inDNS = document.getElementById('inDNS');
const inWorkers = document.getElementById('inWorkers');
const workersSliderVal = document.getElementById('workersSliderVal');
const workersCapHint = document.getElementById('workersCapHint');
const inTurnTCP = document.getElementById('inTurnTCP');
const btnCheckAllHashes = document.getElementById('btnCheckAllHashes');

// Слоты хешей (до 4-х шт)
const hashSlots = [0, 1, 2, 3].map(i => ({
  input: document.getElementById(`inVkHash${i}`),
  dot: document.getElementById(`hashDot${i}`),
  hint: document.getElementById(`hashHint${i}`),
  btn: document.getElementById(`btnCheckHash${i}`)
}));

// Состояние
let isConnected = false;
let isConnecting = false;
let statsTimer = null;
let logCount = 0;
let triggerBoom = false;
let particlesList = [];
let latestDiagReport = null;

// 1. Пассивный залипательный фон (Ambient Canvas)
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

// 3. Динамический расчет лимита воркеров (по 27 на хеш)
function getActiveHashesList() {
  return hashSlots
    .map(s => s.input.value.trim())
    .filter(val => val.length > 0);
}

function updateWorkersSliderLimit() {
  const activeCount = Math.max(1, getActiveHashesList().length);
  const maxWorkers = Math.min(108, activeCount * 27);

  inWorkers.max = maxWorkers;
  if (parseInt(inWorkers.value, 10) > maxWorkers) {
    inWorkers.value = maxWorkers;
  }
  workersSliderVal.textContent = inWorkers.value;

  const hashWord = activeCount === 1 ? 'активный хеш' : (activeCount < 5 ? 'активных хеша' : 'хешей');
  if (workersCapHint) {
    workersCapHint.textContent = `Доступно до ${maxWorkers} воркеров (${activeCount} ${hashWord})`;
  }
}

// 4. Проверка хешей звонков
async function checkSingleHash(idx) {
  const slot = hashSlots[idx];
  if (!slot) return;
  const val = slot.input.value.trim();
  if (!val) {
    slot.dot.className = 'slot-dot';
    slot.hint.textContent = 'Слот пуст';
    slot.hint.className = 'slot-hint-text';
    return;
  }

  slot.dot.className = 'slot-dot checking';
  slot.hint.textContent = 'Проверка комнаты звонка...';
  slot.hint.className = 'slot-hint-text';

  if (!window.go?.main?.App?.CheckHash) {
    slot.dot.className = 'slot-dot';
    slot.hint.textContent = 'Wails API не готов';
    return;
  }

  try {
    const res = await window.go.main.App.CheckHash(val);
    applyHashCheckResultToSlot(slot, res);
  } catch (err) {
    slot.dot.className = 'slot-dot expired';
    slot.hint.textContent = 'Ошибка: ' + err.toString();
    slot.hint.className = 'slot-hint-text expired';
  }
  updateWorkersSliderLimit();
}

function applyHashCheckResultToSlot(slot, res) {
  if (!res) return;
  if (res.status === 'ok') {
    slot.dot.className = 'slot-dot ok';
    slot.hint.textContent = `🟢 Активен (${res.turn_count} TURN, ${res.latency_ms} мс)`;
    slot.hint.className = 'slot-hint-text ok';
  } else if (res.status === 'expired') {
    slot.dot.className = 'slot-dot expired';
    slot.hint.textContent = `🔴 Протух / Комната закрыта (${res.error_message || 'завершён'})`;
    slot.hint.className = 'slot-hint-text expired';
  } else if (res.status === 'captcha') {
    slot.dot.className = 'slot-dot warn';
    slot.hint.textContent = '🟡 Требуется капча VK';
    slot.hint.className = 'slot-hint-text warn';
  } else {
    slot.dot.className = 'slot-dot expired';
    slot.hint.textContent = `🔴 ${res.error_message || 'Ошибка'}`;
    slot.hint.className = 'slot-hint-text expired';
  }
}

async function checkAllHashes() {
  btnCheckAllHashes.disabled = true;
  btnCheckAllHashes.textContent = '⏳ Проверка...';

  const nonEmpties = hashSlots
    .map((s, idx) => ({ idx, val: s.input.value.trim() }))
    .filter(item => item.val.length > 0);

  if (nonEmpties.length === 0) {
    btnCheckAllHashes.textContent = 'Заполните слоты';
    setTimeout(() => {
      btnCheckAllHashes.textContent = '🩺 Проверить все';
      btnCheckAllHashes.disabled = false;
    }, 1500);
    return;
  }

  nonEmpties.forEach(item => {
    hashSlots[item.idx].dot.className = 'slot-dot checking';
    hashSlots[item.idx].hint.textContent = 'Тестирование...';
    hashSlots[item.idx].hint.className = 'slot-hint-text';
  });

  try {
    const rawHashes = nonEmpties.map(item => item.val);
    if (window.go?.main?.App?.CheckAllHashes) {
      const results = await window.go.main.App.CheckAllHashes(rawHashes);
      results.forEach((res, i) => {
        const slotIdx = nonEmpties[i].idx;
        applyHashCheckResultToSlot(hashSlots[slotIdx], res);
      });
    }
  } catch (err) {
    console.error('CheckAllHashes error:', err);
  } finally {
    btnCheckAllHashes.textContent = '🩺 Проверить все';
    btnCheckAllHashes.disabled = false;
    updateWorkersSliderLimit();
  }
}

// 5. Селфдоктор / Запуск диагностики
async function runFullDiagnostics() {
  btnRunDiagnostics.disabled = true;
  btnRunDiagnostics.textContent = '⏳ Анализ...';

  diagSummaryCard.className = 'diag-summary-card status-pending';
  diagSummaryIcon.textContent = '⏳';
  diagSummaryTitle.textContent = 'Выполняется самодиагностика...';
  diagSummaryText.textContent = 'Тестируем физическую сеть, DNS, звонки VK, TURN-релеи, VPS и сквозной выход.';
  diagTimestamp.textContent = 'В процессе...';

  // Временный скелетон пайплайна
  diagPipelineContainer.innerHTML = `
    <div class="pipeline-step-card status-running">
      <div class="step-header">
        <div class="step-title-group">
          <span class="step-status-icon">⏳</span>
          <span class="step-title-text">Проверка ключевых сетевых узлов и окружения...</span>
        </div>
      </div>
    </div>
  `;

  if (!window.go?.main?.App?.RunDiagnostics) {
    btnRunDiagnostics.disabled = false;
    btnRunDiagnostics.textContent = '⚡ Проверить';
    return;
  }

  try {
    const report = await window.go.main.App.RunDiagnostics();
    latestDiagReport = report;
    renderDiagReport(report);
  } catch (err) {
    diagSummaryCard.className = 'diag-summary-card status-error';
    diagSummaryIcon.textContent = '🚨';
    diagSummaryTitle.textContent = 'Ошибка выполнения диагностики';
    diagSummaryText.textContent = err.toString();
  } finally {
    btnRunDiagnostics.disabled = false;
    btnRunDiagnostics.textContent = '⚡ Проверить';
  }
}

function renderDiagReport(report) {
  if (!report) return;

  diagSummaryCard.className = `diag-summary-card status-${report.overall_status}`;
  if (report.overall_status === 'ok') {
    diagSummaryIcon.textContent = '🦊✨';
    diagSummaryTitle.textContent = 'Система полностью готова к работе!';
    diagBadge.classList.remove('active');
  } else if (report.overall_status === 'warn') {
    diagSummaryIcon.textContent = '⚠️';
    diagSummaryTitle.textContent = 'Обнаружены предупреждения';
    diagBadge.classList.add('active');
  } else {
    diagSummaryIcon.textContent = '🚨';
    diagSummaryTitle.textContent = 'Обнаружены критические проблемы!';
    diagBadge.classList.add('active');
  }

  diagSummaryText.textContent = report.summary || 'Результаты проверки узлов:';
  diagTimestamp.textContent = report.timestamp || '';

  // Рендерим шаги пайплайна
  diagPipelineContainer.innerHTML = (report.steps || []).map(step => {
    let icon = '⏳';
    if (step.status === 'ok') icon = '🟢';
    else if (step.status === 'warn') icon = '🟡';
    else if (step.status === 'error') icon = '🔴';

    return `
      <div class="pipeline-step-card status-${step.status}">
        <div class="step-header">
          <div class="step-title-group">
            <span class="step-status-icon">${icon}</span>
            <span class="step-title-text">${escapeHTML(step.title)}</span>
          </div>
          ${step.latency_ms > 0 ? `<span class="step-latency-badge">${step.latency_ms} ms</span>` : ''}
        </div>
        <div class="step-body">
          <div class="step-message">${escapeHTML(step.message)}</div>
          ${step.hint ? `
            <div class="step-hint-box">
              <span>💡</span>
              <span>${escapeHTML(step.hint)}</span>
            </div>
          ` : ''}
        </div>
      </div>
    `;
  }).join('');
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/[&<>'"]/g, tag => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    "'": '&#39;',
    '"': '&quot;'
  }[tag] || tag));
}

function copyDiagReportToClipboard() {
  if (!latestDiagReport) {
    navigator.clipboard.writeText('Отчёт диагностики ещё не сформирован. Нажмите «⚡ Проверить».');
    return;
  }

  let text = `🩺 [FWDTT SELF-DOCTOR REPORT — ${latestDiagReport.timestamp}]\n`;
  text += `Статус: ${latestDiagReport.overall_status.toUpperCase()}\n`;
  text += `Резюме: ${latestDiagReport.summary}\n\n`;

  (latestDiagReport.steps || []).forEach((s, idx) => {
    const icon = s.status === 'ok' ? '[OK]' : (s.status === 'warn' ? '[WARN]' : '[ERR]');
    const latency = s.latency_ms > 0 ? ` (${s.latency_ms} ms)` : '';
    text += `${idx + 1}. ${icon} ${s.title}${latency}\n`;
    text += `   Сообщение: ${s.message}\n`;
    if (s.hint) {
      text += `   💡 Подсказка: ${s.hint}\n`;
    }
    text += '\n';
  });

  navigator.clipboard.writeText(text);

  const prevText = btnCopyDiagReport.textContent;
  btnCopyDiagReport.textContent = 'Скопировано! ✓';
  setTimeout(() => {
    btnCopyDiagReport.textContent = prevText;
  }, 1800);
}

// 6. Обработчики интерфейса
function setupUIHandlers() {
  btnPower.addEventListener('click', handlePowerToggle);

  // Настройки
  const openSettings = () => modalSettings.classList.add('open');
  const closeSettings = () => modalSettings.classList.remove('open');
  btnOpenSettings.addEventListener('click', openSettings);
  serverPill.addEventListener('click', openSettings);
  btnCloseSettings.addEventListener('click', closeSettings);
  modalSettings.querySelector('.drawer-backdrop').addEventListener('click', closeSettings);

  // Диагностика (Self-Doctor)
  const openDiagnostics = () => {
    modalDiagnostics.classList.add('open');
    if (!latestDiagReport) {
      runFullDiagnostics();
    }
  };
  const closeDiagnostics = () => modalDiagnostics.classList.remove('open');
  btnOpenDiagnostics.addEventListener('click', openDiagnostics);
  btnCloseDiagnostics.addEventListener('click', closeDiagnostics);
  btnRunDiagnostics.addEventListener('click', runFullDiagnostics);
  btnCopyDiagReport.addEventListener('click', copyDiagReportToClipboard);
  modalDiagnostics.querySelector('.drawer-backdrop').addEventListener('click', closeDiagnostics);

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

  // Проверка внешнего IP
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

  // Слоты хешей: события
  hashSlots.forEach((slot, idx) => {
    slot.input.addEventListener('input', () => {
      slot.dot.className = 'slot-dot';
      slot.hint.textContent = '';
      updateWorkersSliderLimit();
    });
    slot.btn.addEventListener('click', () => checkSingleHash(idx));
  });

  btnCheckAllHashes.addEventListener('click', checkAllHashes);

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
  const hashes = hashSlots.map(s => s.input.value.trim());
  const activeHashes = hashes.filter(h => h.length > 0);

  return {
    peer_addr: inPeer.value.trim(),
    password: inPassword.value.trim(),
    vk_hash: activeHashes.join(','),
    vk_hashes: hashes,
    num_workers: parseInt(inWorkers.value, 10) || 27,
    conn_mode: inMode.value || 'vpn',
    socks_addr: '127.0.0.1:1080',
    go_dns: inDNS.value,
    obfs_mode: 'audio',
    turn_tcp: inTurnTCP.checked,
    auto_connect: false
  };
}

// 7. Подключение и отключение
async function handlePowerToggle() {
  if (isConnecting) return;

  if (isConnected) {
    setDisconnectedUI();
    if (window.go?.main?.App?.Disconnect) {
      await window.go.main.App.Disconnect();
    }
  } else {
    const cfg = collectFormConfig();
    const activeHashes = cfg.vk_hashes.filter(h => h.length > 0);

    if (!cfg.peer_addr || !cfg.password || activeHashes.length === 0) {
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

  // Разлёт частичек и ударная волна
  if (wasNotConnected) {
    triggerBoom = true;
    const shockwave = document.getElementById('powerShockwave');
    if (shockwave) {
      shockwave.classList.remove('trigger');
      void shockwave.offsetWidth;
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

// 8. Опрос состояния
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

// 9. Логи
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

// 10. Загрузка сохранённого конфига
async function loadSavedConfig() {
  if (!window.go?.main?.App?.LoadConfig) return;
  try {
    const cfg = await window.go.main.App.LoadConfig();
    if (cfg) {
      inPeer.value = cfg.peer_addr || '';
      inPassword.value = cfg.password || '';

      // Заполняем слоты хешей
      const hashes = cfg.vk_hashes || [];
      hashSlots.forEach((slot, i) => {
        slot.input.value = hashes[i] || '';
        slot.dot.className = 'slot-dot';
        slot.hint.textContent = '';
      });

      // Если массив слотов был пуст, но есть vk_hash
      if (hashes.length === 0 && cfg.vk_hash) {
        const parts = cfg.vk_hash.split(/[,;\s]+/).filter(Boolean);
        parts.slice(0, 4).forEach((h, i) => {
          if (hashSlots[i]) hashSlots[i].input.value = h;
        });
      }

      inMode.value = cfg.conn_mode || 'vpn';
      inDNS.value = cfg.go_dns || 'yandex';
      inTurnTCP.checked = !!cfg.turn_tcp;
      if (cfg.peer_addr) pillServerIP.textContent = cfg.peer_addr;

      updateWorkersSliderLimit();
      if (cfg.num_workers) {
        inWorkers.value = cfg.num_workers;
        workersSliderVal.textContent = inWorkers.value;
      }
    }
  } catch (e) {
    console.error('loadSavedConfig error:', e);
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  initAmbientCanvas();
  setupUIHandlers();
  await loadSavedConfig();
  startStatsLoop();
  setupLogStream();
});
