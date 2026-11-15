# Billing Service — AutoShop

Microsserviço de **orçamento e pagamento** da oficina: gera o orçamento da
OS, cobra o cliente por um link de pagamento (Checkout Pro do Mercado
Pago) e estorna quando a Saga precisa desfazer a cobrança.

Faz parte da arquitetura de microsserviços do AutoShop. A visão geral, o
fluxo da Saga e as decisões de arquitetura estão no repositório
[`app-autoshop`](https://github.com/Jackeline2020/app-autoshop/tree/develop/docs):
[arquitetura](https://github.com/Jackeline2020/app-autoshop/blob/develop/docs/architecture.md),
[fluxo da OS](https://github.com/Jackeline2020/app-autoshop/blob/develop/docs/saga-flow.md) e
[pagamento](https://github.com/Jackeline2020/app-autoshop/blob/develop/docs/adr/ADR-007-pagamento.md).

## Responsabilidades

- Geração do orçamento a partir dos serviços e peças da OS
- Aprovação e cancelamento do orçamento (compensação da Saga)
- Cobrança pelo link de pagamento, confirmação pelo webhook do provedor e expiração por prazo
- Estorno do pagamento (compensação da Saga)

## Tecnologias

| Tecnologia | Uso |
|---|---|
| Go + Gin | API HTTP |
| PostgreSQL | Banco do serviço (`budgets`, `budget_items`, `payments`, `processed_messages`) |
| Amazon SQS | Comandos da Saga (`billing-commands`) e eventos para o orquestrador (`os-events`) |
| Mercado Pago / [payment-mock](https://github.com/Jackeline2020/payment-mock-autoshop) | Provedor de pagamento, escolhido por configuração |
| New Relic | APM, logs JSON com `correlation_id` |

## API

Documentação interativa em `/swagger/index.html`.

| Método | Rota | Autenticação | Descrição |
|---|---|---|---|
| `GET` | `/health` | — | Health check |
| `GET` | `/budgets/{order_id}` | JWT | Orçamento da OS, com itens e totais |
| `GET` | `/payments/{order_id}` | JWT | Pagamento da OS, com o link enviado ao cliente |
| `POST` | `/payments/webhook` | — | Aviso do provedor de pagamento |

O webhook não exige autenticação porque o aviso **apenas informa o ID do
pagamento**: o Billing sempre consulta o status no provedor antes de
confirmar, então um aviso falso não consegue aprovar um pagamento.

## Integração com a Saga

| Comando recebido | Ação | Evento enviado |
|---|---|---|
| `GenerateBudget` | Calcula e grava o orçamento | `BudgetGenerated` ou `BudgetGenerationFailed` |
| `CancelBudget` | Cancela o orçamento (compensação) | `BudgetCancelled` |
| `RequestPayment` | Aprova o orçamento e cria o link de pagamento | `PaymentRequested` (com o link) ou `PaymentFailed` |
| — | Webhook: pagamento aprovado ou recusado no provedor | `PaymentConfirmed` ou `PaymentFailed` |
| — | Prazo de pagamento vencido | `PaymentFailed` |
| `RefundPayment` | Estorna o pagamento (compensação) | `PaymentRefunded` |

- **Idempotência:** mensagens repetidas são ignoradas (`processed_messages`);
  gerar orçamento, solicitar pagamento, cancelar e estornar podem ser
  repetidos sem efeito duplicado. O estorno usa chave de idempotência no
  provedor.
- **Pagamento após a compensação:** um pagamento pendente é encerrado no
  estorno, e um pagamento aprovado depois de expirado é estornado
  automaticamente.
- **Falhas:** erros técnicos deixam a mensagem na fila para nova tentativa;
  após 5 tentativas ela vai para a DLQ.

## Configuração

| Variável | Padrão | Descrição |
|---|---|---|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `localhost`, `5432`, `billing`, `billing`, `billing`, `disable` | PostgreSQL |
| `AWS_REGION`, `AWS_ENDPOINT_URL` | `us-east-1`, — | SQS (o endpoint aponta para o LocalStack em desenvolvimento) |
| `BILLING_COMMANDS_QUEUE`, `OS_EVENTS_QUEUE` | `billing-commands`, `os-events` | Filas da Saga |
| `PAYMENT_PROVIDER_URL` | `http://localhost:8090` | Mercado Pago (`https://api.mercadopago.com`) ou mock |
| `PAYMENT_CLIENT_ID`, `PAYMENT_CLIENT_SECRET` | credenciais do mock | Credenciais do provedor |
| `PAYMENT_NOTIFICATION_URL` | `http://localhost:8083/payments/webhook` | Endereço público do webhook |
| `PAYMENT_TIMEOUT` | `15m` | Prazo para o cliente pagar |
| `JWT_SECRET` | — | Validação do token |

## Como rodar localmente

Pré-requisitos: Docker, e o repositório
[`payment-mock-autoshop`](https://github.com/Jackeline2020/payment-mock-autoshop)
clonado na mesma pasta que este.

```bash
docker compose up --build
curl http://localhost:8083/health
```

O `docker compose` sobe o PostgreSQL (com as migrations), o LocalStack
com as filas (porta `4567`), o mock de pagamento (porta `8090`) e o
serviço (porta `8083`). A pasta `localstack/samples/` tem comandos de
exemplo para os dois cenários de pagamento:

```bash
Q=http://localhost:4566/000000000000
send() { docker compose exec localstack awslocal sqs send-message --queue-url $Q/billing-commands --message-body file:///samples/$1.json; }

send generate-budget && send request-payment      # R$ 350,00: aprovado
send generate-budget-recusado && send request-payment-recusado   # R$ 350,13: recusado

docker compose exec localstack awslocal sqs receive-message --queue-url $Q/os-events --max-number-of-messages 10
```

## Testes

Os testes de integração rodam contra o PostgreSQL e são ignorados quando
`DB_HOST` não está definida. Localmente, usam o banco `billing_test`,
criado junto com o PostgreSQL do compose.

```bash
# unitários
go test ./... -cover

# unitários + integração
docker compose up -d postgres
DB_HOST=localhost DB_PORT=5433 DB_NAME=billing_test go test ./... -coverprofile=coverage.out
./scripts/coverage.sh
```

## Cobertura e qualidade

O pipeline exige **cobertura mínima de 80%** (`scripts/coverage.sh`) e
envia a análise para o SonarCloud.

<!-- Evidências: adicionar print do SonarCloud e link do projeto -->

## CI/CD

Pipeline em `.github/workflows/ci-cd.yml`, executado em todo pull request
e em push para `develop` e `main`:

1. Build e `go vet`
2. Testes unitários e de integração (PostgreSQL no próprio pipeline)
3. Verificação de cobertura mínima
4. SonarCloud
5. Build e publicação da imagem Docker no GitHub Container Registry

As branches `main` e `develop` são protegidas: alterações só entram por
pull request, com as checagens acima aprovadas.
