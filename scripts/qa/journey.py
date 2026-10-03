#!/usr/bin/env python3
"""Check, run and measure the app journeys in docs/journeys.

  journey.py check                          docs and their XCUITest files agree
  journey.py list                           every journey, its version and scenarios
  journey.py run <journey> [options]           run a journey's XCUITest and record the result
  journey.py mutants <journey> [options]       run a journey against its seeded bugs
  journey.py report                         speed and correctness per journey

The rules the checks enforce are in docs/journeys/README.md. Results go to
.logs/qa/journeys/results.tsv, one row per scenario and one `*` row per run.
"""

import argparse
import datetime
import json
import os
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

    def driver_files(self):
        return [ROOT / p for p in self.xcuitest]

    def mutants(self):
        folder = QA / (self.id + ".mutants")
        return sorted(folder.glob("*.patch")) if folder.is_dir() else []

    def truth_script(self):
        script = QA / (self.id + ".truth.sh")
        return script if script.exists() else None


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
        for field in ("phone", "email", "code"):
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
                bad(patch, "does not apply to this checkout (git apply --check)")

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


def sh(args, **kwargs):
    return subprocess.run(args, cwd=str(ROOT), universal_newlines=True, **kwargs)


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


def resolve_simulators(journey, mapping):
    sims = dict(mapping)
    missing = [actor for actor in journey.actors if actor not in sims]
    if missing:
        devices = simulator_devices()
        device_type, runtime = simulator_template(devices)
        for actor in missing:
            name = "Monaco Journeys %s" % actor
            sims[actor] = named_simulator(devices, name)
            if not sims[actor]:
                sims[actor] = sh(["xcrun", "simctl", "create", name, device_type, runtime],
                                 stdout=subprocess.PIPE, check=True).stdout.strip()
                print("created simulator %s" % name)
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
        value = sh(["xcrun", "simctl", "getenv", udid, "MONACO_API_BASE_URL"], stdout=subprocess.PIPE,
                   stderr=subprocess.DEVNULL).stdout.strip()
        if value and value != api_base_url:
            name = names.get(udid, udid)
            raise JourneyError(
                "simulator %s has MONACO_API_BASE_URL=%s in its environment, which overrides the journey's API. "
                "Use another simulator with --sim, or clear it with: xcrun simctl spawn %s launchctl unsetenv "
                "MONACO_API_BASE_URL" % (name, value, udid))


