package postgresql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"job-worker-service/internal/entity"
)

var ErrNotFound = errors.New("not found")

type JobRepository struct {
	pool *pgxpool.Pool
}

func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

func (r *JobRepository) Create(ctx context.Context, typ string, priority int, input json.RawMessage) (uuid.UUID, error) {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}

	const q = `
INSERT INTO jobs (type, status, priority, input)
VALUES ($1, 'pending', $2, $3)
RETURNING id;
`
	var id uuid.UUID
	if err := r.pool.QueryRow(ctx, q, typ, priority, input).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *JobRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.Job, error) {
	const q = `
SELECT id, type, status, priority, input, output, error, created_at, updated_at
FROM jobs
WHERE id = $1;
`

	var (
		job         entity.Job
		statusText  string
		inputBytes  []byte
		outputBytes []byte
		errText     *string
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&job.ID,
		&job.Type,
		&statusText,
		&job.Priority,
		&inputBytes,
		&outputBytes, // NULL => nil
		&errText,     // NULL => nil
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	job.Status = entity.JobStatus(statusText)
	job.Input = json.RawMessage(inputBytes)
	if outputBytes != nil {
		job.Output = json.RawMessage(outputBytes)
	} else {
		job.Output = nil
	}
	job.Error = errText
	job.CreatedAt = createdAt
	job.UpdatedAt = updatedAt

	return &job, nil
}

func (r *JobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status entity.JobStatus) error {
	const q = `UPDATE jobs SET status=$2 WHERE id=$1;`

	tag, err := r.pool.Exec(ctx, q, id, string(status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *JobRepository) SetResultDone(ctx context.Context, id uuid.UUID, output json.RawMessage) error {
	if len(output) == 0 {
		output = json.RawMessage(`{}`)
	}
	const q = `UPDATE jobs SET status='done', output=$2, error=NULL WHERE id=$1;`

	tag, err := r.pool.Exec(ctx, q, id, output)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *JobRepository) SetResultError(ctx context.Context, id uuid.UUID, errText string) error {
	const q = `UPDATE jobs SET status='error', error=$2 WHERE id=$1;`

	tag, err := r.pool.Exec(ctx, q, id, errText)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
