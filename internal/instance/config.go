// Copyright 2025 Bedrock Proxy Authors
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the provider instances configuration
type Config struct {
	Global    GlobalConfig               `yaml:"global"`
	Instances map[string]InstanceConfig  `yaml:"instances"`
	Routing   RoutingConfig              `yaml:"routing"`
	Features  map[string]FeatureConfig   `yaml:"features"`
}

// GlobalConfig represents global settings
type GlobalConfig struct {
	Metrics struct {
		Enabled             bool `yaml:"enabled"`
		CaptureRequestBody  bool `yaml:"capture_request_body"`
		CaptureResponseBody bool `yaml:"capture_response_body"`
	} `yaml:"metrics"`
	DefaultTimeout   string                 `yaml:"default_timeout"`
	Authentication   map[string]interface{} `yaml:"authentication"`
}

// InstanceConfig represents a provider instance configuration
type InstanceConfig struct {
	Type           string                 `yaml:"type"`
	Mode           string                 `yaml:"mode"` // transparent or protocol
	Protocol       string                 `yaml:"protocol,omitempty"` // openai, anthropic, etc.
	Description    string                 `yaml:"description"`
	Region         string                 `yaml:"region,omitempty"`
	Endpoint       string                 `yaml:"endpoint,omitempty"`
	BaseURL        string                 `yaml:"base_url,omitempty"`
	ProjectID      string                 `yaml:"project_id,omitempty"`
	Location       string                 `yaml:"location,omitempty"`
	APIVersion     string                 `yaml:"api_version,omitempty"`
	CompartmentID  string                 `yaml:"compartment_id,omitempty"`
	Authentication AuthenticationConfig   `yaml:"authentication"`
	Transformation *TransformationConfig  `yaml:"transformation,omitempty"`
	Endpoints      []EndpointConfig       `yaml:"endpoints"`
	Metrics        MetricsConfig          `yaml:"metrics"`
}

// AuthenticationConfig represents authentication configuration
type AuthenticationConfig struct {
	Type    string `yaml:"type"` // aws_sigv4, api_key, bearer_token, gcp_oauth2
	Service string `yaml:"service,omitempty"` // For AWS
	Region  string `yaml:"region,omitempty"` // For AWS
	Header  string `yaml:"header,omitempty"` // For API key
	Key     string `yaml:"key,omitempty"`
	Token   string `yaml:"token,omitempty"`
}

// TransformationConfig represents transformation configuration
type TransformationConfig struct {
	RequestFrom  string                 `yaml:"request_from"`
	RequestTo    string                 `yaml:"request_to"`
	ResponseFrom string                 `yaml:"response_from"`
	ResponseTo   string                 `yaml:"response_to"`
	Options      map[string]interface{} `yaml:"options,omitempty"`
}

// EndpointConfig represents an endpoint configuration
type EndpointConfig struct {
	Path    string   `yaml:"path"`
	Methods []string `yaml:"methods"`
}

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	Enabled bool              `yaml:"enabled"`
	Labels  map[string]string `yaml:"labels,omitempty"`
}

// RoutingConfig represents routing configuration
type RoutingConfig struct {
	Defaults   map[string]string `yaml:"defaults"`
	PathBased  struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"path_based"`
	Fallback struct {
		Enabled    bool `yaml:"enabled"`
		UseDefault bool `yaml:"use_default"`
	} `yaml:"fallback"`
}

// FeatureConfig represents a feature flag
type FeatureConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description"`
}

// LoadConfig loads provider instances configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// GetInstanceByPath returns the instance configuration for a given request path
func (c *Config) GetInstanceByPath(path string) (*InstanceConfig, string, error) {
	for name, instance := range c.Instances {
		for _, endpoint := range instance.Endpoints {
			if strings.HasPrefix(path, endpoint.Path) {
				return &instance, name, nil
			}
		}
	}

	return nil, "", fmt.Errorf("no instance found for path: %s", path)
}

// GetInstanceByName returns the instance configuration by name
func (c *Config) GetInstanceByName(name string) (*InstanceConfig, error) {
	instance, ok := c.Instances[name]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", name)
	}
	return &instance, nil
}

// GetDefaultInstance returns the default instance for a provider type
func (c *Config) GetDefaultInstance(providerType string) (*InstanceConfig, string, error) {
	defaultName, ok := c.Routing.Defaults[providerType]
	if !ok {
		return nil, "", fmt.Errorf("no default instance for provider type: %s", providerType)
	}

	instance, err := c.GetInstanceByName(defaultName)
	if err != nil {
		return nil, "", err
	}

	return instance, defaultName, nil
}

// ListInstances returns all instance names
func (c *Config) ListInstances() []string {
	names := make([]string, 0, len(c.Instances))
	for name := range c.Instances {
		names = append(names, name)
	}
	return names
}

// ListInstancesByMode returns all instances of a specific mode
func (c *Config) ListInstancesByMode(mode string) map[string]InstanceConfig {
	instances := make(map[string]InstanceConfig)
	for name, instance := range c.Instances {
		if instance.Mode == mode {
			instances[name] = instance
		}
	}
	return instances
}

// ListInstancesByType returns all instances of a specific provider type
func (c *Config) ListInstancesByType(providerType string) map[string]InstanceConfig {
	instances := make(map[string]InstanceConfig)
	for name, instance := range c.Instances {
		if instance.Type == providerType {
			instances[name] = instance
		}
	}
	return instances
}

// IsFeatureEnabled checks if a feature is enabled
func (c *Config) IsFeatureEnabled(featureName string) bool {
	feature, ok := c.Features[featureName]
	if !ok {
		return false
	}
	return feature.Enabled
}
