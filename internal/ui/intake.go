// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package ui

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/services/intake"
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

// GetIntakeProcessForm returns an inline form for manually testing a release against
// the intake pipeline (HTMX partial, swapped into #intake-process-modal).
func (h *Handler) GetIntakeProcessForm(w http.ResponseWriter, r *http.Request) {
	baseURL := h.baseURL()
	body := `<div class="rounded-lg border bg-card p-5 space-y-4 mb-4">
  <div class="flex items-center justify-between">
    <p class="font-medium text-sm">Test Release</p>
    <button class="text-xs text-muted-foreground hover:text-foreground"
      hx-get="data:text/html," hx-target="#intake-process-modal" hx-swap="innerHTML">✕</button>
  </div>
  <p class="text-xs text-muted-foreground">
    Enter a release name to evaluate it through all active intake pipelines without committing any action.
  </p>
  <form hx-post="` + baseURL + `/ui/partials/intake/process"
        hx-target="#intake-process-modal" hx-swap="innerHTML" class="space-y-3">
    <div class="space-y-1.5">
      <label class="text-xs font-medium text-muted-foreground" for="intake-release-name">Release Name</label>
      <input id="intake-release-name" name="release_name" type="text" required autofocus
        class="flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
        placeholder="e.g., Show.S01E01.WEB-DL.x264-GROUP"/>
    </div>
    <div class="flex items-center gap-2">
      <button type="submit"
        class="inline-flex items-center rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors">
        Run Test
      </button>
      <button type="button"
        class="inline-flex items-center rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium hover:bg-accent transition-colors"
        hx-get="data:text/html," hx-target="#intake-process-modal" hx-swap="innerHTML">
        Cancel
      </button>
    </div>
  </form>
</div>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, body)
}

// PostIntakeProcess processes a test release through the intake pipeline and renders
// the result (HTMX partial, swapped into #intake-process-modal).
func (h *Handler) PostIntakeProcess(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	releaseName := r.FormValue("release_name")
	baseURL := h.baseURL()

	if releaseName == "" {
		body := `<div class="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
  Release name is required.
  <button class="ml-2 underline text-xs" hx-get="` + baseURL + `/ui/partials/intake/process-form"
    hx-target="#intake-process-modal" hx-swap="innerHTML">Try again</button>
</div>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
		return
	}

	if h.intakeService == nil {
		body := `<div class="rounded-lg border border-amber-500/50 bg-amber-50 dark:bg-amber-950/20 p-4 text-sm text-amber-700 dark:text-amber-400">
  Intake service is not available (no pipelines configured).
</div>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
		return
	}

	ctx := r.Context()
	result, err := h.intakeService.Process(ctx, intake.Request{
		OwnerID:     1,
		ReleaseName: releaseName,
	})

	var body string
	if err != nil {
		log.Warn().Err(err).Str("release", releaseName).Msg("[UI] intake: process test failed")
		body = `<div class="rounded-lg border border-destructive/50 bg-destructive/10 p-4 space-y-2">
  <p class="text-sm font-medium text-destructive">Processing failed</p>
  <p class="text-xs text-destructive/80">` + err.Error() + `</p>
  <button class="text-xs underline text-muted-foreground" hx-get="` + baseURL + `/ui/partials/intake/process-form"
    hx-target="#intake-process-modal" hx-swap="innerHTML">Try again</button>
</div>`
	} else if result != nil {
		actionClass := "bg-muted text-muted-foreground"
		switch string(result.Action) {
		case "grab":
			actionClass = "bg-green-500/15 text-green-700 dark:text-green-400"
		case "skip":
			actionClass = "bg-muted text-muted-foreground"
		case "remove":
			actionClass = "bg-red-500/15 text-red-700 dark:text-red-400"
		case "upgrade":
			actionClass = "bg-blue-500/15 text-blue-700 dark:text-blue-400"
		}
		matchedStr := "No match"
		if result.Matched && result.Event != nil && result.Event.MatchedTitle != "" {
			matchedStr = result.Event.MatchedTitle
		}
		ruleStr := "—"
		if result.Event != nil && result.Event.MatchedRuleName != "" {
			ruleStr = result.Event.MatchedRuleName
		}
		body = fmt.Sprintf(`<div class="rounded-lg border bg-card p-5 space-y-4">
  <div class="flex items-center justify-between">
    <p class="font-medium text-sm">Test Result</p>
    <button class="text-xs text-muted-foreground hover:text-foreground"
      hx-get="data:text/html," hx-target="#intake-process-modal" hx-swap="innerHTML">✕</button>
  </div>
  <div class="grid gap-3 sm:grid-cols-2">
    <div class="space-y-1">
      <p class="text-xs text-muted-foreground">Release</p>
      <p class="text-sm font-mono">%s</p>
    </div>
    <div class="space-y-1">
      <p class="text-xs text-muted-foreground">Action</p>
      <span class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium %s">%s</span>
    </div>
    <div class="space-y-1">
      <p class="text-xs text-muted-foreground">Matched Title</p>
      <p class="text-sm">%s</p>
    </div>
    <div class="space-y-1">
      <p class="text-xs text-muted-foreground">Matched Rule</p>
      <p class="text-sm">%s</p>
    </div>
  </div>
  <button class="text-xs underline text-muted-foreground" hx-get="`+baseURL+`/ui/partials/intake/process-form"
    hx-target="#intake-process-modal" hx-swap="innerHTML">Test another</button>
</div>`, releaseName, actionClass, string(result.Action), matchedStr, ruleStr)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, body)
}

// intakePipelinesPartial fetches all pipelines (with rules) and renders the list partial.
func (h *Handler) intakePipelinesPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var pipelines []*models.IntakePipeline
	if h.intakePipelineStore != nil {
		var err error
		pipelines, err = h.intakePipelineStore.ListPipelines(ctx, 1)
		if err != nil {
			log.Warn().Err(err).Msg("[UI] intake: failed to list pipelines")
		} else {
			for _, p := range pipelines {
				rules, err := h.intakePipelineStore.ListRules(ctx, p.ID)
				if err == nil {
					for _, rule := range rules {
						p.Rules = append(p.Rules, *rule)
					}
				}
			}
		}
	}

	render(w, r, http.StatusOK, pages.IntakePipelinesListPartial(pipelines, h.baseURL()))
}

// GetIntakePipelineForm renders the blank create-pipeline form.
func (h *Handler) GetIntakePipelineForm(w http.ResponseWriter, r *http.Request) {
	blank := &models.IntakePipeline{}
	render(w, r, http.StatusOK, pages.IntakePipelineFormPartial(blank, h.baseURL(), false))
}

// GetIntakePipelineFormEdit renders the edit-pipeline form pre-populated.
func (h *Handler) GetIntakePipelineFormEdit(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	pipeline, err := h.intakePipelineStore.GetPipeline(r.Context(), id)
	if err != nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	render(w, r, http.StatusOK, pages.IntakePipelineFormPartial(pipeline, h.baseURL(), true))
}

// PostIntakePipeline creates a new intake pipeline.
func (h *Handler) PostIntakePipeline(w http.ResponseWriter, r *http.Request) {
	if h.intakePipelineStore == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	pipeline := &models.IntakePipeline{
		OwnerID:      1,
		Name:         r.FormValue("name"),
		Enabled:      r.FormValue("enabled") == "on",
		SortOrder:    intParam(r.FormValue("sort_order"), 10),
		ContentTypes: r.Form["content_types"],
		Description:  r.FormValue("description"),
	}

	if _, err := h.intakePipelineStore.CreatePipeline(r.Context(), pipeline); err != nil {
		log.Warn().Err(err).Msg("[UI] failed to create pipeline")
		http.Error(w, "failed to create pipeline", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// PutIntakePipeline updates an existing intake pipeline.
func (h *Handler) PutIntakePipeline(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	existing, err := h.intakePipelineStore.GetPipeline(r.Context(), id)
	if err != nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	existing.Name = r.FormValue("name")
	existing.Enabled = r.FormValue("enabled") == "on"
	existing.SortOrder = intParam(r.FormValue("sort_order"), existing.SortOrder)
	existing.ContentTypes = r.Form["content_types"]
	existing.Description = r.FormValue("description")

	if _, err := h.intakePipelineStore.UpdatePipeline(r.Context(), existing); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to update pipeline")
		http.Error(w, "failed to update pipeline", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// DeleteIntakePipeline deletes an intake pipeline and its rules.
func (h *Handler) DeleteIntakePipeline(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.intakePipelineStore.DeletePipeline(r.Context(), id); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to delete pipeline")
		http.Error(w, "failed to delete pipeline", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// PostIntakePipelineToggle toggles the enabled state of an intake pipeline.
func (h *Handler) PostIntakePipelineToggle(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	pipeline, err := h.intakePipelineStore.GetPipeline(r.Context(), id)
	if err != nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	pipeline.Enabled = !pipeline.Enabled
	if _, err := h.intakePipelineStore.UpdatePipeline(r.Context(), pipeline); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to toggle pipeline")
		http.Error(w, "failed to toggle pipeline", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// GetIntakeRuleForm renders the blank create-rule form for a specific pipeline.
func (h *Handler) GetIntakeRuleForm(w http.ResponseWriter, r *http.Request) {
	pipelineID := intParam(chi.URLParam(r, "id"), 0)
	blank := &models.IntakeRule{PipelineID: pipelineID}
	render(w, r, http.StatusOK, pages.IntakeRuleFormPartial(blank, h.baseURL(), false))
}

// GetIntakeRuleFormEdit renders the edit-rule form pre-populated.
func (h *Handler) GetIntakeRuleFormEdit(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	rule, err := h.intakePipelineStore.GetRule(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	render(w, r, http.StatusOK, pages.IntakeRuleFormPartial(rule, h.baseURL(), true))
}

// PostIntakeRule creates a new rule under a pipeline.
func (h *Handler) PostIntakeRule(w http.ResponseWriter, r *http.Request) {
	pipelineID := intParam(chi.URLParam(r, "id"), 0)
	if pipelineID == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	rule := &models.IntakeRule{
		PipelineID: pipelineID,
		Name:       r.FormValue("name"),
		Enabled:    r.FormValue("enabled") == "on",
		SortOrder:  intParam(r.FormValue("sort_order"), 10),
		ExprFilter: r.FormValue("expr_filter"),
		Action:     models.LibraryAction(r.FormValue("action")),
	}

	if _, err := h.intakePipelineStore.CreateRule(r.Context(), rule); err != nil {
		log.Warn().Err(err).Msg("[UI] failed to create intake rule")
		http.Error(w, "failed to create rule", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// PutIntakeRule updates an existing intake rule.
func (h *Handler) PutIntakeRule(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	existing, err := h.intakePipelineStore.GetRule(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	existing.Name = r.FormValue("name")
	existing.Enabled = r.FormValue("enabled") == "on"
	existing.SortOrder = intParam(r.FormValue("sort_order"), existing.SortOrder)
	existing.ExprFilter = r.FormValue("expr_filter")
	existing.Action = models.LibraryAction(r.FormValue("action"))

	if _, err := h.intakePipelineStore.UpdateRule(r.Context(), existing); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to update intake rule")
		http.Error(w, "failed to update rule", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// DeleteIntakeRule removes an intake rule.
func (h *Handler) DeleteIntakeRule(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.intakePipelineStore.DeleteRule(r.Context(), id); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to delete intake rule")
		http.Error(w, "failed to delete rule", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}

// PostIntakeRuleToggle toggles the enabled state of an intake rule.
func (h *Handler) PostIntakeRuleToggle(w http.ResponseWriter, r *http.Request) {
	id := intParam(chi.URLParam(r, "id"), 0)
	if id == 0 || h.intakePipelineStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	rule, err := h.intakePipelineStore.GetRule(r.Context(), id)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	rule.Enabled = !rule.Enabled
	if _, err := h.intakePipelineStore.UpdateRule(r.Context(), rule); err != nil {
		log.Warn().Err(err).Int("id", id).Msg("[UI] failed to toggle intake rule")
		http.Error(w, "failed to toggle rule", http.StatusInternalServerError)
		return
	}

	h.intakePipelinesPartial(w, r)
}
