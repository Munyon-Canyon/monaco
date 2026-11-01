import { describe, expect, it } from "vitest";
import {
  accountDecision,
  completeURL,
  errorCopy,
  failureFromProblem,
  fundOptions,
  initialState,
  loadSession,
  microsToDollars,
  privyEnvProblem,
  privyReadyTimeoutMs,
  readToken,
  saveSession,
  statusFromError,
  statusFromResult,
  transition,
  view,
  type Session,
  type State,
} from "./lib";

const session: Session = {
  session_id: "01890a5d-ac96-774b-bcce-b302099a8057",
  privy_user_id: "did:privy:cm1abcdefghijklmnopqrstuv",
  wallet_address: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P",
  suggested_amount_micros: "25000000",
  usdc_mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
};
const report = { status: "confirmed" as const, idempotencyKey: "k-1" };
const signedIn = { ready: true, authenticated: true };

describe("query parsing", () => {
  it("reads the token from s", () => {
    expect(readToken("?s=abc123")).toBe("abc123");
    expect(initialState("?s=abc123")).toEqual({ step: "exchanging", token: "abc123" });
  });

  it("treats a missing, empty or oversized token as an expired link", () => {
    for (const search of ["", "?s=", "?token=abc", `?s=${"x".repeat(129)}`]) {
      expect(readToken(search)).toBeNull();
      expect(initialState(search)).toEqual({ step: "error", failure: "link_expired", retry: null });
    }
  });

  it("ignores an address in the URL", () => {
    expect(readToken("?s=abc&address=attacker")).toBe("abc");
  });
});

describe("status mapping", () => {
  it("keeps confirmed and submitted", () => {
    expect(statusFromResult({ status: "confirmed" })).toBe("confirmed");
    expect(statusFromResult({ status: "submitted" })).toBe("submitted");
  });

  it("maps a user exit or cancel to cancelled", () => {
    expect(statusFromError(new Error("User exited flow"))).toBe("cancelled");
    expect(statusFromError(new Error("User cancelled funding"))).toBe("cancelled");
  });

  it("treats Privy giving up on its status polling as submitted, never failed", () => {
    expect(statusFromError(new Error("Could not confirm payment status yet."))).toBe("submitted");
    expect(statusFromError(new Error("Unable to check payment status"))).toBe("submitted");
  });

  it("maps any other error to failed", () => {
    expect(statusFromError(new Error("Something went wrong setting up checkout. Please try again."))).toBe("failed");
    expect(statusFromError("boom")).toBe("failed");
  });
});

describe("reload", () => {
  const memory = () => {
    const items = new Map<string, string>();
    return { getItem: (k: string) => items.get(k) ?? null, setItem: (k: string, v: string) => void items.set(k, v) };
  };

  it("resumes an exchanged session after the token is gone", () => {
    const storage = memory();
    saveSession(storage, session, false, 1_000);
    expect(initialState("", loadSession(storage, 2_000))).toEqual({ step: "open", session });
  });

  it("goes back to the app when the status was already reported", () => {
    const storage = memory();
    saveSession(storage, session, true, 1_000);
    expect(initialState("", loadSession(storage, 2_000))).toEqual({ step: "redirect", session });
  });

  it("prefers a fresh token over a saved session", () => {
    const storage = memory();
    saveSession(storage, session, true, 1_000);
    expect(initialState("?s=abc", loadSession(storage, 2_000))).toEqual({ step: "exchanging", token: "abc" });
  });

  it("forgets a stale, future or broken entry and falls back to the neutral copy", () => {
    const storage = memory();
    saveSession(storage, session, false, 1_000);
    expect(loadSession(storage, 1_000 + 31 * 60 * 1000)).toBeNull();
    expect(loadSession(storage, 500)).toBeNull();
    storage.setItem("monaco.fund.session", "{not json");
    expect(loadSession(storage, 2_000)).toBeNull();
    expect(initialState("", null)).toEqual({ step: "error", failure: "link_expired", retry: null });
    expect(errorCopy.link_expired).not.toMatch(/expired|failed|didn't go through/i);
  });

  it("survives storage that throws", () => {
    const broken = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    };
    expect(() => saveSession(broken, session, false, 1)).not.toThrow();
    expect(loadSession(broken, 2)).toBeNull();
  });
});

describe("error copy", () => {
  it("maps link codes to the expired copy", () => {
    expect(failureFromProblem(410, "onramp_link_expired")).toBe("link_expired");
    expect(failureFromProblem(404, "onramp_link_invalid")).toBe("link_expired");
    expect(errorCopy.link_expired).toBe("Check Monaco for your deposit. If it isn't there, tap Deposit again.");
  });

  it("maps 403 to the wrong-account copy", () => {
    expect(failureFromProblem(403, "forbidden")).toBe("wrong_account");
    expect(errorCopy.wrong_account).toBe("Sign in with the account you use in the Monaco app.");
  });

  it("treats a server or rate-limit error as retryable and anything else as unexpected", () => {
    expect(failureFromProblem(503, "unavailable")).toBe("network");
    expect(failureFromProblem(429, "rate_limited")).toBe("network");
    expect(failureFromProblem(409, "onramp_invalid_transition")).toBe("unexpected");
  });

  it("never says xStock and says deposit where it names the action", () => {
    for (const copy of Object.values(errorCopy)) expect(copy).not.toMatch(/xstock/i);
    expect(errorCopy.link_expired).toContain("Deposit");
  });
});

