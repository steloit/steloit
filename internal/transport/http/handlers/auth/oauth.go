package auth

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/response"
)

// getFrontendURL returns the frontend URL from environment or default
func getFrontendURL() string {
	if url := os.Getenv("NEXT_PUBLIC_APP_URL"); url != "" {
		return url
	}
	return "http://localhost:3000"
}

// InitiateGoogleOAuth redirects user to Google OAuth consent screen
// @Summary Initiate Google OAuth
// @Description Start Google OAuth authentication flow
// @Tags Authentication
// @Param invitation_token query string false "Invitation token (if joining via invite)"
// @Success 302 {string} string "Redirect to Google"
// @Router /api/v1/auth/google [get]
func (h *Handler) InitiateGoogleOAuth(c *gin.Context) {
	invitationToken := c.Query("invitation_token")
	var invitePtr *string
	if invitationToken != "" {
		invitePtr = &invitationToken
	}

	// Generate CSRF state token
	state, err := h.oauthProvider.GenerateState(c.Request.Context(), invitePtr)
	if err != nil {
		h.logger.Error("Failed to generate OAuth state", "error", err)
		response.Error(c, err)
		return
	}

	// Get authorization URL
	authURL, err := h.oauthProvider.GetAuthorizationURL("google", state)
	if err != nil {
		h.logger.Error("Failed to get Google authorization URL", "error", err)
		response.Error(c, err)
		return
	}

	// Redirect to Google
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// GoogleCallback handles Google OAuth callback
// @Summary Google OAuth callback
// @Description Handle Google OAuth callback and create session
// @Tags Authentication
// @Param code query string true "Authorization code from Google"
// @Param state query string true "CSRF state token"
// @Success 302 {string} string "Redirect to frontend"
// @Router /api/v1/auth/google/callback [get]
func (h *Handler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		h.logger.Error("Missing code or state in Google callback")
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=oauth_failed")
		return
	}

	// Validate state token (CSRF protection)
	invitationToken, err := h.oauthProvider.ValidateState(c.Request.Context(), state)
	if err != nil {
		h.logger.Error("Invalid OAuth state token", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=invalid_state")
		return
	}

	// Exchange code for token
	token, err := h.oauthProvider.ExchangeCode(c.Request.Context(), "google", code)
	if err != nil {
		h.logger.Error("Failed to exchange Google code", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=token_exchange_failed")
		return
	}

	// Get user profile from Google
	userProfile, err := h.oauthProvider.GetUserProfile(c.Request.Context(), "google", token)
	if err != nil {
		h.logger.Error("Failed to fetch Google user profile", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=profile_fetch_failed")
		return
	}

	// Check if user already exists
	existingUser, err := h.userService.GetUserByEmail(c.Request.Context(), userProfile.Email)
	if err == nil && existingUser != nil {
		// SECURITY CHECK 1: Verify auth method is OAuth (prevent password account takeover)
		if existingUser.AuthMethod != "oauth" {
			h.logger.Warn("Password account attempted OAuth login - blocked for security", "email", userProfile.Email, "auth_method", existingUser.AuthMethod)

			c.Redirect(http.StatusTemporaryRedirect,
				getFrontendURL()+"/auth/signin?error=account_exists_use_password")
			return
		}

		// SECURITY CHECK 2: Verify OAuth provider matches
		if existingUser.OAuthProvider == nil || *existingUser.OAuthProvider != "google" {
			existingProviderName := "a different provider"
			if existingUser.OAuthProvider != nil {
				existingProviderName = *existingUser.OAuthProvider
			}

			h.logger.Warn("OAuth account with wrong provider", "email", userProfile.Email, "stored_provider", existingProviderName, "attempted_provider", "google")

			c.Redirect(http.StatusTemporaryRedirect,
				fmt.Sprintf("%s/auth/signin?error=use_%s", getFrontendURL(), existingProviderName))
			return
		}

		// SECURITY CHECK 3: Verify provider ID matches (prevents account takeover)
		if existingUser.OAuthProviderID != nil && *existingUser.OAuthProviderID != userProfile.ProviderID {
			h.logger.Error("OAuth provider ID mismatch - possible account takeover attempt", "email", userProfile.Email, "provider", "google")

			c.Redirect(http.StatusTemporaryRedirect,
				getFrontendURL()+"/auth/signin?error=authentication_failed")
			return
		}

		// All security checks passed - safe to auto-login
		h.logger.Info("Existing OAuth user logging in", "email", userProfile.Email, "provider", "google", "user_id", existingUser.ID)

		// Generate login tokens
		loginTokens, err := h.authService.GenerateTokensForUser(c.Request.Context(), existingUser.ID)
		if err != nil {
			h.logger.Error("Failed to generate login tokens for existing user", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=login_failed")
			return
		}

		// Create one-time login session to securely pass tokens to frontend
		loginSessionID, err := h.authService.CreateLoginTokenSession(
			c.Request.Context(),
			loginTokens.AccessToken,
			loginTokens.RefreshToken,
			loginTokens.ExpiresIn,
			existingUser.ID,
		)
		if err != nil {
			h.logger.Error("Failed to create login session", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=session_failed")
			return
		}

		// Redirect to frontend token exchange page
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/callback?session=%s&type=login", getFrontendURL(), loginSessionID))
		return
	}

	// New user - create OAuth session for Step 2
	session := &authDomain.OAuthSession{
		Email:           userProfile.Email,
		FirstName:       userProfile.FirstName,
		LastName:        userProfile.LastName,
		Provider:        userProfile.Provider,
		ProviderID:      userProfile.ProviderID,
		InvitationToken: invitationToken,
	}

	sessionID, err := h.authService.CreateOAuthSession(c.Request.Context(), session)
	if err != nil {
		h.logger.Error("Failed to create OAuth session", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=session_creation_failed")
		return
	}

	h.logger.Info("OAuth session created, redirecting to Step 2", "email", userProfile.Email, "provider", "google", "session_id", sessionID)

	// Redirect to frontend Step 2 (personalization)
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/signup?session=%s", getFrontendURL(), sessionID))
}

// InitiateGitHubOAuth redirects user to GitHub OAuth consent screen
// @Summary Initiate GitHub OAuth
// @Description Start GitHub OAuth authentication flow
// @Tags Authentication
// @Param invitation_token query string false "Invitation token (if joining via invite)"
// @Success 302 {string} string "Redirect to GitHub"
// @Router /api/v1/auth/github [get]
func (h *Handler) InitiateGitHubOAuth(c *gin.Context) {
	invitationToken := c.Query("invitation_token")
	var invitePtr *string
	if invitationToken != "" {
		invitePtr = &invitationToken
	}

	// Generate CSRF state token
	state, err := h.oauthProvider.GenerateState(c.Request.Context(), invitePtr)
	if err != nil {
		h.logger.Error("Failed to generate OAuth state", "error", err)
		response.Error(c, err)
		return
	}

	// Get authorization URL
	authURL, err := h.oauthProvider.GetAuthorizationURL("github", state)
	if err != nil {
		h.logger.Error("Failed to get GitHub authorization URL", "error", err)
		response.Error(c, err)
		return
	}

	// Redirect to GitHub
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// GitHubCallback handles GitHub OAuth callback
// @Summary GitHub OAuth callback
// @Description Handle GitHub OAuth callback and create session
// @Tags Authentication
// @Param code query string true "Authorization code from GitHub"
// @Param state query string true "CSRF state token"
// @Success 302 {string} string "Redirect to frontend"
// @Router /api/v1/auth/github/callback [get]
func (h *Handler) GitHubCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		h.logger.Error("Missing code or state in GitHub callback")
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=oauth_failed")
		return
	}

	// Validate state token (CSRF protection)
	invitationToken, err := h.oauthProvider.ValidateState(c.Request.Context(), state)
	if err != nil {
		h.logger.Error("Invalid OAuth state token", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=invalid_state")
		return
	}

	// Exchange code for token
	token, err := h.oauthProvider.ExchangeCode(c.Request.Context(), "github", code)
	if err != nil {
		h.logger.Error("Failed to exchange GitHub code", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=token_exchange_failed")
		return
	}

	// Get user profile from GitHub
	userProfile, err := h.oauthProvider.GetUserProfile(c.Request.Context(), "github", token)
	if err != nil {
		h.logger.Error("Failed to fetch GitHub user profile", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=profile_fetch_failed")
		return
	}

	// Check if user already exists
	existingUser, err := h.userService.GetUserByEmail(c.Request.Context(), userProfile.Email)
	if err == nil && existingUser != nil {
		// SECURITY CHECK 1: Verify auth method is OAuth (prevent password account takeover)
		if existingUser.AuthMethod != "oauth" {
			h.logger.Warn("Password account attempted OAuth login - blocked for security", "email", userProfile.Email, "auth_method", existingUser.AuthMethod)

			c.Redirect(http.StatusTemporaryRedirect,
				getFrontendURL()+"/auth/signin?error=account_exists_use_password")
			return
		}

		// SECURITY CHECK 2: Verify OAuth provider matches
		if existingUser.OAuthProvider == nil || *existingUser.OAuthProvider != "github" {
			existingProviderName := "a different provider"
			if existingUser.OAuthProvider != nil {
				existingProviderName = *existingUser.OAuthProvider
			}

			h.logger.Warn("OAuth account with wrong provider", "email", userProfile.Email, "stored_provider", existingProviderName, "attempted_provider", "github")

			c.Redirect(http.StatusTemporaryRedirect,
				fmt.Sprintf("%s/auth/signin?error=use_%s", getFrontendURL(), existingProviderName))
			return
		}

		// SECURITY CHECK 3: Verify provider ID matches (prevents account takeover)
		if existingUser.OAuthProviderID != nil && *existingUser.OAuthProviderID != userProfile.ProviderID {
			h.logger.Error("OAuth provider ID mismatch - possible account takeover attempt", "email", userProfile.Email, "provider", "github")

			c.Redirect(http.StatusTemporaryRedirect,
				getFrontendURL()+"/auth/signin?error=authentication_failed")
			return
		}

		// All security checks passed - safe to auto-login
		h.logger.Info("Existing OAuth user logging in", "email", userProfile.Email, "provider", "github", "user_id", existingUser.ID)

		// Generate login tokens
		loginTokens, err := h.authService.GenerateTokensForUser(c.Request.Context(), existingUser.ID)
		if err != nil {
			h.logger.Error("Failed to generate login tokens for existing user", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=login_failed")
			return
		}

		// Create one-time login session to securely pass tokens to frontend
		loginSessionID, err := h.authService.CreateLoginTokenSession(
			c.Request.Context(),
			loginTokens.AccessToken,
			loginTokens.RefreshToken,
			loginTokens.ExpiresIn,
			existingUser.ID,
		)
		if err != nil {
			h.logger.Error("Failed to create login session", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=session_failed")
			return
		}

		// Redirect to frontend token exchange page
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/callback?session=%s&type=login", getFrontendURL(), loginSessionID))
		return
	}

	// New user - create OAuth session for Step 2
	session := &authDomain.OAuthSession{
		Email:           userProfile.Email,
		FirstName:       userProfile.FirstName,
		LastName:        userProfile.LastName,
		Provider:        userProfile.Provider,
		ProviderID:      userProfile.ProviderID,
		InvitationToken: invitationToken,
	}

	sessionID, err := h.authService.CreateOAuthSession(c.Request.Context(), session)
	if err != nil {
		h.logger.Error("Failed to create OAuth session", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/auth/signin?error=session_creation_failed")
		return
	}

	h.logger.Info("OAuth session created, redirecting to Step 2", "email", userProfile.Email, "provider", "github", "session_id", sessionID)

	// Redirect to frontend Step 2 (personalization)
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/signup?session=%s", getFrontendURL(), sessionID))
}
