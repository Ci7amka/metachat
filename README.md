# MetaChat

**MetaChat** — платформа для знакомств и социального общения с поддержкой AI. MVP1 реализует базовую функциональность: регистрацию, аутентификацию, обмен сообщениями в реальном времени и управление профилями.

## Архитектура

Микросервисная архитектура на Go с HTTP/JSON для межсервисного взаимодействия:

```
┌──────────────┐
│   Клиент     │
│  (HTTP/WS)   │
└──────┬───────┘
       │
┌──────▼───────┐
│   Gateway    │  :8080  — HTTP API + WebSocket прокси
│              │         — JWT валидация, rate limiting, CORS
└──┬───────┬───┘
   │       │
┌──▼──┐ ┌──▼────────┐
│Auth │ │ Messaging  │
│:50051│ │  :50052    │
└──┬──┘ └──┬────────┘
   │       │
┌──▼───────▼──┐    ┌────────┐
│ PostgreSQL  │    │ Redis  │
│   :5432     │    │ :6379  │
└─────────────┘    └────────┘
```

### Сервисы

| Сервис | Порт | Описание |
|--------|------|----------|
| **Gateway** | 8080 | API Gateway — единственный публичный сервис. Проксирует HTTP-запросы и WebSocket-соединения |
| **Auth** | 50051 | Аутентификация, JWT токены, управление профилями |
| **Messaging** | 50052 | Обмен сообщениями, WebSocket хаб, управление диалогами |

### Технологии

- **Go 1.22+** — язык разработки
- **PostgreSQL 16** — основная база данных
- **Redis 7** — кэширование и сессии
- **JWT (HS256)** — аутентификация (access + refresh токены)
- **WebSocket** — обмен сообщениями в реальном времени
- **Docker + docker-compose** — контейнеризация

## Быстрый старт

### Требования

- Docker и docker-compose
- Go 1.22+ (для локальной разработки)

### Запуск через Docker

```bash
# Клонировать репозиторий
git clone https://github.com/Ci7amka/metachat.git
cd metachat

# Скопировать конфигурацию
cp .env.example .env

# Запустить все сервисы
docker-compose up --build

# Или через Makefile
make run
```

После запуска API доступен по адресу: `http://localhost:8080`

### Локальная сборка

```bash
# Установить зависимости
go mod tidy

# Собрать все сервисы
make build

# Запустить тесты
make test
```

## API Документация

### Аутентификация

#### Регистрация
```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword123",
  "name": "Иван Петров"
}
```

