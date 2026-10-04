import { connect, send, t } from "./bridge.js";

const iconEl = document.getElementById("icon");
const headingEl = document.getElementById("heading");
const statusEl = document.getElementById("status");
const barEl = document.getElementById("bar");
const barFillEl = barEl.querySelector("span");
const reopenBtn = document.getElementById("btn-reopen");

function render() {
  reopenBtn.textContent = t("reopen");
}

function onMessage(msg) {
  if (msg.type === "init") {
    if (msg.icon) iconEl.src = msg.icon;
    render();
  } else if (msg.type === "update") {
    headingEl.textContent = msg.heading || "";
    statusEl.textContent = msg.status || "";
    statusEl.title = msg.status || "";
    if (msg.done) {
      barEl.classList.remove("indeterminate");
      barFillEl.style.transform = "scaleX(1)";
      reopenBtn.disabled = false;
    } else {
      barEl.classList.add("indeterminate");
      barFillEl.style.transform = "";
      reopenBtn.disabled = true;
    }
  }
}

reopenBtn.addEventListener("click", () => {
  if (!reopenBtn.disabled) send({ type: "reopen" });
});

connect("root", onMessage);
