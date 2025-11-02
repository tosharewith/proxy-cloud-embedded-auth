// Copyright 2025 Bedrock Proxy Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/handlers"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/health"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/instance"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/middleware"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/anthropic"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/azure"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/bedrock"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/ibm"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/openai"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/oracle"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers/vertex"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Configuration from environment
	port := getEnv("PORT", "8080")
	tlsPort := getEnv("TLS_PORT", "8443")
	region := getEnv("AWS_REGION", "us-east-1")
	ginMode := getEnv("GIN_MODE", "release")
	authEnabled := getEnv("AUTH_ENABLED", "false") == "true"
	authMode := getEnv("AUTH_MODE", "api_key")
	tlsCertFile := getEnv("TLS_CERT_FILE", "/etc/tls/tls.crt")
	tlsKeyFile := getEnv("TLS_KEY_FILE", "/etc/tls/tls.key")
	tlsEnabled := getEnv("TLS_ENABLED", "false") == "true"
	modelMappingConfig := getEnv("MODEL_MAPPING_CONFIG", "configs/model-mapping.yaml")
	providerInstancesConfig := getEnv("PROVIDER_INSTANCES_CONFIG", "configs/provider-instances.yaml")

	// Set Gin mode
	gin.SetMode(ginMode)

	// Initialize components
	healthChecker := health.NewChecker()

	// Initialize providers
	log.Println("Initializing providers...")
	providerRegistry := make(map[string]providers.Provider)

	// Bedrock provider (always initialize if AWS region is set)
	if region != "" {
		bedrockProvider, err := bedrock.NewBedrockProvider(region)
		if err != nil {
			log.Printf("Warning: Failed to create Bedrock provider: %v", err)
		} else {
			providerRegistry["bedrock"] = bedrockProvider
			log.Printf("✓ Bedrock provider initialized (region: %s)", region)
		}
	}

	// Azure OpenAI provider
	if azureEndpoint := os.Getenv("AZURE_OPENAI_ENDPOINT"); azureEndpoint != "" {
		azureAPIKey := os.Getenv("AZURE_OPENAI_API_KEY")
		if azureAPIKey != "" {
			azureProvider, err := azure.NewAzureProvider(azure.AzureConfig{
				Endpoint:   azureEndpoint,
				APIKey:     azureAPIKey,
				APIVersion: getEnv("AZURE_API_VERSION", "2024-02-15-preview"),
			})
			if err != nil {
				log.Printf("Warning: Failed to create Azure provider: %v", err)
			} else {
				providerRegistry["azure"] = azureProvider
				log.Println("✓ Azure OpenAI provider initialized")
			}
		}
	}

	// OpenAI provider
	if openaiAPIKey := os.Getenv("OPENAI_API_KEY"); openaiAPIKey != "" {
		openaiProvider, err := openai.NewOpenAIProvider(openai.OpenAIConfig{
			APIKey:  openaiAPIKey,
			BaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		})
		if err != nil {
			log.Printf("Warning: Failed to create OpenAI provider: %v", err)
		} else {
			providerRegistry["openai"] = openaiProvider
			log.Println("✓ OpenAI provider initialized")
		}
	}

	// Anthropic provider
	if anthropicAPIKey := os.Getenv("ANTHROPIC_API_KEY"); anthropicAPIKey != "" {
		anthropicProvider, err := anthropic.NewAnthropicProvider(anthropic.AnthropicConfig{
			APIKey:  anthropicAPIKey,
			BaseURL: getEnv("ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"),
		})
		if err != nil {
			log.Printf("Warning: Failed to create Anthropic provider: %v", err)
		} else {
			providerRegistry["anthropic"] = anthropicProvider
			log.Println("✓ Anthropic provider initialized")
		}
	}

	// Google Vertex AI provider
	if gcpProjectID := os.Getenv("GCP_PROJECT_ID"); gcpProjectID != "" {
		vertexProvider, err := vertex.NewVertexProvider(vertex.VertexConfig{
			ProjectID:   gcpProjectID,
			Location:    getEnv("GCP_LOCATION", "us-central1"),
			AccessToken: os.Getenv("GCP_ACCESS_TOKEN"), // Or use Application Default Credentials
		})
		if err != nil {
			log.Printf("Warning: Failed to create Vertex AI provider: %v", err)
		} else {
			providerRegistry["vertex"] = vertexProvider
			log.Println("✓ Google Vertex AI provider initialized")
		}
	}

	// IBM Watson provider
	if ibmAPIKey := os.Getenv("IBM_API_KEY"); ibmAPIKey != "" {
		ibmProjectID := os.Getenv("IBM_PROJECT_ID")
		if ibmProjectID != "" {
			ibmProvider, err := ibm.NewIBMProvider(ibm.IBMConfig{
				APIKey:    ibmAPIKey,
				ProjectID: ibmProjectID,
				BaseURL:   getEnv("IBM_BASE_URL", "https://us-south.ml.cloud.ibm.com"),
			})
			if err != nil {
				log.Printf("Warning: Failed to create IBM Watson provider: %v", err)
			} else {
				providerRegistry["ibm"] = ibmProvider
				log.Println("✓ IBM Watson provider initialized")
			}
		}
	}

	// Oracle Cloud AI provider
	if oracleEndpoint := os.Getenv("ORACLE_ENDPOINT"); oracleEndpoint != "" {
		oracleAuthToken := os.Getenv("ORACLE_AUTH_TOKEN")
		oracleCompartmentID := os.Getenv("ORACLE_COMPARTMENT_ID")
		if oracleAuthToken != "" && oracleCompartmentID != "" {
			oracleProvider, err := oracle.NewOracleProvider(oracle.OracleConfig{
				Endpoint:      oracleEndpoint,
				AuthToken:     oracleAuthToken,
				CompartmentID: oracleCompartmentID,
			})
			if err != nil {
				log.Printf("Warning: Failed to create Oracle Cloud AI provider: %v", err)
			} else {
				providerRegistry["oracle"] = oracleProvider
				log.Println("✓ Oracle Cloud AI provider initialized")
			}
		}
	}

	if len(providerRegistry) == 0 {
		log.Fatal("No providers initialized. Please configure at least one provider.")
	}
	log.Printf("Total providers initialized: %d", len(providerRegistry))

	// Load router configuration
	log.Printf("Loading model mapping configuration from: %s", modelMappingConfig)
	routerConfig, err := router.LoadConfig(modelMappingConfig)
	if err != nil {
		log.Fatalf("Failed to load router config: %v", err)
	}
	log.Println("✓ Model mapping configuration loaded")

	// Initialize router
	aiRouter, err := router.NewRouter(routerConfig, providerRegistry)
	if err != nil {
		log.Fatalf("Failed to create router: %v", err)
	}
	log.Println("✓ Router initialized")

	// Validate configuration
	enabledProviders := routerConfig.ListEnabledProviders()
	log.Printf("Enabled providers: %s", strings.Join(enabledProviders, ", "))

	// Load provider instances configuration for transparent and protocol modes
	log.Printf("Loading provider instances configuration from: %s", providerInstancesConfig)
	instanceConfig, err := instance.LoadConfig(providerInstancesConfig)
	if err != nil {
		log.Printf("Warning: Failed to load provider instances config: %v", err)
		log.Println("Continuing without transparent/protocol mode support")
		instanceConfig = nil
	} else {
		log.Println("✓ Provider instances configuration loaded")
		transparentInstances := instanceConfig.ListInstancesByMode("transparent")
		protocolInstances := instanceConfig.ListInstancesByMode("protocol")
		log.Printf("  - Transparent mode instances: %d", len(transparentInstances))
		log.Printf("  - Protocol mode instances: %d", len(protocolInstances))
	}

	// Initialize handlers
	openaiHandler := handlers.NewOpenAIHandler(aiRouter)

	// Initialize transparent and protocol handlers if config is available
	var transparentHandler *handlers.TransparentHandler
	var protocolHandler *handlers.ProtocolHandler
	if instanceConfig != nil {
		transparentHandler = handlers.NewTransparentHandler(providerRegistry, instanceConfig)
		protocolHandler = handlers.NewProtocolHandler(providerRegistry, instanceConfig)
		log.Println("✓ Transparent and protocol handlers initialized")
	}

	// Initialize Gin router
	ginRouter := gin.New()

	// Global middleware
	ginRouter.Use(middleware.Recovery())
	ginRouter.Use(middleware.RequestID())
	ginRouter.Use(middleware.Logger())
	ginRouter.Use(middleware.Security())
	ginRouter.Use(middleware.Metrics())

	// Health endpoints (no auth required)
	ginRouter.GET("/health", healthHandler(healthChecker))
	ginRouter.GET("/ready", readyHandler(healthChecker, aiRouter))
	ginRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// OpenAI-compatible API endpoints
	openaiGroup := ginRouter.Group("/v1")
	if authEnabled {
		log.Printf("Authentication enabled for OpenAI API: mode=%s", authMode)
		openaiGroup.Use(getAuthMiddleware(authMode))
	}
	{
		openaiGroup.POST("/chat/completions", openaiHandler.ChatCompletions)
		openaiGroup.GET("/models", openaiHandler.ListModels)
		openaiGroup.GET("/models/:model", openaiHandler.GetModel)
	}

	// Transparent mode endpoints (/transparent/{provider}/*)
	if transparentHandler != nil && instanceConfig != nil && instanceConfig.IsFeatureEnabled("transparent_mode") {
		transparentGroup := ginRouter.Group("/transparent")
		if authEnabled {
			log.Printf("Authentication enabled for transparent mode: mode=%s", authMode)
			transparentGroup.Use(getAuthMiddleware(authMode))
		}
		{
			transparentGroup.Any("/*path", transparentHandler.HandleRequest)
		}
		log.Println("✓ Transparent mode endpoints registered: /transparent/*")
	}

	// Protocol mode endpoints (/{protocol}/{instance_name}/*)
	if protocolHandler != nil && instanceConfig != nil && instanceConfig.IsFeatureEnabled("protocol_mode") {
		protocolGroup := ginRouter.Group("/")
		if authEnabled {
			log.Printf("Authentication enabled for protocol mode: mode=%s", authMode)
			protocolGroup.Use(getAuthMiddleware(authMode))
		}
		{
			// Register protocol endpoints (e.g., /openai/bedrock_us1_openai/*)
			protocolGroup.POST("/openai/*path", protocolHandler.HandleRequest)
			protocolGroup.POST("/anthropic/*path", protocolHandler.HandleRequest)
		}
		log.Println("✓ Protocol mode endpoints registered: /{protocol}/*")
	}

	// Native provider API endpoints
	providersGroup := ginRouter.Group("/providers")
	if authEnabled {
		log.Printf("Authentication enabled for provider APIs: mode=%s", authMode)
		providersGroup.Use(getAuthMiddleware(authMode))
	}
	{
		// Register native API endpoints for each provider
		if bedrockProvider, ok := providerRegistry["bedrock"]; ok {
			providersGroup.Any("/bedrock/*path", createProviderHandler(bedrockProvider, healthChecker))
		}
		if azureProvider, ok := providerRegistry["azure"]; ok {
			providersGroup.Any("/azure/*path", createProviderHandler(azureProvider, healthChecker))
		}
		if openaiProvider, ok := providerRegistry["openai"]; ok {
			providersGroup.Any("/openai/*path", createProviderHandler(openaiProvider, healthChecker))
		}
		if anthropicProvider, ok := providerRegistry["anthropic"]; ok {
			providersGroup.Any("/anthropic/*path", createProviderHandler(anthropicProvider, healthChecker))
		}
		if vertexProvider, ok := providerRegistry["vertex"]; ok {
			providersGroup.Any("/vertex/*path", createProviderHandler(vertexProvider, healthChecker))
		}
		if ibmProvider, ok := providerRegistry["ibm"]; ok {
			providersGroup.Any("/ibm/*path", createProviderHandler(ibmProvider, healthChecker))
		}
		if oracleProvider, ok := providerRegistry["oracle"]; ok {
			providersGroup.Any("/oracle/*path", createProviderHandler(oracleProvider, healthChecker))
		}
	}

	// Legacy endpoints (backward compatibility - Bedrock only)
	if bedrockProvider, ok := providerRegistry["bedrock"]; ok {
		legacyGroup := ginRouter.Group("/")
		if authEnabled {
			legacyGroup.Use(getAuthMiddleware(authMode))
		}
		{
			legacyGroup.Any("/v1/bedrock/*path", createProviderHandler(bedrockProvider, healthChecker))
			legacyGroup.Any("/bedrock/*path", createProviderHandler(bedrockProvider, healthChecker))
			legacyGroup.Any("/model/*path", createProviderHandler(bedrockProvider, healthChecker))
		}
	}

	// Print startup banner
	printStartupBanner(port, tlsPort, tlsEnabled, authEnabled, enabledProviders, instanceConfig)

	// Start server(s)
	if tlsEnabled {
		// Start HTTP server in goroutine
		go func() {
			addr := fmt.Sprintf(":%s", port)
			log.Printf("Starting HTTP server on %s", addr)
			if err := ginRouter.Run(addr); err != nil {
				log.Fatalf("Failed to start HTTP server: %v", err)
			}
		}()

		// Start HTTPS/TLS server (blocking)
		addrTLS := fmt.Sprintf(":%s", tlsPort)
		log.Printf("Starting HTTPS/TLS server on %s", addrTLS)
		if err := ginRouter.RunTLS(addrTLS, tlsCertFile, tlsKeyFile); err != nil {
			log.Fatalf("Failed to start HTTPS/TLS server: %v", err)
		}
	} else {
		// Start HTTP server only
		addr := fmt.Sprintf(":%s", port)
		log.Printf("Starting HTTP server on %s", addr)
		if err := ginRouter.Run(addr); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}
}

