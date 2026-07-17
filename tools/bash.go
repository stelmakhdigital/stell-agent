package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxBashOutput = 64 * 1024
	bashTimeout   = 60 * time.Second
)

// bashHeartbeatInterval — интервал heartbeat для команд без вывода;
// переменная, чтобы тесты могли сократить интервал.
var bashHeartbeatInterval = 5 * time.Second

// filesystemScanRe — команды сканирования корня ФС (например `find /`, `find /*`,
// `grep -r pat /`): выполняются минутами, упираются в bash timeout и выглядят как зависание.
// Локальные модели генерируют их при «галлюцинации» путей вне workspace.
var filesystemScanRe = regexp.MustCompile(`(?:^|[;&|(]\s*)(?:find|grep\s+-[a-zA-Z]*[rR][a-zA-Z]*(?:\s+\S+)*?)\s+/(?:\s|$|\*)`)

// rejectFilesystemScan блокирует сканирование всей ФС до запуска с ошибкой,
// возвращающей модель в workspace.
func rejectFilesystemScan(workspace, command string) error {
	if filesystemScanRe.MatchString(command) {
		return fmt.Errorf("refusing to scan the filesystem root: use paths inside the workspace (%s), e.g. `find . -name ...`", workspace)
	}
	return nil
}

// BashResult — результат выполнения shell-команды через Runtime.RunBash.
type BashResult struct {
	Output         string
	ExitCode       int
	Truncated      bool
	Cancelled      bool
	FullOutputPath string
}

// RunBash выполняет команду в workspace с учётом trust/deny и лимитов вывода.
func (r *Runtime) RunBash(ctx context.Context, command string) (BashResult, error) {
	if command == "" {
		return BashResult{}, fmt.Errorf("command required")
	}
	if err := bashAllowed(&r.env); err != nil {
		return BashResult{}, err
	}
	return runBashCommand(ctx, r.env.Workspace, command, ProgressFrom(ctx))
}

func runBashCommand(ctx context.Context, workspace, command string, onProgress ProgressFunc) (BashResult, error) {
	cctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	cmd.Dir = workspace

	var mu sync.Mutex
	var buf strings.Builder
	writer := &progressWriter{fn: func(s string) {
		if onProgress != nil {
			onProgress(s)
		}
	}, buf: &buf, mu: &mu}

	// Долгие «тихие» команды не дают progress; heartbeat с elapsed time,
	// чтобы UI не выглядел зависшим.
	if onProgress != nil {
		start := time.Now()
		heartbeatDone := make(chan struct{})
		defer close(heartbeatDone)
		go func() {
			tick := time.NewTicker(bashHeartbeatInterval)
			defer tick.Stop()
			for {
				select {
				case <-heartbeatDone:
					return
				case <-tick.C:
					mu.Lock()
					out := strings.TrimRight(buf.String(), "\n")
					mu.Unlock()
					elapsed := int(time.Since(start).Seconds())
					marker := fmt.Sprintf("[running %ds / %ds]", elapsed, int(bashTimeout.Seconds()))
					if out == "" {
						onProgress(marker)
					} else {
						onProgress(out + "\n" + marker)
					}
				}
			}
		}()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return BashResult{}, err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(writer, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(writer, stderr)
	}()

	runErr := cmd.Wait()
	wg.Wait()

	if errors.Is(cctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		mu.Lock()
		out := strings.TrimRight(buf.String(), "\n")
		mu.Unlock()
		return BashResult{Output: out, Cancelled: true, ExitCode: -1}, nil
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	mu.Lock()
	full := strings.TrimRight(buf.String(), "\n")
	mu.Unlock()

	content := full
	truncated := false
	var fullPath string
	if len(content) > maxBashOutput {
		truncated = true
		f, err := os.CreateTemp("", "stell-bash-*.log")
		if err == nil {
			_, _ = f.WriteString(full)
			fullPath = f.Name()
			_ = f.Close()
		}
		content = content[:maxBashOutput] + "\n… (truncated)"
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		content += fmt.Sprintf("\n[timed out after %ds and was killed; narrow the command scope]", int(bashTimeout.Seconds()))
	} else if runErr != nil && !truncated {
		content += fmt.Sprintf("\n[exit error: %v]", runErr)
	}
	content = strings.TrimPrefix(content, "\n")
	if content == "" {
		content = "(no output)"
	}
	_ = filepath.Base(fullPath)
	return BashResult{
		Output:         content,
		ExitCode:       exitCode,
		Truncated:      truncated,
		FullOutputPath: fullPath,
	}, nil
}

type progressWriter struct {
	fn  func(string)
	buf *strings.Builder
	mu  *sync.Mutex
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	if w.fn != nil {
		w.fn(w.buf.String())
	}
	w.mu.Unlock()
	return n, err
}
