// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

const $ = (sel, el = document) => el.querySelector(sel);

let walletAddress = "";
let walletUnlocked = false;
let wizardStep = "setup";
let depositAddress = "";
let balances = { dogecoinvm: "0", dogecoin: "0" };
let bridgeLimits = { vmFee: "", dogeFee: "", minDeposit: "", minPegOut: "" };
let lastSendPreview = null;
let lastDepositPreview = null;
let lastWithdrawPreview = null;
let liveBusy = false;
const TOKEN_KEY = "dogevm_token";
const tabTitles = {
  home: "Home",
  wallet: "Wallet",
  activity: "Activity",
  settings: "Settings",
  docs: "API docs",
  learn: "Learn",
};

function browserToken() {
  return sessionStorage.getItem(TOKEN_KEY) || "";
}

function initGate() {
  const box = $("#gate-accept");
  const enter = $("#gate-enter");
  if (!box || !enter) return;
  box.addEventListener("change", () => {
    enter.disabled = !box.checked;
  });
  enter.addEventListener("click", () => {
    if (!box.checked) return;
    localStorage.setItem("dogevm_terms_accepted", "1");
    document.documentElement.classList.add("gate-skip");
  });
}
initGate();

async function api(path, opts = {}) {
  const headers = { "Content-Type": "application/json", ...(opts.headers || {}) };
  const token = browserToken();
  if (token) headers.Authorization = "Bearer " + token;
  const res = await fetch(path, { ...opts, headers });
  const text = await res.text();
  let data;
  try { data = text ? JSON.parse(text) : {}; } catch { data = { error: text }; }
  const banner = $("#auth-banner");
  if (res.status === 401 && data.authRequired && banner) banner.hidden = false;
  if (res.ok && banner && !data.authRequired) banner.hidden = true;
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function show(el, data) {
  if (!el) return;
  el.hidden = false;
  el.textContent = typeof data === "string" ? data : JSON.stringify(data, null, 2);
}

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

function formatDuration(ms) {
  if (ms == null || ms === "" || Number.isNaN(Number(ms))) return "-";
  const n = Number(ms);
  if (n < 1000) return `${Math.round(n)} ms`;
  if (n < 60_000) return `${(n / 1000).toFixed(1)} s`;
  return `${(n / 60_000).toFixed(2)} min`;
}

function formatMult(m) {
  if (m == null || !Number.isFinite(Number(m))) return "-";
  const n = Number(m);
  if (n >= 100) return `~${Math.round(n)}×`;
  if (n >= 10) return `~${n.toFixed(0)}×`;
  return `~${n.toFixed(1)}×`;
}

/** Build structured modal body HTML (escaped). rows: [label, value, opts?] */
function modalContentHTML({ lead = "", rows = [], notes = [], total = null } = {}) {
  const rowHTML = rows.map((row) => {
    const [label, value, opts = {}] = row;
    const cls = opts.mono ? "mono" : (opts.plain ? "" : "num");
    return `<div><dt>${esc(label)}</dt><dd class="${cls}">${esc(value)}</dd></div>`;
  }).join("");
  const totalHTML = total
    ? `<div class="modal-row-total"><dt>${esc(total[0])}</dt><dd class="num">${esc(total[1])}</dd></div>`
    : "";
  return `
    ${lead ? `<p class="modal-lead">${esc(lead)}</p>` : ""}
    ${rows.length || total ? `<dl class="modal-details">${rowHTML}${totalHTML}</dl>` : ""}
    ${(notes || []).map((n) => `<p class="modal-note">${esc(n)}</p>`).join("")}
  `;
}

function previewRows(preview, extras = {}) {
  if (!preview) return { rows: [], total: null, notes: [] };
  const rows = [
    ["Network", networkLabel(preview.chain || extras.chain || "")],
    ["Amount", `${preview.amountDoge} DOGE`],
    ["Network fee", `${preview.feeDoge} DOGE`],
  ];
  if (preview.changeDoge && preview.changeDoge !== "0") {
    rows.push(["Change back", `${preview.changeDoge} DOGE`]);
  }
  if (preview.bridgeFeeDoge || extras.bridgeFee) {
    rows.push(["Bridge fee", `${preview.bridgeFeeDoge || extras.bridgeFee} DOGE`]);
  }
  if (extras.vmFee) {
    rows.push(["Bridge credit fee", `${extras.vmFee} DOGE`]);
  }
  if (preview.sizeBytes) {
    rows.push(["Tx size", `${preview.sizeBytes} bytes`, { plain: true }]);
  }
  if (extras.to) {
    rows.splice(1, 0, ["To", extras.to, { mono: true }]);
  }
  if (extras.from) {
    rows.splice(1, 0, ["From", extras.from, { mono: true }]);
  }
  const totalOut = parseDoge(preview.amountDoge) + parseDoge(preview.feeDoge);
  return {
    rows,
    total: ["Leaves wallet", `${formatDogeNum(totalOut)} DOGE`],
    notes: preview.notes || extras.notes || [],
  };
}

function showModal({ title, body, html, icon = "info", danger = false, confirmLabel = "OK", cancelLabel = null }) {
  return new Promise((resolve) => {
    const root = $("#modal-root");
    const card = root?.querySelector(".modal-card");
    const iconEl = $("#modal-icon");
    const titleEl = $("#modal-title");
    const bodyEl = $("#modal-body");
    const actions = $("#modal-actions");
    if (!root || !card || !actions || !bodyEl) {
      resolve(cancelLabel ? false : true);
      return;
    }
    card.classList.toggle("modal-danger", !!danger);
    iconEl.innerHTML = `<span class="material-symbols-outlined">${esc(icon)}</span>`;
    titleEl.textContent = title || "Notice";
    if (html) {
      bodyEl.classList.remove("plain");
      bodyEl.innerHTML = html;
    } else {
      bodyEl.classList.add("plain");
      bodyEl.textContent = body || "";
    }
    actions.innerHTML = "";

    const finish = (value) => {
      root.hidden = true;
      root.onclick = null;
      document.removeEventListener("keydown", onKey);
      resolve(value);
    };
    const onKey = (ev) => {
      if (ev.key === "Escape") finish(cancelLabel ? false : true);
      if (ev.key === "Enter") finish(true);
    };

    if (cancelLabel) {
      const cancelBtn = document.createElement("button");
      cancelBtn.type = "button";
      cancelBtn.className = "btn ghost";
      cancelBtn.innerHTML = `<span class="material-symbols-outlined">close</span> ${esc(cancelLabel)}`;
      cancelBtn.onclick = () => finish(false);
      actions.appendChild(cancelBtn);
    }
    const okBtn = document.createElement("button");
    okBtn.type = "button";
    okBtn.className = danger ? "btn danger" : "btn primary";
    okBtn.innerHTML = danger
      ? `<span class="material-symbols-outlined">warning</span> ${esc(confirmLabel)}`
      : `<span class="material-symbols-outlined">check</span> ${esc(confirmLabel)}`;
    okBtn.onclick = () => finish(true);
    actions.appendChild(okBtn);

    root.onclick = (ev) => {
      if (ev.target === root) finish(cancelLabel ? false : true);
    };
    card.onclick = (ev) => ev.stopPropagation();

    root.hidden = false;
    document.addEventListener("keydown", onKey);
    okBtn.focus();
  });
}

function uiAlert(message, opts = {}) {
  const html = opts.html || (opts.lead || opts.rows || opts.notes
    ? modalContentHTML({ lead: opts.lead || message, rows: opts.rows || [], notes: opts.notes || [], total: opts.total || null })
    : null);
  return showModal({
    title: opts.title || "Notice",
    body: html ? "" : message,
    html: html || undefined,
    icon: opts.icon || (opts.danger ? "error" : "info"),
    danger: !!opts.danger,
    confirmLabel: opts.okLabel || "OK",
  });
}

function uiConfirm(message, opts = {}) {
  const html = opts.html || (opts.lead || opts.rows || opts.notes || opts.total
    ? modalContentHTML({
      lead: opts.lead || message,
      rows: opts.rows || [],
      notes: opts.notes || [],
      total: opts.total || null,
    })
    : null);
  return showModal({
    title: opts.title || "Confirm",
    body: html ? "" : message,
    html: html || undefined,
    icon: opts.icon || (opts.danger ? "warning" : "help"),
    danger: !!opts.danger,
    confirmLabel: opts.okLabel || "Confirm",
    cancelLabel: opts.cancelLabel || "Cancel",
  });
}

function networkLabel(chain) {
  return chain === "dogecoin" ? "Dogecoin mainnet (day)" : "DogecoinVM / Metal (night)";
}

function dogecoinURI(address, amount) {
  let uri = `dogecoin:${address}`;
  if (amount && Number(amount) > 0) uri += `?amount=${amount}`;
  return uri;
}

function parseDoge(s) {
  const n = Number(String(s || "").trim());
  return Number.isFinite(n) ? n : 0;
}

function formatDogeNum(n) {
  if (!Number.isFinite(n)) return "0";
  let s = n.toFixed(8);
  s = s.replace(/\.?0+$/, "");
  return s === "" ? "0" : s;
}

/** Only digits and a single decimal point. */
function sanitizeAmountInput(raw) {
  let s = String(raw || "").replace(/[^\d.]/g, "");
  const firstDot = s.indexOf(".");
  if (firstDot !== -1) {
    s = s.slice(0, firstDot + 1) + s.slice(firstDot + 1).replace(/\./g, "");
  }
  const parts = s.split(".");
  if (parts[1] && parts[1].length > 8) {
    s = `${parts[0]}.${parts[1].slice(0, 8)}`;
  }
  return s;
}

function bindAmountInputs(root = document) {
  root.querySelectorAll('input[inputmode="decimal"], input.amount').forEach((input) => {
    if (input.dataset.amountBound === "1") return;
    input.dataset.amountBound = "1";
    input.setAttribute("inputmode", "decimal");
    input.setAttribute("autocomplete", "off");
    input.addEventListener("input", () => {
      const next = sanitizeAmountInput(input.value);
      if (input.value !== next) input.value = next;
    });
    input.addEventListener("keydown", (ev) => {
      if (ev.ctrlKey || ev.metaKey || ev.altKey) return;
      const ok = ["Backspace", "Delete", "Tab", "Escape", "Enter", "ArrowLeft", "ArrowRight", "Home", "End"];
      if (ok.includes(ev.key)) return;
      if (ev.key === "." && !input.value.includes(".")) return;
      if (/^\d$/.test(ev.key)) return;
      ev.preventDefault();
    });
    input.addEventListener("paste", (ev) => {
      ev.preventDefault();
      const text = (ev.clipboardData || window.clipboardData).getData("text");
      input.value = sanitizeAmountInput(text);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
  });
}

/** Works on http://localhost without secure-context clipboard API. */
async function copyText(text, btn) {
  const value = String(text || "");
  let ok = false;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(value);
      ok = true;
    }
  } catch { /* fall through */ }
  if (!ok) {
    const ta = document.createElement("textarea");
    ta.value = value;
    ta.setAttribute("readonly", "");
    ta.style.cssText = "position:fixed;left:-9999px;top:0;opacity:0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, value.length);
    try {
      ok = document.execCommand("copy");
    } catch {
      ok = false;
    }
    document.body.removeChild(ta);
  }
  if (btn) {
    const prev = btn.innerHTML;
    btn.classList.add("copied");
    btn.innerHTML = `<span class="material-symbols-outlined">check</span>`;
    setTimeout(() => {
      btn.classList.remove("copied");
      btn.innerHTML = prev;
    }, 1400);
  }
  if (!ok) {
    uiAlert("", {
      title: "Copy",
      icon: "content_copy",
      lead: "Could not copy to the clipboard.",
      notes: ["Select the address text and copy it manually (Ctrl+C / Cmd+C)."],
    });
  }
  return ok;
}

function renderQR(el, text) {
  if (!el) return;
  el.innerHTML = "";
  if (typeof QRCode !== "undefined") {
    // eslint-disable-next-line no-new
    new QRCode(el, {
      text: String(text || ""),
      width: 180,
      height: 180,
      colorDark: "#1a1a1a",
      colorLight: "#ffffff",
      correctLevel: QRCode.CorrectLevel.M,
    });
    return;
  }
  const img = document.createElement("img");
  img.width = 180;
  img.height = 180;
  img.alt = "QR code";
  img.src = `https://api.qrserver.com/v1/create-qr-code/?size=180x180&margin=8&data=${encodeURIComponent(text)}`;
  el.appendChild(img);
}

function addrRowHTML(id, address) {
  return `
    <div class="addr-row">
      <span class="addr" id="${id}">${esc(address || "…")}</span>
      <button type="button" class="btn ghost btn-copy" data-copy-from="${id}" title="Copy address">
        <span class="material-symbols-outlined">content_copy</span>
      </button>
    </div>`;
}

function renderFeeCard(el, preview, extras = {}) {
  if (!el) return;
  if (!preview) {
    el.hidden = true;
    el.innerHTML = "";
    return;
  }
  const rows = [
    ["Amount", `${preview.amountDoge} DOGE`],
    ["Network fee", `${preview.feeDoge} DOGE`],
  ];
  if (preview.changeDoge && preview.changeDoge !== "0") {
    rows.push(["Change back", `${preview.changeDoge} DOGE`]);
  }
  if (preview.bridgeFeeDoge || extras.bridgeFee) {
    rows.push(["Bridge fee", `${preview.bridgeFeeDoge || extras.bridgeFee} DOGE`]);
  }
  if (extras.vmFee) {
    rows.push(["Bridge credit fee", `${extras.vmFee} DOGE`]);
  }
  if (preview.sizeBytes) {
    rows.push(["Tx size", `${preview.sizeBytes} bytes`]);
  }
  const totalOut = parseDoge(preview.amountDoge) + parseDoge(preview.feeDoge);
  el.hidden = false;
  el.innerHTML = `
    <div class="fee-row"><span>Chain</span><strong>${esc(networkLabel(preview.chain || extras.chain || ""))}</strong></div>
    ${rows.map(([k, v]) => `<div class="fee-row"><span>${esc(k)}</span><strong class="num">${esc(v)}</strong></div>`).join("")}
    ${(preview.notes || []).map((n) => `<p class="hint" style="margin:0.35rem 0 0">${esc(n)}</p>`).join("")}
    <div class="fee-total">Leaves wallet ≈ <span class="num">${esc(formatDogeNum(totalOut))}</span> DOGE (amount + network fee)</div>`;
}

async function loadBalances() {
  const bal = await api("/api/balances");
  const vm = bal.chains?.dogecoinvm?.data?.confirmed || "0";
  const doge = bal.chains?.dogecoin?.data?.confirmed || "0";
  balances = { dogecoinvm: vm, dogecoin: doge };
  return balances;
}

/** Update balance labels in-place without rebuilding the wallet wizard. */
function paintBalances() {
  updatePayBalances();
  const l1 = $("#deposit-l1-balance");
  if (l1) l1.textContent = `${balances.dogecoin} DOGE`;
  const balEl = $("#bridge-balances");
  if (balEl && balEl.querySelector(".bal")) {
    const nums = balEl.querySelectorAll(".bal .num");
    if (nums[0]) nums[0].textContent = `${balances.dogecoinvm} DOGE`;
    if (nums[1]) nums[1].textContent = `${balances.dogecoin} DOGE`;
  }
}

function activeTab() {
  return $("#tabs button.active")?.dataset?.tab || "home";
}

async function softRefreshWalletData() {
  if (!walletUnlocked || liveBusy) return;
  liveBusy = true;
  try {
    await loadBalances();
    paintBalances();
    const tab = activeTab();
    if (tab === "activity") {
      await Promise.all([refreshTx(), refreshBridge()]);
    } else if (tab === "wallet" && wizardStep === "bridge") {
      const l1 = $("#deposit-l1-balance");
      if (l1) l1.textContent = `${balances.dogecoin} DOGE`;
    }
  } catch {
    /* ignore transient API errors while live */
  } finally {
    liveBusy = false;
  }
}

function startLiveUpdates() {
  setInterval(() => {
    if (document.hidden) return;
    softRefreshWalletData();
  }, 12_000);

  setInterval(() => {
    if (document.hidden) return;
    if (activeTab() === "activity") {
      refreshTx();
      refreshBridge();
    }
  }, 20_000);

  let tipTimer;
  const onTip = () => {
    clearTimeout(tipTimer);
    tipTimer = setTimeout(() => {
      refreshSpeed();
      refreshNetwork();
      softRefreshWalletData();
    }, 800);
  };

  const connectEvents = () => {
    let es;
    try {
      const token = browserToken();
      const url = token ? "/api/events?token=" + encodeURIComponent(token) : "/api/events";
      es = new EventSource(url);
    } catch {
      return;
    }
    es.onmessage = onTip;
    es.addEventListener("block", onTip);
    es.onerror = () => {
      es.close();
      setTimeout(connectEvents, 5000);
    };
  };
  connectEvents();
}

async function previewSend(chain, to, amount) {
  return api("/api/send", {
    method: "POST",
    body: JSON.stringify({ chain, to, amount: String(amount), preview: true }),
  });
}

/** Largest amount that fits after network fee (uses preview). */
async function spendableMax(chain, to) {
  const bal = parseDoge(balances[chain]);
  if (!(bal > 0) || !to) return "0";
  const probe = Math.min(bal * 0.5, Math.max(bal - 0.001, 0.001));
  if (!(probe > 0)) return "0";
  try {
    const r = await previewSend(chain, to, formatDogeNum(probe));
    const fee = parseDoge(r.preview?.feeDoge);
    let maxAmt = Math.max(0, bal - Math.max(fee, 0.0001));
    for (let i = 0; i < 6; i++) {
      try {
        const r2 = await previewSend(chain, to, formatDogeNum(maxAmt));
        const a = parseDoge(r2.preview.amountDoge);
        const c = parseDoge(r2.preview.changeDoge);
        // If leftover change is large, fold it into amount.
        if (c > 0.00000001) {
          maxAmt = a + c;
          continue;
        }
        return formatDogeNum(a);
      } catch {
        maxAmt = Math.max(0, maxAmt - Math.max(fee * 0.5, 0.001));
      }
    }
    return formatDogeNum(maxAmt);
  } catch (e) {
    throw e;
  }
}

/* Main nav */
function closeSidebar() {
  $("#sidebar")?.classList.remove("open");
  $("#side-backdrop")?.classList.remove("open");
}

document.querySelectorAll("#tabs button").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelectorAll("#tabs button").forEach((b) => b.classList.remove("active"));
    document.querySelectorAll(".panel").forEach((p) => p.classList.remove("active"));
    btn.classList.add("active");
    $(`#tab-${btn.dataset.tab}`).classList.add("active");
    const title = $("#top-title");
    if (title) title.textContent = tabTitles[btn.dataset.tab] || "DogeVM Connect";
    if (btn.dataset.tab === "wallet") refreshWallet();
    if (btn.dataset.tab === "activity") { refreshTx(); refreshBridge(); }
    if (btn.dataset.tab === "settings") loadSettings();
    if (btn.dataset.tab === "docs") fillDocBase();
    closeSidebar();
  });
});
$("#nav-toggle")?.addEventListener("click", () => {
  $("#sidebar")?.classList.toggle("open");
  $("#side-backdrop")?.classList.toggle("open");
});
$("#side-backdrop")?.addEventListener("click", closeSidebar);
document.querySelectorAll("[data-go]").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelector(`#tabs button[data-tab="${btn.dataset.go}"]`)?.click();
  });
});

