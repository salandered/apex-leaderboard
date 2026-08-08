# Tests

- [Running tests](#running-tests)
- [More about tests](#more-about-tests)
	- [OpenAPI spec validation](#openapi-spec-validation)
	- [Integration tests](#integration-tests)
	- [Testify usage](#testify-usage)
	- [Linters](#linters)
	- [Misc](#misc)

## Running tests

Linter

```bash
golangci-lint run
```

Unit tests

```bash
go test ./...
```

Everything (the `integration` tag adds files, does not exclude unit)

```bash
go test -tags=integration ./...
```

```bash
go test -race -tags=integration ./...
```

Integration tests only

```bash
go test -tags=integration -run TestStorageSuite ./storage/
```

## More about tests

### OpenAPI spec validation

Responses that tests produce are auto checked against Open API spec.
This runs **only in tests** (no production middleware).

- `SetupSuite` loads the spec (embedded via the `apispec` package) and builds a router from it.
- Test helpers which work with HTTP requests call `validateAgainstSpec`.
- Since all tests use http helpers, no extra code to test bodies
- NOTE: Requests to paths not in the spec are skipped.

### Integration tests

Storage tests run against a Redis via [testcontainers-go](https://github.com/testcontainers/testcontainers-go). It starts a throwaway Redis container in `SetupSuite`, on a random host port.
`SetupTest` calls `FlushDB` before every method (tests are isolated).

The files carry a `//go:build integration` tag, so `go test ./...` skips them.

### Testify usage

Tests use [testify `suite` package](https://github.com/stretchr/testify) on top of Go's `testing`.

- one Go entry point, `TestSomething`, calls `suite.Run(t, new(SomethingSuite))`
- testify then discovers and runs every `SomethingSuite` method named `Test*` as a subtest
- `SetupSuite` / `TearDownSuite` run once around the whole suite (shared server + client)
- `SetupTest` / `TearDownTest` run before/after **each** `Test*` method — for per-test setup/reset
- `s.Require()` are used to make assertions

```
Go test runner
  └── finds TestSomething(t)        ← Go rule
        └── suite.Run(t, SomethingSuite)  ← testify takes over
              └── SetupSuite()      ← testify hook, setup code
              └── TestIWrote() 	    ← testify finds & runs this (inside SetupTest / TearDownTest)
              └── ...  			    
              └── TearDownSuite()   ← testify hook, cleanup
```

#### Running specific test

Example of running a single suite method:

```bash
go test -tags integration -run TestStorageSuite ./storage/... "-testify.m=TestCreatePlayer"
```

where

- `-tags integration` - build tag
- `-run` selects the Go entry point (`TestStorageSuite`)
- `-testify.m` selects individual suite methods via regex (`TestCreatePlayer|TestGetPlayer` would match several)

### Linters

[golangci-lint](https://golangci-lint.run) runs all linters and formatters the project uses.

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
golangci-lint run
```

Note:

- config `.golangci.yml` is the project root.
- `run` only reports the issues. To format files use `golangci-lint fmt`.
- no need to invoke `go vet`, it is included as a `govet` linter
- to lint another module (like `apiscripts`) you need to run the command inside the module folder (root config would be applied)

#### No `shadow`

See: https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/shadow

It flags errors inside the `if` scopes

```go
err := check()
if err := anotherCheck(); err != nil {}
```

which project allows.

#### Useful commands

```bash
# active linters
golangci-lint linters
# active formatters
golangci-lint formatters
# schema is valid
golangci-lint config verify
```

### Misc

https://go.dev/blog/gofix

```bash
go fix ./...
```
