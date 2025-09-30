package main

import (
    "bytes"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "runtime"
    "io/fs"
    "os/signal"
    "encoding/json"
    "time"

    "heimdal/internal/config"
    "heimdal/internal/fuzzy"
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
    //  - projects: project-init NAME | project-open NAME | project-info [NAME]
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

    // Load fuzzy/aliases config
    fcfg := fuzzy.Load()
    // Expand alias if configured
    if t, ok := fcfg.Aliases[args[0]]; ok {
        args[0] = t
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
    case "project-init":
        if len(args) < 2 { return errors.New("usage: heimdal project-init <name>") }
        return cmdProjectInit(args[1])
    case "project-open":
        if len(args) < 2 { return errors.New("usage: heimdal project-open <name>") }
        return cmdProjectOpen(args[1], promptPrefix)
    case "project-info":
        name := ""
        if len(args) >= 2 { name = args[1] }
        return cmdProjectInfo(name)
    case "app":
        return cmdApp(args[1:])
    case "log":
        return cmdLog(args[1:])
    case "wiki":
        return cmdWiki(args[1:])
    case "aioswiki": // alias for wiki
        return cmdWiki(args[1:])
    case "config":
        return cmdConfig(args[1:])
    default:
        // Try to interpret first token as a project
        first := args[0]
        rest := args[1:]
        if len(rest) >= 1 && rest[0] == "shell" {
            if p, ok := resolveProject(first); ok {
                return cmdProjectOpenWithPath(first, p, promptPrefix)
            }
        }
        if p, ok := resolveProject(first); ok {
            if len(rest) == 0 { return errors.New("usage: heimdal <project> <app> [args...]") }
            app := rest[0]
            return cmdRunWithProject(first, p, app, rest[1:], profile)
        }
        // shorthand: heimdal <app> [args...] if known commands; otherwise fuzzy suggest
        known := map[string]bool{"shell":true,"run":true,"app":true,"log":true,"wiki":true,"aioswiki":true,"project-init":true,"project-open":true,"project-info":true}
        if known[first] {
            return cmdRun(first, rest, profile)
        }
        if fcfg.Fuzzy.Enabled {
            best, score := fuzzy.Suggest(first, fcfg.Commands)
            if best != "" && score >= fcfg.Fuzzy.Threshold {
                fmt.Fprintf(os.Stderr, "Unknown command '%s'. Did you mean '%s'?\n", first, best)
                return errors.New("unknown command")
            }
        }
        // Fallback to running as app
        return cmdRun(first, rest, profile)
    }
}

func usage(prog string) {
    fmt.Println("Heimdal — OS Wrapper CLI (MVP)\n")
    fmt.Println("Usage:")
    fmt.Printf("  %s shell\n", prog)
    fmt.Printf("  %s run <app> [args...]\n", prog)
    fmt.Printf("  %s app add <name> --cmd <cmd> [--args \"--foo --bar\"]\n", prog)
    fmt.Printf("  %s app ls\n", prog)
    fmt.Printf("  %s app rm <name>\n", prog)
    fmt.Printf("  %s project-init <name>\n", prog)
    fmt.Printf("  %s project-open <name>\n", prog)
    fmt.Printf("  %s project-info [name]\n", prog)
    fmt.Printf("  %s aioswiki search <query>\n", prog)
    fmt.Printf("  %s aioswiki show <title>\n", prog)
    fmt.Printf("  %s aioswiki init\n", prog)
    fmt.Printf("  %s aioswiki path\n", prog)
    fmt.Printf("  %s wiki search <query>\n", prog)
    fmt.Printf("  %s wiki show <title>\n", prog)
    fmt.Printf("  %s wiki init\n", prog)
    fmt.Printf("  %s [--profile=permissive|restricted] [--prompt-prefix=\"[hd] \"] <app> [args...]  (shorthand)\n", prog)
    fmt.Printf("  %s config fuzzy show|reload\n", prog)
    fmt.Println("\nEnv/Config:")
    fmt.Println("  Apps manifests in apps/<name>.yaml. Minimal YAML supported: name, cmd, args, env.")
    fmt.Println("  Shell config: ./shell.json or ~/.heimdall/shell.json with keys: shell, virtual_path, prompt_template (use __VPATH__ token).")
}

