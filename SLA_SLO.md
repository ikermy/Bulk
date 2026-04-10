SLA / SLO и метрики для Bulk Service
===================================

Цель этого документа — описать целевые показатели качества (SLO), соглашения об уровне обслуживания (SLA), соответствующие метрики Prometheus и правила оповещений (alerts) для сервиса Bulk.

1. Область ответственности
-------------------------
- API загрузки и обработки батчей (upload -> validate -> confirm -> enqueue)
- Хранение файлов в объектном хранилище (MinIO/S3)
- Взаимодействие с внешней биллинговой системой
- Публикация событий в Kafka

2. Основные метрики (Prometheus)
--------------------------------
- bulk_service_http_requests_total{method,route,status} — счётчик HTTP-запросов
- bulk_service_http_request_duration_seconds_bucket{route,le} — гистограмма HTTP latency (сек)
- bulk_service_batch_jobs_total{batch_id,status} — сколько джобов создано для batch
- bulk_service_batch_jobs_pending{batch_id} — текущее число pending jobs по batch (gauge)
- bulk_service_batch_processing_duration_seconds{batch_id,le} — время обработки batch (histogram)
- bulk_service_kafka_publish_total{topic,result} — попытки публикации в Kafka (success/error)
- bulk_service_kafka_publish_duration_seconds{topic,le} — duration publish
- bulk_service_storage_errors_total{operation,result} — ошибки при работе со storage
- bulk_service_billing_errors_total{method,result} — ошибки биллинга
- bulk_service_db_open_connections — gauge: открытые соединения к БД

3. Предложенные SLO (целевые уровни обслуживания)
-------------------------------------------------
SLO базируются на пользовательско-видимых уровнях: успех API (>= 99.9%), latency (P95 < 1s), надёжность публикаций в Kafka (>= 99.95%) и доступность хранилища.

- Availability (API success rate): 99.9% по запросам с кодом < 500 за 30d
  - Метрика: 1 - (sum(rate(http_requests_total{status=~"5.."}[30d])) / sum(rate(http_requests_total[30d])))
- Latency: P95 < 1s для критичных маршрутов (/api/v1/upload, /api/v1/batch/:id)
  - Метрика: histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, route))
- Kafka publish reliability: >= 99.95% успешных публикаций за 30d
  - Метрика: sum(rate(kafka_publish_total{result="success"}[30d])) / sum(rate(kafka_publish_total[30d]))
- Storage reliability: ошибок меньше 0.01% операций
  - Метрика: rate(storage_errors_total[30d]) / rate(storage_operations_total[30d]) (storage_operations_total можно добавить при необходимости)

Formal SLO definitions
-----------------------
Ниже формализованы SLO, целевые уровни и соответствующие SLIs (метрики):

- API success rate (SLO-A)
  - Цель: >= 99.9% за 30d
  - SLI: 1 - (sum(rate(http_requests_total{status=~"5.."}[30d])) / sum(rate(http_requests_total[30d])))
  - Alerting: при падении < 99.9% — создать P1 инцидент

- Latency P95 (SLO-L)
  - Цель: P95 < 1s на критичных маршрутах (/api/v1/upload, /api/v1/batch/:id)
  - SLI: histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, route))
  - Alerting: P95 > 1s в течение 5m — warning; > 2s — critical

- Kafka publish reliability (SLO-K)
  - Цель: >= 99.95% успешных публикаций за 30d
  - SLI: sum(rate(kafka_publish_total{result="success"}[30d])) / sum(rate(kafka_publish_total[30d]))
  - Alerting: success_ratio < 99.95% — critical

- Processing backlog (SLO-B)
  - Цель: backlog per batch < 100 jobs (operational threshold)
  - SLI: gauge bulk_service_batch_jobs_pending{batch_id}
  - Alerting: value > 100 for 5m — warning; consider auto-scaling

