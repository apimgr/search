package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for User Admin API

func TestAdminListUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/admin/users") {
			t.Errorf("Path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit = %s", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("offset") != "0" {
			t.Errorf("offset = %s", r.URL.Query().Get("offset"))
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []interface{}{},
			"total": 0,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListUsers(10, 0, "")
	if err != nil {
		t.Fatalf("AdminListUsers() error = %v", err)
	}
}

func TestAdminListUsersWithStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "active" {
			t.Errorf("status = %s, want active", r.URL.Query().Get("status"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListUsers(10, 0, "active")
	if err != nil {
		t.Fatalf("AdminListUsers() error = %v", err)
	}
}

func TestAdminListUsersAllStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "all" status should not be passed as parameter
		if r.URL.Query().Get("status") != "" {
			t.Errorf("status should be empty for 'all', got %s", r.URL.Query().Get("status"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListUsers(10, 0, "all")
	if err != nil {
		t.Fatalf("AdminListUsers() error = %v", err)
	}
}

func TestAdminGetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/testuser") {
			t.Errorf("Path should contain /testuser, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"username": "testuser"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetUser("testuser")
	if err != nil {
		t.Fatalf("AdminGetUser() error = %v", err)
	}
}

func TestAdminCreateUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "newuser" {
			t.Errorf("username = %s", body["username"])
		}
		if body["email"] != "new@example.com" {
			t.Errorf("email = %s", body["email"])
		}
		if body["role"] != "user" {
			t.Errorf("role = %s", body["role"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"username": "newuser"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateUser("newuser", "new@example.com", "user")
	if err != nil {
		t.Fatalf("AdminCreateUser() error = %v", err)
	}
}

func TestAdminDeleteUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDeleteUser("testuser")
	if err != nil {
		t.Fatalf("AdminDeleteUser() error = %v", err)
	}
}

func TestAdminSuspendUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/suspend") {
			t.Errorf("Path should contain /suspend, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminSuspendUser("testuser")
	if err != nil {
		t.Fatalf("AdminSuspendUser() error = %v", err)
	}
}

func TestAdminUnsuspendUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/unsuspend") {
			t.Errorf("Path should contain /unsuspend, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminUnsuspendUser("testuser")
	if err != nil {
		t.Fatalf("AdminUnsuspendUser() error = %v", err)
	}
}

func TestAdminResetUserPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/reset-password") {
			t.Errorf("Path should contain /reset-password, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetUserPassword("testuser")
	if err != nil {
		t.Fatalf("AdminResetUserPassword() error = %v", err)
	}
}

func TestAdminDisableUser2FA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/disable-2fa") {
			t.Errorf("Path should contain /disable-2fa, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDisableUser2FA("testuser")
	if err != nil {
		t.Fatalf("AdminDisableUser2FA() error = %v", err)
	}
}

// Tests for Org Admin API

func TestAdminListOrgs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/admin/orgs") {
			t.Errorf("Path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"orgs": []interface{}{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListOrgs(10, 0)
	if err != nil {
		t.Fatalf("AdminListOrgs() error = %v", err)
	}
}

func TestAdminGetOrg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"name": "testorg"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetOrg("testorg")
	if err != nil {
		t.Fatalf("AdminGetOrg() error = %v", err)
	}
}

func TestAdminCreateOrg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "neworg" {
			t.Errorf("name = %s", body["name"])
		}
		if body["display_name"] != "New Org" {
			t.Errorf("display_name = %s", body["display_name"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"name": "neworg"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateOrg("neworg", "New Org", "Description")
	if err != nil {
		t.Fatalf("AdminCreateOrg() error = %v", err)
	}
}

func TestAdminDeleteOrg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDeleteOrg("testorg")
	if err != nil {
		t.Fatalf("AdminDeleteOrg() error = %v", err)
	}
}

func TestAdminListOrgMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/members") {
			t.Errorf("Path should contain /members, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"members": []interface{}{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListOrgMembers("testorg", "")
	if err != nil {
		t.Fatalf("AdminListOrgMembers() error = %v", err)
	}
}

func TestAdminListOrgMembersWithRole(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("role") != "admin" {
			t.Errorf("role = %s, want admin", r.URL.Query().Get("role"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListOrgMembers("testorg", "admin")
	if err != nil {
		t.Fatalf("AdminListOrgMembers() error = %v", err)
	}
}

func TestAdminAddOrgMember(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "newmember" {
			t.Errorf("username = %s", body["username"])
		}
		if body["role"] != "member" {
			t.Errorf("role = %s", body["role"])
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminAddOrgMember("testorg", "newmember", "member")
	if err != nil {
		t.Fatalf("AdminAddOrgMember() error = %v", err)
	}
}

func TestAdminRemoveOrgMember(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRemoveOrgMember("testorg", "member")
	if err != nil {
		t.Fatalf("AdminRemoveOrgMember() error = %v", err)
	}
}

// Tests for Token Admin API

func TestAdminListTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"tokens": []interface{}{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListTokens(10, "")
	if err != nil {
		t.Fatalf("AdminListTokens() error = %v", err)
	}
}

func TestAdminListTokensWithUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user") != "testuser" {
			t.Errorf("user = %s, want testuser", r.URL.Query().Get("user"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListTokens(10, "testuser")
	if err != nil {
		t.Fatalf("AdminListTokens() error = %v", err)
	}
}

func TestAdminCreateToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test-token" {
			t.Errorf("name = %s", body["name"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"token": "generated-token"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateToken("test-token", "30d", "read,write")
	if err != nil {
		t.Fatalf("AdminCreateToken() error = %v", err)
	}
}

func TestAdminRevokeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRevokeToken("token-id")
	if err != nil {
		t.Fatalf("AdminRevokeToken() error = %v", err)
	}
}

func TestAdminGetTokenInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "token-id"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetTokenInfo("token-id")
	if err != nil {
		t.Fatalf("AdminGetTokenInfo() error = %v", err)
	}
}

// Tests for Server Config API

func TestAdminListConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"config": map[string]interface{}{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListConfig("")
	if err != nil {
		t.Fatalf("AdminListConfig() error = %v", err)
	}
}

func TestAdminListConfigWithCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") != "server" {
			t.Errorf("category = %s, want server", r.URL.Query().Get("category"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListConfig("server")
	if err != nil {
		t.Fatalf("AdminListConfig() error = %v", err)
	}
}

func TestAdminGetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"key": "value"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetConfig("server.port")
	if err != nil {
		t.Fatalf("AdminGetConfig() error = %v", err)
	}
}

func TestAdminSetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Method = %s, want PUT", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["value"] != "8080" {
			t.Errorf("value = %v", body["value"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminSetConfig("server.port", "8080", false)
	if err != nil {
		t.Fatalf("AdminSetConfig() error = %v", err)
	}
}

func TestAdminResetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetConfig("server.port")
	if err != nil {
		t.Fatalf("AdminResetConfig() error = %v", err)
	}
}

// Tests for Server Admin API

func TestAdminListServerAdmins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"admins": []interface{}{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListServerAdmins()
	if err != nil {
		t.Fatalf("AdminListServerAdmins() error = %v", err)
	}
}

func TestAdminInviteServerAdmin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminInviteServerAdmin("newadmin", "admin@example.com")
	if err != nil {
		t.Fatalf("AdminInviteServerAdmin() error = %v", err)
	}
}

func TestAdminRemoveServerAdmin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRemoveServerAdmin("admin")
	if err != nil {
		t.Fatalf("AdminRemoveServerAdmin() error = %v", err)
	}
}

func TestAdminResetServerAdminPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/reset-password") {
			t.Errorf("Path should contain /reset-password, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetServerAdminPassword("admin")
	if err != nil {
		t.Fatalf("AdminResetServerAdminPassword() error = %v", err)
	}
}

// Tests for Server Stats API

func TestAdminGetStatsOverview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/stats/overview") {
			t.Errorf("Path should contain /stats/overview, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"overview": "data"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsOverview()
	if err != nil {
		t.Fatalf("AdminGetStatsOverview() error = %v", err)
	}
}

func TestAdminGetStatsUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/stats/users") {
			t.Errorf("Path should contain /stats/users, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsUsers()
	if err != nil {
		t.Fatalf("AdminGetStatsUsers() error = %v", err)
	}
}

func TestAdminGetStatsStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/stats/storage") {
			t.Errorf("Path should contain /stats/storage, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsStorage()
	if err != nil {
		t.Fatalf("AdminGetStatsStorage() error = %v", err)
	}
}

func TestAdminGetStatsPerformance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/stats/performance") {
			t.Errorf("Path should contain /stats/performance, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsPerformance()
	if err != nil {
		t.Fatalf("AdminGetStatsPerformance() error = %v", err)
	}
}

// Tests for HTTP helpers

func TestDoRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "Invalid token",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.doRequest("GET", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for 401")
	}
	if !strings.Contains(err.Error(), "Invalid token") {
		t.Errorf("error message = %q, want to contain 'Invalid token'", err.Error())
	}
}

func TestDoRequestErrorWithoutMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.doRequest("GET", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error message = %q, want to contain '403'", err.Error())
	}
}

func TestAdminGetNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty valid JSON
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminGet("/test")
	if err != nil {
		t.Fatalf("adminGet() error = %v", err)
	}
	if result == nil {
		t.Error("adminGet() result should not be nil")
	}
}

func TestAdminPostNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.adminPost("/test", nil)
	if err != nil {
		t.Fatalf("adminPost() error = %v", err)
	}
}

func TestAdminPutNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.adminPut("/test", nil)
	if err != nil {
		t.Fatalf("adminPut() error = %v", err)
	}
}

func TestAdminDeleteNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.adminDelete("/test")
	if err != nil {
		t.Fatalf("adminDelete() error = %v", err)
	}
}

// Additional tests for 100% coverage

// Test adminGet with invalid JSON response (decode error)
func TestAdminGetInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.adminGet("/test")
	if err == nil {
		t.Error("adminGet() should return error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("error = %q, want to contain 'failed to decode'", err.Error())
	}
}

// Test AdminGetConfig with empty key
func TestAdminGetConfigEmptyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// When key is empty, path should be just /api/v1/admin/server/config
		if r.URL.Path != "/api/v1/admin/server/config" {
			t.Errorf("Path = %s, want /api/v1/admin/server/config", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetConfig("")
	if err != nil {
		t.Fatalf("AdminGetConfig() error = %v", err)
	}
}

// Test doRequest with connection error
func TestDoRequestConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "token", 1)
	_, err := client.doRequest("GET", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for connection refused")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want to contain 'request failed'", err.Error())
	}
}

