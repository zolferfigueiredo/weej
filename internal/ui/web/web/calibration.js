import { connect, send, t } from "./bridge.js";

// Every visible string here comes straight from Go's "step" push (title, body,
// progress, warning, button); this page only ever renders the last one it got.

const titleEl = document.getElementById("title");
const bodyEl = document.getElementById("body");
const progressEl = document.getElementById("progress");
const cancelBtn = document.getElementById("btn-cancel");
const actionBtn = document.getElementById("btn-action");

let action = "skip"; // "skip", "finish", or "continue" (a mixer going on from its knobs to its buttons)

function renderChrome() {
  cancelBtn.textContent = t("cancel");
  actionBtn.textContent = t(action);
}

function onMessage(msg) {
  if (msg.type === "init") {
    renderChrome();
  } else if (msg.type === "step") {
    titleEl.textContent = msg.title || "";
    bodyEl.textContent = msg.body || "";
    progressEl.textContent = msg.progress || "";
    progressEl.classList.toggle("warning", !!msg.warning);
    action = msg.button || "skip";
    renderChrome();
  }
}

cancelBtn.addEventListener("click", () => send({ type: "cancel" }));
actionBtn.addEventListener("click", () => send({ type: action === "skip" ? "skip" : "finish" }));

window.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    e.preventDefault();
    send({ type: "cancel" });
  }
});

connect("root", onMessage);
