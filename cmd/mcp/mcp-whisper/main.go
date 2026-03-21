package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// transcribeResult holds the result of transcribing a single file.
type transcribeResult struct {
	SRTPath string // path to .srt file on disk (empty if txt format or error)
	Text    string // transcription text
	Err     error
}

// transcribeOne transcribes a single audio file and returns the result.
// For srt/vtt/json formats, the output file is kept on disk next to the input.
func transcribeOne(ctx context.Context, path, language, outputFormat, modelPath string, translate bool) transcribeResult {
	if _, err := os.Stat(path); err != nil {
		return transcribeResult{Err: fmt.Errorf("file not found: %s", path)}
	}

	// Convert non-WAV to WAV using ffmpeg
	audioPath := path
	var tmpWav string
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".wav" {
		if _, err := exec.LookPath(ffmpegBin); err != nil {
			return transcribeResult{Err: fmt.Errorf("ffmpeg not found; only WAV supported without ffmpeg")}
		}
		tmpWav = filepath.Join(os.TempDir(), fmt.Sprintf("whisper_%d.wav", time.Now().UnixNano()))
		cmd := exec.Command(ffmpegBin, "-i", path, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-y", tmpWav)
		if out, err := cmd.CombinedOutput(); err != nil {
			return transcribeResult{Err: fmt.Errorf("ffmpeg failed: %s\n%s", err, string(out))}
		}
		audioPath = tmpWav
		defer os.Remove(tmpWav)
	}

	cmdArgs := []string{
		"-m", modelPath,
		"-f", audioPath,
		"-t", threads,
		"-np",
	}

	// Always pass -l flag. whisper-cli defaults to English without it.
	cmdArgs = append(cmdArgs, "-l", language)

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

	execCtx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(execCtx, whisperBin, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return transcribeResult{Err: fmt.Errorf("transcription timed out")}
		}
		return transcribeResult{Err: fmt.Errorf("whisper-cli failed: %s\n%s", err, string(output))}
	}

	// For srt/vtt/json, whisper-cli writes output files next to input
	if outputFormat != "txt" {
		outExt := "." + outputFormat
		outFile := audioPath + outExt
		if data, err := os.ReadFile(outFile); err == nil {
			result := strings.TrimSpace(string(data))
			if result == "" {
				os.Remove(outFile)
				return transcribeResult{Text: "No speech detected."}
			}

			// If input was converted from non-wav, move SRT next to original.
			finalPath := outFile
			if tmpWav != "" {
				finalPath = strings.TrimSuffix(path, filepath.Ext(path)) + outExt
				if err := os.Rename(outFile, finalPath); err != nil {
					if err := copyFile(outFile, finalPath); err != nil {
						os.Remove(outFile)
						return transcribeResult{Text: result}
					}
					os.Remove(outFile)
				}
			}

			return transcribeResult{SRTPath: finalPath, Text: result}
		}
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return transcribeResult{Text: "No speech detected."}
	}
	return transcribeResult{Text: result}
}

func handleTranscribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	language := req.GetString("language", "auto")
	outputFormat := req.GetString("output_format", "txt")
	args := req.GetArguments()
	translate, _ := args["translate"].(bool)

	modelPath := defaultModel
	if modelName := req.GetString("model", ""); modelName != "" {
		modelPath = filepath.Join(modelsDir, "ggml-"+modelName+".bin")
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("model not found: %s", modelPath)), nil
		}
	}

	r := transcribeOne(ctx, path, language, outputFormat, modelPath, translate)
	if r.Err != nil {
		return mcp.NewToolResultError(r.Err.Error()), nil
	}

	if r.SRTPath != "" {
		return mcp.NewToolResultText(fmt.Sprintf("file:%s\n%s", r.SRTPath, r.Text)), nil
	}
	return mcp.NewToolResultText(r.Text), nil
}

