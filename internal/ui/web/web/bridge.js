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
  add_knob: "Add a knob",
  add_profile: "Add a profile",
  auto: "Check automatically",
  available: "WeeJ {version} is available",
  "cal.all_found": "All knobs found",
  "cal.all_found_text": "Your board sends {n} values, and each one has its knob now, so that's all of them. Click Finish to choose what they do.",
  "cal.finish_later": "Finish stops here and leaves it for later.",
  "cal.hold": "Your knobs hold still until you finish.",
  "cal.move": "Move knob {letter} from one end to the other.",
  "cal.move_first": "Move knob {letter} from one end to the other, so WeeJ can tell which knob is which. Each knob then gets {n} seconds of turning, which cleans a jumpy one.",
  "cal.no_knob": "If you don't have a knob {letter}, click Finish.",
  "cal.paused": "Paused with {left}. Keep turning knob {letter}.",
  "cal.skip_keeps": "It's already set up, so Skip keeps it as it is.",
  "cal.turn": "Found it. Now turn it back and forth, from one end to the other, for {n} seconds. The timer only runs while it turns. Skip if it doesn't need cleaning.",
  "cal.waiting_board": "Waiting for the board to connect",
  "cal.waiting_knob": "Waiting for knob {letter} to move",
  "cal.wrong": "That's knob {wrong}. Move knob {letter} instead.",
  calibrate: "Calibrate",
  calibration_title: "WeeJ Calibration",
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
  "job.nothing": "Nothing",
  "job.other_apps": "Other apps",
  "job.system_sounds": "System sounds",
  "job.zoom": "Screen zoom",
  jobs_note: "Tick one or more jobs per knob, for this profile.",
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
  needs_calibration: "Needs calibration",
  never: "Never",
  newest: "WeeJ {version} is currently the newest version available.",
  next_profile: "Next profile",
  no_knobs: "No knobs. Calibrate finds them, or press + to add one.",
  not_connected: "Not connected",
  now_using: "You're now using WeeJ {version}, the newest version available.",
  ok: "OK",
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
  remove_knob: "Remove the last knob",
  remove_knob_info: "What it does in every profile goes with it.",
  remove_knob_q: "Remove knob {letter}?",
  remove_named: "Remove â{name}â?",
  remove_profile: "Remove this profile",
  remove_profile_info: "Its knob choices and shortcut go with it.",
  remove_shortcut: "Remove shortcut",
  remove_this_profile: "Remove this profile?",
  reopen: "Reopen",
  reopen_failed: "Couldn't reopen: {error} Quit and open it yourself.",
  save: "Save",
  scoop_managed: "This copy was installed with Scoop. Update it with: scoop update weej",
  "seconds_left.one": "{n} second left",
  "seconds_left.other": "{n} seconds left",
  "section.apps": "Apps",
  "section.brightness": "Brightness",
  "section.contrast": "Contrast",
  "section.keyboard": "Keyboard backlight",
  "section.night_light": "Night light",
  "section.volume": "Volume",
  "section.zoom": "Zoom",
  sensitivity: "Sensitivity",
  settings: "Settings",
  "short.builtin_display": "Built-in display",
  "short.external": "External",
  "short.screen": "Screen {n}",
  "short.warmth": "Warmth",
  shortcut: "Shortcut",
  shortcut_tip: "Use Ctrl or Alt with a key. Delete clears it, Escape cancels.",
  shortcuts: "Shortcuts",
  skip: "Skip",
  space: "Space",
  speed: "Speed",
  "speed.fast": "Fast",
  "speed.medium": "Medium",
  "speed.slow": "Slow (Recommended)",
  "speed.superFast": "Super fast",
  speed_note: "How soon a change lands after a turn.",
  "tab.about": "About",
  "tab.app": "App settings",
  "tab.connection": "Connection",
  status: "Status",
  port: "Port",
  port_auto: "Automatic",
  port_auto_found: "Automatic ({port})",
  port_note: "Automatic finds your board by itself.",
  port_forced: "Set to {port} when WeeJ was started.",
  baud_rate: "Baud rate",
  baud_note: "Must match Serial.begin() in your board’s sketch.",
  refresh: "Refresh",
  "tab.general": "General",
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

