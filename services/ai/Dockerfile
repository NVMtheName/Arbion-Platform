FROM python:3.13-slim@sha256:9d2e5553305c7c7b0097999bb17187c69b921ccd6bc9d40e4bb5ebe652c00285
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
# Debian security fixes published after the pinned Python image. Upgrade only
# these existing OS packages; keep Python and hashed application dependencies fixed.
RUN apt-get update \
    && apt-get install -y --no-install-recommends --only-upgrade \
        gzip=1.13-1+deb13u1 \
        libpcre2-8-0=10.46-1~deb13u2 \
        libsqlite3-0=3.46.1-7+deb13u2 \
        perl-base=5.40.1-6+deb13u1 \
    && rm -rf /var/lib/apt/lists/*
RUN addgroup --system arbion && adduser --system --ingroup arbion arbion
COPY requirements.lock ./
# Runtime dependencies are installed at build time; remove pip and its vendored tooling.
RUN pip install --no-cache-dir --require-hashes -r requirements.lock \
    && python -m pip uninstall --yes pip
COPY app ./app
USER arbion
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
