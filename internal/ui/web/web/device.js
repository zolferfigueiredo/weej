// The M-VAVE SMC-Mixer as Settings draws it. Sizes are in millimetres off the real panel, from
// Mixxx's diagram, with knob 1's centre at 0,0. The LED and fader sit a little lower than on the
// panel to make room for the label under each knob.
//
// Control ids are core's (internal/core/smc.go): fader i is i, its knob 8+i, and a button is
// 128 plus the note it sends in DAW mode.

export const SMC_COLUMNS = [40, 41, 42, 43, 44, 45, 46, 47, 30, 31, 32, 33, 34, 35, 36, 37];

export function isSMCPort(port) {
  return (port || "").startsWith("midi:") && port.toLowerCase().includes("smc-mixer");
}

const PITCH = 18;
const KNOB_R = 4.68;
const LED = { y: 10, w: 4.32, h: 1.2 };
const TRACK = { top: 15, len: 31.1, w: 1.35 };
const CAP = { w: 7.1, h: 3.8 };
const KNOB_LABEL_Y = 8.7;
const FADER_LABEL_Y = 50.9;
// Matches .smc .label's font-size; a label may take a strip's width, less a gap.
const LABEL_SIZE = 2.6;
const LABEL_MAX = 17;
// M, S, R and Square, top to bottom to the right of the fader, by the first note of each row.
const STRIP_BUTTONS = [
  { first: 16, glyph: "M", y: 20.1 },
  { first: 8, glyph: "S", y: 27.7 },
  { first: 0, glyph: "R", y: 35.4 },
  { first: 24, glyph: "", y: 42.6 },
];
const BUTTON = { x: 8.8, size: 4.6 };
const BOTTOM = { y: 57, w: 7.4, h: 4.2, x0: 5.04, pitch: 12.574 };

// Left to right, as core's smcBottomNotes.
export const SMC_BOTTOM = [
  { note: 94, icon: "play", name: "action.play" },
  { note: 93, icon: "pause", name: "action.pause" },
  { note: 95, icon: "record", name: "smc.record" },
  { note: 91, icon: "previous", name: "action.previous_track" },
  { note: 92, icon: "next", name: "action.next_track" },
  { note: 46, icon: "bankLeft", name: "smc.bank_left" },
  { note: 47, icon: "bankRight", name: "smc.bank_right" },
  { note: 96, icon: "up", name: "smc.up" },
  { note: 97, icon: "down", name: "smc.down" },
  { note: 98, icon: "left", name: "smc.left" },
  { note: 99, icon: "right", name: "smc.right" },
];

const ICONS = {
  play: `<path class="icon" d="M-0.9 -1.2L1.2 0L-0.9 1.2Z"/>`,
  pause: `<path class="icon" d="M-1 -1.1h0.75v2.2h-0.75zM0.25 -1.1h0.75v2.2h-0.75z"/>`,
  record: `<circle class="icon" r="1.1"/>`,
  previous: `<path class="icon" d="M-1.3 -1.1h0.55v2.2h-0.55zM1.3 -1.1L-0.6 0L1.3 1.1Z"/>`,
  next: `<path class="icon" d="M0.75 -1.1h0.55v2.2h-0.55zM-1.3 -1.1L0.6 0L-1.3 1.1Z"/>`,
  bankLeft: `<path class="icon-line" d="M-0.1 -1.1L-1.2 0L-0.1 1.1M1.2 -1.1L0.1 0L1.2 1.1"/>`,
  bankRight: `<path class="icon-line" d="M0.1 -1.1L1.2 0L0.1 1.1M-1.2 -1.1L-0.1 0L-1.2 1.1"/>`,
  up: `<path class="icon" d="M0 -1L1.15 0.9H-1.15Z"/>`,
  down: `<path class="icon" d="M0 1L1.15 -0.9H-1.15Z"/>`,
  left: `<path class="icon" d="M-1 0L0.9 -1.15V1.15Z"/>`,
  right: `<path class="icon" d="M1 0L-0.9 -1.15V1.15Z"/>`,
};

export function isStripControl(id) {
  return id >= 0 && id < 16;
}

// The strip a fader or knob belongs to.
export function stripOf(id) {
  return id % 8;
}

// The name a control goes by, as the inspector heads it and a mute names it.
export function smcControlName(id, t) {
  if (id < 8) return t("mixer.fader", { n: String(id + 1) });
  if (id < 16) return t("mixer.knob", { n: String(id - 7) });
  const note = id - 128;
  if (note >= 0 && note < 32) {
    const n = String((note % 8) + 1);
    const row = STRIP_BUTTONS.find((b) => note >= b.first && note < b.first + 8);
    return row.glyph ? row.glyph + n : t("smc.square", { n });
  }
  const bottom = SMC_BOTTOM.find((b) => b.note === note);
  return bottom ? t(bottom.name) : String(id);
}

export function smcButtonIcon(id) {
  const bottom = SMC_BOTTOM.find((b) => b.note === id - 128);
  return bottom ? `<svg class="head-icon" viewBox="-1.6 -1.6 3.2 3.2" aria-hidden="true">${ICONS[bottom.icon]}</svg>` : "";
}

