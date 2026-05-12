import os
import uuid
import pymysql
import json
import threading
import time
import re
from flask import Flask, jsonify, request, send_file
import requests

app = Flask(__name__)

# ---------- Master flag ----------
is_master = os.environ.get("IS_MASTER", "false") == "true"
if os.path.isfile("master_status"):
    with open("master_status") as f:
        if f.read().strip() == "1":
            is_master = True

# ---------- Configuration ----------
WORKER_PORT = os.environ.get("WORKER_PORT", "8082")
WORKER_NAME = os.environ.get("WORKER_NAME", "worker-python")
MASTER_URL = os.environ.get("MASTER_URL", "http://localhost:8080")
WORKER_ID = str(uuid.uuid4())

DB_HOST = os.environ.get("DB_HOST", "127.0.0.1")
DB_PORT = int(os.environ.get("DB_PORT", "3306"))
DB_USER = os.environ.get("DB_USER", "root")
DB_PASSWORD = os.environ.get("DB_PASSWORD", "")
WORKER_SYSTEM_DB = os.environ.get("DB_NAME", "distdb_worker")

# ---------- Database helpers ----------
def get_db_connection():
    return pymysql.connect(
        host=DB_HOST, port=DB_PORT, user=DB_USER,
        password=DB_PASSWORD, autocommit=True
    )

def ensure_system_db():
    """Create system database and pending_writes table if not exist."""
    conn = get_db_connection()
    cur = conn.cursor()
    cur.execute(f"CREATE DATABASE IF NOT EXISTS {WORKER_SYSTEM_DB}")
    cur.execute(f"""
        CREATE TABLE IF NOT EXISTS {WORKER_SYSTEM_DB}.pending_writes (
            id BIGINT AUTO_INCREMENT PRIMARY KEY,
            query TEXT NOT NULL,
            args JSON,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """)
    # Clear old pending writes from previous runs
    try:
        cur.execute(f"TRUNCATE TABLE {WORKER_SYSTEM_DB}.pending_writes")
        print("Pending writes queue cleared on startup")
    except Exception as e:
        print(f"Warning: could not truncate pending_writes: {e}")

    cur.close()
    conn.close()
    print("Pending writes table ready")

def execute_query(query, args=None):
    """Execute a query on the worker's default connection."""
    conn = get_db_connection()
    cur = conn.cursor()
    cur.execute(query, args or [])
    cur.close()
    conn.close()

def sanitize_identifier(name):
    """Keep only alphanumeric and underscore."""
    return re.sub(r'[^a-zA-Z0-9_]', '', name)

# ---------- Master registration ----------
def register_with_master():
    api_key = os.environ.get("API_KEY", "distdb-default-key")
    worker_ip = os.environ.get('WORKER_IP', 'localhost')
    data = {
        "id": WORKER_ID,
        "name": WORKER_NAME,
        "address": f"http://{worker_ip}:{WORKER_PORT}"
    }
    headers = {"X-API-Key": api_key}
    try:
        resp = requests.post(f"{MASTER_URL}/api/workers/register", json=data, headers=headers, timeout=5)
        if resp.ok:
            print("Registered with master successfully")
        else:
            print(f"Registration failed: {resp.text}")
    except Exception as e:
        print(f"Warning: could not register with master: {e}")

# ---------- Master forwarding + pending queue ----------
def forward_to_master(query, args):
    def _forward():
        print(f"DEBUG: Attempting to forward to master: {query[:80]}")
        body = {"query": query, "args": args}
        api_key = os.environ.get("API_KEY", "distdb-default-key")
        headers = {
            "X-Source-Worker-ID": WORKER_ID,
            "X-API-Key": api_key
        }
        try:
            print(f"DEBUG: POST to {MASTER_URL}/api/replicate")
            resp = requests.post(f"{MASTER_URL}/api/replicate", json=body, headers=headers, timeout=3)
            print(f"DEBUG: Master response status: {resp.status_code}")
            if resp.status_code >= 400:
                raise Exception(f"Master returned {resp.status_code}: {resp.text}")
            else:
                print(f"DEBUG: Successfully forwarded to master")
        except Exception as e:
            print(f"DEBUG: Master unreachable: {e}")

    threading.Thread(target=_forward, daemon=True).start()

