import io
import json
import logging
import os
import secrets
from typing import Dict, Optional

import requests
from flask import Flask, Response, abort, jsonify, request
from pypdf import PdfReader, PdfWriter

logging.basicConfig(level=logging.INFO)

OUTLINE_URL = os.environ.get("OUTLINE_URL", "http://mdpdf:3000")
PDF_GENERATOR_URL = os.environ.get("PDF_GENERATOR_URL", "http://chromeserver:3000")
PDF_GENERATOR_TOKEN = os.environ.get("PDF_GENERATOR_TOKEN", "internal")
AUTH_TOKEN = os.environ.get("AUTH_TOKEN", secrets.token_hex(16))
CSS_OVERRIDE = """
[aria-label='Document title'] { display: none !important; }
a[href*='www.getoutline.com'] { display: none !important; }
div:has(> div > ol:nth-child(2)) { display: none !important; }
body { font-family: Arial, Verdana, Helvetica, sans-serif; font-size: 11pt; }
@page:first { margin-top: -2rem !important; }
h1 { font-size: 19pt !important; }
h2 { font-size: 16pt !important; }
h3 { font-size: 14pt !important; }
h4 { font-size: 12pt !important; }
@page { margin-bottom: 1cm; margin-top: 1cm; }
""".strip()

app = Flask(__name__, static_url_path="", static_folder="web")


def create_outline_session() -> requests.Session:
    session = requests.Session()
    session.headers.update({"Accept": "application/json"})
    return session


def extract_token() -> Optional[str]:
    token = request.args.get("token")
    if token:
        return token

    auth_header = request.headers.get("Authorization")
    if auth_header and auth_header.startswith("Bearer "):
        return auth_header.split(" ", 1)[1]
    return None


def upload_md(session: requests.Session, collection_id: str, token: str, md_txt: str) -> Dict[str, str]:
    files = {
        "collectionId": (None, collection_id),
        "publish": (None, "true"),
        "file": ("document.md", md_txt, "text/markdown"),
    }
    headers = {"Authorization": f"Bearer {token}"}
    import_resp = session.post(f"{OUTLINE_URL}/api/documents.import", files=files, headers=headers, timeout=20)
    import_resp.raise_for_status()
    doc_id = import_resp.json()["data"]["id"]

    share_body = {"documentId": doc_id, "type": "document"}
    share_resp = session.post(f"{OUTLINE_URL}/api/shares.create", json=share_body, headers=headers, timeout=20)
    share_resp.raise_for_status()

    update_resp = session.post(
        f"{OUTLINE_URL}/api/shares.update",
        json={"id": share_resp.json()["data"]["id"], "published": True},
        headers=headers,
        timeout=20,
    )
    update_resp.raise_for_status()
    url_out = update_resp.json()["data"]["url"]
    return {"documentId": doc_id, "url": url_out}


def delete_document(session: requests.Session, document_id: str, token: str) -> None:
    headers = {"Authorization": f"Bearer {token}"}
    try:
        session.post(
            f"{OUTLINE_URL}/api/documents.delete",
            json={"id": document_id, "permanent": True},
            headers=headers,
            timeout=20,
        )
    except Exception as exc:
        app.logger.warning("Failed to delete document %s: %s", document_id, exc)


def build_pdf_payload(url: str) -> Dict[str, object]:
    return {
        "url": url,
        "emulateMediaType": "print",
        "setJavaScriptEnabled": True,
        "waitForTimeout": 2000,
        "options": {
            "format": "A4",
            "margin": {
                "left": "20px",
                "right": "20px",
            },
        },
        "addStyleTag": [{"content": CSS_OVERRIDE}],
    }


def download_pdf(pdfserver: str, pdftoken: str, url: str) -> bytes:
    payload = build_pdf_payload(url)
    pdf_resp = requests.post(
        f"{pdfserver}/pdf?token={pdftoken}",
        json=payload,
        headers={"Content-Type": "application/json"},
        allow_redirects=True,
        timeout=60,
    )
    pdf_resp.raise_for_status()
    return pdf_resp.content


@app.route("/")
def root():
    return app.send_static_file("index.html")


def get_outline_config() -> Optional[Dict[str, str]]:
    if not os.path.isfile("/outline.json"):
        return None
    with open("/outline.json", "r") as f:
        return json.load(f)


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


def pad_markdown(md_txt: str) -> str:
    # Outline markdown importer can drop leading headings without padding
    return "# \n" + md_txt


@app.route("/pdf", methods=["POST"])
def handle_pdf():
    token = extract_token()
    if token != AUTH_TOKEN:
        abort(Response(json.dumps({"error": "unauthorized"}), status=401, mimetype="application/json"))

    data = request.get_json(silent=True) or {}
    md_txt = data.get("md")
    if not isinstance(md_txt, str):
        abort(Response(json.dumps({"error": "missing_md"}), status=400, mimetype="application/json"))

    config = get_outline_config()
    if not config:
        abort(Response(json.dumps({"error": "outline_not_ready"}), status=503, mimetype="application/json"))

    collection_id = config["collectionId"]
    api_key = config["apiKey"]

    outline_session = create_outline_session()
    document_id: Optional[str] = None
    try:
        upload_resp = upload_md(outline_session, collection_id, api_key, pad_markdown(md_txt))
        document_id = upload_resp["documentId"]
        doc_url = upload_resp["url"]
        pdf_data = download_pdf(PDF_GENERATOR_URL, PDF_GENERATOR_TOKEN, doc_url)
        pdf_data = post_process_pdf(pdf_data)
        app.logger.info("Generated PDF for document ID %s URL %s", document_id, doc_url)
        return Response(pdf_data, mimetype="application/pdf")
    except requests.HTTPError as exc:
        app.logger.exception("HTTP error while processing PDF")
        status_code = exc.response.status_code if exc.response is not None else 500
        abort(Response(json.dumps({"error": "pdf_build_failed", "detail": str(exc)}), status=status_code, mimetype="application/json"))
    except Exception as exc:  # pragma: no cover
        app.logger.exception("Unexpected error while processing PDF")
        abort(Response(json.dumps({"error": "internal_error", "detail": str(exc)}), status=500, mimetype="application/json"))
    finally:
        if document_id:
            delete_document(outline_session, document_id, api_key)


def post_process_pdf(pdf_content: bytes) -> bytes:
    try:
        reader = PdfReader(io.BytesIO(pdf_content))
        writer = PdfWriter()
        has_pages = False
        for page in reader.pages:
            text = page.extract_text()
            resources = page.get("/Resources", {})
            has_images = "/XObject" in resources

            if (text and text.strip()) or has_images:
                writer.add_page(page)
                has_pages = True

        if not has_pages:
            return pdf_content

        writer.add_metadata(
            {
                "/Producer": "",
                "/Creator": "",
                "/Title": "",
                "/Subject": "",
                "/Author": "",
                "/Keywords": "",
                "/ModDate": "",
                "/CreationDate": "",
            }
        )

        out_stream = io.BytesIO()
        writer.write(out_stream)
        return out_stream.getvalue()
    except Exception as exc:  # pragma: no cover
        app.logger.warning("PDF post-processing failed: %s", exc)
        return pdf_content


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000)
