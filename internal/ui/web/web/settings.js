import { connect, send, t } from "./bridge.js";
import {
  SMC_COLUMNS,
  boardSVG,
  isSMCName,
  isStripControl,
  showValue,
  smcButtonIcon,
  smcControlName,
  smcSVG,
} from "./device.js";

// A 1x1 transparent GIF: the About tab's icon falls back to this rather than an
// empty src, which some browsers render as a visible broken-image box.
const TRANSPARENT_PIXEL = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";

// --- State --------------------------------------------------------------
// init: the last full "init"/"saved"/"imported" payload (catalog, languages,
// iconPreviews, labels, version, website...). draft: the mutable core.Setup
// JSON being edited; draft.profile doubles as "which profile the editor shows"
// and "which profile core treats as active", since Settings has only one picker
// for both. Nothing here is sent to Go except on an explicit protocol message.
let init = null;
let draft = null;
let labels = {};
let activeTab = "board";
let recording = null; // { field: "profile:<i>" | "next" | "previous" }
let dialog = null; // { kind: "removeProfile" | "removeKnob" }
let saved = null; // the setup as last saved, so Save is enabled only when the draft differs
let importNote = ""; // last import_skipped / import_failed text, shown under the import button
let ports = null; // [{ name, product, usb }] once Go has listed them
let midiInputs = []; // MIDI input names; a "midi:<name>" port reads that mixer
let connection = { connected: false, busy: false, port: "" }; // the board's
let mixerConnection = { connected: false, busy: false, port: "" };
let calibrating = false; // set on the click, so the button greys out before the window opens
let selected = 0; // the SMC-Mixer control the inspector shows: a fader or knob 0-15, or a button id
let boardSelected = 0; // the board's control the inspector shows, by knob index
let live = { board: [], mixer: [] }; // the last frame from each device
let learning = null; // { knob, start }: the frame when the picked control began waiting for its input
const litUntil = new Map(); // control id to when it stops showing as touched
const controlTimers = new Map();

const SECTION_LABEL_KEY = {
  volume: "section.volume",
  brightness: "section.brightness",
  contrast: "section.contrast",
  nightLight: "section.night_light",
  keyboard: "section.keyboard",
  zoom: "section.zoom",
  apps: "section.apps",
};

// --- Small helpers --------------------------------------------------------

function clone(v) {
  return JSON.parse(JSON.stringify(v));
}