def retry_pending_writes():
    """Continuously try to send pending writes to master."""
    while True:
        time.sleep(5)
        try:
            conn = get_db_connection()
            cur = conn.cursor()
            cur.execute(f"SELECT id, query, args FROM {WORKER_SYSTEM_DB}.pending_writes ORDER BY id LIMIT 50")
            rows = cur.fetchall()
            for row in rows:
                pending_id, query, args_json = row
                args = json.loads(args_json) if args_json else []
                body = {"query": query, "args": args}
                try:
                    resp = requests.post(f"{MASTER_URL}/api/replicate", json=body, timeout=3)
                    if resp.status_code < 400:
                        cur.execute(f"DELETE FROM {WORKER_SYSTEM_DB}.pending_writes WHERE id = %s", [pending_id])
                except Exception:
                    pass
            cur.close()
            conn.close()
        except Exception as e:
            print(f"Pending replayer error: {e}")

# ---------- Background master services ----------
def master_health_check():
    """Placeholder: pings workers like the Go master."""
    while True:
        time.sleep(10)
        print("Master health check tick")

def master_replication():
    """Placeholder: replays pending writes to workers."""
    while True:
        time.sleep(10)
        print("Master replication tick")

# ---------- Health endpoint ----------
@app.route('/health')
def health():
    return jsonify({"status": "ok", "node": WORKER_NAME})

# ---------- Existing replication endpoint ----------
@app.route('/api/replicate', methods=['POST'])
def replicate():
    data = request.get_json()
    if not data or 'query' not in data:
        return jsonify({"error": "query required"}), 400

    query = data['query']
    args = data.get('args', [])
    query = query.replace('?', '%s')

    try:
        execute_query(query, args)
        return jsonify({"message": "replicated"})
    except Exception as e:
        print(f"Replication error: {e}")
        return jsonify({"error": str(e)}), 500

# ---------- Dashboard ----------
@app.route('/dashboard')
def serve_dashboard():
    return send_file('../frontend/dashboard.html')

# ---------- CRUD endpoints (open to all nodes) ----------
@app.route('/api/databases/<dbname>/tables/<table>/rows', methods=['POST'])
def insert_row(dbname, table):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    data = request.get_json()
    if not data:
        return jsonify({"error": "JSON required"}), 400

    row_id = str(uuid.uuid4())
    data['id'] = row_id

    columns, placeholders, values = [], [], []
    for col, val in data.items():
        col = sanitize_identifier(col)
        if not col:
            continue
        columns.append(f"`{col}`")
        placeholders.append("%s")
        values.append(val)

    if not columns:
        return jsonify({"error": "no columns"}), 400

    query = f"INSERT INTO `{dbname}`.`{table}` ({','.join(columns)}) VALUES ({','.join(placeholders)})"
    try:
        execute_query(query, values)
    except Exception as e:
        return jsonify({"error": str(e)}), 500

    forward_to_master(query, values)
    return jsonify({"message": "row inserted", "id": row_id}), 201

@app.route('/api/databases/<dbname>/tables/<table>/rows', methods=['GET'])
def select_rows(dbname, table):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    where = request.args.get('where', '')
    query = f"SELECT * FROM `{dbname}`.`{table}`"
    if where:
        query += f" WHERE {where}"

    conn = get_db_connection()
    cur = conn.cursor()
    try:
        cur.execute(query)
        columns = [col[0] for col in cur.description] if cur.description else []
        rows = [dict(zip(columns, row)) for row in cur.fetchall()]
        for row in rows:
            for k, v in row.items():
                if isinstance(v, bytes):
                    row[k] = v.decode('utf-8')
        return jsonify({"rows": rows})
    except Exception as e:
        return jsonify({"error": str(e)}), 500
    finally:
        cur.close()
        conn.close()

