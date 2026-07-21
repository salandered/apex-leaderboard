# Tests

- [Testify usage](#testify-usage)
	- [Running tests](#running-tests)
- [OpenAPI spec validation](#openapi-spec-validation)
- [Integration tests](#integration-tests)

## Testify usage

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

### Running tests

Example of running a single suite method:

```bash
go test -tags integration -run TestStorageSuite ./storage/... "-testify.m=TestCreatePlayer"
```

where

- `-tags integration` - build tag
- `-run` selects the Go entry point (`TestStorageSuite`)
- `-testify.m` selects individual suite methods via regex (`TestCreatePlayer|TestGetPlayer` would match several)

## OpenAPI spec validation

Responses that tests produce are auto checked against Open API spec.
This runs **only in tests** (no production middleware).

- `SetupSuite` loads the spec (embedded via the `apispec` package) and builds a router from it.
- Test helpers which work with HTTP requests call `validateAgainstSpec`.
- Since all tests use http helpers, no extra code to test bodies
- NOTE: Requests to paths not in the spec are skipped.

## Integration tests

Storage tests run against a Redis via [testcontainers-go](https://github.com/testcontainers/testcontainers-go). It starts a throwaway Redis container in `SetupSuite`, on a random host port.
`SetupTest` calls `FlushDB` before every method (tests are isolated).

The files carry a `//go:build integration` tag, so `go test ./...` skips them.
Command: `go test -tags=integration ./...`
