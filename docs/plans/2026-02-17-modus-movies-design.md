# modusGraphMoviesProject Design

Struct-first codegen reference project for Dgraph, using modusgraph.

## Overview

modusGraphMoviesProject demonstrates a struct-first approach to building a
typed client for Dgraph's 1-million movie dataset. Go structs with `dgraph` tags
are the source of truth. A codegen tool (modusGraphGen) produces typed
sub-clients, functional options, query builders, and a Kong CLI from those
definitions.

### Background

This is one of several reference projects exploring codegen approaches for
Dgraph. The others (movieql, movies-pygql) use schema-first pipelines where a
GraphQL schema drives code generation. modusGraphMoviesProject takes the
opposite approach: Go structs drive everything. The projects share the same
underlying Dgraph benchmark dataset but are otherwise independent.

## Repositories

Four repos, each with a single responsibility:

| Repo                                | Role                                        |
|-------------------------------------|---------------------------------------------|
| `mlwelles/dgman`                    | Fork: fix `predicate=` in write + read paths |
| `mlwelles/modusGraph`               | Fork: `[]T` slice support, use forked dgman  |
| `mlwelles/modusGraphGen`           | Codegen: struct → typed client + Kong CLI    |
| `mlwelles/modusGraphMoviesProject`| Reference project: structs + tests           |

All four are cloned side-by-side under `../` and linked via `replace` directives
during development.

## Architecture

```
movies/
  film.go, director.go, ...    Hand-written structs (source of truth)
         |
         | go generate ./...
         v
modusGraphGen
         |
         | generates
         v
movies/                        cmd/movies/main.go
  client_gen.go                     Kong CLI
  film_gen.go
  film_options_gen.go
  film_query_gen.go
  ...
         |
         | connects via dgraph://
         v
Dgraph cluster (Docker)        Data: 1million.rdf.gz loaded via engine.Load()
```

### Key design choices

| Aspect                | Approach                                    |
|-----------------------|---------------------------------------------|
| Source of truth       | Go structs in `movies/`                     |
| Database              | Dgraph cluster (Docker)                     |
| Client library        | modusgraph (`dgraph://`)                    |
| Transport             | gRPC (Dgraph native)                        |
| Codegen               | modusGraphGen (client library + Kong CLI)  |
| Data loading          | RDF via `engine.Load()`                     |
| Schema management     | `WithAutoSchema(true)` from structs         |

## dgman Fork

Fork `dolan-in/dgman` to `mlwelles/dgman`, clone to `../dgman`. Fixes two bugs
where `predicate=` is ignored in mutation and query paths. The upstream master
(`fb912ad`) is identical to `v2.2.0-preview2`; no fixes exist upstream.

### Background: `predicate=` bugs

dgman's `dgraph:"predicate=X"` tag works correctly for **schema generation**
(the Dgraph schema gets the right predicate name) but is broken in two other
paths. Empirical testing confirmed both bugs against `file://` and `dgraph://`
backends using structs where the json tag differs from the predicate name (e.g.,
`json:"releaseDate"` with `dgraph:"predicate=release_date"`).

### Bug 1: Write path in `filterStruct` (`mutate.go:255`)

`MutateBasic()` → `mutate()` → `filterStruct()` builds a `map[string]interface{}`
for the mutation JSON. It uses `field.Tag.Get("json")` as the map key (line 255),
ignoring `schema.Predicate` entirely. Data is stored under the json tag name
(`releaseDate`) instead of the dgraph predicate name (`release_date`).

The `Mutate()`/`Upsert()` → `do()` → `generateMutation()` → `copyNodeValues()`
path is NOT affected --- it correctly uses `schema.Predicate` as map keys
(lines 707, 711, 714). This was confirmed by inspecting the mutation JSON in
verbose logs: Upsert correctly wrote `"release_date":"1999-03-31T00:00:00Z"`.

### Bug 2: Read path in query deserialization (`query.go:514,542`)

All Get/Query paths (`node()`, `nodes()`, `NodesAndCount()`) pass Dgraph's JSON
response directly to `json.Unmarshal(dataBytes, dst)`. Dgraph returns data keyed
by predicate name (`release_date`), but `json.Unmarshal` maps fields by their
`json` struct tag (`releaseDate`). No match → field stays at zero value.

