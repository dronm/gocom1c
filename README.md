# gocom1c

**gocom1c** — библиотека на Go (Golang), предназначенная для организации и управления COM-соединениями с конфигурациями **1С:Предприятие**.

Библиотека решает типичную задачу интеграции Go-приложений с 1С через COM-интерфейс, предоставляя:

- пул COM-соединений;
- потокобезопасное выполнение вызовов;
- контроль времени простоя и размера пула;
- готовые примеры сервисов для синхронной и асинхронной обработки запросов.

Проект ориентирован на использование в высоконагруженных интеграционных сервисах, работающих под Windows.

---

## Возможности

- Управление пулом COM-объектов `V83.COMConnector`
- Ограничение минимального и максимального размера пула
- Таймауты неактивных соединений
- Параллельное выполнение запросов к 1С
- Логирование через пользовательский интерфейс логгера
- Готовые примеры приложений (CLI, HTTP, Redis)

---

## Требования

- Windows
- Установленная платформа **1С:Предприятие 8.x**
- Go **1.20+**
- Доступ к COM-интерфейсу `V83.COMConnector` - зарегистрированная библиотека comcntr32.dll

---

## Установка

```bash
go install github.com/dronm/gocom1c@latest
```

Или добавьте зависимость в проект:
```bash
go get github.com/dronm/gocom1c
```

---
 
## Состав дистрибутива
В репозитории представлены три примера приложений:

- **Консольное приложение**
Простая демонстрация подключения к 1С:Предприятию и выполнения COM-вызова.

- **Windows-служба (HTTP сервер)**
Сервис принимает HTTP-запросы, передаёт их в 1С через COM и возвращает результат синхронно.

- **Windows-служба (Redis клиент)**
Сервис подписывается на канал Redis, обрабатывает входящие сообщения, отправляет их в 1С и публикует результат асинхронно.

---

## Использование
Ниже приведён упрощённый пример использования библиотеки: создание пула COM-соединений и параллельное выполнение команд в 1С.
```golang
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	com_pool "github.com/dronm/gocom1c"
)

type SimpleLogger struct{}

func (l *SimpleLogger) Infof(format string, args ...any) {
	log.Printf("INFO: "+format, args...)
}

func (l *SimpleLogger) Errorf(format string, args ...any) {
	log.Printf("ERROR: "+format, args...)
}

func (l *SimpleLogger) Debugf(format string, args ...any) {
	log.Printf("DEBUG: "+format, args...)
}

func (l *SimpleLogger) Warnf(format string, args ...any) {
	log.Printf("WARN: "+format, args...)
}

func main() {
	cfg := com_pool.Config{
		ConnectionString: `Srvr="srv_name";Ref="db_name";Usr="user_name";Pwd="pwd"`,
		CommandExec:      "WebAPI",
		MaxPoolSize:      1,
		MinPoolSize:      1,
		IdleTimeout:      10 * time.Minute,
		COMObjectID:      "V83.COMConnector",
	}

	logger := &SimpleLogger{}

	// Create pool
	pool, err := com_pool.NewCOMPool(&cfg, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Execute multiple commands concurrently
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			params := map[string]any{
				"client_ref": fmt.Sprintf("client_%d", id),
				"products": []map[string]any{
					{"ref": "22222", "name": "ProductA"},
					{"ref": "33333", "name": "ProductB"},
				},
			}
			paramsB , err := json.Marshal(params)
			if err != nil {
				log.Printf("json.Marshal():%v", err)
				return
			}
			result, err := pool.ExecuteCommand("TestMethod", string(paramsB))
			if err != nil {
				log.Printf("Request %d failed: %v", id, err)
			} else {
				log.Printf("Request %d succeeded: %s", id, result)
			}
		}(i)
	}

	wg.Wait()
}
```

---

