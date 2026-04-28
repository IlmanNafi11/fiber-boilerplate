package handler

import (
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/middleware"
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

func (h *AuthHandler) Me(c fiber.Ctx) error {
	authCtx := middleware.GetAuthContext(c)

	userResp, err := h.authSvc.GetCurrentUser(c.Context(), authCtx.UserID)
	if err != nil {
		return err
	}

	return response.OK(c, "user profile retrieved", userResp)
}


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

func (h *AuthHandler) ResendVerification(c fiber.Ctx) error {
	req := new(authdto.ResendVerificationRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	_ = h.authSvc.ResendVerification(c.Context(), req.Email)

	return response.OK(c, "If an account with that email exists and requires verification, a new email has been sent.", nil)
}

func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	req := new(authdto.ForgotPasswordRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	_ = h.authSvc.ForgotPassword(c.Context(), req.Email)

	return response.OK(c, "If an account with that email exists, a password reset email has been sent.", nil)
}

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