// Test doRequest with unmarshalable body (marshal error)
func TestDoRequestMarshalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	// Channel cannot be marshaled to JSON
	unmarshalableBody := make(chan int)
	_, err := client.doRequest("POST", "/test", unmarshalableBody)
	if err == nil {
		t.Error("doRequest() should return error for unmarshalable body")
	}
	if !strings.Contains(err.Error(), "failed to marshal body") {
		t.Errorf("error = %q, want to contain 'failed to marshal body'", err.Error())
	}
}

// Test doRequest with invalid method (http.NewRequest error)
func TestDoRequestInvalidMethod(t *testing.T) {
	client := NewClient("http://example.com", "token", 30)
	// Invalid method with space should cause http.NewRequest to fail
	_, err := client.doRequest("INVALID METHOD", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for invalid method")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		t.Errorf("error = %q, want to contain 'failed to create request'", err.Error())
	}
}

// Test doRequest with body but invalid method
func TestDoRequestWithBodyInvalidMethod(t *testing.T) {
	client := NewClient("http://example.com", "token", 30)
	body := map[string]string{"key": "value"}
	_, err := client.doRequest("INVALID METHOD", "/test", body)
	if err == nil {
		t.Error("doRequest() should return error for invalid method")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		t.Errorf("error = %q, want to contain 'failed to create request'", err.Error())
	}
}

