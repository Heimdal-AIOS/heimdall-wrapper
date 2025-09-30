package manifest

import (
    "bufio"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

// Manifest is a minimal subset parsed from YAML.
// Only top-level fields name, cmd, args, env are supported in MVP.
type Manifest struct {
    Name string
    Cmd  string
    Args []string
    Env  map[string]string
}

// Load reads a minimal YAML manifest.
func Load(path string) (Manifest, error) {
    f, err := os.Open(path)
    if err != nil {
        return Manifest{}, err
    }
    defer f.Close()

    m := Manifest{Env: map[string]string{}}
    s := bufio.NewScanner(f)
    inEnv := false
    for s.Scan() {
        line := strings.TrimRight(s.Text(), "\r\n")
        if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
            continue
        }
        if !strings.HasPrefix(line, " ") && strings.HasSuffix(line, ":") {
            // entering a section
            key := strings.TrimSuffix(strings.TrimSpace(line), ":")
            if key == "env" {
                inEnv = true
            } else {
                inEnv = false
            }
            continue
        }
        if !strings.HasPrefix(line, " ") {
            inEnv = false
            // root-level key: value
            k, v, ok := splitKV(line)
            if !ok { continue }
            switch k {
            case "name":
                m.Name = v
            case "cmd":
                m.Cmd = v
            case "args":
                // attempt to parse YAML-ish list: ["a", "b"] or [a, b]
                m.Args = parseList(v)
            }
            continue
        }
        if inEnv {
            // env entries are indented
            k, v, ok := splitKV(strings.TrimSpace(line))
            if ok {
                if m.Env == nil { m.Env = map[string]string{} }
                m.Env[k] = v
            }
        }
    }
    if err := s.Err(); err != nil {
        return Manifest{}, err
    }
    if m.Name == "" {
        // Default to filename
        base := filepath.Base(path)
        m.Name = strings.TrimSuffix(base, filepath.Ext(base))
    }
    if m.Cmd == "" {
        m.Cmd = m.Name
    }
    return m, nil
}

func splitKV(line string) (string, string, bool) {
    i := strings.Index(line, ":")
    if i < 0 { return "", "", false }
    k := strings.TrimSpace(line[:i])
    v := strings.TrimSpace(line[i+1:])
    v = trimQuotes(v)
    return k, v, true
}

var listRe = regexp.MustCompile(`^\[(.*)\]$`)

func parseList(s string) []string {
    s = strings.TrimSpace(s)
    m := listRe.FindStringSubmatch(s)
    if len(m) != 2 {
        s = trimQuotes(s)
        if s == "" { return nil }
        return strings.Fields(s)
    }
    inner := m[1]
    if strings.TrimSpace(inner) == "" { return nil }
    // split on commas not considering nested quotes heavily; then trim
    parts := strings.Split(inner, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        q := strings.TrimSpace(trimQuotes(p))
        if q != "" { out = append(out, q) }
    }
    return out
}

func trimQuotes(s string) string {
    s = strings.TrimSpace(s)
    if len(s) >= 2 {
        if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
            return s[1:len(s)-1]
        }
    }
    return s
}

// Save writes a minimal YAML manifest with fields we support.
func Save(path string, m Manifest) error {
    if m.Name == "" || m.Cmd == "" {
        return errors.New("manifest requires name and cmd")
    }
    var b strings.Builder
    fmt.Fprintf(&b, "name: %s\n", m.Name)
    fmt.Fprintf(&b, "cmd: %s\n", m.Cmd)
    if len(m.Args) > 0 {
        // write as YAML list on one line
        fmt.Fprintf(&b, "args: [")
        for i, a := range m.Args {
            if i > 0 { b.WriteString(", ") }
            fmt.Fprintf(&b, "\"%s\"", escapeQuote(a))
        }
        b.WriteString("]\n")
    } else {
        fmt.Fprintf(&b, "args: []\n")
    }
    if len(m.Env) > 0 {
        b.WriteString("env:\n")
        for k, v := range m.Env {
            fmt.Fprintf(&b, "  %s: %s\n", k, v)
        }
    } else {
        b.WriteString("env: {}\n")
    }
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    return os.WriteFile(path, []byte(b.String()), 0o644)
}

func escapeQuote(s string) string {
    return strings.ReplaceAll(s, "\"", "\\\"")
}

