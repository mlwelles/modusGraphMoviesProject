# Implementation Plan

Parallel execution plan for the modusGraphMoviesProject and its three
dependencies. Three subagents run concurrently; a merge phase wires
everything together.

## Execution Model

```
Time ──────────────────────────────────────────────────────►

Subagent A (dgman fork):
  Phase 2.1  Fork + fix write path
  Phase 2.2  Fix read path
  Phase 2.4a dgman tests

Subagent B (modusGraph fork):
  Phase 2.3  Fork + []T slice support + use forked dgman
  Phase 2.4b modusGraph tests ([]T immediately; predicate= e2e after A)

Subagent C (structs + codegen):
  Phase 1.1  Movie structs in this project
  Phase 1.2  modusGraphGen AST parser
  Phase 1.3  Codegen templates
  Phase 1.4  Golden-file tests
  Phase 1.5  CLI generation

Merge (main agent):
  Phase 3.1  Wire go.mod replace directives
  Phase 3.2  Compile check
  Phase 3.3  Docker + data loading
  Phase 3.4  Integration tests
  Phase 3.5  CLI end-to-end
```

### Dependencies

- Subagent C is fully independent.
- Subagent B can start immediately. Its `[]T` work and tests are
  independent of A. The `predicate=` end-to-end tests require A's fixes
  in `../dgman` (via `replace` directive).
- The merge phase requires all three subagents to complete.

---

## Subagent A: dgman Fork

### Phase 2.1: Fork and fix write path

1. Fork `dolan-in/dgman` to `mlwelles/dgman`.
2. Clone to `../dgman`.
3. Fix `filterStruct` in `mutate.go` (around line 231):
   - When `typeCache` has schema info for the struct, use
     `mutateType.schema[i].Predicate` as the map key instead of
     `field.Tag.Get("json")`.
   - Fall back to the json tag for non-dgraph structs (e.g.,
     `time.Time`).
   - Use `schema.OmitEmpty` instead of re-parsing json tag options.

### Phase 2.2: Fix read path

1. Fix query deserialization in `query.go` (around lines 514, 542).
2. Before `json.Unmarshal`, remap JSON keys from predicate names to json
   tag names.
3. Build the `predicate→jsonTag` mapping from the struct's parsed schema
   (already available in `parseDgraphTag`).
4. Apply recursively for nested edge structs.

### Phase 2.4a: dgman fork tests

All tests use structs where `predicate=` differs from the `json` tag.

| Test                         | Description                                  |
|------------------------------|----------------------------------------------|
| MutateBasic round-trip       | Insert, get back, assert field not zero       |
| Mutate/Upsert round-trip    | Same via `do()` path, confirms read fix       |
| Dot-prefixed predicates      | `director.film` round-trips through insert/get|
| Query filter by predicate    | Insert films, filter `ge(release_date, ...)`, assert results |

---

## Subagent B: modusGraph Fork

### Phase 2.3: Fork and modify modusgraph

1. Fork `matthewmcneely/modusgraph` to `mlwelles/modusGraph`.
2. Clone to `../modusGraph`.
3. Update `go.mod` to point to `mlwelles/dgman` (via `replace` directive
   during development). This pulls in both dgman fixes with zero code
   changes in modusgraph.
4. Add `[]T` value-type slice support: make dgman's reflection handle
   `[]Genre` the same way it handles `[]*Genre`. Populate and append by
   value instead of by pointer.

### Phase 2.4b: modusGraph fork tests

