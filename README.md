# Markdown to PDF, but beautiful

![Disclaimer](./.github/teaser.jpg)

## Prerequisites
- Docker and Docker Compose installed
## Security
Do not expose this service publicly or to people you do not trust, as one can likely do SSRF or other attacks. This is intended for local/private network use only.

## Quick start
```bash
echo "AUTH_TOKEN=$(openssl rand -hex 16)" > .env
docker comose up -d

```

Then open http://localhost:5000, paste your auth token when prompted, write markdown, and click **Build PDF**.

API usage (token can be a `token` query param or a `Bearer` header):
```bash
curl -X POST "http://localhost:5000/pdf?token=$AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"md": "# Hello PDF"}' \
  --output hello.pdf
```

## Configuration
- `AUTH_TOKEN` (required): shared secret for the `/pdf` endpoint and UI.

Only the PDF API/UI is published on port `5000` (mapped in `compose.yml`).

## How it works
- `web/` hosts a minimal EasyMDE-based editor that persists content in `localStorage` and sends builds to `/pdf`.

## License
MIT