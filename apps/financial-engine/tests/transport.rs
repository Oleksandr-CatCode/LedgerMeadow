use ledgermeadow_financial_engine::transport::pb::financial_engine_server::FinancialEngine;
use ledgermeadow_financial_engine::transport::{FinancialEngineService, pb};
use tonic::{Code, Request};

fn analysis_transaction(
    id: &str,
    name: &str,
    amount_minor: i64,
    year: i32,
    month: u32,
    day: u32,
) -> pb::AnalysisTransaction {
    pb::AnalysisTransaction {
        transaction_id: id.to_owned(),
        account_id: "account-1".to_owned(),
        name: name.to_owned(),
        merchant_name: Some(name.to_owned()),
        original_description: None,
        amount_minor,
        currency: pb::Currency::Cad as i32,
        date: Some(pb::Date { year, month, day }),
        provider_category_primary: None,
        provider_category_detailed: None,
        account_kind: pb::AnalysisAccountKind::Asset as i32,
        category_id: None,
        category_type: pb::CategoryType::Unspecified as i32,
    }
}

#[tokio::test]
async fn financial_engine_transport_categorizes_and_detects_recurring_transactions() {
    let categorized = FinancialEngineService
        .categorize_transactions(Request::new(pb::CategorizeTransactionsRequest {
            transactions: vec![analysis_transaction(
                "category-1",
                "Streaming service",
                -1_999,
                2026,
                1,
                31,
            )],
            categories: vec![pb::CategoryCandidate {
                category_id: "custom-category".to_owned(),
                name: "Streaming".to_owned(),
                category_type: pb::CategoryType::Expense as i32,
                parent_name: None,
            }],
            signals: Vec::new(),
        }))
        .await
        .expect("categorization transport should succeed")
        .into_inner();
    assert_eq!(categorized.categorizations.len(), 1);
    assert_eq!(
        categorized.categorizations[0].category_id,
        "custom-category"
    );

    let recurring = FinancialEngineService
        .detect_recurring(Request::new(pb::DetectRecurringRequest {
            as_of_date: Some(pb::Date {
                year: 2026,
                month: 4,
                day: 1,
            }),
            transactions: vec![
                analysis_transaction("recurring-1", "Fixed service", -1_999, 2026, 1, 31),
                analysis_transaction("recurring-2", "Fixed service", -1_999, 2026, 2, 28),
                analysis_transaction("recurring-3", "Fixed service", -1_999, 2026, 3, 31),
            ],
            category_evidence: Vec::new(),
            prevailing_frequency: pb::Frequency::Unspecified as i32,
        }))
        .await
        .expect("recurring transport should succeed")
        .into_inner();
    assert_eq!(recurring.candidates.len(), 1);
    assert_eq!(
        recurring.candidates[0].kind,
        pb::RecurringKind::Subscription as i32
    );
    assert_eq!(
        recurring.candidates[0].frequency,
        pb::Frequency::Monthly as i32
    );
    assert_eq!(
        recurring.candidates[0].next_expected_at,
        Some(pb::Date {
            year: 2026,
            month: 4,
            day: 30,
        })
    );
}

#[tokio::test]
async fn financial_engine_analysis_transport_rejects_malformed_transactions() {
    let mut invalid = analysis_transaction("invalid", "Valid name", -100, 2026, 1, 1);
    invalid.date = None;
    let error = FinancialEngineService
        .categorize_transactions(Request::new(pb::CategorizeTransactionsRequest {
            transactions: vec![invalid],
            categories: Vec::new(),
            signals: Vec::new(),
        }))
        .await
        .expect_err("missing transaction date must fail");
    assert_eq!(error.code(), Code::InvalidArgument);

    let mut invalid_currency = analysis_transaction("invalid", "Valid name", -100, 2026, 1, 1);
    invalid_currency.currency = pb::Currency::Unspecified as i32;
    let error = FinancialEngineService
        .categorize_transactions(Request::new(pb::CategorizeTransactionsRequest {
            transactions: vec![invalid_currency],
            categories: Vec::new(),
            signals: Vec::new(),
        }))
        .await
        .expect_err("missing transaction currency must fail");
    assert_eq!(error.code(), Code::InvalidArgument);
}

#[tokio::test]
async fn financial_engine_transport_rejects_oversized_categorization_evidence() {
    let signal = pb::CategorizationSignal {
        learning_key: "0".repeat(64),
        category_id: "category-1".to_owned(),
        personal_observation_count: 1,
        global_user_count: 0,
        global_total_contributor_count: 0,
    };
    let error = FinancialEngineService
        .categorize_transactions(Request::new(pb::CategorizeTransactionsRequest {
            transactions: vec![analysis_transaction(
                "oversized",
                "Bounded request",
                -100,
                2026,
                1,
                1,
            )],
            categories: vec![pb::CategoryCandidate {
                category_id: "category-1".to_owned(),
                name: "Bounded".to_owned(),
                category_type: pb::CategoryType::Expense as i32,
                parent_name: None,
            }],
            signals: vec![signal; 8_193],
        }))
        .await
        .expect_err("oversized categorization evidence must fail");
    assert_eq!(error.code(), Code::ResourceExhausted);
}

