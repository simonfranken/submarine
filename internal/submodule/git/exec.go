package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// gitEnv keeps git non-interactive, lock-free for reads and its output
// parseable.
var gitEnv = append(os.Environ(),
	"GIT_TERMINAL_PROMPT=0",
	"GIT_OPTIONAL_LOCKS=0",
	"LC_ALL=C",
)

// gitError is a failed git command; Stderr is git's message.
type gitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *gitError) Error() string {
	msg := e.Stderr
	if msg == "" {
		msg = e.Err.Error()
	}
	return "git " + strings.Join(e.Args, " ") + ": " + msg
}

func (e *gitError) Unwrap() error { return e.Err }

// run executes git in dir and returns stdout with the trailing newline
// removed. The error includes git's stderr.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runEnv(ctx, dir, nil, args...)
}

// runEnv is run with additional environment variables.
func runEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = slices.Concat(gitEnv, env) // a fresh slice: run is called concurrently
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &gitError{Args: args, Stderr: strings.TrimSpace(stderr.String()), Err: err}
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// try is run for commands whose failure only means "no answer", e.g.
// "no upstream" or "ref does not exist". Context errors still propagate.
func try(ctx context.Context, dir string, args ...string) (string, bool, error) {
	out, err := run(ctx, dir, args...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", false, err
		}
		return "", false, nil
	}
	return out, true, nil
}
