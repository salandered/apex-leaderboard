package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/salandered/apex/models"
	playerid "github.com/salandered/apex/player_id"
)

var (
	StorageError = errors.New("storage")
	CustomError  = fmt.Errorf("%w.custom_error", StorageError)
	ErrNotFound  = fmt.Errorf("%w.not found", StorageError)
)

type Storage interface {
	PutData(c context.Context, playerData *models.PlayerData) error
	GetData(c context.Context, id playerid.PlayerId) (*models.PlayerData, error)
}

type redisStorage struct {
}

func (rs *redisStorage) PutData(c context.Context, playerData *models.PlayerData) error {
	fmt.Println("will be implemented put data")
	return nil
}

func (rs *redisStorage) GetData(c context.Context, id playerid.PlayerId) (*models.PlayerData, error) {
	fmt.Println("will be implemented get data, return defult for now")
	return &models.PlayerData{}, nil
}

func NewStorage() *redisStorage {
	return &redisStorage{}
}

