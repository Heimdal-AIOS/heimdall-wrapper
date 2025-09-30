package wiki

import (
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

type Page struct {
    Title   string   `json:"title"`
    Tags    []string `json:"tags,omitempty"`
    Content string   `json:"content"`
}

type DB struct {
    Pages []Page `json:"pages"`
}

// Locate looks for wiki.json starting from workdir.
func Locate(workdir string) (string, error) {
    // Prefer repo local wiki.json
    p := filepath.Join(workdir, "wiki.json")
    if _, err := os.Stat(p); err == nil {
        return p, nil
    }
    // Fallback: ~/.heimdall/wiki.json
    if h, err := os.UserHomeDir(); err == nil {
        hp := filepath.Join(h, ".heimdall", "wiki.json")
        if _, err := os.Stat(hp); err == nil {
            return hp, nil
        }
        return hp, nil // path to create
    }
    return p, nil
}

func Load(path string) (DB, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return DB{}, err
    }
    var db DB
    if err := json.Unmarshal(b, &db); err != nil {
        return DB{}, err
    }
    return db, nil
}

func Save(path string, db DB) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
    b, err := json.MarshalIndent(db, "", "  ")
    if err != nil { return err }
    return os.WriteFile(path, b, 0o644)
}

func Init(path string) error {
    if _, err := os.Stat(path); err == nil {
        return errors.New("wiki.json already exists")
    }
    sample := DB{Pages: []Page{
        {Title: "Welcome", Tags: []string{"intro"}, Content: "This is your Heimdal wiki. Add pages in wiki.json."},
        {Title: "Claude-Code", Tags: []string{"ai","claude"}, Content: "How to run Claude via Heimdal."},
        {Title: "Gemini-Cli", Tags: []string{"ai","gemini"}, Content: "How to run Gemini via Heimdal."},
    }}
    return Save(path, sample)
}

type Result struct {
    Title   string
    Snippet string
    Score   int
}

// Search performs a simple case-insensitive containment match over title, tags, content.
func Search(db DB, query string, limit int) []Result {
    q := strings.ToLower(strings.TrimSpace(query))
    if q == "" { return nil }
    res := make([]Result, 0)
    for _, p := range db.Pages {
        t := strings.ToLower(p.Title)
        c := strings.ToLower(p.Content)
        score := 0
        if strings.Contains(t, q) { score += 5 }
        for _, tg := range p.Tags { if strings.Contains(strings.ToLower(tg), q) { score += 2 } }
        if strings.Contains(c, q) { score += 1 }
        if score == 0 { continue }
        snippet := makeSnippet(p.Content, q, 160)
        res = append(res, Result{Title: p.Title, Snippet: snippet, Score: score})
    }
    sort.Slice(res, func(i, j int) bool {
        if res[i].Score == res[j].Score { return res[i].Title < res[j].Title }
        return res[i].Score > res[j].Score
    })
    if limit > 0 && len(res) > limit { return res[:limit] }
    return res
}

func Show(db DB, title string) (Page, bool) {
    for _, p := range db.Pages {
        if strings.EqualFold(p.Title, title) { return p, true }
    }
    return Page{}, false
}

func makeSnippet(content, q string, span int) string {
    if span < 40 { span = 40 }
    lc := strings.ToLower(content)
    lq := strings.ToLower(q)
    idx := strings.Index(lc, lq)
    if idx < 0 {
        if len(content) <= span { return content }
        return content[:span] + "…"
    }
    start := idx - span/2
    if start < 0 { start = 0 }
    end := start + span
    if end > len(content) { end = len(content) }
    prefix := ""
    suffix := ""
    if start > 0 { prefix = "…" }
    if end < len(content) { suffix = "…" }
    return prefix + content[start:end] + suffix
}