function mockSettingsInit(enStrings) {
  const profiles = [
    {
      name: "Default",
      jobs: [[{ kind: "master" }], [{ kind: "brightness", screen: 0 }], []],
      shortcut: null,
    },
    {
      name: "Gaming",
      jobs: [
        [{ kind: "master" }],
        [{ kind: "app", exe: "discord.exe" }],
        [{ kind: "app", exe: "spotify.exe" }],
      ],
      shortcut: { vk: 49, mods: 3, key: "1" },
    },
  ];
  const labels = {};
  for (const p of profiles) {
    if (p.shortcut) labels[mockShortcutKeyJSON(p.shortcut)] = mockShortcutLabel(p.shortcut);
  }
  const catalog = [
    { section: "volume", job: { kind: "master" }, title: "Master volume", short: "Master volume", icon: MOCK_JOB_ICONS.master },
    { section: "volume", job: { kind: "microphone" }, title: "Microphone volume", short: "Microphone volume", icon: MOCK_JOB_ICONS.microphone },
    { section: "volume", job: { kind: "systemSounds" }, title: "System sounds", short: "System sounds", icon: MOCK_JOB_ICONS.systemSounds },
    { section: "brightness", job: { kind: "brightness", screen: 0 }, title: "Screen 1 brightness", short: "Screen 1", icon: MOCK_JOB_ICONS.brightness },
    { section: "brightness", job: { kind: "brightness", screen: 1 }, title: "Screen 2 brightness", short: "Screen 2", icon: MOCK_JOB_ICONS.brightness },
    { section: "contrast", job: { kind: "contrast", screen: 0 }, title: "Screen 1 contrast", short: "Screen 1", icon: MOCK_JOB_ICONS.contrast },
    { section: "nightLight", job: { kind: "nightLight" }, title: "Night light warmth", short: "Warmth", icon: MOCK_JOB_ICONS.nightLight },
    { section: "keyboard", job: { kind: "externalKeyboard" }, title: "External keyboard backlight", short: "External", icon: MOCK_JOB_ICONS.externalKeyboard },
    { section: "zoom", job: { kind: "zoom" }, title: "Screen zoom", short: "Screen zoom", icon: MOCK_JOB_ICONS.zoom },
    { section: "apps", job: { kind: "otherApps" }, title: "Other apps", short: "Other apps", icon: MOCK_JOB_ICONS.otherApps },
    { section: "apps", job: { kind: "focusedApp" }, title: "Focused app", short: "Focused app", icon: MOCK_JOB_ICONS.focusedApp },
    { section: "apps", job: { kind: "app", exe: "discord.exe" }, title: "Discord", short: "Discord", icon: MOCK_JOB_ICONS.app },
    { section: "apps", job: { kind: "app", exe: "spotify.exe" }, title: "Spotify", short: "Spotify", icon: MOCK_JOB_ICONS.app },
    { section: "apps", job: { kind: "app", exe: "chrome.exe" }, title: "Google Chrome", short: "Google Chrome", icon: MOCK_JOB_ICONS.app },
  ];
  return {
    type: "init",
    lang: "en",
    strings: enStrings,
    theme: { dark: false, accent: "#0078d4" },
    icon: MOCK_APP_ICON,
    tab: "general",
    version: "1.0.0",
    website: "https://github.com/zolferfigueiredo/weej",
    madeBy: "https://zolfer.com",
    deej: "https://github.com/omriharel/deej",
    languages: [
      { code: "en", name: "English" },
      { code: "de", name: "Deutsch" },
      { code: "fr", name: "Francais" },
      { code: "it", name: "Italiano" },
      { code: "ja", name: "Japanese" },
    ],
    language: "en",
    setup: {
      columns: [0, 1, -1],
      profiles,
      profile: 0,
      nextProfile: null,
      previousProfile: null,
      invertKnobs: false,
      hideTrayIcon: false,
      showProfileList: true,
      trayIcon: "mixer",
      speed: "slow",
      port: "",
      baudRate: 9600,
    },
    catalog,
    iconPreviews: { mixer: svgIcon("#0078d4"), dial: svgIcon("#107c10"), app: svgIcon("#5d5d5d") },
    labels,
    nightLightExperimental: true,
    connection: { connected: true, busy: false, port: "COM6" },
    forcedPort: "",
    baudRates: [9600, 19200, 38400, 57600, 115200],
  };
}