// createProviderHandler creates a handler for native provider API
func createProviderHandler(provider providers.Provider, healthChecker *health.Checker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract path after the prefix
		path := c.Param("path")

		// Build provider request
		body, _ := c.GetRawData()
		providerReq := &providers.ProviderRequest{
			Method:      c.Request.Method,
			Path:        path,
			Headers:     make(map[string]string),
			Body:        body,
			QueryParams: make(map[string]string),
			Context:     c.Request.Context(),
		}

		// Copy headers
		for key := range c.Request.Header {
			providerReq.Headers[key] = c.Request.Header.Get(key)
		}

		// Copy query params
		for key := range c.Request.URL.Query() {
			providerReq.QueryParams[key] = c.Request.URL.Query().Get(key)
		}

		// Invoke provider
		resp, err := provider.Invoke(c.Request.Context(), providerReq)
		if err != nil {
			healthChecker.RecordError()
			if providerErr, ok := err.(*providers.ProviderError); ok {
				c.Data(providerErr.StatusCode, "application/json", []byte(fmt.Sprintf(`{"error":"%s"}`, providerErr.Message)))
			} else {
				c.JSON(500, gin.H{"error": "Internal server error"})
			}
			return
		}

		healthChecker.RecordSuccess()

		// Return response
		for key, value := range resp.Headers {
			c.Header(key, value)
		}
		c.Data(resp.StatusCode, "application/json", resp.Body)
	}
}

