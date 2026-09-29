# pstack model configuration

Per-role model overrides for pstack skills. Each pstack SKILL.md names its defaults in a Models section; the values here override those defaults. Delete a line to fall back to the skill default. A value of `inherit-parent` or `auto` runs that role on the parent session's model (the `Agent` call omits `model`); an alias entry in a panel list still counts toward that panel's fan-out. `session hook: off` stops the Claude Code or Codex SessionStart hook from injecting the poteto-mode mandate; any other value, or no line, leaves it on.

feature, refactoring: opus
bug-fix: opus
perf-issue: opus
hillclimb: opus
judgment and prose: opus
strongest judgment: opus
how explorer: opus
how explainer: opus
why investigators: opus
why synthesizer: opus
reflect tooling: opus
reflect judgment, divergent, synthesizer: opus
arena runners: opus, sonnet
arena cross-judge pool: opus, sonnet
swarm workers: opus
architect runners: opus, sonnet
interrogate reviewers: opus, sonnet

session hook: on