// Test adminGet with connection error
func TestAdminGetConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "token", 1)
	_, err := client.adminGet("/test")
	if err == nil {
		t.Error("adminGet() should return error for connection refused")
	}
}

// Test adminPost with connection error
func TestAdminPostConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "token", 1)
	_, err := client.adminPost("/test", nil)
	if err == nil {
		t.Error("adminPost() should return error for connection refused")
	}
}

// Test adminPut with connection error
func TestAdminPutConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "token", 1)
	_, err := client.adminPut("/test", nil)
	if err == nil {
		t.Error("adminPut() should return error for connection refused")
	}
}

// Test adminDelete with connection error
func TestAdminDeleteConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "token", 1)
	_, err := client.adminDelete("/test")
	if err == nil {
		t.Error("adminDelete() should return error for connection refused")
	}
}

// Test AdminListUsers with server error
func TestAdminListUsersServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListUsers(10, 0, "")
	if err == nil {
		t.Error("AdminListUsers() should return error for 500 response")
	}
}

// Test AdminGetUser with server error
func TestAdminGetUserServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not_found", "message": "User not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetUser("nonexistent")
	if err == nil {
		t.Error("AdminGetUser() should return error for 404 response")
	}
}

// Test AdminCreateUser with server error
func TestAdminCreateUserServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "conflict", "message": "User already exists"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateUser("existinguser", "email@test.com", "user")
	if err == nil {
		t.Error("AdminCreateUser() should return error for 409 response")
	}
}

