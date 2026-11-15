// @title           AutoShop — Billing Service
// @version         1.0
// @description     Orçamento e pagamento das ordens de serviço (Checkout Pro do Mercado Pago ou mock).
// @BasePath        /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Digite: Bearer {seu_token}

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"billing-autoshop/internal/consumer"
	"billing-autoshop/internal/gateway"
	"billing-autoshop/internal/handler"
	"billing-autoshop/internal/messaging"
	"billing-autoshop/internal/middleware"
	"billing-autoshop/internal/notifier"
	"billing-autoshop/internal/repository"
	"billing-autoshop/internal/usecase"
	"billing-autoshop/internal/worker"
	"billing-autoshop/pkg/config"
	"billing-autoshop/pkg/observability"

	_ "billing-autoshop/docs" // documentação Swagger gerada pelo swag

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	nrApp := observability.NewRelicApp()

	db, err := config.NewPostgresPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Filas da Saga: comandos chegam em billing-commands, eventos saem em
	// os-events (ADR-006)
	awsCfg, err := config.LoadAWSConfig(ctx)
	if err != nil {
		log.Fatalf("erro ao carregar configuração da AWS: %v", err)
	}
	sqsClient := sqs.NewFromConfig(awsCfg)
	commandsURL, err := messaging.QueueURL(ctx, sqsClient, config.Env("BILLING_COMMANDS_QUEUE", "billing-commands"))
	if err != nil {
		log.Fatal(err)
	}
	eventsURL, err := messaging.QueueURL(ctx, sqsClient, config.Env("OS_EVENTS_QUEUE", "os-events"))
	if err != nil {
		log.Fatal(err)
	}
	publisher := messaging.NewSQSPublisher(sqsClient, eventsURL)

	budgetRepo := repository.NewBudgetPostgresRepository(db)
	budgetUC := usecase.NewBudgetUseCase(budgetRepo)
	budgetHandler := handler.NewBudgetHandler(budgetUC)

	// Provedor de pagamento: Mercado Pago real ou payment-mock-autoshop,
	// conforme a configuração (ADR-007)
	paymentGateway := gateway.NewMercadoPago(gateway.Config{
		BaseURL:      config.Env("PAYMENT_PROVIDER_URL", "http://localhost:8090"),
		ClientID:     config.Env("PAYMENT_CLIENT_ID", "autoshop-client"),
		ClientSecret: config.Env("PAYMENT_CLIENT_SECRET", "autoshop-secret"),
	})
	paymentTimeout, err := time.ParseDuration(config.Env("PAYMENT_TIMEOUT", "15m"))
	if err != nil {
		log.Fatalf("PAYMENT_TIMEOUT inválido: %v", err)
	}
	paymentUC := usecase.NewPaymentUseCase(
		repository.NewPaymentPostgresRepository(db), budgetUC, paymentGateway, notifier.NewSagaNotifier(publisher),
		usecase.PaymentConfig{
			NotificationURL: config.Env("PAYMENT_NOTIFICATION_URL", "http://localhost:8083/payments/webhook"),
			Timeout:         paymentTimeout,
		},
	)
	paymentHandler := handler.NewPaymentHandler(paymentUC)
	go worker.RunExpiration(ctx, paymentUC, 30*time.Second)

	commandHandler := consumer.NewCommandHandler(budgetUC, paymentUC, publisher)
	commandConsumer := consumer.NewSQSConsumer(sqsClient, commandsURL, commandHandler,
		repository.NewProcessedMessagePostgresRepository(db))
	consumerDone := make(chan struct{})
	go func() {
		commandConsumer.Run(ctx)
		close(consumerDone)
	}()

	r := gin.New()
	r.Use(gin.Recovery())
	if nrApp != nil {
		r.Use(nrgin.Middleware(nrApp))
	}
	r.Use(middleware.Logger())

	r.GET("/health", handler.HealthCheck)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	// Chamado pelo provedor de pagamento: não usa JWT. O aviso só informa o
	// ID, e o status é sempre confirmado no provedor.
	r.POST("/payments/webhook", paymentHandler.Webhook)

	protected := r.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/budgets/:order_id", budgetHandler.GetByOrderID)
		protected.GET("/payments/:order_id", paymentHandler.GetByOrderID)
	}

	srv := &http.Server{
		Addr:              ":" + config.Port(),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("billing-service ouvindo na porta %s", config.Port())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("erro no servidor HTTP: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("encerrando billing-service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("erro ao encerrar o servidor HTTP: %v", err)
	}
	<-consumerDone
	if nrApp != nil {
		nrApp.Shutdown(5 * time.Second)
	}
}