#[tokio::test]
async fn financial_engine_transport_rejects_invalid_recurring_kind_hint() {
    let error = FinancialEngineService
        .detect_recurring(Request::new(pb::DetectRecurringRequest {
            as_of_date: Some(pb::Date {
                year: 2026,
                month: 4,
                day: 1,
            }),
            transactions: Vec::new(),
            category_evidence: vec![pb::CategoryRecurrenceEvidence {
                category_id: "category-1".to_owned(),
                subscription_count: 0,
                bill_count: 0,
                confirmed_subscription_count: 0,
                confirmed_bill_count: 0,
                kind_hint: pb::RecurringKind::Income as i32,
            }],
            prevailing_frequency: pb::Frequency::Unspecified as i32,
        }))
        .await
        .expect_err("income must not be accepted as an outflow category hint");
    assert_eq!(error.code(), Code::InvalidArgument);
}

#[tokio::test]
async fn financial_engine_transport_calculates_dashboard_values() {
    let response = FinancialEngineService
        .calculate_cash_flow(Request::new(pb::CalculateCashFlowRequest {
            currency: pb::Currency::Cad as i32,
            actual: Some(pb::CashFlowComponents {
                income_minor: 20_000,
                fixed_outflow_minor: 5_000,
                variable_outflow_minor: 1_000,
                subscriptions_minor: 500,
                savings_minor: 0,
            }),
            projected: Some(pb::CashFlowComponents {
                income_minor: 0,
                fixed_outflow_minor: 10_000,
                variable_outflow_minor: 0,
                subscriptions_minor: 0,
                savings_minor: 0,
            }),
        }))
        .await
        .expect("cash-flow transport should succeed")
        .into_inner();

    let actual = response.actual.expect("actual summary is required");
    assert_eq!(actual.total_outflow_minor, 6_500);
    assert_eq!(actual.net_minor, 13_500);
    assert_eq!(response.currency, pb::Currency::Cad as i32);
}

#[tokio::test]
async fn financial_engine_transport_calculates_planning_summary() {
    let response = FinancialEngineService
        .calculate_planning_summary(Request::new(pb::CalculatePlanningSummaryRequest {
            currency: pb::Currency::Cad as i32,
            as_of_date: Some(pb::Date {
                year: 2026,
                month: 8,
                day: 24,
            }),
            plan_start_date: Some(pb::Date {
                year: 2026,
                month: 9,
                day: 1,
            }),
            plan_end_date: Some(pb::Date {
                year: 2026,
                month: 9,
                day: 30,
            }),
            items: vec![
                pb::PlanningItem {
                    id: "income-1".to_owned(),
                    name: "Salary".to_owned(),
                    kind: pb::PlanningItemKind::Income as i32,
                    expected_amount_minor: 550_000,
                    next_date: Some(pb::Date {
                        year: 2026,
                        month: 9,
                        day: 1,
                    }),
                    frequency: pb::Frequency::Monthly as i32,
                },
                pb::PlanningItem {
                    id: "bill-1".to_owned(),
                    name: "Rent".to_owned(),
                    kind: pb::PlanningItemKind::Bill as i32,
                    expected_amount_minor: 180_000,
                    next_date: Some(pb::Date {
                        year: 2026,
                        month: 9,
                        day: 1,
                    }),
                    frequency: pb::Frequency::Monthly as i32,
                },
            ],
            allocations: vec![pb::PlanningAllocation {
                id: "budget-1".to_owned(),
                name: "Daily".to_owned(),
                amount_minor: 120_000,
                frequency: pb::Frequency::Monthly as i32,
            }],
        }))
        .await
        .expect("planning transport should succeed")
        .into_inner();

    assert_eq!(response.expected_income_minor, 550_000);
    assert_eq!(response.committed_bills_minor, 180_000);
    assert_eq!(response.unallocated_minor, 250_000);
    assert_eq!(response.allocations.len(), 1);
}

#[tokio::test]
async fn financial_engine_transport_rejects_unspecified_currency() {
    let error = FinancialEngineService
        .calculate_net_worth(Request::new(pb::CalculateNetWorthRequest {
            currency: pb::Currency::Unspecified as i32,
            assets: Vec::new(),
            liabilities: Vec::new(),
        }))
        .await
        .expect_err("unspecified currency must fail");

    assert_eq!(error.code(), Code::InvalidArgument);
}
