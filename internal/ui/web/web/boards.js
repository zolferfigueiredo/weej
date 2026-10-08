// The Boards tab: a tab of its own for each board, with its profiles, and the board drawn or
// listed with what each control does.
import { pageHeight, roomHeight, send, t } from "./bridge.js";
import { boardSVG, showValue, smcButtonIcon, smcSVG } from "./device.js";
import { actionPicks, buttonLines, buttonParams, controlLabel, jobLines, jobPicks } from "./pickers.js";
import {
  S,
  actionsOf,
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
  shownBoards,
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
  const d = currentBoard();
  if (!d) {
    panel.innerHTML = `
    <div class="tabpanel" role="tabpanel">
      <div class="empty-state">
        <p>${esc(t(devices().length ? "boards.none_connected" : "boards.none"))}</p>
        ${devices().length ? "" : `<button class="btn btn-primary" type="button" data-action="switch-tab" data-tab="general">${esc(t("boards.set_up"))}</button>`}
      </div>
    </div>`;
    return;
  }
  const boards = shownBoards()
    .map((b) => `<option value="${escAttr(b.id)}"${b.id === d.id ? " selected" : ""}>${esc(clip(b.name, 30))}</option>`)
    .join("");
  const i = clampIndex(d.profile, d.profiles.length);
  const profiles = d.profiles
    .map((_, n) => `<option value="${n}"${n === i ? " selected" : ""}>${esc(clip(profileLabel(d, n), 30))}</option>`)
    .join("");
  const body = d.view === "list" ? listView(d) : drawView(d);
  keepScroll(() => {
    panel.innerHTML = `
    <div class="tabpanel" role="tabpanel">
      <div class="board-toolbar">
        <select class="select select-inline" id="board-select" aria-label="${escAttr(t("boards.title"))}">${boards}</select>
        <select class="select select-inline" id="profile-select" aria-label="${escAttr(t("profile"))}">${profiles}</select>
        <span class="menu-anchor">
          <button class="btn btn-icon" type="button" data-action="board-menu" aria-haspopup="menu" aria-expanded="${!!S.menu}" title="${escAttr(t("profile"))}" aria-label="${escAttr(t("profile"))}">&#x22ef;</button>
          ${S.menu ? boardMenu(d) : ""}
        </span>
        <span class="spacer"></span>
        ${viewBar(d)}
      </div>
      ${S.importNote ? `<p class="group-note">${esc(S.importNote)}</p>` : ""}
      ${body}
    </div>`;
    fitLists();
  });
}

// The toolbar's menu: the profile shown, and the board's own settings.
function boardMenu(d) {
  const item = (action, label, disabled = false) =>
    `<button class="menu-item" type="button" role="menuitem" data-action="${action}" data-board="${escAttr(d.id)}"${disabled ? " disabled" : ""}>${esc(label)}</button>`;
  return `<div class="menu" role="menu">
            ${item("edit-profile", t("profile.edit"))}
            ${item("add-profile", t("add_profile"))}
            ${item("remove-profile", t("remove_profile"), d.profiles.length <= 1)}
            <div class="menu-sep"></div>
            ${item("import-profile", t("profile.import"))}
            ${item("export-profile", t("profile.export"))}
            <div class="menu-sep"></div>
            ${item("gear", t("boards.settings"))}
          </div>`;
}

const LIST_ROWS = 10;

// Each List card shows LIST_ROWS rows and scrolls past them, and less when the page would
// still be taller than the window can grow, so the page never scrolls as a whole.
export function fitLists() {
  const columns = document.querySelector(".list-columns");
  if (!columns) return;
  const cards = [...columns.querySelectorAll(".card")];
  const scrolled = cards.map((c) => c.scrollTop);
  cards.forEach((c) => (c.style.maxHeight = ""));
  const limits = cards.map((c) => {
    const rows = c.querySelectorAll(":scope > .row");
    if (rows.length <= LIST_ROWS) return Infinity;
    const border = parseFloat(getComputedStyle(c).borderBottomWidth) || 0;
    return Math.ceil(rows[LIST_ROWS - 1].getBoundingClientRect().bottom - c.getBoundingClientRect().top + border);
  });
  cards.forEach((c, i) => {
    if (limits[i] < Infinity) c.style.maxHeight = `${limits[i]}px`;
  });
  const room = roomHeight();
  const over = Math.ceil(pageHeight() - room);
  if (room > 0 && over > 0) {
    const cap = Math.max(Math.max(...cards.map((c) => c.offsetHeight)) - over, 160);
    cards.forEach((c) => {
      if (c.offsetHeight > cap) c.style.maxHeight = `${cap}px`;
    });
  }
  cards.forEach((c, i) => (c.scrollTop = scrolled[i]));
}

export const GEAR_ICON = `<svg class="gear" viewBox="0 0 16 16" aria-hidden="true"><path d="M6.6 1h2.8l.4 1.9 1.1.6 1.8-.7 1.4 2.4-1.4 1.3v1.2l1.4 1.3-1.4 2.4-1.8-.7-1.1.6-.4 1.9H6.6l-.4-1.9-1.1-.6-1.8.7-1.4-2.4 1.4-1.3V7.8L1.9 6.5l1.4-2.4 1.8.7 1.1-.6z"/><circle cx="8" cy="8" r="2.2"/></svg>`;