// getAuthMiddleware returns the appropriate auth middleware
func getAuthMiddleware(authMode string) gin.HandlerFunc {
	switch authMode {
	case "api_key":
		apiKeys := middleware.LoadAPIKeysFromEnv()
		if len(apiKeys) == 0 {
			log.Fatal("API key auth enabled but no keys found. Set BEDROCK_API_KEY_<NAME> env vars")
		}
		log.Printf("Loaded %d API keys", len(apiKeys))
		return middleware.APIKeyAuth(apiKeys)

	case "basic":
		credentials := loadBasicAuthCredentials()
		if len(credentials) == 0 {
			log.Fatal("Basic auth enabled but no credentials found")
		}
		return middleware.BasicAuth(credentials)

	case "service_account":
		allowedSAs := loadAllowedServiceAccounts()
		if len(allowedSAs) == 0 {
			log.Fatal("Service account auth enabled but no allowed accounts found")
		}
		return middleware.ServiceAccountAuth(allowedSAs)

	default:
		log.Printf("Unknown auth mode: %s, running without auth", authMode)
		return func(c *gin.Context) { c.Next() }
	}
}

func healthHandler(checker *health.Checker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker.IsHealthy() {
			c.JSON(200, gin.H{
				"status":  "healthy",
				"service": "ai-gateway",
			})
		} else {
			c.JSON(503, gin.H{
				"status":  "unhealthy",
				"service": "ai-gateway",
			})
		}
	}
}