def reset_journey_simulators(journey, sims, explicit_actors, fresh):
    """Uninstall the app from dedicated simulators before one journey run."""
    targets = [
        (actor, sims[actor]) for actor in journey.actors
        if fresh or actor not in explicit_actors
    ]
    names = simulator_names(dict(targets))
    for actor, udid in targets:
        sh(["xcrun", "simctl", "boot", udid], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        sh(["xcrun", "simctl", "bootstatus", udid, "-b"], check=True,
           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
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


def openapi_output(derived=None):
    derived = derived or DERIVED
    return derived / "Build" / "Intermediates.noindex" / "BuildToolPluginIntermediates" / "mobile-core.output" / "MonacoAPI" / "OpenAPIGenerator" / "GeneratedSources"


def seed_openapi_output(derived=None):
    output = openapi_output(derived)
    expected = [output / name for name in ("Types.swift", "Client.swift", "Server.swift")]
    if all(path.exists() for path in expected):
        return False
    package = ROOT / "packages" / "mobile-core"
    sh([
        "swift", "build", "--package-path", str(package),
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    generators = list((package / ".build").rglob("swift-openapi-generator-tool"))
    if not generators:
        raise JourneyError("could not build swift-openapi-generator-tool")
    output.mkdir(parents=True, exist_ok=True)
    sh([
        str(generators[0]), "generate", str(package / "Sources" / "MonacoAPI" / "openapi.yaml"),
        "--config", str(package / "Sources" / "MonacoAPI" / "openapi-generator-config.yaml"),
        "--output-directory", str(output),
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (output / "Server.swift").touch()
    print("seeded OpenAPI plugin output because #1955 skips the Xcode plugin command")
    return True


def build(sim, log):
    """Build the app and the UI tests once."""
    sh(["scripts/ensure-ios-privy-config.sh", "generate"], check=True, stdout=subprocess.DEVNULL)
    seeded = seed_openapi_output()
    print("building (log: %s)" % os.path.relpath(str(log), str(ROOT)))
    with open(str(log), "w") as out:
        code = sh(xcodebuild(sim, "build-for-testing"), stdout=out, stderr=subprocess.STDOUT).returncode
    if seeded and code != 0 and "Build input files cannot be found" in log.read_text():
        with open(str(log), "a") as out:
            code = sh(xcodebuild(sim, "build-for-testing"), stdout=out, stderr=subprocess.STDOUT).returncode
    if code != 0:
        raise JourneyError("the build failed, see %s" % log)


def actor_environment(accounts, channel, prefix=""):
    env = {prefix + "MONACO_QA_JOURNEYS": "1", prefix + "MONACO_QA_CHANNEL": channel}
    for actor, row in accounts.items():
        for field in ("phone", "email", "code"):
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


def run_xcuitest(journey, scenarios, sims, accounts, channel, run_dir, api_base_url):
    """One row per scenario. A scenario passes when every phase's test passed and none skipped.

    One-actor scenarios share one xcodebuild call, because starting the test runner costs more
    than the tests. A scenario with phases runs them one call at a time, each on its actor's
    simulator. Returns (rows, wall seconds of every call).
    """
    handoff = run_dir / "handoff.json"
    plan = {scenario: xcuitest_phases(journey, scenario) for scenario in scenarios}
    for scenario, phases in plan.items():
        if not phases:
            raise JourneyError("%s has no test for %s" % (journey.id, scenario))
    single = [s for s in scenarios if len(plan[s]) == 1 and plan[s][0][1] == journey.actors[0]]
    calls = [(journey.actors[0], [plan[s][0][2] for s in single])] if single else []
    for scenario in scenarios:
        if scenario not in single:
            calls.extend((actor, [test]) for _, actor, test in plan[scenario])

    outcomes, wall = {}, 0.0
    log = run_dir / "xcuitest.log"
    with open(str(log), "w") as out:
        for actor, tests in calls:
            env = dict(os.environ)
            env.update(actor_environment(accounts, channel, prefix="TEST_RUNNER_"))
            env["TEST_RUNNER_MONACO_QA_ACTOR"] = actor
            env["TEST_RUNNER_MONACO_QA_HANDOFF"] = str(handoff)
            env["TEST_RUNNER_MONACO_QA_API_BASE_URL"] = api_base_url
            only = ["-only-testing:MonacoUITests/%s" % test for test in tests]
            started = time.time()
            done = sh(xcodebuild(sims[actor], *(only + ["test-without-building"])),
                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=env)
            wall += time.time() - started
            out.write(done.stdout)
            outcomes.update(split_by_test(done.stdout))

    rows = []
    step_lines = []
    for scenario in scenarios:
        result, seconds, steps_ms, failed_step = "PASS", 0.0, 0, ""
        for _, _, test in plan[scenario]:
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
    return rows, "%.1f" % wall


def run_truth(journey, accounts, channel):
    script = journey.truth_script()
    if not script:
        return "none"
    env = dict(os.environ)
    env.update(actor_environment(accounts, channel))
    return "ok" if sh([str(script)], env=env).returncode == 0 else "fail"


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
                    "expected": (expected or {}).get(row["scenario"], "FAIL" if expected and row["scenario"] == "*" else "PASS")}
            full.update(row)
            out.write("\t".join(str(full.get(column, "")).replace("\t", " ") for column in COLUMNS) + "\n")


def run_once(journey, journeys, args, sims, accounts, build_name, run_name, scenarios, api_base_url, expected=None):
    run_dir = OUT / run_name
    run_dir.mkdir(parents=True, exist_ok=True)
    rows, wall = run_xcuitest(journey, scenarios, sims, accounts, args.channel, run_dir, api_base_url)
    truth = run_truth(journey, accounts, args.channel)
    for row in rows:
        row["truth"] = truth
    verdicts = [row["result"] for row in rows]
    overall = "ERROR" if "ERROR" in verdicts else ("FAIL" if "FAIL" in verdicts else "PASS")
    summary = {"scenario": "*", "result": overall, "truth": truth, "wall_s": wall,
               "steps_ms": sum(int(row.get("steps_ms") or 0) for row in rows) or ""}
    record(journey, build_name, run_name, rows, summary, expected)
    for row in rows:
        detail = " at %s" % row["failed_step"] if row.get("failed_step") else ""
        timing = " %ss" % row["wall_s"] if row.get("wall_s") else ""
        print("  %s@%s %s %s%s%s" % (journey.id, journey.version, row["scenario"], row["result"], timing, detail))
    return rows, overall


def funding_notice(journey):
    """What the person running a money journey must have sent before it starts, or '' for other journeys."""
    if not journey.funds:
        return ""
    amounts = ", ".join("%s USDC to actor %s" % (amount, actor) for actor, amount in sorted(journey.funds.items()))
    return ("this journey moves real USDC. Before it starts, send %s from the Phantom agent wallet to the actor's "
            "deposit address (docs/journeys/README.md, Journeys that move money)" % amounts)


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


def cmd_list(args):
    for journey in load_journeys().values():
        requires = " requires " + ", ".join(journey.requires) if journey.requires else ""
        print("%s@%s  %s  actors %s  %s%s" % (journey.id, journey.version, journey.meta.get("milestone", ""),
                                             ",".join(journey.actors), " ".join(journey.scenarios), requires))
    return 0


def cmd_run(args):
    journeys = load_journeys()
    journey = pick(journeys, args.journey)
    api_base_url = require_backend()
    scenarios = args.scenario or journey.scenarios
    unknown = [s for s in scenarios if s not in journey.scenarios]
    if unknown:
        raise JourneyError("%s has no scenario %s" % (journey.id, ", ".join(unknown)))
    accounts = load_accounts()
    mapping = dict(pair.split("=", 1) for pair in args.sim)
    sims = resolve_simulators(journey, mapping)
    check_simulator_api_environment(sims, api_base_url)
    OUT.mkdir(parents=True, exist_ok=True)
    funding = funding_notice(journey)
    if funding:
        print(funding)
    if not args.no_build:
        build(sims[journey.actors[0]], OUT / "build.log")
    worst = 0
    for index in range(1, args.runs + 1):
        run_name = "%s-%s-%d" % (stamp(), journey.id.replace("/", "-"), index)
        print("run %d of %d" % (index, args.runs))
        reset_journey_simulators(journey, sims, set(mapping), args.fresh)
        _, overall = run_once(journey, journeys, args, sims, accounts, build_label(), run_name, scenarios, api_base_url)
        worst = max(worst, {"PASS": 0, "FAIL": 1, "ERROR": 2}[overall])
    if funding:
        print("this journey moved real USDC: cash out what is left and withdraw it to the Phantom agent wallet")
    return worst


def cmd_mutants(args):
    journeys = load_journeys()
    journey = pick(journeys, args.journey)
    api_base_url = require_backend()
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
        sims = resolve_simulators(journey, dict(pair.split("=", 1) for pair in args.sim))
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
                sh(["git", "apply", str(patch)], check=True)
                applied = True
                build(sims[journey.actors[0]], OUT / ("build-%s.log" % patch.stem))
                run_name = "%s-%s-%s" % (stamp(), journey.id.replace("/", "-"), patch.stem)
                rows, _ = run_once(journey, journeys, args, sims, accounts, build_label(patch.stem), run_name,
                                   expected_fail, api_base_url, expected={s: "FAIL" for s in expected_fail})
            finally:
                if applied:
                    sh(["git", "apply", "-R", str(patch)], check=True)
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
            if row["scenario"] != "*" and row["expected"] == "FAIL":
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
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command")
    commands.required = True
    commands.add_parser("check").set_defaults(run=cmd_check)
    commands.add_parser("list").set_defaults(run=cmd_list)
    commands.add_parser("report").set_defaults(run=cmd_report)
    for name, handler in (("run", cmd_run), ("mutants", cmd_mutants)):
        sub = commands.add_parser(name)
        sub.set_defaults(run=handler)
        sub.add_argument("journey", help="journey id, such as auth/sign-in")
        sub.add_argument("--sim", action="append", default=[], metavar="ACTOR=UDID",
                         help="the simulator an actor uses; defaults to Monaco Journeys <actor>")
        sub.add_argument("--channel", choices=("sms", "email"), default="sms")
        if name == "run":
            sub.add_argument("--scenario", action="append", metavar="S1", help="default: every scenario")
            sub.add_argument("--runs", type=int, default=1)
            sub.add_argument("--no-build", action="store_true", help="reuse the last build")
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
