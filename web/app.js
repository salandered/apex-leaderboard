const POLL_MS = 5000;
const PAGE_LIMIT = 50;

// The API reports errors as text/plain (http.Error), not JSON, so a failed response body
// must not be handed to res.json().
async function getJSON(url) {
	const res = await fetch(url);
	if (!res.ok) {
		throw new Error(`${res.status} - ${(await res.text()).trim()}`);
	}
	return res.json();
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

document.addEventListener("alpine:init", () => {
	Alpine.data("leaderboard", () => ({
		boards: [],
		boardId: "",
		rows: [],
		total: 0,
		error: "",
		pollSeconds: POLL_MS / 1000,

		// The attribute is already set by the inline script in <head>; this only mirrors it so
		// the button icon can react to it.
		theme: document.documentElement.dataset.theme,

		toggleTheme() {
			this.theme = this.theme === "dark" ? "light" : "dark";
			document.documentElement.dataset.theme = this.theme;
			localStorage.setItem("theme", this.theme);
		},

		async init() {
			await this.loadBoards();
			await this.refresh();
			setInterval(() => this.refresh(), POLL_MS);
		},

		async loadBoards() {
			try {
				const data = await getJSON("/api/v1/boards");
				this.boards = data.boards;
				if (!this.boardId && this.boards.length > 0) {
					this.boardId = this.boards[0].board_id;
				}
				this.error = "";
			} catch (err) {
				this.error = String(err.message ?? err);
			}
		},

		async selectBoard() {
			this.rows = [];
			this.total = 0;
			await this.refresh();
		},

		async refresh() {
			if (!this.boardId) {
				return;
			}
			try {
				const board = encodeURIComponent(this.boardId);
				const data = await getJSON(`/api/v1/boards/${board}/scores?limit=${PAGE_LIMIT}`);
				const names = await Promise.all(data.scores.map(r => playerName(r.player_id)));

				// One assignment after every name resolved, so the table never renders a
				// half filled state
				this.rows = data.scores.map((r, i) => ({ ...r, player_name: names[i] }));
				this.total = data.total;
				this.error = "";
			} catch (err) {
				this.error = String(err.message ?? err);
			}
		},
	}));
});
