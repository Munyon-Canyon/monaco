# Waitlist page: design direction

Locked before building, per the anti-ai-slop skill (`.cursor/skills/anti-ai-slop`, §13 and §15F).
Change this file first if the direction changes; don't improvise tokens in `index.html`.

## Direction
A members' share certificate from a security printer. Two banknote-green inks,
ruled ledger rows, and halftone as the printing technique.

## Intent
Someone deciding whether to join the waitlist for an app where friends run a fund together.
In three seconds: what it is, and that it's a real club, not a crypto toy.

## Signature
The halftone print. The Monaco icon is drawn live on canvas as one cream screen on the dark page,
shaded like lit stone, on a fine screen (dot pitch 0.9% of the icon's width). It prints in from the
baseline, then stays still while three soft blobs wander through it on smooth, non-repeating paths.
A blob thins the cream so the page's own green shows through; a floor of small dots keeps every
stone's shape readable. A pointer over the figures themselves is a lamp held over the print: the
light swings round to it so every stone's shadow follows the hand, the blobs scatter from it and
the cream under it recovers, and it fades out once the pointer leaves the ink. The rest of the page,
and the bare corners of the canvas, leave the mark alone.

## Tokens
Two colours only: Monaco green is the page and cream is everything on it. Any other tone is cream mixed
into the green, never a second green, so nothing can read as a shade off.

| Token | Value | Use |
| --- | --- | --- |
| `--paper` | `#0F291C` | Monaco green, the page background and the only green |
| `--ink` | `#E6E0CC` | soft cream: every piece of text and the button (11.7:1). One text colour |
| `--ink-press` | `#D3CCB6` | button hover |
| `--field` | `transparent` | the email/handle field sits on the page |
| `--ink-soft` / `--ink-faint` | `#909786` | cream 60% into the page: placeholders, disabled button, visited links (5.1:1) |
| `--rule-strong` | `#697666` | cream 42% into the page: field outline (3.2:1) |
| `--rule` | `#2F4436` | cream 15% into the page: footer rule |
| `--error` | `#FF8A7A` | validation errors only (6.8:1) |
| `--screen` | `#E6E0CC` | the icon's one ink: cream dots grow where the stones catch the light |

- Type: Fraunces only. 300 for "Monaco" (opsz 144) and the line under it (opsz 24, always one line);
  400 at text size (opsz 14) for the button, fields, messages and footer. Sentence case, no italics.
  IBM Plex Mono was dropped so the button matches the rest of the page.
- Radius: 3px everywhere (certificate, not app chip).
- Spacing: 8 / 16 / 24 / 40 / 64 / 96.
- Motion: 180ms ease-out for state changes. Three moments: the icon prints in, the blobs wander
  through it, and they scatter from a pointer (30fps, paused off-screen or in a hidden tab); button
  hover. Honors `prefers-reduced-motion`, which also switches the pointer response off.
- Layout: phones get one centred column: icon, "Monaco", the line, then the sign-up bar, with a
  centred footer; it fits a 375x812 phone without scrolling. From 820px the icon and the copy form
  one lockup centred in the window (icon left, copy left-aligned beside it, sharing a vertical
  centre), so there is no dead band between them.
- Form: one control that steps through the sign-up. "Join the waitlist →" opens an email field with
  an → button; that checks the email and swaps in the optional Twitter handle with a final →;
  submitting replaces it with "You're on the list." Escape on the handle steps back to the email.

## Rejected defaults
- Centered full-viewport hero with one line and one CTA.
- Cream + serif + italic "editorial" (the 2026 cluster).
- Fade-up on every paragraph; 3-icon feature grid; cards.
- Invented stats, user counts or testimonials.
