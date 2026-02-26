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

| Repo                                | Role                                        |
|-------------------------------------|---------------------------------------------|
| `matthewmcneely/modusgraph`        | Dgraph client library with struct-based schema management |
| `dolan-in/dgman`                    | Dgraph schema manager (transitive dep; v2.2.0 includes `predicate=` fixes) |
| `mlwelles/modusGraphGen`           | Codegen: struct → typed client + Kong CLI    |
| `mlwelles/modusGraphMoviesProject`| Reference project: structs + tests           |

`modusgraph` and `dgman` are used directly from their upstream repos.
`modusGraphGen` is pinned via a `replace` directive in `go.mod`.

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

## dgman `predicate=` Fixes (Now Upstream)

The `predicate=` fixes described below were originally developed in a fork
(`mlwelles/dgman`) and have since been merged into mainline `dolan-in/dgman`
v2.2.0. This project now uses the upstream release directly.

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

### Fixes (in dgman v2.2.0)

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

### Tests

Tests for these fixes live in the dgman repo:

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

## modusgraph (Upstream)

This project uses the mainline `matthewmcneely/modusgraph` v0.4.0 directly.
The `predicate=` fixes are resolved via dgman v2.2.0 (a transitive dependency).
The `[]T` value-type slice support was already present in modusgraph v0.4.0.

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
- `dgraph` tag: **space-separated** tokens. dgman's `parseStructTag()` regex
  splits on spaces; commas within `index=` are tokenizer separators
  (`index=hash,term,trigram,fulltext`), but commas anywhere else become part of
  the value and cause schema errors.
- `predicate=` needed when the Dgraph predicate differs from the `json` tag
  (dot-prefixed predicates, singular vs plural, reverse edges).
- Reverse edges require **both** `predicate=~X` and the `reverse` keyword.
  Without `reverse`, dgman's `ManagedReverse` flag is not set and the reverse
  edge is not expanded in queries.
- Forward edges with reverse indexing use `predicate=X reverse` (no `~` prefix).

### Struct definitions

```go
// movies/film.go
type Film struct {
    UID                string           `json:"uid,omitempty"`
    DType              []string         `json:"dgraph.type,omitempty"`
    Name               string           `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    InitialReleaseDate time.Time        `json:"initialReleaseDate,omitempty" dgraph:"predicate=initial_release_date index=year"`
    Tagline            string           `json:"tagline,omitempty"`
    Genres             []Genre          `json:"genres,omitempty" dgraph:"predicate=genre reverse count"`
    Countries          []Country        `json:"countries,omitempty" dgraph:"predicate=country reverse"`
    Ratings            []Rating         `json:"ratings,omitempty" dgraph:"predicate=rating reverse"`
    ContentRatings     []ContentRating  `json:"contentRatings,omitempty" dgraph:"predicate=rated reverse"`
    Starring           []Performance    `json:"starring,omitempty" dgraph:"count"`
}

// movies/director.go
type Director struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=director.film reverse count"`
}

// movies/actor.go
type Actor struct {
    UID   string        `json:"uid,omitempty"`
    DType []string      `json:"dgraph.type,omitempty"`
    Name  string        `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Performance `json:"films,omitempty" dgraph:"predicate=actor.film count"`
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
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~genre reverse"`
}

// movies/country.go
type Country struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~country reverse"`
}

// movies/rating.go
type Rating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~rating reverse"`
}

// movies/content_rating.go
type ContentRating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"predicate=~rated reverse"`
}

// movies/location.go
type Location struct {
    UID   string    `json:"uid,omitempty"`
    DType []string  `json:"dgraph.type,omitempty"`
    Name  string    `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Loc   []float64 `json:"loc,omitempty" dgraph:"index=geo type=geo"`
    Email string    `json:"email,omitempty" dgraph:"index=exact upsert"`
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
  movies/                      Hand-written structs + generated client
    cmd/
      movies/                  Generated by modusGraphGen: Kong CLI
        main.go
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

go 1.26.0

replace github.com/mlwelles/modusGraphGen => github.com/mlwelles/modusGraphGen v1.0.0

tool github.com/mlwelles/modusGraphGen

require (
    github.com/alecthomas/kong v1.14.0
    github.com/matthewmcneely/modusgraph v0.4.0
)
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
