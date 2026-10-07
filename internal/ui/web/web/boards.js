// The Boards tab: a tab of its own for each board, with its profiles, and the board drawn or
// listed with what each control does.
import { send, t } from "./bridge.js";
import { boardSVG, showValue, smcButtonIcon, smcSVG } from "./device.js";
import { actionPicks, buttonLines, buttonParams, controlLabel, jobLines, jobPicks, shortcutControl } from "./pickers.js";
import {
  S,
  actionsOf,
  activeProfile,
  alsoUsedBy,
  buttonKey,
  clampIndex,
  clip,
  controlName,
  controlOfKey,
  currentBoard,
  devices,
  esc,
  escAttr,
  isButton,
  isSMC,
  jobsOf,
  kindOf,
  profileLabel,
  statusOf,
  valueAt,
} from "./state.js";

const litUntil = new Map();
const litTimers = new Map();

export function picked(d) {
  const sel = S.selected[d.id];
  if (isSMC(d)) return sel === undefined ? 0 : sel;
  return Math.max(0, Math.min(sel || 0, d.controls.length - 1));
}

export function select(d, k) {
  if (picked(d) === k) return;
  S.selected[d.id] = k;
  S.render();
}

function lit(d) {
  return (id) => (litUntil.get(d.id + ":" + id) || 0) > Date.now();
}

export function statusText(d) {
  if (!d.enabled) return { text: t("status.off"), cls: "" };
  const s = statusOf(d.id);
  if (s.connected) return { text: t("status.connected"), cls: " status-ok" };
  if (s.busy) return { text: t("port_busy", { port: s.port }), cls: " warning" };
  return { text: t("status.disconnected"), cls: " muted" };
}

export function renderBoards() {
  const panel = document.getElementById("panel");
  if (!devices().length) {
    panel.innerHTML = `
    <div class="tabpanel" role="tabpanel">
      <div class="empty-state">
        <p>${esc(t("boards.none"))}</p>
        <button class="btn btn-primary" type="button" data-action="switch-tab" data-tab="general">${esc(t("boards.set_up"))}</button>
      </div>
    </div>`;
    return;
  }
  const d = currentBoard();
  const tabs = devices()
    .map((b) => `<button class="subtab" type="button" role="tab" data-action="board-tab" data-board="${escAttr(b.id)}" aria-selected="${b.id === d.id}">${esc(clip(b.name, 24))}</button>`)
    .join("");
  const st = statusText(d);
  const view = d.view === "list" ? "list" : "draw";
  const viewSwitch = ["draw", "list"]
    .map((v) => `<button class="btn${v === view ? " on" : ""}" type="button" data-action="set-view" data-view="${v}" aria-pressed="${v === view}">${esc(t("view." + v))}</button>`)
    .join("");
  const calibrate = isSMC(d) ? "" : `<button class="btn" type="button" data-action="calibrate-board">${esc(t("calibrate"))}</button>`;
  const head = `
      <div class="board-bar">
        <span class="row-desc${st.cls}">${esc(st.text)}</span>
        <span class="spacer"></span>
        ${d.enabled ? `<span class="segmented">${viewSwitch}</span>${calibrate}` : ""}
        <button class="btn btn-icon" type="button" data-action="gear" data-board="${escAttr(d.id)}" title="${escAttr(t("boards.settings"))}" aria-label="${escAttr(t("boards.settings"))}">${GEAR_ICON}</button>
      </div>`;
  let body;
  if (!d.enabled) body = `<div class="empty-state"><p>${esc(t("boards.off"))}</p></div>`;
  else if (view === "list") body = listView(d);
  else body = drawView(d);
  panel.innerHTML = `
    <div class="tabpanel" role="tabpanel">
      <div class="subtabs" role="tablist">${tabs}</div>
      <div class="general-grid">
        ${profileCard(d)}
        <div class="group">${head}</div>
      </div>
      ${body}
    </div>`;
}

export const GEAR_ICON = `<svg class="gear" viewBox="0 0 16 16" aria-hidden="true"><path d="M6.6 1h2.8l.4 1.9 1.1.6 1.8-.7 1.4 2.4-1.4 1.3v1.2l1.4 1.3-1.4 2.4-1.8-.7-1.1.6-.4 1.9H6.6l-.4-1.9-1.1-.6-1.8.7-1.4-2.4 1.4-1.3V7.8L1.9 6.5l1.4-2.4 1.8.7 1.1-.6z"/><circle cx="8" cy="8" r="2.2"/></svg>`;

