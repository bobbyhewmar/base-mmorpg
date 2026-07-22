package app

import (
	"errors"
	"net/url"
	"strings"
)

type SocialProvider string

const (
	socialProviderGoogle   SocialProvider = "google"
	socialProviderFacebook SocialProvider = "facebook"
)

var (
	errSocialProviderUnsupported = errors.New("social provider unsupported")
	errSocialProviderUnavailable = errors.New("social provider unavailable")
)

type socialAuthBeginResult struct {
	Provider         SocialProvider
	AuthorizationURL string
}

type socialAuthProvider interface {
	Begin() (socialAuthBeginResult, error)
}

type socialAuthService struct {
	providers map[SocialProvider]socialAuthProvider
}

func newSocialAuthService(config SocialAuthConfig) *socialAuthService {
	return &socialAuthService{
		providers: map[SocialProvider]socialAuthProvider{
			socialProviderGoogle: oauthProviderAdapter{
				provider:    socialProviderGoogle,
				authURL:     "https://accounts.google.com/o/oauth2/v2/auth",
				scope:       []string{"openid", "email", "profile"},
				clientID:    config.Google.ClientID,
				redirectURL: config.Google.RedirectURL,
			},
			socialProviderFacebook: oauthProviderAdapter{
				provider:    socialProviderFacebook,
				authURL:     "https://www.facebook.com/v19.0/dialog/oauth",
				scope:       []string{"email", "public_profile"},
				clientID:    config.Facebook.ClientID,
				redirectURL: config.Facebook.RedirectURL,
			},
		},
	}
}

func (service *socialAuthService) Begin(providerRaw string) (socialAuthBeginResult, error) {
	if service == nil {
		return socialAuthBeginResult{}, errSocialProviderUnavailable
	}
	provider := SocialProvider(strings.TrimSpace(strings.ToLower(providerRaw)))
	adapter, ok := service.providers[provider]
	if !ok {
		return socialAuthBeginResult{}, errSocialProviderUnsupported
	}
	return adapter.Begin()
}

type oauthProviderAdapter struct {
	provider    SocialProvider
	authURL     string
	scope       []string
	clientID    string
	redirectURL string
}

func (adapter oauthProviderAdapter) Begin() (socialAuthBeginResult, error) {
	if strings.TrimSpace(adapter.clientID) == "" || strings.TrimSpace(adapter.redirectURL) == "" {
		return socialAuthBeginResult{}, errSocialProviderUnavailable
	}
	authURL, err := url.Parse(adapter.authURL)
	if err != nil {
		return socialAuthBeginResult{}, errSocialProviderUnavailable
	}
	query := authURL.Query()
	query.Set("client_id", adapter.clientID)
	query.Set("redirect_uri", adapter.redirectURL)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(adapter.scope, " "))
	query.Set("state", randomID("social"))
	authURL.RawQuery = query.Encode()
	return socialAuthBeginResult{
		Provider:         adapter.provider,
		AuthorizationURL: authURL.String(),
	}, nil
}
