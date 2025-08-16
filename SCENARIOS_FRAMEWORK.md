# 🎯 Scenarios Framework Documentation

## **Overview**

The Scenarios Framework provides a test-driven approach to complex integration testing, exposing scenario tests as API endpoints for both UI interaction and automated testing.

## **🏗️ Architecture**

### **Clean, Simple Structure**
```
api-server/
├── internal/api/handlers/scenario_api.go             🌐 API endpoints with scenario logic  
└── tests/scenarios/scenarios_api_validation_test.go  🧪 API validation with RabbitMQ state checking
```

### **Key Principles**
- ✅ **Worker service MUST be available** - tests fail if not running
- ✅ **API Reuse (DRY)** - leverages existing `/api/player/start` and `/api/player/delete` endpoints
- ✅ **Real-time Quest Log** - narrative messages broadcast to UI via WebSocket
- ✅ **Direct RabbitMQ Validation** - queries RabbitMQ Management API for state verification
- ✅ **Minimal Complexity** - no adapter patterns or over-engineering

## **🚀 How It Works**

### **1. API Endpoints** (`scenario_api.go`)

**Available Endpoints:**
- `GET /api/scenario-tests/` - List all available scenarios
- `POST /api/scenario-tests/run` - Execute a specific scenario

**Core Framework Features:**
```go
type ScenarioTestFramework struct {
    router *gin.Engine
    hub    *websocket.Hub  // Real-time Quest Log updates
}

// Reuses existing APIs - no code duplication
func (stf *ScenarioTestFramework) CreateWorker(name string, skills []string, failPct float64) error {
    // Calls existing /api/player/start endpoint
}

func (stf *ScenarioTestFramework) StopWorker(name string) error {
    // Calls existing /api/player/delete endpoint  
}

func (stf *ScenarioTestFramework) LogToQuestLog(message string) {
    // Broadcasts narrative messages to UI via WebSocket
}

func (stf *ScenarioTestFramework) ValidateBacklogInQueue(skill string, expectedCount int) (bool, string) {
    // Queries RabbitMQ directly to verify queue state
}
```

### **2. API Validation Tests** (`scenarios_api_validation_test.go`)
```go
func TestLateBind_Escort(t *testing.T) {
    // Check prerequisites first
    require.True(t, servicesRunning(), "All services must be running")
    
    // Call API endpoint
    resp, err := http.Post("/api/scenario-tests/run", payload)
    assert.True(t, resp.Data.Success)
    
    // 🎯 CORE VALIDATION: Query RabbitMQ directly
    unroutableCount := getUnroutableMessageCount()
    escortQueue := getEscortQueueStatus()
    assert.Equal(t, 1, unroutableCount, "Should have 1 unroutable message")
    assert.Equal(t, 1, escortQueue.Consumers, "Should have 1 active worker") 
}
```

## **📋 Current Scenarios**

### **1. Late-bind Escort (Shift Scheduling)**
**Status:** ✅ **IMPLEMENTED**

**Purpose:** Demonstrates worker handoff and backlog processing during shift changes.

**Story:** A morning shift worker handles initial escort requests, then stops for lunch break while requests accumulate. An afternoon shift worker takes over and processes the backlog.

**Steps:**
1. **Reset & Setup** - Clean game state, ensure DLQ topology
2. **Send Initial Message** - Before any worker exists (becomes unroutable)
3. **Morning Shift** - Start worker, process messages, then stop for lunch
4. **Lunch Break Backlog** - Send messages that queue up (validated via RabbitMQ query)
5. **Afternoon Shift** - New worker processes backlog and initial unroutable message

**Validation:**
- ✅ Workers created with 0% failure rate
- ✅ All messages complete successfully
- ✅ Backlog correctly accumulates during lunch break
- ✅ Final state: 1 unroutable message, 1 active worker, 0 pending messages

### **2. Reissuing DLQ (Planned)**
**Status:** 📋 **PLANNED**

**Purpose:** Validate that unroutable and failed messages can be reissued and successfully processed.

**Steps:**
1. Send `gather` message when no worker exists → unroutable DLQ
2. Create "alice" worker
3. Send messages with deterministic pass/fail outcomes
4. Reissue failed message → should now pass
5. Reissue initial unroutable message → should now pass

### **3. Orphaned Skill Queues (Planned)**  
**Status:** 📋 **PLANNED**

**Purpose:** Show queue lifecycle when workers are created/deleted.

**Steps:**
1. Create "alice" → queue becomes active
2. Delete "alice" → queue becomes orphaned but persists
3. Send messages → accumulate in orphaned queue
4. Create "bob" → same queue becomes active, bob processes backlog

## **🔧 Usage**

### **For UI Integration**
```bash
# Start full application stack (REQUIRED)
./start_app.sh

# UI calls: POST /api/scenario-tests/run
# Real-time updates appear in Quest Log
```

### **For CI/CD Testing**
```bash
# Ensure services are running
./start_app.sh &
sleep 10

# Run comprehensive API validation tests
cd api-server
go test ./tests/scenarios -v -timeout 60s
```

### **For Development**
```bash
# Test individual scenarios
curl -X POST http://localhost:9000/api/scenario-tests/run \
  -H "Content-Type: application/json" \
  -d '{"scenario": "late-bind-escort", "parameters": {"message_count": 3}}'

# List available scenarios  
curl -X GET http://localhost:9000/api/scenario-tests/
```

## **📊 Response Format**

```json
{
  "ok": true,
  "data": {
    "success": true,
    "scenario": "late-bind-escort",
    "execution_time_ms": 15234,
    "summary": "Shift scheduling scenario completed successfully",
    "message_states": [
      {
        "id": "escort-msg-1",
        "expected_state": "unroutable",
        "step": 2,
        "timestamp": 1755356298
      }
    ],
    "quest_log_entries": [
      "[SCENARIO] Step 1: Starting shift scheduling scenario...",
      "[SCENARIO] ✅ Morning shift worker created successfully",
      "[SCENARIO] 🍽️ Lunch break started - worker stopped"
    ],
    "expected_system_state": {
      "unroutable_messages": 1,
      "active_workers": 1,
      "pending_escort_messages": 0
    }
  }
}
```

## **🎯 Prerequisites**

**REQUIRED Services:**
- ✅ **RabbitMQ** (`docker-compose up -d`)
- ✅ **API Server** (`localhost:9000`)  
- ✅ **Workers Service** (`localhost:8001`)

**⚠️ All scenarios FAIL if workers service is not available.**

## **✨ Benefits**

| **Aspect** | **Result** |
|------------|------------|
| **Simplicity** | ✅ Clean 2-file architecture |
| **Dependencies** | ✅ Explicit - workers MUST be available |
| **Testing** | ✅ Real service integration, no mocking |
| **UI Integration** | ✅ Direct API endpoints + real-time Quest Log |
| **CI/CD** | ✅ Comprehensive validation with RabbitMQ state checking |
| **Maintenance** | ✅ Minimal code, clear separation of concerns |

## **🔧 Adding New Scenarios**

1. **Add scenario logic** to `scenario_api.go` 
2. **Add validation test** to `scenarios_api_validation_test.go`
3. **Update endpoint routing** if needed
4. **Document** in this file

**Simple, clean, no over-engineering.** 🎉