/* Speedometers */
function drawSpeedo(canvas, gauge, opts = {}) {
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  const w = canvas.width, h = canvas.height;
  ctx.clearRect(0, 0, w, h);

  const cx = w / 2, cy = h * 0.72, r = 108;
  const start = Math.PI * 0.85;
  const end = Math.PI * 0.15 + Math.PI * 2;
  const accent = opts.accent || "#c2a633";
  const accent2 = opts.accent2 || "#8a7420";
  const ink = opts.ink || "#fffaf0";
  const track = opts.track || "rgba(255,250,240,0.12)";

  ctx.lineWidth = 16;
  ctx.strokeStyle = track;
  ctx.beginPath();
  ctx.arc(cx, cy, r, start, end);
  ctx.stroke();

  const grad = ctx.createLinearGradient(0, 0, w, 0);
  grad.addColorStop(0, accent2);
  grad.addColorStop(1, accent);
  const span = end - start;
  const t = Math.max(0, Math.min(1, (gauge || 0) / 100));
  ctx.strokeStyle = grad;
  ctx.beginPath();
  ctx.arc(cx, cy, r, start, start + span * t);
  ctx.stroke();

  const ang = start + span * t;
  ctx.strokeStyle = ink;
  ctx.lineWidth = 3;
  ctx.beginPath();
  ctx.moveTo(cx, cy);
  ctx.lineTo(cx + Math.cos(ang) * (r - 26), cy + Math.sin(ang) * (r - 26));
  ctx.stroke();
  ctx.fillStyle = accent;
  ctx.beginPath();
  ctx.arc(cx, cy, 6, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = ink;
  ctx.font = "700 32px IBM Plex Mono, monospace";
  ctx.textAlign = "center";
  ctx.fillText(`${Math.round(gauge || 0)}`, cx, cy - 26);
  ctx.font = "700 12px Manrope, sans-serif";
  ctx.fillStyle = opts.muted || "rgba(255,250,240,0.65)";
  ctx.fillText(opts.scoreLabel || "speed vs DOGE", cx, cy - 6);
  if (opts.sub) ctx.fillText(opts.sub, cx, cy + 34);
}

function drawSpark(canvas, values, color) {
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  const w = canvas.width, h = canvas.height;
  ctx.clearRect(0, 0, w, h);
  const vals = (values || []).filter((v) => v > 0);
  if (vals.length < 2) return;
  const max = Math.max(...vals);
  const min = Math.min(...vals);
  ctx.strokeStyle = color || "#c2a633";
  ctx.lineWidth = 2;
  ctx.beginPath();
  vals.forEach((v, i) => {
    const x = (i / (vals.length - 1)) * (w - 8) + 4;
    const norm = (v - min) / (max - min || 1);
    const y = 8 + norm * (h - 16);
    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
  });
  ctx.stroke();
}

async function refreshSpeed() {
  try {
    const data = await api("/api/speed");
    const vm = data.dogecoinvm || {};
    const doge = data.dogecoin || {};

    drawSpeedo($("#speedo-vm"), vm.gauge || 0, {
      accent: "#d4b84a", accent2: "#c2a633", ink: "#fffaf0",
      muted: "rgba(255,250,240,0.65)", track: "rgba(255,250,240,0.12)",
      scoreLabel: "speed vs DOGE",
      sub: `${formatDuration(vm.durationMs || vm.medianMs)} · ${formatMult(vm.multiplier)}`,
    });
    $("#vm-mult").textContent = formatMult(vm.multiplier);
    $("#vm-median").textContent = formatDuration(vm.durationMs || vm.medianMs);
    $("#vm-latest").textContent = formatDuration(vm.latestMs);
    $("#vm-p90").textContent = formatDuration(vm.p90Ms);
    $("#vm-hint").textContent = `Takes ${formatDuration(vm.durationMs || vm.medianMs)} to finalize → ${formatMult(vm.multiplier)} vs a ~60 s Dogecoin header.`;
    drawSpark($("#spark-vm"), (vm.samples || []).map((s) => s.medianMs), "#c2a633");

    drawSpeedo($("#speedo-doge"), doge.gauge || 0, {
      accent: "#c2a633", accent2: "#7b3f00", ink: "#1a1a1a",
      muted: "rgba(26,26,26,0.55)", track: "rgba(26,26,26,0.12)",
      scoreLabel: "speed vs DOGE",
      sub: doge.waitingTips
        ? `nominal ~60 s · ${formatMult(1)}`
        : `${formatDuration(doge.durationMs || doge.medianMs)} · ${formatMult(doge.multiplier)}`,
    });
    $("#doge-mult").textContent = formatMult(doge.multiplier);
    $("#doge-median").textContent = formatDuration(doge.durationMs || doge.medianMs);
    $("#doge-latest").textContent = formatDuration(doge.latestMs);
    $("#doge-height").textContent = doge.height != null ? String(doge.height) : "-";
    $("#doge-hint").textContent = doge.waitingTips
      ? "Waiting for tip events. Until then: ~60 s headers ≈ 1× → needle stays low."
      : `Header gap ${formatDuration(doge.durationMs || doge.medianMs)} → ${formatMult(doge.multiplier)}.`;
    drawSpark($("#spark-doge"), (doge.headerGaps || []).map((g) => g.deltaMs), "#7b3f00");

    const tips = (data.tipEvents || []).slice(-8).reverse();
    if (tips.length) {
      $("#tip-list").innerHTML = tips.map((e) => {
        const label = e.chain === "dogecoinvm" ? "DogecoinVM" : "Dogecoin";
        const badge = e.chain === "dogecoinvm" ? "vm" : "doge";
        return `<li><span class="badge ${badge}">${label}</span> tip height ${e.height}</li>`;
      }).join("");
    }
  } catch (e) {
    $("#vm-hint").textContent = e.message;
    $("#doge-hint").textContent = e.message;
  }
}

async function refreshNetwork() {
  try {
    const data = await api("/api/network");
    const st = data.status || {};
    const audit = st.audit || {};
    const dd = $("#net-stats").querySelectorAll("dd");
    dd[0].textContent = st.dogecoinvmHeight ?? "-";
    dd[1].textContent = st.dogecoinHeight ?? "-";
    dd[2].textContent = audit.solvent == null ? "-" : (audit.solvent ? "yes" : "no");
    dd[3].textContent = audit.locked && audit.circulating
      ? `${audit.locked} / ${audit.circulating}`
      : "-";
  } catch (e) {
    console.warn(e);
  }
}

async function postPayment(path, body, outEl) {
  try {
    show(outEl, await api(path, { method: "POST", body: JSON.stringify(body) }));
    softRefreshWalletData();
  } catch (e) {
    show(outEl, { error: e.message });
  }
}

function bindWizardSteps(root, unlocked) {
  root.querySelectorAll("[data-step]").forEach((btn) => {
    btn.addEventListener("click", () => {
      if (!unlocked && btn.dataset.step !== "setup") return;
      wizardStep = btn.dataset.step;
      root.querySelectorAll("[data-step]").forEach((b) => b.classList.toggle("active", b.dataset.step === wizardStep));
      root.querySelectorAll(".wizard-pane").forEach((p) => p.classList.toggle("active", p.dataset.pane === wizardStep));
      if (wizardStep === "pay") {
        updatePayBalances();
        updateReceiveQR();
      }
      if (wizardStep === "bridge") refreshBridgePane();
    });
  });
  root.querySelectorAll("[data-copy-from]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const id = btn.getAttribute("data-copy-from");
      const text = $(`#${id}`)?.textContent?.trim() || "";
      copyText(text, btn);
    });
  });
}

