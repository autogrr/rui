// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/auth"
	"github.com/autogrr/rui/internal/database"
)

func TestAPIKeyFromQuery_AllowsQueryParam(t *testing.T) {
	ctx := t.Context()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	authService := auth.NewService(db)
	sessionManager := scs.New()

	apiKeyValue, _, err := authService.CreateAPIKey(ctx, "test-key")
	require.NoError(t, err)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := IsAuthenticated(authService, sessionManager, nil)
	handler := sessionManager.LoadAndSave(APIKeyFromQuery("apikey")(authMiddleware(okHandler)))

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/cross-seed/apply?apikey="+apiKeyValue, nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
}
