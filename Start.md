$env:WORKER_PORT="8081"
$env:WORKER_NAME="worker-go"
$env:MASTER_URL="http://192.168.1.10:8080"
$env:WORKER_IP="192.168.1.11"
$env:DB_HOST="127.0.0.1"
$env:DB_PORT="3306"
$env:DB_USER="root"
$env:DB_PASSWORD="your_password"
$env:DB_NAME="distdb_worker"
$env:IS_MASTER="false"
$env:STANDBY="false"

mysql -u root

-- On the worker’s MySQL (each worker has its own)
USE distdb_worker;
TRUNCATE TABLE pending_writes;

