# LLM Proxy Auth

A secure, production-ready multi-provider AI Gateway with comprehensive authentication, supporting 7+ AI providers including AWS Bedrock, OpenAI, Anthropic, Azure, Google Vertex AI, IBM watsonx.ai, and Oracle Cloud AI.

Formerly known as Bedrock IAM Proxy.

## Features

### Core Features
- **Multi-Provider Support**: AWS Bedrock, OpenAI, Anthropic Claude, Azure OpenAI, Google Vertex AI, IBM watsonx.ai, Oracle Cloud AI
- **Dual-Mode Architecture**:
  - **Transparent Mode**: Authentication-only passthrough (preserves native APIs)
  - **Protocol Mode**: OpenAI-compatible API with request/response translation
- **Security-First Design**: Distroless container, non-root execution, comprehensive security scanning
- **OpenAI Compatibility**: Drop-in replacement for OpenAI SDK across all providers
- **Parameter Translation**: Comprehensive parameter mapping between provider formats
- **Region-Aware Routing**: Support for multi-region deployments (e.g., Bedrock US/EU)

### Authentication & Security
- **AWS Integration**: Native EKS IRSA support with fallback to EC2 instance profiles
- **Multiple Auth Methods**: API Key, Basic Auth, TOTP/2FA, Service Account, OAuth2
- **No Credentials Needed**: For AWS Bedrock using IAM roles (IRSA)
- **Secret Management**: Kubernetes Secrets, AWS Secrets Manager, HashiCorp Vault integration

### Operations & Observability
- **Observability**: Prometheus metrics per instance/provider/region, structured logging, health checks
- **Production-Ready**: Graceful shutdowns, proper error handling, comprehensive testing
- **Private VPC Support**: Designed for fully private EKS clusters with VPC endpoints
- **Configuration-Driven**: YAML-based instance configuration with environment variable expansion

## Architecture

### High-Level Architecture

```mermaid
flowchart TB
    subgraph Clients["Client Applications"]
        C1[OpenAI SDK Client]
        C2[Native API Client]
        C3[Custom Application]
    end

    subgraph Gateway["LLM Proxy Auth Gateway - Kubernetes Pod"]
        AUTH[User Authentication Router<br/>API Key / 2FA / OAuth2]

        subgraph Routers["Core Routing Layer"]
            MODE_ROUTER[Mode Router<br/>Transparent vs Protocol]
            MODEL_ROUTER[Model Router<br/>Model name → Provider mapping]
            PROVIDER_ROUTER[Provider Router<br/>Route to handler]
        end

        subgraph Handlers["Provider Handlers"]
            P1[Bedrock Handler]
            P2[OpenAI Handler]
            P3[Anthropic Handler]
            P4[Azure Handler]
            P5[Vertex Handler]
            P6[IBM Handler]
            P7[Oracle Handler]
        end

        CRED_ROUTER[Credential Router<br/>Strategy-based selection]
        VAULT_ROUTER[Vault Router<br/>Select vault backend]
    end

    subgraph CredSources["Credential Sources - Auto-Selected by Platform"]
        DETECTOR[Platform Detector<br/>EKS/AKS/GKE/OKE/Generic]

        subgraph WorkloadID["Workload Identity"]
            WI1[AWS IRSA]
            WI2[Azure Managed Identity]
            WI3[GCP Workload Identity]
            WI4[OCI Resource Principal]
        end

        subgraph Vaults["Vault Backends"]
            V1[HashiCorp Vault]
            V2[AWS Secrets Manager]
            V3[Azure Key Vault]
            V4[GCP Secret Manager]
        end

        K8S_SEC[Kubernetes Secrets]
    end

    subgraph AIProviders["AI Providers"]
        BEDROCK[AWS Bedrock]
        OPENAI[OpenAI API]
        ANTHROPIC[Anthropic API]
        AZURE[Azure OpenAI]
        VERTEX[Google Vertex AI]
        IBM_P[IBM watsonx.ai]
        ORACLE[Oracle Cloud AI]
    end

    C1 & C2 & C3 --> AUTH
    AUTH --> MODE_ROUTER

    MODE_ROUTER -->|Transparent or Protocol| MODEL_ROUTER
    MODEL_ROUTER -->|claude-3 → Bedrock<br/>gpt-4 → OpenAI<br/>gemini → Vertex| PROVIDER_ROUTER

    PROVIDER_ROUTER --> P1 & P2 & P3 & P4 & P5 & P6 & P7

    P1 & P2 & P3 & P4 & P5 & P6 & P7 --> CRED_ROUTER

    CRED_ROUTER --> DETECTOR
    DETECTOR -->|Check platform| CRED_ROUTER

    CRED_ROUTER -->|Strategy 1| WorkloadID
    CRED_ROUTER -->|Strategy 2| VAULT_ROUTER
    CRED_ROUTER -->|Strategy 3| K8S_SEC

    VAULT_ROUTER --> V1 & V2 & V3 & V4

    WI1 & V1 & V2 & K8S_SEC --> BEDROCK
    V1 & V2 & V3 & V4 & K8S_SEC --> OPENAI & ANTHROPIC
    WI2 & V1 & V3 & K8S_SEC --> AZURE
    WI3 & V1 & V4 & K8S_SEC --> VERTEX
    V1 & V2 & K8S_SEC --> IBM_P
    WI4 & V1 & K8S_SEC --> ORACLE

    style AUTH fill:#e1f5ff
    style Routers fill:#fff4e1
    style MODE_ROUTER fill:#ffe6f0
    style MODEL_ROUTER fill:#ffe6f0
    style PROVIDER_ROUTER fill:#ffe6f0
    style CRED_ROUTER fill:#f3e5f5
    style VAULT_ROUTER fill:#f3e5f5
    style DETECTOR fill:#fff3e0
    style WorkloadID fill:#e8f5e9
    style Vaults fill:#fff9e6
    style K8S_SEC fill:#ffebee
```

