#!/usr/bin/env python3
"""Check, run and measure the app journeys in docs/journeys.

  journey.py check                          docs and their XCUITest files agree
  journey.py list                           every journey, its version and scenarios
  journey.py run <journey> [options]           run a journey's XCUITest and record the result
  journey.py run --all [options]               every journey in requires order, one build, one backend
  journey.py mutants <journey> [options]       run a journey against its seeded bugs
  journey.py report                         speed and correctness per journey

The rules the checks enforce are in docs/journeys/README.md. Results go to
.logs/qa/journeys/results.tsv, one row per scenario and one `*` row per run.
"""

import argparse
import contextlib
import datetime
import fcntl
import hashlib
import heapq
import json
import os
import random
import re
import signal
import statistics
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOCS = ROOT / "docs" / "journeys"
QA = ROOT / "apps" / "mobile" / "qa" / "journeys"
OUT = ROOT / ".logs" / "qa" / "journeys"
DERIVED = OUT / "derived"
RESULTS = OUT / "results.tsv"
BUNDLE_ID = "com.monaco.app"
DRIVER = "xcuitest"

COLUMNS = [
    "time", "journey", "version", "scenario", "driver", "build", "run", "result", "expected",
    "truth", "wall_s", "steps_ms", "failed_step", "log",
]


class JourneyError(Exception):
    """A problem the person running the script can fix. Printed without a traceback."""


# ---------------------------------------------------------------- journey docs


def parse_front_matter(text):
    """The subset of YAML a journey doc uses: scalars, inline lists, one level of nesting."""
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        raise JourneyError("no front matter: the first line must be ---")
    try:
        end = lines[1:].index("---") + 1
    except ValueError:
        raise JourneyError("front matter is not closed with ---")
    meta, parent = {}, None
    for raw in lines[1:end]:
        line = re.sub(r"\s+#.*$", "", raw).rstrip()
        if not line.strip():
            continue
        if ":" not in line:
            raise JourneyError("front matter line has no key: %r" % raw)
        key, _, value = line.strip().partition(":")
        value = _scalar(value.strip())
        if line[0] in " \t":
            if parent is None:
                raise JourneyError("indented front matter line under no key: %r" % raw)
            meta[parent][key] = value
        elif value == "":
            meta[key], parent = {}, key
        else:
            meta[key], parent = value, None
    return meta, "\n".join(lines[end + 1:])


def _scalar(value):
    if value.startswith("[") and value.endswith("]"):
        return [_scalar(part.strip()) for part in value[1:-1].split(",") if part.strip()]
    if re.fullmatch(r"\d+", value):
        return int(value)
    return value.strip("\"'")


class Journey:
    def __init__(self, path):
        self.path = path
        meta, body = parse_front_matter(path.read_text())
        self.meta = meta
        self.id = meta.get("id", "")
        self.version = meta.get("version")
        self.requires = _as_list(meta.get("requires", []))
        self.actors = _as_list(meta.get("actors", []))
        self.flows = ["%02d" % flow if isinstance(flow, int) else flow for flow in _as_list(meta.get("flows", []))]
        self.xcuitest = _as_list(meta.get("xcuitest", []))
        funds = meta.get("funds", {})
        self.funds = funds if isinstance(funds, dict) else {}
        self.scenarios = re.findall(r"^### (S\d+) ", body, re.M)
        self.steps = re.findall(r"^\| (S\d+\.\d+) \|", body, re.M)
        self.known = parse_known(body, self.steps)

    def driver_files(self):
        return [ROOT / p for p in self.xcuitest]

    def mutants(self):
        folder = QA / (self.id + ".mutants")
        return sorted(folder.glob("*.patch")) if folder.is_dir() else []

    def truth_script(self):
        script = QA / (self.id + ".truth.sh")
        return script if script.exists() else None

    def setup_script(self):
        script = QA / (self.id + ".setup.sh")
        return script if script.exists() else None


KNOWN_SECTION = re.compile(r"^## Known failures on staging\n(.*?)(?=^## |\Z)", re.M | re.S)


def parse_known(body, steps):
    """{step id: (ticket, ...)} from the doc's "Known failures on staging" bullets or table.

    A bullet names its steps before the first colon and its tickets after "Blocked by". A table row
    names its steps in the first cell and its tickets in the last. "S1.2 to S1.5" is every step
    of the doc from S1.2 through S1.5.
    """
    section = KNOWN_SECTION.search(body)
    known = {}
    for line in (section.group(1) if section else "").splitlines():
        if line.startswith("|"):
            cells = line.strip().strip("|").split("|")
            where, tickets = cells[0], re.findall(r"#\d+", cells[-1])
        elif line.startswith("- "):
            where = line[2:].partition(":")[0]
            blocked = re.search(r"[Bb]locked by (.*)", line)
            tickets = re.findall(r"#\d+", blocked.group(1) if blocked else line)
        else:
            continue
        for first, last in re.findall(r"(S\d+\.\d+)(?: to (S\d+\.\d+))?", where):
            ids = steps[steps.index(first):steps.index(last) + 1] if last in steps and first in steps else [first]
            for step in ids:
                known[step] = tuple(dict.fromkeys(known.get(step, ()) + tuple(tickets)))
    return known


def classify_known(rows, known):
    """A scenario that failed at a known step becomes KNOWN; one that passed with a known step, FIXED.

    Returns {scenario: "FAIL"} for both, the doc's expectation. Any other failure stays FAIL.
    """
    expected = {}
    for row in rows:
        if row["result"] == "FAIL" and row["failed_step"] in known:
            row["result"] = "KNOWN"
        elif row["result"] == "PASS" and any(step.split(".")[0] == row["scenario"] for step in known):
            row["result"] = "FIXED"
        else:
            continue
        expected[row["scenario"]] = "FAIL"
    return expected


def _as_list(value):
    if value in ("", None):
        return []
    return value if isinstance(value, list) else [value]


def load_journeys():
    journeys = {}
    for path in sorted(DOCS.rglob("*.md")):
        if path.name == "README.md":
            continue
        journey = Journey(path)
        key = journey.id or str(path)
        journeys[key if key not in journeys else str(path)] = journey
    return journeys


def load_accounts(environ=None):
    """accounts.tsv, with MONACO_QA_<actor>_* from the environment taking over a row's value."""
    environ = os.environ if environ is None else environ
    accounts = {}
    header = None
    for line in (QA / "accounts.tsv").read_text().splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        cells = line.split("\t")
        if header is None:
            header = cells
            continue
        row = dict(zip(header, cells))
        accounts[row["actor"]] = row
    for actor, row in accounts.items():
        for field in ("phone", "email", "code", "name"):
            row[field] = environ.get("MONACO_QA_%s_%s" % (actor, field.upper()), row[field])
    return accounts


