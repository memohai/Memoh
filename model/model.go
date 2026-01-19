package model

import "github.com/memohai/Memoh/model/provider"

type Model struct {
    Provider provider.Provider
    ModelID string
    BaseURL string
    APIKey string
}