**The Four-Router Architecture:**

The gateway uses a **layered routing approach** with specialized routers:

| Router | Purpose | Decision Made |
|--------|---------|---------------|
| **1. Auth Router** | User authentication | Valid API key/2FA? Which user? |
| **2. Mode Router** | Route selection | Transparent mode or Protocol mode? |
| **3. Model Router** | Provider selection | `claude-3` → Bedrock, `gpt-4` → OpenAI, `gemini` → Vertex |
| **4. Provider Router** | Handler dispatch | Route to Bedrock/OpenAI/Anthropic/etc handler |
| **5. Credential Router** | Credential strategy | IRSA? Vault? K8s Secret? |
| **6. Vault Router** | Vault backend selection | HashiCorp? AWS SM? Azure KV? GCP SM? |

**Request Flow:**
```
Client Request
    ↓
[1] Auth Router: Validate user credentials
    ↓
[2] Mode Router: Transparent or Protocol mode?
    ↓
[3] Model Router: Which provider for this model?
    ↓
[4] Provider Router: Route to specific handler
    ↓
[5] Credential Router: Get provider credentials
    ↓
[6] Vault Router: (if needed) Select vault backend
    ↓
AI Provider
```

**Benefits:**
- ✅ **Separation of concerns** - Each router has one job
- ✅ **Easy to test** - Mock individual routers
- ✅ **Observable** - Log decisions at each routing point
- ✅ **Extensible** - Add new providers/vaults/modes without touching other routers
- ✅ **Clean architecture** - Reduces complexity from O(n×m) to O(n+m)

### Credential Acquisition Flow

When a provider handler needs credentials, it tries configured strategies in priority order:

