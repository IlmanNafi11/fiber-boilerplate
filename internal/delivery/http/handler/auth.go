package handler

import (
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/middleware"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	authservice "github.com/ilmannafi/fiber-boilerplate/internal/service/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"

	"github.com/gofiber/fiber/v3"
)

type AuthHandler struct {
	authSvc *authservice.AuthService
}

func NewAuthHandler(authSvc *authservice.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

// Register godoc
// @Summary Register a new user
// @Description Create a new user account with email and password. Only available when public registration is enabled.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.RegisterRequest true "Registration data"
// @Success 201 {object} response.Response{data=auth.UserResponse} "User registered successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 403 {object} response.Response "Registration disabled"
// @Failure 409 {object} response.Response "Email already exists"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/register [post]
func (h *AuthHandler) Register(c fiber.Ctx) error {
	req := new(authdto.RegisterRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	user, err := h.authSvc.Register(c.Context(), req)
	if err != nil {
		return err
	}

	return response.Created(c, "user registered successfully", user)
}

// Login godoc
// @Summary Authenticate user
// @Description Log in with email and password to receive access and refresh tokens
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.LoginRequest true "Login credentials"
// @Success 200 {object} response.Response{data=auth.TokenResponse} "Login successful"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Invalid credentials"
// @Failure 403 {object} response.Response "Email not verified"
// @Failure 429 {object} response.Response "Too many login attempts"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c fiber.Ctx) error {
	req := new(authdto.LoginRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	userAgent := c.Get("User-Agent")
	ip := c.IP()

	tokenResp, err := h.authSvc.Login(c.Context(), req, userAgent, ip)
	if err != nil {
		return err
	}

	return response.OK(c, "login successful", tokenResp)
}

// Refresh godoc
// @Summary Refresh access token
// @Description Exchange a valid refresh token for new access and refresh tokens. Old refresh token is invalidated.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.RefreshRequest true "Refresh token"
// @Success 200 {object} response.Response{data=auth.TokenResponse} "Token refreshed successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Invalid or expired refresh token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(c fiber.Ctx) error {
	req := new(authdto.RefreshRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	tokenResp, err := h.authSvc.RefreshToken(c.Context(), req.RefreshToken)
	if err != nil {
		return err
	}

	return response.OK(c, "token refreshed successfully", tokenResp)
}

// Logout godoc
// @Summary Logout user
// @Description Invalidate the current refresh token and its token family (session)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.LogoutRequest true "Refresh token to invalidate"
// @Success 200 {object} response.Response "Logged out successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c fiber.Ctx) error {
	req := new(authdto.LogoutRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	if err := h.authSvc.Logout(c.Context(), req.RefreshToken); err != nil {
		return err
	}

	return response.OK(c, "logged out successfully", nil)
}

// Me godoc
// @Summary Get current user profile
// @Description Retrieve the authenticated user's profile information
// @Tags Auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response{data=auth.UserResponse} "User profile retrieved"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/me [get]
func (h *AuthHandler) Me(c fiber.Ctx) error {
	authCtx := middleware.GetAuthContext(c)

	userResp, err := h.authSvc.GetCurrentUser(c.Context(), authCtx.UserID)
	if err != nil {
		return err
	}

	return response.OK(c, "user profile retrieved", userResp)
}

// VerifyEmail godoc
// @Summary Verify email address
// @Description Confirm email verification using the token sent to the user's email
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.VerifyEmailRequest true "Verification token"
// @Success 200 {object} response.Response "Email verified successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Invalid or expired token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(c fiber.Ctx) error {
	req := new(authdto.VerifyEmailRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	if err := h.authSvc.VerifyEmail(c.Context(), req.Token); err != nil {
		return err
	}

	return response.OK(c, "email verified successfully", nil)
}

// ResendVerification godoc
// @Summary Resend verification email
// @Description Request a new verification email if the previous token has expired
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.ResendVerificationRequest true "Email address"
// @Success 200 {object} response.Response "Verification email resent if applicable"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/resend-verification [post]
func (h *AuthHandler) ResendVerification(c fiber.Ctx) error {
	req := new(authdto.ResendVerificationRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	_ = h.authSvc.ResendVerification(c.Context(), req.Email)

	return response.OK(c, "If an account with that email exists and requires verification, a new email has been sent.", nil)
}

// ForgotPassword godoc
// @Summary Request password reset
// @Description Request a password reset email. Returns generic response regardless of account existence.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.ForgotPasswordRequest true "Email address"
// @Success 200 {object} response.Response "Reset email sent if account exists"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 429 {object} response.Response "Too many requests"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	req := new(authdto.ForgotPasswordRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	_ = h.authSvc.ForgotPassword(c.Context(), req.Email)

	return response.OK(c, "If an account with that email exists, a password reset email has been sent.", nil)
}

// ResetPassword godoc
// @Summary Reset password
// @Description Reset password using the token from the reset email. Invalidates all active sessions.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.ResetPasswordRequest true "Reset token and new password"
// @Success 200 {object} response.Response "Password reset successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Invalid or expired token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c fiber.Ctx) error {
	req := new(authdto.ResetPasswordRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	if err := h.authSvc.ResetPassword(c.Context(), req.Token, req.NewPassword); err != nil {
		return err
	}

	return response.OK(c, "Password has been reset successfully.", nil)
}
