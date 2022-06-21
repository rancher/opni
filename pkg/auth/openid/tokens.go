package openid

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/lestrrat-go/jwx/jwt/openid"
)

type TokenType string

const (
	Opaque  TokenType = "opaque"
	IDToken TokenType = "id_token"
)

// Claims required for valid ID tokens
var requiredClaims = []string{
	"iss",
	"sub",
	"aud",
	"exp",
	"iat",
}

func GetTokenType(token string) TokenType {
	if isIDToken(token) {
		return IDToken
	}
	return Opaque
}

func isIDToken(token string) bool {
	j, err := jwt.ParseString(token,
		jwt.WithToken(openid.New()),
		jwt.WithValidate(true),
	)
	if err != nil {
		return false
	}
	for _, claim := range requiredClaims {
		if _, ok := j.Get(claim); !ok {
			return false
		}
	}
	return true
}

func ValidateIDToken(token string, keySet jwk.Set) (openid.Token, error) {
	j, err := jwt.ParseString(token,
		jwt.WithKeySet(keySet),
		jwt.WithValidate(true),
		jwt.WithToken(openid.New()),
	)
	if err != nil {
		return nil, err
	}
	return j.(openid.Token), nil
}

func FetchUserInfo(endpoint, token string, opts ...ClientOption) (map[string]interface{}, error) {
	options := &ClientOptions{
		client: http.DefaultClient,
	}
	options.apply(opts...)

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := options.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
