# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Expose internal `/debug/*`, metrics, or Tor control routes in OpenAPI.
- Skip request validation, authorization, or consistent error envelopes.
- Drift route definitions, OpenAPI, GraphQL, and server behavior.

## CRITICAL - ALWAYS DO
- Follow the canonical versioned API route patterns and access rules.
- Document every public route with correct methods, schemas, and responses.
- Keep TLS and Let's Encrypt configuration secure and operator-controlled.

## Key rules
- The server owns API work; clients consume the server contract.
- Internal routes remain internal and must be excluded from served/generated OpenAPI.

For complete details, see AI.md PART 13, 14, 15.
