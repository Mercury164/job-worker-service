// cmd/worker/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"job-worker-service/internal/repository/postgresql"
	"job-worker-service/internal/service"
	"job-worker-service/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pgDSN := mustEnv("POSTGRES_DSN")
	redisAddr := mustEnv("REDIS_ADDR")

	queueKey := envOr("REDIS_QUEUE_KEY", "jobs:queue")
	processingKey := envOr("REDIS_PROCESSING_KEY", "jobs:processing")
	workersCount := envIntOr("WORKERS", 4)

	// Postgres
	pool, err := postgresql.NewPool(ctx, pgDSN)
	if err != nil {
		log.Fatalf("pg: %v", err)
	}
	defer pool.Close()

	// Redis
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	// DI
	repo := postgresql.NewJobRepository(pool)
	baseQueueKey := envOr("REDIS_QUEUE_KEY", "jobs:queue")
	baseProcessingKey := envOr("REDIS_PROCESSING_KEY", "jobs:processing")
	processingMapKey := envOr("REDIS_PROCESSING_MAP_KEY", baseProcessingKey+":map")

	queue := service.NewRedisPriorityQueue(
		rdb,
		processingMapKey,
		service.Lane{QueueKey: baseQueueKey + ":low", ProcessingKey: baseProcessingKey + ":low"},
		service.Lane{QueueKey: baseQueueKey + ":normal", ProcessingKey: baseProcessingKey + ":normal"},
		service.Lane{QueueKey: baseQueueKey + ":high", ProcessingKey: baseProcessingKey + ":high"},
	)

	// ✅ Reaper: периодически возвращает jobs из processing обратно в queue
	// (если воркер падал/перезапускался)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := queue.RequeueStale(ctx, 100)
				if err != nil {
					log.Printf("requeue error: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("requeued %d jobs from processing", n)
				}
			}
		}
	}()

	processor := worker.NewProcessor(repo)
	poolWorkers := worker.NewPool(queue, processor, workersCount)

	log.Printf("worker started: workers=%d", workersCount)
	poolWorkers.Run(ctx)

	log.Printf("[worker] config workers=%d redis_addr=%s queue_key=%s processing_key=%s postgres_dsn=%s",
		workersCount, redisAddr, queueKey, processingKey, redactDSN(pgDSN),
	)

	log.Println("worker stopped")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing env: %s", key)
	}
	return v
}

func envOr(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envIntOr(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func redactDSN(dsn string) string {
	// postgres://user:pass@host:5432/db?...
	// заменим пароль на **** если он есть
	// простая, но рабочая маска: user:pass@ -> user:****@
	// не ломает DSN без пароля
	re := regexp.MustCompile(`://([^:/?#]+):([^@/]+)@`)
	return re.ReplaceAllString(dsn, `://$1:****@`)
}