function profileCard(d) {
  const p = activeProfile(d);
  const i = clampIndex(d.profile, d.profiles.length);
  const options = d.profiles
    .map((_, n) => `<option value="${n}"${n === i ? " selected" : ""}>${esc(clip(profileLabel(d, n), 30))}</option>`)
    .join("");
  const others = alsoUsedBy(p.shortcut, d.id);
  return `
      <div class="group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("profile"))}</h2>
          <select class="select select-inline" id="profile-select">${options}</select>
          <span class="spacer"></span>
          <span class="segmented">
            <button class="btn btn-icon" type="button" data-action="add-profile" title="${escAttr(t("add_profile"))}" aria-label="${escAttr(t("add_profile"))}">+</button>
            <button class="btn btn-icon" type="button" data-action="remove-profile" title="${escAttr(t("remove_profile"))}" aria-label="${escAttr(t("remove_profile"))}"${d.profiles.length <= 1 ? " disabled" : ""}>&minus;</button>
          </span>
        </div>
        <div class="card">
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("name"))}</span></div>
            <div class="row-control">
              <input class="input input-name" id="profile-name" type="text" value="${escAttr(p.name)}" placeholder="${escAttr(t("profile_n", { n: String(i + 1) }))}" />
            </div>
          </div>
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("shortcut"))}</span>
              ${others.length ? `<span class="row-desc">${esc(t("shortcut.also_used", { names: others.join(", ") }))}</span>` : ""}
            </div>
            <div class="row-control">${shortcutControl("profile:" + i, p.shortcut, d.id)}</div>
          </div>
        </div>
      </div>`;
}

// --- Draw --------------------------------------------------------------------

function drawView(d) {
  const k = picked(d);
  const opts = {
    name: (id) => controlName(d, id),
    label: (id) => controlLabel(d, id),
    selected: k,
    lit: lit(d),
  };
  let svg;
  if (isSMC(d)) {
    svg = smcSVG({
      ...opts,
      assigned: (id) => actionsOf(d, id).length > 0,
      value: (id) => (id < 16 ? valueAt(d, id) : -1),
    });
  } else {
    svg = boardSVG({
      ...opts,
      layout: boardLayout(d),
      kind: (c) => kindOf(d, c),
      assigned: (c) => isButton(d, c) && actionsOf(d, buttonKey(d, c)).length > 0,
      waiting: (c) => d.controls[c].input < 0,
      value: (c) => valueAt(d, c),
      addLabel: t("board.add"),
    });
  }
  return `
      <div class="device-row">
        <div class="group">
          <div class="card device-card">${svg}</div>
        </div>
        ${isSMC(d) ? smcInspector(d, k) : boardInspector(d, k)}
        <div class="group-foot device-foot"><span class="group-note">${esc(t(isSMC(d) ? "smc.hint" : "board.hint"))}</span></div>
      </div>`;
}

function smcInspector(d, id) {
  const button = isButton(d, id);
  const empty = button ? !actionsOf(d, id).length : !jobsOf(d, id).length;
  return `
        <div class="group inspector">
          <div class="group-head">
            ${button ? smcButtonIcon(id) : ""}<h2 class="group-title">${esc(controlName(d, id))}</h2>
            <span class="spacer"></span>
            <button class="btn" type="button" data-action="clear-control"${empty ? " disabled" : ""}>${esc(t("clear"))}</button>
          </div>
          <div class="card inspector-card">${button ? actionPicks(d, id) : jobPicks(d, id)}</div>
        </div>`;
}

function boardInspector(d, k) {
  if (!d.controls.length) {
    return `
        <div class="group inspector">
          <div class="card inspector-card"><p class="inspector-empty">${esc(t("board.empty"))}</p></div>
        </div>`;
  }
  const kind = kindOf(d, k);
  const button = kind === "button";
  const key = buttonKey(d, k);
  const empty = button ? !actionsOf(d, key).length : !jobsOf(d, k).length;
  const kinds = ["knob", "fader", "button"]
    .map((kd) => `<button class="btn${kd === kind ? " on" : ""}" type="button" data-action="set-kind" data-kind="${kd}" aria-pressed="${kd === kind}">${esc(t("board.kind_" + kd))}</button>`)
    .join("");
  const found = d.controls[k].input >= 0;
  const input = found
    ? `<span class="row-desc">${esc(t("board.found"))}</span><button class="btn btn-icon btn-subtle" type="button" data-action="find-control" title="${escAttr(t("board.find_again"))}" aria-label="${escAttr(t("board.find_again"))}">&#x21bb;</button>`
    : `<span class="waiting-note">${esc(t("board.not_found"))}</span><button class="btn" type="button" data-action="find-control">${esc(t("board.find"))}</button>`;
  const moves = [
    ["up", "&#x2191;"],
    ["left", "&#x2190;"],
    ["down", "&#x2193;"],
    ["right", "&#x2192;"],
  ]
    .map(([dir, arrow]) => `<button class="btn btn-icon" type="button" data-action="move-control" data-dir="${dir}" title="${escAttr(t("board." + dir))}" aria-label="${escAttr(t("board." + dir))}"${canMove(d, k, dir) ? "" : " disabled"}>${arrow}</button>`)
    .join("");
  return `
        <div class="group inspector">
          <div class="group-head">
            <h2 class="group-title">${esc(controlName(d, k))}</h2>
            <span class="spacer"></span>
            <button class="btn" type="button" data-action="clear-control"${empty ? " disabled" : ""}>${esc(t("clear"))}</button>
            <button class="btn btn-icon" type="button" data-action="remove-control" title="${escAttr(t("remove"))}" aria-label="${escAttr(t("remove"))}">&minus;</button>
          </div>
          <div class="card board-edit">
            <div class="edit-main">
              <div class="edit-row"><span class="segmented">${kinds}</span></div>
              <div class="edit-row">${input}</div>
            </div>
            <div class="arrow-keys">${moves}</div>
          </div>
          <div class="card inspector-card">${button ? actionPicks(d, key) : jobPicks(d, k)}</div>
        </div>`;
}

