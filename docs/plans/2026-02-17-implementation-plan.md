# Implementation Plan

Parallel execution plan for the modusGraphMoviesProject and its three
dependencies. Three subagents run concurrently; a merge phase wires
everything together.

## Execution Model

```
Time ──────────────────────────────────────────────────────►

Subagent A: dgman fixes now in upstream v2.2.0 (no fork needed)
Subagent B: modusgraph used from upstream v0.4.0 (no fork needed)

Subagent C (structs + codegen):
  Phase 1.1  Movie structs in this project
  Phase 1.2  modusGraphGen AST parser
  Phase 1.3  Codegen templates
  Phase 1.4  Golden-file tests
  Phase 1.5  CLI generation

Merge (main agent):
  Phase 3.1  Wire go.mod (upstream deps + modusGraphGen replace)
  Phase 3.2  Compile check
  Phase 3.3  Docker + data loading
  Phase 3.4  Integration tests
  Phase 3.5  CLI end-to-end
```

### Dependencies

- Subagents A and B are no longer needed — upstream repos include all fixes.
- The merge phase requires Subagent C to complete.

---

## Subagent A: dgman Fixes (Now Upstream)

The `predicate=` fixes originally planned for a fork have been merged into
mainline `dolan-in/dgman` v2.2.0. No fork is needed. This project uses
`dolan-in/dgman/v2 v2.2.0` (resolved transitively via modusgraph, upgraded
via explicit `require` in `go.mod`).

## Subagent B: modusgraph (Upstream)

The mainline `matthewmcneely/modusgraph` v0.4.0 already supports `[]T`
value-type slices and uses dgman as a dependency. By requiring dgman v2.2.0
explicitly in this project's `go.mod`, Go's Minimum Version Selection
upgrades the transitive dependency to include the `predicate=` fixes.
No fork is needed.

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

Ensure `go.mod` in this project uses upstream dependencies:

```
replace github.com/mlwelles/modusGraphGen => github.com/mlwelles/modusGraphGen v1.0.0

require (
    github.com/matthewmcneely/modusgraph v0.4.0
    github.com/dolan-in/dgman/v2 v2.2.0  // upgraded from modusgraph's v2.2.0-preview2
)
```

### Phase 3.2: Compile check

Run `go build ./...` in this project. The generated code from Subagent C
must compile against the forked dependencies from Subagents A and B.

### Phase 3.3: Docker + data loading + Makefile

Mirror the scaffolding from `../movieql` with two key differences: (1)
data loading uses modusgraph's `engine.Load()` instead of `dgraph live`,
and (2) schema management uses `WithAutoSchema(true)` from structs
instead of a separate GraphQL schema.

#### docker-compose.yml

Use `dgraph/standalone:latest` (single container, simpler than separate
Zero + Alpha). Use the standard Dgraph ports matching the dgraph-io/tour
docker-compose.yml. Include Ratel for the query UI:

```yaml
services:
  dgraph:
    image: dgraph/standalone:latest
    container_name: modus-movies-dgraph
    ports:
      - "8080:8080"   # HTTP
      - "9080:9080"   # gRPC (modusgraph connects here)
    volumes:
      - ./docker/dgraph:/dgraph
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s

  ratel:
    image: dgraph/ratel:latest
    container_name: modus-movies-ratel
    ports:
      - "8000:8000"
    depends_on:
      - dgraph
```

#### tasks/fetch-data.sh

Same as movieql: download `1million.rdf.gz` + `1million.schema` from
`dgraph-io/tour` into `data/`. Skip download if files already exist.

#### tasks/load-data.go

Go program using modusgraph's `engine.Load()` to load data via gRPC.
If `engine.Load()` does not support RDF bulk loading, fall back to
`dgraph live` via shell exec (like movieql).

```go
engine.Load(ctx, "data/1million.schema", "data/1million.rdf.gz")
```

#### tasks/drop-data.sh

Same as movieql: `POST /alter {"drop_all": true}`. Used by `make reset`.

#### Makefile

```makefile
DGRAPH_ALPHA ?= http://localhost:8080
DGRAPH_GRPC  ?= localhost:9080

help           ## Show all targets
setup          ## deps + docker-up + load-data
reset          ## docker-up + drop-data + load-data
generate       ## Run modusGraphGen (client library + CLI)
build          ## Build the movies CLI binary
test           ## Self-healing: ensure-data + go test
check          ## go vet
docker-up      ## Start Dgraph container
docker-down    ## Stop Dgraph container
fetch-data     ## Download 1million.rdf.gz + schema
load-data      ## fetch-data → engine.Load() or dgraph live
drop-data      ## Drop all data
deps           ## Check Go + Docker
```

Self-healing targets (internal):

- `dgraph-ready`: wait for health endpoint
- `ensure-data`: if film count is 0, run `load-data`

NOT needed (struct-first, no GraphQL schema):

- `apply-schema` (modusgraph uses `WithAutoSchema(true)`)
- `introspect-schema` (codegen reads Go AST, not Dgraph schema)

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