```mermaid
sequenceDiagram
    participant Client
    participant Handler as Provider Handler<br/>(Bedrock/Azure/etc)
    participant Detector as Platform Detector
    participant Strategy as Credential Strategy
    participant WI as Workload Identity<br/>(IRSA/Managed Identity)
    participant Vault as Vault Backend<br/>(HashiCorp/AWS SM/Azure KV)
    participant K8s as Kubernetes Secret
    participant Provider as AI Provider

    Client->>Handler: Request to invoke model
    Handler->>Detector: What platform are we on?
    Detector-->>Handler: Running on EKS/AKS/GKE/etc

    rect rgb(232, 245, 233)
    Note over Handler,WI: Strategy 1: Try Workload Identity
    Handler->>Strategy: Try workload_identity
    Strategy->>WI: Get credentials
    alt Workload Identity Available
        WI-->>Strategy: ✅ Credentials (auto-rotated)
        Strategy-->>Handler: Success
        Handler->>Provider: Request with credentials
        Provider-->>Client: Response
    else Not Available
        WI-->>Strategy: ❌ Not available on this platform
        Note over Handler,Vault: Fallback to Strategy 2
    end
    end

    rect rgb(255, 243, 224)
    Note over Handler,Vault: Strategy 2: Try Vault Backend
    Handler->>Strategy: Try vault backend
    Strategy->>Vault: Get secret from HashiCorp/Cloud Vault
    alt Vault Accessible
        Vault-->>Strategy: ✅ Credentials from vault
        Strategy-->>Handler: Success
        Handler->>Provider: Request with credentials
        Provider-->>Client: Response
    else Vault Not Available
        Vault-->>Strategy: ❌ Vault not configured/accessible
        Note over Handler,K8s: Fallback to Strategy 3
    end
    end

    rect rgb(255, 235, 238)
    Note over Handler,K8s: Strategy 3: Kubernetes Secret (Last Resort)
    Handler->>Strategy: Try kubernetes_secret
    Strategy->>K8s: Read secret from K8s API
    alt Secret Exists
        K8s-->>Strategy: ✅ Static credentials
        Strategy-->>Handler: Success
        Handler->>Provider: Request with credentials
        Provider-->>Client: Response
    else No Secret
        K8s-->>Strategy: ❌ Secret not found
        Strategy-->>Handler: ❌ All strategies failed
        Handler-->>Client: 500 Error - Cannot authenticate
    end
    end
```

**Key Points:**
- ✅ **Tries strategies in order** until one succeeds
- ✅ **Auto-detects platform** capabilities at startup
- ✅ **Caches credentials** until expiry
- ✅ **Auto-refreshes** before expiration
- ✅ **Logs which strategy** is active for observability

**Example Configuration:**

```yaml
# configs/provider-instances.yaml
instances:
  bedrock_us1:
    type: bedrock
    region: us-east-1
    authentication:
      strategies:
        - type: workload_identity  # Strategy 1
          provider: aws
        - type: vault              # Strategy 2
          backend: hashicorp
          path: aws/sts/bedrock-role
        - type: vault              # Strategy 3
          backend: aws_secrets_manager
          secret_name: bedrock-api-key
        - type: kubernetes_secret  # Strategy 4 (fallback)
          secret_name: bedrock-credentials
```

**Runtime Behavior on Different Platforms:**

| Platform | Strategy Used | Why |
|----------|---------------|-----|
| **AWS EKS** | ✅ `workload_identity` (IRSA) | AWS IRSA available, best option |
| **Azure AKS** | ⚠️ `vault` (HashiCorp) | AWS IRSA not available, Vault configured |
| **Google GKE** | ⚠️ `vault` (HashiCorp) | AWS IRSA not available, Vault configured |
| **Dev Laptop (minikube)** | ⚠️ `kubernetes_secret` | No workload identity, no Vault, uses K8s secret |

**Startup Logs Example (on AWS EKS):**

```
2025-01-15 10:23:45 INFO  Platform detected: eks
2025-01-15 10:23:45 INFO  Features available: aws_workload_identity=true
2025-01-15 10:23:46 INFO  [bedrock_us1] Trying strategy: workload_identity
2025-01-15 10:23:46 INFO  [bedrock_us1] ✅ Successfully initialized with workload_identity
2025-01-15 10:23:46 INFO  [bedrock_us1] Credentials will auto-rotate every 60 minutes
```

**Startup Logs Example (on Azure AKS):**

