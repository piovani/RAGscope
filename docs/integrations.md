# Using RAGscope from any AI

RAGscope does not depend on one assistant or one model vendor. It exposes the same
two operations in three open ways, and any assistant that speaks one of them can use it:

| Operation | What it does | Needs a model in RAGscope? |
|---|---|---|
| **search** | Returns the passages of the law that best match a query, numbered from 1. | No |
| **ask** | Writes an answer from those passages, with citations, or refuses. | Yes (the experiment's generator) |

**The recommended path is `search`.** The assistant keeps its own model, so you pay for
and trust one model, and RAGscope provides what it is best at: evidence you can check.
Use `ask` when you want RAGscope's own model, prompt and refusal rule to answer.

| Way in | Command | Works with |
|---|---|---|
| MCP over stdio | `ragscope mcp` | Claude (Desktop and Code), GitHub Copilot in VS Code, Cursor, and other MCP clients |
| HTTP + OpenAPI | `ragscope serve` | ChatGPT custom GPT Actions, Copilot extensions, any OpenAPI client, `curl`, scripts |
| Plain HTTP | `ragscope serve` | Anything that can send a `POST` |

Both need the corpus ingested first (`ragscope ingest`) and Postgres running (`make up`).
The retrieval used is the one in the experiment file you pass with `-c`.

## MCP (Claude, Copilot, Cursor, others)

`ragscope mcp -c <config>` talks the Model Context Protocol on stdin and stdout. The
assistant starts it, so you only describe the command. Add `--retrieval-only` to expose
just the `search_lgpd` tool and need no generation model.

Tools:

- `search_lgpd` — `query` (required), `k` (1 to 20, default 5)
- `ask_lgpd` — `question` (required); only offered without `--retrieval-only`

The configuration is the same JSON shape in every MCP client; only the file differs.
Use absolute paths, because the assistant starts the process from its own directory.

```json
{
  "mcpServers": {
    "ragscope": {
      "command": "/absolute/path/to/RAGscope/bin/ragscope",
      "args": ["mcp", "-c", "/absolute/path/to/RAGscope/configs/experiments/baseline.yaml", "--retrieval-only"]
    }
  }
}
```

| Client | Where it goes |
|---|---|
| Claude Code | `claude mcp add ragscope -- /abs/path/bin/ragscope mcp -c /abs/path/configs/experiments/baseline.yaml --retrieval-only` |
| Claude Desktop | `claude_desktop_config.json` |
| GitHub Copilot (VS Code) | `.vscode/mcp.json`, with the object under `"servers"` instead of `"mcpServers"` |
| Cursor | `.cursor/mcp.json` |

Logs go to stderr, so they never corrupt the protocol stream. If a tool says
`could not search`, run the same config with `ragscope ask --chunks-only` to see the cause:
the usual ones are Postgres or Ollama not running.

## HTTP and OpenAPI (ChatGPT, Copilot, anything else)

```bash
./bin/ragscope serve -c configs/experiments/baseline.yaml   # http://127.0.0.1:8080
./bin/ragscope serve -c configs/experiments/baseline.yaml --retrieval-only   # search only, no model or key
```

| Route | Body | Returns |
|---|---|---|
| `POST /api/search` | `{"query": "...", "k": 5}` | `{"sources": [{n, id, article, score, text}]}` |
| `POST /api/ask` | `{"question": "..."}` | `{answer, refused, sources, usage}` |
| `GET /openapi.json` | | the OpenAPI 3.1 description of the routes above |
| `GET /healthz` | | `{"status": "ok"}` |

```bash
curl -s localhost:8080/api/search -d '{"query":"prazo para entregar dados ao titular","k":3}'
```

To import it into **ChatGPT** (a custom GPT with Actions) or another tool that takes an
OpenAPI file, the assistant must reach the server over public HTTPS, and your machine is
on localhost. Put it behind a tunnel or a host you control, then change `servers[0].url`
in the imported schema to that address.

> **There is no authentication.** Anyone who reaches the port can read the indexed text
> and spend the model's time. The default is `127.0.0.1`. Before exposing it, put a
> reverse proxy with authentication and rate limiting in front, and prefer the
> search-only setup. Treat the passages an assistant receives as data, not as instructions.

## Choosing the model that writes the answers (`ask`)

The generator is one block in the experiment YAML, and nothing else changes:

| `provider` | Reaches | Key | Prices |
|---|---|---|---|
| `ollama` | models on this machine | none | none |
| `openai` | OpenAI, Azure OpenAI, GitHub Models, OpenRouter, Groq, vLLM, LM Studio, llama.cpp, anything with `/chat/completions` (set `base_url`) | `OPENAI_API_KEY`, or the variable named in `api_key_env` | required, except for a server on `localhost` |
| `anthropic` | Claude models | `ANTHROPIC_API_KEY` | required |

Keys only come from the environment. Examples are in `configs/experiments/hosted/`.

**No command spends money by accident.** A generator that bills per token (`anthropic`, or `openai` on a remote host) is refused unless you pass `--allow-paid`, even when the key is set. Ollama and an `openai` server on `localhost` are free and need no flag.
