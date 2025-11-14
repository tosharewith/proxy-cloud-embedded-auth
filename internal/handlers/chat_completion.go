package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tosharewith/llmproxy_auth/internal/providers"
	"github.com/tosharewith/llmproxy_auth/internal/router"
	"github.com/tosharewith/llmproxy_auth/internal/translator"
)

// ChatCompletionHandler handles OpenAI-compatible chat completion requests
type ChatCompletionHandler struct {
	modelRouter *router.ModelRouter
}

// NewChatCompletionHandler creates a new chat completion handler
func NewChatCompletionHandler(modelRouter *router.ModelRouter) *ChatCompletionHandler {
	return &ChatCompletionHandler{
		modelRouter: modelRouter,
	}
}

// Handle processes a chat completion request
func (h *ChatCompletionHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Parse OpenAI request
	var openaiReq translator.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&openaiReq); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON in request body", err)
		return
	}

	// Validate request
	if openaiReq.Model == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "Model is required", nil)
		return
	}

	// Route to provider based on model
	provider, err := h.modelRouter.RouteModel(openaiReq.Model)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Model not supported: %s", openaiReq.Model), err)
		return
	}

	// Handle streaming vs non-streaming
	if openaiReq.Stream {
		h.handleStreaming(w, r, provider, &openaiReq)
	} else {
		h.handleNonStreaming(w, r, provider, &openaiReq)
	}
}

// handleNonStreaming handles non-streaming chat completion
func (h *ChatCompletionHandler) handleNonStreaming(w http.ResponseWriter, r *http.Request, provider providers.Provider, openaiReq *translator.ChatCompletionRequest) {
	ctx := r.Context()

	// Translate request to provider format
	providerReq, err := h.translateRequest(provider.Name(), openaiReq)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "Failed to translate request", err)
		return
	}

	// Call provider
	providerResp, err := provider.Invoke(ctx, providerReq)
	if err != nil {
		h.handleProviderError(w, err)
		return
	}

	// Translate response back to OpenAI format
	openaiResp, err := h.translateResponse(provider.Name(), providerResp.Body, openaiReq.Model)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to translate response", err)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openaiResp)
}

// handleStreaming handles streaming chat completion
func (h *ChatCompletionHandler) handleStreaming(w http.ResponseWriter, r *http.Request, provider providers.Provider, openaiReq *translator.ChatCompletionRequest) {
	ctx := r.Context()

	// Translate request
	providerReq, err := h.translateRequest(provider.Name(), openaiReq)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "Failed to translate request", err)
		return
	}

	// Call provider streaming
	stream, err := provider.InvokeStreaming(ctx, providerReq)
	if err != nil {
		h.handleProviderError(w, err)
		return
	}
	defer stream.Close()

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Stream the response
	// TODO: Implement streaming translation for each provider
	// For now, just proxy the stream
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "internal_error", "Streaming not supported", nil)
		return
	}

	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			// Error during streaming - can't send error response now
			break
		}
	}
}

// translateRequest translates OpenAI request to provider-specific format
func (h *ChatCompletionHandler) translateRequest(providerName string, openaiReq *translator.ChatCompletionRequest) (*providers.ProviderRequest, error) {
	switch providerName {
	case "bedrock":
		// Bedrock requires provider-specific translation
		providerReq, _, err := translator.TranslateOpenAIToBedrock(openaiReq)
		return providerReq, err

	case "openai":
		// OpenAI doesn't need translation - use OpenAI format as-is
		body, err := json.Marshal(openaiReq)
		if err != nil {
			return nil, err
		}
		return &providers.ProviderRequest{
			Method: "POST",
			Path:   "/chat/completions",
			Body:   body,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil

	case "anthropic":
		// Anthropic provider handles translation internally
		// Just pass OpenAI format and let the provider translate
		body, err := json.Marshal(openaiReq)
		if err != nil {
			return nil, err
		}
		return &providers.ProviderRequest{
			Method: "POST",
			Path:   "/messages",
			Body:   body,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil

	case "azure":
		// Azure uses OpenAI format with different path
		body, err := json.Marshal(openaiReq)
		if err != nil {
			return nil, err
		}
		// Azure path includes deployment name (model)
		return &providers.ProviderRequest{
			Method: "POST",
			Path:   fmt.Sprintf("/deployments/%s/chat/completions", openaiReq.Model),
			Body:   body,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			QueryParams: map[string]string{
				"api-version": "2024-02-15-preview",
			},
		}, nil

	default:
		return nil, fmt.Errorf("translation not implemented for provider: %s", providerName)
	}
}

// translateResponse translates provider response to OpenAI format
func (h *ChatCompletionHandler) translateResponse(providerName string, respBody []byte, model string) (*translator.ChatCompletionResponse, error) {
	switch providerName {
	case "bedrock":
		// Parse Bedrock response and translate to OpenAI format
		var bedrockResp translator.BedrockResponse
		if err := json.Unmarshal(respBody, &bedrockResp); err != nil {
			return nil, fmt.Errorf("failed to parse bedrock response: %w", err)
		}

		// Generate request ID
		requestID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())

		// Translate to OpenAI format
		openaiResp := translator.TranslateBedrockToOpenAI(&bedrockResp, model, requestID)
		return openaiResp, nil

	case "openai", "azure":
		// Already in OpenAI format
		var openaiResp translator.ChatCompletionResponse
		if err := json.Unmarshal(respBody, &openaiResp); err != nil {
			return nil, err
		}
		return &openaiResp, nil

	case "anthropic":
		// Anthropic provider already translated to OpenAI format internally
		// Just parse and return
		var openaiResp translator.ChatCompletionResponse
		if err := json.Unmarshal(respBody, &openaiResp); err != nil {
			return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
		}
		return &openaiResp, nil

	default:
		return nil, fmt.Errorf("translation not implemented for provider: %s", providerName)
	}
}

// handleProviderError converts provider error to OpenAI error format
func (h *ChatCompletionHandler) handleProviderError(w http.ResponseWriter, err error) {
	provErr, ok := err.(*providers.ProviderError)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error", err)
		return
	}

	// Map provider error codes to OpenAI error types
	var errorType string
	var statusCode int

	switch provErr.Code {
	case providers.ErrCodeInvalidRequest:
		errorType = "invalid_request_error"
		statusCode = http.StatusBadRequest
	case providers.ErrCodeAuthenticationFail:
		errorType = "authentication_error"
		statusCode = http.StatusUnauthorized
	case providers.ErrCodeRateLimitExceeded:
		errorType = "rate_limit_error"
		statusCode = http.StatusTooManyRequests
	case providers.ErrCodeModelNotFound:
		errorType = "invalid_request_error"
		statusCode = http.StatusNotFound
	case providers.ErrCodeServiceUnavailable:
		errorType = "service_unavailable"
		statusCode = http.StatusServiceUnavailable
	default:
		errorType = "internal_error"
		statusCode = http.StatusInternalServerError
	}

	if provErr.StatusCode != 0 {
		statusCode = provErr.StatusCode
	}

	h.writeError(w, statusCode, errorType, provErr.Message, provErr.Err)
}

// writeError writes an OpenAI-compatible error response
func (h *ChatCompletionHandler) writeError(w http.ResponseWriter, statusCode int, errorType, message string, err error) {
	errorResp := translator.ErrorResponse{
		Error: translator.ErrorDetail{
			Message: message,
			Type:    errorType,
		},
	}

	if err != nil {
		errorResp.Error.Message = fmt.Sprintf("%s: %v", message, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}
