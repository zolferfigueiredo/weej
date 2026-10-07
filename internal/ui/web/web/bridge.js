// bridge.js: the one thing every page imports. Owns the Go<->JS transport (real
// WebView2, or a mock when previewing in a plain browser), the i18n helpers that
// mirror internal/lang's lookup and plural rules, theme application, and height
// reporting. Pages never touch chrome.webview directly.

let lang = "en";
let strings = {};
let handler = null;
let rootEl = null;
let sendImpl = null;
let pendingHeight = null;
let initDone = false;
let early = [];

function reportError(message, source, line) {
  send({ type: "pageError", message: String(message), source: String(source || ""), line: Number(line) || 0 });
}
window.addEventListener("error", (e) => reportError(e.message, e.filename, e.lineno));
window.addEventListener("unhandledrejection", (e) =>
  reportError(e.reason && e.reason.message ? e.reason.message : e.reason, "promise", 0)
);
document.addEventListener("securitypolicyviolation", (e) =>
  reportError("CSP blocked " + e.violatedDirective, e.sourceFile || e.blockedURI, e.lineNumber)
);

// The CSP forbids style attributes in markup but not CSSOM, so pages write data-style and
// it is applied here as soon as the element appears.
function applyDataStyles(node) {
  if (node.nodeType !== 1) return;
  if (node.hasAttribute("data-style")) node.style.cssText = node.getAttribute("data-style");
  node.querySelectorAll("[data-style]").forEach((el) => {
    el.style.cssText = el.getAttribute("data-style");
  });
}
new MutationObserver((mutations) => {
  for (const m of mutations) {
    if (m.type === "attributes") applyDataStyles(m.target);
    m.addedNodes.forEach(applyDataStyles);
  }
}).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ["data-style"] });
applyDataStyles(document.documentElement);

// --- i18n -------------------------------------------------------------------

function applyVars(s, vars) {
  if (!vars) return s;
  let out = s;
  for (const name in vars) {
    out = out.split("{" + name + "}").join(String(vars[name]));
  }
  return out;
}

export function t(key, vars) {
  const s = strings[key];
  if (s === undefined) return key;
  return applyVars(s, vars);
}

// Mirrors internal/lang.pluralForm: which of one/few/many/other a count maps to
// in each language's own plural rule, not just English singular/plural.
function isFewRange(n) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  return mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14);
}

function pluralForm(l, n) {
  switch (l) {
    case "zh":
    case "ja":
    case "ko":
      return "other";
    case "fr":
      return n < 2 ? "one" : "other";
    case "pl":
      if (n === 1) return "one";
      return isFewRange(n) ? "few" : "many";
    case "ru":
    case "uk": {
      const mod10 = n % 10;
      const mod100 = n % 100;
      if (mod10 === 1 && mod100 !== 11) return "one";
      return isFewRange(n) ? "few" : "many";
    }
    default:
      return n === 1 ? "one" : "other";
  }
}

export function plural(key, n, vars) {
  const form = pluralForm(lang, n);
  let s = strings[key + "." + form];
  if (s === undefined) s = strings[key + ".other"];
  if (s === undefined) return key;
  const all = Object.assign({}, vars, { n: String(n) });
  return applyVars(s, all);
}

export function currentLang() {
  return lang;
}

// zh/ja run calibration-style sentences together with no space, like TheeJ's copy.
export function join(...sentences) {
  return lang === "zh" || lang === "ja" ? sentences.join("") : sentences.join(" ");
}

// --- Theme --------------------------------------------------------------

function relativeLuminance(hex) {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex || "");
  if (!m) return 1;
  const chan = (h) => {
    const c = parseInt(h, 16) / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };
  const r = chan(m[1]);
  const g = chan(m[2]);
  const b = chan(m[3]);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function applyTheme(theme) {
  if (!theme) return;
  if (typeof theme.dark === "boolean") {
    document.documentElement.setAttribute("data-theme", theme.dark ? "dark" : "light");
  }
  if (theme.accent) {
    document.documentElement.style.setProperty("--accent", theme.accent);
    document.documentElement.style.setProperty(
      "--accent-text",
      relativeLuminance(theme.accent) > 0.45 ? "#1a1a1a" : "#ffffff"
    );
  }
}

