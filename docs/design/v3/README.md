# Monaco UI v3 reference

v3 is the design the iOS app is being rebuilt to match: a cream-paper, forest-green, glass-chrome look with bordered cards, capsule P&L pills, cover-photo cabal cards, a dark identity card and floating glass chrome. It comes from the Monaco UI 1.5.3 design package. This folder is the in-repo copy, so tickets and PRs can link to it.

| File | What it holds |
| --- | --- |
| [README.md](README.md) | This page: how to open the prototype, image naming, index of every screen and state |
| [tokens.md](tokens.md) | Colours, type, spacing, radii, shadows and motion with exact values and SwiftUI names |
| [components.md](components.md) | Every component: anatomy, sizes, states, tokens, screens, planned SwiftUI file |
| [decisions.md](decisions.md) | The twelve founder decisions and risks R1–R5 |
| [monaco-ui-v3.html](monaco-ui-v3.html) | The self-contained interactive prototype (works offline) |
| `screens/` | 298 reference renders, JPG, 402×874 pt at 3x |
| `contact-sheets/` | One overview sheet per area |

Numbers: 43 screen templates, 255 screen/state combinations, 9 areas. Every template has its default state in Light and Dark; the other 212 states are Light only.

## Open the prototype

1. Download or clone the repo and open `docs/design/v3/monaco-ui-v3.html` in Safari or Chrome. It is one file with the CSS, JS and images embedded (text uses the system font), so it needs no network and no server. GitHub only shows its source; open the local copy.
2. The controls sit above the two phone frames:
   - **Screen**: dropdown grouped by area (`01 · Sign in` … `43 · Manage watchlist`). The left sidebar lists the same screens with their state counts; type in **Find a screen…** to filter it.
   - **State**: dropdown of the chosen screen's states, in the order of the tables below. The first state is the default.
   - **Light / Both / Dark** buttons choose the appearance. Both shows two frames side by side (the dark frame hides on narrow windows).
   - **Larger text** checkbox switches the frames to the 1.22 text scale. Native Dynamic Type must be reviewed in the app.
   - **Solid surfaces** checkbox is the reduce-transparency variant: glass chrome becomes solid.
   - **Walk a flow…** dropdown plus **Previous / Next** steps through eight flows: Sign in, Deposit → fund a cabal, Idea → vote → execution, Cash out → withdraw, Discover → request / follow, Sell proposal, Markets → holding, Native presentation patterns.
3. The frames are live: tapping inside a frame navigates, and the Screen and State controls follow.
4. There are no query parameters. The page reads the URL **hash** once on load: `monaco-ui-v3.html#screen=<template>&state=<state>`, for example `#screen=proposal&state=voted`. An unknown `screen` is ignored; an unknown or missing `state` falls back to the screen's default. Appearance, Larger text and Solid surfaces are not in the hash.

The prototype draws a status bar, home indicator and blur compositor. The app does not (see decision 6 in [decisions.md](decisions.md)). All figures are fixtures.

## Image naming

`screens/<group>__<template>__<state>__<light|dark>.jpg`

- `<group>` is the area in lowercase: `access`, `home`, `cabals`, `money`, `proposals`, `markets`, `feed`, `explore`, `profile`. Note that `balances` and `notifications` live in the Home group, and `leaderboard` in Explore.
- `<template>` and `<state>` are the ids shown in the tables and in the prototype's dropdowns.
- Example: `screens/proposals__proposal__voted__light.jpg`. The dark twin exists only for the default state.
- Contact sheets are `contact-sheets/<group>.jpg`.

Reference images are drawn at the design's 10–13px caption sizes. The app keeps a 15pt text floor, so built screens run a little taller; parity QA accepts that.

