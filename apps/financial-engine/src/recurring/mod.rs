use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};

use crate::categorization::{
    AccountKind, AnalysisTransaction, CategoryType, MAX_CATEGORY_CANDIDATES, canonical_source,
    validate_transactions,
};
use crate::date::Date;
use crate::forecast::Frequency;
use crate::money::{Currency, checked_add};
use crate::{EngineError, EngineResult};

const MIN_CADENCE_OCCURRENCES: usize = 2;
const MAX_AMOUNT_VARIANCE_PERCENT: i64 = 35;
const MIN_AMOUNT_VARIANCE_MINOR: i64 = 500;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum RecurringKind {
    Income,
    Bill,
    Subscription,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DetectRecurringInput {
    pub as_of_date: Date,
    pub transactions: Vec<AnalysisTransaction>,
    pub category_evidence: Vec<CategoryRecurrenceEvidence>,
    pub prevailing_frequency: Option<Frequency>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CategoryRecurrenceEvidence {
    pub category_id: String,
    pub subscription_count: u32,
    pub bill_count: u32,
    pub confirmed_subscription_count: u32,
    pub confirmed_bill_count: u32,
    pub kind_hint: Option<RecurringKind>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum RecurringEvidence {
    Cadence,
    LearnedCategory,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RecurringCandidate {
    pub detection_key: String,
    pub name: String,
    pub kind: RecurringKind,
    pub currency: Currency,
    pub frequency: Frequency,
    pub expected_amount_minor: i64,
    pub next_expected_at: Date,
    pub confidence_basis_points: u32,
    pub occurrence_count: u32,
    pub account_id: String,
    pub category_id: Option<String>,
    pub supporting_transaction_ids: Vec<String>,
    pub explanation: String,
    pub evidence: RecurringEvidence,
    pub observed_amount_minor: Option<i64>,
    pub amount_observation_transaction_ids: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DetectRecurringOutput {
    pub candidates: Vec<RecurringCandidate>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
enum CurrencyKey {
    Cad,
    Usd,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
enum Direction {
    Income,
    Outflow,
}

#[derive(Debug, Clone, Copy)]
struct CadenceFit {
    frequency: Frequency,
    total_deviation_days: u64,
    total_allowed_deviation_days: u64,
}

struct SeriesFit<'a> {
    transactions: Vec<&'a AnalysisTransaction>,
    cadence: CadenceFit,
    amount_deviation: i64,
    confidence_basis_points: u32,
}

struct AmountObservation {
    amount_minor: i64,
    transaction_ids: Vec<String>,
}

pub fn detect_recurring(input: &DetectRecurringInput) -> EngineResult<DetectRecurringOutput> {
    input.as_of_date.validate()?;
    validate_transactions(&input.transactions)?;
    if input.category_evidence.len() > MAX_CATEGORY_CANDIDATES {
        return Err(EngineError::LimitExceeded {
            resource: "recurring category evidence",
            limit: MAX_CATEGORY_CANDIDATES,
        });
    }
    let mut evidence_by_category = BTreeMap::new();
    for evidence in &input.category_evidence {
        if evidence.category_id.is_empty()
            || evidence.confirmed_subscription_count > evidence.subscription_count
            || evidence.confirmed_bill_count > evidence.bill_count
            || evidence_by_category
                .insert(evidence.category_id.as_str(), evidence)
                .is_some()
        {
            return Err(EngineError::InvalidInput(
                "recurring category evidence is invalid",
            ));
        }
    }
    for transaction in &input.transactions {
        if transaction.date > input.as_of_date {
            return Err(EngineError::InvalidInput(
                "analysis transaction date cannot follow the as-of date",
            ));
        }
    }

    let mut groups: BTreeMap<(String, Direction, CurrencyKey), Vec<&AnalysisTransaction>> =
        BTreeMap::new();
    for transaction in &input.transactions {
        if (transaction.account_kind == AccountKind::Liability && transaction.amount_minor > 0)
            || transaction.category_type == Some(CategoryType::Transfer)
        {
            continue;
        }
        let direction = if transaction.amount_minor > 0 {
            Direction::Income
        } else {
            Direction::Outflow
        };
        groups
            .entry((
                canonical_source(transaction),
                direction,
                currency_key(transaction.currency),
            ))
            .or_default()
            .push(transaction);
    }

    let mut candidates = Vec::new();
    for ((source, direction, _), mut transactions) in groups {
        transactions.sort_by(|left, right| {
            left.date
                .cmp(&right.date)
                .then_with(|| left.transaction_id.cmp(&right.transaction_id))
        });
        let direction_label = match direction {
            Direction::Income => "income",
            Direction::Outflow => "outflow",
        };
        if let Some(series) = strongest_series(&transactions)? {
            let SeriesFit {
                transactions: stable_transactions,
                cadence,
                amount_deviation,
                confidence_basis_points,
            } = series;
            let first = stable_transactions
                .first()
                .ok_or(EngineError::ArithmeticOverflow)?;
            let last = stable_transactions
                .last()
                .ok_or(EngineError::ArithmeticOverflow)?;
            let category_id = dominant_category_id(&stable_transactions)?;
            let kind = recurring_kind(
                direction,
                amount_deviation,
                category_id.as_deref(),
                &evidence_by_category,
            );
            let occurrence_count = u32::try_from(stable_transactions.len())
                .map_err(|_| EngineError::ArithmeticOverflow)?;
            let expected_amount_minor = absolute_amount(last)?;
            let amount_observation =
                latest_amount_observation(&transactions, &stable_transactions, cadence.frequency)?
                    .filter(|observation| observation.amount_minor != expected_amount_minor);
            candidates.push(RecurringCandidate {
                detection_key: detection_key(first.currency, direction_label, &source),
                name: transaction_name(first),
                kind,
                currency: first.currency,
                frequency: cadence.frequency,
                expected_amount_minor,
                next_expected_at: next_expected_date(
                    first.date,
                    cadence.frequency,
                    input.as_of_date,
                )?,
                confidence_basis_points,
                occurrence_count,
                account_id: common_account_id(&stable_transactions),
                category_id,
                supporting_transaction_ids: supporting_ids(&stable_transactions),
                explanation: format!(
                    "{occurrence_count} posted {direction_label} transactions match a {} cadence with stable amounts",
                    frequency_label(cadence.frequency)
                ),
                evidence: RecurringEvidence::Cadence,
                observed_amount_minor: amount_observation
                    .as_ref()
                    .map(|observation| observation.amount_minor),
                amount_observation_transaction_ids: amount_observation
                    .map(|observation| observation.transaction_ids)
                    .unwrap_or_default(),
            });
            continue;
        }
        if direction != Direction::Outflow {
            continue;
        }
        let category_id = dominant_category_id(&transactions)?;
        let Some(category_evidence) = category_id
            .as_deref()
            .and_then(|category_id| evidence_by_category.get(category_id).copied())
        else {
            continue;
        };
        let (subscription_support, bill_support) = category_support(category_evidence);
        if subscription_support <= bill_support {
            continue;
        }
        let Some(frequency) = learned_frequency(&transactions, input.prevailing_frequency)? else {
            continue;
        };
        let first = transactions
            .first()
            .ok_or(EngineError::ArithmeticOverflow)?;
        let last = transactions.last().ok_or(EngineError::ArithmeticOverflow)?;
        let occurrence_count =
            u32::try_from(transactions.len()).map_err(|_| EngineError::ArithmeticOverflow)?;
        candidates.push(RecurringCandidate {
            detection_key: detection_key(first.currency, direction_label, &source),
            name: transaction_name(first),
            kind: RecurringKind::Subscription,
            currency: first.currency,
            frequency,
            expected_amount_minor: absolute_amount(last)?,
            next_expected_at: next_expected_date(last.date, frequency, input.as_of_date)?,
            confidence_basis_points: learned_confidence(subscription_support, bill_support)?,
            occurrence_count,
            account_id: common_account_id(&transactions),
            category_id,
            supporting_transaction_ids: supporting_ids(&transactions),
            explanation: format!(
                "category recurrence evidence supports subscriptions ({subscription_support} to {bill_support})"
            ),
            evidence: RecurringEvidence::LearnedCategory,
            observed_amount_minor: None,
            amount_observation_transaction_ids: Vec::new(),
        });
    }
    candidates.sort_by(|left, right| left.detection_key.cmp(&right.detection_key));
    Ok(DetectRecurringOutput { candidates })
}

fn latest_amount_observation(
    transactions: &[&AnalysisTransaction],
    stable_transactions: &[&AnalysisTransaction],
    frequency: Frequency,
) -> EngineResult<Option<AmountObservation>> {
    let first = stable_transactions
        .first()
        .ok_or(EngineError::ArithmeticOverflow)?;
    let latest = transactions.last().ok_or(EngineError::ArithmeticOverflow)?;

    let tolerance_days = match frequency {
        Frequency::Once => return Ok(None),
        Frequency::Weekly => 2_u32,
        Frequency::Biweekly => 3,
        Frequency::Monthly => 4,
        Frequency::Quarterly => 7,
        Frequency::Annually => 14,
    };
    let mut occurrence = 0_u32;
    let mut latest_observation = None;
    loop {
        let expected_date = occurrence_date(first.date, frequency, occurrence)?;
        let mut amount_minor = 0_i64;
        let mut transaction_ids = Vec::new();
        for transaction in transactions {
            if expected_date.days_until(transaction.date)?.unsigned_abs() <= tolerance_days {
                amount_minor = checked_add(amount_minor, absolute_amount(transaction)?)?;
                transaction_ids.push(transaction.transaction_id.clone());
            }
        }
        if !transaction_ids.is_empty() {
            latest_observation = Some(AmountObservation {
                amount_minor,
                transaction_ids,
            });
        }
        if expected_date > latest.date {
            break;
        }
        occurrence = occurrence
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }
    Ok(latest_observation)
}

fn strongest_series<'a>(
    transactions: &[&'a AnalysisTransaction],
) -> EngineResult<Option<SeriesFit<'a>>> {
    let mut best: Option<SeriesFit<'a>> = None;
    for mut series in stable_amount_groups(transactions)? {
        if series.len() < MIN_CADENCE_OCCURRENCES {
            continue;
        }
        series.sort_by(|left, right| {
            left.date
                .cmp(&right.date)
                .then_with(|| left.transaction_id.cmp(&right.transaction_id))
        });
        let Some(cadence) = detect_cadence(&series)? else {
            continue;
        };
        let (_, amount_deviation, allowed_amount_deviation) = expected_amount(&series)?;
        if amount_deviation > allowed_amount_deviation {
            continue;
        }
        let confidence_basis_points = confidence(
            cadence,
            amount_deviation,
            allowed_amount_deviation,
            series.len(),
        )?;
        let replace = best.as_ref().is_none_or(|current| {
            series.len() > current.transactions.len()
                || (series.len() == current.transactions.len()
                    && confidence_basis_points > current.confidence_basis_points)
                || (series.len() == current.transactions.len()
                    && confidence_basis_points == current.confidence_basis_points
                    && series.last().map(|value| value.date)
                        > current.transactions.last().map(|value| value.date))
        });
        if replace {
            best = Some(SeriesFit {
                transactions: series,
                cadence,
                amount_deviation,
                confidence_basis_points,
            });
        }
    }
    Ok(best)
}

fn stable_amount_groups<'a>(
    transactions: &[&'a AnalysisTransaction],
) -> EngineResult<Vec<Vec<&'a AnalysisTransaction>>> {
    let mut ordered = Vec::with_capacity(transactions.len());
    for transaction in transactions {
        ordered.push((absolute_amount(transaction)?, *transaction));
    }
    ordered.sort_by(|(left_amount, left), (right_amount, right)| {
        left_amount
            .cmp(right_amount)
            .then_with(|| left.date.cmp(&right.date))
            .then_with(|| left.transaction_id.cmp(&right.transaction_id))
    });

    let mut ranges = BTreeSet::new();
    for (anchor, _) in &ordered {
        let allowed = i64::try_from(
            i128::from(*anchor)
                .checked_mul(i128::from(MAX_AMOUNT_VARIANCE_PERCENT))
                .ok_or(EngineError::ArithmeticOverflow)?
                / 100,
        )
        .map_err(|_| EngineError::ArithmeticOverflow)?
        .max(MIN_AMOUNT_VARIANCE_MINOR);
        let lower = anchor.saturating_sub(allowed);
        let upper = anchor.saturating_add(allowed);
        let lo = ordered.partition_point(|(amount, _)| *amount < lower);
        let hi = ordered.partition_point(|(amount, _)| *amount <= upper);
        ranges.insert((lo, hi));
    }
    Ok(ranges
        .into_iter()
        .map(|(lo, hi)| {
            ordered[lo..hi]
                .iter()
                .map(|(_, transaction)| *transaction)
                .collect()
        })
        .collect())
}

fn detect_cadence(transactions: &[&AnalysisTransaction]) -> EngineResult<Option<CadenceFit>> {
    let candidates = [
        (Frequency::Weekly, 2_u32),
        (Frequency::Biweekly, 3),
        (Frequency::Monthly, 4),
        (Frequency::Quarterly, 7),
        (Frequency::Annually, 14),
    ];
    let mut best: Option<CadenceFit> = None;
    for (frequency, tolerance_days) in candidates {
        let Some(fit) = fit_cadence(transactions, frequency, tolerance_days)? else {
            continue;
        };
        let replace = best.is_none_or(|current| {
            fit.total_deviation_days
                .checked_mul(current.total_allowed_deviation_days)
                .zip(
                    current
                        .total_deviation_days
                        .checked_mul(fit.total_allowed_deviation_days),
                )
                .is_some_and(|(left, right)| {
                    left < right
                        || (left == right
                            && frequency_rank(fit.frequency) < frequency_rank(current.frequency))
                })
        });
        if replace {
            best = Some(fit);
        }
    }
    Ok(best)
}

fn fit_cadence(
    transactions: &[&AnalysisTransaction],
    frequency: Frequency,
    tolerance_days: u32,
) -> EngineResult<Option<CadenceFit>> {
    let mut total_deviation_days = 0_u64;
    for pair in transactions.windows(2) {
        let expected = occurrence_date(pair[0].date, frequency, 1)?;
        let deviation = u64::from(expected.days_until(pair[1].date)?.unsigned_abs());
        if deviation > u64::from(tolerance_days) {
            return Ok(None);
        }
        total_deviation_days = total_deviation_days
            .checked_add(deviation)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }
    let interval_count = transactions
        .len()
        .checked_sub(1)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let total_allowed_deviation_days = u64::try_from(interval_count)
        .map_err(|_| EngineError::ArithmeticOverflow)?
        .checked_mul(u64::from(tolerance_days))
        .ok_or(EngineError::ArithmeticOverflow)?;
    Ok(Some(CadenceFit {
        frequency,
        total_deviation_days,
        total_allowed_deviation_days,
    }))
}

fn expected_amount(transactions: &[&AnalysisTransaction]) -> EngineResult<(i64, i64, i64)> {
    let mut total = 0_i64;
    let mut amounts = Vec::with_capacity(transactions.len());
    for transaction in transactions {
        let amount = absolute_amount(transaction)?;
        total = checked_add(total, amount)?;
        amounts.push(amount);
    }
    let count = i64::try_from(amounts.len()).map_err(|_| EngineError::ArithmeticOverflow)?;
    let expected = total / count;
    if expected <= 0 {
        return Err(EngineError::InvalidInput(
            "recurring expected amount must be positive",
        ));
    }
    let maximum_deviation = amounts
        .into_iter()
        .map(|amount| amount.abs_diff(expected))
        .max()
        .unwrap_or(0);
    let maximum_deviation =
        i64::try_from(maximum_deviation).map_err(|_| EngineError::ArithmeticOverflow)?;
    let percentage_deviation = i128::from(expected)
        .checked_mul(i128::from(MAX_AMOUNT_VARIANCE_PERCENT))
        .ok_or(EngineError::ArithmeticOverflow)?
        / 100;
    let allowed_deviation = i64::try_from(percentage_deviation)
        .map_err(|_| EngineError::ArithmeticOverflow)?
        .max(MIN_AMOUNT_VARIANCE_MINOR);
    Ok((expected, maximum_deviation, allowed_deviation))
}

fn absolute_amount(transaction: &AnalysisTransaction) -> EngineResult<i64> {
    if transaction.amount_minor > 0 {
        Ok(transaction.amount_minor)
    } else {
        transaction
            .amount_minor
            .checked_neg()
            .ok_or(EngineError::ArithmeticOverflow)
    }
}

fn dominant_category_id(transactions: &[&AnalysisTransaction]) -> EngineResult<Option<String>> {
    let mut counts = BTreeMap::<&str, u32>::new();
    for transaction in transactions {
        if let Some(category_id) = transaction.category_id.as_deref() {
            let count = counts.entry(category_id).or_default();
            *count = count
                .checked_add(1)
                .ok_or(EngineError::ArithmeticOverflow)?;
        }
    }
    Ok(counts
        .into_iter()
        .max_by(
            |(left_category, left_count), (right_category, right_count)| {
                left_count
                    .cmp(right_count)
                    .then_with(|| right_category.cmp(left_category))
            },
        )
        .map(|(category_id, _)| category_id.to_owned()))
}

fn recurring_kind(
    direction: Direction,
    amount_deviation: i64,
    category_id: Option<&str>,
    evidence_by_category: &BTreeMap<&str, &CategoryRecurrenceEvidence>,
) -> RecurringKind {
    if direction == Direction::Income {
        return RecurringKind::Income;
    }
    if let Some(evidence) = category_id.and_then(|id| evidence_by_category.get(id).copied()) {
        if evidence.confirmed_subscription_count != evidence.confirmed_bill_count {
            return if evidence.confirmed_subscription_count > evidence.confirmed_bill_count {
                RecurringKind::Subscription
            } else {
                RecurringKind::Bill
            };
        }
        if let Some(kind) = evidence.kind_hint {
            return kind;
        }
        let (subscription_support, bill_support) = category_support(evidence);
        if subscription_support > bill_support {
            return RecurringKind::Subscription;
        }
        if bill_support > subscription_support {
            return RecurringKind::Bill;
        }
    }
    if amount_deviation == 0 {
        RecurringKind::Subscription
    } else {
        RecurringKind::Bill
    }
}

fn category_support(evidence: &CategoryRecurrenceEvidence) -> (u32, u32) {
    if evidence.confirmed_subscription_count != evidence.confirmed_bill_count {
        (
            evidence.confirmed_subscription_count,
            evidence.confirmed_bill_count,
        )
    } else if evidence.confirmed_subscription_count > 0 {
        (evidence.subscription_count, evidence.bill_count)
    } else {
        (evidence.subscription_count, evidence.bill_count)
    }
}

fn learned_frequency(
    transactions: &[&AnalysisTransaction],
    prevailing_frequency: Option<Frequency>,
) -> EngineResult<Option<Frequency>> {
    if transactions.len() == 1 {
        return Ok(prevailing_frequency.filter(|frequency| *frequency != Frequency::Once));
    }
    let mut gaps = Vec::with_capacity(transactions.len() - 1);
    for pair in transactions.windows(2) {
        gaps.push(
            u32::try_from(pair[0].date.days_until(pair[1].date)?)
                .map_err(|_| EngineError::ArithmeticOverflow)?,
        );
    }
    gaps.sort_unstable();
    let median = if gaps.len() % 2 == 0 {
        gaps[gaps.len() / 2 - 1]
            .checked_add(gaps[gaps.len() / 2])
            .ok_or(EngineError::ArithmeticOverflow)?
            / 2
    } else {
        gaps[gaps.len() / 2]
    };
    Ok([
        (Frequency::Weekly, 7_u32),
        (Frequency::Biweekly, 14),
        (Frequency::Monthly, 30),
        (Frequency::Quarterly, 91),
        (Frequency::Annually, 365),
    ]
    .into_iter()
    .min_by_key(|(frequency, days)| (days.abs_diff(median), frequency_rank(*frequency)))
    .map(|(frequency, _)| frequency))
}

fn learned_confidence(subscription_support: u32, bill_support: u32) -> EngineResult<u32> {
    let total = subscription_support
        .checked_add(bill_support)
        .ok_or(EngineError::ArithmeticOverflow)?;
    if total == 0 {
        return Err(EngineError::ArithmeticOverflow);
    }
    let purity = u64::from(subscription_support)
        .checked_mul(10_000)
        .ok_or(EngineError::ArithmeticOverflow)?
        / u64::from(total);
    let confidence = purity
        .checked_mul(u64::from(total.min(3)))
        .ok_or(EngineError::ArithmeticOverflow)?
        / 3;
    u32::try_from(confidence).map_err(|_| EngineError::ArithmeticOverflow)
}

fn detection_key(currency: Currency, direction: &str, source: &str) -> String {
    format!(
        "{}:{direction}:{}",
        currency_label(currency),
        source.replace(' ', "-")
    )
}

fn transaction_name(transaction: &AnalysisTransaction) -> String {
    transaction
        .merchant_name
        .as_deref()
        .unwrap_or(transaction.name.as_str())
        .trim()
        .to_owned()
}

fn supporting_ids(transactions: &[&AnalysisTransaction]) -> Vec<String> {
    transactions
        .iter()
        .map(|transaction| transaction.transaction_id.clone())
        .collect()
}

fn common_account_id(transactions: &[&AnalysisTransaction]) -> String {
    let Some(first) = transactions.first() else {
        return String::new();
    };
    if transactions
        .iter()
        .all(|transaction| transaction.account_id == first.account_id)
    {
        first.account_id.clone()
    } else {
        String::new()
    }
}

fn confidence(
    cadence: CadenceFit,
    amount_deviation: i64,
    allowed_amount_deviation: i64,
    occurrence_count: usize,
) -> EngineResult<u32> {
    let cadence_penalty = if cadence.total_allowed_deviation_days == 0 {
        0
    } else {
        cadence
            .total_deviation_days
            .checked_mul(2_000)
            .ok_or(EngineError::ArithmeticOverflow)?
            / cadence.total_allowed_deviation_days
    };
    let amount_penalty = i128::from(amount_deviation)
        .checked_mul(1_500)
        .ok_or(EngineError::ArithmeticOverflow)?
        / i128::from(allowed_amount_deviation);
    let occurrence_adjustment = if occurrence_count < 3 {
        i64::try_from(3_usize - occurrence_count)
            .map_err(|_| EngineError::ArithmeticOverflow)?
            .checked_mul(-2_000)
            .ok_or(EngineError::ArithmeticOverflow)?
    } else {
        i64::try_from(occurrence_count - 3)
            .map_err(|_| EngineError::ArithmeticOverflow)?
            .checked_mul(200)
            .ok_or(EngineError::ArithmeticOverflow)?
            .min(500)
    };
    let cadence_score = 2_000_u32
        .checked_sub(u32::try_from(cadence_penalty).map_err(|_| EngineError::ArithmeticOverflow)?)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let amount_score = 1_500_u32
        .checked_sub(u32::try_from(amount_penalty).map_err(|_| EngineError::ArithmeticOverflow)?)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let score = 6_000_i64
        .checked_add(i64::from(cadence_score))
        .and_then(|value| value.checked_add(i64::from(amount_score)))
        .and_then(|value| value.checked_add(occurrence_adjustment))
        .ok_or(EngineError::ArithmeticOverflow)?;
    u32::try_from(score).map_err(|_| EngineError::ArithmeticOverflow)
}

fn next_expected_date(first: Date, frequency: Frequency, as_of_date: Date) -> EngineResult<Date> {
    let mut occurrence = initial_occurrence(first, frequency, as_of_date)?;
    loop {
        let candidate = occurrence_date(first, frequency, occurrence)?;
        if candidate > as_of_date {
            return Ok(candidate);
        }
        occurrence = occurrence
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }
}

fn initial_occurrence(first: Date, frequency: Frequency, as_of_date: Date) -> EngineResult<u32> {
    let days = first.days_until(as_of_date)?;
    if days < 0 {
        return Ok(0);
    }
    match frequency {
        Frequency::Weekly => u32::try_from(days / 7).map_err(|_| EngineError::ArithmeticOverflow),
        Frequency::Biweekly => {
            u32::try_from(days / 14).map_err(|_| EngineError::ArithmeticOverflow)
        }
        Frequency::Monthly | Frequency::Quarterly | Frequency::Annually => {
            let month_difference = i64::from(as_of_date.year - first.year)
                .checked_mul(12)
                .and_then(|value| {
                    value.checked_add(i64::from(as_of_date.month) - i64::from(first.month))
                })
                .ok_or(EngineError::ArithmeticOverflow)?;
            let step = match frequency {
                Frequency::Monthly => 1,
                Frequency::Quarterly => 3,
                Frequency::Annually => 12,
                _ => return Err(EngineError::InvalidInput("recurring frequency is invalid")),
            };
            u32::try_from(month_difference.max(0) / step)
                .map_err(|_| EngineError::ArithmeticOverflow)
        }
        Frequency::Once => Err(EngineError::InvalidInput(
            "one-time frequency is not recurring",
        )),
    }
}

fn occurrence_date(first: Date, frequency: Frequency, occurrence: u32) -> EngineResult<Date> {
    match frequency {
        Frequency::Weekly => first.add_days(
            occurrence
                .checked_mul(7)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Biweekly => first.add_days(
            occurrence
                .checked_mul(14)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Monthly => first.add_months(occurrence),
        Frequency::Quarterly => first.add_months(
            occurrence
                .checked_mul(3)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Annually => first.add_months(
            occurrence
                .checked_mul(12)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Once => Err(EngineError::InvalidInput(
            "one-time frequency is not recurring",
        )),
    }
}

fn currency_key(currency: Currency) -> CurrencyKey {
    match currency {
        Currency::Cad => CurrencyKey::Cad,
        Currency::Usd => CurrencyKey::Usd,
    }
}

fn currency_label(currency: Currency) -> &'static str {
    match currency {
        Currency::Cad => "CAD",
        Currency::Usd => "USD",
    }
}

fn frequency_rank(frequency: Frequency) -> u8 {
    match frequency {
        Frequency::Weekly => 0,
        Frequency::Biweekly => 1,
        Frequency::Monthly => 2,
        Frequency::Quarterly => 3,
        Frequency::Annually => 4,
        Frequency::Once => 5,
    }
}

fn frequency_label(frequency: Frequency) -> &'static str {
    match frequency {
        Frequency::Weekly => "weekly",
        Frequency::Biweekly => "biweekly",
        Frequency::Monthly => "monthly",
        Frequency::Quarterly => "quarterly",
        Frequency::Annually => "annual",
        Frequency::Once => "one-time",
    }
}