function esc(s) {
  return String(s == null ? "" : s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}
const escAttr = esc;

// A to Z, then A2 to Z2, A3 and so on, as core.Letter names them.
function letterFor(i) {
  return String.fromCharCode(65 + (i % 26)) + (i >= 26 ? String(Math.floor(i / 26) + 1) : "");
}

function clip(name, limit) {
  if (!name) return name;
  const chars = Array.from(name);
  if (chars.length <= limit) return name;
  return chars.slice(0, limit - 1).join("").trimEnd() + "…";
}

function clampIndex(i, len) {
  if (len <= 0) return 0;
  if (i < 0 || i >= len) return 0;
  return i;
}

function jobKey(job) {
  if (job.kind === "brightness" || job.kind === "contrast") return job.kind + ":" + job.screen;
  if (job.kind === "app") return job.kind + ":" + job.exe;
  return job.kind;
}

function hasJob(jobs, job) {
  const k = jobKey(job);
  return jobs.some((j) => jobKey(j) === k);
}

function jobTitleFor(job) {
  const k = jobKey(job);
  const entry = init.catalog.find((c) => jobKey(c.job) === k);
  return entry ? entry.short : job.kind;
}

function jobEntryFor(job) {
  const k = jobKey(job);
  return init.catalog.find((c) => jobKey(c.job) === k) || null;
}

function jobLines(jobs) {
  if (!jobs.length) return `<span class="job-line muted">${esc(t("job.empty"))}</span>`;
  return jobs
    .map((job) => {
      const entry = jobEntryFor(job);
      const title = entry ? entry.title : job.exe || job.kind;
      const icon = entry && entry.icon ? `<img src="${entry.icon}" alt="" />` : "";
      return `<span class="job-line">${icon}<span>${esc(title)}</span></span>`;
    })
    .join("");
}

function jobsSummary(jobs) {
  return jobs.map(jobTitleFor).join(", ");
}

function sectionsFromCatalog() {
  const bySection = new Map();
  for (const entry of init.catalog) {
    if (!bySection.has(entry.section)) bySection.set(entry.section, []);
    bySection.get(entry.section).push(entry);
  }
  return bySection;
}

function shortcutKeyJSON(s) {
  if (!s) return null;
  // Field order must match Go's json.Marshal of core.Shortcut{VK,Mods,Key}, since
  // that encoded string is the literal key into init.labels.
  return JSON.stringify({ vk: s.vk, mods: s.mods, key: s.key });
}

function shortcutLabel(s) {
  if (!s) return null;
  return labels[shortcutKeyJSON(s)] || s.key || "";
}

function activeProfile() {
  return draft.profiles[draft.profile];
}

// --- Rendering ------------------------------------------------------------

function render() {
  const focusedId = document.activeElement && document.activeElement.id;

  // The dialog is appended fresh below, not patched in place: drop any previous
  // copy first, or every render while one is open would stack another on top.
  document.querySelectorAll(".dialog-scrim").forEach((el) => el.remove());

  document.getElementById("page-title").textContent = t("settings");
  renderTabs();
  renderFooter();
  if (activeTab === "app") renderApp();
  else if (activeTab === "about") renderAbout();
  else renderGeneral();

  if (dialog) renderDialog();

  if (focusedId) {
    const el = document.getElementById(focusedId);
    if (el) el.focus({ preventScroll: true });
  }
}

function renderTabs() {
  const tabs = [
    ["board", t("tab.board")],
    ["mixer", t("tab.mixer")],
    ["app", t("tab.app")],
    ["about", t("tab.about")],
  ];
  document.getElementById("tabs").innerHTML = tabs
    .map(
      ([id, label]) =>
        `<button class="tab" type="button" role="tab" data-action="switch-tab" data-tab="${id}" aria-selected="${id === activeTab}">${esc(label)}</button>`
    )
    .join("");
}

function renderFooter() {
  document.getElementById("btn-close").textContent = t("close");
  document.getElementById("btn-save").textContent = t("apply");
  updateSaveButton();
}

// Language applies, and is saved, the moment it is picked, so it never waits for Save.
function comparable(setup) {
  return JSON.stringify({ ...setup, language: undefined });
}

function hasChanges() {
  return !!draft && !!saved && comparable(draft) !== comparable(saved);
}

// Comparing with the saved setup, rather than noting that something was edited, also turns
// Save off again when an edit is undone by hand.
function updateSaveButton() {
  document.getElementById("btn-save").disabled = !hasChanges();
}

function shortcutControl(field, shortcut) {
  const isRecording = recording && recording.field === field;
  const label = isRecording ? t("press_shortcut") : shortcut ? shortcutLabel(shortcut) : t("record_shortcut");
  const removeBtn =
    shortcut && !isRecording
      ? `<button class="btn btn-icon btn-subtle" type="button" data-action="remove-shortcut" data-field="${escAttr(field)}" title="${escAttr(t("remove_shortcut"))}" aria-label="${escAttr(t("remove_shortcut"))}">&times;</button>`
      : "";
  return `<button class="btn" type="button" data-action="record" data-field="${escAttr(field)}">${esc(label)}</button>${removeBtn}`;
}

function renderGeneral() {
  const profile = activeProfile();
  const profileOptions = draft.profiles
    .map(
      (p, i) =>
        `<option value="${i}"${i === draft.profile ? " selected" : ""}>${esc(clip(p.name || t("profile_n", { n: String(i + 1) }), 30))}</option>`
    )
    .join("");

  const mixer = usesMixer();
  const cols = knobColumns();
  const knobsHtml = cols.length
    ? cols
        .map((col, i) => {
          const jobs = deviceJobs(profile)[i] || [];
          const needsCal = col === null || col === undefined || col === -1;
          return `
            <div class="row clickable knob-row" id="knob-row-${i}" role="button" tabindex="0" data-action="open-job-menu" data-knob="${i}">
              <span class="knob-grip" aria-hidden="true">${GRIP_ICON}</span>
              <div class="row-main">
                <span class="row-title">${esc(t("knob", { letter: letterFor(i) }))}</span>
                ${needsCal ? `<span class="row-desc warning">${esc(t("needs_calibration"))}</span>` : ""}
              </div>
              <div class="row-control">
                <div class="job-list">${jobLines(jobs)}</div>
                <span class="chev" aria-hidden="true">&#x2304;</span>
              </div>
            </div>`;
        })
        .join("")
    : `<div class="row"><span class="row-desc">${esc(t("no_knobs"))}</span></div>`;

  const profileGroup = `
      <div class="group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("profile"))}</h2>
          <select class="select select-inline" id="profile-select">${profileOptions}</select>
          <span class="spacer"></span>
          <span class="segmented">
            <button class="btn btn-icon" type="button" data-action="add-profile" title="${escAttr(t("add_profile"))}" aria-label="${escAttr(t("add_profile"))}">+</button>
            <button class="btn btn-icon" type="button" data-action="remove-profile" title="${escAttr(t("remove_profile"))}" aria-label="${escAttr(t("remove_profile"))}"${draft.profiles.length <= 1 ? " disabled" : ""}>&minus;</button>
          </span>
        </div>
        <div class="card">
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("name"))}</span></div>
            <div class="row-control">
              <input class="input input-name" id="profile-name" type="text" value="${escAttr(profile.name)}" placeholder="${escAttr(t("profile_n", { n: String(draft.profile + 1) }))}" />
            </div>
          </div>
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("shortcut"))}</span></div>
            <div class="row-control">${shortcutControl("profile:" + draft.profile, profile.shortcut)}</div>
          </div>
        </div>
      </div>`;

  const knobsGroup = `
      <div class="group list-group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("knobs"))}</h2>
          <span class="spacer"></span>
          <span class="segmented">
            <button class="btn btn-icon" type="button" data-action="add-knob" title="${escAttr(t("add_knob"))}" aria-label="${escAttr(t("add_knob"))}">+</button>
            <button class="btn btn-icon" type="button" data-action="remove-knob" title="${escAttr(t("remove_knob"))}" aria-label="${escAttr(t("remove_knob"))}"${cols.length === 0 ? " disabled" : ""}>&minus;</button>
          </span>
        </div>
        <div class="card" data-keep-scroll="knobs">${knobsHtml}</div>
        <div class="group-foot">
          <span class="group-note">${esc(t("jobs_note"))}</span>
        </div>
      </div>`;

  const invertGroup = `
      <div class="group">
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("invert"))}</span>
              <span class="row-desc">${esc(t("invert_note"))}</span>
            </div>
            <div class="row-control"><input class="toggle" id="invert" type="checkbox" role="switch"${draft[mixer ? "invertMixer" : "invertKnobs"] ? " checked" : ""} /></div>
          </div>
        </div>
      </div>`;

  const calibrateGroup = `
      <div class="group">
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("calibrate"))}</span>
              <span class="row-desc">${esc(t(mixer ? "calibrate_note_mixer" : "calibrate_note"))}</span>
            </div>
            <div class="row-control"><button class="btn" type="button" data-action="calibrate"${calibrating ? " disabled" : ""}>${esc(t("calibrate"))}</button></div>
          </div>
        </div>
      </div>`;

  const importFoot = `
      <div class="group-foot">
        <button class="btn" type="button" data-action="import-deej">${esc(t("import_deej"))}</button>
        ${importNote ? `<span class="group-note">${esc(importNote)}</span>` : ""}
      </div>`;

  if (usesSMC()) {
    renderSMCGeneral(profileGroup, importFoot);
    return;
  }
  if (usesBoard()) {
    renderBoardGeneral(profileGroup, importFoot);
    return;
  }

  // Two columns whose rows line up: connection and profile, invert and calibrate, then the knobs
  // and the buttons.
  keepScroll(() => {
    document.getElementById("panel").innerHTML = `
    <div class="tabpanel general-grid" role="tabpanel">
      ${connectionGroup()}${profileGroup}
      ${invertGroup}${calibrateGroup}
      ${knobsGroup}${renderMixerButtons()}
      ${importFoot}
    </div>`;
  });
}

// A redraw keeps where each list was scrolled to.
function keepScroll(draw) {
  const scrolled = new Map();
  document.querySelectorAll("[data-keep-scroll]").forEach((el) => scrolled.set(el.dataset.keepScroll, el.scrollTop));
  draw();
  document.querySelectorAll("[data-keep-scroll]").forEach((el) => {
    if (scrolled.has(el.dataset.keepScroll)) {
      el.scrollTop = scrolled.get(el.dataset.keepScroll);
      return;
    }
    // A control's list opens on the first thing it already does.
    const first = el.querySelector(".chk:checked");
    if (first) el.scrollTop = first.getBoundingClientRect().top - el.getBoundingClientRect().top - 40;
  });
}

// --- SMC-Mixer ----------------------------------------------------------------

// The mixer drawn as it is, with what the control picked on it does beside it.
function renderSMCGeneral(profileGroup, importFoot) {
  const pattern = draft.mixerLights || "off";
  const lightOptions = (init.lightPatterns || ["off"])
    .map((p) => `<option value="${p}"${p === pattern ? " selected" : ""}>${esc(t("lights." + p))}</option>`)
    .join("");
  const invertRow = `
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("invert"))}</span></div>
            <div class="row-control"><input class="toggle" id="invert" type="checkbox" role="switch"${draft.invertMixer ? " checked" : ""} /></div>
          </div>
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("lights"))}</span>
              <span class="row-desc">${esc(t("lights_note"))}</span>
            </div>
            <div class="row-control"><select class="select" id="lights-select">${lightOptions}</select></div>
          </div>`;
  const svg = smcSVG({
    name: (id) => smcControlName(id, t),
    label: controlLabel,
    assigned: (id) => buttonActions(id).length > 0,
    selected,
    value: (id) => (id < 16 ? valueAt(SMC_COLUMNS[id]) : -1),
    lit: (id) => (litUntil.get(id) || 0) > Date.now(),
  });
  keepScroll(() => {
    document.getElementById("panel").innerHTML = `
    <div class="tabpanel general-grid" role="tabpanel">
      ${connectionGroup(invertRow)}${profileGroup}
      <div class="device-row">
        <div class="group">
          <div class="group-head"><h2 class="group-title">${esc(draft.mixerPort)}</h2></div>
          <div class="card device-card">${svg}</div>
        </div>
        ${inspector()}
        <div class="group-foot device-foot"><span class="group-note">${esc(t("smc.hint"))}</span></div>
      </div>
      ${importFoot}
    </div>`;
  });
}

// The label under a fader or knob: its first job and how many more it has. A screen's short
// name says which screen but not whether brightness or contrast, so those keep their title.
function controlLabel(id) {
  if (isButtonControl(id)) {
    const actions = buttonActions(id);
    const title = actions.map(actionLabel).join(", ");
    if (usesSMC()) return { title };
    if (!actions.length) return { text: t("job.empty"), empty: true };
    return { text: actionLabel(actions[0]), more: actions.length - 1, title };
  }
  const jobs = deviceJobs(activeProfile())[id] || [];
  if (!jobs.length) return { text: t("job.empty"), empty: true };
  const titleOf = (job) => (jobEntryFor(job) || {}).title || job.exe || job.kind;
  const entry = jobEntryFor(jobs[0]);
  const text = entry && jobs[0].kind !== "brightness" && jobs[0].kind !== "contrast" ? entry.short : titleOf(jobs[0]);
  return { text, more: jobs.length - 1, title: jobs.map(titleOf).join(", ") };
}

function inspector() {
  const id = selected;
  const strip = isStripControl(id);
  const empty = strip ? !(deviceJobs(activeProfile())[id] || []).length : !buttonActions(id).length;
  return `
        <div class="group inspector">
          <div class="group-head">
            ${strip ? "" : smcButtonIcon(id)}<h2 class="group-title">${esc(smcControlName(id, t))}</h2>
            <span class="spacer"></span>
            <button class="btn" type="button" data-action="clear-control"${empty ? " disabled" : ""}>${esc(t("clear"))}</button>
          </div>
          <div class="card inspector-card" data-keep-scroll="control-${id}">${strip ? jobPicks(id) : actionPicks(id)}</div>
        </div>`;
}

function pickRow(n, checked, data, title, icon, badge) {
  return `<label class="pick-row"><input class="chk" type="checkbox" id="pick-${n}" ${data}${checked ? " checked" : ""} />${icon ? `<img src="${escAttr(icon)}" alt="" />` : ""}<span class="pick-title">${esc(title)}</span>${badge ? `<span class="badge">${esc(t("experimental"))}</span>` : ""}</label>`;
}

function jobPicks(knob) {
  const jobs = deviceJobs(activeProfile())[knob] || [];
  const bySection = sectionsFromCatalog();
  const order = [...bySection.keys()].filter((s) => s !== "apps").concat("apps");
  let n = 0;
  let html = "";
  for (const section of order) {
    html += `<div class="pick-head">${esc(t(SECTION_LABEL_KEY[section] || ""))}</div>`;
    for (const entry of bySection.get(section) || []) {
      const badge = entry.job.kind === "nightLight" && !!init.nightLightExperimental;
      html += pickRow(n++, hasJob(jobs, entry.job), `data-pick="job" data-job="${escAttr(jobKey(entry.job))}"`, entry.title, entry.icon, badge);
    }
  }
  return html + `<div class="pick-more"><button class="btn" type="button" data-action="pick-control-app">${esc(t("other"))}</button></div>`;
}

// The same choices as a button's popup menu, ticked in place; an action that takes a setting
// shows what sets it under its tick.
function actionPicks(cc) {
  const actions = buttonActions(cc);
  const kinds = new Set(actions.map(actionKind));
  let n = 0;
  const row = ([value, title]) => {
    const html = pickRow(n++, kinds.has(value), `data-pick="action" data-value="${escAttr(value)}"`, title);
    const action = PARAM_KINDS.includes(value) && actions.find((a) => actionKind(a) === value);
    return action ? html + `<div class="pick-param">${paramControl(cc, action)}</div>` : html;
  };
  const group = (title, items) => `<div class="pick-head">${esc(title)}</div>` + items.map(row).join("");
  const mute = (k) => [`mute:${k}`, t("action.mute", { name: controlName(k) })];
  const mutes = (from) => Array.from({ length: 8 }, (_, i) => mute(from + i));
  const groups = BUTTON_GROUPS.map(([key, items]) => {
    const list = items.map(([value, k]) => [value, t(k)]);
    if (key === "action.group.weej") {
      list.splice(2, 0, ...draft.profiles.map((_, i) => [`profile:${i}`, t("action.go_profile", { name: profileLabel(i) })]));
    }
    return group(t(key), list);
  });
  const muteGroups = usesBoard()
    ? group(t("knobs"), draft.columns.map((_, k) => k).filter((k) => kindOf(k) !== "button").map(mute))
    : group(t("mixer.faders"), mutes(0)) + group(t("knobs"), mutes(8));
  return groups.join("") + group(t("action.group.fkeys"), FKEYS) + muteGroups;
}

function onPick(el) {
  if (el.dataset.pick === "job") {
    const entry = init.catalog.find((c) => jobKey(c.job) === el.dataset.job);
    if (!entry) return;
    const knobs = deviceJobs(activeProfile());
    const jobs = knobs[picked()] || (knobs[picked()] = []);
    const idx = jobs.findIndex((j) => jobKey(j) === el.dataset.job);
    if (el.checked && idx < 0) jobs.push(entry.job);
    if (!el.checked && idx >= 0) jobs.splice(idx, 1);
    render();
    return;
  }
  const value = el.dataset.value;
  const id = picked();
  onButtonMenuToggle(id, value, el.checked);
  // A website or keys to press need setting right away, so their box takes over.
  if (el.checked && value === "url:") document.getElementById(`url-${id}`)?.focus();
  if (el.checked && value === "keys:") startRecording(`button:${id}`);
}

function selectControl(id) {
  if (id === picked()) return;
  if (usesBoard()) boardSelected = id;
  else selected = id;
  render();
}

// The picked control has no input yet, so the next one moved on the board becomes it.
function waitingForInput() {
  return usesBoard() && boardSelected < draft.columns.length && draft.columns[boardSelected] === -1;
}

// Moving or pressing a control on the mixer lights it, and picks it unless an address is being
// typed or keys recorded for the one picked.
function onControlTouched(id) {
  if (!isDeviceTab() || !document.getElementById(`ctl-${id}`)) return;
  const el = document.activeElement;
  const typing = el && (el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && el.type === "text"));
  if (id !== picked() && !recording && !typing && !waitingForInput()) {
    if (el && el.closest && el.closest(".inspector")) el.blur();
    selectControl(id);
  }
  lightControl(id);
}