describe("fund options", () => {
  it("sends USDC on Solana mainnet to the exchanged wallet with the suggested dollars", () => {
    expect(fundOptions(session, "sandbox")).toEqual({
      source: { defaultAsset: "usd" },
      destination: { chain: "solana:mainnet", asset: session.usdc_mint, address: session.wallet_address },
      defaultAmount: "25",
      environment: "sandbox",
    });
  });

  it("formats micros as dollars and leaves no amount when none was suggested", () => {
    expect(microsToDollars("25500000")).toBe("25.50");
    expect(microsToDollars("1050000")).toBe("1.05");
    expect(microsToDollars("1")).toBe("0");
    expect(microsToDollars(null)).toBeUndefined();
  });

  it("builds the app's return URL", () => {
    expect(completeURL(session.session_id)).toBe(`monaco://deposit/complete?session=${session.session_id}`);
  });
});

describe("state machine", () => {
  const run = (state: State, ...events: Parameters<typeof transition>[1][]) => events.reduce(transition, state);

  it("walks the happy path to redirect", () => {
    const end = run(
      initialState("?s=t"),
      { type: "exchanged", session },
      { type: "fund" },
      { type: "funded", report },
      { type: "reported" },
    );
    expect(end).toEqual({ step: "redirect", session });
  });

  it("shows loading, then login, then ready for an open session", () => {
    const open: State = { step: "open", session };
    expect(view(open, { ready: false, authenticated: false })).toBe("loading");
    expect(view(open, { ready: true, authenticated: false })).toBe("login");
    expect(view(open, signedIn)).toBe("ready");
  });

  it("retries a network error from the step it interrupted, reusing the report's key", () => {
    const reporting: State = { step: "reporting", session, report };
    const failed = transition(reporting, { type: "failed", failure: "network" });
    expect(view(failed, signedIn)).toBe("error");
    expect(transition(failed, { type: "retry" })).toEqual(reporting);

    const funding = transition(transition({ step: "open", session }, { type: "fund" }), { type: "failed", failure: "network" });
    expect(transition(funding, { type: "retry" })).toEqual({ step: "open", session });
  });

  it("re-sends a report rejected for the wrong account once the user signs in again", () => {
    const reporting: State = { step: "reporting", session, report };
    const back = transition(transition(reporting, { type: "failed", failure: "wrong_account" }), { type: "retry" });
    expect(back).toEqual(reporting);
    expect(view(back, { ready: true, authenticated: false })).toBe("login");
    expect(view(back, signedIn)).toBe("reporting");
  });

  it("returns to open after a wrong account is caught before funding, then funds once signed in", () => {
    const open: State = { step: "open", session };
    const back = transition(transition(open, { type: "failed", failure: "wrong_account" }), { type: "retry" });
    expect(back).toEqual(open);
    expect(view(back, { ready: true, authenticated: false })).toBe("login");
    expect(view(back, signedIn)).toBe("ready");
  });

  it("does not retry an expired link", () => {
    const failed = transition({ step: "exchanging", token: "t" }, { type: "failed", failure: "link_expired" });
    expect(transition(failed, { type: "retry" })).toBe(failed);
  });

  it("ignores events that do not fit the current step", () => {
    const exchanging: State = { step: "exchanging", token: "t" };
    expect(transition(exchanging, { type: "fund" })).toBe(exchanging);
    expect(transition(exchanging, { type: "reported" })).toBe(exchanging);
  });
});

describe("account decision", () => {
  const own = session.privy_user_id;
  const signedInAs = (userId: string | undefined) => ({ ready: true, authenticated: true, userId });

  it("never funds a different signed-in account", () => {
    expect(accountDecision(signedInAs("did:privy:cm1someoneelse"), own)).toBe("wrong_account");
    expect(accountDecision(signedInAs(undefined), own)).toBe("wrong_account");
  });

  it("funds the session's own account, and only once", () => {
    const open: State = { step: "open", session };
    expect(view(open, signedIn)).toBe("ready");
    expect(accountDecision(signedInAs(own), own)).toBe("fund");
    const funding = transition(open, { type: "fund" });
    expect(view(funding, signedIn)).toBe("funding");
  });

  it("waits for Privy and asks for a login before it decides", () => {
    expect(accountDecision({ ready: false, authenticated: false, userId: undefined }, own)).toBe("wait");
    expect(accountDecision({ ready: false, authenticated: true, userId: own }, own)).toBe("wait");
    expect(accountDecision({ ready: true, authenticated: false, userId: undefined }, own)).toBe("login");
  });
});

describe("privy readiness", () => {
  it("gives up after 20 seconds with copy that sends the member back to Monaco", () => {
    expect(privyReadyTimeoutMs).toBe(20_000);
    const open: State = { step: "open", session };
    const failed = transition(open, { type: "failed", failure: "unavailable" });
    expect(view(failed, { ready: false, authenticated: false })).toBe("error");
    expect(errorCopy.unavailable).toBe("Couldn't load the payment page. Go back to Monaco and try again.");
    expect(transition(failed, { type: "retry" })).toBe(failed);
  });
});

describe("privy environment guard", () => {
  it("accepts production and sandbox", () => {
    expect(privyEnvProblem("production", true)).toBeNull();
    expect(privyEnvProblem("sandbox", true)).toBeNull();
  });

  it("refuses a host build with the value missing or empty", () => {
    expect(privyEnvProblem(undefined, true)).toMatch(/not set/);
    expect(privyEnvProblem("", true)).toMatch(/not set/);
  });

  it("refuses a misspelled value everywhere", () => {
    for (const onHost of [true, false]) expect(privyEnvProblem("prod", onHost)).toMatch(/"prod"/);
    expect(privyEnvProblem("Production", false)).toMatch(/"Production"/);
  });

  it("lets a local or CI build leave it unset", () => {
    expect(privyEnvProblem(undefined, false)).toBeNull();
    expect(privyEnvProblem("", false)).toBeNull();
  });
});
