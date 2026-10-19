package server

import (
	"encoding/base64"
	"net/http"
	"sort"
	"strings"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engine"
)

const enginePreferenceCookie = "search_pref_engines"
const engineOrderPreferenceCookie = "search_pref_engine_order"

type enginePreferenceCategory struct {
	ID      string
	Checked bool
}

type enginePreferenceRow struct {
	ID         string
	Name       string
	Enabled    bool
	Categories []enginePreferenceCategory
}

func preferenceEngines(registry *engine.Registry) []search.Engine {
	if registry == nil {
		return nil
	}
	engines := registry.GetAll()
	sort.Slice(engines, func(i, j int) bool {
		return strings.ToLower(engines[i].Name()) < strings.ToLower(engines[j].Name())
	})
	return engines
}

func enginePreferenceBytes(registry *engine.Registry) int {
	return 2 + (len(preferenceEngines(registry))*len(model.AllCategories())+7)/8
}

func decodeEnginePreferenceMask(value string, registry *engine.Registry) ([]byte, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	engineCount := len(preferenceEngines(registry))
	if err != nil || len(decoded) != enginePreferenceBytes(registry) || decoded[0] != 1 || int(decoded[1]) != engineCount {
		return nil, false
	}
	return decoded, true
}

func defaultEngineOrder(registry *engine.Registry) []search.Engine {
	engines := preferenceEngines(registry)
	sort.SliceStable(engines, func(i, j int) bool {
		if engines[i].GetPriority() == engines[j].GetPriority() {
			return strings.ToLower(engines[i].Name()) < strings.ToLower(engines[j].Name())
		}
		return engines[i].GetPriority() > engines[j].GetPriority()
	})
	return engines
}

func encodeEngineOrder(registry *engine.Registry, requested []string) string {
	engines := preferenceEngines(registry)
	if len(engines) > 255 {
		return ""
	}
	indexByName := make(map[string]byte, len(engines))
	for index, item := range engines {
		indexByName[strings.ToLower(item.Name())] = byte(index)
	}
	order := make([]byte, 2, len(engines)+2)
	order[0] = 1
	order[1] = byte(len(engines))
	seen := make(map[byte]bool, len(engines))
	for _, name := range requested {
		index, ok := indexByName[strings.ToLower(strings.TrimSpace(name))]
		if ok && !seen[index] {
			order = append(order, index)
			seen[index] = true
		}
	}
	for _, item := range defaultEngineOrder(registry) {
		index := indexByName[strings.ToLower(item.Name())]
		if !seen[index] {
			order = append(order, index)
			seen[index] = true
		}
	}
	return base64.RawURLEncoding.EncodeToString(order)
}

func decodeEngineOrder(value string, registry *engine.Registry) ([]int, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	engines := preferenceEngines(registry)
	if err != nil || len(decoded) != len(engines)+2 || decoded[0] != 1 || int(decoded[1]) != len(engines) {
		return nil, false
	}
	seen := make([]bool, len(engines))
	order := make([]int, len(decoded)-2)
	for index, value := range decoded[2:] {
		if int(value) >= len(engines) || seen[value] {
			return nil, false
		}
		seen[value] = true
		order[index] = int(value)
	}
	return order, true
}

func orderedPreferenceEngines(registry *engine.Registry, request *http.Request) []search.Engine {
	engines := preferenceEngines(registry)
	if request != nil {
		if cookie, err := request.Cookie(engineOrderPreferenceCookie); err == nil {
			if order, valid := decodeEngineOrder(cookie.Value, registry); valid {
				ordered := make([]search.Engine, 0, len(order))
				for _, index := range order {
					ordered = append(ordered, engines[index])
				}
				return ordered
			}
		}
	}
	return defaultEngineOrder(registry)
}

func engineOrderValue(registry *engine.Registry, request *http.Request) string {
	ordered := orderedPreferenceEngines(registry, request)
	names := make([]string, 0, len(ordered))
	for _, item := range ordered {
		names = append(names, item.Name())
	}
	return strings.Join(names, ",")
}

