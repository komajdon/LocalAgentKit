// Package sandbox builds shell commands with the strongest isolation the
// platform offers.
//
// Backends:
//   - Linux: bubblewrap. Only system directories (read-only) and the work
//     directory (read-write) are visible.
//   - macOS: sandbox-exec. Writes are limited to the work directory and temp
//     directories, and the home directory is unreadable outside the work
//     directory. Other system paths stay readable.
//   - Windows and anything else: no isolation.
//
// Every command, isolated or not, gets a scrubbed environment and a bounded
// output buffer, and its process tree is killed on cancellation.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Mode selects how isolation is applied.
type Mode string

const (
	// ModeAuto isolates when a backend works and otherwise runs unisolated.
	ModeAuto Mode = "auto"
	// ModeStrict refuses to run when isolation is unavailable.
	ModeStrict Mode = "strict"
	// ModeOff never isolates.
	ModeOff Mode = "off"
)

// Options configures command isolation.
type Options struct {
	Mode         Mode
	WorkDir      string
	AllowNetwork bool     // only meaningful when isolated
	Hidden       []string // directories masked even if inside WorkDir
}

// Status describes the isolation that will actually be applied.
type Status struct {
	Backend        string `json:"backend"` // bubblewrap | sandbox-exec | none
	Isolated       bool   `json:"isolated"`
	NetworkBlocked bool   `json:"network_blocked"`
	Detail         string `json:"detail"`
}

// String is a one-line description shown with command output.
func (s Status) String() string {
	if !s.Isolated {
		return "isolation: none (" + s.Detail + ")"
	}
	net := "network allowed"
	if s.NetworkBlocked {
		net = "network blocked"
	}
	return "isolation: " + s.Backend + ", " + net
}

// ErrUnavailable reports that strict isolation was requested but cannot be
// provided.
var ErrUnavailable = errors.New("strict sandbox requested but no isolation backend is available")

// findBackend is replaced by tests to simulate a platform without isolation.
var findBackend = platformBackend

type backend struct {
	name string
	// wrap returns the program and arguments that run argv isolated.
	wrap func(o Options, dir string, argv []string) (string, []string, error)
}

// Probe reports the isolation that Command would apply.
func Probe(o Options) Status {
	if o.Mode == ModeOff {
		return Status{Backend: "none", Detail: "sandbox turned off in Settings"}
	}
	b, err := findBackend()
	if err != nil {
		return Status{Backend: "none", Detail: err.Error()}
	}
	return Status{Backend: b.name, Isolated: true, NetworkBlocked: !o.AllowNetwork}
}

// Command prepares `bash -c command` to run in dir. It fails closed: in strict
// mode it returns ErrUnavailable rather than an unisolated command.
func Command(ctx context.Context, o Options, dir, command string) (*exec.Cmd, Status, error) {
	// Anything other than an explicit "off" or "auto" is treated as strict.
	if o.Mode != ModeOff && o.Mode != ModeAuto {
		o.Mode = ModeStrict
	}
	status := Probe(o)
	if !status.Isolated && o.Mode == ModeStrict {
		return nil, status, fmt.Errorf("%w: %s", ErrUnavailable, status.Detail)
	}

	argv := []string{"bash", "-c", limits() + command}
	name, args := argv[0], argv[1:]
	if status.Isolated {
		b, err := findBackend()
		if err != nil {
			return nil, status, err
		}
		if name, args, err = b.wrap(o, dir, argv); err != nil {
			return nil, status, err
		}
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = Environment(os.Environ())
	setProcessTree(cmd)
	// Do not wait forever for a surviving child that holds the output pipe.
	cmd.WaitDelay = 2 * time.Second
	return cmd, status, nil
}

// limits is prepended to every command on Unix: no core dumps, at most 10
// minutes of CPU time per process and 2 GiB per written file. Limits that the
// platform rejects are ignored.
func limits() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "{ ulimit -c 0; ulimit -t 600; ulimit -f 2097152; } 2>/dev/null\n"
}

// allowedEnv lists the variables a command may inherit. Everything else,
// including API keys and tokens in the parent environment, is dropped.
var allowedEnv = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true,
	"LANGUAGE": true, "TERM": true, "TZ": true, "TMPDIR": true, "SHELL": true,
	// Windows needs these to start programs at all.
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "COMSPEC": true, "PATHEXT": true,
	"TEMP": true, "TMP": true, "USERPROFILE": true, "WINDIR": true,
}

// Environment filters env down to the allowed variables.
func Environment(env []string) []string {
	out := []string{}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if allowedEnv[upper] || strings.HasPrefix(upper, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// Output is an io.Writer that keeps at most Max bytes.
type Output struct {
	Max       int
	buf       []byte
	truncated bool
}

// Write keeps what fits and reports the whole input as consumed, so the
// command is not killed by a short write.
func (o *Output) Write(p []byte) (int, error) {
	room := o.Max - len(o.buf)
	if len(p) > room {
		o.truncated = true
		o.buf = append(o.buf, p[:max(room, 0)]...)
	} else {
		o.buf = append(o.buf, p...)
	}
	return len(p), nil
}

// String returns the captured output, noting any truncation.
func (o *Output) String() string {
	if o.truncated {
		return string(o.buf) + "\n[... output truncated ...]"
	}
	return string(o.buf)
}
