# TODO.AI.md — Outstanding items from bootstrap

## Pre-existing test failures (application bugs, not bootstrap gaps)

These test failures exist in the current codebase and must be resolved before CI/CD will pass:

### src/admin (admin_test.go)
- TestHandleLoginGet: status code = 302, want 200 (login page should return 200, not redirect when not logged in)
- TestHandleLoginAlreadyLoggedIn: status code = 302, want 303 (wrong redirect code when already logged in)
- TestHandleLoginPost: status code = 302, want 303 + session cookie not set on successful login
- TestHandleLoginPostInvalidCredentials: status code = 302, want 303 + location should contain error parameter
- TestRequireAuthNoSession: location = "/auth/login", should redirect to login (path prefix mismatch — missing /server prefix)

### src/config (config_test.go)
- TestDefaultConfig: Server.Port = 0, want 64580 (default port not being set to random 64xxx value on first init)
- TestServerConfigGetHTTPPort: GetHTTPPort() with zero = 80, want in range 64000-64999 (port fallback logic incorrect)

### src/database (database_test.go)
- TestClusterNodeStruct: Metadata['region'] = "", want 'us-east' (metadata field not being stored/retrieved)
- TestDatabaseMigratorMigrateAll: migration 18 failed — duplicate column name: manage_token_encrypted (migration adds column that already exists)
- TestClusterManagerTransferPrimary: no other nodes available (cluster transfer requires multi-node setup)

### src/instant (instant_test.go)
- TestEvalSimple/2**3: unsupported expression type: *ast.StarExpr (power operator ** not handled in math evaluator)
- TestMathHandlerHandleError: Handle() returned nil (error path returns nil instead of error result)

### src/search/engine (engines_test.go)
- TestBingTransportConfig: MaxIdleConns = 200, want 100; MaxIdleConnsPerHost = 20, want 10 (transport defaults changed)

## Remaining bootstrap items

- Verify Docker image build (docker/Dockerfile) completes successfully end-to-end (requires docker buildx with multi-platform support)
