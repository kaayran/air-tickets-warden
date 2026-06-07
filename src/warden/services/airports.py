from __future__ import annotations

from dataclasses import dataclass
from functools import lru_cache
from typing import Any, cast

import airportsdata

# Curated shortlist shown before the user types anything (the "frequent" row).
FREQUENT_AIRPORTS: tuple[str, ...] = ("BEG", "BUD", "SOF", "TSR", "ZAG", "BCN", "VIE")

# Upper bound on candidates a single search returns (paginated client-side).
MAX_RESULTS = 40


@dataclass(frozen=True, slots=True)
class AirportHit:
    iata: str
    label: str


@lru_cache(maxsize=1)
def _dataset() -> dict[str, dict[str, Any]]:
    """Offline IATA dataset, loaded once. No network access."""
    return cast("dict[str, dict[str, Any]]", airportsdata.load("IATA"))


def _label(entry: dict[str, Any]) -> str:
    city = entry.get("city") or entry.get("name") or entry["iata"]
    country = entry.get("country") or "??"
    return f"{city}, {country} ({entry['iata']})"


def resolve(iata: str) -> AirportHit | None:
    """Look up a single airport by IATA code; None if unknown."""
    entry = _dataset().get(iata.strip().upper())
    if entry is None:
        return None
    return AirportHit(iata=entry["iata"], label=_label(entry))


def search(query: str, limit: int = MAX_RESULTS) -> list[AirportHit]:
    """Find airports by exact IATA code first, then by city/name substring.

    Case-insensitive, offline. Returns at most ``limit`` hits.
    """
    q = query.strip()
    if not q:
        return []

    data = _dataset()
    hits: list[AirportHit] = []
    seen: set[str] = set()

    upper = q.upper()
    if len(upper) == 3 and upper.isalpha() and upper in data:
        hits.append(AirportHit(iata=upper, label=_label(data[upper])))
        seen.add(upper)

    needle = q.lower()
    for code, entry in data.items():
        if code in seen:
            continue
        city = (entry.get("city") or "").lower()
        name = (entry.get("name") or "").lower()
        if needle in city or needle in name:
            hits.append(AirportHit(iata=code, label=_label(entry)))
            seen.add(code)
            if len(hits) >= limit:
                break

    return hits


def frequent() -> list[AirportHit]:
    """Resolve the curated shortlist to hits, skipping any unknown codes."""
    return [hit for code in FREQUENT_AIRPORTS if (hit := resolve(code)) is not None]
