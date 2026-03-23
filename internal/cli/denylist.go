package cli

import (
	"github.com/christian/crypto-avgr/internal/notknowntokens"
	"github.com/christian/crypto-avgr/internal/portfolio"
)

func loadNotKnownTokens() (path string, skip map[string]struct{}, err error) {
	path = notknowntokens.DefaultPath()
	skip, err = notknowntokens.Load(path)
	return path, skip, err
}

func denylistOptions() (portfolio.DenylistOptions, error) {
	path, skip, err := loadNotKnownTokens()
	if err != nil {
		return portfolio.DenylistOptions{}, err
	}
	return portfolio.DenylistOptions{
		SkipContracts:      skip,
		RecordNotKnownPath: path,
	}, nil
}
