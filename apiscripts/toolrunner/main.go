package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort    = 8091
	defaultTimeout = 30 * time.Second
	maxRequestBody = 16 << 10
	maxOutput      = 64 << 10
)

/*
--- Experimental functionality, dev tooling only ---

The tool runner is a small HTTP service that:
	Discovers executable files in /tools.
	Lists them through GET /dev/tools.
	Runs one through POST /dev/tools/{tool} from /work.
	Allows only one process at a time.
	Enforces argument, request-size, timeout, and output limits.
	Combines stdout and stderr into a JSON response with exit code, duration, timeout, and truncation metadata.
	Uses PORT and TOOL_TIMEOUT.
*/

// runner exposes a fixed tool registry and permits one child process at a time.
type runner struct {
	tools   map[string]string
	workDir string
	timeout time.Duration
	busy    chan struct{}
}

// runRequest contains the arguments passed directly to a selected tool.
type runRequest struct {
	Args []string `json:"args"`
}

// runResponse describes the completed child process and its combined output.
type runResponse struct {
	Tool       string   `json:"tool"`
	Args       []string `json:"args"`
	OK         bool     `json:"ok"`
	ExitCode   int      `json:"exit_code"`
	TimedOut   bool     `json:"timed_out"`
	Truncated  bool     `json:"truncated"`
	DurationMS int64    `json:"duration_ms"`
	Output     string   `json:"output"`
}

func main() {
	tools, err := discoverTools("/tools")
	if err != nil {
		log.Fatal(err)
	}

	timeout := envDuration("TOOL_TIMEOUT", defaultTimeout)
	r := &runner{
		tools:   tools,
		workDir: "/work",
		timeout: timeout,
		busy:    make(chan struct{}, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /dev/tools", r.handleList)
	mux.HandleFunc("POST /dev/tools/{tool}", r.handleRun)

	port := envInt("PORT", defaultPort)
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      timeout + 5*time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("tool runner listening on :%d with %d tools", port, len(tools))
	log.Fatal(server.ListenAndServe())
}

// discoverTools indexes regular executable files in dir by base name.
func discoverTools(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read tools directory: %w", err)
	}

	tools := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect tool %q: %w", entry.Name(), err)
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			tools[entry.Name()] = filepath.Join(dir, entry.Name())
		}
	}
	return tools, nil
}

// handleList returns the available tool names in stable order.
func (r *runner) handleList(w http.ResponseWriter, _ *http.Request) {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	slices.Sort(names)
	writeJSON(w, http.StatusOK, map[string]any{"tools": names})
}

// handleRun runs one known tool with bounded input, duration, and output.
func (r *runner) handleRun(w http.ResponseWriter, req *http.Request) {
	name := req.PathValue("tool")
	command, ok := r.tools[name]
	if !ok {
		http.Error(w, "unknown tool", http.StatusNotFound)
		return
	}

	var body runRequest
	req.Body = http.MaxBytesReader(w, req.Body, maxRequestBody)
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := validateArgs(body.Args); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	select {
	case r.busy <- struct{}{}:
		defer func() { <-r.busy }()
	default:
		http.Error(w, "another tool is running", http.StatusConflict)
		return
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(req.Context(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, body.Args...)
	cmd.Dir = r.workDir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}

	rawOutput := output.Bytes()
	truncated := len(rawOutput) > maxOutput
	if truncated {
		rawOutput = rawOutput[:maxOutput]
	}
	textOutput := strings.ToValidUTF8(string(rawOutput), "")
	if truncated {
		textOutput += "\n... output truncated ...\n"
	}

	writeJSON(w, http.StatusOK, runResponse{
		Tool:       name,
		Args:       body.Args,
		OK:         err == nil,
		ExitCode:   exitCode,
		TimedOut:   errors.Is(ctx.Err(), context.DeadlineExceeded),
		Truncated:  truncated,
		DurationMS: time.Since(started).Milliseconds(),
		Output:     textOutput,
	})
}

// validateArgs enforces the process argument limits before execution.
func validateArgs(args []string) error {
	if len(args) > 32 {
		return fmt.Errorf("at most 32 arguments are allowed")
	}
	for _, arg := range args {
		if len(arg) > 512 {
			return fmt.Errorf("each argument must be at most 512 bytes")
		}
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("arguments must not contain NUL bytes")
		}
	}
	return nil
}

// envDuration reads a positive duration or returns fallback when unset.
func envDuration(name string, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		log.Fatalf("invalid %s %q", name, raw)
	}
	return value
}

// envInt reads a valid TCP port or returns fallback when unset.
func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > 65535 {
		log.Fatalf("invalid %s %q", name, raw)
	}
	return value
}

// writeJSON writes value as a JSON response with the requested status.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
