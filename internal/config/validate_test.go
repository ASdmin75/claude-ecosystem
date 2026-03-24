package config

import (
	"strings"
	"testing"
)

func TestValidateInvalidCronExpression(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", Schedule: "not-a-cron"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid cron expression")
	}
	if !strings.Contains(err.Error(), "invalid cron schedule") {
		t.Errorf("error should mention invalid cron schedule, got: %v", err)
	}
}

func TestValidateMaxTurnsNegative(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", MaxTurns: -1},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for negative max_turns")
	}
	if !strings.Contains(err.Error(), "max_turns must be >= 0") {
		t.Errorf("error should mention max_turns, got: %v", err)
	}
}

func TestValidateMaxTurnsZeroIsValid(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", MaxTurns: 0},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("max_turns=0 should be valid, got: %v", err)
	}
}

func TestValidateMaxTurnsPositiveIsValid(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", MaxTurns: 5},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("max_turns=5 should be valid, got: %v", err)
	}
}

func TestValidatePipelineInvalidCron(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something"},
		},
		Pipelines: []Pipeline{
			{
				Name:     "p1",
				Mode:     "sequential",
				Steps:    []PipelineStep{{Task: "t1"}},
				Schedule: "every-five-minutes",
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for pipeline with invalid cron")
	}
	if !strings.Contains(err.Error(), "invalid cron schedule") {
		t.Errorf("error should mention invalid cron schedule, got: %v", err)
	}
	if !strings.Contains(err.Error(), "pipeline p1") {
		t.Errorf("error should identify the pipeline, got: %v", err)
	}
}

func TestValidatePipelineValidCron(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something"},
		},
		Pipelines: []Pipeline{
			{
				Name:          "p1",
				Mode:          "sequential",
				MaxIterations: 1,
				Steps:         []PipelineStep{{Task: "t1"}},
				Schedule:      "0 9 * * 1-5",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("valid pipeline cron should pass, got: %v", err)
	}
}

func TestValidateTaskWithBothScheduleAndWatch(t *testing.T) {
	// Having both schedule and watch should produce a warning but not an error.
	cfg := &Config{
		Tasks: []Task{
			{
				Name:     "t1",
				Prompt:   "do something",
				Schedule: "0 9 * * *",
				Watch:    &Watch{Paths: []string{"/tmp"}, Extensions: []string{".go"}},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("task with both schedule and watch should not error, got: %v", err)
	}
}

func TestValidateDuplicateTaskName(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "dup", Prompt: "a"},
			{Name: "dup", Prompt: "b"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for duplicate task names")
	}
	if !strings.Contains(err.Error(), "duplicate task name") {
		t.Errorf("error should mention duplicate task name, got: %v", err)
	}
}

func TestValidateEmptyTaskName(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "", Prompt: "do something"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty task name")
	}
	if !strings.Contains(err.Error(), "task name is required") {
		t.Errorf("error should mention task name is required, got: %v", err)
	}
}

func TestValidateEmptyPrompt(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: ""},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty prompt")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("error should mention prompt is required, got: %v", err)
	}
}

func TestValidateInvalidTimeout(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", Timeout: "banana"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid timeout")
	}
	if !strings.Contains(err.Error(), "invalid timeout") {
		t.Errorf("error should mention invalid timeout, got: %v", err)
	}
}

func TestValidateValidConfig(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "do something", Schedule: "*/5 * * * *", Timeout: "30m", MaxTurns: 3},
			{Name: "t2", Prompt: "do other thing"},
		},
		Pipelines: []Pipeline{
			{
				Name:  "p1",
				Mode:  "parallel",
				Steps: []PipelineStep{{Task: "t1"}, {Task: "t2"}},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("valid config should pass validation, got: %v", err)
	}
}
