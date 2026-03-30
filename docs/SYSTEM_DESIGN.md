# 🏗️ SYSTEM DESIGN: MetaChat — Платформа для знакомств с AI

> Dead_Do_ЕстЪ | 🏗️ System Design #MetaChat

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 ОГЛАВЛЕНИЕ

```
├─ 1. Что мы вообще строим
├─ 2. Функциональные требования
├─ 3. Нефункциональные требования
├─ 4. Расчёт нагрузки (Back-of-the-Envelope)
├─ 5. Архитектура — Service Diagram
├─ 6. Схема базы данных
├─ 7. Выбор БД и хранилищ
├─ 8. WebSocket — как работает real-time
├─ 9. AI/ML Pipeline — matching и рекомендации
├─ 10. Шардирование и масштабирование
├─ 11. Безопасность
├─ 12. Фазы запуска (MVP1 → MVP2 → MVP3)
├─ 13. Расчёт стоимости инфраструктуры
└─ 14. Итог — золотое правило Деда
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 1. ЧТО МЫ ВООБЩЕ СТРОИМ

Слушай... Если коротко — **MetaChat** это платформа знакомств.
Но не очередной Tinder, где все свайпают как обезьяны с бананами.

MetaChat — это система, где:
- 🤖 AI анализирует совместимость людей
- 💬 Люди общаются в real-time через WebSocket
- 🧠 ML-модели на PyTorch рекомендуют, кто тебе подходит
- 📊 ClickHouse хранит аналитику, а не просто "лайки налево-направо"

По сути — **мессенджер + рекомендательная система + аналитика**, обёрнутые в микросервисную архитектуру на Go.

**Стек:**

| Компонент | Технология | Зачем |
|-----------|-----------|-------|
| Бэкенд | Go | Быстро, параллельно, не жрёт RAM |
| ML/AI | Python, PyTorch, HuggingFace | Модели совместимости |
| Основная БД | PostgreSQL 16 + pgvector | Профили, сообщения, эмбеддинги |
| Сообщения (скейл) | Cassandra | Когда сообщений станет миллионы |
| Аналитика | ClickHouse | Агрегации, метрики, дашборды |
| Очереди | Apache Kafka | Асинхронность, event-driven |
| Кэш/Сессии | Redis 7 | JWT, online-статус, rate limit |
| Оркестрация | Kubernetes | Когда Docker Compose уже не тянет |

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 2. ФУНКЦИОНАЛЬНЫЕ ТРЕБОВАНИЯ

Что система **должна уметь делать**:

### 👤 Аутентификация и профили
```
├─ Регистрация (email + пароль)
├─ Логин → JWT (access + refresh)
├─ Обновление токена
├─ Редактирование профиля
│  ├─ Имя, био, возраст
│  ├─ Интересы (массив строк)
│  └─ Аватар
└─ Просмотр чужого профиля
```

### 💬 Обмен сообщениями
```
├─ Создание диалога (1-на-1)
├─ Отправка сообщения (текст, в будущем — медиа)
├─ Получение истории сообщений (пагинация курсором)
├─ Прочтение сообщений (read receipts)
├─ Индикатор набора текста (typing)
└─ WebSocket — push в real-time
```

### 🤖 AI-Matching (MVP2+)
```
├─ Генерация эмбеддингов профиля (pgvector)
├─ Рекомендация совместимых пользователей
├─ Анализ диалогов для улучшения matching
└─ ML-pipeline: HuggingFace → PyTorch → pgvector
```

### 📊 Аналитика (MVP3+)
```
├─ DAU/MAU, retention, session length
├─ Метрики конверсии (match → диалог → встреча)
├─ A/B тестирование алгоритмов matching
└─ Дашборды в ClickHouse
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 3. НЕФУНКЦИОНАЛЬНЫЕ ТРЕБОВАНИЯ

Что система **должна выдерживать**:

