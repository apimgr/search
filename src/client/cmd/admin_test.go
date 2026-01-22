package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
)

// Tests for admin command structure

func TestAdminCmdUse(t *testing.T) {
	if adminCmd.Use != "admin" {
		t.Errorf("adminCmd.Use = %q, want 'admin'", adminCmd.Use)
	}
}

func TestAdminCmdShort(t *testing.T) {
	if adminCmd.Short == "" {
		t.Error("adminCmd.Short should not be empty")
	}
}

func TestAdminCmdLong(t *testing.T) {
	if adminCmd.Long == "" {
		t.Error("adminCmd.Long should not be empty")
	}
}

func TestAdminCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "admin" {
			found = true
			break
		}
	}

	if !found {
		t.Error("admin command should be registered with rootCmd")
	}
}

// Tests for admin subcommands

func TestAdminSubcommandsRegistered(t *testing.T) {
	subCommands := []string{"user", "org", "token", "server"}

	for _, subCmd := range subCommands {
		found := false
		for _, cmd := range adminCmd.Commands() {
			if cmd.Use == subCmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("admin subcommand %q should be registered", subCmd)
		}
	}
}

// Tests for outputAdminResult

func TestOutputAdminResultJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "json"
	data := map[string]interface{}{
		"key": "value",
		"num": 123,
	}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() error = %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("outputAdminResult() produced no output")
	}

	adminFormat = ""
}

func TestOutputAdminResultYAML(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "yaml"
	data := map[string]interface{}{"test": "data"}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() yaml error = %v", err)
	}

	adminFormat = ""
}

func TestOutputAdminResultTable(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "table"
	data := map[string]interface{}{
		"username": "testuser",
		"email":    "test@example.com",
	}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() table error = %v", err)
	}

	adminFormat = ""
}

func TestOutputAdminResultTableArray(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "table"
	data := []interface{}{
		map[string]interface{}{"name": "item1"},
		map[string]interface{}{"name": "item2"},
	}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() table array error = %v", err)
	}

	adminFormat = ""
}

func TestOutputAdminResultTableEmptyArray(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "table"
	data := []interface{}{}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() empty array error = %v", err)
	}

	output := buf.String()
	if output != "No results\n" {
		t.Errorf("outputAdminResult() empty array output = %q", output)
	}

	adminFormat = ""
}

func TestOutputAdminResultDefaultFormat(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = ""
	viper.Reset()
	viper.Set("output.format", "table")
	data := map[string]interface{}{"key": "value"}

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() default format error = %v", err)
	}
}

func TestOutputAdminResultOtherType(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	adminFormat = "table"
	data := "simple string"

	err := outputAdminResult(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("outputAdminResult() string type error = %v", err)
	}

	adminFormat = ""
}

// Tests for admin user commands

func TestAdminUserListCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []interface{}{},
			"total": 0,
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	adminLimit = 10
	adminOffset = 0
	adminStatus = ""

	err := adminUserListCmd.RunE(adminUserListCmd, []string{})

	if err != nil {
		t.Fatalf("adminUserListCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminUserGetCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"username": "testuser",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminUserGetCmd.RunE(adminUserGetCmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserGetCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminUserCreateCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"username": "newuser",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	adminEmail = "new@example.com"
	adminRole = "user"

	err := adminUserCreateCmd.RunE(adminUserCreateCmd, []string{"newuser"})

	if err != nil {
		t.Fatalf("adminUserCreateCmd.RunE() error = %v", err)
	}

	adminFormat = ""
	adminEmail = ""
	adminRole = ""
}

func TestAdminUserCreateCmdNoEmail(t *testing.T) {
	viper.Reset()
	server = "http://test"
	apiClient = nil
	adminEmail = ""

	err := adminUserCreateCmd.RunE(adminUserCreateCmd, []string{"newuser"})

	if err == nil {
		t.Error("adminUserCreateCmd.RunE() should error when email is not provided")
	}
}

func TestAdminUserDeleteCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	viper.Reset()
	adminForce = false

	err := adminUserDeleteCmd.RunE(adminUserDeleteCmd, []string{"testuser"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminUserDeleteCmd.RunE() without force error = %v", err)
	}
}

func TestAdminUserDeleteCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminUserDeleteCmd.RunE(adminUserDeleteCmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserDeleteCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

func TestAdminUserSuspendCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := adminUserSuspendCmd.RunE(adminUserSuspendCmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserSuspendCmd.RunE() error = %v", err)
	}
}

func TestAdminUserUnsuspendCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := adminUserUnsuspendCmd.RunE(adminUserUnsuspendCmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserUnsuspendCmd.RunE() error = %v", err)
	}
}

func TestAdminUserResetPasswordCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := adminUserResetPasswordCmd.RunE(adminUserResetPasswordCmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserResetPasswordCmd.RunE() error = %v", err)
	}
}

func TestAdminUserDisable2FACmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := adminUserDisable2FACmd.RunE(adminUserDisable2FACmd, []string{"testuser"})

	if err != nil {
		t.Fatalf("adminUserDisable2FACmd.RunE() error = %v", err)
	}
}

// Tests for admin org commands

func TestAdminOrgListCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"orgs": []interface{}{},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminOrgListCmd.RunE(adminOrgListCmd, []string{})

	if err != nil {
		t.Fatalf("adminOrgListCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminOrgGetCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "testorg",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminOrgGetCmd.RunE(adminOrgGetCmd, []string{"testorg"})

	if err != nil {
		t.Fatalf("adminOrgGetCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminOrgCreateCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "neworg",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	orgDisplayName = "New Org"
	orgDescription = "Description"

	err := adminOrgCreateCmd.RunE(adminOrgCreateCmd, []string{"neworg"})

	if err != nil {
		t.Fatalf("adminOrgCreateCmd.RunE() error = %v", err)
	}

	adminFormat = ""
	orgDisplayName = ""
	orgDescription = ""
}

func TestAdminOrgDeleteCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	adminForce = false

	err := adminOrgDeleteCmd.RunE(adminOrgDeleteCmd, []string{"testorg"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminOrgDeleteCmd.RunE() without force error = %v", err)
	}
}

func TestAdminOrgDeleteCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminOrgDeleteCmd.RunE(adminOrgDeleteCmd, []string{"testorg"})

	if err != nil {
		t.Fatalf("adminOrgDeleteCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

func TestAdminOrgMembersCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []interface{}{},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	adminRole = ""

	err := adminOrgMembersCmd.RunE(adminOrgMembersCmd, []string{"testorg"})

	if err != nil {
		t.Fatalf("adminOrgMembersCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminOrgAddMemberCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminRole = ""

	err := adminOrgAddMemberCmd.RunE(adminOrgAddMemberCmd, []string{"testorg", "newuser"})

	if err != nil {
		t.Fatalf("adminOrgAddMemberCmd.RunE() error = %v", err)
	}
}

func TestAdminOrgAddMemberCmdWithRole(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminRole = "admin"

	err := adminOrgAddMemberCmd.RunE(adminOrgAddMemberCmd, []string{"testorg", "newuser"})

	if err != nil {
		t.Fatalf("adminOrgAddMemberCmd.RunE() with role error = %v", err)
	}

	adminRole = ""
}

func TestAdminOrgRemoveMemberCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	adminForce = false

	err := adminOrgRemoveMemberCmd.RunE(adminOrgRemoveMemberCmd, []string{"testorg", "member"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminOrgRemoveMemberCmd.RunE() without force error = %v", err)
	}
}

func TestAdminOrgRemoveMemberCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminOrgRemoveMemberCmd.RunE(adminOrgRemoveMemberCmd, []string{"testorg", "member"})

	if err != nil {
		t.Fatalf("adminOrgRemoveMemberCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

// Tests for admin token commands

func TestAdminTokenListCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tokens": []interface{}{},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	tokenUser = ""

	err := adminTokenListCmd.RunE(adminTokenListCmd, []string{})

	if err != nil {
		t.Fatalf("adminTokenListCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminTokenCreateCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": "generated-token",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	tokenExpires = "30d"
	tokenScopes = "read,write"

	err := adminTokenCreateCmd.RunE(adminTokenCreateCmd, []string{"test-token"})

	if err != nil {
		t.Fatalf("adminTokenCreateCmd.RunE() error = %v", err)
	}

	adminFormat = ""
	tokenExpires = ""
	tokenScopes = ""
}

func TestAdminTokenRevokeCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	adminForce = false

	err := adminTokenRevokeCmd.RunE(adminTokenRevokeCmd, []string{"token-id"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminTokenRevokeCmd.RunE() without force error = %v", err)
	}
}

func TestAdminTokenRevokeCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminTokenRevokeCmd.RunE(adminTokenRevokeCmd, []string{"token-id"})

	if err != nil {
		t.Fatalf("adminTokenRevokeCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

func TestAdminTokenInfoCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "token-id",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminTokenInfoCmd.RunE(adminTokenInfoCmd, []string{"token-id"})

	if err != nil {
		t.Fatalf("adminTokenInfoCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

// Tests for admin server config commands

func TestAdminServerConfigListCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"
	configCategory = ""

	err := adminServerConfigListCmd.RunE(adminServerConfigListCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerConfigListCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerConfigGetCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key": "value",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerConfigGetCmd.RunE(adminServerConfigGetCmd, []string{"server.port"})

	if err != nil {
		t.Fatalf("adminServerConfigGetCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerConfigGetCmdNoKey(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerConfigGetCmd.RunE(adminServerConfigGetCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerConfigGetCmd.RunE() no key error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerConfigSetCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	configNoReload = false

	err := adminServerConfigSetCmd.RunE(adminServerConfigSetCmd, []string{"server.port", "8080"})

	if err != nil {
		t.Fatalf("adminServerConfigSetCmd.RunE() error = %v", err)
	}
}

func TestAdminServerConfigResetCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	adminForce = false

	err := adminServerConfigResetCmd.RunE(adminServerConfigResetCmd, []string{"server.port"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminServerConfigResetCmd.RunE() without force error = %v", err)
	}
}

func TestAdminServerConfigResetCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminServerConfigResetCmd.RunE(adminServerConfigResetCmd, []string{"server.port"})

	if err != nil {
		t.Fatalf("adminServerConfigResetCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

// Tests for admin server admin commands

func TestAdminServerAdminListCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"admins": []interface{}{},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerAdminListCmd.RunE(adminServerAdminListCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerAdminListCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerAdminInviteCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminEmail = "admin@example.com"

	err := adminServerAdminInviteCmd.RunE(adminServerAdminInviteCmd, []string{"newadmin"})

	if err != nil {
		t.Fatalf("adminServerAdminInviteCmd.RunE() error = %v", err)
	}

	adminEmail = ""
}

func TestAdminServerAdminInviteCmdNoEmail(t *testing.T) {
	viper.Reset()
	server = "http://test"
	apiClient = nil
	adminEmail = ""

	err := adminServerAdminInviteCmd.RunE(adminServerAdminInviteCmd, []string{"newadmin"})

	if err == nil {
		t.Error("adminServerAdminInviteCmd.RunE() should error when email not provided")
	}
}

func TestAdminServerAdminRemoveCmdNoForce(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	adminForce = false

	err := adminServerAdminRemoveCmd.RunE(adminServerAdminRemoveCmd, []string{"admin"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("adminServerAdminRemoveCmd.RunE() without force error = %v", err)
	}
}

func TestAdminServerAdminRemoveCmdWithForce(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminForce = true

	err := adminServerAdminRemoveCmd.RunE(adminServerAdminRemoveCmd, []string{"admin"})

	if err != nil {
		t.Fatalf("adminServerAdminRemoveCmd.RunE() with force error = %v", err)
	}

	adminForce = false
}

func TestAdminServerAdminResetPasswordCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := adminServerAdminResetPasswordCmd.RunE(adminServerAdminResetPasswordCmd, []string{"admin"})

	if err != nil {
		t.Fatalf("adminServerAdminResetPasswordCmd.RunE() error = %v", err)
	}
}

// Tests for admin server stats commands

func TestAdminServerStatsOverviewCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"overview": "data",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerStatsOverviewCmd.RunE(adminServerStatsOverviewCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerStatsOverviewCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerStatsUsersCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerStatsUsersCmd.RunE(adminServerStatsUsersCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerStatsUsersCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerStatsStorageCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerStatsStorageCmd.RunE(adminServerStatsStorageCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerStatsStorageCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

func TestAdminServerStatsPerformanceCmd(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	adminFormat = "json"

	err := adminServerStatsPerformanceCmd.RunE(adminServerStatsPerformanceCmd, []string{})

	if err != nil {
		t.Fatalf("adminServerStatsPerformanceCmd.RunE() error = %v", err)
	}

	adminFormat = ""
}

// Tests for admin flags

func TestAdminPersistentFlags(t *testing.T) {
	flags := []string{"format", "force"}

	for _, flag := range flags {
		if adminCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("admin persistent flag --%s should be registered", flag)
		}
	}
}
