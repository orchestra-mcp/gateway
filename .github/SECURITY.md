# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Orchestra Gateway, please report it responsibly.

**Do not open a public issue.** Instead, email **security@orchestra-mcp.dev** with:

- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (if any)

We will acknowledge your report within 48 hours and provide a timeline for a fix.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |

## Scope

- JWT authentication and token validation
- MCP transport security (session management, SSE)
- WebSocket tunnel relay (connection tokens, browser proxy)
- API key handling (orch_* keys)
- CORS and rate limiting
- Docker container security
