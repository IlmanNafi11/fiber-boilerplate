package product

import (
	"context"
	"errors"
	"testing"
	"time"

	productdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/product"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mock ---

type mockProductRepo struct {
	mock.Mock
}

func (m *mockProductRepo) Create(ctx context.Context, p *productdto.Product) error {
	return m.Called(ctx, p).Error(0)
}

func (m *mockProductRepo) GetByID(ctx context.Context, id string) (*productdto.Product, string, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).(*productdto.Product), args.String(1), args.Error(2)
}

func (m *mockProductRepo) List(ctx context.Context, page, limit int) ([]*productdto.Product, []string, int, error) {
	args := m.Called(ctx, page, limit)
	if args.Get(0) == nil {
		return nil, nil, 0, args.Error(3)
	}
	return args.Get(0).([]*productdto.Product), args.Get(1).([]string), args.Int(2), args.Error(3)
}

func (m *mockProductRepo) Update(ctx context.Context, id string, req *productdto.UpdateProductRequest) (*productdto.Product, string, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).(*productdto.Product), args.String(1), args.Error(2)
}

func (m *mockProductRepo) SoftDelete(ctx context.Context, id string) (*productdto.Product, string, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).(*productdto.Product), args.String(1), args.Error(2)
}

// --- Helpers ---

var (
	ownerID  = "user-owner-1"
	otherID  = "user-other-2"
	adminID  = "user-admin-3"
	now      = time.Now().Truncate(time.Second)
	testMail = "owner@test.com"
)

