import { connect, send, t } from "./bridge.js";
import {
  fitLists,
  moveControl,
  picked,
  pressed,
  renderBoards,
  select,
  showValues,
  touched,
} from "./boards.js";
import { openAdd, openGear, refreshGearPorts, renderDialog } from "./dialogs.js";
import { renderGeneral } from "./general.js";
import {
  BUTTON_MENU_BASE,
  jobKey,
  onPick,
  openButtonMenu,
  openJobMenu,
  replaceKind,
  toggleAction,
  toggleJob,
} from "./pickers.js";
import {
  S,
  activeProfile,
  buttonKey,
  clampIndex,
  clone,
  currentBoard,
  deviceById,
  esc,
  escAttr,
  isButton,
  jobsOf,
  setActions,
} from "./state.js";

// A 1x1 transparent GIF: the About tab's icon falls back to this rather than an empty src, which
// some browsers render as a visible broken-image box.
const TRANSPARENT_PIXEL = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";

// --- Rendering ------------------------------------------------------------

function render() {
  const focusedId = document.activeElement && document.activeElement.id;

  // The dialog is appended fresh below, not patched in place: drop any previous copy first, or
  // every render while one is open would stack another on top.
  document.querySelectorAll(".dialog-scrim").forEach((el) => el.remove());

  document.getElementById("page-title").textContent = t("settings");
  renderTabs();
  renderFooter();
  if (S.tab === "boards") renderBoards();
  else if (S.tab === "about") renderAbout();
  else renderGeneral();

  if (S.dialog) renderDialog();

  if (focusedId) {
    const el = document.getElementById(focusedId);
    if (el) el.focus({ preventScroll: true });
  }
}
S.render = render;

function renderTabs() {
  const tabs = [
    ["general", t("tab.general")],
    ["boards", t("tab.boards")],
    ["about", t("tab.about")],
  ];
  document.getElementById("tabs").innerHTML = tabs
    .map(
      ([id, label]) =>
        `<button class="tab" type="button" role="tab" data-action="switch-tab" data-tab="${id}" aria-selected="${id === S.tab}">${esc(label)}</button>`
    )
    .join("");
}

function renderFooter() {
  document.getElementById("btn-close").textContent = t("close");
  document.getElementById("btn-save").textContent = t("apply");
  updateSaveButton();
}

// Language is saved the moment it is picked, so it never waits for Apply, nor does a board's
// Draw or List.
function comparable(settings) {
  return JSON.stringify({
    ...settings,
    language: undefined,
    devices: (settings.devices || []).map((d) => ({ ...d, view: undefined })),
  });
}

function hasChanges() {
  return !!S.draft && !!S.saved && comparable(S.draft) !== comparable(S.saved);
}

function updateSaveButton() {
  document.getElementById("btn-save").disabled = !hasChanges();
}