**Ответ (201):**
```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "eyJhbGciOi...",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "name": "Иван Петров",
    "bio": "",
    "interests": [],
    "avatar_url": "",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

#### Вход
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Ответ (200):** аналогичен регистрации

#### Обновление токена
```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOi..."
}
```

**Ответ (200):**
```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "eyJhbGciOi..."
}
```

#### Выход
```http
POST /api/v1/auth/logout
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOi..."
}
```

### Профиль

Все запросы к профилю требуют заголовок `Authorization: Bearer <access_token>`.

#### Получить свой профиль
```http
GET /api/v1/profile
Authorization: Bearer <access_token>
```

#### Обновить профиль
```http
PUT /api/v1/profile
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "name": "Иван Иванов",
  "bio": "Разработчик из Москвы",
  "age": 28,
  "interests": ["программирование", "музыка", "путешествия"],
  "avatar_url": "https://example.com/avatar.jpg"
}
```

#### Получить профиль пользователя
```http
GET /api/v1/users/profile?id=<user_uuid>
Authorization: Bearer <access_token>
```

### Сообщения

#### Создать диалог
```http
POST /api/v1/conversations
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "participant_ids": ["<другой_user_uuid>"]
}
```

**Ответ (201):**
```json
{
  "id": "conv-uuid",
  "participants": ["user1-uuid", "user2-uuid"],
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

#### Получить список диалогов
```http
GET /api/v1/conversations
Authorization: Bearer <access_token>
```

#### Получить сообщения в диалоге
```http
GET /api/v1/conversations/messages?conversation_id=<conv_uuid>&limit=50&before=<message_uuid>
Authorization: Bearer <access_token>
```

#### Отправить сообщение
```http
POST /api/v1/messages
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "conversation_id": "<conv_uuid>",
  "content": "Привет! Как дела?",
  "content_type": "text"
}
```

#### Отметить сообщение прочитанным
```http
POST /api/v1/messages/read
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "message_id": "<message_uuid>",
  "conversation_id": "<conv_uuid>"
}
```

### Проверка здоровья
```http
GET /health
```

## WebSocket протокол

Подключение через WebSocket:
```
ws://localhost:8080/api/v1/ws
```

Требуется JWT токен — передается через заголовок `Authorization: Bearer <access_token>`.

### Формат событий

Все сообщения передаются в формате JSON:

```json
{
  "type": "<тип_события>",
  "payload": { ... }
}
```

### Типы событий

#### Отправка сообщения (клиент → сервер)
```json
{
  "type": "message",
  "payload": {
    "conversation_id": "<conv_uuid>",
    "content": "Текст сообщения",
    "content_type": "text"
  }
}
```

#### Получение сообщения (сервер → клиент)
```json
{
  "type": "message",
  "payload": {
    "id": "<msg_uuid>",
    "conversation_id": "<conv_uuid>",
    "sender_id": "<user_uuid>",
    "content": "Текст сообщения",
    "content_type": "text",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

#### Индикатор набора текста
```json
{
  "type": "typing",
  "payload": {
    "conversation_id": "<conv_uuid>",
    "user_id": "<user_uuid>",
    "is_typing": true
  }
}
```

#### Уведомление о прочтении
```json
{
  "type": "read_receipt",
  "payload": {
    "conversation_id": "<conv_uuid>",
    "message_id": "<msg_uuid>",
    "user_id": "<user_uuid>"
  }
}
```

## Переменные окружения

| Переменная | Описание | По умолчанию |
|-----------|----------|-------------|
| `POSTGRES_HOST` | Хост PostgreSQL | `postgres` |
| `POSTGRES_PORT` | Порт PostgreSQL | `5432` |
| `POSTGRES_USER` | Пользователь PostgreSQL | `metachat` |
| `POSTGRES_PASSWORD` | Пароль PostgreSQL | `metachat_secret` |
| `POSTGRES_DB` | Имя базы данных | `metachat` |
| `REDIS_HOST` | Хост Redis | `redis` |
| `REDIS_PORT` | Порт Redis | `6379` |
| `REDIS_PASSWORD` | Пароль Redis | (пусто) |
| `JWT_SECRET` | Секрет для подписи JWT | `change-me-in-production` |
| `GATEWAY_PORT` | Порт Gateway сервиса | `8080` |
| `AUTH_PORT` | Порт Auth сервиса | `50051` |
| `MESSAGING_PORT` | Порт Messaging сервиса | `50052` |
| `AUTH_SERVICE_URL` | URL Auth сервиса | `http://auth:50051` |
| `MESSAGING_SERVICE_URL` | URL Messaging сервиса | `http://messaging:50052` |
| `RATE_LIMIT_RPS` | Лимит запросов в секунду | `10` |
| `RATE_LIMIT_BURST` | Максимальный burst | `20` |

## Структура проекта

```
metachat/
├── README.md                    # Документация (этот файл)
├── go.mod                       # Go модуль
├── Makefile                     # Команды сборки
├── docker-compose.yml           # Docker оркестрация
├── Dockerfile.gateway           # Docker образ Gateway
├── Dockerfile.auth              # Docker образ Auth
├── Dockerfile.messaging         # Docker образ Messaging
├── .env.example                 # Пример переменных окружения
├── .gitignore
├── proto/                       # Protobuf определения (справочно)
│   ├── auth.proto
│   └── messaging.proto
├── config/                      # YAML конфигурации
│   ├── gateway.yaml
│   ├── auth.yaml
│   └── messaging.yaml
├── migrations/                  # Миграции БД
│   ├── 000001_init.up.sql
│   └── 000001_init.down.sql
├── services/
│   ├── gateway/                 # API Gateway
│   │   └── main.go
│   ├── auth/                    # Сервис аутентификации
│   │   ├── main.go
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── repository.go
│   └── messaging/               # Сервис сообщений
│       ├── main.go
│       ├── handler.go
│       ├── service.go
│       ├── repository.go
│       └── websocket.go
└── internal/
    ├── middleware/               # HTTP middleware
    │   ├── auth.go              # JWT авторизация
    │   ├── ratelimit.go         # Rate limiter
    │   └── logging.go           # Логирование + CORS
    ├── models/
    │   └── models.go            # Общие модели данных
    ├── database/
    │   └── postgres.go          # PostgreSQL подключение
    └── redis/
        └── redis.go             # Redis подключение
```

## Лицензия

MIT
