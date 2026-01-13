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
```

Swagger UI: http://localhost:8080/swagger/index.html

## Priority (0/1/2)

Поле priority влияет на порядок обработки:
- 2 — high
- 1 — normal (default)
- 0 — low

Если priority не указан или некорректный — используется 1.

## Redis keys

Используются 3 очереди и 3 processing-листа:

- jobs:queue:high → jobs:processing:high

- jobs:queue:normal → jobs:processing:normal

- jobs:queue:low → jobs:processing:low

- Для корректного ACK хранится mapping:

- jobs:processing:map: job_id -> processing_list_key

Worker забирает задачи в порядке: high → normal → low.

### Тест 
```Powershell
$body = @{
  type     = "echo"
  priority = 2
  input    = @{ hello = "world" }
} | ConvertTo-Json -Depth 10

$resp = Invoke-RestMethod -Method Post `
  -Uri http://localhost:8080/jobs `
  -ContentType "application/json" `
  -Body $body

$resp

Invoke-RestMethod "http://localhost:8080/jobs/$($resp.id)"
Invoke-RestMethod "http://localhost:8080/jobs/$($resp.id)/result"
```

Проверить очереди Redis:
```Powershell
docker compose exec redis redis-cli LLEN jobs:queue:high
docker compose exec redis redis-cli LLEN jobs:queue:normal
docker compose exec redis redis-cli LLEN jobs:queue:low

docker compose exec redis redis-cli LLEN jobs:processing:high
docker compose exec redis redis-cli LLEN jobs:processing:normal
docker compose exec redis redis-cli LLEN jobs:processing:low

docker compose exec redis redis-cli HLEN jobs:processing:map
```
Логи:
```Powershell
docker compose logs -f app worker postgres redis
```

