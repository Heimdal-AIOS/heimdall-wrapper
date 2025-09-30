package main

import (
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "runtime"
    "io/fs"
    "os/signal"

    "heimdal/internal/config"
    "heimdal/internal/manifest"
    "heimdal/internal/universe"
    wikimod "heimdal/internal/wiki"
)

func main() {
    if err := run(os.Args); err != nil {
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }
}

func run(argv []string) error {
    if len(argv) == 0 {
        return errors.New("no argv")
    }
    // Alias behavior: if invoked as `heimdall`, behave the same.
    // Commands:
    //  - shell
    //  - run <app> [args...]
    //  - app add <name> --cmd <cmd> [--args "..."]
    //  - app ls | app rm <name>
    //  - shorthand: heimdal <app> [args...]

    prog := filepath.Base(argv[0])
    args := argv[1:]

    // global flags (very minimal): --profile=permissive|restricted, --prompt-prefix=...
    profile := "permissive"
    promptPrefix := "[hd] "
    filtered := make([]string, 0, len(args))
    for i := 0; i < len(args); i++ {
        a := args[i]
        if strings.HasPrefix(a, "--profile=") {
            profile = strings.TrimPrefix(a, "--profile=")
            continue
        }
        if strings.HasPrefix(a, "--prompt-prefix=") {
            promptPrefix = strings.TrimPrefix(a, "--prompt-prefix=")
            continue
        }
        filtered = append(filtered, a)
    }
    args = filtered

    if len(args) == 0 {
        // No args: print help
        usage(prog)
        return nil
    }

    switch args[0] {
    case "help", "-h", "--help":
        usage(prog)
        return nil
    case "shell":
        return cmdShell(promptPrefix)
    case "run":
        if len(args) < 2 {
            return errors.New("usage: heimdal run <app> [args...]")
        }
        app := args[1]
        rest := args[2:]
        return cmdRun(app, rest, profile)
    case "app":
        return cmdApp(args[1:])
    case "log":
        return cmdLog(args[1:])
    case "wiki":
        return cmdWiki(args[1:])
    case "aioswiki": // alias for wiki
        return cmdWiki(args[1:])
    default:
        // shorthand: heimdal <app> [args...]
        app := args[0]
        rest := args[1:]
        return cmdRun(app, rest, profile)
    }
}

func usage(prog string) {
    fmt.Printf(`Heimdal — OS Wrapper CLI (MVP)

Usage:
  %s shell
  %s run <app> [args...]
  %s app add <name> --cmd <cmd> [--args "--foo --bar"]
  %s app ls
  %s app rm <name>
  %s aioswiki search <query>
  %s aioswiki show <title>
  %s aioswiki init
  %s wiki search <query>
  %s wiki show <title>
  %s wiki init
  %s [--profile=permissive|restricted] [--prompt-prefix="[hd] "] <app> [args...]  (shorthand)

Env/Config:
  Apps manifests in apps/<name>.yaml. Minimal YAML supported: name, cmd, args, env.

`, prog, prog, prog, prog, prog, prog, prog, prog, prog)
}