// handleBatchTranscribe transcribes multiple audio files sequentially in one tool call.
// Returns per-file results as JSON array. This saves tokens by avoiding N round-trips.
func handleBatchTranscribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	filesRaw, ok := args["files"].([]any)
	if !ok || len(filesRaw) == 0 {
		return mcp.NewToolResultError("'files' array is required and must not be empty"), nil
	}

	outputFormat := req.GetString("output_format", "srt")
	language := req.GetString("language", "auto")

	modelPath := defaultModel
	if modelName := req.GetString("model", ""); modelName != "" {
		modelPath = filepath.Join(modelsDir, "ggml-"+modelName+".bin")
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("model not found: %s", modelPath)), nil
		}
	}

	type fileResult struct {
		Path    string `json:"path"`
		SRTPath string `json:"srt_path,omitempty"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}

	// Determine concurrency: WHISPER_WORKERS env or default 4.
	workers := 4
	if w, err := strconv.Atoi(os.Getenv("WHISPER_WORKERS")); err == nil && w > 0 {
		workers = w
	}

	// Pre-filter: separate skipped from work items, preserving original order.
	type workItem struct {
		index int
		path  string
	}
	results := make([]fileResult, len(filesRaw))
	var toProcess []workItem
	skipped := 0

	for i, item := range filesRaw {
		path, _ := item.(string)
		if path == "" {
			results[i] = fileResult{Status: "error", Error: "empty path"}
			continue
		}
		// Skip files that already have an output file on disk (idempotent).
		if outputFormat != "txt" {
			outFile := path + "." + outputFormat
			if _, err := os.Stat(outFile); err == nil {
				results[i] = fileResult{Path: path, SRTPath: outFile, Status: "skipped"}
				skipped++
				continue
			}
		}
		toProcess = append(toProcess, workItem{index: i, path: path})
	}

	// Process files in parallel using a worker pool.
	jobs := make(chan workItem, len(toProcess))
	var wg sync.WaitGroup

	for range min(workers, len(toProcess)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				r := transcribeOne(ctx, job.path, language, outputFormat, modelPath, false)
				if r.Err != nil {
					results[job.index] = fileResult{Path: job.path, Status: "error", Error: r.Err.Error()}
				} else {
					results[job.index] = fileResult{Path: job.path, SRTPath: r.SRTPath, Status: "ok"}
				}
			}
		}()
	}
	for _, item := range toProcess {
		jobs <- item
	}
	close(jobs)
	wg.Wait()

	success, failed := 0, 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			success++
		case "error":
			failed++
		}
	}

	summary := map[string]any{
		"total":   len(filesRaw),
		"success": success,
		"skipped": skipped,
		"failed":  failed,
		"files":   results,
	}

	data, _ := json.Marshal(summary)
	return mcp.NewToolResultText(string(data)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
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
		"tiny": true, "base": true, "small": true, "medium": true, "large-v3": true, "large-v3-turbo": true,
	}
	if !validModels[model] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid model: %s. Valid: tiny, base, small, medium, large-v3, large-v3-turbo", model)), nil
	}

	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create models directory: %s", err)), nil
	}

	filename := fmt.Sprintf("ggml-%s.bin", model)
	destPath := filepath.Join(modelsDir, filename)

	if info, err := os.Stat(destPath); err == nil {
		sizeMB := info.Size() / (1024 * 1024)
		return mcp.NewToolResultText(fmt.Sprintf("Model %s already exists (%d MB): %s", model, sizeMB, destPath)), nil
	}

	url := fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", model)

	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

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
	if _, err := os.Stat(whisperBin); err != nil {
		fmt.Fprintf(os.Stderr, "whisper-cli not found at %s — run 'make setup-whisper' first\n", whisperBin)
		os.Exit(1)
	}

	s := server.NewMCPServer("mcp-whisper", "0.1.0")

	s.AddTool(
		mcp.NewTool("transcribe_audio",
			mcp.WithDescription("Transcribe an audio file to text using whisper.cpp. Supports WAV, MP3, FLAC, OGG, M4A (non-WAV formats require ffmpeg). Returns transcription text. For srt/vtt/json formats, the output file is saved next to the input and the path is returned as 'file:<path>' in the first line."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Path to the audio file.")),
			mcp.WithString("language", mcp.Description("Language code (e.g. 'ru', 'en') or 'auto'. Default: 'auto'.")),
			mcp.WithString("output_format", mcp.Description("Output format: txt, srt, vtt, json. Default: txt."), mcp.Enum("txt", "srt", "vtt", "json")),
			mcp.WithBoolean("translate", mcp.Description("Translate to English instead of transcribing in original language.")),
			mcp.WithString("model", mcp.Description("Model name to use. Must be downloaded first.")),
		),
		handleTranscribe,
	)

	s.AddTool(
		mcp.NewTool("batch_transcribe",
			mcp.WithDescription("Transcribe multiple audio files in one call. Processes sequentially, saves .srt files next to each input file. Returns JSON with per-file results including srt_path. Much more efficient than calling transcribe_audio repeatedly — use this for bulk transcription."),
			mcp.WithArray("files", mcp.Required(), mcp.Description("Array of file paths (strings) to transcribe.")),
			mcp.WithString("output_format", mcp.Description("Output format for all files. Default: srt."), mcp.Enum("txt", "srt", "vtt", "json")),
			mcp.WithString("language", mcp.Description("Language code or 'auto'. Default: 'auto'.")),
			mcp.WithString("model", mcp.Description("Model name to use. Must be downloaded first.")),
		),
		handleBatchTranscribe,
	)

	s.AddTool(
		mcp.NewTool("list_models",
			mcp.WithDescription("List available whisper models in the models directory."),
		),
		handleListModels,
	)

	s.AddTool(
		mcp.NewTool("download_model",
			mcp.WithDescription("Download a whisper model from Hugging Face. Models: tiny (~75MB), base (~142MB), small (~466MB), medium (~1.5GB), large-v3 (~3.1GB), large-v3-turbo (~1.6GB)."),
			mcp.WithString("model", mcp.Required(), mcp.Description("Model to download."), mcp.Enum("tiny", "base", "small", "medium", "large-v3", "large-v3-turbo")),
		),
		handleDownloadModel,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %s\n", err)
		os.Exit(1)
	}
}
