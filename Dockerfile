FROM docker.getoutline.com/outlinewiki/outline:1.1.0
USER root

# Install Redis, Postgres, and Python runtime pieces
RUN apt-get update \
    && apt-get install -y --no-install-recommends redis-server postgresql python3-pip curl ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# App files
COPY entrypoint.sh /entrypoint.sh
COPY install_outline.py /install_outline.py
COPY pdfserver.py /pdfserver.py
RUN pip3 install --no-cache-dir requests flask gunicorn pypdf --break-system-packages
COPY web /web

RUN chmod +x /entrypoint.sh

ENV DATABASE_URL=postgresql://user:pass@127.0.0.1:5432/outline
ENV REDIS_URL=redis://127.0.0.1:6379
# hardcoded is FINE. This is internal only.
ENV SECRET_KEY=a30f5d5b77432d3182821971e7bb4f90006d0ebe09457d8055c37362599e6c56
ENV UTILS_SECRET=a30f5d5b77432d3182821971e7bb4f90006d0ebe09457d8055c37362599e6c56
ENV URL=http://mdpdf:3000
ENV PGSSLMODE=disable
ENV PORT=3000
ENV FORCE_HTTPS=false
ENV FILE_STORAGE=local
EXPOSE 5000
ENTRYPOINT ["/entrypoint.sh"]
CMD ["yarn", "start"]
