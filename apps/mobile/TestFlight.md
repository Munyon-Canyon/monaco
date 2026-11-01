# TestFlight releases

`scripts/ios-release.sh` archives the iOS app and uploads it to TestFlight. Run it from the repo root on macOS. There is no `just` recipe.

The build number is `git rev-list --count HEAD`. It grows along one branch. Checkpoints squash into `main`, so `main` and the next feature branch count from a lower number. A number already uploaded is rejected at upload, and the script then stops before tagging. After a successful upload the script tags that commit `ios/<environment>/<n>` and pushes the tag. If that push fails, push the tag by hand. A rerun is rejected at upload because that build number is already used.

## Prerequisites

- The Apple side, once per app. These are done for `xyz.monacolabs.app`:

  - Team `8Q2V9WHNNT` owns the bundle id `xyz.monacolabs.app` (registered as "Invest with friends"), and the project's `DEVELOPMENT_TEAM` must match it. The App Store Connect API key belongs to the same team.
  - The bundle id has the capabilities `Monaco.entitlements` asks for: Push Notifications, Sign in with Apple and Associated Domains. Without them, automatic signing finds no profile.
  - Two more capabilities are asked for since the notification service extension: Time Sensitive Notifications and Communication Notifications. Add both to the App ID in the developer portal (team `8Q2V9WHNNT`, founder decision Q18) before the first archive that carries them.
  - The app embeds a second target, `MonacoNotificationService` (bundle id `xyz.monacolabs.app.NotificationService`, same team). The archive and `ExportOptions-testflight.plist` need no profile list: automatic signing with `-allowProvisioningUpdates` and the App Store Connect key registers the extension's App ID and profile on the first archive. Its `CFBundleVersion` follows `CURRENT_PROJECT_VERSION`, which `scripts/ios-release.sh` passes for every target, so it matches the app's build number.
  - App Store Connect has an app record for the bundle id ("Monaco - Invest with Friends", app id `6820853661`). The API cannot create one; it is **Apps → + → New App**.
- Xcode, and a clean tree whose `HEAD` is already on a remote branch. The script refuses a dirty tree and a commit no remote branch contains.
- An App Store Connect API key with the App Manager role. Put these three in `.env.staging` with dotenvx. Production releases read them from there too (the names are in `.env.example`; the values stay out of git):

  - `ASC_KEY_ID`
  - `ASC_ISSUER_ID`
  - `ASC_KEY_P8_BASE64` (the `.p8` file, base64-encoded)

- Before #713's first upload, set App Store Connect → **Monaco** → **App Privacy** to the same answers as `apps/mobile/Monaco/PrivacyInfo.xcprivacy`. Linked data, not used for tracking: name (display name), phone number, email address, contacts, photos (avatars and cabal pictures) and other user content (chat, comments and bios) for app functionality; user ID for app functionality and analytics; other financial info (balances, deposits, and trades) for app functionality; product interaction, device ID (PostHog's anonymous device id) and coarse location (PostHog's geo-IP lookup) for analytics. Not linked, not used for tracking: search history (GIPHY search terms) for app functionality. The app does not track and uses no IDFA. Required-reason APIs: UserDefaults (`CA92.1`) and system boot time (`35F9.1`).
- Third-party SDK manifests: Ably is at 1.4.1 or later because 1.4.0 declared Device ID with tracking on, which contradicted `NSPrivacyTracking=false`. Privy ships no manifest and uses `UserDefaults`, which the app's own manifest already declares with `CA92.1`. Open the archive's privacy report (Product → Archive → Generate Privacy Report) and check that it matches this list before the first upload.
- Review Notes: say that a member can delete their account with less than $0.01 left, and that the app is iPhone only.
- The Privy app for the environment you are shipping: `.env.staging` or `.env.production` at the repo root, each with `PRIVY_APP_ID` and `PRIVY_APP_CLIENT_ID` (`client-…`). Staging does not use the dev app from `.env.local`. A missing or incomplete file leaves the build unable to sign in, and the app refuses to launch.
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

## Export compliance

The app only uses standard HTTPS, which is exempt. `apps/mobile/Config/Monaco.xcconfig` sets `ITSAppUsesNonExemptEncryption` to `NO`, so TestFlight asks nothing and the build reaches testers. `scripts/ios-release.sh` refuses an archive without it, because TestFlight would hold that build at `MISSING_EXPORT_COMPLIANCE`.

## Invite testers

1. App Store Connect → **Monaco** → **TestFlight**
2. Select the uploaded build after processing completes
3. **Internal Testing** group → add team members by Apple ID email
4. Share the TestFlight invite link; testers install via the TestFlight app

The backend at the archive's `MONACO_API_BASE_URL` must be reachable over https. Smoke the installed build with [`docs/how-to/demo-checklist.md`](../../docs/how-to/demo-checklist.md).
