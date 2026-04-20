package machineconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OneNoted/pvt/internal/config"
)

func TestResolvePath_Directory(t *testing.T) {
	source := config.ConfigSource{Type: "directory", Path: "/tmp/talos/cluster"}
	got := ResolvePath(source, "mycluster", "cp-1")
	want := "/tmp/talos/cluster/cp-1.yaml"
	if got != want {
		t.Errorf("ResolvePath() = %q, want %q", got, want)
	}
}

func TestResolvePath_Talhelper(t *testing.T) {
	source := config.ConfigSource{Type: "talhelper", Path: "/tmp/talos"}
	got := ResolvePath(source, "mycluster", "cp-1")
	want := "/tmp/talos/clusterconfig/mycluster-cp-1.yaml"
	if got != want {
		t.Errorf("ResolvePath() = %q, want %q", got, want)
	}
}

func TestLoadMachineConfig_Directory(t *testing.T) {
	dir := t.TempDir()
	content := []byte("machine:\n  type: controlplane\n")
	if err := os.WriteFile(filepath.Join(dir, "cp-1.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	source := config.ConfigSource{Type: "directory", Path: dir}
	data, err := LoadMachineConfig(source, "mycluster", "cp-1")
	if err != nil {
		t.Fatalf("LoadMachineConfig() error = %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("LoadMachineConfig() = %q, want %q", data, content)
	}
}

func TestLoadMachineConfig_Talhelper(t *testing.T) {
	dir := t.TempDir()
	clusterDir := filepath.Join(dir, "clusterconfig")
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := []byte("machine:\n  type: worker\n")
	if err := os.WriteFile(filepath.Join(clusterDir, "mycluster-worker-1.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	source := config.ConfigSource{Type: "talhelper", Path: dir}
	data, err := LoadMachineConfig(source, "mycluster", "worker-1")
	if err != nil {
		t.Fatalf("LoadMachineConfig() error = %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("LoadMachineConfig() = %q, want %q", data, content)
	}
}

func TestLoadMachineConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	source := config.ConfigSource{Type: "directory", Path: dir}

	_, err := LoadMachineConfig(source, "mycluster", "nonexistent")
	if err == nil {
		t.Fatal("LoadMachineConfig() expected error for missing file")
	}
}

func TestDiffFilesNormalizesYAML(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.yaml")
	right := filepath.Join(dir, "right.yaml")
	if err := os.WriteFile(left, []byte("machine:\n  type: worker\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("machine: {type: worker}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, different, err := DiffFiles(left, right)
	if err != nil {
		t.Fatalf("DiffFiles() error = %v", err)
	}
	if different {
		t.Fatal("DiffFiles() different = true, want false for equivalent YAML")
	}
}