// --- Height reporting -----------------------------------------------------

// Physical pixels, so Go sizes the window right for any DPI and Windows text size.
function reportHeight() {
  if (!rootEl) return;
  const body = getComputedStyle(document.body);
  const css =
    rootEl.getBoundingClientRect().bottom +
    window.scrollY +
    parseFloat(body.paddingBottom || "0") +
    parseFloat(body.marginBottom || "0");
  send({ type: "height", value: Math.ceil(css * (window.devicePixelRatio || 1)) });
}

function scheduleHeightReport() {
  if (pendingHeight !== null) return;
  pendingHeight = requestAnimationFrame(() => {
    pendingHeight = null;
    reportHeight();
  });
}

// --- Dispatch ---------------------------------------------------------------

function dispatch(msg) {
  if (!msg || typeof msg !== "object") return;
  // Anything Go pushes before init (tab, profile, columns, step) waits and is replayed after it.
  if (!initDone && msg.type !== "init") {
    if (msg.type === "theme") applyTheme(msg);
    early.push(msg);
    return;
  }
  if (msg.type === "init") {
    initDone = true;
    if (typeof msg.lang === "string") lang = msg.lang;
    if (msg.strings) strings = msg.strings;
    if (msg.theme) applyTheme(msg.theme);
  } else if (msg.type === "strings") {
    if (typeof msg.lang === "string") lang = msg.lang;
    if (msg.strings) strings = msg.strings;
  } else if (msg.type === "theme") {
    applyTheme(msg);
  }
  if (handler) handler(msg);
  scheduleHeightReport();
  if (msg.type === "init" && early.length) {
    const queued = early;
    early = [];
    queued.forEach(dispatch);
  }
}

// --- Transport: real WebView2 ------------------------------------------------

function connectReal() {
  window.weej = window.weej || {};
  window.weej.receive = (msg) => dispatch(msg);
  // go-webview2 echoes every posted message straight back as a 'message' event;
  // Go only ever answers through weej.receive(), so the echo must be ignored.
  window.chrome.webview.addEventListener("message", () => {});
  sendImpl = (msg) => {
    window.chrome.webview.postMessage(JSON.stringify(msg));
  };
}

// --- Transport: mock (?mock=settings|calibration|language|update) -----------
//
// Lets every page be opened straight in a normal browser for visual review,
// with sample data standing in for Go. Never reached inside WebView2, since
// window.chrome.webview is always present there.

function svgIcon(fill, glyph) {
  const svg =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">' +
    '<rect width="32" height="32" rx="7" fill="' +
    fill +
    '"/>' +
    (glyph || "") +
    "</svg>";
  return "data:image/svg+xml," + encodeURIComponent(svg);
}

const MOCK_KNOB_GLYPH =
  '<circle cx="16" cy="16" r="8" fill="none" stroke="#fff" stroke-width="2.5"/>' +
  '<rect x="15" y="6" width="2" height="5" fill="#fff"/>';

const MOCK_APP_ICON = svgIcon("#0078d4", MOCK_KNOB_GLYPH);

const MOCK_JOB_ICONS = {
  master: svgIcon("#0078d4"),
  microphone: svgIcon("#5b2d91"),
  systemSounds: svgIcon("#0078d4"),
  brightness: svgIcon("#ca5010"),
  contrast: svgIcon("#ca5010"),
  builtinBrightness: svgIcon("#ca5010"),
  nightLight: svgIcon("#8764b8"),
  externalKeyboard: svgIcon("#107c10"),
  zoom: svgIcon("#008272"),
  otherApps: svgIcon("#5d5d5d"),
  focusedApp: svgIcon("#5d5d5d"),
  app: svgIcon("#5d5d5d"),
};

