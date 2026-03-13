package server

import "testing"

func TestResolveSportFilterAllSports(t *testing.T) {
	selection, err := resolveSportFilter("")
	if err != nil {
		t.Fatalf("resolveSportFilter() error = %v", err)
	}
	if selection.Key != "" {
		t.Fatalf("selection.Key = %q, want empty", selection.Key)
	}
	if selection.storeParam() != nil {
		t.Fatal("selection.storeParam() expected nil for all-sports")
	}
}

func TestResolveSportFilterValidKey(t *testing.T) {
	selection, err := resolveSportFilter("basketball_nba")
	if err != nil {
		t.Fatalf("resolveSportFilter() error = %v", err)
	}
	if selection.Key != "basketball_nba" {
		t.Fatalf("selection.Key = %q, want basketball_nba", selection.Key)
	}
	if selection.Label != "NBA" {
		t.Fatalf("selection.Label = %q, want NBA", selection.Label)
	}
	storeValue := selection.storeParam()
	if storeValue == nil || *storeValue != "NBA" {
		t.Fatalf("selection.storeParam() = %v, want NBA", storeValue)
	}
}

func TestResolveSportFilterInvalidKey(t *testing.T) {
	_, err := resolveSportFilter("soccer_epl")
	if err == nil {
		t.Fatal("resolveSportFilter() expected error for invalid key")
	}
	if got := err.Error(); got == "" || got == "invalid sport filter \"soccer_epl\"" {
		t.Fatalf("resolveSportFilter() error = %q, want explicit allowed values", got)
	}
}