// A control stays lit for a second after it last moved, as a row in the lists does.
function lightControl(id) {
  litUntil.set(id, Date.now() + 1000);
  document.getElementById(`ctl-${id}`)?.classList.add("lit");
  clearTimeout(controlTimers.get(id));
  controlTimers.set(
    id,
    setTimeout(() => document.getElementById(`ctl-${id}`)?.classList.remove("lit"), 1000)
  );
}

function currentDevice() {
  return usesMixer() ? "mixer" : "board";
}

// Drawn the way the volume moves, so 0 to 100% turns clockwise: core's engine reads the board as
// 1 - raw and the mixer as raw, each flipped by its Invert as saved, which is what it runs with.
function valueAt(col) {
  const v = live[currentDevice()][col];
  if (v === undefined || col < 0) return -1;
  const s = saved || draft;
  const flip = currentDevice() === "mixer" ? s.invertMixer : !s.invertKnobs;
  return flip ? 1023 - v : v;
}

// Moves the drawn faders and knobs to a device's new frame, and finds the input of a control
// waiting for one.
function showValues(device, values) {
  const before = live[device] || [];
  live[device] = values;
  if (!isDeviceTab() || !draft || device !== currentDevice()) return;
  const columns = usesSMC() ? SMC_COLUMNS : usesBoard() ? draft.columns : [];
  columns.forEach((col, id) => {
    if (col >= 0 && before[col] !== values[col]) showValue(document, id, valueAt(col));
  });
  if (usesBoard()) learnInput(before);
}

// A control waiting for its input takes the first unused one that swings a good way, so a pot's
// jitter or a brush against another control doesn't count.
const LEARN_SWING = 300;

function learnInput(before) {
  if (!waitingForInput()) {
    learning = null;
    return;
  }
  if (!learning || learning.knob !== boardSelected) {
    learning = { knob: boardSelected, start: before.length ? before : live.board };
    return;
  }
  for (let col = 0; col < live.board.length; col++) {
    if (draft.columns.includes(col) || learning.start[col] === undefined) continue;
    if (Math.abs(live.board[col] - learning.start[col]) >= LEARN_SWING) {
      const k = boardSelected;
      draft.columns[k] = col;
      learning = null;
      render();
      lightControl(k);
      return;
    }
  }
}

// --- Board --------------------------------------------------------------------

// The board drawn in its rows, with what the control picked on it does beside it. A knob, fader
// or button added with + waits for its input until it moves on the board.
function renderBoardGeneral(profileGroup, importFoot) {
  if (boardSelected >= draft.columns.length) boardSelected = Math.max(0, draft.columns.length - 1);
  const svg = boardSVG({
    layout: boardLayout(),
    kind: kindOf,
    name: boardName,
    label: controlLabel,
    assigned: (k) => buttonActions(k).length > 0,
    waiting: (k) => draft.columns[k] === -1,
    selected: draft.columns.length ? boardSelected : -1,
    value: (k) => valueAt(draft.columns[k]),
    lit: (id) => (litUntil.get(id) || 0) > Date.now(),
    addLabel: t("board.add"),
  });
  const calibrate = `<button class="btn" type="button" data-action="calibrate"${calibrating ? " disabled" : ""}>${esc(t("calibrate"))}</button>`;
  const invertRow = `
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("invert"))}</span>
              <span class="row-desc">${esc(t("invert_note"))}</span>
            </div>
            <div class="row-control"><input class="toggle" id="invert" type="checkbox" role="switch"${draft.invertKnobs ? " checked" : ""} /></div>
          </div>`;
  keepScroll(() => {
    document.getElementById("panel").innerHTML = `
    <div class="tabpanel general-grid" role="tabpanel">
      ${connectionGroup(invertRow)}${profileGroup}
      <div class="device-row">
        <div class="group">
          <div class="group-head"><h2 class="group-title">${esc(t("board.title"))}</h2><span class="spacer"></span>${calibrate}</div>
          <div class="card device-card">${svg}</div>
        </div>
        ${boardInspector()}
        <div class="group-foot device-foot"><span class="group-note">${esc(t("board.hint"))}</span></div>
      </div>
      ${importFoot}
    </div>`;
  });
}

function boardInspector() {
  const k = boardSelected;
  if (!draft.columns.length) {
    return `
        <div class="group inspector">
          <div class="group-head"><h2 class="group-title">${esc(t("board.title"))}</h2></div>
          <div class="card inspector-card"><p class="inspector-empty">${esc(t("board.empty"))}</p></div>
        </div>`;
  }
  const kind = kindOf(k);
  const button = kind === "button";
  const empty = button ? !buttonActions(k).length : !(deviceJobs(activeProfile())[k] || []).length;
  const kinds = ["knob", "fader", "button"]
    .map((kd) => `<button class="btn${kd === kind ? " on" : ""}" type="button" data-action="set-kind" data-kind="${kd}" aria-pressed="${kd === kind}">${esc(t("board.kind_" + kd))}</button>`)
    .join("");
  const col = draft.columns[k];
  const input =
    col >= 0
      ? `<span class="row-desc">${esc(t("board.input", { n: String(col + 1) }))}</span><button class="btn btn-icon btn-subtle" type="button" data-action="find-input" title="${escAttr(t("board.find_again"))}" aria-label="${escAttr(t("board.find_again"))}">&#x21bb;</button>`
      : `<span class="waiting-note">${esc(t("board.waiting"))}</span>`;
  const moves = [
    ["up", "&#x2191;"],
    ["left", "&#x2190;"],
    ["down", "&#x2193;"],
    ["right", "&#x2192;"],
  ]
    .map(([dir, arrow]) => `<button class="btn btn-icon" type="button" data-action="move-control" data-dir="${dir}" title="${escAttr(t("board." + dir))}" aria-label="${escAttr(t("board." + dir))}"${canMove(k, dir) ? "" : " disabled"}>${arrow}</button>`)
    .join("");
  return `
        <div class="group inspector">
          <div class="group-head">
            <h2 class="group-title">${esc(boardName(k))}</h2>
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
          <div class="card inspector-card" data-keep-scroll="board-${k}">${button ? actionPicks(k) : jobPicks(k)}</div>
        </div>`;
}

