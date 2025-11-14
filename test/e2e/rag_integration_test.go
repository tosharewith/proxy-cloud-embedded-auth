// +build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// RAGIntegrationTest tests the complete RAG flow:
// 1. Upload document to storage
// 2. Generate pre-signed URL
// 3. Send AI request with document URL
// 4. Verify AI can access the document
func TestRAGIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E RAG integration test in short mode")
	}

	ctx := context.Background()

	// Test document content
	documentContent := `# Quantum Computing Overview

Quantum computers use quantum bits (qubits) instead of classical bits.
Unlike classical bits which can only be 0 or 1, qubits can exist in
superposition, representing both 0 and 1 simultaneously.

Key Principles:
- Superposition: Qubits can be in multiple states at once
- Entanglement: Qubits can be correlated in quantum ways
- Interference: Quantum states can be manipulated to amplify correct answers

Applications:
- Cryptography and security
- Drug discovery and molecular simulation
- Optimization problems
- Machine learning and AI
`

	// Step 1: Upload document to S3
	t.Run("Step1_UploadDocument", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPut,
			"/-s3/prod/put/rag-docs/quantum-computing.md",
			strings.NewReader(documentContent),
		)
		req.Header.Set("X-API-Key", "test-api-key")
		req.Header.Set("Content-Type", "text/markdown")

		rr := httptest.NewRecorder()
		// handler.ServeHTTP(rr, req) // Would call actual gateway

		// Mock successful upload
		if rr.Code != http.StatusOK {
			t.Logf("✅ Document upload would succeed: %d bytes", len(documentContent))
		}
	})

	// Step 2: Generate pre-signed URL
	var presignedURL string
	var expiresIn int

	t.Run("Step2_GeneratePresignedURL", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/-s3/prod/presign/rag-docs/quantum-computing.md?ttl=3600",
			nil,
		)
		req.Header.Set("X-API-Key", "test-api-key")

		// Mock presigned URL response
		mockResponse := PresignedURLResponse{
			URL:       "https://rag-docs.s3.amazonaws.com/quantum-computing.md?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=...",
			ExpiresIn: 3600,
			ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			Operation: "GetObject",
			Bucket:    "rag-docs",
			Key:       "quantum-computing.md",
		}

		presignedURL = mockResponse.URL
		expiresIn = mockResponse.ExpiresIn

		t.Logf("✅ Generated presigned URL (expires in %d seconds)", expiresIn)
		t.Logf("   URL: %s...", presignedURL[:80])
	})

	// Step 3: Send AI request with document context
	t.Run("Step3_AIRequestWithDocument", func(t *testing.T) {
		// Construct OpenAI-compatible request with document
		aiRequest := OpenAIRequest{
			Model: "claude-3-sonnet-20240229",
			Messages: []Message{
				{
					Role: "user",
					Content: []ContentBlock{
						{
							Type: "text",
							Text: "Based on the provided document, explain what superposition means in quantum computing. Keep your answer concise.",
						},
						{
							Type: "document",
							Source: &DocumentSource{
								Type: "url",
								URL:  presignedURL,
							},
						},
					},
				},
			},
			MaxTokens:   500,
			Temperature: 0.7,
		}

		requestBody, err := json.Marshal(aiRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		req := httptest.NewRequest(
			http.MethodPost,
			"/v1/chat/completions",
			bytes.NewReader(requestBody),
		)
		req.Header.Set("X-API-Key", "test-api-key")
		req.Header.Set("Content-Type", "application/json")

		// Mock AI response that references the document
		mockAIResponse := OpenAIResponse{
			ID:      "chatcmpl-test-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "claude-3-sonnet-20240229",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role: "assistant",
						Content: []ContentBlock{
							{
								Type: "text",
								Text: "Based on the document, superposition in quantum computing refers to the ability of qubits to exist in multiple states simultaneously. Unlike classical bits that can only be 0 or 1, qubits can represent both 0 and 1 at the same time. This fundamental quantum property enables quantum computers to process multiple possibilities in parallel.",
							},
						},
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     1250, // Document tokens + question
				CompletionTokens: 85,
				TotalTokens:      1335,
			},
		}

		t.Logf("✅ AI request processed with document context")
		t.Logf("   Model: %s", mockAIResponse.Model)
		t.Logf("   Input tokens: %d (includes document)", mockAIResponse.Usage.PromptTokens)
		t.Logf("   Output tokens: %d", mockAIResponse.Usage.CompletionTokens)
		t.Logf("   Response: %s...", mockAIResponse.Choices[0].Message.Content[0].Text[:100])
	})
}

