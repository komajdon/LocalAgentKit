package main

// Speech input and file dialogs.

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-gui/internal/recorder"
	"agent-gui/internal/store"
	"agent-gui/internal/whisper"
)

// WhisperAvailable reports whether the whisper CLI is installed on PATH.
func (a *App) WhisperAvailable() bool {
	return whisper.Available()
}

// TranscribeAudio receives raw audio bytes (WebM/WAV), runs whisper on them,
// and returns the plain-text transcript.
// On first call the model (~74 MB for base) is downloaded automatically.
func (a *App) TranscribeAudio(audioBytes []byte) (string, error) {
	model := a.config().WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	// Pre-download model and emit progress if it isn't cached yet.
	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(audioBytes, model, dataDir)
}

// StartRecording begins capturing microphone audio in the background.
func (a *App) StartRecording() error {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.recSes != nil {
		return fmt.Errorf("recording already in progress")
	}
	ses, err := recorder.Start()
	if err != nil {
		return err
	}
	a.recSes = ses
	return nil
}

// StopRecording stops the microphone, runs Whisper on the captured audio,
// and returns the transcript.
func (a *App) StopRecording() (string, error) {
	a.recMu.Lock()
	ses := a.recSes
	a.recSes = nil
	a.recMu.Unlock()

	if ses == nil {
		return "", fmt.Errorf("no active recording")
	}

	wavData, err := ses.Stop()
	if err != nil {
		return "", err
	}

	model := a.config().WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(wavData, model, dataDir)
}

func (a *App) PickDirectory() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Working Directory",
	})
	if err != nil {
		return ""
	}
	return dir
}
