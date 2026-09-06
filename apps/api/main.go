package main

import (
	activityhandlers "ledgermeadow/src/modules/activity/handlers"
	activityrepo "ledgermeadow/src/modules/activity/repository"
	activityservices "ledgermeadow/src/modules/activity/services"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	// Embeds the zoneinfo database: financial periods call time.LoadLocation with the
	// user's timezone, and the production image has no system tzdata.
	_ "time/tzdata"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/config"
	analyticshandlers "ledgermeadow/src/modules/analytics/handlers"
	analyticsrepo "ledgermeadow/src/modules/analytics/repository"
	analyticsservices "ledgermeadow/src/modules/analytics/services"
	bankinghandlers "ledgermeadow/src/modules/banking/handlers"
	bankingrepo "ledgermeadow/src/modules/banking/repository"
	bankingservices "ledgermeadow/src/modules/banking/services"
	billhandlers "ledgermeadow/src/modules/bills/handlers"
	billrepo "ledgermeadow/src/modules/bills/repository"
	billservices "ledgermeadow/src/modules/bills/services"
	budgethandlers "ledgermeadow/src/modules/budgets/handlers"
	budgetrepo "ledgermeadow/src/modules/budgets/repository"
	budgetservices "ledgermeadow/src/modules/budgets/services"
	cashflowhandlers "ledgermeadow/src/modules/cashflow/handlers"
	cashflowrepo "ledgermeadow/src/modules/cashflow/repository"
	cashflowservices "ledgermeadow/src/modules/cashflow/services"
	categoryhandlers "ledgermeadow/src/modules/categories/handlers"
	categoryrepo "ledgermeadow/src/modules/categories/repository"
	categoryservices "ledgermeadow/src/modules/categories/services"
	dashboardhandlers "ledgermeadow/src/modules/dashboard/handlers"
	dashboardrepo "ledgermeadow/src/modules/dashboard/repository"
	dashboardservices "ledgermeadow/src/modules/dashboard/services"
	expensesplithandlers "ledgermeadow/src/modules/expensesplits/handlers"
	expensesplitrepo "ledgermeadow/src/modules/expensesplits/repository"
	expensesplitservices "ledgermeadow/src/modules/expensesplits/services"
	goalhandlers "ledgermeadow/src/modules/goals/handlers"
	goalrepo "ledgermeadow/src/modules/goals/repository"
	goalservices "ledgermeadow/src/modules/goals/services"
	householdhandlers "ledgermeadow/src/modules/household/handlers"
	householdrepo "ledgermeadow/src/modules/household/repository"
	householdservices "ledgermeadow/src/modules/household/services"
	inboxhandlers "ledgermeadow/src/modules/inbox/handlers"
	inboxrepo "ledgermeadow/src/modules/inbox/repository"
	inboxservices "ledgermeadow/src/modules/inbox/services"
	loanhandlers "ledgermeadow/src/modules/loans/handlers"
	loanrepo "ledgermeadow/src/modules/loans/repository"
	loanservices "ledgermeadow/src/modules/loans/services"
	manualassethandlers "ledgermeadow/src/modules/manualassets/handlers"
	manualassetrepo "ledgermeadow/src/modules/manualassets/repository"
	manualassetservices "ledgermeadow/src/modules/manualassets/services"
	manualliabilityhandlers "ledgermeadow/src/modules/manualliabilities/handlers"
	manualliabilityrepo "ledgermeadow/src/modules/manualliabilities/repository"
	manualliabilityservices "ledgermeadow/src/modules/manualliabilities/services"
	materializationrepo "ledgermeadow/src/modules/materialization/repository"
	materializationservices "ledgermeadow/src/modules/materialization/services"
	networthhandlers "ledgermeadow/src/modules/networth/handlers"
	networthrepo "ledgermeadow/src/modules/networth/repository"
	networthservices "ledgermeadow/src/modules/networth/services"
	notificationhandlers "ledgermeadow/src/modules/notifications/handlers"
	notificationrepo "ledgermeadow/src/modules/notifications/repository"
	notificationservices "ledgermeadow/src/modules/notifications/services"
	planninghandlers "ledgermeadow/src/modules/planning/handlers"
	planningrepo "ledgermeadow/src/modules/planning/repository"
	planningservices "ledgermeadow/src/modules/planning/services"
	"ledgermeadow/src/modules/platform/changes"
	"ledgermeadow/src/modules/platform/financialengine"
	"ledgermeadow/src/modules/platform/jobs"
	"ledgermeadow/src/modules/platform/plaid"
	"ledgermeadow/src/modules/platform/postgres"
	"ledgermeadow/src/modules/platform/secrets"
	recurringincomehandlers "ledgermeadow/src/modules/recurringincome/handlers"
	recurringincomerepo "ledgermeadow/src/modules/recurringincome/repository"
	recurringincomeservices "ledgermeadow/src/modules/recurringincome/services"
	rulehandlers "ledgermeadow/src/modules/rules/handlers"
	rulerepo "ledgermeadow/src/modules/rules/repository"
	ruleservices "ledgermeadow/src/modules/rules/services"
	searchhandlers "ledgermeadow/src/modules/search/handlers"
	searchrepo "ledgermeadow/src/modules/search/repository"
	searchservices "ledgermeadow/src/modules/search/services"
	spacehandlers "ledgermeadow/src/modules/spaces/handlers"
	spacerepo "ledgermeadow/src/modules/spaces/repository"
	spaceservices "ledgermeadow/src/modules/spaces/services"
	subscriptionhandlers "ledgermeadow/src/modules/subscriptions/handlers"
	subscriptionrepo "ledgermeadow/src/modules/subscriptions/repository"
	subscriptionservices "ledgermeadow/src/modules/subscriptions/services"
	timelinehandlers "ledgermeadow/src/modules/timeline/handlers"
	timelinerepo "ledgermeadow/src/modules/timeline/repository"
	timelineservices "ledgermeadow/src/modules/timeline/services"
	transactionanalysisrepo "ledgermeadow/src/modules/transactionanalysis/repository"
	transactionanalysisservices "ledgermeadow/src/modules/transactionanalysis/services"
	transactionimporthandlers "ledgermeadow/src/modules/transactionimports/handlers"
	transactionimportrepo "ledgermeadow/src/modules/transactionimports/repository"
	transactionimportservices "ledgermeadow/src/modules/transactionimports/services"
	transactionhandlers "ledgermeadow/src/modules/transactions/handlers"
	transactionrepo "ledgermeadow/src/modules/transactions/repository"
	transactionservices "ledgermeadow/src/modules/transactions/services"
	userrepo "ledgermeadow/src/modules/users/repository"
	"ledgermeadow/src/router"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("application stopped", "operation", "startup", "error_code", "APPLICATION_STOPPED")
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	appConfig, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := postgres.Open(ctx, appConfig.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()
	tokenCipher, err := secrets.NewCipherFromFile(appConfig.TokenKeyFile)
	if err != nil {
		return err
	}

	plaidClient, err := plaid.NewClient(appConfig.PlaidClientID, appConfig.PlaidSecret, appConfig.PlaidEnvironment)
	if err != nil {
		return err
	}
	financialEngineClient, err := financialengine.NewClient(appConfig.FinancialEngineAddr)
	if err != nil {
		return err
	}
	defer financialEngineClient.Close()
	userRepository := userrepo.New(database)
	bankingRepository := bankingrepo.New(database)
	transactionRepository := transactionrepo.New(database)
	expenseSplitService := expensesplitservices.New(expensesplitrepo.New(database))
	categoryService := categoryservices.New(categoryrepo.New(database))
	analyticsService := analyticsservices.New(analyticsrepo.New(database))
	cashFlowService := cashflowservices.New(cashflowrepo.New(database))
	timelineService := timelineservices.New(timelinerepo.New(database))
	spaceService := spaceservices.New(spacerepo.New(database))
	billService := billservices.New(billrepo.New(database))
	subscriptionService := subscriptionservices.New(subscriptionrepo.New(database), financialEngineClient)
	recurringIncomeService := recurringincomeservices.New(recurringincomerepo.New(database))
	budgetService := budgetservices.New(budgetrepo.New(database))
	goalService := goalservices.New(goalrepo.New(database))
	ruleService := ruleservices.New(rulerepo.New(database))
	searchService := searchservices.New(searchrepo.New(database))
	planningService := planningservices.New(planningrepo.New(database), financialEngineClient)
	dashboardService := dashboardservices.New(dashboardrepo.New(database))
	activityService := activityservices.New(activityrepo.New(database))
	inboxService := inboxservices.New(inboxrepo.New(database))
	householdService := householdservices.New(householdrepo.New(database))
	notificationService := notificationservices.New(notificationrepo.New(database))
	manualAssetService := manualassetservices.New(manualassetrepo.New(database))
	loanService := loanservices.New(loanrepo.New(database))
	manualLiabilityService := manualliabilityservices.New(manualliabilityrepo.New(database))
	netWorthService := networthservices.New(networthrepo.New(database))
	bankingService := bankingservices.New(
		plaidClient, tokenCipher, bankingRepository, appConfig.PlaidWebhookURL, appConfig.PlaidRedirectURI,
	)
	syncService := bankingservices.NewSyncService(plaidClient, tokenCipher, bankingRepository, transactionRepository)
	materializationService := materializationservices.New(materializationrepo.New(database), financialEngineClient)
	transactionAnalysisService := transactionanalysisservices.New(transactionanalysisrepo.New(database), financialEngineClient)
	transactionService := transactionservices.New(transactionRepository)
	transactionImportService := transactionimportservices.New(transactionimportrepo.New(database))
	authMiddleware := auth.NewMiddleware(appConfig.ClerkSecretKey, appConfig.ClerkAuthorizedParties, userRepository)

	jobRepository := jobs.NewRepository(database)
	if err := jobRepository.EnqueueOutstandingAnalysis(ctx); err != nil {
		return err
	}
	worker := jobs.NewWorker(
		jobRepository, syncService, transactionAnalysisService, materializationService,
		logger.With("component", "worker"), appConfig.WorkerPollInterval,
	)
	go worker.Run(ctx)
	changeHub := changes.NewHub()
	changeListener, err := changes.NewListener(
		appConfig.DatabaseDirectURL, changeHub, logger.With("component", "change_listener"),
	)
	if err != nil {
		return err
	}
	go changeListener.Run(ctx)

	handler := router.New(
		authMiddleware,
		bankinghandlers.New(bankingService),
		transactionimporthandlers.New(transactionImportService),
		transactionhandlers.New(transactionService),
		expensesplithandlers.New(expenseSplitService),
		categoryhandlers.New(categoryService),
		analyticshandlers.New(analyticsService),
		cashflowhandlers.New(cashFlowService),
		timelinehandlers.New(timelineService),
		spacehandlers.New(spaceService),
		billhandlers.New(billService),
		subscriptionhandlers.New(subscriptionService),
		recurringincomehandlers.New(recurringIncomeService),
		budgethandlers.New(budgetService),
		goalhandlers.New(goalService),
		rulehandlers.New(ruleService),
		searchhandlers.New(searchService),
		planninghandlers.New(planningService),
		dashboardhandlers.New(dashboardService),
		activityhandlers.New(activityService),
		inboxhandlers.New(inboxService),
		householdhandlers.New(householdService),
		notificationhandlers.New(notificationService),
		manualassethandlers.New(manualAssetService),
		loanhandlers.New(loanService),
		manualliabilityhandlers.New(manualLiabilityService),
		networthhandlers.New(netWorthService),
		plaid.NewWebhookHandler(plaidClient, bankingRepository),
		changes.NewHandler(changeHub),
		appConfig.WebOrigin,
		logger,
	)
	server := &http.Server{
		Addr: appConfig.HTTPAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("api listening", "operation", "startup", "address", appConfig.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
