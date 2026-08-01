# Setup & Running

## Prerequisites

- Go 1.26.5 or later
- An OpenAI API key

## Configuration

Set the OpenAI API key as an environment variable:

```bash
export OPENAI_API_KEY=sk-your-key-here
```

Alternatively, create a `.env` file (not committed):

```bash
echo 'OPENAI_API_KEY=sk-your-key-here' > .env
source .env
```

## Build

```bash
go build -o bin/agentplane ./cmd/server/
```

## Run

### Development (direct)

```bash
go run ./cmd/server/
```

### Production (binary)

```bash
./bin/agentplane
```

The server starts on **port 3001**:

```
2025/01/01 00:00:00 Starting AgentPlane on :3001
```

## Verify

Send a test request:

```bash
curl -X POST http://localhost:3001/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}'
```

## Troubleshooting

### OPENAI_API_KEY is not set

Ensure the environment variable is exported:

```bash
echo $OPENAI_API_KEY
```

### Connection refused

The server may not be running. Check:

```bash
lsof -i :3001
```

### Unsupported Go version

Run `go version` and ensure it's 1.26.5+. The `go.mod` file can be edited to match your installed Go version by changing the `go` directive.
