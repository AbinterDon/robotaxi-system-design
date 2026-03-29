# Robotaxi System Design — Go Implementation

A working implementation of a robotaxi ride-matching system, built with Go following **Clean Architecture**, **Hexagonal (Ports & Adapters)** patterns, and idiomatic Go conventions.

## Architecture

```mermaid
flowchart LR
    subgraph rider["Rider"]
        R["Rider App"]
    end

    subgraph core["Core Services"]
        FUC["FareUseCase\n報價計算"]
        RUC["RideUseCase\n建立叫車"]
        Q(["Queue\n消息隊列"])
        MUC["MatchingUseCase\n配對 + 派車"]
    end

    subgraph storage["Storage"]
        DB[("SQLite\nFares / Rides")]
        GEO["Redis GEO\nAV 位置"]
        LOCK["Redis\n配對狀態 & 鎖"]
        DISP["Redis\nDispatch Queue"]
    end

    subgraph av["AV Fleet"]
        A["AV Simulator"]
    end

    R -->|"① 取得報價"| FUC
    FUC --> DB

    R -->|"② 確認叫車"| RUC
    RUC --> DB
    RUC -->|"③ 發布請求"| Q

    Q -->|"④ 消費請求"| MUC
    MUC -->|"⑤ 查詢附近AV"| GEO
    MUC -->|"⑥ 搶鎖 + 派車"| LOCK
    MUC -->|"⑦ 發送指令"| DISP
    MUC -->|"⑧ 寫入配對結果"| DB

    A -->|"定時更新位置"| GEO
    DISP -->|"輪詢派車指令"| A
    A -->|"ACCEPT / REJECT"| DISP

    R -->|"⑨ 輪詢結果"| DB

    style Q fill:#f5a623,color:#000
    style GEO fill:#c0392b,color:#fff
    style LOCK fill:#c0392b,color:#fff
    style DISP fill:#c0392b,color:#fff
    style DB fill:#2980b9,color:#fff
```

### Layer Structure (Clean Architecture)

> 依賴方向：外層依賴內層，內層不知道外層的存在

```mermaid
flowchart TD
    subgraph outer["外層 — Adapters（可替換）"]
        H["handler/\nHTTP gin"]
        Rep["repository/\nGORM + SQLite"]
        Redis["redisstore/\nRedis"]
        Que["queue/\nGo channel"]
    end

    subgraph middle["中層 — UseCase（業務邏輯）"]
        UC["usecase/\nFareUseCase · RideUseCase · MatchingUseCase"]
    end

    subgraph inner["內層 — Domain（核心，零依賴）"]
        D["domain/\nentities.go  定義資料結構\nports.go     定義介面"]
    end

    outer -->|"呼叫業務邏輯"| middle
    middle -->|"只依賴介面"| inner
    outer -.->|"實作介面"| inner

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
