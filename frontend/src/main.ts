import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { listen } from "./lib/said";

listen();

mount(App, { target: document.getElementById("app")! });
