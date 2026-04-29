package product

import (
	"context"
	"errors"

	productdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/product"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
)

// ProductRepo defines the interface for product persistence.
// Defined on consumer side (service) per established pattern (D-05).
type ProductRepo interface {
	Create(ctx context.Context, p *productdto.Product) error
	GetByID(ctx context.Context, id string) (*productdto.Product, string, error)
	List(ctx context.Context, page, limit int) ([]*productdto.Product, []string, int, error)
	Update(ctx context.Context, id string, req *productdto.UpdateProductRequest) (*productdto.Product, string, error)
	SoftDelete(ctx context.Context, id string) (*productdto.Product, string, error)
}

type ProductService struct {
	repo ProductRepo
}

func NewProductService(repo ProductRepo) *ProductService {
	return &ProductService{repo: repo}
}

// toResponse converts a Product model + user email into a ProductResponse DTO.
func toResponse(p *productdto.Product, userEmail string) *productdto.ProductResponse {
	return &productdto.ProductResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		UserID:      p.UserID,
		UserEmail:   userEmail,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// Create creates a new product for the authenticated user (PROD-01).
func (s *ProductService) Create(ctx context.Context, userID string, req *productdto.CreateProductRequest) (*productdto.ProductResponse, error) {
	p := &productdto.Product{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		if errors.Is(err, errx.ErrDuplicate) {
			return nil, errx.Conflict("product already exists")
		}
		return nil, errx.Internal("product creation failed", err.Error())
	}

	// Fetch user email for response
	product, userEmail, err := s.repo.GetByID(ctx, p.ID)
	if err != nil {
		return nil, errx.Internal("product lookup failed", err.Error())
	}

	return toResponse(product, userEmail), nil
}

// GetByID retrieves a single active product by ID (PROD-03).
func (s *ProductService) GetByID(ctx context.Context, id string) (*productdto.ProductResponse, error) {
	p, userEmail, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			return nil, errx.NotFound("product not found")
		}
		return nil, errx.Internal("product lookup failed", err.Error())
	}

	return toResponse(p, userEmail), nil
}

// List retrieves a paginated list of active products (PROD-02).
func (s *ProductService) List(ctx context.Context, page, limit int) ([]*productdto.ProductResponse, int, error) {
	products, userEmails, total, err := s.repo.List(ctx, page, limit)
	if err != nil {
		return nil, 0, errx.Internal("product list failed", err.Error())
	}

	responses := make([]*productdto.ProductResponse, len(products))
	for i, p := range products {
		responses[i] = toResponse(p, userEmails[i])
	}

	return responses, total, nil
}

// Update performs a partial update on a product with ownership check (PROD-04).
func (s *ProductService) Update(ctx context.Context, productID, userID, role string, req *productdto.UpdateProductRequest) (*productdto.ProductResponse, error) {
	// Ownership check: fetch product, verify owner or admin (D-12, D-13)
	existing, _, err := s.repo.GetByID(ctx, productID)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			return nil, errx.NotFound("product not found")
		}
		return nil, errx.Internal("product lookup failed", err.Error())
	}

	if existing.UserID != userID && role != user.RoleAdmin {
		return nil, errx.Forbidden("you can only modify your own products")
	}

	p, userEmail, err := s.repo.Update(ctx, productID, req)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			return nil, errx.NotFound("product not found")
		}
		return nil, errx.Internal("product update failed", err.Error())
	}

	return toResponse(p, userEmail), nil
}

// Delete soft-deletes a product with ownership check (PROD-05).
func (s *ProductService) Delete(ctx context.Context, productID, userID, role string) (*productdto.ProductResponse, error) {
	// Ownership check for delete (D-12, D-13)
	existing, _, err := s.repo.GetByID(ctx, productID)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			// Product may already be soft-deleted, try direct fetch for idempotent response
			return s.handleIdempotentDelete(ctx, productID, userID, role)
		}
		return nil, errx.Internal("product lookup failed", err.Error())
	}

	if existing.UserID != userID && role != user.RoleAdmin {
		return nil, errx.Forbidden("you can only delete your own products")
	}

	p, userEmail, err := s.repo.SoftDelete(ctx, productID)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			return nil, errx.NotFound("product not found")
		}
		return nil, errx.Internal("product deletion failed", err.Error())
	}

	return toResponse(p, userEmail), nil
}

// handleIdempotentDelete handles the case where GetByID returns not found
// (product may already be soft-deleted). Uses SoftDelete which fetches
// without the soft-delete filter for idempotent response (D-22).
func (s *ProductService) handleIdempotentDelete(ctx context.Context, productID, userID, role string) (*productdto.ProductResponse, error) {
	p, userEmail, err := s.repo.SoftDelete(ctx, productID)
	if err != nil {
		if errors.Is(err, productdto.ErrProductNotFound) {
			return nil, errx.NotFound("product not found")
		}
		return nil, errx.Internal("product deletion failed", err.Error())
	}

	// Ownership check after fetch (product exists but may be soft-deleted)
	if p.UserID != userID && role != user.RoleAdmin {
		return nil, errx.Forbidden("you can only delete your own products")
	}

	return toResponse(p, userEmail), nil
}