// The saved layout made drawable, as core.CleanLayout does: one row of every knob when there is
// none, and any knob it misses on the last row.
function boardLayout() {
  const n = draft.columns.length;
  const seen = new Set();
  const rows = (draft.boardLayout || [Array.from({ length: n }, (_, i) => i)])
    .map((row) => row.filter((i) => i >= 0 && i < n && !seen.has(i) && seen.add(i)))
    .filter((row) => row.length);
  const missing = Array.from({ length: n }, (_, i) => i).filter((i) => !seen.has(i));
  if (missing.length) {
    if (!rows.length) rows.push([]);
    rows[rows.length - 1].push(...missing);
  }
  return rows;
}

function editableLayout() {
  draft.boardLayout = boardLayout();
  return draft.boardLayout;
}

function setKind(k, kind) {
  draft.boardKinds = draft.columns.map((_, i) => (i === k ? kind : kindOf(i)));
}

function addBoardControl(row) {
  const layout = editableLayout();
  const k = draft.columns.length;
  const last = layout[row] && layout[row][layout[row].length - 1];
  const kind = last === undefined ? "knob" : kindOf(last);
  draft.columns.push(-1);
  setKind(k, kind);
  if (!layout[row]) layout[row] = [];
  layout[row].push(k);
  padJobRows(draft);
  boardSelected = k;
  render();
}

function canMove(k, dir) {
  const layout = boardLayout();
  const r = layout.findIndex((row) => row.includes(k));
  if (r < 0) return false;
  const i = layout[r].indexOf(k);
  if (dir === "left") return i > 0;
  if (dir === "right") return i < layout[r].length - 1;
  // Moving past the first or last row starts a new one, unless the control is alone in its row.
  if (dir === "up") return r > 0 || layout[r].length > 1;
  return r < layout.length - 1 || layout[r].length > 1;
}

function moveControl(k, dir) {
  if (!canMove(k, dir)) return;
  const layout = editableLayout();
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
    draft.boardLayout = layout.filter((row) => row.length);
  }
  render();
}

// Takes control k off the board, as removing a knob does: its input, place, jobs and button
// actions go, and every later control moves down one, its mutes with it.
function boardControlUsed(k) {
  return draft.profiles.some(
    (p) =>
      ((p.jobs || [])[k] || []).length > 0 ||
      Object.entries(p.boardButtons || {}).some(([id, actions]) => (parseInt(id, 10) === k && actions.length > 0) || actions.includes(`mute:${k}`)),
  );
}

function removeBoardControl(k) {
  const shift = (i) => (i > k ? i - 1 : i);
  draft.boardLayout = boardLayout()
    .map((row) => row.filter((i) => i !== k).map(shift))
    .filter((row) => row.length);
  draft.columns.splice(k, 1);
  if (draft.boardKinds) draft.boardKinds.splice(k, 1);
  const muteOf = (a) => (a.startsWith("mute:") ? parseInt(a.slice(5), 10) : -1);
  for (const p of draft.profiles) {
    if (p.jobs) p.jobs.splice(k, 1);
    if (!p.boardButtons) continue;
    const buttons = {};
    for (const [key, actions] of Object.entries(p.boardButtons)) {
      const id = parseInt(key, 10);
      if (id === k) continue;
      const kept = actions.filter((a) => muteOf(a) !== k).map((a) => (muteOf(a) > k ? `mute:${muteOf(a) - 1}` : a));
      if (kept.length) buttons[shift(id)] = kept;
    }
    p.boardButtons = buttons;
  }
  boardSelected = Math.max(0, Math.min(boardSelected, draft.columns.length - 1));
}

// The drawn controls take Enter and Space as buttons do, and the arrow keys walk between them.
function onControlKeydown(e) {
  const ctl = e.target.closest && e.target.closest(".smc .ctl, .smc .add");
  if (!ctl) return;
  if (e.key === "Enter" || e.key === " ") {
    e.preventDefault();
    e.stopPropagation();
    if (ctl.classList.contains("add")) addBoardControl(parseInt(ctl.dataset.row, 10));
    else selectControl(parseInt(ctl.dataset.control, 10));
    return;
  }
  const step = { ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1 }[e.key];
  if (!step) return;
  e.preventDefault();
  const all = [...document.querySelectorAll(".smc .ctl, .smc .add")];
  const next = all[all.indexOf(ctl) + step];
  if (next) next.focus();
}

function renderApp() {
  const langOptions = (init.languages || [])
    .map((l) => `<option value="${escAttr(l.code)}"${l.code === draft.language ? " selected" : ""}>${esc(l.name)}</option>`)
    .join("");

  const profileShortcuts = draft.profiles
    .map(
      (p, i) => `
      <div class="row">
        <div class="row-main"><span class="row-title">${esc(p.name || t("profile_n", { n: String(i + 1) }))}</span></div>
        <div class="row-control">${shortcutControl("profile:" + i, p.shortcut)}</div>
      </div>`
    )
    .join("");

  const trayPreview =
    init.iconPreviews && init.iconPreviews[draft.trayIcon]
      ? `<img src="${init.iconPreviews[draft.trayIcon]}" alt="" data-style="width:20px;height:20px;border-radius:4px;" />`
      : "";
  const iconOptions = ["mixer", "dial", "app"]
    .map((style) => `<option value="${style}"${draft.trayIcon === style ? " selected" : ""}>${esc(t("icon." + style))}</option>`)
    .join("");
  const speedOptions = ["slow", "medium", "fast", "superFast"]
    .map((s) => `<option value="${s}"${draft.speed === s ? " selected" : ""}>${esc(t("speed." + s))}</option>`)
    .join("");

  document.getElementById("panel").innerHTML = `
    <div class="tabpanel" role="tabpanel">
      <div class="group">
        <div class="card">
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("language"))}</span></div>
            <div class="row-control"><select class="select" id="language-select">${langOptions}</select></div>
          </div>
        </div>
      </div>

      <div class="group">
        <h2 class="group-title">${esc(t("shortcuts"))}</h2>
        <div class="card">
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("next_profile"))}</span></div>
            <div class="row-control">${shortcutControl("next", draft.nextProfile)}</div>
          </div>
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("previous_profile"))}</span></div>
            <div class="row-control">${shortcutControl("previous", draft.previousProfile)}</div>
          </div>
          ${profileShortcuts}
        </div>
      </div>

      <div class="group">
        <h2 class="group-title">${esc(t("tray"))}</h2>
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("hide_icon"))}</span>
              <span class="row-desc">${esc(t("hide_icon_note"))}</span>
            </div>
            <div class="row-control"><input class="toggle" id="hide-icon" type="checkbox" role="switch"${draft.hideTrayIcon ? " checked" : ""} /></div>
          </div>
          <div class="row">
            <div class="row-main"><span class="row-title"${draft.hideTrayIcon ? ' data-style="color:var(--text-disabled)"' : ""}>${esc(t("icon"))}</span></div>
            <div class="row-control">
              ${trayPreview}
              <select class="select" id="tray-icon-style"${draft.hideTrayIcon ? " disabled" : ""}>${iconOptions}</select>
            </div>
          </div>
          <div class="row">
            <div class="row-main"><span class="row-title"${draft.hideTrayIcon ? ' data-style="color:var(--text-disabled)"' : ""}>${esc(t("profile_list"))}</span></div>
            <div class="row-control"><input class="toggle" id="show-profile-list" type="checkbox" role="switch"${draft.showProfileList ? " checked" : ""}${draft.hideTrayIcon ? " disabled" : ""} /></div>
          </div>
        </div>
      </div>

      <div class="group">
        <h2 class="group-title">${esc(t("sensitivity"))}</h2>
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("speed"))}</span>
              <span class="row-desc">${esc(t("speed_note"))}</span>
            </div>
            <div class="row-control"><select class="select" id="speed-select">${speedOptions}</select></div>
          </div>
        </div>
      </div>
    </div>`;
}

function portLabel(p) {
  return p.product ? `${p.name} (${p.product})` : p.name;
}

// The board and the mixer each have a tab, and every device setting follows the one showing.
function isDeviceTab() {
  return activeTab === "board" || activeTab === "mixer";
}

function usesMixer() {
  return activeTab === "mixer";
}

function usesSMC() {
  return usesMixer() && isSMCName(draft.mixerPort);
}

function usesBoard() {
  return activeTab === "board";
}

// Go opens Settings on "general": the mixer's tab when it is the only device, else the board's.
function tabFor(tab) {
  if (tab && tab !== "general") return tab;
  return draft.port === "off" && draft.mixerPort ? "mixer" : "board";
}

// The control the inspector shows, on whichever device is drawn.
function picked() {
  return usesBoard() ? boardSelected : selected;
}

function kindOf(k) {
  return (draft.boardKinds || [])[k] || "knob";
}

function isButtonControl(id) {
  return usesBoard() ? kindOf(id) === "button" : !isStripControl(id);
}

function boardName(k) {
  const letter = letterFor(k);
  if (kindOf(k) === "fader") return t("board.fader", { letter });
  if (kindOf(k) === "button") return t("board.button", { letter });
  return t("knob", { letter });
}

// The knobs of whatever is connected, as Go's activeColumns picks them; only how many matters
// here. The SMC-Mixer's are fixed.
function knobColumns() {
  if (!usesMixer()) return draft.columns;
  if (usesSMC()) return SMC_COLUMNS;
  return draft.mixerColumns || draft.columns.map((_, i) => i);
}

