# TODO.AI.md

## Pending

- [ ] Makefile (line 97, `local` target): first recipe line is `@rm -rf` (line 98), not `@mkdir -p $(GO_CACHE) $(GO_BUILD)` — mkdir guard must be the absolute first recipe line per CasjaysDev convention
- [ ] Makefile (line 116, `build` target): first recipe line is `@rm -rf` (line 117), not `@mkdir -p $(GO_CACHE) $(GO_BUILD)` — mkdir guard must be the absolute first recipe line per CasjaysDev convention
