package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultProfile string                `json:"default_profile" yaml:"default_profile"`
	Profiles       map[string]SSHProfile `json:"profiles" yaml:"profiles"`
}

type SSHProfile struct {
	Host                  string `json:"host" yaml:"host"`
	Port                  int    `json:"port" yaml:"port"`
	User                  string `json:"user" yaml:"user"`
	Password              string `json:"password" yaml:"password"`
	PrivateKey            string `json:"private_key" yaml:"private_key"`
	PrivateKeyPath        string `json:"private_key_path" yaml:"private_key_path"`
	PrivateKeyPassphrase  string `json:"private_key_passphrase" yaml:"private_key_passphrase"`
	KnownHosts            string `json:"known_hosts" yaml:"known_hosts"`
	InsecureIgnoreHostKey bool   `json:"insecure_ignore_host_key" yaml:"insecure_ignore_host_key"`
}

type ConfigSource struct {
	Name   string
	Path   string
	Config *Config
}

func defaultConfigCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{".lssh.json", ".lssh.yaml", ".lssh.yml"}
	}

	return []string{
		filepath.Join(home, ".lssh.json"),
		filepath.Join(home, ".lssh.yaml"),
		filepath.Join(home, ".lssh.yml"),
		filepath.Join(home, ".config", "lssh"),
	}
}

func loadConfigSources(path string) ([]ConfigSource, error) {
	if strings.TrimSpace(path) != "" {
		return loadConfigSourcesFromPath(path)
	}

	var sources []ConfigSource
	for _, candidate := range defaultConfigCandidates() {
		loaded, err := loadConfigSourcesFromPath(candidate)
		if err != nil {
			return nil, err
		}
		sources = append(sources, loaded...)
	}

	if len(sources) == 0 {
		return nil, errors.New("no config files found; create ~/.lssh.yaml or add config files under ~/.config/lssh")
	}

	return dedupeConfigSources(sources), nil
}

func loadConfigSourcesFromPath(path string) ([]ConfigSource, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(expanded)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat config path %s: %w", expanded, err)
	}

	if info.IsDir() {
		return loadConfigSourcesFromDir(expanded)
	}

	source, err := loadConfigSource(expanded)
	if err != nil {
		return nil, err
	}
	return []ConfigSource{source}, nil
}

func loadConfigSourcesFromDir(dir string) ([]ConfigSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read config directory %s: %w", dir, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSupportedConfigFile(name) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}

	sort.Strings(paths)

	var sources []ConfigSource
	for _, path := range paths {
		source, err := loadConfigSource(path)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func loadConfigSource(path string) (ConfigSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigSource{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ConfigSource{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	if len(cfg.Profiles) == 0 {
		return ConfigSource{}, fmt.Errorf("config %s does not contain any profiles", path)
	}

	for name, profile := range cfg.Profiles {
		if profile.Host == "" {
			return ConfigSource{}, fmt.Errorf("profile %q in %s is missing host", name, path)
		}
		if profile.User == "" {
			return ConfigSource{}, fmt.Errorf("profile %q in %s is missing user", name, path)
		}
		if profile.Port == 0 {
			profile.Port = 22
			cfg.Profiles[name] = profile
		}
	}

	return ConfigSource{
		Name:   sourceDisplayName(path),
		Path:   path,
		Config: &cfg,
	}, nil
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

func selectProfileAcrossSources(sources []ConfigSource, name string) (ConfigSource, SSHProfile, string, error) {
	type match struct {
		source  ConfigSource
		profile SSHProfile
		name    string
	}

	var matches []match
	for _, source := range sources {
		profile, resolvedName, err := selectProfile(source.Config, name)
		if err != nil {
			continue
		}
		matches = append(matches, match{
			source:  source,
			profile: profile,
			name:    resolvedName,
		})
	}

	switch len(matches) {
	case 0:
		return ConfigSource{}, SSHProfile{}, "", fmt.Errorf("profile %q not found", name)
	case 1:
		return matches[0].source, matches[0].profile, matches[0].name, nil
	default:
		var locations []string
		for _, m := range matches {
			locations = append(locations, fmt.Sprintf("%s (%s)", m.name, m.source.Path))
		}
		sort.Strings(locations)
		return ConfigSource{}, SSHProfile{}, "", fmt.Errorf("profile %q is defined in multiple config files: %s", name, strings.Join(locations, ", "))
	}
}

func sortedProfileNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isSupportedConfigFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".json" || ext == ".yaml" || ext == ".yml"
}

func sourceDisplayName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func dedupeConfigSources(sources []ConfigSource) []ConfigSource {
	seen := make(map[string]struct{}, len(sources))
	deduped := make([]ConfigSource, 0, len(sources))
	for _, source := range sources {
		if _, ok := seen[source.Path]; ok {
			continue
		}
		seen[source.Path] = struct{}{}
		deduped = append(deduped, source)
	}
	return deduped
}
