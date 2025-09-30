package fuzzy

import (
    "encoding/json"
    "math"
    "os"
    "path/filepath"
)

type Config struct {
    Aliases  map[string]string `json:"aliases"`
    Commands []string          `json:"commands"`
    Fuzzy    struct {
        Enabled     bool    `json:"enabled"`
        Threshold   float64 `json:"threshold"`
        AutoExecute bool    `json:"auto_execute"`
    } `json:"fuzzy"`
}

func Load() Config {
    var cfg Config
    // Defaults
    cfg.Fuzzy.Enabled = true
    cfg.Fuzzy.Threshold = 0.8

    // Search repo root then home
    if b, err := os.ReadFile("fuzzy-commands.json"); err == nil {
        _ = json.Unmarshal(b, &cfg)
        return cfg
    }
    if h, err := os.UserHomeDir(); err == nil {
        if b, err := os.ReadFile(filepath.Join(h, ".heimdall", "fuzzy-commands.json")); err == nil {
            _ = json.Unmarshal(b, &cfg)
        }
    }
    return cfg
}

// Suggest returns best match from candidates with a normalized score [0..1].
func Suggest(input string, candidates []string) (best string, score float64) {
    best = ""
    bestScore := 0.0
    for _, c := range candidates {
        s := jaroWinkler(input, c)
        if s > bestScore {
            bestScore = s
            best = c
        }
    }
    return best, bestScore
}

// jaroWinkler similarity (simple implementation)
func jaroWinkler(s1, s2 string) float64 {
    // Convert to rune slices for unicode safety (simple)
    r1 := []rune(s1)
    r2 := []rune(s2)
    if len(r1) == 0 && len(r2) == 0 { return 1 }
    matchDistance := int(math.Floor(math.Max(float64(len(r1)), float64(len(r2)))/2)) - 1
    if matchDistance < 0 { matchDistance = 0 }

    matches1 := make([]bool, len(r1))
    matches2 := make([]bool, len(r2))

    matches := 0
    transpositions := 0

    for i := range r1 {
        start := int(math.Max(0, float64(i-matchDistance)))
        end := int(math.Min(float64(i+matchDistance+1), float64(len(r2))))
        for k := start; k < end; k++ {
            if matches2[k] { continue }
            if r1[i] != r2[k] { continue }
            matches1[i] = true
            matches2[k] = true
            matches++
            break
        }
    }

    if matches == 0 { return 0 }

    k := 0
    for i := range r1 {
        if !matches1[i] { continue }
        for k < len(r2) && !matches2[k] { k++ }
        if k < len(r2) && r1[i] != r2[k] { transpositions++ }
        k++
    }

    m := float64(matches)
    j := (m/float64(len(r1)) + m/float64(len(r2)) + (m - float64(transpositions)/2.0)/m) / 3.0

    // Winkler adjustment
    prefix := 0
    for i := 0; i < int(math.Min(4, math.Min(float64(len(r1)), float64(len(r2))))); i++ {
        if r1[i] == r2[i] { prefix++ } else { break }
    }
    jw := j + float64(prefix)*0.1*(1-j)
    if jw > 1 { jw = 1 }
    return jw
}

