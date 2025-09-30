package universe

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// Session holds info about a Heimdal universe session.
type Session struct {
    ID         string
    Dir        string // root dir for session data
    ContextDir string // where context files live
}

// StartSession creates a session directory and basic context files.
// If home is available, uses $HOME/.heimdall/sessions; otherwise uses CWD.
func StartSession(workdir string) (Session, error) {
    sid := newID()
    base := ""
    if h, err := os.UserHomeDir(); err == nil {
        base = filepath.Join(h, ".heimdall", "sessions")
    } else {
        base = filepath.Join(workdir, ".heimdall-sessions")
    }
    root := filepath.Join(base, sid)
    ctxDir := filepath.Join(root, "context")
    if err := os.MkdirAll(ctxDir, 0o755); err != nil {
        return Session{}, err
    }
    // Write minimal context
    if err := writeFile(filepath.Join(ctxDir, "system.md"), systemBanner()); err != nil {
        return Session{}, err
    }
    // Repo file index (limited)
    _ = writeRepoIndex(ctxDir, workdir)
    // Docs index
    _ = writeDocsIndex(ctxDir, workdir)
    return Session{ID: sid, Dir: root, ContextDir: ctxDir}, nil
}

func newID() string {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err != nil {
        return fmt.Sprintf("%d", time.Now().UnixNano())
    }
    return hex.EncodeToString(b)
}

func writeFile(path, content string) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
    return os.WriteFile(path, []byte(content), 0o644)
}

func systemBanner() string {
    return "# Heimdal Universe\n\nThis session runs inside the Heimdal OS wrapper.\n\n" + time.Now().Format(time.RFC3339) + "\n"
}

func writeRepoIndex(ctxDir, workdir string) error {
    var files []string
    // Walk but limit count and size
    max := 500
    _ = filepath.WalkDir(workdir, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return nil }
        // skip hidden heavy folders
        if d.IsDir() {
            base := filepath.Base(path)
            switch base {
            case ".git", "node_modules", "venv", ".venv", "target", "bin", "build" :
                if path != workdir { return filepath.SkipDir }
            }
            return nil
        }
        rel, _ := filepath.Rel(workdir, path)
        if strings.HasPrefix(rel, ".heimdall") { return nil }
        files = append(files, rel)
        if len(files) >= max { return fmt.Errorf("limit") }
        return nil
    })
    // ignore limit error
    content := "# Repo files (truncated)\n" + strings.Join(files, "\n") + "\n"
    return writeFile(filepath.Join(ctxDir, "repo_files.txt"), content)
}

func writeDocsIndex(ctxDir, workdir string) error {
    docs := filepath.Join(workdir, "docs")
    entries, err := os.ReadDir(docs)
    if err != nil { return nil }
    var lines []string
    lines = append(lines, "# Docs files")
    for _, e := range entries {
        lines = append(lines, e.Name())
    }
    return writeFile(filepath.Join(ctxDir, "docs_files.txt"), strings.Join(lines, "\n")+"\n")
}

