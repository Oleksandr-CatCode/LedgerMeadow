use ledgermeadow_financial_engine::categorization::{
    AccountKind, AnalysisTransaction, CategorizationSignal, CategorizeTransactionsInput,
    CategoryCandidate, CategoryType, MAX_ANALYSIS_TRANSACTIONS, MAX_CATEGORIZATION_SIGNALS,
    MAX_CATEGORY_CANDIDATES, categorize_transactions,
};
use ledgermeadow_financial_engine::date::Date;
use ledgermeadow_financial_engine::forecast::Frequency;
use ledgermeadow_financial_engine::money::Currency;
use ledgermeadow_financial_engine::recurring::{
    CategoryRecurrenceEvidence, DetectRecurringInput, RecurringEvidence, RecurringKind,
    detect_recurring,
};
use ledgermeadow_financial_engine::{EngineError, EngineResult};

fn date(year: i32, month: u8, day: u8) -> Date {
    Date { year, month, day }
}

fn transaction(
    id: &str,
    source: &str,
    amount_minor: i64,
    transaction_date: Date,
    category_id: Option<&str>,
    category_type: Option<CategoryType>,
) -> AnalysisTransaction {
    AnalysisTransaction {
        transaction_id: id.to_owned(),
        account_id: "account-1".to_owned(),
        name: source.to_owned(),
        merchant_name: Some(source.to_owned()),
        original_description: None,
        amount_minor,
        currency: Currency::Cad,
        date: transaction_date,
        provider_category_primary: None,
        provider_category_detailed: None,
        account_kind: AccountKind::Asset,
        category_id: category_id.map(str::to_owned),
        category_type,
    }
}

fn category(id: &str, name: &str, category_type: CategoryType) -> CategoryCandidate {
    CategoryCandidate {
        category_id: id.to_owned(),
        name: name.to_owned(),
        category_type,
        parent_name: None,
    }
}

fn learning_key(
    transaction: AnalysisTransaction,
    categories: Vec<CategoryCandidate>,
) -> EngineResult<String> {
    let output = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![transaction],
        categories,
        signals: Vec::new(),
    })?;
    Ok(output.categorizations[0].learning_key.clone())
}

#[test]
fn categorization_prefers_personal_then_global_learned_evidence() -> EngineResult<()> {
    let transaction = transaction(
        "learned",
        "Unrecognized vendor",
        -1_200,
        date(2026, 8, 1),
        None,
        None,
    );
    let categories = vec![
        category("category-alpha", "Alpha", CategoryType::Expense),
        category("category-beta", "Beta", CategoryType::Expense),
    ];
    let learning_key = learning_key(transaction.clone(), categories.clone())?;
    assert_eq!(learning_key.len(), 64);
    assert!(
        learning_key
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    );

    let output = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![transaction.clone()],
        categories: categories.clone(),
        signals: vec![
            CategorizationSignal {
                learning_key: learning_key.clone(),
                category_id: "category-alpha".to_owned(),
                personal_observation_count: 0,
                global_user_count: 90,
                global_total_contributor_count: 100,
            },
            CategorizationSignal {
                learning_key: learning_key.clone(),
                category_id: "category-beta".to_owned(),
                personal_observation_count: 1,
                global_user_count: 1,
                global_total_contributor_count: 100,
            },
        ],
    })?;
    assert_eq!(
        output.categorizations[0].category_id.as_deref(),
        Some("category-beta")
    );
    assert_eq!(output.categorizations[0].confidence_basis_points, 10_000);
    assert_eq!(output.categorizations[0].reason, "personal_history");

    let global = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![transaction.clone()],
        categories: categories.clone(),
        signals: vec![
            CategorizationSignal {
                learning_key: learning_key.clone(),
                category_id: "category-alpha".to_owned(),
                personal_observation_count: 0,
                global_user_count: 8,
                global_total_contributor_count: 10,
            },
            CategorizationSignal {
                learning_key: learning_key.clone(),
                category_id: "category-beta".to_owned(),
                personal_observation_count: 0,
                global_user_count: 9,
                global_total_contributor_count: 20,
            },
        ],
    })?;
    assert_eq!(
        global.categorizations[0].category_id.as_deref(),
        Some("category-alpha")
    );
    assert_eq!(global.categorizations[0].confidence_basis_points, 8_000);
    assert_eq!(global.categorizations[0].reason, "global_history");

    let tied = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![transaction],
        categories,
        signals: vec![
            CategorizationSignal {
                learning_key: learning_key.clone(),
                category_id: "category-alpha".to_owned(),
                personal_observation_count: 0,
                global_user_count: 1,
                global_total_contributor_count: 2,
            },
            CategorizationSignal {
                learning_key,
                category_id: "category-beta".to_owned(),
                personal_observation_count: 0,
                global_user_count: 1,
                global_total_contributor_count: 2,
            },
        ],
    })?;
    assert_eq!(
        tied.categorizations[0].category_id.as_deref(),
        Some("category-alpha")
    );
    Ok(())
}

