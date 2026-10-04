#!/usr/bin/env python3
"""Exit 2 blocks the command and shows stderr to the agent."""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import sys
from dataclasses import dataclass
from functools import lru_cache

# PRs land in TRUNK through the Graphite merge queue, which takes a PR when it carries QUEUE_LABEL.
TRUNK = "staging"
QUEUE_LABEL = "merge-queue"
# fast-track jumps the Graphite merge queue. Only a human adds it.
FAST_TRACK_LABEL = "fast-track"
CONVENTIONAL_TYPES = "feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert"
CONVENTIONAL_RE = re.compile(rf"^({CONVENTIONAL_TYPES})(\([^()\s]+\))?!?: \S")
HEREDOC_SUBST_RE = re.compile(r"^\$\(\s*cat\s*<<-?\s*(['\"]?)(\w+)\1[ \t]*\n(.*?)\n\s*\2\s*\)\s*$", re.S)
HEREDOC_OP_RE = re.compile(r"<<(-?)[ \t]*(['\"]?)([^\s'\";&|<>()]+)\2")
WRAPPERS = {"nohup", "command", "exec", "time", "builtin"}
SHELL_KEYWORDS = {"do", "then", "else", "elif", "if", "while", "until", "!", "{"}
SHELLS = {"bash", "sh", "zsh"}
LOOP_RE = re.compile(r"(^|[\s;&|(])(while|until|for)\s")
SLEEP_RE = re.compile(r"(\d+(?:\.\d*)?|\.\d+)([smhd]?)")
SLEEP_UNITS = {"": 1, "s": 1, "m": 60, "h": 3600, "d": 86400}
MAX_SLEEP_SECONDS = 10
PR_BODY_USAGE = "scripts/pr-body.sh <pr> <title> <body-file>"
READY_FLOW = (f"Open PRs with `gt submit --stack --no-interactive --draft`, then run `{PR_BODY_USAGE}` for each: "
              "it runs the full PR format check locally and only then sets the title and body and marks the PR ready.")


@dataclass
class Invocation:
    argv: list[str]
    cwd: str
    under_timeout: bool = False
    stdin: str | None = None
    in_loop: bool = False


def strip_heredocs(src: str) -> tuple[str, list[str]]:
    out, bodies, pending = [], [], []
    i, n, quote = 0, len(src), None
    at_word_start = True
    while i < n:
        c = src[i]
        if quote == "'":
            out.append(c)
            quote = None if c == "'" else quote
            i += 1
            continue
        if quote == '"':
            out.append(c)
            if c == "\\" and i + 1 < n:
                out.append(src[i + 1])
                i += 2
                continue
            quote = None if c == '"' else quote
            i += 1
            continue
        if c == "\\" and i + 1 < n:
            if src[i + 1] != "\n":
                out.append(src[i : i + 2])
            i += 2
            at_word_start = False
            continue
        if c in "'\"":
            quote = c
            out.append(c)
            i += 1
            at_word_start = False
            continue
        if c == "#" and at_word_start:
            while i < n and src[i] != "\n":
                i += 1
            continue
        if src.startswith("<<", i) and not src.startswith("<<<", i):
            m = HEREDOC_OP_RE.match(src, i)
            if m:
                pending.append((m.group(3), m.group(1) == "-", len(bodies)))
                bodies.append("")
                out.append(f" @@HEREDOC{len(bodies) - 1}@@ ")
                i = m.end()
                continue
        if c == "\n":
            out.append(" ; ")
            i += 1
            at_word_start = True
            for delim, strip_tabs, idx in pending:
                lines = []
                while i < n:
                    end = src.find("\n", i)
                    end = n if end < 0 else end
                    line = src[i:end]
                    i = end + 1
                    if (line.lstrip("\t") if strip_tabs else line) == delim:
                        break
                    lines.append(line)
                bodies[idx] = "\n".join(lines)
            pending = []
            continue
        at_word_start = c in " \t;&|()"
        out.append(c)
        i += 1
    return "".join(out), bodies


