// Copyright 2025 Bedrock Proxy Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/instance"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers"
	"github.com/bedrock-proxy/bedrock-iam-proxy/pkg/metrics"
	"github.com/gin-gonic/gin"
)

// TransparentHandler handles transparent passthrough requests
// This mode adds authentication and metrics but does not transform requests/responses
type TransparentHandler struct {
	providers map[string]providers.Provider
	config    *instance.Config
}

// NewTransparentHandler creates a new transparent handler
func NewTransparentHandler(providerRegistry map[string]providers.Provider, config *instance.Config) *TransparentHandler {
	return &TransparentHandler{
		providers: providerRegistry,
		config:    config,
	}
}

// HandleRequest handles a transparent passthrough request
func (h *TransparentHandler) HandleRequest(c *gin.Context) {
	startTime := time.Now()

	// Get request path
	path := c.Request.URL.Path

	// Find matching instance
	instanceCfg, instanceName, err := h.config.GetInstanceByPath(path)
	if err != nil {
		log.Printf("No instance found for path %s: %v", path, err)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No provider instance configured for this path",
		})
		return
	}

	// Verify it's a transparent mode instance
	if instanceCfg.Mode != "transparent" {
		log.Printf("Instance %s is not in transparent mode (mode: %s)", instanceName, instanceCfg.Mode)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This endpoint requires transparent mode",
		})
		return
	}

	log.Printf("Transparent passthrough: %s → %s (instance: %s)", path, instanceCfg.Type, instanceName)

	// Get provider
	provider, ok := h.providers[instanceCfg.Type]
	if !ok {
		log.Printf("Provider %s not initialized", instanceCfg.Type)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": fmt.Sprintf("Provider %s not available", instanceCfg.Type),
		})
		return
	}

	// Read request body
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to read request body",
		})
		return
	}

	// Extract the actual provider path
	// Remove the transparent prefix to get the real API path
	// Example: /transparent/bedrock/model/invoke → /model/invoke
	providerPath := extractProviderPath(path, instanceCfg.Endpoints)

	// Build provider request
	providerReq := &providers.ProviderRequest{
		Method:      c.Request.Method,
		Path:        providerPath,
		Headers:     make(map[string]string),
		Body:        body,
		QueryParams: make(map[string]string),
		Context:     c.Request.Context(),
	}

	// Copy headers (except authentication which will be added by provider)
	for key := range c.Request.Header {
		if !isAuthHeader(key) {
			providerReq.Headers[key] = c.Request.Header.Get(key)
		}
	}

	// Copy query params
	for key := range c.Request.URL.Query() {
		providerReq.QueryParams[key] = c.Request.URL.Query().Get(key)
	}

	// Invoke provider (provider handles authentication)
	providerResp, err := provider.Invoke(c.Request.Context(), providerReq)
	if err != nil {
		log.Printf("Provider invocation error: %v", err)
		if providerErr, ok := err.(*providers.ProviderError); ok {
			c.Data(providerErr.StatusCode, "application/json", []byte(providerErr.Message))
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Provider request failed",
			})
		}
		return
	}

	// Record metrics
	if instanceCfg.Metrics.Enabled {
		duration := time.Since(startTime)

		// Build label values
		labelValues := []string{
			c.Request.Method,
			fmt.Sprintf("%d", providerResp.StatusCode),
		}

		// Add custom labels from config
		if instanceCfg.Metrics.Labels != nil {
			for _, value := range instanceCfg.Metrics.Labels {
				labelValues = append(labelValues, value)
			}
		}

		metrics.RequestDuration.WithLabelValues(labelValues[:2]...).Observe(duration.Seconds())
		metrics.RequestsTotal.WithLabelValues(labelValues[:2]...).Inc()
	}

	// Return response as-is (transparent passthrough)
	for key, value := range providerResp.Headers {
		c.Header(key, value)
	}
	c.Data(providerResp.StatusCode, getContentType(providerResp.Headers), providerResp.Body)

	log.Printf("Transparent passthrough completed: %s (status: %d, duration: %v)",
		instanceName, providerResp.StatusCode, time.Since(startTime))
}

// extractProviderPath extracts the actual provider API path from the full request path
func extractProviderPath(fullPath string, endpoints []instance.EndpointConfig) string {
	// Find matching endpoint and strip its prefix
	for _, endpoint := range endpoints {
		if len(fullPath) > len(endpoint.Path) && fullPath[:len(endpoint.Path)] == endpoint.Path {
			// Return everything after the endpoint path
			return fullPath[len(endpoint.Path):]
		}
	}
	// If no match, return as-is
	return fullPath
}

// isAuthHeader checks if a header is an authentication header
func isAuthHeader(headerName string) bool {
	authHeaders := []string{
		"Authorization",
		"X-API-Key",
		"x-api-key",
		"api-key",
		"X-Auth-Token",
	}
	for _, authHeader := range authHeaders {
		if headerName == authHeader {
			return true
		}
	}
	return false
}

// getContentType extracts content type from headers
func getContentType(headers map[string]string) string {
	if ct, ok := headers["Content-Type"]; ok {
		return ct
	}
	if ct, ok := headers["content-type"]; ok {
		return ct
	}
	return "application/json"
}
