// What a control does: a pot's jobs and a button's actions, ticked in place in the inspector or
// in the job menu's popup window, and the labels the drawings and lists show for them.
import { send, t } from "./bridge.js";
import {
  S,
  actionsOf,
  buttonKey,
  controlName,
  esc,
  escAttr,
  isButton,
  isSMC,
  jobsOf,
  profileLabel,
  setActions,
  shortcutLabel,
} from "./state.js";

// A 1x1 transparent GIF, for a menu entry with no icon.
const TRANSPARENT_PIXEL = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";

const SECTION_LABEL_KEY = {
  volume: "section.volume",
  brightness: "section.brightness",
  contrast: "section.contrast",
  nightLight: "section.night_light",
  keyboard: "section.keyboard",
  zoom: "section.zoom",
  apps: "section.apps",
};

export function jobKey(job) {
  if (job.kind === "brightness" || job.kind === "contrast") return job.kind + ":" + job.screen;
  if (job.kind === "app") return job.kind + ":" + job.exe;
  return job.kind;
}

export function hasJob(jobs, job) {
  const k = jobKey(job);
  return jobs.some((j) => jobKey(j) === k);
}

function jobEntryFor(job) {
  const k = jobKey(job);
  return S.init.catalog.find((c) => jobKey(c.job) === k) || null;
}

export function jobLines(jobs) {
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

function sectionsFromCatalog() {
  const bySection = new Map();
  for (const entry of S.init.catalog) {
    if (!bySection.has(entry.section)) bySection.set(entry.section, []);
    bySection.get(entry.section).push(entry);
  }
  return bySection;
}

// A button's functions are core.ButtonAction strings. The ones ending in ":" take a setting after
// it, which the button asks for under its tick.
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
      ["lights.previous", "action.previous_lights"],
      ["lights.on", "action.lights_on"],
      ["lights.off", "action.lights_off"],
    ],
  ],
];
const PARAM_KINDS = ["open:", "close:", "url:", "keys:"];

// The popup menu tells a button from a pot by this offset on the id it sends back.
export const BUTTON_MENU_BASE = 10000;

// F13 to F24, keys no keyboard has, so other apps can bind them to a button without clashing.
const FKEYS = Array.from({ length: 12 }, (_, i) => [`keys:0:${0x7c + i}:F${13 + i}`, `F${13 + i}`]);

// The menu entry an action is ticked under: itself, or the kind of one that takes a setting.
export function actionKind(action) {
  if (FKEYS.some(([value]) => value === action)) return action;
  return PARAM_KINDS.find((kind) => action.startsWith(kind)) || action;
}

// The pots a mute can name: every knob and fader of the board.
function pots(d) {
  return d.controls.map((_, k) => k).filter((k) => !isButton(d, k));
}

export function actionLabel(d, action) {
  const kind = actionKind(action);
  const fkey = FKEYS.find(([value]) => value === kind);
  if (fkey) return fkey[1];
  for (const [, items] of BUTTON_GROUPS) for (const [value, key] of items) if (value === kind) return t(key);
  if (kind.startsWith("mute:")) return t("action.mute", { name: controlName(d, parseInt(kind.slice(5), 10)) });
  if (kind.startsWith("profile:")) return t("action.go_profile", { name: profileLabel(d, parseInt(kind.slice(8), 10)) });
  return kind;
}

// Puts action in place of whatever the button had of the same kind, so each kind is there once.
export function replaceKind(d, key, action) {
  const kind = actionKind(action);
  setActions(d, key, [...actionsOf(d, key).filter((a) => actionKind(a) !== kind), action]);
}

function baseName(path) {
  return path.split(/[\\/]/).pop();
}

// "keys:<mods>:<vk>:<key>", as core.KeysAction writes it.
function keysShortcut(value) {
  const [mods, vk, ...key] = value.split(":");
  return vk ? { vk: parseInt(vk, 10), mods: parseInt(mods, 10), key: key.join(":") } : null;
}

// A shortcut's button: what it is, or Record, with a × to clear it.
export function shortcutControl(field, shortcut, deviceId, dialog = false) {
  const r = S.recording;
  const isRecording = r && r.field === field && r.device === deviceId && r.dialog === dialog;
  const label = isRecording ? t("press_shortcut") : shortcut ? shortcutLabel(shortcut) : t("record_shortcut");
  const data = `data-field="${escAttr(field)}" data-device="${escAttr(deviceId)}"${dialog ? ' data-dialog="1"' : ""}`;
  const removeBtn =
    shortcut && !isRecording
      ? `<button class="btn btn-icon btn-subtle" type="button" data-action="remove-shortcut" ${data} title="${escAttr(t("remove_shortcut"))}" aria-label="${escAttr(t("remove_shortcut"))}">&times;</button>`
      : "";
  return `<button class="btn" type="button" data-action="record" ${data}>${esc(label)}</button>${removeBtn}`;
}

