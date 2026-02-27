# Billing UI / CDR тарификация (Go)

Небольшой учебный сервис на Go, который:

1) принимает **тарифы** (CSV),
2) принимает **список абонентов** (CSV),
3) принимает **CDR** (текстовый файл, построчно) и считает стоимость звонков,

а в ответ отдаёт **итоги по абонентам** и (опционально) **список всех тарифицированных звонков**.

Сервис поднимает HTTP API и отдаёт простую встроенную HTML-страницу (UI) для загрузки файлов через браузер.

---

## Быстрый старт

Требования:
- **Go 1.22+** (в коде используются паттерны `net/http` вида `"POST /path"`).

Запуск:

```bash
make run
# или
make build && ./.bin/ukrainian_call_center_scam_goev
```

По умолчанию сервер слушает `:8080`.
Открой в браузере:

- `http://localhost:8080/`

Демо-файлы лежат в `./example/`.

---

## Конфигурация

Через переменные окружения:

- `ADDR` — адрес сервера (по умолчанию `:8080`).

Пример:

```bash
ADDR=127.0.0.1:9000 make run
```

---

## Использование через UI

В UI порядок действий такой:

1. **Tariffs** — загрузи `example/tariffs.csv`
2. **Subscribers** — загрузи `example/subscribers.csv`
3. **CDR** — загрузи `example/cdr.txt`

В блоке CDR можно включить чекбокс `collect_calls` — тогда сервер вернёт не только итоговые суммы, но и список всех звонков.

---

## Использование через curl

### Загрузка тарифов

```bash
curl -s -F 'file=@example/tariffs.csv' http://localhost:8080/api/v1/tariffs
```

### Загрузка абонентов

```bash
curl -s -F 'file=@example/subscribers.csv' http://localhost:8080/api/v1/subscribers
```

### Тарификация CDR

Только итоги:

```bash
curl -s -F 'file=@example/cdr.txt' 'http://localhost:8080/api/v1/cdr/tariff'
```

Итоги + список звонков:

```bash
curl -s -F 'file=@example/cdr.txt' 'http://localhost:8080/api/v1/cdr/tariff?collect_calls=true'
```

> Хендлеры также поддерживают **raw body** (без multipart). Если `Content-Type` не `multipart/form-data`, то будет читаться `r.Body`.

---

## HTTP API

### `POST /api/v1/tariffs`

Загрузка тарифов (CSV).

- вход: `multipart/form-data` с полем `file` **или** raw body
- ответ: `{ "status": "ok" }` или ошибка

### `POST /api/v1/subscribers`

Загрузка абонентов (CSV).

### `POST /api/v1/cdr/tariff?collect_calls={true|false}`

Тарификация CDR (стримом, построчно).

Ответ (примерная структура):

```json
{
  "status": "ok",
  "totals": [
    {
      "phone_number": "78123260000",
      "client_name": "Office Billing",
      "total_cost_kop": 12345,
      "calls_count": 10
    }
  ],
  "calls": [
    {
      "start_time": "2026-02-03T14:22:10+03:00",
      "end_time": "2026-02-03T14:24:22+03:00",
      "calling_party": "78123260000",
      "called_party": "+79161234567",
      "call_direction": "outgoing",
      "disposition": "answered",
      "duration": 132,
      "billable_sec": 127,
      "cost_kop": 381,
      "tariff": {
        "prefix": "7916",
        "destination": "Москва МТС (мобильный)",
        "priority": 100
      }
    }
  ]
}
```

### `GET /health`

Возвращает `{ "status": "ok" }`.

---

## Форматы входных файлов

### 1) Tariffs CSV (`;`-разделитель)

**Важно:** хедер должен совпасть **строго** (как строка целиком):

```
prefix;destination;rate_per_min;connection_fee;timeband;weekday;priority;effective_date;expiry_date
```

Поля:

- `prefix` — префикс номера (без `+`), например `7916`
- `destination` — описание направления
- `rate_per_min` — цена за минуту, поддерживаются `.` и `,` как десятичный разделитель (например `1.80`)
- `connection_fee` — плата за соединение (добавляется **только** если `disposition=answered`)
- `timeband` — диапазон времени `HH:MM-HH:MM`
    - если начало < конец: обычный интервал в пределах суток
    - если начало > конец: интервал **через полночь**
    - если начало == конец (например `00:00-00:00`): считается **24/7**