func encodeEnginePreferenceMask(registry *engine.Registry, selected []string) string {
	engines := preferenceEngines(registry)
	categories := model.AllCategories()
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		engineName, categoryName, found := strings.Cut(value, ":")
		if found && strings.TrimSpace(engineName) != "" && model.ParseCategory(categoryName).IsValid() {
			selectedSet[strings.ToLower(engineName)+":"+strings.ToLower(categoryName)] = true
		}
	}
	mask := make([]byte, enginePreferenceBytes(registry))
	mask[0] = 1
	mask[1] = byte(len(engines))
	categorySelections := make([]int, len(categories))
	for engineIndex, item := range engines {
		if !item.IsEnabled() {
			continue
		}
		for categoryIndex, category := range categories {
			if !item.SupportsCategory(category) {
				continue
			}
			key := strings.ToLower(item.Name()) + ":" + string(category)
			if selectedSet[key] {
				bit := engineIndex*len(categories) + categoryIndex
				mask[2+bit/8] |= 1 << uint(bit%8)
				categorySelections[categoryIndex]++
			}
		}
	}
	for categoryIndex, category := range categories {
		if categorySelections[categoryIndex] > 0 {
			continue
		}
		for engineIndex, item := range engines {
			if item.IsEnabled() && item.SupportsCategory(category) {
				bit := engineIndex*len(categories) + categoryIndex
				mask[2+bit/8] |= 1 << uint(bit%8)
			}
		}
	}
	return base64.RawURLEncoding.EncodeToString(mask)
}

func engineSelected(mask []byte, engineIndex, categoryIndex, categoryCount int) bool {
	bit := engineIndex*categoryCount + categoryIndex
	return 2+bit/8 < len(mask) && mask[2+bit/8]&(1<<uint(bit%8)) != 0
}

func (s *Server) enginePreferenceRows(r *http.Request) []enginePreferenceRow {
	engines := preferenceEngines(s.registry)
	orderedEngines := orderedPreferenceEngines(s.registry, r)
	mask, hasMask := []byte(nil), false
	if cookie, err := r.Cookie(enginePreferenceCookie); err == nil {
		mask, hasMask = decodeEnginePreferenceMask(cookie.Value, s.registry)
	}
	categories := model.AllCategories()
	indexByName := make(map[string]int, len(engines))
	for index, item := range engines {
		indexByName[strings.ToLower(item.Name())] = index
	}
	rows := make([]enginePreferenceRow, 0, len(engines))
	for _, item := range orderedEngines {
		engineIndex := indexByName[strings.ToLower(item.Name())]
		row := enginePreferenceRow{ID: item.Name(), Name: item.DisplayName(), Enabled: item.IsEnabled()}
		for categoryIndex, category := range categories {
			if !item.SupportsCategory(category) {
				continue
			}
			checked := !hasMask || engineSelected(mask, engineIndex, categoryIndex, len(categories))
			row.Categories = append(row.Categories, enginePreferenceCategory{ID: string(category), Checked: checked})
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) selectedEngineNames(r *http.Request, category model.Category) []string {
	cookie, err := r.Cookie(enginePreferenceCookie)
	if err != nil {
		return nil
	}
	mask, valid := decodeEnginePreferenceMask(cookie.Value, s.registry)
	if !valid {
		return nil
	}
	categories := model.AllCategories()
	categoryIndex := -1
	for index, candidate := range categories {
		if candidate == category {
			categoryIndex = index
			break
		}
	}
	if categoryIndex < 0 {
		return nil
	}
	selected := make([]string, 0)
	stableEngines := preferenceEngines(s.registry)
	indexByName := make(map[string]int, len(stableEngines))
	for index, item := range stableEngines {
		indexByName[strings.ToLower(item.Name())] = index
	}
	for _, item := range orderedPreferenceEngines(s.registry, r) {
		index := indexByName[strings.ToLower(item.Name())]
		if item.IsEnabled() && item.SupportsCategory(category) && engineSelected(mask, index, categoryIndex, len(categories)) {
			selected = append(selected, item.Name())
		}
	}
	return selected
}