| Требование | MVP1 (5 юзеров) | MVP2 (100 юзеров) | MVP3 (1000 юзеров) |
|-----------|-----------------|-------------------|-------------------|
| RPS (запросов/сек) | ~1 | ~50 | ~500 |
| WS соединений | 5 | 100 | 1000 |
| Латентность API | < 200ms | < 100ms | < 50ms |
| Доступность | 95% | 99% | 99.9% |
| Хранение сообщений | PostgreSQL | PostgreSQL | Cassandra |
| Потеря данных | Допустимо | Минимально | Недопустимо |

### Ключевые принципы:
- ✅ **Горизонтальное масштабирование** — каждый сервис можно размножить
- ✅ **Eventual consistency** — для сообщений и статусов это OK
- ✅ **Graceful degradation** — если ML упал, чат работает
- ❌ **НЕ** real-time consistency для аналитики (ClickHouse ≠ OLTP)
- ❌ **НЕ** single point of failure (кроме MVP1, где это допустимо)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 4. РАСЧЁТ НАГРУЗКИ (Back-of-the-Envelope)

Считаем по-взрослому, даже если сейчас у нас 5 пользователей.

### MVP3 — 1000 пользователей

```
Допущения:
├─ DAU = 300 (30% от 1000)
├─ Средняя сессия = 15 минут
├─ Сообщений за сессию = 20
├─ Средний размер сообщения = 200 байт
└─ Пиковый коэффициент = 3x
```

**Сообщения в день:**
```
300 DAU × 20 сообщений = 6 000 сообщений/день
```

**Пиковый RPS (сообщения):**
```
6 000 / 86 400 сек × 3 (пик) ≈ 0.2 RPS
```

0.2 RPS на сообщения. Один PostgreSQL это выдержит с закрытыми глазами.

**Хранение сообщений (год):**
```
6 000 × 365 × 200 байт ≈ 438 МБ/год
```

Даже терабайтный диск будет скучать.

**WebSocket соединения (пик):**
```
300 DAU × 0.5 (одновременно онлайн) = 150 соединений
```

Один инстанс Go-сервиса на 2 ядра/4 ГБ RAM легко тянет 10 000+ WS-соединений.

### Вывод:
> На 1000 пользователей тебе хватит **одного VPS за $20/мес**.
> Kafka, Cassandra, Kubernetes — это всё для масштаба 100K+.
> Не городи Kubernetes ради 5 пользователей. Docker Compose — твой друг.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 5. АРХИТЕКТУРА — SERVICE DIAGRAM

### MVP1 — Docker Compose, всё на одной машине

```
                    ┌──────────────────┐
                    │   Клиент         │
                    │  (HTTP / WS)     │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │   Gateway :8080  │
                    │ ─ JWT валидация  │
                    │ ─ Rate Limiting  │
                    │ ─ CORS           │
                    │ ─ WS прокси      │
                    └───┬──────────┬───┘
                        │          │
              ┌─────────▼──┐  ┌───▼───────────┐
              │ Auth :50051│  │ Messaging      │
              │            │  │   :50052       │
              │ ─ Register │  │ ─ Диалоги      │
              │ ─ Login    │  │ ─ Сообщения    │
              │ ─ JWT      │  │ ─ WebSocket Hub│
              │ ─ Профили  │  │ ─ Read receipts│
              └──────┬─────┘  └──────┬─────────┘
                     │               │
              ┌──────▼───────────────▼──┐   ┌─────────┐
              │    PostgreSQL :5432      │   │ Redis   │
              │ ─ users                 │   │  :6379  │
              │ ─ conversations         │   │ (кэш,   │
              │ ─ messages              │   │ сессии) │
              │ ─ refresh_tokens        │   └─────────┘
              └─────────────────────────┘
```

**Поток запроса (регистрация):**
```
Клиент
  │ POST /api/v1/auth/register
  ▼
Gateway (:8080)
  │ Rate limit check → CORS headers
  │ Proxy → http://auth:50051/register
  ▼
Auth Service (:50051)
  │ Валидация email/password
  │ bcrypt.GenerateFromPassword()
  │ INSERT INTO users → PostgreSQL
  │ Генерация JWT (access + refresh)
  │ INSERT INTO refresh_tokens
  ▼
Gateway
  │ Проксирует ответ
  ▼
Клиент ← { access_token, refresh_token, user }
```

