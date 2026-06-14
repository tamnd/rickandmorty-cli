package rickandmorty

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes The Rick and Morty API as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/rickandmorty-cli/rickandmorty"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences rickandmorty:// URIs by routing to the operations Register
// installs. The same Domain also builds the standalone rickandmorty binary
// (see cli.NewApp), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the Rick and Morty API driver. It carries no state; the per-run
// client is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "rickandmorty",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "rickandmorty",
			Short:  "Explore The Rick and Morty API from the command line",
			Long: `rickandmorty reads public data from rickandmortyapi.com, shapes it into
clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/rickandmorty-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// characters: search/list characters
	kit.Handle(app, kit.OpMeta{
		Name:    "characters",
		Group:   "read",
		List:    true,
		Summary: "Search characters",
		URIType: "character",
	}, listCharacters)

	// character: fetch one character by ID
	kit.Handle(app, kit.OpMeta{
		Name:    "character",
		Group:   "read",
		Single:  true,
		Summary: "Fetch a character by ID",
		URIType: "character",
		Resolver: true,
		Args:    []kit.Arg{{Name: "id", Help: "character ID"}},
	}, getCharacter)

	// episodes: search/list episodes
	kit.Handle(app, kit.OpMeta{
		Name:    "episodes",
		Group:   "read",
		List:    true,
		Summary: "Search episodes",
		URIType: "episode",
	}, listEpisodes)

	// locations: search/list locations
	kit.Handle(app, kit.OpMeta{
		Name:    "locations",
		Group:   "read",
		List:    true,
		Summary: "Search locations",
		URIType: "location",
	}, listLocations)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- input structs ---

type characterListIn struct {
	Name    string  `kit:"flag" help:"filter by name"`
	Status  string  `kit:"flag" help:"filter by status (alive, dead, unknown)"`
	Species string  `kit:"flag" help:"filter by species"`
	Page    int     `kit:"flag" help:"page number (default 1)"`
	Client  *Client `kit:"inject"`
}

type characterGetIn struct {
	ID     string  `kit:"arg" help:"character ID"`
	Client *Client `kit:"inject"`
}

type episodeListIn struct {
	Name    string  `kit:"flag" help:"filter by episode name"`
	Episode string  `kit:"flag" help:"filter by episode code (e.g. S01E01)"`
	Page    int     `kit:"flag" help:"page number (default 1)"`
	Client  *Client `kit:"inject"`
}

type locationListIn struct {
	Name      string  `kit:"flag" help:"filter by name"`
	Type      string  `kit:"flag" help:"filter by type"`
	Dimension string  `kit:"flag" help:"filter by dimension"`
	Page      int     `kit:"flag" help:"page number (default 1)"`
	Client    *Client `kit:"inject"`
}

// --- handlers ---

func listCharacters(ctx context.Context, in characterListIn, emit func(*Character) error) error {
	results, err := in.Client.SearchCharacters(ctx, CharacterQuery{
		Name:    in.Name,
		Status:  in.Status,
		Species: in.Species,
		Page:    in.Page,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func getCharacter(ctx context.Context, in characterGetIn, emit func(*Character) error) error {
	id, err := parseID(in.ID)
	if err != nil {
		return errs.Usage("character id must be a number: %s", in.ID)
	}
	c, err := in.Client.GetCharacter(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	return emit(c)
}

func listEpisodes(ctx context.Context, in episodeListIn, emit func(*Episode) error) error {
	results, err := in.Client.SearchEpisodes(ctx, EpisodeQuery{
		Name:    in.Name,
		Episode: in.Episode,
		Page:    in.Page,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func listLocations(ctx context.Context, in locationListIn, emit func(*Location) error) error {
	results, err := in.Client.SearchLocations(ctx, LocationQuery{
		Name:      in.Name,
		Type:      in.Type,
		Dimension: in.Dimension,
		Page:      in.Page,
	})
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

// Classify turns any accepted input into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty reference")
	}
	// Accept bare numeric ids as characters.
	if _, e := strconv.Atoi(input); e == nil {
		return "character", input, nil
	}
	return "", "", errs.Usage("unrecognized rickandmorty reference: %q", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "character":
		return fmt.Sprintf("%s/api/character/%s", BaseURL, id), nil
	case "episode":
		return fmt.Sprintf("%s/api/episode/%s", BaseURL, id), nil
	case "location":
		return fmt.Sprintf("%s/api/location/%s", BaseURL, id), nil
	default:
		return "", errs.Usage("rickandmorty has no resource type %q", uriType)
	}
}

// --- helpers ---

func parseID(s string) (int, error) {
	s = strings.TrimSpace(s)
	return strconv.Atoi(s)
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
