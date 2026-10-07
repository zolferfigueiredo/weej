// The dialogs: a board's settings (its gear), adding a board, calibrating one, and asking before
// anything is removed.
import { t } from "./bridge.js";
import { shortcutControl } from "./pickers.js";
import { S, alsoUsedBy, clone, controlName, deviceById, devices, esc, escAttr, statusOf } from "./state.js";

export function openGear(id) {
  const d = deviceById(id);
  if (d) S.dialog = { kind: "gear", id, dev: clone(d) };
}

export function openAdd() {
  S.dialog = { kind: "add", name: "", type: "diy", port: "", baud: 9600, knobs: 4, faders: 0, buttons: 0 };
}

export function renderDialog() {
  const dlg = S.dialog;
  let html = "";
  if (dlg.kind === "gear") html = gearDialog(dlg);
  else if (dlg.kind === "add") html = addDialog(dlg);
  else if (dlg.kind === "wizard") html = wizardDialog(dlg);
  else html = confirmDialog(dlg);
  document.getElementById("root").insertAdjacentHTML("beforeend", `<div class="dialog-scrim">${html}</div>`);
}

function field(label, control, note = "") {
  return `
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(label)}</span>
              ${note ? `<span class="row-desc">${esc(note)}</span>` : ""}
            </div>
            <div class="row-control">${control}</div>
          </div>`;
}

function portLabel(p) {
  return p.product ? `${p.name} (${p.product})` : p.name;
}

// The other board using a port or MIDI input, if any.
function usedBy(port, exceptId) {
  const other = devices().find((d) => d.id !== exceptId && d.port && d.port === port);
  return other ? other.name : "";
}

// A board's port: Automatic or a COM port for a DIY board, a MIDI input for any other. One that
// another board uses says so.
function portSelect(id, type, value, exceptId) {
  const option = (v, label) => `<option value="${escAttr(v)}"${value === v ? " selected" : ""}>${esc(label)}</option>`;
  const named = (v, label) => {
    const other = usedBy(v, exceptId);
    return option(v, other ? t("device.used_by", { port: label, board: other }) : label);
  };
  let options;
  if (type === "diy") {
    const list = S.ports || [];
    options = option("", t("port_auto")) + list.map((p) => named(p.name, portLabel(p))).join("");
    if (value && !list.some((p) => p.name === value)) options += option(value, value);
  } else {
    const list = S.midiInputs || [];
    options = (value ? "" : option("", t("device.pick_input"))) + list.map((name) => named(name, name)).join("");
    if (value && !list.includes(value)) options += option(value, value);
  }
  return `<button class="btn btn-icon btn-subtle" type="button" data-action="refresh-ports" title="${escAttr(t("refresh"))}" aria-label="${escAttr(t("refresh"))}">&#x21bb;</button><select class="select" id="${id}">${options}</select>`;
}

function baudSelect(id, value) {
  const rates = (S.init.baudRates || [9600]).slice();
  const v = value || 9600;
  if (!rates.includes(v)) rates.push(v);
  return `<select class="select" id="${id}">${rates.map((r) => `<option value="${r}"${r === v ? " selected" : ""}>${r}</option>`).join("")}</select>`;
}

