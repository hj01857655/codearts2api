FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

# 版本注入：与 .goreleaser.yaml / Makefile 对齐，否则容器里的二进制永远是 dev，
# 控制台的「当前版本」与在线更新比对失去参照。
# 默认值供直接 docker build 时用；docker compose 会把 git tag / commit 传进来（见 docker-compose.yml）。
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN go build -ldflags "-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.buildDate=${BUILD_DATE}" \
      -o /out/codearts2api ./cmd/server && \
    go build -o /out/codearts2api-login ./cmd/login && \
    go build -o /out/codearts2api-credit ./cmd/credit && \
    go build -o /out/codearts2api-apply ./cmd/apply

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/ /usr/local/bin/
COPY config.example.jsonc ./config.example.jsonc
VOLUME ["/app/auths", "/app/data"]
EXPOSE 7866
CMD ["codearts2api", "-config", "config.json"]