#[test]
fn categorization_uses_candidate_text_without_fixed_category_identities_or_fallback()
-> EngineResult<()> {
    let mut lexical = transaction(
        "lexical",
        "Employer deposit",
        50_000,
        date(2026, 8, 1),
        None,
        None,
    );
    lexical.provider_category_detailed = Some("INCOME_WAGES".to_owned());
    let unknown = transaction(
        "unknown",
        "Opaque merchant",
        -1_000,
        date(2026, 8, 2),
        None,
        None,
    );
    let output = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![lexical, unknown],
        categories: vec![
            category("custom-income", "Salary & Wages", CategoryType::Income),
            category(
                "custom-outflow",
                "Everyday Purchases",
                CategoryType::Expense,
            ),
        ],
        signals: Vec::new(),
    })?;

    assert_eq!(output.categorizations.len(), 2);
    assert_eq!(
        output.categorizations[0].category_id.as_deref(),
        Some("custom-income")
    );
    assert_eq!(output.categorizations[0].confidence_basis_points, 8_500);
    assert_eq!(output.categorizations[0].reason, "category_name_overlap");
    assert_eq!(output.categorizations[1].category_id, None);
    assert_eq!(output.categorizations[1].confidence_basis_points, 0);
    assert_eq!(output.categorizations[1].reason, "no_match");
    Ok(())
}

#[test]
fn categorization_ignores_other_labels_and_normalizes_bill_terms() -> EngineResult<()> {
    let mut provider_other = transaction(
        "provider-other",
        "Opaque merchant",
        -1_000,
        date(2026, 8, 1),
        None,
        None,
    );
    provider_other.provider_category_detailed = Some("GENERAL_MERCHANDISE_OTHER".to_owned());
    let cable = transaction(
        "cable",
        "Shaw Cable TV",
        -12_000,
        date(2026, 8, 2),
        None,
        None,
    );
    let loan = transaction(
        "loan",
        "TD Loan Payment",
        -45_000,
        date(2026, 8, 3),
        None,
        None,
    );
    let output = categorize_transactions(&CategorizeTransactionsInput {
        transactions: vec![provider_other, cable, loan],
        categories: vec![
            category("other", "Other", CategoryType::Expense),
            category("utilities", "Utilities", CategoryType::Expense),
            category("debt", "Loan & Debt Payments", CategoryType::Expense),
        ],
        signals: Vec::new(),
    })?;

    assert_eq!(output.categorizations[0].category_id, None);
    assert_eq!(
        output.categorizations[1].category_id.as_deref(),
        Some("utilities")
    );
    assert_eq!(
        output.categorizations[2].category_id.as_deref(),
        Some("debt")
    );
    Ok(())
}

