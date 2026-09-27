---
name: distribute-stack-changes
description: Plan and execute distribution of dirty changes across a stacked branch series. Use when the user asks to move current changes to the correct branch, split dirty work across a PR stack, restack bottom-up, compare a final branch to a temp reference, or produce copyable instructions for another agent to do that work.
disable-model-invocation: true
---

# Distribute Stack Changes

Use this when dirty changes were made at the top of a branch stack but need to live on the lowest branch that owns each behavior.

## Core Rule

Work is copy-only. Create a temp reference from the current dirty state, copy exact files or hunks from that reference onto the correct branches, restack after every branch commit, and verify the final top branch has zero diff from the temp reference.

Do not reimplement the work in a different way. If zero-diff fails, copy missing exact hunks from the temp reference to the lowest owning branch, commit, restack, and recheck.

## Inputs

Gather:
- Current branch and dirty files: `git status --short --branch`
- Exact dirty diff: `git diff --name-status`, `git diff --stat`, and focused `git diff --unified=0 -- <paths>`
- Stack order: `git log --oneline --decorate --graph --all --branches='*<stack-key>*'`
- File ownership: `git log --oneline --decorate --all -- <paths>` and `git ls-tree -r --name-only <branch> <paths>`

If Graphite is available, `gt ls` can help, but do not depend on it. Plain git evidence is enough.

## Branch Ownership

Deduce ownership from the stack itself. Do not assume branch purpose from generic categories like
"frontend", "backend", "worker", "tests", or package names.

For each file or hunk:
1. Find where the file first appears in the stack.
2. Find which branch introduced the behavior the hunk depends on.
3. Check nearby commits for the same domain terms, test names, route names, service names, or UI names.
4. Choose the lowest branch where the copied hunk can apply without inventing compatibility shims.
5. If a lower branch does not yet have the dependency needed by the hunk, move upward until the dependency exists.

Use evidence from `git log -- <paths>`, `git ls-tree`, `git diff <branch>...<branch> -- <paths>`,
and branch commit messages. The skill's job is to infer ownership from this evidence, not from a
predefined branch taxonomy.

## Temp Reference

Create a temp ref before distributing:

```bash
git switch -c temp/<short-topic>-ref
git add <all dirty files for this task>
git commit -m "temp: <short description>"
```

Use this branch as source of truth for every later copy.

## Bottom-Up Workflow

For each branch, from lowest to highest:

1. `git switch <branch>`
2. Copy exact hunks from the temp ref:
   - For mixed files: `git restore -p --source temp/<short-topic>-ref -- <paths>`
   - For files that should match exactly on that branch: `git restore --source temp/<short-topic>-ref -- <paths>`
   - For moves, prefer `git mv <old> <new>` before restoring content
3. Stage only that branch's owned files
4. Commit locally
5. Run smallest meaningful checks for that branch and make sure everything builds
6. Run `gt restack -u`
7. Move to the next branch

Do not push unless the user explicitly asks.

## Output Format

When asked for instructions for another agent, output one copyable markdown code block:

```markdown
Goal: distribute current dirty changes bottom-up by copying exact hunks from temp ref. Do not reimplement. After final restack, top branch must have 0 diff from temp ref for listed files.

Dirty changes:
- <short behavior summary>

Do not push. Prefer `git restore -p --source temp/...`. Copy exact hunks only.

Create temp ref first:

git switch -c temp/<topic>-ref
git add <paths>
git commit -m "temp: <topic>"

Step 1: <lowest-branch>

Copy exact <behavior> hunks:
- <paths>

Expected:
- <owned outcome>

Run:

git switch <lowest-branch>
git restore -p --source temp/<topic>-ref -- <paths>
git add <paths>
git commit -m "<message>"
<checks, including build/typecheck commands needed to prove everything builds>
gt restack -u

Step 2: <next-branch>
...

Final zero-diff validation:

git switch <top-branch>
git diff --exit-code temp/<topic>-ref <top-branch> -- <paths>

If any diff prints, do not stop. Copy missing exact hunks from temp to lowest owning branch, commit, restack, rerun zero-diff.

Ownership:
- <area> -> <branch>
```

## Verification

Always include:
- Per-branch checks that match touched area
- Build/typecheck verification sufficient to prove every affected workspace still builds
- Final `git diff --exit-code temp/<topic>-ref <top-branch> -- <paths>`
- Instruction to continue until zero diff

Prefer targeted checks over full monorepo checks unless the change crosses many packages.

## Safety

- Do not use destructive git commands.
- Do not use `rm` or `git rm` unless the user explicitly approves.
- Do not commit unrelated dirty files.
- Do not preserve compatibility with intermediate top-branch-only work; distribute it to the right owner branch.
- If a hunk no longer applies after restack, resolve by matching the temp ref behavior exactly, then verify with zero diff at the top.
