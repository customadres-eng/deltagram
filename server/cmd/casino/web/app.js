'use strict';

/* ------------------------------------------------------------------ *
 * Delta Casino Mini App
 * Identity comes from Telegram WebApp launch params. DeltaGram signs them
 * into the URL (query and/or fragment), so we read both and never depend on
 * the external telegram-web-app.js being reachable.
 * ------------------------------------------------------------------ */

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
const app = document.getElementById('app');
const nav = document.getElementById('nav');
const toastEl = document.getElementById('toast');
const sheetRoot = document.getElementById('sheetRoot');

const CRASH_K = 0.28; // must match domain.CrashCurveK

const S = {
  view: 'lobby',
  balance: 0,
  starBalance: 0,
  cps: 100,
  // Operator-configured win multiplier in percent. 100 keeps the designed
  // 95% RTP; the lobby and the cashout labels follow it so the client never
  // promises an amount the server will not pay.
  payoutPct: 100,
  nfts: [],
  history: [],
  historyLoaded: false,
  initData: '',
  source: 'none',
  ready: false,
  authError: '',
  uid: 0,
  platform: '',
};

const G = {
  crash: null,
  mines: null,
  upgrade: { target: 2, mode: 'chips', busy: false, result: null, bet: 1 },
  slots: { busy: false, reels: null, result: null, lastBet: 1 },
};

const SYMBOLS = ['cherry', 'lemon', 'grape', 'orange', 'bell', 'star', 'seven', 'wild'];
const GAME_META = {
  slots: { name: 'Слоты', icon: '#g-crown' },
  crash: { name: 'Краш', icon: '#i-rocket' },
  mines: { name: 'Мины', icon: '#i-bomb' },
  upgrade: { name: 'Апгрейд', icon: '#i-gem' },
};

/* ------------------------------ helpers ------------------------------ */

function esc(v) {
  return String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function cps() { return Number(S.cps) || 100; }
function starsOf(chips) { return Number(chips || 0) / cps(); }
// payoutPct mirrors the operator setting the server applies to winning rounds.
// Defaults to 100 so a session fetched before the field existed behaves like
// the designed economy.
function payoutPct() {
  const v = Number(S.payoutPct);
  return Number.isFinite(v) && v >= 0 ? v : 100;
}
// winOf scales a notional payout by the operator setting. Every "you will get
// X" label goes through it, so the shown amount always equals what the server
// settles.
function winOf(chips) {
  return Math.max(0, Math.floor(Number(chips || 0) * payoutPct() / 100));
}
function rtpText() {
  return 'Отдача ' + Math.round(95 * payoutPct() / 100) + '%';
}
function fmtStars(chips, digits = 2) {
  return starsOf(chips).toLocaleString('ru-RU', { maximumFractionDigits: digits });
}
function fmtNum(n) {
  return Number(n || 0).toLocaleString('ru-RU');
}
function star(cls = '') {
  return '<svg class="star-i' + (cls ? ' ' + cls : '') + '"><use href="#i-star"></use></svg>';
}
function fmtMult(v, digits = 2) {
  return Number(v || 0).toFixed(digits).replace('.', ',');
}
function num(v) {
  const n = parseFloat(String(v == null ? '' : v).replace(',', '.'));
  return Number.isFinite(n) ? n : 0;
}

// Minimum amount, in stars, for every player-entered sum: stake, deposit and
// withdrawal. Percent presets never return less than this.
const MIN_STARS = 100;

// availableStars reports the ceiling for a percentage preset: the wallet's
// star balance for deposits, and the game balance (chips -> stars) for stakes
// and withdrawals.
function availableStars(kind) {
  if (kind === 'deposit') return Math.max(0, Math.floor(Number(S.starBalance) || 0));
  return Math.max(0, Math.floor(starsOf(S.balance)));
}

// pctAmount converts a balance percentage into whole stars. The result is
// clamped to [MIN_STARS, available]; 100% always means the whole available
// amount, so "Макс" really empties the source balance in one tap.
function pctAmount(available, pct) {
  const avail = Math.max(0, Math.floor(Number(available) || 0));
  if (avail <= 0) return MIN_STARS;
  if (pct >= 100) return avail;
  const value = Math.floor(avail * pct / 100);
  return Math.min(avail, Math.max(MIN_STARS, value));
}

// quickPresets is the shared 10/25/40/70/Макс row used by stakes and by both
// wallet sheets. Percentages are computed on click from the live balance, so a
// bet that just landed is reflected immediately.
function quickPresets(target) {
  return [[10, '10%'], [25, '25%'], [40, '40%'], [70, '70%'], [100, 'Макс']]
    .map(([pct, label]) => `<button data-act="quick-pct" data-target="${target}" data-pct="${pct}">${label}</button>`)
    .join('');
}

function plural(n, one, few, many) {
  const a = Math.abs(n) % 100;
  const b = a % 10;
  if (a > 10 && a < 20) return many;
  if (b > 1 && b < 5) return few;
  if (b === 1) return one;
  return many;
}
function starsText(n) {
  return fmtNum(n) + ' ' + plural(n, 'звезда', 'звезды', 'звёзд');
}
function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }
function uid() {
  if (window.crypto && crypto.randomUUID) return crypto.randomUUID();
  return Date.now().toString(36) + Math.random().toString(36).slice(2);
}

let toastTimer = 0;
function toast(msg, kind = '') {
  toastEl.textContent = msg;
  toastEl.className = 'toast show ' + kind;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toastEl.className = 'toast ' + kind; }, 2600);
}

function haptic(kind = 'light') {
  try {
    const tg = window.Telegram && window.Telegram.WebApp;
    if (!tg || !tg.HapticFeedback) return;
    if (kind === 'success' || kind === 'error' || kind === 'warning') {
      tg.HapticFeedback.notificationOccurred(kind);
    } else {
      tg.HapticFeedback.impactOccurred(kind);
    }
  } catch (_) { /* optional */ }
}

function syncBalance() {
  $$('[data-balance]').forEach((el) => { el.textContent = fmtStars(S.balance); });
}

