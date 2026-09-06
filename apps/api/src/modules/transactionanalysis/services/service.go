package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	"ledgermeadow/src/modules/transactionanalysis/models"
	shared "ledgermeadow/src/shared/types"
)

const (
	automaticCategoryConfidence uint32 = 7500
	recurringReviewConfidence   uint32 = 6500
	recurringActiveConfidence   uint32 = 8500
)

var ErrEngineResponse = errors.New("financial engine returned an invalid analysis response")

type Repository interface {
	Load(context.Context, shared.UserID, time.Time) (models.Input, error)
	LoadLearningSignals(context.Context, shared.UserID, []string) ([]models.LearningSignal, error)
	Apply(context.Context, shared.UserID, models.Result) error
}

type Engine interface {
	CategorizeTransactions(context.Context, *financialenginepb.CategorizeTransactionsRequest) (*financialenginepb.CategorizeTransactionsResponse, error)
	DetectRecurring(context.Context, *financialenginepb.DetectRecurringRequest) (*financialenginepb.DetectRecurringResponse, error)
}

type Service struct {
	repository Repository
	engine     Engine
	now        func() time.Time
}

func New(repository Repository, engine Engine) *Service {
	return &Service{repository: repository, engine: engine, now: time.Now}
}

func (s *Service) Analyze(ctx context.Context, userID shared.UserID) error {
	input, err := s.repository.Load(ctx, userID, s.now().UTC())
	if err != nil {
		return err
	}
	if len(input.Transactions) == 0 {
		return s.repository.Apply(ctx, userID, models.Result{})
	}

	engineTransactions := make([]*financialenginepb.AnalysisTransaction, 0, len(input.Transactions))
	engineByID := make(map[string]*financialenginepb.AnalysisTransaction, len(input.Transactions))
	transactionsByID := make(map[string]models.Transaction, len(input.Transactions))
	for _, transaction := range input.Transactions {
		converted, err := analysisTransaction(transaction)
		if err != nil {
			return err
		}
		engineTransactions = append(engineTransactions, converted)
		engineByID[transaction.ID] = converted
		transactionsByID[transaction.ID] = transaction
	}
	categories, categoriesByID, err := categoryCandidates(input.Categories)
	if err != nil {
		return err
	}

	request := &financialenginepb.CategorizeTransactionsRequest{
		Transactions: engineTransactions,
		Categories:   categories,
	}
	categorized, err := s.engine.CategorizeTransactions(ctx, request)
	if err != nil {
		return fmt.Errorf("categorize transactions: %w", err)
	}
	learningKeys, err := validateCategorizations(categorized, transactionsByID, categoriesByID)
	if err != nil {
		return err
	}
	signals, err := s.repository.LoadLearningSignals(ctx, userID, learningKeyValues(learningKeys))
	if err != nil {
		return err
	}
	if len(signals) > 0 {
		request.Signals, err = categorizationSignals(signals, learningKeys, categoriesByID)
		if err != nil {
			return err
		}
		categorized, err = s.engine.CategorizeTransactions(ctx, request)
		if err != nil {
			return fmt.Errorf("categorize transactions with learned signals: %w", err)
		}
		learnedKeys, validationErr := validateCategorizations(categorized, transactionsByID, categoriesByID)
		if validationErr != nil || !equalLearningKeys(learningKeys, learnedKeys) {
			return ErrEngineResponse
		}
	}
	learning, assignments, reviews, err := categoryAssignments(
		categorized, transactionsByID, engineByID, categoriesByID,
	)
	if err != nil {
		return err
	}

	byCurrency := make(map[financialenginepb.Currency][]*financialenginepb.AnalysisTransaction, 2)
	for _, transaction := range engineTransactions {
		byCurrency[transaction.Currency] = append(byCurrency[transaction.Currency], transaction)
	}
	evidenceByCategory := make(map[string]models.CategoryRecurrenceEvidence, len(input.CategoryEvidence))
	for _, evidence := range input.CategoryEvidence {
		if categoriesByID[evidence.CategoryID].ID == "" ||
			evidence.ConfirmedSubscriptionCount > evidence.SubscriptionCount ||
			evidence.ConfirmedBillCount > evidence.BillCount {
			return ErrEngineResponse
		}
		evidenceByCategory[evidence.CategoryID] = evidence
	}
	categoryEvidence := make([]*financialenginepb.CategoryRecurrenceEvidence, 0, len(input.Categories))
	for _, category := range input.Categories {
		evidence := evidenceByCategory[category.ID]
		kindHint, err := recurringKindHint(category.RecurringKindHint)
		if err != nil {
			return err
		}
		if kindHint == financialenginepb.RecurringKind_RECURRING_KIND_UNSPECIFIED &&
			evidence.SubscriptionCount == 0 && evidence.BillCount == 0 {
			continue
		}
		categoryEvidence = append(categoryEvidence, &financialenginepb.CategoryRecurrenceEvidence{
			CategoryId:        category.ID,
			SubscriptionCount: evidence.SubscriptionCount, BillCount: evidence.BillCount,
			ConfirmedSubscriptionCount: evidence.ConfirmedSubscriptionCount,
			ConfirmedBillCount:         evidence.ConfirmedBillCount,
			KindHint:                   kindHint,
		})
	}
	prevailingFrequency, err := recurringFrequency(input.PrevailingFrequency)
	if err != nil {
		return err
	}
	candidates := make([]models.RecurringCandidate, 0)
	for _, currency := range []financialenginepb.Currency{
		financialenginepb.Currency_CURRENCY_CAD,
		financialenginepb.Currency_CURRENCY_USD,
	} {
		transactions := byCurrency[currency]
		if len(transactions) == 0 {
			continue
		}
		detected, err := s.engine.DetectRecurring(ctx, &financialenginepb.DetectRecurringRequest{
			AsOfDate: pbDate(input.AsOf), Transactions: transactions,
			CategoryEvidence: categoryEvidence, PrevailingFrequency: prevailingFrequency,
		})
		if err != nil {
			return fmt.Errorf("detect recurring transactions: %w", err)
		}
		mapped, err := recurringCandidates(detected, engineByID, categoriesByID)
		if err != nil {
			return err
		}
		candidates = append(candidates, mapped...)
	}
	return s.repository.Apply(ctx, userID, models.Result{
		LearningKeys: learning, Categories: assignments,
		CategoryReviews: reviews, Recurring: candidates,
	})
}

