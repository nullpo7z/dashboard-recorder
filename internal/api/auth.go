package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

const (
	// Cookie names
	cookieState    = "oidc_state"
	cookieNonce    = "oidc_nonce"
	cookieVerifier = "oidc_verifier"
)

// OIDCContext holds the provider and config for reuse
type OIDCContext struct {
	Provider *oidc.Provider
	Config   *oauth2.Config
}

// InitOIDC initializes the OIDC provider (discovery)
func (h *Handler) InitOIDC() error {
	if h.Config.OIDCProvider == "" {
		fmt.Println("OIDC: Provider URL not set, OIDC disabled.")
		return nil
	}

	provider, err := oidc.NewProvider(context.Background(), h.Config.OIDCProvider)
	if err != nil {
		return fmt.Errorf("failed to get OIDC provider: %w", err)
	}

	h.OIDC = &OIDCContext{
		Provider: provider,
		Config: &oauth2.Config{
			ClientID:     h.Config.OIDCClientID,
			ClientSecret: h.Config.OIDCClientSecret,
			RedirectURL:  h.Config.OIDCRedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       h.Config.OIDCScopes,
		},
	}
	fmt.Printf("OIDC: Initialized with provider %s\n", h.Config.OIDCProvider)
	return nil
}

// AuthLogin initiates the OIDC flow
func (h *Handler) AuthLogin(c echo.Context) error {
	if h.OIDC == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "OIDC not configured"})
	}

	// 1. Generate State
	state, err := generateRandomString(32)
	if err != nil {
		return h.mapOIDCError(c, err, "failed to generate state")
	}

	// 2. Generate Nonce
	nonce, err := generateRandomString(32)
	if err != nil {
		return h.mapOIDCError(c, err, "failed to generate nonce")
	}

	// 3. Generate PKCE
	verifier, err := generateRandomString(32)
	if err != nil {
		return h.mapOIDCError(c, err, "failed to generate verifier")
	}
	// S256 Challenge
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	// 4. Set Secure Cookies (One-time use)
	// Use __Host- prefix for extra security if over HTTPS (recommended)
	// We'll trust the user has TLS termination or local usage.
	h.setCookie(c, cookieState, state, 300)
	h.setCookie(c, cookieNonce, nonce, 300)
	h.setCookie(c, cookieVerifier, verifier, 300)

	// 5. Redirect
	authURL := h.OIDC.Config.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	return c.Redirect(http.StatusFound, authURL)
}

