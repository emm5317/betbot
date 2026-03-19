package adapters

import (
	"testing"

	"betbot/internal/execution"
)

func TestNewBookAdapterPaper(t *testing.T) {
	adapter, err := NewBookAdapter("")
	if err != nil {
		t.Fatalf("NewBookAdapter() error = %v", err)
	}
	if adapter.Name() != execution.AdapterPaper {
		t.Fatalf("adapter.Name() = %q, want %q", adapter.Name(), execution.AdapterPaper)
	}
}

func TestNewBookAdapterLiveFailsClosed(t *testing.T) {
	if _, err := NewBookAdapter(execution.AdapterPinnacle); err == nil {
		t.Fatal("NewBookAdapter() expected error for unimplemented live adapter")
	}
}
