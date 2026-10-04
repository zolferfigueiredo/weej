import { connect, send, t } from "./bridge.js";

const iconEl = document.getElementById("icon");
const headingEl = document.getElementById("heading");
const selectEl = document.getElementById("lang-select");
const noteEl = document.getElementById("note");
const continueBtn = document.getElementById("btn-continue");

function esc(s) {
  return String(s == null ? "" : s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

let languages = [];
let selected = "en";

function render() {
  headingEl.textContent = t("choose_language");
  noteEl.textContent = t("language_note");
  continueBtn.textContent = t("continue");
  selectEl.innerHTML = languages
    .map((l) => `<option value="${esc(l.code)}"${l.code === selected ? " selected" : ""}>${esc(l.name)}</option>`)
    .join("");
}

function onMessage(msg) {
  if (msg.type === "init") {
    languages = msg.languages || [];
    selected = msg.selected || "en";
    if (msg.icon) iconEl.src = msg.icon;
    render();
  } else if (msg.type === "strings") {
    render();
  }
}

selectEl.addEventListener("change", () => {
  selected = selectEl.value;
  send({ type: "preview", code: selected });
});

continueBtn.addEventListener("click", () => {
  send({ type: "continue", code: selected });
});

connect("root", onMessage);
