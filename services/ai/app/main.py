from fastapi import FastAPI

app = FastAPI(title="Arbion AI", version="0.1.0")


@app.get("/healthz")
def health() -> dict[str, str]:
    """Report process liveness without checking downstream dependencies."""
    return {"service": "ai", "status": "ok"}