## Реализация логгера
Библиотека не навязывает конкретную реализацию логирования — достаточно реализовать соответствующий интерфейс:
```golang
type SimpleLogger struct{}

func (l *SimpleLogger) Infof(format string, args ...any) {
	log.Printf("INFO: "+format, args...)
}

func (l *SimpleLogger) Errorf(format string, args ...any) {
	log.Printf("ERROR: "+format, args...)
}

func (l *SimpleLogger) Debugf(format string, args ...any) {
	log.Printf("DEBUG: "+format, args...)
}

func (l *SimpleLogger) Warnf(format string, args ...any) {
	log.Printf("WARN: "+format, args...)
}
```

---

## Создание пула COM-соединений
```golang
func main() {
	cfg := com_pool.Config{
		ConnectionString: `Srvr="srv_name";Ref="db_name";Usr="user_name";Pwd="pwd"`,
		CommandExec:      "WebAPI",
		MaxPoolSize:      1,
		MinPoolSize:      1,
		IdleTimeout:      10 * time.Minute,
		COMObjectID:      "V83.COMConnector",
	}

	logger := &SimpleLogger{}

	pool, err := com_pool.NewCOMPool(&cfg, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
```

---


## Параллельное выполнение команд
```golang
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			params := map[string]any{
				"client_ref": fmt.Sprintf("client_%d", id),
				"products": []map[string]any{
					{"ref": "22222", "name": "ProductA"},
					{"ref": "33333", "name": "ProductB"},
				},
			}

			payload, err := json.Marshal(params)
			if err != nil {
				log.Printf("json.Marshal(): %v", err)
				return
			}

			result, err := pool.ExecuteCommand("TestMethod", string(payload))
			if err != nil {
				log.Printf("Request %d failed: %v", id, err)
				return
			}

			log.Printf("Request %d succeeded: %s", id, result)
		}(i)
	}

	wg.Wait()
}
```

---

## Конфигурация

- Общие параметры
| Имя параметра    | Описание                                                                                                    | Значение по умолчанию |
| -----------------| ----------------------------------------------------------------------------------------------------------- | --------------------- |
| connectionString | Строка подключения к информационной базе 1С:Предприятие.                                                    | —                     |
| commandExec      | Имя экспортируемого метода в конфигурации 1С, вызываемого для обработки HTTP-запросов (например, `WebAPI`). | —                     |
| minPoolSize      | Минимальное количество COM-соединений, создаваемых при запуске сервиса.                                     | `1`                   |
| maxPoolSize      | Максимальное количество COM-соединений в пуле. Ограничивает количество параллельных HTTP-запросов.          | `1`                   |
| comObjectID      | ProgID COM-объекта для подключения к 1С (обычно `V83.COMConnector`).                                        | `V83.COMConnector`    |
| idleTimeout      | Максимальное время простоя COM-соединения перед его закрытием.                                              | `5m`                  |
| waitConnTimeout  | Максимальное время ожидания свободного COM-соединения при высокой нагрузке.                                 | `10s`                 |
| cleanupIdleConn  | Интервал фоновой очистки простаивающих COM-соединений.                                                      | `60s`                 |
| connCloseTimeout | Таймаут корректного закрытия COM-соединения при остановке HTTP-сервиса.                                     | `30s`                 |


## Конфигурация HTTP-сервиса

- Общие параметры
| Имя параметра   | Описание                                                                                                 | Значение по умолчанию |
| ----------------| -------------------------------------------------------------------------------------------------------- | --------------------- |
| logLevel        | Уровень логирования сервиса. Определяет детализацию логов (`debug`, `info`, `warn`, `error`).            | `debug`               |
| logToFile       | Включает запись логов в файл. Имя файла задаётся отдельно. При `false` логирование идёт только в stdout. | `false`               |
| shutdownTimeout | Максимальное время, отводимое на корректное завершение работы HTTP-сервиса (graceful shutdown).          | `10s`                 |

