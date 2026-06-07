from __future__ import annotations

from warden.services import airports


def test_resolve_known_code() -> None:
    hit = airports.resolve("beg")
    assert hit is not None
    assert hit.iata == "BEG"
    assert "BEG" in hit.label


def test_resolve_unknown_code() -> None:
    assert airports.resolve("ZZZ") is None


def test_search_exact_iata_comes_first() -> None:
    hits = airports.search("BCN")
    assert hits, "expected at least one hit for BCN"
    assert hits[0].iata == "BCN"


def test_search_by_city() -> None:
    hits = airports.search("Barcelona")
    codes = {h.iata for h in hits}
    assert "BCN" in codes


def test_search_empty_query_returns_nothing() -> None:
    assert airports.search("   ") == []


def test_search_respects_limit() -> None:
    hits = airports.search("a", limit=5)
    assert len(hits) <= 5


def test_frequent_resolves_curated_shortlist() -> None:
    hits = airports.frequent()
    codes = {h.iata for h in hits}
    # All curated codes are real IATA airports, so none should be dropped.
    assert set(airports.FREQUENT_AIRPORTS) <= codes