// A standalone copy of internal/lang/catalogs/en.json: the mock only runs
// outside WebView2, where Go never sends a "strings" payload, so the preview
// carries its own sample copy of the same catalog rather than fetching a path
// the real host would never serve this way.
const MOCK_EN_STRINGS = {
  about: "About WeeJ",
  add_profile: "Add a profile",
  apply: "Apply",
  auto: "Check automatically",
  available: "WeeJ {version} is available",
  calibrate: "Calibrate",
  cancel: "Cancel",
  check: "Check for updates...",
  check_connection: "Check your connection and try again.",
  check_failed: "Couldn't check for updates",
  checking_download: "Checking the download...",
  choose: "Choose",
  choose_language: "Choose your language",
  clear: "Clear",
  click_to_update: "You have {version}. Click to update.",
  close: "Close",
  connected: "Connected: {port}",
  continue: "Continue",
  daily: "Daily",
  default_profile: "Default",
  download: "Download",
  download_failed: "The download failed.",
  downloading: "Downloading version {version}...",
  experimental: "Experimental",
  finish: "Finish",
  hide_icon: "Hide tray icon",
  hide_icon_note: "Open WeeJ again to get back here.",
  icon: "Icon",
  "icon.app": "App icon",
  "icon.dial": "Dial",
  "icon.mixer": "Mixer",
  import_deej: "Import from deej...",
  import_done: "Your deej setup is now the profile â{name}â.",
  import_failed: "Couldn't read that deej config.",
  import_skipped: "Not imported: {items}",
  inspired_by: "Inspired by",
  install_now: "Install version {version} now",
  installed: "Version {version} is installed.",
  installed_only: "WeeJ updates itself only when it runs from its installed folder.",
  installing: "Installing...",
  invert: "Invert knobs",
  invert_note: "For pots wired the other way round.",
  "job.brightness": "Screen {n} brightness",
  "job.builtin_brightness": "Built-in display brightness",
  "job.contrast": "Screen {n} contrast",
  "job.external_keyboard": "External keyboard backlight",
  "job.focused_app": "Focused app",
  "job.master": "Master volume",
  "job.microphone": "Microphone volume",
  "job.night_light": "Night light warmth",
  "job.empty": "Empty",
  "job.other_apps": "Other apps",
  "job.system_sounds": "System sounds",
  "job.zoom": "Screen zoom",
  knob: "Knob {letter}",
  knobs: "Knobs",
  "knobs_found.one": "{n} knob found",
  "knobs_found.other": "{n} knobs found",
  language: "Language",
  language_note: "You can change it later in Settings or in the menu.",
  later: "Later",
  login: "Launch at login",
  made_by: "Made by",
  name: "Name",
  never: "Never",
  newest: "WeeJ {version} is currently the newest version available.",
  next_profile: "Next profile",
  not_connected: "Not connected",
  now_using: "You're now using WeeJ {version}, the newest version available.",
  ok: "OK",
  on_macos: "On macOS:",
  other: "Other...",
  port_busy: "{port} is in use by another app",
  press_shortcut: "Press Shortcut",
  previous_profile: "Previous profile",
  profile: "Profile",
  profile_list: "Profile list",
  profile_n: "Profile {n}",
  profiles: "Profiles",
  quit: "Quit WeeJ",
  reconnect: "Reconnect",
  record_shortcut: "Record Shortcut",
  remove: "Remove",
  remove_knob_info: "What it does in every profile goes with it.",
  remove_named: "Remove â{name}â?",
  remove_profile: "Remove this profile",
  remove_profile_info: "Its knob choices and shortcut go with it.",
  remove_shortcut: "Remove shortcut",
  remove_this_profile: "Remove this profile?",
  reopen: "Reopen",
  reopen_failed: "Couldn't reopen: {error} Quit and open it yourself.",
  "seconds_left.one": "{n} second left",
  "seconds_left.other": "{n} seconds left",
  "section.apps": "Apps",
  "section.brightness": "Brightness",
  "section.contrast": "Contrast",
  "section.keyboard": "Keyboard backlight",
  "section.night_light": "Night light",
  "section.volume": "Volume",
  "section.zoom": "Zoom",
  settings: "Settings",
  "short.builtin_display": "Built-in display",
  "short.external": "External",
  "short.screen": "Screen {n}",
  "short.warmth": "Warmth",
  shortcut: "Shortcut",
  shortcut_tip: "Use Ctrl or Alt with a key. Delete clears it, Escape cancels.",
  skip: "Skip",
  space: "Space",
  speed: "Speed",
  "speed.fast": "Fast",
  "speed.medium": "Medium",
  "speed.slow": "Slow (Recommended)",
  "speed.superFast": "Super fast",
  speed_note: "How soon a change lands after a turn.",
  "tab.about": "About",
  status: "Status",
  port: "Device",
  port_auto: "Automatic",
  port_note: "Automatic finds your board by itself.",
  port_forced: "Set to {port} when WeeJ was started.",
  baud_rate: "Baud rate",
  baud_note: "Must match Serial.begin() in your board’s sketch.",
  refresh: "Refresh",
  tray: "Tray icon",
  tray_tip: "If you don't see its icon, open Show hidden icons on the taskbar and drag it next to the clock.",
  tray_tip_title: "WeeJ is running",
  up_to_date: "You're up to date!",
  update_available: "Update available!",
  update_complete: "Update complete!",
  update_failed: "Couldn't install the update",
  update_now: "Update Now",
  update_question: "You have {version}. Update now?",
  updating_to: "Updating WeeJ to {version}",
  version: "Version {version}",
  website: "Website",
  webview_missing: "WeeJ needs the Microsoft Edge WebView2 Runtime to open this window.",
  weekly: "Weekly",
  wrong_download: "The download isn't WeeJ {version}.",
};

