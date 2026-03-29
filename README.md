# Robotaxi System Design — Go Implementation

A working implementation of a robotaxi ride-matching system, built with Go following **Clean Architecture**, **Hexagonal (Ports & Adapters)** patterns, and idiomatic Go conventions.

## Architecture

```mermaid
flowchart LR
    subgraph rider["Rider"]
        R["Rider App"]
    end

    subgraph core["Core Services"]
        FUC["FareUseCase\nfare estimation"]
        RUC["RideUseCase\nride creation"]
        Q(["Queue\nmessage buffer"])
        MUC["MatchingUseCase\nmatching + dispatch"]
    end

    subgraph storage["Storage"]
        DB[("SQLite\nFares / Rides")]
        GEO["Redis GEO\nAV locations"]
        LOCK["Redis\nmatching state & lock"]
        DISP["Redis\nDispatch Queue"]
    end

    subgraph av["AV Fleet"]
        A["AV Simulator"]
    end

    R -->|"1. get fare estimate"| FUC
    FUC --> DB

    R -->|"2. request ride"| RUC
    RUC --> DB
    RUC -->|"3. publish"| Q

    Q -->|"4. consume"| MUC
    MUC -->|"5. find nearby AVs"| GEO
    MUC -->|"6. acquire lock + dispatch"| LOCK
    MUC -->|"7. send command"| DISP
    MUC -->|"8. write match result"| DB

    A -->|"periodic location update"| GEO
    DISP -->|"poll for command"| A
    A -->|"ACCEPT / REJECT"| DISP

    R -->|"9. poll ride status"| DB

    style Q fill:#f5a623,color:#000
    style GEO fill:#c0392b,color:#fff
    style LOCK fill:#c0392b,color:#fff
    style DISP fill:#c0392b,color:#fff
    style DB fill:#2980b9,color:#fff
```

### Layer Structure (Clean Architecture)

> Dependencies point inward — outer layers know about inner layers, never the reverse.

```mermaid
flowchart TD
    subgraph outer["Outer — Adapters (swappable)"]
        H["handler/\nHTTP gin"]
        Rep["repository/\nGORM + SQLite"]
        Redis["redisstore/\nRedis"]
        Que["queue/\nGo channel"]
    end

    subgraph middle["Middle — UseCases (business logic)"]
        UC["usecase/\nFareUseCase · RideUseCase · MatchingUseCase"]
    end

    subgraph inner["Inner — Domain (zero dependencies)"]
        D["domain/\nentities.go  data structures\nports.go     interfaces"]
    end

    outer -->|"call use cases"| middle
    middle -->|"depend on interfaces only"| inner
    outer -.->|"implement interfaces"| inner

    style inner fill:#27ae60,color:#fff
    style middle fill:#2980b9,color:#fff
    style outer fill:#7f8c8d,color:#fff
```

## System Design Concepts

| Deep Dive | Problem | Solution | Code Location |
|---|---|---|---|
| Deep Dive 1 | 10M AVs × 1 update/5s = 2M writes/sec | Redis `GEOADD` / `GEOSEARCH` | `redisstore/location.go` |
| Deep Dive 2 | Requests dropped at peak load | Async queue between Ride Service & Matching | `queue/queue.go`, `usecase/ride.go` |
| Deep Dive 3 | Same ride → multiple AVs | Per-ride `SET NX EX` lock + shared Redis state | `redisstore/location.go`, `usecase/matching.go` |
| Deep Dive 4 | Same AV → multiple rides | DB transaction + uniqueness check (partial index equivalent) | `repository/ride_repo.go` → `AssignAV()` |

## API

### Rider
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/fare` | Get fare estimate |
| `POST` | `/rides` | Request a ride (triggers async matching) |
| `GET`  | `/rides/:id` | Poll ride status |

### AV (simplified from gRPC to HTTP)
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/av/location` | Send location update |
| `GET`  | `/av/:id/dispatch` | Poll for dispatch command |
| `POST` | `/av/:id/dispatch/:ride_id/decision` | Submit ACCEPT/REJECT |

## How to Run

**Prerequisites:** Go 1.21+, Docker

```bash
# 1. Start Redis
docker-compose up -d

# 2. Start the server
go run ./cmd/server

# 3. Start the AV fleet simulator (separate terminal)
go run ./scripts/simulate_av

# 4. Test the flow
# Step 1: Get fare estimate
curl -s -X POST http://localhost:8080/fare \
  -H "Content-Type: application/json" \
  -d '{"pickup_location":{"lat":37.7749,"lng":-122.4194},"destination":{"lat":37.7849,"lng":-122.4094}}'

# Step 2: Request a ride (use fare_id from above)
curl -s -X POST http://localhost:8080/rides \
  -H "Content-Type: application/json" \
  -d '{"fare_id":"<fare_id>"}'

# Step 3: Poll until DRIVER_ASSIGNED
curl -s http://localhost:8080/rides/<ride_id>
```

## Testing

```bash
go test ./...               # all tests
go test -race ./...         # with race detector
go test -cover ./...        # with coverage
```

Tests follow table-driven patterns with interface-based mocks — no real DB or Redis required.

## Redis Key Schema

```
geo:av_locations            GEO set: all AV positions
av:status:{av_id}           HASH: status, battery_level, lat, lng
dispatch:av:{av_id}         LIST: pending dispatch commands
decision:ride:{r}:av:{a}    LIST: AV's ACCEPT/REJECT response
match:ride:{ride_id}        HASH: candidates, cursor, status (SEARCHING|DONE)
match:lock:{ride_id}        STRING (SET NX EX): per-ride distributed lock
```

## Production Differences

| This demo | Production |
|-----------|-----------|
| `queue.RideQueue` (in-memory chan) | Kafka / AWS SQS |
| SQLite | PostgreSQL + partial unique index on `rides(av_id)` |
| HTTP polling for AV dispatch | gRPC bidirectional stream |
| Single Redis instance | Redis Cluster (for 2M writes/sec) |
| Single matching goroutine | Horizontally scaled stateless workers |
