# Read iOS app logs

Use this when you need to match an app failure to a backend log line, or pull crash and hang data off a device.

- Every API request sends a fresh `X-Request-Id`, so a line in Console.app matches a line in the
  API log.
- `os.Logger` category `api` (subsystem = bundle id): failures at error, requests over 2 s at
  notice, the rest at debug. It records method, route template, status or transport category
  (`offline`, `timeout`, `cancelled`, `tls`, `other`), duration and the ids. Never tokens, bodies,
  query values or wallet addresses.
- API errors expose `apiRequestID` and `apiSupportReference` ("ref: 3f9a1c20") for support.
- MetricKit crash, hang, CPU and disk-write diagnostics are written as JSON to
  `Application Support/Diagnostics`, newest 20 kept. They stay on the device: pull them from a
  device container or a sysdiagnose.

To stream the `api` category from a running simulator:

```sh
xcrun simctl spawn booted log stream --predicate 'subsystem == "com.monaco.app" AND category == "api"'
```

Not built: uploading those diagnostics anywhere. The iOS app has no remote crash reporting yet;
adding the Sentry SDK through Xcode's package manager is the next step.
