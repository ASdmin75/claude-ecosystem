package scheduler

import (
	"context"
	"log/slog"

	"github.com/ASdmin75/claude-ecosystem/agent"
	"github.com/ASdmin75/claude-ecosystem/config"
	"github.com/ASdmin75/claude-ecosystem/internal/events"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron   *cron.Cron
	runner *agent.Runner
	bus    *events.Bus
	logger *slog.Logger
}

func New(runner *agent.Runner, bus *events.Bus, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		runner: runner,
		bus:    bus,
		logger: logger,
	}
}

func (s *Scheduler) Register(ag config.Agent) error {
	if ag.Schedule == "" {
		return nil
	}

	_, err := s.cron.AddFunc(ag.Schedule, func() {
		s.logger.Info("scheduled agent starting", "agent", ag.Name)

		ctx := context.Background()
		result := s.runner.Run(ctx, ag.Name, ag.Prompt, ag.WorkDir, nil)

		if result.Error != "" {
			s.logger.Error("agent failed", "agent", ag.Name, "error", result.Error)
		} else {
			s.logger.Info("agent completed", "agent", ag.Name, "duration", result.Duration)
		}

		s.bus.Publish(events.Event{
			Type: "agent.completed",
			Payload: map[string]string{
				"agent":  ag.Name,
				"output": result.Output,
				"error":  result.Error,
			},
		})
	})
	if err != nil {
		return err
	}

	s.logger.Info("registered scheduled agent", "agent", ag.Name, "schedule", ag.Schedule)
	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}
