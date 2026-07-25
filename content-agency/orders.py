"""Minimal JSON-file order tracker. No payment processing — this just keeps
a record of what's been promised to whom, at what price, and its status, so
the "company" has a lightweight backend instead of losing track in a chat."""

import json
import os
import uuid
from datetime import datetime

from config import ORDERS_FILE

STATUSES = ["new", "in_progress", "delivered", "paid", "cancelled"]


def _load() -> list[dict]:
    if not os.path.exists(ORDERS_FILE):
        return []
    with open(ORDERS_FILE) as f:
        return json.load(f)


def _save(orders: list[dict]) -> None:
    with open(ORDERS_FILE, "w") as f:
        json.dump(orders, f, indent=2)


def add_order(client: str, description: str, price_eur: float, status: str = "new") -> dict:
    if status not in STATUSES:
        raise ValueError(f"status must be one of {STATUSES}")
    orders = _load()
    order = {
        "id": str(uuid.uuid4())[:8],
        "client": client,
        "description": description,
        "price_eur": price_eur,
        "status": status,
        "created_at": datetime.now().isoformat(),
        "updated_at": datetime.now().isoformat(),
    }
    orders.append(order)
    _save(orders)
    return order


def update_status(order_id: str, status: str) -> dict:
    if status not in STATUSES:
        raise ValueError(f"status must be one of {STATUSES}")
    orders = _load()
    for order in orders:
        if order["id"] == order_id:
            order["status"] = status
            order["updated_at"] = datetime.now().isoformat()
            _save(orders)
            return order
    raise KeyError(f"No order with id {order_id!r}")


def list_orders() -> list[dict]:
    return _load()


def summary() -> dict:
    orders = _load()
    total_paid = sum(o["price_eur"] for o in orders if o["status"] == "paid")
    total_pipeline = sum(o["price_eur"] for o in orders if o["status"] in ("new", "in_progress", "delivered"))
    return {
        "order_count": len(orders),
        "total_paid_eur": total_paid,
        "total_pipeline_eur": total_pipeline,
    }
