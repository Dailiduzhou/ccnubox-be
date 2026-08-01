package user

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/asynccnu/ccnubox-be/bff/web/ijwt"
	userv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/user/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type deleteAccountJWT struct {
	ijwt.Handler
	clearErr error
	calls    *[]string
}

func (h deleteAccountJWT) ClearToken(*gin.Context) error {
	*h.calls = append(*h.calls, "clear-token")
	return h.clearErr
}

type deleteAccountUserClient struct {
	userv1.UserServiceClient
	checkResp *userv1.CheckUserResp
	checkErr  error
	deleteErr error
	calls     *[]string
}

func (c deleteAccountUserClient) CheckUser(
	context.Context,
	*userv1.CheckUserReq,
	...grpc.CallOption,
) (*userv1.CheckUserResp, error) {
	*c.calls = append(*c.calls, "check")
	return c.checkResp, c.checkErr
}

func (c deleteAccountUserClient) DeleteUser(
	context.Context,
	*userv1.DeleteUserReq,
	...grpc.CallOption,
) (*userv1.DeleteUserResp, error) {
	*c.calls = append(*c.calls, "delete")
	return &userv1.DeleteUserResp{}, c.deleteErr
}

func TestDeleteAccountOperationOrder(t *testing.T) {
	tests := []struct {
		name      string
		checkResp *userv1.CheckUserResp
		checkErr  error
		clearErr  error
		deleteErr error
		wantCalls []string
		wantErr   bool
	}{
		{
			name:      "success",
			checkResp: &userv1.CheckUserResp{Success: true},
			wantCalls: []string{"check", "clear-token", "delete"},
		},
		{
			name:      "incorrect password",
			checkErr:  userv1.ErrorIncorrectPasswordError("incorrect password"),
			wantCalls: []string{"check"},
			wantErr:   true,
		},
		{
			name:      "token cleanup failure keeps account",
			checkResp: &userv1.CheckUserResp{Success: true},
			clearErr:  errors.New("redis unavailable"),
			wantCalls: []string{"check", "clear-token"},
			wantErr:   true,
		},
		{
			name:      "delete failure happens after token cleanup",
			checkResp: &userv1.CheckUserResp{Success: true},
			deleteErr: errors.New("database unavailable"),
			wantCalls: []string{"check", "clear-token", "delete"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []string{}
			handler := NewUserHandler(
				deleteAccountJWT{clearErr: tt.clearErr, calls: &calls},
				deleteAccountUserClient{
					checkResp: tt.checkResp,
					checkErr:  tt.checkErr,
					deleteErr: tt.deleteErr,
					calls:     &calls,
				},
				nil,
			)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, err := handler.DeleteAccount(
				ctx,
				DeleteAccountReq{Password: "password"},
				ijwt.UserClaims{StudentId: "2024000000"},
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteAccount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Fatalf("DeleteAccount() calls = %v, want %v", calls, tt.wantCalls)
			}
		})
	}
}
