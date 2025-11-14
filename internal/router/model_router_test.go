package router

import (
	"testing"
)

// ModelProviderMapping represents which provider handles which model
type ModelProviderMapping struct {
	Model    string
	Provider string
	Region   string // Optional region for multi-region support
}

// TestMultiProviderRouting tests routing different models to their providers
func TestMultiProviderRouting(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		expectedProvider string
		expectedRegion   string
	}{
		// AWS Bedrock models
		{
			name:             "Claude 3 Sonnet → Bedrock",
			model:            "claude-3-sonnet-20240229",
			expectedProvider: "bedrock",
			expectedRegion:   "us-east-1",
		},
		{
			name:             "Claude 3 Opus → Bedrock",
			model:            "claude-3-opus-20240229",
			expectedProvider: "bedrock",
			expectedRegion:   "us-east-1",
		},
		{
			name:             "Claude 3.5 Sonnet → Bedrock",
			model:            "claude-3-5-sonnet-20240620",
			expectedProvider: "bedrock",
			expectedRegion:   "us-east-1",
		},
		{
			name:             "Amazon Titan → Bedrock",
			model:            "amazon.titan-text-express-v1",
			expectedProvider: "bedrock",
			expectedRegion:   "us-east-1",
		},

		// OpenAI models
		{
			name:             "GPT-4 → OpenAI",
			model:            "gpt-4",
			expectedProvider: "openai",
			expectedRegion:   "",
		},
		{
			name:             "GPT-4 Turbo → OpenAI",
			model:            "gpt-4-turbo",
			expectedProvider: "openai",
			expectedRegion:   "",
		},
		{
			name:             "GPT-3.5 Turbo → OpenAI",
			model:            "gpt-3.5-turbo",
			expectedProvider: "openai",
			expectedRegion:   "",
		},

		// Azure OpenAI models
		{
			name:             "GPT-4 deployment on Azure → Azure",
			model:            "gpt-4-azure-deployment",
			expectedProvider: "azure",
			expectedRegion:   "eastus",
		},

		// Google Vertex AI models
		{
			name:             "Gemini Pro → Vertex AI",
			model:            "gemini-pro",
			expectedProvider: "vertex",
			expectedRegion:   "us-central1",
		},
		{
			name:             "Gemini 1.5 Pro → Vertex AI",
			model:            "gemini-1.5-pro",
			expectedProvider: "vertex",
			expectedRegion:   "us-central1",
		},

		// Anthropic Direct
		{
			name:             "Claude via Anthropic API → Anthropic",
			model:            "claude-3-sonnet-20240229-anthropic",
			expectedProvider: "anthropic",
			expectedRegion:   "",
		},

		// IBM watsonx.ai
		{
			name:             "Granite model → IBM watsonx",
			model:            "ibm/granite-13b-chat-v2",
			expectedProvider: "ibm",
			expectedRegion:   "us-south",
		},

		// Oracle Cloud AI
		{
			name:             "Cohere model on OCI → Oracle",
			model:            "cohere.command-r-plus",
			expectedProvider: "oracle",
			expectedRegion:   "us-ashburn-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := RouteModelToProvider(tt.model)

			if mapping.Provider != tt.expectedProvider {
				t.Errorf("provider: got %q, want %q", mapping.Provider, tt.expectedProvider)
			}
			if tt.expectedRegion != "" && mapping.Region != tt.expectedRegion {
				t.Errorf("region: got %q, want %q", mapping.Region, tt.expectedRegion)
			}
		})
	}
}

// RouteModelToProvider determines which provider should handle a model
func RouteModelToProvider(model string) ModelProviderMapping {
	// AWS Bedrock models
	bedrockModels := map[string]string{
		"claude-3-sonnet-20240229":    "us-east-1",
		"claude-3-opus-20240229":      "us-east-1",
		"claude-3-5-sonnet-20240620":  "us-east-1",
		"claude-3-haiku-20240307":     "us-east-1",
		"amazon.titan-text-express-v1": "us-east-1",
		"amazon.titan-text-lite-v1":   "us-east-1",
		"ai21.j2-ultra-v1":            "us-east-1",
		"meta.llama2-70b-chat-v1":     "us-east-1",
		"mistral.mistral-7b-instruct-v0:2": "us-east-1",
	}

	if region, ok := bedrockModels[model]; ok {
		return ModelProviderMapping{
			Model:    model,
			Provider: "bedrock",
			Region:   region,
		}
	}

	// OpenAI models
	openaiModels := []string{
		"gpt-4", "gpt-4-turbo", "gpt-4-turbo-preview",
		"gpt-3.5-turbo", "gpt-3.5-turbo-16k",
		"text-davinci-003", "text-davinci-002",
	}
	for _, m := range openaiModels {
		if model == m {
			return ModelProviderMapping{
				Model:    model,
				Provider: "openai",
				Region:   "",
			}
		}
	}

	// Google Vertex AI models
	vertexModels := map[string]string{
		"gemini-pro":     "us-central1",
		"gemini-1.5-pro": "us-central1",
		"gemini-ultra":   "us-central1",
		"text-bison":     "us-central1",
		"chat-bison":     "us-central1",
	}

	if region, ok := vertexModels[model]; ok {
		return ModelProviderMapping{
			Model:    model,
			Provider: "vertex",
			Region:   region,
		}
	}

	// Azure OpenAI (deployment-based)
	if len(model) > 5 && model[len(model)-5:] == "-azure" ||
	   len(model) > 11 && model[len(model)-11:] == "-deployment" {
		return ModelProviderMapping{
			Model:    model,
			Provider: "azure",
			Region:   "eastus",
		}
	}

	// Anthropic Direct API
	if len(model) > 10 && model[len(model)-10:] == "-anthropic" {
		return ModelProviderMapping{
			Model:    model,
			Provider: "anthropic",
			Region:   "",
		}
	}

	// IBM watsonx.ai
	if len(model) > 4 && model[:4] == "ibm/" {
		return ModelProviderMapping{
			Model:    model,
			Provider: "ibm",
			Region:   "us-south",
		}
	}

	// Oracle Cloud AI
	if len(model) > 7 && model[:7] == "cohere." {
		return ModelProviderMapping{
			Model:    model,
			Provider: "oracle",
			Region:   "us-ashburn-1",
		}
	}

	// Default: try OpenAI
	return ModelProviderMapping{
		Model:    model,
		Provider: "openai",
		Region:   "",
	}
}

