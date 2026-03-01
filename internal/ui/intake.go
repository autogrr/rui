// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/ui/layouts"
	"github.com/autogrr/rui/internal/ui/pages"
)

// GetIntake renders the intake pipeline management page.
func (h *Handler) GetIntake(w http.ResponseWriter, r *http.Request) {
	username := UsernameFromContext(r.Context())
	ctx := r.Context()

	insts, _ := h.instanceStore.List(ctx)
	navInsts := make([]layouts.Instance, 0, len(insts))
	for _, inst := range insts {
		navInsts = append(navInsts, layouts.Instance{
			ID:       inst.ID,
			Name:     inst.Name,
			IsActive: inst.IsActive,
		})
	}

	props := pages.IntakeProps{
		BaseURL:     h.baseURL(),
		Username:    username,
		Version:     h.version,
		CurrentPath: "/ui/intake",
		Instances:   navInsts,
	}

	if h.intakePipelineStore != nil {
		pipelines, err := h.intakePipelineStore.ListPipelines(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] intake: failed to list pipelines")
		} else {
			// Populate rules for each pipeline.
			for _, p := range pipelines {
				rules, err := h.intakePipelineStore.ListRules(ctx, p.ID)
				if err == nil {
					for _, rule := range rules {
						p.Rules = append(p.Rules, *rule)
					}
				}
			}
			props.Pipelines = pipelines
		}
	}

	if h.intakeEventStore != nil {
		events, err := h.intakeEventStore.ListRecent(ctx, 1, 50)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] intake: failed to list events")
		} else {
			props.Events = events
		}
	}

	render(w, r, http.StatusOK, pages.Intake(props))
}