This creates a perverse interaction: `MutateBasic` stores data under the wrong
predicate name but reads back fine (both write and read use json tags
consistently). `Mutate`/`Upsert` stores data under the correct predicate name
but reads back as zero (write uses predicate name, read expects json tag). The
upstream modusgraph repo "works" by accident --- it consistently uses the wrong
name on both sides.

### Fork changes

Two fixes, both in dgman:

1. **Fix `filterStruct`** (`mutate.go:231`) --- When `typeCache` has schema info
   for the struct, use `mutateType.schema[i].Predicate` as the map key instead
   of the json tag. Fall back to the json tag for non-dgraph structs (e.g.,
   `time.Time`). Also use `schema.OmitEmpty` instead of re-parsing json tag
   options.

2. **Fix query deserialization** (`query.go`) --- Before calling
   `json.Unmarshal`, remap JSON keys from predicate names to json tag names.
   Build the predicate→jsonTag mapping from the struct's parsed schema (already
   available in `parseDgraphTag`). Apply recursively for nested edge structs.
   This ensures `release_date` in Dgraph's response maps to the
   `json:"releaseDate"` field.

### Fork tests

Tests live in the dgman fork repo. The predicate round-trip tests already exist
in `modusGraph/predicate_test.go` and will be adapted:

- **Write round-trip via `MutateBasic`**: Insert a struct where `predicate=`
  differs from the `json` tag. Get it back, assert the field has the correct
  value (not zero).
- **Write round-trip via `Mutate`/`Upsert`**: Same test but through the `do()`
  path. Confirms the read fix works even when the write side was already correct.
- **Dot-prefixed predicates**: Edge stored under `director.film` (not `films`)
  round-trips correctly through insert and get.
- **Query filter by predicate name**: Insert films, filter with
  `ge(release_date, "1990-01-01")`, assert correct results. Confirms data was
  stored AND retrieved using the predicate name.

## modusgraph Fork

Fork `matthewmcneely/modusgraph` to `mlwelles/modusGraph`, clone to
`../modusGraph`. Kept minimal so changes are easy to upstream.

### Fork changes

Two changes:

1. **Use forked dgman** --- Update `go.mod` to point to `mlwelles/dgman` (via
   `replace` directive during development). This pulls in the `predicate=` fixes
   for both write and read paths. No code changes needed in modusgraph for
   `predicate=` support --- the fixes are entirely in dgman.

2. **`[]T` slice support** --- Make dgman's reflection handle value-type slices
   (`[]Genre`) the same way it handles pointer slices (`[]*Genre`). Populate and
   append by value instead of by pointer. No filtering or zero-value stripping;
   validation is a separate concern handled by `WithValidator()`.

### Fork tests

- **`predicate=` end-to-end through modusgraph**: Insert, Update, Upsert, Get,
  and Query using modusgraph's client API with structs where `predicate=` differs
  from the `json` tag. These tests exercise the full stack (modusgraph →
  dgman → Dgraph) on both `file://` and `dgraph://` backends. Tests already
  exist in `modusGraph/predicate_test.go`.
- **`[]T` slice round-trip**: Insert a struct with `[]Genre` (value-type slice),
  query it back, assert the slice is populated correctly. Compare behavior with
  `[]*Genre` to confirm parity.

## modusGraphGen

New repo `mlwelles/modusGraphGen`, cloned to `../modusGraphGen`. A standalone
codegen tool that reads Go structs with `dgraph` tags and generates a typed
client library. Reusable for any modusgraph project, not specific to movies.

Licensed the same as modusgraph.

### Parse phase

1. Find all exported structs in the target package
2. Extract `json` and `dgraph` tags from each field
3. Identify entities (structs with `UID` + `DType` fields)
4. Detect relationships (fields typed as `[]OtherEntity`)
5. Detect searchability (fields with `index=fulltext`)
6. Detect filterable fields (fields with `index=hash`, `index=year`, etc.)

### Inference rules