def mutant_expectation(patch):
    """The scenarios a seeded bug must fail, from its `expect-fail:` line."""
    match = re.search(r"^expect-fail:\s*(.+)$", patch.read_text(), re.M)
    return re.findall(r"S\d+", match.group(1)) if match else []


# ---------------------------------------------------------------- check


def check_journeys(journeys, accounts, git_apply_check=None):
    """Every way a doc and its XCUITest can disagree. Returns a list of 'path: message'."""
    problems = []

    def bad(path, message):
        problems.append("%s: %s" % (os.path.relpath(str(path), str(ROOT)), message))

    by_id = {}
    for journey in journeys.values():
        if journey.id in by_id:
            bad(journey.path, "duplicate id %s in %s and %s" % (
                journey.id, os.path.relpath(str(by_id[journey.id].path), str(ROOT)),
                os.path.relpath(str(journey.path), str(ROOT))))
        else:
            by_id[journey.id] = journey

    for journey in journeys.values():
        expected_id = str(journey.path.relative_to(DOCS).with_suffix(""))
        if journey.id != expected_id:
            bad(journey.path, "id is %r, the path says %r" % (journey.id, expected_id))
        if not isinstance(journey.version, int) or journey.version < 1:
            bad(journey.path, "version must be a whole number from 1, got %r" % (journey.version,))
        for step in journey.known:
            if step not in journey.steps:
                bad(journey.path, "Known failures names %s, which is not a step of the doc" % step)
        if not journey.scenarios:
            bad(journey.path, "no scenario: add a '### S1 <name>' heading and its step table")
        for scenario in journey.scenarios:
            if not any(step.startswith(scenario + ".") for step in journey.steps):
                bad(journey.path, "scenario %s has no step rows (| %s.1 | …)" % (scenario, scenario))
        if not journey.actors:
            bad(journey.path, "actors is empty")
        for actor in journey.actors:
            if actor not in accounts:
                bad(journey.path, "actor %s has no row in apps/mobile/qa/journeys/accounts.tsv" % actor)
        for actor, amount in journey.funds.items():
            if actor not in journey.actors:
                bad(journey.path, "funds names actor %s, which is not in actors" % actor)
            if not isinstance(amount, int) or amount < 1:
                bad(journey.path, "funds for actor %s must be whole USDC from 1, got %r" % (actor, amount))
        for required in journey.requires:
            if required not in by_id:
                bad(journey.path, "requires %r, which is not a journey doc" % required)
        for flow in journey.flows:
            if not (ROOT / "packages" / "flows" / "backend" / (flow + ".tsv")).exists():
                bad(journey.path, "flows names %s, which has no packages/flows/backend/%s.tsv" % (flow, flow))
        files = journey.driver_files()
        missing = [f for f in files if not f.exists()]
        for f in missing:
            bad(f, "listed as a test file of %s but does not exist" % journey.id)
        if not files:
            bad(journey.path, "no files listed under xcuitest")
        elif not missing:
            combined = "\n".join(f.read_text() for f in files)
            # Swift files carry no comments here, so the stamp is the steps enum's constants.
            stamped_journey = re.search(r'static let id = "([\w/-]+)"', combined)
            stamped_version = re.search(r"static let version = (\d+)", combined)
            if not stamped_journey or stamped_journey.group(1) != journey.id:
                bad(files[0], 'does not declare static let id = "%s"' % journey.id)
            if not stamped_version:
                bad(files[0], "does not declare static let version = %s" % journey.version)
            elif int(stamped_version.group(1)) != journey.version:
                bad(files[0], "built from %s version %s, the doc is at version %s: rebuild it with the ios-journey-qa skill"
                    % (journey.id, stamped_version.group(1), journey.version))
            for step in journey.steps:
                if '"%s"' % step not in combined:
                    bad(files[0], "no step %s" % step)
            for scenario in journey.scenarios:
                if not xcuitest_phases(journey, scenario):
                    bad(files[-1], "no test method named test%s… for scenario %s" % (scenario, scenario))
        for patch in journey.mutants():
            expected = mutant_expectation(patch)
            if not expected:
                bad(patch, "no 'expect-fail: S…' line")
            for scenario in expected:
                if scenario not in journey.scenarios:
                    bad(patch, "expects %s to fail, which is not a scenario of %s" % (scenario, journey.id))
            if git_apply_check and not git_apply_check(patch):
                bad(patch, "does not apply to this checkout for journey %s: regenerate this patch with the ios-journey-qa skill" % journey.id)

    for journey in journeys.values():
        if _has_cycle(journey, by_id):
            bad(journey.path, "requires form a cycle through %s" % journey.id)
    return problems


def _has_cycle(journey, journeys):
    start = journey.id
    stack, seen = [r for r in journey.requires if r in journeys], set()
    while stack:
        current = stack.pop()
        if current == start:
            return True
        if current in seen:
            continue
        seen.add(current)
        stack.extend(r for r in journeys[current].requires if r in journeys)
    return False


def xcuitest_phases(journey, scenario):
    """[(phase, actor, 'Class/method')] for a scenario, in the order to run them.

    A one-actor scenario is one method, test<S1>…. A scenario with more actors is one method
    per phase, test<S1>Phase<n><actor>…, because one xcodebuild call drives one simulator.
    """
    phases = []
    for path in journey.driver_files():
        if not path.exists():
            continue
        text = path.read_text()
        class_name = re.search(r"final class (\w+): XCTestCase", text)
        if not class_name:
            continue
        for method, phase, actor in re.findall(r"func (test%s(?!\d)(?:Phase(\d+)([A-Z]))?\w*)\(" % scenario, text):
            phases.append((int(phase or 1), actor or journey.actors[0], "%s/%s" % (class_name.group(1), method)))
    return sorted(phases)


# ---------------------------------------------------------------- running


class BudgetExpired(Exception):
    """A journey run's budget ran out while a subprocess ran. Carries the output it wrote first."""

    def __init__(self, output=""):
        super().__init__("budget expired")
        self.output = output or ""


class Budget:
    """The seconds one journey run may take: setup scripts, test calls and the truth check together."""

    def __init__(self, seconds):
        self.seconds = seconds
        self.deadline = time.monotonic() + seconds

    def left(self):
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            raise BudgetExpired()
        return remaining


def sh(args, timeout=None, **kwargs):
    """Run a command from the repo root. With a timeout, it runs in its own process group, and on
    expiry the group gets SIGTERM, then SIGKILL after 10 s, and BudgetExpired is raised."""
    if timeout is None:
        return subprocess.run(args, cwd=str(ROOT), universal_newlines=True, **kwargs)
    process = subprocess.Popen(args, cwd=str(ROOT), universal_newlines=True, start_new_session=True, **kwargs)
    try:
        stdout, _ = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        raise BudgetExpired(kill_group(process))
    return subprocess.CompletedProcess(args, process.returncode, stdout)