function updatePayBalances() {
  const chain = $("#pay-network")?.value || "dogecoinvm";
  const bal = balances[chain] || "0";
  const el = $("#pay-balance");
  if (el) el.textContent = `${bal} DOGE`;
  const label = $("#recv-network-label");
  if (label) label.textContent = networkLabel(chain);
}

function updateReceiveQR() {
  const chain = $("#pay-network")?.value || "dogecoinvm";
  const amount = $("#recv-amount")?.value || "";
  updatePayBalances();
  if (!walletAddress) return;
  const addrEl = $("#recv-addr");
  if (addrEl) addrEl.textContent = walletAddress;
  // QR encodes payment URI for scanners; UI only shows the plain address.
  renderQR($("#recv-qr"), dogecoinURI(walletAddress, amount));
}

async function refreshSendFee() {
  const form = $("#send-form");
  const feeEl = $("#send-fee");
  if (!form || !feeEl) return;
  const fd = new FormData(form);
  const to = String(fd.get("to") || "").trim();
  const amount = sanitizeAmountInput(fd.get("amount"));
  if (!to || !amount || !(parseDoge(amount) > 0)) {
    renderFeeCard(feeEl, null);
    return;
  }
  try {
    const r = await previewSend($("#pay-network").value, to, amount);
    lastSendPreview = r.preview || null;
    renderFeeCard(feeEl, r.preview);
  } catch (e) {
    lastSendPreview = null;
    feeEl.hidden = false;
    feeEl.innerHTML = `<p class="hint" style="margin:0;color:inherit">${esc(e.message)}</p>`;
  }
}