// Partial catalogs, just enough for setLanguage/preview to visibly re-render.
const MOCK_CATALOGS = {
  it: {
    settings: "Impostazioni",
    close: "Chiudi",
    save: "Salva",
    choose_language: "Scegli la tua lingua",
    continue: "Continua",
  },
  fr: {
    settings: "Parametres",
    close: "Fermer",
    save: "Enregistrer",
    choose_language: "Choisissez votre langue",
    continue: "Continuer",
  },
  de: {
    settings: "Einstellungen",
    close: "Schliessen",
    save: "Speichern",
    choose_language: "Wahle deine Sprache",
    continue: "Weiter",
  },
};

function mockMergedStrings(code, enStrings) {
  const over = MOCK_CATALOGS[code];
  if (!over) return enStrings;
  return Object.assign({}, enStrings, over);
}

function mockShortcutLabel(s) {
  if (!s) return null;
  const parts = [];
  if (s.mods & 2) parts.push("Ctrl");
  if (s.mods & 1) parts.push("Alt");
  if (s.mods & 4) parts.push("Shift");
  if (s.mods & 8) parts.push("Win");
  parts.push(s.key || "?");
  return parts.join("+");
}

function mockShortcutKeyJSON(s) {
  if (!s) return null;
  return JSON.stringify({ vk: s.vk, mods: s.mods, key: s.key });
}

// Boards for the preview (?mock=settings&devices=diy,smc,midi): one of each kind set up the way
// a real one was. With no devices the page starts as a first start does, asking how many boards.
function mockKnob(input, kind = "knob", reverse = false) {
  return { kind, input, reverse, min: 0, max: 1023 };
}

function mockDIY(id) {
  return {
    id,
    name: "Desk",
    type: "diy",
    enabled: true,
    port: "COM6",
    baudRate: 9600,
    speed: "slow",
    invert: false,
    controls: [mockKnob(0), mockKnob(3), mockKnob(2), mockKnob(4, "fader"), mockKnob(1, "fader"), mockKnob(-1, "button")],
    layout: [[0, 1, 2], [3, 4, 5]],
    view: "draw",
    profiles: [
      { name: "Default", jobs: [[{ kind: "master" }], [{ kind: "brightness", screen: 0 }], [], [{ kind: "microphone" }], [], []], buttons: { 5: ["media.playpause"] } },
      { name: "Gaming", shortcut: { vk: 49, mods: 3, key: "1" }, jobs: [[{ kind: "master" }], [{ kind: "app", exe: "discord.exe" }], [], [], [], []], buttons: {} },
    ],
    profile: 0,
  };
}

