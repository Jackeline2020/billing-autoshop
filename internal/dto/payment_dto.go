package dto

// PaymentNotification é o aviso enviado pelo provedor de pagamento.
type PaymentNotification struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Data   struct {
		ID string `json:"id"`
	} `json:"data"`
}
