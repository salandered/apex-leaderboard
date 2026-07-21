package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[37m"
)

func main() {
	fmt.Print("🚶🌳")
	base := flag.String("base-url", "http://localhost:8090", "Apex service URL")
	boardID := flag.String("board", "demo-cup", "board id to use (fixed id -> 409 on board create after first run)")
	color := flag.String("color", "auto", "colorize output: auto|always|never")
	flag.Parse()

	w := newWalker(strings.TrimRight(*base, "/"), useColor(*color))
	scorePath := fmt.Sprintf("/api/v1/boards/%s/scores", *boardID)

	// Keys are run-scoped: records expire after 24h, so a fixed key would turn the
	// next run's writes into no-ops.
	key := func(name string) string {
		return fmt.Sprintf("walk-%d-%s", time.Now().UnixMilli(), name)
	}

	w.step("Root")
	w.call("GET", "/", nil)

	w.step("POST create player (id is server-generated)")
	created := w.callWithKey("POST", "/api/v1/players", map[string]any{"player_name": "alice"}, key("player"))
	playerID := jsonField(created, "player_id")

	playerPath := "/api/v1/players/" + playerID
	standingPath := scorePath + "/" + playerID

	w.step("GET player")
	w.call("GET", playerPath, nil)

	w.step(fmt.Sprintf("PUT create board %q (201 first run, then 409 - already exists)", *boardID))
	w.call("PUT", "/api/v1/boards/"+*boardID, map[string]any{"board_name": "Demo Cup"})

	w.step("GET boards list (creation order; 'main' always exists)")
	w.call("GET", "/api/v1/boards", nil)

	w.step("GET board (status: active)")
	w.call("GET", "/api/v1/boards/"+*boardID, nil)

	w.step("PUT set score = 100 (first write enrolls the player on the board)")
	w.callWithKey("PUT", standingPath, map[string]any{"player_score": 100}, key("set"))

	incrementKey := key("increment")

	w.step("POST increment by 5 (100 -> 105)")
	w.callWithKey("POST", standingPath+"/increment", map[string]any{"amount": 5}, incrementKey)

	w.step("POST increment retried with the SAME Idempotency-Key (no-op, stays 105)")
	w.callWithKey("POST", standingPath+"/increment", map[string]any{"amount": 5}, incrementKey)

	w.step("POST increment reusing that key with a different amount -> 409")
	w.callWithKey("POST", standingPath+"/increment", map[string]any{"amount": 7}, incrementKey)

	w.step("GET leaderboard")
	w.call("GET", scorePath+"?limit=10", nil)

	w.step("GET single standing (score + 1-based rank)")
	w.call("GET", standingPath, nil)

	w.step("GET history (the ledger, newest first: the set + the increment, retry recorded once)")
	w.call("GET", standingPath+"/history", nil)

	w.step("GET global event feed (oldest first; 0-0 reads from the beginning)")
	w.call("GET", "/api/v1/events?after=0-0&limit=10", nil)

	w.step("POST close board")
	w.call("POST", "/api/v1/boards/"+*boardID+"/close", nil)

	w.step("GET board (status: closed)")
	w.call("GET", "/api/v1/boards/"+*boardID, nil)

	w.step("PUT score on closed board -> 409, no event recorded")
	w.call("PUT", standingPath, map[string]any{"player_score": 999})

	w.step("GET leaderboard still readable while closed")
	w.call("GET", scorePath, nil)

	w.step("POST reopen board")
	w.call("POST", "/api/v1/boards/"+*boardID+"/open", nil)

	w.step("POST increment works again (105 -> 106)")
	w.callWithKey("POST", standingPath+"/increment", map[string]any{"amount": 1}, key("reopen-increment"))

	adminPath := fmt.Sprintf("/api/v1/admin/boards/%s/projection", *boardID)

	w.step("GET admin verify projection (empty mismatches means no drift)")
	w.call("GET", adminPath+"/verify", nil)

	w.step("POST admin rebuild projection from this board's ledger events")
	w.call("POST", adminPath+"/rebuild", nil)

	w.step("GET admin verify projection after rebuild")
	w.call("GET", adminPath+"/verify", nil)
}

type walker struct {
	rc    *resty.Client
	color bool
}

func newWalker(baseURL string, color bool) *walker {
	return &walker{rc: resty.New().SetBaseURL(baseURL), color: color}
}

// useColor resolves the -color mode; "auto" colorizes only when stdout is a terminal.
func useColor(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		info, err := os.Stdout.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
}

func (w *walker) paint(color, s string) string {
	if !w.color {
		return s
	}
	return color + s + ansiReset
}

func (w *walker) step(title string) {
	fmt.Printf("\n%s\n", w.paint(ansiBold+ansiCyan, title))
}

// call executes the request and prints "HTTP <code> <method> <path>" plus the body.
// It never aborts the walk: transport errors are printed and a nil response returned.
func (w *walker) call(method, path string, body any) *resty.Response {
	return w.callWithKey(method, path, body, "")
}

// callWithKey is call plus an Idempotency-Key header (skipped when key is empty).
func (w *walker) callWithKey(method, path string, body any, key string) *resty.Response {
	req := w.rc.R()
	if body != nil {
		req.SetHeader("Content-Type", "application/json").SetBody(body)
	}
	if key != "" {
		req.SetHeader("Idempotency-Key", key)
		fmt.Printf("%s\n", w.paint(ansiGray, "Idempotency-Key: "+key))
	}
	resp, err := req.Execute(method, path)
	if err != nil {
		fmt.Printf("%s %s\n", w.paint(ansiRed, "ERROR"), w.paint(ansiRed, method+" "+path+": "+err.Error()))
		return nil
	}
	status := fmt.Sprintf("HTTP %d", resp.StatusCode())
	fmt.Printf("%s %s\n",
		w.paint(statusColor(resp.StatusCode()), status),
		w.paint(ansiGray, method+" "+path),
	)
	if trimmed := strings.TrimSpace(string(resp.Body())); trimmed != "" {
		fmt.Println(trimmed)
	}
	return resp
}

func statusColor(code int) string {
	switch {
	case code >= 500:
		return ansiRed
	case code >= 400:
		return ansiYellow
	case code >= 200 && code < 300:
		return ansiGreen
	default:
		return ansiGray
	}
}

func jsonField(resp *resty.Response, key string) string {
	if resp == nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(resp.Body(), &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
