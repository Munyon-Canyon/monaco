# Connect an agent to a cabal

A cabal votes in a trading agent with a USDC budget. The app hands back connect instructions with the agent's key. The agent trades by sending intents to Monaco, using the key alone. Monaco executes each trade from the cabal treasury, so the agent never holds the money.

For a walkthrough with no real money, point `MONACO_API` at a local or staging backend, or use the bot's default dry run.

## In the app

1. Open a cabal you're a member of and tap **Propose**.
2. Tap **Add a trading bot**.
3. Enter a name and a budget (the USDC the agent may spend from the pot), then tap **Send to cabal**.
4. Once the cabal votes yes, open **Group → Agent**.
5. Tap **Copy connect instructions**. The text holds the key, the API base URL and a link to `/v1/agent/skill.md`. **Copy key** copies the key alone, for a bot that reads it from `MONACO_AGENT_KEY`.
6. To manage the agent later, use the same **Propose** sheet: **Pause the trading bot**, **Turn the trading bot back on**, or **Remove the trading bot**. Each is a cabal vote.

Never log the key. Any cabal member can copy it again until the agent is removed.

## Connect a ClawPump agent

ClawPump runs the agent. Monaco holds the money and executes the trades. Monaco never stores a ClawPump key; connecting is copy and paste.

1. In ClawPump, create an agent.
2. Add a custom skill. Paste the connect instructions you copied from **Group → Agent → Copy connect instructions**.
3. Add an automation that runs the skill on a schedule. For example, hourly: "Check `/v1/agent` and decide trades."
4. Turn off ClawPump's own trading skill. Otherwise the agent may trade from its own ClawPump wallet instead of through Monaco.

On each run the agent reads `/v1/agent/skill.md`, calls `GET /v1/agent` for its budget, cash and holdings, reads prices from `GET /v1/agent/assets`, and trades with `POST /v1/agent/intents`. Every trade shows in the cabal's activity with the agent's `reason`.

The same text works as the system prompt of any other LLM agent.

The API base in the connect text is the server's `PUBLIC_API_BASE_URL`. A ClawPump agent runs on ClawPump's servers, so that URL must be reachable from the internet. `http://127.0.0.1:8080` only works for an agent on your own machine.

## One intent from the terminal

```bash
export MONACO_AGENT_KEY=<the key from Group → Agent>
export MONACO_API=http://127.0.0.1:8080   # the default

# See the request without sending it
scripts/demo/agent-intent.sh buy AAPLx 1 --dry-run

# Send it (against a local or dev backend)
REASON="demo buy" scripts/demo/agent-intent.sh buy AAPLx 1
scripts/demo/agent-intent.sh sell AAPLx 0.25

# Resend after a timeout: same body, same key, no second trade
IDEMPOTENCY_KEY=<key the first run printed> scripts/demo/agent-intent.sh buy AAPLx 1
```

A buy amount is USD, sent as `usd`. A sell amount is shares, sent as `shares`. The script adds a fresh `idempotencyKey`, prints the request with the key redacted, and prints the response.

## What judges see

- **A vote, not a form.** Adding an agent is a cabal proposal like any buy or sell. Same quorum, same "the cabal decides" model, extended to an autonomous trader.
- **Paste the key and it trades.** The connect instructions are all an agent needs. No group id, no wallet, no custody.
- **The key stays with the cabal.** Only cabal members can copy it, and it is never emailed. Agents authenticate against a SHA-256 hash. The server also keeps the plaintext so members can retrieve it, and wipes both when the cabal votes the agent out.
- **The budget is enforced server-side.** A trade past the budget comes back rejected, not silently capped. Intents are decided one at a time per agent and reserve their amount before the swap, so parallel intents cannot get past the budget. Sells refill it.
- **The agent can only sell what it bought.** Stocks the cabal voted in are out of its reach.
- **Every trade says why.** The agent's `reason` is stored with each intent.
- **The key cannot be guessed.** It is `monaco_ak_` plus 32 random characters, about 158 bits. After 10 wrong keys from one address, the API answers that address `429` and allows one more try a minute. Each key may also send at most 30 intents an hour.
- **The cabal keeps control.** Pause, resume and revoke are each a vote. Pausing keeps the key but blocks trades. Revoking kills the key.
- **Same execution path as a member's vote.** Agent trades settle through the same Jupiter and Privy treasury flow a member-approved buy uses, and land in the same activity feed.