#[test]
fn categorization_rejects_malformed_or_oversized_input() {
    let valid = transaction(
        "valid",
        "Valid merchant",
        -100,
        date(2026, 8, 1),
        None,
        None,
    );
    let categories = vec![category(
        "category-1",
        "Category One",
        CategoryType::Expense,
    )];
    let mut invalid = valid.clone();
    invalid.transaction_id = "invalid".to_owned();
    invalid.name = "   ".to_owned();
    assert_eq!(
        categorize_transactions(&CategorizeTransactionsInput {
            transactions: vec![valid.clone(), invalid],
            categories: categories.clone(),
            signals: Vec::new(),
        }),
        Err(EngineError::InvalidInput("transaction name is invalid"))
    );
    assert!(matches!(
        categorize_transactions(&CategorizeTransactionsInput {
            transactions: vec![valid.clone(); MAX_ANALYSIS_TRANSACTIONS + 1],
            categories: categories.clone(),
            signals: Vec::new(),
        }),
        Err(EngineError::LimitExceeded {
            resource: "analysis transactions",
            limit: MAX_ANALYSIS_TRANSACTIONS,
        })
    ));
    assert!(matches!(
        categorize_transactions(&CategorizeTransactionsInput {
            transactions: vec![valid.clone()],
            categories: vec![categories[0].clone(); MAX_CATEGORY_CANDIDATES + 1],
            signals: Vec::new(),
        }),
        Err(EngineError::LimitExceeded {
            resource: "category candidates",
            limit: MAX_CATEGORY_CANDIDATES,
        })
    ));

    let key = learning_key(valid.clone(), categories.clone()).expect("learning key should exist");
    let invalid_signal = CategorizationSignal {
        learning_key: key,
        category_id: "category-1".to_owned(),
        personal_observation_count: 0,
        global_user_count: 2,
        global_total_contributor_count: 1,
    };
    assert_eq!(
        categorize_transactions(&CategorizeTransactionsInput {
            transactions: vec![valid.clone()],
            categories: categories.clone(),
            signals: vec![invalid_signal.clone()],
        }),
        Err(EngineError::InvalidInput(
            "categorization global winners cannot exceed contributors"
        ))
    );
    assert!(matches!(
        categorize_transactions(&CategorizeTransactionsInput {
            transactions: vec![valid],
            categories,
            signals: vec![invalid_signal; MAX_CATEGORIZATION_SIGNALS + 1],
        }),
        Err(EngineError::LimitExceeded {
            resource: "categorization signals",
            limit: MAX_CATEGORIZATION_SIGNALS,
        })
    ));
}

#[test]
fn recurring_detection_recognizes_supported_frequencies_and_category_types() -> EngineResult<()> {
    let mut transactions = Vec::new();
    let groups = [
        (
            "weekly",
            vec![date(2026, 1, 1), date(2026, 1, 8), date(2026, 1, 15)],
            vec![-1_000, -1_000, -1_000],
            "category-weekly",
            CategoryType::Expense,
        ),
        (
            "biweekly",
            vec![date(2026, 1, 1), date(2026, 1, 15), date(2026, 1, 29)],
            vec![100_000, 100_000, 100_000],
            "category-income",
            CategoryType::Income,
        ),
        (
            "monthly-variable",
            vec![date(2026, 1, 31), date(2026, 2, 28), date(2026, 3, 31)],
            vec![-1_999, -2_099, -1_999],
            "category-variable",
            CategoryType::Expense,
        ),
        (
            "quarterly",
            vec![date(2025, 10, 31), date(2026, 1, 31), date(2026, 4, 30)],
            vec![-12_000, -12_000, -12_000],
            "category-quarterly",
            CategoryType::Expense,
        ),
        (
            "annual",
            vec![date(2024, 2, 29), date(2025, 2, 28), date(2026, 2, 28)],
            vec![-20_000, -20_000, -20_000],
            "category-annual",
            CategoryType::Expense,
        ),
    ];
    for (source, dates, amounts, category_id, category_type) in groups {
        for (index, (transaction_date, amount)) in dates.into_iter().zip(amounts).enumerate() {
            transactions.push(transaction(
                &format!("{source}-{index}"),
                source,
                amount,
                transaction_date,
                Some(category_id),
                Some(category_type),
            ));
        }
    }

    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 5, 1),
        transactions,
        category_evidence: Vec::new(),
        prevailing_frequency: None,
    })?;
    assert_eq!(output.candidates.len(), 5);
    let candidate = |source: &str| {
        output
            .candidates
            .iter()
            .find(|candidate| candidate.detection_key.ends_with(&format!(":{source}")))
            .expect("candidate should exist")
    };
    assert_eq!(candidate("weekly").frequency, Frequency::Weekly);
    assert_eq!(candidate("weekly").kind, RecurringKind::Subscription);
    assert_eq!(candidate("biweekly").kind, RecurringKind::Income);
    assert_eq!(candidate("monthly-variable").frequency, Frequency::Monthly);
    assert_eq!(candidate("monthly-variable").kind, RecurringKind::Bill);
    assert_eq!(candidate("quarterly").next_expected_at, date(2026, 7, 31));
    assert_eq!(candidate("annual").next_expected_at, date(2027, 2, 28));
    Ok(())
}

