[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Work In Progress](https://img.shields.io/badge/Status-Work%20In%20Progress-yellow)](https://guide.unitvectorylabs.com/bestpractices/status/#work-in-progress) 

# invdex

A personal inventory system powered by LLMs to aid in searching and inventorying collectables.

## Features

- **HTMX-powered UI** - Fast, responsive interface with no heavy JavaScript framework
- **Tag-based organization** - Flexible categorization with tags and categories
- **Image upload** - Store images in S3-compatible or GCS backends
- **LLM-assisted cataloging** - AI agent helps identify items, suggest descriptions, and recommend tags
- **Search** - Full-text search across titles and descriptions
- **Stateless design** - No local state; all data in PostgreSQL and object storage

## Configuration

The application is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | *required* |
| `STORAGE_BACKEND` | Storage backend (`s3` or `gcs`) | `s3` |
| `STORAGE_BUCKET` | Storage bucket name | `invdex` |
| `S3_ENDPOINT` | S3-compatible endpoint URL | (AWS default) |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_ACCESS_KEY` | S3 access key | (from AWS config) |
| `S3_SECRET_KEY` | S3 secret key | (from AWS config) |
| `GCS_CREDENTIALS_FILE` | GCS credentials file path | |
| `LLM_PROVIDER` | LLM provider type | `openai` |
| `LLM_ENDPOINT` | LLM API endpoint | `https://api.openai.com/v1` |
| `LLM_API_KEY` | LLM API key | |
| `LLM_MODEL` | LLM model name | `gpt-4o` |

## Development

```bash
# Build
just build

# Run tests
just test

# Run locally (requires PostgreSQL and storage backend)
DATABASE_URL="******localhost:5432/invdex" go run .
```

## Architecture

```
├── main.go                    # Entry point, server setup
├── internal/
│   ├── config/               # Environment-based configuration
│   ├── database/             # PostgreSQL connection, migrations, queries
│   ├── handlers/             # HTTP handlers and HTMX templates
│   │   └── templates/       # Embedded HTML templates
│   ├── llm/                  # LLM client (OpenAI-compatible API)
│   ├── models/               # Data models
│   └── storage/              # File storage (S3/GCS abstraction)
└── Dockerfile                # Multi-stage container build
```

