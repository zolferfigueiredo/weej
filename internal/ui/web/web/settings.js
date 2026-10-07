import { connect, send, t } from "./bridge.js";

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
let activeTab = "general";
let recording = null; // { field: "profile:<i>" | "next" | "previous" }
let dialog = null; // { kind: "removeProfile" | "removeKnob" }
let saved = null; // the setup as last saved, so Save is enabled only when the draft differs
let importNote = ""; // last import_skipped / import_failed text, shown under the import button
let ports = null; // [{ name, product, usb }] once Go has listed them
let midiInputs = []; // MIDI input names; a "midi:<name>" port reads that mixer
let connection = { connected: false, busy: false, port: "" };
let calibrating = false; // set on the click, so the button greys out before the window opens

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
    ["general", t("tab.general")],
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
        <div class="card">${knobsHtml}</div>
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

  // Two columns whose rows line up: connection and profile, invert and calibrate, then the knobs
  // and the buttons. A redraw keeps where each list was scrolled to.
  const listCards = () => document.querySelectorAll(".list-group > .card");
  const scrolled = Array.from(listCards(), (card) => card.scrollTop);
  document.getElementById("panel").innerHTML = `
    <div class="tabpanel general-grid" role="tabpanel">
      ${connectionGroup()}${profileGroup}
      ${invertGroup}${calibrateGroup}
      ${knobsGroup}${renderMixerButtons()}
      ${importFoot}
    </div>`;
  listCards().forEach((card, i) => {
    card.scrollTop = scrolled[i] || 0;
  });
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

const MIDI_PREFIX = "midi:";

function isMidiPort(port) {
  return (port || "").startsWith(MIDI_PREFIX);
}

function midiLabel(port) {
  return `${port.slice(MIDI_PREFIX.length)} (MIDI)`;
}

function usesMixer() {
  return isMidiPort(init.forcedPort || draft.port);
}

