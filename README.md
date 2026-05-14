# DistDB – Distributed Database System

A distributed database system built with **Go (Golang)**, **Python (Flask)**, and **MySQL**, designed to run on multiple real machines over a local network.

## Architecture

┌─────────────────────────────┐
│ Dashboard │
│ (HTML/CSS/JavaScript) │
└──────────────┬──────────────┘
│ REST API
┌──────────────▼──────────────┐
│ Master Node (Go) │
│ - API Gateway │
│ - Query coordination │
│ - Replication manager │
│ - Health monitoring │
│ - Worker registry │
└──┬──────────┬───────────┬───┘
│ │ │
┌────────▼──┐ ┌────▼────┐ ┌───▼────────┐
│ Worker 1 │ │Worker 2 │ │ Worker 3 │
│ (Go) │       │ (Go) │    │ (Python) │
│ Port 8081 │ │Port 8083│ │ Port 8082 │
└───────────┘ └─────────┘ └────────────┘



## Features

### Core Features
- **Master‑Worker Architecture** – One master coordinates multiple workers
- **CRUD Operations** – INSERT, SELECT, UPDATE, DELETE on all nodes
- **Replication** – Statement‑based asynchronous replication with retry queues
- **Fault Tolerance** – System continues working if any worker goes down
- **Automatic Failover** – A standby worker automatically becomes master if the original master fails
- **Health Monitoring** – Master pings workers every second, detects failures instantly
- **Web Dashboard** – Full CRUD interface with worker status, database/table management
- **API Key Authentication** – All inter‑node requests verified with shared secret
- **DDL Management** – CREATE/DROP DATABASE, CREATE/DROP TABLE
- **UUID Records** – All records use UUID primary keys for distributed consistency

### Dashboard Features
- Worker status (up/down) with real‑time updates
- Create/Drop databases and tables
- Insert, view, edit, delete rows
- Promote worker to master (manual failover)
- Refresh button for manual updates

## Technology Stack

| Component | Technology |
|-----------|------------|
| Master Node | Go + Gin framework |
| Worker 1 | Go + Gin framework |
| Worker 2 | Go + Gin framework |
| Worker 3 | Python + Flask |
| Database | MySQL (XAMPP) |
| Communication | JSON over HTTP REST APIs |
| Frontend | HTML/CSS/JavaScript + Bootstrap 5 |
| Authentication | API Key via X-API-Key header |

## Project Structure

dist-db/
├── README.md
├── master/
│ ├── main.go # Entry point, route setup
│ ├── config/config.go # Environment‑based configuration
│ ├── db/db.go # MySQL connection, system tables
│ ├── api/
│ │ ├── workers.go # Worker registration & listing
│ │ ├── databases.go # CREATE/LIST/DROP database handlers
│ │ ├── tables.go # CREATE/LIST/DROP table handlers
│ │ ├── rows.go # CRUD row handlers
│ │ └── middleware.go # API key authentication middleware
│ ├── health/
│ │ └── checker.go # Worker health monitoring
│ └── replication/
│ └── manager.go # Replication queue processor
├── worker-go/
│ ├── main.go # Entry point, route setup
│ ├── config/config.go # Worker configuration
│ ├── db/db.go # MySQL connection, pending tables
│ ├── api/
│ │ ├── rows.go # CRUD handlers
│ │ ├── helpers.go # Forwarding & sanitization
│ │ ├── master_routes.go # Master‑only DDL endpoints
│ │ └── node.go # Promote & update‑master handlers
│ └── health/
│ └── autofailover.go # Automatic failover logic
├── worker-python/
│ ├── app.py # Full Python worker (Flask)
│ └── requirements.txt # Flask, pymysql, requests
├── shared/
│ └── models.go # Shared request/response types
└── frontend/
└── dashboard.html # Web dashboard


## Getting Started

### Prerequisites
- **Go** 1.21+
- **Python** 3.10+
- **MySQL** (XAMPP recommended)
- Each machine needs its own MySQL instance

### Installation

