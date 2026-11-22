# PoNA

Proof of Network Activity — the RUNE block-reward engine.

| Doc | Contents | Status |
|---|---|---|
| [`pona.md`](./pona.md) | PoNA mechanism and module — **ten bands**, tier rates, dPona summary | Implemented |
| [`dpona.md`](./dpona.md) | **dPona** — ten-band ladder, dynamic difficulty controller, Tier 4 fee fallback | **Designed, not implemented** |
| [`original-design.md`](./original-design.md) | The original **Enhancement 3** design record, with a corrections log | Historical |

## Reading order

1. Start with [`pona.md`](./pona.md) §1–2 to understand why PoNA exists and how a
   block is classified.
2. Read [`dpona.md`](./dpona.md) for the redesign that is being implemented.
3. [`original-design.md`](./original-design.md) is for provenance only — it is
   the concept as first proposed, preserved verbatim, and is **not** the spec.

## Credits

The original PoNA Enhancement 3 concept — including the ten-band ladder, the
`quint/quart/trio/duo` exact-count bands, and the dPona dynamic-difficulty
controller — was designed by **RZ | ROD ZISSO (Muroko)**.

It is an original design conceived from scratch, not derived from or based on any
other project or protocol.