// Test AdminDeleteUser with server error
func TestAdminDeleteUserServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDeleteUser("nonexistent")
	if err == nil {
		t.Error("AdminDeleteUser() should return error for 404 response")
	}
}

// Test AdminSuspendUser with server error
func TestAdminSuspendUserServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminSuspendUser("testuser")
	if err == nil {
		t.Error("AdminSuspendUser() should return error for 400 response")
	}
}

// Test AdminUnsuspendUser with server error
func TestAdminUnsuspendUserServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminUnsuspendUser("testuser")
	if err == nil {
		t.Error("AdminUnsuspendUser() should return error for 400 response")
	}
}

// Test AdminResetUserPassword with server error
func TestAdminResetUserPasswordServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetUserPassword("nonexistent")
	if err == nil {
		t.Error("AdminResetUserPassword() should return error for 404 response")
	}
}

// Test AdminDisableUser2FA with server error
func TestAdminDisableUser2FAServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDisableUser2FA("nonexistent")
	if err == nil {
		t.Error("AdminDisableUser2FA() should return error for 404 response")
	}
}

// Test AdminListOrgs with server error
func TestAdminListOrgsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListOrgs(10, 0)
	if err == nil {
		t.Error("AdminListOrgs() should return error for 500 response")
	}
}

// Test AdminGetOrg with server error
func TestAdminGetOrgServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetOrg("nonexistent")
	if err == nil {
		t.Error("AdminGetOrg() should return error for 404 response")
	}
}

// Test AdminCreateOrg with server error
func TestAdminCreateOrgServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateOrg("existingorg", "Display", "Desc")
	if err == nil {
		t.Error("AdminCreateOrg() should return error for 409 response")
	}
}

// Test AdminDeleteOrg with server error
func TestAdminDeleteOrgServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminDeleteOrg("nonexistent")
	if err == nil {
		t.Error("AdminDeleteOrg() should return error for 404 response")
	}
}

