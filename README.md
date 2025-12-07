# Link Checker

Go сервер для проверки доступности ссылок и генерации PDF отчетов.

## Запуск

```bash
go mod download
go run cmd/server/main.go
```

Сервер стартует на `http://localhost:8080`

## API

### 1. Проверить ссылки (POST /check)
```bash
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"links": ["https://google.com", "https://github.com"]}'
```

Ответ:
```json
{"batch_id": 1, "links": [...], "message": "Links are being checked..."}
```

### 2. Статус проверки (GET /status?batch_id=1)
```bash
curl http://localhost:8080/status?batch_id=1
```

Ответ:
```json
{"batch_id": 1, "status": "completed", "urls": [...], "results": [...]}
```

### 3. PDF отчет (GET /report?batch_ids=1)
```bash
curl http://localhost:8080/report?batch_ids=1 --output report.pdf
```

### 4. Проверка здоровья (GET /health)
```bash
curl http://localhost:8080/health
```

## Работа

- Ссылки проверяются асинхронно в фоне
- Результаты сохраняются в папку `data/`
- При перезапуске незавершенные проверки автоматически возобновляются
- Для корректного завершения используйте Ctrl+C
