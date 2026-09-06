package services

import (
	"context"
	"errors"
	"testing"
	"time"

	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	"ledgermeadow/src/modules/transactionanalysis/models"
	shared "ledgermeadow/src/shared/types"
)

const (
	testIncomeCategoryID = "00000000-0000-4000-8000-000000000030"
	testBillCategoryID   = "00000000-0000-4000-8000-000000000031"
	testLearningKey      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type recordingRepository struct {
	input         models.Input
	signals       []models.LearningSignal
	requestedKeys []string
	result        models.Result
	applied       bool
}

func (repository *recordingRepository) Load(context.Context, shared.UserID, time.Time) (models.Input, error) {
	return repository.input, nil
}

func (repository *recordingRepository) LoadLearningSignals(_ context.Context, _ shared.UserID, keys []string) ([]models.LearningSignal, error) {
	repository.requestedKeys = append([]string(nil), keys...)
	return repository.signals, nil
}

func (repository *recordingRepository) Apply(_ context.Context, _ shared.UserID, result models.Result) error {
	repository.result = result
	repository.applied = true
	return nil
}

type analysisEngine struct {
	requests             int
	secondRequestSignals []*financialenginepb.CategorizationSignal
	unauthorizedCategory bool
	recurringEvidence    []*financialenginepb.CategoryRecurrenceEvidence
	prevailingFrequency  financialenginepb.Frequency
}

func (engine *analysisEngine) CategorizeTransactions(_ context.Context, request *financialenginepb.CategorizeTransactionsRequest) (*financialenginepb.CategorizeTransactionsResponse, error) {
	engine.requests++
	if engine.requests == 2 {
		engine.secondRequestSignals = request.Signals
	}
	categorizations := make([]*financialenginepb.Categorization, 0, len(request.Transactions))
	for index, transaction := range request.Transactions {
		categoryID := testIncomeCategoryID
		if engine.unauthorizedCategory {
			categoryID = "00000000-0000-4000-8000-000000000099"
		}
		confidence := uint32(7000)
		if len(request.Signals) > 0 && index == 0 {
			confidence = 9000
		}
		categorizations = append(categorizations, &financialenginepb.Categorization{
			TransactionId: transaction.TransactionId, CategoryId: categoryID,
			LearningKey: testLearningKey, ConfidenceBasisPoints: confidence,
			Reason: "personal_history",
		})
	}
	return &financialenginepb.CategorizeTransactionsResponse{Categorizations: categorizations}, nil
}

func (engine *analysisEngine) DetectRecurring(_ context.Context, request *financialenginepb.DetectRecurringRequest) (*financialenginepb.DetectRecurringResponse, error) {
	engine.recurringEvidence = request.CategoryEvidence
	engine.prevailingFrequency = request.PrevailingFrequency
	categoryID := testIncomeCategoryID
	return &financialenginepb.DetectRecurringResponse{Candidates: []*financialenginepb.RecurringCandidate{{
		DetectionKey: "CAD:INCOME:employer", Name: "Employer payroll",
		Kind:                  financialenginepb.RecurringKind_RECURRING_KIND_INCOME,
		Currency:              financialenginepb.Currency_CURRENCY_CAD,
		Frequency:             financialenginepb.Frequency_FREQUENCY_BIWEEKLY,
		Evidence:              financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_CADENCE,
		ExpectedAmountMinor:   200000,
		NextExpectedAt:        &financialenginepb.Date{Year: 2026, Month: 8, Day: 28},
		ConfidenceBasisPoints: 9000, OccurrenceCount: 3,
		AccountId: request.Transactions[0].AccountId, CategoryId: &categoryID,
		SupportingTransactionIds: []string{
			request.Transactions[2].TransactionId,
			request.Transactions[1].TransactionId,
			request.Transactions[0].TransactionId,
		},
		Explanation: "three biweekly deposits",
	}}}, nil
}

func TestAnalyzeUsesPersonalAndGlobalSignalsBeforeApplyingCategories(t *testing.T) {
	asOf := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	billHint := "BILL"
	repository := &recordingRepository{input: models.Input{
		AsOf: asOf,
		Categories: []models.Category{
			{ID: testIncomeCategoryID, Name: "Employment", Type: "INCOME"},
			{ID: testBillCategoryID, Name: "Utilities", Type: "EXPENSE", RecurringKindHint: &billHint},
		},
		CategoryEvidence:    []models.CategoryRecurrenceEvidence{{CategoryID: testIncomeCategoryID, SubscriptionCount: 2, BillCount: 1, ConfirmedSubscriptionCount: 1}},
		PrevailingFrequency: "BIWEEKLY",
	}, signals: []models.LearningSignal{
		{LearningKey: testLearningKey, CategoryID: testIncomeCategoryID, PersonalObservationCount: 1},
		{LearningKey: testLearningKey, CategoryID: testIncomeCategoryID, GlobalUserCount: 4, GlobalTotalContributorCount: 5},
	}}
	transactionIDs := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
	}
	dates := []time.Time{
		time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
	}
	for index, transactionID := range transactionIDs {
		repository.input.Transactions = append(repository.input.Transactions, models.Transaction{
			ID: transactionID, AccountID: "00000000-0000-4000-8000-000000000010",
			Name: "Employer payroll", AmountMinor: 200000, Currency: "CAD",
			Date: dates[index], AccountKind: "ASSET", CategorySource: "UNASSIGNED",
		})
	}
	engine := &analysisEngine{}
	service := New(repository, engine)
	service.now = func() time.Time { return asOf }
	if err := service.Analyze(context.Background(), shared.UserID("00000000-0000-4000-8000-000000000020")); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if engine.requests != 2 || len(repository.requestedKeys) != 1 || repository.requestedKeys[0] != testLearningKey {
		t.Fatalf("learning pass = requests %d, keys %#v", engine.requests, repository.requestedKeys)
	}
	if len(engine.secondRequestSignals) != 1 || engine.secondRequestSignals[0].PersonalObservationCount != 1 ||
		engine.secondRequestSignals[0].GlobalUserCount != 4 ||
		engine.secondRequestSignals[0].GlobalTotalContributorCount != 5 {
		t.Fatalf("combined learning signals = %#v", engine.secondRequestSignals)
	}
	if len(repository.result.Categories) != 1 || repository.result.Categories[0].CategoryID != testIncomeCategoryID {
		t.Fatalf("automatic categories = %#v", repository.result.Categories)
	}
	if len(repository.result.LearningKeys) != 3 || len(repository.result.CategoryReviews) != 2 {
		t.Fatalf("learning/reviews = %#v / %#v", repository.result.LearningKeys, repository.result.CategoryReviews)
	}
	if len(repository.result.Recurring) != 1 {
		t.Fatalf("recurring candidates = %#v", repository.result.Recurring)
	}
	if len(engine.recurringEvidence) != 2 || engine.recurringEvidence[0].CategoryId != testIncomeCategoryID ||
		engine.recurringEvidence[0].ConfirmedSubscriptionCount != 1 ||
		engine.recurringEvidence[1].CategoryId != testBillCategoryID ||
		engine.recurringEvidence[1].KindHint != financialenginepb.RecurringKind_RECURRING_KIND_BILL ||
		engine.prevailingFrequency != financialenginepb.Frequency_FREQUENCY_BIWEEKLY {
		t.Fatalf("recurring evidence request = %#v / %v", engine.recurringEvidence, engine.prevailingFrequency)
	}
	candidate := repository.result.Recurring[0]
	if candidate.Kind != "INCOME" || candidate.Status != "ACTIVE" ||
		candidate.Frequency != "BIWEEKLY" || candidate.CategoryID != testIncomeCategoryID {
		t.Fatalf("recurring candidate = %#v", candidate)
	}
}

