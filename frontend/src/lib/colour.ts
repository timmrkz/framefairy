// The one colour the app is picked out in. Everything the app draws in its
// own colour takes it from --accent, and the lighter one and the wash are
// mixed from it in app.css, so setting this one moves all three.
//
// What a person typed is untrusted like anything else, so a value that is
// not a colour is ignored rather than written into the page.
const looksLikeColour = /^#?[0-9a-fA-F]{6}$/;

export function wearColour(colour: string) {
  const text = (colour ?? "").trim();
  if (!looksLikeColour.test(text)) return;
  const hex = text.startsWith("#") ? text : `#${text}`;
  document.documentElement.style.setProperty("--accent", hex);
}
