package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"brokle/internal/config"
	appErrors "brokle/pkg/errors"
)

// OAuthProviderService handles OAuth authentication flows
type OAuthProviderService struct {
	googleConfig *oauth2.Config
	githubConfig *oauth2.Config
	redis        *redis.Client
	frontendURL  string
}

// OAuthUserProfile represents user profile data from OAuth provider
type OAuthUserProfile struct {
	Email      string
	FirstName  string
	LastName   string
	Provider   string
	ProviderID string
}

// NewOAuthProviderService creates a new OAuth provider service
func NewOAuthProviderService(authConfig *config.AuthConfig, redisClient *redis.Client, frontendURL string) *OAuthProviderService {
	googleConfig := &oauth2.Config{
		ClientID:     authConfig.GoogleClientID,
		ClientSecret: authConfig.GoogleClientSecret,
		RedirectURL:  authConfig.GoogleRedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	githubConfig := &oauth2.Config{
		ClientID:     authConfig.GitHubClientID,
		ClientSecret: authConfig.GitHubClientSecret,
		RedirectURL:  authConfig.GitHubRedirectURL,
		Scopes:       []string{"user:email", "read:user"},
		Endpoint:     github.Endpoint,
	}

	return &OAuthProviderService{
		googleConfig: googleConfig,
		githubConfig: githubConfig,
		redis:        redisClient,
		frontendURL:  frontendURL,
	}
}

// GenerateState creates a cryptographically secure state token for CSRF protection
func (s *OAuthProviderService) GenerateState(ctx context.Context, invitationToken *string) (string, error) {
	// Generate random state token
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", appErrors.NewInternalError("Failed to generate state token", err)
	}
	state := hex.EncodeToString(bytes)

	// Store state in Redis with invitation token (if any)
	stateData := map[string]any{
		"created_at": time.Now().Unix(),
	}
	if invitationToken != nil {
		stateData["invitation_token"] = *invitationToken
	}

	data, err := json.Marshal(stateData)
	if err != nil {
		return "", appErrors.NewInternalError("Failed to marshal state data", err)
	}

	key := "oauth:state:" + state
	err = s.redis.Set(ctx, key, data, 5*time.Minute).Err()
	if err != nil {
		return "", appErrors.NewInternalError("Failed to store state token", err)
	}

	return state, nil
}

// ValidateState validates the OAuth state token and returns invitation token if present
func (s *OAuthProviderService) ValidateState(ctx context.Context, state string) (*string, error) {
	key := "oauth:state:" + state

	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, appErrors.NewUnauthorizedError("Invalid or expired state token")
	}

	// Delete state token (one-time use)
	s.redis.Del(ctx, key)

	var stateData map[string]any
	if err := json.Unmarshal([]byte(data), &stateData); err != nil {
		return nil, appErrors.NewInternalError("Failed to unmarshal state data", err)
	}

	// Extract invitation token if present
	var invitationToken *string
	if token, ok := stateData["invitation_token"].(string); ok && token != "" {
		invitationToken = &token
	}

	return invitationToken, nil
}

// GetAuthorizationURL generates OAuth authorization URL for the provider
func (s *OAuthProviderService) GetAuthorizationURL(provider, state string) (string, error) {
	switch provider {
	case "google":
		return s.googleConfig.AuthCodeURL(state, oauth2.AccessTypeOnline), nil
	case "github":
		return s.githubConfig.AuthCodeURL(state, oauth2.AccessTypeOnline), nil
	default:
		return "", appErrors.NewValidationError("Unsupported OAuth provider", provider)
	}
}

// ExchangeCode exchanges authorization code for access token
func (s *OAuthProviderService) ExchangeCode(ctx context.Context, provider, code string) (*oauth2.Token, error) {
	var token *oauth2.Token
	var err error

	switch provider {
	case "google":
		token, err = s.googleConfig.Exchange(ctx, code)
	case "github":
		token, err = s.githubConfig.Exchange(ctx, code)
	default:
		return nil, appErrors.NewValidationError("Unsupported OAuth provider", provider)
	}

	if err != nil {
		return nil, appErrors.NewUnauthorizedError("Failed to exchange authorization code")
	}

	return token, nil
}

// GetUserProfile fetches user profile from OAuth provider
func (s *OAuthProviderService) GetUserProfile(ctx context.Context, provider string, token *oauth2.Token) (*OAuthUserProfile, error) {
	switch provider {
	case "google":
		return s.getGoogleUserProfile(ctx, token)
	case "github":
		return s.getGitHubUserProfile(ctx, token)
	default:
		return nil, appErrors.NewValidationError("Unsupported OAuth provider", provider)
	}
}

// getGoogleUserProfile fetches user profile from Google
func (s *OAuthProviderService) getGoogleUserProfile(ctx context.Context, token *oauth2.Token) (*OAuthUserProfile, error) {
	client := s.googleConfig.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to fetch Google user profile", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to read Google response", err)
	}

	var googleUser struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Name          string `json:"name"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.Unmarshal(body, &googleUser); err != nil {
		return nil, appErrors.NewInternalError("Failed to parse Google user data", err)
	}

	// Only accept verified emails
	if !googleUser.VerifiedEmail {
		return nil, appErrors.NewUnauthorizedError("Email not verified with Google")
	}

	return &OAuthUserProfile{
		Email:      googleUser.Email,
		FirstName:  googleUser.GivenName,
		LastName:   googleUser.FamilyName,
		Provider:   "google",
		ProviderID: googleUser.ID,
	}, nil
}

// getGitHubUserProfile fetches user profile from GitHub
func (s *OAuthProviderService) getGitHubUserProfile(ctx context.Context, token *oauth2.Token) (*OAuthUserProfile, error) {
	client := s.githubConfig.Client(ctx, token)

	// Fetch user profile
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to fetch GitHub user profile", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to read GitHub response", err)
	}

	var githubUser struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
		ID    int    `json:"id"`
	}

	if err := json.Unmarshal(body, &githubUser); err != nil {
		return nil, appErrors.NewInternalError("Failed to parse GitHub user data", err)
	}

	// If email is not public, fetch from emails endpoint
	if githubUser.Email == "" {
		emailResp, err := client.Get("https://api.github.com/user/emails")
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to fetch GitHub emails", err)
		}
		defer emailResp.Body.Close()

		emailBody, err := io.ReadAll(emailResp.Body)
		if err != nil {
			return nil, appErrors.NewInternalError("Failed to read GitHub emails", err)
		}

		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}

		if err := json.Unmarshal(emailBody, &emails); err != nil {
			return nil, appErrors.NewInternalError("Failed to parse GitHub emails", err)
		}

		// Find primary verified email
		for _, email := range emails {
			if email.Primary && email.Verified {
				githubUser.Email = email.Email
				break
			}
		}

		if githubUser.Email == "" {
			return nil, appErrors.NewUnauthorizedError("No verified email found in GitHub account")
		}
	}

	// Parse name into first and last name
	firstName, lastName := parseFullName(githubUser.Name)
	if firstName == "" {
		firstName = githubUser.Login // Fallback to username
	}

	return &OAuthUserProfile{
		Email:      githubUser.Email,
		FirstName:  firstName,
		LastName:   lastName,
		Provider:   "github",
		ProviderID: strconv.Itoa(githubUser.ID),
	}, nil
}

// parseFullName splits a full name into first and last name
func parseFullName(fullName string) (string, string) {
	if fullName == "" {
		return "", ""
	}

	// Simple split on first space
	for i, char := range fullName {
		if char == ' ' {
			return fullName[:i], fullName[i+1:]
		}
	}

	// No space found - use full name as first name
	return fullName, ""
}