```
2025-01-15 10:23:45 INFO  Platform detected: aks
2025-01-15 10:23:45 INFO  Features available: azure_workload_identity=true
2025-01-15 10:23:46 INFO  [bedrock_us1] Trying strategy: workload_identity (aws)
2025-01-15 10:23:46 WARN  [bedrock_us1] ⚠️  AWS workload identity not available on aks
2025-01-15 10:23:46 INFO  [bedrock_us1] Trying strategy: vault (hashicorp)
2025-01-15 10:23:47 INFO  [bedrock_us1] ✅ Successfully initialized with vault
2025-01-15 10:23:47 INFO  [bedrock_us1] Credentials TTL: 3600 seconds
```

### Dual-Mode Architecture

The gateway supports two operational modes:

**Transparent Mode** (`/transparent/{provider}`):
- Authentication-only passthrough
- Preserves native provider APIs
- Minimal transformation
- Best for provider-specific features

**Protocol Mode** (`/{protocol}/{instance}`):
- OpenAI-compatible API
- Request/response translation
- Multi-provider abstraction
- Best for standardization

```mermaid
sequenceDiagram
    participant Client
    participant Gateway
    participant Provider

    rect rgb(255, 244, 225)
    Note over Client,Provider: Transparent Mode
    Client->>Gateway: POST /transparent/bedrock/model/claude-3/invoke
    Note right of Client: Native Bedrock format
    Gateway->>Gateway: Authenticate API Key
    Gateway->>Gateway: Add AWS SigV4 signature
    Gateway->>Provider: Forward unchanged request
    Provider-->>Gateway: Native response
    Gateway-->>Client: Return unchanged response
    end

    rect rgb(255, 225, 245)
    Note over Client,Provider: Protocol Mode - OpenAI compatible
    Client->>Gateway: POST /openai/bedrock_us1/chat/completions
    Note right of Client: OpenAI format
    Gateway->>Gateway: Authenticate API Key
    Gateway->>Gateway: Translate OpenAI to Bedrock format
    Gateway->>Gateway: Add AWS SigV4 signature
    Gateway->>Provider: Send translated request
    Provider-->>Gateway: Native response
    Gateway->>Gateway: Translate Bedrock to OpenAI format
    Gateway-->>Client: Return OpenAI-formatted response
    end
```

## Quick Start

### Prerequisites

- Go 1.21+
- Docker with BuildKit
- AWS CLI configured
- kubectl access to EKS cluster

### Build

```bash
# Build the application
go build -v ./cmd/bedrock-proxy

# Run tests
go test ./...

# Build Docker image
docker build -f build/Dockerfile -t bedrock-proxy .
```

### Deploy

1. **Deploy Infrastructure**:
   ```bash
   cd deployments/terraform
   terraform init
   terraform plan
   terraform apply
   ```

2. **Deploy Application**:
   ```bash
   kubectl apply -f deployments/kubernetes/
   ```

3. **Verify Deployment**:
   ```bash
   kubectl get pods -n bedrock-system
   kubectl logs -f deployment/bedrock-proxy -n bedrock-system
   ```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `TLS_PORT` | HTTPS server port | `8443` |
| `TLS_CERT_FILE` | TLS certificate file path | - |
| `TLS_KEY_FILE` | TLS private key file path | - |
| `AWS_REGION` | AWS region | `us-east-1` |
| `GIN_MODE` | Gin mode (debug/release) | `release` |
| `LOG_LEVEL` | Logging level | `info` |
| `AWS_ROLE_ARN` | IAM role ARN (auto-set by IRSA) | - |
| `AWS_WEB_IDENTITY_TOKEN_FILE` | Token file path (auto-set by IRSA) | - |

### AWS Permissions

The proxy requires the following IAM permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream",
        "bedrock:ListFoundationModels",
        "bedrock:GetFoundationModel"
      ],
      "Resource": "*"
    }
  ]
}
```

## API Usage

### Health Endpoints

- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics

### Bedrock Proxy

- `POST /v1/bedrock/invoke-model` - Invoke Bedrock model
- `POST /bedrock/invoke-model` - Alternative endpoint

Example requests:
```bash
# HTTP request
curl -X POST http://localhost:8080/v1/bedrock/invoke-model \
  -H "Content-Type: application/json" \
  -d '{
    "modelId": "amazon.titan-text-express-v1",
    "contentType": "application/json",
    "accept": "application/json",
    "body": "{\"inputText\":\"Hello World\"}"
  }'