async function refreshDepositFee() {
  const feeEl = $("#deposit-fee");
  const amount = sanitizeAmountInput($("#deposit-amount")?.value);
  if (!feeEl || !depositAddress || !amount || !(parseDoge(amount) > 0)) {
    lastDepositPreview = null;
    renderFeeCard(feeEl, null);
    return;
  }
  try {
    const r = await previewSend("dogecoin", depositAddress, amount);
    lastDepositPreview = r.preview || null;
    renderFeeCard(feeEl, r.preview, { vmFee: bridgeLimits.vmFee, chain: "dogecoin" });
  } catch (e) {
    lastDepositPreview = null;
    feeEl.hidden = false;
    feeEl.innerHTML = `<p class="hint" style="margin:0;color:inherit">${esc(e.message)}</p>`;
  }
}

async function refreshWithdrawFee() {
  const form = $("#withdraw-form");
  const feeEl = $("#withdraw-fee");
  if (!form || !feeEl) return;
  const fd = new FormData(form);
  const to = String(fd.get("to") || "").trim();
  const amount = sanitizeAmountInput(fd.get("amount"));
  if (!to || !amount || !(parseDoge(amount) > 0)) {
    lastWithdrawPreview = null;
    renderFeeCard(feeEl, null);
    return;
  }
  try {
    const r = await api("/api/bridge/withdraw", {
      method: "POST",
      body: JSON.stringify({ to, amount, preview: true }),
    });
    lastWithdrawPreview = r.preview || null;
    renderFeeCard(feeEl, r.preview, { bridgeFee: r.preview?.bridgeFeeDoge || bridgeLimits.dogeFee });
  } catch (e) {
    lastWithdrawPreview = null;
    feeEl.hidden = false;
    feeEl.innerHTML = `<p class="hint" style="margin:0;color:inherit">${esc(e.message)}</p>`;
  }
}

