# Semantic wave merge projector v1

## Purpose

The projector serializes Gooo self-improvement proposals, not generic tasks.
Each proposal is an immutable envelope with an identity, ledger base, semantic
read and write sets, evidence digests, tool release locks, causal
dependencies, and an authority scope. The output is a proposed semantic wave;
it never performs a Git merge or commit.

## Contract ownership

The released `.gooo` graph is the only authority for fields, states, state
precedence, allowed authority, exact denominator, proof choices, indicator
classes, fixture expectations, and artifact names. The Go implementation does
not define a second denominator or transition table.

The denominator contains exactly twelve named immutable conditions. Four use
the `FOUNDATION` proof choice, four use `COHERENCE`, and four use `REGRESSION`.
The indicator vector contains four `DRIVER`, four `OUTCOME`, and four
`GUARDRAIL` conditions. These are direct integer counts; the product exposes
no aggregate score or percentage.

## Projection rules

1. All envelopes in one accepted wave must name the released target base.
2. Every required field must be present and every proposal ID must be unique.
3. Every required evidence digest must be present in the fixture evidence set.
4. Every tool lock must be immutable and verified.
5. Every dependency must resolve to a proposal in the envelope set.
6. The dependency graph must be acyclic and is topologically ordered with a
   proposal-ID tie-breaker.
7. A read-after-write overlap is serializable when the writer causally
   precedes the reader.
8. Write/write overlap and unordered read/write overlap are refuted.
9. Requested authority must be a subset of the released authority scope.

Missing or not-yet-verifiable conditions are `UNKNOWN`; contradictory,
malformed, or unsafe conditions are `REFUTED`. The priority is exactly
`REFUTED > UNKNOWN > CLOSED`. An unknown projection always carries
`stage`, `step`, `reason`, `unknown_class`, `next_operation`, and `blocked_by`.

## Effects and replay

The runtime accepts only an absolute empty output directory outside the source
repository and writes exactly six caller-owned artifacts. It computes the
projection twice, with normal and reversed input order, after canonical
sorting. A digest mismatch is `REFUTED` with a replay witness. No Git merge,
commit, repository write, local validation, or cross-project required gate is
performed by the product runtime.