**Поток WebSocket (отправка сообщения):**
```
Клиент
  │ WS: connect ws://gateway:8080/api/v1/ws
  │     (Authorization: Bearer <token>)
  ▼
Gateway
  │ JWT валидация → извлекает userID
  │ WS-прокси → ws://messaging:50052/ws
  │ Двунаправленный прокси (bidirectional)
  ▼
Messaging Hub
  │ Client.readPump() ← читает WS-сообщения
  │ type: "message" → Service.SendMessage()
  │   ├─ INSERT INTO messages → PostgreSQL
  │   ├─ UPDATE conversations.updated_at
  │   └─ Hub.SendToUser(participantIDs) → fan-out
  │       └─ Client.writePump() → WS → Gateway → Клиент
  ▼
Все участники диалога получают сообщение в real-time
```

### MVP2+ — Добавляется Kafka, ML-сервис

```
┌────────────┐     ┌───────────┐     ┌──────────────┐
│  Gateway   │────▶│  Auth     │     │  ML Service  │
│  :8080     │     │  :50051   │     │  (Python)    │
│            │────▶│           │     │  PyTorch     │
└────────────┘     └───────────┘     │  HuggingFace │
      │                              └──────┬───────┘
      │            ┌───────────┐            │
      └───────────▶│ Messaging │            │
                   │  :50052   │            │
                   └─────┬─────┘            │
                         │                  │
                   ┌─────▼──────────────────▼───┐
                   │        Apache Kafka         │
                   │  ─ messages.sent            │
                   │  ─ user.activity            │
                   │  ─ matching.requests        │
                   └─────┬──────────────────┬───┘
                         │                  │
              ┌──────────▼──┐    ┌──────────▼──┐
              │ PostgreSQL  │    │ ClickHouse   │
              │ + pgvector  │    │ (аналитика)  │
              └─────────────┘    └──────────────┘
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 6. СХЕМА БАЗЫ ДАННЫХ

### Entity-Relationship Diagram

```
┌─────────────────────┐       ┌──────────────────────────┐
│       users          │       │     conversations         │
│─────────────────────│       │──────────────────────────│
│ id          UUID PK │◄──┐   │ id          UUID PK      │
│ email  VARCHAR(255) │   │   │ created_at  TIMESTAMPTZ  │
│ password_hash       │   │   │ updated_at  TIMESTAMPTZ  │
│ name   VARCHAR(100) │   │   └────────────┬─────────────┘
│ bio          TEXT    │   │                │
│ age       INTEGER    │   │   ┌────────────▼─────────────┐
│ interests   TEXT[]   │   │   │ conversation_participants │
│ avatar_url  VARCHAR  │   │   │─────────────────────────│
│ created_at TIMESTAMP │   ├──▶│ conversation_id UUID FK  │
│ updated_at TIMESTAMP │   │   │ user_id         UUID FK  │
└─────────────────────┘   │   │ joined_at    TIMESTAMPTZ │
                          │   │ PK(conversation_id,      │
                          │   │    user_id)               │
                          │   └───────────────────────────┘
                          │
                          │   ┌───────────────────────────┐
                          │   │        messages            │
                          │   │───────────────────────────│
                          ├──▶│ id              UUID PK   │
                          │   │ conversation_id UUID FK   │
                          │   │ sender_id       UUID FK   │
                          │   │ content         TEXT      │
                          │   │ content_type    VARCHAR   │
                          │   │ read_at      TIMESTAMPTZ  │
                          │   │ created_at   TIMESTAMPTZ  │
                          │   └───────────────────────────┘
                          │
                          │   ┌───────────────────────────┐
                          │   │     refresh_tokens         │
                          │   │───────────────────────────│
                          └──▶│ id          UUID PK       │
                              │ user_id     UUID FK       │
                              │ token_hash  VARCHAR(255)  │
                              │ expires_at  TIMESTAMPTZ   │
                              │ created_at  TIMESTAMPTZ   │
                              └───────────────────────────┘