async function refreshBridgePane() {
  const balEl = $("#bridge-balances");
  if (!balEl) return;
  try {
    await loadBalances();
    balEl.innerHTML = `
      <div class="bal surface night" style="padding:0.85rem">
        <span class="badge vm">DogecoinVM</span>
        <strong class="num">${esc(balances.dogecoinvm)} DOGE</strong>
        <span class="hint">Night / Metal</span>
      </div>
      <div class="bal surface day" style="padding:0.85rem">
        <span class="badge doge">Dogecoin L1</span>
        <strong class="num">${esc(balances.dogecoin)} DOGE</strong>
        <span class="hint">Day / mainnet · available to deposit</span>
      </div>`;
    const l1 = $("#deposit-l1-balance");
    if (l1) l1.textContent = `${balances.dogecoin} DOGE`;
  } catch (e) {
    balEl.innerHTML = `<p class="hint">${esc(e.message)}</p>`;
  }

  try {
    const dep = await api("/api/bridge/deposit-address", { method: "POST", body: "{}" });
    depositAddress = dep.depositAddress || "";
    bridgeLimits = {
      vmFee: dep.limits?.vmFee || bridgeLimits.vmFee,
      dogeFee: bridgeLimits.dogeFee,
      minDeposit: dep.limits?.minDeposit || "",
      minPegOut: bridgeLimits.minPegOut,
    };
    const addrEl = $("#deposit-addr");
    if (addrEl) addrEl.textContent = depositAddress || "-";
    if (depositAddress) renderQR($("#deposit-qr"), dogecoinURI(depositAddress));
    const hint = $("#deposit-limits-hint");
    if (hint && dep.limits) {
      hint.textContent = `Min deposit ${dep.limits.minDeposit} · max ${dep.limits.maxDeposit} · bridge credits minus ${dep.limits.vmFee} DOGE`;
    }
  } catch (e) {
    const addrEl = $("#deposit-addr");
    if (addrEl) addrEl.textContent = e.message;
  }

  try {
    const net = await api("/api/network");
    if (net.info) {
      bridgeLimits.dogeFee = net.info.dogeFee || bridgeLimits.dogeFee;
      bridgeLimits.vmFee = net.info.vmFee || bridgeLimits.vmFee;
      bridgeLimits.minPegOut = net.info.minPegOut || bridgeLimits.minPegOut;
    }
  } catch { /* ignore */ }

  try {
    const data = await api("/api/bridge/deposits");
    const rows = data.deposits || [];
    $("#deposits").innerHTML = rows.length ? `
      <table><thead><tr><th>Status</th><th>Amount</th><th>Conf</th><th>Tx</th></tr></thead>
      <tbody>${rows.map((d) => `<tr>
        <td>${esc(d.status)}</td><td class="num">${esc(d.amount)}</td>
        <td class="num">${esc(d.confirmations)}/${esc(d.required)}</td>
        <td><code>${esc(d.txid?.slice(0, 12))}…</code></td>
      </tr>`).join("")}</tbody></table>` : `<p class="hint">No deposits yet.</p>`;
  } catch (e) {
    $("#deposits").innerHTML = `<p class="hint">${esc(e.message)}</p>`;
  }
}

function unlockedWizardHTML(w) {
  walletAddress = w.address;
  const step = ["pay", "bridge", "setup"].includes(wizardStep) ? wizardStep : "pay";
  wizardStep = step;
  return `
    <div class="wizard-shell">
      <div class="wizard-head">
        <h2><img class="mark mark-sm" src="/logo.png" alt="" /> Wallet wizard</h2>
        <div class="cta-row" style="margin:0">
          <button type="button" class="btn ghost" id="btn-copy"><span class="material-symbols-outlined">content_copy</span> Copy address</button>
          <button type="button" class="btn ghost" id="btn-lock"><span class="material-symbols-outlined">lock</span> Lock</button>
          <button type="button" class="btn ghost" id="btn-export"><span class="material-symbols-outlined">key</span> Export</button>
          <button type="button" class="btn danger" id="btn-remove"><span class="material-symbols-outlined">delete</span> Remove</button>
        </div>
      </div>
      <div class="wizard-steps">
        <button type="button" data-step="pay" class="${step === "pay" ? "active" : ""}"><span class="material-symbols-outlined">sync_alt</span> Send &amp; receive</button>
        <button type="button" data-step="bridge" class="${step === "bridge" ? "active" : ""}"><span class="material-symbols-outlined">swap_horiz</span> Bridge</button>
        <button type="button" data-step="setup" class="${step === "setup" ? "active" : ""}"><span class="material-symbols-outlined">settings</span> Key</button>
      </div>
      <div class="wizard-body">
        <div class="wizard-pane ${step === "pay" ? "active" : ""}" data-pane="pay">
          <div class="surface day">
            <label class="network-picker">Network
              <select id="pay-network">
                <option value="dogecoinvm">DogecoinVM · Metal (night) - final in seconds</option>
                <option value="dogecoin">Dogecoin mainnet (day) - ~1 min / header</option>
              </select>
            </label>
            <div class="balance-line">Available on this network: <strong class="num" id="pay-balance">…</strong></div>
            <p class="hint">Same <code>D…</code> address on both networks. Choose which UTXO set you mean before you send or show a QR.</p>
            <div class="pay-grid">
              <div>
                <h3><span class="material-symbols-outlined">call_received</span> Receive</h3>
                <p class="hint">Scan to pay on <span id="recv-network-label">DogecoinVM / Metal (night)</span></p>
                <div class="qr-box">
                  <div id="recv-qr"></div>
                  ${addrRowHTML("recv-addr", w.address)}
                </div>
                <label style="margin-top:0.75rem">Optional amount in QR (DOGE)
                  <input id="recv-amount" class="amount" inputmode="decimal" placeholder="e.g. 10" />
                </label>
              </div>
              <div>
                <h3><span class="material-symbols-outlined">send</span> Send</h3>
                <form id="send-form" class="form" style="max-width:none;margin-top:0.35rem">
                  <label>To address
                    <input name="to" autocomplete="off" spellcheck="false" placeholder="D…" required />
                  </label>
                  <label>Amount (DOGE)
                    <div class="amount-row">
                      <input name="amount" class="amount" inputmode="decimal" placeholder="1.0" required />
                      <button type="button" class="btn ghost" id="send-max">Max</button>
                    </div>
                  </label>
                  <div id="send-fee" class="fee-card" hidden></div>
                  <div class="cta-row">
                    <button type="button" class="btn ghost" id="send-preview"><span class="material-symbols-outlined">preview</span> Preview fees</button>
                    <button type="submit" class="btn primary"><span class="material-symbols-outlined">check_circle</span> Broadcast</button>
                  </div>
                </form>
                <pre id="send-out" class="out" hidden></pre>
              </div>
            </div>
          </div>
        </div>

        <div class="wizard-pane ${step === "bridge" ? "active" : ""}" data-pane="bridge">
          <div class="bridge-flow">
            <div id="bridge-balances" class="bridge-balances"></div>
            <p class="bridge-arrow"><span class="material-symbols-outlined">swap_horiz</span> Move DOGE day / night</p>
            <div class="grid-2">
              <article class="surface day">
                <h3><span class="material-symbols-outlined">south</span> Day → night (deposit)</h3>
                <p class="hint">Move from your <strong>Dogecoin L1</strong> balance to the bridge deposit address. After confirmations it credits DogecoinVM.</p>
                <div class="balance-line">L1 available: <strong class="num" id="deposit-l1-balance">…</strong></div>
                <div class="qr-box">
                  <div id="deposit-qr"></div>
                  ${addrRowHTML("deposit-addr", "…")}
                </div>
                <p class="hint" id="deposit-limits-hint"></p>
                <form id="deposit-form" class="form" style="max-width:none">
                  <label>Amount to deposit (DOGE)
                    <div class="amount-row">
                      <input id="deposit-amount" name="amount" class="amount" inputmode="decimal" placeholder="1.0" required />
                      <button type="button" class="btn ghost" id="deposit-max">Max</button>
                    </div>
                  </label>
                  <div id="deposit-fee" class="fee-card" hidden></div>
                  <div class="cta-row">
                    <button type="button" class="btn ghost" id="deposit-preview"><span class="material-symbols-outlined">preview</span> Preview fees</button>
                    <button type="submit" class="btn primary"><span class="material-symbols-outlined">south</span> Deposit from L1</button>
                  </div>
                </form>
                <pre id="deposit-out" class="out" hidden></pre>
                <div class="cta-row">
                  <button type="button" class="btn ghost" id="btn-refresh-deposits"><span class="material-symbols-outlined">refresh</span> Refresh deposits</button>
                </div>
                <div id="deposits" class="table-wrap"></div>
              </article>
              <article class="surface night">
                <h3><span class="material-symbols-outlined">north</span> Night → day (withdraw)</h3>
                <p class="hint">Spend DogecoinVM DOGE back to a Dogecoin mainnet address. Network fee + bridge payout fee apply.</p>
                <form id="withdraw-form" class="form" style="max-width:none">
                  <label>Dogecoin payout address
                    <input name="to" placeholder="D…" value="${esc(w.address)}" required />
                  </label>
                  <label>Amount on DogecoinVM (DOGE)
                    <div class="amount-row">
                      <input name="amount" class="amount" inputmode="decimal" placeholder="2.0" required />
                      <button type="button" class="btn ghost" id="withdraw-max">Max</button>
                    </div>
                  </label>
                  <div id="withdraw-fee" class="fee-card" hidden></div>
                  <div class="cta-row">
                    <button type="button" class="btn ghost" id="withdraw-preview"><span class="material-symbols-outlined">preview</span> Preview fees</button>
                    <button type="submit" class="btn primary"><span class="material-symbols-outlined">outgoing_mail</span> Withdraw</button>
                  </div>
                </form>
                <pre id="withdraw-out" class="out" hidden></pre>
              </article>
            </div>
          </div>
        </div>

        <div class="wizard-pane ${step === "setup" ? "active" : ""}" data-pane="setup">
          <div class="surface night">
            <h3><span class="material-symbols-outlined">key</span> Your key</h3>
            <p class="hint">Unlocked · same address on Dogecoin and DogecoinVM</p>
            ${addrRowHTML("setup-addr", w.address)}
            <pre id="export-out" class="out" hidden></pre>
            <p class="hint">Back up offline. Anyone with this key can spend on both chains.</p>
          </div>
        </div>
      </div>
    </div>`;
}