# HTTPS request (if TLS is configured)
curl -X POST https://localhost:8443/v1/bedrock/invoke-model \
  -H "Content-Type: application/json" \
  -d '{
    "modelId": "amazon.titan-text-express-v1",
    "contentType": "application/json",
    "accept": "application/json",
    "body": "{\"inputText\":\"Hello World\"}"
  }'
```

## Security & Authentication

### 🔐 Multi-Layer Security Architecture

```mermaid
graph LR
    subgraph "Layer 1: Network Security"
        L1[VPC<br/>Security Groups<br/>Private Subnets]
    end

    subgraph "Layer 2: Kubernetes Security"
        L2[RBAC<br/>Network Policies<br/>Pod Security]
    end

    subgraph "Layer 3: Application Auth"
        L3[API Keys<br/>OAuth2<br/>Service Accounts]
    end

    subgraph "Layer 4: Multi-Factor Auth"
        L4[TOTP/2FA<br/>Google Authenticator<br/>Backup Codes]
    end

    subgraph "Layer 5: Provider Auth"
        L5[AWS IAM IRSA<br/>GCP Workload Identity<br/>API Key Management]
    end

    L1 --> L2
    L2 --> L3
    L3 --> L4
    L4 --> L5

    style L1 fill:#ffebee
    style L2 fill:#e8f5e9
    style L3 fill:#e3f2fd
    style L4 fill:#fff3e0
    style L5 fill:#f3e5f5
```

### Authentication Methods

The proxy supports multiple enterprise-grade authentication methods:

#### 1. **Database-Backed API Keys** (Production Recommended)
- Individual API keys stored in embedded SQLite database
- Bcrypt hashed keys for security
- Per-user tracking and audit logging
- Optional key expiration
- Support for key revocation

```bash
# Quick setup with automated script
./scripts/setup-auth.sh

# Or manual setup
kubectl create secret generic bedrock-api-keys \
  --from-literal=API_KEY_USER1=$(openssl rand -hex 32) \
  -n bedrock-system
```

#### 2. **TOTP/2FA (Google Authenticator)**
- Time-based One-Time Passwords
- Backup codes for account recovery
- Per-API-key 2FA configuration
- Compatible with Google Authenticator, Authy, etc.

```bash
# Enable 2FA
export AUTH_ENABLED=true
export REQUIRE_2FA=true
```

#### 3. **AWS Cognito + OAuth2/OIDC**
- Enterprise SSO integration
- Social login (Google, GitHub, etc.)
- AWS ALB-integrated authentication
- JWT token validation

**See:** [`deployments/kubernetes/aws-native-auth.yaml`](deployments/kubernetes/aws-native-auth.yaml)

#### 4. **AWS IAM Identity Center (SSO)**
- Corporate directory integration
- Multi-account access
- Centralized user management

#### 5. **Kubernetes Service Account**
- Zero-config for K8s services
- Automatic mTLS with service mesh
- RBAC-based access control

### Built-in Security Features

- **Container Security**: Distroless base image, non-root execution
- **Network Security**: Private VPC deployment, network policies
- **Authentication**: Multiple methods (API keys, OAuth2, TOTP, IAM)
- **Audit Logging**: Comprehensive request/response tracking
- **Rate Limiting**: Per-user request throttling
- **Encryption**: TLS/HTTPS support, secrets encrypted at rest
- **Compliance**: OWASP, NVD, and Trivy scanning in CI/CD

### Quick Start - Secure Setup

**5-Minute Setup** (API Keys + 2FA):

```bash
# 1. Run automated security setup
./scripts/setup-auth.sh

# 2. Deploy with auth enabled
kubectl apply -f deployments/kubernetes/deployment-with-auth.yaml

