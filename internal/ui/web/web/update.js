import { connect } from "./bridge.js";

const iconEl = document.getElementById("icon");
const headingEl = document.getElementById("heading");
const statusEl = document.getElementById("status");
const barEl = document.getElementById("bar");
const barFillEl = barEl.querySelector("span");

// WeeJ restarts into the new version on its own once it is installed, which closes this window.
function onMessage(msg) {
  if (msg.type === "init") {
    if (msg.icon) iconEl.src = msg.icon;
  } else if (msg.type === "update") {
    headingEl.textContent = msg.heading || "";
    statusEl.textContent = msg.status || "";
    statusEl.title = msg.status || "";
    if (msg.done) {
      barEl.classList.remove("indeterminate");
      barFillEl.style.transform = "scaleX(1)";
    } else {
      barEl.classList.add("indeterminate");
      barFillEl.style.transform = "";
    }
  }
}

connect("root", onMessage);