def kill_group(process, grace=10):
    """SIGTERM a process group, SIGKILL it after `grace` seconds, and return what it wrote."""
    with contextlib.suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGTERM)
    try:
        stdout, _ = process.communicate(timeout=grace)
    except subprocess.TimeoutExpired:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
        stdout, _ = process.communicate()
    # The leader can exit on SIGTERM while a child it started ignores it. macOS answers EPERM, not ESRCH,
    # when the group's only members are zombies not yet reaped.
    with contextlib.suppress(ProcessLookupError, PermissionError):
        os.killpg(process.pid, signal.SIGKILL)
    return stdout


def journey_api_base_url():
    return os.environ.get("MONACO_API_BASE_URL", "http://127.0.0.1:8080")


def backend_is_running(base_url=None, opener=None):
    base_url = base_url or journey_api_base_url()
    health_url = base_url.rstrip("/") + "/healthz"
    try:
        response = (opener or urllib.request.urlopen)(health_url, timeout=2)
        status = getattr(response, "status", None)
        if status is None:
            status = response.getcode()
        close = getattr(response, "close", None)
        if close:
            close()
        return status == 200
    except (OSError, urllib.error.URLError):
        return False


QA_LOCK = "/tmp/monaco-qa.lock"
BACKEND_PORTS = (8080, 8081)


def take_qa_lock(path=QA_LOCK):
    """Journey runs share the backend ports and the simulators, so one runs at a time per machine.

    The lock is the one `/usr/bin/lockf -k /tmp/monaco-qa.lock` takes."""
    handle = open(path, "a")
    try:
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print("waiting for %s: another journey run holds it" % path, flush=True)
        fcntl.flock(handle, fcntl.LOCK_EX)
    return handle


