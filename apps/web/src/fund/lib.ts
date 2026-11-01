export type Session = {
  session_id: string;
  privy_user_id: string;
  wallet_address: string;
  suggested_amount_micros: string | null;
  usdc_mint: string;
};

export type ReportStatus = "confirmed" | "submitted" | "cancelled" | "failed";

export type Report = { status: ReportStatus; idempotencyKey: string };

export type Failure = "link_expired" | "wrong_account" | "network" | "unexpected";

// The steps the page moves through. "open" covers loading, login and ready: which one shows
// depends on Privy's auth state, so view() derives it rather than the reducer storing it.
export type State =
  | { step: "exchanging"; token: string }
  | { step: "open"; session: Session }
  | { step: "funding"; session: Session }
  | { step: "reporting"; session: Session; report: Report }
  | { step: "redirect"; session: Session }
  | { step: "error"; failure: Failure; retry: State | null };

export type Event =
  | { type: "exchanged"; session: Session }
  | { type: "fund" }
  | { type: "funded"; report: Report }
  | { type: "reported" }
  | { type: "failed"; failure: Failure }
  | { type: "retry" };

export type Auth = { ready: boolean; authenticated: boolean };

export type View = "loading" | "exchanging" | "login" | "ready" | "funding" | "reporting" | "redirect" | "error";

export function initialState(search: string, saved: SavedSession | null = null): State {
  const token = readToken(search);
  if (token) return { step: "exchanging", token };
  if (saved) return saved.reported ? { step: "redirect", session: saved.session } : { step: "open", session: saved.session };
  return { step: "error", failure: "link_expired", retry: null };
}

export type SavedSession = { session: Session; reported: boolean; at: number };

const savedKey = "monaco.fund.session";
const savedLifetimeMs = 30 * 60 * 1000;

// The one-time token is spent by the exchange, so a reload would land on "expired". Keeping the
// exchanged session for the tab lets a reload pick the same deposit up again, or go back to the app.
export function saveSession(storage: Pick<Storage, "setItem">, session: Session, reported: boolean, now: number): void {
  try {
    storage.setItem(savedKey, JSON.stringify({ session, reported, at: now } satisfies SavedSession));
  } catch {
    return;
  }
}

export function loadSession(storage: Pick<Storage, "getItem">, now: number): SavedSession | null {
  try {
    const raw = storage.getItem(savedKey);
    if (!raw) return null;
    const saved = JSON.parse(raw) as Partial<SavedSession>;
    const session = saved.session;
    if (!session || typeof session.session_id !== "string" || typeof session.wallet_address !== "string") return null;
    if (typeof saved.at !== "number" || now - saved.at > savedLifetimeMs || saved.at > now) return null;
    return { session, reported: saved.reported === true, at: saved.at };
  } catch {
    return null;
  }
}

export function readToken(search: string): string | null {
  const token = new URLSearchParams(search).get("s");
  return token && token.length <= 128 ? token : null;
}

export function transition(state: State, event: Event): State {
  switch (event.type) {
    case "exchanged":
      return state.step === "exchanging" ? { step: "open", session: event.session } : state;
    case "fund":
      return state.step === "open" ? { step: "funding", session: state.session } : state;
    case "funded":
      return state.step === "funding" ? { step: "reporting", session: state.session, report: event.report } : state;
    case "reported":
      return state.step === "reporting" ? { step: "redirect", session: state.session } : state;
    case "failed":
      return state.step === "error" ? state : { step: "error", failure: event.failure, retry: retryFrom(state, event.failure) };
    case "retry":
      return state.step === "error" && state.retry ? state.retry : state;
  }
}

// A network error repeats the step it interrupted. A report rejected for the wrong account is
// sent again, with the same Idempotency-Key, once the user signs in with the right one.
function retryFrom(state: State, failure: Failure): State | null {
  switch (failure) {
    case "network":
      return state.step === "funding" ? { step: "open", session: state.session } : state;
    case "wrong_account":
      return state;
    case "link_expired":
    case "unexpected":
      return null;
  }
}

// Funding and reporting both need the Privy session, so an open or reporting state waits on it.
export function view(state: State, auth: Auth): View {
  if (state.step !== "open" && state.step !== "reporting") return state.step;
  if (!auth.ready) return "loading";
  if (!auth.authenticated) return "login";
  return state.step === "open" ? "ready" : "reporting";
}

export type AccountInput = { ready: boolean; authenticated: boolean; userId: string | undefined };
export type AccountDecision = "login" | "fund" | "wrong_account" | "wait";

// Safari can still be signed in as someone else, so funding opens only for the session's own Privy user.
export function accountDecision(auth: AccountInput, sessionPrivyUserId: string): AccountDecision {
  if (!auth.ready) return "wait";
  if (!auth.authenticated) return "login";
  return auth.userId === sessionPrivyUserId ? "fund" : "wrong_account";
}

export function statusFromResult(result: { status: "confirmed" | "submitted" }): ReportStatus {
  return result.status;
}

// Privy rejects fund() with a plain Error when the user closes the flow ("User exited flow") or
// backs out of it ("User cancelled funding"). It carries no code, so the message is the signal.
// Privy also rejects when its status polling gives up ("Could not confirm payment status yet.",
// "Unable to check payment status"). The purchase may still complete, so that is submitted, never failed.
export function statusFromError(error: unknown): ReportStatus {
  const message = error instanceof Error ? error.message : String(error);
  if (/exited flow|cancel/i.test(message)) return "cancelled";
  if (/confirm payment status|check payment status/i.test(message)) return "submitted";
  return "failed";
}

export function failureFromProblem(httpStatus: number, code: string | undefined): Failure {
  if (code === "onramp_link_invalid" || code === "onramp_link_expired") return "link_expired";
  if (httpStatus === 403) return "wrong_account";
  if (httpStatus >= 500 || httpStatus === 429) return "network";
  return "unexpected";
}

export const errorCopy: Record<Failure, string> = {
  link_expired: "Check Monaco for your deposit. If it isn't there, tap Deposit again.",
  wrong_account: "Sign in with the account you use in the Monaco app.",
  network: "We couldn't reach Monaco. Check your connection and try again.",
  unexpected: "Something went wrong. Go back to Monaco and tap Deposit again.",
};

// "25000000" micros is "25", "25500000" is "25.50". Privy's defaultAmount is a dollar string.
export function microsToDollars(micros: string | null): string | undefined {
  if (micros === null) return undefined;
  const value = BigInt(micros);
  const dollars = value / 1_000_000n;
  const cents = (value % 1_000_000n) / 10_000n;
  return cents === 0n ? dollars.toString() : `${dollars}.${cents.toString().padStart(2, "0")}`;
}

export function fundOptions(session: Session, environment: "sandbox" | "production") {
  return {
    source: { defaultAsset: "usd" as const },
    destination: { chain: "solana:mainnet" as const, asset: session.usdc_mint, address: session.wallet_address },
    defaultAmount: microsToDollars(session.suggested_amount_micros),
    environment,
  };
}

export function completeURL(sessionId: string): string {
  return `monaco://deposit/complete?session=${encodeURIComponent(sessionId)}`;
}
