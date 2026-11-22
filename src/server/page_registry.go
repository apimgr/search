package server

import "net/http"

import "github.com/apimgr/search/src/common/hubpages"

type HubPage = hubpages.Page

func (s *Server) preferencesPages() []HubPage {
	return hubpages.Preferences(s.ownerManager != nil)
}

func (s *Server) helpPages(r *http.Request) []HubPage {
	return hubpages.Help(s.apiHandler != nil)
}
