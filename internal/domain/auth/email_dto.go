package auth

// Request DTOs for email verification and password reset flows.

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
}

type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email" example:"user@example.com"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email" example:"user@example.com"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	NewPassword string `json:"new_password" validate:"required,min=8" example:"newpassword123"`
}
