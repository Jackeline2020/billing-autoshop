package handler

import (
	"errors"
	"net/http"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/dto"
	"billing-autoshop/internal/usecase"
	pkgerrors "billing-autoshop/pkg/errors"
	"billing-autoshop/pkg/logger"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	usecase *usecase.PaymentUseCase
}

func NewPaymentHandler(u *usecase.PaymentUseCase) *PaymentHandler {
	return &PaymentHandler{usecase: u}
}

// @Summary     Consultar pagamento
// @Description Mostra o pagamento da OS, incluindo o link de pagamento enviado ao cliente
// @Tags        payments
// @Produce     json
// @Security    BearerAuth
// @Param       order_id path string true "ID da OS"
// @Success     200 {object} domain.Payment
// @Failure     404 {object} dto.ErrorResponse
// @Router      /payments/{order_id} [get]
func (h *PaymentHandler) GetByOrderID(c *gin.Context) {
	p, err := h.usecase.GetByOrderID(c.Param("order_id"))
	if errors.Is(err, domain.ErrPaymentNotFound) {
		c.JSON(http.StatusNotFound, errorBody(err))
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorBody(err))
		return
	}
	c.JSON(http.StatusOK, p)
}

// @Summary     Webhook do provedor de pagamento
// @Description Recebe o aviso do Mercado Pago (ou do mock) de que um pagamento mudou. O status é confirmado consultando o provedor.
// @Tags        payments
// @Accept      json
// @Param       request body dto.PaymentNotification false "Aviso do provedor"
// @Param       type    query string false "Tipo do aviso (payment)"
// @Param       data.id query string false "ID do pagamento no provedor"
// @Success     200
// @Failure     400 {object} dto.ErrorResponse
// @Failure     500 {object} dto.ErrorResponse
// @Router      /payments/webhook [post]
func (h *PaymentHandler) Webhook(c *gin.Context) {
	var body dto.PaymentNotification
	_ = c.ShouldBindJSON(&body)

	// O provedor pode enviar os dados no corpo ou na query string
	kind := firstNonEmpty(body.Type, c.Query("type"), c.Query("topic"))
	id := firstNonEmpty(body.Data.ID, c.Query("data.id"), c.Query("id"))

	if kind != "payment" {
		c.Status(http.StatusOK) // outros tipos de aviso não interessam ao Billing
		return
	}
	if id == "" {
		c.JSON(http.StatusBadRequest, errorBody(errors.New("id do pagamento não informado")))
		return
	}

	err := h.usecase.HandleNotification(id)
	if errors.Is(err, domain.ErrPaymentNotFound) {
		logger.Warn("aviso de pagamento desconhecido ignorado", "provider_payment_id", id)
		c.Status(http.StatusOK)
		return
	}
	if err != nil {
		// 500 faz o provedor reenviar o aviso mais tarde
		c.JSON(http.StatusInternalServerError, errorBody(err))
		return
	}
	c.Status(http.StatusOK)
}

func errorBody(err error) dto.ErrorResponse {
	return dto.ErrorResponse{Errors: []pkgerrors.ValidationError{{Field: "payment", Message: err.Error()}}}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
