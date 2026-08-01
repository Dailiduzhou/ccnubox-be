//go:build wireinject

package main

import (
	"github.com/asynccnu/ccnubox-be/be-classlist/biz"
	"github.com/asynccnu/ccnubox-be/be-classlist/biz/usecase"
	"github.com/asynccnu/ccnubox-be/be-classlist/client"
	"github.com/asynccnu/ccnubox-be/be-classlist/conf"
	"github.com/asynccnu/ccnubox-be/be-classlist/crawler"
	"github.com/asynccnu/ccnubox-be/be-classlist/events"
	"github.com/asynccnu/ccnubox-be/be-classlist/grpc"
	"github.com/asynccnu/ccnubox-be/be-classlist/ioc"
	repo "github.com/asynccnu/ccnubox-be/be-classlist/repository"
	"github.com/asynccnu/ccnubox-be/be-classlist/repository/cache"
	"github.com/asynccnu/ccnubox-be/be-classlist/repository/dao"
	"github.com/asynccnu/ccnubox-be/be-classlist/service"
	"github.com/google/wire"
)

func InitApp() (*App, func(), error) {
	wire.Build(
		NewApp,
		usecase.ProviderSet,
		conf.ProviderSet,
		crawler.ProviderSet,
		events.ProviderSet,
		grpc.ProviderSet,
		ioc.ProviderSet,
		cache.ProviderSet,
		dao.ProviderSet,
		repo.ProviderSet,
		service.ProviderSet,
		client.ProviderSet,
		wire.Bind(new(biz.ClassCrawler), new(*crawler.Crawler3)),
	)
	return nil, nil, nil
}
