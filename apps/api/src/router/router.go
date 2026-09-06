package router

import (
	activityhandlers "ledgermeadow/src/modules/activity/handlers"
	"log/slog"
	"net/http"
	"time"

	"ledgermeadow/src/auth"
	analyticshandlers "ledgermeadow/src/modules/analytics/handlers"
	bankinghandlers "ledgermeadow/src/modules/banking/handlers"
	billhandlers "ledgermeadow/src/modules/bills/handlers"
	budgethandlers "ledgermeadow/src/modules/budgets/handlers"
	cashflowhandlers "ledgermeadow/src/modules/cashflow/handlers"
	categoryhandlers "ledgermeadow/src/modules/categories/handlers"
	dashboardhandlers "ledgermeadow/src/modules/dashboard/handlers"
	expensesplithandlers "ledgermeadow/src/modules/expensesplits/handlers"
	goalhandlers "ledgermeadow/src/modules/goals/handlers"
	householdhandlers "ledgermeadow/src/modules/household/handlers"
	inboxhandlers "ledgermeadow/src/modules/inbox/handlers"
	loanhandlers "ledgermeadow/src/modules/loans/handlers"
	manualassethandlers "ledgermeadow/src/modules/manualassets/handlers"
	manualliabilityhandlers "ledgermeadow/src/modules/manualliabilities/handlers"
	networthhandlers "ledgermeadow/src/modules/networth/handlers"
	notificationhandlers "ledgermeadow/src/modules/notifications/handlers"
	planninghandlers "ledgermeadow/src/modules/planning/handlers"
	recurringincomehandlers "ledgermeadow/src/modules/recurringincome/handlers"
	rulehandlers "ledgermeadow/src/modules/rules/handlers"
	searchhandlers "ledgermeadow/src/modules/search/handlers"
	spacehandlers "ledgermeadow/src/modules/spaces/handlers"
	subscriptionhandlers "ledgermeadow/src/modules/subscriptions/handlers"
	timelinehandlers "ledgermeadow/src/modules/timeline/handlers"
	transactionimporthandlers "ledgermeadow/src/modules/transactionimports/handlers"
	transactionhandlers "ledgermeadow/src/modules/transactions/handlers"
	"ledgermeadow/src/shared/httpx"
)

