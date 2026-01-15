// Package api provides the API client for the search server
// Per AI.md PART 36: Admin API methods
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ====== USER ADMIN API ======

// AdminListUsers lists all users
func (c *Client) AdminListUsers(limit, offset int, status string) (interface{}, error) {
	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))
	if status != "" && status != "all" {
		params.Set("status", status)
	}
	return c.adminGet("/api/v1/admin/users?" + params.Encode())
}

// AdminGetUser gets user details
func (c *Client) AdminGetUser(username string) (interface{}, error) {
	return c.adminGet("/api/v1/admin/users/" + url.PathEscape(username))
}

// AdminCreateUser creates a new user
func (c *Client) AdminCreateUser(username, email, role string) (interface{}, error) {
	body := map[string]string{
		"username": username,
		"email":    email,
		"role":     role,
	}
	return c.adminPost("/api/v1/admin/users", body)
}

// AdminDeleteUser deletes a user
func (c *Client) AdminDeleteUser(username string) error {
	_, err := c.adminDelete("/api/v1/admin/users/" + url.PathEscape(username))
	return err
}

// AdminSuspendUser suspends a user
func (c *Client) AdminSuspendUser(username string) error {
	_, err := c.adminPost("/api/v1/admin/users/"+url.PathEscape(username)+"/suspend", nil)
	return err
}

// AdminUnsuspendUser unsuspends a user
func (c *Client) AdminUnsuspendUser(username string) error {
	_, err := c.adminPost("/api/v1/admin/users/"+url.PathEscape(username)+"/unsuspend", nil)
	return err
}

// AdminResetUserPassword sends password reset email
func (c *Client) AdminResetUserPassword(username string) error {
	_, err := c.adminPost("/api/v1/admin/users/"+url.PathEscape(username)+"/reset-password", nil)
	return err
}

// AdminDisableUser2FA disables 2FA for user
func (c *Client) AdminDisableUser2FA(username string) error {
	_, err := c.adminPost("/api/v1/admin/users/"+url.PathEscape(username)+"/disable-2fa", nil)
	return err
}

// ====== ORG ADMIN API ======

// AdminListOrgs lists all organizations
func (c *Client) AdminListOrgs(limit, offset int) (interface{}, error) {
	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))
	return c.adminGet("/api/v1/admin/orgs?" + params.Encode())
}

// AdminGetOrg gets organization details
func (c *Client) AdminGetOrg(orgname string) (interface{}, error) {
	return c.adminGet("/api/v1/admin/orgs/" + url.PathEscape(orgname))
}

// AdminCreateOrg creates a new organization
func (c *Client) AdminCreateOrg(orgname, displayName, description string) (interface{}, error) {
	body := map[string]string{
		"name":         orgname,
		"display_name": displayName,
		"description":  description,
	}
	return c.adminPost("/api/v1/admin/orgs", body)
}

// AdminDeleteOrg deletes an organization
func (c *Client) AdminDeleteOrg(orgname string) error {
	_, err := c.adminDelete("/api/v1/admin/orgs/" + url.PathEscape(orgname))
	return err
}

// AdminListOrgMembers lists organization members
func (c *Client) AdminListOrgMembers(orgname, role string) (interface{}, error) {
	params := url.Values{}
	if role != "" {
		params.Set("role", role)
	}
	path := "/api/v1/admin/orgs/" + url.PathEscape(orgname) + "/members"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	return c.adminGet(path)
}

// AdminAddOrgMember adds a member to organization
func (c *Client) AdminAddOrgMember(orgname, username, role string) error {
	body := map[string]string{
		"username": username,
		"role":     role,
	}
	_, err := c.adminPost("/api/v1/admin/orgs/"+url.PathEscape(orgname)+"/members", body)
	return err
}

// AdminRemoveOrgMember removes a member from organization
func (c *Client) AdminRemoveOrgMember(orgname, username string) error {
	_, err := c.adminDelete("/api/v1/admin/orgs/" + url.PathEscape(orgname) + "/members/" + url.PathEscape(username))
	return err
}

// ====== TOKEN ADMIN API ======

// AdminListTokens lists all tokens
func (c *Client) AdminListTokens(limit int, user string) (interface{}, error) {
	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", limit))
	if user != "" {
		params.Set("user", user)
	}
	return c.adminGet("/api/v1/admin/tokens?" + params.Encode())
}

