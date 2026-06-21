package dnsprovider

import "testing"

func TestDNSNameHelpers(t *testing.T) {
	t.Parallel()

	if got := ensureTrailingDot("example.invalid"); got != "example.invalid." {
		t.Fatalf("expected trailing dot, got %q", got)
	}
	if got := ensureTrailingDot("example.invalid."); got != "example.invalid." {
		t.Fatalf("expected existing trailing dot, got %q", got)
	}
	if got := ensureTrailingDot(" "); got != "" {
		t.Fatalf("expected empty value, got %q", got)
	}
	if got := relativeDNSName("vip.example.invalid.", "example.invalid."); got != "vip" {
		t.Fatalf("expected relative name vip, got %q", got)
	}
	if got := relativeDNSName("other.invalid", "example.invalid"); got != "other.invalid" {
		t.Fatalf("expected unchanged name, got %q", got)
	}
}
