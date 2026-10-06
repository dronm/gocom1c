redis-cli LPUSH spetsov:com1c:commands '{"command": "status", "request_id": "test1"}'

# В другом терминале:
redis-cli MONITOR
