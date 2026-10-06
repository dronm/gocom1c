redis-cli LPUSH spetsov:com1c:commands '{"command": "status", "request_id": "test1"}'

#on other monitor: 
redis-cli MONITOR