func analysisTransaction(transaction models.Transaction) (*financialenginepb.AnalysisTransaction, error) {
	currency, err := pbCurrency(transaction.Currency)
	if err != nil {
		return nil, err
	}
	accountKind := financialenginepb.AnalysisAccountKind_ANALYSIS_ACCOUNT_KIND_UNSPECIFIED
	switch transaction.AccountKind {
	case "ASSET":
		accountKind = financialenginepb.AnalysisAccountKind_ANALYSIS_ACCOUNT_KIND_ASSET
	case "LIABILITY":
		accountKind = financialenginepb.AnalysisAccountKind_ANALYSIS_ACCOUNT_KIND_LIABILITY
	default:
		return nil, errors.New("analysis transaction account kind is unsupported")
	}
	var categoryID *string
	categoryType := financialenginepb.CategoryType_CATEGORY_TYPE_UNSPECIFIED
	if transaction.CategorySource == "USER" {
		if transaction.CategoryID == nil || transaction.CategoryType == nil {
			return nil, errors.New("user-categorized analysis transaction is incomplete")
		}
		categoryID = transaction.CategoryID
		categoryType, err = pbCategoryType(*transaction.CategoryType)
		if err != nil {
			return nil, err
		}
	}
	return &financialenginepb.AnalysisTransaction{
		TransactionId: transaction.ID, AccountId: transaction.AccountID,
		Name: transaction.Name, MerchantName: transaction.MerchantName,
		OriginalDescription: transaction.OriginalDescription,
		AmountMinor:         transaction.AmountMinor, Currency: currency,
		Date:                     pbDate(transaction.Date),
		ProviderCategoryPrimary:  transaction.ProviderCategoryPrimary,
		ProviderCategoryDetailed: transaction.ProviderCategoryDetailed,
		AccountKind:              accountKind, CategoryId: categoryID,
		CategoryType: categoryType,
	}, nil
}