1. **Clone the project** on all machines:
   ```bash
   git clone <repository-url>
   cd dist-db

2. Install Go dependencies (on master & Go workers):
    cd master && go mod tidy
    cd ../worker-go && go mod tidy

3. Install Python dependencies (on Python worker):

    cd worker-python
    python -m venv .venv
    .\.venv\Scripts\Activate.ps1   # Windows
    # source .venv/bin/activate    # Linux/Mac
    pip install -r requirements.txt

Starting the System

1. Start the Master
    cd master
    $env:API_KEY="distdb-secret-key-2026"
    go run main.go

2. Start Worker‑Go (Worker 1)

    cd worker-go
    $env:WORKER_PORT="8081"
    $env:WORKER_NAME="worker-go"
    $env:MASTER_URL="http://localhost:8080"
    $env:WORKER_IP="localhost"
    $env:API_KEY="distdb-secret-key-2026"
    $env:STANDBY="true"               # Enable automatic failover
    $env:IS_MASTER="false"
    go run main.go

3. Start Worker‑Python (Worker 3)

    cd worker-python
    $env:WORKER_PORT="8082"
    $env:WORKER_NAME="worker-python"
    $env:MASTER_URL="http://localhost:8080"
    $env:WORKER_IP="localhost"
    $env:API_KEY="distdb-secret-key-2026"
    $env:IS_MASTER="false"
    python app.py

4. Start Worker‑Go2 (Worker 2)

    cd worker-go2
    $env:WORKER_PORT="8083"
    $env:WORKER_NAME="worker-go2"
    $env:MASTER_URL="http://localhost:8080"
    $env:WORKER_IP="localhost"
    $env:API_KEY="distdb-secret-key-2026"
    $env:IS_MASTER="false"
    go run main.go

Accessing the Dashboard
    Node	URL
    Master	        http://localhost:8080/dashboard
    Worker‑Go	    http://localhost:8081/dashboard
    Worker‑Python	http://localhost:8082/dashboard
    Worker‑Go2	    http://localhost:8083/dashboard

Environment Variables
Variable	Description	Default
API_KEY	Shared secret for inter‑node authentication	distdb-secret-key-2026
MASTER_PORT	Master server port	8080
WORKER_PORT	Worker server port	8081 / 8082
WORKER_NAME	Human‑readable worker name	worker-go / worker-python
MASTER_URL	URL of the master node	http://localhost:8080
WORKER_IP	IP address for self‑registration	localhost
DB_HOST	MySQL host	127.0.0.1
DB_USER	MySQL user	root
DB_PASSWORD	MySQL password	(empty)
DB_NAME	System database name	distdb_system / distdb_worker
IS_MASTER	Start node as master	false
STANDBY	Enable automatic failover	false

How It Works
Data Flow
Client sends a request to the Master.

Master processes the request, executes it on its own MySQL.

Master stores the SQL in the replication_queue table.

The replication manager picks up pending entries and sends them to all healthy workers.

Workers execute the SQL on their own MySQL, keeping data in sync.

Fault Tolerance
Worker down → Master skips it, continues with other workers. Pending writes are retried when the worker comes back.

Master down → A standby worker (with STANDBY=true) automatically promotes itself to master after 15 seconds.

Master returns → The promoted worker detects the original master and demotes itself back to worker.

Replication
Statement‑based replication.

Persistent queue in distdb_system.replication_queue.

Failed writes are retried every 2 seconds.

Queue auto‑cleared on master restart to prevent ghost data.

Health Monitoring
Master pings every worker's /health endpoint every second.

Uses 500ms timeout for fast detection.

Updates worker status in worker_nodes table.

Dashboard shows real‑time status.

Master Endpoints
Method	Path	Description
GET	/health	Master health check
GET	/api/workers	List registered workers
POST	/api/workers/register	Register a worker
POST	/api/databases	Create database
GET	/api/databases	List databases
DELETE	/api/databases/:db	Drop database
POST	/api/databases/:db/tables	Create table
GET	/api/databases/:db/tables	List tables
DELETE	/api/databases/:db/tables/:tbl	Drop table
POST	/api/databases/:db/tables/:tbl/rows	Insert row
GET	/api/databases/:db/tables/:tbl/rows	Select rows
PUT	/api/databases/:db/tables/:tbl/rows/:id	Update row
DELETE	/api/databases/:db/tables/:tbl/rows/:id	Delete row

Worker Endpoints
Method	Path	Description
GET	/health	Worker health check
POST	/api/replicate	Execute replicated SQL
POST	/api/node/promote	Promote to master
POST	/api/node/update-master	Update master URL

Deployment on Multiple Machines
To run on separate machines:

Set static IPs for each machine.

Update WORKER_IP on each worker to its actual IP.

Update MASTER_URL on each worker to the master's IP.

Open firewall ports 8080, 8081, 8082, 8083.

Set API_KEY identically on all machines.

License

    This project is built for educational purposes as part of a distributed systems learning exercise.

Contributors

    Distributed Database System – Built with Go, Python, and MySQL