| Struct characteristic                          | Generated operation                |
|------------------------------------------------|------------------------------------|
| Has `dgraph:"index=fulltext"` on a string      | `Search(ctx, term, opts...)`       |
| Has `UID` + `DType` fields                     | Entity: gets a sub-client          |
| Field is `[]OtherEntity` with `dgraph:"reverse"`| Forward edge, `ByX()` list filter |
| Field is `[]OtherEntity` with `dgraph:"predicate=~X"`| Reverse edge, scanned on query    |
| Has `dgraph:"index=year,type=datetime"`         | `ByYearRange(from, to)` filter    |
| Every entity                                    | `Get`, `Add`, `Update`, `Delete`  |

### Generated files (output into consumer's package)

| File                         | Contents                              |
|------------------------------|---------------------------------------|
| `client_gen.go`              | Client struct, sub-clients, `New()`   |
| `<entity>_gen.go`            | Typed sub-client methods per entity   |
| `<entity>_options_gen.go`    | Functional options per entity         |
| `<entity>_query_gen.go`      | Typed query builder per entity        |
| `page_options_gen.go`        | Shared pagination options             |
| `iter_gen.go`                | Auto-paging iterator                  |
| `cmd/<name>/main.go`        | Complete Kong CLI                     |

### Generated consumer API

Two layers: a high-level API with functional options, and an advanced typed
query builder. Both are generated per entity.

#### Functional options API

CRUD and search with optional configuration via functional options. Every method
returns concrete types, no raw JSON.

```go
client := movies.New("dgraph://localhost:9080")

// Search with pagination
films, _ := client.Film.Search(ctx, "Matrix", movies.First(10))
films, _ = client.Film.Search(ctx, "Matrix", movies.First(10), movies.Offset(20))

// Get by UID
film, _ := client.Film.Get(ctx, "0x1234")

// Add with options
film, _ = client.Film.Add(ctx, "New Film",
    movies.WithReleaseDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
)

// Update with options (only set fields that are provided)
film, _ = client.Film.Update(ctx, "0x1234",
    movies.SetName("Updated Title"),
    movies.SetReleaseDate(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)),
)

// Delete by UID
_ = client.Film.Delete(ctx, "0x1234")

// List with filters (at least one required)
films, _ = client.Film.List(ctx, movies.ByGenre("Action Film"), movies.First(10))
films, _ = client.Film.List(ctx, movies.ByDirector("0x5678"), movies.First(10))
films, _ = client.Film.List(ctx,
    movies.ByYearRange(
        time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
        time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC),
    ),
)

// Auto-paging iterator
for film, err := range client.Film.SearchIter(ctx, "Star Wars") {
    if err != nil { break }
    fmt.Println(film.Name)
}
```

#### Option types (generated per entity)

```go
// Pagination (shared across all entities)
type PageOption interface{ applyPage(*pageConfig) }
func First(n int) PageOption   // movies.First(10)
func Offset(n int) PageOption  // movies.Offset(20)

// Film list filters
type FilmListOption interface{ applyFilmList(*filmListConfig) }
func ByGenre(name string) FilmListOption
func ByDirector(uid string) FilmListOption
func ByYearRange(from, to time.Time) FilmListOption

// Film add options
type FilmAddOption interface{ applyFilmAdd(*filmAddConfig) }
func WithReleaseDate(t time.Time) FilmAddOption

// Film update options (pointer fields for patch semantics)
type FilmUpdateOption interface{ applyFilmUpdate(*filmUpdateConfig) }
func SetName(n string) FilmUpdateOption
func SetReleaseDate(t time.Time) FilmUpdateOption
```

#### Typed query builder (advanced)

For complex queries that combine multiple filters, ordering, and pagination.
Builds DQL under the hood.

```go
var results []movies.Film
client.Film.Query(ctx).
    Filter(movies.NameEq("The Matrix")).
    OrderAsc(movies.FieldInitialReleaseDate).
    First(5).
    Exec(&results)

// Combine filters
client.Film.Query(ctx).
    Filter(movies.HasGenre("Sci-Fi")).
    Filter(movies.ReleasedAfter(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))).
    OrderDesc(movies.FieldInitialReleaseDate).
    First(20).
    Offset(10).
    Exec(&results)
```

## Model Definitions

