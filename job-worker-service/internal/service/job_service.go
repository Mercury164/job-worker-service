package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"job-worker-service/internal/entity"
)

// Порт репозитория (реализация: postgresql.JobRepository)
type JobRepository interface {
	Create(ctx context.Context, typ string, priority int, input json.RawMessage) (uuid.UUID, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Job, error)
}

// Маленький порт очереди только для добавления задач в очередь.
// (Не называем Queue, чтобы не конфликтовать с queue_service.go)
type JobQueue interface {
	Enqueue(ctx context.Context, jobID string, priority int) error
}

type JobService struct {
	repo  JobRepository
	queue JobQueue
}

func NewJobService(repo JobRepository, queue JobQueue) *JobService {
	return &JobService{repo: repo, queue: queue}
}

type CreateJobRequest struct {
	Type     string
	Priority int
	Input    json.RawMessage
}

func (s *JobService) CreateJob(ctx context.Context, req CreateJobRequest) (uuid.UUID, error) {
	if req.Type == "" {
		return uuid.Nil, errors.New("type is required")
	}
	if len(req.Input) == 0 {
		req.Input = json.RawMessage(`{}`)
	}

	priority := req.Priority
	if priority < 0 || priority > 2 {
		priority = 1 // normal
	}

	id, err := s.repo.Create(ctx, req.Type, priority, req.Input)
	if err != nil {
		return uuid.Nil, err
	}

	if err := s.queue.Enqueue(ctx, id.String(), priority); err != nil {
		return uuid.Nil, err
	}

	return id, nil
}

func (s *JobService) GetJob(ctx context.Context, id uuid.UUID) (*entity.Job, error) {
	return s.repo.GetByID(ctx, id)
}
