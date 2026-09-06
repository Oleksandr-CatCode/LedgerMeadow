use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use crate::date::Date;
use crate::money::Currency;
use crate::{EngineError, EngineResult};

pub const MAX_ANALYSIS_TRANSACTIONS: usize = 4_096;
pub const MAX_CATEGORY_CANDIDATES: usize = 200;
pub const MAX_CATEGORIZATION_SIGNALS: usize = 8_192;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_NAME_BYTES: usize = 500;
const MAX_DESCRIPTION_BYTES: usize = 1_000;
const MAX_PROVIDER_CATEGORY_BYTES: usize = 200;

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum CategoryType {
    Income,
    Expense,
    Transfer,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum AccountKind {
    Asset,
    Liability,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AnalysisTransaction {
    pub transaction_id: String,
    pub account_id: String,
    pub name: String,
    pub merchant_name: Option<String>,
    pub original_description: Option<String>,
    pub amount_minor: i64,
    pub currency: Currency,
    pub date: Date,
    pub provider_category_primary: Option<String>,
    pub provider_category_detailed: Option<String>,
    pub account_kind: AccountKind,
    pub category_id: Option<String>,
    pub category_type: Option<CategoryType>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CategoryCandidate {
    pub category_id: String,
    pub name: String,
    pub category_type: CategoryType,
    pub parent_name: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CategorizationSignal {
    pub learning_key: String,
    pub category_id: String,
    pub personal_observation_count: u32,
    pub global_user_count: u32,
    pub global_total_contributor_count: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Categorization {
    pub transaction_id: String,
    pub category_id: Option<String>,
    pub confidence_basis_points: u32,
    pub reason: String,
    pub learning_key: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CategorizeTransactionsInput {
    pub transactions: Vec<AnalysisTransaction>,
    pub categories: Vec<CategoryCandidate>,
    pub signals: Vec<CategorizationSignal>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CategorizeTransactionsOutput {
    pub categorizations: Vec<Categorization>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum MatchSource {
    Personal,
    Global,
    Lexical,
}

struct CategoryMatch<'a> {
    category: &'a CategoryCandidate,
    personal_observation_count: u32,
    global_user_count: u32,
    global_total_contributor_count: u32,
    lexical_score_basis_points: u32,
    source: MatchSource,
}

pub fn categorize_transactions(
    input: &CategorizeTransactionsInput,
) -> EngineResult<CategorizeTransactionsOutput> {
    validate_input(input)?;
    let categories = input
        .categories
        .iter()
        .map(|category| (category.category_id.as_str(), category))
        .collect::<BTreeMap<_, _>>();
    let signals = input
        .signals
        .iter()
        .map(|signal| {
            (
                (signal.learning_key.as_str(), signal.category_id.as_str()),
                signal,
            )
        })
        .collect::<BTreeMap<_, _>>();

    let mut categorizations = Vec::with_capacity(input.transactions.len());
    for transaction in &input.transactions {
        let learning_key = learning_key(transaction);
        if let Some(category_id) = transaction.category_id.as_ref() {
            categorizations.push(Categorization {
                transaction_id: transaction.transaction_id.clone(),
                category_id: Some(category_id.clone()),
                confidence_basis_points: 10_000,
                reason: "existing_category".to_owned(),
                learning_key,
            });
            continue;
        }
        let matched = best_match(transaction, &learning_key, &categories, &signals);
        categorizations.push(Categorization {
            transaction_id: transaction.transaction_id.clone(),
            category_id: matched
                .as_ref()
                .map(|value| value.category.category_id.clone()),
            confidence_basis_points: matched.as_ref().map_or(0, confidence),
            reason: matched.as_ref().map_or_else(
                || "no_match".to_owned(),
                |value| reason(value.source).to_owned(),
            ),
            learning_key,
        });
    }
    Ok(CategorizeTransactionsOutput { categorizations })
}

pub(crate) fn validate_transactions(transactions: &[AnalysisTransaction]) -> EngineResult<()> {
    if transactions.len() > MAX_ANALYSIS_TRANSACTIONS {
        return Err(EngineError::LimitExceeded {
            resource: "analysis transactions",
            limit: MAX_ANALYSIS_TRANSACTIONS,
        });
    }
    let mut transaction_ids = BTreeSet::new();
    for transaction in transactions {
        validate_identifier(&transaction.transaction_id)?;
        validate_identifier(&transaction.account_id)?;
        validate_text(
            &transaction.name,
            MAX_NAME_BYTES,
            "transaction name is invalid",
        )?;
        validate_optional_text(
            transaction.merchant_name.as_deref(),
            MAX_NAME_BYTES,
            "merchant name is invalid",
        )?;
        validate_optional_text(
            transaction.original_description.as_deref(),
            MAX_DESCRIPTION_BYTES,
            "original description is invalid",
        )?;
        validate_optional_text(
            transaction.provider_category_primary.as_deref(),
            MAX_PROVIDER_CATEGORY_BYTES,
            "provider category is invalid",
        )?;
        validate_optional_text(
            transaction.provider_category_detailed.as_deref(),
            MAX_PROVIDER_CATEGORY_BYTES,
            "provider category is invalid",
        )?;
        if let Some(category_id) = transaction.category_id.as_deref() {
            validate_identifier(category_id)?;
            if transaction.category_type.is_none() {
                return Err(EngineError::InvalidInput(
                    "categorized transaction category type is required",
                ));
            }
        }
        transaction.date.validate()?;
        if transaction.amount_minor == 0 || transaction.amount_minor == i64::MIN {
            return Err(EngineError::InvalidInput(
                "transaction amount is outside the supported range",
            ));
        }
        if !transaction_ids.insert(transaction.transaction_id.as_str()) {
            return Err(EngineError::InvalidInput(
                "analysis transaction IDs must be unique",
            ));
        }
    }
    Ok(())
}

pub(crate) fn canonical_source(transaction: &AnalysisTransaction) -> String {
    let source = transaction
        .merchant_name
        .as_deref()
        .unwrap_or(transaction.name.as_str());
    let tokens: Vec<String> = tokens(source)
        .into_iter()
        .filter(|token| !is_noise_token(token))
        .collect();
    if tokens.is_empty() {
        normalize_text(source)
    } else {
        tokens.join(" ")
    }
}

fn validate_input(input: &CategorizeTransactionsInput) -> EngineResult<()> {
    validate_transactions(&input.transactions)?;
    if input.categories.len() > MAX_CATEGORY_CANDIDATES {
        return Err(EngineError::LimitExceeded {
            resource: "category candidates",
            limit: MAX_CATEGORY_CANDIDATES,
        });
    }
    if input.signals.len() > MAX_CATEGORIZATION_SIGNALS {
        return Err(EngineError::LimitExceeded {
            resource: "categorization signals",
            limit: MAX_CATEGORIZATION_SIGNALS,
        });
    }

    let mut category_ids = BTreeSet::new();
    let mut category_types = BTreeMap::new();
    for category in &input.categories {
        validate_identifier(&category.category_id)?;
        validate_text(&category.name, MAX_NAME_BYTES, "category name is invalid")?;
        validate_optional_text(
            category.parent_name.as_deref(),
            MAX_NAME_BYTES,
            "category parent name is invalid",
        )?;
        if !category_ids.insert(category.category_id.as_str()) {
            return Err(EngineError::InvalidInput("category IDs must be unique"));
        }
        category_types.insert(category.category_id.as_str(), category.category_type);
    }
    for transaction in &input.transactions {
        if let Some(category_id) = transaction.category_id.as_deref() {
            let Some(category_type) = category_types.get(category_id) else {
                return Err(EngineError::InvalidInput(
                    "transaction category must reference a category candidate",
                ));
            };
            if Some(*category_type) != transaction.category_type {
                return Err(EngineError::InvalidInput(
                    "transaction category type does not match its category candidate",
                ));
            }
        }
    }

    let learning_keys = input
        .transactions
        .iter()
        .map(learning_key)
        .collect::<BTreeSet<_>>();
    let mut signal_keys = BTreeSet::new();
    for signal in &input.signals {
        validate_learning_key(&signal.learning_key)?;
        validate_identifier(&signal.category_id)?;
        if !learning_keys.contains(&signal.learning_key) {
            return Err(EngineError::InvalidInput(
                "categorization signal must reference a request learning key",
            ));
        }
        if !category_ids.contains(signal.category_id.as_str()) {
            return Err(EngineError::InvalidInput(
                "categorization signal must reference a category candidate",
            ));
        }
        if signal.global_user_count > signal.global_total_contributor_count {
            return Err(EngineError::InvalidInput(
                "categorization global winners cannot exceed contributors",
            ));
        }
        if signal.personal_observation_count == 0 && signal.global_user_count == 0 {
            return Err(EngineError::InvalidInput(
                "categorization signal must contain evidence",
            ));
        }
        if !signal_keys.insert((signal.learning_key.as_str(), signal.category_id.as_str())) {
            return Err(EngineError::InvalidInput(
                "categorization signals must be unique",
            ));
        }
    }
    Ok(())
}

fn best_match<'a>(
    transaction: &AnalysisTransaction,
    learning_key: &str,
    categories: &BTreeMap<&str, &'a CategoryCandidate>,
    signals: &BTreeMap<(&str, &str), &CategorizationSignal>,
) -> Option<CategoryMatch<'a>> {
    let transaction_tokens = analysis_tokens(transaction);
    let mut best = None;
    for category in categories.values() {
        let signal = signals
            .get(&(learning_key, category.category_id.as_str()))
            .copied();
        let personal_observation_count = signal.map_or(0, |value| value.personal_observation_count);
        let global_user_count = signal.map_or(0, |value| value.global_user_count);
        let global_total_contributor_count =
            signal.map_or(0, |value| value.global_total_contributor_count);
        let lexical_score_basis_points = if lexical_type_compatible(transaction, category) {
            lexical_score(category, &transaction_tokens)
        } else {
            0
        };
        let source = if personal_observation_count > 0 {
            MatchSource::Personal
        } else if global_user_count > 0 {
            MatchSource::Global
        } else if lexical_score_basis_points > 0 {
            MatchSource::Lexical
        } else {
            continue;
        };
        let candidate = CategoryMatch {
            category,
            personal_observation_count,
            global_user_count,
            global_total_contributor_count,
            lexical_score_basis_points,
            source,
        };
        if best
            .as_ref()
            .is_none_or(|current| is_better(&candidate, current))
        {
            best = Some(candidate);
        }
    }
    best
}

fn is_better(candidate: &CategoryMatch<'_>, current: &CategoryMatch<'_>) -> bool {
    source_rank(candidate.source)
        .cmp(&source_rank(current.source))
        .then_with(|| {
            candidate
                .personal_observation_count
                .cmp(&current.personal_observation_count)
        })
        .then_with(|| {
            compare_ratio(
                candidate.global_user_count,
                candidate.global_total_contributor_count,
                current.global_user_count,
                current.global_total_contributor_count,
            )
        })
        .then_with(|| candidate.global_user_count.cmp(&current.global_user_count))
        .then_with(|| {
            candidate
                .lexical_score_basis_points
                .cmp(&current.lexical_score_basis_points)
        })
        .then_with(|| {
            current
                .category
                .category_id
                .cmp(&candidate.category.category_id)
        })
        == Ordering::Greater
}

fn compare_ratio(
    left_numerator: u32,
    left_denominator: u32,
    right_numerator: u32,
    right_denominator: u32,
) -> Ordering {
    match (left_denominator, right_denominator) {
        (0, 0) => Ordering::Equal,
        (0, _) => Ordering::Less,
        (_, 0) => Ordering::Greater,
        _ => (u64::from(left_numerator) * u64::from(right_denominator))
            .cmp(&(u64::from(right_numerator) * u64::from(left_denominator))),
    }
}

fn confidence(value: &CategoryMatch<'_>) -> u32 {
    match value.source {
        MatchSource::Personal => 10_000,
        MatchSource::Global => u32::try_from(
            u64::from(value.global_user_count) * 10_000
                / u64::from(value.global_total_contributor_count),
        )
        .unwrap_or(10_000),
        MatchSource::Lexical => value.lexical_score_basis_points,
    }
}

fn reason(source: MatchSource) -> &'static str {
    match source {
        MatchSource::Personal => "personal_history",
        MatchSource::Global => "global_history",
        MatchSource::Lexical => "category_name_overlap",
    }
}

fn source_rank(source: MatchSource) -> u8 {
    match source {
        MatchSource::Personal => 3,
        MatchSource::Global => 2,
        MatchSource::Lexical => 1,
    }
}

fn lexical_score(category: &CategoryCandidate, transaction_tokens: &BTreeSet<String>) -> u32 {
    let category_tokens = [
        Some(category.name.as_str()),
        category.parent_name.as_deref(),
    ]
    .into_iter()
    .flatten()
    .flat_map(tokens)
    .collect::<BTreeSet<_>>();
    let matches = category_tokens.intersection(transaction_tokens).count();
    if matches == 0 || category_tokens.is_empty() {
        return 0;
    }
    let numerator = u64::try_from(matches).unwrap_or(u64::MAX) * 3_000;
    let denominator = u64::try_from(category_tokens.len()).unwrap_or(u64::MAX);
    7_000 + u32::try_from(numerator / denominator).unwrap_or(3_000)
}

fn lexical_type_compatible(
    transaction: &AnalysisTransaction,
    category: &CategoryCandidate,
) -> bool {
    if let Some(category_type) = transaction.category_type {
        return category.category_type == category_type;
    }
    if transaction.account_kind == AccountKind::Asset && transaction.amount_minor > 0 {
        matches!(
            category.category_type,
            CategoryType::Income | CategoryType::Transfer
        )
    } else {
        matches!(
            category.category_type,
            CategoryType::Expense | CategoryType::Transfer
        )
    }
}

fn analysis_tokens(transaction: &AnalysisTransaction) -> BTreeSet<String> {
    [
        Some(transaction.name.as_str()),
        transaction.merchant_name.as_deref(),
        transaction.original_description.as_deref(),
        transaction.provider_category_primary.as_deref(),
        transaction.provider_category_detailed.as_deref(),
    ]
    .into_iter()
    .flatten()
    .flat_map(tokens)
    .filter(|token| !is_noise_token(token))
    .collect()
}

fn learning_key(transaction: &AnalysisTransaction) -> String {
    let mut hasher = Sha256::new();
    for value in [
        "v1".to_owned(),
        canonical_source(transaction),
        normalize_text(
            transaction
                .provider_category_primary
                .as_deref()
                .unwrap_or(""),
        ),
        normalize_text(
            transaction
                .provider_category_detailed
                .as_deref()
                .unwrap_or(""),
        ),
        match transaction.account_kind {
            AccountKind::Asset => "asset".to_owned(),
            AccountKind::Liability => "liability".to_owned(),
        },
        if transaction.amount_minor > 0 {
            "positive".to_owned()
        } else {
            "negative".to_owned()
        },
    ] {
        hasher.update(u64::try_from(value.len()).unwrap_or(u64::MAX).to_be_bytes());
        hasher.update(value.as_bytes());
    }
    format!("{:x}", hasher.finalize())
}

fn validate_learning_key(value: &str) -> EngineResult<()> {
    if value.len() != 64
        || !value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err(EngineError::InvalidInput(
            "categorization learning key is invalid",
        ));
    }
    Ok(())
}

fn tokens(value: &str) -> BTreeSet<String> {
    value
        .split(|character: char| !character.is_alphanumeric())
        .filter(|token| !token.is_empty())
        .map(|token| normalize_token(&token.to_lowercase()))
        .filter(|token| !token.chars().all(|character| character.is_ascii_digit()))
        .collect()
}

fn normalize_token(value: &str) -> String {
    match value {
        "cable" | "electricity" | "hydro" => "utility".to_owned(),
        "fees" => "fee".to_owned(),
        "groceries" => "grocery".to_owned(),
        "payments" => "payment".to_owned(),
        "subscriptions" => "subscription".to_owned(),
        "transfers" => "transfer".to_owned(),
        "utilities" => "utility".to_owned(),
        "wages" => "wage".to_owned(),
        _ => value.to_owned(),
    }
}

fn normalize_text(value: &str) -> String {
    tokens(value).into_iter().collect::<Vec<_>>().join(" ")
}

fn is_noise_token(value: &str) -> bool {
    matches!(
        value,
        "purchase"
            | "payment"
            | "debit"
            | "credit"
            | "preauthorized"
            | "online"
            | "pos"
            | "visa"
            | "mastercard"
            | "inc"
            | "ltd"
            | "llc"
            | "com"
            | "ca"
            | "other"
    )
}

fn validate_identifier(value: &str) -> EngineResult<()> {
    if value.is_empty()
        || value.len() > MAX_IDENTIFIER_BYTES
        || value.trim() != value
        || value.chars().any(char::is_whitespace)
    {
        return Err(EngineError::InvalidInput(
            "analysis transaction identifier is invalid",
        ));
    }
    Ok(())
}

fn validate_optional_text(
    value: Option<&str>,
    maximum_bytes: usize,
    message: &'static str,
) -> EngineResult<()> {
    if let Some(value) = value {
        validate_text(value, maximum_bytes, message)?;
    }
    Ok(())
}

fn validate_text(value: &str, maximum_bytes: usize, message: &'static str) -> EngineResult<()> {
    if value.trim().is_empty()
        || value.len() > maximum_bytes
        || !value.chars().any(char::is_alphanumeric)
    {
        return Err(EngineError::InvalidInput(message));
    }
    Ok(())
}
