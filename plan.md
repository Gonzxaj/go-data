# 🚀 Mini Monitoring System (Netdata-like)

## 🧠 Objetivo

Sistema liviano de monitoreo en Go con:

- CPU
- RAM
- Load
- Disk
- Docker

Incluye:
- API REST
- Frontend (HTML + JS)
- Storage: Memory + InfluxDB
- Arquitectura SOLID

---

# 🧱 Estructura del proyecto

```
cmd/
  └── server/
internal/
  ├── app/
  ├── api/
  ├── collector/
  ├── storage/
  ├── domain/
  ├── config/
web/
  └── static/
config.json
```

---

# 🧩 Principios SOLID

## S - Single Responsibility
- collector → recolecta métricas
- storage → guarda datos
- api → expone endpoints

## O - Open/Closed
- podés agregar nuevos storage sin modificar código existente

## L - Liskov
- todos los storage implementan la misma interfaz

## I - Interface Segregation
- interfaces pequeñas

## D - Dependency Injection
- dependencias se crean en app/main

---

# 🟢 FASE 1 — Setup

```
go mod init monitor
```

```
mkdir -p cmd/server
mkdir -p internal/{app,api,collector,storage,domain,config}
mkdir -p web/static
```

---

# 🟢 FASE 2 — Domain

## internal/domain/metrics.go

```go
package domain

import "time"

type Metrics struct {
    Time  time.Time
    CPU   float64
    RAM   uint64
    Load  float64
    Disk  uint64
}

type DockerStats struct {
    Containers int
    Running    int
}
```

---

# 🟢 FASE 3 — Storage

## Interface

```go
package storage

import "time"
import "monitor/internal/domain"

type Storage interface {
    Save(m domain.Metrics) error
    GetHistory(from, to time.Time) ([]domain.Metrics, error)
}
```

---

## MemoryStore

```go
type MemoryStore struct {
    data []domain.Metrics
    max  int
    mu   sync.RWMutex
}

func (m *MemoryStore) Save(p domain.Metrics) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if len(m.data) >= m.max {
        m.data = m.data[1:]
    }
    m.data = append(m.data, p)
    return nil
}
```

---

## MultiStore

```go
type MultiStore struct {
    stores []Storage
}

func (m *MultiStore) Save(p domain.Metrics) error {
    for _, s := range m.stores {
        go s.Save(p)
    }
    return nil
}
```

---

## InfluxStore (base)

```go
type InfluxStore struct {
    writeAPI api.WriteAPIBlocking
}
```

---

# 🟢 FASE 4 — Collector

## Interface

```go
type Collector interface {
    Collect() domain.Metrics
}
```

---

## Implementación básica

```go
func Collect() domain.Metrics {
    return domain.Metrics{
        Time: time.Now(),
        CPU:  getCPU(),
        RAM:  getRAM(),
        Load: getLoad(),
        Disk: getDisk(),
    }
}
```

---

## Loop

```go
func Start(c Collector, s storage.Storage) {
    for {
        m := c.Collect()
        s.Save(m)
        time.Sleep(time.Second)
    }
}
```

---

# 🟢 FASE 5 — API

```go
func StatsHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(current)
}

func HistoryHandler(w http.ResponseWriter, r *http.Request) {
    data, _ := store.GetHistory(...)
    json.NewEncoder(w).Encode(data)
}
```

---

## Endpoints

```
GET /api/stats
GET /api/history
```

---

# 🟢 FASE 6 — Frontend

## web/static/index.html

- fetch cada 1s
- mostrar métricas
- usar chart.js o similar

---

# 🟢 FASE 7 — Config

## config.json

```json
{
  "use_memory": true,
  "use_influx": true,
  "influx_url": "http://localhost:8086",
  "token": "",
  "bucket": "metrics"
}
```

---

## config.go

```go
type Config struct {
    UseMemory bool
    UseInflux bool
    InfluxURL string
}
```

---

# 🟢 FASE 8 — App (DI)

```go
func Build(cfg Config) {
    var stores []storage.Storage

    if cfg.UseMemory {
        stores = append(stores, NewMemoryStore())
    }

    if cfg.UseInflux {
        stores = append(stores, NewInfluxStore(cfg))
    }

    store := MultiStore{stores: stores}

    collector := NewCollector()

    go Start(collector, store)

    StartAPI(store)
}
```

---

# 🟢 FASE 9 — main.go

```go
func main() {
    cfg := config.Load()
    app.Build(cfg)
}
```

---

# 🧪 FASE 10 — Testing

- test storage
- test collector
- test API

---

# 🚀 FASE 11 — Mejoras

- alertas
- auth
- multi-host
- WebSockets
- retention policy
- agregación (avg/min/max)

---

# ✅ Checklist MVP

- [ ] CPU
- [ ] RAM
- [ ] Load
- [ ] Disk
- [ ] Docker
- [ ] Memory store
- [ ] Influx store
- [ ] API
- [ ] Frontend

---

# 🧠 TL;DR

- Go hace todo
- arquitectura modular
- storage desacoplado
- memory + influx
- escalable sin reescribir
