# Cabals log

Dated record of changes to [cabals.md](../cabals.md). Add one line per change, newest last.

- 2026-09-27: Created. Rules, roles, join and request, invite code and direct invite, edit, leave guards, treasury wallet creation and events decided (#525).
- 2026-10-03: A member's `JoinCabal` or `RequestAccess` is refused with `AlreadyMember` instead of returning the membership, as #573 specifies. Added `GET /v1/invite-codes/{code}`, which resolves a pasted invite code to its cabal.
- 2026-10-03: A repeated `RequestAccess` is refused with `RequestPending` instead of returning the existing request. `DecideAccess` refuses approvals into a banned cabal. Membership hints and the SSE rescope are recorded (#573).
- 2026-10-03: `LeaveCabal` re-checks the member count and role under a lock on the member rows. The `cabal.member_left` hints rescope the leaver's SSE connections (#605).
- 2026-10-03: The invitee's inbox and a cabal's invite list are built, and the inbox reads a partial index on `user_id` (#604).
- 2026-10-03: `InviteMember` is built at `POST /v1/cabals/{id}/invites`. Invites hint `user.<invitee>.cabal_invites` instead of the creator's request list (#604).
- 2026-10-03: The `cabal.invite_expiry` poller is built, and a decision on a past-due invite expires it before refusing with `InviteExpired` (#604).
- 2026-10-08: Removed invite codes (#3785). `cabals.invite_code` is dropped. `Cabal.invite_code` is deprecated and always null, and `GET /v1/invite-codes/{code}` is deprecated and always refuses with `cabal_not_found`, so no old code can bypass `join_mode` and old clients keep decoding.
- 2026-10-08: New cabals default to `open` when `join_mode` is omitted at `CreateCabal`; existing cabals keep their policy (#4112).
- 2026-10-28: Added the `admin` role (#6424). Admins list and decide access requests, invite and revoke any invite, and always vote. The three door permissions refuse with `NotCabalAdmin`, and `Cabal.IsAdmin` is true for the creator and for `admin` rows.
- 2026-10-28: `GET /v1/cabals/{id}` names the admins, creator first, in `admins`, and the `cabal.access_requested` push for a request goes to each admin instead of the creator alone (#6424).
- 2026-10-29: A passed rule proposal now changes a cabal rule through `ApplyRuleChange`, and `PATCH /v1/cabals/{id}` refuses rule fields with `RulesChangeByProposal` (#6510).
