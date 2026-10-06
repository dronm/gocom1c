#!/usr/bin/env bash
set -euo pipefail

# Запускайте в Bash. Укажите в BASE_URL адрес, соответствующий httpAddr в конфигурации сервиса.
# Пример: BASE_URL=http://127.0.0.1:5000 HTTP_USERNAME=username bash curltests.sh
# Настройка аутентификации, примеры выполнения команд и скачивания файлов описаны в README.md.
base_url="${BASE_URL:-http://127.0.0.1:5000}"
http_username="${HTTP_USERNAME:-username}"
base_url="${base_url%/}"

# /health доступен без аутентификации, даже если auth.requireAuth имеет значение true.
curl --fail --show-error "${base_url}/health"

# /status требует базовую HTTP-аутентификацию, если auth.requireAuth имеет значение true.
# curl запрашивает пароль, если переменная HTTP_PASSWORD не задана.
if [[ -n "${HTTP_PASSWORD:-}" ]]; then
	curl --fail --show-error --basic --user "${http_username}:${HTTP_PASSWORD}" "${base_url}/status"
else
	curl --fail --show-error --basic --user "${http_username}" "${base_url}/status"
fi
