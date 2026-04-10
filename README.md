Bulk Service
============

Сервис массовой генерации изображений штрих-кодов. Принимает XLS/XLSX-файл с артикулами, валидирует его, сохраняет в объектное хранилище (MinIO/S3), ставит задания в очередь через Kafka, взаимодействует с биллинговой системой (BFF) и предоставляет REST API для управления пакетами (батчами).

Стек: **Go 1.25**, **Gin**, **Zap**, **PostgreSQL 15**, **Redis 7**, **Kafka**, **MinIO/S3**, **Prometheus**, **Jaeger (OpenTelemetry)**, **Testcontainers**.

---

Быстрый старт (локальная разработка)
-------------------------------------

1. Установить зависимости и собрать:

```powershell
go mod tidy
go build -o bulk.exe ./cmd/server
.\bulk.exe
```

2. Либо использовать Makefile:

```powershell
make build   # сборка бинаря
make run     # сборка + запуск
make test    # юнит-тесты
make tidy    # go mod tidy
make e2e     # интеграционные тесты через docker-compose
```

3. Запустить все зависимости локально (PostgreSQL + Kafka + MinIO + Prometheus + Grafana):

```powershell
docker-compose -f deploy/docker-compose.yml up -d
```

---

Переменные окружения
--------------------

### Сервер

| Переменная           | По умолчанию | Описание                     |
|----------------------|--------------|------------------------------|
| `SERVER_PORT`        | `8080`       | Порт HTTP-сервера            |
| `SERVER_HOST`        | `0.0.0.0`    | Адрес для прослушивания      |
| `SERVER_READ_TIMEOUT`| `30s`        | Таймаут чтения запроса       |
| `SERVER_WRITE_TIMEOUT`| `60s`       | Таймаут записи ответа        |
| `SERVICE_NAME`       | `bulk-service` | Имя сервиса (в логах)      |
| `SERVICE_VERSION`    | `unknown`    | Версия сервиса (в логах)     |

### Логирование

| Переменная   | По умолчанию | Описание                              |
|--------------|--------------|---------------------------------------|
| `LOG_LEVEL`  | `info`       | Уровень логирования: debug/info/warn/error |
| `LOG_FORMAT` | `json`       | Формат логов: json или console        |

### База данных (PostgreSQL)

| Переменная               | По умолчанию | Описание                        |
|--------------------------|--------------|---------------------------------|
| `DATABASE_URL`           | —            | DSN подключения к PostgreSQL    |
| `DATABASE_MAX_OPEN_CONNS`| `25`         | Макс. открытых соединений       |
| `DATABASE_MAX_IDLE_CONNS`| `5`          | Макс. простаивающих соединений  |
| `DB_STATS_INTERVAL`      | `10s`        | Интервал сбора статистики пула  |

### Redis (rate limiting / idempotency)

| Переменная        | По умолчанию              | Описание                 |
|-------------------|---------------------------|--------------------------|
| `REDIS_URL`       | `redis://redis:6379/0`    | URL подключения к Redis  |
| `REDIS_POOL_SIZE` | `10`                      | Размер пула соединений   |

### Kafka

| Переменная                 | По умолчанию    | Описание                          |
|----------------------------|-----------------|-----------------------------------|
| `KAFKA_BROKERS`            | `localhost:9092`| Адреса брокеров (через запятую)   |
| `KAFKA_TOPIC_BULK_JOB`     | `bulk.job`      | Топик публикации заданий          |
| `KAFKA_TOPIC_BULK_RESULT`  | `bulk.result`   | Топик результатов обработки       |
| `KAFKA_TOPIC_BULK_STATUS`  | `bulk.status`   | Топик статусов батчей             |
| `KAFKA_CONSUMER_GROUP`     | `bulk-service`  | Consumer group                    |

### BFF (биллинговая интеграция)

| Переменная           | По умолчанию | Описание                             |
|----------------------|--------------|--------------------------------------|
| `BFF_URL`            | —            | URL BFF-сервиса (обязательная)       |
| `BFF_TIMEOUT`        | `30s`        | Таймаут HTTP-вызовов к BFF           |
| `BFF_RETRY_ATTEMPTS` | `3`          | Количество повторных попыток         |
| `BFF_SERVICE_TOKEN`  | —            | Service token для internal-маршрутов |

### MinIO / S3 (объектное хранилище)

| Переменная           | По умолчанию | Описание                          |
|----------------------|--------------|-----------------------------------|
| `STORAGE_ENDPOINT`   | —            | Адрес MinIO/S3 (host:port)        |
| `STORAGE_ACCESS_KEY` | —            | Access Key                        |
| `STORAGE_SECRET_KEY` | —            | Secret Key                        |
| `STORAGE_BUCKET`     | —            | Имя бакета                        |
| `STORAGE_USE_SSL`    | `false`      | Использовать TLS (`true`/`false`) |
| `STORAGE_BASE_URL`   | —            | Базовый URL для скачивания (опц.) |

