 # E2E / Интеграционные тесты в CI

 Этот документ описывает E2E/integration job, добавленный в GitHub Actions CI для Bulk Service и содержит инструкцию по локальному воспроизведению.

 Что добавлено
 - Новый job `e2e` в `.github/workflows/ci-e2e.yml`:
   - Проверяет/подтягивает образ Kafka (параметризуемый через переменную `KAFKA_IMAGE`).
   - Запускает сервисы из `deploy/docker-compose.int.yml` (Postgres, Zookeeper, Kafka, MinIO при необходимости).
   - Запускает тесты `go test ./...` с переменной окружения `RUN_INT_TESTS=1` и ограничением на выбор e2e-тестов.
   - При провале собирает логи `docker-compose logs` и `docker ps` и загружает их как артефакты.

 Обязательные переменные окружения для CI и локального запуска
 - `KAFKA_IMAGE` — образ Kafka для CI (по умолчанию `bitnami/kafka:3`).
 - `ZOOKEEPER_IMAGE` — образ Zookeeper (по умолчанию `bitnami/zookeeper:3`).
 - `POSTGRES_IMAGE` — образ Postgres (по умолчанию `postgres:15`).
 - `MINIO_IMAGE` — образ MinIO (по умолчанию `minio/minio:latest`).
 - `RUN_INT_TESTS` — установить `1` для запуска интеграционных тестов.

 Локальное воспроизведение

 1) Запуск сервисов через docker-compose:

 ```powershell
 docker-compose -f deploy/docker-compose.int.yml up -d postgres zookeeper kafka
 # дождаться готовности Postgres (пример)
 for ($i=0; $i -lt 30; $i++) { docker exec (docker ps -q -f ancestor=postgres:15) pg_isready -U test -q && break; Start-Sleep -Seconds 1 }
 ```

 2) Запуск тестов локально (в корне репозитория):

 ```powershell
 $env:RUN_INT_TESTS='1'; go test ./... -v
 # или ограниченно: go test ./internal/transport/http -v
 ```

 3) Остановка и очистка:

 ```powershell
 docker-compose -f deploy/docker-compose.int.yml down -v
 ```

 Пояснения и рекомендации
 - CI job использует небольшой retry-логики и таймауты для уменьшения флейковости.
 - При возникновении проблем с manifest для Kafka рекомендовано сменить `KAFKA_IMAGE` на альтернативный образ или кэшировать образ в runner cache.
 - CI загружает логи контейнеров как артефакты при неудаче, это помогает быстро диагностировать проблемы.

 Если нужно — перенесу эти инструкции в `README.md` или добавлю пример скрипта `make e2e`.

