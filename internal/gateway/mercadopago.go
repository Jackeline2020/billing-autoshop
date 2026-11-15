// Package gateway integra o Billing ao provedor de pagamento no formato da
// API do Mercado Pago (Checkout Pro). O mesmo código atende o Mercado Pago
// real e o payment-mock-autoshop; muda apenas a configuração (ADR-007).
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Status de pagamento devolvidos pelo provedor
const (
	ProviderApproved  = "approved"
	ProviderRejected  = "rejected"
	ProviderCancelled = "cancelled"
	ProviderRefunded  = "refunded"
)

var ErrProviderNotFound = errors.New("pagamento não encontrado no provedor")

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
}

type CheckoutRequest struct {
	OrderID         string
	Description     string
	Amount          float64
	NotificationURL string
}

type Checkout struct {
	PreferenceID string
	URL          string
}

type ProviderPayment struct {
	ID                string
	Status            string
	StatusDetail      string
	ExternalReference string
	Amount            float64
}

// MercadoPago é o cliente HTTP do provedor. O token de acesso é obtido com
// client_id/client_secret e reutilizado até perto de expirar.
type MercadoPago struct {
	cfg    Config
	client *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewMercadoPago(cfg Config) *MercadoPago {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &MercadoPago{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (m *MercadoPago) CreateCheckout(ctx context.Context, req CheckoutRequest) (Checkout, error) {
	body := map[string]any{
		"items": []map[string]any{{
			"title":       req.Description,
			"quantity":    1,
			"unit_price":  req.Amount,
			"currency_id": "BRL",
		}},
		"external_reference": req.OrderID,
		"notification_url":   req.NotificationURL,
	}
	var out struct {
		ID        string `json:"id"`
		InitPoint string `json:"init_point"`
	}
	if err := m.do(ctx, http.MethodPost, "/checkout/preferences", body, "", &out); err != nil {
		return Checkout{}, fmt.Errorf("erro ao criar link de pagamento: %w", err)
	}
	return Checkout{PreferenceID: out.ID, URL: out.InitPoint}, nil
}

func (m *MercadoPago) GetPayment(ctx context.Context, providerPaymentID string) (ProviderPayment, error) {
	var out struct {
		ID                json.Number `json:"id"`
		Status            string      `json:"status"`
		StatusDetail      string      `json:"status_detail"`
		ExternalReference string      `json:"external_reference"`
		TransactionAmount float64     `json:"transaction_amount"`
	}
	if err := m.do(ctx, http.MethodGet, "/v1/payments/"+providerPaymentID, nil, "", &out); err != nil {
		return ProviderPayment{}, err
	}
	return ProviderPayment{
		ID:                out.ID.String(),
		Status:            out.Status,
		StatusDetail:      out.StatusDetail,
		ExternalReference: out.ExternalReference,
		Amount:            out.TransactionAmount,
	}, nil
}

// Refund estorna o pagamento. A chave de idempotência evita estorno em
// dobro se a chamada for repetida.
func (m *MercadoPago) Refund(ctx context.Context, providerPaymentID string) error {
	return m.do(ctx, http.MethodPost, "/v1/payments/"+providerPaymentID+"/refunds", map[string]any{}, "refund-"+providerPaymentID, nil)
}

func (m *MercadoPago) accessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token != "" && time.Now().Before(m.tokenExpiry) {
		return m.token, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     m.cfg.ClientID,
		"client_secret": m.cfg.ClientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/oauth/token", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao obter token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("erro ao obter token: status %d", resp.StatusCode)
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.AccessToken == "" {
		return "", errors.New("resposta de token inválida")
	}

	// Renova um minuto antes de expirar
	m.token = out.AccessToken
	m.tokenExpiry = time.Now().Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	return m.token, nil
}

func (m *MercadoPago) invalidateToken() {
	m.mu.Lock()
	m.token = ""
	m.mu.Unlock()
}

// do executa a chamada autenticada. Se o token for recusado (401), obtém
// um novo e tenta mais uma vez.
func (m *MercadoPago) do(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		status, data, err := m.send(ctx, method, path, body, idempotencyKey)
		if err != nil {
			return err
		}
		switch {
		case status == http.StatusUnauthorized && attempt == 0:
			m.invalidateToken()
			continue
		case status == http.StatusNotFound:
			return ErrProviderNotFound
		case status >= 300:
			return fmt.Errorf("provedor respondeu %d: %s", status, strings.TrimSpace(string(data)))
		}
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	}
	return errors.New("token recusado pelo provedor")
}

func (m *MercadoPago) send(ctx context.Context, method, path string, body any, idempotencyKey string) (int, []byte, error) {
	token, err := m.accessToken(ctx)
	if err != nil {
		return 0, nil, err
	}

	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.cfg.BaseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("erro ao chamar o provedor: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}
