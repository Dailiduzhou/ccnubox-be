package ioc

import (
	"github.com/IBM/sarama"
	"github.com/asynccnu/ccnubox-be/be-classlist/conf"
	"github.com/asynccnu/ccnubox-be/be-classlist/events/consumer"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/infra"
)

func InitKafka(cfg *conf.InfraConf) sarama.Client {
	return infra.InitKafka(cfg.Kafka)
}

func InitKafkaConsumerClientFactory(cfg *conf.InfraConf) consumer.ClientFactory {
	return func() sarama.Client {
		return infra.InitKafka(cfg.Kafka)
	}
}
