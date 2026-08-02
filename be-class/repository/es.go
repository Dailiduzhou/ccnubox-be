package repository

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/asynccnu/ccnubox-be/be-class/conf"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/olivere/elastic/v7"
)

func NewEsClient(infraCfg *conf.InfraConf, serverCfg *conf.ServerConf, l logger.Logger) (*elastic.Client, error) {
	ctx := context.Background()
	if infraCfg == nil || infraCfg.Elasticsearch == nil {
		return nil, fmt.Errorf("Elasticsearch infrastructure configuration is missing")
	}
	if serverCfg == nil || serverCfg.Class == nil || serverCfg.Class.Elasticsearch == nil {
		return nil, fmt.Errorf("Elasticsearch policy configuration is missing")
	}

	cfg := infraCfg.Elasticsearch
	if len(cfg.URLs) == 0 {
		return nil, fmt.Errorf("Elasticsearch URLs are empty")
	}
	policy := serverCfg.Class.Elasticsearch

	// 配置 Elasticsearch 的 URL 和嗅探选项
	urlOpt := elastic.SetURL(cfg.URLs...)
	sniffOpt := elastic.SetSniff(cfg.Sniff)

	// 配置基本认证，使用用户名和密码
	authOpt := elastic.SetBasicAuth(cfg.Username, cfg.Password)

	// 创建 Elasticsearch 客户端
	cli, err := elastic.NewClient(urlOpt, sniffOpt, authOpt)
	if err != nil {
		return nil, fmt.Errorf("connect Elasticsearch: %w", err)
	}

	l.Info("connected to Elasticsearch")

	for name, mapping := range map[string]string{classIndexName: classMapping, freeClassroomIndex: freeClassroomMapping, classroomIndex: classroomMapping} {
		if err := createIndex(ctx, cli, policy.KeepDataAfterRestart, name, mapping, l); err != nil {
			return nil, err
		}
	}

	//存入classroom信息
	err = createInitialClassrooms(cli, l)
	if err != nil {
		l.Error("initialize classrooms in Elasticsearch failed", logger.Error(err))
		return nil, err
	}

	return cli, nil
}

func createIndex(ctx context.Context, cli *elastic.Client, keepData bool, indexName string, mapping string, l logger.Logger) error {
	// 检查索引是否存在
	exist, err := cli.IndexExists(indexName).Do(ctx)
	if err != nil {
		return fmt.Errorf("check Elasticsearch index %s: %w", indexName, err)
	}
	//如果存在,并且要求保留数据,则返回
	if exist && keepData {
		return nil
	}
	//下面是不存在或者不保留数据

	// 如果索引存在，先删除索引
	if exist {
		deleteIndex, err := cli.DeleteIndex(indexName).Do(ctx)
		if err != nil {
			return fmt.Errorf("delete Elasticsearch index %s: %w", indexName, err)
		}
		if !deleteIndex.Acknowledged {
			return fmt.Errorf("delete Elasticsearch index %s was not acknowledged", indexName)
		}
		l.Info("deleted existing Elasticsearch index", logger.String("index", indexName))
	}

	// 创建新的索引
	createIdx, err := cli.CreateIndex(indexName).BodyString(mapping).Do(ctx)
	if err != nil {
		return fmt.Errorf("create Elasticsearch index %s: %w", indexName, err)
	}
	if !createIdx.Acknowledged {
		return fmt.Errorf("create Elasticsearch index %s was not acknowledged", indexName)
	}
	l.Info("created Elasticsearch index", logger.String("index", indexName))
	return nil
}

const (
	classroomIndex   = "ccnubox-classroom"
	classroomMapping = `{
	"mappings": {
		"properties": {
			"where": { "type": "keyword" }
		}
	}
}`
)

// 这里直接在编译阶段就将其加载，防止出现挂载之类的问题
//
//go:embed classrooms.json
var classListBytes []byte

func createInitialClassrooms(cli *elastic.Client, l logger.Logger) error {
	var data struct {
		ClassRooms []string `json:"class_rooms"`
		PruneStale bool     `json:"prune_stale"`
	}

	err := json.Unmarshal(classListBytes, &data)
	if err != nil {
		return err
	}
	if len(data.ClassRooms) == 0 {
		return fmt.Errorf("classrooms.json contains no classrooms")
	}

	classroomValues := make([]interface{}, 0, len(data.ClassRooms))
	for _, classroom := range data.ClassRooms {
		tmp := struct {
			Where string `json:"where"`
		}{
			Where: classroom,
		}
		classroomValues = append(classroomValues, classroom)
		_, err = cli.Index().
			Index(classroomIndex).
			Id(fmt.Sprintf("%v", classroom)).
			BodyJson(tmp).
			Do(context.Background())
		if err != nil {
			l.Error("add classroom to Elasticsearch failed", logger.Any("classroom", tmp), logger.Error(err))
			return err
		}
	}

	var deleted int64
	if data.PruneStale {
		// 删除旧教室具有破坏性，只允许经过人工核对的目录显式开启。
		deleteResponse, err := cli.DeleteByQuery().
			Index(classroomIndex).
			Query(elastic.NewBoolQuery().MustNot(elastic.NewTermsQuery("where", classroomValues...))).
			Conflicts("proceed").
			Do(context.Background())
		if err != nil {
			return fmt.Errorf("failed to delete stale classrooms: %w", err)
		}
		deleted = deleteResponse.Deleted
	} else {
		l.Warn("classroom catalog has not enabled stale cleanup; keeping existing classrooms")
	}
	if _, err = cli.Refresh(classroomIndex).Do(context.Background()); err != nil {
		return fmt.Errorf("failed to refresh classroom index: %w", err)
	}

	l.Infof("保存 %d 个教室成功，删除 %d 个旧教室", len(data.ClassRooms), deleted)
	return nil
}