func cmdShell(prefix string) error {
    return cmdShellWith(prefix, "", false, nil)
}

// cmdShellWith starts an interactive shell.
// If fsRoot is non-empty, the shell's working directory is set there and, when restrictOps is true,
// common mutating commands are overridden to encourage Heimdal project commands.
func cmdShellWith(prefix, fsRoot string, restrictOps bool, extraEnv map[string]string) error {
    cfg := loadShellConfig()
    sh := os.Getenv("SHELL")
    if sh == "" {
        sh = "/bin/sh"
    }
    // If config specifies a shell, prefer it
    switch cfg.Shell {
    case "zsh":
        if _, err := os.Stat("/bin/zsh"); err == nil { sh = "/bin/zsh" }
    case "bash":
        if _, err := os.Stat("/bin/bash"); err == nil { sh = "/bin/bash" }
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
    // Prompt config
    if cfg.VirtualPath {
        env["HEIMDAL_VPATH"] = "1"
    }
    if cfg.PromptTemplate != "" {
        env["HEIMDAL_PROMPT_TMPL"] = cfg.PromptTemplate
    }
    if cfg.EntryEcho != "" {
        env["HEIMDAL_ENTRY_ECHO"] = cfg.EntryEcho
    }
    user := os.Getenv("USER")
    host, _ := os.Hostname()
    env["HEIMDAL_USER"] = user
    env["HEIMDAL_HOST"] = host
    // Pass wiki alias helpers from fuzzy config
    {
        fcfg := fuzzy.Load()
        aliases := []string{}
        for k, v := range fcfg.Aliases {
            if v == "aioswiki" || v == "wiki" {
                aliases = append(aliases, k)
            }
        }
        if len(aliases) > 0 {
            env["HEIMDAL_WIKI_ALIASES"] = strings.Join(aliases, ",")
        }
    }
    for k, v := range extraEnv {
        env[k] = v
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
RC_MODE=${HEIMDAL_RC_MODE:-project-then-os}
PROJ_RC=${HEIMDAL_PROJECT_RC_ZSH}
case "$RC_MODE" in
  project-only)
    if [[ -f "$PROJ_RC" ]]; then source "$PROJ_RC"; fi ;;
  project-then-os)
    if [[ -f "$PROJ_RC" ]]; then source "$PROJ_RC"; fi
    if [[ -f "$HOME/.zshrc" ]]; then source "$HOME/.zshrc"; fi ;;
  os-then-project)
    if [[ -f "$HOME/.zshrc" ]]; then source "$HOME/.zshrc"; fi
    if [[ -f "$PROJ_RC" ]]; then source "$PROJ_RC"; fi ;;
  *)
    if [[ -f "$HOME/.zshrc" ]]; then source "$HOME/.zshrc"; fi ;;
esac
# Ensure HEIMDAL_BIN is set (fallback to PATH lookup)
if [[ -z "$HEIMDAL_BIN" ]]; then
  HEIMDAL_BIN=$(command -v heimdal 2>/dev/null)
fi
# Built-in wiki functions
function aioswiki() { command "$HEIMDAL_BIN" wiki "$@" }
function wiki() { aioswiki "$@" }
# Project helpers
function project-init() { command "$HEIMDAL_BIN" project-init "$@" }
function project-open() { command "$HEIMDAL_BIN" project-open "$@" }
if [[ -n "$HEIMDAL_ENTRY_ECHO" ]]; then
  echo "$HEIMDAL_ENTRY_ECHO"
