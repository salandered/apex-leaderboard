package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type healthRepoStub struct {
	pingErr   error
	pingCalls int
}

func (s *healthRepoStub) Ping(context.Context) error {
	s.pingCalls++
	return s.pingErr
}

func TestHandleLiveReturnsOkWithoutCheckingStorage(t *testing.T) {
	store := &healthRepoStub{pingErr: errors.New("must not be called")}
	handler := &HealthHandler{Storage: store}
	recorder := httptest.NewRecorder()

	handler.HandleLive(recorder, httptest.NewRequest(http.MethodGet, "/livez", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"ok"}`, recorder.Body.String())
	require.Equal(t, 0, store.pingCalls) // Ping wasn't called
}

func TestHandleReadyReturnsOkWhenStorageResponds(t *testing.T) {
	store := &healthRepoStub{} // nil instead of error
	handler := &HealthHandler{Storage: store}
	recorder := httptest.NewRecorder()

	handler.HandleReady(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"ok"}`, recorder.Body.String())
	require.Equal(t, 1, store.pingCalls)
}

func TestHandleReadyReturnsServiceUnavailableAndNamesFailedDependency(t *testing.T) {
	store := &healthRepoStub{pingErr: errors.New("redis unavailable")}
	handler := &HealthHandler{Storage: store}
	recorder := httptest.NewRecorder()

	handler.HandleReady(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"status":"unavailable","dependency":"redis"}`, recorder.Body.String())
	require.Equal(t, 1, store.pingCalls)
}
