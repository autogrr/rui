// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/intake"
)

// IntakeHandler handles REST endpoints for the intake pipeline.
type IntakeHandler struct {
	pipelineStore *models.IntakePipelineStore
	eventStore    *models.IntakeEventStore
	svc           *intake.Service
}

// NewIntakeHandler creates a new IntakeHandler.
func NewIntakeHandler(
	pipelineStore *models.IntakePipelineStore,
	eventStore *models.IntakeEventStore,
	svc *intake.Service,
) *IntakeHandler {
	return &IntakeHandler{
		pipelineStore: pipelineStore,
		eventStore:    eventStore,
		svc:           svc,
	}
}

// Routes registers all intake routes under r.
func (h *IntakeHandler) Routes(r chi.Router) {
	r.Route("/intake", func(r chi.Router) {
		r.Post("/process", h.Process)
		r.Get("/events", h.ListEvents)

		r.Route("/pipelines", func(r chi.Router) {
			r.Get("/", h.ListPipelines)
			r.Post("/", h.CreatePipeline)
			r.Get("/{id}", h.GetPipeline)
			r.Put("/{id}", h.UpdatePipeline)
			r.Delete("/{id}", h.DeletePipeline)

			r.Route("/{pipelineId}/rules", func(r chi.Router) {
				r.Get("/", h.ListRulesForPipeline)
				r.Post("/", h.CreatePipelineRule)
				r.Put("/{ruleId}", h.UpdatePipelineRule)
				r.Delete("/{ruleId}", h.DeletePipelineRule)
			})
		})
	})
}

// Process handles POST /api/intake/process
func (h *IntakeHandler) Process(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReleaseName string `json:"release_name"`
		PipelineID  *int   `json:"pipeline_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ReleaseName == "" {
		RespondError(w, http.StatusBadRequest, "release_name required")
		return
	}
	result, err := h.svc.Process(r.Context(), intake.Request{
		OwnerID:     1, // single-user: always user ID 1
		PipelineID:  req.PipelineID,
		ReleaseName: req.ReleaseName,
	})
	if err != nil {
		log.Error().Err(err).Str("release", req.ReleaseName).Msg("[INTAKE] process failed")
		RespondError(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

// ListEvents handles GET /api/intake/events?limit=N
func (h *IntakeHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	events, err := h.eventStore.ListRecent(r.Context(), 1, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	RespondJSON(w, http.StatusOK, events)
}

// ─── Pipelines ────────────────────────────────────────────────────────────────

// ListPipelines handles GET /api/intake/pipelines
func (h *IntakeHandler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	pipelines, err := h.pipelineStore.ListPipelines(r.Context(), 1)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list pipelines")
		return
	}
	RespondJSON(w, http.StatusOK, pipelines)
}

// CreatePipeline handles POST /api/intake/pipelines
func (h *IntakeHandler) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	var p models.IntakePipeline
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.OwnerID = 1
	created, err := h.pipelineStore.CreatePipeline(r.Context(), &p)
	if err != nil {
		log.Error().Err(err).Msg("[INTAKE] create pipeline failed")
		RespondError(w, http.StatusInternalServerError, "failed to create pipeline")
		return
	}
	RespondJSON(w, http.StatusCreated, created)
}

// GetPipeline handles GET /api/intake/pipelines/{id}
func (h *IntakeHandler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	p, err := h.pipelineStore.GetPipeline(r.Context(), id)
	if err != nil {
		if err == models.ErrIntakePipelineNotFound {
			RespondError(w, http.StatusNotFound, "pipeline not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}
	RespondJSON(w, http.StatusOK, p)
}

// UpdatePipeline handles PUT /api/intake/pipelines/{id}
func (h *IntakeHandler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var p models.IntakePipeline
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.ID = id
	updated, err := h.pipelineStore.UpdatePipeline(r.Context(), &p)
	if err != nil {
		if err == models.ErrIntakePipelineNotFound {
			RespondError(w, http.StatusNotFound, "pipeline not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update pipeline")
		return
	}
	RespondJSON(w, http.StatusOK, updated)
}

// DeletePipeline handles DELETE /api/intake/pipelines/{id}
func (h *IntakeHandler) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.pipelineStore.DeletePipeline(r.Context(), id); err != nil {
		if err == models.ErrIntakePipelineNotFound {
			RespondError(w, http.StatusNotFound, "pipeline not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to delete pipeline")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// ─── Pipeline Rules ───────────────────────────────────────────────────────────

// ListRulesForPipeline handles GET /api/intake/pipelines/{pipelineId}/rules
func (h *IntakeHandler) ListRulesForPipeline(w http.ResponseWriter, r *http.Request) {
	pid, ok := parseID(w, r, "pipelineId")
	if !ok {
		return
	}
	rules, err := h.pipelineStore.ListRules(r.Context(), pid)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	RespondJSON(w, http.StatusOK, rules)
}

// CreatePipelineRule handles POST /api/intake/pipelines/{pipelineId}/rules
func (h *IntakeHandler) CreatePipelineRule(w http.ResponseWriter, r *http.Request) {
	pid, ok := parseID(w, r, "pipelineId")
	if !ok {
		return
	}
	var rule models.IntakeRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.PipelineID = pid
	created, err := h.pipelineStore.CreateRule(r.Context(), &rule)
	if err != nil {
		log.Error().Err(err).Msg("[INTAKE] create rule failed")
		RespondError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}
	RespondJSON(w, http.StatusCreated, created)
}

// UpdatePipelineRule handles PUT /api/intake/pipelines/{pipelineId}/rules/{ruleId}
func (h *IntakeHandler) UpdatePipelineRule(w http.ResponseWriter, r *http.Request) {
	ruleID, ok := parseID(w, r, "ruleId")
	if !ok {
		return
	}
	var rule models.IntakeRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.ID = ruleID
	updated, err := h.pipelineStore.UpdateRule(r.Context(), &rule)
	if err != nil {
		if err == models.ErrIntakeRuleNotFound {
			RespondError(w, http.StatusNotFound, "rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}
	RespondJSON(w, http.StatusOK, updated)
}

// DeletePipelineRule handles DELETE /api/intake/pipelines/{pipelineId}/rules/{ruleId}
func (h *IntakeHandler) DeletePipelineRule(w http.ResponseWriter, r *http.Request) {
	ruleID, ok := parseID(w, r, "ruleId")
	if !ok {
		return
	}
	if err := h.pipelineStore.DeleteRule(r.Context(), ruleID); err != nil {
		if err == models.ErrIntakeRuleNotFound {
			RespondError(w, http.StatusNotFound, "rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}
