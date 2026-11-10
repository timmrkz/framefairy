import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import ReportWindow from "./screens/ReportWindow.svelte";
import { listen, said } from "./lib/said";
import { escapeLetsGo } from "./lib/keys";
import { api, onCopy } from "./lib/api";
import { toCopy } from "./lib/copy";

listen();
window.addEventListener("keydown", escapeLetsGo);

// Copy in the Edit menu, in every window, see lib/copy.ts. The Go side puts
// the text on the clipboard. Nothing is copied when nothing is chosen, the
// way the system's own Copy does nothing then.
onCopy(() => {
  const text = toCopy();
  if (!text) return;
  said(`copy: ${text.length} characters`);
  api.copy(text).catch((err) => said(`copy: the clipboard did not take it, ${err}`, { level: "warn" }));
});

// The report window is the same page with ?window=report, see bugs.go.
const report = new URLSearchParams(location.search).get("window") === "report";
mount(report ? ReportWindow : App, { target: document.getElementById("app")! });
