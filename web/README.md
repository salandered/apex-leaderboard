# Web UI

ℹ️ UI is mostly written by AI coding agents, including this README.

Private, static UI for inspecting and changing Apex leaderboards. It uses plain HTML, CSS and a
vendored Alpine.js file. There is no build step, package manager or frontend server.

The public API contract in [`apispec/api.yaml`](../apispec/api.yaml) is the authority for every
request and response used here.

## Running it

```bash
docker compose -f docker-compose.yml -f docker-compose.web.yml up -d
```

Open `http://localhost:8089`.

Caddy serves `web/` and proxies `/api/v1/*` to the Go service. The browser uses only port 8089, so
the static files and API share one origin and need no CORS configuration.

The bind mount is read-only but live. Editing a file under `web/` only needs a browser refresh.
Editing `Caddyfile` needs `docker compose restart web`.

## Files

```text
web/
  index.html                     markup, Alpine attributes and CSS
  app.js                         state, API calls, polling and browser persistence
  logo-mini.png                  favicon
  vendor/alpine-3.15.12.min.js   pinned runtime; do not edit the minified file
Caddyfile                        static file and API proxy rules
docker-compose.web.yml           optional web service
```

`index.html` is large because the project deliberately has no CSS processing or component build
step. Moving its style block to a plain `styles.css` is safe when it becomes inconvenient. It does
not require adopting a frontend toolchain.

## Startup and script order

The order of the two scripts in `index.html` is required:

1. `app.js` registers an `alpine:init` listener.
2. Alpine loads, dispatches that event and scans the document.

Both scripts use `defer` so the document exists before Alpine scans it. Reversing the scripts or
removing `defer` leaves the component unregistered or scans an incomplete document.

The small inline theme script is intentionally not deferred. It sets `data-theme` before the first
paint to avoid a light/dark flash.

## State and requests

`Alpine.data("leaderboard", ...)` is the one page component. Its state falls into four groups:

- board selection and the paginated leaderboard;
- selected-player history;
- the global ledger ticker;
- write forms and their short local log.

All browser requests use relative `/api/v1/...` paths. API error bodies are `text/plain`, so
`getJSON` and `sendJSON` read error responses as text before throwing. Do not call `res.json()` on
an error response.

Server-provided strings are rendered with `x-text`. Do not replace it with `x-html` for names,
identifiers or error messages.

### Request freshness

The leaderboard and history can be requested again before an older request completes. Each uses a
monotonic request id. A response updates the page only while its id is still current.

Keep that check after every `await` added to either path. Capturing `boardId`, `limit`, `offset` and
`playerId` before the first request is also required. Reading them from `this` after an `await` can
mix a response for the old selection with the new selection.

The ledger ticker is single-flight. `pollEvents` returns while a previous poll is running because
two polls with the same cursor would prepend the same events twice.

### Retry-safe writes

Player creation and score writes send an `Idempotency-Key`. `sendIdempotentJSON` keeps the key for a
request signature after any failure because a lost response does not prove the write was rejected.
A retry of the same method, URL and body in the same page session reuses that key. A successful
response removes it.

This protection is session-local. Reloading the page clears pending keys. Persisting them would
need expiry and completed-request handling, which is not justified for this private UI.

Board creation uses `PUT` with a caller-chosen id. Opening and closing are idempotent commands, so
those calls use the ordinary helper.

## Browser persistence

The page treats browser storage as optional. A blocked or full `localStorage` must not stop API
reads or writes.

| Key             | Value                                   |
| --------------- | --------------------------------------- |
| `theme`         | Raw `light` or `dark` string            |
| `board_id`      | Selected board id as JSON               |
| `page_size`     | Selected leaderboard page size as JSON  |
| `ticker_cursor` | Last consumed ledger cursor as JSON     |
| `ticker_events` | Up to 30 displayed ticker rows as JSON  |
| `write_logs`    | Up to five recent write results as JSON |

Ticker rows are checked against the current ledger head during startup. This prevents cached rows
from a flushed Redis instance appearing as current data.

## Adding a feature

1. Read the endpoint in `apispec/api.yaml` first.
2. Add the smallest state fields and method to the Alpine component.
3. Capture request inputs before the first `await`.
4. Decide whether a response can become stale or a call can overlap.
5. Render API strings with `x-text`.
6. Give failures their own visible error state when they should not replace the leaderboard error.
7. Update this file when the data flow or persistence rules change.

Do not copy server validation rules into JavaScript unless the UI needs an earlier hint. The server
remains authoritative, and duplicated rules drift. HTML attributes such as `required`, `min` and
`max` are suitable for basic form guidance.

## Manual verification

```bash
node --check web/app.js
docker compose -f docker-compose.yml -f docker-compose.web.yml config -q
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8089/
curl -s http://localhost:8089/api/v1/boards
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8090/readyz
```

In a browser, check both themes, board switching, pagination, keyboard selection of a player row,
history loading, one score write and the ledger ticker. The network panel should show requests to
port 8089 only, and the console should have no errors.

There are no automated browser tests yet. Keep manual verification until that work lands.
