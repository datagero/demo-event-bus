"""
Python client for interacting with Go worker server via HTTP API.
"""
import os
import json
import time
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, field

try:
    import requests
    REQUESTS_AVAILABLE = True
except ImportError:
    REQUESTS_AVAILABLE = False
    print("⚠️ requests library not available - Go workers will be disabled")

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

class GoWorkerRequest(BaseModel):
    name: str
    skills: List[str]
    speed: float = 1.0

class GoWorkerAction(BaseModel):
    name: str
    action: str  # "pause", "resume", "crash"

@dataclass
class GoWorkersClient:
    """Client for communicating with Go workers server."""
    base_url: str
    timeout: float = 5.0
    enabled: bool = field(init=False)
    roster: Dict[str, Any] = field(default_factory=dict, init=False)
    
    def __post_init__(self):
        self.enabled = REQUESTS_AVAILABLE and self._check_server()
        if not self.enabled:
            print(f"🐍 Go workers disabled (server: {self.base_url})")
        else:
            print(f"🔗 Go workers enabled (server: {self.base_url})")
    
    def _check_server(self) -> bool:
        """Check if Go workers server is available."""
        if not REQUESTS_AVAILABLE:
            return False
        try:
            resp = requests.get(f"{self.base_url}/health", timeout=2)
            return resp.status_code == 200
        except:
            return False
    
    def _request(self, method: str, endpoint: str, **kwargs) -> Optional[dict]:
        """Make HTTP request to Go workers server."""
        if not self.enabled:
            return None
        
        try:
            resp = requests.request(
                method, 
                f"{self.base_url}{endpoint}", 
                timeout=self.timeout,
                **kwargs
            )
            resp.raise_for_status()
            return resp.json() if resp.content else {}
        except Exception as e:
            print(f"❌ Go workers request failed: {e}")
            return None
    
    def create_worker(self, name: str, skills: List[str], speed: float = 1.0, fail_pct: float = 0.1, workers: int = 1) -> bool:
        """Create a new Go worker using the correct Go server API."""
        # Use the correct Go server API
        worker_config = {
            "player": name,
            "skills": skills,
            "fail_pct": fail_pct,
            "speed_multiplier": speed,
            "workers": workers,
            "routing_mode": "skill"  # Default to skill-based routing
        }
        
        result = self._request("POST", "/start", json=worker_config)
        if result and result.get("ok"):
            # Update local roster for UI
            self.roster[name] = {
                "name": name,
                "skills": skills,
                "speed": speed,
                "status": "online",
                "works": 0,
                "fails": 0,
                "type": "go",
                "fail_pct": fail_pct,
                "workers": workers
            }
            return True
        return False
    
    def delete_worker(self, name: str) -> bool:
        """Delete a Go worker using the correct Go server API."""
        result = self._request("POST", "/stop", json={"player": name})
        if result and result.get("ok"):
            # Remove from local roster
            if name in self.roster:
                del self.roster[name]
            return True
        return False
    
    def control_worker(self, name: str, action: str) -> bool:
        """Control a Go worker using the correct Go server API."""
        endpoint = None
        if action == "pause":
            endpoint = "/pause"
        elif action == "resume":
            endpoint = "/resume" 
        else:
            # For crash or other actions, simulate locally
            if name in self.roster:
                self.roster[name]["status"] = "reconnecting"
                return True
            return False
            
        if endpoint:
            result = self._request("POST", endpoint, json={"player": name})
            if result and result.get("ok"):
                # Update local status
                if name in self.roster:
                    if action == "pause":
                        self.roster[name]["status"] = "disconnected"
                    elif action == "resume":
                        self.roster[name]["status"] = "online"
                return True
        return False
    
    def refresh_roster(self):
        """Refresh the cached roster from Go workers server."""
        result = self._request("GET", "/workers")
        if result and "workers" in result:
            # Convert Go workers format to match Python roster format
            self.roster = {}
            for worker in result["workers"]:
                name = worker["name"]
                self.roster[name] = {
                    "name": name,
                    "skills": worker["skills"],
                    "speed": worker["speed"],
                    "status": worker["status"],
                    "works": worker.get("works", 0),
                    "fails": worker.get("fails", 0),
                    "type": "go"  # Mark as Go worker
                }
    
    def get_status(self) -> dict:
        """Get overall status of Go workers system."""
        # Refresh enabled status on each call
        self.enabled = self._check_server()
        if not self.enabled:
            return {"enabled": False, "workers": 0}
        
        # Get status from Go server
        result = self._request("GET", "/status")
        if result:
            worker_count = result.get("worker_count", 0)
            worker_names = result.get("workers", [])
            
            # Update local roster with workers from Go server
            # Only add new workers, preserve existing worker details
            current_workers = set(worker_names)
            # Remove workers that no longer exist
            to_remove = [name for name in self.roster.keys() if name not in current_workers]
            for name in to_remove:
                del self.roster[name]
            
            # Add new workers
            for worker_name in worker_names:
                if worker_name not in self.roster:
                    self.roster[worker_name] = {
                        "name": worker_name,
                        "skills": ["gather", "slay", "escort"],  # Default skills for now
                        "speed": 1.0,
                        "status": "online",
                        "works": 0,
                        "fails": 0,
                        "type": "go",
                        "fail_pct": 0.1,
                        "speed_multiplier": 1.0,
                        "workers": 1
                    }
            
            return {
                "enabled": True,
                "workers": worker_names,  # Return actual worker names, not count
                "worker_count": worker_count,
                "total_messages": 0,  # Not tracked by Go server yet
                "active_connections": worker_count  # Assume all workers are connected
            }
        return {"enabled": False, "workers": 0}