Recording rules
---------------
Для уменьшения нагрузки на Prometheus и упрощения выражений SLO добавлены recording rules в `deploy/prometheus/rules.yml`:

- `bulk_service:errors_total:30d`
- `bulk_service:requests_total:30d`
- `bulk_service:request_error_ratio:30d`
- `bulk_service:kafka_publish_success_ratio:30d`
- `bulk_service:latency_p95:5m`

Эти правила позволяют быстро вычислять SLO и строить дашборды.

4. SLA (договорные обязательства)
--------------------------------
Пример SLA для внешних клиентов:
- Доступность API: 99.9% в месяц. Компенсации обсуждаются отдельно.
- Время восстановления при критических инцидентах: 1 час (при наличии доступа к инфраструктуре).

5. Правила оповещений (Prometheus rules)
----------------------------------------
В репозитории имеется файл `deploy/prometheus/rules.yml` с базовыми правилами. Ключевые алерты:

- HighErrorRate
  - expr: суммарный rate 5xx по маршруту > 1/s (5m)
  - severity: critical

- HighP95Latency
  - expr: P95 > 1s (5m)
  - severity: warning

- KafkaPublishErrors (новое)
  - expr: sum(rate(bulk_service_kafka_publish_total{result!="success"}[5m])) by (topic) > 0
  - severity: critical
  - действие: проверить consumer/producer, посмотреть контейнерные логи и сеть до broker

- JobsBacklog (новое)
  - expr: sum(bulk_service_batch_jobs_pending) by (batch_id) > 100
  - severity: warning
  - действие: проверить очередь заданий, дефицит воркеров/скейлинг

- StorageErrors (новое)
  - expr: sum(rate(bulk_service_storage_errors_total[5m])) by (operation) > 0
  - severity: critical
  - действие: проверить MinIO/S3, креденшелы, сетевые ошибки

- DBConnectionsHigh (новое)
  - expr: bulk_service_db_open_connections > 100
  - severity: warning
  - действие: проверить пулы соединений, leaks, DB health

6. Runbook — краткая инструкция действий при срабатывании алерта
--------------------------------------------------------------
- HighErrorRate / HighP95Latency
  1. Проверить метрики CPU/Memory и GC у приложения
  2. Посмотреть логи приложения (t.Log / контейнерные логи)
  3. Проверить downstream зависимые сервисы (billing, storage, db)
  4. Если проблема в росте нагрузки — временно увеличить реплики/воркеры

- KafkaPublishErrors
  1. Проверить broker доступность (ping, docker logs)
  2. Проверить сетевые ACL / DNS
  3. Посмотреть Producer retry/backoff

- JobsBacklog
  1. Проверить очередь jobs в БД (SELECT COUNT WHERE status='pending')
  2. Проверить, работают ли воркеры/потребители
  3. Запустить дополнительные воркеры или увеличить throughput

- StorageErrors
  1. Проверить доступность MinIO/S3, креденшелы
  2. Посмотреть подробные ошибки в логах приложения

7. Дальнейшие улучшения (roadmap)
---------------------------------
- Добавить экспорт метрик по storage operations (total, success) для вычисления error rate
- Интеграция с SLO/SLI платформой (например, SLIs в Grafana) и автоматический отчёт SLO
- Автоматическое подключение и тестирование Kafka через `testcontainers-go/modules/kafka` для CI

8. Как включить и проверить локально
-----------------------------------
1) Запустить Prometheus с `deploy/prometheus/prometheus.yml` и подключить `deploy/prometheus/rules.yml`.
2) Убедиться, что сервис экспортирует /metrics (обычно через интеграцию с Prometheus client).
3) Запустить интеграционные тесты (`RUN_INT_TESTS=1`) чтобы генерировать метрики.

Файл `deploy/prometheus/rules.yml` уже обновлён и содержит дополнительные правила.

Контакты и эскалация: команда SRE / разработчики проекта.

