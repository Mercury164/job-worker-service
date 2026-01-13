# job-worker-service (Redis Priority Queue + Worker)

Микросервис для асинхронной обработки длительных задач на Go с **PostgreSQL** и **Redis**.  
HTTP API принимает job, сохраняет в Postgres и ставит `job_id` в Redis очередь. Отдельный **worker** обрабатывает задачи в фоне, обновляет статус и сохраняет результат.

Ключевые фичи:
- reliable queue на Redis (**BRPOPLPUSH**: queue → processing + ACK)
- **priority lanes**: high / normal / low
- Swagger/OpenAPI (swaggo)
- Docker Compose (app + worker + postgres + redis)

## Сервисы
- **app** (8080): REST `POST /jobs`, `GET /jobs/{id}`, `GET /jobs/{id}/result`, `/health`, `/swagger`
- **worker**: слушает Redis очереди, обновляет `jobs.status`, пишет `output/error`
- **postgres**: хранит таблицу `jobs`
- **redis**: очередь задач (priority lanes + processing map)

## Запуск
```bash
docker compose up -d --build
