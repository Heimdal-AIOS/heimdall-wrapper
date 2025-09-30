package main

import (
    "archive/zip"
    "bytes"
    "errors"
    "fmt"
    "io"
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
    case "project-pack":
        if os.Getenv("HEIMDAL_SESSION") != "" { return errors.New("run project-pack outside Heimdal shell") }
        if len(args) < 2 { return errors.New("usage: heimdal project-pack <name> [-o output.zip]") }
        name := args[1]
        out := ""
        for i:=2;i<len(args);i++{ if args[i]=="-o" && i+1<len(args){ out=args[i+1]; i++ } }
        return cmdProjectPack(name, out)
    case "project-unpack":
        if os.Getenv("HEIMDAL_SESSION") != "" { return errors.New("run project-unpack outside Heimdal shell") }
        if len(args) < 2 { return errors.New("usage: heimdal project-unpack <archive.zip> [--dest DIR]") }
        archive := args[1]
        dest := ""
        for i:=2;i<len(args);i++{ if args[i]=="--dest" && i+1<len(args){ dest=args[i+1]; i++ } }
        return cmdProjectUnpack(archive, dest)
    case "mkdir":
        return cmdFSMakeDir(args[1:])
    case "newfile":
        return cmdFSNewFile(args[1:])
    case "ls":
        return cmdFSList(args[1:])
    case "rm":
        return cmdFSRemove(args[1:])
    case "mv":
        return cmdFSMove(args[1:])
    case "cat":
        return cmdFSCat(args[1:])
    case "pwd":
        return cmdFSPwd(args[1:])
    case "append":
        return cmdFSAppend(args[1:])
    case "annotate":
        return cmdFSAnnotate(args[1:])
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
    inSession := os.Getenv("HEIMDAL_SESSION") != ""
    fmt.Println("Heimdal — AI:OS Wrapper CLI\n")
    if !inSession {
        fmt.Println("Outside Heimdal (project/system commands):")
        fmt.Printf("  %s project-init <name>\n", prog)
        fmt.Printf("  %s project-open <name>\n", prog)
        fmt.Printf("  %s project-info [name]\n", prog)
        fmt.Printf("  %s project-pack <name> [-o output.zip]\n", prog)
        fmt.Printf("  %s project-unpack <archive.zip> [--dest DIR]\n", prog)
        fmt.Printf("  %s aioswiki search|show|init|path\n", prog)
        fmt.Printf("  %s app add|ls|rm ...\n", prog)
        fmt.Printf("  %s run <app> [args...]\n", prog)
        fmt.Printf("  %s shell\n", prog)
        fmt.Printf("  %s config fuzzy show|reload\n", prog)
    } else {
        fmt.Println("Inside Heimdal ([hd] prompt):")
        fmt.Println("  aioswiki search <q> | show <title> | init | path")
        fmt.Println("  project-open <name> | project-init <name>")
        fmt.Println("  app add|ls|rm ...  | run <app> [args...]")
        fmt.Printf("  %s [--profile=permissive|restricted] [--prompt-prefix=\"[hd] \"] <app> [args...]\n", prog)
        fmt.Println("  config fuzzy show|reload")
        fmt.Println()
        fmt.Println("Note: project-pack/unpack must be run outside Heimdal.")
    }
    fmt.Println("\nEnv/Config:")
    fmt.Println("  Apps manifests in apps/<name>.yaml. Minimal YAML supported: name, cmd, args, env.")
    fmt.Println("  Shell config: ./shell.json or ~/.heimdall/shell.json (shell, rc_mode, project_rc_dir, virtual_path, prompt_template, entry_echo).")
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

// --- Project pack/unpack (run outside Heimdal shell) ---
func cmdProjectPack(name, out string) error {
    // Find bundle dir
    db, ok := resolveProject(name)
    var dir string
    if ok {
        dir = projectBundleDirFromDB(db)
        if dir == "" { return fmt.Errorf("project '%s' is legacy single-file; pack not supported yet", name) }
    } else {
        // try default locations
        cwd, _ := os.Getwd()
        d := filepath.Join(cwd, name+".aiosproj")
        if st, err := os.Stat(d); err == nil && st.IsDir() { dir = d } else {
            if h, err := os.UserHomeDir(); err == nil {
                d = filepath.Join(h, ".heimdall", "projects", name+".aiosproj")
                if st, err := os.Stat(d); err == nil && st.IsDir() { dir = d }
            }
        }
        if dir == "" { return fmt.Errorf("project bundle not found for: %s", name) }
    }
    if out == "" {
        out = name + ".aiosproj.zip"
    }
    return zipDir(dir, out)
}

func cmdProjectUnpack(archive, dest string) error {
    if dest == "" {
        if h, err := os.UserHomeDir(); err == nil {
            dest = filepath.Join(h, ".heimdall", "projects")
        } else {
            cwd, _ := os.Getwd(); dest = cwd
        }
    }
    if err := os.MkdirAll(dest, 0o755); err != nil { return err }
    return unzipTo(archive, dest)
}

func zipDir(srcDir, dstZip string) error {
    zf, err := os.Create(dstZip)
    if err != nil { return err }
    defer zf.Close()
    zw := zip.NewWriter(zf)
    defer zw.Close()
    base := filepath.Dir(srcDir)
    return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
        if err != nil { return err }
        rel, _ := filepath.Rel(base, path)
        if info.IsDir() {
            if rel == "." { return nil }
            _, err := zw.Create(rel + "/")
            return err
        }
        fh, err := zip.FileInfoHeader(info)
        if err != nil { return err }
        fh.Name = rel
        w, err := zw.CreateHeader(fh)
        if err != nil { return err }
        f, err := os.Open(path)
        if err != nil { return err }
        defer f.Close()
        _, err = io.Copy(w, f)
        return err
    })
}

