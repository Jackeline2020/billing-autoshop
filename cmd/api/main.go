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

	"billing-autoshop/internal/handler"
	"billing-autoshop/internal/middleware"
	"billing-autoshop/internal/repository"
	"billing-autoshop/internal/usecase"
	"billing-autoshop/pkg/config"
	"billing-autoshop/pkg/observability"

	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
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

	budgetRepo := repository.NewBudgetPostgresRepository(db)
	budgetUC := usecase.NewBudgetUseCase(budgetRepo)
	budgetHandler := handler.NewBudgetHandler(budgetUC)

	r := gin.New()
	r.Use(gin.Recovery())
	if nrApp != nil {
		r.Use(nrgin.Middleware(nrApp))
	}
	r.Use(middleware.Logger())

	r.GET("/health", handler.HealthCheck)

	protected := r.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/budgets/:order_id", budgetHandler.GetByOrderID)
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
	if nrApp != nil {
		nrApp.Shutdown(5 * time.Second)
	}
}
