package handler_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/gateway"
	"billing-autoshop/internal/handler"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type paymentRouterMocks struct {
	payments *mocks.PaymentRepositoryMock
	gateway  *mocks.GatewayMock
	notifier *mocks.NotifierMock
}

func setupPaymentRouter() (*gin.Engine, paymentRouterMocks) {
	gin.SetMode(gin.TestMode)
	m := paymentRouterMocks{payments: new(mocks.PaymentRepositoryMock), gateway: new(mocks.GatewayMock), notifier: &mocks.NotifierMock{}}
	uc := usecase.NewPaymentUseCase(m.payments, usecase.NewBudgetUseCase(new(mocks.BudgetRepositoryMock)), m.gateway, m.notifier,
		usecase.PaymentConfig{Timeout: time.Minute})
	h := handler.NewPaymentHandler(uc)
	r := gin.New()
	r.POST("/payments/webhook", h.Webhook)
	r.GET("/payments/:order_id", h.GetByOrderID)
	return r, m
}

func post(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestPaymentHandler_GetByOrderID(t *testing.T) {
	r, m := setupPaymentRouter()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{ID: "p1", OrderID: "os-1", CheckoutURL: "http://checkout/1"}, nil)
	m.payments.On("FindByOrderID", "x").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	m.payments.On("FindByOrderID", "erro").Return(domain.Payment{}, errors.New("banco fora"))

	w := get(r, "/payments/os-1")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "http://checkout/1")
	assert.Equal(t, http.StatusNotFound, get(r, "/payments/x").Code)
	assert.Equal(t, http.StatusInternalServerError, get(r, "/payments/erro").Code)
}

func approvedNotification(m paymentRouterMocks, id string) {
	m.gateway.On("GetPayment", id).Return(gateway.ProviderPayment{ID: id, Status: "approved", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{ID: "p1", OrderID: "os-1", Status: domain.PaymentPending}, nil)
	m.payments.On("Update", mock.Anything, domain.PaymentPending).Return(nil)
}

func TestWebhook_Body(t *testing.T) {
	r, m := setupPaymentRouter()
	approvedNotification(m, "123")

	w := post(r, "/payments/webhook", `{"type":"payment","action":"payment.created","data":{"id":"123"}}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, m.notifier.Confirmed, 1)
}

func TestWebhook_QueryString(t *testing.T) {
	r, m := setupPaymentRouter()
	approvedNotification(m, "456")

	w := post(r, "/payments/webhook?type=payment&data.id=456", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, m.notifier.Confirmed, 1)
}

func TestWebhook_IgnoresOtherTypes(t *testing.T) {
	r, m := setupPaymentRouter()
	w := post(r, "/payments/webhook", `{"type":"merchant_order","data":{"id":"1"}}`)
	assert.Equal(t, http.StatusOK, w.Code)
	m.gateway.AssertNotCalled(t, "GetPayment", mock.Anything)
}

func TestWebhook_MissingID(t *testing.T) {
	r, _ := setupPaymentRouter()
	assert.Equal(t, http.StatusBadRequest, post(r, "/payments/webhook", `{"type":"payment"}`).Code)
}

func TestWebhook_UnknownPaymentIsIgnored(t *testing.T) {
	r, m := setupPaymentRouter()
	m.gateway.On("GetPayment", "999").Return(gateway.ProviderPayment{}, gateway.ErrProviderNotFound)
	assert.Equal(t, http.StatusOK, post(r, "/payments/webhook", `{"type":"payment","data":{"id":"999"}}`).Code)
}

func TestWebhook_ProviderErrorAsksForRetry(t *testing.T) {
	r, m := setupPaymentRouter()
	m.gateway.On("GetPayment", "1").Return(gateway.ProviderPayment{}, errors.New("timeout"))
	assert.Equal(t, http.StatusInternalServerError, post(r, "/payments/webhook", `{"type":"payment","data":{"id":"1"}}`).Code)
}
