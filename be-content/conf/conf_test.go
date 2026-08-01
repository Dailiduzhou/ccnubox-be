package conf

import (
	"os"
	"testing"
)

func TestInitInfraConfig(t *testing.T) {
	if os.Getenv("RUN_NACOS_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_NACOS_INTEGRATION_TESTS=1 to run Nacos integration test")
	}
	infra := InitInfraConfig()
	if infra == nil {
		t.Fatal("Failed to init infraConfig")
	}
}

func TestInitTransConfig(t *testing.T) {
	if os.Getenv("RUN_NACOS_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_NACOS_INTEGRATION_TESTS=1 to run Nacos integration test")
	}
	trans := InitServerConf()
	if trans == nil {
		t.Fatal("Failed to init transConfig")
	}
}
