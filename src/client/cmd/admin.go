// Package cmd implements CLI commands for the search client
// Per AI.md PART 36: --admin commands for server administration
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	// Admin command flags
	adminLimit  int
	adminOffset int
	adminStatus string
	adminRole   string
	adminEmail  string
	adminForce  bool
	adminFormat string
	// Config command flags
	configCategory string
	configNoReload bool
	// Token command flags
	tokenExpires string
	tokenScopes  string
	tokenUser    string
	// Org command flags
	orgDisplayName  string
	orgDescription  string
)

// adminCmd is the root admin command
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin CLI - manage users, organizations, and API tokens",
	Long: `Admin CLI - manage users, organizations, and API tokens.

AUTHENTICATION REQUIRED:
  Admin token must be set and valid. Use one of:
  1. Environment variable: SEARCH_TOKEN=adm_xxx...
  2. Flag: --token adm_xxx...

  Token must have admin scope (prefix: adm_). User tokens (usr_) will be rejected.`,
}

// ====== USER COMMANDS ======

var adminUserCmd = &cobra.Command{
	Use:   "user",
	Short: "User management commands",
	Long:  `User management commands: list, get, create, delete, suspend, unsuspend, reset-password, disable-2fa`,
}

var adminUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		users, err := apiClient.AdminListUsers(adminLimit, adminOffset, adminStatus)
		if err != nil {
			return err
		}
		return outputAdminResult(users)
	},
}

var adminUserGetCmd = &cobra.Command{
	Use:   "get <username>",
	Short: "Get user details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		user, err := apiClient.AdminGetUser(args[0])
		if err != nil {
			return err
		}
		return outputAdminResult(user)
	},
}

var adminUserCreateCmd = &cobra.Command{
	Use:   "create <username>",
	Short: "Create new user (sends invite email)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if adminEmail == "" {
			return fmt.Errorf("--email is required")
		}
		if err := initClient(); err != nil {
			return err
		}
		result, err := apiClient.AdminCreateUser(args[0], adminEmail, adminRole)
		if err != nil {
			return err
		}
		return outputAdminResult(result)
	},
}

var adminUserDeleteCmd = &cobra.Command{
	Use:   "delete <username>",
	Short: "Delete user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to delete user %q? Use --force to confirm.\n", args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminDeleteUser(args[0])
	},
}

var adminUserSuspendCmd = &cobra.Command{
	Use:   "suspend <username>",
	Short: "Suspend user account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminSuspendUser(args[0])
	},
}

var adminUserUnsuspendCmd = &cobra.Command{
	Use:   "unsuspend <username>",
	Short: "Unsuspend user account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminUnsuspendUser(args[0])
	},
}

var adminUserResetPasswordCmd = &cobra.Command{
	Use:   "reset-password <username>",
	Short: "Send password reset email to user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminResetUserPassword(args[0])
	},
}

var adminUserDisable2FACmd = &cobra.Command{
	Use:   "disable-2fa <username>",
	Short: "Disable two-factor authentication for user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminDisableUser2FA(args[0])
	},
}

// ====== ORG COMMANDS ======

var adminOrgCmd = &cobra.Command{
	Use:   "org",
	Short: "Organization management commands",
	Long:  `Organization management commands: list, get, create, delete, members, add-member, remove-member`,
}

var adminOrgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all organizations",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		orgs, err := apiClient.AdminListOrgs(adminLimit, adminOffset)
		if err != nil {
			return err
		}
		return outputAdminResult(orgs)
	},
}

var adminOrgGetCmd = &cobra.Command{
	Use:   "get <orgname>",
	Short: "Get organization details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		org, err := apiClient.AdminGetOrg(args[0])
		if err != nil {
			return err
		}
		return outputAdminResult(org)
	},
}

var adminOrgCreateCmd = &cobra.Command{
	Use:   "create <orgname>",
	Short: "Create new organization",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		result, err := apiClient.AdminCreateOrg(args[0], orgDisplayName, orgDescription)
		if err != nil {
			return err
		}
		return outputAdminResult(result)
	},
}

