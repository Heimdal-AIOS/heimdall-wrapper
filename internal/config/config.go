package config

import (
    "os"
    "path/filepath"
)

// EnsureAppsDir ensures the apps directory exists in the repository at ./apps
// or, if not present, under the user's home as ~/.heimdall/apps.
func EnsureAppsDir() (string, error) {
    // Prefer repo-local ./apps if writable
    cwd, _ := os.Getwd()
    local := filepath.Join(cwd, "apps")
    if err := os.MkdirAll(local, 0o755); err == nil {
        return local, nil
    }
    // Fallback to $HOME/.heimdall/apps
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    dir := filepath.Join(home, ".heimdall", "apps")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return "", err
    }
    return dir, nil
}

