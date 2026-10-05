import { describe, expect, it } from "vitest";
import {
  completeURL,
  errorCopy,
  failureFromProblem,
  fundOptions,
  initialState,
  microsToDollars,
  readToken,
  statusFromError,
  statusFromResult,
  transition,
  view,
  type Session,
  type State,
} from "./lib";

const session: Session = {
  session_id: "01890a5d-ac96-774b-bcce-b302099a8057",
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

  it("maps any other error to failed", () => {
    expect(statusFromError(new Error("Something went wrong setting up checkout. Please try again."))).toBe("failed");
    expect(statusFromError("boom")).toBe("failed");
  });
});

describe("error copy", () => {
  it("maps link codes to the expired copy", () => {
    expect(failureFromProblem(410, "onramp_link_expired")).toBe("link_expired");
    expect(failureFromProblem(404, "onramp_link_invalid")).toBe("link_expired");
    expect(errorCopy.link_expired).toBe("This link has expired. Go back to Monaco and tap Deposit again.");
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
