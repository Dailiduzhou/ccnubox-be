package grpc

import (
	"context"

	v1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/grade/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 保留旧 RPC 协议，不再抓取或预加载排名。
func (s *GradeServiceServer) GetRankByTerm(ctx context.Context, req *v1.GetRankByTermReq) (*v1.GetRankByTermResp, error) {
	return nil, status.Error(codes.Unimplemented, "排名功能已下线")
}

func (s *GradeServiceServer) LoadRank(ctx context.Context, req *v1.LoadRankReq) (*v1.EmptyResp, error) {
	return nil, status.Error(codes.Unimplemented, "排名功能已下线")
}