// --- Draw --------------------------------------------------------------------

// Draw | List, with a gear beside Draw that turns on moving a DIY or MIDI board's controls.
function viewBar(d) {
  const view = d.view === "list" ? "list" : "draw";
  const button = (v) =>
    `<button class="btn${v === view ? " on" : ""}" type="button" data-action="set-view" data-view="${v}" aria-pressed="${v === view}">${esc(t("view." + v))}</button>`;
  const arranging = !!S.arrange[d.id];
  const gear =
    view === "draw" && !isSMC(d) && d.controls.length
      ? `<button class="btn btn-icon${arranging ? " on" : ""}" type="button" data-action="arrange" aria-pressed="${arranging}" title="${escAttr(t("board.arrange"))}" aria-label="${escAttr(t("board.arrange"))}">${GEAR_ICON}</button>`
      : "";
  return `<span class="segmented">${button("draw")}${gear}${button("list")}</span>`;
}

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
    });
  }
  return `
      <div class="device-row">
        <div class="card device-card">${svg}</div>
        ${isSMC(d) ? smcInspector(d, k) : boardInspector(d, k)}
        <div class="group-foot device-foot"><span class="group-note">${esc(t(isSMC(d) ? "smc.hint" : "board.hint"))}</span></div>
      </div>`;
}

function smcInspector(d, id) {
  const button = isButton(d, id);
  const empty = button ? !actionsOf(d, id).length : !jobsOf(d, id).length;
  return `
        <div class="card inspector">
          <div class="inspector-head">
            ${button ? smcButtonIcon(id) : ""}<h2 class="group-title">${esc(controlName(d, id))}</h2>
            <span class="spacer"></span>
            <button class="btn" type="button" data-action="clear-control"${empty ? " disabled" : ""}>${esc(t("clear"))}</button>
          </div>
          <div class="inspector-list" data-keep-scroll="control-${d.id}-${id}">${button ? actionPicks(d, id) : jobPicks(d, id)}</div>
        </div>`;
}

function boardInspector(d, k) {
  if (!d.controls.length) {
    return `
        <div class="card inspector"><p class="inspector-empty">${esc(t("board.empty"))}</p></div>`;
  }
  const kind = kindOf(d, k);
  const button = kind === "button";
  const key = buttonKey(d, k);
  const empty = button ? !actionsOf(d, key).length : !jobsOf(d, k).length;
  const moves = [
    ["up", "&#x2191;"],
    ["left", "&#x2190;"],
    ["down", "&#x2193;"],
    ["right", "&#x2192;"],
  ]
    .map(([dir, arrow]) => `<button class="btn btn-icon" type="button" data-action="move-control" data-dir="${dir}" title="${escAttr(t("board." + dir))}" aria-label="${escAttr(t("board." + dir))}"${canMove(d, k, dir) ? "" : " disabled"}>${arrow}</button>`)
    .join("");
  return `
        <div class="card inspector">
          <div class="inspector-head">
            <h2 class="group-title">${esc(controlName(d, k))}</h2>
            <span class="spacer"></span>
            <button class="btn" type="button" data-action="clear-control"${empty ? " disabled" : ""}>${esc(t("clear"))}</button>
          </div>
          ${S.arrange[d.id] ? `<div class="board-edit"><div class="arrow-keys">${moves}</div></div>` : ""}
          <div class="inspector-list" data-keep-scroll="board-${d.id}-${k}">${button ? actionPicks(d, key) : jobPicks(d, k)}</div>
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
  const card = (title, rows, name) => `
        <div class="group list-group">
          <div class="group-head"><h2 class="group-title">${esc(title)}</h2></div>
          <div class="card" data-keep-scroll="${name}-${d.id}">${rows}</div>
        </div>`;
  return `
      <div class="list-columns">
        ${card(t("knobs"), potRows(knobs), "knobs")}
        ${card(t("list.faders"), potRows(faders), "faders")}
        ${card(t("list.buttons"), buttonRows, "buttons")}
      </div>`;
}

// --- Live --------------------------------------------------------------------

// A redraw keeps where each list was scrolled to; a list opened fresh starts on the first thing
// its control already does.
function keepScroll(draw) {
  const scrolled = new Map();
  document.querySelectorAll("[data-keep-scroll]").forEach((el) => scrolled.set(el.dataset.keepScroll, el.scrollTop));
  draw();
  document.querySelectorAll("[data-keep-scroll]").forEach((el) => {
    if (scrolled.has(el.dataset.keepScroll)) {
      el.scrollTop = scrolled.get(el.dataset.keepScroll);
      return;
    }
    const first = el.querySelector(".chk:checked");
    if (first) el.scrollTop = first.getBoundingClientRect().top - el.getBoundingClientRect().top - 40;
  });
}

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