```

### Индексы

```
idx_messages_conversation    → messages(conversation_id, created_at DESC)
idx_refresh_tokens_hash      → refresh_tokens(token_hash)
idx_refresh_tokens_user      → refresh_tokens(user_id)
users.email                  → UNIQUE INDEX (неявный)
```

**Почему такая схема:**
- ✅ `conversation_participants` — связь many-to-many, поддерживает групповые чаты
- ✅ `messages` отдельно от `conversations` — можно пагинировать курсором
- ✅ `refresh_tokens` хранит хэш, а не сам токен — безопаснее
- ✅ `TEXT[]` для интересов — PostgreSQL нативно поддерживает массивы
- ❌ НЕ используем JSONB для профиля — строгая схема лучше для MVP

### SQL миграция (000001_init.up.sql)

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- для gen_random_uuid()

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(100) NOT NULL,
    bio TEXT DEFAULT '',
    age INTEGER,
    interests TEXT[] DEFAULT '{}',
    avatar_url VARCHAR(500) DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE conversation_participants (
    conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    content_type VARCHAR(20) DEFAULT 'text',
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_messages_conversation
    ON messages(conversation_id, created_at DESC);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 7. ВЫБОР БД И ХРАНИЛИЩ

### Тест одним вопросом Деда

Перед выбором любой технологии задай себе вопрос:
> «А PostgreSQL это не потянет?»

Если внятного ответа нет — бери PostgreSQL.

### Что и зачем

| Хранилище | Когда подключаем | Зачем | А без него? |
|-----------|-----------------|-------|-------------|
| **PostgreSQL** | MVP1 | Всё: юзеры, диалоги, сообщения, токены | Никак, это основа |
| **Redis** | MVP1 | Кэш сессий, rate limiting, online-статус | Можно жить без, но лучше с ним |
| **pgvector** | MVP2 | Эмбеддинги профилей для matching | Без AI нет matching |
| **Cassandra** | MVP3 | Сообщения при 100K+ пользователей | PostgreSQL упрётся в запись |
| **ClickHouse** | MVP3 | Аналитика, метрики, A/B тесты | Можно SQL, но медленно |
| **Kafka** | MVP2 | Асинхронная обработка событий | Синхронные вызовы (работает, но не скейлится) |

### PostgreSQL: главная лошадка

```
Сообщения в день (1000 юзеров):   6 000
Сообщения в год:                  2 190 000
Размер данных (год):              ~500 МБ
Один SELECT с индексом:           < 1 мс
Максимальный размер таблицы:      32 ТБ
```

PostgreSQL при 1000 пользователях — это как нанимать грузовик, чтобы перевезти один чемодан. Но грузовик надёжный.

### Когда PostgreSQL перестанет хватать

```
Сигнал #1: Запись сообщений > 10 000 RPS
           → Подключаем Cassandra (write-optimized)

Сигнал #2: Аналитические запросы тормозят OLTP
           → Подключаем ClickHouse (отдельный read-path)

Сигнал #3: Сервисы начинают ждать друг друга
           → Подключаем Kafka (async event bus)
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 8. WEBSOCKET — КАК РАБОТАЕТ REAL-TIME

### Архитектура WebSocket Hub

```
                    Gateway (WS Proxy)
                         │
            ┌────────────▼────────────┐
            │    Messaging Service     │
            │                          │
            │   ┌──────────────────┐   │
            │   │      Hub         │   │
            │   │  (goroutine)     │   │
            │   │                  │   │
            │   │  clients map:    │   │
            │   │  userID → []*Cli │   │
            │   │                  │   │
            │   │  Register()      │   │
            │   │  Unregister()    │   │
            │   │  SendToUser()    │   │
            │   └──────────────────┘   │
            │                          │
            │   ┌──────┐ ┌──────┐     │
            │   │Client│ │Client│ ... │
            │   │userA │ │userB │     │
            │   │      │ │      │     │
            │   │read  │ │read  │     │
            │   │Pump()│ │Pump()│     │
            │   │write │ │write │     │
            │   │Pump()│ │Pump()│     │
            │   └──────┘ └──────┘     │
            └──────────────────────────┘
```

### Формат WS-событий

Все сообщения — JSON:

```json
{
  "type": "<тип_события>",
  "payload": { ... }
}
```

### Типы событий