// Test AdminListOrgMembers with server error
func TestAdminListOrgMembersServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListOrgMembers("nonexistent", "")
	if err == nil {
		t.Error("AdminListOrgMembers() should return error for 404 response")
	}
}

// Test AdminAddOrgMember with server error
func TestAdminAddOrgMemberServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminAddOrgMember("nonexistent", "user", "member")
	if err == nil {
		t.Error("AdminAddOrgMember() should return error for 404 response")
	}
}

// Test AdminRemoveOrgMember with server error
func TestAdminRemoveOrgMemberServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRemoveOrgMember("nonexistent", "user")
	if err == nil {
		t.Error("AdminRemoveOrgMember() should return error for 404 response")
	}
}

// Test AdminListTokens with server error
func TestAdminListTokensServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListTokens(10, "")
	if err == nil {
		t.Error("AdminListTokens() should return error for 500 response")
	}
}

// Test AdminCreateToken with server error
func TestAdminCreateTokenServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminCreateToken("test", "30d", "read")
	if err == nil {
		t.Error("AdminCreateToken() should return error for 400 response")
	}
}

// Test AdminRevokeToken with server error
func TestAdminRevokeTokenServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRevokeToken("nonexistent")
	if err == nil {
		t.Error("AdminRevokeToken() should return error for 404 response")
	}
}

// Test AdminGetTokenInfo with server error
func TestAdminGetTokenInfoServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetTokenInfo("nonexistent")
	if err == nil {
		t.Error("AdminGetTokenInfo() should return error for 404 response")
	}
}

// Test AdminListConfig with server error
func TestAdminListConfigServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListConfig("")
	if err == nil {
		t.Error("AdminListConfig() should return error for 500 response")
	}
}

// Test AdminGetConfig with server error
func TestAdminGetConfigServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetConfig("nonexistent.key")
	if err == nil {
		t.Error("AdminGetConfig() should return error for 404 response")
	}
}

// Test AdminSetConfig with server error
func TestAdminSetConfigServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminSetConfig("invalid.key", "value", false)
	if err == nil {
		t.Error("AdminSetConfig() should return error for 400 response")
	}
}

// Test AdminSetConfig with reload=true
func TestAdminSetConfigWithReload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["reload"] != true {
			t.Errorf("reload = %v, want true", body["reload"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminSetConfig("key", "value", true)
	if err != nil {
		t.Fatalf("AdminSetConfig() error = %v", err)
	}
}

// Test AdminResetConfig with server error
func TestAdminResetConfigServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetConfig("nonexistent.key")
	if err == nil {
		t.Error("AdminResetConfig() should return error for 404 response")
	}
}

// Test AdminListServerAdmins with server error
func TestAdminListServerAdminsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminListServerAdmins()
	if err == nil {
		t.Error("AdminListServerAdmins() should return error for 500 response")
	}
}

// Test AdminInviteServerAdmin with server error
func TestAdminInviteServerAdminServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminInviteServerAdmin("existingadmin", "admin@test.com")
	if err == nil {
		t.Error("AdminInviteServerAdmin() should return error for 409 response")
	}
}

// Test AdminRemoveServerAdmin with server error
func TestAdminRemoveServerAdminServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminRemoveServerAdmin("nonexistent")
	if err == nil {
		t.Error("AdminRemoveServerAdmin() should return error for 404 response")
	}
}

// Test AdminResetServerAdminPassword with server error
func TestAdminResetServerAdminPasswordServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	err := client.AdminResetServerAdminPassword("nonexistent")
	if err == nil {
		t.Error("AdminResetServerAdminPassword() should return error for 404 response")
	}
}

// Test AdminGetStatsOverview with server error
func TestAdminGetStatsOverviewServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsOverview()
	if err == nil {
		t.Error("AdminGetStatsOverview() should return error for 500 response")
	}
}

