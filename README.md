# New API

This repository contains a modified distribution of New API, an LLM gateway and
AI asset management system with a Go backend and a React/Vite console.

This project is based on the upstream New API project by QuantumNous:

- Upstream repository: https://github.com/QuantumNous/new-api
- Upstream snapshot used for attribution reference: `2497c7549f16acf8a360b13ff1c88cf3c109d210`

The project is licensed under the GNU Affero General Public License v3.0 or
later. See [LICENSE](./LICENSE) and [NOTICE](./NOTICE).

## Features

- OpenAI-compatible relay endpoints such as `/v1/chat/completions`,
  `/v1/responses`, `/v1/embeddings`, image/audio endpoints, and realtime
  websocket support.
- Multi-provider adaptor layer under `relay/channel/*`.
- Admin and user console built with React/Vite and served by the Go backend.
- Quota, billing, pricing, usage logs, subscription, redemption, and product
  management features.
- Optional Redis-backed cache/session behavior and MySQL/PostgreSQL/SQLite
  database support depending on runtime configuration.
- Optional nested `codex-service-go` module for Codex-related proxy and instance
  management features.

## Source Availability

If you deploy a modified version as a network service, AGPL users must be able
to obtain the Corresponding Source for the exact version that is running. Publish
and link the matching source tag for every deployed release.

This repository intentionally does not contain production secrets, databases,
logs, private deployment files, or service-specific environment files.

## Quick Start

### Docker Compose

Copy the deployment example and adjust secrets before starting:

```bash
mkdir -p data
cp .env.deploy.example data/.env
docker compose up -d --build
```

The default compose file builds the image locally from this source tree.

### Local Development

Backend:

```bash
go run main.go
```

Frontend:

```bash
cd web
bun install
bun run dev
```

The Vite dev server proxies console API calls to the backend. The default proxy
target is `http://127.0.0.1:3000`; override it with `VITE_PROXY_TARGET` if
needed.

### Build

Frontend assets must be built into `web/dist` before building the Go binary,
because the backend embeds the console bundle.

```bash
cd web
bun install
bun run build
cd ..
go build -o one-api
```

Docker can build the complete frontend and backend in one image:

```bash
docker build -t new-api:local .
```

## Project Layout

```text
.
├── main.go                # Backend entrypoint
├── router/                # Route wiring
├── controller/            # HTTP handlers
├── middleware/            # Auth, rate limit, request IDs, logging
├── model/                 # DB models, migrations, cache/sync helpers
├── service/               # Business logic
├── relay/                 # Upstream relay/adaptor subsystem
├── setting/               # Runtime settings layers
├── constant/              # Shared constants
├── dto/                   # Request/response DTOs
├── web/                   # React/Vite frontend
└── codex-service-go/      # Nested Go module
```

## Configuration

Use `.env.example` as the broad reference and `.env.deploy.example` as a
Docker-oriented deployment template.

Important categories:

- `PORT`, `FRONTEND_BASE_URL`, `GIN_MODE`
- `SQL_DSN`, `SQLITE_PATH`, `LOG_SQL_DSN`
- `REDIS_CONN_STRING`
- `SESSION_SECRET`, `CRYPTO_SECRET`
- provider/channel keys configured through the console or database

Do not commit real `.env` files, API keys, database dumps, logs, or generated
runtime state.

## Security

Public deployments should set strong random values for session/crypto secrets,
use a real database instead of local throwaway data, and avoid exposing database
or Redis ports to the public internet.

If you discover a vulnerability, see [SECURITY.md](./SECURITY.md).

## License

This project is distributed under the GNU Affero General Public License v3.0 or
later. Some source files retain upstream copyright and license headers. Preserve
those notices when modifying or redistributing the project.

Commercial use is allowed under the AGPL, but modified network services must
provide the corresponding source code to their users under the same license.

## Acknowledgements

This project is based on work from the New API community, including:

- QuantumNous/new-api: https://github.com/QuantumNous/new-api
- Calcium-Ion/new-api: https://github.com/Calcium-Ion/new-api
- songquanpeng/one-api: https://github.com/songquanpeng/one-api
