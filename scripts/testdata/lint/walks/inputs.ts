// The interface's own code opening the microphone, the camera or the
// screen, or reading the clipboard.
import { Clipboard } from "@wailsio/runtime";

export async function take() {
  await navigator.mediaDevices.getUserMedia({ audio: true });
  await navigator.mediaDevices.getDisplayMedia();
  await navigator.clipboard.readText();
  await Clipboard.Text();
  // Putting something on the clipboard is no read.
  await navigator.clipboard.writeText("link");
  await Clipboard.SetText("link");
}