func cmdShell(prefix string) error {
    sh := os.Getenv("SHELL")
    if sh == "" {
        sh = "/bin/sh"
    }
    base := filepath.Base(sh)

    // Common env
    env := map[string]string{}
    for _, kv := range os.Environ() {
        if i := strings.IndexByte(kv, '='); i >= 0 {
            env[kv[:i]] = kv[i+1:]
        }
    }
    env["HEIMDAL"] = "1"
    env["HEIMDAL_PREFIX"] = prefix
    // Pass the absolute path to this heimdal binary for shell functions
    if exe, err := os.Executable(); err == nil {
        env["HEIMDAL_BIN"] = exe
    }

    var cmd *exec.Cmd
    cleanup := func() {}

    switch base {
    case "zsh":
        tmpDir, err := os.MkdirTemp("", "heimdal-zsh-rc-*")
        if err != nil { return err }
        // Create a shim .zshrc that sources user rc, then ensures prefix via precmd hook.
        shim := `# Heimdal zsh shim
emulate -L zsh
export HEIMDAL=1
export HEIMDAL_PREFIX=${HEIMDAL_PREFIX:-"[hd] "}
export HEIMDAL_BIN=${HEIMDAL_BIN:-""}
if [[ -f "$HOME/.zshrc" ]]; then
  source "$HOME/.zshrc"
fi
# Ensure HEIMDAL_BIN is set (fallback to PATH lookup)
if [[ -z "$HEIMDAL_BIN" ]]; then
  HEIMDAL_BIN=$(command -v heimdal 2>/dev/null)
fi
# Built-in wiki functions
function aioswiki() { command "$HEIMDAL_BIN" wiki "$@" }
function wiki() { aioswiki "$@" }
function _heimdal_prompt_prefix() {
  local p="${HEIMDAL_PREFIX}"
  if [[ -n "$p" ]] && [[ "${PROMPT}" != ${p}* ]]; then
    PROMPT="${p}${PROMPT}"
  fi
}
precmd_functions+=(_heimdal_prompt_prefix)
`
        if err := os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(shim), fs.FileMode(0644)); err != nil {
            return err
        }
        env["ZDOTDIR"] = tmpDir
        cmd = exec.Command(sh, "-i")
        // cleanup on exit
        cleanup = func() { os.RemoveAll(tmpDir) }
    case "bash":
        tmpFile, err := os.CreateTemp("", "heimdal-bash-rc-*.sh")
        if err != nil { return err }
        tmpPath := tmpFile.Name()
        _ = tmpFile.Close()
        shim := `# Heimdal bash shim
export HEIMDAL=1
export HEIMDAL_PREFIX=${HEIMDAL_PREFIX:-"[hd] "}
export HEIMDAL_BIN=${HEIMDAL_BIN:-""}
if [ -f "$HOME/.bashrc" ]; then
  . "$HOME/.bashrc"
fi
# Ensure HEIMDAL_BIN is set (fallback to PATH lookup)
if [ -z "$HEIMDAL_BIN" ]; then
  HEIMDAL_BIN=$(command -v heimdal 2>/dev/null)
fi
# Built-in wiki functions
aioswiki() { command "$HEIMDAL_BIN" wiki "$@"; }
wiki() { aioswiki "$@"; }
__heimdal_ps1() {
  case "$PS1" in
    ${HEIMDAL_PREFIX}*) ;;
    *) PS1="${HEIMDAL_PREFIX}${PS1}";;
  esac
}
PROMPT_COMMAND="__heimdal_ps1; ${PROMPT_COMMAND}"
`
        if err := os.WriteFile(tmpPath, []byte(shim), fs.FileMode(0644)); err != nil { return err }
        cmd = exec.Command(sh, "--rcfile", tmpPath, "-i")
        cleanup = func() { os.Remove(tmpPath) }
    default:
        // Fallback: try to set PS1/PROMPT via env; may be overridden by user rc.
        if _, ok := env["PS1"]; !ok {
            env["PS1"] = prefix + "$ "
        } else {
            env["PS1"] = prefix + env["PS1"]
        }
        env["PROMPT"] = env["PS1"]
        cmd = exec.Command(sh)
    }

    // Wire stdio
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    // Rebuild env list
    envList := make([]string, 0, len(env))
    for k, v := range env {
        envList = append(envList, k+"="+v)
    }
    cmd.Env = envList

    // Ensure cleanup on Ctrl-C and normal exit
    defer cleanup()
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    go func() {
        <-c
        cleanup()
        if cmd.Process != nil && runtime.GOOS != "windows" {
            _ = cmd.Process.Signal(os.Interrupt)
        }
    }()

    return cmd.Run()
}

func cmdRun(app string, rest []string, profile string) error {
    // Create a Heimdal universe session and context
    cwd, _ := os.Getwd()
    sess, err := universe.StartSession(cwd)
    if err != nil { return err }

    appsDir, err := config.EnsureAppsDir()
    if err != nil {
        return err
    }
    maniPath := filepath.Join(appsDir, app+".yaml")
    var m manifest.Manifest
    if _, err := os.Stat(maniPath); err == nil {
        m, err = manifest.Load(maniPath)
        if err != nil {
            return fmt.Errorf("load manifest: %w", err)
        }
    } else {
        // Fallback: treat name as command directly
        m = manifest.Manifest{Name: app, Cmd: app}
    }

    // Build command and args
    cmdName := m.Cmd
    cmdArgs := append([]string{}, m.Args...)
    cmdArgs = append(cmdArgs, rest...)

    // Env: start from host env, then overlay Heimdal universe env, then manifest env
    envMap := map[string]string{}
    for _, kv := range os.Environ() {
        if i := strings.IndexByte(kv, '='); i >= 0 {
            envMap[kv[:i]] = kv[i+1:]
        }
    }
    envMap["HEIMDAL"] = "1"
    envMap["HEIMDAL_UNIVERSE"] = "1"
    envMap["HEIMDAL_SESSION"] = sess.ID
    envMap["HEIMDAL_CONTEXT_DIR"] = sess.ContextDir
    envMap["HEIMDAL_WORKDIR"] = cwd
    for k, v := range m.Env {
        envMap[k] = os.ExpandEnv(v)
    }
    envList := make([]string, 0, len(envMap))
    for k, v := range envMap {
        envList = append(envList, k+"="+v)
    }

    fmt.Fprintf(os.Stderr, "[heimdal] running app=%s cmd=%s profile=%s\n", app, cmdName, profile)

    cmd := exec.Command(cmdName, cmdArgs...)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = envList

    return cmd.Run()
}

