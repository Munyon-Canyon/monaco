# TestFlight releases

`scripts/ios-release.sh` archives the iOS app and uploads it to TestFlight. Run it from the repo root on macOS. There is no `just` recipe.

The build number is `git rev-list --count HEAD`. It grows along one branch. Checkpoints squash into `main`, so `main` and the next feature branch count from a lower number. A number already uploaded is rejected at upload, and the script then stops before tagging. After a successful upload the script tags that commit `ios/<environment>/<n>` and pushes the tag. If that push fails, push the tag by hand. A rerun is rejected at upload because that build number is already used.

## Prerequisites

- Xcode, and a clean tree whose `HEAD` is already on a remote branch. The script refuses a dirty tree and a commit no remote branch contains.
- An App Store Connect API key with the App Manager role. Put these three in `.env.local` with dotenvx (the names are in `.env.example`; the values stay out of git):

  - `ASC_KEY_ID`
  - `ASC_ISSUER_ID`
  - `ASC_KEY_P8_BASE64` (the `.p8` file, base64-encoded)

- An `https` API URL for the environment you are shipping. The script stops before archiving when that URL is empty or not https.

  ```bash
  dotenvx set MONACO_STAGING_API_BASE_URL https://<staging-or-tunnel-host> -f .env.local --plain
  dotenvx set MONACO_PRODUCTION_API_BASE_URL https://<production-host> -f .env.local --plain
  ```

The script decodes the API key into a mode-600 temp file, passes it to `xcodebuild` as `-authenticationKeyPath`, and deletes the file on exit. Signing does not use an Apple ID in Xcode.

## Ship a build

```bash
scripts/ios-release.sh staging
scripts/ios-release.sh production
```

Staging and production of the same commit share one build number (`git rev-list --count HEAD`) and the same bundle id, so the second upload is rejected. Promoting a tested staging commit to production is not supported until that numbering is decided.

The archive is `~/Library/Developer/Xcode/Archives/<YYYY-MM-DD>/Monaco-<environment>-<n>.xcarchive`. Its dSYMs stay on the machine for symbolicating crashes. Before upload, the script checks the archived `Info.plist`: `CFBundleVersion` is the build number, `MONACO_ENVIRONMENT` is `staging` or `production`, and `MONACO_API_BASE_URL` starts with `https://`. A mismatch stops the script before upload.

## Map a build number back to a commit

```bash
git fetch origin tag ios/staging/<n>
git rev-parse ios/staging/<n>
```

Use `ios/production/<n>` for a production build. `<n>` is the TestFlight build number.

## Invite testers

1. App Store Connect → **Monaco** → **TestFlight**
2. Select the uploaded build after processing completes
3. **Internal Testing** group → add team members by Apple ID email
4. Share the TestFlight invite link; testers install via the TestFlight app

The backend at the archive's `MONACO_API_BASE_URL` must be reachable over https. Smoke the installed build with [`docs/how-to/demo-checklist.md`](../../docs/how-to/demo-checklist.md).