// TestProviderCapabilities tests provider-specific capabilities
func TestProviderCapabilities(t *testing.T) {
	tests := []struct {
		provider         string
		supportsStreaming bool
		supportsVision    bool
		supportsTools     bool
		maxTokens         int
	}{
		{
			provider:         "bedrock",
			supportsStreaming: true,
			supportsVision:    true,
			supportsTools:     true,
			maxTokens:         200000,
		},
		{
			provider:         "openai",
			supportsStreaming: true,
			supportsVision:    true,
			supportsTools:     true,
			maxTokens:         128000,
		},
		{
			provider:         "anthropic",
			supportsStreaming: true,
			supportsVision:    true,
			supportsTools:     true,
			maxTokens:         200000,
		},
		{
			provider:         "vertex",
			supportsStreaming: true,
			supportsVision:    true,
			supportsTools:     true,
			maxTokens:         32000,
		},
		{
			provider:         "azure",
			supportsStreaming: true,
			supportsVision:    true,
			supportsTools:     true,
			maxTokens:         128000,
		},
		{
			provider:         "ibm",
			supportsStreaming: false,
			supportsVision:    false,
			supportsTools:     false,
			maxTokens:         8192,
		},
		{
			provider:         "oracle",
			supportsStreaming: true,
			supportsVision:    false,
			supportsTools:     true,
			maxTokens:         4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			caps := GetProviderCapabilities(tt.provider)

			if caps.SupportsStreaming != tt.supportsStreaming {
				t.Errorf("streaming: got %v, want %v", caps.SupportsStreaming, tt.supportsStreaming)
			}
			if caps.SupportsVision != tt.supportsVision {
				t.Errorf("vision: got %v, want %v", caps.SupportsVision, tt.supportsVision)
			}
			if caps.SupportsTools != tt.supportsTools {
				t.Errorf("tools: got %v, want %v", caps.SupportsTools, tt.supportsTools)
			}
			if caps.MaxTokens != tt.maxTokens {
				t.Errorf("max tokens: got %d, want %d", caps.MaxTokens, tt.maxTokens)
			}
		})
	}
}

// ProviderCapabilities represents what a provider supports
type ProviderCapabilities struct {
	SupportsStreaming bool
	SupportsVision    bool
	SupportsTools     bool
	MaxTokens         int
}

// GetProviderCapabilities returns capabilities for a provider
func GetProviderCapabilities(provider string) ProviderCapabilities {
	capabilities := map[string]ProviderCapabilities{
		"bedrock": {
			SupportsStreaming: true,
			SupportsVision:    true,
			SupportsTools:     true,
			MaxTokens:         200000,
		},
		"openai": {
			SupportsStreaming: true,
			SupportsVision:    true,
			SupportsTools:     true,
			MaxTokens:         128000,
		},
		"anthropic": {
			SupportsStreaming: true,
			SupportsVision:    true,
			SupportsTools:     true,
			MaxTokens:         200000,
		},
		"vertex": {
			SupportsStreaming: true,
			SupportsVision:    true,
			SupportsTools:     true,
			MaxTokens:         32000,
		},
		"azure": {
			SupportsStreaming: true,
			SupportsVision:    true,
			SupportsTools:     true,
			MaxTokens:         128000,
		},
		"ibm": {
			SupportsStreaming: false,
			SupportsVision:    false,
			SupportsTools:     false,
			MaxTokens:         8192,
		},
		"oracle": {
			SupportsStreaming: true,
			SupportsVision:    false,
			SupportsTools:     true,
			MaxTokens:         4096,
		},
	}

	if caps, ok := capabilities[provider]; ok {
		return caps
	}

	// Default capabilities
	return ProviderCapabilities{
		SupportsStreaming: false,
		SupportsVision:    false,
		SupportsTools:     false,
		MaxTokens:         4096,
	}
}