def create_go_workers_router(client: GoWorkersClient, broadcast_func) -> APIRouter:
    """Create FastAPI router for Go workers endpoints."""
    router = APIRouter(prefix="/api/go-workers", tags=["go-workers"])
    
    @router.get("/status")
    async def get_go_status():
        """Get Go workers status."""
        return client.get_status()
    
    @router.post("/workers")
    async def create_go_worker(req: GoWorkerRequest):
        """Create a new Go worker."""
        if not client.enabled:
            raise HTTPException(status_code=503, detail="Go workers not available")
        
        success = client.create_worker(req.name, req.skills, req.speed)
        if success:
            broadcast_func("worker_created", {
                "name": req.name, 
                "type": "go",
                "skills": req.skills,
                "speed": req.speed
            })
            return {"success": True}
        else:
            raise HTTPException(status_code=400, detail="Failed to create Go worker")
    
    @router.delete("/workers/{name}")
    async def delete_go_worker(name: str):
        """Delete a Go worker."""
        if not client.enabled:
            raise HTTPException(status_code=503, detail="Go workers not available")
        
        success = client.delete_worker(name)
        if success:
            broadcast_func("worker_deleted", {"name": name, "type": "go"})
            return {"success": True}
        else:
            raise HTTPException(status_code=404, detail="Worker not found")
    
    @router.post("/workers/{name}/control")
    async def control_go_worker(name: str, action: GoWorkerAction):
        """Control a Go worker."""
        if not client.enabled:
            raise HTTPException(status_code=503, detail="Go workers not available")
        
        success = client.control_worker(name, action.action)
        if success:
            broadcast_func("worker_action", {
                "name": name, 
                "type": "go",
                "action": action.action
            })
            return {"success": True}
        else:
            raise HTTPException(status_code=400, detail="Failed to control worker")
    
    # Webhook endpoint for Go workers to send events back to Python
    @router.post("/webhook/events")
    async def receive_go_events(event: dict):
        """Receive events from Go workers via webhook."""
        event_type = event.get("type")
        worker_event = event.get("event")  # "accept", "completed", "failed"
        message_data = event.get("message", {})
        player = event.get("player")
        
        # Handle message events from Go workers - see timeline_statuses.md for documentation
        if event_type == "message_event" and worker_event:
            if worker_event == "accept":
                # Broadcast acceptance for timeline (UI expects 'player_accept' event)
                # Reference: timeline_statuses.md - RECEIVED status implementation
                broadcast_func("player_accept", {
                    "case_id": message_data.get("case_id"),
                    "quest_type": message_data.get("quest_type"),
                    "difficulty": message_data.get("difficulty"),
                    "player": player,
                    "source": "go-worker"
                })
            elif worker_event == "completed":
                # Broadcast completion for timeline (UI expects 'result_done' event)
                # Reference: timeline_statuses.md - COMPLETED status implementation
                broadcast_func("result_done", {
                    "case_id": message_data.get("case_id"),
                    "quest_type": message_data.get("quest_type"),
                    "difficulty": message_data.get("difficulty"),
                    "player": player,
                    "points": message_data.get("points", 0),
                    "source": "go-worker"
                })
            elif worker_event == "failed":
                # Broadcast failure for timeline (UI expects 'result_fail' event)
                # Reference: timeline_statuses.md - FAILED status implementation
                broadcast_func("result_fail", {
                    "case_id": message_data.get("case_id"),
                    "quest_type": message_data.get("quest_type"),
                    "difficulty": message_data.get("difficulty"),
                    "player": player,
                    "points": 0,
                    "source": "go-worker"
                })
        elif event_type == "worker_status_change":
            client.refresh_roster()
            broadcast_func("roster_update", {"type": "go"})
        elif event_type == "chaos_event":
            # Handle chaos events from Go chaos manager
            chaos_event = event.get("event")  # "disconnect", "reconnect", "reconnect_failed"
            description = event.get("description", "")
            player = event.get("player")
            
            if chaos_event == "disconnect":
                # Broadcast disconnect warning for timeline and logs
                broadcast_func("chaos_disconnect", {
                    "player": player,
                    "description": description,
                    "source": "go-chaos"
                })
            elif chaos_event == "reconnect":
                # Broadcast successful reconnect for timeline and logs
                broadcast_func("chaos_reconnect", {
                    "player": player,
                    "description": description,
                    "source": "go-chaos"
                })
            elif chaos_event == "reconnect_failed":
                # Broadcast failed reconnect for timeline and logs
                broadcast_func("chaos_reconnect_failed", {
                    "player": player,
                    "description": description,
                    "source": "go-chaos"
                })
        
        return {"received": True}
    
    return router