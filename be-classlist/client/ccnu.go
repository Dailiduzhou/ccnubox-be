package client

import (
	"context"
	"fmt"

	"github.com/asynccnu/ccnubox-be/be-classlist/biz"
	userv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/user/v1"
)

type CCNUService struct {
	user userv1.UserServiceClient
}

func NewCCNUService(user userv1.UserServiceClient) biz.CCNUService {
	return &CCNUService{user: user}
}

func (c *CCNUService) GetCookie(ctx context.Context, stuID string) (string, error) {
	resp, err := c.user.GetCookie(ctx, &userv1.GetCookieRequest{
		StudentId: stuID,
	})
	if err != nil {
		return "", fmt.Errorf("get cookie from user service: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("get cookie from user service: %w", biz.ErrCookieUnavailable)
	}
	return resp.Cookie, nil
}
