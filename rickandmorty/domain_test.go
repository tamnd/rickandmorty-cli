package rickandmorty

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The
// client's HTTP behaviour is covered in rickandmorty_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "rickandmorty" {
		t.Errorf("Scheme = %q, want rickandmorty", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "rickandmorty" {
		t.Errorf("Identity.Binary = %q, want rickandmorty", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
		ok  bool
	}{
		{"1", "character", "1", true},
		{"42", "character", "42", true},
		{"abc", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.ok {
			if err != nil || typ != tc.typ || id != tc.id {
				t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
					tc.in, typ, id, err, tc.typ, tc.id)
			}
		} else {
			if err == nil {
				t.Errorf("Classify(%q) expected error, got (%q, %q, nil)", tc.in, typ, id)
			}
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"character", "1", BaseURL + "/api/character/1"},
		{"episode", "5", BaseURL + "/api/episode/5"},
		{"location", "3", BaseURL + "/api/location/3"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("bogus", "1")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	ch := &Character{ID: 1, Name: "Rick Sanchez", Status: "Alive", Species: "Human"}
	u, err := h.Mint(ch)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "rickandmorty://character/1"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("rickandmorty", "7")
	if err != nil || got.String() != "rickandmorty://character/7" {
		t.Errorf("ResolveOn = (%q, %v), want rickandmorty://character/7", got.String(), err)
	}
}
