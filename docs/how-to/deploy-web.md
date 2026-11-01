# Deploy web

The web app in `apps/web` (the card deposit page at `/fund`, the invite landing at `/r/<code>` and the waitlist) runs on Vercel. Nothing deploys it automatically: `apps/web/vercel.json` sets `git.deploymentEnabled` to `false`, so a commit landing on `staging` or `main` starts no Vercel deployment. Someone deploys by hand once `staging` is green, the way the backend and the iOS app ship.

Why: with auto deploys on, every commit that landed on `staging` built both projects. `monaco-staging` succeeded, but the production project `monaco` failed on each one and flooded the founder with failed-deploy alerts (about 70 commits on 11-01).

| Project | Serves | Production branch |
| --- | --- | --- |
| `monaco-staging` | `https://monaco-staging.vercel.app`, used by staging and TestFlight builds | `staging` |
| `monaco` | `monacolabs.xyz`, used by production | `main` |

Both live in the `munyon-canyon` Vercel team, with root directory `apps/web`. Their settings and env vars are in [apps/web/README.md](https://github.com/Munyon-Canyon/monaco/blob/main/apps/web/README.md#fund-page-for-staging).

## Deploy

1. Start from a clean checkout at a green `staging` commit (`ci / ci-ok` passed on it). For production, use the `main` commit that the promotion PR produced.

        git status --short        # prints nothing
        git rev-parse HEAD        # the sha you are deploying

2. Link the checkout to the project you are deploying. The link is the git-ignored `apps/web/.vercel/` folder and holds one project at a time.

        npx vercel@65.9.0 link --yes --cwd apps/web --project monaco-staging --scope munyon-canyon

3. Deploy staging first, from the repo root.

        npx vercel@65.9.0 deploy --prod --cwd apps/web --scope munyon-canyon

    `--prod` is needed because the project's production branch is `staging`; a deploy without it is a preview behind Vercel Authentication, which phones cannot open.

4. Check staging: `curl -sS -o /dev/null -w '%{http_code}\n' https://monaco-staging.vercel.app/fund` prints `200`, and the `Web deploy check` workflow on that deployment is green (see below).

5. Only after the staging check passes, deploy production from the `main` checkout: link `--project monaco`, run the same `deploy --prod` command, and check `https://monacolabs.xyz/fund`.

Bump the pinned CLI version in this page on purpose, not by dropping the version.

## What runs on a manual deploy

`.github/workflows/web-deploy-check.yml` still runs on each deployment's `deployment_status` event, manual ones included. It curls `/fund` on the new deployment and fails unless it answers 200 (a protected deployment that answers 401 is skipped). A red run means the fund page is missing or broken on that deployment: card deposits would open a 404 on the phone, so fix it and deploy again.

## Check that nothing deploys by itself

After a commit lands on `staging`, no new `monaco` or `monaco-staging` deployment appears for its sha:

    gh api 'repos/Munyon-Canyon/monaco/deployments?per_page=5'

If one does, check that `apps/web/vercel.json` still has `"git": { "deploymentEnabled": false }` and that no branch overrides it in the project's Settings, Git.

Each project may still post a `Vercel – <project>` status on a pushed commit while its Git connection is on. That status is not a deployment, and the dashboard's Git connection is where to switch it off.