function gearDialog(dlg) {
  const d = dlg.dev;
  const st = statusOf(d.id);
  let status = t("status.disconnected");
  if (!d.enabled) status = t("status.off");
  else if (st.connected) status = t("connected", { port: st.port });
  else if (st.busy) status = t("port_busy", { port: st.port });
  const speedOptions = ["slow", "medium", "fast", "superFast"]
    .map((s) => `<option value="${s}"${d.speed === s ? " selected" : ""}>${esc(t("speed." + s))}</option>`)
    .join("");
  const pattern = d.lights || "off";
  const lightOptions = (S.init.lightPatterns || ["off"])
    .map((p) => `<option value="${p}"${p === pattern ? " selected" : ""}>${esc(t("lights." + p))}</option>`)
    .join("");
  const shortcutRow = (field_, label, s) => {
    const others = alsoUsedBy(s, d.id);
    return field(label, shortcutControl(field_, s, d.id, true), others.length ? t("shortcut.also_used", { names: others.join(", ") }) : "");
  };
  const forced = d.type === "diy" && S.init.forcedPort && devices().find((b) => b.type === "diy") === deviceById(d.id);
  return `
      <div class="dialog dialog-wide" role="dialog" aria-modal="true">
        <div class="dialog-title">${esc(t("boards.settings_of", { name: d.name || t("device.type." + d.type) }))}</div>
        <div class="dialog-body form">
          ${field(t("name"), `<input class="input input-name" id="dlg-name" type="text" value="${escAttr(d.name)}" />`)}
          ${field(t("device.type"), `<span class="row-desc">${esc(t("device.type." + d.type))}</span>`)}
          ${field(t("status"), `<span class="row-desc${st.connected ? " status-ok" : ""}">${esc(status)}</span><button class="btn" type="button" data-action="reconnect" data-board="${escAttr(d.id)}">${esc(t("reconnect"))}</button>`)}
          ${field(t("port"), forced ? `<span class="row-desc">${esc(S.init.forcedPort)}</span>` : portSelect("dlg-port", d.type, d.port, d.id), forced ? t("port_forced", { port: S.init.forcedPort }) : t(d.type === "diy" ? "port_note" : "mixer_port_note"))}
          ${d.type === "diy" ? field(t("baud_rate"), baudSelect("dlg-baud", d.baudRate), t("baud_note")) : ""}
          ${d.type === "diy" ? field(t("speed"), `<select class="select" id="dlg-speed">${speedOptions}</select>`, t("speed_note")) : ""}
          ${field(t("invert"), `<input class="toggle" id="dlg-invert" type="checkbox" role="switch"${d.invert ? " checked" : ""} />`, t("invert_note"))}
          ${d.type === "smc" ? field(t("lights"), `<select class="select" id="dlg-lights">${lightOptions}</select>`, t("lights_note")) : ""}
          ${shortcutRow("next", t("next_profile"), d.nextProfile)}
          ${shortcutRow("previous", t("previous_profile"), d.previousProfile)}
        </div>
        <div class="dialog-actions">
          <button class="btn btn-danger" type="button" data-action="remove-board">${esc(t("device.remove"))}</button>
          <span class="spacer"></span>
          ${d.type === "smc" ? "" : `<button class="btn" type="button" data-action="calibrate-board" data-board="${escAttr(d.id)}">${esc(t("calibrate"))}</button>`}
          <button class="btn" type="button" data-action="cancel-dialog">${esc(t("cancel"))}</button>
          <button class="btn btn-primary" type="button" data-action="save-board">${esc(t("save"))}</button>
        </div>
      </div>`;
}

function addDialog(dlg) {
  const types = ["diy", "smc", "midi"]
    .map((k) => `<button class="btn${k === dlg.type ? " on" : ""}" type="button" data-action="add-type" data-type="${k}" aria-pressed="${k === dlg.type}">${esc(t("device.type." + k))}</button>`)
    .join("");
  const count = (id, label, value) =>
    field(label, `<input class="input input-count" id="${id}" type="number" min="0" max="64" value="${value}" />`);
  const counted = dlg.type !== "smc";
  const ready = dlg.name.trim() !== "" && (dlg.type === "diy" || dlg.port !== "") && (!counted || dlg.knobs + dlg.faders + dlg.buttons > 0);
  return `
      <div class="dialog dialog-wide" role="dialog" aria-modal="true">
        <div class="dialog-title">${esc(S.setup ? t("setup.board", { n: String(S.setup.done + 1), of: String(S.setup.total) }) : t("add.title"))}</div>
        <div class="dialog-body form">
          ${field(t("name"), `<input class="input input-name" id="add-name" type="text" value="${escAttr(dlg.name)}" placeholder="${escAttr(t("add.name_hint"))}" />`)}
          ${field(t("device.type"), `<span class="segmented">${types}</span>`, t("add.type_note." + dlg.type))}
          ${field(t("port"), portSelect("add-port", dlg.type, dlg.port, ""), t(dlg.type === "diy" ? "port_note" : "mixer_port_note"))}
          ${dlg.type === "diy" ? field(t("baud_rate"), baudSelect("add-baud", dlg.baud), t("baud_note")) : ""}
          ${counted ? count("add-knobs", t("knobs"), dlg.knobs) + count("add-faders", t("list.faders"), dlg.faders) + count("add-buttons", t("list.buttons"), dlg.buttons) : ""}
        </div>
        <div class="dialog-actions">
          <span class="spacer"></span>
          <button class="btn" type="button" data-action="cancel-dialog">${esc(t("cancel"))}</button>
          <button class="btn btn-primary" type="button" data-action="add-confirm"${ready ? "" : " disabled"}>${esc(t(counted ? "add.next" : "add.add"))}</button>
        </div>
      </div>`;
}