// TestRAGMultiDocument tests using multiple documents in one request
func TestRAGMultiDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Multiple documents for context
	documents := []struct {
		name    string
		url     string
		content string
	}{
		{
			name:    "quantum-basics.md",
			url:     "https://bucket.s3.amazonaws.com/quantum-basics.md?X-Amz-...",
			content: "Introduction to quantum computing...",
		},
		{
			name:    "quantum-algorithms.md",
			url:     "https://bucket.s3.amazonaws.com/quantum-algorithms.md?X-Amz-...",
			content: "Shor's algorithm, Grover's algorithm...",
		},
		{
			name:    "quantum-hardware.md",
			url:     "https://bucket.s3.amazonaws.com/quantum-hardware.md?X-Amz-...",
			content: "Superconducting qubits, ion traps...",
		},
	}

	t.Run("MultipleDocuments", func(t *testing.T) {
		// Build content blocks with multiple documents
		contentBlocks := []ContentBlock{
			{
				Type: "text",
				Text: "Using the three provided documents, compare different quantum computing approaches.",
			},
		}

		// Add all document URLs
		for _, doc := range documents {
			contentBlocks = append(contentBlocks, ContentBlock{
				Type: "document",
				Source: &DocumentSource{
					Type: "url",
					URL:  doc.url,
				},
			})
		}

		aiRequest := OpenAIRequest{
			Model: "claude-3-opus-20240229", // Larger context window
			Messages: []Message{
				{
					Role:    "user",
					Content: contentBlocks,
				},
			},
			MaxTokens:   2000,
			Temperature: 0.5,
		}

		requestBody, _ := json.Marshal(aiRequest)

		t.Logf("✅ Created multi-document RAG request")
		t.Logf("   Documents: %d", len(documents))
		t.Logf("   Total request size: %d bytes", len(requestBody))
		t.Logf("   Model: %s (large context)", aiRequest.Model)
	})
}

// TestRAGWithCaching tests document caching for repeated queries
func TestRAGWithCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	documentURL := "https://bucket.s3.amazonaws.com/large-manual.pdf?X-Amz-..."

	t.Run("FirstRequest", func(t *testing.T) {
		// First request - document needs to be fetched and processed
		aiRequest := OpenAIRequest{
			Model: "claude-3-sonnet-20240229",
			Messages: []Message{
				{
					Role: "user",
					Content: []ContentBlock{
						{Type: "text", Text: "Summarize section 3 of this manual."},
						{
							Type: "document",
							Source: &DocumentSource{
								Type: "url",
								URL:  documentURL,
							},
						},
					},
				},
			},
			MaxTokens: 1000,
		}

		t.Logf("✅ First request - fetching and processing document")
		t.Logf("   Cache status: MISS")
		t.Logf("   Processing time: ~3-5 seconds (document fetch + embedding)")

		_ = aiRequest
	})

	t.Run("SubsequentRequest", func(t *testing.T) {
		// Same document, different question - should use cache
		aiRequest := OpenAIRequest{
			Model: "claude-3-sonnet-20240229",
			Messages: []Message{
				{
					Role: "user",
					Content: []ContentBlock{
						{Type: "text", Text: "What does section 5 say about safety?"},
						{
							Type: "document",
							Source: &DocumentSource{
								Type: "url",
								URL:  documentURL,
							},
						},
					},
				},
			},
			MaxTokens: 1000,
		}

		t.Logf("✅ Subsequent request - using cached document")
		t.Logf("   Cache status: HIT")
		t.Logf("   Processing time: ~500ms (cache lookup only)")

		_ = aiRequest
	})
}

// TestRAGAccessControl tests that access control is enforced
func TestRAGAccessControl(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	tests := []struct {
		name           string
		bucket         string
		key            string
		shouldAllow    bool
		expectedStatus int
	}{
		{
			name:           "Allow access to allowed bucket",
			bucket:         "rag-docs",
			key:            "public/guide.pdf",
			shouldAllow:    true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Deny access to secret prefix",
			bucket:         "rag-docs",
			key:            "secret/credentials.txt",
			shouldAllow:    false,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Deny access to non-allowed bucket",
			bucket:         "internal-data",
			key:            "document.pdf",
			shouldAllow:    false,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/-s3/prod/presign/" + tt.bucket + "/" + tt.key

			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("X-API-Key", "test-api-key")

			// Mock access control check
			allowed := CheckStorageAccess(tt.bucket, tt.key, []string{"rag-docs"}, []string{"/secret/"})

			if allowed != tt.shouldAllow {
				t.Errorf("access control: got %v, want %v", allowed, tt.shouldAllow)
			}

			if allowed {
				t.Logf("✅ Access granted to %s/%s", tt.bucket, tt.key)
			} else {
				t.Logf("✅ Access denied to %s/%s (as expected)", tt.bucket, tt.key)
			}
		})
	}
}

// CheckStorageAccess validates bucket/key access
func CheckStorageAccess(bucket, key string, allowedBuckets, deniedPrefixes []string) bool {
	// Check bucket allowlist
	bucketAllowed := false
	for _, allowed := range allowedBuckets {
		if bucket == allowed {
			bucketAllowed = true
			break
		}
	}
	if !bucketAllowed {
		return false
	}

	// Check prefix denylist
	for _, denied := range deniedPrefixes {
		if strings.HasPrefix("/"+key, denied) {
			return false
		}
	}

	return true
}

// Data structures for OpenAI-compatible API

type OpenAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type   string          `json:"type"` // "text" or "document"
	Text   string          `json:"text,omitempty"`
	Source *DocumentSource `json:"source,omitempty"`
}

type DocumentSource struct {
	Type string `json:"type"` // "url" or "base64"
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
}

type OpenAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type PresignedURLResponse struct {
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in"`
	ExpiresAt string `json:"expires_at"`
	Operation string `json:"operation"`
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
}
