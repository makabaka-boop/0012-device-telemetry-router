package httpapi

import (
	"net/http"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/service"
)

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var in service.RuleInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()}))
		return
	}
	rule, err := s.svc.CreateRule(operatorContext(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.svc.ListRules(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rules})
}

func (s *Server) handleGetRule(w http.ResponseWriter, r *http.Request) {
	rule, err := s.svc.GetRule(r.Context(), r.PathValue("rule_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("rule_id")
	var in service.RuleInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()}))
		return
	}
	rule, err := s.svc.UpdateRule(operatorContext(r), ruleID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("rule_id")
	if err := s.svc.DeleteRule(operatorContext(r), ruleID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"rule_id": ruleID, "deleted": "true"})
}

func (s *Server) handleRuleChanges(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("rule_id")
	changes, err := s.svc.RuleChanges(r.Context(), ruleID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": changes})
}