// setBalance animates the change so wins and bets count up or down.
function setBalance(v) {
  const from = S.balance;
  const to = Number(v) || 0;
  S.balance = to;
  animateBalance(from, to);
}
function animateBalance(from, to) {
  const els = $$('[data-balance]');
  if (!els.length) return;
  if (Math.abs(to - from) < 0.005) { els.forEach((el) => { el.textContent = fmtStars(to); }); return; }
  const t0 = performance.now();
  const dur = 520;
  function step(now) {
    const p = Math.min(1, (now - t0) / dur);
    const e = 1 - Math.pow(1 - p, 3);
    const val = from + (to - from) * e;
    els.forEach((el) => { el.textContent = fmtStars(val); });
    if (p < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

/* ------------------------------ transport ------------------------------ */

function parseParams(raw) {
  const out = {};
  if (!raw) return out;
  try { new URLSearchParams(raw).forEach((v, k) => { out[k] = v; }); } catch (_) {}
  return out;
}
function fragmentQuery(hash) {
  if (!hash) return '';
  let h = hash.replace(/^#/, '');
  if (!h) return '';
  const q = h.indexOf('?');
  if (q >= 0) return h.slice(q + 1);
  return h.includes('=') ? h : '';
}
function readLaunch() {
  let data = '';
  let source = 'none';
  try {
    const hash = parseParams(fragmentQuery(location.hash));
    if (hash.tgWebAppData) { data = hash.tgWebAppData; source = 'hash'; }
    if (!data) {
      const query = parseParams(location.search.replace(/^\?/, ''));
      if (query.tgWebAppData) { data = query.tgWebAppData; source = 'query'; }
    }
    const tg = window.Telegram && window.Telegram.WebApp;
    if (!data && tg && tg.initData) { data = tg.initData; source = 'sdk'; }
    if (tg) {
      S.platform = tg.platform || '';
      try { tg.ready(); } catch (_) {}
      try { tg.expand(); } catch (_) {}
    }
  } catch (_) {}
  return { data, source };
}

async function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (S.initData) headers['X-Telegram-Init-Data'] = S.initData;
  const res = await fetch('/api' + path, Object.assign({}, opts, {
    headers: Object.assign(headers, opts.headers || {}),
  }));
  let body = {};
  try { body = await res.json(); } catch (_) {}
  if (!res.ok || body.ok === false) {
    const err = new Error(body.message || body.error || ('HTTP ' + res.status));
    err.code = body.error || ('HTTP_' + res.status);
    throw err;
  }
  return body;
}

/* ------------------------------ boot ------------------------------ */

async function boot() {
  const launch = readLaunch();
  S.initData = launch.data;
  S.source = launch.source;
  if (!S.initData) {
    S.authError = 'В ссылке нет данных авторизации Telegram (tgWebAppData).';
    S.ready = true;
    render();
    return;
  }
  await refreshSession(true);
  loadHistory();
  // The operator can retune payouts at any time; poll the session so a
  // long-open Mini App follows the new setting without a relaunch. Only the
  // percentage is taken, so an active game is never re-rendered by the poll.
  setInterval(refreshPayoutPct, 60000);
}

async function refreshSession(firstLoad = false) {
  try {
    const s = await api('/session');
    S.balance = Number(s.balance) || 0;
    S.starBalance = Number(s.star_balance) || 0;
    S.cps = Number(s.chips_per_star) || 100;
    S.payoutPct = Number.isFinite(Number(s.payout_percent)) ? Number(s.payout_percent) : 100;
    S.nfts = Array.isArray(s.nfts) ? s.nfts : [];
    S.uid = Number(s.user_id) || 0;
    S.authError = '';
    S.ready = true;
    render();
  } catch (e) {
    S.authError = e.message || String(e);
    S.ready = true;
    if (firstLoad) { S.nfts = []; S.balance = 0; }
    render();
  }
}

// refreshPayoutPct polls only the economy knob so a long-open Mini App follows
// an operator retune. It deliberately does not call render(): an active crash
// or mines round must not be rebuilt by a background poll.
async function refreshPayoutPct() {
  if (!S.initData || S.authError) return;
  try {
    const s = await api('/session');
    const pct = Number(s.payout_percent);
    if (Number.isFinite(pct)) S.payoutPct = pct;
  } catch (_) {}
}

/* ------------------------------ shell ------------------------------ */
function topBar() {
  return `<header class="top">
    <div class="brand">
      <div class="brand-mark"><svg><use href="#i-crown"></use></svg></div>
      <div><span class="brand-name">DELTA</span><span class="brand-sub">CASINO</span></div>
    </div>
    <button class="balance-pill" data-act="go" data-view="wallet">
      <svg><use href="#i-star"></use></svg>
      <span data-balance>${fmtStars(S.balance)}</span>
    </button>
  </header>`;
}

function authView() {
  const isBrowser = S.source === 'none';
  return `<div class="auth">
    <div class="brand" style="justify-content:center;margin-bottom:20px">
      <div class="brand-mark" style="width:44px;height:44px"><svg style="width:26px;height:26px"><use href="#i-crown"></use></svg></div>
      <div><span class="brand-name" style="font-size:17px">DELTA</span><span class="brand-sub">CASINO</span></div>
    </div>
    <div class="auth-card">
      <div class="auth-icon"><svg><use href="#i-shield"></use></svg></div>
      <h2>${isBrowser ? 'Откройте казино в DeltaGram' : 'Не удалось войти'}</h2>
      <p>${isBrowser
        ? 'Эта страница работает только внутри DeltaGram: мессенджер подписывает вход и передаёт его казино.'
        : 'Сервер не принял подпись входа. Обычно это значит, что ссылка устарела или открыта не из бота.'}</p>
      <ol>
        <li>Откройте чат <b>@deltagram_casino_bot</b>.</li>
        <li>Нажмите кнопку <b>«Казино»</b> внизу чата.</li>
        <li>Если не помогло — закройте мини-приложение и откройте заново.</li>
      </ol>
      ${S.authError ? `<div class="reason">${esc(S.authError)}</div>` : ''}
      <button class="btn gold" data-act="retry">Повторить вход</button>
      <div class="debug">источник входа: ${esc(S.source)}${S.platform ? ' · ' + esc(S.platform) : ''}<br>${esc(location.href.slice(0, 180))}</div>
    </div>
  </div>`;
}

const NAV_FOR = { lobby: 'lobby', wallet: 'wallet', nft: 'nft', history: 'history', crash: 'lobby', mines: 'lobby', upgrade: 'lobby', slots: 'lobby' };

function render() {
  if (!S.ready) return;
  if (S.authError) {
    app.innerHTML = authView();
    nav.style.display = 'none';
    return;
  }
  nav.style.display = '';
  let body = '';
  switch (S.view) {
    case 'wallet': body = walletView(); break;
    case 'nft': body = nftView(); break;
    case 'history': body = historyView(); break;
    case 'crash': body = crashView(); break;
    case 'mines': body = minesView(); break;
    case 'upgrade': body = upgradeView(); break;
    case 'slots': body = slotsView(); break;
    default: body = lobbyView();
  }
  app.innerHTML = topBar() + `<div class="screen-in">${body}</div>`;
  const active = NAV_FOR[S.view] || '';
  $$('.nav-btn').forEach((b) => b.classList.toggle('active', b.dataset.nav === active));
  bindView();
}

function go(view) {
  if (G.crash) {
    if (G.crash.raf) { cancelAnimationFrame(G.crash.raf); G.crash.raf = 0; }
    if (G.crash.poll) { clearTimeout(G.crash.poll); G.crash.poll = 0; }
  }
  closeSheet();
  S.view = view;
  render();
  window.scrollTo(0, 0);
  if (view === 'history') loadHistory();
}

/* ------------------------------ lobby ------------------------------ */

function heroArt() {
  return `<div class="hero-art"><svg viewBox="0 0 130 130">
    <defs>
      <linearGradient id="heroD" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#f6e2a6"/><stop offset=".55" stop-color="#dcb266"/><stop offset="1" stop-color="#9c7228"/>
      </linearGradient>
      <linearGradient id="heroV" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#b4a0ff"/><stop offset="1" stop-color="#6d4ee0"/>
      </linearGradient>
    </defs>
    <circle cx="72" cy="66" r="48" fill="none" stroke="rgba(233,199,124,.18)" stroke-width="1"/>
    <circle cx="72" cy="66" r="36" fill="none" stroke="rgba(233,199,124,.1)" stroke-width="1" stroke-dasharray="3 6"/>
    <path d="M72 22l36 36-36 50-36-50z" fill="url(#heroD)" opacity=".95"/>
    <path d="M72 34l25 25-25 36-25-36z" fill="#0f1219" opacity=".35"/>
    <path d="M36 88l18 18-18 14-18-14z" fill="url(#heroV)" opacity=".9"/>
    <path d="M104 30l12 12-12 16-12-16z" fill="#fff" opacity=".16"/>
    <circle cx="72" cy="66" r="6" fill="#fff" opacity=".4"/>
  </svg></div>`;
}

function heroParticles() {
  const colors = ['#e9c77c', '#f6e2a6', '#fff'];
  let html = '';
  for (let i = 0; i < 9; i++) {
    const left = 8 + Math.random() * 84;
    const delay = Math.random() * 5;
    const dur = 3.5 + Math.random() * 3;
    const c = colors[i % 3];
    html += `<i class="hero-particle" style="left:${left}%;bottom:-10px;animation-delay:${delay}s;animation-duration:${dur}s;background:${c}"></i>`;
  }
  return html;
}

function gameArt(kind) {
  if (kind === 'crash') {
    return `<svg viewBox="0 0 170 96" preserveAspectRatio="none">
      <defs><linearGradient id="gcr" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#45a8ff"/><stop offset="1" stop-color="#9fd0ff"/></linearGradient></defs>
      <g stroke="rgba(255,255,255,.06)" stroke-width="1">
        <path d="M10 76h150M10 56h150M10 36h150M10 16h150"/>
        <path d="M34 12v68M66 12v68M98 12v68M130 12v68"/>
      </g>
      <path d="M8 82C34 79 52 72 70 58s34-28 58-36" fill="none" stroke="url(#gcr)" stroke-width="3" stroke-linecap="round"/>
      <path d="M8 82C34 79 52 72 70 58s34-28 58-36v44H8z" fill="rgba(69,168,255,.13)"/>
      <g transform="translate(112 12) rotate(34)">
        <path d="M0 14c0-7 4-12 10-12s10 5 10 12l-3 9H3z" fill="#e6eefb"/>
        <circle cx="10" cy="10" r="4" fill="#45a8ff"/>
        <path d="M5 23l-3 8 7-3zM15 23l3 8-7-3z" fill="#8fa6c4"/>
        <path d="M8 25h4l-1 7h-2z" fill="#ffb03a"/>
      </g>
      <circle cx="70" cy="58" r="3.5" fill="#9fd0ff"/>
      <circle cx="70" cy="58" r="9" fill="none" stroke="rgba(69,168,255,.4)" stroke-width="1.5"/>
    </svg>`;
  }
  if (kind === 'mines') {
    return `<svg viewBox="0 0 170 96">
      <circle cx="146" cy="34" r="34" fill="rgba(34,211,138,.09)"/>
      <g transform="translate(13 6)">
        ${gridCells(5, 3, 24, 6, '#161b28', 'rgba(255,255,255,.06)')}
        <use href="#i-gem" x="31" y="0" width="24" height="24"/>
        <use href="#i-gem" x="91" y="0" width="24" height="24"/>
        <use href="#i-bomb" x="1" y="30" width="24" height="24"/>
        <use href="#i-gem" x="121" y="30" width="24" height="24"/>
        <use href="#i-gem" x="61" y="60" width="24" height="24"/>
      </g>
    </svg>`;
  }
  if (kind === 'upgrade') {
    return `<svg viewBox="0 0 170 96">
      <circle cx="85" cy="48" r="34" fill="none" stroke="rgba(139,108,255,.2)" stroke-width="2" stroke-dasharray="4 7"/>
      <circle cx="85" cy="48" r="24" fill="rgba(139,108,255,.13)"/>
      <path d="M85 20l22 22-22 34-22-34z" fill="#8b6cff"/>
      <path d="M85 29l14 14-14 22-14-22z" fill="#b5a0ff" opacity=".8"/>
      <path d="M85 42v24M77 50l8-8 8 8" stroke="#fff" stroke-width="3" fill="none" stroke-linecap="round" stroke-linejoin="round" opacity=".85"/>
      <circle cx="42" cy="30" r="2" fill="#fff" opacity=".35"/>
      <circle cx="130" cy="64" r="2.6" fill="#fff" opacity=".25"/>
    </svg>`;
  }
  return `<svg viewBox="0 0 170 96" preserveAspectRatio="none">
    <defs>
      <linearGradient id="gateSky" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#3a2a68"/><stop offset="1" stop-color="#120f22"/>
      </linearGradient>
      <linearGradient id="gateGold" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#fff3c4"/><stop offset="1" stop-color="#b8892f"/>
      </linearGradient>
    </defs>
    <rect width="170" height="96" fill="url(#gateSky)"/>
    <g opacity=".45" fill="#fff">
      <circle cx="24" cy="18" r="1"/><circle cx="62" cy="10" r=".8"/><circle cx="96" cy="20" r="1.1"/>
      <circle cx="132" cy="12" r=".9"/><circle cx="150" cy="28" r=".8"/><circle cx="44" cy="30" r=".7"/>
    </g>
    <path d="M18 88V34l9-8 9 8v54M134 88V34l9-8 9 8v54" fill="none" stroke="url(#gateGold)" stroke-width="3" opacity=".85"/>
    <path d="M36 88V26h98v62" fill="none" stroke="url(#gateGold)" stroke-width="2.4" opacity=".7"/>
    <path d="M36 26L85 10l49 16" fill="none" stroke="url(#gateGold)" stroke-width="2.4" opacity=".85"/>
    <path d="M52 88V40M74 88V40M96 88V40M118 88V40" stroke="url(#gateGold)" stroke-width="5" opacity=".28"/>
    <use href="#g-gem-purple" x="46" y="46" width="17" height="17"/>
    <use href="#g-gem-blue" x="72" y="46" width="17" height="17"/>
    <use href="#g-gem-green" x="98" y="46" width="17" height="17"/>
    <use href="#g-crown" x="66" y="66" width="18" height="18"/>
    <use href="#g-gem-red" x="92" y="66" width="18" height="18"/>
    <circle cx="126" cy="70" r="15" fill="rgba(242,200,94,.18)"/>
    <use href="#g-orb" x="112" y="56" width="28" height="28"/>
  </svg>`;
}

function gridCells(cols, rows, size, gap, fill, stroke) {
  let out = '';
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      out += `<rect x="${c * (size + gap)}" y="${r * (size + gap)}" width="${size}" height="${size}" rx="7" fill="${fill}" stroke="${stroke}" stroke-width="1"/>`;
    }
  }
  return out;
}

// tickerItems renders the "live wins" strip. With no history it shows a few
// house wins so the lobby never looks empty.
function tickerItems() {
  const names = ['KryptoKing', 'Lucky_7', 'NovaStar', 'ZeusFan', 'MisterX', 'GoldRush', 'Astra', 'Vega'];
  const games = ['Краш', 'Мины', 'Апгрейд', 'Слоты'];
  let items = [];
  if (S.history.length) {
    items = S.history.slice(0, 8).filter((r) => r.status === 'won').map((r) => ({
      name: names[(r.round_id || 0) % names.length],
      game: (GAME_META[r.game] || {}).name || 'Игра',
      amount: fmtStars(r.payout),
    }));
  }
  for (let i = items.length; i < 6; i++) {
    items.push({
      name: names[(i * 3 + 1) % names.length],
      game: games[i % games.length],
      amount: (1 + ((i * 37) % 40)) + ',' + ((i * 7) % 10),
    });
  }
  return items.map((it) => `<span class="tick"><b>${esc(it.name)}</b> выиграл <em>${it.amount}</em> ${star('sm')} в «${esc(it.game)}»</span>`).join('');
}

// bestWin returns the largest payout in the loaded history, in chips.
function bestWin() {
  let best = 0;
  S.history.forEach((r) => { if (Number(r.payout) > best) best = Number(r.payout); });
  return best;
}

function lobbyView() {
  const recent = S.history.slice(0, 3);
  const games = [
    { key: 'crash', name: 'Краш', desc: 'Ракета × множитель', rtp: rtpText(), tag: 'ТОП' },
    { key: 'mines', name: 'Мины', desc: 'Поле 5×5', rtp: rtpText() },
    { key: 'upgrade', name: 'Апгрейд', desc: 'Колесо × множитель', rtp: rtpText(), tag: 'ХИТ' },
    { key: 'slots', name: 'Слоты', desc: 'Поле 6×5', rtp: rtpText(), tag: 'NEW' },
  ];
  return `
  <section class="hero">
    <div class="hero-kicker"><svg class="ic"><use href="#i-shield"></use></svg>ЧЕСТНАЯ ИГРА</div>
    <h1>DELTA<br>CASINO</h1>
    <p>Четыре игры на звёзды и настоящие NFT-подарки. Каждый раунд проверяется по открытому ключу.</p>
    <button class="btn gold" data-act="scroll-games">Начать играть <svg class="ic" style="width:16px;height:16px"><use href="#i-arrow-right"></use></svg></button>
    ${heroArt()}
    ${heroParticles()}
  </section>

  <div class="jackpot">
    <div class="jp-label"><svg class="ic"><use href="#i-trophy"></use></svg>ВАШ РЕКОРД</div>
    <div class="jp-value">${fmtStars(bestWin())} <span>${star('lg')}</span></div>
    <div class="jp-sub">${bestWin() > 0 ? 'лучший выигрыш за всё время' : 'выиграйте первый раунд, чтобы открыть счёт'}</div>
  </div>

  <div class="quick-actions">
    <button class="quick-action dep" data-act="open-deposit"><svg class="ic"><use href="#i-arrow-down"></use></svg>Пополнить</button>
    <button class="quick-action wd" data-act="open-withdraw"><svg class="ic"><use href="#i-arrow-up"></use></svg>Вывести</button>
  </div>

  <div class="stats">
    <div class="stat"><div class="v gold" data-balance>${fmtStars(S.balance)}</div><div class="k">Баланс</div></div>
    <div class="stat"><div class="v">${S.nfts.length}</div><div class="k">NFT</div></div>
    <div class="stat"><div class="v">4</div><div class="k">Игры</div></div>
  </div>

  <div class="sec" id="games"><h2>Игры</h2><span class="link">${rtpText()}</span></div>
  <div class="games">
    ${games.map((g) => `
      <button class="game game-${g.key}" data-act="go" data-view="${g.key}">
        <div class="game-art"><span class="rtp-chip">${g.rtp}</span>${g.tag ? `<span class="game-tag">${g.tag}</span>` : ''}${gameArt(g.key)}</div>
        <div class="game-body">
          <div><div class="game-name">${g.name}</div><div class="game-desc">${g.desc}</div></div>
          <span class="game-go"><svg class="ic"><use href="#i-arrow-right"></use></svg></span>
        </div>
      </button>`).join('')}
  </div>

  <div class="ticker"><div class="ticker-track">${tickerItems()}</div></div>

  <div class="sec"><h2>Последние игры</h2><span class="link" data-act="go" data-view="history">Все <svg class="ic"><use href="#i-arrow-right"></use></svg></span></div>
  <div class="hist card">${recent.length ? recent.map(histRow).join('') : `<div class="empty" style="padding:22px"><svg><use href="#i-clock"></use></svg>Сыграйте первый раунд — история появится здесь.</div>`}</div>

  <div class="notice" style="margin:14px 16px 0"><svg class="ic"><use href="#i-shield"></use></svg><div>Каждый раунд использует серверный ключ и ключ игрока. После расчёта серверный ключ раскрывается — результат можно проверить.</div></div>`;
}

/* ------------------------------ wallet & sheets ------------------------------ */

function screenHead(title, rtp, back = 'lobby') {
  return `<div class="screen-head">
    <button class="screen-back" data-act="go" data-view="${back}"><svg class="ic"><use href="#i-chevron-left"></use></svg></button>
    <h1>${title}</h1>
    ${rtp ? `<span class="rtp">${rtp}</span>` : ''}
  </div>`;
}

function betBox(inputId, value, label = 'Ставка') {
  const avail = availableStars('bet');
  const floor = Math.min(MIN_STARS, avail);
  const v = Math.min(Math.max(Math.round(num(value)) || floor, floor), Math.max(avail, floor));
  return `<div class="bet-label"><label>${label}</label><span class="avail">Доступно <b data-balance>${fmtStars(S.balance)}</b> ${star('sm')}</span></div>
  <div class="stepper">
    <button class="step" data-act="bet-step" data-target="${inputId}" data-dir="-1"><svg class="ic"><use href="#i-minus"></use></svg></button>
    <div class="field"><svg><use href="#i-star"></use></svg><input id="${inputId}" inputmode="decimal" autocomplete="off" value="${v}"></div>
    <button class="step" data-act="bet-step" data-target="${inputId}" data-dir="1"><svg class="ic"><use href="#i-plus"></use></svg></button>
  </div>
  <div class="quick">${quickPresets(inputId)}</div>`;
}

function walletView() {
  const withdrawMax = Math.floor(starsOf(S.balance));
  return `${screenHead('Кошелёк', '', 'lobby')}
  <div class="balance-card">
    <div class="lb">Игровой баланс</div>
    <div class="row"><svg class="star"><use href="#i-star"></use></svg><span class="big" data-balance>${fmtStars(S.balance)}</span></div>
    <div class="sub">Это <b>${fmtNum(S.balance)}</b> фишек · 1 ${star('sm')} = ${cps()} фишек</div>
  </div>

  <div class="wallet-actions">
    <button class="wallet-action dep" data-act="open-deposit"><svg class="ic"><use href="#i-arrow-down"></use></svg><span>Пополнить</span></button>
    <button class="wallet-action wd" data-act="open-withdraw"><svg class="ic"><use href="#i-arrow-up"></use></svg><span>Вывести</span></button>
  </div>

  <div class="card" style="margin:14px 16px 0">
    <div class="sheet-info" style="margin:0">
      <div class="row"><span>Звёзды DeltaGram</span><b>${fmtNum(S.starBalance)}</b></div>
      <div class="row"><span>Доступно к выводу</span><b>${fmtNum(withdrawMax)}</b></div>
      <div class="row"><span>Курс</span><b>1 ⭐ = ${cps()} фишек</b></div>
    </div>
  </div>

  <div class="rate"><svg><use href="#i-star"></use></svg>Курс фиксированный: 100 фишек = 1 ${star('sm')}</div>`;
}

function openSheet(html) {
  sheetRoot.innerHTML = `<div class="sheet-overlay" data-act="sheet-overlay">
    <div class="sheet">${html}</div>
  </div>`;
}
function closeSheet() {
  sheetRoot.innerHTML = '';
}

function depositSheet() {
  const avail = availableStars('deposit');
  const enough = avail >= MIN_STARS;
  const start = Math.min(MIN_STARS, avail);
  return `<div class="sheet-grab"></div>
  <div class="sheet-head"><h2>Пополнение звёздами</h2><button class="sheet-close" data-act="sheet-close"><svg class="ic"><use href="#i-close"></use></svg></button></div>
  <div class="sheet-bal">На балансе DeltaGram: <b>${fmtNum(S.starBalance)}</b> ${star('sm')}</div>
  <div class="sheet-amount"><svg><use href="#i-star"></use></svg><input id="depAmount" inputmode="numeric" autocomplete="off" value="${start}"><span class="cur">⭐</span></div>
  <div class="sheet-convert">Вы получите <b id="depConv">${fmtNum(start * cps())}</b> фишек</div>
  <div class="sheet-presets">${quickPresets('depAmount')}</div>
  <div class="sheet-info">
    <div class="row"><span>Курс обмена</span><b>1 ⭐ = ${cps()} фишек</b></div>
    <div class="row"><span>Минимум</span><b>${MIN_STARS} ⭐</b></div>
    <div class="row"><span>Комиссия</span><b>0%</b></div>
  </div>
  <button class="btn green" id="depBtn" data-act="deposit" style="margin-top:16px"${enough ? '' : ' disabled'}>Пополнить</button>
  ${enough ? '' : `<div class="sheet-hint">Для пополнения нужно минимум ${MIN_STARS} ⭐ на балансе DeltaGram.</div>`}`;
}

function withdrawSheet() {
  const max = availableStars('withdraw');
  const enough = max >= MIN_STARS;
  const start = Math.min(MIN_STARS, max);
  return `<div class="sheet-grab"></div>
  <div class="sheet-head"><h2>Вывод звёзд</h2><button class="sheet-close" data-act="sheet-close"><svg class="ic"><use href="#i-close"></use></svg></button></div>
  <div class="sheet-bal">Доступно к выводу: <b>${fmtNum(max)}</b> ${star('sm')}</div>
  <div class="sheet-amount"><svg><use href="#i-star"></use></svg><input id="wdAmount" inputmode="numeric" autocomplete="off" value="${start}"><span class="cur">⭐</span></div>
  <div class="sheet-convert">Спишется <b id="wdConv">${fmtNum(start * cps())}</b> фишек</div>
  <div class="sheet-presets">${quickPresets('wdAmount')}</div>
  <div class="sheet-info">
    <div class="row"><span>Вывод</span><b>целыми звёздами</b></div>
    <div class="row"><span>Минимум</span><b>${MIN_STARS} ⭐</b></div>
    <div class="row"><span>Комиссия</span><b>0%</b></div>
  </div>
  <button class="btn ghost" id="wdBtn" data-act="withdraw" style="margin-top:16px"${enough ? '' : ' disabled'}>Вывести</button>
  ${enough ? '' : `<div class="sheet-hint">Для вывода нужно минимум ${MIN_STARS} ⭐ (${fmtNum(MIN_STARS * cps())} фишек) на игровом балансе.</div>`}`;
}

function updateSheetConv() {
  const dep = $('#depAmount');
  if (dep) {
    const v = Math.floor(num(dep.value));
    const conv = $('#depConv');
    if (conv) conv.textContent = fmtNum(Math.max(0, v) * cps());
  }
  const wd = $('#wdAmount');
  if (wd) {
    const v = Math.floor(num(wd.value));
    const conv = $('#wdConv');
    if (conv) conv.textContent = fmtNum(Math.max(0, v) * cps());
  }
}

function sheetSuccess(title, sub) {
  sheetRoot.innerHTML = `<div class="sheet-overlay">
    <div class="sheet">
      <div class="sheet-success">
        <div class="ring"><svg><use href="#i-check"></use></svg></div>
        <h3>${title}</h3>
        <p>${sub}</p>
      </div>
    </div>
  </div>`;
  setTimeout(closeSheet, 1400);
}

async function doDeposit() {
  const el = $('#depAmount');
  const stars = Math.floor(num(el ? el.value : 0));
  if (stars < MIN_STARS) return toast('Минимальное пополнение — ' + MIN_STARS + ' ⭐', 'err');
  if (stars > S.starBalance) return toast('На балансе DeltaGram не хватает звёзд', 'err');
  const btn = $('#depBtn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spin-loader"></span> Зачисляем…'; }
  try {
    const r = await api('/deposit/stars', { method: 'POST', body: JSON.stringify({ stars, key: uid() }) });
    S.starBalance -= stars;
    setBalance(Number(r.balance) || 0);
    haptic('success');
    sheetSuccess('Зачислено ' + starsText(stars), fmtNum(stars * cps()) + ' фишек на игровой баланс');
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; btn.textContent = 'Пополнить'; }
  }
}

