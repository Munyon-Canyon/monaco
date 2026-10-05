import { useEffect, useReducer, useRef } from "react";
import { useFiatOnramp, useLogout, usePrivy } from "@privy-io/react-auth";
import { exchangeToken, FundError, reportStatus } from "./api";
import { completeURL, errorCopy, fundOptions, statusFromError, statusFromResult, transition, view, type State } from "./lib";

const api = import.meta.env.VITE_MONACO_API_URL;
const environment = import.meta.env.VITE_PRIVY_ENV === "production" ? "production" : "sandbox";

function failureOf(error: unknown) {
  return error instanceof FundError ? error.failure : "unexpected";
}

export function App({ initial }: { initial: State }) {
  const [state, dispatch] = useReducer(transition, initial);
  const { ready, authenticated, login, getAccessToken } = usePrivy();
  const { logout } = useLogout();
  const { fund } = useFiatOnramp();
  const step = view(state, { ready, authenticated });
  // Each (state, step) pair starts its side effect once, including under StrictMode's double effects.
  const started = useRef<{ state: State; step: string } | null>(null);

  useEffect(() => {
    if (started.current?.state === state && started.current.step === step) return;
    switch (step) {
      case "exchanging":
        if (state.step !== "exchanging") return;
        started.current = { state, step };
        exchangeToken(api, state.token).then(
          (session) => dispatch({ type: "exchanged", session }),
          (error) => dispatch({ type: "failed", failure: failureOf(error) }),
        );
        return;
      case "login":
        started.current = { state, step };
        login({ loginMethods: ["sms", "email"] });
        return;
      case "ready":
        if (state.step !== "open") return;
        started.current = { state, step };
        dispatch({ type: "fund" });
        fund(fundOptions(state.session, environment)).then(
          (result) => dispatch({ type: "funded", report: { status: statusFromResult(result), idempotencyKey: crypto.randomUUID() } }),
          (error) => dispatch({ type: "funded", report: { status: statusFromError(error), idempotencyKey: crypto.randomUUID() } }),
        );
        return;
      case "reporting":
        if (state.step !== "reporting") return;
        started.current = { state, step };
        getAccessToken()
          .then((token) => {
            if (!token) throw new FundError("wrong_account");
            return reportStatus(api, state.session.session_id, state.report, token);
          })
          .then(
            () => dispatch({ type: "reported" }),
            (error) => dispatch({ type: "failed", failure: failureOf(error) }),
          );
        return;
      case "redirect":
        if (state.step !== "redirect") return;
        location.replace(completeURL(state.session.session_id));
        return;
    }
  }, [state, step]);

  return (
    <main>
      <img src="/assets/mark-ink.svg" alt="" width={40} height={40} />
      <Screen state={state} step={step} login={() => login({ loginMethods: ["sms", "email"] })} retry={() => dispatch({ type: "retry" })} logout={() => logout().then(() => dispatch({ type: "retry" }))} />
    </main>
  );
}

function Screen(props: { state: State; step: ReturnType<typeof view>; login: () => void; retry: () => void; logout: () => void }) {
  const { state, step } = props;
  switch (step) {
    case "loading":
    case "exchanging":
      return <p role="status">Getting your deposit ready…</p>;
    case "login":
      return (
        <>
          <h1>Sign in to deposit</h1>
          <p>Use the phone number or email you sign in to Monaco with.</p>
          <button onClick={props.login}>Sign in</button>
        </>
      );
    case "ready":
    case "funding":
      return <p role="status">Opening the deposit…</p>;
    case "reporting":
    case "redirect":
      return <p role="status">Taking you back to Monaco…</p>;
    case "error": {
      if (state.step !== "error") return null;
      return (
        <>
          <p role="alert">{errorCopy[state.failure]}</p>
          {state.failure === "wrong_account" && <button onClick={props.logout}>Log out</button>}
          {state.failure === "network" && state.retry && <button onClick={props.retry}>Try again</button>}
        </>
      );
    }
  }
}