// AdminCreateToken creates a new API token
func (c *Client) AdminCreateToken(name, expires, scopes string) (interface{}, error) {
	body := map[string]string{
		"name":    name,
		"expires": expires,
		"scopes":  scopes,
	}
	return c.adminPost("/api/v1/admin/tokens", body)
}

// AdminRevokeToken revokes a token
func (c *Client) AdminRevokeToken(tokenID string) error {
	_, err := c.adminDelete("/api/v1/admin/tokens/" + url.PathEscape(tokenID))
	return err
}

// AdminGetTokenInfo gets token details
func (c *Client) AdminGetTokenInfo(tokenID string) (interface{}, error) {
	return c.adminGet("/api/v1/admin/tokens/" + url.PathEscape(tokenID))
}

// ====== SERVER CONFIG API ======

// AdminListConfig lists all configuration keys
func (c *Client) AdminListConfig(category string) (interface{}, error) {
	params := url.Values{}
	if category != "" {
		params.Set("category", category)
	}
	path := "/api/v1/admin/server/config"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	return c.adminGet(path)
}

// AdminGetConfig gets configuration value
func (c *Client) AdminGetConfig(key string) (interface{}, error) {
	path := "/api/v1/admin/server/config"
	if key != "" {
		path += "/" + url.PathEscape(key)
	}
	return c.adminGet(path)
}

// AdminSetConfig sets configuration value
func (c *Client) AdminSetConfig(key, value string, reload bool) error {
	body := map[string]interface{}{
		"value":  value,
		"reload": reload,
	}
	_, err := c.adminPut("/api/v1/admin/server/config/"+url.PathEscape(key), body)
	return err
}

// AdminResetConfig resets configuration to default
func (c *Client) AdminResetConfig(key string) error {
	_, err := c.adminDelete("/api/v1/admin/server/config/" + url.PathEscape(key))
	return err
}

// ====== SERVER ADMIN API ======

// AdminListServerAdmins lists all server admins
func (c *Client) AdminListServerAdmins() (interface{}, error) {
	return c.adminGet("/api/v1/admin/server/admins")
}

// AdminInviteServerAdmin invites a new server admin
func (c *Client) AdminInviteServerAdmin(username, email string) error {
	body := map[string]string{
		"username": username,
		"email":    email,
	}
	_, err := c.adminPost("/api/v1/admin/server/admins", body)
	return err
}

// AdminRemoveServerAdmin removes a server admin
func (c *Client) AdminRemoveServerAdmin(username string) error {
	_, err := c.adminDelete("/api/v1/admin/server/admins/" + url.PathEscape(username))
	return err
}

// AdminResetServerAdminPassword sends password reset to admin
func (c *Client) AdminResetServerAdminPassword(username string) error {
	_, err := c.adminPost("/api/v1/admin/server/admins/"+url.PathEscape(username)+"/reset-password", nil)
	return err
}

// ====== SERVER STATS API ======

// AdminGetStatsOverview gets general server statistics
func (c *Client) AdminGetStatsOverview() (interface{}, error) {
	return c.adminGet("/api/v1/admin/server/stats/overview")
}

// AdminGetStatsUsers gets user statistics
func (c *Client) AdminGetStatsUsers() (interface{}, error) {
	return c.adminGet("/api/v1/admin/server/stats/users")
}

// AdminGetStatsStorage gets storage usage statistics
func (c *Client) AdminGetStatsStorage() (interface{}, error) {
	return c.adminGet("/api/v1/admin/server/stats/storage")
}

// AdminGetStatsPerformance gets performance metrics
func (c *Client) AdminGetStatsPerformance() (interface{}, error) {
	return c.adminGet("/api/v1/admin/server/stats/performance")
}

// ====== HTTP HELPERS ======

func (c *Client) adminGet(path string) (interface{}, error) {
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

func (c *Client) adminPost(path string, body interface{}) (interface{}, error) {
	resp, err := c.doRequest("POST", path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Some POST requests may return no body
		return nil, nil
	}
	return result, nil
}

func (c *Client) adminPut(path string, body interface{}) (interface{}, error) {
	resp, err := c.doRequest("PUT", path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}
	return result, nil
}

func (c *Client) adminDelete(path string) (interface{}, error) {
	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequest(method, c.BaseURL+path, bodyReader)
	} else {
		req, err = http.NewRequest(method, c.BaseURL+path, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", ProjectName, Version))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		resp.Body.Close()
		if errResp.Message != "" {
			return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.Message)
		}
		return nil, fmt.Errorf("server error %d", resp.StatusCode)
	}

	return resp, nil
}
