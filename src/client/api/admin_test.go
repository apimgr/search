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