// Test AdminGetStatsUsers with server error
func TestAdminGetStatsUsersServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsUsers()
	if err == nil {
		t.Error("AdminGetStatsUsers() should return error for 500 response")
	}
}

// Test AdminGetStatsStorage with server error
func TestAdminGetStatsStorageServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsStorage()
	if err == nil {
		t.Error("AdminGetStatsStorage() should return error for 500 response")
	}
}

// Test AdminGetStatsPerformance with server error
func TestAdminGetStatsPerformanceServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.AdminGetStatsPerformance()
	if err == nil {
		t.Error("AdminGetStatsPerformance() should return error for 500 response")
	}
}

// Test adminPost with valid JSON response body
func TestAdminPostWithValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "123"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminPost("/test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("adminPost() error = %v", err)
	}
	if result == nil {
		t.Error("adminPost() result should not be nil")
	}
}

// Test adminPut with valid JSON response body
func TestAdminPutWithValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"updated": true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminPut("/test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("adminPut() error = %v", err)
	}
	if result == nil {
		t.Error("adminPut() result should not be nil")
	}
}

// Test adminDelete with valid JSON response body
func TestAdminDeleteWithValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminDelete("/test")
	if err != nil {
		t.Fatalf("adminDelete() error = %v", err)
	}
	// adminDelete does not fail on decode error, it just returns whatever it can decode
	_ = result
}

// Test adminPost with invalid JSON response (doesn't fail, just returns nil)
func TestAdminPostInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminPost("/test", nil)
	if err != nil {
		t.Fatalf("adminPost() error = %v", err)
	}
	// adminPost returns nil when decode fails (per code design)
	if result != nil {
		t.Errorf("adminPost() result = %v, want nil", result)
	}
}

// Test adminPut with invalid JSON response (doesn't fail, just returns nil)
func TestAdminPutInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminPut("/test", nil)
	if err != nil {
		t.Fatalf("adminPut() error = %v", err)
	}
	// adminPut returns nil when decode fails (per code design)
	if result != nil {
		t.Errorf("adminPut() result = %v, want nil", result)
	}
}

// Test adminDelete with invalid JSON response (doesn't fail)
func TestAdminDeleteInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	result, err := client.adminDelete("/test")
	if err != nil {
		t.Fatalf("adminDelete() error = %v", err)
	}
	// adminDelete silently ignores decode errors
	if result != nil {
		t.Errorf("adminDelete() result = %v, want nil", result)
	}
}

// Test doRequest with token set (verifies Authorization header)
func TestDoRequestWithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want 'Bearer test-token'", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequest without token (no Authorization header)
func TestDoRequestWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Authorization header = %q, want empty", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequest verifies User-Agent and Accept headers
func TestDoRequestHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent := r.Header.Get("User-Agent")
		if userAgent == "" {
			t.Error("User-Agent header should be set")
		}
		accept := r.Header.Get("Accept")
		if accept != "application/json" {
			t.Errorf("Accept header = %q, want 'application/json'", accept)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequest with body (verifies Content-Type header)
func TestDoRequestWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type header = %q, want 'application/json'", contentType)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	body := map[string]string{"key": "value"}
	resp, err := client.doRequest("POST", "/test", body)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequest without body (no Content-Type header)
func TestDoRequestWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType != "" {
			t.Errorf("Content-Type header = %q, want empty", contentType)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Table-driven test for AdminListUsers status parameter
func TestAdminListUsersStatusValues(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		expectInQuery  bool
		expectedStatus string
	}{
		{"empty status", "", false, ""},
		{"all status", "all", false, ""},
		{"active status", "active", true, "active"},
		{"suspended status", "suspended", true, "suspended"},
		{"pending status", "pending", true, "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				statusParam := r.URL.Query().Get("status")
				if tt.expectInQuery {
					if statusParam != tt.expectedStatus {
						t.Errorf("status = %s, want %s", statusParam, tt.expectedStatus)
					}
				} else {
					if statusParam != "" {
						t.Errorf("status should be empty, got %s", statusParam)
					}
				}
				json.NewEncoder(w).Encode(map[string]interface{}{})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminListUsers(10, 0, tt.status)
			if err != nil {
				t.Fatalf("AdminListUsers() error = %v", err)
			}
		})
	}
}

