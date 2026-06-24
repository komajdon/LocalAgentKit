// Package whisper transcribes audio using the whisper.cpp CLI binary.
//
// It looks for the binary in two places (in order):
//  1. <executable_dir>/whisper      — bundled alongside the app
//  2. "whisper" on $PATH            — system install or openai-whisper CLI
//
// Audio is converted to 16 kHz mono WAV via ffmpeg before being passed
// to whisper.cpp, which expects that format.
package whisper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	transcribeTimeout = 5 * time.Minute
	downloadTimeout   = 30 * time.Minute

	modelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"
)

// ModelSizes lists supported model names ordered from fastest to most accurate.
var ModelSizes = []string{"tiny", "base", "small", "medium", "large"}

// Available returns true if the whisper binary and ffmpeg are both on PATH or
// bundled next to the executable.
func Available() bool {
	return findBinary() != "" && hasFfmpeg()
}

// Transcribe converts audioData (WebM/WAV bytes) to text.
// model is one of: tiny, base, small, medium, large (default: base).
// dataDir is the app's data directory; models are cached there.
func Transcribe(audioData []byte, model, dataDir string) (string, error) {
	if model == "" {
		model = "base"
	}
	bin := findBinary()
	if bin == "" {
		return "", fmt.Errorf("whisper binary not found; run 'make bundle-whisper' or install openai-whisper")
	}
	if !hasFfmpeg() {
		return "", fmt.Errorf("ffmpeg is required for audio conversion but was not found on PATH")
	}

	modelPath, err := ensureModel(model, dataDir)
	if err != nil {
		return "", fmt.Errorf("whisper model: %w", err)
	}

	// Write incoming audio to a temp file
	audioFile, err := os.CreateTemp("", "whisper-in-*.audio")
	if err != nil {
		return "", err
	}
	defer os.Remove(audioFile.Name())
	if _, err := audioFile.Write(audioData); err != nil {
		audioFile.Close()
		return "", err
	}
	audioFile.Close()

	// Convert to 16 kHz mono WAV (required by whisper.cpp)
	wavFile, err := os.CreateTemp("", "whisper-wav-*.wav")
	if err != nil {
		return "", err
	}
	wavFile.Close()
	defer os.Remove(wavFile.Name())

	ffCtx, ffCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ffCancel()
	ffCmd := exec.CommandContext(ffCtx, "ffmpeg",
		"-y", "-i", audioFile.Name(),
		"-ar", "16000", "-ac", "1",
		"-c:a", "pcm_s16le",
		wavFile.Name(),
	)
	if out, err := ffCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %s", strings.TrimSpace(string(out)))
	}

	// Output directory for the .txt transcript
	outDir, err := os.MkdirTemp("", "whisper-out-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(outDir)

	ctx, cancel := context.WithTimeout(context.Background(), transcribeTimeout)
	defer cancel()

	// whisper.cpp CLI flags differ from openai-whisper; detect which one we have.
	var cmd *exec.Cmd
	if isWhisperCPP(bin) {
		outBase := filepath.Join(outDir, "out")
		cmd = exec.CommandContext(ctx, bin,
			"-m", modelPath,
			"-f", wavFile.Name(),
			"-otxt",
			"-of", outBase,
			"--no-timestamps",
		)
	} else {
		// openai-whisper CLI
		cmd = exec.CommandContext(ctx, bin,
			wavFile.Name(),
			"--model_dir", filepath.Dir(modelPath),
			"--model", model,
			"--output_format", "txt",
			"--output_dir", outDir,
			"--fp16", "False",
		)
	}

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("transcription timed out after %s", transcribeTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("whisper: %s", strings.TrimSpace(string(out)))
	}

	// Read the first .txt file produced
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".txt" {
			raw, err := os.ReadFile(filepath.Join(outDir, e.Name()))
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(raw)), nil
		}
	}
	return "", fmt.Errorf("whisper produced no output file")
}

// EnsureModel downloads the GGML model to dataDir/models/ if it is not already
// present. Returns the absolute path to the model file.
func EnsureModel(model, dataDir string) (string, error) {
	return ensureModel(model, dataDir)
}

// ModelPath returns the expected on-disk path for a given model name.
func ModelPath(model, dataDir string) string {
	return filepath.Join(dataDir, "models", fmt.Sprintf("ggml-%s.bin", model))
}

// ──────────────────────────────────────────────────────────────────────────────

func ensureModel(model, dataDir string) (string, error) {
	modelsDir := filepath.Join(dataDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return "", err
	}

	// whisper.cpp uses ggml-<model>.bin; openai-whisper caches differently.
	// We always store as ggml-<model>.bin so we control the path.
	modelFile := fmt.Sprintf("ggml-%s.bin", model)
	modelPath := filepath.Join(modelsDir, modelFile)

	if _, err := os.Stat(modelPath); err == nil {
		return modelPath, nil // already downloaded
	}

	url := modelBaseURL + modelFile
	if err := downloadFile(url, modelPath); err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	return modelPath, nil
}

func downloadFile(url, dst string) error {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dst)
}

func findBinary() string {
	// 1. Bundled next to the executable
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		bundled := filepath.Join(filepath.Dir(exe), "whisper")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}
	// 2. System PATH (openai-whisper or whisper.cpp installed globally)
	for _, name := range []string{"whisper", "whisper-cpp"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	// 3. Common user-local install dirs (pip install --user, brew, etc.)
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "whisper"),
		filepath.Join(home, "go", "bin", "whisper"),
		"/usr/local/bin/whisper",
		"/opt/homebrew/bin/whisper",
		"/usr/lib/ai-agent/whisper", // shipped by the .deb / .rpm package
		"/usr/libexec/ai-agent/whisper",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func hasFfmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// isWhisperCPP tries to distinguish the whisper.cpp binary from the Python
// openai-whisper CLI by checking the help output.
func isWhisperCPP(bin string) bool {
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		// whisper.cpp exits non-zero on --help; openai-whisper does not
		return true
	}
	return !strings.Contains(string(out), "openai")
}
