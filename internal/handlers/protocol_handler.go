// Copyright 2025 Bedrock Proxy Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/instance"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/providers"
	"github.com/bedrock-proxy/bedrock-iam-proxy/internal/translator"
	"github.com/bedrock-proxy/bedrock-iam-proxy/pkg/metrics"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ProtocolHandler handles protocol-based requests with transformations
type ProtocolHandler struct {
	providers map[string]providers.Provider
	config    *instance.Config
}

// NewProtocolHandler creates a new protocol handler
func NewProtocolHandler(providerRegistry map[string]providers.Provider, config *instance.Config) *ProtocolHandler {
	return &ProtocolHandler{
		providers: providerRegistry,
		config:    config,
	}
}

// HandleRequest handles a protocol-based request with transformations
func (h *ProtocolHandler) HandleRequest(c *gin.Context) {
	startTime := time.Now()

	// Get request path
	path := c.Request.URL.Path

	// Find matching instance
	instanceCfg, instanceName, err := h.config.GetInstanceByPath(path)
	if err != nil {
		log.Printf("No instance found for path %s: %v", path, err)
		c.JSON(http.StatusNotFound, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: "No provider instance configured for this path",
				Type:    "invalid_request_error",
				Code:    "instance_not_found",
			},
		})
		return
	}

	// Verify it's a protocol mode instance
	if instanceCfg.Mode != "protocol" {
		log.Printf("Instance %s is not in protocol mode (mode: %s)", instanceName, instanceCfg.Mode)
		c.JSON(http.StatusBadRequest, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: "This endpoint requires protocol mode",
				Type:    "invalid_request_error",
				Code:    "invalid_mode",
			},
		})
		return
	}

	log.Printf("Protocol request: %s → %s (instance: %s, protocol: %s)",
		path, instanceCfg.Type, instanceName, instanceCfg.Protocol)

	// Get provider
	provider, ok := h.providers[instanceCfg.Type]
	if !ok {
		log.Printf("Provider %s not initialized", instanceCfg.Type)
		c.JSON(http.StatusServiceUnavailable, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: fmt.Sprintf("Provider %s not available", instanceCfg.Type),
				Type:    "service_error",
				Code:    "provider_unavailable",
			},
		})
		return
	}

	// Parse request based on protocol
	if instanceCfg.Protocol == "openai" {
		h.handleOpenAIProtocol(c, provider, instanceCfg, instanceName, startTime)
	} else {
		c.JSON(http.StatusNotImplemented, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: fmt.Sprintf("Protocol %s not yet implemented", instanceCfg.Protocol),
				Type:    "not_implemented_error",
				Code:    "protocol_not_implemented",
			},
		})
	}
}

// handleOpenAIProtocol handles OpenAI protocol requests
func (h *ProtocolHandler) handleOpenAIProtocol(
	c *gin.Context,
	provider providers.Provider,
	instanceCfg *instance.InstanceConfig,
	instanceName string,
	startTime time.Time,
) {
	// Parse OpenAI request
	var req translator.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: "Invalid request body",
				Type:    "invalid_request_error",
				Code:    "invalid_json",
			},
		})
		return
	}

	// Generate request ID
	requestID := fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8])

	// Apply transformation
	var providerReq *providers.ProviderRequest
	var err error

	if instanceCfg.Transformation == nil {
		// No transformation specified - treat as passthrough
		reqBody, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
				Error: translator.ErrorDetail{
					Message: "Failed to marshal request",
					Type:    "internal_error",
					Code:    "marshal_failed",
				},
			})
			return
		}
		providerReq = &providers.ProviderRequest{
			Method: "POST",
			Path:   "/chat/completions",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body:    reqBody,
			Context: c.Request.Context(),
		}
	} else {
		// Apply transformation based on configuration
		transformTo := instanceCfg.Transformation.RequestTo

		switch transformTo {
		case "bedrock_converse":
			providerReq, _, err = translator.TranslateOpenAIToConverseAPI(&req)
		case "openai":
			// Passthrough
			reqBody, err := json.Marshal(req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
					Error: translator.ErrorDetail{
						Message: "Failed to marshal request",
						Type:    "internal_error",
						Code:    "marshal_failed",
					},
				})
				return
			}
			providerReq = &providers.ProviderRequest{
				Method: "POST",
				Path:   "/chat/completions",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body:    reqBody,
				Context: c.Request.Context(),
			}
		default:
			// For other transformations, let provider handle it
			reqBody, err := json.Marshal(req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
					Error: translator.ErrorDetail{
						Message: "Failed to marshal request",
						Type:    "internal_error",
						Code:    "marshal_failed",
					},
				})
				return
			}
			providerReq = &providers.ProviderRequest{
				Method: "POST",
				Path:   "/chat/completions",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body:    reqBody,
				Context: c.Request.Context(),
			}
		}
	}

	if err != nil {
		log.Printf("Translation error: %v", err)
		c.JSON(http.StatusBadRequest, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: fmt.Sprintf("Failed to translate request: %v", err),
				Type:    "invalid_request_error",
				Code:    "translation_failed",
			},
		})
		return
	}

	// Invoke provider
	providerResp, err := provider.Invoke(c.Request.Context(), providerReq)
	if err != nil {
		log.Printf("Provider invocation error: %v", err)
		h.handleProviderError(c, err)
		return
	}

	// Parse and translate response
	var openaiResp *translator.ChatCompletionResponse

	if instanceCfg.Transformation != nil && instanceCfg.Transformation.ResponseFrom == "bedrock_converse" {
		// Translate from Bedrock Converse to OpenAI
		var converseResp translator.ConverseResponse
		if err := json.Unmarshal(providerResp.Body, &converseResp); err != nil {
			log.Printf("Failed to parse Bedrock response: %v", err)
			c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
				Error: translator.ErrorDetail{
					Message: "Failed to parse provider response",
					Type:    "internal_error",
					Code:    "response_parse_error",
				},
			})
			return
		}
		openaiResp = translator.TranslateConverseToOpenAI(&converseResp, req.Model, requestID)
	} else {
		// Response is already in OpenAI format or translated by provider
		if err := json.Unmarshal(providerResp.Body, &openaiResp); err != nil {
			log.Printf("Failed to parse provider response: %v", err)
			c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
				Error: translator.ErrorDetail{
					Message: "Failed to parse provider response",
					Type:    "internal_error",
					Code:    "response_parse_error",
				},
			})
			return
		}
	}

	// Set metadata
	openaiResp.ID = requestID
	openaiResp.Created = startTime.Unix()

	// Record metrics
	if instanceCfg.Metrics.Enabled {
		duration := time.Since(startTime)
		metrics.RequestDuration.WithLabelValues("POST", "200").Observe(duration.Seconds())
		metrics.RequestsTotal.WithLabelValues("POST", "200").Inc()
	}

	log.Printf("Protocol request completed: %s (status: 200, duration: %v)", instanceName, time.Since(startTime))

	c.JSON(http.StatusOK, openaiResp)
}

// handleProviderError converts provider errors to protocol error format
func (h *ProtocolHandler) handleProviderError(c *gin.Context, err error) {
	if providerErr, ok := err.(*providers.ProviderError); ok {
		statusCode := providerErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}

		c.JSON(statusCode, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: providerErr.Message,
				Type:    "provider_error",
				Code:    providerErr.Code,
			},
		})
	} else {
		c.JSON(http.StatusInternalServerError, translator.ErrorResponse{
			Error: translator.ErrorDetail{
				Message: "Internal server error",
				Type:    "internal_error",
				Code:    "unknown_error",
			},
		})
	}
}