var adminOrgDeleteCmd = &cobra.Command{
	Use:   "delete <orgname>",
	Short: "Delete organization",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to delete organization %q? Use --force to confirm.\n", args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminDeleteOrg(args[0])
	},
}

var adminOrgMembersCmd = &cobra.Command{
	Use:   "members <orgname>",
	Short: "List organization members",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		members, err := apiClient.AdminListOrgMembers(args[0], adminRole)
		if err != nil {
			return err
		}
		return outputAdminResult(members)
	},
}

var adminOrgAddMemberCmd = &cobra.Command{
	Use:   "add-member <orgname> <username>",
	Short: "Add user to organization",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		role := adminRole
		if role == "" {
			role = "member"
		}
		return apiClient.AdminAddOrgMember(args[0], args[1], role)
	},
}

var adminOrgRemoveMemberCmd = &cobra.Command{
	Use:   "remove-member <orgname> <username>",
	Short: "Remove user from organization",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to remove %q from %q? Use --force to confirm.\n", args[1], args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminRemoveOrgMember(args[0], args[1])
	},
}

// ====== TOKEN COMMANDS ======

var adminTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "API token management commands",
	Long:  `API token management commands: list, create, revoke, info`,
}

var adminTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		tokens, err := apiClient.AdminListTokens(adminLimit, tokenUser)
		if err != nil {
			return err
		}
		return outputAdminResult(tokens)
	},
}

var adminTokenCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create new API token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		result, err := apiClient.AdminCreateToken(args[0], tokenExpires, tokenScopes)
		if err != nil {
			return err
		}
		return outputAdminResult(result)
	},
}

var adminTokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token_id>",
	Short: "Revoke token immediately",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to revoke token %q? Use --force to confirm.\n", args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminRevokeToken(args[0])
	},
}

var adminTokenInfoCmd = &cobra.Command{
	Use:   "info <token_id>",
	Short: "Get token details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		info, err := apiClient.AdminGetTokenInfo(args[0])
		if err != nil {
			return err
		}
		return outputAdminResult(info)
	},
}

// ====== SERVER COMMANDS ======

var adminServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Server admin CLI - configuration and management",
	Long: `Server admin CLI - server configuration and management.

AUTHENTICATION REQUIRED:
  Server admin token must be set and valid. Use one of:
  1. Environment variable: SEARCH_TOKEN=adm_xxx...
  2. Flag: --token adm_xxx...`,
}

// Server Config commands
var adminServerConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Server configuration commands",
	Long:  `Server configuration commands: list, get, set, reset`,
}

var adminServerConfigListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		config, err := apiClient.AdminListConfig(configCategory)
		if err != nil {
			return err
		}
		return outputAdminResult(config)
	},
}

var adminServerConfigGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get configuration value",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		key := ""
		if len(args) > 0 {
			key = args[0]
		}
		value, err := apiClient.AdminGetConfig(key)
		if err != nil {
			return err
		}
		return outputAdminResult(value)
	},
}

var adminServerConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminSetConfig(args[0], args[1], !configNoReload)
	},
}

var adminServerConfigResetCmd = &cobra.Command{
	Use:   "reset <key>",
	Short: "Reset configuration to default value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to reset %q to default? Use --force to confirm.\n", args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminResetConfig(args[0])
	},
}

// Server Admin commands
var adminServerAdminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Server admin management commands",
	Long:  `Server admin management commands: list, invite, remove, reset-password`,
}

var adminServerAdminListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all server admins",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		admins, err := apiClient.AdminListServerAdmins()
		if err != nil {
			return err
		}
		return outputAdminResult(admins)
	},
}

var adminServerAdminInviteCmd = &cobra.Command{
	Use:   "invite <username>",
	Short: "Invite new server admin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if adminEmail == "" {
			return fmt.Errorf("--email is required")
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminInviteServerAdmin(args[0], adminEmail)
	},
}

