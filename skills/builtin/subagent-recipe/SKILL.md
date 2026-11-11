---
name: subagent-recipe
description: "Use when coordinating several subagents on one repo: parallel builds, adversarial reviews, fix loops, and consolidating the branches. Briefs as files, worktrees per agent, reviewer ≠ fixer."
---

# Recipe: coordinating subagents for parallel build, adversarial review, and fix loops

For a coordinating agent on any repo. Written after two full passes on a
multi-tree project (Python API + Vue frontend + Elixir realtime, shared
Postgres); nothing here depends on that stack. Every rule was learned by
getting it wrong at least once.

---

## 0. Your job

You coordinate. You do not write code. You write briefs, launch agents,
read their notes, relay verdicts, make small decisions, escalate large
ones to the owner as a numbered list, and consolidate at the end.

If you find yourself reading source "just to check something" for more
than a few minutes, stop and write a brief instead. If you find yourself
running the test suite, stop; that is the agent's job.

The subagent tool returns immediately; each agent's `message_parent`
wakes you, so end your turn rather than checking on them. Do not poll in
a loop. Do not start speculative work to fill idle time; the owner will
ask why, and they will be right.

---

## 1. Roles

| Role | Count | Does | Never does |
|---|---|---|---|
| Fixer / builder | one per branch | fixes exactly the items in its brief, in its own worktree; writes notes | touches other branches' files; fixes adjacent things it noticed |
| Reviewer | one per page/area | adversarial, cold, read-only; findings with counterexamples | fixes anything |
| Re-reviewer | the SAME reviewer | diffs the fix branch, re-runs its own counterexample, attacks the fix | trusts the fixer's notes |
| Consolidator | one, at the end | merges all branches, dedupes overlap, runs everything, writes the PR text | changes behaviour |

Reviewers and fixers never talk to each other. Every message routes
through you as a file. That separation keeps the reviewer adversarial.

---

## 2. Briefs are files

Write `<work>/fixer_<slug>.md`, then launch with a two-line prompt:
"Your brief is at <path>. `cat` it first and follow it exactly." The
prompt field does not shell-expand; files survive a paused conversation
and a dead subagent.

Two rules that go in every ENV.md, because agents get them wrong:

- **All agent output is a `.md` file on disk** (notes, designs, review
  findings), written incrementally. The chat reply is a two-line pointer
  to the file. A dead or interrupted agent then loses nothing, and the
  coordinator can hand the file to the next agent verbatim.
- **No long sleeps.** Never `sleep` for 5+ minutes waiting on a server,
  a job or a test run; poll in short intervals with a bounded total, or
  use `timeout`. A tool call that blocks for minutes looks identical to a
  hung agent from the outside.

Three layers, so the environment text is written once and never drifts:

**`ENV.md`** (shared by every agent)
- worktree path pattern, branch naming, what is symlinked (venv,
  node_modules, build caches), "never touch the main checkout"
- DB host/creds; **every DB CLI call gets the password in the environment
  and a `timeout`** -- an interactive prompt hangs the tool indefinitely
- per-agent private test database name (set in the worktree's env file)
- rules for the production-shaped DB: SELECT freely; fixtures only
  inside BEGIN...ROLLBACK; never call code paths that commit
- the exact check commands per tree; "run the suites for the trees you
  touched, fully"
- no dev servers, no push, no merge, no PR; how to commit without the
  repo's hook if the worktree lacks its tools
- notes file path and required sections
- owner constraints, **verbatim, in quotes**, so nobody re-litigates

**`<FAMILY>_COMMON.md`** when several tasks are the same shape (four
pages onto one shared stack): the shared rules, the settled shared API,
the sibling branches and which files each owns.

**`fixer_<slug>.md`**: the items, with finding IDs quoted and file:line;
what "done" looks like; what is NOT in scope; sibling branches to avoid.

---

## 3. Worktrees and branches

One factory script, `mkwt.sh <slug> [base]`:
```
git fetch -q origin
git worktree add -q <fix-root>/<slug> -b <prefix>/<slug> ${base:-origin/main}
# symlink heavy deps from the main checkout (venv, node_modules, build caches)
# copy the env file; override the test DB name to something private per slug
```

**Choosing the base is a design decision:**
- independent tasks -> `origin/main`
- tasks that need another branch's output -> that branch (pages that use a
  freshly extracted shared component branch from the extraction branch)
- anything launched after a batch has been merged -> the integration
  branch (section 9), so it sees the merged state

Before the first launch: drop leftover test DBs from the last pass;
snapshot row counts on the production-shaped DB (`pg_stat_user_tables`
or equivalent) for the leak check at the end.

