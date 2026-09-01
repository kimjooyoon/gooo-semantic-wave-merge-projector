# Gooo semantic wave merge projector

This repository is a read-only semantic serializer for Gooo self-improvement
proposal envelopes. It does not merge or commit Git state. It reads immutable
proposal envelopes, projects a deterministic serializable wave, and preserves
the unresolved frontier with evidence-backed witnesses.

The released `.gooo/semantic-wave-merge-projector.gooo` graph owns the proposal
fields, state transitions, authority scope, exact twelve-condition denominator,
proof and indicator vectors, canonical fixtures, and six-artifact output
contract. Go supplies only parser, evaluator, generator, and runtime behavior.

The accepted state is rendered as `MERGEABLE(CLOSED)`. `UNKNOWN` preserves the
six required fields `stage`, `step`, `reason`, `unknown_class`,
`next_operation`, and `blocked_by`. `REFUTED` is reserved for contradictory or
malformed evidence. Precedence is `REFUTED > UNKNOWN > CLOSED`.

Read/write overlap is serializable only when the writer causally precedes the
reader. Write/write overlap, unordered read/write overlap, cycles, and
authority escalation are refuted with deterministic conflict witnesses. A
stale base, missing evidence, or mutable/unverified tool lock is unknown until
the missing condition is resolved.

The runtime requires an absolute empty caller-owned output directory outside
the source repository. It writes exactly six artifacts and never writes the
source repository:

- `wave-projection.json`
- `wave-distribution.json`
- `generated-assertions.json`
- `projection-events.ndjson`
- `replay-receipt.json`
- `report.md`

GitHub Actions is the validation authority. Local test, build, vet, formatting,
shell, actionlint, assertion, generator, and conformance executions are not
part of the release procedure.

The runtime command used by Actions is:

```text
go run ./cmd/projector conformance \
  --source .gooo/semantic-wave-merge-projector.gooo \
  --cases fixtures/cases \
  --output /absolute/path/to/empty/caller-output \
  --root /absolute/path/to/source-repository
```