function knobCount() {
  return knobColumns().length;
}

// Each device has its own knob jobs in every profile: jobs for the board, mixerJobs for the
// mixer, as core.Profile keeps them.
function deviceJobs(profile) {
  const key = usesMixer() ? "mixerJobs" : "jobs";
  return profile[key] || (profile[key] = []);
}

function padJobRows(setup) {
  for (const p of setup.profiles) {
    const jobs = deviceJobs(p);
    while (jobs.length < knobCount()) jobs.push([]);
  }
}

// A button's functions are core.ButtonAction strings, ticked in the same popup menu as a knob's
// jobs. The ones ending in ":" take a setting after it, which the button's row asks for.
const BUTTON_GROUPS = [
  [
    "action.group.media",
    [
      ["media.playpause", "action.play_pause"],
      ["media.play", "action.play"],
      ["media.pause", "action.pause"],
      ["media.stop", "action.stop"],
      ["media.previous", "action.previous_track"],
      ["media.next", "action.next_track"],
      ["volume.up", "action.volume_up"],
      ["volume.down", "action.volume_down"],
      ["mute.all", "action.mute_all"],
    ],
  ],
  [
    "action.group.apps",
    [
      ["open:", "action.open_app"],
      ["close:", "action.close_app"],
      ["url:", "action.open_url"],
    ],
  ],
  [
    "action.group.system",
    [
      ["mute.mic", "action.mute_mic"],
      ["keys:", "action.keys"],
      ["nightlight", "action.night_light"],
      ["screens.off", "action.screens_off"],
      ["pc.lock", "action.lock"],
      ["pc.sleep", "action.sleep"],
    ],
  ],
  [
    "action.group.weej",
    [
      ["profile.previous", "previous_profile"],
      ["profile.next", "next_profile"],
      ["settings", "action.open_settings"],
      ["lights.next", "action.next_lights"],
    ],
  ],
];
const PARAM_KINDS = ["open:", "close:", "url:", "keys:"];

// The popup menu tells a button from a knob by this offset on the id it sends back.
const BUTTON_MENU_BASE = 10000;

// F13 to F24, keys no keyboard has, so other apps can bind them to a button without clashing.
const FKEYS = Array.from({ length: 12 }, (_, i) => [`keys:0:${0x7c + i}:F${13 + i}`, `F${13 + i}`]);

// The menu entry an action is ticked under: itself, or the kind of one that takes a setting.
function actionKind(action) {
  if (FKEYS.some(([value]) => value === action)) return action;
  return PARAM_KINDS.find((kind) => action.startsWith(kind)) || action;
}

function profileLabel(i) {
  const p = draft.profiles[i];
  return p && p.name ? p.name : t("profile_n", { n: String(i + 1) });
}

// A mute names its control: the SMC-Mixer's fader or knob, the board's, or another mixer's knob.
function controlName(knob) {
  if (usesSMC()) return smcControlName(knob, t);
  return usesBoard() ? boardName(knob) : t("knob", { letter: letterFor(knob) });
}

function actionLabel(action) {
  const kind = actionKind(action);
  const fkey = FKEYS.find(([value]) => value === kind);
  if (fkey) return fkey[1];
  for (const [, items] of BUTTON_GROUPS) for (const [value, key] of items) if (value === kind) return t(key);
  if (kind.startsWith("mute:")) return t("action.mute", { name: controlName(parseInt(kind.slice(5), 10)) });
  if (kind.startsWith("profile:")) return t("action.go_profile", { name: profileLabel(parseInt(kind.slice(8), 10)) });
  return kind;
}

// The board's buttons keep their actions apart from the mixer's, as core.Profile does.
function buttonsKey() {
  return usesBoard() ? "boardButtons" : "buttons";
}

function buttonActions(cc) {
  return (activeProfile()[buttonsKey()] || {})[cc] || [];
}

function setButtonActions(cc, actions) {
  const profile = activeProfile();
  const key = buttonsKey();
  profile[key] = profile[key] || {};
  if (actions.length) profile[key][cc] = actions;
  else delete profile[key][cc];
}

// Puts action in place of whatever the button had of the same kind, so each kind is there once.
function replaceKind(cc, action) {
  const kind = actionKind(action);
  setButtonActions(cc, [...buttonActions(cc).filter((a) => actionKind(a) !== kind), action]);
}

let appNames = {}; // an app's path or exe, as an action holds it, to the name the picker found

function baseName(path) {
  return path.split(/[\\/]/).pop();
}

// "keys:<mods>:<vk>:<key>", as core.KeysAction writes it.
function keysShortcut(value) {
  const [mods, vk, ...key] = value.split(":");
  return vk ? { vk: parseInt(vk, 10), mods: parseInt(mods, 10), key: key.join(":") } : null;
}

// What sets the part of an action after its ":": an address box, keys to record or an app.
function paramControl(cc, action) {
  const kind = actionKind(action);
  const value = action.slice(kind.length);
  if (kind === "url:") {
    return `<input class="input" type="text" id="url-${cc}" data-button-url="${cc}" value="${escAttr(value)}" placeholder="https://" spellcheck="false" />`;
  }
  if (kind === "keys:") return shortcutControl(`button:${cc}`, keysShortcut(value));
  const label = value ? appNames[value] || baseName(value) : t("choose");
  return `<button class="btn" type="button" data-action="pick-button-app" data-cc="${cc}" data-mode="${kind.slice(0, -1)}">${esc(label)}</button>`;
}

function buttonParams(cc) {
  return buttonActions(cc)
    .filter((action) => PARAM_KINDS.includes(actionKind(action)))
    .map((action) => `<div class="button-param"><span class="param-label">${esc(actionLabel(action))}</span>${paramControl(cc, action)}</div>`)
    .join("");
}

function buttonLines(cc) {
  const actions = buttonActions(cc);
  if (!actions.length) return `<span class="job-line muted">${esc(t("job.empty"))}</span>`;
  return actions.map((a) => `<span class="job-line"><span>${esc(actionLabel(a))}</span></span>`).join("");
}

function openButtonMenu(cc, row) {
  const r = row.getBoundingClientRect();
  const kinds = new Set(buttonActions(cc).map(actionKind));
  const item = (action, title) => ({ job: { action }, title, icon: TRANSPARENT_PIXEL, checked: kinds.has(action) });
  const sections = BUTTON_GROUPS.map(([key, items]) => ({ title: t(key), items: items.map(([v, k]) => item(v, t(k))) }));
  const weej = sections[sections.length - 1];
  weej.items.splice(2, 0, ...draft.profiles.map((_, i) => item(`profile:${i}`, t("action.go_profile", { name: profileLabel(i) }))));
  sections.push({ title: t("action.group.fkeys"), items: FKEYS.map(([v, label]) => item(v, label)) });
  const mutes = [];
  for (let i = 0; i < knobCount(); i++) mutes.push(item(`mute:${i}`, t("action.mute", { name: controlName(i) })));
  sections.push({ title: t("action.group.knobs"), items: mutes });
  send({
    type: "openJobMenu",
    knob: BUTTON_MENU_BASE + cc,
    anchor: { left: r.left, top: r.top, right: r.right, bottom: r.bottom },
    model: { sections },
  });
}

function onButtonMenuToggle(cc, action, checked) {
  if (!checked) {
    setButtonActions(cc, buttonActions(cc).filter((a) => actionKind(a) !== action));
  } else if (PARAM_KINDS.includes(action)) {
    replaceKind(cc, action);
    if (action === "open:" || action === "close:") send({ type: "pickApp", button: cc, mode: action.slice(0, -1) });
  } else if (!buttonActions(cc).includes(action)) {
    setButtonActions(cc, [...buttonActions(cc), action]);
  }
  render();
}

function buttonOrder() {
  return draft.mixerButtonOrder || init.mixerButtonDefaults || [];
}

function editableButtonOrder() {
  if (!draft.mixerButtonOrder) draft.mixerButtonOrder = clone(init.mixerButtonDefaults || []);
  return draft.mixerButtonOrder;
}

// A row added with + reads -1 until a mixer button is pressed to fill it in.
function renderMixerButtons() {
  const mixer = usesMixer();
  const order = mixer ? buttonOrder() : [];
  const rows = order.length
    ? order
        .map((cc, i) => {
          const title = `<span class="row-title">${esc(t("mixer.button", { n: String(i + 1) }))}</span>`;
          if (cc < 0) {
            return `
          <div class="row">
            <div class="row-main">${title}<span class="row-desc warning">${esc(t("mixer.press"))}</span></div>
          </div>`;
          }
          return `
          <div class="row clickable" data-cc="${cc}" role="button" tabindex="0" data-action="open-button-menu" data-button="${cc}">
            <div class="row-main">${title}${buttonParams(cc)}</div>
            <div class="row-control">
              <div class="job-list">${buttonLines(cc)}</div>
              <span class="chev" aria-hidden="true">&#x2304;</span>
            </div>
          </div>`;
        })
        .join("")
    : `<div class="row"><span class="row-desc">${esc(t("mixer.none"))}</span></div>`;
  return `
      <div class="group list-group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("mixer_buttons"))}</h2>
          <span class="spacer"></span>
          <span class="segmented">
            <button class="btn btn-icon" type="button" data-action="add-button" title="${escAttr(t("add_button"))}" aria-label="${escAttr(t("add_button"))}"${mixer ? "" : " disabled"}>+</button>
            <button class="btn btn-icon" type="button" data-action="remove-button" title="${escAttr(t("remove_button"))}" aria-label="${escAttr(t("remove_button"))}"${order.length === 0 ? " disabled" : ""}>&minus;</button>
          </span>
        </div>
        <div class="card" data-keep-scroll="buttons">${rows}</div>
        <div class="group-foot"><span class="group-note">${esc(t("mixer_buttons_note"))}</span></div>
      </div>`;
}