function esc(s) {
  return String(s == null ? "" : s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

let measure = null;

function labelWidth(text) {
  if (!measure) {
    measure = document.createElement("canvas").getContext("2d");
    measure.font = `100px ${getComputedStyle(document.body).fontFamily}`;
  }
  return (measure.measureText(text).width * LABEL_SIZE) / 100;
}

function fitLabel(text, max) {
  if (labelWidth(text) <= max) return text;
  const chars = Array.from(text);
  while (chars.length > 1 && labelWidth(chars.join("").trimEnd() + "…") > max) chars.pop();
  return chars.join("").trimEnd() + "…";
}

function capY(value) {
  const v = value < 0 ? 0.5 : value / 1023;
  return TRACK.top + (1 - v) * TRACK.len;
}

function pointerAngle(value) {
  return -135 + (270 * Math.max(0, value)) / 1023;
}

// opts: { name(id), label(id) -> { text, more, empty, title }, assigned(id), selected, values[16],
// strips[8], lit(id) }
export function smcSVG(opts) {
  const classes = (id, extra) => {
    let c = "ctl" + (extra ? " " + extra : "");
    if (id === opts.selected) c += " selected";
    if (opts.lit(id)) c += " lit";
    return c;
  };
  const control = (id, extra, body) => {
    const label = opts.label(id);
    const title = label.title ? `${opts.name(id)}: ${label.title}` : opts.name(id);
    return `<g class="${classes(id, extra)}" id="ctl-${id}" data-action="select-control" data-control="${id}" role="button" tabindex="0" aria-label="${esc(title)}"${id === opts.selected ? ' aria-current="true"' : ""}><title>${esc(title)}</title>${body}</g>`;
  };
  const labelText = (id, y) => {
    const l = opts.label(id);
    const more = l.more ? ` +${l.more}` : "";
    const text = fitLabel(l.text, LABEL_MAX - (more ? labelWidth(more) : 0)) + more;
    return `<text class="label${l.empty ? " empty" : ""}" y="${y}">${esc(text)}</text>`;
  };

  let out = "";
  for (let s = 0; s < 8; s++) {
    const x = s * PITCH;
    const knob = 8 + s;
    const kv = opts.values[knob];
    out += `<g transform="translate(${x} 0)">`;
    out += control(
      knob,
      "knob" + (kv < 0 ? " unknown" : ""),
      `<circle class="face" r="${KNOB_R}"/>` +
        `<line class="pointer" x1="0" y1="-1.7" x2="0" y2="-3.9" transform="rotate(${pointerAngle(kv)})"/>` +
        labelText(knob, KNOB_LABEL_Y)
    );
    out += `<rect class="led${opts.strips[s] ? " on" : ""}" data-strip="${s}" x="${-LED.w / 2}" y="${LED.y}" width="${LED.w}" height="${LED.h}" rx="${LED.h / 2}"/>`;
    const fv = opts.values[s];
    out += control(
      s,
      "fader" + (fv < 0 ? " unknown" : ""),
      `<rect class="well" x="-4.2" y="${TRACK.top - 2.6}" width="8.4" height="${TRACK.len + 5.2}" rx="1.4"/>` +
        `<rect class="track" x="${-TRACK.w / 2}" y="${TRACK.top}" width="${TRACK.w}" height="${TRACK.len}" rx="${TRACK.w / 2}"/>` +
        `<g class="cap" transform="translate(0 ${capY(fv)})"><rect x="${-CAP.w / 2}" y="${-CAP.h / 2}" width="${CAP.w}" height="${CAP.h}" rx="0.7"/><line x1="${-CAP.w / 2 + 0.8}" x2="${CAP.w / 2 - 0.8}"/></g>` +
        labelText(s, FADER_LABEL_Y)
    );
    for (const b of STRIP_BUTTONS) {
      const id = 128 + b.first + s;
      const half = BUTTON.size / 2;
      const glyph = b.glyph
        ? `<text class="glyph" y="0.95">${b.glyph}</text>`
        : `<rect class="glyph-box" x="-0.9" y="-0.9" width="1.8" height="1.8" rx="0.35"/>`;
      out += control(
        id,
        "button",
        `<g transform="translate(${BUTTON.x} ${b.y})"><rect class="face" x="${-half}" y="${-half}" width="${BUTTON.size}" height="${BUTTON.size}" rx="0.8"/>${glyph}` +
          (opts.assigned(id) ? `<circle class="dot" cx="${half - 0.9}" cy="${-half + 0.9}" r="0.55"/>` : "") +
          `</g>`
      );
    }
    out += `</g>`;
  }
  SMC_BOTTOM.forEach((b, i) => {
    const id = 128 + b.note;
    const hw = BOTTOM.w / 2;
    const hh = BOTTOM.h / 2;
    out += control(
      id,
      "button",
      `<g transform="translate(${BOTTOM.x0 + i * BOTTOM.pitch} ${BOTTOM.y})"><rect class="face" x="${-hw}" y="${-hh}" width="${BOTTOM.w}" height="${BOTTOM.h}" rx="0.8"/>${ICONS[b.icon]}` +
        (opts.assigned(id) ? `<circle class="dot" cx="${hw - 0.9}" cy="${-hh + 0.9}" r="0.55"/>` : "") +
        `</g>`
    );
  });
  return `<svg class="smc" viewBox="-8.8 -7.7 150 69.8" role="group">${out}</svg>`;
}

// Moves a drawn fader's cap or knob's pointer without drawing the mixer again.
export function smcShowValue(root, id, value) {
  const g = root.querySelector(`.smc .ctl[data-control="${id}"]`);
  if (!g) return;
  g.classList.toggle("unknown", value < 0);
  if (id < 8) g.querySelector(".cap").setAttribute("transform", `translate(0 ${capY(value)})`);
  else g.querySelector(".pointer").setAttribute("transform", `rotate(${pointerAngle(value)})`);
}

export function smcShowStrip(root, strip, on) {
  const led = root.querySelector(`.smc .led[data-strip="${strip}"]`);
  if (led) led.classList.toggle("on", on);
}
