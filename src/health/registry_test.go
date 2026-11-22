package health

import "testing"

func TestRegistryRetainsOrderAndCachedSafeValues(t *testing.T) {
	registry := NewRegistry()
	state := "ok"
	count := int64(7)
	if err := registry.Register("geoip_country", true, func() string { return state }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("blocklist_ads", false, func() string { return "private error detail" }); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterStat("searches_total", func() int64 { return count }); err != nil {
		t.Fatal(err)
	}
	state = "degraded"
	count = 12
	checks := registry.Checks()
	if len(checks) != 2 || checks[0].Key != "geoip_country" || checks[0].Status != "ok" || checks[1].Status != "error" {
		t.Fatalf("checks before refresh = %#v", checks)
	}
	stats := registry.Stats()
	if len(stats) != 1 || stats[0].Key != "searches_total" || stats[0].Value != 7 {
		t.Fatalf("stats before refresh = %#v", stats)
	}
	registry.Refresh()
	if got := registry.Checks()[0].Status; got != "degraded" {
		t.Fatalf("refreshed check status = %q, want degraded", got)
	}
	if got := registry.Stats()[0].Value; got != 12 {
		t.Fatalf("refreshed stat = %d, want 12", got)
	}
}

func TestRegistryRejectsDuplicateAndInvalidRegistrations(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("geoip_country", false, func() string { return "ok" }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("geoip_country", false, func() string { return "ok" }); err == nil {
		t.Fatal("duplicate check registration succeeded")
	}
	if err := registry.Register("../private", false, func() string { return "ok" }); err == nil {
		t.Fatal("invalid public key registration succeeded")
	}
}
