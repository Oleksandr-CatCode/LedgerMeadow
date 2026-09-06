use std::collections::HashSet;

use serde::{Deserialize, Serialize};

use crate::money::{Currency, checked_sub, checked_sum};
use crate::{EngineError, EngineResult};

pub const MAX_ANALYTICS_BUCKETS: usize = 512;
pub const MAX_VALUATIONS: usize = 512;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_LABEL_BYTES: usize = 200;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AnalyticsBucket {
    pub id: String,
    pub label: String,
    pub amount_minor: i64,
    pub item_count: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AnalyticsBreakdownInput {
    pub currency: Currency,
    pub buckets: Vec<AnalyticsBucket>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AnalyticsBreakdownItem {
    pub id: String,
    pub label: String,
    pub amount_minor: i64,
    pub item_count: u32,
    pub share_basis_points: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AnalyticsBreakdownOutput {
    pub currency: Currency,
    pub total_minor: i64,
    pub item_count: u64,
    pub buckets: Vec<AnalyticsBreakdownItem>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Valuation {
    pub id: String,
    pub label: String,
    pub amount_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct NetWorthInput {
    pub currency: Currency,
    pub assets: Vec<Valuation>,
    pub liabilities: Vec<Valuation>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct NetWorthOutput {
    pub currency: Currency,
    pub total_assets_minor: i64,
    pub total_liabilities_minor: i64,
    pub net_worth_minor: i64,
}

pub fn calculate_analytics_breakdown(
    input: &AnalyticsBreakdownInput,
) -> EngineResult<AnalyticsBreakdownOutput> {
    if input.buckets.len() > MAX_ANALYTICS_BUCKETS {
        return Err(EngineError::LimitExceeded {
            resource: "analytics buckets",
            limit: MAX_ANALYTICS_BUCKETS,
        });
    }
    validate_buckets(&input.buckets)?;
    let total_minor = checked_sum(input.buckets.iter().map(|bucket| bucket.amount_minor))?;
    let item_count = input.buckets.iter().try_fold(0_u64, |total, bucket| {
        total
            .checked_add(u64::from(bucket.item_count))
            .ok_or(EngineError::ArithmeticOverflow)
    })?;
    let mut buckets = input.buckets.clone();
    buckets.sort_by(|left, right| {
        right
            .amount_minor
            .cmp(&left.amount_minor)
            .then_with(|| left.id.cmp(&right.id))
    });

    if total_minor == 0 {
        return Ok(AnalyticsBreakdownOutput {
            currency: input.currency,
            total_minor,
            item_count,
            buckets: buckets
                .into_iter()
                .map(|bucket| AnalyticsBreakdownItem {
                    id: bucket.id,
                    label: bucket.label,
                    amount_minor: bucket.amount_minor,
                    item_count: bucket.item_count,
                    share_basis_points: 0,
                })
                .collect(),
        });
    }

    let mut shares = Vec::with_capacity(buckets.len());
    let mut assigned = 0_u32;
    for bucket in &buckets {
        let numerator = i128::from(bucket.amount_minor)
            .checked_mul(10_000)
            .ok_or(EngineError::ArithmeticOverflow)?;
        let divisor = i128::from(total_minor);
        let floor =
            u32::try_from(numerator / divisor).map_err(|_| EngineError::ArithmeticOverflow)?;
        assigned = assigned
            .checked_add(floor)
            .ok_or(EngineError::ArithmeticOverflow)?;
        shares.push((floor, numerator % divisor));
    }
    let mut remainder_order: Vec<usize> = (0..shares.len()).collect();
    remainder_order.sort_by(|left, right| {
        shares[*right]
            .1
            .cmp(&shares[*left].1)
            .then_with(|| buckets[*left].id.cmp(&buckets[*right].id))
    });
    let leftover = 10_000_u32
        .checked_sub(assigned)
        .ok_or(EngineError::ArithmeticOverflow)?;
    for index in remainder_order
        .into_iter()
        .take(usize::try_from(leftover).map_err(|_| EngineError::ArithmeticOverflow)?)
    {
        let share = shares
            .get_mut(index)
            .ok_or(EngineError::ArithmeticOverflow)?;
        share.0 = share
            .0
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }

    let output = buckets
        .into_iter()
        .zip(shares)
        .map(|(bucket, share)| AnalyticsBreakdownItem {
            id: bucket.id,
            label: bucket.label,
            amount_minor: bucket.amount_minor,
            item_count: bucket.item_count,
            share_basis_points: share.0,
        })
        .collect();
    Ok(AnalyticsBreakdownOutput {
        currency: input.currency,
        total_minor,
        item_count,
        buckets: output,
    })
}

pub fn calculate_net_worth(input: &NetWorthInput) -> EngineResult<NetWorthOutput> {
    let count = input
        .assets
        .len()
        .checked_add(input.liabilities.len())
        .ok_or(EngineError::ArithmeticOverflow)?;
    if count > MAX_VALUATIONS {
        return Err(EngineError::LimitExceeded {
            resource: "net-worth valuations",
            limit: MAX_VALUATIONS,
        });
    }
    validate_valuations(&input.assets, &input.liabilities)?;
    let total_assets_minor = checked_sum(input.assets.iter().map(|item| item.amount_minor))?;
    let total_liabilities_minor =
        checked_sum(input.liabilities.iter().map(|item| item.amount_minor))?;
    Ok(NetWorthOutput {
        currency: input.currency,
        total_assets_minor,
        total_liabilities_minor,
        net_worth_minor: checked_sub(total_assets_minor, total_liabilities_minor)?,
    })
}

fn validate_buckets(buckets: &[AnalyticsBucket]) -> EngineResult<()> {
    let mut ids = HashSet::with_capacity(buckets.len());
    for bucket in buckets {
        validate_common(&bucket.id, &bucket.label, bucket.amount_minor)?;
        if !ids.insert(bucket.id.as_str()) {
            return Err(EngineError::InvalidInput(
                "analytics bucket IDs must be unique",
            ));
        }
    }
    Ok(())
}

fn validate_valuations(assets: &[Valuation], liabilities: &[Valuation]) -> EngineResult<()> {
    let mut ids = HashSet::with_capacity(assets.len() + liabilities.len());
    for valuation in assets.iter().chain(liabilities) {
        validate_common(&valuation.id, &valuation.label, valuation.amount_minor)?;
        if !ids.insert(valuation.id.as_str()) {
            return Err(EngineError::InvalidInput("valuation IDs must be unique"));
        }
    }
    Ok(())
}

fn validate_common(id: &str, label: &str, amount_minor: i64) -> EngineResult<()> {
    if id.is_empty()
        || id.len() > MAX_IDENTIFIER_BYTES
        || label.is_empty()
        || label.len() > MAX_LABEL_BYTES
        || amount_minor < 0
    {
        return Err(EngineError::InvalidInput("analytics item is invalid"));
    }
    Ok(())
}
