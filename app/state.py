from __future__ import annotations
import time
from typing import Dict, List, Optional


class GameState:
    def __init__(
        self,
        quests_state: Dict[str, Dict],
        player_stats: Dict[str, Dict],
        inflight_by_player: Dict[str, set],
        scoreboard: Dict[str, int],
        fails: Dict[str, int],
        roster: Dict[str, Dict],
        dlq_messages: List[dict],
    ) -> None:
        self.quests_state = quests_state
        self.player_stats = player_stats
        self.inflight_by_player = inflight_by_player
        self.scoreboard = scoreboard
        self.fails = fails
        self.roster = roster
        self.dlq_messages = dlq_messages

    # Recorders
    def record_issued(self, quest_id: str, quest_type: str, issued_at: Optional[float] = None) -> None:
        self.quests_state[quest_id] = {
            "quest_type": quest_type,
            "status": "pending",
            "issued_at": issued_at or time.time(),
        }

    def record_accepted(self, quest_id: str, quest_type: str, player: str, work_sec: float, ts: Optional[float] = None) -> None:
        q = self.quests_state.setdefault(quest_id, {"quest_type": quest_type})
        q.update({
            "status": "accepted",
            "assigned_to": player,
            "accepted_at": ts or time.time(),
            "work_sec": work_sec,
        })
        st = self.player_stats.setdefault(player, {"accepted": 0, "completed": 0, "failed": 0})
        st["accepted"] += 1
        self.inflight_by_player.setdefault(player, set()).add(quest_id)

    def record_drop(self, player: str, quest_id: str, quest_type: str) -> None:
        self.inflight_by_player.get(player, set()).discard(quest_id)
        q = self.quests_state.setdefault(quest_id, {"quest_type": quest_type})
        q.update({"status": "pending", "assigned_to": None})

    def record_requeue(self, player: str, quest_id: str, quest_type: str) -> None:
        self.record_drop(player, quest_id, quest_type)

    def record_dlq(self, player: str, quest_id: str, quest_type: str) -> None:
        self.inflight_by_player.get(player, set()).discard(quest_id)
        self.dlq_messages.append({
            "quest_id": quest_id,
            "quest_type": quest_type,
            "player": player,
            "ts": time.time(),
        })

    def record_done(self, player: str, quest_id: str, quest_type: str) -> None:
        q = self.quests_state.setdefault(quest_id, {"quest_type": quest_type})
        q.update({"status": "completed", "completed_at": time.time()})
        st = self.player_stats.setdefault(player, {"accepted": 0, "completed": 0, "failed": 0})
        st["completed"] = st.get("completed", 0) + 1
        self.inflight_by_player.get(player, set()).discard(quest_id)

    def record_fail(self, player: str, quest_id: str, quest_type: str) -> None:
        q = self.quests_state.setdefault(quest_id, {"quest_type": quest_type})
        q.update({"status": "failed", "completed_at": time.time()})
        st = self.player_stats.setdefault(player, {"accepted": 0, "completed": 0, "failed": 0})
        st["failed"] = st.get("failed", 0) + 1
        self.inflight_by_player.get(player, set()).discard(quest_id)

    # Metrics snapshot
    def snapshot(self) -> Dict:
        per_type: Dict[str, Dict[str, int]] = {}
        for q in self.quests_state.values():
            qt = q.get("quest_type", "?")
            per_type.setdefault(qt, {"pending": 0, "accepted": 0, "completed": 0, "failed": 0})
            st = q.get("status", "pending")
            key = "accepted" if st == "accepted" else st
            if key in per_type[qt]:
                per_type[qt][key] += 1
        total_pending = sum(m.get("pending", 0) for m in per_type.values())
        # enrich player stats with inflight
        enriched_players: Dict[str, Dict] = {}
        for p in self.roster.keys():
            base = self.player_stats.setdefault(p, {"accepted": 0, "completed": 0, "failed": 0})
            infl = len(self.inflight_by_player.get(p, set()))
            enriched_players[p] = {**base, "inflight": infl}
        return {
            "metrics": {"per_type": per_type, "total_pending": total_pending},
            "player_stats": enriched_players,
        }