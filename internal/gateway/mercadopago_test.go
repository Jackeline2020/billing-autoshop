package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"billing-autoshop/internal/gateway"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// provider simula as rotas do Mercado Pago usadas pelo adaptador.
type provider struct {
	tokens      atomic.Int32
	rejectToken atomic.Bool // recusa a próxima chamada com 401
	lastBody    map[string]any
	lastIdemKey string
}

func (p *provider) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["client_secret"] != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.tokens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	authorized := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if p.rejectToken.CompareAndSwap(true, false) || r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("POST /checkout/preferences", authorized(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&p.lastBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "pref-1", "init_point": "http://checkout/pref-1"})
	}))
	mux.HandleFunc("GET /v1/payments/{id}", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "404" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":1000000001,"status":"approved","status_detail":"accredited","external_reference":"os-1","transaction_amount":350.5}`))
	}))
	mux.HandleFunc("POST /v1/payments/{id}/refunds", authorized(func(w http.ResponseWriter, r *http.Request) {
		p.lastIdemKey = r.Header.Get("X-Idempotency-Key")
		if r.PathValue("id") == "500" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("erro interno"))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func newClient(t *testing.T, p *provider, secret string) *gateway.MercadoPago {
	ts := p.server(t)
	return gateway.NewMercadoPago(gateway.Config{BaseURL: ts.URL + "/", ClientID: "cid", ClientSecret: secret})
}

func TestCreateCheckout(t *testing.T) {
	p := &provider{}
	mp := newClient(t, p, "secret")

	co, err := mp.CreateCheckout(context.Background(), gateway.CheckoutRequest{
		OrderID: "os-1", Description: "OS os-1", Amount: 350.5, NotificationURL: "http://billing/payments/webhook",
	})
	require.NoError(t, err)
	assert.Equal(t, "pref-1", co.PreferenceID)
	assert.Equal(t, "http://checkout/pref-1", co.URL)
	assert.Equal(t, "os-1", p.lastBody["external_reference"])
	assert.Equal(t, "http://billing/payments/webhook", p.lastBody["notification_url"])
}

func TestTokenIsReused(t *testing.T) {
	p := &provider{}
	mp := newClient(t, p, "secret")

	_, err := mp.GetPayment(context.Background(), "1")
	require.NoError(t, err)
	_, err = mp.GetPayment(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, int32(1), p.tokens.Load())
}

func TestTokenRenewedAfter401(t *testing.T) {
	p := &provider{}
	mp := newClient(t, p, "secret")
	_, _ = mp.GetPayment(context.Background(), "1")

	p.rejectToken.Store(true)
	pay, err := mp.GetPayment(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, "approved", pay.Status)
	assert.Equal(t, int32(2), p.tokens.Load())
}

func TestGetPayment(t *testing.T) {
	mp := newClient(t, &provider{}, "secret")
	pay, err := mp.GetPayment(context.Background(), "1000000001")
	require.NoError(t, err)
	assert.Equal(t, "1000000001", pay.ID)
	assert.Equal(t, "os-1", pay.ExternalReference)
	assert.Equal(t, 350.5, pay.Amount)
	assert.Equal(t, "accredited", pay.StatusDetail)

	_, err = mp.GetPayment(context.Background(), "404")
	assert.ErrorIs(t, err, gateway.ErrProviderNotFound)
}

func TestRefund(t *testing.T) {
	p := &provider{}
	mp := newClient(t, p, "secret")

	require.NoError(t, mp.Refund(context.Background(), "1000000001"))
	assert.Equal(t, "refund-1000000001", p.lastIdemKey)

	err := mp.Refund(context.Background(), "500")
	assert.ErrorContains(t, err, "500")
}

func TestInvalidCredentials(t *testing.T) {
	mp := newClient(t, &provider{}, "errado")
	_, err := mp.CreateCheckout(context.Background(), gateway.CheckoutRequest{OrderID: "os-1", Amount: 1})
	assert.ErrorContains(t, err, "token")
}

func TestProviderUnreachable(t *testing.T) {
	mp := gateway.NewMercadoPago(gateway.Config{BaseURL: "http://127.0.0.1:1", ClientID: "c", ClientSecret: "s"})
	_, err := mp.GetPayment(context.Background(), "1")
	assert.Error(t, err)
}
