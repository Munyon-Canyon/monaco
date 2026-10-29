# F design package

F is the Monaco app's design: option F, "Native", chosen by the founder on 10-26. This folder is the in-repo copy of the designer's round 11, the final set, as it stood after the founder's 10-27 evening notes. Tickets in the F launch milestone link their reference frames here.

The designer drew 141 frames. This folder keeps 120: **Launch (phase 1) 104** and **Later (phase 2) 16**. The 21 Alternatives not chosen are not in the repo.

## Files

| Path | What it holds |
| --- | --- |
| `screens/<frame>.png` | One render per frame. A phone is 393 × 852 pt at 2x (786 × 1704); the boards (`01-tokens`, `01-tokens-type`, `01-tokens-controls`, `01-chart-variants`) are wider |
| `index.html` | The gallery: every frame with its caption, Launch then Later. Open it in a browser from a checkout; it loads nothing from the network |
| `notes.md` | The founder's decisions, the Feed sizes and Feed comments of 10-27, the launch versus later split, the fixture table (sample data only) and the build map: each Launch frame, the app screen it becomes and the ticket that builds it |
| `kit.md` | F's kit: tokens, components, density rules, states and the 14 parts. CSS class names there are the designer's; the app's names are in `docs/design.md` |
| `qa/` | The milestone's QA findings. Tickets #6583 to #6587 add it |

## Reading a frame name

A frame is `<screen number>-<screen>-<state>`: `04-vote-holding` is screen 04 (the proposal), the vote, while the hold runs. `04b-hold-1-pick` is the same hold as Home's vote card draws it. A `-scrolled` frame is the same screen scrolled down.

Dark is the default appearance. A frame ending `-light` is the light twin of the dark frame with the same name minus the suffix (`02-home-dark` has `02-home-light`).

## Phase 2

Later frames are phase 2: widgets, StandBy, Live Activities, the Dynamic Island, Follow, Spotlight, Siri, seasons and streaks. They are kept as reference and are never built in the F launch milestone. A Launch frame never draws one of them; `notes.md` lists what each Launch frame dropped.

## Superseded

The earlier v3 design package is removed. Never build from it.
