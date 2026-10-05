import { failureFromProblem, type Failure, type Report, type Session } from "./lib";

export class FundError extends Error {
  constructor(readonly failure: Failure) {
    super(failure);
  }
}

export async function exchangeToken(api: string, token: string): Promise<Session> {
  return send(`${api}/v1/onramp/sessions/exchange`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
}

export async function reportStatus(api: string, sessionId: string, report: Report, accessToken: string): Promise<void> {
  await send(`${api}/v1/onramp/sessions/${encodeURIComponent(sessionId)}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${accessToken}`,
      "Idempotency-Key": report.idempotencyKey,
    },
    body: JSON.stringify({ status: report.status }),
  });
}

async function send<T>(url: string, init: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(url, init);
  } catch {
    throw new FundError("network");
  }
  if (response.ok) return (await response.json()) as T;
  const problem = (await response.json().catch(() => ({}))) as { code?: string };
  throw new FundError(failureFromProblem(response.status, problem.code));
}
