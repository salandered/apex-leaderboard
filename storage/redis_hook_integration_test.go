//go:build integration

package storage

import (
	"log/slog"
)

func (s *StorageSuite) TestRedisHookLogsEveryCommandAtDebugLevel() {
	s.createMainBoard()
	playerId := s.createPlayer("hooked")
	buf := captureLogs(s.T(), slog.LevelDebug)

	// when
	s.Require().NoError(
		s.storage.IncrementScore(s.ctx(), playerId, testBoardId, 25, "req-1", ""),
	)

	// then
	out := buf.String()
	s.Require().Contains(out, `"msg":"redis cmd"`)
	s.Require().Contains(out, `"cmd":"evalsha"`)
	s.Require().Contains(out, `"component":"storage"`)
}

func (s *StorageSuite) TestRedisHookLogsOneLinePerPipeline() {
	s.createMainBoard()
	playerId := s.createPlayer("hooked")
	s.Require().NoError(
		s.storage.IncrementScore(s.ctx(), playerId, testBoardId, 25, "req-1", ""),
	)
	buf := captureLogs(s.T(), slog.LevelDebug)

	// when
	_, _, err := s.storage.GetStanding(s.ctx(), playerId, testBoardId)

	// then
	s.Require().NoError(err)
	out := buf.String()
	s.Require().Contains(out, `"msg":"redis pipeline"`)
	s.Require().Contains(out, `"n":2`)
	s.Require().Contains(out, `"cmds":"zrevrank zcard"`)
}

// the whole point of the Enabled check in the hook: no cost, and no lines, below debug
func (s *StorageSuite) TestRedisHookStaysSilentBelowDebugLevel() {
	s.createMainBoard()
	playerId := s.createPlayer("hooked")
	buf := captureLogs(s.T(), slog.LevelInfo)

	s.Require().NoError(
		s.storage.IncrementScore(s.ctx(), playerId, testBoardId, 25, "req-1", ""),
	)

	s.Require().NotContains(buf.String(), "redis")
}
