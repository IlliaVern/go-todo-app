# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27.3

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/todo-app ./cmd/api

FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /app

COPY --from=build /out/todo-app /app/todo-app

EXPOSE 8000
USER nonroot:nonroot

HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/todo-app", "healthcheck"]

ENTRYPOINT ["/app/todo-app"]