- Аутентификация
| Имя параметра | Описание                                                 | Значение по умолчанию |
| --------------| -------------------------------------------------------- | --------------------- |
| requireAuth   | Включает HTTP-аутентификацию для всех входящих запросов. | `false`               |
| username      | Имя пользователя для HTTP-аутентификации.                | —                     |
| password      | Пароль пользователя для HTTP-аутентификации.             | —                     |

- HTTP-сервер
| Имя параметра| Описание                                                            | Значение по умолчанию |
| -------------| ------------------------------------------------------------------- | --------------------- |
| httpAddr     | Адрес и порт, на котором HTTP-сервер принимает входящие соединения. | `:8080`               |
| readTimeout  | Максимальное время чтения HTTP-запроса от клиента.                  | `120s`                |
| writeTimeout | Максимальное время записи HTTP-ответа клиенту.                      | `30s`                 |
| idleTimeout  | Максимальное время простоя keep-alive соединения.                   | `60s`                 |


## Конфигурация Redis-сервиса
- Общие параметры
| Имя параметра   | Описание                                                                                                                        | Значение по умолчанию |
| ----------------| ------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| logLevel        | Уровень логирования сервиса. Определяет, какие сообщения будут записываться в лог (например: `debug`, `info`, `warn`, `error`). | `info`                |
| logToFile       | Включает запись логов в файл. При `false` логирование выполняется только в stdout.                                              | `false`               |
| shutdownTimeout | Максимальное время, отводимое на корректное завершение работы сервиса при остановке (graceful shutdown).                        | `30s`                 |

- Параметры подключения
| Имя параметра | Описание                                                                 | Значение по умолчанию |
| ------------- | ------------------------------------------------------------------------ | --------------------- |
| host          | Адрес Redis-сервера.                                                     | `localhost`           |
| port          | Порт Redis-сервера.                                                      | `6379`                |
| username      | Имя пользователя Redis (используется при включённой ACL-аутентификации). | —                     |
| password      | Пароль для подключения к Redis.                                          | —                     |
| db            | Номер базы данных Redis.                                                 | `0`                   |

- Параметры очередей
| Имя параметра   | Описание                                                                         | Значение по умолчанию |
| --------------- | -------------------------------------------------------------------------------- | --------------------- |
| commandQueue    | Имя очереди (list), из которой сервис читает входящие команды для отправки в 1С. | `com:commands`        |
| responseQueue   | Имя очереди (list), в которую публикуются результаты выполнения команд в 1С.     | `com:responses`       |

- Таймауты Redis
| Имя параметра  | Описание                                                                                                               | Значение по умолчанию |
| -------------- | ---------------------------------------------------------------------------------------------------------------------- | --------------------- |
| readTimeout  | Таймаут операций чтения из Redis.                                                                                      | `5s`                  |
| writeTimeout | Таймаут операций записи в Redis.                                                                                       | `5s`                  |
| blPopTimeout | Таймаут блокирующего ожидания команды в очереди (BLPOP). Позволяет сервису корректно реагировать на завершение работы. | `10s`                 |

---


## Формат задания временных интервалов

Все параметры конфигурации, имеющие тип **Duration**, задаются в соответствии со стандартным синтаксисом time.Duration языка Go.
Поддерживаются следующие суффиксы времени:

- ns — наносекунды

- µs / us — микросекунды

- ms — миллисекунды

- s — секунды

- m — минуты

- h — часы

## Примеры
```text
500ms   // 500 миллисекунд
10s     // 10 секунд
1m      // 1 минута
5m      // 5 минут
1h      // 1 час
```

Значения могут комбинироваться:
```text
1h30m
2m10s
```


---

## Ограничения и замечания
- Библиотека предназначена только для Windows

- Потокобезопасность обеспечивается на уровне пула, а не самой 1С

- Рекомендуется ограничивать размер пула в соответствии с лицензией 1С

---

## Лицензия
Проект распространяется под лицензией **MIT**.
Подробности см. в файле LICENSE.
