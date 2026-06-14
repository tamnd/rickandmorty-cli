package rickandmorty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- helpers ---

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.BaseURL = srv.URL
	c.Rate = 0 // no pacing in tests
	return c
}

// --- basic transport tests ---

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

// --- character tests ---

func TestSearchCharacters(t *testing.T) {
	payload := map[string]any{
		"info": map[string]any{"count": 1, "pages": 1},
		"results": []map[string]any{
			{
				"id":      1,
				"name":    "Rick Sanchez",
				"status":  "Alive",
				"species": "Human",
				"gender":  "Male",
				"origin":  map[string]any{"name": "Earth (C-137)", "url": ""},
				"location": map[string]any{"name": "Citadel of Ricks", "url": ""},
				"episode": []string{"ep1", "ep2", "ep3"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/character" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.SearchCharacters(context.Background(), CharacterQuery{Name: "rick"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	ch := results[0]
	if ch.ID != 1 {
		t.Errorf("ID = %d, want 1", ch.ID)
	}
	if ch.Name != "Rick Sanchez" {
		t.Errorf("Name = %q, want Rick Sanchez", ch.Name)
	}
	if ch.Origin != "Earth (C-137)" {
		t.Errorf("Origin = %q, want Earth (C-137)", ch.Origin)
	}
	if ch.Location != "Citadel of Ricks" {
		t.Errorf("Location = %q, want Citadel of Ricks", ch.Location)
	}
	if ch.Episodes != 3 {
		t.Errorf("Episodes = %d, want 3", ch.Episodes)
	}
}

func TestGetCharacter(t *testing.T) {
	payload := map[string]any{
		"id":      2,
		"name":    "Morty Smith",
		"status":  "Alive",
		"species": "Human",
		"gender":  "Male",
		"origin":  map[string]any{"name": "Earth (C-137)", "url": ""},
		"location": map[string]any{"name": "Earth (Replacement Dimension)", "url": ""},
		"episode": []string{"ep1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/character/2" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ch, err := c.GetCharacter(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if ch.ID != 2 {
		t.Errorf("ID = %d, want 2", ch.ID)
	}
	if ch.Name != "Morty Smith" {
		t.Errorf("Name = %q, want Morty Smith", ch.Name)
	}
	if ch.Episodes != 1 {
		t.Errorf("Episodes = %d, want 1", ch.Episodes)
	}
}

// --- episode tests ---

func TestSearchEpisodes(t *testing.T) {
	payload := map[string]any{
		"info": map[string]any{"count": 1, "pages": 1},
		"results": []map[string]any{
			{
				"id":         1,
				"name":       "Pilot",
				"air_date":   "December 2, 2013",
				"episode":    "S01E01",
				"characters": []string{"c1", "c2", "c3", "c4", "c5"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/episode" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.SearchEpisodes(context.Background(), EpisodeQuery{Episode: "S01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	ep := results[0]
	if ep.ID != 1 {
		t.Errorf("ID = %d, want 1", ep.ID)
	}
	if ep.Name != "Pilot" {
		t.Errorf("Name = %q, want Pilot", ep.Name)
	}
	if ep.Episode != "S01E01" {
		t.Errorf("Episode = %q, want S01E01", ep.Episode)
	}
	if ep.CharCount != 5 {
		t.Errorf("CharCount = %d, want 5", ep.CharCount)
	}
}

// --- location tests ---

func TestSearchLocations(t *testing.T) {
	payload := map[string]any{
		"info": map[string]any{"count": 1, "pages": 1},
		"results": []map[string]any{
			{
				"id":        1,
				"name":      "Earth (C-137)",
				"type":      "Planet",
				"dimension": "Dimension C-137",
				"residents": []string{"r1", "r2"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/location" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.SearchLocations(context.Background(), LocationQuery{Type: "planet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	loc := results[0]
	if loc.ID != 1 {
		t.Errorf("ID = %d, want 1", loc.ID)
	}
	if loc.Name != "Earth (C-137)" {
		t.Errorf("Name = %q, want Earth (C-137)", loc.Name)
	}
	if loc.Dimension != "Dimension C-137" {
		t.Errorf("Dimension = %q, want Dimension C-137", loc.Dimension)
	}
	if loc.Residents != 2 {
		t.Errorf("Residents = %d, want 2", loc.Residents)
	}
}

// --- helpers ---

func TestPageStr(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "1"},
		{42, "42"},
	}
	for _, tc := range cases {
		got := pageStr(tc.in)
		if got != tc.want {
			t.Errorf("pageStr(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	c := NewClient()
	c.BaseURL = "https://example.com"

	got := c.buildURL("/api/character", map[string]string{"name": "rick", "status": ""})
	// status is empty, should be omitted; name should appear
	u, err := http.NewRequest(http.MethodGet, got, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.URL.Query().Get("name") != "rick" {
		t.Errorf("name param missing from %s", got)
	}
	if u.URL.Query().Get("status") != "" {
		t.Errorf("empty status param should be omitted, got %s", got)
	}
}