func categoryCandidates(
	categories []models.Category,
) ([]*financialenginepb.CategoryCandidate, map[string]models.Category, error) {
	if len(categories) > 200 {
		return nil, nil, ErrEngineResponse
	}
	converted := make([]*financialenginepb.CategoryCandidate, 0, len(categories))
	byID := make(map[string]models.Category, len(categories))
	for _, category := range categories {
		if category.ID == "" || byID[category.ID].ID != "" {
			return nil, nil, ErrEngineResponse
		}
		categoryType, err := pbCategoryType(category.Type)
		if err != nil {
			return nil, nil, err
		}
		byID[category.ID] = category
		converted = append(converted, &financialenginepb.CategoryCandidate{
			CategoryId: category.ID, Name: category.Name,
			CategoryType: categoryType, ParentName: category.ParentName,
		})
	}
	return converted, byID, nil
}

func pbCategoryType(value string) (financialenginepb.CategoryType, error) {
	switch value {
	case "INCOME":
		return financialenginepb.CategoryType_CATEGORY_TYPE_INCOME, nil
	case "EXPENSE":
		return financialenginepb.CategoryType_CATEGORY_TYPE_EXPENSE, nil
	case "TRANSFER":
		return financialenginepb.CategoryType_CATEGORY_TYPE_TRANSFER, nil
	default:
		return financialenginepb.CategoryType_CATEGORY_TYPE_UNSPECIFIED,
			errors.New("analysis category type is unsupported")
	}
}

func recurringKindHint(value *string) (financialenginepb.RecurringKind, error) {
	if value == nil {
		return financialenginepb.RecurringKind_RECURRING_KIND_UNSPECIFIED, nil
	}
	switch *value {
	case "BILL":
		return financialenginepb.RecurringKind_RECURRING_KIND_BILL, nil
	case "SUBSCRIPTION":
		return financialenginepb.RecurringKind_RECURRING_KIND_SUBSCRIPTION, nil
	default:
		return financialenginepb.RecurringKind_RECURRING_KIND_UNSPECIFIED,
			errors.New("analysis category recurring kind hint is unsupported")
	}
}

func validateCategorizations(
	response *financialenginepb.CategorizeTransactionsResponse,
	transactions map[string]models.Transaction,
	categories map[string]models.Category,
) (map[string]string, error) {
	if response == nil || len(response.Categorizations) != len(transactions) {
		return nil, ErrEngineResponse
	}
	seen := make(map[string]struct{}, len(response.Categorizations))
	learningKeys := make(map[string]string, len(response.Categorizations))
	for _, categorization := range response.Categorizations {
		if categorization == nil || transactions[categorization.TransactionId].ID == "" ||
			categorization.ConfidenceBasisPoints > 10000 || !validLearningKey(categorization.LearningKey) {
			return nil, ErrEngineResponse
		}
		if _, exists := seen[categorization.TransactionId]; exists {
			return nil, ErrEngineResponse
		}
		seen[categorization.TransactionId] = struct{}{}
		if categorization.CategoryId != "" && categories[categorization.CategoryId].ID == "" {
			return nil, ErrEngineResponse
		}
		if categorization.CategoryId == "" && categorization.ConfidenceBasisPoints != 0 {
			return nil, ErrEngineResponse
		}
		learningKeys[categorization.TransactionId] = categorization.LearningKey
	}
	return learningKeys, nil
}