def tokenize(src: str) -> list[list[str]]:
    lexer = shlex.shlex(src, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    commands, current, skip_next = [], [], False
    for tok in lexer:
        if skip_next:
            skip_next = False
            continue
        if tok and all(ch in "();<>|&" for ch in tok):
            if "<" in tok or ">" in tok:
                if current and current[-1].isdigit():
                    current.pop()
                skip_next = True
                continue
            if current:
                commands.append(current)
            current = []
            continue
        current.append(tok)
    if current:
        commands.append(current)
    return commands


def unwrap(argv: list[str]) -> tuple[list[str], bool]:
    under_timeout = False
    while argv:
        head = os.path.basename(argv[0])
        if re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", argv[0]):
            argv = argv[1:]
        elif head in WRAPPERS or head in SHELL_KEYWORDS or head == "heavy.sh":
            argv = argv[1:]
        elif head in {"timeout", "gtimeout"}:
            under_timeout = True
            rest = argv[1:]
            while rest and rest[0].startswith("-"):
                rest = rest[2:] if rest[0] in {"-s", "-k", "--signal", "--kill-after"} else rest[1:]
            argv = rest[1:]
        elif head == "nice":
            rest = argv[1:]
            if rest and rest[0] == "-n":
                rest = rest[2:]
            elif rest and re.match(r"^-(n)?-?\d+$|^--adjustment=", rest[0]):
                rest = rest[1:]
            argv = rest
        elif head == "env":
            rest = argv[1:]
            while rest and (rest[0].startswith("-") or "=" in rest[0]):
                rest = rest[2:] if rest[0] in {"-u", "-C", "-S"} else rest[1:]
            argv = rest
        else:
            break
    return argv, under_timeout


def parse(src: str, cwd: str) -> list[Invocation]:
    flat, bodies = strip_heredocs(src)
    in_loop = bool(LOOP_RE.search(flat))
    parsed: list[Invocation] = []
    for raw in tokenize(flat):
        stdin = None
        argv = []
        for tok in raw:
            m = re.fullmatch(r"@@HEREDOC(\d+)@@", tok)
            if m:
                stdin = bodies[int(m.group(1))]
            else:
                argv.append(tok)
        argv, under_timeout = unwrap(argv)
        if not argv:
            continue
        head = os.path.basename(argv[0])
        if head == "cd":
            if len(argv) > 1 and argv[1] != "-":
                cwd = os.path.normpath(os.path.join(cwd, os.path.expanduser(os.path.expandvars(argv[1]))))
            continue
        if head in SHELLS and "-c" in argv[1:-1]:
            inner = parse(argv[argv.index("-c") + 1], cwd)
            for inv in inner:
                inv.under_timeout = inv.under_timeout or under_timeout
                inv.in_loop = inv.in_loop or in_loop
            parsed.extend(inner)
            continue
        looped = in_loop
        if head == "watch" and "gh" in argv:
            argv, looped = argv[argv.index("gh") :], True
        parsed.append(Invocation(argv, cwd, under_timeout, stdin, looped))
    return parsed


def run(args: list[str], cwd: str, timeout: int = 20) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=timeout)


@dataclass
class Git:
    cwd: str
    sub: str
    args: list[str]


def as_git(inv: Invocation) -> Git | None:
    if os.path.basename(inv.argv[0]) != "git":
        return None
    cwd, rest = inv.cwd, inv.argv[1:]
    while rest and rest[0].startswith("-"):
        if rest[0] == "-C" and len(rest) > 1:
            cwd = os.path.normpath(os.path.join(cwd, os.path.expanduser(rest[1])))
            rest = rest[2:]
        elif rest[0] in {"-c", "--git-dir", "--work-tree", "--namespace"}:
            rest = rest[2:]
        else:
            rest = rest[1:]
    if not rest:
        return None
    return Git(cwd, rest[0], rest[1:])


def short_cluster(arg: str) -> str:
    return arg[1:] if re.fullmatch(r"-[A-Za-z]+", arg) else ""


@dataclass
class Push:
    remote: str
    targets: list[tuple[str, str, bool]]
    force: bool
    blanket_lease: bool
    explicit_leases: set[str]
    everything: bool


