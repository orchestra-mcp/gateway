# Orchestra Gateway

Unified Go API server for [Orchestra MCP](https://orchestra-mcp.dev) — serves REST API, WebSocket tunnels, MCP Streamable HTTP transport, and health tracking.

## Architecture

```
                 Caddy (TLS + subdomains)
                        │
         ┌──────────────┼──────────────┐
         │              │              │
   api.orchestra-   mcp.orchestra-  orchestra-
     mcp.dev         mcp.dev        mcp.dev
         │              │              │
         └──────┬───────┘              │
                │                      │
           ┌────┴────┐           ┌─────┴──────┐
           │ Gateway  │           │  Next.js   │
           │ :8080    │           │  :3000     │
           └────┬────┘           └────────────┘
                │
    ┌───────────┼───────────┐
    │           │           │
 Tunnels    MCP Tools    Health
 /tunnels   /mcp         /api/health
 /actions   100+ tools   Water, Caffeine
 WebSocket  SSE/JSON-RPC Pomodoro, Sleep
```

### Route Groups

| Subdomain | Path | Description |
|-----------|------|-------------|
| `api.*` | `/tunnels/*` | Tunnel CRUD, reverse WS, browser proxy |
| `api.*` | `/actions/*` | Smart action dispatch through tunnels |
| `api.*` | `/api/health/*` | Health debug API (water, caffeine, meals, sleep, pomodoro) |
| `api.*` | `/health` | Service health check |
| `mcp.*` | `POST /mcp` | MCP JSON-RPC 2.0 request/response |
| `mcp.*` | `GET /mcp` | MCP SSE stream for server-initiated messages |

## Quick Start

```bash
# Development
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/orchestra_web?sslmode=disable"
export JWT_SECRET="your-dev-secret"
go run ./cmd/main.go

# Docker
docker build -t orchestra-gateway .
docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://..." \
  -e JWT_SECRET="..." \
  orchestra-gateway
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | `postgres://...localhost:5432/orchestra_web` | PostgreSQL connection string |
| `JWT_SECRET` | Yes | - | HMAC-SHA256 JWT signing secret |
| `PORT` | No | `8080` | Server port |
| `APP_ENV` | No | `development` | Environment (development/production) |
| `ALLOWED_ORIGINS` | No | localhost + orchestra-mcp.dev | CORS allowed origins (comma-separated) |
| `WEB_API_BASE_URL` | No | `https://orchestra-mcp.dev` | Base URL for admin tool forwarding |
| `UPLOAD_DIR` | No | `uploads` | File upload directory |
| `REPO_BASE_DIR` | No | `/var/orchestra/repos` | Repository workspace base |
| `PUBLIC_RATE_LIMIT` | No | `10` | Rate limit per IP per minute |

## MCP Transport

The gateway implements [MCP 2025-11-25](https://modelcontextprotocol.io) Streamable HTTP transport:

- **`POST /mcp`** — JSON-RPC 2.0 request/response. Handles `initialize`, `ping`, `tools/list`, `tools/call`.
- **`GET /mcp`** — SSE stream. Returns server-initiated messages (tool results, notifications).
- **Sessions** — `Mcp-Session-Id` header for persistent sessions with 10-minute idle timeout.

### Authentication

- Bearer JWT token in `Authorization` header
- API keys prefixed with `orch_` (hashed with SHA-256, looked up in user settings)
- Session fallback: Claude.ai sends token only on `initialize`, then references session ID

### 100+ Tools

Tools span: profile, marketplace, admin (platform stats, users, badges, teams, content moderation, settings, CMS), user settings (preferences, notifications, API keys, sessions, integrations), and user content (skills, agents, workflows, notes, API collections, presentations, community posts, shares).

## Tunnel System

WebSocket-based relay connecting browsers to local MCP CLI instances:

1. **Register** — CLI registers tunnel with connection token
2. **Reverse Connect** — CLI opens persistent WebSocket (`/tunnels/reverse?connection_token=...`)
3. **Browser Proxy** — Browser opens WebSocket (`/tunnels/:id/ws?token=JWT`)
4. **Relay** — Gateway wraps messages in `RelayEnvelope` and forwards between browser and CLI

### Smart Actions

REST endpoint that dispatches MCP tool calls through tunnels with progress streaming:

```
POST /actions/execute
{
  "tunnel_id": "...",
  "action": "run_tool",
  "tool_name": "create_feature",
  "params": { "title": "New feature" }
}
```

## Health Debug API

Full health tracking system — water intake, caffeine monitoring, meal logging, pomodoro timer, sleep tracking, and daily snapshots.

## Building

```bash
# Local build
go build -o gateway ./cmd/main.go

# Docker build
docker build -t ghcr.io/orchestra-mcp/gateway:latest .

# Run tests
go test ./...
```

## Deployment

Part of the [Orchestra MCP Docker Compose stack](https://github.com/orchestra-mcp/deploy):

```yaml
gateway:
  image: ghcr.io/orchestra-mcp/gateway:latest
  environment:
    DATABASE_URL: postgres://postgres:${POSTGRES_PASSWORD}@supabase-db:5432/postgres
    JWT_SECRET: ${JWT_SECRET}
  depends_on:
    supabase-db:
      condition: service_healthy
```

## License

MIT
