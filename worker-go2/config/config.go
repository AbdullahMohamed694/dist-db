package config

import (
	"os"

	"github.com/google/uuid"
)

var Cfg *Config

type Config struct {
	ServerPort string
	WorkerName string
	MasterURL  string
	WorkerID   string
	IsMaster   bool
	Standby    bool
	WorkerIP   string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
}

func Load() *Config {
	if Cfg != nil {
		return Cfg
	}
	Cfg = &Config{
		ServerPort: getEnv("WORKER_PORT", "8081"),
		WorkerName: getEnv("WORKER_NAME", "worker-go"),
		MasterURL:  getEnv("MASTER_URL", "http://localhost:8080"),
		WorkerID:   uuid.New().String(),
		IsMaster:   getEnv("IS_MASTER", "false") == "true",
		Standby:    getEnv("STANDBY", "false") == "true",
		WorkerIP:   getEnv("WORKER_IP", "localhost"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "distdb_worker"),
	}
	return Cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}