async function doWithdraw() {
  const el = $('#wdAmount');
  const stars = Math.floor(num(el ? el.value : 0));
  if (stars < MIN_STARS) return toast('Минимальный вывод — ' + MIN_STARS + ' ⭐', 'err');
  if (stars * cps() > S.balance) return toast('Недостаточно фишек для вывода', 'err');
  const btn = $('#wdBtn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spin-loader"></span> Выводим…'; }
  try {
    const r = await api('/withdraw/stars', { method: 'POST', body: JSON.stringify({ stars, key: uid() }) });
    S.starBalance += stars;
    setBalance(Number(r.balance) || 0);
    haptic('success');
    sheetSuccess('Выведено ' + starsText(stars), 'Звёзды вернулись на баланс DeltaGram');
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; btn.textContent = 'Вывести'; }
  }
}

/* ------------------------------ NFT ------------------------------ */

// NFT artwork: the casino draws the actual gift, not a placeholder. The
// server hands back the same Lottie model + pattern DeltaGram plays, plus the
// collectible backdrop palette, and lottie-web renders it on the card.
const nftArtCache = new Map();

function hexColor(v) {
  const n = Number(v) || 0;
  return '#' + ((n & 0xffffff).toString(16).padStart(6, '0'));
}

function nftArtEl(rowID) {
  return document.querySelector(`[data-nft-art="${rowID}"]`);
}

function paintNftArt(rowID, art) {
  const host = nftArtEl(rowID);
  if (!host || !art) return;
  host.style.background = `radial-gradient(120% 90% at 50% 12%, ${hexColor(art.center)} 0%, ${hexColor(art.edge)} 72%)`;
  host.style.boxShadow = `inset 0 0 60px ${hexColor(art.pattern_color)}44`;
  if (!window.lottie) return;
  host.innerHTML = '';
  const stage = document.createElement('div');
  stage.className = 'nft-lottie';
  host.appendChild(stage);
  try {
    if (art.pattern && art.pattern.v) {
      const back = document.createElement('div');
      back.className = 'nft-lottie-back';
      stage.appendChild(back);
      window.lottie.loadAnimation({
        container: back, renderer: 'svg', loop: true, autoplay: true, animationData: art.pattern,
      });
    }
    if (art.model && art.model.v) {
      const front = document.createElement('div');
      front.className = 'nft-lottie-front';
      stage.appendChild(front);
      window.lottie.loadAnimation({
        container: front, renderer: 'svg', loop: true, autoplay: true, animationData: art.model,
      });
    }
  } catch (_) { /* a malformed animation must never break the inventory */ }
}

