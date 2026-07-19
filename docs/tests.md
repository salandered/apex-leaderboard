# Tests

## Testify usage

Tests use [testify `suite` package](https://github.com/stretchr/testify) on top of Go's `testing`.

* one Go entry point, `TestAPI`, calls `suite.Run(t, new(APISuite))`
* testify then discovers and runs every `APISuite` method named `Test*` as a subtest
* `SetupSuite` / `TearDownSuite` run once around the whole suite (shared server + client)
* `SetupTest` / `TearDownTest` run before/after **each** `Test*` method — for per-test setup/reset
* `s.Require()` are used to make assertions

## How it all connects

```
Go test runner
  └── finds TestAPI(t)              ← Go rule
        └── suite.Run(t, APISuite)  ← testify takes over
              └── SetupSuite()      ← testify hook, setup code
              └── TestIWrote() 	    ← testify finds & runs this (inside SetupTest / TearDownTest)
              └── ...  			    
              └── TearDownSuite()   ← testify hook, cleanup
```

## Running tests

Go runs tests per **package**, not per file. There is no flag to run a single `_test.go` file —
scope by package plus test name instead.

* whole package: `go test ./storage/...`
* storage tests live behind a build tag and need `-tags integration`
* `-run` selects the Go entry point (`TestStorageSuite`), which is the whole suite
* `-testify.m` selects individual suite methods; it takes a regex, so
  `-testify.m=TestCreatePlayer|TestGetPlayer` matches several

Running a single suite method:

```
go test -tags integration -run TestStorageSuite ./storage/... "-testify.m=TestCreatePlayer"
```

NOTE: on PowerShell the `-testify.m` flag must be quoted and use `=` as shown. Passing it as
`-testify.m TestCreatePlayer` makes PowerShell split the argument, and the test binary reports
`flag provided but not defined: -testify`.

## OpenAPI spec validation

Responses that tests produce are auto checked against Open API spec. Library
[kin-openapi](https://github.com/getkin/kin-openapi) is used.
This runs **only in tests** (no production middleware).

* `SetupSuite` loads `api.yaml` (embedded) and builds a router from it.
* Test helpers which work with HTTP requests call `validateAgainstSpec`.
* Since all tests use http helpers, no extra code to test bodies
* NOTE: Requests to paths not in the spec are skipped.

## TODO: add info about testcontainers go with Redis
