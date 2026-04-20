package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DefaultProfile string                `yaml:"default_profile"`
	Profiles       map[string]SSHProfile `yaml:"profiles"`
}

type SSHProfile struct {
	Host                  string `yaml:"host"`
	Port                  int    `yaml:"port"`
	User                  string `yaml:"user"`
	Password              string `yaml:"password"`
	KnownHosts            string `yaml:"known_hosts"`
	InsecureIgnoreHostKey bool   `yaml:"insecure_ignore_host_key"`
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".lssh.json"
	}
	return filepath.Join(home, ".lssh.json")
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if len(cfg.Profiles) == 0 {
		return nil, errors.New("config does not contain any profiles")
	}

	for name, profile := range cfg.Profiles {
		if profile.Host == "" {
			return nil, fmt.Errorf("profile %q is missing host", name)
		}
		if profile.User == "" {
			return nil, fmt.Errorf("profile %q is missing user", name)
		}
		if profile.Port == 0 {
			profile.Port = 22
			cfg.Profiles[name] = profile
		}
	}

	return &cfg, nil
}

func selectProfile(cfg *Config, name string) (SSHProfile, string, error) {
	if name == "" {
		name = cfg.DefaultProfile
	}
	if name == "" {
		if len(cfg.Profiles) == 1 {
			for onlyName, profile := range cfg.Profiles {
				return profile, onlyName, nil
			}
		}
		return SSHProfile{}, "", errors.New("profile name is required when default_profile is not set")
	}

	profile, ok := cfg.Profiles[name]
	if !ok {
		return SSHProfile{}, "", fmt.Errorf("profile %q not found", name)
	}
	return profile, name, nil
}