function addButton() {
  editableButtonOrder().push(-1);
  render();
}

function removeButton() {
  const cc = editableButtonOrder().pop();
  if (cc >= 0) for (const p of draft.profiles) if (p.buttons) delete p.buttons[cc];
  render();
}

// A pressed button not on the list yet fills the first row waiting for one.
function onMixerButtonPressed(cc) {
  if (!isDeviceTab()) return;
  const order = buttonOrder();
  if (!order.includes(cc) && order.includes(-1)) {
    const editable = editableButtonOrder();
    editable[editable.indexOf(-1)] = cc;
    render();
    updateSaveButton();
  }
  lightRow(document.querySelector(`.row[data-cc="${cc}"]`));
}

// A row stays lit for a second after its control's last move, so a moving fader holds it steady.
const litTimers = new WeakMap();

function lightRow(row) {
  if (!isDeviceTab() || !row) return;
  if (!row.classList.contains("lit")) {
    row.classList.add("lit");
    row.scrollIntoView({ block: "nearest" });
  }
  clearTimeout(litTimers.get(row));
  litTimers.set(
    row,
    setTimeout(() => row.classList.remove("lit"), 1000)
  );
}

function connectionGroup(extra = "") {
  const midi = usesMixer();
  const conn = midi ? mixerConnection : connection;
  const forced = midi ? "" : init.forcedPort || "";
  const value = (midi ? draft.mixerPort : draft.port) || "";
  const option = (v, label) => `<option value="${escAttr(v)}"${value === v ? " selected" : ""}>${esc(label)}</option>`;
  let portOptions;
  if (forced) {
    portOptions = `<option selected>${esc(forced)}</option>`;
  } else if (midi) {
    portOptions = option("", t("port_off")) + midiInputs.map((name) => option(name, name)).join("");
    // A saved device that is unplugged right now still has to show as the choice.
    if (value && !midiInputs.includes(value)) portOptions += option(value, value);
  } else {
    const list = ports || [];
    const autoLabel = !value && conn.connected && conn.port ? t("port_auto_found", { port: conn.port }) : t("port_auto");
    portOptions = option("", autoLabel) + option("off", t("port_off")) + list.map((p) => option(p.name, portLabel(p))).join("");
    if (value && value !== "off" && !list.some((p) => p.name === value)) portOptions += option(value, value);
  }

  const rates = (init.baudRates || [9600]).slice();
  if (draft.baudRate && !rates.includes(draft.baudRate)) rates.push(draft.baudRate);
  const baudOptions = rates
    .map((r) => `<option value="${r}"${r === draft.baudRate ? " selected" : ""}>${r}</option>`)
    .join("");

  let status = t("not_connected");
  let statusClass = "";
  if (conn.connected) {
    status = t("connected", { port: conn.port });
    statusClass = " status-ok";
  } else if (conn.busy && conn.port) {
    status = t("port_busy", { port: conn.port });
    statusClass = " warning";
  }

  return `
      <div class="group">
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("status"))}</span>
              <span class="row-desc${statusClass}">${esc(status)}</span>
            </div>
            <div class="row-control"><button class="btn" type="button" data-action="reconnect">${esc(t("reconnect"))}</button></div>
          </div>
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("port"))}</span>
              <span class="row-desc">${esc(forced ? t("port_forced", { port: forced }) : t(midi ? "mixer_port_note" : "port_note"))}</span>
            </div>
            <div class="row-control">
              <button class="btn btn-icon btn-subtle" type="button" data-action="refresh-ports" title="${escAttr(t("refresh"))}" aria-label="${escAttr(t("refresh"))}"${forced ? " disabled" : ""}>&#x21bb;</button>
              <select class="select" id="port-select"${forced ? " disabled" : ""}>${portOptions}</select>
            </div>
          </div>
          ${
            midi
              ? ""
              : `<div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("baud_rate"))}</span>
              <span class="row-desc">${esc(t("baud_note"))}</span>
            </div>
            <div class="row-control"><select class="select" id="baud-select">${baudOptions}</select></div>
          </div>`
          }${extra}
        </div>
      </div>`;
}

function hostnameOf(url) {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

function renderAbout() {
  document.getElementById("panel").innerHTML = `
    <div class="tabpanel" role="tabpanel" data-style="align-items:center;text-align:center;padding-top:4px;gap:0;">
      <img src="${escAttr(init.icon || TRANSPARENT_PIXEL)}" alt="" width="64" height="64" data-style="border-radius:14px;" />
      <h2 data-style="font:600 16px var(--font-display);margin:12px 0 0;">WeeJ</h2>
      <p class="row-desc" data-style="margin-top:2px;">${esc(t("version", { version: init.version }))}</p>
      <button class="btn" type="button" data-action="check-updates" data-style="margin-top:14px;">${esc(t("check"))}</button>
      <p data-style="margin-top:20px;"><a href="#" data-action="open-url" data-url="${escAttr(init.website)}">${esc(t("website"))}</a></p>
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("made_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.madeBy)}">${esc(hostnameOf(init.madeBy))}</a></p>
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("on_macos"))} <a href="#" data-action="open-url" data-url="${escAttr(init.theej)}">TheeJ</a></p>
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("inspired_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.deej)}">deej</a></p>
    </div>`;
}

// The job menu is a popup window of its own (jobs.html), so it can hang past this window's
// edge. This page owns the list, the catalog and the knob's unsaved jobs, so it sends the
// whole menu, and Go hands every tick back as jobMenuToggle or jobMenuClear.
function openJobMenu(knob, row) {
  const r = row.getBoundingClientRect();
  const knobJobs = deviceJobs(activeProfile())[knob] || [];
  const item = (entry) => ({
    job: entry.job,
    title: entry.title,
    icon: entry.icon,
    badge: entry.job.kind === "nightLight" && !!init.nightLightExperimental,
    checked: hasJob(knobJobs, entry.job),
  });
  const bySection = sectionsFromCatalog();
  const sections = [];
  for (const [section, entries] of bySection) {
    if (section === "apps") continue;
    sections.push({ title: t(SECTION_LABEL_KEY[section] || ""), items: entries.map(item) });
  }
  sections.push({ title: t("section.apps"), items: (bySection.get("apps") || []).map(item), other: true });
  send({
    type: "openJobMenu",
    knob,
    anchor: { left: r.left, top: r.top, right: r.right, bottom: r.bottom },
    model: { sections },
  });
}

function renderDialog() {
  let title, body;
  if (dialog.kind === "removeProfile") {
    const name = activeProfile().name;
    title = name ? t("remove_named", { name }) : t("remove_this_profile");
    body = t("remove_profile_info");
  } else if (dialog.kind === "removeControl") {
    title = t("remove_named", { name: boardName(boardSelected) });
    body = t("remove_knob_info");
  } else {
    title = t("remove_knob_q", { letter: letterFor(knobCount() - 1) });
    body = t("remove_knob_info");
  }
  const html = `
    <div class="dialog-scrim" data-action="cancel-dialog">
      <div class="dialog">
        <div class="dialog-title">${esc(title)}</div>
        <div class="dialog-body">${esc(body)}</div>
        <div class="dialog-actions">
          <button class="btn btn-primary btn-danger" type="button" data-action="confirm-dialog">${esc(t("remove"))}</button>
          <button class="btn" type="button" data-action="cancel-dialog">${esc(t("cancel"))}</button>
        </div>
      </div>
    </div>`;
  document.getElementById("root").insertAdjacentHTML("beforeend", html);
}

// --- Mutations --------------------------------------------------------------

function addProfile() {
  draft.profiles.push({ name: "", jobs: [], mixerJobs: [], shortcut: null, buttons: {} });
  padJobRows(draft);
  draft.profile = draft.profiles.length - 1;
  render();
}

function removeProfileConfirmed() {
  draft.profiles.splice(draft.profile, 1);
  draft.profile = clampIndex(draft.profile, draft.profiles.length);
}

// --- Knob reorder -----------------------------------------------------------

// Six dots, Windows' sign that a row can be dragged to a new place.
const GRIP_ICON = `<svg viewBox="0 0 8 14"><circle cx="2" cy="2" r="1.25"/><circle cx="6" cy="2" r="1.25"/><circle cx="2" cy="7" r="1.25"/><circle cx="6" cy="7" r="1.25"/><circle cx="2" cy="12" r="1.25"/><circle cx="6" cy="12" r="1.25"/></svg>`;

const DRAG_THRESHOLD = 4;
let drag = null; // { from, to, startY, pointerId, row, active, rows: [{ el, top, height }] }
let swallowClick = false;

// Moves a knob's jobs to another knob in the profile shown, the knobs in between shifting by
// one, as cards in a list do. Knobs keep their letters, inputs and calibration.
function moveKnobJobs(from, to) {
  const jobs = deviceJobs(activeProfile());
  while (jobs.length < knobCount()) jobs.push([]);
  const [moved] = jobs.splice(from, 1);
  jobs.splice(to, 0, moved);
}

function onGripPointerDown(e) {
  const grip = e.target.closest(".knob-grip");
  if (!grip || e.button !== 0) return;
  e.preventDefault();
  const row = grip.closest(".knob-row");
  const knob = Number(row.dataset.knob);
  drag = { from: knob, to: knob, startY: e.clientY, pointerId: e.pointerId, row, active: false, rows: [] };
  row.setPointerCapture(e.pointerId);
}

function onDragMove(e) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  const dy = e.clientY - drag.startY;
  if (!drag.active) {
    if (Math.abs(dy) < DRAG_THRESHOLD) return;
    drag.active = true;
    drag.rows = [...document.querySelectorAll(".knob-row")].map((el) => {
      const r = el.getBoundingClientRect();
      return { el, top: r.top, height: r.height };
    });
    drag.row.classList.add("dragging");
    drag.row.closest(".card").classList.add("reordering");
  }
  updateDrag(dy);
}

