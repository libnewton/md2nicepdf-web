# Markdown to PDF, but beautiful

![Disclaimer](./.github/teaser.jpg)

## Prerequisites
- Docker and Docker Compose installed
## Security
Do not expose this service publicly or to people you do not trust, as one can likely do SSRF or other attacks via Outline's import features. This is intended for local/private network use only.

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

The Outline app itself is not exposed outside the container; only the PDF API/UI is published on port `5000` (mapped in `compose.yml`).

## How it works
- `entrypoint.sh` starts Redis and Postgres inside the container, launches Outline, waits for it to become healthy, runs the installer script, and starts the Gunicorn-served Flask app.
- `install_outline.py` performs the one-time Outline bootstrap (team/user, collection, API key) and saves the credentials to `/outline.json` for the PDF service.
- `pdfserver.py` validates the auth token, pads markdown for Outline parsing, uploads the document, requests a print-friendly PDF from Browserless with custom CSS, strips metadata/blank pages, and cleans up the Outline document.
- `web/` hosts a minimal EasyMDE-based editor that persists content in `localStorage` and sends builds to `/pdf`.

## Notes
- No volumes are defined; data inside Postgres/Redis lives only for the container lifetime. Add volumes if you need persistence.
- Network access from the container to Browserless/Chromium is required (provided by the `chromeserver` service in `compose.yml`).
## License
MIT