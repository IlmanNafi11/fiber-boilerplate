package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	productdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/product"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mock Service ---

type mockProductService struct {
	mock.Mock
}

func (m *mockProductService) Create(ctx context.Context, userID string, req *productdto.CreateProductRequest) (*productdto.ProductResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productdto.ProductResponse), args.Error(1)
}

func (m *mockProductService) GetByID(ctx context.Context, id string) (*productdto.ProductResponse, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productdto.ProductResponse), args.Error(1)
}

func (m *mockProductService) List(ctx context.Context, page, limit int) ([]*productdto.ProductResponse, int, error) {
	args := m.Called(ctx, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*productdto.ProductResponse), args.Int(1), args.Error(2)
}

func (m *mockProductService) Update(ctx context.Context, productID, userID, role string, req *productdto.UpdateProductRequest) (*productdto.ProductResponse, error) {
	args := m.Called(ctx, productID, userID, role, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productdto.ProductResponse), args.Error(1)
}

func (m *mockProductService) Delete(ctx context.Context, productID, userID, role string) (*productdto.ProductResponse, error) {
	args := m.Called(ctx, productID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*productdto.ProductResponse), args.Error(1)
}

// --- Helpers ---

var (
	handlerOwnerID = "user-handler-1"
	handlerRole    = "user"
	handlerNow     = time.Now().Truncate(time.Second)
)

func sampleResponse() *productdto.ProductResponse {
	return &productdto.ProductResponse{
		ID:          "prod-1",
		Name:        "Widget",
		Description: "A test widget",
		Price:       9.99,
		UserID:      handlerOwnerID,
		UserEmail:   "owner@test.com",
		CreatedAt:   handlerNow,
		UpdatedAt:   handlerNow,
	}
}

// setupHandlerTest creates a Fiber app with product routes and auth context injection.
func setupHandlerTest(svc *mockProductService) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: response.ErrorHandler(nil),
	})

	handler := &ProductHandler{productSvc: svc}

	// Route group with auth context injection via middleware
	api := app.Group("/api/v1/products", func(c fiber.Ctx) error {
		// Inject auth context for tests (simulates JWT middleware)
		c.Locals("auth", authdto.AuthContext{
			UserID: handlerOwnerID,
			Role:   handlerRole,
		})
		return c.Next()
	})

	api.Post("/", handler.Create)
	api.Get("/", handler.List)
	api.Get("/:id", handler.GetByID)
	api.Patch("/:id", handler.Update)
	api.Delete("/:id", handler.Delete)

	return app
}

func decodeResponse(t *testing.T, body io.Reader) response.Response {
	t.Helper()
	var result response.Response
	b, err := io.ReadAll(body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &result))
	return result
}

func ptrStr(s string) *string { return &s }

// --- Tests ---