func learningKeyValues(byTransactionID map[string]string) []string {
	values := make([]string, 0, len(byTransactionID))
	seen := make(map[string]struct{}, len(byTransactionID))
	for _, value := range byTransactionID {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func equalLearningKeys(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for transactionID, learningKey := range left {
		if right[transactionID] != learningKey {
			return false
		}
	}
	return true
}

func categorizationSignals(
	signals []models.LearningSignal,
	learningKeys map[string]string,
	categories map[string]models.Category,
) ([]*financialenginepb.CategorizationSignal, error) {
	if len(signals) > 8192 {
		return nil, ErrEngineResponse
	}
	allowedKeys := make(map[string]struct{}, len(learningKeys))
	for _, value := range learningKeys {
		allowedKeys[value] = struct{}{}
	}
	type key struct{ learning, category string }
	combined := make(map[key]*financialenginepb.CategorizationSignal, len(signals))
	for _, signal := range signals {
		if _, exists := allowedKeys[signal.LearningKey]; !exists ||
			categories[signal.CategoryID].ID == "" ||
			signal.PersonalObservationCount > 1 ||
			signal.GlobalUserCount > signal.GlobalTotalContributorCount ||
			(signal.GlobalUserCount > 0 && signal.GlobalUserCount < 3) ||
			(signal.GlobalUserCount == 0 && signal.GlobalTotalContributorCount != 0) ||
			(signal.PersonalObservationCount == 0 && signal.GlobalUserCount == 0) {
			return nil, ErrEngineResponse
		}
		pair := key{learning: signal.LearningKey, category: signal.CategoryID}
		item := combined[pair]
		if item == nil {
			item = &financialenginepb.CategorizationSignal{
				LearningKey: signal.LearningKey, CategoryId: signal.CategoryID,
			}
			combined[pair] = item
		}
		if signal.PersonalObservationCount > 0 {
			if item.PersonalObservationCount > 0 {
				return nil, ErrEngineResponse
			}
			item.PersonalObservationCount = signal.PersonalObservationCount
		}
		if signal.GlobalUserCount > 0 {
			if item.GlobalUserCount > 0 {
				return nil, ErrEngineResponse
			}
			item.GlobalUserCount = signal.GlobalUserCount
			item.GlobalTotalContributorCount = signal.GlobalTotalContributorCount
		}
	}
	keys := make([]key, 0, len(combined))
	for pair := range combined {
		keys = append(keys, pair)
	}
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].learning < keys[right].learning ||
			(keys[left].learning == keys[right].learning && keys[left].category < keys[right].category)
	})
	converted := make([]*financialenginepb.CategorizationSignal, 0, len(keys))
	for _, pair := range keys {
		converted = append(converted, combined[pair])
	}
	return converted, nil
}

func categoryAssignments(
	response *financialenginepb.CategorizeTransactionsResponse,
	transactions map[string]models.Transaction,
	engineTransactions map[string]*financialenginepb.AnalysisTransaction,
	categories map[string]models.Category,
) ([]models.LearningKeyAssignment, []models.CategoryAssignment, []models.CategoryReview, error) {
	learningKeys, err := validateCategorizations(response, transactions, categories)
	if err != nil {
		return nil, nil, nil, err
	}
	learning := make([]models.LearningKeyAssignment, 0, len(learningKeys))
	assignments := make([]models.CategoryAssignment, 0, len(response.Categorizations))
	reviews := make([]models.CategoryReview, 0)
	for _, categorization := range response.Categorizations {
		learning = append(learning, models.LearningKeyAssignment{
			TransactionID: categorization.TransactionId,
			LearningKey:   categorization.LearningKey,
		})
		transaction := transactions[categorization.TransactionId]
		if transaction.CategorySource == "USER" {
			continue
		}
		if categorization.CategoryId == "" {
			reviews = append(reviews, models.CategoryReview{
				TransactionID: categorization.TransactionId,
				Explanation:   "No reliable category match was found.",
			})
			continue
		}
		category := categories[categorization.CategoryId]
		if categorization.ConfidenceBasisPoints >= automaticCategoryConfidence {
			assignments = append(assignments, models.CategoryAssignment{
				TransactionID: categorization.TransactionId, CategoryID: categorization.CategoryId,
				ConfidenceBasisPoints: categorization.ConfidenceBasisPoints,
			})
			engineTransaction := engineTransactions[categorization.TransactionId]
			engineTransaction.CategoryId = &categorization.CategoryId
			engineTransaction.CategoryType, err = pbCategoryType(category.Type)
			if err != nil {
				return nil, nil, nil, err
			}
		} else {
			reviews = append(reviews, models.CategoryReview{
				TransactionID:         categorization.TransactionId,
				SuggestedCategoryID:   categorization.CategoryId,
				ConfidenceBasisPoints: categorization.ConfidenceBasisPoints,
				Explanation:           categorizationExplanation(categorization.Reason),
			})
		}
	}
	sort.Slice(learning, func(left, right int) bool {
		return learning[left].TransactionID < learning[right].TransactionID
	})
	sort.Slice(assignments, func(left, right int) bool {
		return assignments[left].TransactionID < assignments[right].TransactionID
	})
	sort.Slice(reviews, func(left, right int) bool {
		return reviews[left].TransactionID < reviews[right].TransactionID
	})
	return learning, assignments, reviews, nil
}