| Test                         | Depends on A? | Description                          |
|------------------------------|:---:|----------------------------------------------|
| `[]T` slice round-trip       | No  | Insert `[]Genre`, query back, assert populated. Compare with `[]*Genre`. |
| `predicate=` e2e (Insert)    | Yes | Insert through modusgraph client API, get back, assert correct. |
| `predicate=` e2e (Update)    | Yes | Update, Upsert, Get, Query with predicate= structs. |
| `predicate=` e2e (file://)   | Yes | Same tests on `file://` backend.            |
| `predicate=` e2e (dgraph://) | Yes | Same tests on `dgraph://` backend.          |

---

## Subagent C: Structs + Codegen

### Phase 1.1: Movie structs in this project

1. Create struct files in `movies/`:
   - `film.go`, `director.go`, `actor.go`, `performance.go`
   - `genre.go`, `country.go`, `rating.go`, `content_rating.go`,
     `location.go`
2. Create `movies/generate.go` with `go:generate` directive.
3. Update `go.mod` to full dependency set with `replace` directives.
4. Add `.gitignore` (data/, vendor/, binaries).

### Phase 1.2: modusGraphGen AST parser

1. Create `mlwelles/modusGraphGen` repo, clone to `../modusGraphGen`.
2. Use `go/ast` + `go/parser` to load the target package.
3. Extract exported structs with `json` and `dgraph` tags.
4. Build a model representing:
   - Entities (structs with `UID` + `DType` fields)
   - Relationships (`[]OtherEntity` fields)
   - Searchable fields (`index=fulltext`)
   - Filterable fields (`index=hash`, `index=year`, etc.)
   - Reverse edges (`predicate=~X`)
5. Unit test each inference rule against the movie structs.

### Phase 1.3: Codegen templates

Build `text/template` templates for each generated file:

| Template output              | Contents                                     |
|------------------------------|----------------------------------------------|
| `client_gen.go`              | Client struct, sub-clients, `New(connStr)`   |
| `<entity>_gen.go`            | CRUD + Search + List methods per entity      |
| `<entity>_options_gen.go`    | Functional option types and constructors     |
| `<entity>_query_gen.go`      | Typed query builder with Filter/Order/Exec   |
| `page_options_gen.go`        | Shared PageOption interface, First, Offset   |
| `iter_gen.go`                | Auto-paging iterator (Go 1.23+ range-over-func) |

### Phase 1.4: Golden-file tests

1. Run modusGraphGen against the movie structs from Phase 1.1.
2. Compare generated output against checked-in golden files.
3. Verify generated code compiles (`go build` in test).
4. Test both client library and CLI golden files.

### Phase 1.5: CLI generation

1. Add template for `cmd/<name>/main.go`.
2. Generate a Kong CLI wiring sub-client methods to commands.
3. Golden-file test for CLI output.

---

## Merge Phase

### Phase 3.1: Wire go.mod

Ensure `go.mod` in this project has all three `replace` directives:

```
replace github.com/dolan-in/dgman/v2 => ../dgman
replace github.com/matthewmcneely/modusgraph => ../modusGraph
replace github.com/mlwelles/modusGraphGen => ../modusGraphGen
```

### Phase 3.2: Compile check

Run `go build ./...` in this project. The generated code from Subagent C
must compile against the forked dependencies from Subagents A and B.

### Phase 3.3: Docker + data loading

1. Create `docker-compose.yml` (Dgraph Zero + Alpha).
2. Create `tasks/fetch-data.sh` to download `1million.rdf.gz` + schema.
3. Create `tasks/load-data.go` using `engine.Load()`.
4. Create `Makefile` with all targets from the design doc.

### Phase 3.4: Integration tests

Run against the loaded 1M dataset:

| Test                         | Expected                                     |
|------------------------------|----------------------------------------------|
| Search "Matrix"              | Returns films containing "Matrix"            |
| Search "Blade Runner"        | Returns at least one film                    |
| Search "Star Wars"           | Returns multiple films                       |
| ByGenre "Action Film"        | Returns films in the action genre            |
| ByYearRange 1999             | Returns films released in 1999               |
| Person search "Coppola"      | Returns director(s) named Coppola            |
| Mutation round-trip          | Add → Get → Update → Delete → verify gone    |
| Query builder                | Typed filters, ordering, pagination          |
| Iterator                     | Auto-paging across pages                     |

Guard with `testing.Short()` to skip when Dgraph is unavailable.

### Phase 3.5: CLI end-to-end

Run the generated Kong CLI against live Dgraph:

```
movies film search "Matrix" --first=5
movies film get <uid>
movies film list --genre="Action Film"
movies director search "Coppola"
```

Verify output matches expected results from the 1M dataset.