| Type | Направление | Payload | Описание |
|------|------------|---------|----------|
| `message` | клиент → сервер | `{conversation_id, content, content_type}` | Отправка сообщения |
| `message` | сервер → клиент | `{id, conversation_id, sender_id, content, ...}` | Получение сообщения |
| `typing` | оба | `{conversation_id, user_id, is_typing}` | Набор текста |
| `read_receipt` | сервер → клиент | `{conversation_id, message_id, user_id}` | Прочтение |

### Fan-out: как сообщение доходит до всех

```
User A отправляет сообщение
        │
        ▼
  readPump() парсит JSON
        │
        ▼
  Service.SendMessage()
  ├─ INSERT INTO messages (PostgreSQL)
  ├─ UPDATE conversations.updated_at
  └─ Получаем список участников
        │
        ▼
  Hub.SendToUser(participantID)
  ├─ userA clients → send channel → writePump() → WS → userA
  └─ userB clients → send channel → writePump() → WS → userB
```

**Ограничение MVP1:** Hub хранит соединения в памяти одного процесса.
При масштабировании на несколько инстансов — нужен Redis Pub/Sub для кросс-инстансного fan-out.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 9. AI/ML PIPELINE — MATCHING И РЕКОМЕНДАЦИИ

### MVP2: Базовый matching

```
┌─────────────┐    ┌──────────────────┐    ┌─────────────┐
│ User Profile │───▶│  HuggingFace     │───▶│  pgvector    │
│ (bio, age,   │    │  Transformers    │    │  (embeddings)│
│  interests)  │    │  sentence-bert   │    │              │
└─────────────┘    └──────────────────┘    └──────┬──────┘
                                                   │
                          ┌────────────────────────▼──┐
                          │  Cosine Similarity Search  │
                          │  SELECT * FROM users       │
                          │  ORDER BY embedding <=>    │
                          │    target_embedding        │
                          │  LIMIT 10;                 │
                          └────────────────────────────┘
```

**Алгоритм:**

1. Пользователь заполняет профиль (имя, био, интересы, возраст)
2. ML-сервис конвертирует текстовые данные в вектор (embedding, 768 измерений)
3. Вектор сохраняется в PostgreSQL через расширение pgvector
4. При запросе рекомендаций — cosine similarity поиск ближайших векторов
5. Результат: топ-10 наиболее совместимых пользователей

```sql
-- Пример запроса (pgvector)
SELECT id, name, bio,
       1 - (embedding <=> $1) AS compatibility_score
FROM users
WHERE id != $2
  AND age BETWEEN $3 AND $4
ORDER BY embedding <=> $1
LIMIT 10;
```

### MVP3: Продвинутый matching

```
Дополнительные сигналы:
├─ История переписок (NLP-анализ тональности)
├─ Время ответа (заинтересованность)
├─ Длина сообщений (вовлечённость)
├─ Общие темы (topic modeling)
└─ Обратная связь (понравился/не понравился)
```

Всё это — фичи для модели, обучаемой на PyTorch.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 10. ШАРДИРОВАНИЕ И МАСШТАБИРОВАНИЕ

### Стратегия шардирования

На 1000 пользователей шардирование **не нужно**. Точка.

Но когда понадобится (100K+):

```
Таблица messages:
├─ Шард ключ: conversation_id
├─ Почему: все сообщения диалога на одном шарде
├─ Плюс: нет кросс-шардовых JOIN при чтении
└─ Минус: горячие диалоги (100K сообщений) → hotspot

Таблица users:
├─ Шард ключ: user_id (UUID, равномерное распределение)
├─ Почему: UUID уже рандомный, балансировка "из коробки"
└─ Поиск по email: нужен глобальный индекс или lookup таблица
```

### Горизонтальное масштабирование сервисов

```
MVP1 (5 юзеров):
  Gateway ×1 → Auth ×1 → Messaging ×1
  PostgreSQL ×1, Redis ×1
  Всё на одном VPS

MVP2 (100 юзеров):
  Gateway ×2 (за Load Balancer)
  Auth ×2, Messaging ×2
  PostgreSQL ×1 (primary + read replica)
  Redis ×1, Kafka ×1
  ML Service ×1 (GPU)

MVP3 (1000 юзеров):
  Gateway ×3 (Kubernetes Ingress)
  Auth ×3, Messaging ×3
  PostgreSQL ×1 primary + 2 replicas
  Redis Cluster ×3
  Kafka ×3 brokers
  Cassandra ×3 nodes
  ClickHouse ×2
  ML Service ×2 (GPU)
```

