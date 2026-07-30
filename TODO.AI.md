# TODO.AI.md

## Pending

- `ConvertHandler`'s explicit-direction regex (`src/instant/convert.go`, `patterns[0]`: `^(?:convert\s+)?(\d+(?:\.\d+)?)\s*([a-zA-Z°]+)\s+(?:to|in|->)\s+([a-zA-Z°]+)$`) matches any `"<number> <word> to <word>"` phrase, not just real unit names — e.g. a query like `"10 things to do"` false-positive-matches as a conversion request (unit="things"→"do") instead of falling through to the normal search/instant-answer flow. Pre-existing bug, not introduced by the imperial-default/global-unit-conversion feature; found incidentally by beta-tester agent during that feature's beta test. Fix: validate the captured `from`/`to` groups against the known unit table before treating the query as a conversion; only treat it as a bare/explicit conversion match if both resolve to a recognized unit.
