package worker

import (
	"context"
	"log"
	"time"

	"job-worker-service/internal/service"
)

type Pool struct {
	queue      service.Queue
	processor  *Processor
	workers    int
	claimDelay time.Duration
}

func NewPool(queue service.Queue, processor *Processor, workers int) *Pool {
	if workers <= 0 {
		workers = 4
	}
	return &Pool{
		queue:      queue,
		processor:  processor,
		workers:    workers,
		claimDelay: 5 * time.Second,
	}
}

func (p *Pool) Run(ctx context.Context) {
	log.Printf("worker pool started: workers=%d", p.workers)

	jobCh := make(chan string)

	// N воркеров
	for i := 0; i < p.workers; i++ {
		go func(n int) {
			for jobID := range jobCh {
				err := p.processor.Process(ctx, jobID)
				if err != nil {
					log.Printf("[worker-%d] process job %s error: %v", n, jobID, err)
				}

				// В любом случае ACK: job уже переведён в done/error в БД (или упал раньше).
				// Если Process() упал до обновления статуса — тогда reaper вернёт id обратно.
				if ackErr := p.queue.Ack(ctx, jobID); ackErr != nil {
					log.Printf("[worker-%d] ack job %s error: %v", n, jobID, ackErr)
				}
			}
		}(i + 1)
	}

	// Listener: atomically claim from queue -> processing
	for {
		select {
		case <-ctx.Done():
			close(jobCh)
			log.Println("worker pool stopped")
			return
		default:
			jobID, err := p.queue.ClaimBlocking(ctx, p.claimDelay)
			if err != nil {
				// timeout/redis.Nil/ctx cancel — не фатально
				continue
			}
			select {
			case jobCh <- jobID:
			case <-ctx.Done():
				close(jobCh)
				return
			}
		}
	}
}
