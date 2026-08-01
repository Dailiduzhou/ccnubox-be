package data

import (
	"os"
	"strings"
	"testing"
)

func TestKafkaConfigAuthentication(t *testing.T) {
	t.Run("anonymous", func(t *testing.T) {
		if initProducerConfig("", "").Net.SASL.Enable {
			t.Fatal("producer enabled SASL without credentials")
		}
		if initConsumerConfig("", "").Net.SASL.Enable {
			t.Fatal("consumer enabled SASL without credentials")
		}
	})

	t.Run("credentials", func(t *testing.T) {
		if !initProducerConfig("user", "password").Net.SASL.Enable {
			t.Fatal("producer did not enable SASL with credentials")
		}
		if !initConsumerConfig("user", "password").Net.SASL.Enable {
			t.Fatal("consumer did not enable SASL with credentials")
		}
	})
}

func TestKafkaBuildersRejectEmptyBrokers(t *testing.T) {
	if _, err := (KafkaProducerBuilder{}).Build(); err == nil {
		t.Fatal("producer accepted empty broker list")
	}
	if _, err := (KafkaConsumerBuilder{}).Build("test"); err == nil {
		t.Fatal("consumer accepted empty broker list")
	}
}

func TestKafkaBuildersIntegration(t *testing.T) {
	if os.Getenv("RUN_KAFKA_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_KAFKA_INTEGRATION_TESTS=1 to run Kafka integration test")
	}
	brokers := strings.Split(os.Getenv("KAFKA_TEST_BROKERS"), ",")
	username := os.Getenv("KAFKA_TEST_USERNAME")
	password := os.Getenv("KAFKA_TEST_PASSWORD")

	producer, err := (KafkaProducerBuilder{
		brokers: brokers, username: username, password: password,
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	consumer, err := (KafkaConsumerBuilder{
		brokers: brokers, username: username, password: password,
	}).Build("ccnubox-test")
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
}