function mockSMC(id) {
  const controls = [];
  for (let i = 0; i < 16; i++) controls.push(mockKnob(i < 8 ? 40 + i : 22 + i, i < 8 ? "fader" : "knob"));
  const jobs = Array.from({ length: 16 }, () => []);
  jobs[0] = [{ kind: "master" }];
  jobs[1] = [{ kind: "brightness", screen: 0 }];
  jobs[2] = [{ kind: "brightness", screen: 1 }];
  jobs[3] = [{ kind: "app", exe: "discord.exe" }, { kind: "app", exe: "spotify.exe" }, { kind: "otherApps" }];
  jobs[5] = [{ kind: "microphone" }];
  jobs[8] = [{ kind: "zoom" }];
  const buttons = { 174: ["profile.previous"], 175: ["profile.next"], 222: ["media.playpause"], 227: ["settings", "url:https://weej.zolfer.com"] };
  for (let i = 0; i < 8; i++) buttons[144 + i] = [`mute:${i}`];
  return {
    id,
    name: "SMC-Mixer",
    type: "smc",
    enabled: true,
    port: "SMC-Mixer",
    speed: "slow",
    invert: false,
    controls,
    view: "draw",
    profiles: [{ name: "Default", jobs, buttons }],
    profile: 0,
    lights: "eqgame",
  };
}

function mockMIDI(id) {
  return {
    id,
    name: "nanoKONTROL",
    type: "midi",
    enabled: false,
    port: "nanoKONTROL2",
    speed: "slow",
    invert: false,
    controls: [mockKnob(16), mockKnob(17), mockKnob(0, "fader"), mockKnob(1, "fader"), mockKnob(171, "button"), mockKnob(-1, "button")],
    layout: [[0, 1], [2, 3], [4, 5]],
    view: "list",
    profiles: [{ name: "Default", jobs: [[{ kind: "master" }], [], [], [], [], []], buttons: { 171: ["media.next"] } }],
    profile: 0,
  };
}

const MOCK_MAKERS = { diy: mockDIY, smc: mockSMC, midi: mockMIDI };

// Served from the repository root, the preview reads the real English catalog, so every string
// shows; anywhere else it keeps the sample copy above.
function loadMockStrings(fallback) {
  return fetch("../../../lang/catalogs/en.json")
    .then((r) => (r.ok ? r.json() : fallback))
    .catch(() => fallback);
}