function startSettingsMock(post, enStrings) {
  let draft = null;
  return (msg) => {
    switch (msg.type) {
      case "ready":
        draft = mockSettingsInit(enStrings);
        post(draft);
        break;
      case "save":
        post({ type: "saved", setup: msg.setup });
        break;
      case "listPorts":
        setTimeout(() => {
          post({
            type: "ports",
            ports: [
              { name: "COM1", product: "", usb: false },
              { name: "COM6", product: "USB Serial", usb: true },
            ],
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
        setTimeout(() => {
          post({ type: "imported", setup: draft.setup, skipped: ["unknown.exe"] });
        }, 200);
        break;
      case "setLanguage":
        post({ type: "strings", lang: msg.code, strings: mockMergedStrings(msg.code, enStrings) });
        break;
      case "record":
        break;
      case "key": {
        const hasMod = msg.ctrl || msg.alt;
        if (!hasMod && msg.key !== "Delete" && msg.key !== "Backspace") {
          post({ type: "rejected", field: msg.field });
          break;
        }
        if (msg.key === "Delete" || msg.key === "Backspace") {
          post({ type: "recorded", field: msg.field, shortcut: null, label: null });
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
      case "stopRecording":
      case "calibrate":
      case "checkUpdates":
      case "openUrl":
      case "close":
        break;
    }
  };
}

function startCalibrationMock(post, enStrings) {
  const T = (key, vars) => applyVars(enStrings[key] || key, vars);
  const letters = ["A", "B", "C"];
  let found = [true, false, false];
  let step = 0;
  let timers = [];
  const clear = () => {
    timers.forEach(clearTimeout);
    timers = [];
  };
  const push = (p) => post(Object.assign({ type: "step" }, p));

  function waitingKnob(i) {
    push({ title: T("knob", { letter: letters[i] }), body: T("cal.waiting_knob", { letter: letters[i] }), progress: "", warning: false, button: "skip" });
  }
  function wrongKnob(i, wrongLetter) {
    push({
      title: T("knob", { letter: letters[i] }),
      body: T("cal.waiting_knob", { letter: letters[i] }),
      progress: T("cal.wrong", { wrong: wrongLetter, letter: letters[i] }),
      warning: true,
      button: "skip",
    });
    timers.push(setTimeout(() => waitingKnob(i), 1400));
  }
  function turning(i, secondsLeft) {
    if (secondsLeft <= 0) {
      found[i] = true;
      runStep(i + 1);
      return;
    }
    push({
      title: T("knob", { letter: letters[i] }),
      body: T("cal.turn", { n: "4" }),
      progress: T("seconds_left." + (secondsLeft === 1 ? "one" : "other"), { n: String(secondsLeft) }),
      warning: false,
      button: "skip",
    });
    timers.push(setTimeout(() => turning(i, secondsLeft - 1), 250 * 4));
  }
  function runStep(i) {
    step = i;
    if (i >= letters.length) {
      push({ title: T("cal.all_found"), body: T("cal.all_found_text", { n: String(letters.length) }), progress: "", warning: false, button: "finish" });
      return;
    }
    waitingKnob(i);
    timers.push(setTimeout(() => wrongKnob(i, letters[(i + 1) % letters.length]), 1800));
    timers.push(setTimeout(() => turning(i, 4), 3600));
  }

  return (msg) => {
    switch (msg.type) {
      case "ready":
        post({ type: "init", lang: "en", strings: enStrings, theme: { dark: false, accent: "#0078d4" } });
        timers.push(setTimeout(() => runStep(0), 500));
        break;
      case "skip":
        clear();
        runStep(step + 1);
        break;
      case "finish":
      case "cancel":
        clear();
        break;
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
    post({
      type: "init",
      lang: "en",
      strings: enStrings,
      theme: init.theme,
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
    case "calibration":
      dispatcher = startCalibrationMock(post, enStrings);
      break;
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
