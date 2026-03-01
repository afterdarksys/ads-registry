#!/usr/bin/env python3

import json
import subprocess
import sys
import os
import time
import socket

CONFIG_FILE = "config.json"

def log(msg):
    print(f"[*] {msg}")

def load_config():
    if not os.path.exists(CONFIG_FILE):
        log(f"Error: {CONFIG_FILE} not found. Please create it first.")
        sys.exit(1)
    with open(CONFIG_FILE, 'r') as f:
        return json.load(f)

def check_postgres(dsn):
    # Very basic port checking for postgres (host and port extraction)
    # Extracts "host=127.0.0.1 port=5432" logic loosely
    host = "localhost"
    port = 5432
    if "host=" in dsn:
        host = dsn.split("host=")[1].split(" ")[0]
    if "port=" in dsn:
        try:
            port = int(dsn.split("port=")[1].split(" ")[0])
        except ValueError:
            pass
            
    log(f"Checking PostgreSQL connectivity at {host}:{port}...")
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(3)
    try:
        s.connect((host, port))
        s.close()
        log("PostgreSQL is reachable.")
    except Exception as e:
        log(f"Warning: PostgreSQL is not reachable at {host}:{port}. Error: {e}")

def check_sqlite(path):
    log(f"Checking SQLite database at {path}...")
    parent_dir = os.path.dirname(path)
    if parent_dir and not os.path.exists(parent_dir):
        log(f"Creating parent directory for database: {parent_dir}")
        os.makedirs(parent_dir, exist_ok=True)
    if not os.path.exists(path):
        log(f"SQLite file doesn't exist yet, it will be automatically created by the Go binary.")
    else:
        log("SQLite file found.")

def run_tests():
    log("Running unit tests...")
    result = subprocess.run(["go", "test", "./..."])
    if result.returncode != 0:
        log("Tests failed. Aborting build.")
        sys.exit(1)
    log("Tests passed successfully.")

def build_binary():
    log("Compiling Go binary 'ads-registry'...")
    # CGO_ENABLED=1 is required for go-sqlite3
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    
    result = subprocess.run(["go", "build", "-o", "ads-registry", "./cmd/ads-registry/"], env=env)
    if result.returncode != 0:
        log("Compilation failed.")
        sys.exit(1)
    log("Successfully built 'ads-registry'.")

def main():
    log("Starting ADS Registry Automation Build Process")
    
    cfg = load_config()
    db_cfg = cfg.get("database", {})
    
    # Optional Database Initialization and Checking
    if db_cfg.get("db_check", False):
        driver = db_cfg.get("driver")
        dsn = db_cfg.get("dsn")
        if driver == "sqlite3":
            check_sqlite(dsn)
        elif driver == "postgres" or driver == "pgsqllite":
            check_postgres(dsn)
        else:
            log(f"Unknown driver: {driver}")

    # Testing and Compilation
    run_tests()
    build_binary()
    
    log("Automation suite completed! You can now run './ads-registry serve'")

if __name__ == "__main__":
    main()
