package service

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type Queue interface {
	Enqueue(ctx context.Context, jobID string, priority int) error
	ClaimBlocking(ctx context.Context, timeout time.Duration) (string, error)
	Ack(ctx context.Context, jobID string) error
	RequeueStale(ctx context.Context, maxPerLane int64) (int64, error)
}

type Lane struct {
	QueueKey      string
	ProcessingKey string
}

// redisPriorityQueue implements a reliable queue with priorities using Redis lists.
// Lanes: high/normal/low.
// Claim: BRPOPLPUSH lane.queue -> lane.processing
// Ack:   LREM from correct processing list (stored in processingMapKey hash)
type redisPriorityQueue struct {
	rdb              *redis.Client
	processingMapKey string

	low    Lane
	normal Lane
	high   Lane
}

func NewRedisPriorityQueue(rdb *redis.Client, processingMapKey string, low, normal, high Lane) Queue {
	return &redisPriorityQueue{
		rdb:              rdb,
		processingMapKey: processingMapKey,
		low:              low,
		normal:           normal,
		high:             high,
	}
}

func clampPriority(p int) int {
	if p < 0 {
		return 0
	}
	if p > 2 {
		return 2
	}
	return p
}

func (q *redisPriorityQueue) laneByPriority(p int) Lane {
	switch clampPriority(p) {
	case 2:
		return q.high
	case 1:
		return q.normal
	default:
		return q.low
	}
}

func (q *redisPriorityQueue) Enqueue(ctx context.Context, jobID string, priority int) error {
	ln := q.laneByPriority(priority)
	return q.rdb.LPush(ctx, ln.QueueKey, jobID).Err()
}

// ClaimBlocking tries high->normal->low with small blocking slots,
// so it is "mostly blocking" but still respects priority.
func (q *redisPriorityQueue) ClaimBlocking(ctx context.Context, timeout time.Duration) (string, error) {
	// if timeout <= 0, loop forever (like a worker daemon)
	forever := timeout <= 0
	deadline := time.Now().Add(timeout)

	slot := 1 * time.Second
	if !forever && timeout < slot {
		slot = timeout
	}

	for {
		// stop if timed out
		if !forever && time.Now().After(deadline) {
			return "", redis.Nil
		}

		// try lanes in priority order
		for _, ln := range []Lane{q.high, q.normal, q.low} {
			wait := slot
			if !forever {
				remain := time.Until(deadline)
				if remain <= 0 {
					return "", redis.Nil
				}
				if remain < wait {
					wait = remain
				}
			}

			id, err := q.rdb.BRPopLPush(ctx, ln.QueueKey, ln.ProcessingKey, wait).Result()
			if err == nil {
				// remember which processing list holds this id (for Ack)
				if hErr := q.rdb.HSet(ctx, q.processingMapKey, id, ln.ProcessingKey).Err(); hErr != nil {
					// can't safely ack later => return error
					return "", hErr
				}
				return id, nil
			}

			if errors.Is(err, redis.Nil) {
				// nothing in this lane during wait slot
				continue
			}
			return "", err
		}
	}
}

func (q *redisPriorityQueue) Ack(ctx context.Context, jobID string) error {
	processingKey, err := q.rdb.HGet(ctx, q.processingMapKey, jobID).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// mapping is missing (e.g. old jobs or manual вмешательство) — попробуем удалить из всех processing
			_ = q.rdb.LRem(ctx, q.high.ProcessingKey, 1, jobID).Err()
			_ = q.rdb.LRem(ctx, q.normal.ProcessingKey, 1, jobID).Err()
			_ = q.rdb.LRem(ctx, q.low.ProcessingKey, 1, jobID).Err()
			return nil
		}
		return err
	}

	if err := q.rdb.LRem(ctx, processingKey, 1, jobID).Err(); err != nil {
		return err
	}
	_ = q.rdb.HDel(ctx, q.processingMapKey, jobID).Err()
	return nil
}

// RequeueStale moves items from processing back to queue per lane.
// It's a simple "reaper": at-least-once delivery.
func (q *redisPriorityQueue) RequeueStale(ctx context.Context, maxPerLane int64) (int64, error) {
	var moved int64

	for _, ln := range []Lane{q.high, q.normal, q.low} {
		for i := int64(0); i < maxPerLane; i++ {
			id, err := q.rdb.RPopLPush(ctx, ln.ProcessingKey, ln.QueueKey).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					break
				}
				return moved, err
			}
			if id != "" {
				moved++
				_ = q.rdb.HDel(ctx, q.processingMapKey, id).Err()
			}
		}
	}

	return moved, nil
}