### Лимиты

| Переменная                | По умолчанию | Описание                             |
|---------------------------|--------------|--------------------------------------|
| `MAX_FILE_SIZE_MB`        | `10`         | Максимальный размер XLS-файла (МБ)   |
| `MAX_ROWS_PER_BATCH`      | `1000`       | Макс. строк в одном батче            |
| `MAX_CONCURRENT_BATCHES`  | `5`          | Макс. батчей в обработке одновременно|
| `MAX_BATCHES_PER_HOUR`    | `10`         | Макс. батчей в час на пользователя   |

---

API эндпойнты
-------------

### Служебные

| Метод | Путь      | Описание                                         |
|-------|-----------|--------------------------------------------------|
| GET   | /health   | Проверка работоспособности сервиса               |
| GET   | /ready    | Readiness probe (проверяет доступность DB и Kafka)|
| GET   | /metrics  | Метрики Prometheus                               |

### Пользовательский API `/api/v1`

| Метод | Путь                          | Описание                                          |
|-------|-------------------------------|---------------------------------------------------|
| POST  | /api/v1/upload                | Загрузка XLS-файла, создание батча                |
| GET   | /api/v1/batches               | Список батчей текущего пользователя               |
| GET   | /api/v1/batch/:id             | Статус и детали батча                             |
| GET   | /api/v1/batch/:id/status      | Алиас для статуса батча                           |
| POST  | /api/v1/batch/:id/confirm     | Подтверждение батча (запуск обработки)            |
| POST  | /api/v1/batch/:id/cancel      | Отмена батча                                      |
| POST  | /api/v1/batch/:id/export      | Запуск асинхронного экспорта результатов          |
| GET   | /api/v1/export/:id            | Статус / результат экспорта                       |
| GET   | /api/v1/batch/:id/errors.xlsx | Скачать XLSX с ошибками валидации                 |
| GET   | /api/v1/batch/:id/results.xlsx| Скачать XLSX с результатами (buildId, URLs)       |
| GET   | /api/v1/template/:revision    | Скачать шаблон XLS для заданной ревизии           |

### Административный API `/api/v1/admin`

| Метод | Путь                                  | Описание                                    |
|-------|---------------------------------------|---------------------------------------------|
| GET   | /api/v1/admin/stats                   | Общая статистика сервиса                    |
| GET   | /api/v1/admin/batches                 | Список всех батчей (с пагинацией)           |
| GET   | /api/v1/admin/batches/:id             | Детали батча (для администратора)           |
| PUT   | /api/v1/admin/batches/:id/config      | Обновление конфигурации батча               |
| POST  | /api/v1/admin/batches/:id/restart     | Перезапуск упавших заданий в батче          |
| GET   | /api/v1/admin/users/:userId/batches   | Батчи конкретного пользователя              |
| GET   | /api/v1/admin/config/bulk-limits      | Получить текущие runtime-лимиты             |
| PUT   | /api/v1/admin/config/bulk-limits      | Обновить runtime-лимиты без перезапуска     |

Полная OpenAPI-спецификация: [`api/openapi.yaml`](api/openapi.yaml)

---

Миграции БД
-----------

