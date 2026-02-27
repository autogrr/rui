// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// ------------------------------------------------------------------
// GET /ui/automations — full page
// ------------------------------------------------------------------

func (h *Handler) GetAutomations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	insts := h.navInstances(r)
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	if instanceID == 0 && len(insts) > 0 {
		instanceID = insts[0].ID
	}

	p := pages.AutomationsProps{
		BaseURL:    h.baseURL(),
		Username:   UsernameFromContext(ctx),
		Version:    h.version,
		Instances:  insts,
		InstanceID: instanceID,
	}

	h.fillAutomationsData(ctx, &p, instanceID, insts)

	render(w, r, http.StatusOK, pages.Automations(p))
}

// ------------------------------------------------------------------
// GET /ui/partials/automations — HTMX table fragment
// ------------------------------------------------------------------

func (h *Handler) GetAutomationsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	insts := h.navInstances(r)
	instanceID := intParam(r.URL.Query().Get("instance_id"), 0)
	if instanceID == 0 && len(insts) > 0 {
		instanceID = insts[0].ID
	}

	p := pages.AutomationsProps{
		BaseURL:    h.baseURL(),
		Instances:  insts,
		InstanceID: instanceID,
	}

	h.fillAutomationsData(ctx, &p, instanceID, insts)

	render(w, r, http.StatusOK, pages.AutomationsPartial(p))
}

// ------------------------------------------------------------------
// POST /ui/partials/automations/{instanceId}/rules/{id}/toggle
// ------------------------------------------------------------------

func (h *Handler) PostAutomationToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	instanceID := intParam(chi.URLParam(r, "instanceId"), 0)
	id := intParam(chi.URLParam(r, "id"), 0)

	a, err := h.automationStore.Get(ctx, instanceID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	a.Enabled = !a.Enabled

	updated, err := h.automationStore.Update(ctx, a)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}

	insts := h.navInstances(r)
	instanceName := ""
	for _, inst := range insts {
		if inst.ID == instanceID {
			instanceName = inst.Name
			break
		}
	}

	item := pages.AutomationFromModel(updated, instanceName)
	render(w, r, http.StatusOK, pages.AutomationRowTempl(item, h.baseURL()))
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func (h *Handler) fillAutomationsData(ctx context.Context, p *pages.AutomationsProps, instanceID int, insts []layouts.Instance) {
	if h.automationStore == nil || instanceID == 0 {
		return
	}

	automations, err := h.automationStore.ListByInstance(ctx, instanceID)
	if err != nil {
		p.AutomationsError = err.Error()
	} else {
		instanceName := ""
		for _, inst := range insts {
			if inst.ID == instanceID {
				instanceName = inst.Name
				break
			}
		}
		for _, a := range automations {
			p.Automations = append(p.Automations, pages.AutomationFromModel(a, instanceName))
		}
	}

	if h.automationActivityStore == nil {
		return
	}

	activities, err := h.automationActivityStore.ListByInstance(ctx, instanceID, 50)
	if err == nil {
		for _, act := range activities {
			p.Activity = append(p.Activity, automationActivityItemFromModel(act))
		}
	}
}

func automationActivityItemFromModel(a *models.AutomationActivity) pages.AutomationActivityItem {
	return pages.AutomationActivityItem{
		ID: int64(a.ID),
		AutomationID: func() int {
			if a.RuleID != nil {
				return *a.RuleID
			}
			return 0
		}(),
		Name:        a.RuleName,
		TorrentHash: a.Hash,
		TorrentName: a.TorrentName,
		Action:      a.Action,
		Message:     a.Reason,
		CreatedAt:   a.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
