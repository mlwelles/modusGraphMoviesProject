# modusgraph-movies-project Design

Struct-first codegen reference project for Dgraph, using modusgraph.

## Overview

modusgraph-movies-project demonstrates a struct-first approach to building a
typed client for Dgraph's 1-million movie dataset. Go structs with `dgraph` tags
are the source of truth. A codegen tool (modusgraph-gen) produces typed
sub-clients, functional options, query builders, and a Kong CLI from those
definitions.

### Background

This is one of several reference projects exploring codegen approaches for
Dgraph. The others (movieql, movies-pygql) use schema-first pipelines where a
GraphQL schema drives code generation. modusgraph-movies-project takes the
opposite approach: Go structs drive everything. The projects share the same
underlying Dgraph benchmark dataset but are otherwise independent.

## Repositories

Three repos, each with a single responsibility:

| Repo                                | Role                                        |
|-------------------------------------|---------------------------------------------|
| `mlwelles/modusgraph`               | Fork: `pred=` tag + `[]T` slice support     |
| `mlwelles/modusgraph-gen`           | Codegen: struct → typed client + Kong CLI    |
| `mlwelles/modusgraph-movies-project`| Reference project: structs + tests           |

All three are cloned side-by-side under `../` and linked via `replace` directives
during development.

## Architecture

```
movies/
  film.go, director.go, ...    Hand-written structs (source of truth)
         |
         | go generate ./...
         v
modusgraph-gen
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
| Codegen               | modusgraph-gen (client library + Kong CLI)  |
| Data loading          | RDF via `engine.Load()`                     |
| Schema management     | `WithAutoSchema(true)` from structs         |

## modusgraph Fork

Fork `matthewmcneely/modusgraph` to `mlwelles/modusgraph`, clone to
`../modusgraph`. Kept minimal so changes are easy to upstream.

### Fork changes

Two additions to dgman's tag handling:

1. **`pred=` tag** --- Resolve the Dgraph predicate name from `dgraph:"pred=X"`
   before falling back to the `json` tag. This decouples JSON serialization keys
   from Dgraph predicate names.

   Resolution order:
   - `dgraph:"pred=X"` -> use `X`
   - No `pred=` -> fall back to `json` tag (backwards compatible)

2. **`[]T` slice support** --- Make dgman's reflection handle value-type slices
   (`[]Genre`) the same way it handles pointer slices (`[]*Genre`). Populate and
   append by value instead of by pointer. No filtering or zero-value stripping;
   validation is a separate concern handled by `WithValidator()`.

### Fork tests

Each fork change gets tests alongside the existing modusgraph/dgman test suite:

- **`pred=` tag resolution**: struct with `dgraph:"pred=director.film"` resolves
  to predicate `director.film`, not the `json` tag. Verify fallback to `json`
  tag when no `pred=` is present.
- **`[]T` slice round-trip**: Insert a struct with `[]Genre` (value-type slice),
  query it back, assert the slice is populated correctly. Compare behavior with
  `[]*Genre` to confirm parity.
- **`pred=` + reverse edges**: struct with `dgraph:"pred=~genre"` correctly
  resolves the reverse predicate.

## modusgraph-gen

New repo `mlwelles/modusgraph-gen`, cloned to `../modusgraph-gen`. A standalone
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
| Field is `[]OtherEntity` with `dgraph:"pred=~X"`| Reverse edge, scanned on query    |
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

### Generated consumer API (two layers)

High-level API:

```go
client := movies.New("dgraph://localhost:9080")

films, _ := client.Film.Search(ctx, "Matrix", movies.First(10))
film, _  := client.Film.Get(ctx, "0x1234")
_        = client.Film.Add(ctx, &movies.Film{Name: "New Film"})
_        = client.Film.Update(ctx, &movies.Film{UID: "0x1234", Name: "Updated"})
_        = client.Film.Delete(ctx, "0x1234")
```

Typed query builder (advanced):

```go
var results []movies.Film
client.Film.Query(ctx).
    Filter(movies.NameEq("The Matrix")).
    OrderAsc(movies.FieldInitialReleaseDate).
    First(5).
    Exec(&results)
