package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	const existingName = "TEST_AGENT_EXISTING"
	const loadedName = "TEST_AGENT_LOADED"
	t.Setenv(existingName, "from-shell")
	os.Unsetenv(loadedName)
	t.Cleanup(func() { os.Unsetenv(loadedName) })

	path := filepath.Join(t.TempDir(), ".env")
	data := []byte("# comment\n" + existingName + "=from-file\n" + loadedName + "=\"from-file\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(existingName); got != "from-file" {
		t.Fatalf("existing value was not overwritten: %q", got)
	}
	if got := os.Getenv(loadedName); got != "from-file" {
		t.Fatalf("loaded value = %q", got)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	if err := loadEnvFile(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
}
