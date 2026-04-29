package handler

import (
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/middleware"
	productdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/product"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"

	"context"

	"github.com/gofiber/fiber/v3"
)

// ProductServiceProvider defines the service interface the handler depends on.
type ProductServiceProvider interface {
	Create(ctx context.Context, userID string, req *productdto.CreateProductRequest) (*productdto.ProductResponse, error)
	GetByID(ctx context.Context, id string) (*productdto.ProductResponse, error)
	List(ctx context.Context, page, limit int) ([]*productdto.ProductResponse, int, error)
	Update(ctx context.Context, productID, userID, role string, req *productdto.UpdateProductRequest) (*productdto.ProductResponse, error)
	Delete(ctx context.Context, productID, userID, role string) (*productdto.ProductResponse, error)
}

type ProductHandler struct {
	productSvc ProductServiceProvider
}

func NewProductHandler(productSvc ProductServiceProvider) *ProductHandler {
	return &ProductHandler{productSvc: productSvc}
}

// Create handles POST /api/v1/products (PROD-01)
func (h *ProductHandler) Create(c fiber.Ctx) error {
	req := new(productdto.CreateProductRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	authCtx := middleware.GetAuthContext(c)
	product, err := h.productSvc.Create(c.Context(), authCtx.UserID, req)
	if err != nil {
		return err
	}

	return response.Created(c, "product created successfully", product)
}

// GetByID handles GET /api/v1/products/:id (PROD-03)
func (h *ProductHandler) GetByID(c fiber.Ctx) error {
	id := c.Params("id")

	product, err := h.productSvc.GetByID(c.Context(), id)
	if err != nil {
		return err
	}

	return response.OK(c, "product retrieved successfully", product)
}

// List handles GET /api/v1/products with pagination (PROD-02)
func (h *ProductHandler) List(c fiber.Ctx) error {
	query := new(productdto.PaginationQuery)
	if err := c.Bind().Query(query); err != nil {
		return err
	}

	// Apply defaults (D-24)
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Limit == 0 {
		query.Limit = 10
	}

	products, total, err := h.productSvc.List(c.Context(), query.Page, query.Limit)
	if err != nil {
		return err
	}

	return response.Paginated(c, "products retrieved successfully", products, query.Page, query.Limit, total)
}

// Update handles PATCH /api/v1/products/:id (PROD-04)
func (h *ProductHandler) Update(c fiber.Ctx) error {
	id := c.Params("id")

	req := new(productdto.UpdateProductRequest)
	if err := c.Bind().JSON(req); err != nil {
		return err
	}

	authCtx := middleware.GetAuthContext(c)
	product, err := h.productSvc.Update(c.Context(), id, authCtx.UserID, authCtx.Role, req)
	if err != nil {
		return err
	}

	return response.OK(c, "product updated successfully", product)
}

// Delete handles DELETE /api/v1/products/:id (PROD-05)
func (h *ProductHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")

	authCtx := middleware.GetAuthContext(c)
	product, err := h.productSvc.Delete(c.Context(), id, authCtx.UserID, authCtx.Role)
	if err != nil {
		return err
	}

	return response.OK(c, "product deleted successfully", product)
}
