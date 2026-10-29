# Push fixtures

Sample `.apns` payloads for the simulator. Push one to this checkout's lane simulator:

```sh
UDID=$(scripts/gold-sim-udid.sh)
xcrun simctl push "$UDID" xyz.monacolabs.app apps/mobile/PushFixtures/vote_reminder.apns
```

Long-press the banner to see the actions. `vote_reminder.apns` (category `VOTE`) shows Yes, No and Read the thesis. `vote_deciding.apns` (category `VOTE_DECIDING`) shows Yes and No. Yes or No opens the hold sheet with that pick made and votes nothing until the hold completes. Read the thesis opens the proposal screen.

The fixtures belong to the backend tickets that define each push; do not edit them here. Never open Simulator.app.
