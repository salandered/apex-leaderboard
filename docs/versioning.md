# Versioning

The version is injected at **build time** using Go's `-ldflags`. Read about it [here](https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications)
The value is baked into the compiled binary / image.

## How it works

* `docker-compose.yml` passes a `VERSION` env var as a build arg
* `Dockerfile` passes it to `go build` using `go build -ldflags="-X <var-path>.version=${VERSION}"`
* The linker then patches it to the actual variable 'version', defined on a package-level

## Development

Nothing should be set, a default value (like `dev`) is used.

## Git tag as version

Common thing to use is a git tag via `git describe`.

```bash
VERSION=$(git describe --tags --always) docker compose build
```

Its value depends on where `HEAD` is:

| Situation                        | Output                                          |
| -------------------------------- | ----------------------------------------------- |
| HEAD is exactly on tag `v0.0.4`  | `v0.0.4`                                        |
| HEAD is 3 commits after `v0.0.4` | `v0.0.4-3-gf5efd71` (tag + commits-since + SHA) |
| No tags exist yet                | `f5efd71` (bare SHA, the `--always` fallback)   |

## CI/CD usage

The intended flow:

1. A developer or CI script tags a release commit and pushes the tag:

   ```bash
   git tag v0.0.4
   git push origin v0.0.4
   ```

1. CI uses it to set the version:

   ```bash
   VERSION=$(git describe --tags --always) docker compose build
   ```

## Troubleshooting

Shallow clone (`--depth 1`) strips tags and will break `git describe`. Probably it will return just the SHA.
For example, on GitHub Actions, use `actions/checkout` with `fetch-depth: 0` (and `fetch-tags: true`).