// What sets the part of an action after its ":": an address box, keys to record or an app.
function paramControl(d, key, action) {
  const kind = actionKind(action);
  const value = action.slice(kind.length);
  if (kind === "url:") {
    return `<input class="input" type="text" id="url-${key}" data-button-url="${key}" value="${escAttr(value)}" placeholder="https://" spellcheck="false" />`;
  }
  if (kind === "keys:") return shortcutControl(`button:${key}`, keysShortcut(value), d.id);
  const label = value ? S.appNames[value] || baseName(value) : t("choose");
  return `<button class="btn" type="button" data-action="pick-button-app" data-key="${key}" data-mode="${kind.slice(0, -1)}">${esc(label)}</button>`;
}

export function buttonParams(d, key) {
  return actionsOf(d, key)
    .filter((action) => PARAM_KINDS.includes(actionKind(action)))
    .map((action) => `<div class="button-param"><span class="param-label">${esc(actionLabel(d, action))}</span>${paramControl(d, key, action)}</div>`)
    .join("");
}

export function buttonLines(d, key) {
  const actions = actionsOf(d, key);
  if (!actions.length) return `<span class="job-line muted">${esc(t("job.empty"))}</span>`;
  return actions.map((a) => `<span class="job-line"><span>${esc(actionLabel(d, a))}</span></span>`).join("");
}

// The label under a drawn control: its first job or action and how many more it has. A screen's
// short name says which screen but not whether brightness or contrast, so those keep their title.
export function controlLabel(d, k) {
  if (isButton(d, k)) {
    const actions = actionsOf(d, buttonKey(d, k));
    const title = actions.map((a) => actionLabel(d, a)).join(", ");
    if (isSMC(d)) return { title };
    if (!actions.length) return { text: t("job.empty"), empty: true };
    return { text: actionLabel(d, actions[0]), more: actions.length - 1, title };
  }
  const jobs = jobsOf(d, k);
  if (!jobs.length) return { text: t("job.empty"), empty: true };
  const titleOf = (job) => (jobEntryFor(job) || {}).title || job.exe || job.kind;
  const entry = jobEntryFor(jobs[0]);
  const text = entry && jobs[0].kind !== "brightness" && jobs[0].kind !== "contrast" ? entry.short : titleOf(jobs[0]);
  return { text, more: jobs.length - 1, title: jobs.map(titleOf).join(", ") };
}

function pickRow(n, checked, data, title, icon, badge) {
  return `<label class="pick-row"><input class="chk" type="checkbox" id="pick-${n}" ${data}${checked ? " checked" : ""} />${icon ? `<img src="${escAttr(icon)}" alt="" />` : ""}<span class="pick-title">${esc(title)}</span>${badge ? `<span class="badge">${esc(t("experimental"))}</span>` : ""}</label>`;
}

export function jobPicks(d, k) {
  const jobs = jobsOf(d, k);
  const bySection = sectionsFromCatalog();
  const order = [...bySection.keys()].filter((s) => s !== "apps").concat("apps");
  let n = 0;
  let html = "";
  for (const section of order) {
    html += `<div class="pick-head">${esc(t(SECTION_LABEL_KEY[section] || ""))}</div>`;
    for (const entry of bySection.get(section) || []) {
      const badge = entry.job.kind === "nightLight" && !!S.init.nightLightExperimental;
      html += pickRow(n++, hasJob(jobs, entry.job), `data-pick="job" data-job="${escAttr(jobKey(entry.job))}"`, entry.title, entry.icon, badge);
    }
  }
  return html + `<div class="pick-more"><button class="btn" type="button" data-action="pick-control-app">${esc(t("other"))}</button></div>`;
}