Single set of structs in `movies/`. Each struct serves both modusgraph (via
`json`/`dgraph` tags) and the consumer API. No separate model/domain layers.

### Tag conventions

- `json` tag: camelCase, for JSON serialization. Consumer-facing.
- `dgraph` tag: index directives, type hints, and `predicate=` for predicate mapping.
- `predicate=` needed when the Dgraph predicate differs from the `json` tag
  (dot-prefixed predicates, singular vs plural, reverse edges).

### Struct definitions

```go
// movies/film.go
type Film struct {
    UID                string           `json:"uid,omitempty"`
    DType              []string         `json:"dgraph.type,omitempty"`
    Name               string           `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    InitialReleaseDate time.Time        `json:"initialReleaseDate,omitempty" dgraph:"predicate=initial_release_date index=year"`
    Tagline            string           `json:"tagline,omitempty"`
    Genres             []Genre          `json:"genres,omitempty" dgraph:"predicate=genre,reverse,count"`
    Countries          []Country        `json:"countries,omitempty" dgraph:"predicate=country,reverse"`
    Ratings            []Rating         `json:"ratings,omitempty" dgraph:"predicate=rating,reverse"`
    ContentRatings     []ContentRating  `json:"contentRatings,omitempty" dgraph:"predicate=rated,reverse"`
    Starring           []Performance    `json:"starring,omitempty" dgraph:"count"`
}

// movies/director.go
type Director struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=director.film,reverse,count"`
}

// movies/actor.go
type Actor struct {
    UID   string        `json:"uid,omitempty"`
    DType []string      `json:"dgraph.type,omitempty"`
    Name  string        `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Performance `json:"films,omitempty" dgraph:"predicate=actor.film,count"`
}

// movies/performance.go
type Performance struct {
    UID           string   `json:"uid,omitempty"`
    DType         []string `json:"dgraph.type,omitempty"`
    CharacterNote string   `json:"characterNote,omitempty" dgraph:"predicate=performance.character_note"`
}

// movies/genre.go
type Genre struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~genre"`
}

// movies/country.go
type Country struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~country"`
}

// movies/rating.go
type Rating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~rating"`
}

// movies/content_rating.go
type ContentRating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~rated"`
}