### WebSocket и масштабирование — главная проблема

```
Проблема:
  User A подключён к Messaging-1
  User B подключён к Messaging-2
  A пишет B → сообщение не дойдёт (Hub в памяти!)

Решение (MVP2+):
  Redis Pub/Sub как транспорт между инстансами

  Messaging-1: Hub.SendToUser("B") → не нашёл → PUBLISH redis "user:B" msg
  Messaging-2: SUBSCRIBE redis "user:B" → получил → Hub.SendToUser("B") → ✅
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 11. БЕЗОПАСНОСТЬ

### Аутентификация

```
Алгоритм: JWT HS256
├─ Access Token:  15 минут TTL
├─ Refresh Token: 30 дней TTL
├─ Хранение refresh: hash в PostgreSQL (HMAC-SHA256)
└─ Ротация: при refresh — старый токен удаляется
```

**Почему HS256 а не RS256:**
- HS256 проще (один секрет)
- Для микросервисов за одним Gateway — достаточно
- RS256 нужен когда токен валидируют третьи стороны

### Пароли

```
Хэширование: bcrypt (cost = 10, по умолчанию)
├─ Радужные таблицы → бесполезны (salt встроен)
├─ Brute force → ~100 мс на хэш (медленно = хорошо)
└─ Хранение: только хэш, пароль нигде не сохраняется
```

### Rate Limiting

```
Алгоритм: Token Bucket
├─ RPS: 10 запросов/сек на клиента
├─ Burst: 20 (всплеск)
├─ Ключ: user_id (авторизован) или IP (анонимный)
└─ Очистка: каждые 5 минут удаляются неактивные бакеты
```

### Что нужно добавить для продакшена

```
❌ Сейчас:
├─ CORS: Allow-Origin = * (открыто для всех)
├─ HTTPS: нет (нужен reverse proxy)
├─ Input validation: базовая
├─ SQL injection: защищён (pgx параметризация)
└─ XSS: нет фильтрации контента сообщений

✅ Нужно (MVP2):
├─ CORS: ограничить домены
├─ HTTPS: Nginx + Let's Encrypt
├─ Input: строгая валидация + rate limit по эндпоинтам
├─ Content: sanitize HTML в сообщениях
├─ Audit log: кто что делал
└─ GDPR: право на удаление данных
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 12. ФАЗЫ ЗАПУСКА

### MVP1 → MVP2 → MVP3