async function refreshWallet() {
  const box = $("#wallet-wizard");
  if (!box) return;
  try {
    const w = await api("/api/wallet");
    if (!w.exists) {
      wizardStep = "setup";
      walletAddress = "";
      walletUnlocked = false;
      box.innerHTML = `
        <div class="wizard-shell">
          <div class="wizard-head">
            <h2><img class="mark mark-sm" src="/logo.png" alt="" /> Get started</h2>
          </div>
          <div class="wizard-body">
            <div class="grid-2">
              <article class="surface night">
                <h3><span class="material-symbols-outlined">add</span> Create a wallet</h3>
                <p class="hint">New Dogecoin key, encrypted on this computer.</p>
                <form id="create-form" class="form">
                  <label>Passphrase
                    <input name="passphrase" type="password" required minlength="8" />
                  </label>
                  <button class="btn primary" type="submit"><span class="material-symbols-outlined">add</span> Create</button>
                </form>
              </article>
              <article class="surface day">
                <h3><span class="material-symbols-outlined">login</span> Import a key</h3>
                <p class="hint">Paste a Dogecoin WIF or 64-char hex private key.</p>
                <form id="import-wallet-form" class="form">
                  <label>Passphrase
                    <input name="passphrase" type="password" required minlength="8" />
                  </label>
                  <label>WIF or hex
                    <input name="secret" autocomplete="off" spellcheck="false" required />
                  </label>
                  <button class="btn primary" type="submit"><span class="material-symbols-outlined">login</span> Import</button>
                </form>
              </article>
            </div>
          </div>
        </div>`;
      $("#create-form").onsubmit = async (ev) => {
        ev.preventDefault();
        const fd = new FormData(ev.target);
        await api("/api/wallet/create", { method: "POST", body: JSON.stringify({ passphrase: fd.get("passphrase") }) });
        wizardStep = "pay";
        refreshWallet();
      };
      $("#import-wallet-form").onsubmit = async (ev) => {
        ev.preventDefault();
        const fd = new FormData(ev.target);
        const secret = String(fd.get("secret")).trim();
        const body = { passphrase: fd.get("passphrase") };
        if (/^[0-9a-fA-F]{64}$/.test(secret)) body.hex = secret;
        else body.wif = secret;
        await api("/api/wallet/import", { method: "POST", body: JSON.stringify(body) });
        wizardStep = "pay";
        refreshWallet();
      };
      return;
    }

    if (!w.unlocked) {
      wizardStep = "setup";
      walletAddress = w.address;
      walletUnlocked = false;
      box.innerHTML = `
        <div class="wizard-shell">
          <div class="wizard-head"><h2>Unlock wallet</h2></div>
          <div class="wizard-body">
            <article class="surface day">
              <p class="hint">Wallet found</p>
              <p class="addr">${esc(w.address)}</p>
              <form id="unlock-form" class="form">
                <label>Passphrase
                  <input name="passphrase" type="password" required />
                </label>
                <button class="btn primary" type="submit"><span class="material-symbols-outlined">lock_open</span> Unlock</button>
              </form>
            </article>
          </div>
        </div>`;
      $("#unlock-form").onsubmit = async (ev) => {
        ev.preventDefault();
        const fd = new FormData(ev.target);
        await api("/api/wallet/unlock", { method: "POST", body: JSON.stringify({ passphrase: fd.get("passphrase") }) });
        wizardStep = "pay";
        refreshWallet();
      };
      return;
    }

    walletUnlocked = true;
    walletAddress = w.address;
    box.innerHTML = unlockedWizardHTML(w);
    bindWizardSteps(box, true);
    bindAmountInputs(box);

    try { await loadBalances(); } catch { /* ignore */ }
    paintBalances();
    updateReceiveQR();

    $("#btn-copy").onclick = () => copyText(w.address, $("#btn-copy"));
    $("#btn-lock").onclick = async () => {
      await api("/api/wallet/lock", { method: "POST", body: "{}" });
      walletUnlocked = false;
      wizardStep = "setup";
      refreshWallet();
    };
    $("#btn-export").onclick = async () => {
      const ok = await uiConfirm("", {
        title: "Export private key",
        icon: "key",
        danger: true,
        okLabel: "Show key",
        lead: "Show the private key on this screen?",
        notes: [
          "Anyone who sees this key can spend your DOGE on Dogecoin and DogecoinVM.",
          "Only continue if nobody can see your screen, then store the key offline.",
        ],
      });
      if (!ok) return;
      wizardStep = "setup";
      box.querySelectorAll("[data-step]").forEach((b) => b.classList.toggle("active", b.dataset.step === "setup"));
      box.querySelectorAll(".wizard-pane").forEach((p) => p.classList.toggle("active", p.dataset.pane === "setup"));
      show($("#export-out"), await api("/api/wallet/export", { method: "POST", body: "{}" }));
    };
    $("#btn-remove").onclick = async () => {
      const ok = await uiConfirm("", {
        title: "Remove wallet",
        icon: "delete",
        danger: true,
        okLabel: "Remove",
        lead: "Delete the encrypted wallet file from this computer?",
        rows: walletAddress ? [["Address", walletAddress, { mono: true }]] : [],
        notes: [
          "This does not delete coins on-chain. You can recover with a backed-up key.",
          "Without a backup, funds tied to this key are lost forever.",
        ],
      });
      if (!ok) return;
      await api("/api/wallet/remove", { method: "POST", body: "{}" });
      walletUnlocked = false;
      wizardStep = "setup";
      refreshWallet();
    };

    $("#pay-network").onchange = async () => {
      updatePayBalances();
      updateReceiveQR();
      await refreshSendFee();
    };
    $("#recv-amount").oninput = updateReceiveQR;

    let sendFeeTimer;
    const scheduleSendFee = () => {
      clearTimeout(sendFeeTimer);
      sendFeeTimer = setTimeout(refreshSendFee, 350);
    };
    $("#send-form").querySelector('[name="to"]').addEventListener("input", scheduleSendFee);
    $("#send-form").querySelector('[name="amount"]').addEventListener("input", scheduleSendFee);

    $("#send-max").onclick = async () => {
      const to = $("#send-form").querySelector('[name="to"]').value.trim() || w.address;
      try {
        const max = await spendableMax($("#pay-network").value, to);
        $("#send-form").querySelector('[name="amount"]').value = max;
        await refreshSendFee();
      } catch (e) {
        uiAlert("", {
          title: "Max amount",
          icon: "error",
          danger: true,
          lead: "Could not estimate a max spendable amount.",
          notes: [e.message],
        });
      }
    };
    $("#send-preview").onclick = () => refreshSendFee();
    $("#send-form").onsubmit = async (ev) => {
      ev.preventDefault();
      const fd = new FormData(ev.target);
      const chain = $("#pay-network").value;
      const to = String(fd.get("to") || "").trim();
      const amount = sanitizeAmountInput(fd.get("amount"));
      await refreshSendFee();
      const detail = previewRows(lastSendPreview, { chain, to });
      const ok = await uiConfirm("", {
        title: "Confirm send",
        icon: "send",
        okLabel: "Broadcast",
        lead: `Broadcast this payment on ${networkLabel(chain)}?`,
        rows: detail.rows.length ? detail.rows : [
          ["Network", networkLabel(chain)],
          ["To", to, { mono: true }],
          ["Amount", `${amount} DOGE`],
        ],
        total: detail.total,
        notes: detail.notes.length
          ? detail.notes
          : ["Signed locally - your private key stays on this computer."],
      });
      if (!ok) return;
      postPayment("/api/send", {
        chain, to, amount, preview: false,
      }, $("#send-out"));
    };

    let depositFeeTimer;
    const scheduleDepositFee = () => {
      clearTimeout(depositFeeTimer);
      depositFeeTimer = setTimeout(refreshDepositFee, 350);
    };
    $("#deposit-amount")?.addEventListener("input", scheduleDepositFee);
    $("#deposit-max").onclick = async () => {
      if (!depositAddress) {
        await refreshBridgePane();
      }
      if (!depositAddress) {
        uiAlert("", {
          title: "Bridge deposit",
          icon: "hourglass_top",
          lead: "Deposit address not ready yet.",
          notes: ["Open the Bridge step again in a moment, or check your Metal API connection."],
        });
        return;
      }
      try {
        await loadBalances();
        const max = await spendableMax("dogecoin", depositAddress);
        $("#deposit-amount").value = max;
        await refreshDepositFee();
      } catch (e) {
        uiAlert("", {
          title: "Max deposit",
          icon: "error",
          danger: true,
          lead: "Could not estimate a max deposit.",
          notes: [e.message],
        });
      }
    };
    $("#deposit-preview").onclick = () => refreshDepositFee();
    $("#deposit-form").onsubmit = async (ev) => {
      ev.preventDefault();
      if (!depositAddress) {
        uiAlert("", {
          title: "Bridge deposit",
          icon: "hourglass_top",
          lead: "Deposit address not ready yet.",
          notes: ["Open the Bridge step again in a moment, or check your Metal API connection."],
        });
        return;
      }
      const amount = sanitizeAmountInput($("#deposit-amount").value);
      await refreshDepositFee();
      const detail = previewRows(lastDepositPreview, {
        chain: "dogecoin",
        to: depositAddress,
        vmFee: bridgeLimits.vmFee,
        notes: [
          "This spends from Dogecoin L1 (day) to your personal deposit address.",
          "DOGE sent to your normal D… wallet address stays on L1 and does not bridge.",
        ],
      });
      const okDep = await uiConfirm("", {
        title: "Confirm deposit",
        icon: "south",
        okLabel: "Deposit",
        lead: "Move DOGE from Dogecoin mainnet into DogecoinVM via the bridge?",
        rows: detail.rows.length ? detail.rows : [
          ["From", "Dogecoin L1 (day)"],
          ["Deposit address", depositAddress, { mono: true }],
          ["Amount", `${amount} DOGE`],
        ],
        total: detail.total,
        notes: detail.notes,
      });
      if (!okDep) return;
      try {
        const r = await api("/api/send", {
          method: "POST",
          body: JSON.stringify({ chain: "dogecoin", to: depositAddress, amount, preview: false }),
        });
        show($("#deposit-out"), r);
        await refreshBridgePane();
      } catch (e) {
        show($("#deposit-out"), { error: e.message });
      }
    };

    $("#btn-refresh-deposits").onclick = refreshBridgePane;

    let withdrawFeeTimer;
    const scheduleWithdrawFee = () => {
      clearTimeout(withdrawFeeTimer);
      withdrawFeeTimer = setTimeout(refreshWithdrawFee, 350);
    };
    $("#withdraw-form").querySelector('[name="to"]').addEventListener("input", scheduleWithdrawFee);
    $("#withdraw-form").querySelector('[name="amount"]').addEventListener("input", scheduleWithdrawFee);
    $("#withdraw-max").onclick = async () => {
      const to = $("#withdraw-form").querySelector('[name="to"]').value.trim() || w.address;
      try {
        await loadBalances();
        // Withdraw builds a special tx; use VM balance minus a small buffer via iterative preview.
        let amt = parseDoge(balances.dogecoinvm);
        for (let i = 0; i < 10 && amt > 0; i++) {
          try {
            const r = await api("/api/bridge/withdraw", {
              method: "POST",
              body: JSON.stringify({ to, amount: formatDogeNum(amt), preview: true }),
            });
            $("#withdraw-form").querySelector('[name="amount"]').value = r.preview.amountDoge;
            renderFeeCard($("#withdraw-fee"), r.preview, { bridgeFee: r.preview.bridgeFeeDoge });
            return;
          } catch {
            amt = Math.max(0, amt - 0.01);
          }
        }
        uiAlert("", {
          title: "Max withdraw",
          icon: "error",
          danger: true,
          lead: "Could not estimate a max withdrawal for the current balance.",
          notes: ["Try a slightly smaller amount, or preview fees after adjusting the destination."],
        });
      } catch (e) {
        uiAlert("", {
          title: "Max withdraw",
          icon: "error",
          danger: true,
          lead: "Could not estimate a max withdrawal.",
          notes: [e.message],
        });
      }
    };
    $("#withdraw-preview").onclick = () => refreshWithdrawFee();
    $("#withdraw-form").onsubmit = async (ev) => {
      ev.preventDefault();
      const fd = new FormData(ev.target);
      const to = String(fd.get("to") || "").trim();
      const amount = sanitizeAmountInput(fd.get("amount"));
      await refreshWithdrawFee();
      const detail = previewRows(lastWithdrawPreview, {
        chain: "dogecoinvm",
        to,
        bridgeFee: lastWithdrawPreview?.bridgeFeeDoge || bridgeLimits.dogeFee,
        notes: [
          "Broadcasts on DogecoinVM (night). Peg-out credits Dogecoin L1 after bridge processing.",
        ],
      });
      const okW = await uiConfirm("", {
        title: "Confirm withdraw",
        icon: "north",
        okLabel: "Withdraw",
        lead: "Withdraw DOGE from DogecoinVM back toward Dogecoin mainnet?",
        rows: detail.rows.length ? detail.rows : [
          ["From", "DogecoinVM (night)"],
          ["To", to, { mono: true }],
          ["Amount", `${amount} DOGE`],
        ],
        total: detail.total,
        notes: detail.notes,
      });
      if (!okW) return;
      postPayment("/api/bridge/withdraw", { to, amount, preview: false }, $("#withdraw-out"));
    };

    if (wizardStep === "bridge") refreshBridgePane();
    else if (wizardStep === "pay") refreshSendFee();
  } catch (e) {
    box.innerHTML = `<article class="surface day"><p class="hint">${esc(e.message)}</p></article>`;
  }
}

