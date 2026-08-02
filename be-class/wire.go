//go:build wireinject

package main

import (
	"github.com/asynccnu/ccnubox-be/be-class/biz/usecase"
	"github.com/asynccnu/ccnubox-be/be-class/client"
	"github.com/asynccnu/ccnubox-be/be-class/conf"
	"github.com/asynccnu/ccnubox-be/be-class/cron"
	classgrpc "github.com/asynccnu/ccnubox-be/be-class/grpc"
	classhttp "github.com/asynccnu/ccnubox-be/be-class/http"
	"github.com/asynccnu/ccnubox-be/be-class/ioc"
	"github.com/asynccnu/ccnubox-be/be-class/repository"
	"github.com/asynccnu/ccnubox-be/be-class/repository/lock"
	"github.com/asynccnu/ccnubox-be/be-class/service"
	"github.com/google/wire"
)

func InitApp() (*App, func(), error) {
	wire.Build(
		NewApp,
		conf.ProviderSet,
		ioc.ProviderSet,
		client.ProviderSet,
		repository.ProviderSet,
		lock.ProviderSet,
		usecase.ProviderSet,
		service.ProviderSet,
		classgrpc.ProviderSet,
		classhttp.ProviderSet,
		cron.ProviderSet,
		wire.Bind(new(usecase.EsProxy), new(*repository.ClassData)),
		wire.Bind(new(usecase.ClassListService), new(*client.ClassListService)),
		wire.Bind(new(usecase.FreeClassRoomData), new(*repository.FreeClassroomData)),
		wire.Bind(new(usecase.ClassData), new(*repository.ClassData)),
		wire.Bind(new(usecase.CookieClient), new(*client.UserService)),
		wire.Bind(new(usecase.Cache), new(*repository.Cache)),
		wire.Bind(new(classhttp.FreeClassRoomSaver), new(*usecase.FreeClassroomBiz)),
		wire.Bind(new(classhttp.ClassroomJSONProvider), new(*repository.ClassroomJSONData)),
		wire.Bind(new(service.ClassroomJSONProvider), new(*repository.ClassroomJSONData)),
	)
	return nil, nil, nil
}