```
┌──────────────────────────────────────────────────────────────┐
│                        MVP1 (5 юзеров)                       │
│                                                              │
│  ✅ Регистрация / Логин / JWT                                │
│  ✅ Профили (CRUD)                                           │
│  ✅ Диалоги и сообщения                                      │
│  ✅ WebSocket (real-time)                                     │
│  ✅ Read receipts + Typing                                    │
│  ✅ Rate limiting                                             │
│  ✅ Docker Compose                                            │
│                                                              │
│  Инфра: 1 VPS, PostgreSQL, Redis                             │
│  Стоимость: ~$20/мес                                         │
│  Срок: 2-3 недели                                            │
├──────────────────────────────────────────────────────────────┤
│                       MVP2 (100 юзеров)                      │
│                                                              │
│  ⬜ AI-matching (pgvector + HuggingFace)                      │
│  ⬜ Kafka для event-driven                                    │
│  ⬜ Push-уведомления                                          │
│  ⬜ Медиа-сообщения (фото, голосовые)                         │
│  ⬜ Redis Pub/Sub для WS-масштабирования                      │
│  ⬜ Мониторинг (Prometheus + Grafana)                         │
│  ⬜ CI/CD pipeline                                            │
│                                                              │
│  Инфра: 2-3 VPS, GPU для ML                                  │
│  Стоимость: ~$200-500/мес                                    │
│  Срок: 4-6 недель                                            │
├──────────────────────────────────────────────────────────────┤
│                       MVP3 (1000 юзеров)                     │
│                                                              │
│  ⬜ Cassandra для сообщений                                   │
│  ⬜ ClickHouse для аналитики                                  │
│  ⬜ Kubernetes оркестрация                                    │
│  ⬜ A/B тестирование matching                                 │
│  ⬜ Продвинутый ML (NLP, sentiment)                           │
│  ⬜ Модерация контента                                        │
│  ⬜ Гео-фильтрация                                           │
│  ⬜ Admin panel                                               │
│                                                              │
│  Инфра: Kubernetes кластер, managed DB                       │
│  Стоимость: ~$1000-2000/мес                                  │
│  Срок: 8-12 недель                                           │
└──────────────────────────────────────────────────────────────┘
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 13. РАСЧЁТ СТОИМОСТИ ИНФРАСТРУКТУРЫ

### MVP1 — Минимальный запуск

| Статья | Стоимость/мес | Комментарий |
|--------|:------------:|-------------|
| VPS (2 vCPU, 4 ГБ RAM) | $10-20 | Hetzner/DigitalOcean |
| Домен | $1 | .com или .ru |
| SSL сертификат | $0 | Let's Encrypt |
| **Итого** | **~$20/мес** | |

### MVP2 — С AI и масштабированием

| Статья | Стоимость/мес | Комментарий |
|--------|:------------:|-------------|
| VPS ×2 (App) | $40 | Gateway + сервисы |
| VPS ×1 (GPU) | $50-150 | ML inference |
| Managed PostgreSQL | $25 | С бэкапами |
| Managed Redis | $15 | |
| S3 (медиа) | $5 | Фото, аватары |
| Мониторинг | $0 | Self-hosted Grafana |
| **Итого** | **~$200-400/мес** | |

### MVP3 — Полноценная платформа

| Статья | Стоимость/мес | Комментарий |
|--------|:------------:|-------------|
| Kubernetes кластер | $200-400 | 3-5 нод |
| Managed PostgreSQL | $50 | Primary + replicas |
| Managed Redis Cluster | $30 | |
| Cassandra (3 ноды) | $150 | Или managed |
| ClickHouse | $100 | Или Altinity Cloud |
| GPU (ML) | $150-300 | 2 инстанса |
| Kafka | $50-100 | Или managed (Confluent) |
| S3 + CDN | $20 | |
| Мониторинг | $30 | Datadog/Grafana Cloud |
| **Итого** | **~$800-1500/мес** | |

### Разработка (единоразовые затраты)

| Фаза | Человеко-часы | Стоимость* |
|------|:------------:|:---------:|
| MVP1 | ~80-120 ч | $4 000-6 000 |
| MVP2 | ~200-300 ч | $10 000-15 000 |
| MVP3 | ~300-500 ч | $15 000-25 000 |
| **Итого** | **~600-900 ч** | **$29 000-46 000** |

*При ставке $50/ч (средний Go/Python разработчик)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

## 📍 14. ИТОГ

### Золотое правило Деда:

> Технология нейтральна.
> Kubernetes не спасёт плохой продукт.
> PostgreSQL не сломается от 5 пользователей.
> AI-matching бесполезен, если людям не о чём говорить.

MetaChat MVP1 — это:
- 3 микросервиса на Go (~3 200 строк кода)
- PostgreSQL + Redis
- Docker Compose для запуска одной командой
- WebSocket для real-time
- JWT для безопасности

Это работает. Это запускается. Это можно показать инвестору.

Всё остальное — Kafka, Cassandra, ClickHouse, PyTorch — подключается, когда в этом появляется реальная необходимость, а не когда хочется нарисовать красивую схему на доске.

Не городите Kubernetes ради пяти пользователей.
Не запускайте Kafka, когда у вас 6 000 сообщений в день.
Не тренируйте ML-модель, пока нет данных.

Начните с простого. Усложняйте, когда простое перестанет работать.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Dead_Do_ЕстЪ | 🏗️ System Design #MetaChat

#MetaChat #SystemDesign #Go #PostgreSQL #WebSocket #DeadDoЕстЪ
