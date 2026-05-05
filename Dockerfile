ARG BUN_IMAGE=oven/bun:latest
ARG GOLANG_IMAGE=golang:1.24-alpine
ARG ALPINE_IMAGE=alpine:latest

FROM ${BUN_IMAGE} AS builder

ARG APP_VERSION=
ARG NPM_REGISTRY=https://registry.npmjs.org

WORKDIR /build
COPY web/package.json .
COPY web/bun.lock .
RUN bun install --frozen-lockfile --registry "${NPM_REGISTRY}"
COPY ./web .
COPY ./VERSION .
# 降低前端打包内存占用：
# - 关闭 sourcemap 与压缩（minify）以避免 OOM（exit code 137）
# - 直接使用 bunx 执行 vite，避免额外开销
RUN app_version="${APP_VERSION:-$(cat VERSION)}" \
    && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION="${app_version}" \
    bunx vite build --sourcemap false --minify false

FROM ${GOLANG_IMAGE} AS builder2

ARG APP_VERSION=
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

WORKDIR /build

ADD go.mod go.sum ./
COPY codex-service-go/go.mod codex-service-go/go.sum ./codex-service-go/
RUN go mod download

COPY . .
COPY ./VERSION .
COPY --from=builder /build/dist ./web/dist
RUN app_version="${APP_VERSION:-$(cat VERSION)}" \
    && go build -ldflags "-s -w -X 'one-api/common.Version=${app_version}'" -o one-api

FROM ${GOLANG_IMAGE} AS builder2_prebuilt

ARG APP_VERSION=
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

WORKDIR /build

ADD go.mod go.sum ./
COPY codex-service-go/go.mod codex-service-go/go.sum ./codex-service-go/
RUN go mod download

COPY . .
COPY ./VERSION .
# 使用宿主机预构建的 web/dist（避免在镜像内构建前端导致 OOM）
COPY web/dist ./web/dist
RUN app_version="${APP_VERSION:-$(cat VERSION)}" \
    && go build -ldflags "-s -w -X 'one-api/common.Version=${app_version}'" -o one-api

FROM ${ALPINE_IMAGE} AS final

ARG ALPINE_MIRROR=

RUN if [ -n "${ALPINE_MIRROR}" ]; then \
      sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories; \
    fi \
    && apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata ffmpeg \
    && update-ca-certificates

COPY --from=builder2 /build/one-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]

FROM ${ALPINE_IMAGE} AS final-prebuilt

ARG ALPINE_MIRROR=

RUN if [ -n "${ALPINE_MIRROR}" ]; then \
      sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories; \
    fi \
    && apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata ffmpeg \
    && update-ca-certificates

COPY --from=builder2_prebuilt /build/one-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]