// The calibration of whatever is connected, as Go's activeColumns picks it. A mixer never
// calibrated reads knob i from its column i, as core.Setup.ForMixer does.
function knobColumns() {
  if (!usesMixer()) return draft.columns;
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

function actionLabel(action) {
  const kind = actionKind(action);
  const fkey = FKEYS.find(([value]) => value === kind);
  if (fkey) return fkey[1];
  for (const [, items] of BUTTON_GROUPS) for (const [value, key] of items) if (value === kind) return t(key);
  if (kind.startsWith("mute:")) return t("action.mute", { letter: letterFor(parseInt(kind.slice(5), 10)) });
  if (kind.startsWith("profile:")) return t("action.go_profile", { name: profileLabel(parseInt(kind.slice(8), 10)) });
  return kind;
}

function buttonActions(cc) {
  return (activeProfile().buttons || {})[cc] || [];
}

function setButtonActions(cc, actions) {
  const profile = activeProfile();
  profile.buttons = profile.buttons || {};
  if (actions.length) profile.buttons[cc] = actions;
  else delete profile.buttons[cc];
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

function buttonParams(cc) {
  return buttonActions(cc)
    .map((action) => {
      const kind = actionKind(action);
      if (!PARAM_KINDS.includes(kind)) return "";
      const value = action.slice(kind.length);
      let control;
      if (kind === "url:") {
        control = `<input class="input" type="text" data-button-url="${cc}" value="${escAttr(value)}" placeholder="https://" spellcheck="false" />`;
      } else if (kind === "keys:") {
        control = shortcutControl(`button:${cc}`, keysShortcut(value));
      } else {
        const label = value ? appNames[value] || baseName(value) : t("choose");
        control = `<button class="btn" type="button" data-action="pick-button-app" data-cc="${cc}" data-mode="${kind.slice(0, -1)}">${esc(label)}</button>`;
      }
      return `<div class="button-param"><span class="param-label">${esc(actionLabel(action))}</span>${control}</div>`;
    })
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
  for (let i = 0; i < knobCount(); i++) mutes.push(item(`mute:${i}`, t("action.mute", { letter: letterFor(i) })));
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
        <div class="card">${rows}</div>
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
  if (activeTab !== "general") return;
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
  if (activeTab !== "general" || !row) return;
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

function connectionGroup() {
  const forced = init.forcedPort || "";
  const autoLabel =
    !draft.port && connection.connected && connection.port ? t("port_auto_found", { port: connection.port }) : t("port_auto");
  const list = ports || [];
  let portOptions;
  if (forced) {
    portOptions = `<option selected>${esc(forced)}</option>`;
  } else {
    portOptions =
      `<option value=""${draft.port ? "" : " selected"}>${esc(autoLabel)}</option>` +
      list
        .map((p) => `<option value="${escAttr(p.name)}"${draft.port === p.name ? " selected" : ""}>${esc(portLabel(p))}</option>`)
        .join("") +
      midiInputs
        .map((name) => MIDI_PREFIX + name)
        .map((port) => `<option value="${escAttr(port)}"${draft.port === port ? " selected" : ""}>${esc(midiLabel(port))}</option>`)
        .join("");
    // A saved port that is unplugged right now still has to show as the choice.
    if (draft.port && !list.some((p) => p.name === draft.port) && !midiInputs.some((name) => MIDI_PREFIX + name === draft.port)) {
      const label = isMidiPort(draft.port) ? midiLabel(draft.port) : draft.port;
      portOptions += `<option value="${escAttr(draft.port)}" selected>${esc(label)}</option>`;
    }
  }
  const midi = isMidiPort(forced || draft.port);

  const rates = (init.baudRates || [9600]).slice();
  if (draft.baudRate && !rates.includes(draft.baudRate)) rates.push(draft.baudRate);
  const baudOptions = rates
    .map((r) => `<option value="${r}"${r === draft.baudRate ? " selected" : ""}>${r}</option>`)
    .join("");

  let status = t("not_connected");
  let statusClass = "";
  if (connection.connected) {
    status = t("connected", { port: connection.port });
    statusClass = " status-ok";
  } else if (connection.busy && connection.port) {
    status = t("port_busy", { port: connection.port });
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
              <span class="row-desc">${esc(forced ? t("port_forced", { port: forced }) : t("port_note"))}</span>
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
          }
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
      if (activeTab === "general") send({ type: "listPorts" });
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
      send({ type: "calibrate" });
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
  switch (el.id) {
    case "profile-select":
      draft.profile = parseInt(el.value, 10);
      render();
      break;
    case "invert":
      draft[usesMixer() ? "invertMixer" : "invertKnobs"] = el.checked;
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
      draft.port = el.value;
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
      activeTab = msg.tab || "general";
      connection = msg.connection || connection;
      calibrating = !!msg.calibrating;
      if (activeTab === "general") send({ type: "listPorts" });
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
      saved = clone(draft);
      render();
      break;
    case "strings":
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
      if (activeTab === "general") render();
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
      onMixerButtonPressed(msg.cc);
      break;
    // Go-initiated: a knob's control moved, so its row lights up the same way.
    case "knobMoved":
      lightRow(document.getElementById(`knob-row-${msg.knob}`));
      break;
    // Go-initiated: the board connected, dropped or got blocked by another app.
    case "connection":
      connection = { connected: !!msg.connected, busy: !!msg.busy, port: msg.port || "" };
      if (activeTab === "general") {
        send({ type: "listPorts" });
        render();
      }
      break;
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
      activeTab = msg.tab || activeTab;
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
document.getElementById("btn-close").addEventListener("click", () => send({ type: "close" }));
document.getElementById("btn-save").addEventListener("click", doSave);

window.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || recording || dialog) return;
  if ((e.target.tagName || "").toLowerCase() === "textarea") return;
  e.preventDefault();
  doSave();
});

connect("root", onMessage);
