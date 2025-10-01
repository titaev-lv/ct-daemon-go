# Trading Daemon Project

## Overview

This project is a Go-based trading daemon with an HTTP launcher for controlling the process.  
It interacts with a database (currently MySQL) for storing user accounts, trading pairs, orders, and trade history.

Key features:  
- Unified main.go as entry point + launcher.  
- Listens HTTP requests on port 8080 for start, stop, restart, status.  
- Saves state in state/state.json to restore daemon state on restart.  
- Modular structure for workers: orderbook, trader, exchange, collector.  
- Supports hot reload of state if container crashes.  
- Logging with levels (INFO, ERROR, DEBUG).  

## File Structure

daemon-go.d/
└── Dockerfile

daemon-go/
├── main.go
├── config/config.conf
├── models/db.go
├── workers/manager.go
├── workers/orderbook.go
├── workers/trader.go
├── workers/exchange.go
├── workers/collector.go
├── state/state.go
└── utils/logger.go

daemon/
└── (PHP sources for daemon-php)

state/
└── (state.json will be stored here)

## Running with Docker Compose

docker-compose up -d --build

### HTTP Actions

Send HTTP requests to http://<daemon_host>:8080/?action=<command>:
- start — start trading daemon  
- stop — stop trading daemon  
- restart — restart trading daemon  
- status — get current daemon state  

Example:

curl "http://localhost:8080/?action=status"

## Updating Go Source Code

1. Modify source files in daemon-go/.  
2. Rebuild container:

docker-compose build ct_daemon_go
docker-compose up -d ct_daemon_go

Or rebuild binary inside container:

docker exec -it ct_daemon_go sh
cd /home/ctdaemon/ctdaemon
go build -o ctdaemon main.go

## Configuration

Configuration is stored in config/config.conf:

[daemon]
poll_interval = 10
http_port = 8080
state_file = /home/ctdaemon/state.json
max_workers = 10

[database]
type = mysql
host = mysql
port = 3306
user = root
password = root
database = trade_db
max_open_connections = 20

[websocket]
ping_interval = 5
reconnect_delay = 3

[logging]
level = INFO
file = /home/ctdaemon/logs/daemon.log
max_size_mb = 100

[trading]
min_order_amount = 0.001
min_order_quote_amount = 1
step_price = 0.0001
step_volume = 0.0001
bbo_only = 1
fin_protection = 1

## Database Migration

Project is designed for multiple DBs:
- Currently MySQL.  
- Can switch to PostgreSQL by changing type = postgresql and driver in models/db.go.  
- Queries might need minor adjustments.
