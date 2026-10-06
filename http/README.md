# HTTP API: аутентификация и примеры curl

Примеры ниже предназначены для Bash (Linux, macOS, WSL или Git Bash). Примеры для Windows PowerShell приведены в последнем разделе. Замените имя пользователя, пароль, адрес сервера и ссылки на объекты 1С своими значениями.

## Настройка HTTP-аутентификации

Добавьте следующие поля в существующий файл `http/config.json`, сохранив остальные настройки, включая блок `com`:

```json
{
	"httpAddr": "127.0.0.1:5000",
	"auth": {
		"requireAuth": true,
		"username": "username",
		"password": "password"
	}
}
```

После изменения конфигурации перезапустите HTTP-службу. Эти учётные данные используются для доступа к HTTP API и задаются отдельно от `Usr` и `Pwd` в строке подключения к 1С.

В поставляемой конфигурации указан адрес `127.0.0.1:6000`, а аутентификация не включена. Чтобы использовать порт `5000` из примеров, примените настройки выше либо замените порт в командах на указанный в вашей конфигурации. Адрес `127.0.0.1` принимает только локальные подключения. Для доступа с другого компьютера укажите подходящий сетевой интерфейс сервера либо используйте `"httpAddr": ":5000"` для прослушивания всех интерфейсов и разрешите доступ в межсетевом экране. Примеры с адресом `212.113.243.234:5000` предполагают, что служба доступна по этому адресу.

| Метод | Маршрут | Аутентификация при `auth.requireAuth: true` | Назначение |
| --- | --- | --- | --- |
| GET | `/health` | Не требуется | Проверить, отвечает ли HTTP-служба. |
| GET | `/status` | HTTP Basic | Получить состояние пула COM-соединений. |
| POST | `/execute` | HTTP Basic | Выполнить команду 1С и получить ответ в формате JSON. |
| POST | `/bin-data` | HTTP Basic | Выполнить команду 1С и скачать сформированный файл. |
| POST | `/start` | HTTP Basic | Инициализировать пул COM-соединений. |
| POST | `/stop` | HTTP Basic | Закрыть пул COM-соединений. |

Если `auth.requireAuth` имеет значение `false` или не задан, защищённые маршруты также принимают запросы без аутентификации. Маршрут `/health` всегда доступен без аутентификации. Если для защищённого маршрута учётные данные не переданы или неверны, сервер возвращает HTTP `401 Unauthorized`.

## Формирование заголовка Basic-аутентификации

Соедините имя пользователя HTTP и пароль через двоеточие, затем закодируйте полученную строку в Base64 без завершающего перевода строки:

```bash
printf '%s' 'username:password' | base64
```

Результат:

```text
dXNlcm5hbWU6cGFzc3dvcmQ=
```

Для учётных данных из примера можно использовать и такую команду:

```bash
echo -n 'username:password' | base64
```

Получится следующий заголовок запроса:

```text
Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=
```

Передайте его в curl с помощью аргумента:

```bash
-H 'Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ='
```

Последний фрагмент — это аргумент curl, который нужно добавить к полной команде, как показано ниже. При изменении учётных данных сформируйте новое значение Base64: значение из примера подходит только для пользователя `username` с паролем `password`. Base64 — обратимое кодирование, а не шифрование. Для передачи реальных учётных данных по сети используйте HTTPS.

Для формирования заголовка из учётных данных, введённых интерактивно в Bash:

```bash
read -r -p 'Имя пользователя HTTP: ' HTTP_USERNAME
read -r -s -p 'Пароль HTTP: ' HTTP_PASSWORD
printf '\n'
AUTH_TOKEN=$(printf '%s:%s' "$HTTP_USERNAME" "$HTTP_PASSWORD" | base64 | tr -d '\r\n')

curl --fail --show-error http://127.0.0.1:5000/status \
	-H "Authorization: Basic ${AUTH_TOKEN}"

unset HTTP_PASSWORD AUTH_TOKEN
```

Команда `tr` удаляет переводы строк, чтобы заголовок оставался одной строкой, даже если утилита Base64 переносит длинный результат. Имя пользователя не может содержать двоеточие; в пароле двоеточия допустимы.

### Автоматическое формирование заголовка средствами curl

Параметр `--user` автоматически формирует тот же заголовок Basic-аутентификации:

```bash
curl --fail --show-error --basic --user 'username:password' \
	http://127.0.0.1:5000/status
```

Чтобы ввести пароль интерактивно, передайте только имя пользователя:

```bash
curl --fail --show-error --basic --user 'username' \
	http://127.0.0.1:5000/status
```

В запросе используйте либо параметр `--user`, либо вручную сформированный заголовок `Authorization`.

## Проверка доступности без аутентификации

```bash
curl --fail --show-error http://212.113.243.234:5000/health
```

Для локальной службы:

```bash
curl --fail --show-error http://127.0.0.1:5000/health
```

Ответ при успешном выполнении:

```json
{
	"success": true,
	"payload": "OK"
}
```

Такой ответ подтверждает, что HTTP-служба отвечает на запросы. Подключение к 1С при этом не проверяется; для получения состояния пула COM-соединений используйте `/status`.

## Получение состояния пула с аутентификацией

```bash
curl --fail --show-error http://212.113.243.234:5000/status \
	-H 'Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ='
```

Для локальной службы:

```bash
curl --fail --show-error http://127.0.0.1:5000/status \
	-H 'Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ='
```

Ответ представляет собой JSON с полями `success` и `payload`. В `payload` содержится поле `status` со значением `running` (пул запущен) или `stopped` (пул остановлен). Если пул существует, в `payload` также присутствуют поля `connCount` и `connStatuses`.

## Выполнение команды из JSON-файла

Оба маршрута выполнения команд принимают JSON-объект с полями `command` и `params`. Доступные команды и значения их параметров определяются используемой реализацией интеграции с 1С. Следующие примеры предполагают, что в ней реализованы команды `new_order` и `print_order`.

Создайте файл `new_order_data.json` в текущем каталоге либо используйте готовый файл [examples/new_order_data.json](examples/new_order_data.json):

```json
{
	"command": "new_order",
	"params": {
		"warehouse_ref": "dc878a44-0576-11f0-a2c1-00155d327e58",
		"firm_ref": "86d45fa6-42c2-11ec-bbb4-00155d327a0c",
		"client_ref": "f2e229a7-c37d-11ef-a2b1-00155d327e58",
		"contract_ref": "f2e229ab-c37d-11ef-a2b1-00155d327e58",
		"agreement_ref": "46ab0e50-22d2-11ed-bbb7-00155d327a0b",
		"ecommerce_id": "777",
		"payed": false,
		"products": [
			{
				"product_ref": "d29b7144-63d6-11ed-a27e-00155d32870a",
				"quant": 80,
				"price": 120,
				"total": 7680,
				"measure_unit_k": 1,
				"char_ref": "e70b1d7d-63d6-11ed-a27e-00155d32870a"
			}
		]
	}
}
```

Выполните команду из каталога, содержащего файл `new_order_data.json`:

```bash
curl --fail --show-error -X POST http://127.0.0.1:5000/execute \
	-H 'Content-Type: application/json' \
	-H 'Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=' \
	-d @new_order_data.json
```

Та же команда с использованием параметра аутентификации curl:

```bash
curl --fail --show-error -X POST http://127.0.0.1:5000/execute \
	--basic --user 'username:password' \
	-H 'Content-Type: application/json' \
	-d @new_order_data.json
```

Аргумент `-d @new_order_data.json` загружает тело запроса из локального файла. Маршрут `/execute` возвращает JSON с полями `success` и `payload`, содержащим результат команды, либо с `success: false` и полем `error`, если команда завершилась с ошибкой.

## Скачивание печатной формы заказа в формате PDF

Создайте файл `print_order_data.json` в текущем каталоге либо используйте готовый файл [examples/print_order_data.json](examples/print_order_data.json):

```json
{
	"command": "print_order",
	"params": {
		"order_ref": "5cde9a27-db00-11f0-a2ee-00155d07cb0b"
	}
}
```

Выполните команду из каталога, содержащего файл `print_order_data.json`:

```bash
curl --fail --show-error -X POST http://127.0.0.1:5000/bin-data \
	-H 'Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=' \
	-H 'Content-Type: application/json' \
	-d @print_order_data.json \
	-o print.pdf
```

Та же команда с использованием параметра аутентификации curl:

```bash
curl --fail --show-error -X POST http://127.0.0.1:5000/bin-data \
	--basic --user 'username:password' \
	-H 'Content-Type: application/json' \
	-d @print_order_data.json \
	-o print.pdf
```

Аргумент `-o print.pdf` сохраняет содержимое ответа в локальный файл в текущем каталоге. Команда 1С должна сформировать PDF и вернуть путь к этому файлу на сервере в поле `payload` успешного ответа; маршрут `/bin-data` передаст содержимое файла в curl. Имя `print.pdf` не преобразует файл другого формата в PDF. Параметр `--fail` завершает curl с ошибкой при ошибочном HTTP-ответе, чтобы содержимое сообщения об ошибке не было сохранено как PDF.

## Примеры для Windows PowerShell

Явно вызывайте `curl.exe`. В Windows PowerShell имя `curl` может быть псевдонимом команды `Invoke-WebRequest`. Символ продолжения строки Bash (`\`) не подходит для PowerShell, поэтому каждая команда ниже записана в одну строку.

Сформируйте то же значение Base64 из учётных данных примера:

```powershell
$credentials = 'username:password'
$authToken = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($credentials))
$authToken
```

Результат:

```text
dXNlcm5hbWU6cGFzc3dvcmQ=
```

Выполните команды из каталога, содержащего оба JSON-файла:

```powershell
curl.exe --fail --show-error http://127.0.0.1:5000/health
curl.exe --fail --show-error http://127.0.0.1:5000/status -H "Authorization: Basic $authToken"
curl.exe --fail --show-error -X POST http://127.0.0.1:5000/execute -H 'Content-Type: application/json' -H "Authorization: Basic $authToken" -d '@new_order_data.json'
curl.exe --fail --show-error -X POST http://127.0.0.1:5000/bin-data -H 'Content-Type: application/json' -H "Authorization: Basic $authToken" -d '@print_order_data.json' -o print.pdf
```

Аргументы файлов `@...` заключены в кавычки для PowerShell. Сохраняйте файлы запросов в кодировке UTF-8 без маркера порядка байтов (BOM). Вместо явного заголовка можно использовать `--basic --user 'username:password'`.