def current_branch(cwd: str) -> str:
    try:
        return run(["git", "symbolic-ref", "--quiet", "--short", "HEAD"], cwd).stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return ""


def parse_push(git: Git) -> Push:
    positionals, force, blanket, leases, everything = [], False, False, set(), False
    args = list(git.args)
    while args:
        a = args.pop(0)
        if a == "--":
            positionals.extend(args)
            break
        if a == "--force" or "f" in short_cluster(a):
            force = True
        elif a.startswith("--force-with-lease"):
            m = re.fullmatch(r"--force-with-lease=([^:]+):([0-9a-fA-F]{7,40})", a)
            if m:
                leases.add(m.group(1).removeprefix("refs/heads/"))
            else:
                blanket = True
        elif a in {"--all", "--mirror", "--branches"}:
            everything = True
        elif a in {"--repo", "-o", "--push-option", "--receive-pack", "--exec"}:
            args = args[1:]
        elif not a.startswith("-"):
            positionals.append(a)
    remote = positionals[0] if positionals else "origin"
    targets = []
    for spec in positionals[1:]:
        plus = spec.startswith("+")
        spec = spec.removeprefix("+")
        src, _, dst = spec.partition(":")
        dst = dst if ":" in spec else src
        if src == "HEAD":
            src = current_branch(git.cwd)
            dst = src if ":" not in spec else dst
        targets.append((src.removeprefix("refs/heads/"), dst.removeprefix("refs/heads/"), plus))
    if len(positionals) <= 1 and not everything:
        branch = current_branch(git.cwd)
        if branch:
            targets.append((branch, branch, False))
    return Push(remote, targets, force, blanket, leases, everything)


def is_protected(branch: str, cwd: str) -> bool:
    names = {"main", TRUNK}
    try:
        common = run(["git", "rev-parse", "--git-common-dir"], cwd).stdout.strip()
        with open(os.path.join(cwd, common, ".graphite_repo_config")) as f:
            names |= {t["name"] for t in json.load(f).get("trunks", [])}
    except (OSError, ValueError, KeyError, subprocess.SubprocessError):
        pass
    return branch in names


def rule_mutation(inv: Invocation) -> str | None:
    head, args = os.path.basename(inv.argv[0]), inv.argv[1:]
    runs_mutation = (
        head == "gremlins"
        or (head == "monacoctl" and args[:1] == ["mutation"])
        or (head == "go" and args[:1] == ["run"] and any(
            a.rstrip("/").endswith("cmd/monacoctl") and args[i + 1 : i + 2] == ["mutation"]
            for i, a in enumerate(args)))
        or (head == "just" and "mutation" in args)
    )
    if runs_mutation:
        return ("mutation testing runs in CI (the Mutation job), not locally. "
                "A local run keeps a test database per surviving mutant and has filled the shared test Postgres.")
    return None


def rule_push_protected(inv: Invocation) -> str | None:
    git = as_git(inv)
    if not git or git.sub != "push":
        return None
    push = parse_push(git)
    if push.everything:
        return f"git push --all/--mirror/--branches can push main or {TRUNK}. Push one named branch."
    for _, dst, _ in push.targets:
        if is_protected(dst, git.cwd):
            return (f"git push to '{dst}' is not allowed. main changes only through the operator's checkpoint PR, "
                    f"and {TRUNK} only through the Graphite merge queue after ci-ok and verify pass. "
                    "Push your ticket branch with gt submit --stack.")
    return None


def rule_force_push(inv: Invocation) -> str | None:
    git = as_git(inv)
    if not git or git.sub != "push":
        return None
    push = parse_push(git)
    if push.force:
        return ("git push --force/-f is not allowed. Use --force-with-lease=<branch>:<expected sha>, "
                "with the sha you last saw on the remote.")
    if push.blanket_lease:
        return ("--force-with-lease without <branch>:<sha> trusts whatever a background fetch left in "
                "origin/<branch>. Name the expected sha: --force-with-lease=<branch>:<sha>.")
    for _, dst, plus in push.targets:
        if plus and dst not in push.explicit_leases:
            return (f"'+{dst}' force-pushes without a lease. Use --force-with-lease={dst}:<expected sha>.")
    return None


