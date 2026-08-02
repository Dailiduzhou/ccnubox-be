package repository

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewEsClient,
	NewClassData,
	NewFreeClassroomData,
	NewClassroomJSONData,
	NewCultivateStrategyData,
	NewCache,
)
