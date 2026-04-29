package product

import "time"

// CreateProductRequest is the DTO for POST /api/v1/products
type CreateProductRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=255" example:"Wireless Mouse"`
	Description string  `json:"description" validate:"max=1000" example:"Ergonomic wireless mouse with USB receiver"`
	Price       float64 `json:"price" validate:"gte=0" example:"29.99"`
}

// UpdateProductRequest is the DTO for PATCH /api/v1/products/:id
// Uses pointer fields to distinguish "not sent" (nil) from "sent empty/zero"
type UpdateProductRequest struct {
	Name        *string  `json:"name" validate:"omitempty,min=1,max=255" example:"Wireless Mouse Pro"`
	Description *string  `json:"description" validate:"omitempty,max=1000" example:"Updated description"`
	Price       *float64 `json:"price" validate:"omitempty,gte=0" example:"39.99"`
}

// ProductResponse is the DTO returned to clients for single product and list items
type ProductResponse struct {
	ID          string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string    `json:"name" example:"Wireless Mouse"`
	Description string    `json:"description" example:"Ergonomic wireless mouse"`
	Price       float64   `json:"price" example:"29.99"`
	UserID      string    `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	UserEmail   string    `json:"user_email" example:"user@example.com"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PaginationQuery is the DTO for pagination query parameters
type PaginationQuery struct {
	Page  int `query:"page" validate:"omitempty,min=1" example:"1"`
	Limit int `query:"limit" validate:"omitempty,min=1,max=100" example:"10"`
}