func categorizationExplanation(reason string) string {
	switch reason {
	case "personal_history":
		return "This matches a category you previously selected."
	case "global_history":
		return "This matches an aggregate category pattern confirmed by multiple users."
	case "category_name_overlap":
		return "The transaction data overlaps with the category name."
	default:
		return "No reliable category match was found."
	}
}

func validLearningKey(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func recurringCandidates(
	response *financialenginepb.DetectRecurringResponse,
	byID map[string]*financialenginepb.AnalysisTransaction,
	categories map[string]models.Category,
) ([]models.RecurringCandidate, error) {
	if response == nil || len(response.Candidates) > models.MaxTransactions/3 {
		return nil, ErrEngineResponse
	}
	candidates := make([]models.RecurringCandidate, 0, len(response.Candidates))
	seenSupportingIDs := make(map[string]struct{})
	seenObservationIDs := make(map[string]struct{})
	for _, candidate := range response.Candidates {
		if candidate == nil || candidate.DetectionKey == "" || candidate.Name == "" ||
			candidate.ExpectedAmountMinor <= 0 || candidate.ConfidenceBasisPoints > 10000 ||
			candidate.OccurrenceCount == 0 ||
			int(candidate.OccurrenceCount) != len(candidate.SupportingTransactionIds) ||
			len(candidate.SupportingTransactionIds) == 0 ||
			byID[candidate.SupportingTransactionIds[0]] == nil {
			return nil, ErrEngineResponse
		}
		learnedCategory := candidate.Evidence == financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_LEARNED_CATEGORY
		if candidate.Evidence != financialenginepb.RecurringEvidence_RECURRING_EVIDENCE_CADENCE && !learnedCategory {
			return nil, ErrEngineResponse
		}
		if !learnedCategory {
			if candidate.OccurrenceCount < 2 {
				return nil, ErrEngineResponse
			}
			if candidate.ConfidenceBasisPoints < recurringReviewConfidence {
				continue
			}
		}
		if (candidate.ObservedAmountMinor == nil) != (len(candidate.AmountObservationTransactionIds) == 0) ||
			(candidate.ObservedAmountMinor != nil && (*candidate.ObservedAmountMinor <= 0 || learnedCategory)) {
			return nil, ErrEngineResponse
		}
		kind := recurringKind(candidate.Kind)
		frequency := frequency(candidate.Frequency)
		currency := currencyCode(candidate.Currency)
		categoryID := candidate.GetCategoryId()
		next, err := goDate(candidate.NextExpectedAt)
		if kind == "" || frequency == "" || currency == "" || err != nil ||
			(learnedCategory && (kind != "SUBSCRIPTION" || categoryID == "")) ||
			(categoryID != "" && categories[categoryID].ID == "") {
			return nil, ErrEngineResponse
		}
		if !learnedCategory && candidate.OccurrenceCount < 3 && kind != "SUBSCRIPTION" {
			continue
		}
		for _, transactionID := range candidate.SupportingTransactionIds {
			if transaction := byID[transactionID]; transaction == nil || currencyCode(transaction.Currency) != currency {
				return nil, ErrEngineResponse
			}
			if _, exists := seenSupportingIDs[transactionID]; exists {
				return nil, ErrEngineResponse
			}
			seenSupportingIDs[transactionID] = struct{}{}
		}
		for _, transactionID := range candidate.AmountObservationTransactionIds {
			if transaction := byID[transactionID]; transaction == nil || currencyCode(transaction.Currency) != currency {
				return nil, ErrEngineResponse
			}
			if _, exists := seenObservationIDs[transactionID]; exists {
				return nil, ErrEngineResponse
			}
			seenObservationIDs[transactionID] = struct{}{}
		}
		status := "UNKNOWN"
		if !learnedCategory && candidate.ConfidenceBasisPoints >= recurringActiveConfidence {
			status = "ACTIVE"
		}
		candidates = append(candidates, models.RecurringCandidate{
			DetectionKey: candidate.DetectionKey, Name: candidate.Name, Kind: kind,
			Currency: currency, Frequency: frequency,
			ExpectedAmountMinor:   candidate.ExpectedAmountMinor,
			NextExpectedAt:        next.Format("2006-01-02"),
			ConfidenceBasisPoints: candidate.ConfidenceBasisPoints,
			OccurrenceCount:       candidate.OccurrenceCount, AccountID: candidate.AccountId,
			CategoryID: categoryID, SupportingIDs: candidate.SupportingTransactionIds,
			Explanation: candidate.Explanation, Status: status,
			ObservedAmountMinor:  candidate.ObservedAmountMinor,
			AmountObservationIDs: candidate.AmountObservationTransactionIds,
		})
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].DetectionKey < candidates[right].DetectionKey })
	return candidates, nil
}