// --- List --------------------------------------------------------------------

function listView(d) {
  const of = (kind) => d.controls.map((_, k) => k).filter((k) => kindOf(d, k) === kind);
  const knobs = of("knob");
  const faders = of("fader");
  const potRows = (list) =>
    list.length
      ? list
          .map(
            (k) => `
            <div class="row clickable" id="row-${k}" role="button" tabindex="0" data-action="open-job-menu" data-control="${k}">
              <div class="row-main"><span class="row-title">${esc(controlName(d, k))}</span></div>
              <div class="row-control"><div class="job-list">${jobLines(jobsOf(d, k))}</div><span class="chev" aria-hidden="true">&#x2304;</span></div>
            </div>`
          )
          .join("")
      : `<div class="row"><span class="row-desc">${esc(t("list.none"))}</span></div>`;
  const keys = isSMC(d) ? S.init.smcButtons || [] : of("button").map((k) => buttonKey(d, k)).filter((key) => key !== undefined && key >= 0);
  const buttonRows = keys.length
    ? keys
        .map(
          (key) => `
            <div class="row clickable" id="brow-${key}" role="button" tabindex="0" data-action="open-button-menu" data-key="${key}">
              <div class="row-main"><span class="row-title">${esc(controlName(d, controlOfKey(d, key)))}</span>${buttonParams(d, key)}</div>
              <div class="row-control"><div class="job-list">${buttonLines(d, key)}</div><span class="chev" aria-hidden="true">&#x2304;</span></div>
            </div>`
        )
        .join("")
    : `<div class="row"><span class="row-desc">${esc(t("list.none"))}</span></div>`;
  const card = (title, rows) => `
        <div class="group list-group">
          <div class="group-head"><h2 class="group-title">${esc(title)}</h2></div>
          <div class="card">${rows}</div>
        </div>`;
  return `
      <div class="list-columns">
        ${card(t("knobs"), potRows(knobs))}
        ${card(t("list.faders"), potRows(faders))}
        ${card(t("list.buttons"), buttonRows)}
      </div>`;
}

// --- Live --------------------------------------------------------------------

function showing(d) {
  return S.tab === "boards" && currentBoard() && currentBoard().id === d.id && d.enabled;
}

// Moves the drawn controls to a board's new values.
export function showValues(id, values) {
  const before = S.live[id] || [];
  S.live[id] = values;
  const d = currentBoard();
  if (!d || d.id !== id || !showing(d) || d.view === "list") return;
  values.forEach((v, k) => {
    if (before[k] !== v) showValue(document, k, v);
  });
}

// A control that moved or was pressed lights for a second, and is picked unless something is
// being typed or recorded.
export function touched(id, k) {
  const d = devices().find((b) => b.id === id);
  if (!d || k < 0 || !showing(d)) return;
  if (d.view === "list") {
    lightRow(document.getElementById(isButton(d, k) ? `brow-${buttonKey(d, k)}` : `row-${k}`));
    return;
  }
  const el = document.activeElement;
  const typing = el && (el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && el.type === "text"));
  if (k !== picked(d) && !S.recording && !typing && !S.dialog) {
    if (el && el.closest && el.closest(".inspector")) el.blur();
    select(d, k);
  }
  const key = d.id + ":" + k;
  litUntil.set(key, Date.now() + 1000);
  document.getElementById(`ctl-${k}`)?.classList.add("lit");
  clearTimeout(litTimers.get(key));
  litTimers.set(key, setTimeout(() => document.getElementById(`ctl-${k}`)?.classList.remove("lit"), 1000));
}

export function pressed(id, key) {
  const d = devices().find((b) => b.id === id);
  if (d) touched(id, controlOfKey(d, key));
}

