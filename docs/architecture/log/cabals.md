# Cabals log

Dated record of changes to [cabals.md](../cabals.md). Add one line per change, newest last.

- 2026-09-27: Created. Rules, roles, join and request, invite code and direct invite, edit, leave guards, treasury wallet creation and events decided (#525).
- 2026-10-03: A member's `JoinCabal` or `RequestAccess` is refused with `AlreadyMember` instead of returning the membership, as #573 specifies. Added `GET /v1/invite-codes/{code}`, which resolves a pasted invite code to its cabal.
- 2026-10-03: A repeated `RequestAccess` is refused with `RequestPending` instead of returning the existing request. `DecideAccess` refuses approvals into a banned cabal. Membership hints and the SSE rescope are recorded (#573).
