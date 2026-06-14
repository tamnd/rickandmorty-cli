// Package rickandmorty is the library behind the rickandmorty command line:
// the HTTP client, request shaping, and the typed data models for The Rick and
// Morty API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package rickandmorty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Host is the site this client talks to.
const Host = "rickandmortyapi.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// DefaultUserAgent identifies the client to the API.
const DefaultUserAgent = "rickandmorty-cli/0.1 (tamnd87@gmail.com)"

// --- data models ---

// Character is a single character record, flattened from the API response.
type Character struct {
	ID       int    `json:"id"       kit:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Species  string `json:"species"`
	Gender   string `json:"gender"`
	Origin   string `json:"origin"`
	Location string `json:"location"`
	Episodes int    `json:"episodes"`
}

// Episode is a single episode record.
type Episode struct {
	ID        int    `json:"id"        kit:"id"`
	Name      string `json:"name"`
	AirDate   string `json:"air_date"`
	Episode   string `json:"episode"`
	CharCount int    `json:"char_count"`
}

// Location is a single location record.
type Location struct {
	ID        int    `json:"id"        kit:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Dimension string `json:"dimension"`
	Residents int    `json:"residents"`
}

// --- raw API shapes ---

type apiInfo struct {
	Count int    `json:"count"`
	Pages int    `json:"pages"`
	Next  string `json:"next"`
	Prev  string `json:"prev"`
}

type rawNameURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type rawCharacter struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	Status   string     `json:"status"`
	Species  string     `json:"species"`
	Type     string     `json:"type"`
	Gender   string     `json:"gender"`
	Origin   rawNameURL `json:"origin"`
	Location rawNameURL `json:"location"`
	Image    string     `json:"image"`
	Episode  []string   `json:"episode"`
	URL      string     `json:"url"`
}

func (r rawCharacter) toCharacter() *Character {
	return &Character{
		ID:       r.ID,
		Name:     r.Name,
		Status:   r.Status,
		Species:  r.Species,
		Gender:   r.Gender,
		Origin:   r.Origin.Name,
		Location: r.Location.Name,
		Episodes: len(r.Episode),
	}
}

type rawEpisode struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	AirDate    string   `json:"air_date"`
	Episode    string   `json:"episode"`
	Characters []string `json:"characters"`
	URL        string   `json:"url"`
}

func (r rawEpisode) toEpisode() *Episode {
	return &Episode{
		ID:        r.ID,
		Name:      r.Name,
		AirDate:   r.AirDate,
		Episode:   r.Episode,
		CharCount: len(r.Characters),
	}
}

type rawLocation struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Dimension string   `json:"dimension"`
	Residents []string `json:"residents"`
	URL       string   `json:"url"`
}

func (r rawLocation) toLocation() *Location {
	return &Location{
		ID:        r.ID,
		Name:      r.Name,
		Type:      r.Type,
		Dimension: r.Dimension,
		Residents: len(r.Residents),
	}
}

type characterPage struct {
	Info    apiInfo        `json:"info"`
	Results []rawCharacter `json:"results"`
}

type episodePage struct {
	Info    apiInfo      `json:"info"`
	Results []rawEpisode `json:"results"`
}

type locationPage struct {
	Info    apiInfo       `json:"info"`
	Results []rawLocation `json:"results"`
}

// --- client ---

// Config holds the tunable parameters for the client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for talking to rickandmortyapi.com.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		Rate:      200 * time.Millisecond,
		Timeout:   15 * time.Second,
		Retries:   3,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to The Rick and Morty API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches a URL and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- API methods ---

// CharacterQuery holds the search parameters for the character list endpoint.
type CharacterQuery struct {
	Name    string
	Status  string
	Species string
	Page    int
}

// SearchCharacters searches the character list endpoint with the given query.
func (c *Client) SearchCharacters(ctx context.Context, q CharacterQuery) ([]*Character, error) {
	u := c.buildURL("/api/character", map[string]string{
		"name":    q.Name,
		"status":  q.Status,
		"species": q.Species,
		"page":    pageStr(q.Page),
	})
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var page characterPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse characters: %w", err)
	}
	out := make([]*Character, 0, len(page.Results))
	for _, r := range page.Results {
		out = append(out, r.toCharacter())
	}
	return out, nil
}

// GetCharacter fetches a single character by its integer ID.
func (c *Client) GetCharacter(ctx context.Context, id int) (*Character, error) {
	u := fmt.Sprintf("%s/api/character/%d", c.BaseURL, id)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw rawCharacter
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse character: %w", err)
	}
	return raw.toCharacter(), nil
}

// EpisodeQuery holds the search parameters for the episode list endpoint.
type EpisodeQuery struct {
	Name    string
	Episode string
	Page    int
}

// SearchEpisodes searches the episode list endpoint with the given query.
func (c *Client) SearchEpisodes(ctx context.Context, q EpisodeQuery) ([]*Episode, error) {
	u := c.buildURL("/api/episode", map[string]string{
		"name":    q.Name,
		"episode": q.Episode,
		"page":    pageStr(q.Page),
	})
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var page episodePage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse episodes: %w", err)
	}
	out := make([]*Episode, 0, len(page.Results))
	for _, r := range page.Results {
		out = append(out, r.toEpisode())
	}
	return out, nil
}

// LocationQuery holds the search parameters for the location list endpoint.
type LocationQuery struct {
	Name      string
	Type      string
	Dimension string
	Page      int
}

// SearchLocations searches the location list endpoint with the given query.
func (c *Client) SearchLocations(ctx context.Context, q LocationQuery) ([]*Location, error) {
	u := c.buildURL("/api/location", map[string]string{
		"name":      q.Name,
		"type":      q.Type,
		"dimension": q.Dimension,
		"page":      pageStr(q.Page),
	})
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var page locationPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse locations: %w", err)
	}
	out := make([]*Location, 0, len(page.Results))
	for _, r := range page.Results {
		out = append(out, r.toLocation())
	}
	return out, nil
}

// buildURL assembles a full URL from a path and a map of query parameters,
// omitting any parameters with empty values.
func (c *Client) buildURL(path string, params map[string]string) string {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	v := url.Values{}
	for k, val := range params {
		if val != "" {
			v.Set(k, val)
		}
	}
	if len(v) == 0 {
		return base + path
	}
	return base + path + "?" + v.Encode()
}

// pageStr converts a page number to a string, returning an empty string for
// zero or negative values so the parameter is omitted (the API defaults to 1).
func pageStr(page int) string {
	if page <= 0 {
		return ""
	}
	return strconv.Itoa(page)
}
