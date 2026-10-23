# Monaco personality

What a member knows us by with the logo covered. Six signatures, each drawn once in `Design/` and shown everywhere its trigger appears. One signature per screen at a time; nothing competes. Rejected on sight: mascots, memes, confetti, casino, crypto. The palette, the 15pt floor and AA are untouched (`design.md`).

| # | Signature | What it is | Where it appears | Tokens and motion | Why it is ours |
| --- | --- | --- | --- | --- | --- |
| 1 | **One hand** | A 16-grid pixel glyph family (`PixelBitmap`, `MonacoGlyph`), drawn by the hand that drew the pixel animals. One grid, one 1-unit stroke, square corners, no anti-aliasing. SF Symbols stay the fallback for anything not drawn. | Tab bar, cabal action discs, outcome orb, empty-state tile, toast glyph, row chevron, glass buttons, vote buttons | Glyph colour is the text colour of its context (`ink`, `brand`, `secondaryText`, `onBrand`, `onCover`). Sizes 16, 20, 22, 30 at the 16 grid. No motion of its own. | Robinhood and Coinbase use stroked line icons; ours are built from the same squares as the faces beside them, so the whole app reads as one drawing. |
| 2 | **A cabal is a place** | A cabal without a picture is never a bare tint: its emblem, one of eight pixel characters from the Monaco cast (`CabalEmblem`, chosen by id like its tint), tiles across the cover in `onCover` at low alpha and sits in the mark instead of initials. | Cover hero, cover cards, cabal marks in every row, invite and share covers | `CabalTint.fill` under `MonacoTheme.emblemPattern` (`onCover` at 0.12) over the flat `coverOverlay`. Pattern cell 28pt, offset rows. No motion. | Eight tints times five colours give every cabal a face and a colour at a glance, drawn in the house hand, without stock photos or a gradient. |
| 3 | **The settle** | A hero figure rolls with `.numericText` in the direction of the change and lands with weight: a 1.03 to 1 spring on the figure. Row figures roll without the weight. | Home total, cabal pot, stock price, amount entry, proposal amount, your slice | `MonacoMotion.settle`: `spring(response: 0.45, dampingFraction: 0.72)`; roll `snappy(0.25)`. Off under Reduce Motion. | Robinhood's number spins like a slot; ours counts and then sits down, the way a figure lands in a ledger. |
| 4 | **The stamp** | Money or a vote landing gets one moment, the same every time: the pixel check stamps in (scale 0.6 to 1, opacity 0 to 1), `.success` haptic, then the toast or outcome. Never confetti. | Deposit landed, fund confirmed, vote recorded, trade filled, cash-out sent, withdrawal sent | `MonacoMotion.stamp`: `spring(response: 0.38, dampingFraction: 0.6)`; glyph `brand` on `brandSoft` (outcome) or `toastSuccessGlyph` on `toastFill` (toast). Reduce Motion: fade only. | Coinbase celebrates; Monaco acknowledges. One stamp, like a teller's. |
| 5 | **The vote object** | The vote bar is the most recognisable thing in the app: square-ended segments (yes `profitVivid`, no `lossVivid`, waiting `hairline`), drawing on from the left when they appear and re-flowing when a vote lands, under the "n yes votes to pass" line. | Proposal screen, proposal card footer, votes sheet, home needs-your-vote | `MonacoMotion.drawOn`: `easeOut(0.45)` scale from the leading edge; re-flow `easeOut(0.2)`. Bar 8pt, gap 3pt. | No competitor has a vote. Its shape is the same squares as the glyphs, so it is ours before it is read. |
| 6 | **A face for everyone** | Nobody is initials. A member without a photo is a pixel animal picked by a stable hash of their id, the same on every screen and device. | Every avatar: rows, boards, cards, chat, comments, proposals | `PixelAnimal.forSeed(id)`, drawn `interpolation(.none)` in the avatar circle. | Our default face is drawn, not typeset. It is the first thing a new member sees of us. |

## Rules of use

- A glyph that replaces a label keeps the label as its accessibility label.
- A signature that cannot appear everywhere its trigger appears does not ship.
- New motion goes in `MonacoMotion`; new glyphs go in `PixelGlyphs.swift`; nothing in `Features/` draws a bitmap or writes an animation curve.
- Gradients stay banned (decision 12). The emblem pattern is flat colour at one alpha.

## Not tonight, decisions for the founder

- A member-chosen cabal emblem needs a backend field; tonight the emblem is picked by id.
- The cast (aliens, dogs, bingo, pilates, quants) as member faces would change every existing default face. Shipped as cabal emblems only.
- A wordmark was not attempted tonight; "monaco" stays SF Pro Bold (decision 9). The four-figure mark in `LaunchMark` is the candidate.