@app.route('/api/databases/<dbname>/tables/<table>/rows/<row_id>', methods=['PUT'])
def update_row(dbname, table, row_id):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    data = request.get_json()
    if not data:
        return jsonify({"error": "JSON required"}), 400

    sets, values = [], []
    for col, val in data.items():
        col = sanitize_identifier(col)
        if not col:
            continue
        sets.append(f"`{col}` = %s")
        values.append(val)

    if not sets:
        return jsonify({"error": "no columns"}), 400

    query = f"UPDATE `{dbname}`.`{table}` SET {', '.join(sets)} WHERE `id` = %s"
    values.append(row_id)

    try:
        execute_query(query, values)
    except Exception as e:
        return jsonify({"error": str(e)}), 500

    forward_to_master(query, values)
    return jsonify({"message": "row updated"})

@app.route('/api/databases/<dbname>/tables/<table>/rows/<row_id>', methods=['DELETE'])
def delete_row(dbname, table, row_id):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    query = f"DELETE FROM `{dbname}`.`{table}` WHERE `id` = %s"
    try:
        execute_query(query, [row_id])
    except Exception as e:
        return jsonify({"error": str(e)}), 500

    forward_to_master(query, [row_id])
    return jsonify({"message": "row deleted"})

# ---------- DDL endpoints (databases/tables) ----------
# CREATE DATABASE – allowed on all nodes
@app.route('/api/databases', methods=['POST'])
def create_database():
    data = request.get_json()
    name = sanitize_identifier(data.get('name', ''))
    if not name:
        return jsonify({"error": "name required"}), 400
    query = f"CREATE DATABASE IF NOT EXISTS `{name}`"
    execute_query(query)
    # Forward the DDL to master for replication to other nodes
    forward_to_master(query, [])
    return jsonify({"message": "database created", "name": name})

# LIST DATABASES – allowed on all nodes
@app.route('/api/databases', methods=['GET'])
def list_databases():
    conn = get_db_connection()
    cur = conn.cursor()
    cur.execute("SHOW DATABASES")
    dbs = [row[0] for row in cur.fetchall() if row[0] not in (
        'information_schema', 'mysql', 'performance_schema', 'sys',
        'distdb_system', 'distdb_worker'
    )]
    cur.close()
    conn.close()
    return jsonify({"databases": dbs})

# DROP DATABASE – master ONLY
@app.route('/api/databases/<dbname>', methods=['DELETE'])
def drop_database(dbname):
    if not is_master:
        return jsonify({"error": "only master can drop database"}), 403
    dbname = sanitize_identifier(dbname)
    query = f"DROP DATABASE IF EXISTS `{dbname}`"
    execute_query(query)
    forward_to_master(query, [])
    return jsonify({"message": "database dropped"})

# CREATE TABLE – allowed on all nodes
@app.route('/api/databases/<dbname>/tables', methods=['POST'])
def create_table(dbname):
    dbname = sanitize_identifier(dbname)
    data = request.get_json()
    tname = sanitize_identifier(data.get('name', ''))
    if not tname:
        return jsonify({"error": "table name required"}), 400
    cols = data.get('columns', [])
    col_defs = "`id` CHAR(36) PRIMARY KEY"
    for c in cols:
        cname = sanitize_identifier(c.get('name', ''))
        ctype = c.get('type', '')
        if cname and cname != 'id':
            col_defs += f", `{cname}` {ctype}"
    query = f"CREATE TABLE IF NOT EXISTS `{dbname}`.`{tname}` ({col_defs})"
    execute_query(query)
    forward_to_master(query, [])
    return jsonify({"message": "table created"})

