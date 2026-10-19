package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search/engine"
)

func TestEnginePreferenceMaskEncodesCategorySelectionsWithinCookieLimit(t *testing.T) {
	registry := engine.DefaultRegistry()
	available := preferenceEngines(registry)
	var selected []string
	var wantGeneral string
	for _, item := range available {
		if item.IsEnabled() && item.SupportsCategory(model.CategoryGeneral) {
			wantGeneral = item.Name()
			selected = []string{item.Name() + ":general"}
			break
		}
	}
	if wantGeneral == "" {
		t.Fatal("default registry has no enabled general engine")
	}
	encoded := encodeEnginePreferenceMask(registry, selected)
	if len(encoded) > 64 {
		t.Fatalf("engine preference cookie is %d bytes, exceeds the 64-byte limit", len(encoded))
	}
	mask, valid := decodeEnginePreferenceMask(encoded, registry)
	if !valid {
		t.Fatal("encoded engine preference mask did not decode")
	}
	request := httptest.NewRequest("GET", "/search?q=test", nil)
	request.AddCookie(&http.Cookie{Name: enginePreferenceCookie, Value: encoded})
	server := &Server{registry: registry}
	if got := server.selectedEngineNames(request, model.CategoryGeneral); len(got) != 1 || got[0] != wantGeneral {
		t.Fatalf("selected general engines = %v, want [%s]", got, wantGeneral)
	}
	for index, item := range available {
		for categoryIndex, category := range model.AllCategories() {
			want := item.IsEnabled() && item.SupportsCategory(category) && (category != model.CategoryGeneral || item.Name() == wantGeneral)
			if got := engineSelected(mask, index, categoryIndex, len(model.AllCategories())); got != want {
				t.Fatalf("mask selection for %s/%s = %t, want %t", item.Name(), category, got, want)
			}
		}
	}
}

func TestEnginePreferenceMaskPreventsEmptySupportedCategories(t *testing.T) {
	registry := engine.DefaultRegistry()
	encoded := encodeEnginePreferenceMask(registry, nil)
	mask, valid := decodeEnginePreferenceMask(encoded, registry)
	if !valid {
		t.Fatal("default engine preference mask did not decode")
	}
	for categoryIndex, category := range model.AllCategories() {
		found := false
		for engineIndex, item := range preferenceEngines(registry) {
			if item.IsEnabled() && item.SupportsCategory(category) && engineSelected(mask, engineIndex, categoryIndex, len(model.AllCategories())) {
				found = true
				break
			}
		}
		for _, item := range preferenceEngines(registry) {
			if item.IsEnabled() && item.SupportsCategory(category) {
				if !found {
					t.Errorf("supported category %s has no enabled engine", category)
				}
				break
			}
		}
	}
}

func TestEnginePreferenceOrderOverridesDefaultPriority(t *testing.T) {
	registry := engine.DefaultRegistry()
	defaultOrder := defaultEngineOrder(registry)
	if len(defaultOrder) < 2 {
		t.Skip("default registry has fewer than two engines")
	}
	wanted := []string{defaultOrder[1].Name(), defaultOrder[0].Name()}
	for _, item := range defaultOrder[2:] {
		wanted = append(wanted, item.Name())
	}
	encoded := encodeEngineOrder(registry, wanted)
	if len(encoded) > 64 {
		t.Fatalf("engine-order cookie is %d bytes, exceeds the 64-byte limit", len(encoded))
	}
	order, valid := decodeEngineOrder(encoded, registry)
	if !valid || len(order) != len(wanted) {
		t.Fatalf("encoded engine order did not decode: valid=%v order=%v", valid, order)
	}
	request := httptest.NewRequest("GET", "/server/preferences", nil)
	request.AddCookie(&http.Cookie{Name: engineOrderPreferenceCookie, Value: encoded})
	ordered := orderedPreferenceEngines(registry, request)
	for index, name := range wanted {
		if ordered[index].Name() != name {
			t.Fatalf("engine order[%d]=%q, want %q", index, ordered[index].Name(), name)
		}
	}
}