async function refreshTx() {
  try {
    const data = await api("/api/transactions");
    const rows = data.transactions || [];
    $("#tx-table").innerHTML = rows.length ? `
      <table><thead><tr><th>Chain</th><th>Net</th><th>Conf</th><th>Txid</th></tr></thead>
      <tbody>${rows.map((t) => {
        const badge = t.chain === "dogecoinvm" ? "vm" : "doge";
        const label = t.chain === "dogecoinvm" ? "VM" : "L1";
        return `<tr>
          <td><span class="badge ${badge}">${label}</span></td>
          <td>${esc(t.net)}</td><td>${esc(t.confirmations)}</td>
          <td><code title="${esc(t.txid)}">${esc(t.txid?.slice(0, 14))}…</code></td>
        </tr>`;
      }).join("")}</tbody></table>
      <p class="hint">${esc(data.transparency || "")}</p>` : `<p class="hint">No history yet. Unlock the wallet and fund an address.</p>`;
  } catch (e) {
    $("#tx-table").innerHTML = `<p class="hint">${esc(e.message)}</p>`;
  }
}

async function refreshBridge() {
  try {
    const data = await api("/api/bridge/activity");
    const rows = data.activity || [];
    $("#bridge-table").innerHTML = rows.length ? `
      <table><thead><tr><th>Type</th><th>Status</th><th>Amount</th><th>Link</th></tr></thead>
      <tbody>${rows.map((a) => `<tr>
        <td>${esc(a.type)}</td><td>${esc(a.status)}</td><td>${esc(a.amount)}</td>
        <td><code>${esc((a.dogecoinvmTxid || a.dogecoinTxid || "").slice(0, 12))}…</code></td>
      </tr>`).join("")}</tbody></table>` : `<p class="hint">No recent bridge activity.</p>`;
  } catch (e) {
    $("#bridge-table").innerHTML = `<p class="hint">${esc(e.message)}</p>`;
  }
}

