// The General tab: the boards, each with its switch, status and settings, then the app's own.
import { t } from "./bridge.js";
import { GEAR_ICON, statusText } from "./boards.js";
import { S, devices, esc, escAttr } from "./state.js";

export function renderGeneral() {
  const draft = S.draft;
  const counts = [1, 2, 3, 4]
    .map((n) => `<button class="btn" type="button" data-action="setup-count" data-count="${n}">${n}</button>`)
    .join("");
  const rows = devices().length
    ? devices().map(boardRow).join("")
    : `<div class="setup">
            <p class="setup-title">${esc(t("setup.title"))}</p>
            <p class="row-desc">${esc(t("setup.note"))}</p>
            <div class="segmented setup-counts">${counts}</div>
          </div>`;
  const langOptions = (S.init.languages || [])
    .map((l) => `<option value="${escAttr(l.code)}"${l.code === draft.language ? " selected" : ""}>${esc(l.name)}</option>`)
    .join("");
  const hidden = draft.hideTrayIcon;

  document.getElementById("panel").innerHTML = `
    <div class="tabpanel general-list" role="tabpanel">
      <div class="group">
        <div class="group-head">
          <h2 class="group-title">${esc(t("boards.title"))}</h2>
          <span class="spacer"></span>
          <button class="btn" type="button" data-action="add-board">${esc(t("boards.add"))}</button>
        </div>
        <div class="card">${rows}</div>
      </div>

      <div class="group">
        <h2 class="group-title">${esc(t("general.app"))}</h2>
        <div class="card">
          <div class="row">
            <div class="row-main"><span class="row-title">${esc(t("language"))}</span></div>
            <div class="row-control"><select class="select" id="language-select">${langOptions}</select></div>
          </div>
          <div class="row">
            <div class="row-main">
              <span class="row-title">${esc(t("hide_icon"))}</span>
              <span class="row-desc">${esc(t("hide_icon_note"))}</span>
            </div>
            <div class="row-control"><input class="toggle" id="hide-icon" type="checkbox" role="switch"${hidden ? " checked" : ""} /></div>
          </div>
          <div class="row">
            <div class="row-main"><span class="row-title${hidden ? " disabled" : ""}">${esc(t("profile_list"))}</span></div>
            <div class="row-control"><input class="toggle" id="show-profile-list" type="checkbox" role="switch"${draft.showProfileList ? " checked" : ""}${hidden ? " disabled" : ""} /></div>
          </div>
        </div>
      </div>
    </div>`;
}

// A board's row: switched on or off at once, its name opening its tab, its status and its gear.
function boardRow(d) {
  const st = statusText(d);
  return `
          <div class="row board-row">
            <input class="toggle" type="checkbox" role="switch" data-enable="${escAttr(d.id)}" aria-label="${escAttr(t("boards.on", { name: d.name }))}"${d.enabled ? " checked" : ""} />
            <div class="row-main">
              <button class="link-button row-title" type="button" data-action="open-board" data-board="${escAttr(d.id)}">${esc(d.name)}</button>
              <span class="row-desc">${esc(t("device.type." + d.type))}</span>
            </div>
            <span class="board-status${st.cls}">${esc(st.text)}</span>
            <button class="btn btn-icon" type="button" data-action="gear" data-board="${escAttr(d.id)}" title="${escAttr(t("boards.settings"))}" aria-label="${escAttr(t("boards.settings"))}">${GEAR_ICON}</button>
          </div>`;
}