def rule_push_behind(inv: Invocation) -> str | None:
    git = as_git(inv)
    if not git or git.sub != "push":
        return None
    push = parse_push(git)
    if not re.fullmatch(r"[A-Za-z0-9._-]+", push.remote):
        return None
    for src, dst, _ in push.targets:
        if not src or not dst:
            continue
        try:
            remote = run(["git", "ls-remote", push.remote, f"refs/heads/{dst}"], git.cwd).stdout.split()
            if not remote:
                continue
            sha = remote[0]
            if run(["git", "cat-file", "-e", f"{sha}^{{commit}}"], git.cwd).returncode != 0:
                run(["git", "fetch", "--quiet", push.remote, dst], git.cwd, timeout=60)
            missing = run(["git", "log", "--oneline", "--cherry-pick", "--right-only", "--no-merges",
                           f"{src}...{sha}"], git.cwd).stdout.strip()
        except (OSError, subprocess.SubprocessError):
            return f"could not compare '{src}' with {push.remote}/{dst}; retry once the remote is reachable."
        if missing:
            return (f"{push.remote}/{dst} has commits that '{src}' lacks, so this push would drop them:\n"
                    f"{missing}\nBring them in first (git pull --rebase, or gt sync --no-restack and gt restack), then push.")
    return None


