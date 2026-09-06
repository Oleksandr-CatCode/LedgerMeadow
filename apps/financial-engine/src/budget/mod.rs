use serde::{Deserialize, Serialize};

use crate::money::{Currency, mul_div_ceil_nonnegative, mul_div_floor_nonnegative};
use crate::{EngineError, EngineResult};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum BudgetStatus {
    OnTrack,
    Fast,
    Critical,
    Exceeded,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct BudgetStatusInput {
    pub currency: Currency,
    pub limit_minor: i64,
    pub spent_minor: i64,
    pub warning_threshold_percent: u8,
    pub critical_threshold_percent: u8,
    pub elapsed_units: u32,
    pub period_units: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct BudgetStatusOutput {
    pub currency: Currency,
    pub status: BudgetStatus,
    pub remaining_minor: i64,
    pub expected_spend_minor: i64,
    pub projected_spend_minor: i64,
    pub used_basis_points: i64,
    pub elapsed_basis_points: i64,
}

pub fn calculate_budget_status(input: &BudgetStatusInput) -> EngineResult<BudgetStatusOutput> {
    if input.limit_minor <= 0
        || input.spent_minor < 0
        || input.warning_threshold_percent == 0
        || input.warning_threshold_percent > 100
        || input.critical_threshold_percent < input.warning_threshold_percent
        || input.critical_threshold_percent > 100
        || input.period_units == 0
        || input.elapsed_units > input.period_units
    {
        return Err(EngineError::InvalidInput(
            "budget inputs are outside the supported range",
        ));
    }

    let used_basis_points =
        mul_div_floor_nonnegative(input.spent_minor, 10_000, input.limit_minor)?;
    let elapsed_basis_points = mul_div_floor_nonnegative(
        i64::from(input.elapsed_units),
        10_000,
        i64::from(input.period_units),
    )?;
    let expected_spend_minor = mul_div_floor_nonnegative(
        input.limit_minor,
        i64::from(input.elapsed_units),
        i64::from(input.period_units),
    )?;
    let projected_spend_minor = if input.elapsed_units == 0 {
        0
    } else {
        mul_div_ceil_nonnegative(
            input.spent_minor,
            i64::from(input.period_units),
            i64::from(input.elapsed_units),
        )?
    };
    let warning_basis_points = i64::from(input.warning_threshold_percent) * 100;
    let critical_basis_points = i64::from(input.critical_threshold_percent) * 100;
    let status = if input.spent_minor >= input.limit_minor {
        BudgetStatus::Exceeded
    } else if used_basis_points >= critical_basis_points {
        BudgetStatus::Critical
    } else if used_basis_points >= warning_basis_points || projected_spend_minor > input.limit_minor
    {
        BudgetStatus::Fast
    } else {
        BudgetStatus::OnTrack
    };

    Ok(BudgetStatusOutput {
        currency: input.currency,
        status,
        remaining_minor: input
            .limit_minor
            .checked_sub(input.spent_minor)
            .ok_or(EngineError::ArithmeticOverflow)?,
        expected_spend_minor,
        projected_spend_minor,
        used_basis_points,
        elapsed_basis_points,
    })
}
