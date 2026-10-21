package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitEnv keeps git non-interactive, lock-free for reads and its output
// parseable.
var gitEnv = append(os.Environ(),
	"GIT_TERMINAL_PROMPT=0",
	"GIT_OPTIONAL_LOCKS=0",
	"LC_ALL=C",
)

// run executes git in dir and returns stdout with the trailing newline
// removed. The error includes git's stderr.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
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
