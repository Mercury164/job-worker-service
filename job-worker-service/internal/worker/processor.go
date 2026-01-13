package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"job-worker-service/internal/entity"
)

type JobRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Job, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status entity.JobStatus) error
	SetResultDone(ctx context.Context, id uuid.UUID, output json.RawMessage) error
	SetResultError(ctx context.Context, id uuid.UUID, errText string) error
}

type Processor struct {
	repo JobRepo
}

func NewProcessor(repo JobRepo) *Processor {
	return &Processor{repo: repo}
}

func (p *Processor) Process(ctx context.Context, jobID string) error {
	start := time.Now()

	id, err := uuid.Parse(jobID)
	if err != nil {
		log.Printf("[worker] job_id=%s parse_error=%v", jobID, err)
		return err
	}

	// статус -> processing
	if err := p.repo.UpdateStatus(ctx, id, entity.StatusProcessing); err != nil {
		log.Printf("[worker] job_id=%s update_status=processing error=%v", id.String(), err)
		return err
	}

	job, err := p.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("[worker] job_id=%s get_job error=%v", id.String(), err)
		return err
	}

	log.Printf("[worker] job_id=%s type=%s status=processing", id.String(), job.Type)

	out, procErr := doWork(job.Type, job.Input)
	if procErr != nil {
		msg := procErr.Error()
		_ = p.repo.SetResultError(ctx, id, msg)

		log.Printf("[worker] job_id=%s type=%s status=error duration_ms=%d error=%s",
			id.String(), job.Type, time.Since(start).Milliseconds(), msg,
		)
		return procErr
	}

	if err := p.repo.SetResultDone(ctx, id, out); err != nil {
		log.Printf("[worker] job_id=%s type=%s set_done error=%v", id.String(), job.Type, err)
		return err
	}

	log.Printf("[worker] job_id=%s type=%s status=done duration_ms=%d",
		id.String(), job.Type, time.Since(start).Milliseconds(),
	)
	return nil
}

func doWork(typ string, input json.RawMessage) (json.RawMessage, error) {
	switch typ {
	case "generate_report":
		time.Sleep(2 * time.Second)
		return json.RawMessage(`{"report_url":"https://example.local/report/123"}`), nil
	case "convert_video":
		time.Sleep(3 * time.Second)
		return json.RawMessage(`{"file_url":"https://example.local/video/converted.mp4"}`), nil
	case "echo":
		time.Sleep(1 * time.Second)
		// просто вернуть input
		if len(input) == 0 {
			return json.RawMessage(`{}`), nil
		}
		return input, nil
	default:
		return nil, errors.New("unknown job type: " + typ)
	}
}