# 3. Get API key and enable 2FA
API_KEY=$(kubectl get secret bedrock-api-keys -n bedrock-system -o jsonpath='{.data.API_KEY_ADMIN}' | base64 -d)

# 4. Use with API key and TOTP
curl -H "X-API-Key: $API_KEY" \
     -H "X-TOTP-Code: 123456" \
     https://bedrock-proxy/model/claude-3-sonnet/invoke
```

**AWS EKS Native** (Cognito OAuth2):

```bash
# Deploy with Cognito authentication
kubectl apply -f deployments/kubernetes/aws-native-auth.yaml

# Access via browser - redirects to AWS Cognito login
https://bedrock-proxy.example.com
```

### Documentation

- **[🚀 Security Quick Start](docs/SECURITY-QUICKSTART.md)** - 5-minute setup guide
- **[📚 Complete Authorization Guide](docs/AUTHORIZATION.md)** - All auth methods
- **[☁️ AWS EKS Integration](deployments/kubernetes/aws-native-auth.yaml)** - Native AWS auth

### Security Scanning

The project includes comprehensive security scanning:

```bash
# OWASP Dependency Check
dependency-check --project bedrock-proxy --scan .

# Trivy container scan
trivy image bedrock-proxy:latest

# Go security check
gosec ./...

# Run all tests including security tests
go test ./... -v
```

## Development

### Project Structure

```
.
├── cmd/bedrock-proxy/          # Main application
├── internal/                   # Private application code
│   ├── auth/                  # AWS authentication
│   ├── health/                # Health checking
│   ├── middleware/            # HTTP middleware
│   └── proxy/                 # Bedrock proxy logic
├── pkg/                       # Public packages
│   └── metrics/              # Prometheus metrics
├── deployments/              # Deployment configurations
│   ├── kubernetes/           # K8s manifests
│   └── terraform/            # Infrastructure code
├── build/                    # Build configurations
│   └── Dockerfile           # Multi-stage Dockerfile
└── .github/workflows/        # CI/CD pipelines
```

### Adding New Features

1. Add code in appropriate `internal/` package
2. Add tests with `_test.go` suffix
3. Update metrics in `pkg/metrics/`
4. Update documentation

### Testing

```bash
# Unit tests
go test ./...

# Integration tests (requires AWS credentials)
go test ./... -tags=integration

# Benchmark tests
go test -bench=. ./...
```

## Monitoring

### Metrics

The proxy exposes Prometheus metrics at `/metrics`:

- `bedrock_proxy_requests_total` - Total requests
- `bedrock_proxy_request_duration_seconds` - Request duration
- `http_requests_total` - HTTP request count
- `health_check_status` - Health status

### Logging

Structured JSON logging with:
- Request ID correlation
- AWS credential events
- Error details
- Performance metrics

### Alerting

Example Prometheus alerting rules:

```yaml
- alert: BedrockProxyDown
  expr: up{job="bedrock-proxy"} == 0
  for: 1m
  annotations:
    summary: "Bedrock proxy is down"

- alert: BedrockProxyHighErrorRate
  expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
  for: 2m
  annotations:
    summary: "High error rate in Bedrock proxy"
```

## Troubleshooting

### Common Issues

1. **Authentication Errors**:
   - Check IAM role permissions
   - Verify IRSA configuration
   - Check AWS credentials in logs

2. **Connection Timeouts**:
   - Verify VPC endpoints
   - Check security groups
   - Review network policies

3. **High Memory Usage**:
   - Check request patterns
   - Monitor concurrent connections
   - Review resource limits

### Debug Mode

Enable debug logging:
```bash
export GIN_MODE=debug
export LOG_LEVEL=debug
./bedrock-proxy
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0 - see LICENSE file for details.

## Disclaimer

This software is provided "as is" without warranty of any kind. No commitments
are made regarding throughput, reliability, latency, or any performance
characteristics. Use at your own risk.

The software is not intended for use in safety-critical systems or where
failure could result in personal injury or severe property or environmental damage.

## Support

For issues and questions:
- Create GitHub issue
- Check troubleshooting guide
- Review logs and metrics