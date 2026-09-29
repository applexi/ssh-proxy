package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Upstream UpstreamConfig        `yaml:"server"`
	Users    map[string]UserConfig `yaml:"users"`
	LogDir   string                `yaml:"log_dir"`
}

type UpstreamConfig struct {
	// Upstream server information
	Host string `yaml:"host"`
	Port int    `yaml:"port"`

	// Upstream config information
	Username       string `yaml:"username"`
	Password       string `yaml:"password,omitempty"`
	PrivateKeyFile string `yaml:"private_key_file,omitempty"`
}

type UserConfig struct {
	Password  string `yaml:"password,omitempty"`
	PublicKey string `yaml:"public_key,omitempty"`
}

// Parses from config file path based on the [Config], [UpstreamConfig], and [UserConfig] structs
func ParseConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Validates that the proxy configuration is correct
func (c *Config) Validate() error {
	if c.Upstream.Host == "" {
		return fmt.Errorf("Upstream server host is required")
	}

	if c.Upstream.Port == 0 {
		return fmt.Errorf("Upstream server port is required")
	}

	if c.Upstream.Username == "" {
		return fmt.Errorf("Upstream server username is required")
	}

	if c.Upstream.Password == "" && c.Upstream.PrivateKeyFile == "" {
		return fmt.Errorf(
			"Upstream server must specify password or private key file",
		)
	}

	if len(c.Users) == 0 {
		return fmt.Errorf("at least one user must be configured")
	}

	for username, user := range c.Users {
		if username == "" {
			return fmt.Errorf("User username cannot be empty")
		}

		if user.PublicKey == "" && user.Password == "" {
			return fmt.Errorf("User %q must have public_key or password", username)
		}
	}

	if c.LogDir == "" {
		return fmt.Errorf("Log directory is required")
	}

	return nil
}
