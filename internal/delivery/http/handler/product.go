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

// Create godoc
// @Summary Create a new product
// @Description Create a product with validated fields. Requires authentication.
// @Tags Products
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body product.CreateProductRequest true "Product data"
// @Success 201 {object} response.Response{data=product.ProductResponse} "Product created successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/products [post]
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

// GetByID godoc
// @Summary Get product by ID
// @Description Retrieve a single product by its UUID
// @Tags Products
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Product ID"
// @Success 200 {object} response.Response{data=product.ProductResponse} "Product retrieved successfully"
// @Failure 400 {object} response.Response "Invalid product ID"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 404 {object} response.Response "Product not found"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/products/{id} [get]
func (h *ProductHandler) GetByID(c fiber.Ctx) error {
	id := c.Params("id")

	product, err := h.productSvc.GetByID(c.Context(), id)
	if err != nil {
		return err
	}

	return response.OK(c, "product retrieved successfully", product)
}

// List godoc
// @Summary List products
// @Description Retrieve a paginated list of products
// @Tags Products
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" example(1)
// @Param limit query int false "Items per page" example(10)
// @Success 200 {object} response.Response{data=[]product.ProductResponse} "Products retrieved successfully"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/products [get]
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

// Update godoc
// @Summary Update a product
// @Description Update product fields. Only the owner or admin can update. Uses partial update (sent fields only).
// @Tags Products
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Product ID"
// @Param request body product.UpdateProductRequest true "Fields to update"
// @Success 200 {object} response.Response{data=product.ProductResponse} "Product updated successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 403 {object} response.Response "Not authorized to update this product"
// @Failure 404 {object} response.Response "Product not found"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/products/{id} [patch]
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

// Delete godoc
// @Summary Delete a product
// @Description Soft-delete a product. Only the owner or admin can delete.
// @Tags Products
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Product ID"
// @Success 200 {object} response.Response{data=product.ProductResponse} "Product deleted successfully"
// @Failure 400 {object} response.Response "Invalid product ID"
// @Failure 401 {object} response.Response "Missing or invalid token"
// @Failure 403 {object} response.Response "Not authorized to delete this product"
// @Failure 404 {object} response.Response "Product not found"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/products/{id} [delete]
func (h *ProductHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")

	authCtx := middleware.GetAuthContext(c)
	product, err := h.productSvc.Delete(c.Context(), id, authCtx.UserID, authCtx.Role)
	if err != nil {
		return err
	}

	return response.OK(c, "product deleted successfully", product)
}