def rule_claude_timeout(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "claude" or inv.under_timeout:
        return None
    if any(a in {"-p", "--print"} for a in inv.argv[1:]):
        return "claude -p must run under timeout, e.g. `timeout 180 claude -p ...`. A headless run can hang forever."
    return None


def gh_selector(args: list[str], valued: set[str]) -> tuple[str | None, list[str]]:
    selector, repo = None, []
    i = 0
    while i < len(args):
        a = args[i]
        if a in {"-R", "--repo"} and i + 1 < len(args):
            repo = ["-R", args[i + 1]]
            i += 2
            continue
        if a.startswith("--repo="):
            repo = ["-R", a.split("=", 1)[1]]
        elif a in valued:
            i += 2
            continue
        elif not a.startswith("-") and selector is None:
            selector = a
        i += 1
    return selector, repo


def rule_merge_needs_verify(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gh" or inv.argv[1:3] != ["pr", "merge"]:
        return None
    valued = {"-b", "--body", "-F", "--body-file", "-t", "--subject", "-A", "--author-email", "--match-head-commit"}
    selector, repo = gh_selector(inv.argv[3:], valued)
    try:
        view = run(["gh", "pr", "view", *([selector] if selector else []), *repo,
                    "--json", "baseRefName,headRefOid,number,url"], inv.cwd)
        if view.returncode != 0:
            return f"could not resolve the PR to check its verify status: {view.stderr.strip()}"
        pr = json.loads(view.stdout)
        if pr["baseRefName"] == "main":
            return f"PR #{pr['number']} targets main. Only the operator merges into main, by hand in GitHub."
        if pr["baseRefName"] == TRUNK:
            return (f"PR #{pr['number']} targets {TRUNK}, which takes PRs only through the Graphite merge queue; "
                    "a direct merge skips stage 2. Land the stack with `monacoctl agents land-stack <top-pr>`, "
                    f"which adds the {QUEUE_LABEL} label to each PR once stage 1 passes.")
        slug = re.match(r"https://github\.com/([^/]+/[^/]+)/pull/", pr["url"]).group(1)
        statuses = run(["gh", "api", f"repos/{slug}/commits/{pr['headRefOid']}/statuses?per_page=100"], inv.cwd)
        if statuses.returncode != 0:
            return f"could not read statuses for {pr['headRefOid']}: {statuses.stderr.strip()}"
        verify = [s for s in json.loads(statuses.stdout) if s.get("context") == "verify"]
    except (OSError, ValueError, KeyError, AttributeError, subprocess.SubprocessError) as e:
        return f"could not check the verify status: {e}"
    if verify and verify[0].get("state") == "success":
        return None
    state = verify[0].get("state") if verify else "missing"
    return (f"PR #{pr['number']} head {pr['headRefOid'][:12]} has no verify success (latest: {state}). "
            "An independent verifier posts it after checking that exact sha.")


def rule_edit_base(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gh" or inv.argv[1:3] != ["pr", "edit"]:
        return None
    if any(a in {"-B", "--base"} or a.startswith("--base=") for a in inv.argv[3:]):
        return "gh pr edit --base is not allowed. Graphite owns every base: gt submit --stack sets them."
    return None


def rule_queue_label(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gh":
        return None
    if inv.argv[1:3] in (["pr", "edit"], ["issue", "edit"]):
        args = inv.argv[3:]
        labels = [v for i, a in enumerate(args) if a == "--add-label" for v in args[i + 1 : i + 2]]
        labels += [a.split("=", 1)[1] for a in args if a.startswith("--add-label=")]
        added = {s.strip() for v in labels for s in v.split(",")}
    elif inv.argv[1:2] == ["api"] and any("/labels" in a for a in inv.argv[2:]):
        added = {w for a in inv.argv[2:] for w in re.findall(r"[\w-]+", a)}
    else:
        return None
    if FAST_TRACK_LABEL in added:
        return f"the {FAST_TRACK_LABEL} label jumps the Graphite merge queue. Only a human adds it."
    if QUEUE_LABEL in added:
        return (f"the {QUEUE_LABEL} label puts a PR in the Graphite merge queue. Only "
                "`monacoctl agents land-stack <top-pr>` adds it, after stage 1 passes on every PR.")
    return None


def rule_inline_pr_body(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gh" or inv.argv[1:2] != ["pr"] or inv.argv[2:3] not in (["edit"], ["create"]):
        return None
    if any(a in {"-b", "--body"} or a.startswith("--body=") for a in inv.argv[3:]):
        return ("inline PR bodies are not allowed; a quoted heredoc once ran a command by accident. "
                f"Write the body to a file, then run {PR_BODY_USAGE}.")
    return None


def commit_messages(inv: Invocation) -> list[str | None] | None:
    head, args = os.path.basename(inv.argv[0]), inv.argv[1:]
    git = as_git(inv)
    msg_flags = {"m", "message"}
    if git and git.sub == "commit":
        args, cwd, file_flags = git.args, git.cwd, {"F", "file"}
    elif head == "gt" and args[:1] in (["create"], ["c"], ["modify"], ["m"]):
        args, cwd, file_flags = args[1:], inv.cwd, set()
    else:
        return None
    messages: list[str | None] = []
    short_flags = {f for f in msg_flags | file_flags if len(f) == 1}
    i = 0
    while i < len(args):
        a, name, value = args[i], None, None
        if a.startswith("--"):
            name, eq, value = a[2:].partition("=")
            if not eq:
                value = args[i + 1] if i + 1 < len(args) else None
                i += 1 if name in msg_flags | file_flags else 0
        elif a.startswith("-") and len(a) > 1:
            for j, ch in enumerate(a[1:], start=1):
                if ch in short_flags:
                    name, value = ch, a[j + 1 :] or (args[i + 1] if i + 1 < len(args) else None)
                    i += 0 if a[j + 1 :] else 1
                    break
        i += 1
        if name in msg_flags and value is not None:
            messages.append(message_text(value))
        elif name in file_flags and value is not None:
            messages.append(file_text(value, cwd, inv.stdin))
    return messages


def message_text(value: str) -> str | None:
    m = HEREDOC_SUBST_RE.match(value)
    if m:
        return m.group(3)
    return None if "$" in value or "`" in value else value


def file_text(path: str, cwd: str, stdin: str | None) -> str | None:
    if path == "-":
        return stdin
    try:
        with open(os.path.join(cwd, os.path.expanduser(path))) as f:
            return f.read()
    except OSError:
        return None


def rule_conventional_commit(inv: Invocation) -> str | None:
    messages = commit_messages(inv)
    if not messages or messages[0] is None:
        return None
    subject = messages[0].strip().splitlines()[0] if messages[0].strip() else ""
    if CONVENTIONAL_RE.match(subject):
        return None
    return (f'commit subject "{subject}" is not a Conventional Commit. Use `type(scope)!: subject` with type one of '
            f"{CONVENTIONAL_TYPES.replace('|', ', ')}. The /commit skill writes one from the diff.")


def owner_checkout(cwd: str) -> bool:
    parts = [p for p in os.path.normpath(cwd).split(os.sep) if p]
    if ".worktrees" in parts and parts.index(".worktrees") + 1 < len(parts):
        return True
    return os.path.isdir(os.path.join(cwd, ".git"))


def rule_raw_history(inv: Invocation) -> str | None:
    git = as_git(inv)
    if git is None or git.sub not in {"rebase", "merge"}:
        return None
    if not owner_checkout(git.cwd):
        return None
    return ("raw git rebase and git merge are blocked in a checkout. "
            "Update with gt sync --no-interactive --no-restack, then gt restack.")


def rule_sync_restacks(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gt":
        return None
    args = inv.argv[1:]
    while args and args[0].startswith("-"):
        args = args[2:] if args[0] == "--cwd" else args[1:]
    if args[:1] != ["sync"] or "--no-restack" in args:
        return None
    return ("plain gt sync restacks every tracked branch, including other agents' stacks mid-build. "
            "Run gt sync --no-interactive --no-restack, then gt restack on your own stack.")


@dataclass
class Role:
    top: str
    common: str
    queued: dict | None = None


@lru_cache(maxsize=None)
def agent_role(cwd: str) -> Role | None:
    try:
        out = run(["git", "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir"], cwd)
    except (OSError, subprocess.SubprocessError):
        return None
    if out.returncode != 0:
        return None
    top, common = out.stdout.split("\n")[:2]
    records = os.path.join(common, ".monaco", "agents")
    try:
        names = sorted(n for n in os.listdir(records) if n.endswith(".json"))
    except OSError:
        return None
    for name in names:
        try:
            with open(os.path.join(records, name)) as f:
                record = json.load(f)
            worktree = record.get("worktree") or ""
        except (OSError, ValueError, AttributeError):
            continue
        if worktree and os.path.realpath(worktree) == os.path.realpath(top):
            return Role(top, common, record.get("queued"))
    return None


def milestone_in(path: str) -> str | None:
    try:
        with open(path) as f:
            m = re.search(r'^\s*milestone\s*=\s*"([^"]+)"', f.read(), re.M)
    except FileNotFoundError:
        return None
    return m.group(1) if m else None


def checked_tree(role: Role, cwd: str) -> tuple[str, bool]:
    tree = run(["git", "rev-parse", "HEAD^{tree}"], cwd).stdout.strip()
    try:
        milestone = (milestone_in(os.path.join(role.common, ".monaco", "agents.local.toml"))
                     or milestone_in(os.path.join(role.top, ".monaco", "agents.toml")))
    except (OSError, ValueError):
        milestone = None
    if not tree or not milestone:
        return tree, False
    return tree, os.path.isfile(os.path.join(role.common, "pstack", milestone, "checks", tree))


def rule_role_push_needs_check(inv: Invocation) -> str | None:
    git = as_git(inv)
    submits = os.path.basename(inv.argv[0]) == "gt" and inv.argv[1:2] in (["submit"], ["ss"])
    if not submits and not (git and git.sub == "push"):
        return None
    cwd = git.cwd if git else inv.cwd
    role = agent_role(cwd)
    if role is None:
        return None
    tree, passed = checked_tree(role, cwd)
    if passed:
        return None
    return (f"`monacoctl agents check` has not passed on this tree ({tree[:12] or 'unknown'}). "
            "Commit, run `cd apps/backend && go run ./cmd/monacoctl agents check`, then push.")


def rule_queued_stack(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gt" or inv.argv[1:2] not in (["submit"], ["s"], ["ss"], ["modify"], ["m"],
                                                                      ["restack"], ["r"]):
        return None
    role = agent_role(inv.cwd)
    if role is None or not isinstance(role.queued, dict):
        return None
    top = role.queued.get("top")
    return (f"this stack is in the merge queue as #{top}. gt {inv.argv[1]} would reset its bases and pull it out "
            f"of the queue. When #{top} merges or leaves the queue, run `monacoctl agents land-stack {top}` to "
            "clear the mark, then fix the stack.")


STACK_MUTATIONS = (["submit"], ["s"], ["ss"], ["modify"], ["m"], ["restack"], ["r"])


def queued_heads(cwd: str) -> dict[str, int]:
    try:
        out = run(["gh", "pr", "list", "--state", "open", "--label", QUEUE_LABEL, "--limit", "100",
                   "--json", "number,headRefName"], cwd)
        if out.returncode != 0:
            return {}
        return {pr["headRefName"]: pr["number"] for pr in json.loads(out.stdout)}
    except (OSError, ValueError, KeyError, subprocess.SubprocessError):
        return {}


def is_ancestor(a: str, b: str, cwd: str) -> bool:
    return run(["git", "merge-base", "--is-ancestor", a, b], cwd).returncode == 0


def same_stack(queued: str, here: str, cwd: str) -> bool:
    if is_ancestor(queued, here, cwd):
        return True
    trunk = f"refs/remotes/origin/{TRUNK}"
    has_trunk = run(["git", "rev-parse", "--verify", "--quiet", trunk], cwd).returncode == 0
    on_trunk = has_trunk and is_ancestor(here, trunk, cwd)
    return not on_trunk and is_ancestor(here, queued, cwd)


# Graphite keeps its own queue: a push to a queued stack can be requeued with heads no verifier saw, and removing the
# label alone does not take a stack out. monacoctl agents dequeue waits until Graphite lets go.
def rule_push_queued_stack(inv: Invocation) -> str | None:
    git = as_git(inv)
    if git and git.sub == "push":
        cwd, pushed = git.cwd, [dst for _, dst, _ in parse_push(git).targets]
    elif os.path.basename(inv.argv[0]) == "gt" and inv.argv[1:2] in STACK_MUTATIONS:
        cwd, pushed = inv.cwd, []
    else:
        return None
    heads = queued_heads(cwd)
    if not heads:
        return None
    here = current_branch(cwd) or "HEAD"
    for head, number in heads.items():
        if head in pushed or head == here:
            hit = True
        else:
            local = run(["git", "rev-parse", "--verify", "--quiet", f"refs/heads/{head}"], cwd).returncode == 0
            hit = local and same_stack(head, here, cwd)
        if hit:
            return (f"#{number} ({head}) is in the Graphite merge queue, and this changes its stack. Graphite would "
                    "requeue heads no verifier saw. Run `monacoctl agents dequeue <top-pr>` first; it removes the "
                    f"{QUEUE_LABEL} label and waits until Graphite lets go.")
    return None


def flag_on(args: list[str], name: str) -> bool:
    on = False
    for a in args:
        flag, eq, value = a.lstrip("-").partition("=")
        if a.startswith("-") and flag == name:
            on = not eq or value.lower() in {"true", "1"}
    return on


def heavy_test(inv: Invocation) -> bool:
    head, args = os.path.basename(inv.argv[0]), inv.argv[1:]
    if head == "test-backend.sh" or (head in SHELLS and any(os.path.basename(a) == "test-backend.sh" for a in args)):
        return True
    if head == "just":
        words = [a for a in args if not a.startswith("-")]
        return words[:1] in (["test"], ["verify"]) and words[1:2] in ([], ["backend"])
    if head != "go" or args[:1] != ["test"]:
        return False
    if flag_on(args, "race") or not flag_on(args, "short"):
        return True
    role = agent_role(inv.cwd)
    backend = role and os.path.realpath(inv.cwd) == os.path.realpath(os.path.join(role.top, "apps", "backend"))
    return bool(backend) and "./..." in args


def rule_role_heavy_tests(inv: Invocation) -> str | None:
    if not heavy_test(inv) or agent_role(inv.cwd) is None:
        return None
    return ("owners and verifiers do not run the full suite: CI stage 1 runs the repo-wide checks and the merge "
            "queue runs every test before anything lands. Run `monacoctl agents check` (stage 0) instead.")


def sleep_seconds(args: list[str]) -> float | None:
    total = 0.0
    for a in args:
        if a == "infinity":
            return float("inf")
        m = SLEEP_RE.fullmatch(a)
        if not m:
            return None
        total += float(m.group(1)) * SLEEP_UNITS[m.group(2)]
    return total


def polls_ci(inv: Invocation) -> bool:
    head, args = os.path.basename(inv.argv[0]), inv.argv[1:]
    if head == "sleep":
        seconds = sleep_seconds(args)
        return seconds is not None and seconds > MAX_SLEEP_SECONDS
    if head != "gh":
        return False
    if args[:2] == ["run", "watch"]:
        return True
    return args[:2] == ["pr", "checks"] and (inv.in_loop or "--watch" in args)


def rule_role_ci_polling(inv: Invocation) -> str | None:
    if not polls_ci(inv) or agent_role(inv.cwd) is None:
        return None
    return (f"owners and verifiers do not wait on CI (no gh run watch, no gh pr checks --watch or in a loop, "
            f"no sleep over {MAX_SLEEP_SECONDS}s). Push, set the PR body, and exit; read checks once with "
            "`gh pr checks <n>`.")


def rule_role_submit_draft(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gt" or inv.argv[1:2] not in (["submit"], ["s"], ["ss"]):
        return None
    args = inv.argv[2:]
    shorts = "".join(short_cluster(a) for a in args)
    if flag_on(args, "publish") or "p" in shorts:
        reason = "gt submit --publish marks every submitted PR ready before its title and body are checked."
    elif flag_on(args, "draft") or "d" in shorts:
        return None
    else:
        reason = "gt submit without --draft opens a new PR ready, titled with the commit subject and an empty body."
    if agent_role(inv.cwd) is None:
        return None
    return f"{reason} {READY_FLOW}"


def rule_role_pr_ready(inv: Invocation) -> str | None:
    if os.path.basename(inv.argv[0]) != "gh" or inv.argv[1:3] != ["pr", "ready"] or "--undo" in inv.argv[3:]:
        return None
    if agent_role(inv.cwd) is None:
        return None
    return f"gh pr ready skips the local PR format check. {READY_FLOW}"


RULES = [
    rule_mutation,
    rule_push_protected,
    rule_force_push,
    rule_push_behind,
    rule_claude_timeout,
    rule_merge_needs_verify,
    rule_edit_base,
    rule_queue_label,
    rule_inline_pr_body,
    rule_conventional_commit,
    rule_raw_history,
    rule_sync_restacks,
    rule_queued_stack,
    rule_push_queued_stack,
    rule_role_push_needs_check,
    rule_role_heavy_tests,
    rule_role_ci_polling,
    rule_role_submit_draft,
    rule_role_pr_ready,
]


def verdict(command: str, cwd: str) -> str | None:
    try:
        invocations = parse(command, cwd)
    except ValueError:
        return None
    for inv in invocations:
        for rule in RULES:
            reason = rule(inv)
            if reason:
                return reason
    return None


EDIT_TOOLS = {"Edit", "Write", "MultiEdit"}
ERROR_CODE_CASES = "ErrorCodeCases.gen.swift"


def generated_file_reason(file_path: str) -> str | None:
    if os.path.basename(file_path) == ERROR_CODE_CASES:
        return "generated file; run: cd apps/backend && go generate ./api"
    if file_path.endswith(".gen.swift"):
        return "generated file; run: cd apps/backend && go run ./cmd/gen flows"
    return None


def main() -> int:
    try:
        event = json.load(sys.stdin)
    except ValueError:
        return 0
    tool_input = event.get("tool_input") or {}
    if event.get("tool_name") in EDIT_TOOLS:
        reason = generated_file_reason(tool_input.get("file_path") or "")
        if reason:
            print(f"blocked by scripts/agent-guard.py: {reason}", file=sys.stderr)
            return 2
        return 0
    command = tool_input.get("command") or ""
    if not command:
        return 0
    reason = verdict(command, event.get("cwd") or os.getcwd())
    if reason:
        print(f"blocked by scripts/agent-guard.py: {reason}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
