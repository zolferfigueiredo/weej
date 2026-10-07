// What every part of Settings shares: the settings being edited, the boards' live state, and
// small helpers. The settings are the file's own shape (core.EncodeSettings): devices, each with
// its controls and profiles, and the app's own keys.
import { t } from "./bridge.js";
import { smcControlName } from "./device.js";

export const S = {
  init: null,
  draft: null,
  // saved is the settings as last saved, so Apply is on only when the draft differs.
  saved: null,
  labels: {},
  tab: "general",
  // board is the ID of the board the Boards tab shows; selected is each board's picked control.
  board: null,
  selected: {},
  dialog: null,
  // recording is { field, device, dialog }: dialog when the gear's dialog owns the shortcut.
  recording: null,
  status: {},
  live: {},
  ports: null,
  midiInputs: [],
  wizard: null,
  importNote: "",
  // setup is { total, done } while boards are set up one after another, from the first start.
  setup: null,
  appNames: {},
  render: () => {},
};

export function clone(v) {
  return JSON.parse(JSON.stringify(v));
}

export function esc(s) {
  return String(s == null ? "" : s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}
export const escAttr = esc;

// A to Z, then A2 to Z2, A3 and so on, as core.Letter names them.
export function letterFor(i) {
  return String.fromCharCode(65 + (i % 26)) + (i >= 26 ? String(Math.floor(i / 26) + 1) : "");
}

export function clip(name, limit) {
  if (!name) return name;
  const chars = Array.from(name);
  if (chars.length <= limit) return name;
  return chars.slice(0, limit - 1).join("").trimEnd() + "…";
}

export function clampIndex(i, len) {
  if (len <= 0 || i < 0 || i >= len) return 0;
  return i;
}

export function devices(from = S.draft) {
  return (from && from.devices) || [];
}

export function deviceById(id, from = S.draft) {
  return devices(from).find((d) => d.id === id) || null;
}

// The boards the Boards tab lists: the connected ones.
export function shownBoards() {
  return devices().filter((d) => statusOf(d.id).connected);
}

// The board the Boards tab shows: the one picked while it is connected, else the first that is.
export function currentBoard() {
  const shown = shownBoards();
  return shown.find((d) => d.id === S.board) || shown[0] || null;
}

export function isSMC(d) {
  return d && d.type === "smc";
}

export function activeProfile(d) {
  return d.profiles[clampIndex(d.profile, d.profiles.length)];
}

export function profileLabel(d, i) {
  const p = d.profiles[i];
  return p && p.name ? p.name : t("profile_n", { n: String(i + 1) });
}

export function statusOf(id) {
  return S.status[id] || { connected: false, busy: false, port: "" };
}

export function kindOf(d, k) {
  return (d.controls[k] || {}).kind || "knob";
}

// What a board's control is called: an SMC-Mixer's by its place, any other's by kind and letter.
export function controlName(d, k) {
  if (isSMC(d)) return smcControlName(k, t);
  const letter = letterFor(k);
  if (kindOf(d, k) === "fader") return t("board.fader", { letter });
  if (kindOf(d, k) === "button") return t("board.button", { letter });
  return t("knob", { letter });
}

// Whether a picked control is a button: an SMC-Mixer's ids from 128, any other's by kind.
export function isButton(d, k) {
  return isSMC(d) ? k >= 16 : kindOf(d, k) === "button";
}

// How a board's profiles key a button, as core.DeviceProfile does: a DIY board's by control, a
// MIDI board's by the id it sends.
export function buttonKey(d, k) {
  if (isSMC(d) || d.type === "diy") return k;
  return (d.controls[k] || {}).input;
}

// The control a button key belongs to, the other way round.
export function controlOfKey(d, key) {
  if (isSMC(d) || d.type === "diy") return key;
  return d.controls.findIndex((c) => c.kind === "button" && c.input === key);
}

export function jobsOf(d, k) {
  const p = activeProfile(d);
  if (!p.jobs) p.jobs = [];
  while (p.jobs.length < d.controls.length) p.jobs.push([]);
  return p.jobs[k] || (p.jobs[k] = []);
}

export function actionsOf(d, key) {
  const p = activeProfile(d);
  return ((p.buttons || {})[key]) || [];
}

export function setActions(d, key, actions) {
  const p = activeProfile(d);
  p.buttons = p.buttons || {};
  if (actions.length) p.buttons[key] = actions;
  else delete p.buttons[key];
}

export function valueAt(d, k) {
  const v = (S.live[d.id] || [])[k];
  return v === undefined || v === null ? -1 : v;
}

export function shortcutKeyJSON(s) {
  if (!s) return null;
  // Field order must match Go's json.Marshal of core.Shortcut{VK,Mods,Key}, since that encoded
  // string is the literal key into labels.
  return JSON.stringify({ vk: s.vk, mods: s.mods, key: s.key });
}

export function shortcutLabel(s) {
  if (!s) return null;
  return S.labels[shortcutKeyJSON(s)] || s.key || "";
}

// The other boards a shortcut also switches, by name.
export function alsoUsedBy(s, deviceId) {
  if (!s) return [];
  const same = (o) => o && o.vk === s.vk && o.mods === s.mods;
  return devices()
    .filter((d) => d.id !== deviceId && d.enabled)
    .filter((d) => same(d.nextProfile) || same(d.previousProfile) || d.profiles.some((p) => same(p.shortcut)))
    .map((d) => d.name);
}
