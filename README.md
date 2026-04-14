# Brokle – The open source platform for AI teams

<p align="center">
  <a href="https://docs.brokle.com">Docs</a> •
  <a href="https://docs.brokle.com/quickstart">Quick Start</a> •
  <a href="https://discord.gg/brokle">Discord</a> •
  <a href="https://github.com/steloit/steloit/issues">Issues</a> •
  <a href="https://brokle.com">Website</a>
</p>

Debug, evaluate, and optimize your LLM applications with complete visibility. Open source. OpenTelemetry-native. Self-host anywhere.

## Quick Start

```bash
git clone https://github.com/steloit/steloit.git
cd steloit
make setup && make dev
```

| Service | URL |
|---------|-----|
| Dashboard | http://localhost:3000 |
| API | http://localhost:8080 |

**Prerequisites:** Docker and Docker Compose

📚 **Full setup guide**: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)


## SDK Integration

### Python

```bash
pip install brokle
```

```python
from brokle import Brokle

client = Brokle(api_key="bk_...")

with client.trace("my-agent") as trace:
    response = openai.chat.completions.create(...)
```

### JavaScript/TypeScript

```bash
npm install brokle
```

```typescript
import { Brokle } from 'brokle';

const client = new Brokle({ apiKey: 'bk_...' });

await client.trace('my-agent', async () => {
  const response = await openai.chat.completions.create(...);
});
```

### OpenTelemetry

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:8080
export OTEL_EXPORTER_OTLP_HEADERS="x-api-key=bk_..."
```


## Integrations

| Framework | Status | Docs |
|-----------|--------|------|
| OpenAI | ✅ Native | [Guide](https://docs.brokle.com/integrations/openai) |
| Anthropic | ✅ Native | [Guide](https://docs.brokle.com/integrations/anthropic) |
| LangChain | ✅ Supported | [Guide](https://docs.brokle.com/integrations/langchain) |
| LlamaIndex | ✅ Supported | [Guide](https://docs.brokle.com/integrations/llamaindex) |
| OpenTelemetry | ✅ Native | [Guide](https://docs.brokle.com/integrations/opentelemetry) |


## Features

### 👁️ Observability
Complete traces of every AI call with latency, token usage, and cost. Debug chains, agents, and complex pipelines step by step.

### 📊 Evaluation
Automated quality scoring with LLM-as-judge, custom evaluators, and experiments at scale. Define what quality means for your use case.

### 📝 Prompt Management
Version control for prompts with full history. A/B test variations with real traffic and roll back instantly.


## Why Brokle?

- **Open Source** – Transparent, extensible, and community-driven
- **OpenTelemetry Native** – Built on open standards, no vendor lock-in
- **Self-Host Anywhere** – Keep your data on your infrastructure
- **Unified Platform** – Observe, evaluate, and manage in one tool


## Documentation

- 🚀 [**Getting Started**](docs/DEVELOPMENT.md) — Setup and development guide
- 📡 [**API Reference**](docs/API.md) — REST & WebSocket documentation
- 🏗️ [**Architecture**](docs/ARCHITECTURE.md) — System design and technical details
- 🚢 [**Deployment**](docs/DEPLOYMENT.md) — Production-ready options


## Troubleshooting

<details>
<summary><b>Port 8080 already in use</b></summary>

```bash
lsof -ti:8080 | xargs kill -9
```
</details>

<details>
<summary><b>Docker containers not starting</b></summary>

```bash
docker-compose down -v
make setup
```
</details>

<details>
<summary><b>Database migration errors</b></summary>

```bash
make migrate-down
make migrate-up
```
</details>

Need help? Join [Discord](https://discord.gg/brokle) or open a [GitHub Issue](https://github.com/steloit/steloit/issues).


## Contributing

We welcome contributions! See our [Contributing Guide](docs/CONTRIBUTING.md) to get started.


## License

MIT licensed, except for `ee/` folders. See [LICENSE](LICENSE) for details.


## Community

- [Discord](https://discord.gg/brokle) – Chat with the team
- [Twitter](https://twitter.com/BrokleAI) – Updates and news
- [GitHub Discussions](https://github.com/steloit/steloit/discussions) – Questions and ideas

---

<p align="center">
  <b>If Brokle helps you ship AI, give us a star!</b>
</p>
