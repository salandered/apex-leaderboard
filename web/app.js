const POLL_MS = 5000;
// The API caps a leaderboard page at 100 (maxListLimit) and rejects anything outside [1, 100]
// with a 400, so every offered size has to stay inside that range.
const PAGE_SIZES = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 25;
const EVENT_LIMIT = 100; // maxEventLimit on the API
const TICKER_MAX = 30;   // rows kept in the DOM; the ledger itself is never trimmed

// A stored cursor older than this starts at the tail instead of resuming. Previously displayed
// rows are still kept; new events shift the oldest ones out of the capped list.
const TICKER_RESUME_MS = 15 * 60 * 1000;

// "theme" is written by the inline script in <head> and stays a raw string; everything here is
// JSON, so the two never share a key.
const BOARD_KEY = "board_id";
const PAGE_SIZE_KEY = "page_size";
const CURSOR_KEY = "ticker_cursor";
const EVENTS_KEY = "ticker_events";
const WRITE_LOGS_KEY = "write_logs";
const WRITE_LOG_LIMIT = 5;

// Retain a key after a failed request because a network error does not prove that the server
// rejected it. Retrying the same payload in this browser session then remains a no-op.
const pendingWriteKeys = new Map();

const PLAYER_NAMES = [
	"Alice", "Bob", "Carol", "Dave", "Erin", "Frank", "Grace", "Heidi", "Ivan", "Judy",
	"Mallory", "Steven Even", "Todd Odd", "Elwood Shannon"
];
const BOARD_FIRST_WORDS = [
	"weekly", "daily", "boldly", "wildly", "softly", "loosely", "monthly", "nightly", "hourly", "friendly", "lovely", "yearly", "silly"
];
const BOARD_SECOND_WORDS = [
	"board", "backboard", "gourd", "cardboard", "keyboard", "dashboard", "checkerboard", "breadboard",
	"soundboard", "surfboard", "floorboard", "billboard", "sketchboard", "chipboard", "hardboard", "blockboard"
];

function randomItem(items) {
	return items[Math.floor(Math.random() * items.length)];
}

function capitalize(word) {
	return word[0].toUpperCase() + word.slice(1);
}

// The API reports errors as text/plain (http.Error), not JSON, so a failed response body
// must not be handed to res.json().
async function getJSON(url) {
	const res = await fetch(url);
	if (!res.ok) {
		throw new Error(`${res.status} - ${(await res.text()).trim()}`);
	}
	return res.json();
}

async function sendJSON(url, method, body, headers = {}) {
	const options = { method, headers: { ...headers } };
	if (body !== undefined) {
		options.headers["Content-Type"] = "application/json";
		options.body = JSON.stringify(body);
	}
	const res = await fetch(url, options);
	if (!res.ok) {
		throw new Error(`${res.status} - ${(await res.text()).trim()}`);
	}
	if (res.status === 204) {
		return null;
	}
	const text = await res.text();
	return text === "" ? null : JSON.parse(text);
}

async function sendIdempotentJSON(url, method, body) {
	const signature = `${method} ${url}\n${JSON.stringify(body)}`;
	let key = pendingWriteKeys.get(signature);
	if (key === undefined) {
		key = crypto.randomUUID();
		pendingWriteKeys.set(signature, key);
	}

	const result = await sendJSON(url, method, body, { "Idempotency-Key": key });
	pendingWriteKeys.delete(signature);
	return result;
}

// Standings rows carry player_id only, so a board of N rows costs N extra lookups. 
// TODO: delete when adding batched HGETALL join in ListScoresResp (MVP-1 deferred work)
// Keyed by player id and shared across polls and boards. 
const nameCache = new Map();

async function fetchPlayerName(playerId) {
	const data = await getJSON(`/api/v1/players/${encodeURIComponent(playerId)}`);
	return data.player_name;
}

function playerName(playerId) {
	let pending = nameCache.get(playerId);
	if (pending === undefined) {
		pending = fetchPlayerName(playerId).catch(() => {
			nameCache.delete(playerId); // next poll retry instead of caching a failure
			return playerId;
		});
		nameCache.set(playerId, pending);
	}
	return pending;
}

// A stored entry can be corrupt or written by an older shape of this page, so a bad one is
// dropped rather than allowed to break init.
function readStored(key, fallback) {
	try {
		const raw = localStorage.getItem(key);
		return raw === null ? fallback : JSON.parse(raw);
	} catch {
		removeStored(key);
		return fallback;
	}
}