func New(
	authMiddleware *auth.Middleware,
	banking *bankinghandlers.Handler,
	transactionImports *transactionimporthandlers.Handler,
	transactions *transactionhandlers.Handler,
	expenseSplits *expensesplithandlers.Handler,
	categories *categoryhandlers.Handler,
	analytics *analyticshandlers.Handler,
	cashFlow *cashflowhandlers.Handler,
	timeline *timelinehandlers.Handler,
	spaces *spacehandlers.Handler,
	bills *billhandlers.Handler,
	subscriptions *subscriptionhandlers.Handler,
	recurringIncome *recurringincomehandlers.Handler,
	budgets *budgethandlers.Handler,
	goals *goalhandlers.Handler,
	rules *rulehandlers.Handler,
	search *searchhandlers.Handler,
	planning *planninghandlers.Handler,
	dashboard *dashboardhandlers.Handler,
	activity *activityhandlers.Handler,
	inbox *inboxhandlers.Handler,
	household *householdhandlers.Handler,
	notifications *notificationhandlers.Handler,
	manualAssets *manualassethandlers.Handler,
	loans *loanhandlers.Handler,
	manualLiabilities *manualliabilityhandlers.Handler,
	netWorth *networthhandlers.Handler,
	plaidWebhook http.Handler,
	changeStream http.Handler,
	webOrigin string,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("POST /api/v1/webhooks/plaid", plaidWebhook)
	mux.Handle("GET /api/v1/changes", authMiddleware.RequireUser(changeStream))
	mux.Handle("POST /api/v1/bank-connections/link-token", authMiddleware.RequireUser(http.HandlerFunc(banking.CreateLinkToken)))
	mux.Handle("POST /api/v1/bank-connections/exchange", authMiddleware.RequireUser(http.HandlerFunc(banking.Exchange)))
	mux.Handle("GET /api/v1/bank-connections", authMiddleware.RequireUser(http.HandlerFunc(banking.ConnectionStatus)))
	mux.Handle("DELETE /api/v1/bank-connections/{id}", authMiddleware.RequireUser(http.HandlerFunc(banking.Disconnect)))
	mux.Handle("GET /api/v1/accounts", authMiddleware.RequireUser(http.HandlerFunc(banking.Accounts)))
	mux.Handle("GET /api/v1/accounts/{id}", authMiddleware.RequireUser(http.HandlerFunc(banking.AccountDetail)))
	mux.Handle("POST /api/v1/bank-connections/{id}/refresh", authMiddleware.RequireUser(http.HandlerFunc(banking.Refresh)))
	mux.Handle("POST /api/v1/bank-connections/{id}/link-token", authMiddleware.RequireUser(http.HandlerFunc(banking.CreateUpdateLinkToken)))
	mux.Handle("POST /api/v1/imports/rbc-csv/preview", authMiddleware.RequireUser(http.HandlerFunc(transactionImports.Preview)))
	mux.Handle("POST /api/v1/imports/rbc-csv", authMiddleware.RequireUser(http.HandlerFunc(transactionImports.Import)))
	mux.Handle("GET /api/v1/transactions", authMiddleware.RequireUser(http.HandlerFunc(transactions.List)))
	mux.Handle("PATCH /api/v1/transactions/bulk", authMiddleware.RequireUser(http.HandlerFunc(transactions.BulkUpdate)))
	mux.Handle("GET /api/v1/transactions/{id}", authMiddleware.RequireUser(http.HandlerFunc(transactions.Detail)))
	mux.Handle("PATCH /api/v1/transactions/{id}", authMiddleware.RequireUser(http.HandlerFunc(transactions.Update)))
	mux.Handle("GET /api/v1/transactions/{id}/split", authMiddleware.RequireUser(http.HandlerFunc(expenseSplits.Get)))
	mux.Handle("PUT /api/v1/transactions/{id}/split", authMiddleware.RequireUser(http.HandlerFunc(expenseSplits.Replace)))
	mux.Handle("DELETE /api/v1/transactions/{id}/split", authMiddleware.RequireUser(http.HandlerFunc(expenseSplits.Clear)))
	mux.Handle("GET /api/v1/categories", authMiddleware.RequireUser(http.HandlerFunc(categories.List)))
	mux.Handle("POST /api/v1/categories", authMiddleware.RequireUser(http.HandlerFunc(categories.Create)))
	mux.Handle("GET /api/v1/analytics", authMiddleware.RequireUser(http.HandlerFunc(analytics.Get)))
	mux.Handle("GET /api/v1/cash-flow", authMiddleware.RequireUser(http.HandlerFunc(cashFlow.Get)))
	mux.Handle("GET /api/v1/timeline", authMiddleware.RequireUser(http.HandlerFunc(timeline.Get)))
	mux.Handle("GET /api/v1/spaces", authMiddleware.RequireUser(http.HandlerFunc(spaces.List)))
	mux.Handle("POST /api/v1/spaces", authMiddleware.RequireUser(http.HandlerFunc(spaces.Create)))
	mux.Handle("GET /api/v1/bills", authMiddleware.RequireUser(http.HandlerFunc(bills.List)))
	mux.Handle("POST /api/v1/bills", authMiddleware.RequireUser(http.HandlerFunc(bills.Create)))
	mux.Handle("GET /api/v1/bills/{id}", authMiddleware.RequireUser(http.HandlerFunc(bills.Detail)))
	mux.Handle("PATCH /api/v1/bills/{id}", authMiddleware.RequireUser(http.HandlerFunc(bills.Update)))
	mux.Handle("GET /api/v1/subscriptions", authMiddleware.RequireUser(http.HandlerFunc(subscriptions.List)))
	mux.Handle("POST /api/v1/subscriptions", authMiddleware.RequireUser(http.HandlerFunc(subscriptions.Create)))
	mux.Handle("GET /api/v1/subscriptions/{id}", authMiddleware.RequireUser(http.HandlerFunc(subscriptions.Detail)))
	mux.Handle("PATCH /api/v1/subscriptions/{id}", authMiddleware.RequireUser(http.HandlerFunc(subscriptions.Update)))
	mux.Handle("GET /api/v1/recurring-income", authMiddleware.RequireUser(http.HandlerFunc(recurringIncome.List)))
	mux.Handle("GET /api/v1/recurring-income/{id}", authMiddleware.RequireUser(http.HandlerFunc(recurringIncome.Detail)))
	mux.Handle("PATCH /api/v1/recurring-income/{id}", authMiddleware.RequireUser(http.HandlerFunc(recurringIncome.Update)))
	mux.Handle("GET /api/v1/budgets", authMiddleware.RequireUser(http.HandlerFunc(budgets.List)))
	mux.Handle("POST /api/v1/budgets", authMiddleware.RequireUser(http.HandlerFunc(budgets.Create)))
	mux.Handle("GET /api/v1/goals", authMiddleware.RequireUser(http.HandlerFunc(goals.List)))
	mux.Handle("POST /api/v1/goals", authMiddleware.RequireUser(http.HandlerFunc(goals.Create)))
	mux.Handle("GET /api/v1/rules", authMiddleware.RequireUser(http.HandlerFunc(rules.List)))
	mux.Handle("POST /api/v1/rules", authMiddleware.RequireUser(http.HandlerFunc(rules.Create)))
	mux.Handle("PATCH /api/v1/rules/{id}", authMiddleware.RequireUser(http.HandlerFunc(rules.Update)))
	mux.Handle("DELETE /api/v1/rules/{id}", authMiddleware.RequireUser(http.HandlerFunc(rules.Delete)))
	mux.Handle("POST /api/v1/rules/{id}/duplicate", authMiddleware.RequireUser(http.HandlerFunc(rules.Duplicate)))
	mux.Handle("PUT /api/v1/rules/reorder", authMiddleware.RequireUser(http.HandlerFunc(rules.Reorder)))
	mux.Handle("GET /api/v1/search", authMiddleware.RequireUser(http.HandlerFunc(search.Prefix)))
	mux.Handle("GET /api/v1/planning", authMiddleware.RequireUser(http.HandlerFunc(planning.Get)))
	mux.Handle("GET /api/v1/dashboard", authMiddleware.RequireUser(http.HandlerFunc(dashboard.Get)))
	mux.Handle("GET /api/v1/activity-summary", authMiddleware.RequireUser(http.HandlerFunc(activity.Get)))
	mux.Handle("GET /api/v1/inbox", authMiddleware.RequireUser(http.HandlerFunc(inbox.List)))
	mux.Handle("POST /api/v1/inbox/{id}/resolve", authMiddleware.RequireUser(http.HandlerFunc(inbox.Resolve)))
	mux.Handle("GET /api/v1/household", authMiddleware.RequireUser(http.HandlerFunc(household.Get)))
	mux.Handle("POST /api/v1/household", authMiddleware.RequireUser(http.HandlerFunc(household.Create)))
	mux.Handle("GET /api/v1/notifications", authMiddleware.RequireUser(http.HandlerFunc(notifications.List)))
	mux.Handle("PATCH /api/v1/notifications/{id}/read", authMiddleware.RequireUser(http.HandlerFunc(notifications.MarkRead)))
	mux.Handle("GET /api/v1/notification-preferences", authMiddleware.RequireUser(http.HandlerFunc(notifications.Preferences)))
	mux.Handle("PATCH /api/v1/notification-preferences", authMiddleware.RequireUser(http.HandlerFunc(notifications.UpdatePreference)))
	mux.Handle("GET /api/v1/manual-assets", authMiddleware.RequireUser(http.HandlerFunc(manualAssets.List)))
	mux.Handle("POST /api/v1/manual-assets", authMiddleware.RequireUser(http.HandlerFunc(manualAssets.Create)))
	mux.Handle("GET /api/v1/loans", authMiddleware.RequireUser(http.HandlerFunc(loans.List)))
	mux.Handle("POST /api/v1/loans", authMiddleware.RequireUser(http.HandlerFunc(loans.Create)))
	mux.Handle("POST /api/v1/loans/{id}/scenario", authMiddleware.RequireUser(http.HandlerFunc(loans.Scenario)))
	mux.Handle("GET /api/v1/loans/{id}", authMiddleware.RequireUser(http.HandlerFunc(loans.Detail)))
	mux.Handle("PATCH /api/v1/loans/{id}", authMiddleware.RequireUser(http.HandlerFunc(loans.Update)))
	mux.Handle("GET /api/v1/manual-liabilities", authMiddleware.RequireUser(http.HandlerFunc(manualLiabilities.List)))
	mux.Handle("POST /api/v1/manual-liabilities", authMiddleware.RequireUser(http.HandlerFunc(manualLiabilities.Create)))
	mux.Handle("GET /api/v1/net-worth", authMiddleware.RequireUser(http.HandlerFunc(netWorth.Get)))

	return privacyHeaders(requestLogging(logger, cors(webOrigin, mux)))
}

func privacyHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(response, request)
	})
}

func cors(webOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin != "" && origin != webOrigin {
			httpx.WriteError(response, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Request origin is not allowed.")
			return
		}
		if origin == webOrigin {
			response.Header().Set("Access-Control-Allow-Origin", webOrigin)
			response.Header().Set("Vary", "Origin")
			response.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			response.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodOptions || request.URL.Path == "/health" || request.URL.Path == "/api/v1/changes" {
			next.ServeHTTP(response, request)
			return
		}
		started := time.Now()
		loggedResponse := &statusResponseWriter{ResponseWriter: response}
		next.ServeHTTP(loggedResponse, request)
		if loggedResponse.status == 0 {
			loggedResponse.status = http.StatusOK
		}
		level := slog.LevelInfo
		switch {
		case loggedResponse.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case loggedResponse.status >= http.StatusBadRequest:
			level = slog.LevelWarn
		case request.Method == http.MethodGet || request.Method == http.MethodHead:
			level = slog.LevelDebug
		}
		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		logger.Log(request.Context(), level, "http request completed",
			"operation", "http_request", "method", request.Method,
			"route", route, "status", loggedResponse.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