function mockSettingsInit(enStrings) {
  const kinds = (new URLSearchParams(location.search).get("devices") || "").split(",").filter((k) => MOCK_MAKERS[k]);
  const devices = kinds.map((k, i) => MOCK_MAKERS[k](`d${i + 1}`));
  const labels = {};
  for (const d of devices) for (const p of d.profiles) if (p.shortcut) labels[mockShortcutKeyJSON(p.shortcut)] = mockShortcutLabel(p.shortcut);
  const status = {};
  const values = {};
  for (const d of devices) {
    status[d.id] = { connected: d.enabled, busy: false, port: d.port || "COM6" };
    values[d.id] = d.controls.map((c, k) => (c.kind === "button" ? -1 : (k * 211) % 1024));
  }
  const catalog = [
    { section: "volume", job: { kind: "master" }, title: "Master volume", short: "Master volume", icon: MOCK_JOB_ICONS.master },
    { section: "volume", job: { kind: "microphone" }, title: "Microphone volume", short: "Microphone volume", icon: MOCK_JOB_ICONS.microphone },
    { section: "volume", job: { kind: "systemSounds" }, title: "System sounds", short: "System sounds", icon: MOCK_JOB_ICONS.systemSounds },
    { section: "brightness", job: { kind: "brightness", screen: 0 }, title: "Screen 1 brightness", short: "Screen 1", icon: MOCK_JOB_ICONS.brightness },
    { section: "brightness", job: { kind: "brightness", screen: 1 }, title: "Screen 2 brightness", short: "Screen 2", icon: MOCK_JOB_ICONS.brightness },
    { section: "contrast", job: { kind: "contrast", screen: 0 }, title: "Screen 1 contrast", short: "Screen 1", icon: MOCK_JOB_ICONS.contrast },
    { section: "nightLight", job: { kind: "nightLight" }, title: "Night light warmth", short: "Warmth", icon: MOCK_JOB_ICONS.nightLight },
    { section: "zoom", job: { kind: "zoom" }, title: "Screen zoom", short: "Screen zoom", icon: MOCK_JOB_ICONS.zoom },
    { section: "apps", job: { kind: "otherApps" }, title: "Other apps", short: "Other apps", icon: MOCK_JOB_ICONS.otherApps },
    { section: "apps", job: { kind: "focusedApp" }, title: "Focused app", short: "Focused app", icon: MOCK_JOB_ICONS.focusedApp },
    { section: "apps", job: { kind: "app", exe: "discord.exe" }, title: "Discord", short: "Discord", icon: MOCK_JOB_ICONS.app },
    { section: "apps", job: { kind: "app", exe: "spotify.exe" }, title: "Spotify", short: "Spotify", icon: MOCK_JOB_ICONS.app },
  ];
  const smcButtons = [];
  for (let s = 0; s < 8; s++) for (const first of [16, 8, 0, 24]) smcButtons.push(128 + first + s);
  for (const note of [94, 93, 95, 91, 92, 46, 47, 96, 97, 98, 99]) smcButtons.push(128 + note);
  return {
    type: "init",
    lang: "en",
    strings: enStrings,
    theme: { dark: false, accent: "#0078d4" },
    icon: MOCK_APP_ICON,
    tab: new URLSearchParams(location.search).get("tab") || "general",
    version: "2.0.0",
    website: "https://weej.zolfer.com",
    madeBy: "https://zolfer.com",
    theej: "https://theej.zolfer.com",
    deej: "https://github.com/omriharel/deej",
    languages: [
      { code: "en", name: "English" },
      { code: "de", name: "Deutsch" },
      { code: "it", name: "Italiano" },
    ],
    language: "en",
    settings: { version: 2, devices, added: devices.length, hideTrayIcon: false, showProfileList: true, trayIcon: "mixer", language: "en" },
    catalog,
    iconPreviews: { mixer: svgIcon("#0078d4"), dial: svgIcon("#107c10"), app: svgIcon("#5d5d5d") },
    labels,
    nightLightExperimental: true,
    status,
    values,
    wizard: null,
    forcedPort: "",
    baudRates: [9600, 19200, 38400, 57600, 115200],
    smcButtons,
    lightPatterns: ["off", "on", "random", "eq", "eq2", "eqgame", "fire", "chase", "bounce", "wave", "sparkle", "blink", "rain", "matrix", "snake", "fill", "explode", "checker", "rise", "zigzag", "orbit", "heartbeat", "stars", "bars", "ball", "comet", "helix", "breathe", "clock"],
  };
}

// A scripted calibration: each control is found, swept and held, or pressed three times.
function mockWizard(post, device, controls) {
  const steps = [];
  controls.forEach((k, index) => {
    const button = device.controls[k].kind === "button";
    const base = { device: device.id, control: k, kind: device.controls[k].kind, index, total: controls.length, warning: "", other: -1, done: false };
    if (button) {
      for (let n = 0; n <= 2; n++) steps.push({ ...base, stage: "press", count: n, need: 3, level: 0 });
    } else {
      steps.push({ ...base, stage: "find", count: 0, need: 2, level: 0 });
      for (let n = 1; n <= 2; n++) for (const level of [300, 700, 1023]) steps.push({ ...base, stage: "sweep", count: n, need: 2, level });
    }
  });
  steps.push({ device: device.id, control: -1, kind: "", stage: "done", count: 0, need: 0, level: 0, warning: "", other: -1, index: controls.length, total: controls.length, done: true });
  let i = 0;
  const timer = setInterval(() => {
    if (i >= steps.length) {
      clearInterval(timer);
      return;
    }
    post(Object.assign({ type: "wizard" }, steps[i++]));
  }, 350);
  return () => clearInterval(timer);
}

