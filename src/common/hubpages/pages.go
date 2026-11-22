package hubpages

import "sort"

type Page struct {
	Path           string
	Title          string
	TitleKey       string
	DescriptionKey string
	Order          int
}

func Preferences(ownerTokens bool) []Page {
	pages := []Page{
		{Path: "/server/preferences/tools", Title: "Search tools", TitleKey: "preferences.tools_page_title", DescriptionKey: "preferences.tools_page_description", Order: 20},
		{Path: "/server/preferences/sync", Title: "Sync preferences", TitleKey: "preferences.sync_title", DescriptionKey: "preferences.sync_description", Order: 30},
	}
	if ownerTokens {
		pages = append(pages, Page{Path: "/server/preferences/tokens", Title: "API tokens", TitleKey: "preferences.owner_tokens.heading", DescriptionKey: "preferences.owner_tokens.description", Order: 10})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Order < pages[j].Order })
	return pages
}

func Help(mcpEnabled bool) []Page {
	if !mcpEnabled {
		return nil
	}
	return []Page{{Path: "/server/help/mcp", Title: "MCP connection help", TitleKey: "mcp_help.title", DescriptionKey: "mcp_help.description", Order: 10}}
}