#[test]
fn recurring_detection_skips_transfers_and_requires_stable_repetition() -> EngineResult<()> {
    let mut transactions = vec![
        transaction(
            "fixed-1",
            "Fixed service 123",
            -1_999,
            date(2026, 1, 31),
            Some("category-fixed"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "fixed-2",
            "Fixed service 456",
            -1_999,
            date(2026, 2, 28),
            Some("category-fixed"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "fixed-3",
            "Fixed service 789",
            -1_999,
            date(2026, 3, 31),
            Some("category-fixed"),
            Some(CategoryType::Expense),
        ),
    ];
    for (index, day) in [1, 15, 29].into_iter().enumerate() {
        transactions.push(transaction(
            &format!("transfer-{index}"),
            "Internal movement",
            -5_000,
            date(2026, 3, day),
            Some("category-transfer"),
            Some(CategoryType::Transfer),
        ));
    }
    transactions.extend([
        transaction("two-1", "Only two", -500, date(2026, 1, 1), None, None),
        transaction("two-2", "Only two", -500, date(2026, 2, 1), None, None),
        transaction(
            "unstable-1",
            "Unstable bill",
            -500,
            date(2026, 1, 1),
            None,
            None,
        ),
        transaction(
            "unstable-2",
            "Unstable bill",
            -5_000,
            date(2026, 2, 1),
            None,
            None,
        ),
        transaction(
            "unstable-3",
            "Unstable bill",
            -50_000,
            date(2026, 3, 1),
            None,
            None,
        ),
    ]);

    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 4, 1),
        transactions,
        category_evidence: Vec::new(),
        prevailing_frequency: None,
    })?;
    assert_eq!(output.candidates.len(), 2);
    let candidate = output
        .candidates
        .iter()
        .find(|candidate| candidate.detection_key == "CAD:outflow:fixed-service")
        .expect("fixed service candidate should exist");
    assert_eq!(candidate.detection_key, "CAD:outflow:fixed-service");
    assert_eq!(candidate.kind, RecurringKind::Subscription);
    assert_eq!(candidate.category_id.as_deref(), Some("category-fixed"));
    assert_eq!(candidate.expected_amount_minor, 1_999);
    assert_eq!(candidate.next_expected_at, date(2026, 4, 30));
    assert_eq!(candidate.occurrence_count, 3);
    let two_occurrences = output
        .candidates
        .iter()
        .find(|candidate| candidate.detection_key == "CAD:outflow:only-two")
        .expect("two-occurrence candidate should exist");
    assert_eq!(two_occurrences.confidence_basis_points, 7_500);
    assert_eq!(two_occurrences.occurrence_count, 2);
    Ok(())
}

#[test]
fn recurring_detection_separates_amount_streams_for_one_counterparty() -> EngineResult<()> {
    let transactions = vec![
        transaction(
            "primary-1",
            "Shared counterparty",
            -155_000,
            date(2026, 6, 1),
            None,
            None,
        ),
        transaction(
            "secondary-1",
            "Shared counterparty",
            -7_500,
            date(2026, 6, 3),
            None,
            None,
        ),
        transaction(
            "primary-2",
            "Shared counterparty",
            -155_000,
            date(2026, 7, 2),
            None,
            None,
        ),
        transaction(
            "secondary-2",
            "Shared counterparty",
            -7_500,
            date(2026, 7, 6),
            None,
            None,
        ),
        transaction(
            "primary-3",
            "Shared counterparty",
            -162_500,
            date(2026, 8, 3),
            None,
            None,
        ),
    ];

    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 8, 23),
        transactions,
        category_evidence: Vec::new(),
        prevailing_frequency: None,
    })?;

    assert_eq!(output.candidates.len(), 1);
    let candidate = &output.candidates[0];
    assert_eq!(candidate.frequency, Frequency::Monthly);
    assert_eq!(candidate.kind, RecurringKind::Bill);
    assert_eq!(candidate.expected_amount_minor, 162_500);
    assert_eq!(candidate.next_expected_at, date(2026, 9, 1));
    assert_eq!(candidate.occurrence_count, 3);
    assert_eq!(
        candidate.supporting_transaction_ids,
        ["primary-1", "primary-2", "primary-3"]
    );
    Ok(())
}