function hostnameOf(url) {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

function renderAbout() {
  const init = S.init;
  document.getElementById("panel").innerHTML = `
    <div class="tabpanel about" role="tabpanel">
      <img class="about-icon" src="${escAttr(init.icon || TRANSPARENT_PIXEL)}" alt="" width="64" height="64" />
      <h2 class="about-name">WeeJ</h2>
      <p class="row-desc">${esc(t("version", { version: init.version }))}</p>
      <button class="btn about-check" type="button" data-action="check-updates">${esc(t("check"))}</button>
      <p class="about-line"><a href="#" data-action="open-url" data-url="${escAttr(init.website)}">${esc(t("website"))}</a></p>
      <p class="row-desc about-line">${esc(t("made_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.madeBy)}">${esc(hostnameOf(init.madeBy))}</a></p>
      <p class="row-desc about-line">${esc(t("on_macos"))} <a href="#" data-action="open-url" data-url="${escAttr(init.theej)}">TheeJ</a></p>
      <p class="row-desc about-line">${esc(t("inspired_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.deej)}">deej</a></p>
    </div>`;
}

// --- Saving -----------------------------------------------------------------

function doSave() {
  if (!hasChanges()) return;
  send({ type: "save", settings: S.draft });
}

// saveBoard saves one board at once, with whatever the page has of it.
function saveBoard(d) {
  send({ type: "setDevice", device: d });
}

// Calibrating works on the board as saved, so the page's edits to it are saved first.
function calibrate(d, controls) {
  saveBoard(d);
  S.dialog = { kind: "wizard", id: d.id };
  send({ type: "calibrate", device: d.id, controls });
  render();
}

// Puts a board Go saved into both copies, leaving the page's other edits as they are.
function upsertDevice(dev) {
  for (const s of [S.draft, S.saved]) {
    if (!s) continue;
    s.devices = s.devices || [];
    const i = s.devices.findIndex((d) => d.id === dev.id);
    if (i >= 0) s.devices[i] = clone(dev);
    else s.devices.push(clone(dev));
  }
}

function removeDevice(id) {
  for (const s of [S.draft, S.saved]) {
    if (s) s.devices = (s.devices || []).filter((d) => d.id !== id);
  }
  if (S.board === id) S.board = null;
  if (S.dialog && S.dialog.id === id) S.dialog = null;
}

// --- Shortcut recording -----------------------------------------------------

// The board a shortcut being recorded belongs to: the gear's own copy while its dialog is open.
function recordTarget(r) {
  if (r.dialog && S.dialog && S.dialog.kind === "gear") return S.dialog.dev;
  return deviceById(r.device);
}

function applyRecorded(r, shortcut) {
  const d = recordTarget(r);
  if (!d) return;
  const field = r.field;
  if (field === "next") d.nextProfile = shortcut;
  else if (field === "previous") d.previousProfile = shortcut;
  else if (field.indexOf("profile:") === 0) {
    const i = parseInt(field.slice("profile:".length), 10);
    if (d.profiles[i]) d.profiles[i].shortcut = shortcut;
  } else if (field.indexOf("button:") === 0) {
    const key = parseInt(field.slice("button:".length), 10);
    replaceKind(d, key, shortcut ? `keys:${shortcut.mods}:${shortcut.vk}:${shortcut.key}` : "keys:");
  }
}

const MODIFIER_KEYS = new Set(["Control", "Alt", "AltGraph", "Shift", "Meta", "OS"]);

function onRecordKeydown(e) {
  const r = S.recording;
  if (!r) return;
  e.preventDefault();
  e.stopPropagation();
  if (e.key === "Escape") {
    stopRecording(true);
    return;
  }
  // Holding Ctrl or Alt fires a keydown for the modifier itself; wait for the real key.
  if (MODIFIER_KEYS.has(e.key)) return;
  const isClear = (e.key === "Delete" || e.key === "Backspace") && !e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey;
  // A hotkey needs a Ctrl or Alt chord; a key a button presses can be any key.
  if (!isClear && !e.ctrlKey && !e.altKey && r.field.indexOf("button:") !== 0) return;
  const d = recordTarget(r);
  send({
    type: "key",
    field: r.field,
    vk: e.keyCode,
    key: e.key,
    ctrl: e.ctrlKey,
    alt: e.altKey,
    shift: e.shiftKey,
    meta: e.metaKey,
    draft: d
      ? { profiles: d.profiles.map((p) => p.shortcut || null), next: d.nextProfile || null, previous: d.previousProfile || null }
      : { profiles: [], next: null, previous: null },
  });
}

function startRecording(field, device, dialog) {
  S.recording = { field, device, dialog };
  // Go releases the saved hotkeys meanwhile, or a chord already in use never reaches the page.
  send({ type: "record" });
  window.addEventListener("keydown", onRecordKeydown, true);
  window.addEventListener("blur", onWindowBlurWhileRecording);
  render();
}

function onWindowBlurWhileRecording() {
  stopRecording(true);
}

function stopRecording(notifyGo) {
  if (!S.recording) return;
  window.removeEventListener("keydown", onRecordKeydown, true);
  window.removeEventListener("blur", onWindowBlurWhileRecording);
  S.recording = null;
  if (notifyGo) send({ type: "stopRecording" });
  render();
}

// --- Events -----------------------------------------------------------------

function board() {
  return currentBoard();
}

function onClick(e) {
  if (S.recording) {
    const r = S.recording;
    const stillInside = e.target.closest(`[data-field="${r.field}"][data-device="${r.device}"]`);
    if (!stillInside) stopRecording(true);
  }

  const target = e.target.closest("[data-action]");
  if (!target) return;
  const d = board();

  switch (target.dataset.action) {
    case "switch-tab":
      S.tab = target.dataset.tab;
      render();
      break;
    case "board-tab":
    case "open-board":
      S.board = target.dataset.board;
      S.tab = "boards";
      render();
      break;
    case "gear":
      send({ type: "listPorts" });
      openGear(target.dataset.board);
      render();
      break;
    case "add-board":
      send({ type: "listPorts" });
      openAdd();
      render();
      break;
    case "setup-count":
      S.setup = { total: parseInt(target.dataset.count, 10), done: 0 };
      send({ type: "listPorts" });
      openAdd();
      render();
      break;
    case "add-type":
      S.dialog.type = target.dataset.type;
      S.dialog.port = "";
      if (S.dialog.type === "smc") S.dialog.port = (S.midiInputs || []).find((n) => n.toLowerCase().includes("smc-mixer")) || "";
      render();
      break;
    case "add-confirm": {
      const a = S.dialog;
      S.dialog = { kind: "adding", type: a.type, counted: a.type !== "smc" };
      send({
        type: "addDevice",
        name: a.name.trim(),
        deviceType: a.type,
        port: a.port,
        baudRate: a.baud,
        knobs: a.knobs,
        faders: a.faders,
        buttons: a.buttons,
      });
      render();
      break;
    }
    case "save-board": {
      const dev = S.dialog.dev;
      S.dialog = null;
      saveBoard(dev);
      render();
      break;
    }
    case "remove-board":
      S.dialog = { kind: "confirm", what: "board", id: S.dialog.id, back: S.dialog };
      render();
      break;
    case "calibrate-board": {
      const dev = S.dialog && S.dialog.kind === "gear" ? S.dialog.dev : d;
      if (dev) calibrate(dev);
      break;
    }
    case "find-control":
      if (d) calibrate(d, [picked(d)]);
      break;
    case "wizard-op":
      send({ type: "wizard", op: target.dataset.op });
      break;
    case "set-view":
      if (d) {
        d.view = target.dataset.view;
        const saved = deviceById(d.id, S.saved);
        if (saved) saved.view = d.view;
        saveBoard(d);
        render();
      }
      break;
    case "add-profile":
      if (d) {
        const p = { name: "", jobs: d.controls.map(() => []), buttons: {}, shortcut: null };
        d.profiles.push(p);
        d.profile = d.profiles.length - 1;
        render();
      }
      break;
    case "remove-profile":
      if (d && d.profiles.length > 1) {
        S.dialog = { kind: "confirm", what: "profile", id: d.id };
        render();
      }
      break;
    case "confirm-dialog":
      confirmDialog();
      render();
      break;
    case "cancel-dialog":
      if (S.dialog && S.dialog.kind === "add") S.setup = null;
      S.dialog = S.dialog && S.dialog.back ? S.dialog.back : null;
      render();
      break;
    case "select-control":
      if (d) select(d, parseInt(target.dataset.control, 10));
      break;
    case "open-job-menu":
      if (d) openJobMenu(d, parseInt(target.dataset.control, 10), target);
      break;
    case "open-button-menu":
      // Typing in a button's address box or clicking its own controls is not a click on the row.
      if (d && !e.target.closest(".button-param")) openButtonMenu(d, parseInt(target.dataset.key, 10), target);
      break;
    case "record": {
      const r = S.recording;
      const field = target.dataset.field;
      const device = target.dataset.device;
      const dialog = !!target.dataset.dialog;
      if (r && r.field === field && r.device === device && r.dialog === dialog) stopRecording(true);
      else startRecording(field, device, dialog);
      break;
    }
    case "remove-shortcut":
      applyRecorded({ field: target.dataset.field, device: target.dataset.device, dialog: !!target.dataset.dialog }, null);
      render();
      break;
    case "pick-button-app":
      send({ type: "pickApp", button: parseInt(target.dataset.key, 10), mode: target.dataset.mode });
      break;
    case "clear-control":
      if (d) {
        const k = picked(d);
        if (isButton(d, k)) setActions(d, buttonKey(d, k), []);
        else jobsOf(d, k).length = 0;
        render();
      }
      break;
    case "pick-control-app":
      if (d) send({ type: "pickApp", knob: picked(d) });
      break;
    case "move-control":
      if (d) {
        moveControl(d, picked(d), target.dataset.dir);
        render();
      }
      break;
    case "refresh-ports":
      send({ type: "listPorts" });
      break;
    case "reconnect":
      send({ type: "reconnect", device: target.dataset.board || (d && d.id) });
      break;
    case "import-deej":
      send({ type: "importDeej" });
      break;
    case "check-updates":
      send({ type: "checkUpdates" });
      break;
    case "open-url":
      e.preventDefault();
      send({ type: "openUrl", url: target.dataset.url });
      break;
  }
  updateSaveButton();
}

function confirmDialog() {
  const dlg = S.dialog;
  S.dialog = null;
  const d = deviceById(dlg.id);
  if (!d) return;
  if (dlg.what === "profile") {
    d.profiles.splice(d.profile, 1);
    d.profile = clampIndex(d.profile, d.profiles.length);
  } else if (dlg.what === "board") {
    send({ type: "removeDevice", device: d.id });
  }
}

function onInput(e) {
  const el = e.target;
  const d = board();
  // No render() for typing: it would rebuild the input and lose the caret mid-word.
  if (el.id === "profile-name" && d) activeProfile(d).name = el.value;
  if (el.id === "dlg-name" && S.dialog) S.dialog.dev.name = el.value;
  if (el.id === "add-name" && S.dialog) {
    S.dialog.name = el.value;
    const ready = document.querySelector('[data-action="add-confirm"]');
    if (ready) ready.disabled = !addReady();
  }
  if (["add-knobs", "add-faders", "add-buttons"].includes(el.id) && S.dialog) {
    S.dialog[el.id.slice(4)] = Math.max(0, Math.min(64, parseInt(el.value, 10) || 0));
    const ready = document.querySelector('[data-action="add-confirm"]');
    if (ready) ready.disabled = !addReady();
  }
  if (el.dataset.buttonUrl && d) replaceKind(d, parseInt(el.dataset.buttonUrl, 10), `url:${el.value.trim()}`);
  updateSaveButton();
}

function addReady() {
  const a = S.dialog;
  const counted = a.type !== "smc";
  return a.name.trim() !== "" && (a.type === "diy" || a.port !== "") && (!counted || a.knobs + a.faders + a.buttons > 0);
}

function onChange(e) {
  const el = e.target;
  const d = board();
  if (el.dataset.pick && d) {
    const field = onPick(d, picked(d), el);
    render();
    if (field) startRecording(field, d.id, false);
    updateSaveButton();
    return;
  }
  if (el.dataset.enable) {
    const dev = deviceById(el.dataset.enable);
    if (dev) {
      dev.enabled = el.checked;
      saveBoard(dev);
    }
    return;
  }
  switch (el.id) {
    case "profile-select":
      if (d) d.profile = parseInt(el.value, 10);
      render();
      break;
    case "language-select":
      S.draft.language = el.value;
      send({ type: "setLanguage", code: el.value });
      break;
    case "hide-icon":
      S.draft.hideTrayIcon = el.checked;
      render();
      break;
    case "show-profile-list":
      S.draft.showProfileList = el.checked;
      break;
    case "dlg-port":
      S.dialog.dev.port = el.value;
      break;
    case "dlg-baud":
      S.dialog.dev.baudRate = parseInt(el.value, 10);
      break;
    case "dlg-speed":
      S.dialog.dev.speed = el.value;
      break;
    case "dlg-lights":
      S.dialog.dev.lights = el.value === "off" ? "" : el.value;
      break;
    case "add-port":
      S.dialog.port = el.value;
      render();
      break;
    case "add-baud":
      S.dialog.baud = parseInt(el.value, 10);
      break;
  }
  updateSaveButton();
}

// The drawn controls take Enter and Space as buttons do, and the arrow keys walk between them.
function onControlKeydown(e) {
  const ctl = e.target.closest && e.target.closest(".smc .ctl");
  if (!ctl) return;
  const d = board();
  if (e.key === "Enter" || e.key === " ") {
    e.preventDefault();
    e.stopPropagation();
    if (!d) return;
    select(d, parseInt(ctl.dataset.control, 10));
    render();
    return;
  }
  const step = { ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1 }[e.key];
  if (!step) return;
  e.preventDefault();
  const all = [...document.querySelectorAll(".smc .ctl")];
  const next = all[all.indexOf(ctl) + step];
  if (next) next.focus();
}

// While boards are being set up one after another, the next one's Add follows the last one's
// calibration; once they are all in, the Boards tab shows them.
function nextInSetup() {
  if (!S.setup) return;
  if (S.setup.done < S.setup.total) {
    openAdd();
    return;
  }
  S.setup = null;
  S.tab = "boards";
}

// --- Messages from Go ---------------------------------------------------------

// Go opens Settings on "general", "boards" or "about".
function tabFor(tab) {
  return tab === "boards" || tab === "about" ? tab : "general";
}

function onMessage(msg) {
  switch (msg.type) {
    case "room":
      fitLists();
      break;
    case "init":
      S.init = msg;
      S.draft = clone(msg.settings || { devices: [] });
      S.draft.devices = S.draft.devices || [];
      S.saved = clone(S.draft);
      S.labels = Object.assign({}, msg.labels || {});
      S.status = msg.status || {};
      S.live = msg.values || {};
      S.tab = tabFor(msg.tab);
      S.wizard = msg.wizard || null;
      if (S.wizard) S.dialog = { kind: "wizard", id: S.wizard.device };
      send({ type: "listPorts" });
      render();
      break;
    case "strings":
      render();
      break;
    case "saved":
      S.draft = clone(msg.settings);
      S.draft.devices = S.draft.devices || [];
      S.saved = clone(S.draft);
      render();
      break;
    case "deviceSaved":
      upsertDevice(msg.device);
      render();
      break;
    case "added": {
      const adding = S.dialog && S.dialog.kind === "adding" ? S.dialog : null;
      S.board = msg.device;
      if (S.setup) S.setup.done++;
      if (adding && adding.counted) {
        S.dialog = { kind: "wizard", id: msg.device };
        send({ type: "calibrate", device: msg.device });
      } else {
        S.dialog = null;
        nextInSetup();
      }
      render();
      break;
    }
    case "deviceRemoved":
      removeDevice(msg.device);
      render();
      break;
    case "imported":
      S.importNote = msg.skipped && msg.skipped.length ? t("import_skipped", { items: msg.skipped.join(", ") }) : "";
      S.board = msg.device;
      render();
      break;
    case "importFailed":
      S.importNote = t("import_failed");
      render();
      break;
    case "status":
      S.status[msg.device] = { connected: !!msg.connected, busy: !!msg.busy, port: msg.port || "" };
      if (S.tab !== "about" || S.dialog) render();
      break;
    case "values":
      showValues(msg.device, msg.values || []);
      break;
    case "moved":
      touched(msg.device, msg.control);
      break;
    case "pressed":
      pressed(msg.device, msg.key);
      break;
    // Go-initiated: a shortcut, the tray or a button switched a board's profile, already saved.
    case "profile":
      for (const s of [S.draft, S.saved]) {
        const d = deviceById(msg.device, s);
        if (d) d.profile = clampIndex(msg.profile, d.profiles.length);
      }
      render();
      break;
    // Go-initiated: a button changed a board's light pattern, already saved.
    case "lights":
      for (const s of [S.draft, S.saved]) {
        const d = deviceById(msg.device, s);
        if (d) d.lights = msg.pattern;
      }
      if (S.dialog && S.dialog.kind === "gear" && S.dialog.id === msg.device) S.dialog.dev.lights = msg.pattern;
      render();
      break;
    case "wizard": {
      if (msg.end) {
        S.wizard = null;
        if (S.dialog && S.dialog.kind === "wizard") S.dialog = null;
        nextInSetup();
        render();
        break;
      }
      const before = S.wizard;
      S.wizard = msg;
      if (!S.dialog || S.dialog.kind !== "wizard") S.dialog = { kind: "wizard", id: msg.device };
      const same =
        before &&
        ["device", "control", "stage", "count", "warning", "other", "index", "done"].every((k) => before[k] === msg[k]);
      if (!same) render();
      break;
    }
    case "ports":
      S.ports = msg.ports || [];
      S.midiInputs = msg.midi || [];
      if (S.dialog && S.dialog.kind === "gear") refreshGearPorts();
      if (S.dialog && S.dialog.kind === "add") {
        if (S.dialog.type === "smc" && !S.dialog.port) {
          S.dialog.port = S.midiInputs.find((n) => n.toLowerCase().includes("smc-mixer")) || "";
        }
        render();
      }
      break;
    case "buttonAppPicked": {
      const d = board();
      if (!d) break;
      const value = msg.mode === "open" ? msg.path : msg.exe;
      S.appNames[value] = msg.name;
      replaceKind(d, msg.button, `${msg.mode}:${value}`);
      render();
      break;
    }
    case "jobMenuToggle": {
      const d = board();
      if (!d) break;
      if (msg.knob >= BUTTON_MENU_BASE) toggleAction(d, msg.knob - BUTTON_MENU_BASE, msg.job.action, msg.checked);
      else toggleJob(d, msg.knob, msg.job, msg.checked);
      render();
      break;
    }
    case "jobMenuClear": {
      const d = board();
      if (!d) break;
      if (msg.knob >= BUTTON_MENU_BASE) setActions(d, msg.knob - BUTTON_MENU_BASE, []);
      else jobsOf(d, msg.knob).length = 0;
      render();
      break;
    }
    case "appPicked": {
      const d = board();
      if (!d) break;
      const jobs = jobsOf(d, msg.knob);
      if (!jobs.some((j) => jobKey(j) === jobKey(msg.entry.job))) jobs.push(msg.entry.job);
      if (!S.init.catalog.some((c) => jobKey(c.job) === jobKey(msg.entry.job))) S.init.catalog.push(msg.entry);
      render();
      break;
    }
    case "recorded":
      if (msg.shortcut) S.labels[JSON.stringify({ vk: msg.shortcut.vk, mods: msg.shortcut.mods, key: msg.shortcut.key })] = msg.label;
      if (S.recording) applyRecorded(S.recording, msg.shortcut);
      stopRecording(true);
      break;
    case "rejected":
      break;
    // Go-initiated: the window was already open and got asked to show a tab (the tray's About).
    case "tab":
      S.tab = tabFor(msg.tab || S.tab);
      render();
      break;
  }
  if (S.draft) updateSaveButton();
}

document.getElementById("root").addEventListener("click", onClick);
document.getElementById("root").addEventListener("input", onInput);
document.getElementById("root").addEventListener("change", onChange);
document.getElementById("root").addEventListener("keydown", onControlKeydown);
document.getElementById("btn-close").addEventListener("click", () => send({ type: "close" }));
document.getElementById("btn-save").addEventListener("click", doSave);

window.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || S.recording || S.dialog) return;
  if ((e.target.tagName || "").toLowerCase() === "textarea") return;
  e.preventDefault();
  doSave();
});

connect("root", onMessage);

window.addEventListener("resize", fitLists);