def listeners(ports=BACKEND_PORTS, run=None):
    """[(pid, cwd)] of the processes listening on the backend ports."""
    run = run or sh
    found = run(["lsof", "-nP", "-sTCP:LISTEN", "-t"] + ["-iTCP:%d" % port for port in ports],
                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout
    result = []
    for pid in sorted(set(found.split()), key=int):
        fields = run(["lsof", "-a", "-p", pid, "-d", "cwd", "-Fn"],
                     stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout
        cwd = next((line[1:] for line in fields.splitlines() if line.startswith("n")), "?")
        result.append((pid, cwd))
    return result


def refuse_busy_ports(busy, ports=BACKEND_PORTS):
    if busy:
        raise JourneyError(
            "port %s is already in use by %s: stop that backend first, journey.py starts its own" % (
                " or ".join(str(port) for port in ports),
                ", ".join("pid %s (%s)" % (pid, cwd) for pid, cwd in busy)))


def start_backend(base_url, timeout=300):
    log = OUT / "backend.log"
    OUT.mkdir(parents=True, exist_ok=True)
    print("starting the backend (log: %s)" % os.path.relpath(str(log), str(ROOT)))
    # The backend refuses unknown MONACO_ variables at boot, and a run's account overrides are MONACO_QA_.
    env = {key: value for key, value in os.environ.items() if not key.startswith("MONACO_QA_")}
    if env.get("QA_FAKE_RPC") == "1":
        env["SOLANA_RPC_URL"] = env.get("QA_FAKES_URL", "http://127.0.0.1:8099") + "/rpc/"
    with open(str(log), "w") as out:
        process = subprocess.Popen(["just", "run", "backend"], cwd=str(ROOT), env=env, stdout=out,
                                   stderr=subprocess.STDOUT, start_new_session=True)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if backend_is_running(base_url):
            return process
        if process.poll() is not None:
            raise JourneyError("the backend stopped at boot, see %s" % log)
        time.sleep(2)
    stop_backend(process)
    raise JourneyError("the backend did not answer /healthz within %d s, see %s" % (timeout, log))


def stop_backend(process):
    sh(["just", "stop", "backend"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        process.wait(timeout=30)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGTERM)


@contextlib.contextmanager
def journey_backend():
    """The api a run talks to. MONACO_API_BASE_URL names one that is already up; otherwise the run
    takes the QA lock, refuses ports another process holds, and starts and stops its own backend."""
    if "MONACO_API_BASE_URL" in os.environ:
        yield require_backend()
        return
    with take_qa_lock():
        refuse_busy_ports(listeners())
        base_url = journey_api_base_url()
        process = start_backend(base_url)
        try:
            yield base_url
        finally:
            stop_backend(process)


def require_backend():
    base_url = journey_api_base_url()
    if not backend_is_running(base_url):
        raise JourneyError("the backend is not running: start it with just run backend")
    return base_url


def git_apply_check(patch):
    return sh(["git", "apply", "--check", str(patch)], stderr=subprocess.DEVNULL).returncode == 0


def build_label(mutant=None):
    sha = sh(["git", "rev-parse", "--short", "HEAD"], stdout=subprocess.PIPE).stdout.strip()
    return "%s+mutant:%s" % (sha, mutant) if mutant else sha


def simulator_devices():
    result = sh(["xcrun", "simctl", "list", "devices", "--json"], stdout=subprocess.PIPE, check=True)
    return json.loads(result.stdout).get("devices", {})


def simulator_template(devices):
    gold = sh(["scripts/gold-sim-udid.sh"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    if gold.returncode == 0:
        gold_udid = gold.stdout.strip()
        for runtime, items in devices.items():
            for item in items:
                if item.get("udid") == gold_udid:
                    return item["deviceTypeIdentifier"], runtime
    runtimes = json.loads(sh(["xcrun", "simctl", "list", "runtimes", "--json"], stdout=subprocess.PIPE,
                              check=True).stdout).get("runtimes", [])
    available = [runtime for runtime in runtimes if runtime.get("isAvailable") and runtime.get("platform") == "iOS"]
    if not available:
        raise JourneyError("no available iOS simulator runtime")
    runtime = max(available, key=lambda item: tuple(int(part) for part in item["version"].split(".")))
    phones = [item for item in runtime.get("supportedDeviceTypes", []) if item.get("productFamily") == "iPhone"]
    if not phones:
        raise JourneyError("the newest available iOS runtime has no iPhone device type")
    return phones[0]["identifier"], runtime["identifier"]


def named_simulator(devices, name):
    for items in devices.values():
        for item in items:
            if item.get("name") == name and item.get("isAvailable"):
                return item["udid"]
    return ""


def lane_name():
    """The lane this checkout is: a linked worktree's directory name, or None for the primary checkout."""
    result = subprocess.run(
        ["git", "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-toplevel"],
        cwd=str(ROOT), universal_newlines=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    if result.returncode != 0:
        return None
    git_dir, common_dir, top = result.stdout.split("\n")[:3]
    return None if git_dir == common_dir else Path(top).name


def register_lane_simulator(udid, lane, name):
    """Record a lane's simulator where scripts/stop-mobile.sh looks for them (see scripts/lane-sim-udid.sh)."""
    common = sh(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                stdout=subprocess.PIPE, check=True).stdout.strip()
    with open(os.path.join(common, "monaco-lane-sims.tsv"), "a") as registry:
        registry.write("%s\t%s\t%s\n" % (udid, lane, name))


def journey_simulator_name(actor):
    """Actor simulators are per lane, so journeys in two worktrees never share one."""
    lane = lane_name()
    return "Monaco Journeys %s %s" % (lane, actor) if lane else "Monaco Journeys %s" % actor


def resolve_simulators(journey, mapping):
    sims = dict(mapping)
    missing = [actor for actor in journey.actors if actor not in sims]
    if missing:
        devices = simulator_devices()
        device_type, runtime = simulator_template(devices)
        for actor in missing:
            name = journey_simulator_name(actor)
            sims[actor] = named_simulator(devices, name)
            if not sims[actor]:
                sims[actor] = sh(["xcrun", "simctl", "create", name, device_type, runtime],
                                 stdout=subprocess.PIPE, check=True).stdout.strip()
                print("created simulator %s" % name)
                lane = lane_name()
                if lane:
                    register_lane_simulator(sims[actor], lane, name)
    if len(set(sims.values())) != len(sims):
        raise JourneyError("two actors share one simulator: %s" % sims)
    return sims


def simulator_names(sims):
    names = {}
    for items in simulator_devices().values():
        for item in items:
            if item.get("udid") in sims.values():
                names[item["udid"]] = item.get("name", item["udid"])
    return names


def check_simulator_api_environment(sims, api_base_url):
    names = simulator_names(sims)
    for udid in sims.values():
        boot_simulator(udid)
        value = sh(["xcrun", "simctl", "getenv", udid, "MONACO_API_BASE_URL"], stdout=subprocess.PIPE,
                   stderr=subprocess.DEVNULL).stdout.strip()
        if value and value != api_base_url:
            name = names.get(udid, udid)
            raise JourneyError(
                "simulator %s has MONACO_API_BASE_URL=%s in its environment, which overrides the journey's API. "
                "Use another simulator with --sim, or clear it with: xcrun simctl spawn %s launchctl unsetenv "
                "MONACO_API_BASE_URL" % (name, value, udid))


BOOTED = []


def boot_simulator(udid):
    sh(["xcrun", "simctl", "boot", udid], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    sh(["xcrun", "simctl", "bootstatus", udid, "-b"], check=True,
       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if udid not in BOOTED:
        BOOTED.append(udid)


@contextlib.contextmanager
def simulator_shutdown(keep=False):
    """Shuts down every simulator the run booted when it ends, passes, fails or is interrupted. A simulator
    that was already booted before the run is left alone. keep=True (--keep-sims) skips the shutdown."""
    del BOOTED[:]
    before = set()
    for items in simulator_devices().values():
        before.update(item["udid"] for item in items if item.get("state") == "Booted")
    handlers = {signum: signal.signal(signum, lambda signum, frame: (_ for _ in ()).throw(KeyboardInterrupt()))
                for signum in (signal.SIGTERM, signal.SIGHUP)}
    try:
        yield
    finally:
        for signum, handler in handlers.items():
            signal.signal(signum, handler)
        if not keep:
            for udid in BOOTED:
                if udid not in before:
                    sh(["xcrun", "simctl", "shutdown", udid], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        del BOOTED[:]


def reset_journey_simulators(journey, sims, explicit_actors, fresh):
    """Uninstall the app from dedicated simulators before one journey run."""
    targets = [
        (actor, sims[actor]) for actor in journey.actors
        if fresh or actor not in explicit_actors
    ]
    names = simulator_names(dict(targets))
    for actor, udid in targets:
        boot_simulator(udid)
        sh(["xcrun", "simctl", "uninstall", udid, BUNDLE_ID],
           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        print("reset app on simulator %s" % names.get(udid, udid))


def xcodebuild(sim, *extra):
    return [
        "scripts/qa/xcode-lock.sh", "xcode", "xcodebuild",
        "-project", "apps/mobile/Monaco.xcodeproj", "-scheme", "Monaco", "-configuration", "Debug",
        "-destination", "platform=iOS Simulator,id=%s" % sim,
        "-derivedDataPath", str(DERIVED), "-onlyUsePackageVersionsFromResolvedFile",
        "-skipMacroValidation", "-skipPackagePluginValidation",
        # Ad-hoc signed: an unsigned build drops the Privy session (docs/how-to/local-simulator.md).
        "CODE_SIGN_IDENTITY=-", "CODE_SIGNING_REQUIRED=NO", "CODE_SIGNING_ALLOWED=YES",
    ] + list(extra)


APP_SOURCES = ("apps/mobile", "packages/mobile-core")


def build_stamp():
    """sha256 of HEAD, the uncommitted diff of the app sources, and their untracked files."""
    digest = hashlib.sha256()
    git = lambda *args: sh(["git"] + list(args), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout or ""
    digest.update(git("rev-parse", "HEAD").encode())
    digest.update(git("diff", "HEAD", "--", *APP_SOURCES).encode())
    untracked = git("ls-files", "--others", "--exclude-standard", "--", *APP_SOURCES)
    digest.update(untracked.encode())
    # The names alone miss an edit to a file that is not committed yet.
    for name in untracked.splitlines():
        with contextlib.suppress(OSError):
            digest.update((ROOT / name).read_bytes())
    return digest.hexdigest()


def stamp_file():
    return DERIVED / "build.stamp"


def build_is_current(current):
    """The last build is of these sources and its test bundle is still there."""
    path = stamp_file()
    return (path.exists() and path.read_text().strip() == current
            and any((DERIVED / "Build" / "Products").glob("*.xctestrun")))


def ensure_build(sim, log, rebuild=False):
    """Build unless the stamp says the last build is of these sources. Returns True when it built."""
    current = build_stamp()
    if not rebuild and build_is_current(current):
        print("reusing build %s" % current[:8])
        return False
    build(sim, log, current)
    return True


def build(sim, log, current=None):
    """Build the app and the UI tests, and stamp the build with the sources it came from."""
    with contextlib.suppress(FileNotFoundError):
        stamp_file().unlink()
    sh(["scripts/ensure-ios-privy-config.sh", "generate"], check=True, stdout=subprocess.DEVNULL)
    print("building (log: %s)" % os.path.relpath(str(log), str(ROOT)))
    with open(str(log), "w") as out:
        code = sh(xcodebuild(sim, "build-for-testing"), stdout=out, stderr=subprocess.STDOUT).returncode
    if code != 0:
        raise JourneyError("the build failed, see %s" % log)
    DERIVED.mkdir(parents=True, exist_ok=True)
    stamp_file().write_text((current or build_stamp()) + "\n")


def new_run_id():
    """{QA.run}: a short id unique to one run, so a value a run writes never matches an earlier run's."""
    alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
    return "".join(random.SystemRandom().choice(alphabet) for _ in range(6))


def actor_environment(accounts, channel, run_id, prefix=""):
    env = {prefix + "MONACO_QA_JOURNEYS": "1", prefix + "MONACO_QA_CHANNEL": channel, prefix + "MONACO_QA_RUN": run_id}
    if os.environ.get("MONACO_QA_REFUND_ADDRESS"):
        env[prefix + "MONACO_QA_REFUND_ADDRESS"] = os.environ["MONACO_QA_REFUND_ADDRESS"]
    for actor, row in accounts.items():
        for field in ("phone", "email", "code", "name"):
            env["%sMONACO_QA_%s_%s" % (prefix, actor, field.upper())] = row[field]
    return env


def parse_steps(output, include_timings=False):
    """(total ms of finished steps, the step that began and never ended) from JOURNEYSTEP lines."""
    total, open_steps, timings = 0, [], []
    for line in output.splitlines():
        cells = line.split("\t")
        if len(cells) < 4 or cells[0] != "JOURNEYSTEP":
            continue
        if cells[1] == "begin":
            open_steps.append(cells[3])
        elif cells[1] == "end" and len(cells) >= 5:
            depth = len(open_steps) - 1
            ms = int(cells[4])
            timings.append((cells[3], ms, depth))
            if depth == 0:
                total += ms
            if cells[3] in open_steps:
                open_steps.pop(len(open_steps) - 1 - open_steps[::-1].index(cells[3]))
    # A step that wraps another (P1 signing out through S3) stays open with it: the innermost
    # open step is the one that failed.
    result = total, (open_steps[-1] if open_steps else "")
    return result + (timings,) if include_timings else result


def split_by_test(output):
    """{method: (verdict, seconds, its slice of the output)} from one xcodebuild test log."""
    tests = {}
    pattern = r"Test Case '-\[\S+ (\w+)\]' started\.(.*?)Test Case '-\[\S+ \1\]' (passed|failed|skipped) \((\d+\.\d+) seconds\)"
    for method, body, verdict, seconds in re.findall(pattern, output, re.S):
        tests[method] = (verdict, float(seconds), body)
    return tests


def run_xcuitest(journey, scenarios, sims, accounts, channel, run_dir, api_base_url, run_id, budget=None):
    """One row per scenario. A scenario passes when every phase's test passed and none skipped.

    One-actor scenarios share one xcodebuild call, because starting the test runner costs more
    than the tests. A scenario with phases runs them one call at a time, each on its actor's
    simulator. A journey with a setup script runs every scenario in calls of its own, after
    `<journey>.setup.sh <scenario>` has put the backend in that scenario's starting state.
    With a budget, every call gets what is left of it, and when it runs out the scenarios not
    finished get TIMEOUT. Returns (rows, wall seconds of every call, '<scenario> <phase>' where
    the budget ran out or None).
    """
    handoff = run_dir / "handoff.json"
    setup = journey.setup_script()
    plan = {scenario: xcuitest_phases(journey, scenario) for scenario in scenarios}
    for scenario, phases in plan.items():
        if not phases:
            raise JourneyError("%s has no test for %s" % (journey.id, scenario))
    single = [] if setup else [s for s in scenarios if len(plan[s]) == 1 and plan[s][0][1] == journey.actors[0]]
    calls = [(None, ",".join(single), "test", journey.actors[0], [plan[s][0][2] for s in single])] if single else []
    for scenario in scenarios:
        if scenario not in single:
            calls.extend((scenario if index == 0 else None, scenario,
                          "phase %d" % phase if len(plan[scenario]) > 1 else "test", actor, [test])
                         for index, (phase, actor, test) in enumerate(plan[scenario]))
    left = budget.left if budget else (lambda: None)

    outcomes, wall, timed_out, partial = {}, 0.0, None, ""
    log = run_dir / "xcuitest.log"
    with open(str(log), "w") as out:
        for starts, label, phase, actor, tests in calls:
            try:
                if setup and starts:
                    phase_now = "setup"
                    out.flush()
                    env = dict(os.environ)
                    env.update(actor_environment(accounts, channel, run_id))
                    env["MONACO_QA_HANDOFF"] = str(handoff)
                    if sh([str(setup), starts], env=env, stdout=out, stderr=subprocess.STDOUT,
                          timeout=left()).returncode != 0:
                        raise JourneyError("%s could not set up %s, see %s" % (
                            os.path.relpath(str(setup), str(ROOT)), starts, os.path.relpath(str(log), str(ROOT))))
                phase_now = phase
                env = dict(os.environ)
                env.update(actor_environment(accounts, channel, run_id, prefix="TEST_RUNNER_"))
                env["TEST_RUNNER_MONACO_QA_ACTOR"] = actor
                env["TEST_RUNNER_MONACO_QA_HANDOFF"] = str(handoff)
                env["TEST_RUNNER_MONACO_QA_API_BASE_URL"] = api_base_url
                # A failed test otherwise waits up to 600 s for a sysdiagnose the journey log never reads.
                only = ["-only-testing:MonacoUITests/%s" % test for test in tests] + ["-collect-test-diagnostics", "never"]
                started = time.time()
                try:
                    done = sh(xcodebuild(sims[actor], *(only + ["test-without-building"])),
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=env, timeout=left())
                finally:
                    wall += time.time() - started
            except BudgetExpired as expired:
                out.write(expired.output)
                partial, timed_out = expired.output, "%s %s" % (label, phase_now)
                break
            out.write(done.stdout)
            outcomes.update(split_by_test(done.stdout))

    rows = []
    step_lines = []
    for scenario in scenarios:
        result, seconds, steps_ms, failed_step = "PASS", 0.0, 0, ""
        for _, _, test in plan[scenario]:
            if timed_out and test.split("/")[1] not in outcomes:
                result, failed_step = "TIMEOUT", parse_steps(partial)[1] or timed_out
                break
            verdict, took, body = outcomes.get(test.split("/")[1], ("missing", 0.0, ""))
            ms, open_step, timings = parse_steps(body, include_timings=True)
            step_lines.extend("%s\t%s\t%d\t%d\n" % ((scenario,) + timing) for timing in timings)
            seconds += took
            steps_ms += ms
            if verdict in ("missing", "skipped"):
                result, failed_step = "ERROR", "the test did not run (%s)" % verdict
            elif verdict == "failed":
                result, failed_step = "FAIL", open_step or "outside a step"
            if result != "PASS":
                break
        rows.append({
            "scenario": scenario, "result": result, "wall_s": "%.1f" % seconds,
            "steps_ms": steps_ms, "failed_step": failed_step, "log": os.path.relpath(str(log), str(ROOT)),
        })
    (run_dir / "steps.tsv").write_text("".join(step_lines))
    return rows, "%.1f" % wall, timed_out



def run_truth(journey, accounts, channel, run_id, handoff, budget=None):
    script = journey.truth_script()
    if not script:
        return "none"
    env = dict(os.environ)
    env.update(actor_environment(accounts, channel, run_id))
    env["MONACO_QA_HANDOFF"] = str(handoff)
    try:
        return "ok" if sh([str(script)], env=env, timeout=budget.left() if budget else None).returncode == 0 else "fail"
    except BudgetExpired:
        return "timeout"


def record(journey, build_name, run_name, rows, summary, expected=None):
    """Append the scenario rows and the run's `*` row to results.tsv."""
    OUT.mkdir(parents=True, exist_ok=True)
    new_file = not RESULTS.exists()
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    with open(str(RESULTS), "a") as out:
        if new_file:
            out.write("\t".join(COLUMNS) + "\n")
        for row in rows + [summary]:
            full = {"time": now, "journey": journey.id, "version": journey.version, "driver": DRIVER,
                    "build": build_name, "run": run_name,
                    "expected": (expected or {}).get(row["scenario"], "PASS")}
            full.update(row)
            out.write("\t".join(str(full.get(column, "")).replace("\t", " ") for column in COLUMNS) + "\n")


def run_once(journey, journeys, args, sims, accounts, build_name, run_name, scenarios, api_base_url, expected=None):
    run_dir = OUT / run_name
    run_dir.mkdir(parents=True, exist_ok=True)
    run_id = new_run_id()
    print("  {QA.run} is %s" % run_id)
    budget = Budget(args.timeout)
    rows, wall, timed_out = run_xcuitest(journey, scenarios, sims, accounts, args.channel, run_dir, api_base_url,
                                         run_id, budget)
    truth = run_truth(journey, accounts, args.channel, run_id, run_dir / "handoff.json", budget)
    if truth == "timeout" and not timed_out:
        timed_out = "* truth"
    if timed_out:
        print("%s timed out after %d s in %s" % (journey.id, budget.seconds, timed_out))
    for row in rows:
        row["truth"] = truth
    if expected is None:
        expected = classify_known(rows, journey.known)
    verdicts = [row["result"] for row in rows] + (["TIMEOUT"] if timed_out else [])
    overall = next((v for v in ("ERROR", "TIMEOUT", "FAIL") if v in verdicts), "PASS")
    summary = {"scenario": "*", "result": overall, "truth": truth, "wall_s": wall,
               "steps_ms": sum(int(row.get("steps_ms") or 0) for row in rows) or ""}
    record(journey, build_name, run_name, rows, summary, expected)
    for row in rows:
        detail = " at %s" % row["failed_step"] if row.get("failed_step") else ""
        if row["result"] == "KNOWN":
            detail += " (%s)" % ", ".join(journey.known[row["failed_step"]])
        timing = " %ss" % row["wall_s"] if row.get("wall_s") else ""
        print("  %s@%s %s %s%s%s" % (journey.id, journey.version, row["scenario"], row["result"], timing, detail))
    return rows, overall


def monacoctl():
    """The built bin/monacoctl, or go run of the source when it is not built."""
    if (ROOT / "bin" / "monacoctl").exists():
        return ["bin/monacoctl"]
    return ["go", "-C", "apps/backend", "run", "./cmd/monacoctl"]


def with_dotenv(args):
    result = sh(["scripts/with-dotenv-local.sh"] + args, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    return result.stdout.strip() if result.returncode == 0 else ""


def actor_user_id(row):
    """The actor's Monaco user id in the local database, or '' before the actor's first sign-in."""
    privy_user_id = row.get("privy_user_id", "")
    if not re.match(r"^did:privy:[a-z0-9]+$", privy_user_id):
        return ""
    query = "SELECT id FROM users WHERE privy_user_id = '%s'" % privy_user_id
    return with_dotenv(["apps/mobile/qa/journeys/psql.sh", "-tAc", query])


def funding_notice(journey, accounts, user_id=actor_user_id):
    """What the person running a money journey must have sent before it starts, or '' for other journeys.
    Each actor is funded from the QA pot with monacoctl qa fund, or from the Phantom agent wallet."""
    if not journey.funds:
        return ""
    lines = ["this journey moves real USDC. Before it starts, fund each actor from one of the two QA wallets "
             "(docs/journeys/README.md, Journeys that move money):"]
    for actor, amount in sorted(journey.funds.items()):
        user = user_id(accounts.get(actor, {})) or "<%s's user id, after its first sign-in>" % actor
        lines.append("  actor %s, %s USDC: scripts/with-dotenv-local.sh bin/monacoctl qa fund --user %s --usdc %s, "
                     "or send %s USDC from the Phantom agent wallet to %s's deposit address"
                     % (actor, amount, user, amount, amount, actor))
    return "\n".join(lines)


def refund_address():
    """{QA.refund_address}: where a money journey sends what is left. An exported MONACO_QA_REFUND_ADDRESS
    (the Phantom agent wallet) wins; otherwise it is the QA pot's address from monacoctl qa pot --address."""
    address = os.environ.get("MONACO_QA_REFUND_ADDRESS") or with_dotenv(monacoctl() + ["qa", "pot", "--address"])
    if not address:
        raise JourneyError("this journey moves real USDC and has no refund address: decrypt .env.local so "
                           "monacoctl qa pot --address works, or export MONACO_QA_REFUND_ADDRESS, the Phantom "
                           "agent wallet's address (docs/journeys/README.md)")
    os.environ["MONACO_QA_REFUND_ADDRESS"] = address
    return address


def stamp():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def pick(journeys, journey_id):
    if journey_id not in journeys:
        raise JourneyError("no journey %r. Known: %s" % (journey_id, ", ".join(sorted(journeys))))
    return journeys[journey_id]


# ---------------------------------------------------------------- commands


def cmd_check(args):
    journeys = load_journeys()
    problems = check_journeys(journeys, load_accounts(), git_apply_check)
    for problem in problems:
        print(problem)
    if problems:
        return 1
    print("%d journey%s checked: docs and tests agree" % (len(journeys), "" if len(journeys) == 1 else "s"))
    return 0


def tsv_rows(path):
    lines = [line.split("\t") for line in path.read_text().splitlines() if line.strip()]
    if not lines:
        return []
    return [dict(zip(lines[0], cells)) for cells in lines[1:]]


def cmd_coverage(args):
    backend = ROOT / "packages" / "flows" / "backend"
    app = ROOT / "packages" / "flows" / "app"
    journeys = load_journeys()
    print("| ID | Flow | Backend | App | Journeys |")
    print("| --- | --- | --- | --- | --- |")
    uncovered = []
    for path in sorted(backend.glob("*.tsv")):
        rows = tsv_rows(path)
        if not rows:
            continue
        row = rows[0]
        flow_id = row.get("id", path.stem)
        app_rows = tsv_rows(app / (flow_id + ".tsv")) if (app / (flow_id + ".tsv")).exists() else []
        app_status = app_rows[0].get("status", "-") if app_rows else "-"
        listed = [item.id for item in journeys.values() if flow_id in item.flows]
        if not listed:
            uncovered.append(flow_id)
        print("| %s | %s | %s | %s | %s |" % (
            flow_id, row.get("flow", "-"), row.get("status", "-"), app_status,
            ", ".join(listed) or "-"))
    print("uncovered: %s" % (", ".join(uncovered) or "-"))
    return 0


def cmd_list(args):
    for journey in load_journeys().values():
        requires = " requires " + ", ".join(journey.requires) if journey.requires else ""
        print("%s@%s  %s  actors %s  %s%s" % (journey.id, journey.version, journey.meta.get("milestone", ""),
                                             ",".join(journey.actors), " ".join(journey.scenarios), requires))
    return 0


EXIT_CODES = {"PASS": 0, "FAIL": 1, "TIMEOUT": 1, "ERROR": 2}


def cmd_run(args):
    if args.all == bool(args.journey):
        raise JourneyError("name one journey, or pass --all")
    if args.all and args.scenario:
        raise JourneyError("--scenario names one journey's scenarios: drop it with --all")
    builder = once_builder(args)
    with simulator_shutdown(args.keep_sims), journey_backend() as api_base_url:
        if not args.all:
            return run_journey(args, api_base_url, args.journey, builder)
        return run_all(args, api_base_url, builder)


def once_builder(args):
    """Builds at most once per invocation, and not at all when the stamp matches or --no-build."""
    done = []

    def builder(sim):
        if not args.no_build and not done:
            ensure_build(sim, OUT / "build.log", args.rebuild)
        done.append(sim)
    return builder


def requires_order(journeys):
    """Journey ids with each after the journeys it requires, ties by id."""
    by_id = {journey.id: journey for journey in journeys.values()}
    waiting = {key: {r for r in journey.requires if r in by_id} for key, journey in by_id.items()}
    ready = [key for key, needs in waiting.items() if not needs]
    heapq.heapify(ready)
    order = []
    while ready:
        current = heapq.heappop(ready)
        order.append(current)
        for key, needs in waiting.items():
            if current in needs:
                needs.discard(current)
                if not needs:
                    heapq.heappush(ready, key)
    return order


def run_all(args, api_base_url, builder):
    """Every journey in requires order on one build and one backend. Returns the worst exit code."""
    journeys = load_journeys()
    worst = 0
    for journey_id in requires_order(journeys):
        if journeys[journey_id].funds:
            try:
                refund_address()
            except JourneyError:
                print("%s SKIP funds" % journey_id)
                continue
        print("== %s" % journey_id, flush=True)
        try:
            code = run_journey(args, api_base_url, journey_id, builder)
        except JourneyError as error:
            print("error: %s: %s" % (journey_id, error), file=sys.stderr)
            code = 2
        worst = max(worst, code)
    return worst


def run_journey(args, api_base_url, journey_id, builder):
    journeys = load_journeys()
    journey = pick(journeys, journey_id)
    scenarios = args.scenario or journey.scenarios
    unknown = [s for s in scenarios if s not in journey.scenarios]
    if unknown:
        raise JourneyError("%s has no scenario %s" % (journey.id, ", ".join(unknown)))
    accounts = load_accounts()
    mapping = dict(pair.split("=", 1) for pair in args.sim)
    sims = resolve_simulators(journey, mapping)
    check_simulator_api_environment(sims, api_base_url)
    OUT.mkdir(parents=True, exist_ok=True)
    funding = funding_notice(journey, accounts)
    refund = refund_address() if funding else ""
    if funding:
        print(funding)
    builder(sims[journey.actors[0]])
    worst = 0
    for index in range(1, args.runs + 1):
        run_name = "%s-%s-%d" % (stamp(), journey.id.replace("/", "-"), index)
        print("run %d of %d" % (index, args.runs))
        reset_journey_simulators(journey, sims, set(mapping), args.fresh)
        _, overall = run_once(journey, journeys, args, sims, accounts, build_label(), run_name, scenarios, api_base_url)
        worst = max(worst, EXIT_CODES[overall])
    if funding:
        print("this journey moved real USDC: cash out what is left and withdraw it to %s, the run's "
              "{QA.refund_address}" % refund)
    return worst


def cmd_mutants(args):
    with simulator_shutdown(args.keep_sims), journey_backend() as api_base_url:
        return run_mutants(args, api_base_url)


def run_mutants(args, api_base_url):
    journeys = load_journeys()
    journey = pick(journeys, args.journey)
    all_patches = journey.mutants()
    patches = [p for p in all_patches if not args.only or p.stem in args.only]
    old_handlers = {signum: signal.signal(signum, lambda signum, frame: (_ for _ in ()).throw(KeyboardInterrupt()))
                    for signum in (signal.SIGTERM, signal.SIGHUP)}
    try:
        for patch in all_patches:
            if sh(["git", "apply", "-R", "--check", str(patch)]).returncode == 0:
                sh(["git", "apply", "-R", str(patch)], check=True)
                print("reverted a seeded bug left applied by an earlier run: %s" % patch.stem)
        if not patches:
            raise JourneyError("%s has no seeded bugs under %s.mutants/" % (journey.id, journey.id))
        if sh(["git", "diff", "--quiet", "--", "apps/mobile/Monaco", "packages/mobile-core"]).returncode != 0:
            raise JourneyError("the app sources have uncommitted changes: commit or set them aside before seeding bugs")
        accounts = load_accounts()
        mapping = dict(pair.split("=", 1) for pair in args.sim)
        sims = resolve_simulators(journey, mapping)
        check_simulator_api_environment(sims, api_base_url)
        caught = 0
        for patch in patches:
            expected_fail = mutant_expectation(patch)
            if not expected_fail:
                print("skipped %s: no expect-fail line" % patch.stem)
                continue
            print("seeded bug %s: %s must fail" % (patch.stem, ", ".join(expected_fail)))
            applied = False
            try:
                reset_journey_simulators(journey, sims, set(mapping), False)
                sh(["git", "apply", str(patch)], check=True)
                applied = True
                build(sims[journey.actors[0]], OUT / ("build-%s.log" % patch.stem))
                run_name = "%s-%s-%s" % (stamp(), journey.id.replace("/", "-"), patch.stem)
                rows, _ = run_once(journey, journeys, args, sims, accounts, build_label(patch.stem), run_name,
                                   expected_fail, api_base_url, expected=dict({s: "FAIL" for s in expected_fail}, **{"*": "FAIL"}))
            finally:
                if applied:
                    sh(["git", "apply", "-R", str(patch)], check=True)
                    with contextlib.suppress(FileNotFoundError):
                        stamp_file().unlink()
            if all(row["result"] == "FAIL" for row in rows):
                caught += 1
                print("  caught")
            else:
                print("  MISSED: the test did not fail on a build with this bug in")
    finally:
        try:
            if "sims" in locals():
                build(sims[journey.actors[0]], OUT / "build.log")
        finally:
            for signum, handler in old_handlers.items():
                signal.signal(signum, handler)
    print("caught %d of %d seeded bugs" % (caught, len(patches)))
    return 0 if caught == len(patches) else 1


def summarize(rows):
    """Per journey: speed on clean builds, flake rate, catch rate and false passes."""
    groups = {}
    for row in rows:
        groups.setdefault((row["journey"], row["driver"]), []).append(row)
    table = []
    for (journey, driver), group in sorted(groups.items()):
        on_clean_build = [r for r in group if r["scenario"] == "*" and "+mutant:" not in r["build"]]
        # A run whose tests never started (the runner or the simulator fell over) says nothing
        # about the journey: it is counted on its own, not as a flake.
        clean = [r for r in on_clean_build if r["result"] != "ERROR"]
        passed = [r for r in clean if r["result"] == "PASS"]
        seeded = {}
        for row in group:
            if row["scenario"] != "*" and "+mutant:" in row["build"]:
                seeded.setdefault(row["build"], []).append(row)
        caught = [rows for rows in seeded.values() if all(row["result"] == "FAIL" for row in rows)]
        truth_misses = [r for r in clean if r["result"] == "PASS" and r["truth"] == "fail"]

        def median(column, source=passed):
            values = [float(r[column]) for r in source if r.get(column) not in ("", None)]
            return statistics.median(values) if values else None

        table.append({
            "journey": journey, "driver": DRIVER, "runs": len(clean), "errors": len(on_clean_build) - len(clean),
            "flake": (len(clean) - len(passed)) / float(len(clean)) if clean else None,
            "wall_s": median("wall_s"), "steps_s": (median("steps_ms") or 0) / 1000.0 or None,
            "seeded": len(seeded), "caught": len(caught),
            "false_passes": len(seeded) - len(caught) + len(truth_misses),
        })
    return table


def latest_outcomes(rows, known):
    """Per journey, its last clean run: passing scenarios, known failures with tickets, fixed ones and new failures."""
    last = {}
    for row in rows:
        if row["scenario"] == "*" and "+mutant:" not in row["build"]:
            last[row["journey"]] = row["run"]
    table = []
    for journey_id, run in sorted(last.items()):
        item = {"journey": journey_id, "run": run, "passing": [], "known": [], "fixed": [], "new": []}
        tickets = known.get(journey_id, {})
        for row in rows:
            if row["journey"] != journey_id or row["run"] != run or row["scenario"] == "*":
                continue
            if row["result"] == "PASS":
                item["passing"].append(row["scenario"])
            elif row["result"] == "KNOWN":
                item["known"].append("%s at %s (%s)" % (
                    row["scenario"], row["failed_step"], ", ".join(tickets.get(row["failed_step"], ())) or "no ticket"))
            elif row["result"] == "FIXED":
                item["fixed"].append(row["scenario"])
            else:
                item["new"].append("%s %s at %s" % (row["scenario"], row["result"], row["failed_step"] or "-"))
        table.append(item)
    return table


def cmd_report(args):
    if not RESULTS.exists():
        raise JourneyError("no results yet: run a journey first")
    lines = RESULTS.read_text().splitlines()
    rows = [dict(zip(lines[0].split("\t"), line.split("\t"))) for line in lines[1:]]

    def show(value, pattern="%.1f"):
        return "-" if value is None else pattern % value

    print("| Journey | Runs | Did not start | Flake rate | Median wall s | Median steps s | Caught | False passes |")
    print("| --- | --- | --- | --- | --- | --- | --- | --- |")
    for item in summarize(rows):
        caught = "%d of %d" % (item["caught"], item["seeded"]) if item["seeded"] else "-"
        flake = "-" if item["flake"] is None else "%d%%" % round(item["flake"] * 100)
        print("| %s | %d | %d | %s | %s | %s | %s | %d |" % (
            item["journey"], item["runs"], item["errors"], flake, show(item["wall_s"]), show(item["steps_s"]), caught,
            item["false_passes"]))
    known = {journey_id: loaded.known for journey_id, loaded in load_journeys().items()}
    print()
    print("| Journey | Last run | Passing | Known failing | Fixed | New failures |")
    print("| --- | --- | --- | --- | --- | --- |")
    for item in latest_outcomes(rows, known):
        print("| %s | %s | %s |" % (item["journey"], item["run"], " | ".join(
            ", ".join(item[key]) or "-" for key in ("passing", "known", "fixed", "new"))))
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command")
    commands.required = True
    commands.add_parser("check").set_defaults(run=cmd_check)
    commands.add_parser("coverage").set_defaults(run=cmd_coverage)
    commands.add_parser("list").set_defaults(run=cmd_list)
    commands.add_parser("report").set_defaults(run=cmd_report)
    for name, handler in (("run", cmd_run), ("mutants", cmd_mutants)):
        sub = commands.add_parser(name)
        sub.set_defaults(run=handler)
        sub.add_argument("journey", nargs="?" if name == "run" else None, help="journey id, such as auth/sign-in")
        sub.add_argument("--sim", action="append", default=[], metavar="ACTOR=UDID",
                         help="the simulator an actor uses; defaults to Monaco Journeys [<lane>] <actor>")
        sub.add_argument("--channel", choices=("sms", "email"), default="sms")
        sub.add_argument("--timeout", type=int, default=300, metavar="SECONDS",
                         help="budget for one journey run: setup, tests and truth check (default 300)")
        sub.add_argument("--keep-sims", action="store_true",
                         help="leave the simulators this run booted running, for debugging")
        if name == "run":
            sub.add_argument("--scenario", action="append", metavar="S1", help="default: every scenario")
            sub.add_argument("--runs", type=int, default=1)
            sub.add_argument("--all", action="store_true", help="every journey, in requires order, on one build")
            sub.add_argument("--no-build", action="store_true", help="reuse the last build")
            sub.add_argument("--rebuild", action="store_true", help="build even when the stamp matches")
            sub.add_argument("--fresh", action="store_true", help="also reinstall the app on --sim simulators")
        else:
            sub.add_argument("--only", action="append", metavar="NAME", help="a seeded bug's file name, no .patch")
    args = parser.parse_args(argv)
    try:
        return args.run(args)
    except JourneyError as error:
        print("error: %s" % error, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
