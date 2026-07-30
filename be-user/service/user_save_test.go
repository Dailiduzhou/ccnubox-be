package service

import (
	"context"
	"testing"

	usercrypto "github.com/asynccnu/ccnubox-be/be-user/pkg/crypto"
	"github.com/asynccnu/ccnubox-be/be-user/repository/dao"
	"github.com/asynccnu/ccnubox-be/be-user/repository/model"
)

type saveUserDAO struct {
	user        *model.User
	saveCalls   int
	deleteCalls int
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

func (d *saveUserDAO) Delete(context.Context, string) error {
	d.deleteCalls++
	d.user = nil
	return nil
}

type deleteUserCache struct {
	deleteCalls int
}

func (*deleteUserCache) GetCookie(context.Context, string) (string, error) {
	return "", nil
}

func (*deleteUserCache) SetCookie(context.Context, string, string) error {
	return nil
}

func (*deleteUserCache) GetLibraryToken(context.Context, string, string) (string, error) {
	return "", nil
}

func (*deleteUserCache) SetLibraryToken(context.Context, string, string, string) error {
	return nil
}

func (c *deleteUserCache) DeleteUserData(context.Context, string) error {
	c.deleteCalls++
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

func TestDeleteVerifiesPasswordAndRemovesUserData(t *testing.T) {
	cryptoClient, err := usercrypto.NewCrypto("muxiStudioSecret")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cryptoClient.Encrypt("password")
	if err != nil {
		t.Fatal(err)
	}
	userDAO := &saveUserDAO{user: &model.User{StudentId: "2024000000", Password: encrypted}}
	userCache := &deleteUserCache{}
	service := &userService{dao: userDAO, cache: userCache, cryptoClient: cryptoClient}

	if err := service.Delete(context.Background(), "2024000000", "wrong"); err == nil {
		t.Fatal("Delete() accepted wrong password")
	}
	if userDAO.deleteCalls != 0 {
		t.Fatal("Delete() removed user after wrong password")
	}

	if err := service.Delete(context.Background(), "2024000000", "password"); err != nil {
		t.Fatal(err)
	}
	if userDAO.deleteCalls != 1 {
		t.Fatalf("Delete() DAO calls = %d, want 1", userDAO.deleteCalls)
	}
	if userCache.deleteCalls != 1 {
		t.Fatalf("Delete() cache calls = %d, want 1", userCache.deleteCalls)
	}
}