---

## 4. Review brief (cold, adversarial)

No PR description, no design doc. "Assume bugs exist. First write one
paragraph of what this does, THEN hunt." Checklist: data grain,
GROUP BY key vs denormalised column, LEFT JOIN fan-out, NULLs, bound
params, dates/timezones, snapshot vs live columns, EXPLAIN the main
query, auth at every layer (nav / route / router / transport), frontend
reuse vs hand-rolled, tests that execute code vs string-match.

Deliverable: verdict line at top; findings ranked blocker / should-fix /
nit, each with file:line and a concrete counterexample; what you ran;
verified-OK list; questions for the author. Hand over the design doc
AFTER for a claims-vs-code table.

---

## 5. Fixer brief

- Finding IDs quoted. "Fix ONLY these."
- One commit per finding (or tightly coupled group). Body: what was
  wrong, how a user sees it, what changed.
- Every correctness fix ships a test that fails before and passes after,
  fixture-backed (executes the real code/SQL), not `assert "<literal>" in
  source`.
- If a finding needs a business decision, list it; do not guess.
- Notes: per finding done / not / why, hash, test name; things seen but
  NOT fixed; check-output summary lines.
- Write notes incrementally and commit as you go, so an interruption
  loses little.

**Cleanup / refactor passes have a different bar and it must be
stated:** "no behaviour change, and prove it." Proofs that worked:
- backend: a capture harness recording every query + bind for N
  representative inputs on the base and on the branch, diffed
  byte-for-byte (the reviewer then ran its own, larger set)
- frontend: a mount test written against the OLD template, committed
  FIRST, then carried unchanged through the rewrite -- the commit order
  is the proof
- "behaviour-preserving" means user-visible features, data, API calls,
  URL params, permissions. It does NOT mean keeping hand-rolled
  substitutes for the project's shared mechanism (a button pager where
  the codebase uses infinite scroll is a hand-rolled substitute, not a
  feature). Ask the owner about the shared mechanism BEFORE briefing.

---

## 6. Re-review brief

Same reviewer. Read the diff cold BEFORE the fixer's notes. For each
in-scope finding: actually fixed (re-run your counterexample)? Does the
new test fail on the base? Attack the fix: new fan-out, new NULL hole,
semantics changed for cases the finding was not about, before/after
plan, a second spelling of a rule, scope creep, changes to shared code
affecting OTHER consumers (grep them all). Mutate two things and confirm
the tests catch it; revert. Say explicitly: "verify by reading the code,
not by trusting the notes."

Verdict: `mergeable` / `mergeable with nits` / `not mergeable: <reason>`.
Round N appends a section; the verdict line at the top is updated.

---

## 7. Loops

- `not mergeable` -> same fixer, reviewer's finding quoted verbatim,
  "fix ONLY F0 and F1; nits are NOT to be fixed" -> same reviewer,
  "verify ONLY that". Three rounds was the most ever needed.
- Owner feedback that reverses a fix -> same fixer, same reviewer, owner
  quoted.
- Nits: accept unless cheap and on a line already touched. Never bounce a
  branch for nits.
- Rewording a wrong commit body is a legitimate round-2 item; the fixer
  rebases its own unpushed branch.

---

## 8. Parallelism and dependencies

3-4 agents at a time (shared DB). High reasoning. Budget ~30-60 min per
fix, ~40 per review; a large rewrite longer.

Disjoint trees (api / frontend / transport) always run in parallel.
Same tree, disjoint files: fine, but name the sibling branches and
their files in each brief.

**Shared code first, then fan out.** If several tasks will need the
same helper or will edit the same shared component, one agent settles
that first and the others branch from its result. Extracting only what
is ALREADY duplicated is not enough -- the parallel agents will each
still need something from the shared stack you did not anticipate, and
three of them will add the same prop to the same component three ways.
Better: the shared-step agent also rebuilds the first, simplest member
of the family, so the shared-stack decisions get made once on a
concrete example; the rest fan out from that.

---

## 9. Merging the parallel work (the integration branch)

This is where parallel work either lands or falls apart. Do it as a
distinct step with its own agent and brief; do not let fixers merge.

**9.1 Maintain an integration branch throughout, not just at the end.**
As soon as two branches are `mergeable`, dry-run merge them:
```
git worktree add <fix-root>/_integ -b <prefix>/integration origin/main
for b in <branches in order>; do
  git merge -q --no-edit $b && echo "ok $b" \
  || { echo "CONFLICT $b: $(git diff --name-only --diff-filter=U)"; git merge --abort; }
done
```
Rebuild it whenever a branch changes (round 2 commits, rebases). Run the
full suite on it, not just on each branch: git merges cleanly things
that are wrong (an import one branch removed that another still uses; a
helper renamed on one side).