// movies/location.go
type Location struct {
    UID   string    `json:"uid,omitempty"`
    DType []string  `json:"dgraph.type,omitempty"`
    Name  string    `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Loc   []float64 `json:"loc,omitempty" dgraph:"index=geo,type=geo"`
    Email string    `json:"email,omitempty" dgraph:"index=exact,upsert"`
}
```

### Fields where `predicate=` is needed vs auto-fallback

| Field              | `json` tag          | Dgraph predicate             | Needs `predicate=`? |
|--------------------|---------------------|------------------------------|----------------|
| Name               | `name`              | `name`                       | No             |
| InitialReleaseDate | `initialReleaseDate`| `initial_release_date`       | Yes            |
| Tagline            | `tagline`           | `tagline`                    | No             |
| CharacterNote      | `characterNote`     | `performance.character_note` | Yes            |
| Starring           | `starring`          | `starring`                   | No             |
| Genres (Film)      | `genres`            | `genre`                      | Yes            |
| Films (Director)   | `films`             | `director.film`              | Yes            |
| Films (Genre)      | `films`             | `~genre`                     | Yes            |
| Email              | `email`             | `email`                      | No             |

## Project Structure

```
modusGraphMoviesProject/
  cmd/
    movies/                    Generated by modusGraphGen: Kong CLI
      main.go
  movies/                      Hand-written structs + generated client
    film.go
    director.go
    actor.go
    performance.go
    genre.go
    country.go
    rating.go
    content_rating.go
    location.go
    client_gen.go              Generated by modusGraphGen
    film_gen.go                Generated by modusGraphGen
    film_options_gen.go        Generated by modusGraphGen
    film_query_gen.go          Generated by modusGraphGen
    page_options_gen.go        Generated by modusGraphGen
    iter_gen.go                Generated by modusGraphGen
    generate.go                go:generate directives
  data/                        Downloaded benchmark data (gitignored)
    1million.rdf.gz
    1million.schema
  tasks/
    fetch-data.sh              Download benchmark data
    load-data.go               Load via engine.Load()
  docker/
    dgraph/
  docker-compose.yml
  Makefile
  go.mod
  go.sum
  .gitignore
  LICENSE
  README.md
```

### Hand-written vs generated

| Component                               | Hand-written | Generated by          |
|-----------------------------------------|:------------:|:---------------------:|
| Struct definitions (`movies/`)          | Yes          |                       |
| Typed client library (`movies/*_gen.go`)|              | modusGraphGen         |
| CLI (`cmd/movies/`)                     |              | modusGraphGen         |
| Makefile, docker-compose, etc.          | Yes          |                       |

## Tooling

### Makefile targets

```
make help          Show all targets
make setup         Full onboarding: deps + Dgraph + load data
make reset         Drop data, reload
make generate      Run modusGraphGen (client library + CLI)
make build         Build the movies CLI binary
make test          Run tests (self-healing: bootstraps Dgraph if needed)
make check         go vet
make docker-up     Start Dgraph cluster
make docker-down   Stop Dgraph cluster
make fetch-data    Download 1million.rdf.gz + schema
make load-data     Load via engine.Load()
make deps          Check Go + Docker
```

### docker-compose.yml

Dgraph Zero + Alpha. Ports:
- Zero: 5080, 6080
- Alpha: 8080 (HTTP), 9080 (gRPC --- modusgraph connects here)

### go.mod

```
module github.com/mlwelles/modusGraphMoviesProject

go 1.25

tool github.com/mlwelles/modusGraphGen

require (
    github.com/alecthomas/kong v1.14.0
    github.com/matthewmcneely/modusgraph v0.3.1
    github.com/mlwelles/modusGraphGen v0.1.0
)

replace github.com/dolan-in/dgman/v2 => ../dgman
replace github.com/matthewmcneely/modusgraph => ../modusGraph
replace github.com/mlwelles/modusGraphGen => ../modusGraphGen
```

### Data loading

`tasks/load-data.go` uses modusgraph's `engine.Load()` to load the benchmark
dataset directly:

```go
engine.Load(ctx, "data/1million.schema", "data/1million.rdf.gz")
```

### CLI commands

```
movies film search <term> [--first=10] [--offset=0]
movies film get <uid>
movies film list --genre=<name> | --director=<uid> | --from=<rfc3339> --to=<rfc3339>
movies film add --name=<name> --release-date=<rfc3339>
movies film update <uid> --release-date=<rfc3339>
movies film delete <uid>
movies person search <term> [--first=10] [--offset=0]
movies actor get <uid>
movies director get <uid>
movies director add --name=<name>
```

## Testing

### Strategy

Integration tests run against the deterministic 1M dataset. The dataset is
loaded once via `make load-data`; read tests query known entities. Only mutation
tests create/delete their own data.

Self-healing: `make test` auto-bootstraps Dgraph and loads data if needed.

### Test layers

1. **modusGraphGen unit tests** (in `../modusGraphGen`)
   - `go/ast` parser: given struct with tags, assert correct model
   - Inference: entity detection, searchability, filter detection, relationships
   - Template rendering: given model, assert generated code compiles
   - CLI generation: given model, assert generated Kong CLI compiles
   - Golden files: compare output against checked-in snapshots for both
     client library and CLI

2. **Integration tests** (`movies/`)
   - Read tests against known 1M data (Matrix, Blade Runner, Star Wars,
     Action Film genre, 1999 year range, Coppola)
   - Mutation round-trip: Add -> Get -> Update -> Delete -> verify gone
   - Query builder tests: typed filters, ordering, pagination
   - Iterator tests: auto-paging across pages
   - `testing.Short()` guard to skip integration tests

### Known test data

| Query                  | Expected                                |
|------------------------|-----------------------------------------|
| Search "Matrix"        | Returns films containing "Matrix"       |
| Search "Blade Runner"  | Returns at least one film               |
| Search "Star Wars"     | Returns multiple films                  |
| ByGenre "Action Film"  | Returns films in the action genre       |
| ByYearRange 1999       | Returns films released in 1999          |
| Person search "Coppola"| Returns director(s) named Coppola       |