var adminServerAdminRemoveCmd = &cobra.Command{
	Use:   "remove <username>",
	Short: "Remove server admin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !adminForce {
			fmt.Printf("Are you sure you want to remove admin %q? Use --force to confirm.\n", args[0])
			return nil
		}
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminRemoveServerAdmin(args[0])
	},
}

var adminServerAdminResetPasswordCmd = &cobra.Command{
	Use:   "reset-password <username>",
	Short: "Send password reset to server admin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		return apiClient.AdminResetServerAdminPassword(args[0])
	},
}

// Server Stats commands
var adminServerStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Server statistics commands",
	Long:  `Server statistics commands: overview, users, storage, performance`,
}

var adminServerStatsOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "General server statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		stats, err := apiClient.AdminGetStatsOverview()
		if err != nil {
			return err
		}
		return outputAdminResult(stats)
	},
}

var adminServerStatsUsersCmd = &cobra.Command{
	Use:   "users",
	Short: "User statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		stats, err := apiClient.AdminGetStatsUsers()
		if err != nil {
			return err
		}
		return outputAdminResult(stats)
	},
}

var adminServerStatsStorageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Storage usage statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		stats, err := apiClient.AdminGetStatsStorage()
		if err != nil {
			return err
		}
		return outputAdminResult(stats)
	},
}

var adminServerStatsPerformanceCmd = &cobra.Command{
	Use:   "performance",
	Short: "Performance metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(); err != nil {
			return err
		}
		stats, err := apiClient.AdminGetStatsPerformance()
		if err != nil {
			return err
		}
		return outputAdminResult(stats)
	},
}