Миграции находятся в директории `migrations/`. Применить с помощью [golang-migrate](https://github.com/golang-migrate/migrate):

```powershell
migrate -path migrations -database "$Env:DATABASE_URL" up
```

---

Интеграционные тесты
---------------------

Используют `docker-compose -f deploy/docker-compose.int.yml` (PostgreSQL + Zookeeper + Kafka + MinIO).

**Windows (PowerShell):**

```powershell
# Запустить зависимости
docker-compose -f deploy/docker-compose.int.yml up -d

# Подождать инициализации
Start-Sleep -Seconds 15

# Запустить все интеграционные тесты
$Env:RUN_INT_TESTS = '1'
go test -tags=integration ./... -v

# Остановить зависимости
docker-compose -f deploy/docker-compose.int.yml down -v
```

**Linux / macOS:**

```bash
docker-compose -f deploy/docker-compose.int.yml up -d
sleep 15
RUN_INT_TESTS=1 go test -tags=integration ./... -v
docker-compose -f deploy/docker-compose.int.yml down -v
```

Запустить только конкретный тест (E2E upload → storage → DB):

```powershell
$Env:RUN_INT_TESTS = '1'
go test ./internal/transport/http -run TestE2E_Upload_SaveAndDBUpdate -v -tags=integration
```

Kafka-тест с Testcontainers:

```powershell
$Env:RUN_TESTCONTAINERS = '1'
go test -tags=integration ./internal/kafka -run TestKafka_WithTestcontainers -v
```

---

CI/CD (GitHub Actions)
----------------------

Файл `.github/workflows/ci.yml` содержит пять последовательных стадий (Go 1.25):

1. **lint** — `golangci-lint` + запрет прямых HTTP-вызовов вне `internal/billing`
2. **test** — `gofmt`, `go vet`, `staticcheck`, юнит-тесты с race-детектором, проверка покрытия ≥ 70%
3. **integration** — запуск PostgreSQL + Kafka через docker-compose, применение миграций, тесты адаптеров
4. **e2e** — полный E2E-прогон (`RUN_INT_TESTS=1 go test -tags=integration ./...`)
5. **build** — сборка Docker-образа, пуш в registry

Деплой происходит автоматически:
- ветка `develop` → окружение **staging**
- ветка `main` → окружение **production** (требует подтверждения)

---

Docker
------

```powershell
# Сборка образа (многоэтапная, финальный образ — distroless)
docker build -t bulk-service:latest .

# Запуск
docker run -p 8080:8080 `
  -e DATABASE_URL="postgres://user:pass@host:5432/bulk_service" `
  -e KAFKA_BROKERS="kafka:9092" `
  -e BFF_URL="http://bff:8080" `
  -e STORAGE_ENDPOINT="minio:9000" `
  bulk-service:latest
```

---

Kubernetes
----------

Манифесты находятся в `deploy/k8s/`:

| Файл                    | Описание                               |
|-------------------------|----------------------------------------|
| `deployment-dev.yaml`   | Deployment для dev-окружения           |
| `deployment-staging.yaml`| Deployment для staging                |
| `deployment-prod.yaml`  | Deployment для production              |
| `service.yaml`          | Service (ClusterIP)                    |
| `hpa-prod.yaml`         | HorizontalPodAutoscaler для production |
| `pdb-prod.yaml`         | PodDisruptionBudget для production     |

---

Мониторинг (Prometheus + Grafana)
----------------------------------

Сервис экспортирует метрики на `GET /metrics`. Основные метрики:

- `bulk_service_http_requests_total{method,route,status}` — счётчик HTTP-запросов
- `bulk_service_http_request_duration_seconds_bucket` — гистограмма latency
- `bulk_service_batch_jobs_total{batch_id,status}` — счётчик заданий батча
- `bulk_service_kafka_publish_total{topic,result}` — публикации в Kafka
- `bulk_service_storage_errors_total{operation,result}` — ошибки хранилища
- `bulk_service_db_open_connections` — открытые соединения к БД

Grafana-дашборды и конфигурация Prometheus: `deploy/grafana/` и `deploy/prometheus/`.

SLO и правила оповещений описаны в файле [`SLA_SLO.md`](SLA_SLO.md).

---

Трассировка (OpenTelemetry / Jaeger)
-------------------------------------

Сервис инструментирован OpenTelemetry. Для включения экспорта span-ов задайте переменную окружения:

```
JAEGER_ENDPOINT=http://jaeger:14268/api/traces
```

---

Структура проекта
-----------------

```
cmd/server/          — точка входа
internal/
  adapters/          — адаптеры к внешним системам (PostgreSQL, Redis)
  batch/             — бизнес-логика батча
  billing/           — HTTP-клиент BFF/биллинга
  config/            — загрузка конфигурации из env
  db/                — инициализация и пул соединений PostgreSQL
  di/                — dependency injection (DI-контейнер)
  domain/            — доменные типы и агрегаты
  exporter/          — экспорт результатов в XLSX
  history/           — история изменений батча
  kafka/             — Kafka producer/consumer
  limits/            — runtime-управление лимитами
  logging/           — инициализация zap-логгера
  metrics/           — регистрация метрик Prometheus
  parser/            — парсинг XLS/XLSX-файлов
  ports/             — интерфейсы (порты) для адаптеров
  repo/              — репозитории (слой данных)
  storage/           — клиент MinIO/S3
  tracing/           — инициализация OpenTelemetry
  transport/http/    — HTTP-роутер, хэндлеры, middleware, метрики
  usecase/           — use-case логика
  validation/        — валидация строк XLS
pkg/
  validation/        — общие утилиты валидации
  xls/               — общие утилиты работы с XLS
migrations/          — SQL-миграции
deploy/              — docker-compose, K8s, Prometheus, Grafana
api/openapi.yaml     — OpenAPI 3.0 спецификация
```
