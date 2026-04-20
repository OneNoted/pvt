package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfigFilePrefersExplicitEnv(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	explicit := filepath.Join(tmp, "explicit.yaml")

	if err := os.MkdirAll(filepath.Join(home, ".config", "pvt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicit, []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "pvt", "config.yaml"), []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PVT_CONFIG", explicit)

	got := discoverConfigFile()
	if got != explicit {
		t.Fatalf("expected explicit config %q, got %q", explicit, got)
	}
}

func TestDiscoverConfigFilePrefersLocalRepoConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(tmp, "repo")

	if err := os.MkdirAll(filepath.Join(home, ".config", "pvt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "pvt", "config.yaml"), []byte("home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pvt.yaml"), []byte("repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PVT_CONFIG", "")

	got := discoverConfigFile()
	want := filepath.Join(repo, "pvt.yaml")
	if got != want {
		t.Fatalf("expected local config %q, got %q", want, got)
	}
}

func TestDiscoverConfigFilePrefersHomeConfigYamlOverLegacyPvtYaml(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")

	if err := os.MkdirAll(filepath.Join(home, ".config", "pvt"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".config", "pvt", "config.yaml")
	legacyPath := filepath.Join(home, ".config", "pvt", "pvt.yaml")
	if err := os.WriteFile(configPath, []byte("config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PVT_CONFIG", "")

	got := discoverConfigFile()
	if got != configPath {
		t.Fatalf("expected home config %q, got %q", configPath, got)
	}
}
