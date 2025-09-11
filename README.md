# Auto Spec Planner MVP

Stack:

- Backend: Go (Fiber) + Postgres (migrations)
- Vector: Qdrant
- LLM + Vector service: Python (FastAPI + sentence-transformers + qdrant-client)
- Web: Vite + Vue 3

## 0) Start infra

```bash
docker compose up -d
```

## 1) Backend (Go)

```bash
cd backend
cp .env.example .env
# Install golang-migrate first (e.g., brew install golang-migrate)
make migrate-up
go run ./cmd/server
# listens on :8080
```

## 2) LLM & Vector service (Python)

```bash
cd ../llm_backend
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
uvicorn app:app --reload --port 8000
# Qdrant at http://localhost:6333
```

## 3) Web (Vite + Vue 3)

```bash
cd ../web
pnpm i
pnpm dev      # http://localhost:5173
```

## 4) Try it

- Open Web UI, enter a brief, click "Generate".
- If NEW -> spec saved to Postgres and upserted to Qdrant.
- If DUPLICATE -> you get a list of similar specs.

### Notes

- `llm_backend/app.py` uses a placeholder generator. Replace this with your actual LLM call and JSON schema validation.
- Similarity threshold can be tuned via backend env `SIM_THRESHOLD`.
- Consider adding a queue (Redis) to process jobs async when scale.

### db

1. make migrate-up
2. make migrate-down
3. delete all vector db

```bash
curl -X DELETE http://localhost:8000/vector/clear
```
