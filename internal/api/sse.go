package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asdmin/claude-ecosystem/internal/events"
)

// handleEvents sends Server-Sent Events for all system events.
// GET /api/v1/events
// Clients receive task.started, task.completed, pipeline.started, pipeline.completed events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	ch := make(chan events.Event, 128)
	done := make(chan struct{})

	eventTypes := []string{
		"task.started", "task.completed",
		"pipeline.started", "pipeline.completed",
		"task.cancelled",
	}
	for _, et := range eventTypes {
		s.bus.Subscribe(et, func(e events.Event) {
			select {
			case ch <- e:
			default:
			}
		})
	}

	go func() {
		<-r.Context().Done()
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		case evt := <-ch:
			data, err := json.Marshal(evt.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}
	}
}

// handleExecutionStream sends Server-Sent Events for a specific execution.
// GET /api/v1/executions/{id}/stream
func (s *Server) handleExecutionStream(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Verify the execution exists.
	_, err := s.store.GetExecution(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusNotFound, "execution not found: "+execID)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Channel to receive matching events.
	ch := make(chan events.Event, 64)
	done := make(chan struct{})

	// Subscribe to task and pipeline completion events.
	s.bus.Subscribe("task.completed", func(e events.Event) {
		if e.Payload["execution_id"] == execID {
			select {
			case ch <- e:
			default:
			}
		}
	})
	s.bus.Subscribe("pipeline.completed", func(e events.Event) {
		if e.Payload["execution_id"] == execID {
			select {
			case ch <- e:
			default:
			}
		}
	})
	s.bus.Subscribe("task.output", func(e events.Event) {
		if e.Payload["execution_id"] == execID {
			select {
			case ch <- e:
			default:
			}
		}
	})

	go func() {
		<-r.Context().Done()
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		case evt := <-ch:
			data, err := json.Marshal(evt.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()

			// If this is a completion event, close the stream.
			if evt.Type == "task.completed" || evt.Type == "pipeline.completed" {
				return
			}
		}
	}
}

// handleTaskStream sends Server-Sent Events for a task's live output.
// GET /api/v1/tasks/{name}/stream
func (s *Server) handleTaskStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	t := s.findTask(name)
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	ch := make(chan events.Event, 64)
	done := make(chan struct{})

	// Subscribe to events for this task name.
	s.bus.Subscribe("task.completed", func(e events.Event) {
		if e.Payload["task"] == name {
			select {
			case ch <- e:
			default:
			}
		}
	})
	s.bus.Subscribe("task.output", func(e events.Event) {
		if e.Payload["task"] == name {
			select {
			case ch <- e:
			default:
			}
		}
	})

	go func() {
		<-r.Context().Done()
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		case evt := <-ch:
			data, err := json.Marshal(evt.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}
	}
}
