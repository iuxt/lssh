package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigSourcesFromDir(t *testing.T) {
	dir := t.TempDir()

	configA := `default_profile: dev
profiles:
  dev:
    host: 127.0.0.1
    user: root
`
	configB := `profiles:
  prod:
    host: 10.0.0.10
    user: deploy
    port: 2222
`

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(configA), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(configB), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	sources, err := loadConfigSources(dir)
	if err != nil {
		t.Fatalf("loadConfigSources returned error: %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("expected 2 config sources, got %d", len(sources))
	}

	if sources[0].Name != "a" || sources[1].Name != "b" {
		t.Fatalf("unexpected source names: %#v", []string{sources[0].Name, sources[1].Name})
	}

	if got := sources[0].Config.Profiles["dev"].Port; got != 22 {
		t.Fatalf("expected default port 22, got %d", got)
	}
}

func TestSelectProfileAcrossSources(t *testing.T) {
	sources := []ConfigSource{
		{
			Name: "alpha",
			Path: "/tmp/alpha.yaml",
			Config: &Config{
				Profiles: map[string]SSHProfile{
					"dev": {Host: "127.0.0.1", User: "root", Port: 22},
				},
			},
		},
		{
			Name: "beta",
			Path: "/tmp/beta.yaml",
			Config: &Config{
				Profiles: map[string]SSHProfile{
					"prod": {Host: "10.0.0.10", User: "deploy", Port: 22},
				},
			},
		},
	}

	source, profile, name, err := selectProfileAcrossSources(sources, "prod")
	if err != nil {
		t.Fatalf("selectProfileAcrossSources returned error: %v", err)
	}
	if name != "prod" || source.Name != "beta" || profile.User != "deploy" {
		t.Fatalf("unexpected selection: source=%s name=%s user=%s", source.Name, name, profile.User)
	}
}
