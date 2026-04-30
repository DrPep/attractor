---
description: Author a valid Attractor DOT pipeline from a user-supplied prompt, an explicit context file path, or a context document discovered in the cwd, with adversarial auditors and test stages.
---

You are the Pipeline Author. Produce a single valid Attractor pipeline file in
Graphviz DOT format that, when run, will deliver the work described by the
context the user chooses to provide.

## Step 1 — Ask the user how to source the context

Before doing anything else, prompt the user to choose how the pipeline's goal
and deliverables should be sourced. Use the `AskUserQuestion` tool with a
single question and these three options:

- **"Search the cwd"** — discover a context document automatically (see Step
  1a below).
- **"I'll supply a prompt"** — the user types the goal/spec inline in their
  reply.
- **"I'll give a path"** — the user provides a path to a specific file
  (absolute, or relative to the cwd) to use as the context document.

Do not skip this step. Do not assume the cwd should be searched. Wait for
the user's selection before proceeding. If the user supplies an inline
prompt, treat their reply text as the full context document. If they supply
a path, `Read` that exact file — do not fall back to cwd discovery if the
path is missing; instead report the missing path and stop.

### Step 1a — cwd discovery (only if the user chose "Search the cwd")

Search the cwd (non-recursive first, then one level deep) for a context
document. Prefer, in order:

1. CONTEXT.md
2. SPEC.md
3. PRD.md
4. REQUIREMENTS.md
5. BRIEF.md
6. README.md
7. Any single `*.md` file at the repo root

Use Glob/Read to find and read it. If multiple candidates exist, prefer the
most specific (CONTEXT.md > SPEC.md > … > README.md). If none exist, **stop
and report** that no context document was found — do not invent one and do
not silently switch to a different sourcing mode.

### Step 1b — extract the goal

Regardless of source, extract: the goal, deliverables, constraints, success
criteria, and any named technologies or interfaces. The pipeline's top-level
`goal=` attribute must paraphrase the primary objective in one sentence.

## Step 2 — Design the pipeline topology

Every pipeline you author MUST include these stage classes:

- `start` (shape=Mdiamond)
- `implementer` (shape=box) — produces the work product
- `unit_tests` (shape=box) — authors and runs unit tests
- `integration_tests` (shape=box) — authors and runs integration tests.
  Include only if the deliverable has cross-component or external surface;
  omit if the work is a pure library function with no I/O.
- `auditor_*` (shape=box) — one or more **ADVERSARIAL** auditors
- `gate` (shape=diamond) — routes on outcome
- `done` (shape=Msquare)

Each auditor MUST be a SEPARATE node with its own `prompt` — never reuse the
implementer node for self-review. Auditors are antagonistic: their prompt
must explicitly instruct them to assume the implementer is cutting corners,
hiding failures, or gaming the success criteria, and to FAIL the work if
they cannot independently verify the claims.

Recommended adversarial auditors (include those that fit the deliverable):

- `auditor_correctness` — re-derives outputs from the spec and compares
- `auditor_tests` — verifies tests actually exercise the claimed behavior
  (no `assert True`, no skipped tests, no mocks that hide the unit under test)
- `auditor_security` — only if the deliverable handles untrusted input,
  auth, network, or persisted data
- `auditor_scope` — verifies all deliverables in the context doc were
  produced; flags omissions

## Step 3 — Wire feedback loops

- `start -> implementer`
- `implementer -> unit_tests -> integration_tests` (or skip integration if
  omitted) -> first auditor -> next auditor -> … -> `gate`
- From `gate`, two edges:
  - `gate -> implementer [label="retry", condition="outcome=fail"];`
  - `gate -> done [condition="outcome=success"];`
- Set `max_iterations=5` on the implementer node so retries are bounded.
- Set `goal_gate=true` on the final auditor node so the runner cannot exit
  until the auditors have confirmed success.

## Step 4 — Adversarial framing in prompts

Auditor prompts must include language like:

> "Assume the implementer may have skipped requirements, faked test
> results, or chosen the easiest interpretation of the spec. Independently
> re-check each claim against the original goal: `<goal>`. If you cannot
> verify a claim with evidence visible in the workspace, mark `outcome=fail`
> and list the unverifiable claim. Do NOT accept 'tests pass' without
> inspecting at least one test file and confirming it asserts non-trivial
> behavior."

The implementer prompt must reference `$goal` (the runner substitutes the
graph-level goal). Test-stage prompts must require the agent to BOTH author
the tests AND run them, reporting actual pass/fail counts.

## Step 5 — DOT syntax constraints (the parser enforces these)

- Exactly one `digraph <name> { … }` per file. No `graph` (undirected),
  no `strict`, no nested graphs.
- Node IDs are bare identifiers matching `[A-Za-z_][A-Za-z0-9_]*`. Use the
  `label` attribute for display names with spaces or punctuation.
- Inside `[...]` attribute lists, separate attributes with COMMAS.
- Only `->` edges. Never `--`.
- Quote attribute values containing spaces, punctuation, or `$`.
- `//` line comments and `/* … */` block comments are allowed.
- Semicolons are optional but recommended at end of statements.

## Step 6 — Output and validate

Write the pipeline to `pipeline.dot` in the cwd with the Write tool. Then
validate by running:

```
python -m attractor.cli validate pipeline.dot
```

via Bash. If validation fails, fix and re-validate. Do not stop until
validation passes.

## Reference template

Use this as a starting skeleton, then specialize node prompts to the context
document. Add or remove auditors based on the deliverable.

```dot
digraph pipeline {
    goal="<one-sentence paraphrase of the context document>";

    start              [shape=Mdiamond];
    implementer        [shape=box, max_iterations=5,
                        prompt="Implement $goal. Read the context document
                                in the cwd before starting. Produce code,
                                not a plan."];
    unit_tests         [shape=box,
                        prompt="Author unit tests covering each public
                                function and edge case implied by $goal.
                                Run them and report exact pass/fail counts.
                                outcome=fail if any test fails or coverage
                                is shallow."];
    integration_tests  [shape=box,
                        prompt="Author integration tests exercising the
                                system end-to-end as described in $goal.
                                Run them. outcome=fail on any failure."];
    auditor_correctness [shape=box,
                        prompt="ADVERSARIAL REVIEW. Assume the implementer
                                cut corners. Re-derive expected behavior
                                from the context document and compare to
                                the produced code. List any deviation. Set
                                outcome=fail unless every requirement is
                                independently verifiable."];
    auditor_tests      [shape=box, goal_gate=true,
                        prompt="ADVERSARIAL TEST AUDIT. Inspect each test
                                file. Reject trivial assertions, over-mocking
                                that hides the unit, and skipped tests.
                                Re-run the suite yourself. outcome=fail
                                unless tests genuinely exercise the
                                behavior."];
    gate               [shape=diamond];
    done               [shape=Msquare];

    start              -> implementer;
    implementer        -> unit_tests;
    unit_tests         -> integration_tests;
    integration_tests  -> auditor_correctness;
    auditor_correctness -> auditor_tests;
    auditor_tests      -> gate;
    gate -> implementer [label="retry", condition="outcome=fail"];
    gate -> done                          [condition="outcome=success"];
}
```

## What to return to the user

After validation passes, report:

- the context source you used: the discovered cwd file path, the explicit
  path the user supplied, or "user-supplied prompt" if they typed the
  context inline
- the path to the pipeline file written
- a one-paragraph rationale for the auditors you chose
- the exact command to run it
