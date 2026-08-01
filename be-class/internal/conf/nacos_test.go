package conf

import (
	"fmt"
	"os"
	"testing"
)

func TestInitBootstrap(t *testing.T) {
	if os.Getenv("RUN_NACOS_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_NACOS_INTEGRATION_TESTS=1 to run Nacos integration tests")
	}

	bc := InitBootstrap()
	if bc == nil {
		t.Fatal("初始化失败")
	}
	fmt.Printf("Bootstrap: %+v\n", bc)
}
