# syntax=docker/dockerfile:1

FROM golang:1.26.2-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Build the runner separately. Every other package main becomes a tool named
# after its directory, so adding another command does not require editing this file.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	set -eu; \
	mkdir -p /out/tools /out/work; \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/toolrunner ./toolrunner; \
	for package in $(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...); do \
		name="${package##*/}"; \
		if [ "$name" = "toolrunner" ]; then continue; fi; \
		if [ -e "/out/tools/$name" ]; then echo "duplicate tool name: $name"; exit 1; fi; \
		echo "building tool: $name"; \
		CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "/out/tools/$name" "$package"; \
	done

FROM scratch

USER 65532:65532
COPY --from=build /out/toolrunner /toolrunner
COPY --from=build /out/tools /tools
COPY --from=build --chown=65532:65532 /out/work /work

WORKDIR /work
EXPOSE 8091
ENTRYPOINT ["/toolrunner"]
