package conf

import (
	"fmt"
	"os"
	"testing"
)

func TestInitInfraConfig(t *testing.T) {
	requireNacosIntegration(t)
	infra := InitInfraConfig()
	if infra == nil {
		t.Fatal("Failed to init infraConfig")
	}

	fmt.Printf("InitInfraConfig: %+v\n", infra)
}

func TestInitTransConfig(t *testing.T) {
	requireNacosIntegration(t)
	trans := InitServerConf()
	if trans == nil {
		t.Fatal("Failed to init transConfig")
	}

	fmt.Printf("InitServerConf: %+v\n", trans)
}

func requireNacosIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_NACOS_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_NACOS_INTEGRATION_TESTS=1 to run Nacos integration tests")
	}
}
