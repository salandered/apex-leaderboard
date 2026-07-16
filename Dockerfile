# syntax=docker/dockerfile:1

FROM golang:1.26.2-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
# persistent cache layer managed by BuildKit; '/go/pkg/mod' is GOMODCACHE
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# CGO_ENABLED=0 - no libc dependency, runs on `scratch`/distroless
# -ldflags "-s -w" strips debug info
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/apex .


FROM alpine:3.22
RUN apk add --no-cache ca-certificates

# run as a non-root user
RUN addgroup -S apex && adduser -S -G apex apex
USER apex

COPY --from=build /out/apex /apex

EXPOSE 8090

ENTRYPOINT ["/apex"]