- `weekday` — дни недели:
    - `1`..`7`, где `1=Пн`, …, `7=Вс`
    - поддерживаются диапазоны `1-5` и списки `1,3,5`
- `priority` — приоритет тарифа (чем больше, тем важнее)
- `effective_date` — дата начала действия `YYYY-MM-DD`
- `expiry_date` — дата окончания действия `YYYY-MM-DD` (в коде превращается в *exclusive* границу `expiry_date + 24h`, то есть дата окончания по сути **включительная**)

Пример (см. `example/tariffs.csv`).

### 2) Subscribers CSV (`;`-разделитель)

Хедер должен совпасть строго:

```
phone_number;client_name
```

Поля:
- `phone_number` — номер абонента
- `client_name` — имя/название (может быть пустым)

### 3) CDR (`|`-разделитель)

Файл читается построчно. На каждой строке ожидаются поля:

1. `StartTime` — `YYYY-MM-DD HH:MM:SS`
2. `EndTime` — `YYYY-MM-DD HH:MM:SS`
3. `CallingParty`
4. `CalledParty` (может начинаться с `+` — при матчинге тарифа `+` игнорируется)
5. `direction` — `incoming` | `outgoing` | `internal`
6. `disposition` — `answered` | `busy` | `no_answer` | `failed`
7. `duration` — int
8. `billable_sec` — int
9. **зарезервировано/игнорируется** (в примере там `0.45`)
10. `account_code` — строка (может быть пустой)
11. `call_id`
12. `trunk_name`

Пример (см. `example/cdr.txt`).

---

## Логика тарификации

- Стоимость считается **только для исходящих** (`direction=outgoing`) звонков.
    - Для `incoming` и `internal` стоимость будет `0`.
- Выбор тарифа:
    1) из всех тарифов, чей `prefix` совпадает с началом номера `CalledParty`,
    2) оставляем применимые по дате/времени/дню недели,
    3) выбираем тариф с **максимальным `priority`**,
    4) при равном `priority` выбираем тариф с **самым длинным префиксом**.
- Формула стоимости:
    - если `disposition=answered`, добавляем `connection_fee`
    - затем добавляем `rate_per_min * billable_sec / 60`

> Деление целочисленное (округление вниз до копейки).

---

## Архитектура проекта

- `cmd/` — точка входа (HTTP сервер, роуты, embedded UI)
- `internal/billing/model` — доменные модели (CDR, тарифы, деньги, timeband, enum’ы)
- `internal/billing/repo` — интерфейсы репозиториев
- `internal/billing/repo/memory` — in-memory реализации (с атомарными снапшотами для быстрых чтений)
- `internal/billing/service` — бизнес-логика (загрузка CSV, матчинги тарифов, воркер-пул тарификации)
- `internal/billing/handlers/http` — HTTP API + DTO
- `web/` — статический UI, который встраивается в бинарник через `go:embed`
- `example/` — примеры входных файлов

---

## Разработка

### Makefile targets

- `make build` — сборка бинарника в `./.bin/`
- `make run` — собрать и запустить
- `make build-linux` — статический Linux amd64 бинарник
- `make build-linux-arm64` — статический Linux arm64 бинарник
- `make test` — `go test ./...`
- `make clean` — удалить `./.bin/`

---

## Примечание про Go toolchain

В `go.mod` указан `go 1.25.4`. Если у тебя стоит другая версия Go, то:

- либо выставь `GOTOOLCHAIN=local` и подправь версию в `go.mod` на твою (`1.22+`),
- либо установи нужный toolchain.

---

## Идеи для улучшений (если захочешь)

- прогресс обработки CDR (SSE/WebSocket)
- более строгая валидация входных строк (проверка количества полей)
- хранение в PostgreSQL (репозитории уже выделены интерфейсами)
- округление стоимости (например, до секунд/минут по правилам тарификации)