// The same choices as a button's popup menu, ticked in place; an action that takes a setting
// shows what sets it under its tick.
export function actionPicks(d, key) {
  const actions = actionsOf(d, key);
  const kinds = new Set(actions.map(actionKind));
  let n = 0;
  const row = ([value, title]) => {
    const html = pickRow(n++, kinds.has(value), `data-pick="action" data-value="${escAttr(value)}"`, title);
    const action = PARAM_KINDS.includes(value) && actions.find((a) => actionKind(a) === value);
    return action ? html + `<div class="pick-param">${paramControl(d, key, action)}</div>` : html;
  };
  const group = (title, items) => `<div class="pick-head">${esc(title)}</div>` + items.map(row).join("");
  const groups = BUTTON_GROUPS.map(([gk, items]) => {
    const list = items.map(([value, k]) => [value, t(k)]);
    if (gk === "action.group.weej") {
      list.splice(2, 0, ...d.profiles.map((_, i) => [`profile:${i}`, t("action.go_profile", { name: profileLabel(d, i) })]));
    }
    return group(t(gk), list);
  });
  const mutes = pots(d).map((k) => [`mute:${k}`, t("action.mute", { name: controlName(d, k) })]);
  return groups.join("") + group(t("action.group.fkeys"), FKEYS) + group(t("action.group.knobs"), mutes);
}

// A tick in the inspector: a job on or off the picked pot, or an action on or off its button.
export function onPick(d, k, el) {
  if (el.dataset.pick === "job") {
    const entry = S.init.catalog.find((c) => jobKey(c.job) === el.dataset.job);
    if (!entry) return;
    toggleJob(d, k, entry.job, el.checked);
    return;
  }
  const value = el.dataset.value;
  const key = buttonKey(d, k);
  toggleAction(d, key, value, el.checked);
  if (el.checked && value === "url:") document.getElementById(`url-${key}`)?.focus();
  return el.checked && value === "keys:" ? `button:${key}` : null;
}

export function toggleJob(d, k, job, checked) {
  const jobs = jobsOf(d, k);
  const idx = jobs.findIndex((j) => jobKey(j) === jobKey(job));
  if (checked && idx < 0) jobs.push(job);
  if (!checked && idx >= 0) jobs.splice(idx, 1);
}

export function toggleAction(d, key, action, checked) {
  if (!checked) {
    setActions(d, key, actionsOf(d, key).filter((a) => actionKind(a) !== action));
  } else if (PARAM_KINDS.includes(action)) {
    replaceKind(d, key, action);
    if (action === "open:" || action === "close:") send({ type: "pickApp", button: key, mode: action.slice(0, -1) });
  } else if (!actionsOf(d, key).includes(action)) {
    setActions(d, key, [...actionsOf(d, key), action]);
  }
}

// The job menu is a popup window of its own (jobs.html), so it can hang past this window's edge.
// This page owns the list and the board's unsaved jobs, so it sends the whole menu, and Go hands
// every tick back as jobMenuToggle or jobMenuClear.
export function openJobMenu(d, k, row) {
  const r = row.getBoundingClientRect();
  const jobs = jobsOf(d, k);
  const item = (entry) => ({
    job: entry.job,
    title: entry.title,
    icon: entry.icon,
    badge: entry.job.kind === "nightLight" && !!S.init.nightLightExperimental,
    checked: hasJob(jobs, entry.job),
  });
  const bySection = sectionsFromCatalog();
  const sections = [];
  for (const [section, entries] of bySection) {
    if (section === "apps") continue;
    sections.push({ title: t(SECTION_LABEL_KEY[section] || ""), items: entries.map(item) });
  }
  sections.push({ title: t("section.apps"), items: (bySection.get("apps") || []).map(item), other: true });
  send({ type: "openJobMenu", knob: k, anchor: { left: r.left, top: r.top, right: r.right, bottom: r.bottom }, model: { sections } });
}

export function openButtonMenu(d, key, row) {
  const r = row.getBoundingClientRect();
  const kinds = new Set(actionsOf(d, key).map(actionKind));
  const item = (action, title) => ({ job: { action }, title, icon: TRANSPARENT_PIXEL, checked: kinds.has(action) });
  const sections = BUTTON_GROUPS.map(([gk, items]) => ({ title: t(gk), items: items.map(([v, k]) => item(v, t(k))) }));
  const weej = sections[sections.length - 1];
  weej.items.splice(2, 0, ...d.profiles.map((_, i) => item(`profile:${i}`, t("action.go_profile", { name: profileLabel(d, i) }))));
  sections.push({ title: t("action.group.fkeys"), items: FKEYS.map(([v, label]) => item(v, label)) });
  sections.push({ title: t("action.group.knobs"), items: pots(d).map((k) => item(`mute:${k}`, t("action.mute", { name: controlName(d, k) }))) });
  send({ type: "openJobMenu", knob: BUTTON_MENU_BASE + key, anchor: { left: r.left, top: r.top, right: r.right, bottom: r.bottom }, model: { sections } });
}