func readyHandler(checker *health.Checker, aiRouter *router.Router) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if providers are healthy
		healthResults := aiRouter.HealthCheck(c.Request.Context())
		allHealthy := true
		for _, err := range healthResults {
			if err != nil {
				allHealthy = false
				break
			}
		}

		if checker.IsHealthy() && allHealthy {
			c.JSON(200, gin.H{
				"status": "ready",
			})
		} else {
			c.JSON(503, gin.H{
				"status": "not ready",
			})
		}
	}
}

func loadBasicAuthCredentials() map[string]string {
	creds := make(map[string]string)

	// Load from BASIC_AUTH_CREDENTIALS env var (format: user1:pass1,user2:pass2)
	if credsEnv := os.Getenv("BASIC_AUTH_CREDENTIALS"); credsEnv != "" {
		for _, pair := range strings.Split(credsEnv, ",") {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) == 2 {
				creds[parts[0]] = parts[1]
			}
		}
	}

	return creds
}

func loadAllowedServiceAccounts() []string {
	var accounts []string

	// Load from ALLOWED_SERVICE_ACCOUNTS env var (format: ns1/sa1,ns2/sa2)
	if sasEnv := os.Getenv("ALLOWED_SERVICE_ACCOUNTS"); sasEnv != "" {
		for _, sa := range strings.Split(sasEnv, ",") {
			sa = strings.TrimSpace(sa)
			if sa != "" {
				accounts = append(accounts, sa)
			}
		}
	}

	return accounts
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func printStartupBanner(port, tlsPort string, tlsEnabled, authEnabled bool, enabledProviders []string, instanceConfig *instance.Config) {
	banner := `
╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║              🚀 Multi-Provider AI Gateway                   ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝

Configuration:
`
	fmt.Println(banner)
	fmt.Printf("  • HTTP Port:         %s\n", port)
	if tlsEnabled {
		fmt.Printf("  • HTTPS Port:        %s (enabled)\n", tlsPort)
	}
	fmt.Printf("  • Authentication:    %v\n", authEnabled)
	fmt.Printf("  • Enabled Providers: %s\n", strings.Join(enabledProviders, ", "))

	// Show instance configuration if available
	if instanceConfig != nil {
		transparentInstances := instanceConfig.ListInstancesByMode("transparent")
		protocolInstances := instanceConfig.ListInstancesByMode("protocol")
		fmt.Printf("  • Transparent Mode:  %d instances\n", len(transparentInstances))
		fmt.Printf("  • Protocol Mode:     %d instances\n", len(protocolInstances))
	}

	fmt.Println()
	fmt.Println("API Endpoints:")
	fmt.Printf("  • OpenAI-compatible: http://localhost:%s/v1/chat/completions\n", port)
	fmt.Printf("  • List models:       http://localhost:%s/v1/models\n", port)

	// Show transparent mode endpoints
	if instanceConfig != nil && instanceConfig.IsFeatureEnabled("transparent_mode") {
		fmt.Printf("  • Transparent mode:  http://localhost:%s/transparent/{provider}/...\n", port)
	}

	// Show protocol mode endpoints
	if instanceConfig != nil && instanceConfig.IsFeatureEnabled("protocol_mode") {
		fmt.Printf("  • Protocol mode:     http://localhost:%s/{protocol}/{instance}/...\n", port)
	}

	fmt.Printf("  • Native Bedrock:    http://localhost:%s/providers/bedrock/...\n", port)
	fmt.Printf("  • Health check:      http://localhost:%s/health\n", port)
	fmt.Printf("  • Metrics:           http://localhost:%s/metrics\n", port)
	fmt.Println()
	fmt.Println("🎯 Ready to accept requests!")
	fmt.Println()
}