# LIST TABLES – allowed on all nodes
@app.route('/api/databases/<dbname>/tables', methods=['GET'])
def list_tables(dbname):
    dbname = sanitize_identifier(dbname)
    conn = get_db_connection()
    cur = conn.cursor()
    try:
        cur.execute(f"SHOW TABLES FROM `{dbname}`")
        tables = [row[0] for row in cur.fetchall()]
        return jsonify({"database": dbname, "tables": tables})
    except Exception as e:
        return jsonify({"error": str(e)}), 500
    finally:
        cur.close()
        conn.close()

# DROP TABLE – allowed on all nodes
@app.route('/api/databases/<dbname>/tables/<table>', methods=['DELETE'])
def drop_table(dbname, table):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    query = f"DROP TABLE IF EXISTS `{dbname}`.`{table}`"
    execute_query(query)
    forward_to_master(query, [])
    return jsonify({"message": "table dropped"})

# TABLE SCHEMA – allowed on all nodes
@app.route('/api/databases/<dbname>/tables/<table>/schema', methods=['GET'])
def get_table_schema(dbname, table):
    dbname = sanitize_identifier(dbname)
    table = sanitize_identifier(table)
    conn = get_db_connection()
    cur = conn.cursor()
    try:
        cur.execute(f"SHOW COLUMNS FROM `{dbname}`.`{table}`")
        columns = []
        for row in cur.fetchall():
            field, col_type = row[0], row[1]
            if isinstance(field, bytes):
                field = field.decode('utf-8')
            if isinstance(col_type, bytes):
                col_type = col_type.decode('utf-8')
            columns.append({"name": field, "type": col_type})
        return jsonify({"columns": columns})
    except Exception as e:
        return jsonify({"error": str(e)}), 500
    finally:
        cur.close()
        conn.close()

# ---------- Node management ----------
@app.route('/api/node/promote', methods=['POST'])
def promote():
    global is_master
    if is_master:
        return jsonify({"message": "already master"})

    try:
        conn = get_db_connection()
        cur = conn.cursor()
        cur.execute("CREATE DATABASE IF NOT EXISTS distdb_system")
        cur.execute("""
            CREATE TABLE IF NOT EXISTS distdb_system.replication_queue (
                id BIGINT AUTO_INCREMENT PRIMARY KEY,
                query TEXT NOT NULL,
                args JSON,
                source_worker_id VARCHAR(36) NULL,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                retries INT DEFAULT 0,
                last_attempt TIMESTAMP NULL,
                status ENUM('pending','processing','completed','failed') DEFAULT 'pending'
            )
        """)
        cur.execute("""
            CREATE TABLE IF NOT EXISTS distdb_system.worker_nodes (
                id VARCHAR(36) PRIMARY KEY,
                name VARCHAR(100) NOT NULL,
                address VARCHAR(255) NOT NULL,
                status ENUM('up','down','unknown') DEFAULT 'unknown',
                last_heartbeat TIMESTAMP NULL,
                registered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            )
        """)
        conn.close()
    except Exception as e:
        return jsonify({"error": str(e)}), 500

    is_master = True
    with open("master_status", "w") as f:
        f.write("1")

    threading.Thread(target=master_health_check, daemon=True).start()
    threading.Thread(target=master_replication, daemon=True).start()
    print("Worker-Python promoted to master")
    return jsonify({"message": "promoted to master"})

@app.route('/api/node/update-master', methods=['POST'])
def update_master():
    data = request.get_json()
    new_master = data.get('master_url')
    global MASTER_URL
    MASTER_URL = new_master
    threading.Thread(target=register_with_master, daemon=True).start()
    return jsonify({"message": "master updated"})

# ---------- Main ----------
if __name__ == '__main__':
    ensure_system_db()
    register_with_master()
    threading.Thread(target=retry_pending_writes, daemon=True).start()
    print(f"{WORKER_NAME} starting on port {WORKER_PORT}")
    app.run(host='0.0.0.0', port=int(WORKER_PORT))