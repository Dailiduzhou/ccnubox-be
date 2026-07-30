package service

import (
	"context"
	"testing"

	usercrypto "github.com/asynccnu/ccnubox-be/be-user/pkg/crypto"
	"github.com/asynccnu/ccnubox-be/be-user/repository/dao"
	"github.com/asynccnu/ccnubox-be/be-user/repository/model"
)

type saveUserDAO struct {
	user      *model.User
	saveCalls int
}

func (d *saveUserDAO) FindByStudentId(context.Context, string) (*model.User, error) {
	if d.user == nil {
		return nil, dao.UserNotFound
	}
	copyOfUser := *d.user
	return &copyOfUser, nil
}

func (d *saveUserDAO) Save(_ context.Context, user *model.User) error {
	d.saveCalls++
	copyOfUser := *user
	d.user = &copyOfUser
	return nil
}

func TestSaveSkipsUnchangedPassword(t *testing.T) {
	cryptoClient, err := usercrypto.NewCrypto("muxiStudioSecret")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cryptoClient.Encrypt("password")
	if err != nil {
		t.Fatal(err)
	}
	userDAO := &saveUserDAO{user: &model.User{StudentId: "2024000000", Password: encrypted}}
	service := &userService{dao: userDAO, cryptoClient: cryptoClient}

	if err := service.Save(context.Background(), "2024000000", "password"); err != nil {
		t.Fatal(err)
	}
	if userDAO.saveCalls != 0 {
		t.Fatalf("Save() wrote unchanged password %d times", userDAO.saveCalls)
	}
}
