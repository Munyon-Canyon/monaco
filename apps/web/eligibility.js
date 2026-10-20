// The founders' line under the sign-up form, from WAITLIST_ELIGIBILITY_LINE at build time. It is operator copy,
// but it lands in HTML, so it is escaped. Empty or blank leaves no element at all, not an empty one.

const MARKER = "<!--eligibility-->";
const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };

export function withEligibility(html, line) {
  const text = (line ?? "").trim();
  const element = text ? `<p class="elig mono">${text.replace(/[&<>"']/g, (c) => ESCAPES[c])}</p>` : "";
  // A function, so a "$&" or "$'" in the founders' words is text and not a replacement pattern.
  return html.replace(MARKER, () => element);
}
