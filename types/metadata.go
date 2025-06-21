package types

import (
	"os"

	cmtjson "github.com/cometbft/cometbft/libs/json"
)

type ChainDataMetadata struct {
	ChainID    string   `json:"chain_id"`
	Height     int64    `json:"height"`
	Hash       string   `json:"hash"`
	Time       string   `json:"time"`
	Proposer   string   `json:"proposer"`
	Validators []string `json:"validators"`
}

func (m *ChainDataMetadata) SaveAs(filePath string) error {
	jsonBytes, err := cmtjson.Marshal(m)
	if err != nil {
		return err
	}
	err = os.WriteFile(filePath, jsonBytes, 0600)
	if err != nil {
		return err
	}
	return nil
}
