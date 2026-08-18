# go.mod

**File:** `go.mod`

## Module

| Field | Value |
|-------|-------|
| Module path | `github.com/dexterhere04/AgentPlane` |
| Go version | `1.26.5` |

## Import Path

All internal imports use the module path prefix:

```go
import "github.com/dexterhere04/AgentPlane/internal/handlers"
import "github.com/dexterhere04/AgentPlane/internal/proxy"
import "github.com/dexterhere04/AgentPlane/internal/config"
```

## Direct Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/jackc/pgx/v5` | PostgreSQL driver and connection pool (`internal/db`, stores) |
| `github.com/google/uuid` | UUID generation for users and API keys |
| `github.com/joho/godotenv` | Loads `.env` at startup (`cmd/server`) |
| `github.com/aws/aws-sdk-go-v2/config` + `service/secretsmanager` | AWS Secrets Manager backend (`internal/secrets/aws.go`) |
| `github.com/Azure/azure-sdk-for-go/sdk/azidentity` + `security/keyvault/azsecrets` | Azure Key Vault backend (`internal/secrets/azure.go`) |
| `github.com/zricethezav/gitleaks/v8` | Secret detection in the `secrets` guardrail (`internal/guardrail/providers/secrets`) |

Most of `go.sum`'s remaining entries are transitive dependencies of these (notably the AWS SDK, Azure SDK, and gitleaks toolchains).

## Building

```bash
go build ./cmd/server/
go build ./cmd/keygeneration/
```

The `go` directive in `go.mod` specifies the expected language version. If your installed Go is older, change the directive or update Go.