$("#btn-refresh-tx")?.addEventListener("click", refreshTx);
$("#btn-refresh-bridge")?.addEventListener("click", refreshBridge);
$("#import-form")?.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  try {
    await api("/api/doge/import", { method: "POST", body: JSON.stringify({ txid: fd.get("txid") }) });
    refreshTx();
  } catch (e) {
    uiAlert("", {
      title: "Import payment",
      icon: "error",
      danger: true,
      lead: "Could not import that payment.",
      notes: [e.message],
    });
  }
});

function rememberToken(token) {
  if (token) sessionStorage.setItem(TOKEN_KEY, token);
  else sessionStorage.removeItem(TOKEN_KEY);
}

async function loadSettings() {
  const status = $("#settings-status");
  try {
    const cfg = await api("/api/config");
    if ($("#set-user")) $("#set-user").value = cfg.user_token || "";
    if ($("#set-admin")) $("#set-admin").value = cfg.admin_token || "";
    if (status) {
      status.textContent = cfg.authRequired
        ? "Tokens are on. Calls need a user or admin token."
        : "Tokens are off. This computer can call the API without a token.";
    }
  } catch (e) {
    if (status) status.textContent = e.message;
  }
  if ($("#set-browser")) $("#set-browser").value = browserToken();
}

$("#settings-form")?.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const user_token = $("#set-user")?.value.trim() || "";
  const admin_token = $("#set-admin")?.value.trim() || "";
  try {
    await api("/api/config", {
      method: "POST",
      body: JSON.stringify({ user_token, admin_token }),
    });
    if (admin_token) rememberToken(admin_token);
    else if (!user_token) rememberToken("");
    await loadSettings();
    uiAlert("", {
      title: "Tokens saved",
      icon: "check",
      lead: admin_token
        ? "This browser will send the admin token on later calls."
        : "Open access is on. No token is required.",
    });
  } catch (e) {
    uiAlert("", {
      title: "Could not save tokens",
      icon: "error",
      danger: true,
      lead: "The tokens were not saved.",
      notes: [e.message],
    });
  }
});

$("#browser-token-form")?.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  rememberToken($("#set-browser")?.value.trim() || "");
  try {
    await api("/api/wallet");
    const banner = $("#auth-banner");
    if (banner) banner.hidden = true;
    uiAlert("", {
      title: "Browser token",
      icon: "key",
      lead: browserToken()
        ? "This browser will send that token."
        : "This browser will not send a token.",
    });
  } catch (e) {
    uiAlert("", {
      title: "Browser token",
      icon: "error",
      danger: true,
      lead: "That token was not accepted.",
      notes: [e.message],
    });
  }
});

$("#auth-banner-form")?.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  rememberToken(String(fd.get("token") || "").trim());
  try {
    await api("/api/wallet");
    const banner = $("#auth-banner");
    if (banner) banner.hidden = true;
    refreshNetwork();
    refreshWallet();
  } catch (e) {
    uiAlert("", {
      title: "Token",
      icon: "error",
      danger: true,
      lead: "That token was not accepted.",
      notes: [e.message],
    });
  }
});

let docBaseFilled = false;
function fillDocBase() {
  if (docBaseFilled) return;
  const base = location.origin;
  document.querySelectorAll("[data-base]").forEach((el) => {
    el.innerHTML = el.innerHTML.replaceAll("http://127.0.0.1:8787", base);
  });
  const label = $("#doc-base");
  if (label) label.textContent = base;
  docBaseFilled = true;
}

document.querySelectorAll("[data-copy]").forEach((btn) => {
  btn.addEventListener("click", () => {
    const pre = btn.parentElement?.querySelector("pre");
    if (pre) copyText(pre.innerText, btn);
  });
});

drawSpeedo($("#speedo-vm"), 95, {
  accent: "#d4b84a", accent2: "#c2a633", ink: "#fffaf0",
  muted: "rgba(255,250,240,0.65)", track: "rgba(255,250,240,0.12)",
  scoreLabel: "speed vs DOGE", sub: "~200 ms · ~300×",
});
drawSpeedo($("#speedo-doge"), 12, {
  accent: "#c2a633", accent2: "#7b3f00", ink: "#1a1a1a",
  muted: "rgba(26,26,26,0.55)", track: "rgba(26,26,26,0.12)",
  scoreLabel: "speed vs DOGE", sub: "~60 s · ~1×",
});
refreshSpeed();
refreshNetwork();
refreshWallet();
setInterval(refreshSpeed, 5000);
setInterval(refreshNetwork, 15000);
startLiveUpdates();
