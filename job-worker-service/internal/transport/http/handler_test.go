package httptransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"job-worker-service/internal/entity"
	"job-worker-service/internal/service"
	httptransport "job-worker-service/internal/transport/http"
)

// ---- fakes ----

type repoWithJobs struct {
	createID uuid.UUID
	jobs     map[uuid.UUID]*entity.Job
}

func (r *repoWithJobs) Create(ctx context.Context, typ string, priority int, input json.RawMessage) (uuid.UUID, error) {
	now := time.Now().UTC()

	j := &entity.Job{
		ID:        r.createID,
		Type:      typ,
		Status:    entity.StatusPending,
		Priority:  priority,
		Input:     input,
		Output:    json.RawMessage(`{}`),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if r.jobs == nil {
		r.jobs = map[uuid.UUID]*entity.Job{}
	}
	r.jobs[r.createID] = j
	return r.createID, nil
}

func (r *repoWithJobs) GetByID(ctx context.Context, id uuid.UUID) (*entity.Job, error) {
	j, ok := r.jobs[id]
	if !ok {
		return nil, context.Canceled // любой err => handler вернёт 404 (в твоей реализации)
	}
	return j, nil
}

type queueStub struct {
	enqueuedIDs        []string
	enqueuedPriorities []int
}

func (q *queueStub) Enqueue(ctx context.Context, jobID string, priority int) error {
	q.enqueuedIDs = append(q.enqueuedIDs, jobID)
	q.enqueuedPriorities = append(q.enqueuedPriorities, priority)
	return nil
}

// ---- helpers ----

func newTestRouter(repo service.JobRepository, queue service.JobQueue) http.Handler {
	svc := service.NewJobService(repo, queue)
	h := httptransport.NewHandler(svc)
	return httptransport.Routes(h)
}

// ---- tests ----

func TestHTTP_CreateJob_201_AndPriorityStored(t *testing.T) {
	id := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	repo := &repoWithJobs{createID: id, jobs: map[uuid.UUID]*entity.Job{}}
	queue := &queueStub{}
	router := newTestRouter(repo, queue)

	body := `{"type":"echo","priority":2,"input":{"hello":"world"}}`
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v, body=%s", err, rr.Body.String())
	}
	if resp.ID != id.String() {
		t.Fatalf("expected id=%s, got %s", id.String(), resp.ID)
	}

	// очередь получила priority=2
	if len(queue.enqueuedIDs) != 1 || queue.enqueuedIDs[0] != id.String() {
		t.Fatalf("expected enqueue id=%s, got %#v", id.String(), queue.enqueuedIDs)
	}
	if len(queue.enqueuedPriorities) != 1 || queue.enqueuedPriorities[0] != 2 {
		t.Fatalf("expected enqueue priority=2, got %#v", queue.enqueuedPriorities)
	}

	// GET /jobs/{id} должен вернуть priority=2
	req2 := httptest.NewRequest(http.MethodGet, "/jobs/"+id.String(), nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr2.Code, rr2.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v, body=%s", err, rr2.Body.String())
	}

	// числа в map[string]any становятся float64
	if got["priority"] != float64(2) {
		t.Fatalf("expected priority=2, got %v", got["priority"])
	}
}

func TestHTTP_CreateJob_DefaultPriorityIs1(t *testing.T) {
	id := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	repo := &repoWithJobs{createID: id, jobs: map[uuid.UUID]*entity.Job{}}
	queue := &queueStub{}
	router := newTestRouter(repo, queue)

	body := `{"type":"echo","input":{"hello":"world"}}` // без priority
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rr.Code, rr.Body.String())
	}

	if len(queue.enqueuedPriorities) != 1 || queue.enqueuedPriorities[0] != 1 {
		t.Fatalf("expected default priority=1, got %#v", queue.enqueuedPriorities)
	}
}

func TestHTTP_GetJobResult_409_WhenNotDone(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	repo := &repoWithJobs{
		createID: id,
		jobs: map[uuid.UUID]*entity.Job{
			id: {
				ID:        id,
				Type:      "echo",
				Status:    entity.StatusProcessing,
				Priority:  1,
				Input:     json.RawMessage(`{"a":1}`),
				Output:    json.RawMessage(`{"a":1}`),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}
	queue := &queueStub{}
	router := newTestRouter(repo, queue)

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id.String()+"/result", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTP_GetJobResult_200_WhenDone_ReturnsRawJSON(t *testing.T) {
	id := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	repo := &repoWithJobs{
		createID: id,
		jobs: map[uuid.UUID]*entity.Job{
			id: {
				ID:        id,
				Type:      "echo",
				Status:    entity.StatusDone,
				Priority:  1,
				Input:     json.RawMessage(`{"hello":"world"}`),
				Output:    json.RawMessage(`{"hello":"world"}`),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}
	queue := &queueStub{}
	router := newTestRouter(repo, queue)

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id.String()+"/result", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}

	got := strings.TrimSpace(rr.Body.String())
	if got != `{"hello":"world"}` {
		t.Fatalf("expected raw json output, got %s", got)
	}
}
