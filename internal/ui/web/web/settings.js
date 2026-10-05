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
let popover = null; // { knob, place: { top, left, height }, scrollTop }
let dialog = null; // { kind: "removeProfile" | "removeKnob" }
let importNote = ""; // last import_skipped / import_failed text, shown under the import button
let ports = null; // [{ name, product, usb }] once Go has listed them
let connection = { connected: false, busy: false, port: "" };

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

function letterFor(i) {
  return String.fromCharCode(65 + i);
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
  if (!jobs.length) return `<span class="job-line muted">${esc(t("job.nothing"))}</span>`;
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

  // Overlays are appended fresh below, not patched in place: drop any previous
  // copy first, or every render while one is open would stack another on top.
  const shownPopover = document.querySelector(".popover");
  if (popover && shownPopover) popover.scrollTop = shownPopover.scrollTop;
  document.querySelectorAll(".popover-scrim, .popover, .dialog-scrim").forEach((el) => el.remove());

  document.getElementById("page-title").textContent = t("settings");
  renderTabs();
  renderFooter();
  if (activeTab === "app") renderApp();
  else if (activeTab === "connection") renderConnection();
  else if (activeTab === "about") renderAbout();
  else renderGeneral();

  if (popover) renderPopover();
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
    ["connection", t("tab.connection")],
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
  document.getElementById("btn-save").textContent = t("save");
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

  const knobsHtml = draft.columns.length
    ? draft.columns
        .map((col, i) => {
          const jobs = profile.jobs[i] || [];
          const needsCal = col === null || col === undefined || col === -1;
          return `
            <div class="row clickable knob-row" role="button" tabindex="0" data-action="open-popover" data-knob="${i}">
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

  document.getElementById("panel").innerHTML = `
    <div class="tabpanel" role="tabpanel">
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
      </div>

      <div class="group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("knobs"))}</h2>
          <span class="spacer"></span>
          <span class="segmented">
            <button class="btn btn-icon" type="button" data-action="add-knob" title="${escAttr(t("add_knob"))}" aria-label="${escAttr(t("add_knob"))}"${draft.columns.length >= 26 ? " disabled" : ""}>+</button>
            <button class="btn btn-icon" type="button" data-action="remove-knob" title="${escAttr(t("remove_knob"))}" aria-label="${escAttr(t("remove_knob"))}"${draft.columns.length === 0 ? " disabled" : ""}>&minus;</button>
          </span>
        </div>
        <div class="card">${knobsHtml}</div>
        <div class="group-foot">
          <span class="group-note">${esc(t("jobs_note"))}</span>
          <button class="btn" type="button" data-action="calibrate">${esc(t("calibrate"))}</button>
        </div>
      </div>

      <div class="group">
        <div class="card">
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("invert"))}</span>
              <span class="row-desc">${esc(t("invert_note"))}</span>
            </div>
            <div class="row-control"><input class="toggle" id="invert" type="checkbox" role="switch"${draft.invertKnobs ? " checked" : ""} /></div>
          </div>
        </div>
        <div class="group-foot end">
          ${importNote ? `<span class="group-note">${esc(importNote)}</span>` : ""}
          <button class="btn" type="button" data-action="import-deej">${esc(t("import_deej"))}</button>
        </div>
      </div>
    </div>`;
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

function renderConnection() {
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
        .join("");
    // A saved port that is unplugged right now still has to show as the choice.
    if (draft.port && !list.some((p) => p.name === draft.port)) {
      portOptions += `<option value="${escAttr(draft.port)}" selected>${esc(draft.port)}</option>`;
    }
  }

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

  document.getElementById("panel").innerHTML = `
    <div class="tabpanel" role="tabpanel">
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
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("baud_rate"))}</span>
              <span class="row-desc">${esc(t("baud_note"))}</span>
            </div>
            <div class="row-control"><select class="select" id="baud-select">${baudOptions}</select></div>
          </div>
        </div>
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
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("on_macos"))} <a href="#" data-action="open-url" data-url="${escAttr(init.theej)}">TheeJ</a></p>
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("made_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.madeBy)}">${esc(hostnameOf(init.madeBy))}</a></p>
      <p class="row-desc" data-style="margin-top:6px;">${esc(t("inspired_by"))} <a href="#" data-action="open-url" data-url="${escAttr(init.deej)}">deej</a></p>
    </div>`;
}

function renderPopover() {
  const knobJobs = activeProfile().jobs[popover.knob] || [];
  const bySection = sectionsFromCatalog();
  let html = `<div class="popover-scrim" data-action="close-popover"></div>`;
  html += `<div class="popover">`;
  html += `<div class="popover-item" data-action="clear-jobs" data-knob="${popover.knob}"><span>${esc(t("clear"))}</span></div>`;
  html += `<div class="popover-sep"></div>`;
  for (const [section, entries] of bySection) {
    if (section === "apps") continue;
    html += `<div class="popover-header">${esc(t(SECTION_LABEL_KEY[section] || ""))}</div>`;
    for (const entry of entries) html += popoverItem(entry, knobJobs);
  }
  const apps = bySection.get("apps") || [];
  html += `<div class="popover-header">${esc(t("section.apps"))}</div>`;
  for (const entry of apps) html += popoverItem(entry, knobJobs);
  html += `<div class="popover-item" data-action="pick-app" data-knob="${popover.knob}"><span>${esc(t("other"))}</span></div>`;
  html += `</div>`;
  document.getElementById("root").insertAdjacentHTML("beforeend", html);
  placePopover();
  document.querySelector(".popover").scrollTop = popover.scrollTop;
}

// The window is only as tall as its content, so a menu hanging past the bottom
// would be cut off: it opens above its knob when there's more room there. It's
// placed once, so ticking a job, which grows the knob's row, never moves it.
function placePopover() {
  const el = document.querySelector(".popover");
  if (!el) return;
  const margin = 8;
  if (!popover.place) {
    const gap = 4;
    const rect = document.querySelector(`.knob-row[data-knob="${popover.knob}"]`).getBoundingClientRect();
    const wanted = el.offsetHeight;
    const below = window.innerHeight - rect.bottom - gap - margin;
    const above = rect.top - gap - margin;
    const up = wanted > below && above > below;
    const height = Math.min(wanted, up ? above : below);
    popover.place = {
      top: up ? rect.top - gap - height : rect.bottom + gap,
      left: Math.max(margin, Math.min(rect.left, window.innerWidth - margin - el.offsetWidth)),
      height,
    };
  }
  const { top, left, height } = popover.place;
  el.style.top = `${top}px`;
  el.style.left = `${left}px`;
  el.style.maxHeight = `${Math.min(height, window.innerHeight - margin - top)}px`;
}

function popoverItem(entry, knobJobs) {
  const checked = hasJob(knobJobs, entry.job);
  const id = "chk-" + jobKey(entry.job).replace(/[^a-zA-Z0-9_-]/g, "_");
  const badge =
    entry.job.kind === "nightLight" && init.nightLightExperimental ? `<span class="badge">${esc(t("experimental"))}</span>` : "";
  return `<label class="popover-item" for="${id}">
    <input class="chk" type="checkbox" id="${id}" data-action="toggle-job" data-knob="${popover.knob}" data-job='${escAttr(JSON.stringify(entry.job))}'${checked ? " checked" : ""} />
    <img src="${entry.icon}" alt="" />
    <span data-style="flex:1 1 auto;">${esc(entry.title)}</span>
    ${badge}
  </label>`;
}

function renderDialog() {
  let title, body;
  if (dialog.kind === "removeProfile") {
    const name = activeProfile().name;
    title = name ? t("remove_named", { name }) : t("remove_this_profile");
    body = t("remove_profile_info");
  } else {
    title = t("remove_knob_q", { letter: letterFor(draft.columns.length - 1) });
    body = t("remove_knob_info");
  }
  const html = `
    <div class="dialog-scrim" data-action="cancel-dialog">
      <div class="dialog">
        <div class="dialog-title">${esc(title)}</div>
        <div class="dialog-body">${esc(body)}</div>
        <div class="dialog-actions">
          <button class="btn" type="button" data-action="cancel-dialog">${esc(t("cancel"))}</button>
          <button class="btn btn-primary btn-danger" type="button" data-action="confirm-dialog">${esc(t("remove"))}</button>
        </div>
      </div>
    </div>`;
  document.getElementById("root").insertAdjacentHTML("beforeend", html);
}

// --- Mutations --------------------------------------------------------------

function addProfile() {
  draft.profiles.push({ name: "", jobs: draft.columns.map(() => []), shortcut: null });
  draft.profile = draft.profiles.length - 1;
  render();
}

function removeProfileConfirmed() {
  draft.profiles.splice(draft.profile, 1);
  draft.profile = clampIndex(draft.profile, draft.profiles.length);
}

function addKnob() {
  if (draft.columns.length >= 26) return;
  draft.columns.push(-1);
  for (const p of draft.profiles) p.jobs.push([]);
  render();
}

function removeKnobConfirmed() {
  draft.columns.pop();
  for (const p of draft.profiles) p.jobs.pop();
}

function openPopover(knob) {
  popover = { knob, place: null, scrollTop: 0 };
  render();
}

function closePopover() {
  popover = null;
  render();
}

function applyRecorded(field, shortcut) {
  if (field === "next") draft.nextProfile = shortcut;
  else if (field === "previous") draft.previousProfile = shortcut;
  else if (field && field.indexOf("profile:") === 0) {
    const i = parseInt(field.slice("profile:".length), 10);
    if (draft.profiles[i]) draft.profiles[i].shortcut = shortcut;
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
  if (!isClear && !e.ctrlKey && !e.altKey) return; // wait for a real Ctrl/Alt chord
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
  if (!draft) return;
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
      if (activeTab === "connection") send({ type: "listPorts" });
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
      if (draft.columns.length > 0) {
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
    case "open-popover":
      openPopover(parseInt(target.dataset.knob, 10));
      break;
    case "close-popover":
      closePopover();
      break;
    case "clear-jobs":
      activeProfile().jobs[parseInt(target.dataset.knob, 10)] = [];
      render();
      break;
    case "pick-app":
      send({ type: "pickApp", knob: parseInt(target.dataset.knob, 10) });
      break;
    case "record": {
      const field = target.dataset.field;
      if (recording && recording.field === field) stopRecording(true);
      else startRecording(field);
      break;
    }
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
}

function onChange(e) {
  const el = e.target;
  if (el.dataset && el.dataset.action === "toggle-job") {
    const knob = parseInt(el.dataset.knob, 10);
    const job = JSON.parse(el.dataset.job);
    const jobs = activeProfile().jobs[knob] || (activeProfile().jobs[knob] = []);
    const k = jobKey(job);
    const idx = jobs.findIndex((j) => jobKey(j) === k);
    if (el.checked && idx < 0) jobs.push(job);
    if (!el.checked && idx >= 0) jobs.splice(idx, 1);
    render();
    return;
  }
  switch (el.id) {
    case "profile-select":
      draft.profile = parseInt(el.value, 10);
      render();
      break;
    case "invert":
      draft.invertKnobs = el.checked;
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
}

function onMessage(msg) {
  switch (msg.type) {
    case "init":
      init = msg;
      draft = clone(msg.setup);
      labels = Object.assign({}, msg.labels || {});
      activeTab = msg.tab || "general";
      connection = msg.connection || connection;
      if (activeTab === "connection") send({ type: "listPorts" });
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
      render();
      break;
    case "strings":
      render();
      break;
    case "saved":
      draft = clone(msg.setup);
      draft.profile = clampIndex(draft.profile, draft.profiles.length);
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
      if (activeTab === "connection") render();
      break;
    // Go-initiated: the board connected, dropped or got blocked by another app.
    case "connection":
      connection = { connected: !!msg.connected, busy: !!msg.busy, port: msg.port || "" };
      if (activeTab === "connection") {
        send({ type: "listPorts" });
        render();
      }
      break;
    case "importFailed":
      importNote = t("import_failed");
      render();
      break;
    case "appPicked": {
      const jobs = activeProfile().jobs[msg.knob] || (activeProfile().jobs[msg.knob] = []);
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
      render();
      break;
    // Go-initiated: Calibration finished with a different column mapping
    // while this window was already open, so the draft's columns are stale.
    case "columns":
      draft.columns = msg.columns;
      render();
      break;
    // Go-initiated: the window was already open and got asked to switch tab
    // (e.g. the tray's About item) instead of opening a new one.
    case "tab":
      activeTab = msg.tab || activeTab;
      render();
      break;
  }
}

document.getElementById("root").addEventListener("click", onClick);
document.getElementById("root").addEventListener("input", onInput);
document.getElementById("root").addEventListener("change", onChange);
document.getElementById("btn-close").addEventListener("click", () => send({ type: "close" }));
document.getElementById("btn-save").addEventListener("click", doSave);
// Ticking a job resizes the window under an open menu.
window.addEventListener("resize", () => {
  if (popover) placePopover();
});

window.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || recording || popover || dialog) return;
  if ((e.target.tagName || "").toLowerCase() === "textarea") return;
  e.preventDefault();
  doSave();
});

connect("root", onMessage);
