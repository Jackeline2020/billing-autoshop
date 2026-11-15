package handler

import (
	"errors"
	"net/http"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/dto"
	"billing-autoshop/internal/usecase"
	pkgerrors "billing-autoshop/pkg/errors"

	"github.com/gin-gonic/gin"
)

type BudgetHandler struct {
	usecase *usecase.BudgetUseCase
}

func NewBudgetHandler(u *usecase.BudgetUseCase) *BudgetHandler {
	return &BudgetHandler{usecase: u}
}

// @Summary     Consultar orçamento
// @Description Mostra o orçamento da OS, com itens, totais e status
// @Tags        budgets
// @Produce     json
// @Security    BearerAuth
// @Param       order_id path string true "ID da OS"
// @Success     200 {object} domain.Budget
// @Failure     404 {object} dto.ErrorResponse
// @Router      /budgets/{order_id} [get]
func (h *BudgetHandler) GetByOrderID(c *gin.Context) {
	b, err := h.usecase.GetByOrderID(c.Param("order_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, b)
}

// respondError traduz erros de domínio para o status HTTP adequado.
func respondError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, domain.ErrBudgetNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, dto.ErrorResponse{Errors: []pkgerrors.ValidationError{{Field: "billing", Message: err.Error()}}})
}