// paintNftWin renders the just-won gift inside the result panel so the player
// sees the actual artwork the casino is handing over, not a text line.
async function paintNftWin(rowID) {
  const host = document.querySelector(`[data-nft-win="${rowID}"] .nft-win-stage`);
  if (!host) return;
  let art = nftArtCache.get(rowID);
  if (!art) {
    try { art = await api('/nft/' + rowID + '/art'); nftArtCache.set(rowID, art); } catch (_) { return; }
  }
  host.style.background = `radial-gradient(120% 90% at 50% 12%, ${hexColor(art.center)} 0%, ${hexColor(art.edge)} 72%)`;
  host.style.boxShadow = `inset 0 0 60px ${hexColor(art.pattern_color)}44`;
  if (!window.lottie) return;
  host.innerHTML = '';
  const stage = document.createElement('div');
  stage.className = 'nft-lottie';
  host.appendChild(stage);
  try {
    if (art.pattern && art.pattern.v) {
      const back = document.createElement('div');
      back.className = 'nft-lottie-back';
      stage.appendChild(back);
      window.lottie.loadAnimation({ container: back, renderer: 'svg', loop: true, autoplay: true, animationData: art.pattern });
    }
    if (art.model && art.model.v) {
      const front = document.createElement('div');
      front.className = 'nft-lottie-front';
      stage.appendChild(front);
      window.lottie.loadAnimation({ container: front, renderer: 'svg', loop: true, autoplay: true, animationData: art.model });
    }
  } catch (_) {}
}

async function loadNftArt(rowID) {
  if (nftArtCache.has(rowID)) { paintNftArt(rowID, nftArtCache.get(rowID)); return; }
  const host = nftArtEl(rowID);
  if (host) host.classList.add('loading');
  try {
    const art = await api('/nft/' + rowID + '/art');
    nftArtCache.set(rowID, art);
    paintNftArt(rowID, art);
  } catch (_) {
    /* keep the fallback glyph */
  } finally {
    if (host) host.classList.remove('loading');
  }
}

function nftView() {
  return `${screenHead('NFT-инвентарь', '', 'lobby')}
  <div class="sec" style="margin-bottom:12px"><h2>Ваши NFT</h2><span class="link" data-act="refresh-nft"><svg class="ic"><use href="#i-refresh"></use></svg>Обновить</span></div>
  ${S.nfts.length ? `<div class="nft-grid">${S.nfts.map((n) => {
    const title = n.title || 'NFT';
    const num = n.num ? '#' + n.num : '#' + n.row_id;
    return `<div class="nft">
      <div class="nft-art" data-nft-art="${n.row_id}">
        <svg class="nft-fallback"><use href="#i-gem"></use></svg>
      </div>
      <div class="nft-info">
        <div class="nft-title">${esc(title)}</div>
        <div class="nft-num">${esc(num)}</div>
        <button class="btn ghost" data-act="withdraw-nft" data-id="${n.row_id}">Вывести в профиль</button>
      </div>
    </div>`;
  }).join('')}</div>` : `<div class="card empty" style="margin:0 16px"><svg><use href="#i-gift"></use></svg>Пока нет NFT.<br>Выиграйте подарок в режиме «NFT-приз».</div>`}
  <div class="notice" style="margin:14px 16px 0"><svg class="ic"><use href="#i-gift"></use></svg><div>Выигранные NFT хранятся у казино. Нажмите «Вывести в профиль», чтобы вернуть подарок в DeltaGram.</div></div>
  <div class="notice" style="margin:10px 16px 0"><svg class="ic"><use href="#i-shield"></use></svg><div>В режиме «NFT-приз» победа выдаёт настоящий подарок из банка казино. Если подходящего NFT нет — приз придёт фишками.</div></div>`;
}

async function withdrawNft(id) {
  try {
    await api('/withdraw/nft', { method: 'POST', body: JSON.stringify({ gift_row_id: id }) });
    S.nfts = S.nfts.filter((n) => Number(n.row_id) !== Number(id));
    haptic('success');
    toast('NFT возвращён в профиль', 'ok');
    render();
  } catch (e) {
    toast(e.message, 'err');
  }
}

async function refreshNft() {
  try {
    const s = await api('/session');
    S.nfts = Array.isArray(s.nfts) ? s.nfts : [];
    S.balance = Number(s.balance) || 0;
    S.starBalance = Number(s.star_balance) || 0;
    render();
    toast('Инвентарь обновлён', 'ok');
  } catch (e) {
    toast(e.message, 'err');
  }
}

/* ------------------------------ history ------------------------------ */

function relTime(ts) {
  if (!ts) return '';
  const t = new Date(ts);
  if (Number.isNaN(t.getTime())) return '';
  const diff = Math.max(0, (Date.now() - t.getTime()) / 1000);
  if (diff < 60) return 'только что';
  if (diff < 3600) return Math.floor(diff / 60) + ' мин назад';
  if (diff < 86400) return Math.floor(diff / 3600) + ' ч назад';
  return t.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}

function histRow(r) {
  const meta = GAME_META[r.game] || { name: r.game || 'Игра', icon: '#i-bolt' };
  const won = r.status === 'won';
  const lost = r.status === 'lost';
  const active = r.status === 'active';
  const amt = won ? '+' + fmtStars(r.payout) : (lost ? '−' + fmtStars(r.stake) : '—');
  const cls = won ? 'green' : (lost ? 'red' : 'muted');
  const mult = r.multiplier > 0 ? '×' + fmtMult(r.multiplier) : (active ? 'в игре' : '—');
  return `<div class="hist-row">
    <div class="hist-icon"><svg><use href="${meta.icon}"></use></svg></div>
    <div class="hist-main">
      <div class="hist-name">${esc(meta.name)}</div>
      <div class="hist-time">ставка ${fmtStars(r.stake)} ${star('sm')} · раунд #${r.round_id}</div>
    </div>
    <div class="hist-amt"><div class="a ${cls}">${amt}</div><div class="m">${mult}</div></div>
  </div>`;
}

function historyView() {
  let content;
  if (!S.historyLoaded) {
    content = `<div class="empty"><div class="spin-loader" style="margin:0 auto 12px"></div>Загружаем историю…</div>`;
  } else if (!S.history.length) {
    content = `<div class="empty"><svg><use href="#i-clock"></use></svg>История пока пуста.</div>`;
  } else {
    content = S.history.map(histRow).join('');
  }
  return `${screenHead('История игр', '', 'lobby')}
  <div class="sec"><h2>Раунды</h2><span class="link" data-act="refresh-history"><svg class="ic"><use href="#i-refresh"></use></svg>Обновить</span></div>
  <div class="hist card" style="margin:0 16px">${content}</div>`;
}

async function loadHistory(force = false) {
  if (S.historyLoaded && !force) return;
  try {
    const r = await api('/rounds?limit=40');
    S.history = Array.isArray(r.rounds) ? r.rounds : [];
    S.historyLoaded = true;
    if (S.view === 'history' || S.view === 'lobby') render();
  } catch (e) {
    S.historyLoaded = true;
    if (S.view === 'history') render();
  }
}

/* ------------------------------ input binding ------------------------------ */

function bindView() {
  const up = $('#upRange');
  if (up) up.addEventListener('input', updateUpgradeUI);
  const mines = $('#minesRange');
  if (mines) {
    mines.addEventListener('input', () => {
      const lb = $('#minesCount');
      if (lb) lb.textContent = mines.value;
    });
  }
  const dep = $('#depAmount');
  if (dep) dep.addEventListener('input', updateSheetConv);
  const wd = $('#wdAmount');
  if (wd) wd.addEventListener('input', updateSheetConv);
  if (S.view === 'crash') {
    if (G.crash && G.crash.active) {
      resumeCrash();
    } else if (G.crash && G.crash.lastMult) {
      drawCrash(G.crash);
    } else {
      drawCrash({ idle: true, mult: 1, headMult: 2.4, lost: false, crashPoint: 0 });
    }
  }
  if (S.view === 'nft') {
    S.nfts.forEach((n) => loadNftArt(n.row_id));
  }
}

/* ------------------------------ delegation ------------------------------ */

document.addEventListener('click', (ev) => {
  const el = ev.target.closest('[data-act]');
  if (!el) return;
  const act = el.dataset.act;
  switch (act) {
    case 'go': go(el.dataset.view || 'lobby'); break;
    case 'retry':
      S.authError = '';
      S.ready = false;
      app.innerHTML = '<div class="boot"><div class="boot-mark"><svg class="boot-crown"><use href="#i-crown"></use></svg></div><div class="boot-text">DELTA CASINO</div><div class="boot-dots"><i></i><i></i><i></i></div></div>';
      refreshSession(true);
      break;
    case 'scroll-games': {
      const g = $('#games');
      if (g) g.scrollIntoView({ behavior: 'smooth', block: 'start' });
      break;
    }
    case 'sheet-close': closeSheet(); break;
    case 'sheet-overlay':
      if (ev.target === el) closeSheet();
      break;
    case 'open-deposit': openSheet(depositSheet()); bindView(); break;
    case 'open-withdraw': openSheet(withdrawSheet()); bindView(); break;
    case 'quick-pct': {
      const input = $('#' + el.dataset.target);
      if (!input) break;
      const kind = el.dataset.target === 'depAmount' ? 'deposit' : 'bet';
      input.value = String(pctAmount(availableStars(kind), Number(el.dataset.pct) || 0));
      updateSheetConv();
      break;
    }
    case 'bet-step': {
      const input = $('#' + el.dataset.target);
      if (!input) break;
      const cur = num(input.value) || MIN_STARS;
      const next = Math.max(MIN_STARS, Math.round((cur + Number(el.dataset.dir)) * 100) / 100);
      input.value = String(next);
      break;
    }
    case 'deposit': doDeposit(); break;
    case 'withdraw': doWithdraw(); break;
    case 'withdraw-nft': withdrawNft(Number(el.dataset.id)); break;
    case 'refresh-nft': refreshNft(); break;
    case 'refresh-history': loadHistory(true); break;
    case 'crash-start': startCrash(); break;
    case 'crash-cash': cashoutCrash(); break;
    case 'toggle-auto': toggleAuto(); break;
    case 'mines-start': startMines(); break;
    case 'mine-cell': revealMine(Number(el.dataset.cell)); break;
    case 'mines-cash': cashoutMines(); break;
    case 'up-mode': setUpgradeMode(el.dataset.mode); break;
    case 'up-target': setUpgradeTarget(el.dataset.value); break;
    case 'up-play': playUpgrade(); break;
    case 'slots-spin': spinSlots(); break;
    default: break;
  }
});

/* ------------------------------ shared game helpers ------------------------------ */

function readStake(id) {
  const el = $('#' + id);
  const stars = num(el ? el.value : 0);
  if (!(stars > 0)) { toast('Введите ставку', 'err'); return 0; }
  const chips = Math.round(stars * cps());
  if (chips < MIN_STARS * cps()) { toast('Минимальная ставка — ' + MIN_STARS + ' ⭐', 'err'); return 0; }
  if (chips > S.balance) { toast('Недостаточно фишек. Пополните кошелёк.', 'err'); return 0; }
  return chips;
}

function hideResult(id) {
  const el = $('#' + id);
  if (el) { el.className = 'result-box'; el.innerHTML = ''; }
}

function showResult(id, kind, html) {
  const el = $('#' + id);
  if (!el) return;
  el.className = 'result-box show ' + kind;
  el.innerHTML = html;
}