func sampleProduct(userID string) *productdto.Product {
	return &productdto.Product{
		ID:          "prod-1",
		UserID:      userID,
		Name:        "Widget",
		Description: "A test widget",
		Price:       9.99,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func sampleCreateReq() *productdto.CreateProductRequest {
	return &productdto.CreateProductRequest{
		Name:        "Widget",
		Description: "A test widget",
		Price:       9.99,
	}
}

func ptr(s string) *string   { return &s }
func ptf(f float64) *float64 { return &f }

// --- Tests ---

// Create: success path (PROD-01)
func TestCreate_Success(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	req := sampleCreateReq()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*product.Product")).
		Run(func(args mock.Arguments) {
			p := args.Get(1).(*productdto.Product)
			p.ID = "prod-new"
			p.CreatedAt = now
			p.UpdatedAt = now
		}).Return(nil)

	repo.On("GetByID", mock.Anything, "prod-new").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Create(context.Background(), ownerID, req)
	require.NoError(t, err)
	assert.Equal(t, "Widget", resp.Name)
	assert.Equal(t, ownerID, resp.UserID)
	assert.Equal(t, testMail, resp.UserEmail)
	repo.AssertExpectations(t)
}

// Create: duplicate returns 409 (PROD-01)
func TestCreate_Duplicate(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*product.Product")).
		Return(errx.ErrDuplicate)

	resp, err := svc.Create(context.Background(), ownerID, sampleCreateReq())
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 409, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// Create: repo failure returns 500
func TestCreate_RepoError(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*product.Product")).
		Return(errors.New("connection lost"))

	resp, err := svc.Create(context.Background(), ownerID, sampleCreateReq())
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// GetByID: success (PROD-03)
func TestGetByID_Success(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.GetByID(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, "prod-1", resp.ID)
	assert.Equal(t, testMail, resp.UserEmail)
	repo.AssertExpectations(t)
}

// GetByID: not found returns 404
func TestGetByID_NotFound(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-missing").
		Return(nil, "", productdto.ErrProductNotFound)

	resp, err := svc.GetByID(context.Background(), "prod-missing")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 404, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// List: success with pagination (PROD-02)
func TestList_Success(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	products := []*productdto.Product{sampleProduct(ownerID)}
	emails := []string{testMail}

	repo.On("List", mock.Anything, 1, 10).
		Return(products, emails, 1, nil)

	resp, total, err := svc.List(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, resp, 1)
	assert.Equal(t, "Widget", resp[0].Name)
	repo.AssertExpectations(t)
}

// List: empty result
func TestList_Empty(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("List", mock.Anything, 2, 10).
		Return([]*productdto.Product{}, []string{}, 0, nil)

	resp, total, err := svc.List(context.Background(), 2, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, resp)
	repo.AssertExpectations(t)
}

// List: repo error returns 500
func TestList_RepoError(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("List", mock.Anything, 1, 10).
		Return(nil, nil, 0, errors.New("db down"))

	resp, total, err := svc.List(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 0, total)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// Update: owner can update (PROD-04)
func TestUpdate_OwnerSuccess(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	req := &productdto.UpdateProductRequest{Name: ptr("Updated")}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	updated := sampleProduct(ownerID)
	updated.Name = "Updated"
	repo.On("Update", mock.Anything, "prod-1", req).
		Return(updated, testMail, nil)

	resp, err := svc.Update(context.Background(), "prod-1", ownerID, user.RoleUser, req)
	require.NoError(t, err)
	assert.Equal(t, "Updated", resp.Name)
	repo.AssertExpectations(t)
}

// Update: admin can update another user's product (PROD-04)
func TestUpdate_AdminBypass(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	req := &productdto.UpdateProductRequest{Price: ptf(19.99)}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	updated := sampleProduct(ownerID)
	updated.Price = 19.99
	repo.On("Update", mock.Anything, "prod-1", req).
		Return(updated, testMail, nil)

	resp, err := svc.Update(context.Background(), "prod-1", adminID, user.RoleAdmin, req)
	require.NoError(t, err)
	assert.Equal(t, 19.99, resp.Price)
	repo.AssertExpectations(t)
}

// Update: non-owner non-admin gets 403 (T-7-03)
func TestUpdate_Forbidden(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	req := &productdto.UpdateProductRequest{Name: ptr("Hacked")}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Update(context.Background(), "prod-1", otherID, user.RoleUser, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 403, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
	// Update must NOT be called when authorization fails
	repo.AssertNotCalled(t, "Update")
}

// Update: not found returns 404
func TestUpdate_NotFound(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	req := &productdto.UpdateProductRequest{Name: ptr("X")}

	repo.On("GetByID", mock.Anything, "prod-missing").
		Return(nil, "", productdto.ErrProductNotFound)

	resp, err := svc.Update(context.Background(), "prod-missing", ownerID, user.RoleUser, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 404, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// Delete: owner can delete (PROD-05)
func TestDelete_OwnerSuccess(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	repo.On("SoftDelete", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Delete(context.Background(), "prod-1", ownerID, user.RoleUser)
	require.NoError(t, err)
	assert.Equal(t, "prod-1", resp.ID)
	repo.AssertExpectations(t)
}

// Delete: admin can delete another user's product (PROD-05)
func TestDelete_AdminBypass(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	repo.On("SoftDelete", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Delete(context.Background(), "prod-1", adminID, user.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, "prod-1", resp.ID)
	repo.AssertExpectations(t)
}

// Delete: non-owner non-admin gets 403 (T-7-03)
func TestDelete_Forbidden(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Delete(context.Background(), "prod-1", otherID, user.RoleUser)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 403, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
	repo.AssertNotCalled(t, "SoftDelete")
}

// Delete: already soft-deleted returns idempotent 200 (D-22)
func TestDelete_IdempotentAlreadySoftDeleted(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	// GetByID fails (product is soft-deleted, filtered by deleted_at IS NULL)
	repo.On("GetByID", mock.Anything, "prod-1").
		Return(nil, "", productdto.ErrProductNotFound)

	// SoftDelete fetches without filter and returns product data
	repo.On("SoftDelete", mock.Anything, "prod-1").
		Return(sampleProduct(ownerID), testMail, nil)

	resp, err := svc.Delete(context.Background(), "prod-1", ownerID, user.RoleUser)
	require.NoError(t, err)
	assert.Equal(t, "prod-1", resp.ID)
	repo.AssertExpectations(t)
}

// Delete: idempotent path still checks ownership (T-7-03)
func TestDelete_IdempotentForbidsNonOwner(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-1").
		Return(nil, "", productdto.ErrProductNotFound)

	p := sampleProduct(ownerID)
	repo.On("SoftDelete", mock.Anything, "prod-1").
		Return(p, testMail, nil)

	resp, err := svc.Delete(context.Background(), "prod-1", otherID, user.RoleUser)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 403, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}

// Delete: not found (truly nonexistent) returns 404
func TestDelete_TrulyNotFound(t *testing.T) {
	repo := new(mockProductRepo)
	svc := &ProductService{repo: repo}

	repo.On("GetByID", mock.Anything, "prod-ghost").
		Return(nil, "", productdto.ErrProductNotFound)

	repo.On("SoftDelete", mock.Anything, "prod-ghost").
		Return(nil, "", productdto.ErrProductNotFound)

	resp, err := svc.Delete(context.Background(), "prod-ghost", ownerID, user.RoleUser)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 404, err.(*errx.AppError).HTTPStatus)
	repo.AssertExpectations(t)
}
