import { mount } from "svelte";
import "../../src/app.css";
import Motion from "./Motion.svelte";

mount(Motion, { target: document.getElementById("motion")! });
