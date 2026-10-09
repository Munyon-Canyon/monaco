# Build context is the repo root: docker build -f apps/backend/deployments/Dockerfile .
FROM golang:1.26-bookworm AS build
WORKDIR /src/apps/backend
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/ ./cmd/api ./cmd/worker ./cmd/monacoctl

FROM debian:bookworm-slim AS atlas
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*
WORKDIR /repo
COPY scripts/install-atlas.sh scripts/
COPY apps/backend/.atlas-version apps/backend/
RUN scripts/install-atlas.sh

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 monaco
WORKDIR /app/apps/backend
COPY apps/backend/go.mod apps/backend/.atlas-version ./
COPY apps/backend/migrations/ migrations/
COPY --from=atlas /repo/.bin/atlas /app/.bin/atlas
COPY --from=build /out/ /app/bin/
COPY apps/backend/deployments/predeploy.sh /app/bin/predeploy
ENV PATH=/app/bin:$PATH
USER monaco
CMD ["api"]