Where an image and the app disagree, [decisions.md](decisions.md#which-source-wins) says which wins. The `expanded` states of `review-proposal` and `votes` show the prototype's full-height sheet, which the app never draws: every native sheet opens at one half-height detent. Use those two images for content order only.

## Index

In each row the first link is the default state (Light, with the Dark twin in brackets); every other state is Light.


### Access (2) · [contact sheet](contact-sheets/access.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `signin` | Sign in | [`phone`](screens/access__signin__phone__light.jpg) ([dark](screens/access__signin__phone__dark.jpg)) · [`email`](screens/access__signin__email__light.jpg) · [`invalid`](screens/access__signin__invalid__light.jpg) · [`sending`](screens/access__signin__sending__light.jpg) · [`error`](screens/access__signin__error__light.jpg) |
| `verify` | Verify code | [`ready`](screens/access__verify__ready__light.jpg) ([dark](screens/access__verify__ready__dark.jpg)) · [`invalid`](screens/access__verify__invalid__light.jpg) · [`expired`](screens/access__verify__expired__light.jpg) · [`verifying`](screens/access__verify__verifying__light.jpg) · [`error`](screens/access__verify__error__light.jpg) |

### Home (3) · [contact sheet](contact-sheets/home.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `home` | Home | [`loss`](screens/home__home__loss__light.jpg) ([dark](screens/home__home__loss__dark.jpg)) · [`gain`](screens/home__home__gain__light.jpg) · [`empty`](screens/home__home__empty__light.jpg) · [`cash-only`](screens/home__home__cash-only__light.jpg) · [`multiple-proposals`](screens/home__home__multiple-proposals__light.jpg) · [`voted`](screens/home__home__voted__light.jpg) · [`executing`](screens/home__home__executing__light.jpg) · [`completed`](screens/home__home__completed__light.jpg) · [`expired`](screens/home__home__expired__light.jpg) · [`blocked`](screens/home__home__blocked__light.jpg) · [`no-proposals`](screens/home__home__no-proposals__light.jpg) · [`proposals-loading`](screens/home__home__proposals-loading__light.jpg) · [`proposals-error`](screens/home__home__proposals-error__light.jpg) · [`loading`](screens/home__home__loading__light.jpg) · [`stale`](screens/home__home__stale__light.jpg) · [`error`](screens/home__home__error__light.jpg) |
| `balances` | Your money | [`ready`](screens/home__balances__ready__light.jpg) ([dark](screens/home__balances__ready__dark.jpg)) · [`cash-only`](screens/home__balances__cash-only__light.jpg) · [`loading`](screens/home__balances__loading__light.jpg) · [`error`](screens/home__balances__error__light.jpg) |
| `notifications` | Activity | [`unread`](screens/home__notifications__unread__light.jpg) ([dark](screens/home__notifications__unread__dark.jpg)) · [`read`](screens/home__notifications__read__light.jpg) · [`empty`](screens/home__notifications__empty__light.jpg) · [`loading`](screens/home__notifications__loading__light.jpg) · [`error`](screens/home__notifications__error__light.jpg) |

### Cabals (10) · [contact sheet](contact-sheets/cabals.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `cabals` | Your cabals | [`ready`](screens/cabals__cabals__ready__light.jpg) ([dark](screens/cabals__cabals__ready__dark.jpg)) · [`no-action`](screens/cabals__cabals__no-action__light.jpg) · [`empty`](screens/cabals__cabals__empty__light.jpg) · [`filtered`](screens/cabals__cabals__filtered__light.jpg) · [`loading`](screens/cabals__cabals__loading__light.jpg) · [`error`](screens/cabals__cabals__error__light.jpg) · [`no-share`](screens/cabals__cabals__no-share__light.jpg) · [`pnl-unavailable`](screens/cabals__cabals__pnl-unavailable__light.jpg) · [`rank-loading`](screens/cabals__cabals__rank-loading__light.jpg) · [`rank-error`](screens/cabals__cabals__rank-error__light.jpg) · [`rank-empty`](screens/cabals__cabals__rank-empty__light.jpg) |
| `discover` | Discover cabals | [`results`](screens/cabals__discover__results__light.jpg) ([dark](screens/cabals__discover__results__dark.jpg)) · [`no-results`](screens/cabals__discover__no-results__light.jpg) · [`request-pending`](screens/cabals__discover__request-pending__light.jpg) · [`loading`](screens/cabals__discover__loading__light.jpg) · [`error`](screens/cabals__discover__error__light.jpg) |
| `create-cabal` | Start a cabal | [`ready`](screens/cabals__create-cabal__ready__light.jpg) ([dark](screens/cabals__create-cabal__ready__dark.jpg)) · [`invalid`](screens/cabals__create-cabal__invalid__light.jpg) · [`selected-voters`](screens/cabals__create-cabal__selected-voters__light.jpg) · [`creating`](screens/cabals__create-cabal__creating__light.jpg) · [`error`](screens/cabals__create-cabal__error__light.jpg) |
| `cabal` | Cabal overview | [`ready`](screens/cabals__cabal__ready__light.jpg) ([dark](screens/cabals__cabal__ready__dark.jpg)) · [`no-share`](screens/cabals__cabal__no-share__light.jpg) · [`cash-only`](screens/cabals__cabal__cash-only__light.jpg) · [`no-proposals`](screens/cabals__cabal__no-proposals__light.jpg) · [`voted`](screens/cabals__cabal__voted__light.jpg) · [`executing`](screens/cabals__cabal__executing__light.jpg) · [`completed`](screens/cabals__cabal__completed__light.jpg) · [`loading`](screens/cabals__cabal__loading__light.jpg) · [`stale`](screens/cabals__cabal__stale__light.jpg) · [`error`](screens/cabals__cabal__error__light.jpg) |
| `public-cabal` | Public cabal | [`preview`](screens/cabals__public-cabal__preview__light.jpg) ([dark](screens/cabals__public-cabal__preview__dark.jpg)) · [`pending`](screens/cabals__public-cabal__pending__light.jpg) · [`member`](screens/cabals__public-cabal__member__light.jpg) · [`closed`](screens/cabals__public-cabal__closed__light.jpg) · [`private`](screens/cabals__public-cabal__private__light.jpg) · [`error`](screens/cabals__public-cabal__error__light.jpg) |
| `cabal-info` | Cabal information | [`member`](screens/cabals__cabal-info__member__light.jpg) ([dark](screens/cabals__cabal-info__member__dark.jpg)) · [`admin`](screens/cabals__cabal-info__admin__light.jpg) · [`saving`](screens/cabals__cabal-info__saving__light.jpg) · [`error`](screens/cabals__cabal-info__error__light.jpg) |
| `members` | Members & voters | [`members`](screens/cabals__members__members__light.jpg) ([dark](screens/cabals__members__members__dark.jpg)) · [`choose-voters`](screens/cabals__members__choose-voters__light.jpg) · [`requests`](screens/cabals__members__requests__light.jpg) · [`empty`](screens/cabals__members__empty__light.jpg) · [`error`](screens/cabals__members__error__light.jpg) |
| `invite` | Share / open invitation | [`share`](screens/cabals__invite__share__light.jpg) ([dark](screens/cabals__invite__share__dark.jpg)) · [`preview`](screens/cabals__invite__preview__light.jpg) · [`invalid`](screens/cabals__invite__invalid__light.jpg) · [`expired`](screens/cabals__invite__expired__light.jpg) · [`pending`](screens/cabals__invite__pending__light.jpg) |
| `chat` | Cabal chat | [`ready`](screens/cabals__chat__ready__light.jpg) ([dark](screens/cabals__chat__ready__dark.jpg)) · [`empty`](screens/cabals__chat__empty__light.jpg) · [`sending`](screens/cabals__chat__sending__light.jpg) · [`failed`](screens/cabals__chat__failed__light.jpg) · [`offline`](screens/cabals__chat__offline__light.jpg) |
| `holding` | Cabal asset holding | [`ready`](screens/cabals__holding__ready__light.jpg) ([dark](screens/cabals__holding__ready__dark.jpg)) · [`loss`](screens/cabals__holding__loss__light.jpg) · [`empty`](screens/cabals__holding__empty__light.jpg) · [`loading`](screens/cabals__holding__loading__light.jpg) · [`stale`](screens/cabals__holding__stale__light.jpg) · [`error`](screens/cabals__holding__error__light.jpg) |

### Money (10) · [contact sheet](contact-sheets/money.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `fund-cabal` | Deposit to cabal | [`ready`](screens/money__fund-cabal__ready__light.jpg) ([dark](screens/money__fund-cabal__ready__dark.jpg)) · [`insufficient`](screens/money__fund-cabal__insufficient__light.jpg) · [`submitting`](screens/money__fund-cabal__submitting__light.jpg) · [`completed`](screens/money__fund-cabal__completed__light.jpg) · [`error`](screens/money__fund-cabal__error__light.jpg) |
| `cash-out` | Cash out your share | [`ready`](screens/money__cash-out__ready__light.jpg) ([dark](screens/money__cash-out__ready__dark.jpg)) · [`insufficient`](screens/money__cash-out__insufficient__light.jpg) · [`review`](screens/money__cash-out__review__light.jpg) · [`processing`](screens/money__cash-out__processing__light.jpg) · [`completed`](screens/money__cash-out__completed__light.jpg) · [`error`](screens/money__cash-out__error__light.jpg) |
| `add-money` | Deposit | [`ready`](screens/money__add-money__ready__light.jpg) ([dark](screens/money__add-money__ready__dark.jpg)) · [`unavailable`](screens/money__add-money__unavailable__light.jpg) · [`error`](screens/money__add-money__error__light.jpg) |
| `card-funding` | Card / Apple Pay | [`handoff`](screens/money__card-funding__handoff__light.jpg) ([dark](screens/money__card-funding__handoff__dark.jpg)) · [`loading`](screens/money__card-funding__loading__light.jpg) · [`returned`](screens/money__card-funding__returned__light.jpg) · [`cancelled`](screens/money__card-funding__cancelled__light.jpg) · [`error`](screens/money__card-funding__error__light.jpg) |
| `network` | Choose a network | [`ready`](screens/money__network__ready__light.jpg) ([dark](screens/money__network__ready__dark.jpg)) · [`selected`](screens/money__network__selected__light.jpg) · [`loading`](screens/money__network__loading__light.jpg) · [`unavailable`](screens/money__network__unavailable__light.jpg) |
| `deposit` | Receive USDC | [`address`](screens/money__deposit__address__light.jpg) ([dark](screens/money__deposit__address__dark.jpg)) · [`copied`](screens/money__deposit__copied__light.jpg) · [`waiting`](screens/money__deposit__waiting__light.jpg) · [`received`](screens/money__deposit__received__light.jpg) · [`error`](screens/money__deposit__error__light.jpg) |
| `withdraw` | Withdraw to wallet | [`ready`](screens/money__withdraw__ready__light.jpg) ([dark](screens/money__withdraw__ready__dark.jpg)) · [`invalid`](screens/money__withdraw__invalid__light.jpg) · [`insufficient`](screens/money__withdraw__insufficient__light.jpg) · [`review`](screens/money__withdraw__review__light.jpg) · [`processing`](screens/money__withdraw__processing__light.jpg) · [`completed`](screens/money__withdraw__completed__light.jpg) · [`error`](screens/money__withdraw__error__light.jpg) |
| `activity` | Account activity | [`ready`](screens/money__activity__ready__light.jpg) ([dark](screens/money__activity__ready__dark.jpg)) · [`pending`](screens/money__activity__pending__light.jpg) · [`empty`](screens/money__activity__empty__light.jpg) · [`loading`](screens/money__activity__loading__light.jpg) · [`error`](screens/money__activity__error__light.jpg) |
| `transaction` | Transaction receipt | [`completed`](screens/money__transaction__completed__light.jpg) ([dark](screens/money__transaction__completed__dark.jpg)) · [`pending`](screens/money__transaction__pending__light.jpg) · [`failed`](screens/money__transaction__failed__light.jpg) · [`unknown`](screens/money__transaction__unknown__light.jpg) · [`error`](screens/money__transaction__error__light.jpg) |
| `group-cash` | Cabal cash | [`ready`](screens/money__group-cash__ready__light.jpg) ([dark](screens/money__group-cash__ready__dark.jpg)) · [`unavailable`](screens/money__group-cash__unavailable__light.jpg) · [`loading`](screens/money__group-cash__loading__light.jpg) · [`error`](screens/money__group-cash__error__light.jpg) |

### Proposals (7) · [contact sheet](contact-sheets/proposals.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `propose` | Propose · action chooser | [`ready`](screens/proposals__propose__ready__light.jpg) ([dark](screens/proposals__propose__ready__dark.jpg)) · [`no-holdings`](screens/proposals__propose__no-holdings__light.jpg) · [`not-member`](screens/proposals__propose__not-member__light.jpg) |
| `select-stock` | Choose stock / holding | [`buy`](screens/proposals__select-stock__buy__light.jpg) ([dark](screens/proposals__select-stock__buy__dark.jpg)) · [`sell`](screens/proposals__select-stock__sell__light.jpg) · [`no-results`](screens/proposals__select-stock__no-results__light.jpg) · [`no-holdings`](screens/proposals__select-stock__no-holdings__light.jpg) · [`loading`](screens/proposals__select-stock__loading__light.jpg) · [`error`](screens/proposals__select-stock__error__light.jpg) |
| `trade-amount` | Proposal amount & thesis | [`ready`](screens/proposals__trade-amount__ready__light.jpg) ([dark](screens/proposals__trade-amount__ready__dark.jpg)) · [`sell`](screens/proposals__trade-amount__sell__light.jpg) · [`insufficient`](screens/proposals__trade-amount__insufficient__light.jpg) · [`empty-thesis`](screens/proposals__trade-amount__empty-thesis__light.jpg) · [`error`](screens/proposals__trade-amount__error__light.jpg) |
| `review-proposal` | Review proposal · sheet | [`ready`](screens/proposals__review-proposal__ready__light.jpg) ([dark](screens/proposals__review-proposal__ready__dark.jpg)) · [`sell`](screens/proposals__review-proposal__sell__light.jpg) · [`long-thesis`](screens/proposals__review-proposal__long-thesis__light.jpg) · [`expanded`](screens/proposals__review-proposal__expanded__light.jpg) · [`submitting`](screens/proposals__review-proposal__submitting__light.jpg) · [`sent`](screens/proposals__review-proposal__sent__light.jpg) · [`error`](screens/proposals__review-proposal__error__light.jpg) |
| `proposals` | All proposals | [`open`](screens/proposals__proposals__open__light.jpg) ([dark](screens/proposals__proposals__open__dark.jpg)) · [`closed`](screens/proposals__proposals__closed__light.jpg) · [`empty`](screens/proposals__proposals__empty__light.jpg) · [`loading`](screens/proposals__proposals__loading__light.jpg) · [`error`](screens/proposals__proposals__error__light.jpg) |
| `proposal` | Proposal & discussion | [`open`](screens/proposals__proposal__open__light.jpg) ([dark](screens/proposals__proposal__open__dark.jpg)) · [`sell`](screens/proposals__proposal__sell__light.jpg) · [`voted`](screens/proposals__proposal__voted__light.jpg) · [`not-voter`](screens/proposals__proposal__not-voter__light.jpg) · [`approved`](screens/proposals__proposal__approved__light.jpg) · [`executed`](screens/proposals__proposal__executed__light.jpg) · [`expired`](screens/proposals__proposal__expired__light.jpg) · [`blocked`](screens/proposals__proposal__blocked__light.jpg) · [`rejected`](screens/proposals__proposal__rejected__light.jpg) · [`error`](screens/proposals__proposal__error__light.jpg) |
| `votes` | Votes · sheet | [`open`](screens/proposals__votes__open__light.jpg) ([dark](screens/proposals__votes__open__dark.jpg)) · [`recorded`](screens/proposals__votes__recorded__light.jpg) · [`closed`](screens/proposals__votes__closed__light.jpg) · [`expired`](screens/proposals__votes__expired__light.jpg) · [`rejected`](screens/proposals__votes__rejected__light.jpg) · [`empty`](screens/proposals__votes__empty__light.jpg) · [`expanded`](screens/proposals__votes__expanded__light.jpg) · [`loading`](screens/proposals__votes__loading__light.jpg) · [`error`](screens/proposals__votes__error__light.jpg) |

### Markets (3) · [contact sheet](contact-sheets/markets.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `markets` | Markets | [`ready`](screens/markets__markets__ready__light.jpg) ([dark](screens/markets__markets__ready__dark.jpg)) · [`watchlist-empty`](screens/markets__markets__watchlist-empty__light.jpg) · [`no-results`](screens/markets__markets__no-results__light.jpg) · [`loading`](screens/markets__markets__loading__light.jpg) · [`stale`](screens/markets__markets__stale__light.jpg) · [`error`](screens/markets__markets__error__light.jpg) |
| `instrument` | Stock · token market | [`ready`](screens/markets__instrument__ready__light.jpg) ([dark](screens/markets__instrument__ready__dark.jpg)) · [`loss`](screens/markets__instrument__loss__light.jpg) · [`no-holders`](screens/markets__instrument__no-holders__light.jpg) · [`no-proposals`](screens/markets__instrument__no-proposals__light.jpg) · [`loading`](screens/markets__instrument__loading__light.jpg) · [`stale`](screens/markets__instrument__stale__light.jpg) · [`unavailable`](screens/markets__instrument__unavailable__light.jpg) |
| `watchlist` | Manage watchlist | [`ready`](screens/markets__watchlist__ready__light.jpg) ([dark](screens/markets__watchlist__ready__dark.jpg)) · [`empty`](screens/markets__watchlist__empty__light.jpg) · [`no-results`](screens/markets__watchlist__no-results__light.jpg) · [`loading`](screens/markets__watchlist__loading__light.jpg) · [`error`](screens/markets__watchlist__error__light.jpg) |

### Feed (2) · [contact sheet](contact-sheets/feed.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `feed` | Feed | [`all`](screens/feed__feed__all__light.jpg) ([dark](screens/feed__feed__all__dark.jpg)) · [`proposals`](screens/feed__feed__proposals__light.jpg) · [`trades`](screens/feed__feed__trades__light.jpg) · [`price-moves`](screens/feed__feed__price-moves__light.jpg) · [`following`](screens/feed__feed__following__light.jpg) · [`pnl-private`](screens/feed__feed__pnl-private__light.jpg) · [`empty`](screens/feed__feed__empty__light.jpg) · [`loading`](screens/feed__feed__loading__light.jpg) · [`load-more-error`](screens/feed__feed__load-more-error__light.jpg) · [`error`](screens/feed__feed__error__light.jpg) |
| `post` | Post & comments | [`ready`](screens/feed__post__ready__light.jpg) ([dark](screens/feed__post__ready__dark.jpg)) · [`empty`](screens/feed__post__empty__light.jpg) · [`replying`](screens/feed__post__replying__light.jpg) · [`sending`](screens/feed__post__sending__light.jpg) · [`failed`](screens/feed__post__failed__light.jpg) · [`unavailable`](screens/feed__post__unavailable__light.jpg) |

### Explore (1) · [contact sheet](contact-sheets/explore.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `leaderboard` | Leaderboard | [`cabals`](screens/explore__leaderboard__cabals__light.jpg) ([dark](screens/explore__leaderboard__cabals__dark.jpg)) · [`people`](screens/explore__leaderboard__people__light.jpg) · [`filtered`](screens/explore__leaderboard__filtered__light.jpg) · [`empty`](screens/explore__leaderboard__empty__light.jpg) · [`loading`](screens/explore__leaderboard__loading__light.jpg) · [`stale`](screens/explore__leaderboard__stale__light.jpg) · [`error`](screens/explore__leaderboard__error__light.jpg) |

### Profile (5) · [contact sheet](contact-sheets/profile.jpg)

| Template | Title | States (default first) |
| --- | --- | --- |
| `profile` | Your profile | [`ready`](screens/profile__profile__ready__light.jpg) ([dark](screens/profile__profile__ready__dark.jpg)) · [`new`](screens/profile__profile__new__light.jpg) · [`loading`](screens/profile__profile__loading__light.jpg) · [`error`](screens/profile__profile__error__light.jpg) |
| `person` | Public user profile | [`public`](screens/profile__person__public__light.jpg) ([dark](screens/profile__person__public__dark.jpg)) · [`following`](screens/profile__person__following__light.jpg) · [`private`](screens/profile__person__private__light.jpg) · [`not-found`](screens/profile__person__not-found__light.jpg) · [`error`](screens/profile__person__error__light.jpg) |
| `people` | Find friends | [`results`](screens/profile__people__results__light.jpg) ([dark](screens/profile__people__results__dark.jpg)) · [`following`](screens/profile__people__following__light.jpg) · [`no-results`](screens/profile__people__no-results__light.jpg) · [`loading`](screens/profile__people__loading__light.jpg) · [`error`](screens/profile__people__error__light.jpg) |
| `edit-profile` | Edit profile | [`ready`](screens/profile__edit-profile__ready__light.jpg) ([dark](screens/profile__edit-profile__ready__dark.jpg)) · [`invalid`](screens/profile__edit-profile__invalid__light.jpg) · [`saving`](screens/profile__edit-profile__saving__light.jpg) · [`saved`](screens/profile__edit-profile__saved__light.jpg) · [`error`](screens/profile__edit-profile__error__light.jpg) |
| `settings` | Settings | [`ready`](screens/profile__settings__ready__light.jpg) ([dark](screens/profile__settings__ready__dark.jpg)) · [`saving`](screens/profile__settings__saving__light.jpg) · [`error`](screens/profile__settings__error__light.jpg) · [`sign-out`](screens/profile__settings__sign-out__light.jpg) |
