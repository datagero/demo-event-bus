"""
Card Quest Challenge - A plug-and-play game mode for Queue Quest

This module provides a card-based challenge system that teaches data engineering
concepts through timed scenarios and scoring mechanics.
"""

import random
import time
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, field


@dataclass
class CardEffect:
    """Represents an active card effect."""
    effect_id: str
    card: dict
    expires_at: float
    params: dict = field(default_factory=dict)


class CardGame:
    """Main card game controller."""
    
    def __init__(self, broadcast_fn, game_state, players_dict, skill_ttl_dict, start_player_fn):
        """Initialize card game with dependencies from main application."""
        self.broadcast = broadcast_fn
        self.game_state = game_state
        self.players = players_dict
        self.skill_ttl_ms = skill_ttl_dict
        self.start_player = start_player_fn
        
        # Game state
        self.active = False
        self.card_draw_timer = 0
        self.round_duration = 300  # 5 minutes default
        self.round_start_time = 0
        self.game_score = 1000
        self.cards_drawn = []
        self.active_effects: Dict[str, CardEffect] = {}
        
        # Card definitions
        self.card_deck = {
            "green": [
                {"id": "new_hire", "name": "New Hire", "desc": "A new character joins with a random skill. Create their queue and bind it fast.", "duration": 0},
                {"id": "cross_training", "name": "Cross-Training", "desc": "An existing character learns an extra skill — bind them to more routing keys.", "duration": 0},
                {"id": "upgrade_queue", "name": "Upgrade Queue", "desc": "Extend retention on one queue for the next 2 minutes.", "duration": 120},
                {"id": "scale_up", "name": "Scale Up", "desc": "Add 1 more consumer to an overloaded queue (max 2 per queue).", "duration": 0},
                {"id": "alternate_route", "name": "Alternate Route", "desc": "Create a second queue for a skill to act as a hot standby.", "duration": 0},
            ],
            "yellow": [
                {"id": "message_surge", "name": "Message Surge", "desc": "For the next 15 seconds, incoming messages double.", "duration": 15},
                {"id": "skill_boom", "name": "Skill Boom", "desc": "One skill's message rate triples for 20 seconds.", "duration": 20},
                {"id": "multi_skill_wave", "name": "Multi-Skill Wave", "desc": "Messages for 3 random skills arrive in quick bursts.", "duration": 10},
                {"id": "steady_growth", "name": "Steady Growth", "desc": "All skills see +20% sustained increase until end of round.", "duration": -1},
            ],
            "red": [
                {"id": "character_quits", "name": "Character Quits", "desc": "Remove a random character (delete their queue/bindings).", "duration": 0},
                {"id": "queue_blocked", "name": "Queue Blocked", "desc": "A queue stops delivering (consumer paused) for 15 seconds.", "duration": 15},
                {"id": "retention_shrink", "name": "Retention Shrink", "desc": "One queue's retention period halves for 30 seconds.", "duration": 30},
                {"id": "network_partition", "name": "Network Partition", "desc": "All consumers stop acking for 10 seconds — messages pile up.", "duration": 10},
                {"id": "exchange_misbind", "name": "Exchange Misbind", "desc": "A routing key binding disappears for 1 skill — unroutable messages start appearing.", "duration": 20},
            ],
            "black": [
                {"id": "storm_of_skills", "name": "Storm of Skills", "desc": "5 new skills are added at once, but no queues for them exist yet.", "duration": 0},
                {"id": "mass_resignation", "name": "Mass Resignation", "desc": "Half your characters leave.", "duration": 0},
                {"id": "broker_glitch", "name": "Broker Glitch", "desc": "20% of messages sent during the next 10 seconds become unroutable.", "duration": 10},
                {"id": "retention_purge", "name": "Retention Purge", "desc": "All queues instantly drop expired messages.", "duration": 0},
                {"id": "random_replay", "name": "Random Replay", "desc": "A DLQ flushes back into the main exchange at high speed.", "duration": 5},
            ]
        }

    def start_round(self, duration: int = 300) -> bool:
        """Start a new card game round."""
        if self.active:
            return False
            
        self.active = True
        self.card_draw_timer = 30  # First card in 30 seconds
        self.round_start_time = time.time()
        self.round_duration = duration
        self.game_score = 1000
        self.cards_drawn = []
        self.active_effects = {}
        
        self.broadcast("card_game_started", {"duration": duration, "score": self.game_score})
        return True

    def stop_round(self) -> dict:
        """Stop the current card game."""
        if not self.active:
            return {"error": "No active game"}
            
        final_score = self.game_score
        self.active = False
        self.active_effects = {}
        
        self.broadcast("card_game_stopped", {"final_score": final_score})
        return {"final_score": final_score}

    def get_status(self) -> dict:
        """Get current card game status."""
        elapsed = time.time() - self.round_start_time if self.active else 0
        return {
            "active": self.active,
            "score": self.game_score,
            "timer": self.card_draw_timer,
            "elapsed": elapsed,
            "duration": self.round_duration,
            "cards_drawn": self.cards_drawn,
            "active_effects": [effect.__dict__ for effect in self.active_effects.values()]
        }

    def draw_random_card(self) -> dict:
        """Draw a random card weighted by type."""
        # Weight distribution: 40% green, 30% yellow, 20% red, 10% black
        weights = [40, 30, 20, 10]
        colors = ['green', 'yellow', 'red', 'black']
        
        color = random.choices(colors, weights=weights)[0]
        card = random.choice(self.card_deck[color]).copy()
        card['color'] = color
        
        return card

    def manual_draw(self) -> dict:
        """Manually draw a card (for testing)."""
        if not self.active:
            return {"error": "Card game not active"}
            
        card = self.draw_random_card()
        self.cards_drawn.append(card)
        self.apply_card_effect(card)
        
        return {"card": card}

    def apply_card_effect(self, card: dict):
        """Apply a card's effect to the game state."""
        effect_id = f"{card['id']}_{int(time.time())}"
        expires_at = time.time() + card['duration'] if card['duration'] > 0 else 0
        
        # Apply specific card effects
        if card['id'] == 'new_hire':
            self._effect_new_hire()
        elif card['id'] == 'cross_training':
            self._effect_cross_training()
        elif card['id'] == 'message_surge':
            self._effect_message_surge(effect_id, expires_at)
        elif card['id'] == 'skill_boom':
            self._effect_skill_boom(effect_id, expires_at)
        elif card['id'] == 'character_quits':
            self._effect_character_quits()
        elif card['id'] == 'queue_blocked':
            self._effect_queue_blocked(effect_id, expires_at)
        elif card['id'] == 'network_partition':
            self._effect_network_partition(effect_id, expires_at)
        elif card['id'] == 'broker_glitch':
            self._effect_broker_glitch(effect_id, expires_at)
        elif card['id'] == 'retention_shrink':
            self._effect_retention_shrink(effect_id, expires_at)
        # Add more effects as needed...
        
        # Store effect for tracking
        if expires_at > 0:
            self.active_effects[effect_id] = CardEffect(
                effect_id=effect_id,
                card=card,
                expires_at=expires_at
            )
            
        self.broadcast("card_applied", {"card": card, "effect_id": effect_id, "expires_at": expires_at})

    def update_effects(self):
        """Remove expired effects and apply ongoing ones."""
        now = time.time()
        expired_ids = [eid for eid, effect in self.active_effects.items() 
                      if effect.expires_at > 0 and effect.expires_at <= now]
        
        for eid in expired_ids:
            effect = self.active_effects[eid]
            self._restore_effect(effect)
            del self.active_effects[eid]
            self.broadcast("card_expired", {"effect_id": eid, "type": effect.card['id']})

    def tick(self):
        """Called every second to update game state."""
        if not self.active:
            return
            
        # Update card timer
        self.card_draw_timer -= 1
        if self.card_draw_timer <= 0:
            # Draw a new card
            card = self.draw_random_card()
            self.cards_drawn.append(card)
            self.apply_card_effect(card)
            self.card_draw_timer = 30  # Reset to 30 seconds
            self.broadcast("card_drawn", {"card": card, "next_in": self.card_draw_timer})
        
        # Check round end
        elapsed = time.time() - self.round_start_time
        if elapsed >= self.round_duration:
            self.active = False
            self.broadcast("round_ended", {"score": self.game_score, "duration": elapsed})
            
        # Update effects
        self.update_effects()

    def adjust_score(self, delta: int, reason: str = ""):
        """Adjust game score and broadcast if significant."""
        if not self.active:
            return
            
        self.game_score += delta
        if abs(delta) >= 5:  # Only broadcast significant changes
            self.broadcast("score_changed", {"score": self.game_score, "delta": delta, "reason": reason})

    # Card effect implementations
    def _effect_new_hire(self):
        """Add a new random character."""
        skills = ['gather', 'slay', 'escort']
        name = f"temp{random.randint(1000,9999)}"
        skill = random.choice(skills)
        self.start_player(name, [skill], 0.05, 1.0, 1)
        self.broadcast("card_effect", {"type": "new_hire", "player": name, "skill": skill})

    def _effect_cross_training(self):
        """Add random skill to existing player."""
        if not self.players:
            return
            
        player = random.choice(list(self.players.keys()))
        skills = ['gather', 'slay', 'escort']
        current_skills = self.players[player].get('skills', [])
        new_skills = [s for s in skills if s not in current_skills]
        
        if new_skills:
            new_skill = random.choice(new_skills)
            self.players[player]['skills'].append(new_skill)
            self.broadcast("card_effect", {"type": "cross_training", "player": player, "new_skill": new_skill})

    def _effect_message_surge(self, effect_id: str, expires_at: float):
        """Double message rate for duration."""
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "message_surge", "name": "Message Surge"},
            expires_at=expires_at,
            params={"multiplier": 2.0}
        )

    def _effect_skill_boom(self, effect_id: str, expires_at: float):
        """Triple message rate for one skill."""
        skills = ['gather', 'slay', 'escort']
        skill = random.choice(skills)
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "skill_boom", "name": "Skill Boom"},
            expires_at=expires_at,
            params={"skill": skill, "multiplier": 3.0}
        )

    def _effect_character_quits(self):
        """Remove a random character."""
        if not self.players:
            return
            
        player = random.choice(list(self.players.keys()))
        self.broadcast("card_effect", {"type": "character_quits", "player": player})
        # Note: Actual deletion should be handled by the main application

    def _effect_queue_blocked(self, effect_id: str, expires_at: float):
        """Block a random queue."""
        if not self.players:
            return
            
        player = random.choice(list(self.players.keys()))
        if player in self.players:
            self.players[player]['paused'] = True
            
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "queue_blocked", "name": "Queue Blocked"},
            expires_at=expires_at,
            params={"player": player}
        )

    def _effect_network_partition(self, effect_id: str, expires_at: float):
        """Simulate network partition."""
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "network_partition", "name": "Network Partition"},
            expires_at=expires_at
        )

    def _effect_broker_glitch(self, effect_id: str, expires_at: float):
        """Make messages unroutable."""
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "broker_glitch", "name": "Broker Glitch"},
            expires_at=expires_at,
            params={"fail_rate": 0.2}
        )

    def _effect_retention_shrink(self, effect_id: str, expires_at: float):
        """Halve retention for one skill."""
        skills = ['gather', 'slay', 'escort']
        skill = random.choice(skills)
        original_ttl = self.skill_ttl_ms.get(skill, 30000)
        self.skill_ttl_ms[skill] = original_ttl // 2
        
        self.active_effects[effect_id] = CardEffect(
            effect_id=effect_id,
            card={"id": "retention_shrink", "name": "Retention Shrink"},
            expires_at=expires_at,
            params={"skill": skill, "original_ttl": original_ttl}
        )

    def _restore_effect(self, effect: CardEffect):
        """Restore state after effect expires."""
        if effect.card['id'] == 'retention_shrink':
            skill = effect.params['skill']
            self.skill_ttl_ms[skill] = effect.params['original_ttl']
        elif effect.card['id'] == 'queue_blocked':
            player = effect.params['player']
            if player in self.players:
                self.players[player]['paused'] = False

    def is_effect_active(self, effect_type: str) -> bool:
        """Check if a specific effect type is currently active."""
        return any(effect.card['id'] == effect_type for effect in self.active_effects.values())

    def get_effect_multiplier(self, context: str = "message") -> float:
        """Get message rate multiplier from active effects."""
        multiplier = 1.0
        
        for effect in self.active_effects.values():
            if effect.card['id'] == 'message_surge':
                multiplier *= effect.params.get('multiplier', 2.0)
            elif effect.card['id'] == 'steady_growth':
                multiplier *= 1.2
                
        return multiplier

    def get_skill_multiplier(self, skill: str) -> float:
        """Get skill-specific rate multiplier."""
        multiplier = 1.0
        
        for effect in self.active_effects.values():
            if effect.card['id'] == 'skill_boom' and effect.params.get('skill') == skill:
                multiplier *= effect.params.get('multiplier', 3.0)
                
        return multiplier

    def should_fail_message(self) -> bool:
        """Check if message should fail due to broker glitch."""
        for effect in self.active_effects.values():
            if effect.card['id'] == 'broker_glitch':
                fail_rate = effect.params.get('fail_rate', 0.2)
                return random.random() < fail_rate
        return False

    def get_deck(self) -> dict:
        """Get all available cards by color."""
        return self.card_deck