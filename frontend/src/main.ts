import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import ReportWindow from "./screens/ReportWindow.svelte";
import { listen } from "./lib/said";

listen();

// The report window is the same page with ?window=report, see bugs.go.
const report = new URLSearchParams(location.search).get("window") === "report";
mount(report ? ReportWindow : App, { target: document.getElementById("app")! });
