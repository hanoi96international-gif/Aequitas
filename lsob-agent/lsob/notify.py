"""Signal and fill notifications: console always, Telegram when configured.

Uses urllib from the standard library — a notifier is not worth a dependency,
and a notifier that can crash the agent is worse than none, so every send is
best-effort and failures are logged rather than raised.
"""

from __future__ import annotations

import json
import logging
import os
import urllib.error
import urllib.parse
import urllib.request

from .config import NotifyConfig

log = logging.getLogger("lsob.notify")


class Notifier:
    def __init__(self, cfg: NotifyConfig) -> None:
        self.cfg = cfg
        self.token = os.environ.get(cfg.telegram_token_env, "").strip()
        self.chat_id = os.environ.get(cfg.telegram_chat_env, "").strip()

    @property
    def telegram_enabled(self) -> bool:
        return bool(self.token and self.chat_id)

    def send(self, message: str) -> None:
        if self.cfg.console:
            log.info(message)
        if not self.telegram_enabled:
            return
        url = f"https://api.telegram.org/bot{self.token}/sendMessage"
        payload = urllib.parse.urlencode(
            {"chat_id": self.chat_id, "text": message, "disable_web_page_preview": "true"}
        ).encode()
        request = urllib.request.Request(url, data=payload)
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                body = json.loads(response.read().decode() or "{}")
            if not body.get("ok", False):
                log.warning("telegram rejected the message: %s", body.get("description"))
        except (urllib.error.URLError, TimeoutError, ValueError) as exc:
            log.warning("telegram notification failed: %s", exc)
