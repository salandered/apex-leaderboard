package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
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
	client *redis.Client
}

func (rs *redisStorage) PutData(ctx context.Context, playerData *models.PlayerData) error {
	// rs.client.ZAdd()
	fmt.Println("will be implemented put data")
	return nil
}

func (rs *redisStorage) GetData(ctx context.Context, id playerid.PlayerId) (*models.PlayerData, error) {
	fmt.Println("will be implemented get data, return defult for now")
	return &models.PlayerData{}, nil
}

func NewStorage() *redisStorage {
	return &redisStorage{
		client: redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // no password
			DB:       0,  // use default DB
		}),
	}
}