function wizardDialog(dlg) {
  const d = deviceById(dlg.id);
  const w = S.wizard && S.wizard.device === dlg.id ? S.wizard : null;
  const name = d ? d.name : "";
  let body;
  if (!w || !d) {
    body = `<p class="wizard-text">${esc(t("wizard.starting"))}</p>`;
  } else if (w.done) {
    body = `<p class="wizard-text">${esc(t("wizard.done"))}</p>`;
  } else {
    const control = controlName(d, w.control);
    const pot = w.kind !== "button";
    let text;
    if (w.stage === "find") text = t("wizard.find", { name: control });
    else if (w.stage === "sweep") text = t(w.count >= 2 ? "wizard.hold" : "wizard.sweep");
    else text = t("wizard.press", { name: control });
    let warning = "";
    if (w.warning === "wrong" && w.other >= 0) warning = t("wizard.wrong", { other: controlName(d, w.other), name: control });
    if (w.warning === "mismatch") warning = t("wizard.mismatch", { name: control });
    const counter = w.stage === "find" ? "" : t("wizard.count", { n: String(w.count), of: String(w.need) });
    const connected = statusOf(d.id).connected;
    body = `
          ${w.index === 0 && pot ? `<p class="wizard-note">${esc(t("wizard.zero"))}</p>` : ""}
          <p class="wizard-step">${esc(t("wizard.step", { i: String(w.index + 1), n: String(w.total) }))}: <strong>${esc(control)}</strong></p>
          <p class="wizard-text">${esc(text)}</p>
          ${pot ? `<div class="level"><div class="level-fill" id="wizard-level"></div></div>` : ""}
          ${counter ? `<p class="wizard-count">${esc(counter)}</p>` : ""}
          ${warning ? `<p class="wizard-warning warning">${esc(warning)}</p>` : ""}
          ${connected ? "" : `<p class="wizard-note">${esc(t("wizard.waiting", { name }))}</p>`}`;
  }
  const done = w && w.done;
  return `
      <div class="dialog dialog-wide" role="dialog" aria-modal="true">
        <div class="dialog-title">${esc(t("wizard.title", { name }))}</div>
        <div class="dialog-body">${body}</div>
        <div class="dialog-actions">
          ${done ? "" : `<button class="btn" type="button" data-action="wizard-op" data-op="redo">${esc(t("wizard.redo"))}</button><button class="btn" type="button" data-action="wizard-op" data-op="skip">${esc(t("wizard.skip"))}</button>`}
          <span class="spacer"></span>
          <button class="btn" type="button" data-action="wizard-op" data-op="cancel">${esc(t("cancel"))}</button>
          ${done ? `<button class="btn btn-primary" type="button" data-action="wizard-op" data-op="finish">${esc(t("wizard.finish"))}</button>` : ""}
        </div>
      </div>`;
}

// The level bar follows the pot without drawing the dialog again.
export function showWizardLevel() {
  const bar = document.getElementById("wizard-level");
  if (bar && S.wizard) bar.style.width = `${Math.round((S.wizard.level / 1023) * 100)}%`;
}

function confirmDialog(dlg) {
  let title = "";
  let body = "";
  const d = deviceById(dlg.id);
  if (dlg.what === "profile" && d) {
    const name = d.profiles[d.profile] && d.profiles[d.profile].name;
    title = name ? t("remove_named", { name }) : t("remove_this_profile");
    body = t("remove_profile_info");
  } else if (dlg.what === "control" && d) {
    title = t("remove_named", { name: controlName(d, dlg.k) });
    body = t("remove_knob_info");
  } else if (dlg.what === "board" && d) {
    title = t("remove_named", { name: d.name });
    body = t("device.remove_info");
  }
  return `
      <div class="dialog" role="alertdialog" aria-modal="true">
        <div class="dialog-title">${esc(title)}</div>
        <div class="dialog-body">${esc(body)}</div>
        <div class="dialog-actions">
          <button class="btn btn-primary btn-danger" type="button" data-action="confirm-dialog">${esc(t("remove"))}</button>
          <button class="btn" type="button" data-action="cancel-dialog">${esc(t("cancel"))}</button>
        </div>
      </div>`;
}
