// Package recorder captures microphone audio via arecord (ALSA) or
// pw-record (PipeWire) and returns raw WAV bytes.
package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Session represents one active recording.
type Session struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	outFile string
	done    chan struct{}
	err     error
}

// Start begins recording to a temp WAV file.
// It returns immediately; the recording runs in the background.
func Start() (*Session, error) {
	f, err := os.CreateTemp("", "agent-rec-*.wav")
	if err != nil {
		return nil, err
	}
	f.Close()

	recorder, args := buildCmd(f.Name())
	cmd := exec.Command(recorder, args...)

	s := &Session{
		cmd:     cmd,
		outFile: f.Name(),
		done:    make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		os.Remove(f.Name())
		return nil, fmt.Errorf("start recorder: %w", err)
	}

	go func() {
		s.err = cmd.Wait()
		close(s.done)
	}()

	return s, nil
}

// Stop terminates the recording and returns the captured WAV bytes.
func (s *Session) Stop() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd.Process != nil {
		// SIGINT tells arecord/pw-record to flush and close the WAV header.
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
	<-s.done
	defer os.Remove(s.outFile)

	data, err := os.ReadFile(s.outFile)
	if err != nil {
		return nil, err
	}
	if len(data) < 44 { // WAV header is 44 bytes
		return nil, fmt.Errorf("recording too short or empty")
	}
	return data, nil
}

// buildCmd returns the best available recording command.
// Records 16-bit mono PCM at 16 kHz — exactly what whisper.cpp expects.
func buildCmd(outFile string) (string, []string) {
	if p, err := exec.LookPath("arecord"); err == nil {
		return p, []string{
			"-f", "S16_LE",  // 16-bit signed little-endian
			"-r", "16000",   // 16 kHz
			"-c", "1",       // mono
			"-t", "wav",
			outFile,
		}
	}
	// PipeWire fallback
	return "pw-record", []string{
		"--rate=16000",
		"--channels=1",
		"--format=s16",
		outFile,
	}
}
