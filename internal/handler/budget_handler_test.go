package handler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/handler"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupBudgetRouter() (*gin.Engine, *mocks.BudgetRepositoryMock) {
	gin.SetMode(gin.TestMode)
	repo := new(mocks.BudgetRepositoryMock)
	h := handler.NewBudgetHandler(usecase.NewBudgetUseCase(repo))
	r := gin.New()
	r.GET("/budgets/:order_id", h.GetByOrderID)
	return r, repo
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestBudgetHandler_GetByOrderID(t *testing.T) {
	r, repo := setupBudgetRouter()
	repo.On("FindByOrderID", "os-1").Return(domain.Budget{ID: "b1", OrderID: "os-1", Total: 80}, nil)
	repo.On("FindByOrderID", "x").Return(domain.Budget{}, domain.ErrBudgetNotFound)
	repo.On("FindByOrderID", "erro").Return(domain.Budget{}, errors.New("banco fora"))

	w := get(r, "/budgets/os-1")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"total":80`)

	assert.Equal(t, http.StatusNotFound, get(r, "/budgets/x").Code)
	assert.Equal(t, http.StatusInternalServerError, get(r, "/budgets/erro").Code)
}
