package openid

import (
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"
)

type UserInfo struct {
	raw              map[string]interface{}
	identifyingClaim string
}

func (uid *UserInfo) UserID() (string, error) {
	if v, ok := uid.raw[uid.identifyingClaim]; ok {
		return fmt.Sprint(v), nil
	}
	return "", fmt.Errorf("identifying claim %q not found in user info", uid.identifyingClaim)
}

type UserInfoCache struct {
	ClientOptions
	cache      map[string]*UserInfo // key=access token
	knownUsers map[string]string    // key=user id, value=access token
	mu         sync.Mutex
	config     *OpenidConfig
	wellKnown  *WellKnownConfiguration
	logger     *zap.SugaredLogger
}

func NewUserInfoCache(
	config *OpenidConfig,
	logger *zap.SugaredLogger,
	opts ...ClientOption,
) (*UserInfoCache, error) {
	options := ClientOptions{
		client: http.DefaultClient,
	}
	options.apply(opts...)

	wellKnown, err := config.GetWellKnownConfiguration()
	if err != nil {
		return nil, err
	}
	if config.IdentifyingClaim == "" {
		return nil, fmt.Errorf("no identifying claim set")
	}
	return &UserInfoCache{
		ClientOptions: options,
		cache:         make(map[string]*UserInfo),
		knownUsers:    make(map[string]string),
		config:        config,
		wellKnown:     wellKnown,
		logger:        logger,
	}, nil
}

func (c *UserInfoCache) Get(accessToken string) (*UserInfo, error) {
	lg := c.logger
	c.mu.Lock()
	defer c.mu.Unlock()
	if info, ok := c.cache[accessToken]; ok {
		return info, nil
	}
	lg.Debug("fetching user info from openid provider")
	rawUserInfo, err := FetchUserInfo(c.wellKnown.UserinfoEndpoint, accessToken,
		WithHTTPClient(c.client),
	)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to fetch user info")
		return nil, err
	}
	info := &UserInfo{
		raw:              rawUserInfo,
		identifyingClaim: c.config.IdentifyingClaim,
	}
	id, err := info.UserID()
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("user info is invalid")
		return nil, err
	}
	if previousAccessToken, ok := c.knownUsers[id]; ok {
		if previousAccessToken != accessToken {
			lg.With(
				info.identifyingClaim, id,
			).Debug("user access token was refreshed")
			c.knownUsers[id] = accessToken
			delete(c.cache, previousAccessToken)
		}
	} else {
		c.knownUsers[id] = accessToken
	}
	c.cache[accessToken] = info
	return info, nil
}