function startSettingsMock(post, enStrings) {
  let init = null;
  let stopWizard = null;
  let wizardDevice = null;
  const find = (id) => init.settings.devices.find((d) => d.id === id);
  const saveDevice = (d) => {
    const i = init.settings.devices.findIndex((x) => x.id === d.id);
    if (i >= 0) init.settings.devices[i] = d;
    else init.settings.devices.push(d);
    post({ type: "deviceSaved", device: JSON.parse(JSON.stringify(d)) });
  };
  return (msg) => {
    switch (msg.type) {
      case "ready":
        loadMockStrings(enStrings).then((strings) => {
          init = mockSettingsInit(strings);
          post(init);
          window.weejMock = { post };
        });
        break;
      case "save":
        init.settings = JSON.parse(JSON.stringify(msg.settings));
        post({ type: "saved", settings: msg.settings });
        break;
      case "setDevice":
        saveDevice(msg.device);
        break;
      case "addDevice": {
        const id = `d${++init.settings.added}`;
        const controls = [];
        for (const [kind, n] of [["knob", msg.knobs], ["fader", msg.faders], ["button", msg.buttons]]) {
          for (let i = 0; i < n; i++) controls.push(mockKnob(-1, kind));
        }
        const d =
          msg.deviceType === "smc"
            ? Object.assign(mockSMC(id), { name: msg.name, port: msg.port, lights: "" })
            : { id, name: msg.name, type: msg.deviceType, enabled: true, port: msg.port, baudRate: msg.baudRate, speed: "slow", invert: false, controls, view: "draw", profiles: [{ name: "Default", jobs: controls.map(() => []), buttons: {} }], profile: 0 };
        saveDevice(d);
        post({ type: "status", device: id, connected: true, busy: false, port: msg.port || "COM7" });
        post({ type: "added", device: id });
        break;
      }
      case "removeDevice":
        init.settings.devices = init.settings.devices.filter((d) => d.id !== msg.device);
        post({ type: "deviceRemoved", device: msg.device });
        break;
      case "calibrate": {
        const d = find(msg.device);
        if (!d) break;
        if (stopWizard) stopWizard();
        wizardDevice = d;
        stopWizard = mockWizard(post, d, msg.controls && msg.controls.length ? msg.controls : d.controls.map((_, k) => k));
        break;
      }
      case "wizard":
        if (stopWizard) stopWizard();
        stopWizard = null;
        if (msg.op === "finish" && wizardDevice) {
          wizardDevice.controls.forEach((c, k) => {
            if (c.input < 0) c.input = 10 + k;
          });
          saveDevice(wizardDevice);
        }
        if (msg.op === "finish" || msg.op === "cancel") post({ type: "wizard", device: wizardDevice && wizardDevice.id, end: true });
        break;
      case "listPorts":
        setTimeout(() => {
          post({
            type: "ports",
            ports: [
              { name: "COM1", product: "", usb: false },
              { name: "COM6", product: "USB Serial", usb: true },
            ],
            midi: ["SMC-Mixer", "nanoKONTROL2"],
          });
        }, 100);
        break;
      case "pickApp":
        setTimeout(() => {
          post({
            type: "appPicked",
            knob: msg.knob,
            entry: { section: "apps", job: { kind: "app", exe: "vlc.exe" }, title: "VLC", short: "VLC", icon: MOCK_JOB_ICONS.app },
          });
        }, 200);
        break;
      case "importDeej":
        break;
      case "setLanguage":
        post({ type: "strings", lang: msg.code, strings: mockMergedStrings(msg.code, enStrings) });
        break;
      case "key": {
        const hasMod = msg.ctrl || msg.alt;
        if (msg.key === "Delete" || msg.key === "Backspace") {
          post({ type: "recorded", field: msg.field, shortcut: null, label: null });
          break;
        }
        if (!hasMod && msg.field.indexOf("button:") !== 0) {
          post({ type: "rejected", field: msg.field });
          break;
        }
        const shortcut = {
          vk: msg.vk,
          mods: (msg.ctrl ? 2 : 0) | (msg.alt ? 1 : 0) | (msg.shift ? 4 : 0) | (msg.meta ? 8 : 0),
          key: msg.key.length === 1 ? msg.key.toUpperCase() : msg.key,
        };
        post({ type: "recorded", field: msg.field, shortcut, label: mockShortcutLabel(shortcut) });
        break;
      }
    }
  };
}

