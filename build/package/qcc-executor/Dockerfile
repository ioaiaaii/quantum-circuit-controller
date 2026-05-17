# syntax=docker/dockerfile:1
# Build context: ./qcc-executor   (the Python project root).  Invoke via:
#   docker build -f build/package/qcc-executor/Dockerfile -t qcc-executor:latest qcc-executor
FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_PROJECT_ENVIRONMENT=/app/.venv

# Pod-Security-Standards "restricted" requires runAsNonRoot. Create an
# unprivileged user up front and own /app so uv can write its venv/cache
# during the build step below.
RUN groupadd --system --gid 1001 qcc \
 && useradd --system --uid 1001 --gid qcc --home-dir /app --shell /usr/sbin/nologin qcc

WORKDIR /app
RUN pip install --no-cache-dir uv

# Install third-party deps first so they cache independently of src changes.
COPY --chown=qcc:qcc pyproject.toml uv.lock README.md ./
RUN uv sync --frozen --no-dev --no-install-project

# Now copy the project source and install it into the venv.
COPY --chown=qcc:qcc src ./src
RUN uv sync --frozen --no-dev && chown -R qcc:qcc /app

USER 1001:1001
EXPOSE 9000
CMD ["uv", "run", "--no-sync", "python", "-m", "qcc_executor"]
