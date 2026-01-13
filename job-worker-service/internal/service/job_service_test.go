package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"job-worker-service/internal/entity"
	"job-worker-service/internal/repository/postgresql"
	"job-worker-service/internal/service"
)

type fakeRepo struct {
	createCalled int
	lastType     string
	lastInput    json.RawMessage
	lastPriority int

	createID  uuid.UUID
	createErr error
}

func (r *fakeRepo) Create(ctx context.Context, typ string, priority int, input json.RawMessage) (uuid.UUID, error) {

	r.createCalled++
	r.lastType = typ
	r.lastPriority = priority
	r.lastInput = input
	if r.createErr != nil {
		return uuid.Nil, r.createErr
	}
	return r.createID, nil
}

func (r *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (*entity.Job, error) {
	return nil, postgresql.ErrNotFound
}
func (r *fakeRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status entity.JobStatus) error {
	return nil
}
func (r *fakeRepo) SetResultDone(ctx context.Context, id uuid.UUID, output json.RawMessage) error {
	return nil
}
func (r *fakeRepo) SetResultError(ctx context.Context, id uuid.UUID, errText string) error {
	return nil
}

type fakeQueue struct {
	enqueuedIDs        []string
	enqueuedPriorities []int
	enqueueErr         error
}

func (q *fakeQueue) Enqueue(ctx context.Context, jobID string, priority int) error {
	q.enqueuedIDs = append(q.enqueuedIDs, jobID)
	q.enqueuedPriorities = append(q.enqueuedPriorities, priority)
	return q.enqueueErr
}

// Остальные методы интерфейса Queue нам в этих тестах не нужны, но они должны существовать
func (q *fakeQueue) ClaimBlocking(ctx context.Context, timeout time.Duration) (string, error) {
	return "", errors.New("not implemented")
}
func (q *fakeQueue) Ack(ctx context.Context, jobID string) error                { return nil }
func (q *fakeQueue) RequeueStale(ctx context.Context, max int64) (int64, error) { return 0, nil }

func TestJobService_CreateJob_PriorityPropagates(t *testing.T) {
	ctx := context.Background()
	id := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	repo := &fakeRepo{createID: id}
	queue := &fakeQueue{}
	svc := service.NewJobService(repo, queue)

	_, err := svc.CreateJob(ctx, service.CreateJobRequest{
		Type:     "echo",
		Priority: 2,
		Input:    json.RawMessage(`{"x":1}`),
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if repo.lastPriority != 2 {
		t.Fatalf("expected repo priority=2, got %d", repo.lastPriority)
	}
	if len(queue.enqueuedPriorities) != 1 || queue.enqueuedPriorities[0] != 2 {
		t.Fatalf("expected enqueue priority=2, got %#v", queue.enqueuedPriorities)
	}
}

func TestJobService_CreateJob_PriorityClampedToNormal(t *testing.T) {
	ctx := context.Background()
	id := uuid.MustParse("77777777-7777-7777-7777-777777777777")

	repo := &fakeRepo{createID: id}
	queue := &fakeQueue{}
	svc := service.NewJobService(repo, queue)

	_, err := svc.CreateJob(ctx, service.CreateJobRequest{
		Type:     "echo",
		Priority: 999, // invalid
		Input:    json.RawMessage(`{"x":1}`),
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if repo.lastPriority != 1 {
		t.Fatalf("expected repo priority=1 (clamped), got %d", repo.lastPriority)
	}
	if len(queue.enqueuedPriorities) != 1 || queue.enqueuedPriorities[0] != 1 {
		t.Fatalf("expected enqueue priority=1 (clamped), got %#v", queue.enqueuedPriorities)
	}
}