**9.2 Merge order.** Cross-cutting branches first (shared helpers,
escaping, base classes), then the branches that depend on them, then
leaf branches. Branches cut from another branch merge after their base.
Record the order.

**9.3 Conflicts.** The consolidator resolves them and records every one
in `consolidation.md`: file, both sides' intent, what won and why. If a
resolution requires judgement about behaviour, it stops and asks you;
you ask the original fixer or the owner.

**9.4 Migrations / numbered artefacts.** Every branch cut from main takes
the same "next" number. The consolidator renumbers or combines into one
per PR, and repoints any test that reads the file by name.

**9.5 Dedupe cross-branch overlap.** Name the known overlaps in the brief
(e.g. "three branches edited ReportHeader: commits X, Y, Z"). After
merging, the consolidator diffs the versions, keeps the union of
behaviour with one prop name/default, confirms every consumer still
passes, and records which version won.

**9.6 Consistency across a family built in parallel.** Table: member x
concern (which header component, which search composable, which state
classes, file split, etc.). Where two members do the same thing two
ways, harmonise to the majority / the established sibling in ONE
commit whose body lists each change -- renames and ordering only. If
harmonising would be a real behaviour change, list it instead.

**9.7 Full verification on the merged tree.** Every suite in every
tree, lint and format checks, and the leak diff against the row-count
snapshot from section 3 (name every table that moved and attribute it;
background jobs on a dev DB will move a few -- say which).

**9.8 PR description.** Title; one paragraph of what the pass is;
**"Visible changes" at the top** -- every user-visible delta, each
already disclosed in a commit body; per-branch sections in plain
language (what was wrong/duplicated, what it is now, line deltas);
"Reviewed" (verdict + round count per branch, pointing at the notes);
"Deferred" (what was punted and why).

**9.9 Push.** Rename the integration branch to the owner's naming
convention, push, open the PR. Check tooling before promising: `gh`
needs an API token, SSH alone will push but not open a PR. Delete the
temporary remote branch if you pushed one.

**9.10 Cleanup.** Remove all worktrees, delete local branches, drop
per-agent test DBs. Leave the review trail on disk unless told.

---

## 10. Interruptions

Subagents keep running while the conversation pauses, and their
worktrees, commits, and incremental notes survive either way. Symptoms
of one that died or stalled: empty response, or a notes file stuck at
"IN PROGRESS". Re-message the SAME slug: "You were
interrupted. Run git status/diff; revert any test mutation you left;
finish; mark what you cannot get to as 'not examined'." Do not re-run
the brief from scratch.

If an agent overruns: "write what you have, mark the rest not
examined". Narrow the brief; do not re-run it.

---

## 11. Coordinator discipline (the ones that bit)

- **Do not do work nobody asked for.** Even if it is on the plan; ask
  before adding a branch to fill idle time.
- **Do not do the agents' work.** Reading source to write a brief: fine.
  Running suites, fixing things yourself: no.
- **Ask about the shared mechanism before briefing a family.**
  "Behaviour-preserving" is about features, not widgets.
- **Keep a numbered A list (what changes) and B list (decisions needed).**
  Owners answer against numbers. Verify a claim before relaying it.
- **Report in human language.** Branch slugs mean nothing to the owner:
  "what got done / what got punted", two sentences per branch.
- **Don't alter production-shaped data unasked.** When you do, say
  exactly what you ran.
- **When a deploy fails after your merge, read the log before assuming
  the code.** (Last time: cluster DNS on the CI runner.)

---

## 12. File layout

```
<work>/                         e.g. /tmp/reviews/<pass>/
  ENV.md                        shared environment + rules
  <FAMILY>_COMMON.md            shared rules for a family of like tasks
  <FAMILY>_REREVIEW.md          shared re-review brief for that family
  mkwt.sh                       worktree factory
  fixer_<slug>.md               per-task brief
  rereview_<slug>.md            per-task re-review brief
  notes_<slug>.md               fixer's deliverable (round sections appended)
  rereview_notes_<slug>.md      reviewer's deliverable (verdict line at top)
  CONSOLIDATE.md                consolidator brief
  consolidation.md              merge order, conflicts, dedupe, consistency table, suite lines, leak diff
  rowcounts_before.txt / rowcounts_after.txt
  PR_DESCRIPTION.md
<fix-root>/<slug>               worktrees (delete after)
<fix-root>/_integ               integration worktree (delete after)
```
