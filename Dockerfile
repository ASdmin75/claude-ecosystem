# Stage 1: Build React frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binaries
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy built frontend into embed directory
COPY --from=frontend /app/web/dist ./internal/ui/dist/
# Build all binaries
RUN CGO_ENABLED=0 go build -o bin/server ./cmd/server
RUN CGO_ENABLED=0 go build -o bin/hook ./cmd/hook
RUN for dir in cmd/mcp/*/; do \
      name=$(basename "$dir"); \
      CGO_ENABLED=0 go build -o "bin/$name" "./$dir"; \
    done

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
# Install Claude Code CLI (requires npm)
RUN apk add --no-cache nodejs npm && npm install -g @anthropic-ai/claude-code
WORKDIR /app
COPY --from=builder /app/bin/ ./bin/
# Default config and agents directory
COPY tasks.yaml ./tasks.yaml
COPY .claude/agents/ ./.claude/agents/
RUN mkdir -p data

EXPOSE 3580
ENTRYPOINT ["./bin/server"]
CMD ["-config", "tasks.yaml"]