func init() {
	// Add admin command to root
	rootCmd.AddCommand(adminCmd)

	// Global admin flags
	adminCmd.PersistentFlags().StringVar(&adminFormat, "format", "table", "Output format: table, json, yaml")
	adminCmd.PersistentFlags().BoolVar(&adminForce, "force", false, "Skip confirmation prompts")

	// User commands
	adminCmd.AddCommand(adminUserCmd)
	adminUserCmd.AddCommand(adminUserListCmd)
	adminUserCmd.AddCommand(adminUserGetCmd)
	adminUserCmd.AddCommand(adminUserCreateCmd)
	adminUserCmd.AddCommand(adminUserDeleteCmd)
	adminUserCmd.AddCommand(adminUserSuspendCmd)
	adminUserCmd.AddCommand(adminUserUnsuspendCmd)
	adminUserCmd.AddCommand(adminUserResetPasswordCmd)
	adminUserCmd.AddCommand(adminUserDisable2FACmd)

	// User list flags
	adminUserListCmd.Flags().IntVar(&adminLimit, "limit", 100, "Limit results")
	adminUserListCmd.Flags().IntVar(&adminOffset, "offset", 0, "Offset for pagination")
	adminUserListCmd.Flags().StringVar(&adminStatus, "status", "all", "Filter by status: active, suspended, all")

	// User create flags
	adminUserCreateCmd.Flags().StringVar(&adminEmail, "email", "", "User's email address (required)")
	adminUserCreateCmd.Flags().StringVar(&adminRole, "role", "user", "User role: user, admin")

	// Org commands
	adminCmd.AddCommand(adminOrgCmd)
	adminOrgCmd.AddCommand(adminOrgListCmd)
	adminOrgCmd.AddCommand(adminOrgGetCmd)
	adminOrgCmd.AddCommand(adminOrgCreateCmd)
	adminOrgCmd.AddCommand(adminOrgDeleteCmd)
	adminOrgCmd.AddCommand(adminOrgMembersCmd)
	adminOrgCmd.AddCommand(adminOrgAddMemberCmd)
	adminOrgCmd.AddCommand(adminOrgRemoveMemberCmd)

	// Org list flags
	adminOrgListCmd.Flags().IntVar(&adminLimit, "limit", 100, "Limit results")
	adminOrgListCmd.Flags().IntVar(&adminOffset, "offset", 0, "Offset for pagination")

	// Org create flags
	adminOrgCreateCmd.Flags().StringVar(&orgDisplayName, "display-name", "", "Display name")
	adminOrgCreateCmd.Flags().StringVar(&orgDescription, "description", "", "Organization description")

	// Org members flags
	adminOrgMembersCmd.Flags().StringVar(&adminRole, "role", "", "Filter by role: owner, admin, member")
	adminOrgAddMemberCmd.Flags().StringVar(&adminRole, "role", "member", "Member role: owner, admin, member")

	// Token commands
	adminCmd.AddCommand(adminTokenCmd)
	adminTokenCmd.AddCommand(adminTokenListCmd)
	adminTokenCmd.AddCommand(adminTokenCreateCmd)
	adminTokenCmd.AddCommand(adminTokenRevokeCmd)
	adminTokenCmd.AddCommand(adminTokenInfoCmd)

	// Token list flags
	adminTokenListCmd.Flags().IntVar(&adminLimit, "limit", 100, "Limit results")
	adminTokenListCmd.Flags().StringVar(&tokenUser, "user", "", "Filter by username")

	// Token create flags
	adminTokenCreateCmd.Flags().StringVar(&tokenExpires, "expires", "90d", "Token expiration: 30d, 1y, never")
	adminTokenCreateCmd.Flags().StringVar(&tokenScopes, "scopes", "read,write", "Comma-separated scopes")

	// Server commands
	adminCmd.AddCommand(adminServerCmd)

	// Server config commands
	adminServerCmd.AddCommand(adminServerConfigCmd)
	adminServerConfigCmd.AddCommand(adminServerConfigListCmd)
	adminServerConfigCmd.AddCommand(adminServerConfigGetCmd)
	adminServerConfigCmd.AddCommand(adminServerConfigSetCmd)
	adminServerConfigCmd.AddCommand(adminServerConfigResetCmd)

	// Server config flags
	adminServerConfigListCmd.Flags().StringVar(&configCategory, "category", "", "Filter by category: server, auth, registration, email")
	adminServerConfigSetCmd.Flags().BoolVar(&configNoReload, "no-reload", false, "Don't reload config after change")

	// Server admin commands
	adminServerCmd.AddCommand(adminServerAdminCmd)
	adminServerAdminCmd.AddCommand(adminServerAdminListCmd)
	adminServerAdminCmd.AddCommand(adminServerAdminInviteCmd)
	adminServerAdminCmd.AddCommand(adminServerAdminRemoveCmd)
	adminServerAdminCmd.AddCommand(adminServerAdminResetPasswordCmd)

	// Server admin invite flags
	adminServerAdminInviteCmd.Flags().StringVar(&adminEmail, "email", "", "Admin's email address (required)")

	// Server stats commands
	adminServerCmd.AddCommand(adminServerStatsCmd)
	adminServerStatsCmd.AddCommand(adminServerStatsOverviewCmd)
	adminServerStatsCmd.AddCommand(adminServerStatsUsersCmd)
	adminServerStatsCmd.AddCommand(adminServerStatsStorageCmd)
	adminServerStatsCmd.AddCommand(adminServerStatsPerformanceCmd)
}

// outputAdminResult outputs the result in the requested format
func outputAdminResult(data interface{}) error {
	format := adminFormat
	if format == "" {
		format = getOutputFormat()
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		// For now, output as JSON with yaml label
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	default:
		// Table format - try to format nicely
		switch v := data.(type) {
		case []interface{}:
			if len(v) == 0 {
				fmt.Println("No results")
				return nil
			}
			// Output as JSON for now, proper table formatting would require type assertions
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		case map[string]interface{}:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for k, val := range v {
				fmt.Fprintf(w, "%s:\t%v\n", k, val)
			}
			return w.Flush()
		default:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		}
	}
}
