<!-- markdownlint-disable MD040 -->

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

## OpenAPI spec validation

Every response the tests produce is auto checked against Open API spec. Library
[kin-openapi](https://github.com/getkin/kin-openapi) is used.
This runs **only in tests** (no production middleware).

* `SetupSuite` loads `api.yaml` (embedded) and builds a router from it.
* Test helpers which work with HTTP requests call `validateAgainstSpec`.
* Since all tests use http helpers, no extra code to test bodies
* NOTE: Requests to paths not in the spec are skipped.