const rowTimers = new WeakMap();

function lightRow(row) {
  if (!row) return;
  if (!row.classList.contains("lit")) {
    row.classList.add("lit");
    row.scrollIntoView({ block: "nearest" });
  }
  clearTimeout(rowTimers.get(row));
  rowTimers.set(row, setTimeout(() => row.classList.remove("lit"), 1000));
}

// --- Layout ------------------------------------------------------------------

// The saved layout made drawable, as core.CleanLayout does: one row of every control when there
// is none, and any control it misses on the last row.
export function boardLayout(d) {
  const n = d.controls.length;
  const seen = new Set();
  const rows = (d.layout || [Array.from({ length: n }, (_, i) => i)])
    .map((row) => row.filter((i) => i >= 0 && i < n && !seen.has(i) && seen.add(i)))
    .filter((row) => row.length);
  const missing = Array.from({ length: n }, (_, i) => i).filter((i) => !seen.has(i));
  if (missing.length) {
    if (!rows.length) rows.push([]);
    rows[rows.length - 1].push(...missing);
  }
  return rows;
}

function editableLayout(d) {
  d.layout = boardLayout(d);
  return d.layout;
}

export function addControl(d, row) {
  const layout = editableLayout(d);
  const k = d.controls.length;
  const last = layout[row] && layout[row][layout[row].length - 1];
  const kind = last === undefined ? "knob" : kindOf(d, last);
  d.controls.push({ kind, input: -1, reverse: false, min: 0, max: 1023 });
  if (!layout[row]) layout[row] = [];
  layout[row].push(k);
  for (const p of d.profiles) {
    p.jobs = p.jobs || [];
    while (p.jobs.length < d.controls.length) p.jobs.push([]);
  }
  S.selected[d.id] = k;
}

export function canMove(d, k, dir) {
  const layout = boardLayout(d);
  const r = layout.findIndex((row) => row.includes(k));
  if (r < 0) return false;
  const i = layout[r].indexOf(k);
  if (dir === "left") return i > 0;
  if (dir === "right") return i < layout[r].length - 1;
  // Moving past the first or last row starts a new one, unless the control is alone in its row.
  if (dir === "up") return r > 0 || layout[r].length > 1;
  return r < layout.length - 1 || layout[r].length > 1;
}

export function moveControl(d, k, dir) {
  if (!canMove(d, k, dir)) return;
  const layout = editableLayout(d);
  const r = layout.findIndex((row) => row.includes(k));
  const i = layout[r].indexOf(k);
  if (dir === "left" || dir === "right") {
    const j = i + (dir === "left" ? -1 : 1);
    [layout[r][i], layout[r][j]] = [layout[r][j], layout[r][i]];
  } else {
    const to = r + (dir === "up" ? -1 : 1);
    layout[r].splice(i, 1);
    if (to < 0) layout.unshift([k]);
    else if (to >= layout.length) layout.push([k]);
    else layout[to].splice(Math.min(i, layout[to].length), 0, k);
    d.layout = layout.filter((row) => row.length);
  }
}

// Whether removing control k loses anything: jobs or actions in any profile, or a mute pointing
// at it.
export function controlUsed(d, k) {
  const key = String(buttonKey(d, k));
  return d.profiles.some(
    (p) =>
      ((p.jobs || [])[k] || []).length > 0 ||
      Object.entries(p.buttons || {}).some(([id, actions]) => (id === key && actions.length > 0) || actions.includes(`mute:${k}`))
  );
}

// Takes control k off the board: its place, jobs and actions go, and every later control moves
// down one, its mutes with it. A DIY board's buttons go by control, so their keys move too.
export function removeControl(d, k) {
  const shift = (i) => (i > k ? i - 1 : i);
  const key = buttonKey(d, k);
  const wasButton = isButton(d, k);
  d.layout = boardLayout(d)
    .map((row) => row.filter((i) => i !== k).map(shift))
    .filter((row) => row.length);
  d.controls.splice(k, 1);
  const muteOf = (a) => (a.startsWith("mute:") ? parseInt(a.slice(5), 10) : -1);
  for (const p of d.profiles) {
    if (p.jobs) p.jobs.splice(k, 1);
    const buttons = {};
    for (const [id, actions] of Object.entries(p.buttons || {})) {
      let n = parseInt(id, 10);
      if (wasButton && n === key) continue;
      if (d.type === "diy") {
        if (n === k) continue;
        n = shift(n);
      }
      const kept = actions.filter((a) => muteOf(a) !== k).map((a) => (muteOf(a) > k ? `mute:${muteOf(a) - 1}` : a));
      if (kept.length) buttons[n] = kept;
    }
    p.buttons = buttons;
  }
  S.selected[d.id] = Math.max(0, Math.min(k, d.controls.length - 1));
}