// POST /api/v1/products - success (PROD-01)
func TestProductHandler_Create_Success(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	expected := sampleResponse()
	svc.On("Create", mock.Anything, handlerOwnerID, mock.AnythingOfType("*product.CreateProductRequest")).
		Return(expected, nil)

	body, _ := json.Marshal(productdto.CreateProductRequest{
		Name:        "Widget",
		Description: "A test widget",
		Price:       9.99,
	})
	req := httptest.NewRequest("POST", "/api/v1/products", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.True(t, result.Success)
	assert.Equal(t, "product created successfully", result.Message)
	svc.AssertExpectations(t)
}

// GET /api/v1/products/:id - success (PROD-03)
func TestProductHandler_GetByID_Success(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	expected := sampleResponse()
	svc.On("GetByID", mock.Anything, "prod-1").Return(expected, nil)

	req := httptest.NewRequest("GET", "/api/v1/products/prod-1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.True(t, result.Success)
	assert.Equal(t, "product retrieved successfully", result.Message)
	svc.AssertExpectations(t)
}

// GET /api/v1/products/:id - not found returns 404
func TestProductHandler_GetByID_NotFound(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	svc.On("GetByID", mock.Anything, "prod-missing").
		Return(nil, errx.NotFound("product not found"))

	req := httptest.NewRequest("GET", "/api/v1/products/prod-missing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	svc.AssertExpectations(t)
}

// GET /api/v1/products - paginated list (PROD-02)
func TestProductHandler_List_Paginated(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	items := []*productdto.ProductResponse{sampleResponse()}
	svc.On("List", mock.Anything, 1, 10).Return(items, 1, nil)

	req := httptest.NewRequest("GET", "/api/v1/products?page=1&limit=10", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.True(t, result.Success)
	require.NotNil(t, result.Meta)
	assert.Equal(t, 1, result.Meta.Page)
	assert.Equal(t, 10, result.Meta.Limit)
	assert.Equal(t, 1, result.Meta.Total)
	svc.AssertExpectations(t)
}

// GET /api/v1/products - default pagination values (D-24)
func TestProductHandler_List_DefaultPagination(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	svc.On("List", mock.Anything, 1, 10).Return([]*productdto.ProductResponse{}, 0, nil)

	// No page/limit query params — handler should apply defaults
	req := httptest.NewRequest("GET", "/api/v1/products", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	svc.AssertExpectations(t)
}

// PATCH /api/v1/products/:id - success (PROD-04)
func TestProductHandler_Update_Success(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	updated := sampleResponse()
	updated.Name = "Updated"
	svc.On("Update", mock.Anything, "prod-1", handlerOwnerID, handlerRole, mock.AnythingOfType("*product.UpdateProductRequest")).
		Return(updated, nil)

	body, _ := json.Marshal(productdto.UpdateProductRequest{
		Name: ptrStr("Updated"),
	})
	req := httptest.NewRequest("PATCH", "/api/v1/products/prod-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.True(t, result.Success)
	assert.Equal(t, "product updated successfully", result.Message)
	svc.AssertExpectations(t)
}

// PATCH /api/v1/products/:id - forbidden returns 403
func TestProductHandler_Update_Forbidden(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	svc.On("Update", mock.Anything, "prod-1", handlerOwnerID, handlerRole, mock.AnythingOfType("*product.UpdateProductRequest")).
		Return(nil, errx.Forbidden("you can only modify your own products"))

	body, _ := json.Marshal(productdto.UpdateProductRequest{
		Name: ptrStr("Hacked"),
	})
	req := httptest.NewRequest("PATCH", "/api/v1/products/prod-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
	svc.AssertExpectations(t)
}

// DELETE /api/v1/products/:id - success (PROD-05)
func TestProductHandler_Delete_Success(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	svc.On("Delete", mock.Anything, "prod-1", handlerOwnerID, handlerRole).
		Return(sampleResponse(), nil)

	req := httptest.NewRequest("DELETE", "/api/v1/products/prod-1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.True(t, result.Success)
	assert.Equal(t, "product deleted successfully", result.Message)
	svc.AssertExpectations(t)
}

// DELETE /api/v1/products/:id - forbidden returns 403
func TestProductHandler_Delete_Forbidden(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	svc.On("Delete", mock.Anything, "prod-1", handlerOwnerID, handlerRole).
		Return(nil, errx.Forbidden("you can only delete your own products"))

	req := httptest.NewRequest("DELETE", "/api/v1/products/prod-1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
	svc.AssertExpectations(t)
}

// Verify auth context is properly extracted (D-12, D-13)
func TestProductHandler_UsesAuthContext(t *testing.T) {
	svc := new(mockProductService)
	app := setupHandlerTest(svc)

	// The service must receive the user ID from the injected auth context
	svc.On("Create", mock.Anything, handlerOwnerID, mock.AnythingOfType("*product.CreateProductRequest")).
		Return(sampleResponse(), nil)

	body, _ := json.Marshal(productdto.CreateProductRequest{
		Name:  "Test",
		Price: 5.0,
	})
	req := httptest.NewRequest("POST", "/api/v1/products", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	svc.AssertExpectations(t)
}