function updateDrag(dy) {
  const { rows, from } = drag;
  const self = rows[from];
  const first = rows[0];
  const last = rows[rows.length - 1];
  const offset = Math.min(Math.max(dy, first.top - self.top), last.top + last.height - self.top - self.height);
  // A row gives way once the dragged row's leading edge passes its middle, so a tall row can
  // still reach the first and last places.
  const top = self.top + offset;
  const bottom = top + self.height;
  let to = from;
  rows.forEach((r, i) => {
    const middle = r.top + r.height / 2;
    if (i < from && top < middle) to--;
    if (i > from && bottom > middle) to++;
  });
  drag.to = to;

  // The rows slide, but the letters stay in order top to bottom: the jobs move, the knobs don't.
  rows.forEach((r, i) => {
    let shift = 0;
    let slot = i;
    if (i === from) {
      shift = offset;
      slot = to;
    } else if (from < to && i > from && i <= to) {
      shift = -self.height;
      slot = i - 1;
    } else if (from > to && i >= to && i < from) {
      shift = self.height;
      slot = i + 1;
    }
    r.el.style.transform = shift ? `translateY(${shift}px)` : "";
    r.el.querySelector(".row-title").textContent = t("knob", { letter: letterFor(slot) });
  });
}

function onDragEnd(e) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  if (drag.active && e.type === "pointerup") updateDrag(e.clientY - drag.startY);
  const { active, from, to } = drag;
  drag = null;
  if (!active) return; // a plain click on the grip opens the job menu like the rest of the row
  // The click that ends a drag must not open the job menu. If none comes, the next one counts.
  swallowClick = true;
  setTimeout(() => (swallowClick = false), 0);
  if (e.type === "pointerup" && to !== from) moveKnobJobs(from, to);
  render();
}