function writeStored(key, value) {
	try {
		localStorage.setItem(key, JSON.stringify(value));
	} catch {
		// Persistence is optional. The page still works when storage is blocked or full.
	}
}

function removeStored(key) {
	try {
		localStorage.removeItem(key);
	} catch {
		// Persistence is optional. The page still works when storage is blocked.
	}
}

document.addEventListener("alpine:init", () => {
	Alpine.data("leaderboard", () => ({
		boards: [],
		boardId: "",
		rows: [],
		total: 0,
		limit: DEFAULT_PAGE_SIZE,
		offset: 0,
		error: "",
		pollSeconds: POLL_MS / 1000,
		writeBusy: false,
		writeLogs: [],
		newBoard: { id: "", name: "", status: "active" },
		newPlayerName: "",
		scoreWrite: { boardId: "", playerId: "", operation: "set", amount: 0 },
		brandColors: ["", "vibrant-orange", "vibrant-orange-2", "vibrant-orange-3"],
		brandColorIndex: 0,
		boardBusy: false,
		boardActionError: "",
		copiedPlayerId: "",
		historyPlayerId: "",
		historyPlayerName: "",
		historyEvents: [],
		historyLoading: false,
		historyError: "",
		historyRequestId: 0,
		refreshRequestId: 0,

		// The ledger is global, so the ticker deliberately ignores the selected board and
		// labels each row with the board it landed on.
		events: [],
		eventCursor: "",
		tickerError: "",
		tickerPolling: false,

		// The attribute is already set by the inline script in <head>; this only mirrors it so
		// the button icon can react to it.
		theme: document.documentElement.dataset.theme,

		// Ranks come from the API already absolute (the row at result index i has rank
		// offset+i+1), so the pager only ever describes the window, never renumbers it.
		get pageCount() {
			return Math.max(1, Math.ceil(this.total / this.limit));
		},

		get pageLabel() {
			return `page ${Math.floor(this.offset / this.limit) + 1} of ${this.pageCount}`;
		},

		get rangeLabel() {
			if (this.boards.length === 0) {
				return "no boards";
			}
			if (this.total === 0) {
				return "no players";
			}
			return `${this.offset + 1}-${this.offset + this.rows.length} of ${this.total} players`;
		},

		get hasPrev() {
			return this.offset > 0;
		},

		get hasNext() {
			return this.offset + this.limit < this.total;
		},

		get selectedBoard() {
			return this.boards.find(b => b.board_id === this.boardId) ?? null;
		},

		get boardCreatedLabel() {
			if (!this.selectedBoard) {
				return "";
			}
			return `created ${new Date(this.selectedBoard.created_at).toLocaleDateString()}`;
		},

		get brandColor() {
			return this.brandColors[this.brandColorIndex];
		},

		cycleBrandColor() {
			this.brandColorIndex = (this.brandColorIndex + 1) % this.brandColors.length;
		},

		toggleTheme() {
			this.theme = this.theme === "dark" ? "light" : "dark";
			document.documentElement.dataset.theme = this.theme;
			try {
				localStorage.setItem("theme", this.theme);
			} catch {
				// The visible theme still changes when persistence is unavailable.
			}
		},

		async init() {
			const storedLimit = readStored(PAGE_SIZE_KEY, DEFAULT_PAGE_SIZE);
			this.limit = PAGE_SIZES.includes(storedLimit) ? storedLimit : DEFAULT_PAGE_SIZE;
			const storedLogs = readStored(WRITE_LOGS_KEY, []);
			this.writeLogs = Array.isArray(storedLogs)
				? storedLogs.filter(log => typeof log?.message === "string").slice(-WRITE_LOG_LIMIT)
				: [];
			await this.loadBoards();
			await this.restoreTicker();
			await this.refresh();
			setInterval(() => {
				this.refresh();
				this.pollEvents();
			}, POLL_MS);
		},

		async loadBoards() {
			try {
				const data = await getJSON("/api/v1/boards");
				const stored = readStored(BOARD_KEY, "");
				this.boards = data.boards;

				// boardId is assigned in the same tick as boards on purpose: x-model syncs the
				// select against whatever options exist when its effect runs, so restoring the
				// id any earlier would leave the dropdown showing a different board than the
				// one being fetched.
				//
				// A stored id can also name a board this instance does not have (a different or
				// a flushed redis), so it is checked against the list rather than trusted. An
				// empty id is never in the list either, which covers the first visit.
				const known = this.boards.some(b => b.board_id === stored);
				this.boardId = known ? stored : (this.boards[0]?.board_id ?? "");
				const scoreBoardKnown = this.boards.some(b => b.board_id === this.scoreWrite.boardId);
				if (!scoreBoardKnown) {
					this.scoreWrite.boardId = this.boardId;
				}
				this.error = "";
			} catch (err) {
				this.error = String(err.message ?? err);
			}
		},

		async selectBoard() {
			writeStored(BOARD_KEY, this.boardId);
			this.scoreWrite.boardId = this.boardId;
			this.boardActionError = "";
			this.clearHistory();
			this.rows = [];
			this.total = 0;
			this.offset = 0;
			await this.refresh();
		},

		async toggleBoard() {
			const board = this.selectedBoard;
			if (!board) {
				return;
			}

			this.boardBusy = true;
			this.boardActionError = "";
			const action = board.status === "closed" ? "open" : "close";
			try {
				const id = encodeURIComponent(board.board_id);
				await sendJSON(`/api/v1/boards/${id}/${action}`, "POST");
				board.status = action === "open" ? "active" : "closed";
			} catch (err) {
				this.boardActionError = String(err.message ?? err);
			} finally {
				this.boardBusy = false;
			}
		},

		async loadHistory(row) {
			const selection = window.getSelection();
			if (selection && !selection.isCollapsed) {
				return;
			}

			if (this.historyPlayerId === row.player_id) {
				this.clearHistory();
				return;
			}

			const requestId = ++this.historyRequestId;
			const boardId = this.boardId;
			const playerId = row.player_id;
			this.historyPlayerId = playerId;
			this.historyPlayerName = row.player_name;
			this.historyEvents = [];
			this.historyLoading = true;
			this.historyError = "";
			try {
				const board = encodeURIComponent(boardId);
				const player = encodeURIComponent(playerId);
				const data = await getJSON(`/api/v1/boards/${board}/scores/${player}/history?limit=10`);
				if (requestId === this.historyRequestId && this.historyPlayerId === playerId) {
					this.historyEvents = data.events;
				}
			} catch (err) {
				if (requestId === this.historyRequestId) {
					this.historyError = String(err.message ?? err);
				}
			} finally {
				if (requestId === this.historyRequestId) {
					this.historyLoading = false;
				}
			}
		},

		clearHistory() {
			this.historyRequestId++;
			this.historyPlayerId = "";
			this.historyPlayerName = "";
			this.historyEvents = [];
			this.historyLoading = false;
			this.historyError = "";
		},

		async copyPlayerId(playerId) {
			try {
				await navigator.clipboard.writeText(playerId);
				this.copiedPlayerId = playerId;
				setTimeout(() => {
					if (this.copiedPlayerId === playerId) {
						this.copiedPlayerId = "";
					}
				}, 1000);
			} catch (err) {
				this.error = `copy failed - ${String(err.message ?? err)}`;
			}
		},

		async createBoard() {
			if (this.writeBusy) {
				return;
			}
			this.writeBusy = true;
			try {
				const id = this.newBoard.id;
				await sendJSON(`/api/v1/boards/${encodeURIComponent(id)}`, "PUT", {
					board_name: this.newBoard.name,
					status: this.newBoard.status,
				});
				await this.loadBoards();
				this.boardId = id;
				this.scoreWrite.boardId = id;
				writeStored(BOARD_KEY, id);
				this.offset = 0;
				await this.refresh();
				this.appendWriteLog(`created board ${id}`);
				this.newBoard = { id: "", name: "", status: "active" };
			} catch (err) {
				this.appendWriteLog(String(err.message ?? err), "error");
			} finally {
				this.writeBusy = false;
			}
		},

		randomizeBoard() {
			const first = randomItem(BOARD_FIRST_WORDS);
			const second = randomItem(BOARD_SECOND_WORDS);
			this.newBoard.id = `${first}-${second}`;
			this.newBoard.name = `${capitalize(first)} ${capitalize(second)}`;
		},

		appendWriteLog(message, level = "success") {
			const entry = { id: `${Date.now()}-${Math.random()}`, level, message };
			this.writeLogs = [...this.writeLogs, entry].slice(-WRITE_LOG_LIMIT);
			writeStored(WRITE_LOGS_KEY, this.writeLogs);
		},

		async createPlayer() {
			if (this.writeBusy) {
				return;
			}
			this.writeBusy = true;
			try {
				const name = this.newPlayerName;
				const data = await sendIdempotentJSON("/api/v1/players", "POST", { player_name: name });
				this.scoreWrite.playerId = data.player_id;
				this.appendWriteLog(`created ${name} - ${data.player_id}`);
				this.newPlayerName = "";
			} catch (err) {
				this.appendWriteLog(String(err.message ?? err), "error");
			} finally {
				this.writeBusy = false;
			}
		},

		randomizePlayer() {
			const suffix = Math.random().toString(36).slice(2, 5).padEnd(3, "0").toUpperCase();
			this.newPlayerName = `${randomItem(PLAYER_NAMES)} ${suffix}`;
		},

		async writeScore() {
			if (this.writeBusy) {
				return;
			}
			this.writeBusy = true;
			try {
				const board = encodeURIComponent(this.scoreWrite.boardId);
				const player = encodeURIComponent(this.scoreWrite.playerId);
				const increment = this.scoreWrite.operation === "increment";
				const suffix = increment ? "/increment" : "";
				const method = increment ? "POST" : "PUT";
				const body = increment
					? { amount: this.scoreWrite.amount }
					: { player_score: this.scoreWrite.amount };
				await sendIdempotentJSON(`/api/v1/boards/${board}/scores/${player}${suffix}`, method, body);
				this.appendWriteLog(`${this.scoreWrite.operation} score on ${this.scoreWrite.boardId}`);
				await Promise.all([this.refresh(), this.pollEvents()]);
			} catch (err) {
				this.appendWriteLog(String(err.message ?? err), "error");
			} finally {
				this.writeBusy = false;
			}
		},

		// Rows per page changed, so the current offset points into a different page. Going back
		// to the top is predictable; trying to keep a row in view is not worth the arithmetic.
		async setPageSize() {
			writeStored(PAGE_SIZE_KEY, this.limit);
			this.offset = 0;
			await this.refresh();
		},

		async prevPage() {
			this.offset = Math.max(0, this.offset - this.limit);
			await this.refresh();
		},

		async nextPage() {
			this.offset += this.limit;
			await this.refresh();
		},

		// /events is forward-only and `after` is required, so "0-0" would replay the whole ledger
		// just to reach the tail - there is no reverse order and no latest-N form. Stream ids
		// are <unix millis>-<sequence> and the endpoint only validates that shape, so a cursor
		// built from the clock reads as "from now on". The boundary is only as accurate as this
		// browser's clock agrees with the redis one; both are on this host.
		//
		// A stored cursor carries its own age in that millisecond half. Resuming a recent one
		// closes the gap a reload would otherwise leave; resuming a days-old one would crawl the
		// backlog a page per tick, so anything staler than TICKER_RESUME_MS starts at the tail.
		async restoreTicker() {
			const storedCursor = readStored(CURSOR_KEY, "");
			const stored = readStored(EVENTS_KEY, []);
			const storedEvents = Array.isArray(stored) ? stored.slice(0, TICKER_MAX) : [];
			const cursor = storedCursor || storedEvents[0]?.event_id || "";
			const millis = Number.parseInt(cursor, 10);
			const fresh = Number.isFinite(millis) && Date.now() - millis < TICKER_RESUME_MS;

			if (!fresh) {
				if (Number.isFinite(millis) && !(await this.ledgerContinuesFrom(millis))) {
					this.eventCursor = `${Date.now()}-0`;
					this.events = [];
					removeStored(CURSOR_KEY);
					removeStored(EVENTS_KEY);
					return;
				}
				this.eventCursor = `${Date.now()}-0`;
				this.events = storedEvents;
				writeStored(CURSOR_KEY, this.eventCursor);
				return;
			}

			if (!(await this.ledgerContinuesFrom(millis))) {
				this.eventCursor = `${Date.now()}-0`;
				this.events = [];
				removeStored(CURSOR_KEY);
				removeStored(EVENTS_KEY);
				return;
			}

			this.eventCursor = cursor;
			this.events = storedEvents;
		},

		// Stored rows are only meaningful if the ledger they came from is still the same one.
		// A flushed (or otherwise reset) redis breaks that silently: the cursor keeps working,
		// no request fails, and the page happily shows events that exist nowhere any more.
		// The head of the feed settles it - an empty ledger has nothing to resume, and a head
		// newer than where we stopped means the stream restarted underneath us.
		async ledgerContinuesFrom(cursorMillis) {
			try {
				const data = await getJSON("/api/v1/events?after=0-0&limit=1");
				if (data.events.length === 0) {
					return false;
				}
				return Number.parseInt(data.events[0].event_id, 10) <= cursorMillis;
			} catch {
				// A probe that could not run is not evidence of a reset, so keep what we have
				// rather than discarding rows over a transient failure.
				return true;
			}
		},

		async pollEvents() {
			if (this.tickerPolling) {
				return;
			}
			this.tickerPolling = true;
			try {
				const after = encodeURIComponent(this.eventCursor);
				const data = await getJSON(`/api/v1/events?after=${after}&limit=${EVENT_LIMIT}`);
				this.tickerError = "";
				if (data.events.length === 0) {
					return; // next_after echoes the input cursor, so there is nothing to advance
				}

				const names = await Promise.all(data.events.map(e => playerName(e.player_id)));

				// The feed is oldest first; the ticker reads newest first.
				const fresh = data.events
					.map((e, i) => ({ ...e, player_name: names[i] }))
					.reverse();

				// A burst larger than EVENT_LIMIT is not lost: the cursor advances one page per
				// tick until the feed catches up.
				this.eventCursor = data.next_after;
				this.events = [...fresh, ...this.events].slice(0, TICKER_MAX);
				writeStored(CURSOR_KEY, this.eventCursor);
				writeStored(EVENTS_KEY, this.events);
			} catch (err) {
				// Kept apart from `error`: a failing ticker must not blank out the board's own
				// status line, and vice versa.
				this.tickerError = String(err.message ?? err);
			} finally {
				this.tickerPolling = false;
			}
		},

		// Events carry board_id only. The board list is already loaded for the picker, so this
		// costs no request; an event on a board not in the list falls back to the raw id.
		boardName(boardId) {
			return this.boards.find(b => b.board_id === boardId)?.board_name ?? boardId;
		},

		// The ticker prints the slug next to the name, but boardName falls back to the slug for
		// a board the picker does not know, and printing it twice would read as a bug.
		boardLabel(boardId) {
			const name = this.boardName(boardId);
			return name === boardId ? "" : name;
		},

		eventAmount(event) {
			if (event.type === "set") {
				return `= ${event.amount}`;
			}
			return event.amount >= 0 ? `+${event.amount}` : String(event.amount);
		},

		// Full UTC timestamps keep history and the live feed comparable. Milliseconds matter
		// because the ledger orders events by them.
		eventTimestamp(event) {
			return new Date(event.created_at).toISOString().replace("T", " ").slice(0, 23);
		},

		fetchPage(boardId, limit, offset) {
			const board = encodeURIComponent(boardId);
			return getJSON(`/api/v1/boards/${board}/scores?limit=${limit}&offset=${offset}`);
		},

		async refresh() {
			const requestId = ++this.refreshRequestId;
			const boardId = this.boardId;
			const limit = this.limit;
			let offset = this.offset;
			if (!boardId) {
				this.rows = [];
				this.total = 0;
				return;
			}
			try {
				let data = await this.fetchPage(boardId, limit, offset);
				if (requestId !== this.refreshRequestId) {
					return;
				}

				// A board can shrink under the poll (or under a projection rebuild) and leave
				// the offset past the end: the API answers an empty page while still reporting
				// the real total. Snap to the last page that has rows rather than showing an
				// empty table next to a live prev button.
				if (data.scores.length === 0 && offset > 0 && data.total > 0) {
					offset = (Math.ceil(data.total / limit) - 1) * limit;
					data = await this.fetchPage(boardId, limit, offset);
					if (requestId !== this.refreshRequestId) {
						return;
					}
				}

				const names = await Promise.all(data.scores.map(r => playerName(r.player_id)));
				if (requestId !== this.refreshRequestId) {
					return;
				}

				// One assignment after every name resolved, so the table never renders a
				// half filled state
				this.offset = offset;
				this.rows = data.scores.map((r, i) => ({ ...r, player_name: names[i] }));
				this.total = data.total;
				this.error = "";
			} catch (err) {
				if (requestId === this.refreshRequestId) {
					this.error = String(err.message ?? err);
				}
			}
		},
	}));
});
