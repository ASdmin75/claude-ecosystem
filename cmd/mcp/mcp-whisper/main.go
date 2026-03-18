package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var tools = []tool{
	{
		Name:        "transcribe_audio",
		Description: "Transcribe an audio file to text using whisper.cpp. Supports WAV, MP3, FLAC, OGG, M4A (non-WAV formats require ffmpeg). Returns transcription text or subtitles.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the audio file.",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Language code (e.g. 'ru', 'en', 'de') or 'auto' for auto-detection. Default: 'auto'.",
				},
				"output_format": map[string]any{
					"type":        "string",
					"description": "Output format: 'txt' (plain text), 'srt' (subtitles), 'vtt' (WebVTT), 'json' (timestamped). Default: 'txt'.",
					"enum":        []string{"txt", "srt", "vtt", "json"},
				},
				"translate": map[string]any{
					"type":        "boolean",
					"description": "Translate to English instead of transcribing in original language.",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model name to use (e.g. 'small', 'large-v3'). Overrides default model. Must be downloaded first.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "list_models",
		Description: "List available whisper models in the models directory.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "download_model",
		Description: "Download a whisper model from Hugging Face. Models: tiny (~75MB), base (~142MB), small (~466MB), medium (~1.5GB), large-v3 (~3.1GB).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"model": map[string]any{
					"type":        "string",
					"description": "Model to download.",
					"enum":        []string{"tiny", "base", "small", "medium", "large-v3"},
				},
			},
			"required": []string{"model"},
		},
	},
}

var (
	whisperBin string
	defaultModel string
	modelsDir  string
	threads    string
	ffmpegBin  string
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

func textResult(text string) any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func errorResult(text string) any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": "Error: " + text},
		},
		"isError": true,
	}
}

func handleToolCall(params map[string]any) (any, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "transcribe_audio":
		return handleTranscribe(args)
	case "list_models":
		return handleListModels()
	case "download_model":
		return handleDownloadModel(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func handleTranscribe(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	language, _ := args["language"].(string)
	if language == "" {
		language = "auto"
	}

	outputFormat, _ := args["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "txt"
	}

	translate, _ := args["translate"].(bool)

	modelPath := defaultModel
	if modelName, ok := args["model"].(string); ok && modelName != "" {
		modelPath = filepath.Join(modelsDir, "ggml-"+modelName+".bin")
		if _, err := os.Stat(modelPath); err != nil {
			return nil, fmt.Errorf("model not found: %s (use download_model to get it)", modelPath)
		}
	}

	// Convert non-WAV to WAV using ffmpeg
	audioPath := path
	var tmpWav string
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".wav" {
		if _, err := exec.LookPath(ffmpegBin); err != nil {
			return nil, fmt.Errorf("ffmpeg not found; only WAV files are supported without ffmpeg")
		}
		tmpWav = filepath.Join(os.TempDir(), fmt.Sprintf("whisper_%d.wav", time.Now().UnixNano()))
		cmd := exec.Command(ffmpegBin, "-i", path, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-y", tmpWav)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg conversion failed: %s\n%s", err, string(out))
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, whisperBin, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("transcription timed out")
		}
		return nil, fmt.Errorf("whisper-cli failed: %s\n%s", err, string(output))
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
				return textResult("No speech detected."), nil
			}
			return textResult(result), nil
		}
		// Fallback: check if output was written to stdout
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return textResult("No speech detected."), nil
	}

	return textResult(result), nil
}

func handleListModels() (any, error) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return textResult("No models directory found. Run 'make setup-whisper' to set up."), nil
		}
		return nil, fmt.Errorf("failed to read models directory: %w", err)
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
		return textResult("No models found in " + modelsDir + ". Use download_model to get one."), nil
	}

	return textResult("Available models:\n" + strings.Join(models, "\n")), nil
}

func handleDownloadModel(args map[string]any) (any, error) {
	model, _ := args["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	validModels := map[string]bool{
		"tiny": true, "base": true, "small": true, "medium": true, "large-v3": true,
	}
	if !validModels[model] {
		return nil, fmt.Errorf("invalid model: %s. Valid: tiny, base, small, medium, large-v3", model)
	}

	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	filename := fmt.Sprintf("ggml-%s.bin", model)
	destPath := filepath.Join(modelsDir, filename)

	// Check if already exists
	if info, err := os.Stat(destPath); err == nil {
		sizeMB := info.Size() / (1024 * 1024)
		return textResult(fmt.Sprintf("Model %s already exists (%d MB): %s", model, sizeMB, destPath)), nil
	}

	url := fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", model)

	// Download to temp file first for atomicity
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath) // cleanup on error

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	written, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return nil, fmt.Errorf("download interrupted: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return nil, fmt.Errorf("failed to finalize download: %w", err)
	}

	sizeMB := written / (1024 * 1024)
	return textResult(fmt.Sprintf("Downloaded model %s (%d MB) to %s", model, sizeMB, destPath)), nil
}

func main() {
	// Verify whisper-cli exists
	if _, err := os.Stat(whisperBin); err != nil {
		fmt.Fprintf(os.Stderr, "whisper-cli not found at %s — run 'make setup-whisper' first\n", whisperBin)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		var resp jsonRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo":     map[string]any{"name": "mcp-whisper", "version": "0.1.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = map[string]any{"code": -32602, "message": "invalid params: " + err.Error()}
			} else {
				result, err := handleToolCall(params)
				if err != nil {
					resp.Result = errorResult(err.Error())
				} else {
					resp.Result = result
				}
			}
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}
