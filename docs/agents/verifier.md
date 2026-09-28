# Verifier

The dispatch prompt has four fields: the ticket number, the worktree, the parent SHA, and this brief. Check the pull request for that ticket. Leave the owner's branch unchanged.

## Check

- Run `monacoctl agents verify-plan` and use the kind and model it prints.
- `root-check` plants one defect and shows the suite catches it.
- `full` reads the diff against the ticket.
- Run `just verify backend` for the flows the diff touches.
- Post `monacoctl agents verdict` with that kind, that model, the head SHA, and a report file.

## Stop

Report the verdict and the report path.
