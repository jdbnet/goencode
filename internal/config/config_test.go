package config

import (
	"testing"
)

func TestListenAddrAndPortAliases(t *testing.T) {
	t.Setenv("GOENCODE_LISTEN_ADDR", "")
	t.Setenv("GOENCODE_PORT", "")
	t.Setenv("GOENCODE_SERVER_LISTEN", "127.0.0.1")
	t.Setenv("GOENCODE_SERVER_PORT", "9090")

	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ListenAddr != "127.0.0.1" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1", cfg.Server.ListenAddr)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestCanonicalListenOverridesAlias(t *testing.T) {
	t.Setenv("GOENCODE_LISTEN_ADDR", "10.0.0.1")
	t.Setenv("GOENCODE_SERVER_LISTEN", "127.0.0.1")
	t.Setenv("GOENCODE_PORT", "8081")
	t.Setenv("GOENCODE_SERVER_PORT", "9090")

	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ListenAddr != "10.0.0.1" {
		t.Fatalf("ListenAddr = %q, want 10.0.0.1", cfg.Server.ListenAddr)
	}
	if cfg.Server.Port != 8081 {
		t.Fatalf("Port = %d, want 8081", cfg.Server.Port)
	}
}

func TestEncoderWorkersEnvAndClamp(t *testing.T) {
	t.Setenv("GOENCODE_ENCODER_WORKERS", "4")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != 4 {
		t.Fatalf("Workers = %d, want 4", cfg.Encoder.Workers)
	}

	t.Setenv("GOENCODE_ENCODER_WORKERS", "99")
	cfg, err = LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != maxEncoderWorkers {
		t.Fatalf("Workers = %d, want %d", cfg.Encoder.Workers, maxEncoderWorkers)
	}
}

func TestEncoderWorkersDefault(t *testing.T) {
	t.Setenv("GOENCODE_ENCODER_WORKERS", "")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != 1 {
		t.Fatalf("Workers = %d, want 1", cfg.Encoder.Workers)
	}
	if cfg.Encoder.MinFreeGB != 5 {
		t.Fatalf("MinFreeGB = %d, want 5", cfg.Encoder.MinFreeGB)
	}
}