func unzipTo(zipPath, dest string) error {
    r, err := zip.OpenReader(zipPath)
    if err != nil { return err }
    defer r.Close()
    for _, f := range r.File {
        fp := filepath.Join(dest, f.Name)
        if !strings.HasPrefix(fp, filepath.Clean(dest)+string(os.PathSeparator)) {
            return fmt.Errorf("illegal path in zip: %s", f.Name)
        }
        if f.FileInfo().IsDir() {
            if err := os.MkdirAll(fp, 0o755); err != nil { return err }
            continue
        }
        if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil { return err }
        rc, err := f.Open()
        if err != nil { return err }
        w, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
        if err != nil { rc.Close(); return err }
        if _, err := io.Copy(w, rc); err != nil { w.Close(); rc.Close(); return err }
        w.Close(); rc.Close()
    }
    return nil
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

-- Files/Flatfiles schema (MVP)
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,               -- 'dir'|'file'
  note TEXT,
  aicom TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(project_id, path),
  FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS file_tags (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  tag TEXT NOT NULL,
  FOREIGN KEY(file_id) REFERENCES files(id)
);

CREATE TABLE IF NOT EXISTS file_lines (
  id INTEGER PRIMARY KEY,
  file_id INTEGER NOT NULL,
  lineno INTEGER NOT NULL,
  content TEXT NOT NULL,
  side TEXT,
  aicom TEXT,
  UNIQUE(file_id, lineno),
  FOREIGN KEY(file_id) REFERENCES files(id)
);
COMMIT;`, escapeSQL(projectRoot))
}

func escapeSQL(s string) string { return strings.ReplaceAll(s, "'", "''") }

// --- File/Dir commands (DB-backed) ---

func cmdFSMakeDir(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fsMkdirUsage()
        return nil
    }
    if len(args) < 1 { fsMkdirUsage(); return nil }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    path := args[0]
    meta := strings.Join(args[1:], " ")
    tags, note, aicom := parseMeta(meta)
    root := projectBundleDirFromDB(db)
    projRoot := root
    if projRoot == "" { projRoot, _ = os.Getwd() }
    // Ensure project exists and get id
    pid, err := ensureProjectRow(db, projRoot)
    if err != nil { return err }
    // Create each path component as dir; apply meta only to final
    comps := strings.Split(strings.Trim(path, "/"), "/")
    cur := ""
    for i, c := range comps {
        if c == "" { continue }
        if cur == "" { cur = c } else { cur = cur + "/" + c }
        name := c
        if err := upsertDir(db, pid, cur, name, i == len(comps)-1, tags, note, aicom); err != nil { return err }
    }
    fmt.Println("ok")
    return nil
}

func cmdFSNewFile(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fsNewfileUsage()
        return nil
    }
    if len(args) < 1 { fsNewfileUsage(); return nil }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    // parse flags
    content := ""
    filtered := []string{}
    for i := 0; i < len(args); i++ {
        if args[i] == "--content" && i+1 < len(args) { content = args[i+1]; i++; continue }
        filtered = append(filtered, args[i])
    }
    path := filtered[0]
    meta := strings.Join(filtered[1:], " ")
    tags, note, aicom := parseMeta(meta)
    root := projectBundleDirFromDB(db)
    projRoot := root
    if projRoot == "" { projRoot, _ = os.Getwd() }
    pid, err := ensureProjectRow(db, projRoot)
    if err != nil { return err }
    // Ensure parent dirs
    if dir := filepath.Dir(path); dir != "." && dir != "" {
        _ = cmdFSMakeDir([]string{dir})
    }
    name := filepath.Base(path)
    if err := upsertFile(db, pid, path, name, tags, note, aicom, content); err != nil { return err }
    fmt.Println("ok")
    return nil
}

func cmdFSList(args []string) error {
    if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
        fsLsUsage()
        return nil
    }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    base := "."
    if len(args) > 0 { base = args[0] }
    pid, err := currentProjectID(db)
    if err != nil { return err }
    // List immediate children
    like := escapeSQL(strings.Trim(base, "/"))
    if like == "." || like == "" { like = "" } else { like = like + "/" }
    sql := fmt.Sprintf("SELECT path,name,type FROM files WHERE project_id=%d AND (path LIKE '%s%%' OR path='%s') ORDER BY path;", pid, like, like)
    out, err := runSQLiteQuery(db, sql)
    if err != nil { return err }
    fmt.Print(out)
    return nil
}

func fsMkdirUsage() {
    fmt.Println("mkdir — create DB-backed directory entries")
    fmt.Println("Usage: heimdal mkdir <path/to/dir> [metadata]")
    fmt.Println("Metadata: @@tag1,tag2   ::AICOM content::   // note")
}

func fsNewfileUsage() {
    fmt.Println("newfile — create DB-backed flatfile")
    fmt.Println("Usage: heimdal newfile <path/to/name.type> [metadata] [--content \"text\"]")
    fmt.Println("Metadata: @@tag1,tag2   ::AICOM content::   // note")
}

func fsLsUsage() {
    fmt.Println("ls — list directories/files from DB (not OS)")
    fmt.Println("Usage: heimdal ls [path]")
}

func cmdFSAppend(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fmt.Println("append — append a new line to a flatfile")
        fmt.Println("Usage: heimdal append <path/to/file> --text \"content\"")
        return nil
    }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    // parse flags
    text := ""
    filtered := []string{}
    for i := 0; i < len(args); i++ {
        if args[i] == "--text" && i+1 < len(args) { text = args[i+1]; i++; continue }
        filtered = append(filtered, args[i])
    }
    if len(filtered) < 1 || text == "" { fmt.Println("Usage: heimdal append <path> --text \"content\""); return nil }
    path := strings.Trim(filtered[0], "/")
    pid, err := currentProjectID(db)
    if err != nil { return err }
    fid, err := getFileID(db, pid, path)
    if err != nil { return err }
    // Insert next line
    ins := fmt.Sprintf("INSERT INTO file_lines(file_id, lineno, content) SELECT %d, COALESCE(MAX(lineno)+1,1), '%s' FROM file_lines WHERE file_id=%d;", fid, escapeSQL(text), fid)
    if _, err := runSQLiteExec(db, ins); err != nil { return err }
    fmt.Println("ok")
    return nil
}

func cmdFSAnnotate(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fmt.Println("annotate — set metadata on a specific line")
        fmt.Println("Usage: heimdal annotate <path/to/file> --line N \"// note @@tag1,tag2 ::AICOM::\"")
        return nil
    }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    // parse flags
    var lineNumStr string
    filtered := []string{}
    for i := 0; i < len(args); i++ {
        if args[i] == "--line" && i+1 < len(args) { lineNumStr = args[i+1]; i++; continue }
        filtered = append(filtered, args[i])
    }
    if len(filtered) < 2 || lineNumStr == "" { fmt.Println("Usage: heimdal annotate <path> --line N \"// note @@tag ::aicom::\""); return nil }
    path := strings.Trim(filtered[0], "/")
    meta := strings.Join(filtered[1:], " ")
    tags, note, aicom := parseMeta(meta)
    pid, err := currentProjectID(db)
    if err != nil { return err }
    fid, err := getFileID(db, pid, path)
    if err != nil { return err }
    // ensure line exists
    ins := fmt.Sprintf("INSERT OR IGNORE INTO file_lines(file_id, lineno, content) VALUES (%d, %s, '') ;", fid, lineNumStr)
    if _, err := runSQLiteExec(db, ins); err != nil { return err }
    // update
    set := []string{}
    if note != "" { set = append(set, fmt.Sprintf("side='%s'", escapeSQL(note))) }
    if aicom != "" { set = append(set, fmt.Sprintf("aicom='%s'", escapeSQL(aicom))) }
    if len(set) > 0 {
        upd := fmt.Sprintf("UPDATE file_lines SET %s WHERE file_id=%d AND lineno=%s;", strings.Join(set, ","), fid, lineNumStr)
        if _, err := runSQLiteExec(db, upd); err != nil { return err }
    }
    // file-level tags if provided
    if len(tags) > 0 {
        for _, t := range tags {
            insTag := fmt.Sprintf("INSERT INTO file_tags(file_id, tag) VALUES (%d, '%s');", fid, escapeSQL(t))
            if _, err := runSQLiteExec(db, insTag); err != nil { return err }
        }
    }
    fmt.Println("ok")
    return nil
}

func getFileID(dbPath string, pid int, path string) (int, error) {
    q := fmt.Sprintf("SELECT id FROM files WHERE project_id=%d AND path='%s' AND type='file';", pid, escapeSQL(path))
    out, err := runSQLiteQuery(dbPath, q)
    if err != nil { return 0, err }
    out = strings.TrimSpace(out)
    if out == "" { return 0, fmt.Errorf("file not found in DB: %s", path) }
    var id int
    fmt.Sscanf(out, "%d", &id)
    return id, nil
}

func cmdFSRemove(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fmt.Println("rm — remove file/dir from DB")
        fmt.Println("Usage: heimdal rm [-r] <path>")
        return nil
    }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    recursive := false
    filtered := []string{}
    for _, a := range args { if a == "-r" { recursive = true } else { filtered = append(filtered, a) } }
    if len(filtered) < 1 { fmt.Println("Usage: heimdal rm [-r] <path>"); return nil }
    path := strings.Trim(filtered[0], "/")
    pid, err := currentProjectID(db); if err != nil { return err }
    // Check type
    q := fmt.Sprintf("SELECT type FROM files WHERE project_id=%d AND path='%s';", pid, escapeSQL(path))
    typ, err := runSQLiteQuery(db, q); if err != nil { return err }
    typ = strings.TrimSpace(typ)
    if typ == "" { return fmt.Errorf("not found: %s", path) }
    if typ == "dir" && !recursive { return errors.New("is a directory; use -r to remove recursively") }
    if typ == "file" {
        if err := deleteFileAndData(db, pid, path); err != nil { return err }
    } else {
        // recursive delete: find all under prefix
        prefix := escapeSQL(path)
        // delete lines
        _, err := runSQLiteExec(db, fmt.Sprintf("DELETE FROM file_lines WHERE file_id IN (SELECT id FROM files WHERE project_id=%d AND (path='%s' OR path LIKE '%s/%%'));", pid, prefix, prefix))
        if err != nil { return err }
        _, err = runSQLiteExec(db, fmt.Sprintf("DELETE FROM file_tags WHERE file_id IN (SELECT id FROM files WHERE project_id=%d AND (path='%s' OR path LIKE '%s/%%'));", pid, prefix, prefix))
        if err != nil { return err }
        _, err = runSQLiteExec(db, fmt.Sprintf("DELETE FROM files WHERE project_id=%d AND (path='%s' OR path LIKE '%s/%%');", pid, prefix, prefix))
        if err != nil { return err }
    }
    fmt.Println("ok")
    return nil
}

func deleteFileAndData(db string, pid int, path string) error {
    p := escapeSQL(path)
    if _, err := runSQLiteExec(db, fmt.Sprintf("DELETE FROM file_lines WHERE file_id IN (SELECT id FROM files WHERE project_id=%d AND path='%s');", pid, p)); err != nil { return err }
    if _, err := runSQLiteExec(db, fmt.Sprintf("DELETE FROM file_tags WHERE file_id IN (SELECT id FROM files WHERE project_id=%d AND path='%s');", pid, p)); err != nil { return err }
    if _, err := runSQLiteExec(db, fmt.Sprintf("DELETE FROM files WHERE project_id=%d AND path='%s';", pid, p)); err != nil { return err }
    return nil
}

func cmdFSMove(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fmt.Println("mv — move/rename file or directory in DB")
        fmt.Println("Usage: heimdal mv <src> <dst>")
        return nil
    }
    if len(args) < 2 { fmt.Println("Usage: heimdal mv <src> <dst>"); return nil }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    src := strings.Trim(args[0], "/")
    dst := strings.Trim(args[1], "/")
    pid, err := currentProjectID(db); if err != nil { return err }
    // Determine type
    typ, err := runSQLiteQuery(db, fmt.Sprintf("SELECT type FROM files WHERE project_id=%d AND path='%s';", pid, escapeSQL(src)))
    if err != nil { return err }
    typ = strings.TrimSpace(typ)
    if typ == "" { return fmt.Errorf("not found: %s", src) }
    if typ == "file" {
        // Update single file path and name
        name := filepath.Base(dst)
        _, err := runSQLiteExec(db, fmt.Sprintf("UPDATE files SET path='%s', name='%s' WHERE project_id=%d AND path='%s';", escapeSQL(dst), escapeSQL(name), pid, escapeSQL(src)))
        if err != nil { return err }
        fmt.Println("ok")
        return nil
    }
    // Directory move: update all under prefix
    // Fetch affected ids and paths
    out, err := runSQLiteQuery(db, fmt.Sprintf("SELECT id||'\t'||path FROM files WHERE project_id=%d AND (path='%s' OR path LIKE '%s/%%') ORDER BY LENGTH(path);", pid, escapeSQL(src), escapeSQL(src)))
    if err != nil { return err }
    lines := strings.Split(strings.TrimSpace(out), "\n")
    for _, line := range lines {
        if strings.TrimSpace(line) == "" { continue }
        parts := strings.SplitN(line, "\t", 2)
        if len(parts) != 2 { continue }
        var id int
        fmt.Sscanf(parts[0], "%d", &id)
        oldp := parts[1]
        var newp string
        if oldp == src { newp = dst } else {
            tail := strings.TrimPrefix(oldp, src)
            if strings.HasPrefix(tail, "/") { tail = tail[1:] }
            if tail == "" { newp = dst } else { newp = dst + "/" + tail }
        }
        name := filepath.Base(newp)
        _, err := runSQLiteExec(db, fmt.Sprintf("UPDATE files SET path='%s', name='%s' WHERE id=%d;", escapeSQL(newp), escapeSQL(name), id))
        if err != nil { return err }
    }
    fmt.Println("ok")
    return nil
}

func cmdFSCat(args []string) error {
    if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
        fmt.Println("cat — print flatfile content from DB")
        fmt.Println("Usage: heimdal cat <path/to/file>")
        return nil
    }
    db := os.Getenv("HEIMDAL_PROJECT_DB")
    if db == "" { return errors.New("not in project context: HEIMDAL_PROJECT_DB not set") }
    path := strings.Trim(args[0], "/")
    pid, err := currentProjectID(db); if err != nil { return err }
    fid, err := getFileID(db, pid, path); if err != nil { return err }
    out, err := runSQLiteQuery(db, fmt.Sprintf("SELECT content FROM file_lines WHERE file_id=%d ORDER BY lineno;", fid))
    if err != nil { return err }
    if out != "" { fmt.Print(out) }
    return nil
}

func cmdFSPwd(args []string) error {
    if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
        fmt.Println("pwd — print virtual working directory")
        fmt.Println("Usage: heimdal pwd")
        return nil
    }
    vroot := os.Getenv("HEIMDAL_VROOT")
    cwd, _ := os.Getwd()
    if vroot != "" && strings.HasPrefix(cwd, vroot) {
        rel := strings.TrimPrefix(cwd, vroot)
        if rel == "" { fmt.Println("/") } else { fmt.Println("/" + strings.TrimPrefix(rel, "/")) }
        return nil
    }
    fmt.Println(cwd)
    return nil
}

func parseMeta(s string) (tags []string, note, aicom string) {
    s = strings.TrimSpace(s)
    if s == "" { return nil, "", "" }
    // very simple parses: // note, @@tag1,tag2, ::...:: or ::NAME: Text:
    // split by spaces, but consume tokens
    for s != "" {
        if strings.HasPrefix(s, "//") {
            note = strings.TrimSpace(strings.TrimPrefix(s, "//"))
            break
        }
        if strings.HasPrefix(s, "@@") {
            t := strings.TrimSpace(strings.TrimPrefix(s, "@@"))
            for _, p := range strings.Split(t, ",") {
                p = strings.TrimSpace(p); if p!="" { tags = append(tags, p) }
            }
            break
        }
        if strings.HasPrefix(s, "::") {
            // take everything between :: and :: if present, else rest
            body := strings.TrimPrefix(s, "::")
            if i := strings.LastIndex(body, "::"); i >= 0 { body = body[:i] }
            aicom = strings.TrimSpace(body)
            break
        }
        // nothing recognized
        break
    }
    return
}

func ensureProjectRow(dbPath, root string) (int, error) {
    sql := fmt.Sprintf("INSERT OR IGNORE INTO projects(root, created_at) VALUES ('%s', datetime('now'));", escapeSQL(root))
    if _, err := runSQLiteExec(dbPath, sql); err != nil { return 0, err }
    idSQL := fmt.Sprintf("SELECT id FROM projects WHERE root='%s';", escapeSQL(root))
    out, err := runSQLiteQuery(dbPath, idSQL)
    if err != nil { return 0, err }
    out = strings.TrimSpace(out)
    if out == "" { return 0, errors.New("failed to resolve project id") }
    // parse int
    var id int
    fmt.Sscanf(out, "%d", &id)
    return id, nil
}

func currentProjectID(dbPath string) (int, error) {
    root := projectBundleDirFromDB(dbPath)
    if root == "" { root, _ = os.Getwd() }
    return ensureProjectRow(dbPath, root)
}

func upsertDir(dbPath string, pid int, path, name string, applyMeta bool, tags []string, note, aicom string) error {
    sql := fmt.Sprintf("INSERT OR IGNORE INTO files(project_id,path,name,type,created_at) VALUES (%d,'%s','%s','dir',datetime('now'));", pid, escapeSQL(strings.Trim(path, "/")), escapeSQL(name))
    if _, err := runSQLiteExec(dbPath, sql); err != nil { return err }
    if applyMeta {
        if note != "" || aicom != "" {
            upd := fmt.Sprintf("UPDATE files SET note=COALESCE(note,'' )||'%s', aicom=COALESCE(aicom,'')||'%s' WHERE project_id=%d AND path='%s';", escapeSQL(note), escapeSQL("\n"+aicom), pid, escapeSQL(strings.Trim(path, "/")))
            if _, err := runSQLiteExec(dbPath, upd); err != nil { return err }
        }
        if len(tags) > 0 {
            for _, t := range tags {
                ins := fmt.Sprintf("INSERT INTO file_tags(file_id, tag) SELECT id, '%s' FROM files WHERE project_id=%d AND path='%s';", escapeSQL(t), pid, escapeSQL(strings.Trim(path, "/")))
                if _, err := runSQLiteExec(dbPath, ins); err != nil { return err }
            }
        }
    }
    return nil
}

func upsertFile(dbPath string, pid int, path, name string, tags []string, note, aicom, content string) error {
    p := strings.Trim(path, "/")
    sql := fmt.Sprintf("INSERT OR IGNORE INTO files(project_id,path,name,type,note,aicom,created_at) VALUES (%d,'%s','%s','file','%s','%s',datetime('now'));", pid, escapeSQL(p), escapeSQL(name), escapeSQL(note), escapeSQL(aicom))
    if _, err := runSQLiteExec(dbPath, sql); err != nil { return err }
    if len(tags) > 0 {
        for _, t := range tags {
            ins := fmt.Sprintf("INSERT INTO file_tags(file_id, tag) SELECT id, '%s' FROM files WHERE project_id=%d AND path='%s';", escapeSQL(t), pid, escapeSQL(p))
            if _, err := runSQLiteExec(dbPath, ins); err != nil { return err }
        }
    }
    if content != "" {
        ins := fmt.Sprintf("INSERT INTO file_lines(file_id, lineno, content) SELECT id, 1, '%s' FROM files WHERE project_id=%d AND path='%s';", escapeSQL(content), pid, escapeSQL(p))
        if _, err := runSQLiteExec(dbPath, ins); err != nil { return err }
    }
    return nil
}

func runSQLiteExec(dbPath, sql string) (string, error) {
    if _, err := exec.LookPath("sqlite3"); err != nil { return "", errors.New("sqlite3 CLI not found") }
    cmd := exec.Command("sqlite3", dbPath)
    cmd.Stdin = bytes.NewBufferString(sql)
    b, err := cmd.CombinedOutput()
    return string(b), err
}

func runSQLiteQuery(dbPath, sql string) (string, error) {
    if _, err := exec.LookPath("sqlite3"); err != nil { return "", errors.New("sqlite3 CLI not found") }
    cmd := exec.Command("sqlite3", dbPath, sql)
    b, err := cmd.CombinedOutput()
    return string(b), err
}