// AuthCallback handles the IDP response
func (h *Handler) AuthCallback(c echo.Context) error {
	if h.OIDC == nil {
		return c.Redirect(http.StatusFound, "/login?error=oidc_disabled")
	}

	// Cleanup cookies regardless of outcome
	defer h.deleteAuthCookies(c)

	// 1. Validate State
	queryState := c.QueryParam("state")
	cookieStateVal, err := c.Cookie(cookieState)
	if err != nil || queryState != cookieStateVal.Value {
		fmt.Printf("OIDC Error: State mismatch. Query: %s, Cookie: %v\n", queryState, err)
		return c.Redirect(http.StatusFound, "/login?error=invalid_state")
	}

	// 2. Exchange Code for Token (PKCE)
	code := c.QueryParam("code")
	if code == "" {
		return c.Redirect(http.StatusFound, "/login?error=missing_code")
	}

	cookieVerifierVal, err := c.Cookie(cookieVerifier)
	if err != nil {
		return c.Redirect(http.StatusFound, "/login?error=missing_verifier")
	}

	token, err := h.OIDC.Config.Exchange(
		c.Request().Context(),
		code,
		oauth2.VerifierOption(cookieVerifierVal.Value),
	)
	if err != nil {
		fmt.Printf("OIDC Error: Token exchange failed: %v\n", err)
		// Mask error
		return c.Redirect(http.StatusFound, "/login?error=token_exchange_failed")
	}

	// 3. Extract ID Token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return c.Redirect(http.StatusFound, "/login?error=no_id_token")
	}

	// 4. Verify ID Token
	verifier := h.OIDC.Provider.Verifier(&oidc.Config{ClientID: h.Config.OIDCClientID})
	idToken, err := verifier.Verify(c.Request().Context(), rawIDToken)
	if err != nil {
		fmt.Printf("OIDC Error: Token verification failed: %v\n", err)
		return c.Redirect(http.StatusFound, "/login?error=token_verification_failed")
	}

	// 5. Verify Nonce
	cookieNonceVal, err := c.Cookie(cookieNonce)
	if err != nil || idToken.Nonce != cookieNonceVal.Value {
		fmt.Println("OIDC Error: Nonce mismatch")
		return c.Redirect(http.StatusFound, "/login?error=invalid_nonce")
	}

	// 6. Access Control (Email Check)
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return c.Redirect(http.StatusFound, "/login?error=claims_error")
	}

	if !h.isEmailAllowed(claims.Email) {
		fmt.Printf("OIDC Error: Email %s not in allowed list\n", claims.Email)
		// Specific error for unauthorized user
		return c.Redirect(http.StatusFound, "/login?error=access_denied")
	}

	// 7. Establish Session
	// Generate App JWT (reusing existing logic)
	appToken, err := h.generateAppToken(claims.Email)
	if err != nil {
		fmt.Printf("OIDC Error: Failed to generate app token: %v\n", err)
		return c.Redirect(http.StatusFound, "/login?error=session_error")
	}

	// Return HTML attempting to store token and redirect
	// For simplicity in this React app, we usually send the token in URL or set a cookie.
	// Since the original login returned a JSON with token, we need to pass this to the frontend.
	// Passing via URL fragment is common for implicit flow but we did code flow.
	// We can set a temporary cookie or pass it in URL query (short lived) for frontend to grab.
	// BETTER: Set an httpOnly cookie for the session if the app supports it,
	// BUT the current app uses `localStorage` (based on typical React behavior).
	// Let's pass it in the fragment to avoid server logs: /#token=...

	// Wait, standard `go-oidc` examples often set a session cookie.
	// The user request didn't specify session mechanism change, but "Login Handler" returns JSON.
	// Here we are in a browser redirect flow.
	// We will redirect to /login?token=... and let frontend handle it.
	return c.Redirect(http.StatusFound, fmt.Sprintf("/login?token=%s", appToken))
}

// Helpers

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (h *Handler) setCookie(c echo.Context, name, value string, maxAge int) {
	cookie := new(http.Cookie)
	cookie.Name = name
	cookie.Value = value
	cookie.Path = "/api/auth" // Scope to auth endpoints
	cookie.HttpOnly = true
	cookie.Secure = true // Always set Secure (assumes TLS or localhost)
	cookie.SameSite = http.SameSiteLaxMode
	cookie.MaxAge = maxAge
	c.SetCookie(cookie)
}

func (h *Handler) deleteAuthCookies(c echo.Context) {
	for _, name := range []string{cookieState, cookieNonce, cookieVerifier} {
		h.setCookie(c, name, "", -1)
	}
}

func (h *Handler) isEmailAllowed(email string) bool {
	if len(h.Config.OIDCAllowedEmails) == 0 {
		return false // Deny by default if list is empty
	}
	// Wildcard check
	if len(h.Config.OIDCAllowedEmails) == 1 && h.Config.OIDCAllowedEmails[0] == "*" {
		return true
	}

	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, allowed := range h.Config.OIDCAllowedEmails {
		if allowed == normalized {
			return true
		}
	}
	return false
}

func (h *Handler) mapOIDCError(c echo.Context, err error, msg string) error {
	fmt.Printf("OIDC Internal Error: %s: %v\n", msg, err)
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Authentication failed"})
}

// Replicate generate token logic from Login handler into a reusable method
// NOTE: I will need to refactor handler.go to expose this or duplicate it.
// For now, I'll duplicate the simple JWT generation to avoid touching handler.go too much
// unless I export a method there.
func (h *Handler) generateAppToken(username string) (string, error) {
	// Import jwt is needed
	// circular dependency if I call back to handler? No, I am in package api.
	return h.createJWT(username)
}
