# Reply-finder

`monacoctl marketing watch` finds X posts worth a human reply and sends each one to a Telegram channel. It only reads X. A person writes every reply, and no code path posts, likes, follows or messages on X.

## Set up once

You do these steps by hand. The code cannot do them for you.

1. Create an X developer account on pay-per-use billing, add credits and copy the app's bearer token exactly as the portal shows it, with any `%` signs left in.
2. Create a Telegram bot with @BotFather and copy its token. Add the bot to the channel as an admin, and find the channel's chat id (a number that starts with `-100`, or `@channelname`).
3. Put the three keys in the encrypted `.env.local`:

        dotenvx set X_BEARER_TOKEN "<token>" -f .env.local
        dotenvx set TELEGRAM_BOT_TOKEN "<token>" -f .env.local
        dotenvx set TELEGRAM_CHAT_ID "<chat id>" -f .env.local

| Key | Value |
| --- | --- |
| `X_BEARER_TOKEN` | The X app's bearer token. Recent search is the only call made with it. |
| `TELEGRAM_BOT_TOKEN` | The token @BotFather gave the bot. |
| `TELEGRAM_CHAT_ID` | The channel the alerts go to. |

## Run it

Run these from the repo root. `--dry-run` needs no keys. Edit the weights in `.monaco/marketing.toml` and run it again to see what changes.

    go run -C apps/backend ./cmd/monacoctl marketing watch --dry-run
    scripts/with-dotenv-local.sh go run -C apps/backend ./cmd/monacoctl marketing watch --once

| Command | What it does |
| --- | --- |
| `marketing watch --dry-run` | Scores a saved set of search results with the current `.monaco/marketing.toml` and prints the list. It makes no network call and writes no file. |
| `marketing watch --once` | Polls once and exits with 1 if a search or a send failed. Run it from cron every five minutes. |
| `marketing watch` | Polls every five minutes until it gets Ctrl-C or SIGTERM. |

For a long run, build once with `just build backend` and start `scripts/with-dotenv-local.sh bin/monacoctl marketing watch`. Run one watcher per checkout, because the state file has no lock.

## How a post is chosen

| Step | What happens |
| --- | --- |
| Search | One X recent search per `[[rule]]`, with the languages and exclusions added to the query. A rule's first poll looks back one hour. Later polls ask for posts newer than the rule's last one (`since_id`). |
| Drop | Posts already seen, posts older than `score.recency_minutes`, authors under `exclude.min_followers`, and posts that hold only cashtags. |
| Score | Recency, an author inside the follower band, a question mark, no outbound link and the rule's keyword weight, each times its `[score]` weight. |
| Keep | Posts at or above `min_score`, best first, until `daily_cap` alerts have gone out that UTC day. |
| Send | One Telegram message per kept post: handle, follower count, score, the first `[[angle]]` whose keywords appear in the post, and the post URL. |

`--dry-run` shows each saved post with its score and whether it would alert. The saved searches are in `apps/backend/cmd/monacoctl/marketing/testdata/search.json`, one per rule name. A rule with no entry there is named in a `no saved search for:` line.

## Files

| File | Holds |
| --- | --- |
| `.monaco/marketing.toml` | The rules, weights, caps and angles. Tracked. An unknown key fails the load. |
| `.build/marketing/state.json` | Each rule's last post id, the post ids seen in the last 7 days, and the UTC day's alert count and estimated spend in millionths of a dollar. |
| `.build/marketing/alerts.jsonl` | One line per alert: `post_id`, `score`, `angle`, `rules`, `at`. Join it to replies and sign-ups by `post_id`. |
| `.build/marketing/watch.log` | The JSON log lines that also go to stderr. |

## Cost and failures

- X bills per resource it returns: $0.005 a post and $0.010 an author. Its pricing page does not say whether the authors that come back with the posts are billed, so the watcher counts them. `daily_spend_usd` stops polling for the rest of the UTC day once the estimate reaches it, and 5 dollars a day is the plan's $150 a month. X bills the same resource once per UTC day, so reading a post twice costs nothing more, but the estimate still counts it. Compare the first day's usage on the X dashboard with `spent_micros` in `state.json`.
- A search that fails is skipped and tried again on the next poll. When every search fails three polls in a row, one Telegram message says so.
- A Telegram failure stops the poll before any cursor moves, so the next poll reads the posts again. State is saved after every alert, so a restart loses none and repeats at most the one that was in flight.

## Not built yet

The filtered stream, and the reply log that records who replied with which line and what followed.
