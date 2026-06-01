**[English version](README.md)** | **Русская версия**

<div align="center">
  <img src="web/public/icon-512x512.png" alt="PopuGate" width="192" height="192">

# PopuGate

**Современный менеджер MTProto-прокси для Telegram с веб-интерфейсом, Telegram-ботом и системой мониторинга.**

[![Build](https://github.com/fussraider/PopuGate/actions/workflows/build.yml/badge.svg)](https://github.com/fussraider/PopuGate/actions/workflows/build.yml)
[![Release](https://github.com/fussraider/PopuGate/actions/workflows/release.yml/badge.svg)](https://github.com/fussraider/PopuGate/actions/workflows/release.yml)
[![GitHub release](https://img.shields.io/github/v/release/fussraider/PopuGate?include_prereleases)](https://github.com/fussraider/PopuGate/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/fussraider/PopuGate?v=0.1.6)](https://goreportcard.com/report/github.com/fussraider/PopuGate?v=0.1.6)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Swagger](https://img.shields.io/badge/API-Swagger-green)](https://petstore.swagger.io/?url=https://raw.githubusercontent.com/fussraider/PopuGate/master/docs/swagger.json)
[![GHCR Backend](https://img.shields.io/badge/ghcr.io-fussraider%2Fpopugate-blue)](https://github.com/fussraider/PopuGate/pkgs/container/popugate)
[![GHCR Web](https://img.shields.io/badge/ghcr.io-fussraider%2Fpopugate--web-blue)](https://github.com/fussraider/PopuGate/pkgs/container/popugate-web)
</div>

> **Дисклеймер:** PopuGate вдохновлен проектом [MTProxyMax](https://github.com/SamNet-dev/MTProxyMax) — спасибо автору за идею. Проект разрабатывается с активным использованием нейросетей (AI-assisted development) и может содержать недоработки — он находится в стадии активной разработки. Буду признателен за bug-репорты и pull requests.

---

## 🐳 Запуск в Docker (рекомендуется)

Рекомендуемый способ запуска со встроенным веб-интерфейсом и проксированием через Nginx:

1. Создайте файл `docker-compose.yml`:
```yaml
services:
  popugate-backend:
    image: ghcr.io/fussraider/popugate:latest
    container_name: popugate-backend
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_ADMIN
    volumes:
      - ./data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - ADMIN_PASSWORD=mysecretpassword
      - POPUGATE_DATA_DIR=/data
      - TZ=Europe/Moscow

  popugate-web:
    image: ghcr.io/fussraider/popugate-web:latest
    container_name: popugate-web
    restart: unless-stopped
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "80:80"
      - "8443:8443"
    environment:
      - DOMAIN_NAME=your-domain.com
      - BACKEND_URL=http://host.docker.internal:8090/api/
    volumes:
      - ./certbot/conf:/etc/letsencrypt:ro
      - ./certbot/www:/var/www/certbot:ro
    depends_on:
      - popugate-backend

  certbot:
    image: certbot/certbot
    container_name: certbot
    volumes:
      - ./certbot/conf:/etc/letsencrypt
      - ./certbot/www:/var/www/certbot
    entrypoint: "/bin/sh -c 'trap exit TERM; while :; do certbot renew; sleep 12h & wait $${!}; done;'"
```

2. Запустите стек:
```bash
docker compose up -d
```

<details>
<summary>Сборка образов вручную</summary>

```bash
# Бэкенд
DOCKER_BUILDKIT=1 docker buildx build -t popugate . --load

# Фронтенд
cd web && DOCKER_BUILDKIT=1 docker buildx build -t popugate-web . --load
```
</details>

> **Примечание:** Контейнер бэкенда запускается в `network_mode: host` — все порты (8090, 443, 9090 и др.) привязываются напрямую к хосту. Не добавляйте секцию `ports` для `popugate-backend`. Веб-контейнер подключается к бэкенду через `host.docker.internal` (настраивается через переменную окружения `BACKEND_URL`).

---

## 🚀 Быстрый старт (Бинарный файл)

Перед запуском убедитесь, что на сервере **установлен и запущен Docker**. Запуск от имени `root` (для доступа к `iptables` и Docker).

```bash
# 1. Скачайте последнюю версию
wget -O popugate https://github.com/fussraider/PopuGate/releases/latest/download/popugate-linux-amd64
chmod +x popugate

# 2. Установите пароль администратора
sudo ./popugate setup

# 3. Запустите сервер
sudo ./popugate server
```

Бэкенд доступен на порту `8090`, прокси-движок — на порту `443`. Для запуска в фоне установите systemd-службу (см. ниже).

**Поддерживаемые платформы:** Linux (Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky, AlmaLinux, Alpine).

---

## 🌐 Сетевые настройки

| Порт | Назначение |
|------|-----------|
| `8090` | REST API и веб-интерфейс |
| `80` / `8443` | Веб-интерфейс HTTP/HTTPS (Docker Compose) |
| `443` | Входящие подключения MTProto (занят движком) |
| `9090` | Метрики Prometheus |

Порты прокси и метрик настраиваются индивидуально для каждого инстанса.

---

## ⚙️ Конфигурация

### Переменные окружения

| Переменная | Описание |
|-----------|----------|
| `ADMIN_PASSWORD` | Пароль администратора (первый запуск) |
| `POPUGATE_DATA_DIR` | Рабочая директория (БД, конфиги, кэши). Также задаётся флагом `--data` (`-d`) |
| `POPUGATE_DEPLOYMENT` | Тип развертывания (`docker` — устанавливается автоматически в образе) |
| `DEBUG` / `GIN_MODE` | Режим отладки (`true`/`debug` = отладка, по умолчанию — release) |
| `LOG_LEVEL` | Уровень логирования (`debug`, `info`, `warn`, `error`, `fatal`) |
| `BACKUP_ENCRYPTION_KEY` | Ключ шифрования бэкапов (64 hex-символа, AES-256-GCM) |
| `TELEMT_VERSION` | Переопределение версии движка telemt |
| `TELEMT_COMMIT` | Переопределение commit/ref для сборки движка |
| `TELEMT_REPO` | Переопределение URL репозитория движка |

### Флаги командной строки

- `--port <число>` (`-p`) — порт HTTP-сервера (по умолчанию: `8090`)
- `--data <путь>` (`-d`) — рабочая директория (приоритетнее `POPUGATE_DATA_DIR`)
- `--db <путь>` — путь к файлу SQLite (по умолчанию: `<data-dir>/settings.db`)

```bash
# Пример: запуск с кастомной конфигурацией
sudo -E ./popugate server --port 9090 --data /var/lib/popugate
```

---

## 🔒 Настройка HTTPS (SSL)

При использовании Docker Compose можно настроить автоматический выпуск SSL-сертификатов Let's Encrypt:

```bash
sudo ./scripts/init-ssl.sh your-domain.com your-email@example.com
```

Укажите домен в `docker-compose.yml`: `DOMAIN_NAME=your-domain.com`. Сертификаты обновляются автоматически каждые 12 часов. Для работы Let's Encrypt порт 80 должен быть доступен из интернета.

**Защита от фишинга:** При указанном `DOMAIN_NAME` (не `localhost`) nginx автоматически отклоняет запросы с неизвестными заголовками `Host` — чужие домены, направленные на ваш сервер, не получат ответа. Бэкенд также проверяет заголовок `Host`, если в настройках указан `web_url`.

---

## 🖥️ Возможности веб-интерфейса

Веб-интерфейс поддерживает **светлую/тёмную тему** и **двуязычность** (Русский / English).

### 📊 Панель управления (Dashboard)
Статус прокси, активные секреты, подключения, трафик, быстрые действия (старт/стоп/перезагрузка), здоровье системы, мониторинг ресурсов (CPU, память, диск) в реальном времени.

### 🔑 Секреты (Secrets)
Управление ключами доступа: создание, удаление, ротация, лимиты (подключения, IP, квота трафика), срок действия, QR-коды, теги, архивация, массовые операции, поиск, экспорт/импорт JSON.

### 📋 Шаблоны (Templates)
Предустановки лимитов (подключения, IP, квота, срок, теги) для быстрого применения к секретам.

### 🖥️ Инстансы (Instances)
Независимые прокси с собственным портом, доменами маскировки, FakeTLS и тегами доступа. Мультидоменность, горячая перезагрузка, логи (SSE), массовые операции.

- **Антиблокировка**: Пофрагментная настройка TCPMSS для обхода реассемблера DPI и TLS-фронтинг для защиты от активного зондирования
- **Перезагрузка без даунтайма (Swing Routing)**: Пересоздаёт контейнер на альтернативном порту, атомарно перенаправляет трафик через iptables NAT, плавно дрейнирует старые соединения, затем останавливает старый контейнер — без потери одного активного соединения
- **Горячая перезагрузка конфига**: Применение изменений секретов и апстримов к запущенному контейнеру ммгновенно, без перезапуска
- **Лив-логи**: Стриминг логов контейнера в реальном времени через SSE

### 🔀 Upstreams
Цепочки прокси (SOCKS4/SOCKS5) с балансировкой весов и привязкой к сетевым интерфейсам.

### 🌍 Гео-блокировка (Geoblock)
Ограничение доступа по странам (blacklist/whitelist) через `iptables`.

### 📈 Трафик (Traffic)
Глобальная статистика, графики истории активных подключений, визуализация долей трафика пользователей (donut-диаграмма), детальная статистика по каждому секрету.

### 🤖 Telegram Bot
Управление прокси, статистика, создание секретов, QR-коды и просмотр задач планировщика — прямо из Telegram.

### 🔄 Репликация (Replication)
Master-Slave синхронизация настроек и секретов между серверами через SSH.

### 💾 Резервные копии (Backups)
Автоматические ежедневные бэкапы (БД, конфиги движка, SSH-ключи) с ротацией по дням хранения. Опциональное AES-256-GCM шифрование. Скачивание и восстановление через веб-интерфейс.

### 🕐 Планировщик (Scheduler)
Управление фоновыми задачами: включение/отключение, изменение расписания (cron), ручной запуск, история выполнения с деталями ошибок.

**Стандартные задачи:** traffic-flush, quota-check, expiry-check, health-check, upstream-health, telegram-report, replication-sync, update-check, telemt-check, token-cleanup, daily-backup, backup-cleanup, history-cleanup, quota-reset, auto-rotate.

### 🆙 Обновления (Updates)
Автоматическая проверка и ручное применение обновлений. Бинарный режим — скачивание с GitHub + перезапуск systemd. Docker-режим — pull нового образа + пересоздание контейнера.

### 🐳 Docker
Проверка наличия Docker, установка, сборка и обновление образа движка `telemt`.

### 🖥️ Системное меню (System)
Установка/удаление systemd-службы, перезапуск, просмотр статуса и информации о системе.

- **Настройка TCP-сети**: Включение/отключение оптимизаций ядра TCP BBR и TCP FastOpen (sysctl) одним кликом, с автоматическим резервным копированием и возможностью отката к исходным значениям

### ⚙️ Настройки (Settings)
Глобальные параметры: CPU/memory лимиты Docker, кастомный IP, FakeTLS, PROXY-протокол, кастомные URL Telegram, Ad Tag, авторотация секретов, режим обслуживания, ротация бекапов, режим отладки.

### 📊 Лив-телеметрия на дашборде
Заголовок карточки «Движок и здоровье» отображает пульсирующий зелёный бейдж **Live** при подключенном WebSocket-потоке статуса — мгновенная визуальная индикация лив-телеметрии.

---

## 🛠 Системная служба (Systemd)

PopuGate поддерживает нативную установку как служба `systemd` — автозапуск при загрузке и перезапуск при сбоях. Установка доступна через веб-интерфейс (раздел System).

```bash
sudo systemctl status popugate
sudo systemctl restart popugate
sudo systemctl stop popugate
```

---

## 📖 API Справочник

REST API полностью документирован через **Swagger / OpenAPI 2.0**.

- **Интерактивный UI** (на вашем сервере): `http://<ваш-сервер>:8090/swagger/index.html`
- **Внешний просмотрщик**: [Swagger Petstore viewer](https://petstore.swagger.io/?url=https://raw.githubusercontent.com/fussraider/PopuGate/master/docs/swagger.json)

Все защищённые эндпоинты требуют токен `Bearer` JWT в заголовке `Authorization` (получается через `POST /api/v1/auth/login`).

### WebSocket-эндпоинты потоковой передачи

Четыре эндпоинта передают данные через WebSocket (не стандартный HTTP REST):

| Эндпоинт | Описание |
|----------|-------------|
| `GET /api/v1/proxy/status/ws` | Статус прокси в реальном времени |
| `GET /api/v1/traffic/live/ws` | Лив-метрики (подключения, пользователи) |
| `GET /api/v1/system/resources/ws` | Мониторинг ресурсов сервера (CPU, RAM, диск, нагрузка) |
| `GET /api/v1/engine/update/ws` | Прогресс обновления движка |

WebSocket-соединения также требуют заголовок `Authorization: Bearer <токен>` (или параметр `?token=` для браузерных клиентов, не поддерживающих задание заголовков).

---

## 🛠 Разработка

### Сборка бэкенда

```bash
make tidy        # Установить зависимости
make build       # Сборка под текущую ОС → bin/popugate
make cross-build # linux/amd64 + linux/arm64
```

Требования: **Go 1.26+**, **Make**.

### Сборка фронтенда

```bash
cd web
pnpm install
pnpm run build   # → web/dist/
```

Собранные файлы отдаются через Nginx (см. Docker Compose) или другой веб-сервер с проксированием к бэкенду.

### Тестирование и линтинг

```bash
make test   # Все тесты (in-memory SQLite, без Docker)
make lint   # golangci-lint
make fmt    # gofmt + goimports
```

Тесты изолированы и не требуют Docker или сетевого окружения.

Качество кода контролируется через `gocyclo` — цикломатическая сложность всех функций не превышает 15.