function scrollResult(id) {
  const el = $('#' + id);
  if (!el || !el.scrollIntoView) return;
  try { el.scrollIntoView({ behavior: 'smooth', block: 'center' }); } catch (_) {}
}

function coinBurst(host) {
  if (!host) return;
  const rain = document.createElement('div');
  rain.className = 'coin-rain';
  for (let i = 0; i < 16; i++) {
    const c = document.createElement('i');
    c.style.left = (4 + Math.random() * 92) + '%';
    c.style.animationDelay = (Math.random() * 0.4) + 's';
    c.style.animationDuration = (0.8 + Math.random() * 0.8) + 's';
    rain.appendChild(c);
  }
  host.appendChild(rain);
  setTimeout(() => rain.remove(), 1900);
}

function winOverlay(amountStars, multiplier) {
  const amt = Number(amountStars || 0).toLocaleString('ru-RU', { maximumFractionDigits: 2 });
  const ov = document.createElement('div');
  ov.className = 'win-overlay';
  ov.innerHTML = `<div class="win-card">
    <div class="lb">ВЫИГРЫШ</div>
    <div class="amt">+${amt} <svg style="width:22px;height:22px;color:var(--gold)"><use href="#i-star"></use></svg></div>
    <div class="sub">множитель <span class="big">×${fmtMult(multiplier)}</span></div>
  </div>`;
  document.body.appendChild(ov);
  setTimeout(() => ov.remove(), 2200);
}

/* ================================================================== *
 * CRASH
 * ================================================================== */

const CRASH_STARS = (() => {
  const a = [];
  let s = 987654321;
  const rnd = () => { s = (s * 1103515245 + 12345) & 0x7fffffff; return s / 0x7fffffff; };
  for (let i = 0; i < 70; i++) a.push({ x: rnd(), y: rnd(), r: rnd() * 1.2 + 0.4, a: rnd() * 0.5 + 0.25 });
  return a;
})();

function crashView() {
  const c = G.crash;
  const active = !!(c && c.active);
  const mult = active && c.mult ? c.mult : (c && c.lastMult ? c.lastMult : 1);
  const stake = c && c.stake ? c.stake : 0;
  const cashLabel = 'Забрать ' + fmtStars(winOf(Math.floor(stake * Math.max(1, mult)))) + ' ' + star('sm');
  const stateLabel = active ? 'в полёте' : (c && c.state ? c.state : 'готов к запуску');
  const lost = c && c.lost;
  const won = c && c.result && c.result.kind === 'win';
  return `${screenHead('Краш', rtpText())}
  <div class="crash-stage">
    <canvas id="crashCanvas"></canvas>
    <div class="crash-hud">
      <div class="crash-mult ${lost ? 'lost' : (won ? 'won' : '')}" id="crashMult">${fmtMult(mult)}<span class="x">×</span></div>
      <div class="crash-state" id="crashState">${stateLabel}</div>
    </div>
    ${active ? `<div class="crash-cash"><button class="btn gold" id="crashCashBtn" data-act="crash-cash">${cashLabel}</button></div>` : ''}
  </div>
  <div class="card" style="margin:12px 16px 0">
    ${active ? '' : betBox('crashBet', c && c.lastBet ? c.lastBet : MIN_STARS)}
    <div class="auto-row" style="margin-top:${active ? '0' : '14px'}">
      <div class="txt">Авто-кэшаут <span class="dim">×</span></div>
      <div style="display:flex;align-items:center;gap:9px">
        <input id="autoMult" inputmode="decimal" value="${c && c.auto ? fmtMult(c.auto) : '2,00'}">
        <button class="switch ${c && c.auto ? 'on' : ''}" id="autoSwitch" data-act="toggle-auto" aria-label="Авто-кэшаут"></button>
      </div>
    </div>
    <div style="margin-top:14px">
      ${active
        ? ''
        : `<button class="btn green" id="crashStartBtn" data-act="crash-start">Запустить ракету</button>`}
    </div>
    <div class="result-box ${c && c.result ? 'show ' + c.result.kind : ''}" id="crashResult">${c && c.result ? c.result.html : ''}</div>
    <div class="notice"><svg class="ic"><use href="#i-shield"></use></svg><div>Ракета летит, множитель растёт. Заберите до краша — иначе ставка сгорит. ${rtpText()}.</div></div>
  </div>`;
}

function crashCurveMult(t) { return Math.min(100, Math.exp(CRASH_K * t)); }

async function startCrash() {
  if (G.crash && G.crash.active) return;
  const stake = readStake('crashBet');
  if (!stake) return;
  const btn = $('#crashStartBtn');
  if (btn) { btn.disabled = true; btn.textContent = 'Запускаем…'; }
  try {
    const r = await api('/rounds/crash', {
      method: 'POST',
      body: JSON.stringify({ stake, client_seed: uid(), key: uid() }),
    });
    S.balance = Number(r.balance) || 0;
    syncBalance();
    const sw = $('#autoSwitch');
    const autoOn = !!(sw && sw.classList.contains('on'));
    const autoVal = Math.max(1.05, num($('#autoMult') ? $('#autoMult').value : 0) || 2);
    // Sync the local clock to the server's round start so the animated
    // multiplier tracks the real one.
    const started = Number(r.started_at) || 0;
    const t0 = started ? (performance.now() - (Date.now() - started)) : performance.now();
    G.crash = {
      roundId: r.round_id, stake, active: true, t0,
      mult: 1, headMult: 1, crashPoint: 0, auto: autoOn ? autoVal : 0,
      raf: 0, poll: 0, lastBet: $('#crashBet') ? $('#crashBet').value : 1,
      state: 'в полёте', result: null, lost: false, lastMult: 0, boomAt: 0,
    };
    haptic('medium');
    render();
    resumeCrash();
    crashPoll();
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; btn.textContent = 'Запустить ракету'; }
  }
}

function resumeCrash() {
  const c = G.crash;
  if (!c || !c.active) return;
  if (c.raf) cancelAnimationFrame(c.raf);
  c.raf = requestAnimationFrame(crashTick);
}

function crashTick(now) {
  const c = G.crash;
  if (!c || !c.active) { if (c) c.raf = 0; return; }
  const t = (now - c.t0) / 1000;
  c.mult = crashCurveMult(t);
  c.headMult = c.mult;
  c.lastMult = c.mult;
  const mEl = $('#crashMult');
  if (mEl) mEl.innerHTML = fmtMult(c.mult) + '<span class="x">×</span>';
  const cash = $('#crashCashBtn');
  if (cash) cash.innerHTML = 'Забрать ' + fmtStars(winOf(Math.floor(c.stake * c.mult))) + ' ' + star('sm');
  drawCrash(c);
  if (c.auto && c.mult >= c.auto) { cashoutCrash(); return; }
  c.raf = requestAnimationFrame(crashTick);
}

// crashPoll asks the server for the live state so the rocket crashes on its
// own even if the player never presses cash out.
function crashPoll() {
  const c = G.crash;
  if (!c || !c.active) { c.poll = 0; return; }
  api('/rounds/' + c.roundId + '/crash-state', { method: 'GET' })
    .then((r) => {
      if (!G.crash || G.crash.roundId !== c.roundId) return;
      if (r.status === 'lost') {
        c.crashPoint = Number(r.outcome && r.outcome.crash) || c.mult;
        finishCrash(false, r);
      } else {
        c.poll = setTimeout(crashPoll, 350);
      }
    })
    .catch(() => {
      if (c.active) c.poll = setTimeout(crashPoll, 900);
    });
}

function crashStateMult(c) {
  // Determine the multiplier to display once the round is over.
  if (c.result && c.result.kind === 'win') return c.lastMult;
  if (c.crashPoint) return c.crashPoint;
  return c.lastMult || 1;
}

async function cashoutCrash() {
  const c = G.crash;
  if (!c || !c.active) return;
  c.active = false;
  if (c.raf) { cancelAnimationFrame(c.raf); c.raf = 0; }
  if (c.poll) { clearTimeout(c.poll); c.poll = 0; }
  const mult = Math.max(1.01, Math.floor((c.mult || 1) * 100) / 100);
  const btn = $('#crashCashBtn');
  if (btn) { btn.disabled = true; btn.textContent = 'Забираем…'; }
  try {
    const r = await api('/rounds/' + c.roundId + '/cashout', {
      method: 'POST',
      body: JSON.stringify({ multiplier: mult }),
    });
    S.balance = Number(r.balance) || 0;
    if (r.status === 'won') finishCrash(true, r, mult);
    else finishCrash(false, r);
  } catch (e) {
    c.active = true;
    resumeCrash();
    if (btn) { btn.disabled = false; btn.innerHTML = 'Забрать ' + fmtStars(winOf(Math.floor(c.stake * (c.mult || 1)))) + ' ' + star('sm'); }
    toast(e.message, 'err');
  }
}

function finishCrash(won, r, mult) {
  const c = G.crash;
  if (!c) return;
  c.active = false;
  if (c.raf) { cancelAnimationFrame(c.raf); c.raf = 0; }
  if (c.poll) { clearTimeout(c.poll); c.poll = 0; }
  c.crashPoint = Number(r.outcome && r.outcome.crash) || c.crashPoint || c.mult;
  c.lost = !won;
  const finalMult = won ? (mult || c.mult || 1) : c.crashPoint;
  c.lastMult = finalMult;
  c.headMult = finalMult;
  c.mult = finalMult;
  c.state = won ? 'вы забрали' : 'краш';
  const paidMult = Number(r.multiplier) || finalMult;
  if (won) {
    c.result = { kind: 'win', html: '<span class="big green">+' + fmtStars(r.payout) + ' ' + star('sm') + '</span>Кэшаут на ×' + fmtMult(paidMult) + ' · краш на ×' + fmtMult(c.crashPoint) };
    haptic('success');
  } else {
    c.boomAt = performance.now();
    c.result = { kind: 'lose', html: '<span class="big red">Краш на ×' + fmtMult(c.crashPoint) + '</span>Ставка ' + fmtStars(c.stake) + ' ' + star('sm') + ' сгорела' };
    haptic('error');
  }
  render();
  drawCrash(G.crash);
  if (won) {
    coinBurst($('.crash-stage'));
    winOverlay(starsOf(r.payout), paidMult);
  }
  loadHistory(true);
}

function toggleAuto() {
  const sw = $('#autoSwitch');
  if (!sw) return;
  sw.classList.toggle('on');
  const on = sw.classList.contains('on');
  const val = Math.max(1.05, num($('#autoMult') ? $('#autoMult').value : 0) || 2);
  if (G.crash) G.crash.auto = on ? val : 0;
  haptic('light');
}

