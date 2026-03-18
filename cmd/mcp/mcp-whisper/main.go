package main

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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	whisperBin   string
	defaultModel string
	modelsDir    string
	threads      string
	ffmpegBin    string
)

func init() {
	whisperBin = envOr("WHISPER_BIN", "./data/whisper/bin/whisper-cli")
	defaultModel = envOr("WHISPER_MODEL", "./data/whisper/models/ggml-small.bin")
	modelsDir = envOr("WHISPER_MODELS_DIR", "./data/whisper/models")
	threads = envOr("WHISPER_THREADS", "8")
	ffmpegBin = envOr("FFMPEG_BIN", "ffmpeg")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func handleTranscribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	if _, err := os.Stat(path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", path)), nil
	}

	language := req.GetString("language", "auto")
	outputFormat := req.GetString("output_format", "txt")

	args := req.GetArguments()
	translate, _ := args["translate"].(bool)

	modelPath := defaultModel
	modelName := req.GetString("model", "")
	if modelName != "" {
		modelPath = filepath.Join(modelsDir, "ggml-"+modelName+".bin")
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("model not found: %s (use download_model to get it)", modelPath)), nil
		}
	}

	// Convert non-WAV to WAV using ffmpeg
	audioPath := path
	var tmpWav string
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".wav" {
		if _, err := exec.LookPath(ffmpegBin); err != nil {
			return mcp.NewToolResultError("ffmpeg not found; only WAV files are supported without ffmpeg"), nil
		}
		tmpWav = filepath.Join(os.TempDir(), fmt.Sprintf("whisper_%d.wav", time.Now().UnixNano()))
		cmd := exec.Command(ffmpegBin, "-i", path, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-y", tmpWav)
		if out, err := cmd.CombinedOutput(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("ffmpeg conversion failed: %s\n%s", err, string(out))), nil
		}
		audioPath = tmpWav
		defer os.Remove(tmpWav)
	}

	// Build whisper-cli command
	cmdArgs := []string{
		"-m", modelPath,
		"-f", audioPath,
		"-t", threads,
		"-np", // no progress
	}

	if language != "auto" {
		cmdArgs = append(cmdArgs, "-l", language)
	}

	if translate {
		cmdArgs = append(cmdArgs, "-tr")
	}

	switch outputFormat {
	case "srt":
		cmdArgs = append(cmdArgs, "-osrt")
	case "vtt":
		cmdArgs = append(cmdArgs, "-ovtt")
	case "json":
		cmdArgs = append(cmdArgs, "-oj")
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(execCtx, whisperBin, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return mcp.NewToolResultError("transcription timed out"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("whisper-cli failed: %s\n%s", err, string(output))), nil
	}

	// For srt/vtt/json, whisper-cli writes output files next to input
	if outputFormat != "txt" {
		outExt := "." + outputFormat
		// whisper-cli creates files like: audioPath + ".srt"
		outFile := audioPath + outExt
		if data, err := os.ReadFile(outFile); err == nil {
			defer os.Remove(outFile)
			result := strings.TrimSpace(string(data))
			if result == "" {
				return mcp.NewToolResultText("No speech detected."), nil
			}
			return mcp.NewToolResultText(result), nil
		}
		// Fallback: check if output was written to stdout
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return mcp.NewToolResultText("No speech detected."), nil
	}

	return mcp.NewToolResultText(result), nil
}

func handleListModels(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText("No models directory found. Run 'make setup-whisper' to set up."), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to read models directory: %s", err)), nil
	}

	var models []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "ggml-") || !strings.HasSuffix(name, ".bin") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		sizeMB := info.Size() / (1024 * 1024)
		modelName := strings.TrimPrefix(name, "ggml-")
		modelName = strings.TrimSuffix(modelName, ".bin")
		models = append(models, fmt.Sprintf("- %s (%d MB)", modelName, sizeMB))
	}

	if len(models) == 0 {
		return mcp.NewToolResultText("No models found in " + modelsDir + ". Use download_model to get one."), nil
	}

	return mcp.NewToolResultText("Available models:\n" + strings.Join(models, "\n")), nil
}

func handleDownloadModel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	model, err := req.RequireString("model")
	if err != nil {
		return mcp.NewToolResultError("model is required"), nil
	}

	validModels := map[string]bool{
		"tiny": true, "base": true, "small": true, "medium": true, "large-v3": true,
	}
	if !validModels[model] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid model: %s. Valid: tiny, base, small, medium, large-v3", model)), nil
	}

	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create models directory: %s", err)), nil
	}

	filename := fmt.Sprintf("ggml-%s.bin", model)
	destPath := filepath.Join(modelsDir, filename)

	// Check if already exists
	if info, err := os.Stat(destPath); err == nil {
		sizeMB := info.Size() / (1024 * 1024)
		return mcp.NewToolResultText(fmt.Sprintf("Model %s already exists (%d MB): %s", model, sizeMB, destPath)), nil
	}

	url := fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", model)

	// Download to temp file first for atomicity
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath) // cleanup on error

	resp, err := http.Get(url)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: %s", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: HTTP %d", resp.StatusCode)), nil
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create temp file: %s", err)), nil
	}

	written, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("download interrupted: %s", err)), nil
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to finalize download: %s", err)), nil
	}

	sizeMB := written / (1024 * 1024)
	return mcp.NewToolResultText(fmt.Sprintf("Downloaded model %s (%d MB) to %s", model, sizeMB, destPath)), nil
}

func main() {
	// Verify whisper-cli exists
	if _, err := os.Stat(whisperBin); err != nil {
		fmt.Fprintf(os.Stderr, "whisper-cli not found at %s — run 'make setup-whisper' first\n", whisperBin)
		os.Exit(1)
	}

	s := server.NewMCPServer("mcp-whisper", "0.1.0")

	s.AddTool(
		mcp.NewTool("transcribe_audio",
			mcp.WithDescription("Transcribe an audio file to text using whisper.cpp. Supports WAV, MP3, FLAC, OGG, M4A (non-WAV formats require ffmpeg). Returns transcription text or subtitles."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the audio file."),
			),
			mcp.WithString("language",
				mcp.Description("Language code (e.g. 'ru', 'en', 'de') or 'auto' for auto-detection. Default: 'auto'."),
			),
			mcp.WithString("output_format",
				mcp.Description("Output format: 'txt' (plain text), 'srt' (subtitles), 'vtt' (WebVTT), 'json' (timestamped). Default: 'txt'."),
				mcp.Enum("txt", "srt", "vtt", "json"),
			),
			mcp.WithBoolean("translate",
				mcp.Description("Translate to English instead of transcribing in original language."),
			),
			mcp.WithString("model",
				mcp.Description("Model name to use (e.g. 'small', 'large-v3'). Overrides default model. Must be downloaded first."),
			),
		),
		handleTranscribe,
	)

	s.AddTool(
		mcp.NewTool("list_models",
			mcp.WithDescription("List available whisper models in the models directory."),
		),
		handleListModels,
	)

	s.AddTool(
		mcp.NewTool("download_model",
			mcp.WithDescription("Download a whisper model from Hugging Face. Models: tiny (~75MB), base (~142MB), small (~466MB), medium (~1.5GB), large-v3 (~3.1GB)."),
			mcp.WithString("model",
				mcp.Required(),
				mcp.Description("Model to download."),
				mcp.Enum("tiny", "base", "small", "medium", "large-v3"),
			),
		),
		handleDownloadModel,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %s\n", err)
		os.Exit(1)
	}
}