function onKnobKeydown(e) {
  if (!e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
  const row = e.target.closest && e.target.closest(".knob-row");
  if (!row) return;
  const from = Number(row.dataset.knob);
  const to = from + (e.key === "ArrowUp" ? -1 : 1);
  if (to < 0 || to >= knobCount()) return;
  e.preventDefault();
  moveKnobJobs(from, to);
  render();
  const moved = document.getElementById(`knob-row-${to}`);
  if (moved) moved.focus();
}

// Adding or removing a knob changes the calibration of whatever is connected; the other input
// keeps its own knobs.
function editableKnobColumns() {
  if (!usesMixer()) return draft.columns;
  if (!draft.mixerColumns) draft.mixerColumns = draft.columns.map((_, i) => i);
  return draft.mixerColumns;
}

function addKnob() {
  editableKnobColumns().push(-1);
  padJobRows(draft);
  render();
}

function removeKnobConfirmed() {
  editableKnobColumns().pop();
  for (const p of draft.profiles) {
    const jobs = deviceJobs(p);
    if (jobs.length > knobCount()) jobs.pop();
  }
}

function applyRecorded(field, shortcut) {
  if (field === "next") draft.nextProfile = shortcut;
  else if (field === "previous") draft.previousProfile = shortcut;
  else if (field && field.indexOf("profile:") === 0) {
    const i = parseInt(field.slice("profile:".length), 10);
    if (draft.profiles[i]) draft.profiles[i].shortcut = shortcut;
  } else if (field && field.indexOf("button:") === 0) {
    const cc = field.slice("button:".length);
    replaceKind(cc, shortcut ? `keys:${shortcut.mods}:${shortcut.vk}:${shortcut.key}` : "keys:");
  }
}

// --- Shortcut recording -----------------------------------------------------

function currentDraftShortcuts() {
  return {
    profiles: draft.profiles.map((p) => p.shortcut || null),
    next: draft.nextProfile || null,
    previous: draft.previousProfile || null,
  };
}

const MODIFIER_KEYS = new Set(["Control", "Alt", "AltGraph", "Shift", "Meta", "OS"]);

function onRecordKeydown(e) {
  if (!recording) return;
  e.preventDefault();
  e.stopPropagation();
  if (e.key === "Escape") {
    stopRecording(true);
    return;
  }
  // Holding Ctrl or Alt fires a keydown for the modifier itself; wait for the real key.
  if (MODIFIER_KEYS.has(e.key)) return;
  const isClear = (e.key === "Delete" || e.key === "Backspace") && !e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey;
  // A hotkey needs a Ctrl/Alt chord; a key a mixer button presses can be any key.
  if (!isClear && !e.ctrlKey && !e.altKey && recording.field.indexOf("button:") !== 0) return;
  send({
    type: "key",
    field: recording.field,
    vk: e.keyCode,
    key: e.key,
    ctrl: e.ctrlKey,
    alt: e.altKey,
    shift: e.shiftKey,
    meta: e.metaKey,
    draft: currentDraftShortcuts(),
  });
}

function startRecording(field) {
  recording = { field };
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
  if (!recording) return;
  window.removeEventListener("keydown", onRecordKeydown, true);
  window.removeEventListener("blur", onWindowBlurWhileRecording);
  recording = null;
  if (notifyGo) send({ type: "stopRecording" });
  render();
}

// --- Events -----------------------------------------------------------------

function doSave() {
  if (!hasChanges()) return;
  send({ type: "save", setup: draft });
}

function onClick(e) {
  if (recording) {
    const stillInside = e.target.closest(`[data-field="${recording.field}"]`);
    if (!stillInside) stopRecording(true);
  }

  const target = e.target.closest("[data-action]");
  if (!target) return;

  switch (target.dataset.action) {
    case "switch-tab":
      activeTab = target.dataset.tab;
      if (isDeviceTab()) send({ type: "listPorts" });
      render();
      break;
    case "add-profile":
      addProfile();
      break;
    case "remove-profile":
      if (draft.profiles.length > 1) {
        dialog = { kind: "removeProfile" };
        render();
      }
      break;
    case "add-knob":
      addKnob();
      break;
    case "remove-knob":
      if (knobCount() > 0) {
        dialog = { kind: "removeKnob" };
        render();
      }
      break;
    case "confirm-dialog":
      if (dialog && dialog.kind === "removeProfile") removeProfileConfirmed();
      else if (dialog && dialog.kind === "removeKnob") removeKnobConfirmed();
      else if (dialog && dialog.kind === "removeControl") removeBoardControl(boardSelected);
      dialog = null;
      render();
      break;
    case "cancel-dialog":
      dialog = null;
      render();
      break;
    case "open-job-menu":
      openJobMenu(parseInt(target.dataset.knob, 10), target);
      break;
    case "open-button-menu":
      // Typing in a button's address box or clicking its own controls is not a click on the row.
      if (!e.target.closest(".button-param")) openButtonMenu(parseInt(target.dataset.button, 10), target);
      break;
    case "record": {
      const field = target.dataset.field;
      if (recording && recording.field === field) stopRecording(true);
      else startRecording(field);
      break;
    }
    case "pick-button-app":
      send({ type: "pickApp", button: parseInt(target.dataset.cc, 10), mode: target.dataset.mode });
      break;
    case "select-control":
      selectControl(parseInt(target.dataset.control, 10));
      break;
    case "clear-control":
      if (isButtonControl(picked())) setButtonActions(picked(), []);
      else deviceJobs(activeProfile())[picked()] = [];
      render();
      break;
    case "pick-control-app":
      send({ type: "pickApp", knob: picked() });
      break;
    case "add-control":
      addBoardControl(parseInt(target.dataset.row, 10));
      break;
    case "set-kind":
      setKind(boardSelected, target.dataset.kind);
      render();
      break;
    case "move-control":
      moveControl(boardSelected, target.dataset.dir);
      break;
    case "find-input":
      draft.columns[boardSelected] = -1;
      render();
      break;
    case "remove-control":
      if (boardControlUsed(boardSelected)) dialog = { kind: "removeControl" };
      else removeBoardControl(boardSelected);
      render();
      break;
    case "add-button":
      addButton();
      break;
    case "remove-button":
      removeButton();
      break;
    case "refresh-ports":
      send({ type: "listPorts" });
      break;
    case "reconnect":
      send({ type: "reconnect" });
      break;
    case "remove-shortcut":
      applyRecorded(target.dataset.field, null);
      render();
      break;
    case "calibrate":
      calibrating = true;
      render();
      send({ type: "calibrate", device: currentDevice() });
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
}

function onInput(e) {
  if (e.target.id === "profile-name") {
    // No render() here: it would tear down and rebuild the input, dropping
    // focus and the caret mid-type. The profile picker label catches up next
    // time something else forces a redraw.
    activeProfile().name = e.target.value;
  }
  if (e.target.dataset.buttonUrl) replaceKind(e.target.dataset.buttonUrl, `url:${e.target.value.trim()}`);
  updateSaveButton();
}

function onChange(e) {
  const el = e.target;
  if (el.dataset.pick) {
    onPick(el);
    updateSaveButton();
    return;
  }
  switch (el.id) {
    case "profile-select":
      draft.profile = parseInt(el.value, 10);
      render();
      break;
    case "invert":
      draft[usesMixer() ? "invertMixer" : "invertKnobs"] = el.checked;
      break;
    case "lights-select":
      draft.mixerLights = el.value === "off" ? "" : el.value;
      break;
    case "language-select":
      draft.language = el.value;
      send({ type: "setLanguage", code: draft.language });
      break;
    case "hide-icon":
      draft.hideTrayIcon = el.checked;
      render();
      break;
    case "tray-icon-style":
      draft.trayIcon = el.value;
      render();
      break;
    case "show-profile-list":
      draft.showProfileList = el.checked;
      break;
    case "speed-select":
      draft.speed = el.value;
      break;
    case "port-select":
      if (usesMixer()) draft.mixerPort = el.value;
      else draft.port = el.value;
      padJobRows(draft);
      render();
      break;
    case "baud-select":
      draft.baudRate = parseInt(el.value, 10);
      break;
  }
  updateSaveButton();
}

function onMessage(msg) {
  switch (msg.type) {
    case "init":
      // The window grows to fit the page (bridge.js reportHeight), so the mixer's lists get a cap
      // of about 8 rows, less on a short screen, and scroll inside it; about 520px is the rest.
      document.documentElement.style.setProperty("--list-max", `${Math.max(200, Math.min(360, screen.availHeight - 520))}px`);
      init = msg;
      draft = clone(msg.setup);
      labels = Object.assign({}, msg.labels || {});
      activeTab = tabFor(msg.tab);
      connection = msg.connection || connection;
      mixerConnection = msg.mixerConnection || mixerConnection;
      calibrating = !!msg.calibrating;
      // A device that sent nothing yet comes as null.
      live = { board: (msg.values && msg.values.board) || [], mixer: (msg.values && msg.values.mixer) || [] };
      if (isDeviceTab()) send({ type: "listPorts" });
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
      saved = clone(draft);
      render();
      break;
    case "strings":
      render();
      break;
    // Go-initiated: a button stepped the button lights to another pattern, already saved.
    case "mixerLights":
      draft.mixerLights = msg.pattern;
      if (saved) saved.mixerLights = msg.pattern;
      render();
      break;
    case "saved":
      draft = clone(msg.setup);
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
      saved = clone(draft);
      render();
      break;
    case "imported":
      draft = clone(msg.setup);
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
      importNote = msg.skipped && msg.skipped.length ? t("import_skipped", { items: msg.skipped.join(", ") }) : "";
      render();
      break;
    case "ports":
      ports = msg.ports || [];
      midiInputs = msg.midi || [];
      if (isDeviceTab()) render();
      break;
    // Go-initiated: a mixer button was pressed, so its row lights up to show which one it is.
    // Go-initiated: Calibration opened or closed, from here or from the tray.
    case "calibrating":
      calibrating = !!msg.on;
      render();
      break;
    case "buttonAppPicked": {
      const value = msg.mode === "open" ? msg.path : msg.exe;
      appNames[value] = msg.name;
      replaceKind(String(msg.button), `${msg.mode}:${value}`);
      render();
      updateSaveButton();
      break;
    }
    case "mixerButton":
      if (usesSMC()) onControlTouched(msg.id);
      else if (usesMixer()) onMixerButtonPressed(msg.id);
      break;
    // Go-initiated: a knob's control moved, so its row lights up the same way.
    case "knobMoved":
      if (msg.device !== currentDevice()) break;
      if (usesSMC() || usesBoard()) onControlTouched(msg.knob);
      else lightRow(document.getElementById(`knob-row-${msg.knob}`));
      break;
    // Go-initiated: a button on the board was pressed.
    case "boardButton":
      if (usesBoard()) onControlTouched(msg.knob);
      break;
    // Go-initiated: a device's last frame, about 20 times a second.
    case "values":
      showValues(msg.device, msg.values || []);
      break;
    // Go-initiated: the board or the mixer connected, dropped or got blocked by another app.
    case "connection": {
      const conn = { connected: !!msg.connected, busy: !!msg.busy, port: msg.port || "" };
      if (msg.device === "mixer") mixerConnection = conn;
      else connection = conn;
      if (isDeviceTab()) {
        send({ type: "listPorts" });
        render();
      }
      break;
    }
    case "importFailed":
      importNote = t("import_failed");
      render();
      break;
    case "jobMenuToggle": {
      if (msg.knob >= BUTTON_MENU_BASE) {
        onButtonMenuToggle(msg.knob - BUTTON_MENU_BASE, msg.job.action, msg.checked);
        break;
      }
      const knobs = deviceJobs(activeProfile());
      const jobs = knobs[msg.knob] || (knobs[msg.knob] = []);
      const k = jobKey(msg.job);
      const idx = jobs.findIndex((j) => jobKey(j) === k);
      if (msg.checked && idx < 0) jobs.push(msg.job);
      if (!msg.checked && idx >= 0) jobs.splice(idx, 1);
      render();
      break;
    }
    case "jobMenuClear":
      if (msg.knob >= BUTTON_MENU_BASE) setButtonActions(msg.knob - BUTTON_MENU_BASE, []);
      else deviceJobs(activeProfile())[msg.knob] = [];
      render();
      break;
    case "appPicked": {
      const knobs = deviceJobs(activeProfile());
      const jobs = knobs[msg.knob] || (knobs[msg.knob] = []);
      if (!hasJob(jobs, msg.entry.job)) jobs.push(msg.entry.job);
      if (!init.catalog.some((c) => jobKey(c.job) === jobKey(msg.entry.job))) init.catalog.push(msg.entry);
      render();
      break;
    }
    case "recorded":
      if (msg.shortcut) labels[shortcutKeyJSON(msg.shortcut)] = msg.label;
      applyRecorded(msg.field, msg.shortcut);
      stopRecording(true);
      break;
    case "rejected":
      break;
    // Go-initiated, not a reply to any message this page sent: a hotkey or a
    // tray profile click moved the active profile while Settings was open.
    case "profile":
      draft.profile = clampIndex(msg.profile, draft.profiles.length);
      if (saved) saved.profile = draft.profile; // Go has already saved the switch
      render();
      break;
    // Go-initiated: Calibration finished with a different column mapping
    // while this window was already open, so the draft's columns are stale.
    case "columns": {
      const changes = { [msg.mixer ? "mixerColumns" : "columns"]: msg.columns };
      if (msg.mixer) changes.mixerButtonOrder = msg.buttonOrder;
      // Calibration saved these already.
      for (const setup of saved ? [draft, saved] : [draft]) {
        Object.assign(setup, clone(changes));
        if (msg.mixer) (msg.profileButtons || []).forEach((b, i) => setup.profiles[i] && (setup.profiles[i].buttons = clone(b)));
        padJobRows(setup);
      }
      render();
      break;
    }
    // Go-initiated: the window was already open and got asked to switch tab
    // (e.g. the tray's About item) instead of opening a new one.
    case "tab":
      activeTab = tabFor(msg.tab || activeTab);
      render();
      break;
  }
}

document.getElementById("root").addEventListener(
  "click",
  (e) => {
    if (!swallowClick) return;
    swallowClick = false;
    e.stopPropagation();
    e.preventDefault();
  },
  true
);
document.getElementById("root").addEventListener("click", onClick);
document.getElementById("root").addEventListener("input", onInput);
document.getElementById("root").addEventListener("change", onChange);
document.getElementById("root").addEventListener("pointerdown", onGripPointerDown);
document.getElementById("root").addEventListener("pointermove", onDragMove);
document.getElementById("root").addEventListener("pointerup", onDragEnd);
document.getElementById("root").addEventListener("pointercancel", onDragEnd);
document.getElementById("root").addEventListener("keydown", onKnobKeydown);
document.getElementById("root").addEventListener("keydown", onControlKeydown);
document.getElementById("btn-close").addEventListener("click", () => send({ type: "close" }));
document.getElementById("btn-save").addEventListener("click", doSave);

window.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || recording || dialog) return;
  if ((e.target.tagName || "").toLowerCase() === "textarea") return;
  e.preventDefault();
  doSave();
});

connect("root", onMessage);
