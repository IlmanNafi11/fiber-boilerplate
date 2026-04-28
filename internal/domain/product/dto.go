package product

import "time"

// CreateProductRequest is the DTO for POST /api/v1/products
type CreateProductRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=255"`
	Description string  `json:"description" validate:"max=1000"`
	Price       float64 `json:"price" validate:"gte=0"`
}

// UpdateProductRequest is the DTO for PATCH /api/v1/products/:id
// Uses pointer fields to distinguish "not sent" (nil) from "sent empty/zero"
type UpdateProductRequest struct {
	Name        *string  `json:"name" validate:"omitempty,min=1,max=255"`
	Description *string  `json:"description" validate:"omitempty,max=1000"`
	Price       *float64 `json:"price" validate:"omitempty,gte=0"`
}

// ProductResponse is the DTO returned to clients for single product and list items
type ProductResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PaginationQuery is the DTO for pagination query parameters
type PaginationQuery struct {
	Page  int `query:"page" validate:"omitempty,min=1"`
	Limit int `query:"limit" validate:"omitempty,min=1,max=100"`
}