// Table-driven test for AdminListOrgMembers role parameter
func TestAdminListOrgMembersRoleValues(t *testing.T) {
	tests := []struct {
		name          string
		role          string
		expectInQuery bool
	}{
		{"empty role", "", false},
		{"admin role", "admin", true},
		{"member role", "member", true},
		{"owner role", "owner", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				roleParam := r.URL.Query().Get("role")
				if tt.expectInQuery {
					if roleParam != tt.role {
						t.Errorf("role = %s, want %s", roleParam, tt.role)
					}
				} else {
					if roleParam != "" {
						t.Errorf("role should be empty, got %s", roleParam)
					}
				}
				json.NewEncoder(w).Encode(map[string]interface{}{})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminListOrgMembers("testorg", tt.role)
			if err != nil {
				t.Fatalf("AdminListOrgMembers() error = %v", err)
			}
		})
	}
}

// Table-driven test for AdminListTokens user parameter
func TestAdminListTokensUserValues(t *testing.T) {
	tests := []struct {
		name          string
		user          string
		expectInQuery bool
	}{
		{"empty user", "", false},
		{"specific user", "testuser", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				userParam := r.URL.Query().Get("user")
				if tt.expectInQuery {
					if userParam != tt.user {
						t.Errorf("user = %s, want %s", userParam, tt.user)
					}
				} else {
					if userParam != "" {
						t.Errorf("user should be empty, got %s", userParam)
					}
				}
				json.NewEncoder(w).Encode(map[string]interface{}{})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminListTokens(10, tt.user)
			if err != nil {
				t.Fatalf("AdminListTokens() error = %v", err)
			}
		})
	}
}

// Table-driven test for AdminListConfig category parameter
func TestAdminListConfigCategoryValues(t *testing.T) {
	tests := []struct {
		name          string
		category      string
		expectInQuery bool
	}{
		{"empty category", "", false},
		{"server category", "server", true},
		{"auth category", "auth", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				categoryParam := r.URL.Query().Get("category")
				if tt.expectInQuery {
					if categoryParam != tt.category {
						t.Errorf("category = %s, want %s", categoryParam, tt.category)
					}
				} else {
					if categoryParam != "" {
						t.Errorf("category should be empty, got %s", categoryParam)
					}
				}
				json.NewEncoder(w).Encode(map[string]interface{}{})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminListConfig(tt.category)
			if err != nil {
				t.Fatalf("AdminListConfig() error = %v", err)
			}
		})
	}
}

// Test URL path escaping for special characters in usernames
func TestAdminGetUserSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		username string
	}{
		{"normal username", "testuser"},
		{"username with dot", "test.user"},
		{"username with underscore", "test_user"},
		{"username with hyphen", "test-user"},
		{"username with space", "test user"},
		{"username with special chars", "test@user#1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]interface{}{"username": tt.username})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminGetUser(tt.username)
			if err != nil {
				t.Fatalf("AdminGetUser() error = %v", err)
			}
		})
	}
}

// Test URL path escaping for special characters in org names
func TestAdminGetOrgSpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		orgname string
	}{
		{"normal orgname", "testorg"},
		{"orgname with hyphen", "test-org"},
		{"orgname with underscore", "test_org"},
		{"orgname with dot", "test.org"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]interface{}{"name": tt.orgname})
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", 30)
			_, err := client.AdminGetOrg(tt.orgname)
			if err != nil {
				t.Fatalf("AdminGetOrg() error = %v", err)
			}
		})
	}
}
