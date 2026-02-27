# modusGraphMoviesProject

Reference project demonstrating struct-first code generation for
[Dgraph](https://dgraph.io) using
[modusgraph](https://github.com/matthewmcneely/modusgraph) and its built-in
`modusgraph-gen` code generator.

Hand-written Go structs with `json` and `dgraph` tags are the single source of
truth. Running `go generate` produces a fully typed client library, query
builders, auto-paging iterators, and a Kong CLI — all from those struct
definitions. The project loads Dgraph's 1-million movie dataset and runs
integration tests against it.

## What This Demonstrates

- **Struct-first codegen**: Go structs drive everything — schema, client code,
  and CLI. No separate schema files, no manual client wiring.
- **Typed sub-clients**: Each entity (Film, Director, Genre, ...) gets its own
  sub-client with `Get`, `Add`, `Update`, `Delete`, `Search`, and `List`.
- **Query builders**: Fluent, type-safe query construction with `Filter`,
  `OrderAsc`/`OrderDesc`, `First`, `Offset`, and `ExecAndCount`.
- **Auto-paging iterators**: Go 1.23+ `range`-over-func iterators that
  transparently page through large result sets.
- **Generated CLI**: A complete [Kong](https://github.com/alecthomas/kong)
  command-line interface with subcommands per entity.
- **Edge relationships**: Forward edges, reverse edges (`~predicate`), and
  `count` directives all inferred from struct tags.
- **Raw DQL queries**: Execute arbitrary Dgraph Query Language queries via the
  `QueryRaw` Go client method or the `query` CLI subcommand (argument or stdin).
- **Dual connection modes**: Connect to a remote Dgraph cluster via gRPC
  (`--addr`) or run an embedded Dgraph instance from a local directory (`--dir`).
- **Integration tests**: Full CRUD, search, pagination, query builder, iterator,
  and reverse-edge tests against live Dgraph.

## Prerequisites

- **Go** 1.23+ (for range-over-func iterators)
- **Docker** with Docker Compose v2 (for the Dgraph cluster)

## Quick Start

```sh
# 1. Clone the project
git clone https://github.com/mlwelles/modusGraphMoviesProject.git
# Dependencies (modusgraph, dgman) are fetched
# automatically via go.mod

# 2. Full setup: check deps, start Dgraph, load the 1M movie dataset
cd modusGraphMoviesProject
make setup

# 3. Run code generation (optional — generated files are already committed)
make generate

# 4. Build the CLI
make build

# 5. Try it out
./bin/movies film search "Matrix" --first=5
./bin/movies genre list
./bin/movies director search "Coppola"

# 6. Run integration tests
make test
```

Or step-by-step if you want to auto-install missing dependencies:

```sh
AUTO_INSTALL=true make setup
```

## Repository Architecture

This project depends on the following upstream libraries, all resolved
automatically via `go.mod`:

| Repo | Role |
|------|------|
| [`matthewmcneely/modusgraph`](https://github.com/matthewmcneely/modusgraph) | Dgraph client library with struct-based schema management and `modusgraph-gen` code generator |
| [`dolan-in/dgman`](https://github.com/dolan-in/dgman) | Dgraph schema manager (transitive dep of modusgraph) |
| [`mlwelles/modusGraphMoviesProject`](https://github.com/mlwelles/modusGraphMoviesProject) | **This repo**: reference project with structs, tests, and data loading |

```
modusGraphMoviesProject/   <-- you are here
  movies/
    film.go, director.go, ...   Hand-written structs (source of truth)
    client_gen.go               Generated typed client
    film_gen.go                 Generated Film sub-client
    film_query_gen.go           Generated Film query builder
    iter_gen.go                 Generated auto-paging iterators
    cmd/movies/main.go          Generated Kong CLI
  data/                         1M movie dataset (downloaded by make)
  docker-compose.yml            Dgraph Zero + Alpha
  Makefile                      Build automation
```

## Features

### Standard modusGraph Features

- **Typed CRUD**: `Get`, `Add`, `Update`, `Delete` per entity — fully typed,
  no manual query construction needed
- **Fulltext search**: `Search` method on entities with `index=fulltext` fields,
  using Dgraph's stemming and stop-word removal
- **Fluent query builders**: `Filter`, `OrderAsc`/`OrderDesc`, `First`, `Offset`,
  `Exec`, and `ExecAndCount` for complex queries
- **Auto-paging iterators**: Go 1.23+ `iter.Seq2` iterators (`SearchIter`,
  `ListIter`) that transparently page through large result sets
- **Functional options**: `First(n)`, `Offset(n)` pagination options shared
  across all entity operations
- **Auto-schema management**: `modusgraph.WithAutoSchema(true)` creates and
  updates Dgraph schema from struct tags automatically
- **Optional struct validation**: Integrates with `go-playground/validator` for
  field-level validation on mutations

### Query and Connection Features

- **Raw DQL queries**: `QueryRaw(ctx, query, vars)` on the Go client executes
  arbitrary Dgraph Query Language queries and returns raw JSON
- **CLI query subcommand**: `movies query '<dql>'` or `echo '<dql>' | movies query`
  for ad-hoc queries from the command line
- **Dual connection modes**:
  - **gRPC** (`--addr` / `DGRAPH_ADDR`): Connect to a remote Dgraph cluster
  - **Embedded** (`--dir` / `DGRAPH_DIR`): Run Dgraph in-process from a local
    directory — no external cluster required

## Struct Definitions

The `movies/` directory contains hand-written Go structs that serve as the
single source of truth. Each struct uses two tag systems:

- **`json` tag**: Controls JSON serialization. Uses camelCase names.
- **`dgraph` tag**: Controls how the field maps to Dgraph. Uses
  space-separated directives.

### Entity Detection

A struct is recognized as a Dgraph entity when it has **both** `UID` and
`DType` fields. Only entity structs get a generated sub-client:

```go
type Film struct {
    UID   string   `json:"uid,omitempty"`          // required — identifies the node
    DType []string `json:"dgraph.type,omitempty"`  // required — Dgraph type discriminator
    // ... other fields
}
```

### The `dgraph` Tag

The `dgraph` tag uses **space-separated** tokens for independent directives.
Commas appear only within `index=` to separate multiple index types.

```go
// Space separates independent directives:
dgraph:"predicate=initial_release_date index=year"

// Commas separate index tokenizers within index=:
dgraph:"index=hash,term,trigram,fulltext"

// Forward edge with reverse indexing and count:
dgraph:"predicate=genre reverse count"

// Reverse edge (requires BOTH ~ prefix AND reverse keyword):
dgraph:"predicate=~genre reverse"

// Standalone flags:
dgraph:"count"
dgraph:"upsert"
```

### Tag Directives Reference

| Directive | Example | Effect |
|-----------|---------|--------|
| `predicate=X` | `predicate=initial_release_date` | Override the Dgraph predicate name (default: json tag value) |
| `predicate=~X` | `predicate=~genre` | Declare a reverse edge (must also include `reverse`) |
| `index=types` | `index=hash,term,trigram,fulltext` | Add search indexes (see Index Types below) |
| `reverse` | `reverse` | Enable reverse edge traversal. On forward edges, enables `~predicate` queries. On reverse edges (`predicate=~X`), required to set dgman's `ManagedReverse` flag |
| `count` | `count` | Enable `count(predicate)` queries on this edge |
| `upsert` | `upsert` | Mark field for upsert deduplication (find-or-create) |
| `type=X` | `type=geo` | Dgraph type hint for non-standard types |

### Index Types for Strings

Dgraph offers several index types for `string` predicates. You can combine
multiple index types on the same field:

| Index | DQL Functions | Use Case |
|-------|---------------|----------|
| `hash` | `eq` | Fast equality matching on exact strings. Hashes the string, so good for long values |
| `exact` | `eq`, `lt`, `le`, `gt`, `ge` | Equality and inequality (lexicographic) on full strings |
| `term` | `allofterms`, `anyofterms` | Match all or any of a list of whitespace-delimited terms |
| `fulltext` | `alloftext`, `anyoftext` | Full-text search with stemming and stop-word removal ("run" matches "running", "ran"). Supports 18 languages |
| `trigram` | `regexp` | Regular expression matching via trigram decomposition |

### Index Types for Other Scalar Types

| Type | Index | DQL Functions |
|------|-------|---------------|
| `int` | (default) | `eq`, `lt`, `le`, `gt`, `ge` |
| `float` | (default) | `eq`, `lt`, `le`, `gt`, `ge` |
| `datetime` | `year`, `month`, `day`, `hour` | `eq`, `lt`, `le`, `gt`, `ge` at the specified granularity |
| `geo` | `geo` | `near`, `within`, `contains`, `intersects` |
| `bool` | (default) | `eq` |

### When `predicate=` Is Needed

Use `predicate=` when the Dgraph predicate name differs from the `json` tag:

| Field | `json` tag | Dgraph predicate | Why `predicate=` is needed |
|-------|-----------|------------------|---------------------------|
| `InitialReleaseDate` | `initialReleaseDate` | `initial_release_date` | snake_case predicate vs camelCase JSON |
| `CharacterNote` | `characterNote` | `performance.character_note` | Dot-prefixed predicate |
| `Genres` (on Film) | `genres` | `genre` | Singular predicate, plural JSON |
| `Films` (on Director) | `films` | `director.film` | Namespaced predicate |
| `Films` (on Genre) | `films` | `~genre` | Reverse edge traversal |

### Forward vs Reverse Edges

**Forward edge**: Film has genres. The `genre` predicate points Film → Genre.

```go
// Film.go — forward edge
Genres []Genre `json:"genres,omitempty" dgraph:"predicate=genre reverse count"`
//                                              ^^^^^^^^^^^^^^^^ ^^^^^^^ ^^^^^
//                                              predicate name   enable   enable
//                                                               ~genre   count()
//                                                               queries
```

**Reverse edge**: Genre lists films that reference it. Uses `~genre` to
traverse the edge backward:

```go
// Genre.go — reverse edge
Films []Film `json:"films,omitempty" dgraph:"predicate=~genre reverse"`
//                                           ^^^^^^^^^^^^^^^ ^^^^^^^
//                                           ~ prefix means  REQUIRED: sets
//                                           reverse edge    ManagedReverse flag
```

### All Struct Definitions

```go
// movies/film.go
type Film struct {
    UID                string          `json:"uid,omitempty"`
    DType              []string        `json:"dgraph.type,omitempty"`
    Name               string          `json:"name,omitempty" dgraph:"index=hash,term,trigram,fulltext"`
    InitialReleaseDate time.Time       `json:"initialReleaseDate,omitempty" dgraph:"predicate=initial_release_date index=year"`
    Tagline            string          `json:"tagline,omitempty"`
    Genres             []Genre         `json:"genres,omitempty" dgraph:"predicate=genre reverse count"`
    Countries          []Country       `json:"countries,omitempty" dgraph:"predicate=country reverse"`
    Ratings            []Rating        `json:"ratings,omitempty" dgraph:"predicate=rating reverse"`
    ContentRatings     []ContentRating `json:"contentRatings,omitempty" dgraph:"predicate=rated reverse"`
    Starring           []Performance   `json:"starring,omitempty" dgraph:"count"`
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

## What Gets Generated

Running `go generate ./movies` (or `make generate`) invokes `modusgraph-gen`,
which reads the struct definitions and produces these files:

| File | Contents |
|------|----------|
| `client_gen.go` | `Client` struct with sub-clients per entity, `New()`, `NewFromClient()`, `Close()` |
| `page_options_gen.go` | `First(n)` and `Offset(n)` pagination options (shared across entities) |
| `iter_gen.go` | `SearchIter` and `ListIter` auto-paging iterators per entity |
| `<entity>_gen.go` | `Get`, `Add`, `Update`, `Delete`, `Search`, `List` methods per entity |
| `<entity>_options_gen.go` | Functional options per entity (reserved for future expansion) |
| `<entity>_query_gen.go` | Typed query builder per entity (`Filter`, `OrderAsc`, `Exec`, etc.) |
| `cmd/movies/main.go` | Complete Kong CLI with subcommands per entity |

### Inference Rules

modusGraphGen uses the struct tags to decide what to generate:

| Struct characteristic | What gets generated |
|-----------------------|--------------------|
| Has `UID` + `DType` fields | Recognized as entity — gets a typed sub-client |
| String field with `index=fulltext` | `Search(ctx, term, opts...)` method + `SearchIter` iterator |
| Field typed `[]OtherEntity` | Edge relationship (no special code, handled by modusgraph) |
| `predicate=~X` with `reverse` | Reverse edge (expanded in queries by dgman's `ManagedReverse`) |
| Every entity | `Get`, `Add`, `Update`, `Delete`, `List`, `ListIter`, `Query` |

## Generated Client API

### Client Setup

```go
import "github.com/mlwelles/modusGraphMoviesProject/movies"

client, err := movies.New("dgraph://localhost:9080",
    modusgraph.WithAutoSchema(true),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

The `Client` struct exposes a sub-client for every entity:

```go
client.Film          // *FilmClient
client.Director      // *DirectorClient
client.Actor         // *ActorClient
client.Genre         // *GenreClient
client.Country       // *CountryClient
client.Rating        // *RatingClient
client.ContentRating // *ContentRatingClient
client.Location      // *LocationClient
client.Performance   // *PerformanceClient
```

### Raw DQL Queries (QueryRaw)

For queries that go beyond the typed API, `QueryRaw` executes arbitrary DQL
against the database and returns the raw JSON response:

```go
ctx := context.Background()

// Simple query
resp, err := client.QueryRaw(ctx,
    `{ q(func: has(name), first: 5) { uid name } }`, nil)
fmt.Println(string(resp))

// Parameterized query with variables
resp, err = client.QueryRaw(ctx,
    `query search($term: string) {
        q(func: alloftext(name, $term), first: 10) {
            uid name
        }
    }`,
    map[string]string{"$term": "Matrix"},
)
```

### CRUD Operations

Every entity sub-client provides `Get`, `Add`, `Update`, and `Delete`:

```go
ctx := context.Background()

// Add a new film — UID is populated after insertion
film := &movies.Film{
    Name:               "The Matrix",
    InitialReleaseDate: time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC),
    Tagline:            "Welcome to the Real World",
    Genres:             []movies.Genre{action, scifi}, // edge relationships
}
err := client.Film.Add(ctx, film)
// film.UID is now set, e.g. "0x4e2a"

// Get by UID — returns the full entity with edges expanded
got, err := client.Film.Get(ctx, film.UID)
fmt.Println(got.Name)    // "The Matrix"
fmt.Println(got.Genres)  // [{UID:"0x..." Name:"Action"}, {UID:"0x..." Name:"Sci-Fi"}]

// Update — set fields on the struct and call Update
got.Tagline = "Welcome to the Real World (1999)"
err = client.Film.Update(ctx, got)

// Delete by UID
err = client.Film.Delete(ctx, film.UID)
```

### Search (Fulltext)

Generated for entities that have a string field with `index=fulltext`. Uses
Dgraph's `alloftext` function, which supports stemming and stop-word removal:

```go
// Basic search
films, err := client.Film.Search(ctx, "Matrix")

// With pagination
films, err = client.Film.Search(ctx, "Matrix",
    movies.First(10),
    movies.Offset(20),
)

// Search is generated for Film, Director, Actor, Genre, Country,
// Rating, ContentRating, and Location — any entity with a fulltext-indexed field.
directors, err := client.Director.Search(ctx, "Coppola")
actors, err := client.Actor.Search(ctx, "Keanu")
```

### List with Pagination

Retrieve all entities of a type with cursor-based pagination:

```go
page1, err := client.Film.List(ctx, movies.First(10))
page2, err := client.Film.List(ctx, movies.First(10), movies.Offset(10))

genres, err := client.Genre.List(ctx, movies.First(50))
```

### Query Builder

For complex queries combining filters, ordering, and pagination. Builds DQL
under the hood:

```go
// Filter + order + limit
var results []movies.Film
err := client.Film.Query(ctx).
    Filter(`alloftext(name, "Star")`).
    OrderAsc("name").
    First(5).
    Exec(&results)

// Order by date descending
err = client.Film.Query(ctx).
    First(10).
    OrderDesc("initial_release_date").
    Exec(&results)

// Count total matching results
var results []movies.Film
count, err := client.Film.Query(ctx).
    Filter(`alloftext(name, "Matrix")`).
    First(10).
    ExecAndCount(&results)
fmt.Printf("Got %d results out of %d total\n", len(results), count)
```

The `Filter` method accepts raw DQL filter expressions. Common patterns:

```go
// Fulltext search (requires index=fulltext)
Filter(`alloftext(name, "Star Wars")`)

// Term matching (requires index=term)
Filter(`allofterms(name, "Star Wars")`)  // must contain both "Star" AND "Wars"
Filter(`anyofterms(name, "Star Wars")`)  // must contain "Star" OR "Wars"

// Equality (requires index=hash or index=exact)
Filter(`eq(name, "The Matrix")`)

// Date range (requires index=year on datetime field)
Filter(`ge(initial_release_date, "1999-01-01") AND le(initial_release_date, "1999-12-31")`)

// Regular expression (requires index=trigram)
Filter(`regexp(name, /matrix/i)`)
```

### Auto-Paging Iterators

Uses Go 1.23+ `range`-over-func to iterate through all pages automatically.
Each call fetches the next page of 50 results transparently:

```go
// Iterate over all films matching "Star Wars"
for film, err := range client.Film.SearchIter(ctx, "Star Wars") {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(film.Name)
}

// Iterate over all genres
for genre, err := range client.Genre.ListIter(ctx) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(genre.Name)
}
```

`SearchIter` is generated only for entities with a fulltext-indexed field.
`ListIter` is generated for every entity.

### Reverse Edge Traversal

Reverse edges let you traverse relationships backward. When you `Get` a Genre,
its `Films` field is populated with all films in that genre:

```go
// Genre.Films is a reverse edge (predicate=~genre)
action, err := client.Genre.Get(ctx, actionGenreUID)
fmt.Println(action.Films) // all films in the Action genre

// Director.Films is a forward edge (predicate=director.film)
// with reverse enabled — so Film also sees its directors
coppola, err := client.Director.Get(ctx, coppolaUID)
for _, film := range coppola.Films {
    fmt.Println(film.Name)  // "The Godfather", "Apocalypse Now", etc.
}
```

## Generated CLI

The generated Kong CLI at `movies/cmd/movies/main.go` provides subcommands for
every entity. Build and run:

```sh
make build
./bin/movies --help
```

### Available Commands

```
Usage: movies <command> [flags]

Flags:
  --addr string    Dgraph gRPC address (default "dgraph://localhost:9080", env DGRAPH_ADDR)
  --dir string     Local database directory (embedded mode, mutually exclusive with --addr)

Commands:
  query         Execute a raw DQL query
  film          Manage Film entities
  director      Manage Director entities
  actor         Manage Actor entities
  genre         Manage Genre entities
  country       Manage Country entities
  rating        Manage Rating entities
  content-rating Manage ContentRating entities
  location      Manage Location entities
  performance   Manage Performance entities
```

### Connection Modes

The CLI supports two mutually exclusive connection modes:

- **gRPC (default)**: Connect to a running Dgraph cluster via `--addr`
  ```sh
  ./bin/movies --addr dgraph://localhost:9080 film search "Matrix"
  ```
- **Embedded**: Run Dgraph in-process from a local directory via `--dir`
  ```sh
  ./bin/movies --dir /tmp/movies-db film search "Matrix"
  ```

### Query Subcommand

The `query` subcommand executes raw
[DQL (Dgraph Query Language)](https://dgraph.io/docs/dql/) queries directly
against the database. The query can be passed as a positional argument or piped
via stdin.

```sh
# Pass query as argument
./bin/movies query '{ q(func: has(name@en), first: 5) { uid name@en } }'

# Pipe query via stdin
echo '{ q(func: has(name@en), first: 5) { uid name@en } }' | ./bin/movies query

# Use embedded mode
./bin/movies --dir /tmp/movies-db query '{ q(func: type(Film), first: 3) { uid name } }'

# Disable pretty-printing
./bin/movies query --no-pretty '{ q(func: type(Genre)) { uid name } }'

# Set a custom timeout
./bin/movies query --timeout=60s '{ q(func: type(Film), first: 1000) { uid name } }'
```

### Entity Subcommands

Each entity has the same subcommand pattern:

```sh
# Search (entities with fulltext index)
./bin/movies film search "Matrix" --first=5
./bin/movies director search "Coppola"
./bin/movies actor search "Keanu"

# Get by UID
./bin/movies film get 0x4e2a

# List with pagination
./bin/movies genre list --first=20
./bin/movies film list --first=10 --offset=30

# Add a new entity
./bin/movies film add --name="New Film" --tagline="A new film"
./bin/movies genre add --name="Musical"
./bin/movies director add --name="New Director"

# Delete by UID
./bin/movies film delete 0x4e2a
```

Output is JSON, making it easy to pipe to `jq`:

```sh
./bin/movies film search "Matrix" | jq '.[].name'
```

## Makefile

```
make help          Show all targets and environment variables
make setup         Full onboarding: check deps, start Dgraph, load 1M dataset
make reset         Drop all data, reload from scratch
make generate      Run modusgraph-gen (regenerate client library + CLI)
make build         Build the movies CLI binary to bin/movies
make test          Run integration tests (self-healing: bootstraps Dgraph if needed)
make check         Run go vet on all packages
make docker-up     Start Dgraph containers
make docker-down   Stop Dgraph containers
make deps          Check all dependencies (Go, Docker)
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DGRAPH_ALPHA` | `http://localhost:8080` | Dgraph Alpha HTTP endpoint |
| `DGRAPH_GRPC` | `localhost:9080` | Dgraph gRPC endpoint |
| `AUTO_INSTALL` | `false` | Set to `true` to auto-install missing deps (Go, Docker) |

### Docker Services

`docker-compose.yml` runs Dgraph Zero + Alpha:

| Service | Port | Purpose |
|---------|------|---------|
| dgraph | 8080 | HTTP API (health checks, data loading) |
| dgraph | 9080 | gRPC (modusgraph connects here) |
| ratel | 8000 | Dgraph Ratel UI (visual query interface) |

## Testing

Integration tests in `movies/integration_test.go` run against live Dgraph.
They seed a well-known dataset of films, directors, actors, and genres, then
exercise the full generated API:

| Test | What it verifies |
|------|-----------------|
| `TestSearchFilmMatrix` | Fulltext search returns films matching "Matrix" |
| `TestSearchFilmStarWars` | Fulltext search returns multiple Star Wars films |
| `TestSearchDirectorCoppola` | Director search finds Francis Ford Coppola |
| `TestSearchActorKeanu` | Actor search finds Keanu Reeves |
| `TestListFilmsWithPagination` | `First(3)` returns 3, `Offset(3)` returns different results |
| `TestListGenres` | Genre list returns seeded genres |
| `TestQueryBuilderFilterAndOrder` | Filter + OrderAsc produces alphabetically sorted results |
| `TestQueryBuilderExecAndCount` | ExecAndCount returns both results and total count |
| `TestQueryBuilderOrderDesc` | OrderDesc by date produces newest-first ordering |
| `TestFilmSearchIterator` | SearchIter yields results via range-over-func |
| `TestGenreListIterator` | ListIter pages through all genres |
| `TestMutationRoundTrip` | Add → Get → Update → Get → Search → Delete → verify gone |
| `TestGenreReverseEdge` | Genre.Films populated via ~genre reverse edge |
| `TestCountryReverseEdge` | Country.Films populated via ~country reverse edge |
| `TestForwardEdgeUpdateReflectsInReverse` | Updating Film.Genres immediately reflects in Genre.Films |
| `TestDirectorWithFilms` | Director.Films populated via director.film forward edge |

```sh
# Run all tests (requires Dgraph running with data loaded)
make test

# Skip integration tests
go test -short ./...
```

## License

Apache 2.0. See [LICENSE](LICENSE).
