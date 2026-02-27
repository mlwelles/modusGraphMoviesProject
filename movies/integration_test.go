package movies_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matthewmcneely/modusgraph"

	"github.com/mlwelles/modusGraphMoviesProject/movies"
)

// testAddr returns the Dgraph gRPC address or empty if not set.
func testAddr() string {
	return os.Getenv("DGRAPH_TEST_ADDR")
}

// skipIfNoDgraph skips the test if DGRAPH_TEST_ADDR is not set or -short is passed.
func skipIfNoDgraph(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if testAddr() == "" {
		t.Skip("Skipping: DGRAPH_TEST_ADDR not set")
	}
}

// newTestClient creates a movies.Client connected to the test Dgraph instance.
func newTestClient(t *testing.T) *movies.Client {
	t.Helper()
	c, err := movies.New("dgraph://"+testAddr(), modusgraph.WithAutoSchema(true))
	if err != nil {
		t.Fatalf("movies.New: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// seedOnce ensures test data is seeded exactly once across all tests.
var seedOnce sync.Once
var seedErr error

// seedData inserts a small set of well-known movies, directors, actors, and
// genres through the modusgraph API. This ensures dgraph.type is set and
// predicates match our struct definitions.
func seedData(t *testing.T, c *movies.Client) {
	t.Helper()
	seedOnce.Do(func() {
		seedErr = doSeed(c)
	})
	if seedErr != nil {
		t.Fatalf("seed data: %v", seedErr)
	}
}

func doSeed(c *movies.Client) error {
	ctx := context.Background()

	// Genres
	action := &movies.Genre{Name: "Action"}
	scifi := &movies.Genre{Name: "Sci-Fi"}
	drama := &movies.Genre{Name: "Drama"}
	crime := &movies.Genre{Name: "Crime"}
	adventure := &movies.Genre{Name: "Adventure"}
	for _, g := range []*movies.Genre{action, scifi, drama, crime, adventure} {
		if err := c.Genre.Add(ctx, g); err != nil {
			return err
		}
	}

	// Films
	matrix := &movies.Film{
		Name:               "The Matrix",
		InitialReleaseDate: time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC),
		Tagline:            "Welcome to the Real World",
		Genres:             []movies.Genre{*action, *scifi},
	}
	matrixReloaded := &movies.Film{
		Name:               "The Matrix Reloaded",
		InitialReleaseDate: time.Date(2003, 5, 15, 0, 0, 0, 0, time.UTC),
		Tagline:            "Free your mind",
		Genres:             []movies.Genre{*action, *scifi},
	}
	starWarsIV := &movies.Film{
		Name:               "Star Wars: Episode IV - A New Hope",
		InitialReleaseDate: time.Date(1977, 5, 25, 0, 0, 0, 0, time.UTC),
		Genres:             []movies.Genre{*action, *scifi, *adventure},
	}
	starWarsV := &movies.Film{
		Name:               "Star Wars: Episode V - The Empire Strikes Back",
		InitialReleaseDate: time.Date(1980, 5, 21, 0, 0, 0, 0, time.UTC),
		Genres:             []movies.Genre{*action, *scifi, *adventure},
	}
	godfather := &movies.Film{
		Name:               "The Godfather",
		InitialReleaseDate: time.Date(1972, 3, 24, 0, 0, 0, 0, time.UTC),
		Tagline:            "An offer you can't refuse",
		Genres:             []movies.Genre{*crime, *drama},
	}
	godfatherII := &movies.Film{
		Name:               "The Godfather Part II",
		InitialReleaseDate: time.Date(1974, 12, 20, 0, 0, 0, 0, time.UTC),
		Genres:             []movies.Genre{*crime, *drama},
	}
	apocalypse := &movies.Film{
		Name:               "Apocalypse Now",
		InitialReleaseDate: time.Date(1979, 8, 15, 0, 0, 0, 0, time.UTC),
		Genres:             []movies.Genre{*drama},
	}
	warGames := &movies.Film{
		Name:               "WarGames",
		InitialReleaseDate: time.Date(1983, 6, 3, 0, 0, 0, 0, time.UTC),
		Tagline:            "Is it a game, or is it real?",
		Genres:             []movies.Genre{*scifi, *drama},
	}

	for _, f := range []*movies.Film{matrix, matrixReloaded, starWarsIV, starWarsV, godfather, godfatherII, apocalypse, warGames} {
		if err := c.Film.Add(ctx, f); err != nil {
			return err
		}
	}

	// Directors
	wachowski := &movies.Director{
		Name:  "Lana Wachowski",
		Films: []movies.Film{*matrix, *matrixReloaded},
	}
	lucas := &movies.Director{
		Name:  "George Lucas",
		Films: []movies.Film{*starWarsIV},
	}
	coppola := &movies.Director{
		Name:  "Francis Ford Coppola",
		Films: []movies.Film{*godfather, *godfatherII, *apocalypse},
	}

	for _, d := range []*movies.Director{wachowski, lucas, coppola} {
		if err := c.Director.Add(ctx, d); err != nil {
			return err
		}
	}

	// Actors
	keanu := &movies.Actor{Name: "Keanu Reeves"}
	carrie := &movies.Actor{Name: "Carrie-Anne Moss"}
	hamill := &movies.Actor{Name: "Mark Hamill"}
	pacino := &movies.Actor{Name: "Al Pacino"}
	brando := &movies.Actor{Name: "Marlon Brando"}

	for _, a := range []*movies.Actor{keanu, carrie, hamill, pacino, brando} {
		if err := c.Actor.Add(ctx, a); err != nil {
			return err
		}
	}

	return nil
}

// --- Search tests ---

func TestSearchFilmMatrix(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	results, err := c.Film.Search(ctx, "Matrix")
	if err != nil {
		t.Fatalf("Film.Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one film matching 'Matrix', got 0")
	}

	found := false
	for _, f := range results {
		t.Logf("Found film: %s (uid=%s)", f.Name, f.UID)
		if f.Name != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("search returned results but none had a name")
	}
}

func TestSearchFilmStarWars(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	results, err := c.Film.Search(ctx, "Star Wars")
	if err != nil {
		t.Fatalf("Film.Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected multiple Star Wars films, got %d", len(results))
	}
	for _, f := range results {
		t.Logf("Found film: %s (uid=%s)", f.Name, f.UID)
	}
}

func TestSearchDirectorCoppola(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	results, err := c.Director.Search(ctx, "Coppola")
	if err != nil {
		t.Fatalf("Director.Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one director matching 'Coppola', got 0")
	}
	for _, d := range results {
		t.Logf("Found director: %s (uid=%s)", d.Name, d.UID)
	}
}

func TestSearchActorKeanu(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	results, err := c.Actor.Search(ctx, "Keanu")
	if err != nil {
		t.Fatalf("Actor.Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one actor matching 'Keanu', got 0")
	}
	for _, a := range results {
		t.Logf("Found actor: %s (uid=%s)", a.Name, a.UID)
	}
}

// --- List + pagination tests ---

func TestListFilmsWithPagination(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	page1, err := c.Film.List(ctx, movies.First(3))
	if err != nil {
		t.Fatalf("Film.List page 1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("expected 3 films on page 1, got %d", len(page1))
	}

	page2, err := c.Film.List(ctx, movies.First(3), movies.Offset(3))
	if err != nil {
		t.Fatalf("Film.List page 2: %v", err)
	}
	if len(page2) == 0 {
		t.Fatal("expected films on page 2, got 0")
	}

	// Pages should have different results
	if page1[0].UID == page2[0].UID {
		t.Fatal("page 1 and page 2 returned the same first film")
	}
	t.Logf("Page 1: %d films, Page 2: %d films", len(page1), len(page2))
}

func TestListGenres(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	genres, err := c.Genre.List(ctx, movies.First(10))
	if err != nil {
		t.Fatalf("Genre.List: %v", err)
	}
	if len(genres) == 0 {
		t.Fatal("expected at least one genre, got 0")
	}
	for _, g := range genres {
		t.Logf("Genre: %s (uid=%s)", g.Name, g.UID)
	}
}

// --- Query builder tests ---

func TestQueryBuilderFilterAndOrder(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	var results []movies.Film
	err := c.Film.Query(ctx).
		Filter(`alloftext(name, "Star")`).
		First(10).
		OrderAsc("name").
		Exec(&results)
	if err != nil {
		t.Fatalf("FilmQuery.Exec: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected films matching 'Star', got 0")
	}

	// Verify ordering: names should be alphabetical
	for i := 1; i < len(results); i++ {
		if results[i].Name < results[i-1].Name {
			t.Errorf("results not in ascending order: %q came after %q",
				results[i].Name, results[i-1].Name)
		}
	}
	t.Logf("Found %d films matching 'Star', ordered by name", len(results))
}

func TestQueryBuilderExecAndCount(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	var results []movies.Film
	count, err := c.Film.Query(ctx).
		Filter(`alloftext(name, "Matrix")`).
		First(5).
		ExecAndCount(&results)
	if err != nil {
		t.Fatalf("FilmQuery.ExecAndCount: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got 0")
	}
	if count < len(results) {
		t.Fatalf("count (%d) should be >= len(results) (%d)", count, len(results))
	}
	t.Logf("Got %d results, total count: %d", len(results), count)
}

func TestQueryBuilderOrderDesc(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	var results []movies.Film
	err := c.Film.Query(ctx).
		First(5).
		OrderDesc("initial_release_date").
		Exec(&results)
	if err != nil {
		t.Fatalf("FilmQuery.Exec desc: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected films, got 0")
	}

	// Verify ordering: dates should be newest first
	for i := 1; i < len(results); i++ {
		if results[i].InitialReleaseDate.After(results[i-1].InitialReleaseDate) {
			t.Errorf("results not in descending date order: %v came after %v",
				results[i].InitialReleaseDate, results[i-1].InitialReleaseDate)
		}
	}
	t.Logf("Found %d films, newest first", len(results))
}

// --- Iterator tests ---

func TestFilmSearchIterator(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	count := 0
	for film, err := range c.Film.SearchIter(ctx, "Matrix") {
		if err != nil {
			t.Fatalf("SearchIter error: %v", err)
		}
		count++
		t.Logf("Iterator film %d: %s (uid=%s)", count, film.Name, film.UID)
		if count >= 10 {
			break
		}
	}
	if count == 0 {
		t.Fatal("iterator returned no films for 'Matrix'")
	}
	t.Logf("Iterator yielded %d films", count)
}

func TestGenreListIterator(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	count := 0
	for genre, err := range c.Genre.ListIter(ctx) {
		if err != nil {
			t.Fatalf("ListIter error: %v", err)
		}
		count++
		t.Logf("Iterator genre %d: %s", count, genre.Name)
	}
	if count == 0 {
		t.Fatal("genre iterator returned no results")
	}
	t.Logf("Genre iterator yielded %d genres total", count)
}

// --- Mutation round-trip test ---

func TestMutationRoundTrip(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	ctx := context.Background()

	// Add a new film
	newFilm := &movies.Film{
		Name:               "Integration Test Film",
		InitialReleaseDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Tagline:            "A film created by integration tests",
	}
	err := c.Film.Add(ctx, newFilm)
	if err != nil {
		t.Fatalf("Film.Add: %v", err)
	}
	if newFilm.UID == "" {
		t.Fatal("expected UID to be set after Add")
	}
	uid := newFilm.UID
	t.Logf("Created film with UID: %s", uid)

	// Get it back
	got, err := c.Film.Get(ctx, uid)
	if err != nil {
		t.Fatalf("Film.Get: %v", err)
	}
	if got.Name != "Integration Test Film" {
		t.Fatalf("expected name %q, got %q", "Integration Test Film", got.Name)
	}
	if got.Tagline != "A film created by integration tests" {
		t.Fatalf("expected tagline %q, got %q", "A film created by integration tests", got.Tagline)
	}

	// Update it
	got.Tagline = "Updated by integration tests"
	err = c.Film.Update(ctx, got)
	if err != nil {
		t.Fatalf("Film.Update: %v", err)
	}

	// Get again to verify update
	updated, err := c.Film.Get(ctx, uid)
	if err != nil {
		t.Fatalf("Film.Get after update: %v", err)
	}
	if updated.Tagline != "Updated by integration tests" {
		t.Fatalf("expected updated tagline, got %q", updated.Tagline)
	}

	// Search for it
	results, err := c.Film.Search(ctx, "Integration Test Film")
	if err != nil {
		t.Fatalf("Film.Search: %v", err)
	}
	found := false
	for _, r := range results {
		if r.UID == uid {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not find created film via search")
	}

	// Delete it
	err = c.Film.Delete(ctx, uid)
	if err != nil {
		t.Fatalf("Film.Delete: %v", err)
	}

	// Verify deletion — Get should return error or empty result
	deleted, err := c.Film.Get(ctx, uid)
	if err != nil {
		t.Logf("Film.Get after delete correctly returned error: %v", err)
	} else if deleted.Name != "" {
		t.Fatalf("expected empty film after delete, got name=%q", deleted.Name)
	}
	t.Log("Mutation round-trip passed: Add -> Get -> Update -> Get -> Search -> Delete -> Verify")
}

// --- Reverse relationship tests ---

// TestGenreReverseEdge verifies that querying a Genre via Get returns
// films linked through the ~genre reverse edge.
func TestGenreReverseEdge(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	// Find the "Action" genre
	genres, err := c.Genre.Search(ctx, "Action")
	if err != nil {
		t.Fatalf("Genre.Search: %v", err)
	}
	if len(genres) == 0 {
		t.Fatal("expected to find Action genre")
	}

	// Get the full genre by UID — should include reverse Films edge
	action, err := c.Genre.Get(ctx, genres[0].UID)
	if err != nil {
		t.Fatalf("Genre.Get: %v", err)
	}
	t.Logf("Genre: %s (uid=%s), reverse films: %d", action.Name, action.UID, len(action.Films))
	for _, f := range action.Films {
		t.Logf("  ~genre Film: %s (uid=%s)", f.Name, f.UID)
	}

	// Seed data has Action genre on: Matrix, Matrix Reloaded, Star Wars IV, Star Wars V
	if len(action.Films) < 4 {
		t.Fatalf("expected at least 4 films in Action genre via reverse edge, got %d", len(action.Films))
	}
}

// TestCountryReverseEdge verifies the ~country reverse edge by creating
// a film with a country and checking the country's reverse Films field.
func TestCountryReverseEdge(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	ctx := context.Background()

	// Create a country
	usa := &movies.Country{Name: "United States"}
	if err := c.Country.Add(ctx, usa); err != nil {
		t.Fatalf("Country.Add: %v", err)
	}
	t.Logf("Created country: %s (uid=%s)", usa.Name, usa.UID)

	// Create a film with that country on the forward edge
	film := &movies.Film{
		Name:               "Reverse Edge Test Film",
		InitialReleaseDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Countries:          []movies.Country{*usa},
	}
	if err := c.Film.Add(ctx, film); err != nil {
		t.Fatalf("Film.Add: %v", err)
	}
	t.Logf("Created film: %s (uid=%s) with country %s", film.Name, film.UID, usa.UID)

	// Get the country and check the reverse edge
	gotCountry, err := c.Country.Get(ctx, usa.UID)
	if err != nil {
		t.Fatalf("Country.Get: %v", err)
	}
	t.Logf("Country: %s, reverse films: %d", gotCountry.Name, len(gotCountry.Films))
	for _, f := range gotCountry.Films {
		t.Logf("  ~country Film: %s (uid=%s)", f.Name, f.UID)
	}

	found := false
	for _, f := range gotCountry.Films {
		if f.UID == film.UID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected film %s to appear in country's reverse Films edge", film.UID)
	}

	// Cleanup
	_ = c.Film.Delete(ctx, film.UID)
	_ = c.Country.Delete(ctx, usa.UID)
}

// TestForwardEdgeUpdateReflectsInReverse verifies that when a forward edge
// is updated on a Film, the change is immediately visible through the
// reverse edge on the target entity.
func TestForwardEdgeUpdateReflectsInReverse(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	ctx := context.Background()

	// Create two genres
	comedy := &movies.Genre{Name: "Comedy Test"}
	thriller := &movies.Genre{Name: "Thriller Test"}
	if err := c.Genre.Add(ctx, comedy); err != nil {
		t.Fatalf("Genre.Add Comedy: %v", err)
	}
	if err := c.Genre.Add(ctx, thriller); err != nil {
		t.Fatalf("Genre.Add Thriller: %v", err)
	}
	t.Logf("Created genres: Comedy=%s, Thriller=%s", comedy.UID, thriller.UID)

	// Create a film with only Comedy genre initially
	film := &movies.Film{
		Name:               "Forward-Reverse Update Test",
		InitialReleaseDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Genres:             []movies.Genre{*comedy},
	}
	if err := c.Film.Add(ctx, film); err != nil {
		t.Fatalf("Film.Add: %v", err)
	}
	t.Logf("Created film: %s (uid=%s) with Comedy genre", film.Name, film.UID)

	// Verify Comedy shows the film in its reverse edge
	gotComedy, err := c.Genre.Get(ctx, comedy.UID)
	if err != nil {
		t.Fatalf("Genre.Get Comedy: %v", err)
	}
	comedyHasFilm := false
	for _, f := range gotComedy.Films {
		if f.UID == film.UID {
			comedyHasFilm = true
		}
	}
	if !comedyHasFilm {
		t.Fatalf("expected film in Comedy's reverse edge before update, got %d films", len(gotComedy.Films))
	}
	t.Logf("Before update: Comedy has %d film(s) including our test film", len(gotComedy.Films))

	// Verify Thriller does NOT yet show the film
	gotThriller, err := c.Genre.Get(ctx, thriller.UID)
	if err != nil {
		t.Fatalf("Genre.Get Thriller: %v", err)
	}
	for _, f := range gotThriller.Films {
		if f.UID == film.UID {
			t.Fatal("film should NOT be in Thriller's reverse edge before update")
		}
	}
	t.Logf("Before update: Thriller has %d film(s), none is our test film", len(gotThriller.Films))

	// --- Update the film: add Thriller as a second genre ---
	film.Genres = []movies.Genre{*comedy, *thriller}
	if err := c.Film.Update(ctx, film); err != nil {
		t.Fatalf("Film.Update: %v", err)
	}
	t.Log("Updated film to include both Comedy and Thriller genres")

	// Verify the film's forward edge now includes both genres
	gotFilm, err := c.Film.Get(ctx, film.UID)
	if err != nil {
		t.Fatalf("Film.Get after update: %v", err)
	}
	if len(gotFilm.Genres) < 2 {
		t.Fatalf("expected at least 2 genres on film after update, got %d", len(gotFilm.Genres))
	}
	t.Logf("Film after update has %d genres", len(gotFilm.Genres))

	// Verify Thriller NOW shows the film in its reverse edge
	gotThriller2, err := c.Genre.Get(ctx, thriller.UID)
	if err != nil {
		t.Fatalf("Genre.Get Thriller after update: %v", err)
	}
	thrillerHasFilm := false
	for _, f := range gotThriller2.Films {
		if f.UID == film.UID {
			thrillerHasFilm = true
		}
	}
	if !thrillerHasFilm {
		t.Fatalf("expected film in Thriller's reverse edge after update, got %d films: %+v",
			len(gotThriller2.Films), gotThriller2.Films)
	}
	t.Logf("After update: Thriller now has %d film(s) including our test film", len(gotThriller2.Films))

	// Comedy should still have the film
	gotComedy2, err := c.Genre.Get(ctx, comedy.UID)
	if err != nil {
		t.Fatalf("Genre.Get Comedy after update: %v", err)
	}
	comedyStillHasFilm := false
	for _, f := range gotComedy2.Films {
		if f.UID == film.UID {
			comedyStillHasFilm = true
		}
	}
	if !comedyStillHasFilm {
		t.Fatalf("expected film to still be in Comedy's reverse edge after update")
	}
	t.Log("After update: Comedy still has the test film")

	// Cleanup
	_ = c.Film.Delete(ctx, film.UID)
	_ = c.Genre.Delete(ctx, comedy.UID)
	_ = c.Genre.Delete(ctx, thriller.UID)
	t.Log("Forward edge update correctly reflected in reverse edges")
}

// --- Multi-entity relationship test ---

func TestDirectorWithFilms(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	results, err := c.Director.Search(ctx, "Coppola")
	if err != nil {
		t.Fatalf("Director.Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one director matching 'Coppola'")
	}

	// Get the full director by UID to see the films edge
	director, err := c.Director.Get(ctx, results[0].UID)
	if err != nil {
		t.Fatalf("Director.Get: %v", err)
	}
	t.Logf("Director: %s (uid=%s), films: %d", director.Name, director.UID, len(director.Films))
	for _, f := range director.Films {
		t.Logf("  Film: %s (uid=%s)", f.Name, f.UID)
	}
}

// --- Raw DQL query tests ---

func TestQueryRaw(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	// Query for films using raw DQL.
	query := `{
		q(func: type(Film), first: 5, orderasc: name) {
			uid
			name
		}
	}`
	resp, err := c.QueryRaw(ctx, query, nil)
	if err != nil {
		t.Fatalf("QueryRaw: %v", err)
	}

	// Response should be valid JSON containing film data.
	if len(resp) == 0 {
		t.Fatal("QueryRaw returned empty response")
	}

	// Should contain at least one of our seeded film names.
	body := string(resp)
	if !strings.Contains(body, "The Matrix") && !strings.Contains(body, "Star Wars") {
		t.Errorf("expected response to contain seeded film names, got: %s", body)
	}
	t.Logf("QueryRaw response: %s", body)
}

func TestQueryRawWithVars(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	seedData(t, c)
	ctx := context.Background()

	// Parameterized query using DQL variables.
	query := `query films($term: string) {
		q(func: alloftext(name, $term)) {
			uid
			name
		}
	}`
	vars := map[string]string{"$term": "Matrix"}
	resp, err := c.QueryRaw(ctx, query, vars)
	if err != nil {
		t.Fatalf("QueryRaw with vars: %v", err)
	}

	body := string(resp)
	if !strings.Contains(body, "Matrix") {
		t.Errorf("expected response to contain 'Matrix', got: %s", body)
	}
	t.Logf("QueryRaw with vars response: %s", body)
}

func TestQueryRawEmptyResult(t *testing.T) {
	skipIfNoDgraph(t)
	c := newTestClient(t)
	ctx := context.Background()

	// Query for something that definitely doesn't exist.
	query := `{
		q(func: eq(name, "zzzNonExistentFilm99999")) {
			uid
			name
		}
	}`
	resp, err := c.QueryRaw(ctx, query, nil)
	if err != nil {
		t.Fatalf("QueryRaw: %v", err)
	}

	// Should return valid JSON even with no results.
	if len(resp) == 0 {
		t.Fatal("QueryRaw returned empty response for no-match query")
	}
	t.Logf("QueryRaw empty result response: %s", string(resp))
}
