package qdrant

import (
	"fmt"
	"time"

	"digital.vasic.vectordb/pkg/client"
)

// Config holds Qdrant connection configuration.
type Config struct {
	Host     string        `json:"host" yaml:"host"`
	HTTPPort int           `json:"http_port" yaml:"http_port"`
	GRPCPort int           `json:"grpc_port" yaml:"grpc_port"`
	APIKey   string        `json:"api_key" yaml:"api_key"`
	TLS      bool          `json:"tls" yaml:"tls"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`

	// Collection defaults
	DefaultDistance client.DistanceMetric `json:"default_distance" yaml:"default_distance"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		HTTPPort:        6333,
		GRPCPort:        6334,
		TLS:             false,
		Timeout:         30 * time.Second,
		DefaultDistance: client.DistanceCosine,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("http_port must be between 1 and 65535")
	}
	if c.GRPCPort <= 0 || c.GRPCPort > 65535 {
		return fmt.Errorf("grpc_port must be between 1 and 65535")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// GetHTTPURL returns the HTTP API URL.
func (c *Config) GetHTTPURL() string {
	scheme := "http"
	if c.TLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.Host, c.HTTPPort)
}

// GetGRPCAddress returns the gRPC address.
func (c *Config) GetGRPCAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.GRPCPort)
}