func cmdApp(args []string) error {
    if len(args) == 0 {
        return errors.New("usage: heimdal app [add|ls|rm] ...")
    }
    sub := args[0]
    switch sub {
    case "ls":
        appsDir, err := config.EnsureAppsDir()
        if err != nil { return err }
        entries, err := os.ReadDir(appsDir)
        if err != nil { return err }
        for _, e := range entries {
            if e.IsDir() { continue }
            name := e.Name()
            if strings.HasSuffix(name, ".yaml") {
                fmt.Println(strings.TrimSuffix(name, ".yaml"))
            }
        }
        return nil
    case "rm":
        if len(args) < 2 { return errors.New("usage: heimdal app rm <name>") }
        name := args[1]
        appsDir, err := config.EnsureAppsDir()
        if err != nil { return err }
        path := filepath.Join(appsDir, name+".yaml")
        return os.Remove(path)
    case "add":
        // heimdal app add <name> --cmd <cmd> [--args "..."]
        if len(args) < 2 { return errors.New("usage: heimdal app add <name> --cmd <cmd> [--args \"...\"]") }
        name := args[1]
        var cmdVal string
        var argsVal string
        for i := 2; i < len(args); i++ {
            a := args[i]
            if a == "--cmd" && i+1 < len(args) {
                cmdVal = args[i+1]
                i++
                continue
            }
            if a == "--args" && i+1 < len(args) {
                argsVal = args[i+1]
                i++
                continue
            }
        }
        if cmdVal == "" { return errors.New("--cmd is required") }
        appsDir, err := config.EnsureAppsDir()
        if err != nil { return err }
        m := manifest.Manifest{
            Name: name,
            Cmd:  cmdVal,
            Args: splitArgs(argsVal),
            Env:  map[string]string{},
        }
        path := filepath.Join(appsDir, name+".yaml")
        if err := manifest.Save(path, m); err != nil { return err }
        fmt.Println("added:", path)
        return nil
    default:
        return errors.New("usage: heimdal app [add|ls|rm] ...")
    }
}

func cmdLog(args []string) error {
    if len(args) == 0 || args[0] == "tail" {
        // Placeholder: print note for now.
        fmt.Println("log tail: not implemented in MVP. Future: stream session logs.")
        return nil
    }
    return errors.New("usage: heimdal log tail")
}

func cmdWiki(args []string) error {
    if len(args) == 0 { return errors.New("usage: heimdal wiki [search|show|init] ...") }
    sub := args[0]
    cwd, _ := os.Getwd()
    path, err := wikimod.Locate(cwd)
    if err != nil { return err }
    switch sub {
    case "init":
        if err := wikimod.Init(path); err != nil { return err }
        fmt.Println("initialized wiki at:", path)
        return nil
    case "search":
        if len(args) < 2 { return errors.New("usage: heimdal wiki search <query>") }
        return wikiSearch(path, strings.Join(args[1:], " "))
    case "show":
        if len(args) < 2 { return errors.New("usage: heimdal wiki show <title>") }
        return wikiShow(path, strings.Join(args[1:], " "))
    default:
        return errors.New("usage: heimdal wiki [search|show|init] ...")
    }
}

func wikiSearch(path, query string) error {
    db, err := wikimod.Load(path)
    if err != nil { return err }
    results := wikimod.Search(db, query, 10)
    if len(results) == 0 {
        fmt.Println("no results")
        return nil
    }
    for _, r := range results {
        fmt.Printf("- %s\n  %s\n", r.Title, r.Snippet)
    }
    return nil
}

func wikiShow(path, title string) error {
    db, err := wikimod.Load(path)
    if err != nil { return err }
    if p, ok := wikimod.Show(db, title); ok {
        fmt.Printf("# %s\n\n%s\n", p.Title, p.Content)
        return nil
    }
    return fmt.Errorf("page not found: %s", title)
}


// splitArgs splits a simple space-delimited string into args.
// This is a naive splitter; for complex cases provide args via manifest directly.
func splitArgs(s string) []string {
    s = strings.TrimSpace(s)
    if s == "" { return nil }
    // Do not attempt full shell parsing; split on spaces.
    parts := strings.Fields(s)
    return parts
}
