package httptransport

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"job-worker-service/internal/entity"
	"job-worker-service/internal/service"
)

type Handler struct {
	jobSvc *service.JobService
}

func NewHandler(jobSvc *service.JobService) *Handler {
	return &Handler{jobSvc: jobSvc}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, apiError{Message: msg})
}

type createJobDTO struct {
	Type     string                 `json:"type"`
	Priority *int                   `json:"priority,omitempty"` // 0=low,1=normal,2=high (nil => default 1)
	Input    map[string]interface{} `json:"input"`
}

type createJobResp struct {
	ID string `json:"id"`
}

type jobResp struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Status    entity.JobStatus       `json:"status"`
	Priority  int                    `json:"priority"`
	Input     map[string]interface{} `json:"input"`
	Output    map[string]interface{} `json:"output,omitempty"`
	Error     *string                `json:"error,omitempty"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// CreateJob godoc
// @Summary Create a new job
// @Description Creates job in DB (pending) and enqueues it for background processing.
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body createJobDTO true "job payload (priority: 0=low,1=normal,2=high)"
// @Success 201 {object} createJobResp
// @Failure 400 {object} apiError
// @Failure 500 {object} apiError
// @Router /jobs [post]
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var dto createJobDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	priority := 1
	if dto.Priority != nil {
		priority = *dto.Priority
	}

	rawInput, err := json.Marshal(dto.Input)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid input")
		return
	}

	id, err := h.jobSvc.CreateJob(r.Context(), service.CreateJobRequest{
		Type:     dto.Type,
		Priority: priority,
		Input:    rawInput,
	})
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, createJobResp{ID: id.String()})
}

// GetJob godoc
// @Summary Get job by id
// @Tags jobs
// @Produce json
// @Param id path string true "job id (uuid)"
// @Success 200 {object} jobResp
// @Failure 400 {object} apiError
// @Failure 404 {object} apiError
// @Router /jobs/{id} [get]
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	j, err := h.jobSvc.GetJob(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "job not found")
		return
	}

	resp := jobResp{
		ID:        j.ID.String(),
		Type:      j.Type,
		Status:    j.Status,
		Priority:  j.Priority,
		Error:     j.Error,
		CreatedAt: j.CreatedAt.Format(time.RFC3339),
		UpdatedAt: j.UpdatedAt.Format(time.RFC3339),
	}

	// input/output: json.RawMessage -> map
	if len(j.Input) > 0 {
		_ = json.Unmarshal(j.Input, &resp.Input)
	}
	if j.Status == entity.StatusDone && len(j.Output) > 0 {
		_ = json.Unmarshal(j.Output, &resp.Output)
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// GetJobResult godoc
// @Summary Get job result
// @Tags jobs
// @Produce json
// @Param id path string true "job id (uuid)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apiError
// @Failure 404 {object} apiError
// @Failure 409 {object} apiError
// @Router /jobs/{id}/result [get]
func (h *Handler) GetJobResult(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	j, err := h.jobSvc.GetJob(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if j.Status != entity.StatusDone {
		h.writeError(w, http.StatusConflict, "job not done")
		return
	}

	// отдаем raw json без лишнего \n
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(j.Output)
}
