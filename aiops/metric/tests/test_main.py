from metric_analysis import main
from fastapi.testclient import TestClient
import pytest

client = TestClient(main.app)

def test_root():
    response = client.get("/")
    assert response.status_code == 200
    assert response.json() == {"message": "Opni Metric Analysis Backend API"}

def test_healthcheck():
    response = client.get("/healthcheck")
    assert response.status_code == 200
    assert response.json() == {'healthcheck': 'Everything OK!'}


