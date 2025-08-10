# Architecture Decision Records (ADRs)

## ADR-001: Go Migration Strategy - Hybrid Architecture

**Date**: August 10, 2025  
**Status**: Approved  
**Decision Maker**: Project Team

### Context

The demo-event-bus application currently uses:
- **Python FastAPI** for web server, APIs, and game logic (~2,600 lines)
- **Go workers** for high-performance message processing (~1,200 lines) 
- **JavaScript/HTML frontend** for real-time UI (~1,500 lines)

We evaluated migrating further components to Go to improve performance and operational simplicity.

### Decision

**We choose Option 1: Hybrid Architecture** with phased migration:

#### **Phase 1: Core API Migration** ⭐ **(IN PROGRESS)**
- **Migrate**: HTTP APIs, WebSocket broadcaster, worker management
- **Keep**: Game logic, scenarios, card game in Python
- **Communication**: HTTP APIs between Go and Python services
- **Timeline**: 2-3 weeks

#### **Future Phases** *(Optional)*
- **Phase 2**: Game state management (1-2 weeks)
- **Phase 3**: Educational scenarios (1 week)  
- **Phase 4**: Gamification system (2-3 weeks, high complexity)

### Rationale

#### **Why Hybrid Over Full Go Migration**
1. **Lower Risk**: Gradual migration vs big-bang rewrite
2. **Faster Delivery**: Immediate performance gains without full rewrite
3. **Best of Both**: Go performance + Python flexibility
4. **Educational Value**: Demonstrates microservices communication patterns
5. **Team Efficiency**: Leverage Python's rapid prototyping for game logic

#### **Why Phase 1 First**
1. **High Impact**: API performance is user-facing
2. **Clear Boundaries**: Well-defined interfaces
3. **Low Complexity**: Stateless HTTP operations
4. **Immediate Benefits**: WebSocket performance gains
5. **Foundation**: Sets up architecture for future phases

### Expected Benefits

#### **Performance**
- **5-10x faster** API response times
- **Better WebSocket** handling (thousands of concurrent connections)
- **Lower memory** footprint for core services
- **Faster startup** times (compiled Go vs Python imports)

#### **Operational**
- **Simpler deployment** for core API service (single binary)
- **Better resource** utilization
- **Cleaner service** boundaries

#### **Development**
- **Type safety** for critical API layer
- **Hot reload** already implemented with `air`
- **Unified language** for workers + APIs
- **Educational value** of Go web development

### Architecture After Phase 1

```
┌─────────────────┐    HTTP APIs    ┌─────────────────┐
│   Go API Server │ ←──────────────→ │ Python Game Svc │
│                 │                 │                 │
│ • HTTP APIs     │                 │ • Game Logic    │
│ • WebSocket     │                 │ • Scenarios     │
│ • Worker Mgmt   │                 │ • Card Game     │
│ • Static Files  │                 │ • Complex State │
└─────────────────┘                 └─────────────────┘
         │                                   │
         └────── WebSocket ──────────────────┘
         │                Frontend           │
┌─────────────────┐                 ┌─────────────────┐
│  Go Workers     │                 │    RabbitMQ     │
│                 │ ←──────────────→ │                 │
│ • Message Proc  │                 │ • Message Broker│
│ • Chaos System  │                 │ • Queues        │
└─────────────────┘                 └─────────────────┘
```

### Implementation Plan - Phase 1

#### **Core Components to Migrate**
1. **HTTP Router** (FastAPI → Go web framework)
2. **WebSocket Handler** (FastAPI WebSocket → Go WebSocket)  
3. **API Endpoints** (~30 endpoints)
4. **Static File Serving** 
5. **Health Checks**

#### **Components to Keep in Python**
1. **Game Logic** (`card_game.py`, `state.py`)
2. **Scenarios** (`scenarios.py`) 
3. **Complex Business Rules**
4. **Rapid Prototyping Features**

#### **Communication Protocol**
- **Go → Python**: HTTP REST APIs
- **Python → Go**: HTTP webhooks (existing pattern)
- **Go → Frontend**: WebSocket (direct)
- **Python → Frontend**: via Go WebSocket proxy

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| **Dual service complexity** | Clear service boundaries, automated deployment |
| **Cross-service debugging** | Structured logging, health checks, monitoring |
| **API versioning** | Use semantic versioning, backward compatibility |
| **Team Go expertise** | Pair programming, code reviews, documentation |

### Success Criteria

#### **Phase 1 Success Metrics**
- [ ] **API Performance**: 5x faster response times
- [ ] **WebSocket**: Handle 1000+ concurrent connections  
- [ ] **Reliability**: 99.9% uptime for API service
- [ ] **Development**: Hot reload working for Go APIs
- [ ] **Compatibility**: All existing features work unchanged

#### **Decision Review**
- **Review Date**: After Phase 1 completion (~3 weeks)
- **Go/No-Go Criteria**: Performance gains, development velocity, operational complexity

### Alternatives Considered

1. **Full Go Migration**: Higher risk, longer timeline, more complexity
2. **Stay Python-Heavy**: Miss performance opportunities, single-language benefits
3. **Complete Rewrite**: Too disruptive, would lose educational content iterations

---

## Implementation Status

- [x] **ADR Documented**
- [x] **Team Alignment** 
- [ ] **Phase 1 Implementation** (IN PROGRESS)
- [ ] **Performance Testing**
- [ ] **Production Deployment**

---

*This document will be updated as implementation progresses and decisions are validated.*