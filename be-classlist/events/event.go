package events

import (
	"github.com/asynccnu/ccnubox-be/be-classlist/events/delay"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	delay.NewDelayKafka,
	delay.NewDelayKafkaConfig,
)