function startLanguageMock(post, enStrings) {
  return (msg) => {
    switch (msg.type) {
      case "ready":
        post({
          type: "init",
          lang: "en",
          strings: enStrings,
          theme: { dark: false, accent: "#0078d4" },
          languages: [
            { code: "en", name: "English" },
            { code: "de", name: "Deutsch" },
            { code: "fr", name: "Francais" },
            { code: "it", name: "Italiano" },
            { code: "pt", name: "Portugues" },
          ],
          selected: "en",
          icon: MOCK_APP_ICON,
        });
        break;
      case "preview":
        post({ type: "strings", lang: msg.code, strings: mockMergedStrings(msg.code, enStrings) });
        break;
      case "continue":
        break;
    }
  };
}

function startJobsMock(post, enStrings) {
  return (msg) => {
    if (msg.type !== "ready") return;
    const init = mockSettingsInit(enStrings);
    const sections = new Map();
    for (const entry of init.catalog) {
      if (!sections.has(entry.section)) sections.set(entry.section, []);
      sections.get(entry.section).push({ job: entry.job, title: entry.title, icon: entry.icon, badge: false, checked: false });
    }
    post({ type: "init", lang: "en", strings: enStrings, theme: init.theme });
    post({
      type: "menu",
      knob: 0,
      model: {
        sections: [...sections].map(([name, items]) => ({ title: name, items, other: name === "apps" })),
      },
    });
  };
}

function startUpdateMock(post, enStrings) {
  const T = (key, vars) => applyVars(enStrings[key] || key, vars);
  return (msg) => {
    switch (msg.type) {
      case "ready":
        post({ type: "init", lang: "en", strings: enStrings, theme: { dark: false, accent: "#0078d4" }, icon: MOCK_APP_ICON });
        post({ type: "update", heading: T("updating_to", { version: "1.0.1" }), status: T("downloading", { version: "1.0.1" }), done: false });
        setTimeout(() => {
          post({ type: "update", heading: T("updating_to", { version: "1.0.1" }), status: T("installing"), done: false });
        }, 1500);
        setTimeout(() => {
          post({ type: "update", heading: T("updating_to", { version: "1.0.1" }), status: T("installed", { version: "1.0.1" }), done: true });
        }, 3000);
        break;
      case "reopen":
        break;
    }
  };
}

function connectMock(page) {
  const enStrings = Object.assign({}, MOCK_EN_STRINGS);
  let dispatcher;
  const post = (msg) => dispatch(msg);
  switch (page) {
    case "language":
      dispatcher = startLanguageMock(post, enStrings);
      break;
    case "update":
      dispatcher = startUpdateMock(post, enStrings);
      break;
    case "jobs":
      dispatcher = startJobsMock(post, enStrings);
      break;
    default:
      dispatcher = startSettingsMock(post, enStrings);
      break;
  }
  sendImpl = dispatcher;
}

export function send(msg) {
  if (sendImpl) sendImpl(msg);
}

// --- Entry point --------------------------------------------------------------

// Call once per page after registering the message handler. rootId names the
// element whose rendered height is reported to Go after every change.
export function connect(rootId, onMsg) {
  handler = onMsg;
  rootEl = document.getElementById(rootId);

  if (window.chrome && window.chrome.webview) {
    connectReal();
  } else {
    const params = new URLSearchParams(location.search);
    const page = params.get("mock") || document.body.dataset.page || "settings";
    connectMock(page);
  }

  const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
  applyTheme({ dark: systemDark.matches });
  systemDark.addEventListener("change", (e) => applyTheme({ dark: e.matches }));

  if (rootEl && "ResizeObserver" in window) {
    new ResizeObserver(scheduleHeightReport).observe(rootEl);
  }

  send({ type: "ready" });
}