```

## Model Definitions

Single set of structs in `movies/`. Each struct serves both modusgraph (via
`json`/`dgraph` tags) and the consumer API. No separate model/domain layers.

### Tag conventions

- `json` tag: camelCase, for JSON serialization. Consumer-facing.
- `dgraph` tag: index directives, type hints, and `pred=` for predicate mapping.
- `pred=` needed when the Dgraph predicate differs from the `json` tag
  (dot-prefixed predicates, singular vs plural, reverse edges).

### Struct definitions

```go
// movies/film.go
type Film struct {
    UID                string           `json:"uid,omitempty"`
    DType              []string         `json:"dgraph.type,omitempty"`
    Name               string           `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    InitialReleaseDate time.Time        `json:"initialReleaseDate,omitempty" dgraph:"index=year,type=datetime,pred=initial_release_date"`
    Tagline            string           `json:"tagline,omitempty"`
    Genres             []Genre          `json:"genres,omitempty" dgraph:"pred=genre,reverse,count"`
    Countries          []Country        `json:"countries,omitempty" dgraph:"pred=country,reverse"`
    Ratings            []Rating         `json:"ratings,omitempty" dgraph:"pred=rating,reverse"`
    ContentRatings     []ContentRating  `json:"contentRatings,omitempty" dgraph:"pred=rated,reverse"`
    Starring           []Performance    `json:"starring,omitempty" dgraph:"count"`
}

// movies/director.go
type Director struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"pred=director.film,reverse,count"`
}

// movies/actor.go
type Actor struct {
    UID   string        `json:"uid,omitempty"`
    DType []string      `json:"dgraph.type,omitempty"`
    Name  string        `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Performance `json:"films,omitempty" dgraph:"pred=actor.film,count"`
}

// movies/performance.go
type Performance struct {
    UID           string   `json:"uid,omitempty"`
    DType         []string `json:"dgraph.type,omitempty"`
    CharacterNote string   `json:"characterNote,omitempty" dgraph:"pred=performance.character_note"`
}

// movies/genre.go
type Genre struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"pred=~genre"`
}

// movies/country.go
type Country struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"pred=~country"`
}

// movies/rating.go
type Rating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"pred=~rating"`
}

// movies/content_rating.go
type ContentRating struct {
    UID   string   `json:"uid,omitempty"`
    DType []string `json:"dgraph.type,omitempty"`
    Name  string   `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    Films []Film   `json:"films,omitempty" dgraph:"pred=~rated"`
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

### Fields where `pred=` is needed vs auto-fallback

| Field              | `json` tag          | Dgraph predicate             | Needs `pred=`? |
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
modusgraph-movies-project/
  cmd/
    movies/                    Generated by modusgraph-gen: Kong CLI
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
    client_gen.go              Generated by modusgraph-gen
    film_gen.go                Generated by modusgraph-gen
    film_options_gen.go        Generated by modusgraph-gen
    film_query_gen.go          Generated by modusgraph-gen
    page_options_gen.go        Generated by modusgraph-gen
    iter_gen.go                Generated by modusgraph-gen
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
| Typed client library (`movies/*_gen.go`)|              | modusgraph-gen        |
| CLI (`cmd/movies/`)                     |              | modusgraph-gen        |
| Makefile, docker-compose, etc.          | Yes          |                       |

## Tooling

### Makefile targets

```
make help          Show all targets
make setup         Full onboarding: deps + Dgraph + load data
make reset         Drop data, reload
make generate      Run modusgraph-gen (client library + CLI)
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
module github.com/mlwelles/modusgraph-movies-project

go 1.25

tool github.com/mlwelles/modusgraph-gen

require (
    github.com/alecthomas/kong v1.14.0
    github.com/matthewmcneely/modusgraph v0.3.1
    github.com/mlwelles/modusgraph-gen v0.1.0
)

replace github.com/matthewmcneely/modusgraph => ../modusgraph
replace github.com/mlwelles/modusgraph-gen => ../modusgraph-gen
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

1. **modusgraph-gen unit tests** (in `../modusgraph-gen`)
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