function drawCrash(c) {
  const cv = $('#crashCanvas');
  if (!cv) return;
  const w = cv.clientWidth || 340;
  const h = cv.clientHeight || 300;
  const dpr = Math.min(2, window.devicePixelRatio || 1);
  if (cv.width !== Math.round(w * dpr) || cv.height !== Math.round(h * dpr)) {
    cv.width = Math.round(w * dpr);
    cv.height = Math.round(h * dpr);
  }
  const ctx = cv.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);

  // star field
  for (const s of CRASH_STARS) {
    ctx.globalAlpha = s.a * (c.idle ? 0.4 : 1);
    ctx.fillStyle = '#cfe0ff';
    ctx.beginPath();
    ctx.arc(s.x * w, s.y * h, s.r, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;

  // grid
  ctx.strokeStyle = 'rgba(255,255,255,.04)';
  ctx.lineWidth = 1;
  for (let i = 1; i < 4; i++) { const y = (h * i) / 4; ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke(); }
  for (let i = 1; i < 6; i++) { const x = (w * i) / 6; ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke(); }

  const idle = !!c.idle;
  const head = Math.max(1.02, c.headMult || c.mult || 1);
  const maxM = Math.max(1.6, head * 1.18);
  const tHead = Math.log(head) / CRASH_K;
  const maxT = Math.max(1.2, tHead * 1.08);
  const X = (t) => 12 + (Math.min(t, maxT) / maxT) * (w - 24);
  const Y = (m) => h - 18 - ((Math.min(Math.max(1, m), maxM) - 1) / (maxM - 1)) * (h - 60);
  const steps = 100;

  if (idle) ctx.globalAlpha = 0.4;

  // area fill
  ctx.beginPath();
  ctx.moveTo(X(0), Y(1));
  for (let i = 1; i <= steps; i++) { const t = (maxT * i) / steps; ctx.lineTo(X(t), Y(crashCurveMult(t))); }
  ctx.lineTo(X(maxT), h - 18);
  ctx.lineTo(X(0), h - 18);
  ctx.closePath();
  const grad = ctx.createLinearGradient(0, 0, 0, h);
  grad.addColorStop(0, c.lost ? 'rgba(255,84,104,.28)' : 'rgba(74,168,255,.22)');
  grad.addColorStop(1, 'rgba(74,168,255,0)');
  ctx.fillStyle = grad;
  ctx.fill();

  // curve
  ctx.beginPath();
  ctx.moveTo(X(0), Y(1));
  for (let i = 1; i <= steps; i++) { const t = (maxT * i) / steps; ctx.lineTo(X(t), Y(crashCurveMult(t))); }
  ctx.strokeStyle = c.lost ? '#ff5468' : '#4aa8ff';
  ctx.lineWidth = 2.8;
  ctx.lineJoin = 'round';
  ctx.lineCap = 'round';
  ctx.shadowColor = c.lost ? 'rgba(255,84,104,.75)' : 'rgba(74,168,255,.75)';
  ctx.shadowBlur = 12;
  ctx.stroke();
  ctx.shadowBlur = 0;
  ctx.globalAlpha = 1;

  const hx = X(tHead);
  const hy = Y(head);

  // crash marker line
  if (c.crashPoint && c.crashPoint > 1.001) {
    const tc = Math.log(Math.min(c.crashPoint, 100)) / CRASH_K;
    if (tc <= maxT * 1.05) {
      const cx = X(tc);
      ctx.setLineDash([4, 5]);
      ctx.beginPath(); ctx.moveTo(cx, 14); ctx.lineTo(cx, h - 22);
      ctx.strokeStyle = 'rgba(255,84,104,.5)';
      ctx.lineWidth = 1.4;
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = '#ff8b99';
      ctx.font = '700 10px -apple-system, Segoe UI, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('×' + fmtMult(c.crashPoint), cx, 12);
    }
  }

  if (c.lost) {
    drawExplosion(ctx, hx, hy, c);
  } else {
    drawRocket(ctx, hx, hy, c);
  }
}

function drawRocket(ctx, x, y, c) {
  // tangent along the curve
  const t = Math.log(Math.min(c.mult || 1, 99)) / CRASH_K;
  const t2 = t + 0.08;
  const maxM = Math.max(1.6, (c.headMult || c.mult || 1) * 1.18);
  const maxT = Math.max(1.2, t2 * 1.4);
  const X = (tt) => 12 + (Math.min(tt, maxT) / maxT) * (ctx.canvas.clientWidth - 24);
  const Y = (m) => (ctx.canvas.clientHeight) - 18 - ((Math.min(Math.max(1, m), maxM) - 1) / (maxM - 1)) * ((ctx.canvas.clientHeight) - 60);
  const x2 = X(t2), y2 = Y(crashCurveMult(t2));
  const ang = Math.atan2(y2 - y, x2 - x);

  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(ang);

  // flame (flickers)
  const flick = Math.random() * 4 - 2;
  const fg = ctx.createLinearGradient(-26, 0, -4, 0);
  fg.addColorStop(0, 'rgba(255,176,58,0)');
  fg.addColorStop(1, 'rgba(255,176,58,.9)');
  ctx.fillStyle = fg;
  ctx.beginPath();
  ctx.moveTo(-5, -5 + flick * 0.3);
  ctx.lineTo(-24 - flick, 0);
  ctx.lineTo(-5, 5 - flick * 0.3);
  ctx.closePath();
  ctx.fill();

  // body
  ctx.fillStyle = '#eef4fd';
  ctx.beginPath();
  ctx.roundRect ? ctx.roundRect(-5, -7, 22, 14, 7) : ctx.rect(-5, -7, 22, 14);
  ctx.fill();
  // nose
  ctx.fillStyle = '#4aa8ff';
  ctx.beginPath();
  ctx.moveTo(17, -7); ctx.lineTo(26, 0); ctx.lineTo(17, 7); ctx.closePath();
  ctx.fill();
  // fins
  ctx.fillStyle = '#8fa6c4';
  ctx.beginPath(); ctx.moveTo(-2, -7); ctx.lineTo(-10, -14); ctx.lineTo(2, -7); ctx.closePath(); ctx.fill();
  ctx.beginPath(); ctx.moveTo(-2, 7); ctx.lineTo(-10, 14); ctx.lineTo(2, 7); ctx.closePath(); ctx.fill();
  // window
  ctx.fillStyle = '#2b3547';
  ctx.beginPath(); ctx.arc(4, 0, 3.4, 0, Math.PI * 2); ctx.fill();
  ctx.fillStyle = '#9fd0ff';
  ctx.beginPath(); ctx.arc(3, -1, 1.2, 0, Math.PI * 2); ctx.fill();

  ctx.restore();

  // glow
  ctx.save();
  ctx.globalCompositeOperation = 'lighter';
  const glow = ctx.createRadialGradient(x, y, 0, x, y, 26);
  glow.addColorStop(0, 'rgba(159,208,255,.55)');
  glow.addColorStop(1, 'rgba(159,208,255,0)');
  ctx.fillStyle = glow;
  ctx.beginPath(); ctx.arc(x, y, 26, 0, Math.PI * 2); ctx.fill();
  ctx.restore();
}

function drawExplosion(ctx, x, y, c) {
  const age = performance.now() - (c.boomAt || performance.now());
  const p = Math.min(1, age / 600);
  const n = 22;
  ctx.save();
  ctx.globalCompositeOperation = 'lighter';
  for (let i = 0; i < n; i++) {
    const ang = (i / n) * Math.PI * 2 + (i % 3) * 0.3;
    const dist = p * (34 + (i % 5) * 12);
    const px = x + Math.cos(ang) * dist;
    const py = y + Math.sin(ang) * dist;
    const alpha = (1 - p) * 0.9;
    ctx.globalAlpha = Math.max(0, alpha);
    ctx.fillStyle = i % 2 ? '#ffb03a' : '#ff5468';
    ctx.beginPath();
    ctx.arc(px, py, 3.4 * (1 - p * 0.6), 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.restore();
  ctx.globalAlpha = 1;
  // fireball
  const fp = Math.min(1, age / 220);
  ctx.globalAlpha = (1 - fp) * 0.7;
  const fg = ctx.createRadialGradient(x, y, 0, x, y, 20 + fp * 20);
  fg.addColorStop(0, '#fff');
  fg.addColorStop(0.4, '#ffb03a');
  fg.addColorStop(1, 'rgba(255,84,104,0)');
  ctx.fillStyle = fg;
  ctx.beginPath(); ctx.arc(x, y, 20 + fp * 20, 0, Math.PI * 2); ctx.fill();
  ctx.globalAlpha = 1;

  if (p < 1) requestAnimationFrame(() => drawCrash(c));
}

/* ================================================================== *
 * MINES
 * ================================================================== */

function minesMultiplier(mines, revealed) {
  const total = 25;
  const safe = total - mines;
  let fair = 1;
  for (let k = 0; k < revealed; k++) fair *= (total - k) / (safe - k);
  return fair * 0.95;
}

function minesView() {
  const m = G.mines;
  const active = !!(m && m.active);
  const mines = m ? m.mines : 3;
  const safe = m ? (m.revealed ? m.revealed.length : 0) : 0;
  const next = minesMultiplier(mines, safe + 1);
  const cashAmt = m && safe > 0 ? winOf(Math.floor(m.stake * (m.multiplier || minesMultiplier(m.mines, safe)))) : 0;
  const resultHtml = m && m.result ? minesResultHtml(m.result) : '';
  return `${screenHead('Мины', rtpText())}
  <div class="wrap">
    <div class="card">
      ${active ? '' : betBox('minesBet', m && m.lastBet ? m.lastBet : MIN_STARS)}
      <div class="bet-label" style="margin-top:${active ? '0' : '14px'}"><label>Мин на поле</label><span class="avail">${active ? 'зафиксировано' : 'от 1 до 10'}</span></div>
      <input class="range" id="minesRange" type="range" min="1" max="10" step="1" value="${m ? m.mines : 3}" ${active ? 'disabled' : ''}>
      <div class="up-scale"><span>1 мина</span><span>10 мин</span></div>
      ${active ? `<button class="btn green" style="margin-top:14px" id="minesCashBtn" data-act="mines-cash" ${safe > 0 ? '' : 'disabled'}>Забрать ${safe > 0 ? fmtStars(cashAmt) + ' ' + star('sm') : ''}</button>`
        : `<button class="btn green" style="margin-top:14px" data-act="mines-start">Начать игру</button>`}
    </div>

    <div class="mines-top">
      <span class="muted">Открыто <b id="minesSafe">${safe}</b> / ${25 - mines}</span>
      <span class="mines-mult" id="minesMult">${active ? 'Следующий ×' + fmtMult(next) : '×' + fmtMult(minesMultiplier(mines, 1))}</span>
    </div>
    <div class="mines-grid" id="minesGrid">${minesCells()}</div>
    <div class="result-box ${m && m.result ? 'show ' + (m.result.won ? 'win' : 'lose') : ''}" id="minesResult">${resultHtml}</div>
    <div class="notice"><svg class="ic"><use href="#i-gem"></use></svg><div>Открывайте безопасные клетки — множитель растёт. Наткнётесь на мину — ставка сгорит.</div></div>
  </div>`;
}

function minesCells() {
  const m = G.mines;
  let out = '';
  for (let i = 0; i < 25; i++) {
    const st = m && m.cells ? m.cells[i] : '';
    let cls = '';
    let inner = '<span class="dot"></span>';
    if (st === 'gem') { cls = 'gem'; inner = '<svg><use href="#i-gem"></use></svg>'; }
    else if (st === 'boom') { cls = 'boom'; inner = '<svg><use href="#i-bomb"></use></svg>'; }
    else if (st === 'dim') { cls = 'dim'; inner = '<svg><use href="#i-bomb"></use></svg>'; }
    const disabled = !m || !m.active;
    out += `<button class="mine-cell ${cls}" data-act="mine-cell" data-cell="${i}" ${disabled ? 'disabled' : ''}>${inner}</button>`;
  }
  return out;
}

function cellInner(kind) {
  if (kind === 'gem') return '<svg><use href="#i-gem"></use></svg>';
  if (kind === 'boom' || kind === 'dim') return '<svg><use href="#i-bomb"></use></svg>';
  return '<span class="dot"></span>';
}

// revealCellDom flips a single cell in place with a pop animation.
function revealCellDom(cell, kind) {
  const btn = $(`.mine-cell[data-cell="${cell}"]`);
  if (!btn) return;
  btn.classList.remove('gem', 'boom', 'dim');
  btn.innerHTML = cellInner(kind);
  // restart the animation
  void btn.offsetWidth;
  if (kind) btn.classList.add(kind);
}

function minesResultHtml(res) {
  if (res.won) {
    return '<span class="big green">+' + fmtStars(res.payout) + ' ' + star('sm') + '</span>×' + fmtMult(res.multiplier) + ' за ' + res.revealed + ' ' + plural(res.revealed, 'клетку', 'клетки', 'клеток');
  }
  return '<span class="big red">Мины!</span>Ставка ' + fmtStars(res.stake) + ' ' + star('sm') + ' сгорела';
}

async function startMines() {
  if (G.mines && G.mines.active) return;
  const stake = readStake('minesBet');
  if (!stake) return;
  const minesEl = $('#minesRange');
  const mines = Number(minesEl ? minesEl.value : 3) || 3;
  const btn = $('[data-act="mines-start"]');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spin-loader"></span> Открываем поле…'; }
  try {
    const r = await api('/rounds/mines', {
      method: 'POST',
      body: JSON.stringify({ stake, mines, client_seed: uid(), key: uid() }),
    });
    S.balance = Number(r.balance) || 0;
    syncBalance();
    G.mines = {
      id: r.round_id, stake, mines, active: true, cells: {},
      revealed: [], multiplier: 0, result: null,
      lastBet: $('#minesBet') ? $('#minesBet').value : 1,
    };
    haptic('medium');
    render();
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; btn.textContent = 'Начать игру'; }
  }
}

async function revealMine(cell) {
  const m = G.mines;
  if (!m || !m.active) return;
  if (m.cells[cell]) return;
  const btn = $(`.mine-cell[data-cell="${cell}"]`);
  if (btn) { btn.disabled = true; }
  try {
    const r = await api('/rounds/' + m.id + '/reveal', {
      method: 'POST',
      body: JSON.stringify({ cell }),
    });
    S.balance = Number(r.balance) || 0;
    syncBalance();
    if (r.status === 'lost') {
      m.active = false;
      m.cells[cell] = 'boom';
      m.revealed = Array.isArray(r.outcome && r.outcome.revealed) ? r.outcome.revealed : m.revealed;
      for (let i = 0; i < 25; i++) if (!m.cells[i]) m.cells[i] = 'dim';
      m.result = { won: false, stake: m.stake };
      haptic('error');
      revealCellDom(cell, 'boom');
      // reveal the remaining mines with a short stagger
      const dims = [];
      for (let i = 0; i < 25; i++) if (m.cells[i] === 'dim') dims.push(i);
      dims.forEach((d, idx) => setTimeout(() => revealCellDom(d, 'dim'), 90 + idx * 35));
      render();
      setTimeout(() => scrollResult('minesResult'), 120);
      loadHistory(true);
      return;
    }
    m.cells[cell] = 'gem';
    m.revealed = Array.isArray(r.outcome && r.outcome.revealed) ? r.outcome.revealed : m.revealed.concat(cell);
    m.multiplier = Number(r.outcome && r.outcome.multiplier) || minesMultiplier(m.mines, m.revealed.length);
    revealCellDom(cell, 'gem');
    updateMinesUI();
    haptic('light');
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; }
  }
}

function updateMinesUI() {
  const m = G.mines;
  if (!m) return;
  const safe = m.revealed.length;
  const safeEl = $('#minesSafe');
  if (safeEl) safeEl.textContent = safe;
  const nextEl = $('#minesMult');
  if (nextEl) nextEl.textContent = 'Следующий ×' + fmtMult(minesMultiplier(m.mines, safe + 1));
  const cash = $('#minesCashBtn');
  if (cash) {
    const amount = winOf(Math.floor(m.stake * (m.multiplier || minesMultiplier(m.mines, safe))));
    cash.disabled = safe === 0;
    cash.innerHTML = safe > 0 ? 'Забрать ' + fmtStars(amount) + ' ' + star('sm') : 'Забрать';
  }
}

async function cashoutMines() {
  const m = G.mines;
  if (!m || !m.active || !m.revealed.length) return;
  const btn = $('#minesCashBtn');
  if (btn) { btn.disabled = true; btn.textContent = 'Забираем…'; }
  try {
    const r = await api('/rounds/' + m.id + '/mines-cashout', { method: 'POST' });
    S.balance = Number(r.balance) || 0;
    syncBalance();
    m.active = false;
    m.multiplier = Number(r.multiplier) || Number(r.outcome && r.outcome.multiplier) || m.multiplier;
    m.result = { won: true, payout: r.payout, multiplier: m.multiplier, revealed: m.revealed.length };
    haptic('success');
    render();
    scrollResult('minesResult');
    winOverlay(starsOf(r.payout), m.multiplier);
    loadHistory(true);
  } catch (e) {
    toast(e.message, 'err');
    if (btn) { btn.disabled = false; btn.textContent = 'Забрать'; }
  }
}

/* ================================================================== *
 * UPGRADE
 * ================================================================== */

/* ================================================================== *
 * UPGRADE — wheel with a spinning needle (upgrader.pro layout)
 * ================================================================== */

const UP_TARGETS = [1.5, 2, 3, 5, 10];

function upChance(target) {
  return Math.min(1, 0.95 / target);
}

// upWheelSvg draws the wheel: a green "win" arc sized by the chance, a dark
// "lose" arc for the rest, and a needle that spins and settles.
function upWheelSvg(target, needleDeg) {
  const chance = upChance(target);
  const R = 84;
  const C = 2 * Math.PI * R;
  const winLen = C * chance;
  const lossLen = C - winLen;
  return `<svg viewBox="0 0 200 200">
    <defs>
      <linearGradient id="upWin" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#35e3a1"/>
        <stop offset="1" stop-color="#12a06d"/>
      </linearGradient>
      <radialGradient id="upCore" cx="35%" cy="30%">
        <stop offset="0" stop-color="#1a2231"/>
        <stop offset="100%" stop-color="#0a0d14"/>
      </radialGradient>
      <linearGradient id="upNeedleFill" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#fff3c4"/>
        <stop offset="45%" stop-color="#ffd166"/>
        <stop offset="100%" stop-color="#e8863a"/>
      </linearGradient>
      <filter id="upNeedleGlow" x="-60%" y="-60%" width="220%" height="220%">
        <feGaussianBlur stdDeviation="3" result="b"/>
        <feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
      </filter>
    </defs>
    <circle cx="100" cy="100" r="94" fill="rgba(0,0,0,.35)"/>
    <circle cx="100" cy="100" r="${R}" fill="none" stroke="#242a38" stroke-width="20"/>
    <circle id="upWinArc" cx="100" cy="100" r="${R}" fill="none" stroke="url(#upWin)" stroke-width="20"
      stroke-dasharray="${winLen.toFixed(2)} ${lossLen.toFixed(2)}" stroke-dashoffset="0"
      transform="rotate(-90 100 100)" stroke-linecap="butt"/>
    <g id="upNeedle" transform="rotate(${Number(needleDeg) || 0} 100 100)" style="transform-origin:100px 100px">
      <path d="M100 20 L107 40 L93 40 Z" fill="url(#upNeedleFill)" filter="url(#upNeedleGlow)"/>
      <path d="M100 40 L103 96 L97 96 Z" fill="#ffd166" opacity=".85"/>
      <circle cx="100" cy="100" r="13" fill="#0b0e15" stroke="#ffd166" stroke-width="3"/>
      <circle cx="100" cy="100" r="5" fill="#ffd166"/>
    </g>
    <circle cx="100" cy="100" r="46" fill="url(#upCore)" stroke="rgba(255,255,255,.06)"/>
  </svg>`;
}

function upgradeView() {
  const u = G.upgrade;
  const target = Number(u.target) || 2;
  const chance = upChance(target);
  const resultHtml = u.result ? upgradeResultHtml(u.result) : '';
  return `${screenHead('Апгрейд', rtpText())}
  <div class="wrap">
    <div class="card">
      <div class="seg">
        <button class="${u.mode === 'chips' ? 'on' : ''}" data-act="up-mode" data-mode="chips">Фишки</button>
        <button class="${u.mode === 'nft' ? 'on' : ''}" data-act="up-mode" data-mode="nft">NFT-приз</button>
      </div>
      <div class="up-stage">
        <div class="up-wheel" id="upGauge">
          ${upWheelSvg(target, u.needle)}
          <div class="up-center">
            <div class="up-chance" id="upChance">${Math.round(chance * 100)}%</div>
            <div class="up-mult" id="upTarget">×${fmtMult(target)}</div>
          </div>
        </div>
      </div>
      <div class="up-targets">
        ${UP_TARGETS.map((t) => `<button class="up-target ${Number(target) === t ? 'on' : ''}" data-act="up-target" data-value="${t}">×${fmtMult(t, t < 10 ? 1 : 0)}</button>`).join('')}
      </div>
      <div class="up-slider">
        <input id="upRange" type="range" min="1.1" max="20" step="0.1" value="${target}">
        <div class="up-scale"><span>×1,10</span><span>×20,00</span></div>
      </div>
    </div>

    <div class="card">
      ${betBox('upBet', u.bet || MIN_STARS)}
      <button class="btn green" style="margin-top:13px" id="upPlayBtn" data-act="up-play">${u.mode === 'nft' ? 'Крутить за NFT' : 'Крутить'}</button>
      <div class="result-box ${u.result ? 'show ' + (u.result.kind === 'lost' ? 'lose' : 'win') : ''}" id="upResult">${resultHtml}</div>
      <div class="notice"><svg class="ic"><use href="#${u.mode === 'nft' ? 'i-gift' : 'i-gem'}"></use></svg><div>${u.mode === 'nft'
        ? 'Стрелка останавливается в зелёной зоне — приз настоящий NFT из банка казино. Если подходящего NFT нет, выигрыш придёт фишками.'
        : 'Стрелка останавливается в зелёной зоне — выигрыш ставка × множитель. Шанс = 95% ÷ множитель · ${rtpText()}.'}</div></div>
    </div>
  </div>`;
}

function upgradeResultHtml(res) {
  if (res.kind === 'nft') {
    return '<span class="big gold">NFT получен</span>'
      + '<div class="nft-win" data-nft-win="' + res.id + '"><div class="nft-win-stage"></div></div>'
      + 'Подарок #' + res.id + ' · ценность ' + fmtStars(res.value) + ' ' + star('sm')
      + '<br><span class="muted">Уже в NFT-инвентаре — можно вывести в профиль.</span>';
  }
  if (res.kind === 'won') {
    return '<span class="big green">+' + fmtStars(res.payout) + ' ' + star('sm') + '</span>Множитель ×' + fmtMult(res.target);
  }
  return '<span class="big red">Не повезло</span>Ставка ' + fmtStars(res.stake) + ' ' + star('sm') + ' сгорела';
}

function updateUpgradeUI() {
  const range = $('#upRange');
  if (!range) return;
  const u = G.upgrade;
  u.target = Math.max(1.1, Number(range.value) || 2);
  const chance = upChance(u.target);
  const circumference = 2 * Math.PI * 84;
  const chanceEl = $('#upChance');
  if (chanceEl) chanceEl.textContent = Math.round(chance * 100) + '%';
  const targetEl = $('#upTarget');
  if (targetEl) targetEl.textContent = '×' + fmtMult(u.target);
  const arc = $('#upWinArc');
  if (arc) arc.setAttribute('stroke-dasharray', (circumference * chance).toFixed(2) + ' ' + (circumference * (1 - chance)).toFixed(2));
  $$('.up-target').forEach((b) => b.classList.toggle('on', Number(b.dataset.value) === Number(u.target)));
}

function setUpgradeTarget(value) {
  const u = G.upgrade;
  const bet = $('#upBet');
  if (bet) u.bet = bet.value;
  u.target = Math.max(1.1, Number(value) || 2);
  render();
}

function setUpgradeMode(mode) {
  if (mode !== 'chips' && mode !== 'nft') return;
  const bet = $('#upBet');
  if (bet) G.upgrade.bet = bet.value;
  G.upgrade.mode = mode;
  G.upgrade.result = null;
  render();
}

// spinUpNeedle animates the needle to a final angle. The wheel is drawn with
// the win arc starting at the top (12 o'clock), so a final angle inside
// [0, chance*360) means the server's win, and anything else is a loss.
function spinUpNeedle(target, won) {
  const needle = $('#upNeedle');
  const chance = upChance(target);
  const winDeg = chance * 360;
  // Land comfortably inside the zone rather than exactly on its edge.
  const angle = won
    ? (0.06 + Math.random() * 0.86) * winDeg
    : winDeg + 12 + Math.random() * (359.9 - winDeg - 24);
  const turns = 4 + Math.floor(Math.random() * 3);
  const final = turns * 360 + angle;
  // Remember the final angle so a re-render keeps the needle where it landed.
  G.upgrade.needle = ((final % 360) + 360) % 360;
  if (!needle) return Promise.resolve();
  needle.style.transition = 'transform 1.9s cubic-bezier(.15,.85,.15,1)';
  needle.style.transform = `rotate(${final.toFixed(1)}deg)`;
  return sleep(1950);
}

async function playUpgrade() {
  const u = G.upgrade;
  if (u.busy) return;
  const stake = readStake('upBet');
  if (!stake) return;
  const target = Number(u.target) || 2;
  u.busy = true;
  u.bet = $('#upBet') ? $('#upBet').value : 1;
  const gauge = $('#upGauge');
  const needle = $('#upNeedle');
  G.upgrade.needle = 0;
  if (needle) { needle.style.transition = 'none'; needle.style.transform = 'rotate(0deg)'; void needle.offsetWidth; }
  if (gauge) gauge.classList.add('spinning');
  const btn = $('#upPlayBtn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spin-loader"></span> Крутим…'; }
  hideResult('upResult');
  const started = Date.now();
  try {
    const path = u.mode === 'nft' ? '/play/upgrade-nft' : '/play/upgrade';
    const r = await api(path, {
      method: 'POST',
      body: JSON.stringify({ stake, target, client_seed: uid(), key: uid() }),
    });
    const won = r.status === 'won' || !!r.nft_gift_row_id;
    await Promise.all([spinUpNeedle(target, won), sleep(Math.max(0, 300 - (Date.now() - started)))]);
    if (gauge) gauge.classList.remove('spinning');
    S.balance = Number(r.balance) || 0;
    setBalance(S.balance);
    if (u.mode === 'nft' && r.nft_gift_row_id) {
      u.result = { kind: 'nft', id: r.nft_gift_row_id, value: Number(r.nft_value) || Number(r.payout) || 0 };
      haptic('success');
      await refreshSession();
    } else if (r.status === 'won') {
      const paidMult = Number(r.multiplier) || target;
      u.result = { kind: 'won', payout: r.payout, target: paidMult };
      haptic('success');
      winOverlay(starsOf(r.payout), paidMult);
      coinBurst($('#upGauge'));
    } else {
      u.result = { kind: 'lost', stake };
      haptic('error');
    }
    render();
    if (u.result && u.result.kind === 'nft') paintNftWin(u.result.id);
    scrollResult('upResult');
    loadHistory(true);
  } catch (e) {
    if (gauge) gauge.classList.remove('spinning');
    toast(e.message, 'err');
  } finally {
    u.busy = false;
    const b = $('#upPlayBtn');
    if (b) { b.disabled = false; b.textContent = u.mode === 'nft' ? 'Крутить за NFT' : 'Крутить'; }
  }
}


/* ================================================================== *
 * SLOTS — 6x5 scatter-pays "Gates" board
 * ================================================================== */

// gatesSymbols maps the engine's symbol index to a sprite id. Index 8 is the
// multiplier orb, which draws its own value text instead of a symbol.
const GATE_SYMBOLS = [
  { id: 'g-gem-blue', name: 'Сапфир' },
  { id: 'g-gem-green', name: 'Изумруд' },
  { id: 'g-gem-purple', name: 'Аметист' },
  { id: 'g-gem-red', name: 'Рубин' },
  { id: 'g-gem-gold', name: 'Топаз' },
  { id: 'g-hourglass', name: 'Песочные часы' },
  { id: 'g-ring', name: 'Кольцо' },
  { id: 'g-crown', name: 'Корона' },
];
const GATE_ORB = 8;
const GATE_COLS = 6;
const GATE_ROWS = 5;

function gateCell(index) {
  const col = Math.floor(index / GATE_ROWS);
  const row = index % GATE_ROWS;
  return { col, row };
}

function gateSymbolSvg(sym) {
  const meta = GATE_SYMBOLS[sym];
  return meta ? `<svg><use href="#${meta.id}"></use></svg>` : '';
}

// gatesBoardHtml renders the 30-cell board. `orbs` maps cell index -> multiplier
// value for the golden orbs and `hits` is the set of winning cells to highlight.
function gatesBoardHtml(grid, orbs = {}, hits = {}, cleared = {}) {
  let cells = '';
  // Render top row first: engine index is col*rows+row with row 0 at the bottom.
  for (let row = GATE_ROWS - 1; row >= 0; row--) {
    for (let col = 0; col < GATE_COLS; col++) {
      const i = col * GATE_ROWS + row;
      const sym = grid && grid[i] != null ? grid[i] : 0;
      const orb = orbs[i];
      const cls = ['gate-cell'];
      if (sym === GATE_ORB) cls.push('orb');
      if (hits[i]) cls.push('hit');
      if (cleared[i]) cls.push('cleared');
      const inner = sym === GATE_ORB
        ? `<span class="orb-face"><b>×${orb || 2}</b></span>`
        : gateSymbolSvg(sym);
      cells += `<div class="${cls.join(' ')}" data-cell="${i}">${inner}</div>`;
    }
  }
  return cells;
}

function gatesOrbMap(initial, tumbles) {
  // Orb values are drawn per tumble; the first tumble that sees a cell wins.
  const map = {};
  (tumbles || []).forEach((t) => {
    (t.orbs || []).forEach((o) => { if (map[o.cell] == null) map[o.cell] = o.value; });
  });
  return map;
}

function gatesPaytable() {
  const rows = GATE_SYMBOLS.map((s, i) => {
    const name = s.name;
    const label = i === 7 ? '×2,34 / ×6,6 / ×24,4' : i === 6 ? '×1,42 / ×3,76 / ×13,2' : i === 5 ? '×0,91 / ×2,84 / ×9,44' : i >= 3 ? '×0,56 / ×1,42 / ×4,67' : '×0,46 / ×1,12 / ×3,76';
    return `<div class="pay-row"><span class="sym"><svg class="pay-sym"><use href="#${s.id}"></use></svg>${name}</span><b class="gold">${label}</b></div>`;
  }).join('');
  return rows;
}

function slotsView() {
  const st = G.slots;
  const grid = st.grid || Array.from({ length: GATE_COLS * GATE_ROWS }, (_, i) => (i * 3 + 5) % 8);
  const orbs = st.orbs || {};
  const res = st.result;
  // Reveal orbs that landed in the last spin even before the animation runs.
  return `${screenHead('Слоты', rtpText())}
  <div class="gates" id="slotsFrame">
    <div class="gates-sky"></div>
    <div class="gates-temple"><svg viewBox="0 0 340 46" preserveAspectRatio="none">
      <path d="M0 44h340v2H0z" fill="rgba(233,199,124,.22)"/>
      <path d="M14 44V20l10-8 10 8v24M306 44V20l10-8 10 8v24M34 44V14h272v30" fill="none" stroke="rgba(233,199,124,.28)" stroke-width="2"/>
      <path d="M34 14L170 2l136 12" fill="none" stroke="rgba(233,199,124,.38)" stroke-width="2"/>
      <path d="M60 44V22M96 44V22M132 44V22M168 44V22M204 44V22M240 44V22M276 44V22" stroke="rgba(233,199,124,.12)" stroke-width="6"/>
    </svg></div>
    <div class="gates-grid" id="slotsGrid">${gatesBoardHtml(grid, orbs)}</div>
    <div class="gates-mult" id="slotsOrbTotal">${st.orbTotal > 1 ? '×' + fmtMult(st.orbTotal) : ''}</div>
  </div>
  <div class="slots-result ${res ? (res.won ? 'green' : 'muted') : 'dim'}" id="slotsResult">${res ? res.text : '8 и более одинаковых символов платят в любом месте поля'}</div>

  <div class="card" style="margin:12px 16px 0">
    ${betBox('slotsBet', st.lastBet || MIN_STARS)}
    <button class="btn gold" style="margin-top:13px" id="slotsSpinBtn" data-act="slots-spin">Крутить</button>
    <div class="notice"><svg class="ic"><use href="#i-bolt"></use></svg><div>Выигрышные символы исчезают, новые падают сверху — серия продолжается. Золотые сферы умножают выигрыш. ${rtpText()}.</div></div>
  </div>

  <details class="paytable card" style="margin:12px 16px 0">
    <summary>Таблица выплат</summary>
    <div class="pay-rows">
      <div class="pay-head">Символ · 8–9 / 10–11 / 12+</div>
      ${gatesPaytable()}
      <div class="pay-row"><span class="sym">Любые 8+ символов</span><b class="gold">платят</b></div>
    </div>
  </details>`;
}

async function spinSlots() {
  const s = G.slots;
  if (s.busy) return;
  const stake = readStake('slotsBet');
  if (!stake) return;
  s.lastBet = $('#slotsBet') ? $('#slotsBet').value : 1;
  const btn = $('#slotsSpinBtn');
  const resEl = $('#slotsResult');
  const frame = $('#slotsFrame');
  if (btn) btn.disabled = true;
  if (resEl) { resEl.className = 'slots-result dim'; resEl.textContent = 'Поле вращается…'; }
  s.busy = true;
  try {
    const r = await api('/play/slots', {
      method: 'POST',
      body: JSON.stringify({ stake, client_seed: uid(), key: uid() }),
    });
    const out = r.outcome || {};
    const initial = Array.isArray(out.initial) ? out.initial : (Array.isArray(out.grid) ? out.grid : []);
    const tumbles = Array.isArray(out.tumbles) ? out.tumbles : [];
    const orbTotal = Number(out.orb_total) || 1;
    const won = r.status === 'won';
    s.grid = initial.slice();
    s.orbs = gatesOrbMap(initial, tumbles);
    s.orbTotal = orbTotal;
    s.result = {
      won,
      text: won
        ? 'Выигрыш +' + fmtStars(r.payout) + ' ' + star('sm') + ' · ×' + fmtMult(r.multiplier)
        : 'Без выигрыша — попробуйте ещё раз',
    };

    // Opening: land the initial board.
    const grid = $('#slotsGrid');
    if (grid) grid.innerHTML = gatesBoardHtml(initial, s.orbs);
    if (frame) frame.classList.add('landing');
    await sleep(420);
    if (frame) frame.classList.remove('landing');

    // Replay every tumble: highlight the winning cells, drop the new board.
    for (let i = 0; i < tumbles.length; i++) {
      const t = tumbles[i];
      const hits = {};
      (t.removed || []).forEach((c) => { hits[c] = true; });
      const el = $('#slotsGrid');
      if (el) el.innerHTML = gatesBoardHtml(s.grid, s.orbs, hits);
      const mult = $('#slotsOrbTotal');
      if (mult && t.orbs && t.orbs.length) {
        mult.textContent = '×' + fmtMult(t.orbs.reduce((a, o) => a + Number(o.value || 0), 0));
        mult.classList.add('pop');
        setTimeout(() => mult.classList.remove('pop'), 420);
      }
      haptic('light');
      await sleep(460);
      s.grid = Array.isArray(t.grid) ? t.grid.slice() : s.grid;
      const el2 = $('#slotsGrid');
      if (el2) {
        el2.innerHTML = gatesBoardHtml(s.grid, s.orbs);
        el2.classList.add('dropin');
      }
      await sleep(30);
      if (el2) el2.classList.remove('dropin');
      await sleep(230);
    }

    const multEl = $('#slotsOrbTotal');
    if (multEl) multEl.textContent = orbTotal > 1 ? '×' + fmtMult(orbTotal) : '';
    S.balance = Number(r.balance) || 0;
    setBalance(S.balance);
    if (won) {
      haptic('success');
      coinBurst(frame);
    } else {
      haptic('light');
    }
    if (resEl) { resEl.className = 'slots-result ' + (won ? 'green' : 'muted'); resEl.innerHTML = s.result.text; }
    loadHistory(true);
  } catch (e) {
    toast(e.message, 'err');
    if (resEl) { resEl.className = 'slots-result dim'; resEl.textContent = '8 и более одинаковых символов платят в любом месте поля'; }
  } finally {
    s.busy = false;
    if (btn) btn.disabled = false;
  }
}

/* ------------------------------ boot ------------------------------ */

boot();
