# monacolabs.xyz

Waitlist landing page, plus two serverless functions. Vite builds the site into `dist/`.

| Path | What it is |
|---|---|
| `index.html` | The waitlist page. The build copies it to `dist/` unchanged |
| `vite.config.ts` | The build: one HTML input per page, hashed bundles under `/_app/` |
| `lib/waitlist.js` | Signup and health logic, tested in `test/` |
| `functions/api/waitlist.js`, `functions/api/health.js` | Cloudflare Pages adapters |
| `api/waitlist.js`, `api/health.js` | Vercel adapters |
| `public/_headers`, `wrangler.toml` | Cloudflare config |
| `vercel.json` | Vercel config |
| `fund/index.html`, `src/fund/` | The card deposit page at `/fund`. Logic in `src/fund/lib.ts`, tested with Vitest |
| `csp.ts` | The fund page's Content-Security-Policy, written into its HTML at build time |
| `public/assets/video-poster.png` | Poster for the demo video slot |

`api/` and `functions/` are two thin adapters over the same two functions in
`lib/waitlist.js` — `handleSignup()` and `checkHealth()`, both returning
`{ status, body }`. Both hosts are live on purpose so the domain can move without
downtime. Once `monacolabs.xyz` points at Cloudflare, delete `api/`, `vercel.json`
and the `dev` script.

## Run and test

```bash
cd apps/web && npm ci                          # once, and after package.json changes
cd apps/web && npm test                        # both adapters and the shared logic
cd apps/web && npm run build                   # writes dist/
cd apps/web && npx wrangler pages dev dist      # Cloudflare, serves /api/* for real
cd apps/web && npx vercel dev                   # Vercel
```

`wrangler pages dev` reads no secrets on its own; pass them for a local run:

```bash
npx wrangler pages dev --binding SUPABASE_URL=… SUPABASE_ANON_KEY=… IP_HASH_SALT=local ALLOWED_ORIGINS=http://127.0.0.1:8788
```

### Fund page against a local backend

The backend sends the app to `http://localhost:5173/fund` outside staging and production, unless `FUND_PAGE_URL` is set. `just run` does not start that page. Start it with Vite:

```bash
cd apps/web && npm ci && VITE_MONACO_API_URL=http://localhost:8080 VITE_PRIVY_APP_ID=<PRIVY_APP_ID from just show-env> VITE_PRIVY_ENV=sandbox npx vite --port 5173
```

Or copy `.env.example` to `.env.local` and fill in `VITE_PRIVY_APP_ID`. Vite reads it on start.

Without the page, **Pay with card or Apple Pay** opens a URL nothing serves.

CORS needs no setup. `WEB_ALLOWED_ORIGINS` already allows `http://localhost:5173` outside staging and production. If you set it yourself, include that origin.

## Deploy to Cloudflare Pages

Apply the waitlist migrations (`apps/web/migrations/000026`–`000029`, in order) to the hosted
database first. Without them `join_waitlist()` does not exist, `/api/health` reports
`supabase: "waitlist table missing"`, and signups return a 502 rather than storing
anything.

| Setting | Value |
|---|---|
| Project source | Connect to Git, repo `Munyon-Canyon/monaco`, production branch `main` |
| Root directory | `apps/web` |
| Framework preset | None |
| Build command | `npm ci && npm run build` |
| Build output directory | `dist` |

`wrangler.toml` supplies the output directory and the `nodejs_compat` compatibility flag, which the
`node:crypto` import in `lib/waitlist.js` needs. It is checked in so the deploy
does not depend on dashboard state.

| Env var | Value |
|---|---|
| `SUPABASE_URL` | `https://<ref>.supabase.co` for the Monaco project |
| `SUPABASE_ANON_KEY` | The project's anon (publishable) key. Never the service role key |
| `IP_HASH_SALT` | Any long random string. IPs are stored only as salted hashes |
| `ALLOWED_ORIGINS` | Optional. Defaults to monacolabs.xyz and trymonaco.xyz (with and without `www`). Set it to the preview URL on Preview |

The fund page also needs three build-time variables (see `.env.example`). Vite inlines them, so
none is secret:

| Env var | Value |
|---|---|
| `VITE_MONACO_API_URL` | The API base URL. The fund page's CSP allows only this API origin |
| `VITE_PRIVY_APP_ID` | The Privy app id the iOS app uses |
| `VITE_PRIVY_ENV` | `production` on Production, `sandbox` on Preview |

Set all of them on both Production and Preview, then:

1. Workers & Pages → the project → Custom domains → add `monacolabs.xyz` and
   `www.monacolabs.xyz`. Cloudflare writes the DNS records itself if the zone is
   already on the account; otherwise move the nameservers first.
2. Check `https://monacolabs.xyz/api/health` returns `"status": "ok"`.
3. Sign up once and confirm the row lands in `waitlist`.

Notes:

- `public/_headers` replaces `vercel.json`'s headers. Keep the two in sync while both hosts
  are live.
- `cleanUrls` needs no config: Pages serves `/foo` for `foo.html` and redirects
  `/foo.html` to `/foo` by default.
- Only `dist/` is uploaded. `functions/` stays at the project root, where Pages reads
  it, and is not copied into `dist/`.
- Files that must keep their name, such as `_headers` and `assets/`, live in `public/`.
  Vite copies them to `dist/` as they are. Hashed bundles go to `/_app/` so they never
  collide with `/assets/*`.

## Demo video (not on the page yet)

There is deliberately no video section on the page — it goes back when there is a video
to put in it. What matters is that it stays cheap when it does.

**Never commit the video, and never serve it from Pages.** Host it on **Cloudflare R2**,
where egress is free, or as an **unlisted YouTube/Vimeo embed**. A 20MB file served from
a metered host is roughly 100GB after five thousand plays, which is where a free tier
stops being free.

To add it back:

1. A `<section class="video">` with a 16/9 frame, after the hero and before `</main>`.
2. For a file: `<video controls preload="none" poster="/assets/video-poster.png">`.
   `preload="none"` is the whole point — browsers otherwise fetch the first chunk of a
   video on every page load, watched or not.
3. For an embed: show the poster behind a real `<button>` and build the `<iframe>` on
   click, so the third party is not contacted by visitors who never press play.

`public/assets/video-poster.png` is already in the repo for this.

## Data

Signups are in the `waitlist` table. Export with the Supabase table editor or
`select email, twitter, source, created_at from waitlist order by created_at`.
