<!-- markdownlint-disable MD040 -->

# Tests

## Testify usage

Tests use [testify `suite` package](https://github.com/stretchr/testify) on top of Go's `testing`.

* one Go entry point, `TestAPI`, calls `suite.Run(t, new(APISuite))`
* testify then discovers and runs every `APISuite` method named `Test*` as a subtest
* `SetupSuite` / `TearDownSuite` run once around the whole suite (shared server + client)
* `s.Require()` are used to make assertions

## How it all connects

```
Go test runner
  └── finds TestAPI(t)              ← Go rule
        └── suite.Run(t, APISuite)  ← testify takes over
              └── SetupSuite()      ← testify hook, setup code
              └── TestX()    	    ← testify finds & runs this
              └── TestY()  		    ← testify finds & runs this
              └── ...  			    ← testify finds & runs this
              └── TearDownSuite()   ← testify hook, cleanup
```

## OpenAPI spec validation

Every response the tests produce is auto checked against Open API spec. Library
[kin-openapi](https://github.com/getkin/kin-openapi) is used.
This runs **only in tests** (no production middleware).

* `SetupSuite` loads `api.yaml` (embedded) and builds a router from it.
* Test helpers which work with HTTP requests call `validateAgainstSpec`.
* Since all tests use http helpers, no extra code to test bodies
* A test fails if a response's body, headers, or status code don't match what the spec
  declares for that route (e.g. a handler returning a field the spec doesn't define).
  Requests to paths not in the spec are skipped.
