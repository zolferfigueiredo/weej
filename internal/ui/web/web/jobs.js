import { connect, send, t } from "./bridge.js";

// The job menu, in its own popup window. Settings builds the list and gets every tick back.

const root = document.getElementById("root");
let knob = 0;
let sections = []; // [{ title, items: [{ job, title, icon, badge, checked }], other }]

function esc(s) {
  return String(s == null ? "" : s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

function item(entry, s, i) {
  const id = `job-${s}-${i}`;
  const badge = entry.badge ? `<span class="badge">${esc(t("experimental"))}</span>` : "";
  return `<label class="popover-item" for="${id}">
    <input class="chk" type="checkbox" id="${id}" data-section="${s}" data-item="${i}"${entry.checked ? " checked" : ""} />
    <img src="${esc(entry.icon)}" alt="" />
    <span data-style="flex:1 1 auto;">${esc(entry.title)}</span>
    ${badge}
  </label>`;
}

function render() {
  let html = `<button class="popover-item" type="button" data-action="clear">${esc(t("clear"))}</button>`;
  html += `<div class="popover-sep"></div>`;
  sections.forEach((section, s) => {
    html += `<div class="popover-header">${esc(section.title)}</div>`;
    html += section.items.map((entry, i) => item(entry, s, i)).join("");
    if (section.other) html += `<button class="popover-item" type="button" data-action="pick-app">${esc(t("other"))}</button>`;
  });
  root.innerHTML = html;
}

root.addEventListener("change", (e) => {
  const el = e.target;
  if (!el.matches("input.chk")) return;
  const entry = sections[Number(el.dataset.section)].items[Number(el.dataset.item)];
  entry.checked = el.checked;
  send({ type: "toggle", job: entry.job, checked: el.checked });
});

root.addEventListener("click", (e) => {
  const target = e.target.closest("[data-action]");
  if (!target) return;
  if (target.dataset.action === "clear") {
    for (const section of sections) for (const entry of section.items) entry.checked = false;
    root.querySelectorAll("input.chk").forEach((el) => (el.checked = false));
    send({ type: "clear" });
  } else if (target.dataset.action === "pick-app") {
    send({ type: "pickApp", knob });
  }
});

window.addEventListener("keydown", (e) => {
  if (e.key === "Escape") send({ type: "close" });
});

// Go shows the popup only once it hears back how tall this knob's list is.
function onMessage(msg) {
  if (msg.type === "menu") {
    knob = msg.knob;
    sections = (msg.model && msg.model.sections) || [];
    render();
    window.scrollTo(0, 0);
    const height = root.getBoundingClientRect().height * (window.devicePixelRatio || 1);
    send({ type: "menuReady", height: Math.ceil(height) });
  } else if (msg.type === "strings") {
    render();
  }
}

connect("root", onMessage);