#[test]
fn recurring_detection_anchor_windows_preserve_series_with_new_outliers() -> EngineResult<()> {
    let transactions = vec![
        transaction(
            "apple-1",
            "APPLE.COM/BILL",
            -2_570,
            date(2026, 5, 20),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "apple-2",
            "APPLE.COM/BILL",
            -2_570,
            date(2026, 6, 22),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "apple-3",
            "APPLE.COM/BILL",
            -2_570,
            date(2026, 7, 20),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "apple-outlier-1",
            "APPLE.COM/BILL",
            -1_343,
            date(2026, 8, 20),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "apple-outlier-2",
            "APPLE.COM/BILL",
            -144,
            date(2026, 8, 20),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
    ];

    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 8, 24),
        transactions,
        category_evidence: Vec::new(),
        prevailing_frequency: None,
    })?;

    assert_eq!(output.candidates.len(), 1);
    let candidate = &output.candidates[0];
    assert_eq!(candidate.frequency, Frequency::Monthly);
    assert_eq!(candidate.expected_amount_minor, 2_570);
    assert_eq!(candidate.next_expected_at, date(2026, 9, 20));
    assert_eq!(candidate.occurrence_count, 3);
    assert_eq!(
        candidate.supporting_transaction_ids,
        ["apple-1", "apple-2", "apple-3"]
    );
    assert_eq!(candidate.observed_amount_minor, Some(1_487));
    assert_eq!(
        candidate.amount_observation_transaction_ids,
        ["apple-outlier-1", "apple-outlier-2"]
    );
    Ok(())
}

#[test]
fn recurring_detection_uses_confirmed_kind_and_learned_category_evidence() -> EngineResult<()> {
    let transactions = vec![
        transaction(
            "insurance-1",
            "Insurance payment",
            -10_000,
            date(2026, 5, 1),
            Some("category-insurance"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "insurance-2",
            "Insurance payment",
            -10_000,
            date(2026, 6, 1),
            Some("category-insurance"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "insurance-3",
            "Insurance payment",
            -10_000,
            date(2026, 7, 1),
            Some("category-insurance"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "new-subscription",
            "New recurring merchant",
            -2_000,
            date(2026, 8, 1),
            Some("category-subscription"),
            Some(CategoryType::Expense),
        ),
    ];

    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 8, 24),
        transactions,
        category_evidence: vec![
            CategoryRecurrenceEvidence {
                category_id: "category-insurance".to_owned(),
                subscription_count: 2,
                bill_count: 1,
                confirmed_subscription_count: 0,
                confirmed_bill_count: 1,
                kind_hint: None,
            },
            CategoryRecurrenceEvidence {
                category_id: "category-subscription".to_owned(),
                subscription_count: 1,
                bill_count: 0,
                confirmed_subscription_count: 0,
                confirmed_bill_count: 0,
                kind_hint: None,
            },
        ],
        prevailing_frequency: Some(Frequency::Monthly),
    })?;

    assert_eq!(output.candidates.len(), 2);
    let insurance = output
        .candidates
        .iter()
        .find(|candidate| candidate.category_id.as_deref() == Some("category-insurance"))
        .expect("insurance candidate should exist");
    assert_eq!(insurance.kind, RecurringKind::Bill);
    assert_eq!(insurance.evidence, RecurringEvidence::Cadence);
    let learned = output
        .candidates
        .iter()
        .find(|candidate| candidate.evidence == RecurringEvidence::LearnedCategory)
        .expect("learned subscription candidate should exist");
    assert_eq!(learned.kind, RecurringKind::Subscription);
    assert_eq!(learned.evidence, RecurringEvidence::LearnedCategory);
    assert_eq!(learned.frequency, Frequency::Monthly);
    assert_eq!(learned.occurrence_count, 1);
    assert_eq!(learned.confidence_basis_points, 3_333);
    Ok(())
}

#[test]
fn recurring_detection_uses_category_kind_hint_for_fixed_bills() -> EngineResult<()> {
    let transactions = vec![
        transaction(
            "cable-1",
            "Shaw Cable TV",
            -12_000,
            date(2026, 5, 1),
            Some("category-utilities"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "cable-2",
            "Shaw Cable TV",
            -12_000,
            date(2026, 6, 1),
            Some("category-utilities"),
            Some(CategoryType::Expense),
        ),
        transaction(
            "cable-3",
            "Shaw Cable TV",
            -12_000,
            date(2026, 7, 1),
            Some("category-utilities"),
            Some(CategoryType::Expense),
        ),
    ];
    let output = detect_recurring(&DetectRecurringInput {
        as_of_date: date(2026, 8, 24),
        transactions,
        category_evidence: vec![CategoryRecurrenceEvidence {
            category_id: "category-utilities".to_owned(),
            subscription_count: 0,
            bill_count: 0,
            confirmed_subscription_count: 0,
            confirmed_bill_count: 0,
            kind_hint: Some(RecurringKind::Bill),
        }],
        prevailing_frequency: None,
    })?;

    assert_eq!(output.candidates.len(), 1);
    assert_eq!(output.candidates[0].kind, RecurringKind::Bill);
    Ok(())
}
