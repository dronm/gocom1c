# Simple health check
curl http://127.0.0.1:60000/health

curl http://127.0.0.1:60000/status

# Test command with string parameter
curl -X POST http://127.0.0.1:60000/execute ^
  -H "Content-Type: application/json" ^
  -d "{\"command\": \"TestMethod\",\"params\": {\"param1\":\"Hello from curl\"}}"
