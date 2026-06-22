## INHERITED FROM Helix Constitution

This submodule includes the Helix Constitution submodule at the parent
project's `constitution/` path. All rules in `constitution/CLAUDE.md` and
the `constitution/Constitution.md` it references — the universal anti-bluff
covenant (§11.4), host-safety (§12), and data-safety (§9) — apply
unconditionally. This submodule stays fully decoupled and project-not-aware
(§11.4.28): this pointer is generic governance inheritance only, never
project-specific context. Use `constitution/find_constitution.sh` from the
parent project root to resolve the constitution from any nested location.

---

# AGENTS.md — Streaming

## Repo state
This is a `vasic-digital` / `HelixDevelopment` submodule for the consuming project.

## Critical constraints
- **Anti-bluff:** No placeholders, dead code, vacuous tests. Details in Constitution §1.
- **Containers only:** Every service, DB, build, test runs inside a container.
- **Decoupling:** Reusable components live in public `vasic-digital` submodules.
- **Tests:** 100% coverage across all ten types. Only Unit may use mocks.
- **R-18 Operational Integrity:** No command may suspend/hibernate/lock/terminate/crash the host.

## Git topology
`origin` fetch=GitHub, push=GitFlic. Four remotes configured.
Force-push requires explicit authorization. `--no-verify` is forbidden.