func TestAnalyzeRejectsUnauthorizedEngineCategory(t *testing.T) {
	repository := &recordingRepository{input: models.Input{
		AsOf:       time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
		Categories: []models.Category{{ID: testIncomeCategoryID, Name: "Employment", Type: "INCOME"}},
		Transactions: []models.Transaction{{
			ID:        "00000000-0000-4000-8000-000000000001",
			AccountID: "00000000-0000-4000-8000-000000000010",
			Name:      "Employer payroll", AmountMinor: 200000, Currency: "CAD",
			Date:        time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
			AccountKind: "ASSET", CategorySource: "UNASSIGNED",
		}},
	}}
	service := New(repository, &analysisEngine{unauthorizedCategory: true})
	err := service.Analyze(context.Background(), shared.UserID("00000000-0000-4000-8000-000000000020"))
	if !errors.Is(err, ErrEngineResponse) {
		t.Fatalf("unauthorized category error = %v", err)
	}
	if repository.applied {
		t.Fatal("unauthorized engine category was persisted")
	}
}

func TestRecurringCandidatesKeepLearnedCategoryAsUnknown(t *testing.T) {
	transactionID := "00000000-0000-4000-8000-000000000001"
	categoryID := "00000000-0000-4000-8000-000000000040"
	response := &financialenginepb.DetectRecurringResponse{Candidates: []*financialenginepb.RecurringCandidate{{
		DetectionKey: "CAD:outflow:new-merchant", Name: "New merchant",
		Kind:                  financialenginepb.RecurringKind_RECURRING_KIND_SUBSCRIPTION,
		Currency:              financialenginepb.Currency_CURRENCY_CAD,
		Frequency:             financialenginepb.Frequency_FREQUENCY_MONTHLY,
		ExpectedAmountMinor:   2_000,
		NextExpectedAt:        &financialenginepb.Date{Year: 2026, Month: 9, Day: 1},
		ConfidenceBasisPoints: 3_333, OccurrenceCount: 1,
		AccountId: "00000000-0000-4000-8000-000000000010", CategoryId: &categoryID,
		SupportingTransactionIds: []string{transactionID},
		Explanation:              "learned category evidence", Evidence: financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_LEARNED_CATEGORY,
	}}}

	candidates, err := recurringCandidates(
		response,
		map[string]*financialenginepb.AnalysisTransaction{
			transactionID: {TransactionId: transactionID, Currency: financialenginepb.Currency_CURRENCY_CAD},
		},
		map[string]models.Category{categoryID: {ID: categoryID, Name: "Subscriptions", Type: "EXPENSE"}},
	)
	if err != nil {
		t.Fatalf("recurring candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Status != "UNKNOWN" || candidates[0].OccurrenceCount != 1 {
		t.Fatalf("learned recurring candidates = %#v", candidates)
	}
}

func TestRecurringCandidatesCarryAmountChangeObservation(t *testing.T) {
	supportingIDs := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
	}
	observationIDs := []string{
		"00000000-0000-4000-8000-000000000004",
		"00000000-0000-4000-8000-000000000005",
	}
	observedAmount := int64(1_487)
	response := &financialenginepb.DetectRecurringResponse{Candidates: []*financialenginepb.RecurringCandidate{{
		DetectionKey: "CAD:outflow:merchant", Name: "Merchant",
		Kind:                  financialenginepb.RecurringKind_RECURRING_KIND_SUBSCRIPTION,
		Currency:              financialenginepb.Currency_CURRENCY_CAD,
		Frequency:             financialenginepb.Frequency_FREQUENCY_MONTHLY,
		ExpectedAmountMinor:   2_570,
		NextExpectedAt:        &financialenginepb.Date{Year: 2026, Month: 9, Day: 20},
		ConfidenceBasisPoints: 9_000, OccurrenceCount: 3,
		SupportingTransactionIds:        supportingIDs,
		ObservedAmountMinor:             &observedAmount,
		AmountObservationTransactionIds: observationIDs,
		Evidence:                        financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_CADENCE,
	}}}
	byID := make(map[string]*financialenginepb.AnalysisTransaction, len(supportingIDs)+len(observationIDs))
	for _, transactionID := range append(supportingIDs, observationIDs...) {
		byID[transactionID] = &financialenginepb.AnalysisTransaction{
			TransactionId: transactionID,
			Currency:      financialenginepb.Currency_CURRENCY_CAD,
		}
	}

	candidates, err := recurringCandidates(response, byID, nil)
	if err != nil {
		t.Fatalf("recurring candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ObservedAmountMinor == nil ||
		*candidates[0].ObservedAmountMinor != observedAmount ||
		len(candidates[0].AmountObservationIDs) != len(observationIDs) {
		t.Fatalf("amount observation candidate = %#v", candidates)
	}
}

func TestRecurringCandidatesKeepThreeOccurrenceConstraintForIncome(t *testing.T) {
	transactionIDs := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
	}
	response := &financialenginepb.DetectRecurringResponse{Candidates: []*financialenginepb.RecurringCandidate{{
		DetectionKey: "CAD:income:employer", Name: "Employer",
		Kind:                  financialenginepb.RecurringKind_RECURRING_KIND_INCOME,
		Currency:              financialenginepb.Currency_CURRENCY_CAD,
		Frequency:             financialenginepb.Frequency_FREQUENCY_BIWEEKLY,
		ExpectedAmountMinor:   200_000,
		NextExpectedAt:        &financialenginepb.Date{Year: 2026, Month: 9, Day: 1},
		ConfidenceBasisPoints: 7_500, OccurrenceCount: 2,
		SupportingTransactionIds: transactionIDs,
		Evidence:                 financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_CADENCE,
	}}}
	byID := make(map[string]*financialenginepb.AnalysisTransaction, len(transactionIDs))
	for _, transactionID := range transactionIDs {
		byID[transactionID] = &financialenginepb.AnalysisTransaction{
			TransactionId: transactionID,
			Currency:      financialenginepb.Currency_CURRENCY_CAD,
		}
	}

	candidates, err := recurringCandidates(response, byID, nil)
	if err != nil {
		t.Fatalf("recurring candidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("two-occurrence income candidates = %#v, want none", candidates)
	}
}
