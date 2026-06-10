# Build the static attractor binary, then run it in a minimal Debian image that
# still provides a shell + ripgrep/git for the coding agent's tools.
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/attractor ./cmd/attractor

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates ripgrep git \
 && rm -rf /var/lib/apt/lists/*
RUN groupadd --gid 1000 attractor \
 && useradd --uid 1000 --gid 1000 --create-home --shell /bin/bash attractor

COPY --from=build /out/attractor /usr/local/bin/attractor

USER attractor
WORKDIR /sandbox

ENTRYPOINT ["attractor"]
