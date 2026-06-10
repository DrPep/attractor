package tools

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// CommandResult captures the outcome of a shell command.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// ExecutionEnvironment is where tools run: local, container, etc.
type ExecutionEnvironment interface {
	WorkingDirectory() string
	RunCommand(ctx context.Context, command string, timeout time.Duration) CommandResult
	ReadFile(path string) (string, error)
	WriteFile(path, content string) error
	ListFiles(pattern, basePath string) ([]string, error)
	FileExists(path string) bool
}

// LocalEnvironment executes tools on the local machine via /bin/sh.
type LocalEnvironment struct {
	workingDir string
}

// NewLocalEnvironment returns an environment rooted at workingDir (cwd if empty).
func NewLocalEnvironment(workingDir string) *LocalEnvironment {
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}
	return &LocalEnvironment{workingDir: workingDir}
}

// WorkingDirectory returns the environment's root directory.
func (e *LocalEnvironment) WorkingDirectory() string { return e.workingDir }

// RunCommand runs a shell command with a timeout.
func (e *LocalEnvironment) RunCommand(ctx context.Context, command string, timeout time.Duration) CommandResult {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", command)
	cmd.Dir = e.workingDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if cctx.Err() == context.DeadlineExceeded {
		return CommandResult{Stderr: "Command timed out", ExitCode: -1, TimedOut: true}
	}
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return CommandResult{Stderr: err.Error(), ExitCode: -1}
		}
	}
	return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

// ReadFile reads a file relative to the working directory.
func (e *LocalEnvironment) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(e.resolve(path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes a file relative to the working directory, creating parents.
func (e *LocalEnvironment) WriteFile(path, content string) error {
	resolved := e.resolve(path)
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resolved, []byte(content), 0o644)
}

// ListFiles returns files matching a glob pattern under basePath.
func (e *LocalEnvironment) ListFiles(pattern, basePath string) ([]string, error) {
	base := e.workingDir
	if basePath != "" {
		if filepath.IsAbs(basePath) {
			base = basePath
		} else {
			base = filepath.Join(e.workingDir, basePath)
		}
	}
	matches, err := filepath.Glob(filepath.Join(base, pattern))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && !info.IsDir() {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	return files, nil
}

// FileExists reports whether a path exists relative to the working directory.
func (e *LocalEnvironment) FileExists(path string) bool {
	_, err := os.Stat(e.resolve(path))
	return err == nil
}

func (e *LocalEnvironment) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.workingDir, path)
}
