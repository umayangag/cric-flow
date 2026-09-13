"""The vocabulary of wicket kinds, and what each one is to the scorecard (IMPORT-06).

A wicket in a Cricsheet file is one of fourteen kinds, and they are not all the same thing:
six are the bowler's (bowled, caught, caught and bowled, hit wicket, lbw, stumped), six are
dismissals nobody bowls (run out, retired out, obstructing the field, handled the ball,
hit the ball twice, timed out), and two are not dismissals at all -- a batter who retires
hurt or retires not out may come back. Until IMPORT-06 was fixed this pass counted a
retired-hurt batter as a dismissal, and the go-app importer credited every kind to the
bowler.

``configs/wicket_kinds.json`` is the one list. The importer reads it (``internal/wicketkinds``)
for ``bowling_data.wickets`` and ``match_inning.wickets_lost``; this module reads it for the
``wickets`` and ``dismissals`` targets and the innings' wicket counts, on both rating
sources. Both sides reading one file is the point: a set carried in each language would be
two sets the day one of them was edited. A kind the file does not name is an error rather
than a guess, because the only way a new kind arrives is Cricsheet adding one, and the
right answer to that is a reviewed edit to the file.
"""

from __future__ import annotations

import json
import logging
import os
from dataclasses import dataclass
from functools import lru_cache
from typing import FrozenSet, Optional, Sequence

logger = logging.getLogger(__name__)

FILE_NAME = "wicket_kinds.json"
ENV_VAR = "GO_APP_WICKET_KINDS"

#: The one kind the keeper flag reads: a stumping is the bowler's wicket and the keeper's credit.
STUMPED = "stumped"


def normalise(kind: str) -> str:
    """A kind as the vocabulary spells it: lower case, trimmed. The archive has always
    spelled every kind exactly so; this is defensive, the same way the importer is."""
    return kind.strip().lower()


@dataclass(frozen=True)
class Vocabulary:
    """The reviewed set of wicket kinds, by what the scorecard makes of each."""

    credited_to_bowler: FrozenSet[str]
    dismissal_not_credited: FrozenSet[str]
    not_out: FrozenSet[str]

    def __post_init__(self) -> None:
        classes = {
            "credited_to_bowler": self.credited_to_bowler,
            "dismissal_not_credited": self.dismissal_not_credited,
            "not_out": self.not_out,
        }
        seen: dict = {}
        for name, kinds in classes.items():
            if not kinds:
                raise ValueError(f"the {name} list is empty")
            for kind in kinds:
                if not kind or kind != normalise(kind):
                    raise ValueError(f"{kind!r} is not in the vocabulary's own spelling (lower case, trimmed)")
                if kind in seen:
                    raise ValueError(f"{kind!r} is listed twice ({seen[kind]} and {name}); a kind is one thing")
                seen[kind] = name

    def _known(self, kind: str) -> str:
        name = normalise(kind)
        if name not in self.credited_to_bowler and name not in self.dismissal_not_credited and name not in self.not_out:
            raise ValueError(f"wicket kind {kind!r} is not in the vocabulary ({FILE_NAME})")
        return name

    def is_dismissal(self, kind: str) -> bool:
        """Whether the innings lost a wicket: every kind but a retirement not out."""
        return self._known(kind) not in self.not_out

    def is_credited_to_bowler(self, kind: str) -> bool:
        """Whether the wicket goes into the bowler's column."""
        return self._known(kind) in self.credited_to_bowler

    def is_stumping(self, kind: str) -> bool:
        return self._known(kind) == STUMPED


def _search_paths() -> Sequence[str]:
    here = os.path.dirname(os.path.abspath(__file__))
    return (
        os.path.join(here, "..", "..", "..", "configs", FILE_NAME),
        os.path.join("configs", FILE_NAME),
        os.path.join("..", "configs", FILE_NAME),
    )


def load(path: Optional[str] = None) -> Vocabulary:
    """Read the vocabulary from ``path``, else from ``GO_APP_WICKET_KINDS``, else from
    ``configs/`` beside the repository.

    Unlike the team lineage, an absent file is an error: nothing can say what a run out is
    to the bowler without it, and a pass that guessed would be the defect the file exists
    to close. A file that does not parse or does not validate is an error for the same
    reason the lineage's is: it is committed data.
    """
    candidates = [path] if path else [os.environ.get(ENV_VAR) or "", *_search_paths()]
    for candidate in candidates:
        if candidate and os.path.exists(candidate):
            with open(candidate) as fh:
                raw = json.load(fh)
            vocabulary = Vocabulary(
                credited_to_bowler=frozenset(raw.get("credited_to_bowler") or []),
                dismissal_not_credited=frozenset(raw.get("dismissal_not_credited") or []),
                not_out=frozenset(raw.get("not_out") or []),
            )
            logger.info(
                "wicket kinds: %d kinds from %s",
                len(vocabulary.credited_to_bowler) + len(vocabulary.dismissal_not_credited) + len(vocabulary.not_out),
                candidate,
            )
            return vocabulary
    logger.error("wicket kinds: no %s found under configs/ (set %s)", FILE_NAME, ENV_VAR)
    raise FileNotFoundError(f"no {FILE_NAME} found under configs/ (set {ENV_VAR})")


@lru_cache(maxsize=1)
def vocabulary() -> Vocabulary:
    """The committed vocabulary, read once per process."""
    return load()