fi
# Helper scripts so child processes can call 'aioswiki' directly
# Use a stable per-session path so all children inherit it reliably
HEIMDAL_HELPER_DIR="${HEIMDAL_HELPER_DIR:-$HOME/.heimdall/sessions/$HEIMDAL_SESSION/bin}"
mkdir -p "$HEIMDAL_HELPER_DIR"
cat > "$HEIMDAL_HELPER_DIR/aioswiki" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
chmod +x "$HEIMDAL_HELPER_DIR/aioswiki"
cat > "$HEIMDAL_HELPER_DIR/wiki" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
chmod +x "$HEIMDAL_HELPER_DIR/wiki"
export PATH="$HEIMDAL_HELPER_DIR:$PATH"
if [[ -n "$HEIMDAL_WIKI_ALIASES" ]]; then
  IFS=',' read -r -A __aliases <<< "$HEIMDAL_WIKI_ALIASES"
  for __a in "${__aliases[@]}"; do
    cat > "$HEIMDAL_HELPER_DIR/${__a}" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
    chmod +x "$HEIMDAL_HELPER_DIR/${__a}"
  done
fi
` + func() string { if restrictOps { return `
# Restrict mutating commands inside project shell
function _heimdal_block(){ echo "[heimdal] disabled here. Use 'heimdal newfile/mkdir/annotate/export' instead." >&2; return 1 }
alias mkdir=_heimdal_block
alias mv=_heimdal_block
alias rm=_heimdal_block
alias cp=_heimdal_block
alias touch=_heimdal_block
` } else { return "" } }() + `
function __heimdal_vpath(){
  local vr="$HEIMDAL_VROOT"
  local p="$PWD"
  if [[ -n "$vr" && "$p" == ${vr}* ]]; then
    local rel="${p#$vr}"
    if [[ -z "$rel" ]]; then echo "/"; else echo "/$rel"; fi
  else
    echo "$p"
  fi
}
function __heimdal_write_vpath(){
  local f="$HEIMDAL_VPATH_FILE"
  if [[ -n "$f" ]]; then
    __heimdal_vpath >| "$f" 2>/dev/null
  fi
}
function _heimdal_prompt_prefix() {
  local tmpl="${HEIMDAL_PROMPT_TMPL}"
  local who="${HEIMDAL_USER}"; local host="${HEIMDAL_HOST}";
  [[ -z "$who" ]] && who=$(whoami)
  [[ -z "$host" ]] && host="%m"
  if [[ -n "$tmpl" ]]; then
    local v=""
    if [[ "$HEIMDAL_VPATH" == "1" && -n "$HEIMDAL_VROOT" ]]; then v=$(__heimdal_vpath); fi
    local rendered="${tmpl//__VPATH__/$v}"
    PROMPT="${HEIMDAL_PREFIX}${rendered} "
    return
  fi
  if [[ "$HEIMDAL_VPATH" == "1" && -n "$HEIMDAL_VROOT" ]]; then
    local v=$(__heimdal_vpath)
    PROMPT="${HEIMDAL_PREFIX}${who}@${host} ${v} %(!.#.$) "
    return
  fi
  local p="${HEIMDAL_PREFIX}"
  if [[ -n "$p" ]] && [[ "${PROMPT}" != ${p}* ]]; then
    PROMPT="${p}${PROMPT}"
  fi
}
precmd_functions+=(_heimdal_prompt_prefix)
chpwd_functions+=(__heimdal_write_vpath)
__heimdal_write_vpath
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
RC_MODE=${HEIMDAL_RC_MODE:-project-then-os}
PROJ_RC=${HEIMDAL_PROJECT_RC_BASH}
case "$RC_MODE" in
  project-only)
    [ -f "$PROJ_RC" ] && . "$PROJ_RC" ;;
  project-then-os)
    [ -f "$PROJ_RC" ] && . "$PROJ_RC"
    [ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc" ;;
  os-then-project)
    [ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"
    [ -f "$PROJ_RC" ] && . "$PROJ_RC" ;;
  *)
    [ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc" ;;
esac
# Ensure HEIMDAL_BIN is set (fallback to PATH lookup)
if [ -z "$HEIMDAL_BIN" ]; then
  HEIMDAL_BIN=$(command -v heimdal 2>/dev/null)
fi
# Built-in wiki functions
aioswiki() { command "$HEIMDAL_BIN" wiki "$@"; }
wiki() { aioswiki "$@"; }
# Project helpers
project-init() { command "$HEIMDAL_BIN" project-init "$@"; }
project-open() { command "$HEIMDAL_BIN" project-open "$@"; }
if [ -n "$HEIMDAL_ENTRY_ECHO" ]; then
  echo "$HEIMDAL_ENTRY_ECHO"
fi
HEIMDAL_HELPER_DIR="${HEIMDAL_HELPER_DIR:-$HOME/.heimdall/sessions/$HEIMDAL_SESSION/bin}"
mkdir -p "$HEIMDAL_HELPER_DIR"
cat > "$HEIMDAL_HELPER_DIR/aioswiki" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
chmod +x "$HEIMDAL_HELPER_DIR/aioswiki"
cat > "$HEIMDAL_HELPER_DIR/wiki" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
chmod +x "$HEIMDAL_HELPER_DIR/wiki"
export PATH="$HEIMDAL_HELPER_DIR:$PATH"
if [ -n "$HEIMDAL_WIKI_ALIASES" ]; then
  IFS=',' read -r -a __aliases <<< "$HEIMDAL_WIKI_ALIASES"
  for __a in "${__aliases[@]}"; do
    cat > "$HEIMDAL_HELPER_DIR/${__a}" <<'EOF'
#!/bin/sh
BIN="${HEIMDAL_BIN:-$(command -v heimdal 2>/dev/null)}"
if [ -z "$BIN" ]; then
  echo "heimdal binary not found (set HEIMDAL_BIN or add to PATH)" >&2
  exit 127
fi
exec "$BIN" wiki "$@"
EOF
    chmod +x "$HEIMDAL_HELPER_DIR/${__a}"
  done
fi
` + func() string { if restrictOps { return `
# Restrict mutating commands inside project shell
_heimdal_block(){ echo "[heimdal] disabled here. Use 'heimdal newfile/mkdir/annotate/export' instead." >&2; return 1; }
alias mkdir=_heimdal_block
alias mv=_heimdal_block
alias rm=_heimdal_block
alias cp=_heimdal_block
alias touch=_heimdal_block
` } else { return "" } }() + `
__heimdal_vpath(){
  vr="$HEIMDAL_VROOT"; p="$PWD";
  case "$p" in
    "$vr"*) rel="${p#$vr}"; [ -z "$rel" ] && echo "/" || echo "/$rel" ;;
    *) echo "$p" ;;
  esac
}
__heimdal_write_vpath(){
  f="$HEIMDAL_VPATH_FILE"; [ -n "$f" ] && __heimdal_vpath > "$f" 2>/dev/null || true
}
__heimdal_ps1() {
  tmpl="${HEIMDAL_PROMPT_TMPL}"; who="${HEIMDAL_USER}"; host="${HEIMDAL_HOST}";
  [ -z "$who" ] && who="$(whoami)"
  [ -z "$host" ] && host="$(hostname)"
  if [ -n "$tmpl" ]; then
    v=""; if [ "$HEIMDAL_VPATH" = "1" ] && [ -n "$HEIMDAL_VROOT" ]; then v="$(__heimdal_vpath)"; fi
    rendered="${tmpl//__VPATH__/$v}"
    PS1="${HEIMDAL_PREFIX}${rendered} "
    return
  fi
  if [ "$HEIMDAL_VPATH" = "1" ] && [ -n "$HEIMDAL_VROOT" ]; then
    v="$(__heimdal_vpath)"; PS1="${HEIMDAL_PREFIX}${who}@${host} ${v} $ "; return
  fi
  case "$PS1" in
    ${HEIMDAL_PREFIX}*) ;;
    *) PS1="${HEIMDAL_PREFIX}${PS1}";;
  esac
}
PROMPT_COMMAND="__heimdal_ps1; __heimdal_write_vpath; ${PROMPT_COMMAND}"
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
    if fsRoot != "" {
        // ensure exists and set as workdir
        _ = os.MkdirAll(fsRoot, 0o755)
        cmd.Dir = fsRoot
    }

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
    if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
        wikiUsage()
        return nil
    }
    sub := args[0]
    workdir := os.Getenv("HEIMDAL_WORKDIR")
    if workdir == "" {
        workdir, _ = os.Getwd()
    }
    path, err := wikimod.Locate(workdir)
    if err != nil { return err }
    switch sub {
    case "init":
        if err := wikimod.Init(path); err != nil { return err }
        fmt.Println("initialized wiki at:", path)
        return nil
    case "path":
        fmt.Println(path)
        return nil
    case "search":
        if len(args) < 2 { return errors.New("usage: heimdal wiki search <query>") }
        if !fileExists(path) {
            fmt.Printf("no wiki.json found at %s\n", path)
            fmt.Println("run: heimdal aioswiki init")
            return nil
        }
        return wikiSearch(path, strings.Join(args[1:], " "))
    case "show":
        if len(args) < 2 { return errors.New("usage: heimdal wiki show <title>") }
        if !fileExists(path) {
            fmt.Printf("no wiki.json found at %s\n", path)
            fmt.Println("run: heimdal aioswiki init")
            return nil
        }
        return wikiShow(path, strings.Join(args[1:], " "))
    default:
        wikiUsage()
        return nil
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

func wikiUsage() {
    // Attempt to show where wiki.json would be located for context
    wd := os.Getenv("HEIMDAL_WORKDIR")
    if wd == "" { wd, _ = os.Getwd() }
    p, _ := wikimod.Locate(wd)
    fmt.Println("aioswiki — Heimdal Wiki (RAG manpages)")
    fmt.Println()
    fmt.Println("Usage:")
    fmt.Println("  aioswiki search <query>")
    fmt.Println("  aioswiki show <title>")
    fmt.Println("  aioswiki init")
    fmt.Println("  aioswiki path")
    fmt.Println()
    if p != "" { fmt.Println("wiki.json path:", p) }
}

func fileExists(p string) bool {
    st, err := os.Stat(p)
    return err == nil && !st.IsDir()
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

// --- Config commands ---
func cmdConfig(args []string) error {
    if len(args) == 0 { return errors.New("usage: heimdal config fuzzy [show|reload]") }
    domain := args[0]
    switch domain {
    case "fuzzy":
        if len(args) < 2 { return errors.New("usage: heimdal config fuzzy [show|reload]") }
        switch args[1] {
        case "show":
            cfg := fuzzy.Load()
            b, _ := json.MarshalIndent(cfg, "", "  ")
            fmt.Println(string(b))
            return nil
        case "reload":
            return regenerateWikiHelpers()
        default:
            return errors.New("usage: heimdal config fuzzy [show|reload]")
        }
    default:
        return fmt.Errorf("unknown config domain: %s", domain)
    }
}

func regenerateWikiHelpers() error {
    cfg := fuzzy.Load()
    // Determine session helper dir
    sess := os.Getenv("HEIMDAL_SESSION")
    if sess == "" {
        return errors.New("not in a Heimdal session (HEIMDAL_SESSION not set)")
    }
    home, err := os.UserHomeDir()
    if err != nil { return err }
    dir := filepath.Join(home, ".heimdall", "sessions", sess, "bin")
    if err := os.MkdirAll(dir, 0o755); err != nil { return err }
    bin := os.Getenv("HEIMDAL_BIN")
    if bin == "" { bin = "heimdal" }
    // Ensure core helpers
    if err := writeHelper(dir, "aioswiki", bin); err != nil { return err }
    if err := writeHelper(dir, "wiki", bin); err != nil { return err }
    // Domain aliases
    for _, a := range cfg.EffectiveWikiAliases() {
        if a == "aioswiki" || a == "wiki" { continue }
        if err := writeHelper(dir, a, bin); err != nil { return err }
    }
    fmt.Println("fuzzy helpers regenerated in:", dir)
    return nil
}

func writeHelper(dir, name, bin string) error {
    sh := "#!/bin/sh\nBIN=\"${HEIMDAL_BIN:-$(command -v " + bin + " 2>/dev/null)}\"\n" +
        "if [ -z \"$BIN\" ]; then echo \"heimdal binary not found (set HEIMDAL_BIN or add to PATH)\" >&2; exit 127; fi\n" +
        "exec \"$BIN\" wiki \"$@\"\n"
    path := filepath.Join(dir, name)
    if err := os.WriteFile(path, []byte(sh), 0o755); err != nil { return err }
    return nil
}

// ShellConfig holds optional shell customization.
type ShellConfig struct {
    Shell          string `json:"shell"`
    VirtualPath    bool   `json:"virtual_path"`
    PromptTemplate string `json:"prompt_template"`
    EntryEcho      string `json:"entry_echo"`
    RcMode         string `json:"rc_mode"`
    ProjectRcDir   string `json:"project_rc_dir"`
}

func loadShellConfig() ShellConfig {
    // Prefer repo-local shell.json, then ~/.heimdall/shell.json
    var cfg ShellConfig
    cwd, _ := os.Getwd()
    paths := []string{filepath.Join(cwd, "shell.json")}
    if h, err := os.UserHomeDir(); err == nil {
        paths = append(paths, filepath.Join(h, ".heimdall", "shell.json"))
    }
    for _, p := range paths {
        b, err := os.ReadFile(p)
        if err != nil { continue }
        _ = json.Unmarshal(b, &cfg)
        break
    }
    return cfg
}

// --- Project support (MVP) ---

// resolveProject tries to find a sqlite file for the given project name.
// It looks in CWD as ./NAME.sqlite, then in ~/.heimdall/projects/NAME.sqlite.
func resolveProject(name string) (string, bool) {
    if name == "" { return "", false }
    // CWD bundle
    cwd, _ := os.Getwd()
    bundle := filepath.Join(cwd, name+".aiosproj")
    if st, err := os.Stat(bundle); err == nil && st.IsDir() {
        db := filepath.Join(bundle, "project.sqlite")
        if st2, err2 := os.Stat(db); err2 == nil && !st2.IsDir() { return db, true }
    }
    // CWD legacy
    p := filepath.Join(cwd, name+".sqlite")
    if st, err := os.Stat(p); err == nil && !st.IsDir() { return p, true }
    // Home dir fallback
    if h, err := os.UserHomeDir(); err == nil {
        bundle = filepath.Join(h, ".heimdall", "projects", name+".aiosproj")
        if st, err := os.Stat(bundle); err == nil && st.IsDir() {
            db := filepath.Join(bundle, "project.sqlite")
            if st2, err2 := os.Stat(db); err2 == nil && !st2.IsDir() { return db, true }
        }
        p = filepath.Join(h, ".heimdall", "projects", name+".sqlite")
        if st, err := os.Stat(p); err == nil && !st.IsDir() { return p, true }
    }
    return "", false
}

func ensureProjectBundle(name string) (dbPath, bundleDir string, err error) {
    // Prefer CWD
    cwd, _ := os.Getwd()
    dir := filepath.Join(cwd, name+".aiosproj")
    if err2 := os.MkdirAll(dir, 0o755); err2 == nil {
        db := filepath.Join(dir, "project.sqlite")
        if err3 := touchFile(db); err3 == nil {
            _ = os.MkdirAll(filepath.Join(dir, "rc"), 0o755)
            _ = writeMeta(filepath.Join(dir, "meta.json"), name)
            return db, dir, nil
        }
    }
    // Fallback to ~/.heimdall/projects
    h, err := os.UserHomeDir()
    if err != nil { return "", "", err }
    root := filepath.Join(h, ".heimdall", "projects", name+".aiosproj")
    if err := os.MkdirAll(root, 0o755); err != nil { return "", "", err }
    db := filepath.Join(root, "project.sqlite")
    if err := touchFile(db); err != nil { return "", "", err }
    _ = os.MkdirAll(filepath.Join(root, "rc"), 0o755)
    _ = writeMeta(filepath.Join(root, "meta.json"), name)
    return db, root, nil
}

func touchFile(p string) error {
    if _, err := os.Stat(p); err == nil { return nil }
    return os.WriteFile(p, []byte{}, 0o644)
}

func writeMeta(path, name string) error {
    meta := fmt.Sprintf("{\n  \"name\": \"%s\",\n  \"created_at\": \"%s\",\n  \"version\": 1\n}\n", name, time.Now().Format(time.RFC3339))
    return os.WriteFile(path, []byte(meta), 0o644)
}

func cmdProjectInit(name string) error {
    if _, ok := resolveProject(name); ok {
        return fmt.Errorf("project already exists: %s", name)
    }
    p, bundle, err := ensureProjectBundle(name)
    if err != nil { return err }
    // Try to initialize DB schema via sqlite3 CLI if available
    if err := initProjectDB(p); err != nil {
        // Graceful: write .sql next to DB and inform the user
        sqlPath := p + ".init.sql"
        _ = os.WriteFile(sqlPath, []byte(projectInitSQL(filepath.Dir(p))), 0o644)
        fmt.Println("created bundle:", bundle)
        fmt.Println("db:", p)
        fmt.Println("note: sqlite3 not found or init failed. Run manually:")
        fmt.Printf("  sqlite3 %s < %s\n", p, sqlPath)
        return nil
    }
    fmt.Println("created and initialized bundle:", bundle)
    fmt.Println("db:", p)
    return nil
}

func cmdProjectOpen(name, prefix string) error {
    p, ok := resolveProject(name)
    if !ok { return fmt.Errorf("project not found: %s", name) }
    return cmdProjectOpenWithPath(name, p, prefix)
}

func cmdProjectOpenWithPath(name, dbPath, prefix string) error {
    if !strings.Contains(prefix, name) {
        prefix = "[hd:" + name + "] "
    }
    // Create session to host project fs view
    cwd, _ := os.Getwd()
    sess, err := universe.StartSession(cwd)
    if err != nil { return err }
    fsRoot := filepath.Join(sess.Dir, "fs", name)
    user := os.Getenv("USER")
    host, _ := os.Hostname()
    cfg := loadShellConfig()
    projDir := projectBundleDirFromDB(dbPath)
    extra := map[string]string{
        "HEIMDAL_PROJECT_NAME": name,
        "HEIMDAL_PROJECT_DB": dbPath,
        "HEIMDAL_PROJECT_DIR": projDir,
        "HEIMDAL_SESSION": sess.ID,
        "HEIMDAL_CONTEXT_DIR": sess.ContextDir,
        "HEIMDAL_WORKDIR": cwd,
        "HEIMDAL_UNIVERSE": "1",
        "HEIMDAL_VROOT": fsRoot,
        "HEIMDAL_VPATH": func() string { if cfg.VirtualPath { return "1" }; return "" }(),
        "HEIMDAL_USER": user,
        "HEIMDAL_HOST": host,
    }
    // Project-specific rc
    rcDir := cfg.ProjectRcDir
    if rcDir == "" {
        if projDir != "" {
            rcDir = filepath.Join(projDir, "rc")
        } else {
            rcDir = "~/.heimdall/projects/{name}/rc"
            rcDir = strings.ReplaceAll(rcDir, "{name}", name)
            rcDir = expandPath(rcDir)
        }
    } else {
        rcDir = strings.ReplaceAll(rcDir, "{name}", name)
        rcDir = expandPath(rcDir)
    }
    extra["HEIMDAL_RC_MODE"] = cfg.RcMode
    extra["HEIMDAL_PROJECT_RC_ZSH"] = filepath.Join(rcDir, ".zshrc")
    extra["HEIMDAL_PROJECT_RC_BASH"] = filepath.Join(rcDir, "bashrc")
    // Open shell in isolated fs root with restricted mutating commands
    return cmdShellWith(prefix, fsRoot, true, extra)
}

// expandPath expands leading ~ to user home and environment variables.
func expandPath(p string) string {
    if strings.HasPrefix(p, "~") {
        if h, err := os.UserHomeDir(); err == nil {
            p = filepath.Join(h, strings.TrimPrefix(p, "~"))
        }
    }
    return os.ExpandEnv(p)
}

func cmdRunWithProject(project, dbPath, app string, rest []string, profile string) error {
    // Create a session and set project env then run app
    cwd, _ := os.Getwd()
    sess, err := universe.StartSession(cwd)
    if err != nil { return err }

    appsDir, err := config.EnsureAppsDir()
    if err != nil { return err }
    maniPath := filepath.Join(appsDir, app+".yaml")
    var m manifest.Manifest
    if _, err := os.Stat(maniPath); err == nil {
        m, err = manifest.Load(maniPath)
        if err != nil { return fmt.Errorf("load manifest: %w", err) }
    } else {
        m = manifest.Manifest{Name: app, Cmd: app}
    }
    cmdName := m.Cmd
    cmdArgs := append([]string{}, m.Args...)
    cmdArgs = append(cmdArgs, rest...)

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
    envMap["HEIMDAL_PROJECT_NAME"] = project
    envMap["HEIMDAL_PROJECT_DB"] = dbPath
    for k, v := range m.Env { envMap[k] = os.ExpandEnv(v) }
    envList := make([]string, 0, len(envMap))
    for k, v := range envMap { envList = append(envList, k+"="+v) }

    fmt.Fprintf(os.Stderr, "[heimdal] project=%s app=%s cmd=%s profile=%s\n", project, app, cmdName, profile)
    cmd := exec.Command(cmdName, cmdArgs...)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = envList
    return cmd.Run()
}

func cmdProjectInfo(name string) error {
    if name == "" {
        // List known (best-effort): scan CWD and ~/.heimdall/projects
        cwd, _ := os.Getwd()
        if ents, err := os.ReadDir(cwd); err == nil {
            for _, e := range ents {
                if e.IsDir() && strings.HasSuffix(e.Name(), ".aiosproj") {
                    fmt.Printf("- %s\n", e.Name())
                }
            }
        }
        if glob, _ := filepath.Glob(filepath.Join(cwd, "*.sqlite")); len(glob) > 0 {
            for _, p := range glob { fmt.Printf("- %s\n", filepath.Base(p)) }
        }
        if h, err := os.UserHomeDir(); err == nil {
            dir := filepath.Join(h, ".heimdall", "projects")
            if ents, err := os.ReadDir(dir); err == nil {
                for _, e := range ents {
                    if e.IsDir() && strings.HasSuffix(e.Name(), ".aiosproj") {
                        fmt.Printf("- %s\n", e.Name())
                    }
                    if !e.IsDir() && strings.HasSuffix(e.Name(), ".sqlite") {
                        fmt.Printf("- %s\n", e.Name())
                    }
                }
            }
        }
        return nil
    }
    if p, ok := resolveProject(name); ok {
        fmt.Printf("project: %s\n", name)
        if d := projectBundleDirFromDB(p); d != "" {
            fmt.Printf("dir: %s\n", d)
        }
        fmt.Printf("db: %s\n", p)
        return nil
    }
    return fmt.Errorf("project not found: %s", name)
}

func projectBundleDirFromDB(dbPath string) string {
    if strings.HasSuffix(dbPath, string(os.PathSeparator)+"project.sqlite") {
        return filepath.Dir(dbPath)
    }
    return ""
}

// --- Project DB initialization using sqlite3 CLI (no Go driver dependency) ---
func initProjectDB(dbPath string) error {
    if _, err := exec.LookPath("sqlite3"); err != nil {
        return err
    }
    sql := projectInitSQL(filepath.Dir(dbPath))
    cmd := exec.Command("sqlite3", dbPath)
    cmd.Stdin = bytes.NewBufferString(sql)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func projectInitSQL(projectRoot string) string {
    // Minimal v1 schema + metadata. Extend as needed.
    return fmt.Sprintf(`PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
BEGIN;
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);

CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY,
  root TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);
INSERT OR IGNORE INTO projects(root, created_at) VALUES ('%s', datetime('now'));

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_id INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  profile TEXT NOT NULL,
  context_dir TEXT NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL,
  app TEXT NOT NULL,
  cmdline TEXT NOT NULL,
  started_at TEXT NOT NULL,
  exit_code INTEGER,
  FOREIGN KEY(session_id) REFERENCES sessions(id)
);
COMMIT;`, escapeSQL(projectRoot))
}

func escapeSQL(s string) string { return strings.ReplaceAll(s, "'", "''") }