func pbCurrency(value string) (financialenginepb.Currency, error) {
	switch value {
	case "CAD":
		return financialenginepb.Currency_CURRENCY_CAD, nil
	case "USD":
		return financialenginepb.Currency_CURRENCY_USD, nil
	default:
		return financialenginepb.Currency_CURRENCY_UNSPECIFIED, errors.New("analysis transaction currency is unsupported")
	}
}

func currencyCode(value financialenginepb.Currency) string {
	switch value {
	case financialenginepb.Currency_CURRENCY_CAD:
		return "CAD"
	case financialenginepb.Currency_CURRENCY_USD:
		return "USD"
	default:
		return ""
	}
}

func pbDate(value time.Time) *financialenginepb.Date {
	return &financialenginepb.Date{Year: int32(value.Year()), Month: uint32(value.Month()), Day: uint32(value.Day())}
}

func goDate(value *financialenginepb.Date) (time.Time, error) {
	if value == nil || value.Year < 1970 || value.Month < 1 || value.Month > 12 || value.Day < 1 || value.Day > 31 {
		return time.Time{}, ErrEngineResponse
	}
	date := time.Date(int(value.Year), time.Month(value.Month), int(value.Day), 0, 0, 0, 0, time.UTC)
	if date.Year() != int(value.Year) || date.Month() != time.Month(value.Month) || date.Day() != int(value.Day) {
		return time.Time{}, ErrEngineResponse
	}
	return date, nil
}

func recurringKind(value financialenginepb.RecurringKind) string {
	switch value {
	case financialenginepb.RecurringKind_RECURRING_KIND_INCOME:
		return "INCOME"
	case financialenginepb.RecurringKind_RECURRING_KIND_BILL:
		return "BILL"
	case financialenginepb.RecurringKind_RECURRING_KIND_SUBSCRIPTION:
		return "SUBSCRIPTION"
	default:
		return ""
	}
}

func frequency(value financialenginepb.Frequency) string {
	switch value {
	case financialenginepb.Frequency_FREQUENCY_WEEKLY:
		return "WEEKLY"
	case financialenginepb.Frequency_FREQUENCY_BIWEEKLY:
		return "BIWEEKLY"
	case financialenginepb.Frequency_FREQUENCY_MONTHLY:
		return "MONTHLY"
	case financialenginepb.Frequency_FREQUENCY_QUARTERLY:
		return "QUARTERLY"
	case financialenginepb.Frequency_FREQUENCY_ANNUALLY:
		return "ANNUALLY"
	default:
		return ""
	}
}

func recurringFrequency(value string) (financialenginepb.Frequency, error) {
	switch value {
	case "":
		return financialenginepb.Frequency_FREQUENCY_UNSPECIFIED, nil
	case "WEEKLY":
		return financialenginepb.Frequency_FREQUENCY_WEEKLY, nil
	case "BIWEEKLY":
		return financialenginepb.Frequency_FREQUENCY_BIWEEKLY, nil
	case "MONTHLY":
		return financialenginepb.Frequency_FREQUENCY_MONTHLY, nil
	case "QUARTERLY":
		return financialenginepb.Frequency_FREQUENCY_QUARTERLY, nil
	case "ANNUALLY":
		return financialenginepb.Frequency_FREQUENCY_ANNUALLY, nil
	default:
		return financialenginepb.Frequency_FREQUENCY_UNSPECIFIED, errors.New("stored recurring frequency is unsupported")
	}
}
